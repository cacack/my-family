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
	readStore := &failingPersonReadStore{memory.NewReadModelStore()}
	handler := command.NewHandler(eventStore, readStore)

	result, err := handler.CreatePerson(context.Background(), command.CreatePersonInput{GivenName: "Ada", Surname: "Lovelace"})
	if err != nil {
		t.Fatalf("CreatePerson returned error despite successful append: %v", err)
	}

	events, err := eventStore.ReadStream(context.Background(), result.ID)
	if err != nil {
		t.Fatalf("GetStream: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected both events to be appended, got %d events", len(events))
	}

	out := buf.String()
	for _, want := range []string{"projection failed after append", "PersonCreated", result.ID.String(), "read model unavailable"} {
		if !strings.Contains(out, want) {
			t.Errorf("log output missing %q:\n%s", want, out)
		}
	}

	// The failure also leaves a durable marker naming the event (#845), so
	// `serve` can warn about it on restart and `myfamily rebuild` can clear it.
	failures, err := readStore.ListProjectionFailures(context.Background(), 10)
	if err != nil {
		t.Fatalf("ListProjectionFailures: %v", err)
	}
	if len(failures) != 1 || failures[0].StreamID != result.ID || failures[0].Version != 1 || failures[0].EventType != "PersonCreated" {
		t.Errorf("projection failures = %+v, want one for the PersonCreated", failures)
	}
}

// A command that returns its projection error (here a GEDCOM import) records
// the same marker: the events before the failure are durable either way.
func TestProjectionFailureThatFailsTheCommandIsRecorded(t *testing.T) {
	readStore := &failingPersonReadStore{memory.NewReadModelStore()}
	handler := command.NewHandler(memory.NewEventStore(), readStore)

	// The import reports the failure in its result; either way the marker must exist.
	_, _ = handler.ImportGedcom(context.Background(), command.ImportGedcomInput{
		Filename: "t.ged",
		Reader:   strings.NewReader("0 HEAD\n1 GEDC\n2 VERS 5.5.1\n0 @I1@ INDI\n1 NAME Ada /Lovelace/\n0 TRLR\n"),
	})
	failures, err := readStore.ListProjectionFailures(context.Background(), 10)
	if err != nil {
		t.Fatalf("ListProjectionFailures: %v", err)
	}
	if len(failures) == 0 || failures[0].EventType != "PersonCreated" {
		t.Errorf("projection failures = %+v, want the import's PersonCreated", failures)
	}
}
