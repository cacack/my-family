package memory_test

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/repository/memory"
)

// TestBatchLookup_Parity runs the #697 batched-lookup scenario against the
// in-memory backend; the sqlite and postgres packages run the same body (DB-001).
func TestBatchLookup_Parity(t *testing.T) {
	runBatchLookupScenario(t, memory.NewReadModelStore())
}

// runBatchLookupScenario is the backend-agnostic body of the #697 batched-lookup
// parity test. Each backend package carries an identical copy (there is no shared
// test harness in this repo); keeping it byte-identical is the DB-001 parity
// guarantee. For every entity type it proves the Get*ByIDs lookup resolves the
// ADR-005 overlay exactly as N single-row Get calls would: a branch row wins, a
// branch tombstone hides main's row, an untouched id falls back to main, another
// branch's rows never leak, unknown ids and duplicates are harmless, and the
// result is ordered by id. Fixtures use neutral placeholder names only.
func runBatchLookupScenario(t *testing.T, store repository.ReadModelStore) {
	t.Helper()
	ctx := context.Background()
	branch := domain.BranchID(uuid.New())
	other := domain.BranchID(uuid.New())
	now := time.Now().UTC().Truncate(time.Second)

	// Four ids per type: kept (main only), edited (branch shadow), deleted
	// (branch tombstone), added (branch only); plus one id nothing ever wrote.
	type ids struct{ kept, edited, deleted, added, missing uuid.UUID }
	newIDs := func() ids { return ids{uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()} }
	lookup := func(i ids) []uuid.UUID {
		// Duplicates on purpose: a batch must collapse them.
		return []uuid.UUID{i.missing, i.added, i.deleted, i.edited, i.kept, i.kept, i.edited}
	}

	// --- Persons ---
	p := newIDs()
	person := func(id uuid.UUID, given string) *repository.PersonReadModel {
		return &repository.PersonReadModel{ID: id, GivenName: given, Surname: "Sample", FullName: given + " Sample", Version: 1, UpdatedAt: now}
	}
	for _, row := range []*repository.PersonReadModel{person(p.kept, "Kept"), person(p.edited, "MainEdit"), person(p.deleted, "Doomed")} {
		mustNoErr(t, "save main person", store.SavePerson(ctx, domain.MainBranchID, row))
	}
	mustNoErr(t, "save branch person", store.SavePerson(ctx, branch, person(p.edited, "BranchEdit")))
	mustNoErr(t, "save added person", store.SavePerson(ctx, branch, person(p.added, "Added")))
	mustNoErr(t, "delete branch person", store.DeletePerson(ctx, branch, p.deleted))
	mustNoErr(t, "save other-branch person", store.SavePerson(ctx, other, person(p.kept, "Leaked")))

	personName := func(r repository.PersonReadModel) string { return r.GivenName }
	personID := func(r repository.PersonReadModel) uuid.UUID { return r.ID }
	getPerson := func(b domain.BranchID) func(uuid.UUID) (*repository.PersonReadModel, error) {
		return func(id uuid.UUID) (*repository.PersonReadModel, error) { return store.GetPerson(ctx, b, id) }
	}
	got, err := store.GetPersonsByIDs(ctx, domain.MainBranchID, lookup(p))
	mustNoErr(t, "persons on main", err)
	checkBatch(t, "persons on main", got, personID, personName, getPerson(domain.MainBranchID),
		map[uuid.UUID]string{p.kept: "Kept", p.edited: "MainEdit", p.deleted: "Doomed"})
	got, err = store.GetPersonsByIDs(ctx, branch, lookup(p))
	mustNoErr(t, "persons on branch", err)
	checkBatch(t, "persons on branch", got, personID, personName, getPerson(branch),
		map[uuid.UUID]string{p.kept: "Kept", p.edited: "BranchEdit", p.added: "Added"})

	// --- Families ---
	f := newIDs()
	family := func(id uuid.UUID, given string) *repository.FamilyReadModel {
		return &repository.FamilyReadModel{ID: id, Partner1GivenName: given, Partner1Surname: "Sample",
			RelationshipType: domain.RelationMarriage, Version: 1, UpdatedAt: now}
	}
	for _, row := range []*repository.FamilyReadModel{family(f.kept, "Kept"), family(f.edited, "MainEdit"), family(f.deleted, "Doomed")} {
		mustNoErr(t, "save main family", store.SaveFamily(ctx, domain.MainBranchID, row))
	}
	mustNoErr(t, "save branch family", store.SaveFamily(ctx, branch, family(f.edited, "BranchEdit")))
	mustNoErr(t, "save added family", store.SaveFamily(ctx, branch, family(f.added, "Added")))
	mustNoErr(t, "delete branch family", store.DeleteFamily(ctx, branch, f.deleted))
	mustNoErr(t, "save other-branch family", store.SaveFamily(ctx, other, family(f.kept, "Leaked")))

	familyName := func(r repository.FamilyReadModel) string { return r.Partner1GivenName }
	familyID := func(r repository.FamilyReadModel) uuid.UUID { return r.ID }
	getFamily := func(b domain.BranchID) func(uuid.UUID) (*repository.FamilyReadModel, error) {
		return func(id uuid.UUID) (*repository.FamilyReadModel, error) { return store.GetFamily(ctx, b, id) }
	}
	fams, err := store.GetFamiliesByIDs(ctx, domain.MainBranchID, lookup(f))
	mustNoErr(t, "families on main", err)
	checkBatch(t, "families on main", fams, familyID, familyName, getFamily(domain.MainBranchID),
		map[uuid.UUID]string{f.kept: "Kept", f.edited: "MainEdit", f.deleted: "Doomed"})
	fams, err = store.GetFamiliesByIDs(ctx, branch, lookup(f))
	mustNoErr(t, "families on branch", err)
	checkBatch(t, "families on branch", fams, familyID, familyName, getFamily(branch),
		map[uuid.UUID]string{f.kept: "Kept", f.edited: "BranchEdit", f.added: "Added"})

	// --- Sources ---
	s := newIDs()
	source := func(id uuid.UUID, title string) *repository.SourceReadModel {
		return &repository.SourceReadModel{ID: id, SourceType: domain.SourceBook, Title: title, Version: 1, UpdatedAt: now}
	}
	for _, row := range []*repository.SourceReadModel{source(s.kept, "Kept"), source(s.edited, "MainEdit"), source(s.deleted, "Doomed")} {
		mustNoErr(t, "save main source", store.SaveSource(ctx, domain.MainBranchID, row))
	}
	mustNoErr(t, "save branch source", store.SaveSource(ctx, branch, source(s.edited, "BranchEdit")))
	mustNoErr(t, "save added source", store.SaveSource(ctx, branch, source(s.added, "Added")))
	mustNoErr(t, "delete branch source", store.DeleteSource(ctx, branch, s.deleted))
	mustNoErr(t, "save other-branch source", store.SaveSource(ctx, other, source(s.kept, "Leaked")))

	sourceName := func(r repository.SourceReadModel) string { return r.Title }
	sourceID := func(r repository.SourceReadModel) uuid.UUID { return r.ID }
	getSource := func(b domain.BranchID) func(uuid.UUID) (*repository.SourceReadModel, error) {
		return func(id uuid.UUID) (*repository.SourceReadModel, error) { return store.GetSource(ctx, b, id) }
	}
	srcs, err := store.GetSourcesByIDs(ctx, domain.MainBranchID, lookup(s))
	mustNoErr(t, "sources on main", err)
	checkBatch(t, "sources on main", srcs, sourceID, sourceName, getSource(domain.MainBranchID),
		map[uuid.UUID]string{s.kept: "Kept", s.edited: "MainEdit", s.deleted: "Doomed"})
	srcs, err = store.GetSourcesByIDs(ctx, branch, lookup(s))
	mustNoErr(t, "sources on branch", err)
	checkBatch(t, "sources on branch", srcs, sourceID, sourceName, getSource(branch),
		map[uuid.UUID]string{s.kept: "Kept", s.edited: "BranchEdit", s.added: "Added"})

	// --- Citations (all cite the main-only source s.kept) ---
	c := newIDs()
	citation := func(id uuid.UUID, page string) *repository.CitationReadModel {
		return &repository.CitationReadModel{ID: id, SourceID: s.kept, SourceTitle: "Kept", Page: page,
			FactType: domain.FactPersonBirth, FactOwnerID: p.kept, Version: 1, CreatedAt: now}
	}
	for _, row := range []*repository.CitationReadModel{citation(c.kept, "Kept"), citation(c.edited, "MainEdit"), citation(c.deleted, "Doomed")} {
		mustNoErr(t, "save main citation", store.SaveCitation(ctx, domain.MainBranchID, row))
	}
	mustNoErr(t, "save branch citation", store.SaveCitation(ctx, branch, citation(c.edited, "BranchEdit")))
	mustNoErr(t, "save added citation", store.SaveCitation(ctx, branch, citation(c.added, "Added")))
	mustNoErr(t, "delete branch citation", store.DeleteCitation(ctx, branch, c.deleted))
	mustNoErr(t, "save other-branch citation", store.SaveCitation(ctx, other, citation(c.kept, "Leaked")))

	citationName := func(r repository.CitationReadModel) string { return r.Page }
	citationID := func(r repository.CitationReadModel) uuid.UUID { return r.ID }
	getCitation := func(b domain.BranchID) func(uuid.UUID) (*repository.CitationReadModel, error) {
		return func(id uuid.UUID) (*repository.CitationReadModel, error) { return store.GetCitation(ctx, b, id) }
	}
	cits, err := store.GetCitationsByIDs(ctx, domain.MainBranchID, lookup(c))
	mustNoErr(t, "citations on main", err)
	checkBatch(t, "citations on main", cits, citationID, citationName, getCitation(domain.MainBranchID),
		map[uuid.UUID]string{c.kept: "Kept", c.edited: "MainEdit", c.deleted: "Doomed"})
	cits, err = store.GetCitationsByIDs(ctx, branch, lookup(c))
	mustNoErr(t, "citations on branch", err)
	checkBatch(t, "citations on branch", cits, citationID, citationName, getCitation(branch),
		map[uuid.UUID]string{c.kept: "Kept", c.edited: "BranchEdit", c.added: "Added"})

	// --- Empty and all-unknown id sets ---
	for _, b := range []domain.BranchID{domain.MainBranchID, branch} {
		if rows, err := store.GetPersonsByIDs(ctx, b, nil); err != nil || len(rows) != 0 {
			t.Fatalf("GetPersonsByIDs(nil) = %v, %v; want empty", rows, err)
		}
		if rows, err := store.GetFamiliesByIDs(ctx, b, []uuid.UUID{}); err != nil || len(rows) != 0 {
			t.Fatalf("GetFamiliesByIDs(empty) = %v, %v; want empty", rows, err)
		}
		if rows, err := store.GetSourcesByIDs(ctx, b, []uuid.UUID{uuid.New()}); err != nil || len(rows) != 0 {
			t.Fatalf("GetSourcesByIDs(unknown) = %v, %v; want empty", rows, err)
		}
		if rows, err := store.GetCitationsByIDs(ctx, b, []uuid.UUID{uuid.New(), uuid.New()}); err != nil || len(rows) != 0 {
			t.Fatalf("GetCitationsByIDs(unknown) = %v, %v; want empty", rows, err)
		}
	}
}

