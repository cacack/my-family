package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
	pgstore "github.com/cacack/my-family/internal/repository/postgres"
)

// TestMediaWritesTakeTheBlobLock proves the media writes whose checks read other
// branches' rows serialize on the media advisory lock (#759 review): while
// another transaction holds it, SaveMedia on a branch and DeleteMedia on main
// both wait, and both finish once it is released. Without the lock a branch
// shadow could commit between a main delete's live-shadow check and its commit,
// leaving the shadow with no bytes.
func TestMediaWritesTakeTheBlobLock(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	db, cleanup := setupPostgres(t)
	defer cleanup()
	ctx := context.Background()

	readStore, err := pgstore.NewReadModelStore(db)
	if err != nil {
		t.Fatalf("create read model store: %v", err)
	}
	branchStore, err := pgstore.NewBranchStore(db)
	if err != nil {
		t.Fatalf("create branch store: %v", err)
	}
	projector := repository.NewProjector(readStore, branchStore)

	person := domain.NewPerson("Lock", "Holder")
	m := domain.NewMedia("scan", "person", person.ID)
	m.MimeType, m.MediaType, m.Filename = "image/jpeg", domain.MediaPhoto, "scan.jpg"
	m.FileData, m.ThumbnailData, m.FileSize = []byte("FILE"), []byte("THUMB"), 4
	branch, err := domain.NewBranch("lock-probe", "", 0)
	if err != nil {
		t.Fatalf("NewBranch: %v", err)
	}
	for i, ev := range []domain.Event{domain.NewPersonCreated(person), domain.NewMediaCreated(m), domain.NewBranchCreated(branch)} {
		if err := projector.Project(ctx, ev, int64(i+1), domain.MainBranchID); err != nil {
			t.Fatalf("project %s: %v", ev.EventType(), err)
		}
	}
	branchID := domain.BranchID(branch.ID)
	edit, err := readStore.GetMedia(ctx, branchID, m.ID)
	if err != nil || edit == nil {
		t.Fatalf("branch GetMedia = %+v (err=%v)", edit, err)
	}
	edit.Title = "scan (branch)"

	waitsForLock := func(label string, write func() error) {
		t.Helper()
		holder, err := db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatalf("%s: begin holder tx: %v", label, err)
		}
		if _, err := holder.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1)`, pgstore.MediaBlobLockKey); err != nil {
			t.Fatalf("%s: take lock: %v", label, err)
		}
		done := make(chan error, 1)
		go func() { done <- write() }()
		select {
		case err := <-done:
			_ = holder.Rollback()
			t.Fatalf("%s: finished (err=%v) while the media lock was held; want it to wait", label, err)
		case <-time.After(300 * time.Millisecond):
		}
		if err := holder.Commit(); err != nil {
			t.Fatalf("%s: release lock: %v", label, err)
		}
		select {
		case err := <-done:
			if err != nil {
				t.Fatalf("%s: %v", label, err)
			}
		case <-time.After(10 * time.Second):
			t.Fatalf("%s: still blocked after the lock was released", label)
		}
	}

	waitsForLock("branch SaveMedia", func() error { return readStore.SaveMedia(ctx, branchID, edit) })
	waitsForLock("main DeleteMedia", func() error { return readStore.DeleteMedia(ctx, domain.MainBranchID, m.ID) })

	// The branch shadow committed first, so main's delete kept the bytes for it.
	got, err := readStore.GetMediaWithData(ctx, branchID, m.ID)
	if err != nil || got == nil || string(got.FileData) != "FILE" {
		t.Errorf("branch GetMediaWithData after main delete = %+v (err=%v), want its shadow with main's bytes", got, err)
	}
}
