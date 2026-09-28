package command_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/cacack/my-family/internal/command"
	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/repository/memory"
)

// failingPersonReadStore fails every person projection write.
type failingPersonReadStore struct {
	*memory.ReadModelStore
}

func (s *failingPersonReadStore) SavePerson(context.Context, domain.BranchID, *repository.PersonReadModel) error {
	return errors.New("read model unavailable")
}

// A projection failure after a successful append must not fail the command,
// but it must be logged rather than silently dropped: with persistent storage
// the read model otherwise drifts from the event log with no trace.
func TestExecute_ProjectionFailureIsLogged(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })

	eventStore := memory.NewEventStore()
	handler := command.NewHandler(eventStore, &failingPersonReadStore{memory.NewReadModelStore()})

	result, err := handler.CreatePerson(context.Background(), command.CreatePersonInput{GivenName: "Ada", Surname: "Lovelace"})
	if err != nil {
		t.Fatalf("CreatePerson returned error despite successful append: %v", err)
	}

	events, err := eventStore.ReadStream(context.Background(), result.ID)
	if err != nil {
		t.Fatalf("GetStream: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected the event to be appended, got %d events", len(events))
	}

	out := buf.String()
	for _, want := range []string{"projection failed after append", "PersonCreated", result.ID.String(), "read model unavailable"} {
		if !strings.Contains(out, want) {
			t.Errorf("log output missing %q:\n%s", want, out)
		}
	}
}