// checkBatch asserts a batched lookup returned exactly want (id -> label), in
// ascending id order, with every row identical to what the single-row getter
// returns for that id on the same branch.
func checkBatch[T any](t *testing.T, what string, got []T, idOf func(T) uuid.UUID, label func(T) string,
	single func(uuid.UUID) (*T, error), want map[uuid.UUID]string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s: got %d rows, want %d", what, len(got), len(want))
	}
	for i, row := range got {
		id := idOf(row)
		if i > 0 && idOf(got[i-1]).String() >= id.String() {
			t.Fatalf("%s: rows not in ascending id order at %d", what, i)
		}
		wantLabel, ok := want[id]
		if !ok {
			t.Fatalf("%s: unexpected id %s", what, id)
		}
		if l := label(row); l != wantLabel {
			t.Fatalf("%s: id %s label = %q, want %q", what, id, l, wantLabel)
		}
		one, err := single(id)
		if err != nil || one == nil {
			t.Fatalf("%s: single-row getter for %s = %v, %v", what, id, one, err)
		}
		if !reflect.DeepEqual(*one, row) {
			t.Fatalf("%s: batch row differs from single-row getter for %s:\n batch  %+v\n single %+v", what, id, row, *one)
		}
	}
}

func mustNoErr(t *testing.T, what string, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("%s: %v", what, err)
	}
}
