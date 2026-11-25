package repository_test

import (
	"bytes"
	"context"
	"log"
	"strings"
	"testing"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/repository/memory"
)

// TestProjector_PlaceCycleWarningsCannotForgeLogLines: the projector logs the
// keys of an ignored alias or parent, and a key is text from a GEDCOM file or
// a request. A key holding a line break must stay inside its own log line, so
// nobody can write a second, made-up entry through it (CodeQL go/log-injection
// on #1737).
func TestProjector_PlaceCycleWarningsCannotForgeLogLines(t *testing.T) {
	var out bytes.Buffer
	previous := log.Writer()
	log.SetOutput(&out)
	t.Cleanup(func() { log.SetOutput(previous) })

	ctx := context.Background()
	projector := repository.NewProjector(memory.NewReadModelStore(), nil)
	const forged = "2026/01/01 00:00:00 ERROR forged entry"
	plain, hostile := "plain,place", "hostile\n"+forged+"\r\n,place"
	plainID, hostileID := repository.PlaceID(plain), repository.PlaceID(hostile)

	events := []domain.Event{
		// A parent cycle: plain's parent is hostile, then hostile's is plain.
		domain.NewPlaceUpdated(plainID, plain, plain, map[string]any{"parent": hostile}),
		domain.NewPlaceUpdated(hostileID, hostile, hostile, map[string]any{"parent": plain}),
		// An alias cycle between two other places.
		domain.NewPlaceAliased("a"+hostile, "a", "b"+hostile, "b", repository.PlaceID("a"+hostile), repository.PlaceID("b"+hostile)),
		domain.NewPlaceAliased("b"+hostile, "b", "a"+hostile, "a", repository.PlaceID("b"+hostile), repository.PlaceID("a"+hostile)),
	}
	for _, e := range events {
		if err := projector.Project(ctx, e, 1, domain.MainBranchID); err != nil {
			t.Fatalf("Project(%T): %v", e, err)
		}
	}

	lines := strings.Split(strings.TrimRight(out.String(), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("logged %d lines, want the 2 cycle warnings:\n%s", len(lines), out.String())
	}
	for i, want := range []string{"ignoring a place parent that would close a cycle", "ignoring a place alias that would close a cycle"} {
		if !strings.Contains(lines[i], want) || !strings.Contains(lines[i], `\n`+forged) {
			t.Errorf("line %d = %q, want %q with the line break escaped", i, lines[i], want)
		}
		if strings.Contains(lines[i], "\r") || strings.HasPrefix(lines[i], forged) {
			t.Errorf("line %d = %q carries a raw line break or starts with the forged entry", i, lines[i])
		}
	}
}
