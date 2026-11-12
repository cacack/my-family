package sqlite_test

import (
	"bytes"
	"context"
	"database/sql"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/repository/sqlite"
)

// TestReadModelStore_CorruptJSONColumnsAreLogged checks that a repository or
// submitter whose address/phone/email column holds JSON of the wrong shape
// still reads back (without the field) and that the decode failure is logged
// rather than swallowed (#527 item 5).
func TestReadModelStore_CorruptJSONColumnsAreLogged(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:?_foreign_keys=on")
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()
	store, err := sqlite.NewReadModelStore(db)
	if err != nil {
		t.Fatalf("NewReadModelStore: %v", err)
	}
	ctx := context.Background()
	addr := &domain.Address{City: "Springfield"}
	repoID, subID := uuid.New(), uuid.New()
	if err := store.SaveRepository(ctx, &repository.RepositoryReadModel{ID: repoID, Name: "Archives", Address: addr, Version: 1, UpdatedAt: time.Now()}); err != nil {
		t.Fatalf("SaveRepository: %v", err)
	}
	if err := store.SaveSubmitter(ctx, &repository.SubmitterReadModel{ID: subID, Name: "Jane", Address: addr, Phone: []string{"555"}, Version: 1, UpdatedAt: time.Now()}); err != nil {
		t.Fatalf("SaveSubmitter: %v", err)
	}
	if _, err := db.Exec(`UPDATE repositories SET address = '[1,2]' WHERE id = ?`, repoID.String()); err != nil {
		t.Fatalf("corrupt repository address: %v", err)
	}
	if _, err := db.Exec(`UPDATE submitters SET address = '[1,2]', phone = '{"a":1}' WHERE id = ?`, subID.String()); err != nil {
		t.Fatalf("corrupt submitter columns: %v", err)
	}

	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	defer slog.SetDefault(prev)

	repo, err := store.GetRepository(ctx, repoID)
	if err != nil || repo == nil || repo.Address != nil {
		t.Fatalf("GetRepository = %+v, %v; want the repository without an address", repo, err)
	}
	repos, _, err := store.ListRepositories(ctx, repository.DefaultListOptions())
	if err != nil || len(repos) != 1 || repos[0].Address != nil {
		t.Fatalf("ListRepositories = %+v, %v; want one repository without an address", repos, err)
	}
	sub, err := store.GetSubmitter(ctx, subID)
	if err != nil || sub == nil || sub.Address != nil || sub.Phone != nil {
		t.Fatalf("GetSubmitter = %+v, %v; want the submitter without address or phone", sub, err)
	}
	subs, _, err := store.ListSubmitters(ctx, repository.DefaultListOptions())
	if err != nil || len(subs) != 1 || subs[0].Address != nil || subs[0].Phone != nil {
		t.Fatalf("ListSubmitters = %+v, %v; want one submitter without address or phone", subs, err)
	}

	logged := buf.String()
	for _, want := range []string{
		"table=repositories column=address id=" + repoID.String(),
		"table=submitters column=address id=" + subID.String(),
		"table=submitters column=phone id=" + subID.String(),
	} {
		if !strings.Contains(logged, want) {
			t.Errorf("log missing %q:\n%s", want, logged)
		}
	}
}
