package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/cacack/my-family/internal/backup"
	"github.com/cacack/my-family/internal/config"
	"github.com/cacack/my-family/internal/storage"
)

const (
	backupUsage       = "usage: myfamily backup PATH"
	verifyBackupUsage = "usage: myfamily verify-backup PATH"
)

// runBackup runs `myfamily backup PATH` (#15).
func runBackup(args []string, out io.Writer) error {
	if len(args) != 1 || args[0] == "" {
		return errors.New(backupUsage)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return writeBackup(ctx, config.Load(), args[0], out)
}

// writeBackup writes a backup archive of cfg's database to path, which must
// not exist yet, so an earlier backup is never overwritten. A failed backup
// removes what it wrote: a file left at path is a whole archive.
func writeBackup(ctx context.Context, cfg *config.Config, path string, out io.Writer) error {
	// #nosec G304 G703 -- path is the file the operator asked for.
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("create backup: %w", err)
	}
	m, err := storage.Backup(ctx, cfg, f, version)
	if err == nil {
		err = f.Sync()
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return errors.Join(err, os.Remove(path)) // #nosec G703 -- the file this call created.
	}
	fmt.Fprintf(out, "Wrote %s: %d events, last log position %d.\n", path, m.EventCount, m.LastPosition)
	fmt.Fprintln(out, "It holds the whole tree, living people included, and every account's password hash: keep it as safe as the database.")
	return nil
}

// runVerifyBackup runs `myfamily verify-backup PATH`: it exits non-zero,
// naming the damaged entry, unless the archive is intact.
func runVerifyBackup(args []string, out io.Writer) error {
	if len(args) != 1 || args[0] == "" {
		return errors.New(verifyBackupUsage)
	}
	m, err := backup.VerifyFile(args[0])
	if err != nil {
		return fmt.Errorf("verify %s: %w", args[0], err)
	}
	fmt.Fprintf(out, "%s is intact: %d events, last log position %d, written by my-family %s at %s.\n",
		args[0], m.EventCount, m.LastPosition, m.AppVersion, m.CreatedAt)
	return nil
}
