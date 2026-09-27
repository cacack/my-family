package sqlite_test

import (
	"bytes"
	"context"
	"os"
	"testing"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/repository/sqlite"
)

// TestSaveMediaStoresBinaryAsBlob guards the storage class of the media byte
// columns (#759 review): file and thumbnail bytes must bind as BLOB, never as
// TEXT, so binary data with NUL bytes and invalid UTF-8 is stored intact. It
// covers a main upload, a main re-save (the upsert path) and a branch-owned
// upload.
func TestSaveMediaStoresBinaryAsBlob(t *testing.T) {
	ctx := context.Background()
	tmpFile, err := os.CreateTemp("", "myfamily-media-blob-*.db")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	tmpFile.Close()
	t.Cleanup(func() { os.Remove(tmpFile.Name()) })

	db, err := sqlite.OpenDB(tmpFile.Name())
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	readStore, err := sqlite.NewReadModelStore(db)
	if err != nil {
		t.Fatalf("create read model store: %v", err)
	}
	branchStore, err := sqlite.NewBranchStore(db)
	if err != nil {
		t.Fatalf("create branch store: %v", err)
	}
	projector := repository.NewProjector(readStore, branchStore)

	// A JPEG-like header with an embedded NUL and bytes that are not valid UTF-8.
	file := []byte{0xff, 0xd8, 0x00, 0x01, 0xfe, 0x00, 0x80, 0xc3}
	thumb := []byte{0x89, 'P', 'N', 'G', 0x00, 0x1a, 0xff, 0x00}

	person := domain.NewPerson("Alex", "Binary")
	media := domain.NewMedia("scan", "person", person.ID)
	media.MimeType = "image/jpeg"
	media.MediaType = domain.MediaPhoto
	media.Filename = "scan.jpg"
	media.FileData = file
	media.ThumbnailData = thumb
	media.FileSize = int64(len(file))

	branch, err := domain.NewBranch("blob-check", "", 0)
	if err != nil {
		t.Fatalf("NewBranch: %v", err)
	}
	branchID := domain.BranchID(branch.ID)
	for i, ev := range []domain.Event{
		domain.NewPersonCreated(person),
		domain.NewMediaCreated(media),
		domain.NewBranchCreated(branch),
	} {
		if err := projector.Project(ctx, ev, int64(i+1), domain.MainBranchID); err != nil {
			t.Fatalf("project %s: %v", ev.EventType(), err)
		}
	}

	branchMedia := domain.NewMedia("branch scan", "person", person.ID)
	branchMedia.MimeType = "image/jpeg"
	branchMedia.MediaType = domain.MediaPhoto
	branchMedia.Filename = "branch.jpg"
	branchMedia.FileData = file
	branchMedia.ThumbnailData = thumb
	branchMedia.FileSize = int64(len(file))
	if err := projector.Project(ctx, domain.NewMediaCreated(branchMedia), 4, branchID); err != nil {
		t.Fatalf("project branch upload: %v", err)
	}

	check := func(label string, branch domain.BranchID, id uuid.UUID) {
		t.Helper()
		var fileType, thumbType string
		var fileLen, thumbLen int
		err := db.QueryRow(`SELECT typeof(file_data), typeof(thumbnail_data), length(file_data), length(thumbnail_data)
			FROM media WHERE id = ? AND branch_id = ?`, id.String(), branch.String()).Scan(&fileType, &thumbType, &fileLen, &thumbLen)
		if err != nil {
			t.Fatalf("%s: read storage class: %v", label, err)
		}
		if fileType != "blob" || thumbType != "blob" {
			t.Errorf("%s: typeof(file_data, thumbnail_data) = %s, %s; want blob, blob", label, fileType, thumbType)
		}
		if fileLen != len(file) || thumbLen != len(thumb) {
			t.Errorf("%s: stored lengths = %d, %d; want %d, %d", label, fileLen, thumbLen, len(file), len(thumb))
		}
		got, err := readStore.GetMediaWithData(ctx, branch, id)
		if err != nil || got == nil {
			t.Fatalf("%s: GetMediaWithData = %+v (err=%v)", label, got, err)
		}
		if !bytes.Equal(got.FileData, file) || !bytes.Equal(got.ThumbnailData, thumb) {
			t.Errorf("%s: round-tripped bytes = %x / %x; want %x / %x", label, got.FileData, got.ThumbnailData, file, thumb)
		}
	}
	check("main upload", domain.MainBranchID, media.ID)
	check("branch upload", branchID, branchMedia.ID)

	// Re-saving the main row takes the ON CONFLICT path; the bytes must stay BLOB.
	rm, err := readStore.GetMediaWithData(ctx, domain.MainBranchID, media.ID)
	if err != nil || rm == nil {
		t.Fatalf("GetMediaWithData before re-save = %+v (err=%v)", rm, err)
	}
	rm.Title = "scan (re-saved)"
	if err := readStore.SaveMedia(ctx, domain.MainBranchID, rm); err != nil {
		t.Fatalf("re-save main media: %v", err)
	}
	check("main re-save", domain.MainBranchID, media.ID)
}
