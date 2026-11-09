package integration_test

import (
	"bytes"
	"context"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/command"
	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/gedcom"
	"github.com/cacack/my-family/internal/repository"
)

// Citation external identifiers (issue #1175). An Ancestry _APID on a
// citation is kept as a typed external ID, so export can write it back.

// ancestryAPIDs are the citation _APID values in ancestry-sample.ged.
var ancestryAPIDs = []string{"1,5678::87654321", "1,7602::2771226", "1,9024::12345678"}

func TestGedcomExport_KeepsCitationAncestryAPIDs(t *testing.T) {
	data, err := os.ReadFile("../../testdata/gedcom-5.5/ancestry-sample.ged")
	if err != nil {
		t.Fatal(err)
	}
	for _, backend := range backends {
		t.Run(backend.name, func(t *testing.T) {
			ctx := context.Background()
			st := backend.setup(t)
			importGedcomBytes(t, ctx, st, data)
			imported := citationExternalIDs(t, ctx, st.read)
			if values := externalIDValues(imported); !slices.Equal(values, ancestryAPIDs) {
				t.Fatalf("imported citation external IDs = %v, want the APIDs %v", imported, ancestryAPIDs)
			}

			// The IDs travel in CitationCreated, so a replay of the log keeps them.
			replayed := backend.setup(t)
			replayLog(t, ctx, st.events, replayed.read)
			if got := citationExternalIDs(t, ctx, replayed.read); !slices.Equal(got, imported) {
				t.Errorf("replayed citation external IDs = %v, want %v", got, imported)
			}

			buf := &bytes.Buffer{}
			if _, err := gedcom.NewExporter(st.read).Export(ctx, buf); err != nil {
				t.Fatalf("export: %v", err)
			}
			for _, apid := range ancestryAPIDs {
				if !strings.Contains(buf.String(), " _APID "+apid+"\n") {
					t.Errorf("export lacks _APID %s; got:\n%s", apid, buf.String())
				}
			}

			again := backend.setup(t)
			importGedcomBytes(t, ctx, again, buf.Bytes())
			if got := citationExternalIDs(t, ctx, again.read); !slices.Equal(got, imported) {
				t.Errorf("re-imported citation external IDs = %v, want %v", got, imported)
			}
		})
	}
}

func importGedcomBytes(t *testing.T, ctx context.Context, st stores, data []byte) {
	t.Helper()
	h := command.NewHandlerWithBranches(st.events, st.read, st.branches, st.snapshots)
	if _, err := h.ImportGedcom(ctx, command.ImportGedcomInput{Filename: "t.ged", Reader: bytes.NewReader(data)}); err != nil {
		t.Fatalf("import: %v", err)
	}
}

// citationExternalIDs lists every main-branch citation external ID as
// "fact type|value|type", sorted, so stores with different citation UUIDs
// compare equal.
func citationExternalIDs(t *testing.T, ctx context.Context, rs repository.ReadModelStore) []string {
	t.Helper()
	citations, err := repository.ListAll(ctx, 100, rs.ListCitations)
	if err != nil {
		t.Fatalf("list citations: %v", err)
	}
	var out []string
	for _, c := range citations {
		ids, err := rs.GetCitationExternalIDs(ctx, domain.MainBranchID, c.ID)
		if err != nil {
			t.Fatalf("citation external IDs: %v", err)
		}
		for _, id := range ids {
			out = append(out, string(c.FactType)+"|"+id.Value+"|"+id.Type)
		}
	}
	slices.Sort(out)
	return out
}

// externalIDValues returns the value part of citationExternalIDs entries, sorted.
func externalIDValues(entries []string) []string {
	out := make([]string, len(entries))
	for i, e := range entries {
		out[i] = strings.Split(e, "|")[1]
	}
	slices.Sort(out)
	return out
}

