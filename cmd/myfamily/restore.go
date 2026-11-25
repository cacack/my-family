package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/cacack/my-family/internal/config"
	"github.com/cacack/my-family/internal/storage"
)

const restoreUsage = "usage: myfamily restore PATH"

// runRestore runs `myfamily restore PATH` (#1642).
func runRestore(args []string, out io.Writer) error {
	if len(args) != 1 || args[0] == "" {
		return errors.New(restoreUsage)
	}
	// Ctrl-C during the load rolls it back; during the replay, the saved
	// rebuild state keeps the read model flagged until `myfamily rebuild`.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return restoreBackup(ctx, config.Load(), args[0], out)
}

// restoreBackup restores the archive at path into cfg's empty database.
func restoreBackup(ctx context.Context, cfg *config.Config, path string, out io.Writer) error {
	result, err := storage.Restore(ctx, cfg, path)
	if err != nil {
		return err
	}
	m := result.Manifest
	fmt.Fprintf(out, "Restored %s into %s, written by my-family %s at %s: %d events, last log position %d.\n",
		path, result.Target, m.AppVersion, m.CreatedAt, m.EventCount, m.LastPosition)
	fmt.Fprintln(out, "Everyone signs in again; pending invites and password reset links were not kept and must be issued again.")
	return nil
}
