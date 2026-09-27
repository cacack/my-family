package memory_test

import (
	"context"
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/repository/memory"
)

// TestKinshipLookup_Parity runs the #829 batched kinship-lookup scenario against
// the in-memory backend; the sqlite and postgres packages run the same body (DB-001).
func TestKinshipLookup_Parity(t *testing.T) {
	runKinshipLookupScenario(t, memory.NewReadModelStore())
}

// runKinshipLookupScenario is the backend-agnostic body of the #829 batched
// kinship-lookup parity test. Each backend package carries an identical copy
// (there is no shared test harness in this repo); keeping it byte-identical is
// the DB-001 parity guarantee. It proves GetPedigreeEdgesByPersonIDs,
// GetFamiliesForPersons and GetFamilyChildrenByFamilyIDs resolve the ADR-005
// overlay exactly as the single-row reads do — a branch row wins, a branch
// tombstone hides main's row, an untouched row falls back to main, another
// branch never leaks — and return their rows in the documented order. Fixtures
// use neutral placeholder names only.
func runKinshipLookupScenario(t *testing.T, store repository.ReadModelStore) {
	t.Helper()
	ctx := context.Background()
	branch := domain.BranchID(uuid.New())
	other := domain.BranchID(uuid.New())
	now := time.Now().UTC().Truncate(time.Second)
	byString := func(ids ...uuid.UUID) []uuid.UUID {
		out := append([]uuid.UUID(nil), ids...)
		sort.Slice(out, func(i, j int) bool { return out[i].String() < out[j].String() })
		return out
	}

	// Empty inputs never touch the store.
	edges, err := store.GetPedigreeEdgesByPersonIDs(ctx, branch, nil)
	mustNoErr(t, "edges of nobody", err)
	families, err := store.GetFamiliesForPersons(ctx, branch, nil)
	mustNoErr(t, "families of nobody", err)
	children, err := store.GetFamilyChildrenByFamilyIDs(ctx, branch, nil)
	mustNoErr(t, "children of no family", err)
	if edges != nil || families != nil || children != nil {
		t.Fatalf("empty inputs returned rows: %v %v %v", edges, families, children)
	}

	// --- Pedigree edges ---
	kept, edited, deleted, added, missing := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	mainFather, branchFather, mother := uuid.New(), uuid.New(), uuid.New()
	edge := func(person, father uuid.UUID, fatherName string) *repository.PedigreeEdge {
		return &repository.PedigreeEdge{PersonID: person, FatherID: &father, MotherID: &mother, FatherName: fatherName, MotherName: "Mother Sample"}
	}
	for _, e := range []*repository.PedigreeEdge{edge(kept, mainFather, "Main"), edge(edited, mainFather, "Main"), edge(deleted, mainFather, "Main")} {
		mustNoErr(t, "save main edge", store.SavePedigreeEdge(ctx, domain.MainBranchID, e))
	}
	mustNoErr(t, "save branch edge", store.SavePedigreeEdge(ctx, branch, edge(edited, branchFather, "Branch")))
	mustNoErr(t, "save added edge", store.SavePedigreeEdge(ctx, branch, edge(added, branchFather, "Branch")))
	mustNoErr(t, "delete branch edge", store.DeletePedigreeEdge(ctx, branch, deleted))
	mustNoErr(t, "save other-branch edge", store.SavePedigreeEdge(ctx, other, edge(kept, branchFather, "Leaked")))

	lookup := []uuid.UUID{missing, added, deleted, edited, kept, kept}
	checkEdges := func(what string, b domain.BranchID, want map[uuid.UUID]string) {
		t.Helper()
		got, err := store.GetPedigreeEdgesByPersonIDs(ctx, b, lookup)
		mustNoErr(t, what, err)
		if len(got) != len(want) {
			t.Fatalf("%s: got %d edges, want %d", what, len(got), len(want))
		}
		for i, e := range got {
			if i > 0 && got[i-1].PersonID.String() >= e.PersonID.String() {
				t.Fatalf("%s: edges not in ascending person id order at %d", what, i)
			}
			if e.FatherName != want[e.PersonID] {
				t.Fatalf("%s: edge %s father name = %q, want %q", what, e.PersonID, e.FatherName, want[e.PersonID])
			}
			one, err := store.GetPedigreeEdge(ctx, b, e.PersonID)
			if err != nil || one == nil || !reflect.DeepEqual(*one, e) {
				t.Fatalf("%s: batch edge %+v differs from single-row read %+v (%v)", what, e, one, err)
			}
		}
	}
	checkEdges("edges on main", domain.MainBranchID, map[uuid.UUID]string{kept: "Main", edited: "Main", deleted: "Main"})
	checkEdges("edges on branch", branch, map[uuid.UUID]string{kept: "Main", edited: "Branch", added: "Branch"})

	// --- Families for persons ---
	p1, p2, p3 := uuid.New(), uuid.New(), uuid.New()
	fKept, fBoth, fRepartnered, fDeleted, fAdded, fLeaked := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	family := func(id uuid.UUID, partner1, partner2 *uuid.UUID, label string) *repository.FamilyReadModel {
		return &repository.FamilyReadModel{ID: id, Partner1ID: partner1, Partner2ID: partner2, Partner1GivenName: label,
			Partner1Surname: "Sample", RelationshipType: domain.RelationMarriage, Version: 1, UpdatedAt: now}
	}
	for _, f := range []*repository.FamilyReadModel{
		family(fKept, &p1, nil, "Kept"),
		family(fBoth, &p1, &p2, "Both"),
		family(fRepartnered, &p2, nil, "MainPartner"),
		family(fDeleted, nil, &p1, "Doomed"),
	} {
		mustNoErr(t, "save main family", store.SaveFamily(ctx, domain.MainBranchID, f))
	}
	mustNoErr(t, "repartner family on branch", store.SaveFamily(ctx, branch, family(fRepartnered, &p3, nil, "BranchPartner")))
	mustNoErr(t, "save added family", store.SaveFamily(ctx, branch, family(fAdded, nil, &p1, "Added")))
	mustNoErr(t, "delete branch family", store.DeleteFamily(ctx, branch, fDeleted))
	mustNoErr(t, "save other-branch family", store.SaveFamily(ctx, other, family(fLeaked, &p1, nil, "Leaked")))

	checkFamilies := func(what string, b domain.BranchID, persons []uuid.UUID, want map[uuid.UUID]string) {
		t.Helper()
		got, err := store.GetFamiliesForPersons(ctx, b, persons)
		mustNoErr(t, what, err)
		if len(got) != len(want) {
			t.Fatalf("%s: got %d families, want %d", what, len(got), len(want))
		}
		for i, f := range got {
			if i > 0 && got[i-1].ID.String() >= f.ID.String() {
				t.Fatalf("%s: families not in ascending id order at %d", what, i)
			}
			wantLabel, ok := want[f.ID]
			if !ok || f.Partner1GivenName != wantLabel {
				t.Fatalf("%s: family %s label = %q, want %q (expected: %v)", what, f.ID, f.Partner1GivenName, wantLabel, ok)
			}
			one, err := store.GetFamily(ctx, b, f.ID)
			if err != nil || one == nil || !reflect.DeepEqual(*one, f) {
				t.Fatalf("%s: batch family %+v differs from single-row read %+v (%v)", what, f, one, err)
			}
		}
	}
	checkFamilies("families on main", domain.MainBranchID, []uuid.UUID{p1, p2, p1},
		map[uuid.UUID]string{fKept: "Kept", fBoth: "Both", fRepartnered: "MainPartner", fDeleted: "Doomed"})
	checkFamilies("families on branch", branch, []uuid.UUID{p1, p2},
		map[uuid.UUID]string{fKept: "Kept", fBoth: "Both", fAdded: "Added"})
	checkFamilies("re-partnered family on branch", branch, []uuid.UUID{p3},
		map[uuid.UUID]string{fRepartnered: "BranchPartner"})
	checkFamilies("re-partnered family on main", domain.MainBranchID, []uuid.UUID{p3}, map[uuid.UUID]string{})

	// --- Family children ---
	famA, famB, famC, famNone := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	one, two := 1, 2
	child := func(familyID uuid.UUID, given string, seq *int) *repository.FamilyChildReadModel {
		return &repository.FamilyChildReadModel{FamilyID: familyID, PersonID: uuid.New(), PersonGivenName: given,
			PersonSurname: "Sample", RelationshipType: domain.ChildBiological, Sequence: seq}
	}
	unsequencedA, unsequencedB := child(famA, "Adam", nil), child(famA, "Bree", nil)
	second, first := child(famA, "Zed", &two), child(famA, "Yan", &one)
	keptB, doomedB := child(famB, "Kept", nil), child(famB, "Doomed", nil)
	addedB, addedC := child(famB, "Added", nil), child(famC, "Only", nil)
	for _, c := range []*repository.FamilyChildReadModel{unsequencedB, second, unsequencedA, first, keptB, doomedB} {
		mustNoErr(t, "save main child", store.SaveFamilyChild(ctx, domain.MainBranchID, c))
	}
	mustNoErr(t, "delete branch child", store.DeleteFamilyChild(ctx, branch, famB, doomedB.PersonID))
	mustNoErr(t, "save added child", store.SaveFamilyChild(ctx, branch, addedB))
	mustNoErr(t, "save branch-only child", store.SaveFamilyChild(ctx, branch, addedC))
	mustNoErr(t, "save other-branch child", store.SaveFamilyChild(ctx, other, child(famA, "Leaked", nil)))

	checkChildren := func(what string, b domain.BranchID, want map[uuid.UUID][]string) {
		t.Helper()
		got, err := store.GetFamilyChildrenByFamilyIDs(ctx, b, []uuid.UUID{famNone, famC, famB, famA, famA})
		mustNoErr(t, what, err)
		var gotOrder, wantOrder []string
		for _, c := range got {
			gotOrder = append(gotOrder, c.FamilyID.String()+"/"+c.PersonGivenName)
		}
		for _, familyID := range byString(famA, famB, famC) {
			for _, given := range want[familyID] {
				wantOrder = append(wantOrder, familyID.String()+"/"+given)
			}
			// The same links as the single-family read, whatever its order.
			single, err := store.GetFamilyChildren(ctx, b, familyID)
			mustNoErr(t, what+" single", err)
			var batch []repository.FamilyChildReadModel
			for _, c := range got {
				if c.FamilyID == familyID {
					batch = append(batch, c)
				}
			}
			sortChildren := func(cs []repository.FamilyChildReadModel) {
				sort.Slice(cs, func(i, j int) bool { return cs[i].PersonID.String() < cs[j].PersonID.String() })
			}
			sortChildren(single)
			sortChildren(batch)
			if len(single) != len(batch) || (len(batch) > 0 && !reflect.DeepEqual(single, batch)) {
				t.Fatalf("%s: family %s batch children %+v differ from single-family read %+v", what, familyID, batch, single)
			}
		}
		if !reflect.DeepEqual(gotOrder, wantOrder) {
			t.Fatalf("%s: children = %v, want %v", what, gotOrder, wantOrder)
		}
	}
	checkChildren("children on main", domain.MainBranchID, map[uuid.UUID][]string{
		famA: {"Yan", "Zed", "Adam", "Bree"},
		famB: {"Doomed", "Kept"},
	})
	checkChildren("children on branch", branch, map[uuid.UUID][]string{
		famA: {"Yan", "Zed", "Adam", "Bree"},
		famB: {"Added", "Kept"},
		famC: {"Only"},
	})
}
