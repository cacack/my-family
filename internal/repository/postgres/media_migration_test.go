package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	pgstore "github.com/cacack/my-family/internal/repository/postgres"
)

// preMediaBranchDDL is the media table as it stood before #759: keyed by a lone
// id, with file_data NOT NULL. runBranchMigration must bring it to a
// (id, branch_id) key with a deleted tombstone and a nullable file_data (a
// branch shadow row stores no bytes), keeping its rows and their bytes.
const preMediaBranchDDL = `
	CREATE TABLE media (
		id UUID PRIMARY KEY,
		entity_type VARCHAR(20) NOT NULL,
		entity_id UUID NOT NULL,
		title VARCHAR(500) NOT NULL,
		description TEXT,
		mime_type VARCHAR(100) NOT NULL,
		media_type VARCHAR(20) NOT NULL,
		filename VARCHAR(255) NOT NULL,
		file_size BIGINT NOT NULL,
		file_data BYTEA NOT NULL,
		thumbnail_data BYTEA,
		crop_left INTEGER,
		crop_top INTEGER,
		crop_width INTEGER,
		crop_height INTEGER,
		gedcom_xref VARCHAR(50),
		version BIGINT NOT NULL DEFAULT 1,
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		files JSONB,
		format VARCHAR(100),
		translations JSONB
	);
`

// TestReadModelStore_MigratesMediaToBranchKeys covers the in-place upgrade of a
// database created before #759: media gains branch_id + deleted, a
// branch_id-bearing primary key and a nullable file_data; its rows land on main
// with their bytes; a branch metadata edit then stores no copy of them; and
// reopening is a no-op.
func TestReadModelStore_MigratesMediaToBranchKeys(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	db, cleanup := setupPostgres(t)
	defer cleanup()

	if _, err := db.Exec(preMediaBranchDDL); err != nil {
		t.Fatalf("create pre-#759 media table: %v", err)
	}
	mediaID, ownerID := uuid.New(), uuid.New()
	if _, err := db.Exec(`
		INSERT INTO media (id, entity_type, entity_id, title, mime_type, media_type, filename, file_size, file_data, thumbnail_data)
		VALUES ($1, 'person', $2, 'Portrait', 'image/jpeg', 'photo', 'portrait.jpg', 4, 'FILE', 'THUMB')
	`, mediaID, ownerID); err != nil {
		t.Fatalf("seed media: %v", err)
	}

	store, err := pgstore.NewReadModelStore(db)
	if err != nil {
		t.Fatalf("create read model store: %v", err)
	}
	if !primaryKeyHasBranch(t, db, "media") {
		t.Errorf("media primary key was not migrated to include branch_id")
	}
	if !hasColumn(t, db, "media", "deleted") {
		t.Errorf("media gained no deleted column")
	}
	var nullable string
	if err := db.QueryRow(`
		SELECT is_nullable FROM information_schema.columns
		WHERE table_name = 'media' AND column_name = 'file_data'
	`).Scan(&nullable); err != nil {
		t.Fatalf("inspect media.file_data: %v", err)
	}
	if nullable != "YES" {
		t.Errorf("media.file_data is_nullable = %q, want YES", nullable)
	}

	ctx := context.Background()
	main := domain.MainBranchID
	got, err := store.GetMediaWithData(ctx, main, mediaID)
	if err != nil || got == nil || got.Title != "Portrait" || string(got.FileData) != "FILE" || string(got.ThumbnailData) != "THUMB" {
		t.Fatalf("migrated media on main = %+v (err=%v)", got, err)
	}

	// A branch metadata edit of the migrated row stores no bytes and reads main's.
	branch := domain.BranchID(uuid.New())
	shadow := *got
	shadow.Title = "Portrait (branch reading)"
	shadow.UpdatedAt = time.Now()
	if err := store.SaveMedia(ctx, branch, &shadow); err != nil {
		t.Fatalf("branch SaveMedia on migrated table: %v", err)
	}
	var fileNull, thumbNull bool
	if err := db.QueryRow(`SELECT file_data IS NULL, thumbnail_data IS NULL FROM media WHERE id = $1 AND branch_id = $2`,
		mediaID, branch.UUID()).Scan(&fileNull, &thumbNull); err != nil {
		t.Fatalf("read branch shadow row: %v", err)
	}
	if !fileNull || !thumbNull {
		t.Errorf("branch shadow row stores bytes (file NULL=%v, thumbnail NULL=%v), want none", fileNull, thumbNull)
	}
	if got, err := store.GetMediaWithData(ctx, branch, mediaID); err != nil || got == nil || got.Title != shadow.Title || string(got.FileData) != "FILE" {
		t.Errorf("branch GetMediaWithData = %+v (err=%v), want the branch title over main's bytes", got, err)
	}

	// Reopening is a no-op that keeps the branch row.
	store2, err := pgstore.NewReadModelStore(db)
	if err != nil {
		t.Fatalf("reopen read model store: %v", err)
	}
	if got, err := store2.GetMedia(ctx, branch, mediaID); err != nil || got == nil || got.Title != shadow.Title {
		t.Errorf("branch GetMedia after reopen = %+v (err=%v), want the branch title", got, err)
	}
}
