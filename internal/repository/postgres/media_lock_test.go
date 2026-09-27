package postgres_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

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

// TestMediaLockComesBeforeRowLocks proves every read-model delete that cascades
// to media takes the media advisory lock before it writes any row (#759
// review). Otherwise a mainline DeletePerson that had already locked a
// family_children row would wait for the media lock while a DeleteFamily
// holding that lock waited for the row — a deadlock that aborts a projection
// whose event is already appended. While another transaction holds the lock,
// each delete must be waiting on it with no RowExclusiveLock on any table yet.
func TestMediaLockComesBeforeRowLocks(t *testing.T) {
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

	person := domain.NewPerson("Lock", "Order")
	family := domain.NewFamily()
	source := domain.NewSource("Parish register", domain.SourceBook)
	branch, err := domain.NewBranch("lock-order", "", 0)
	if err != nil {
		t.Fatalf("NewBranch: %v", err)
	}
	events := []domain.Event{
		domain.NewPersonCreated(person),
		domain.NewFamilyCreated(family),
		domain.NewChildLinkedToFamily(domain.NewFamilyChild(family.ID, person.ID, domain.ChildBiological)),
		domain.NewSourceCreated(source),
	}
	for _, owner := range []struct {
		kind string
		id   uuid.UUID
	}{{"person", person.ID}, {"family", family.ID}, {"source", source.ID}} {
		m := domain.NewMedia(owner.kind+" scan", owner.kind, owner.id)
		m.MimeType, m.MediaType, m.Filename = "image/jpeg", domain.MediaPhoto, "scan.jpg"
		m.FileData, m.FileSize = []byte("FILE"), 4
		events = append(events, domain.NewMediaCreated(m))
	}
	events = append(events, domain.NewBranchCreated(branch))
	for i, ev := range events {
		if err := projector.Project(ctx, ev, int64(i+1), domain.MainBranchID); err != nil {
			t.Fatalf("project %s: %v", ev.EventType(), err)
		}
	}
	branchID := domain.BranchID(branch.ID)

	locksFirst := func(label string, write func() error) {
		t.Helper()
		holder, err := db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatalf("%s: begin holder tx: %v", label, err)
		}
		defer func() { _ = holder.Rollback() }()
		if _, err := holder.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1)`, pgstore.MediaBlobLockKey); err != nil {
			t.Fatalf("%s: take lock: %v", label, err)
		}
		done := make(chan error, 1)
		go func() { done <- write() }()

		var waiter int
		deadline := time.Now().Add(10 * time.Second)
		for {
			err := db.QueryRowContext(ctx, `SELECT pid FROM pg_locks
				WHERE locktype = 'advisory' AND NOT granted LIMIT 1`).Scan(&waiter)
			if err == nil {
				break
			}
			if !errors.Is(err, sql.ErrNoRows) {
				t.Fatalf("%s: find waiter: %v", label, err)
			}
			select {
			case err := <-done:
				t.Fatalf("%s: finished (err=%v) while the media lock was held; want it to wait", label, err)
			default:
			}
			if time.Now().After(deadline) {
				t.Fatalf("%s: never waited on the media lock", label)
			}
			time.Sleep(20 * time.Millisecond)
		}

		rows, err := db.QueryContext(ctx, `SELECT c.relname FROM pg_locks l
			JOIN pg_class c ON c.oid = l.relation
			WHERE l.pid = $1 AND l.locktype = 'relation' AND l.mode = 'RowExclusiveLock'`, waiter)
		if err != nil {
			t.Fatalf("%s: list row locks: %v", label, err)
		}
		var written []string
		for rows.Next() {
			var rel string
			if err := rows.Scan(&rel); err != nil {
				t.Fatalf("%s: scan: %v", label, err)
			}
			written = append(written, rel)
		}
		if err := rows.Err(); err != nil {
			t.Fatalf("%s: rows: %v", label, err)
		}
		_ = rows.Close()
		if len(written) > 0 {
			t.Errorf("%s: wrote %v before taking the media lock; want the lock first", label, written)
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

	locksFirst("branch DeletePerson", func() error { return readStore.DeletePerson(ctx, branchID, person.ID) })
	locksFirst("branch DeleteSource", func() error { return readStore.DeleteSource(ctx, branchID, source.ID) })
	locksFirst("main DeletePerson", func() error { return readStore.DeletePerson(ctx, domain.MainBranchID, person.ID) })
	locksFirst("main DeleteFamily", func() error { return readStore.DeleteFamily(ctx, domain.MainBranchID, family.ID) })
	locksFirst("main DeleteSource", func() error { return readStore.DeleteSource(ctx, domain.MainBranchID, source.ID) })
	locksFirst("PurgeBranch", func() error { return readStore.PurgeBranch(ctx, branchID) })
}
