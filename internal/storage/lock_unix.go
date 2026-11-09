//go:build !windows

package storage

import (
	"errors"
	"syscall"
)

func (l *fileLock) tryLock(exclusive bool) (bool, error) {
	how := syscall.LOCK_SH
	if exclusive {
		how = syscall.LOCK_EX
	}
	err := syscall.Flock(int(l.f.Fd()), how|syscall.LOCK_NB) // #nosec G115 -- a file descriptor fits in an int
	if errors.Is(err, syscall.EWOULDBLOCK) {
		return false, nil
	}
	return err == nil, err
}

func (l *fileLock) unlock() error {
	return syscall.Flock(int(l.f.Fd()), syscall.LOCK_UN) // #nosec G115 -- a file descriptor fits in an int
}
