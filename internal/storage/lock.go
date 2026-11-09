package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
)

// Access is what a process opens the database for. Open takes a lock for it
// that lasts until Close, so `myfamily rebuild` never runs while `serve` does
// against the same database, and the other way round (#1232).
type Access int

const (
	// AccessServe shares the lock: any number of servers may hold it at once.
	AccessServe Access = iota
	// AccessRebuild holds the lock alone. It is the only access under which
	// Stores.RebuildReadModel runs.
	AccessRebuild
)

var (
	// ErrServerRunning is returned by Open for AccessRebuild while a server
	// holds the database.
	ErrServerRunning = errors.New("a myfamily server is running against this database; stop it first")
	// ErrRebuildRunning is returned by Open while a rebuild holds the database.
	ErrRebuildRunning = errors.New("a myfamily rebuild is in progress on this database; wait for it to finish")
	// ErrRebuildNotExclusive is returned by RebuildReadModel on stores not
	// opened for AccessRebuild.
	ErrRebuildNotExclusive = errors.New("the read model can only be rebuilt from stores opened for rebuild (storage.AccessRebuild)")
)

// dbLock is a shared/exclusive lock on one database that is released when
// the process exits, however it exits.
type dbLock interface {
	// tryLock takes the lock in the given mode without waiting, reporting
	// whether it got it.
	tryLock(exclusive bool) (bool, error)
	// unlock releases a lock tryLock took.
	unlock() error
	// close releases the lock, if held, and what it is built on.
	close() error
}

// acquire takes l for access, or explains who holds it.
func acquire(l dbLock, access Access) error {
	exclusive := access == AccessRebuild
	ok, err := l.tryLock(exclusive)
	if err != nil {
		return fmt.Errorf("lock database: %w", err)
	}
	if ok {
		return nil
	}
	if !exclusive {
		return ErrRebuildRunning
	}
	// Servers hold the lock shared, a rebuild exclusively: if a shared lock
	// can be had, the holders are servers.
	if shared, err := l.tryLock(false); err == nil && shared {
		if err := l.unlock(); err != nil {
			return fmt.Errorf("lock database: %w", err)
		}
		return ErrServerRunning
	}
	return ErrRebuildRunning
}

// fileLock locks a file next to a SQLite database (SQLite's own locks only
// last for a transaction). The file is left in place: removing it would let
// two processes lock different files of the same name.
type fileLock struct {
	f *os.File
}

func openFileLock(path string) (*fileLock, error) {
	// #nosec G304 -- path is the operator's SQLITE_PATH plus ".lock".
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, err
	}
	return &fileLock{f: f}, nil
}

func (l *fileLock) close() error {
	return l.f.Close() // closing the file releases its lock
}

// advisoryLockKey is the PostgreSQL advisory lock key for the database, the
// ASCII of "myfamily". Advisory locks are scoped to the current database.
const advisoryLockKey int64 = 0x6d7966616d696c79

// advisoryLock holds a PostgreSQL session-level advisory lock on a connection
// set aside for it; the server releases it when that connection ends.
type advisoryLock struct {
	conn      *sql.Conn
	exclusive bool
	held      bool
}

func (l *advisoryLock) tryLock(exclusive bool) (bool, error) {
	query := "SELECT pg_try_advisory_lock_shared($1)"
	if exclusive {
		query = "SELECT pg_try_advisory_lock($1)"
	}
	var ok bool
	if err := l.conn.QueryRowContext(context.Background(), query, advisoryLockKey).Scan(&ok); err != nil {
		return false, err
	}
	if ok {
		l.exclusive, l.held = exclusive, true
	}
	return ok, nil
}

func (l *advisoryLock) unlock() error {
	query := "SELECT pg_advisory_unlock_shared($1)"
	if l.exclusive {
		query = "SELECT pg_advisory_unlock($1)"
	}
	if _, err := l.conn.ExecContext(context.Background(), query, advisoryLockKey); err != nil {
		return err
	}
	l.held = false
	return nil
}

func (l *advisoryLock) close() error {
	var err error
	if l.held {
		err = l.unlock()
	}
	// The connection goes back to the pool, which closes it with the database.
	return errors.Join(err, l.conn.Close())
}
