package query

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// fakeUserNames is a UserNames over a fixed map, or one that fails.
type fakeUserNames struct {
	names map[uuid.UUID]string
	err   error
	calls int
}

func (f *fakeUserNames) DisplayNames(_ context.Context, ids []uuid.UUID) (map[uuid.UUID]string, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	out := make(map[uuid.UUID]string)
	for _, id := range ids {
		if name, ok := f.names[id]; ok {
			out[id] = name
		}
	}
	return out, nil
}

// personEventBy is a PersonCreated stored event whose metadata names userID
// ("" for none).
func personEventBy(t *testing.T, userID string, position int64) repository.StoredEvent {
	t.Helper()
	personID := uuid.New()
	data, err := json.Marshal(domain.NewPersonCreated(&domain.Person{ID: personID, GivenName: "Ada", Surname: "Lovelace"}))
	if err != nil {
		t.Fatal(err)
	}
	evt := repository.StoredEvent{
		ID: uuid.New(), StreamID: personID, StreamType: "person", EventType: "PersonCreated",
		Data: data, Version: 1, Position: position, Timestamp: time.Now(),
	}
	if userID != "" {
		if evt.Metadata, err = json.Marshal(domain.EventMetadata{UserID: userID}); err != nil {
			t.Fatal(err)
		}
	}
	return evt
}

func TestHistory_NamesWhoMadeEachChange(t *testing.T) {
	known, gone := uuid.New(), uuid.New()
	events := []repository.StoredEvent{
		personEventBy(t, known.String(), 1),
		personEventBy(t, gone.String(), 2),
		personEventBy(t, "", 3),
		personEventBy(t, "not-a-uuid", 4),
		personEventBy(t, known.String(), 5),
	}
	readStore := &mockReadModelStore{}

	names := &fakeUserNames{names: map[uuid.UUID]string{known: "Ada Lovelace"}}
	service := NewHistoryService(&mockEventStore{}, readStore)
	service.UseUserNames(names)
	entries, err := service.transformStoredEvents(context.Background(), events)
	if err != nil {
		t.Fatalf("transformStoredEvents: %v", err)
	}
	if names.calls != 1 {
		t.Errorf("DisplayNames called %d times, want one batched call", names.calls)
	}
	want := []*string{ptr("Ada Lovelace"), nil, nil, nil, ptr("Ada Lovelace")}
	for i, entry := range entries {
		if got := entry.UserName; (got == nil) != (want[i] == nil) || (got != nil && *got != *want[i]) {
			t.Errorf("entry %d UserName = %v, want %v", i, deref(got), deref(want[i]))
		}
	}

	// A failed lookup names no one but still returns the history.
	failing := NewHistoryService(&mockEventStore{}, readStore)
	failing.UseUserNames(&fakeUserNames{err: errors.New("db down")})
	entries, err = failing.transformStoredEvents(context.Background(), events[:1])
	if err != nil {
		t.Fatalf("transformStoredEvents with a failing lookup: %v", err)
	}
	if entries[0].UserName != nil || entries[0].UserID == nil {
		t.Errorf("failed lookup: entry = %+v, want the user ID and no name", entries[0])
	}

	// Without UseUserNames entries carry only the ID.
	plain := NewHistoryService(&mockEventStore{}, readStore)
	entries, _ = plain.transformStoredEvents(context.Background(), events[:1])
	if entries[0].UserName != nil {
		t.Errorf("no lookup configured: UserName = %q, want nil", *entries[0].UserName)
	}
}

func deref(s *string) string {
	if s == nil {
		return "<nil>"
	}
	return *s
}
