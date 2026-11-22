package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/auth"
	"github.com/cacack/my-family/internal/repository"
)

func TestAccountStore_APITokenContract(t *testing.T) {
	for backend, open := range accountBackends() {
		t.Run(string(backend), func(t *testing.T) {
			stores := open(t)
			t.Cleanup(func() { _ = stores.Close() })
			runAPITokenContract(t, stores.Accounts, func(t *testing.T, userID uuid.UUID) string {
				return storedAPITokenRows(t, stores, userID)
			})
		})
	}
}

// storedAPITokenRows returns the stored API token rows as text: every column
// of every row of the api_tokens table on SQL, and in memory, where the
// records are the rows, each whole record of userID's.
func storedAPITokenRows(t *testing.T, stores *Stores, userID uuid.UUID) string {
	t.Helper()
	if stores.db == nil {
		tokens, err := stores.Accounts.ListAPITokens(context.Background(), userID)
		if err != nil {
			t.Fatalf("ListAPITokens: %v", err)
		}
		return fmt.Sprintf("%+v", tokens)
	}
	rows, err := stores.db.Query(`SELECT * FROM api_tokens`)
	if err != nil {
		t.Fatalf("read api_tokens: %v", err)
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	for rows.Next() {
		values := make([]sql.NullString, len(cols))
		ptrs := make([]any, len(cols))
		for i := range values {
			ptrs[i] = &values[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			t.Fatalf("scan api_tokens: %v", err)
		}
		for _, v := range values {
			b.WriteString(v.String + " ")
		}
		b.WriteString("\n")
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

// runAPITokenContract checks that API tokens round-trip, list per user
// newest first, record their last use and are revoked only by their user
// (#1416), and that the store holds a token's hash, never the token.
func runAPITokenContract(t *testing.T, store repository.AccountStore, rawRows func(*testing.T, uuid.UUID) string) {
	ctx := context.Background()
	now := at(time.Now())
	users := make([]*repository.User, 2)
	for i, name := range []string{"ada", "grace"} {
		users[i] = &repository.User{ID: uuid.New(), Username: name, DisplayName: name, PasswordHash: "h", CreatedAt: now}
		if err := store.CreateUser(ctx, users[i], repository.DefaultTreeID, repository.RoleEditor); err != nil {
			t.Fatalf("CreateUser(%s): %v", name, err)
		}
	}
	ada, grace := users[0], users[1]

	older := &repository.APIToken{ID: uuid.New(), TokenHash: "hash-older", UserID: ada.ID, Name: "older", CreatedAt: now.Add(-time.Hour)}
	newer := &repository.APIToken{ID: uuid.New(), TokenHash: "hash-newer", UserID: ada.ID, Name: "newer", CreatedAt: now}
	theirs := &repository.APIToken{ID: uuid.New(), TokenHash: "hash-theirs", UserID: grace.ID, Name: "theirs", CreatedAt: now}
	for _, token := range []*repository.APIToken{older, newer, theirs} {
		if err := store.CreateAPIToken(ctx, token); err != nil {
			t.Fatalf("CreateAPIToken(%s): %v", token.Name, err)
		}
	}

	got, err := store.GetAPIToken(ctx, "hash-older")
	if err != nil {
		t.Fatalf("GetAPIToken: %v", err)
	}
	if got.ID != older.ID || got.UserID != ada.ID || got.Name != "older" || !got.CreatedAt.Equal(older.CreatedAt) || got.LastUsedAt != nil {
		t.Errorf("GetAPIToken = %+v, want %+v", got, older)
	}
	if _, err := store.GetAPIToken(ctx, "missing"); !errors.Is(err, repository.ErrAPITokenNotFound) {
		t.Errorf("GetAPIToken of a missing token: err = %v, want ErrAPITokenNotFound", err)
	}

	used := now.Add(time.Minute)
	if err := store.TouchAPIToken(ctx, "hash-older", used); err != nil {
		t.Fatalf("TouchAPIToken: %v", err)
	}
	if got, _ := store.GetAPIToken(ctx, "hash-older"); got == nil || got.LastUsedAt == nil || !got.LastUsedAt.Equal(used) {
		t.Errorf("after TouchAPIToken, token = %+v, want last used %v", got, used)
	}
	if err := store.TouchAPIToken(ctx, "missing", used); !errors.Is(err, repository.ErrAPITokenNotFound) {
		t.Errorf("TouchAPIToken of a missing token: err = %v, want ErrAPITokenNotFound", err)
	}

	list, err := store.ListAPITokens(ctx, ada.ID)
	if err != nil {
		t.Fatalf("ListAPITokens: %v", err)
	}
	if len(list) != 2 || list[0].ID != newer.ID || list[1].ID != older.ID {
		t.Errorf("ListAPITokens = %+v, want ada's two, newest first", list)
	}
	if list, err := store.ListAPITokens(ctx, uuid.New()); err != nil || len(list) != 0 {
		t.Errorf("ListAPITokens of a user with none = %+v, %v; want empty", list, err)
	}

	if _, err := store.DeleteAPIToken(ctx, grace.ID, older.ID); !errors.Is(err, repository.ErrAPITokenNotFound) {
		t.Errorf("DeleteAPIToken by another user: err = %v, want ErrAPITokenNotFound", err)
	}
	deleted, err := store.DeleteAPIToken(ctx, ada.ID, older.ID)
	if err != nil || deleted.Name != "older" {
		t.Fatalf("DeleteAPIToken = %+v, %v; want the older token", deleted, err)
	}
	if _, err := store.GetAPIToken(ctx, "hash-older"); !errors.Is(err, repository.ErrAPITokenNotFound) {
		t.Errorf("GetAPIToken after delete: err = %v, want ErrAPITokenNotFound", err)
	}
	if _, err := store.DeleteAPIToken(ctx, ada.ID, older.ID); !errors.Is(err, repository.ErrAPITokenNotFound) {
		t.Errorf("DeleteAPIToken twice: err = %v, want ErrAPITokenNotFound", err)
	}

	// Through the auth service, as the server issues them: the stored row
	// holds the token's hash and not the token.
	created, err := auth.NewService(store).CreateAPIToken(ctx, auth.Actor{UserID: ada.ID}, "issued")
	if err != nil {
		t.Fatalf("CreateAPIToken through auth: %v", err)
	}
	raw := rawRows(t, ada.ID)
	if !strings.Contains(raw, created.TokenHash) {
		t.Fatalf("stored rows %q lack the issued token's hash: the check would prove nothing", raw)
	}
	secret := strings.TrimPrefix(created.Token, auth.APITokenPrefix)
	if strings.Contains(raw, secret) {
		t.Errorf("the stored rows hold the token's secret: %q", raw)
	}
}
