package api

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/cacack/my-family/internal/backup"
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
		if errors.Is(err, backup.ErrReadModelOnlyValues) {
			// The owner can fix this one, so it is a refusal naming the fix
			// (#1830), not a failed download.
			if err := errors.Join(f.Close(), os.Remove(f.Name())); err != nil {
				return nil, err
			}
			return DownloadBackup409JSONResponse{
				Code: "backup_needs_rebuild",
				Message: "This database holds coordinates, external IDs, brick walls or snapshots that its history " +
					"does not record, so a backup would lose them. Stop every server and run \"myfamily rebuild\", " +
					"which records them, then back up again.",
			}, nil
		}
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