// TestCitationExternalIDs_BranchOverlayAndCascade checks a citation's
// external-ID bucket behaves like a source's on every backend: a branch reads
// main's bucket until it writes its own, an empty branch bucket hides main's,
// and deleting the citation, or its source, takes the bucket with it on that
// branch only.
func TestCitationExternalIDs_BranchOverlayAndCascade(t *testing.T) {
	for _, backend := range backends {
		t.Run(backend.name, func(t *testing.T) {
			ctx := context.Background()
			rs := backend.setup(t).read
			main := domain.MainBranchID
			branch := domain.BranchID(uuid.New())
			sourceID := uuid.New()
			if err := rs.SaveSource(ctx, main, &repository.SourceReadModel{ID: sourceID, SourceType: domain.SourceBook, Title: "Census", Version: 1, UpdatedAt: time.Now()}); err != nil {
				t.Fatalf("save source: %v", err)
			}
			newCitation := func() uuid.UUID {
				id := uuid.New()
				if err := rs.SaveCitation(ctx, main, &repository.CitationReadModel{ID: id, SourceID: sourceID, FactType: domain.FactPersonBirth, FactOwnerID: uuid.New(), Version: 1, CreatedAt: time.Now()}); err != nil {
					t.Fatalf("save citation: %v", err)
				}
				if err := rs.ReplaceCitationExternalIDs(ctx, main, id, []repository.CitationExternalIDReadModel{{Value: "MAIN-1", Type: "t"}, {Value: "MAIN-2"}}); err != nil {
					t.Fatalf("replace main external IDs: %v", err)
				}
				return id
			}
			read := func(b domain.BranchID, id uuid.UUID) string {
				t.Helper()
				ids, err := rs.GetCitationExternalIDs(ctx, b, id)
				if err != nil {
					t.Fatalf("get external IDs: %v", err)
				}
				var vals []string
				for i, x := range ids {
					if x.CitationID != id || x.Sequence != i {
						t.Errorf("row %d = %+v, want citation %s sequence %d", i, x, id, i)
					}
					vals = append(vals, x.Value)
				}
				return strings.Join(vals, ",")
			}

			cit := newCitation()
			if got := read(branch, cit); got != "MAIN-1,MAIN-2" {
				t.Errorf("fresh branch reads %q, want main's bucket", got)
			}
			if err := rs.ReplaceCitationExternalIDs(ctx, branch, cit, []repository.CitationExternalIDReadModel{{Value: "BRANCH-1"}}); err != nil {
				t.Fatalf("replace branch external IDs: %v", err)
			}
			if got := read(branch, cit); got != "BRANCH-1" {
				t.Errorf("branch reads %q after its own write, want BRANCH-1", got)
			}
			if err := rs.ReplaceCitationExternalIDs(ctx, branch, cit, nil); err != nil {
				t.Fatalf("empty branch external IDs: %v", err)
			}
			if got := read(branch, cit); got != "" {
				t.Errorf("branch reads %q after emptying its bucket, want none", got)
			}
			if got := read(main, cit); got != "MAIN-1,MAIN-2" {
				t.Errorf("main reads %q after branch writes, want its own bucket", got)
			}

			deleted := newCitation()
			if err := rs.DeleteCitation(ctx, branch, deleted); err != nil {
				t.Fatalf("delete citation on branch: %v", err)
			}
			if got := read(branch, deleted); got != "" {
				t.Errorf("branch reads %q for its deleted citation, want none", got)
			}
			if got := read(main, deleted); got != "MAIN-1,MAIN-2" {
				t.Errorf("main reads %q after a branch delete, want its bucket", got)
			}
			if err := rs.DeleteCitation(ctx, main, deleted); err != nil {
				t.Fatalf("delete citation on main: %v", err)
			}
			if got := read(main, deleted); got != "" {
				t.Errorf("main reads %q for its deleted citation, want none", got)
			}

			if err := rs.DeleteSource(ctx, main, sourceID); err != nil {
				t.Fatalf("delete source on main: %v", err)
			}
			if got := read(main, cit); got != "" {
				t.Errorf("main reads %q for a citation of a deleted source, want none", got)
			}
		})
	}
}
