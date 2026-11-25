package api

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"time"
)

// BackupWriter writes a backup archive of the instance to w (ADR-010), as
// storage.Stores.Backup does.
type BackupWriter func(ctx context.Context, w io.Writer) error

// WithBackup supplies what writes the owner's backup download (#15). It is
// optional: without it, as in demo mode, the download is refused.
func WithBackup(write BackupWriter) ServerOption {
	return func(s *Server) {
		s.backup = write
	}
}

// DownloadBackup implements StrictServerInterface. The archive is written to
// a temporary file first, so a backup that fails is an error response rather
// than a cut-off download, and the file is removed once it has been sent.
func (ss *StrictServer) DownloadBackup(ctx context.Context, _ DownloadBackupRequestObject) (DownloadBackupResponseObject, error) {
	if ss.server.backup == nil {
		return DownloadBackup409JSONResponse{
			Code:    "backup_unavailable",
			Message: "This server runs in memory (DEMO_MODE) and keeps nothing to back up",
		}, nil
	}
	f, err := os.CreateTemp("", "myfamily-backup-*.zip")
	if err != nil {
		return nil, fmt.Errorf("create backup file: %w", err)
	}
	discard := func(err error) (DownloadBackupResponseObject, error) {
		return nil, errors.Join(err, f.Close(), os.Remove(f.Name()))
	}
	if err := ss.server.backup(ctx, f); err != nil {
		return discard(fmt.Errorf("write backup: %w", err))
	}
	size, err := f.Seek(0, io.SeekCurrent)
	if err != nil {
		return discard(err)
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return discard(err)
	}
	name := "myfamily-backup-" + time.Now().UTC().Format("20060102T150405Z") + ".zip"
	return DownloadBackup200ApplicationzipResponse{
		Body:          &removeOnClose{f},
		ContentLength: size,
		Headers:       DownloadBackup200ResponseHeaders{ContentDisposition: strPtr(`attachment; filename="` + name + `"`)},
	}, nil
}

// removeOnClose is a temporary file that is deleted when it is closed.
type removeOnClose struct{ *os.File }

func (f *removeOnClose) Close() error {
	return errors.Join(f.File.Close(), os.Remove(f.Name()))
}
