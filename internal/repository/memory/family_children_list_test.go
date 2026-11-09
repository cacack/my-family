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

// TestListAllFamilyChildren_Parity runs the #838 whole-tree child-link scenario
// against the in-memory backend; the other backend packages run the same body (DB-001).
func TestListAllFamilyChildren_Parity(t *testing.T) {
	runListAllFamilyChildrenScenario(t, memory.NewReadModelStore())
}

// runListAllFamilyChildrenScenario is the backend-agnostic body of the #838
// whole-tree child-link read. Each backend package carries an identical copy
// (DB-001). It proves ListAllFamilyChildren returns, in one call, exactly the
// links GetFamilyChildren returns family by family on the same scope — a branch
// link added, a branch unlink hiding main's link, a branch edit shadowing main's
// row, another branch's links never leaking — ordered by family id then person
// id. Fixtures use neutral placeholder names only.
func runListAllFamilyChildrenScenario(t *testing.T, store repository.ReadModelStore) {
	t.Helper()
	ctx := context.Background()
	branch := domain.BranchID(uuid.New())
	other := domain.BranchID(uuid.New())
	now := time.Now().UTC().Truncate(time.Second)

	people := make([]uuid.UUID, 6)
	for i := range people {
		people[i] = uuid.New()
		if err := store.SavePerson(ctx, domain.MainBranchID, &repository.PersonReadModel{
			ID: people[i], GivenName: "Child", Surname: "Sample", FullName: "Child Sample", Version: 1, UpdatedAt: now,
		}); err != nil {
			t.Fatalf("save person: %v", err)
		}
	}
	families := []uuid.UUID{uuid.New(), uuid.New(), uuid.New()}
	for _, id := range families {
		if err := store.SaveFamily(ctx, domain.MainBranchID, &repository.FamilyReadModel{
			ID: id, RelationshipType: domain.RelationMarriage, Version: 1, UpdatedAt: now,
		}); err != nil {
			t.Fatalf("save family: %v", err)
		}
	}
	link := func(scope domain.BranchID, family, person uuid.UUID, rel domain.ChildRelationType) {
		t.Helper()
		if err := store.SaveFamilyChild(ctx, scope, &repository.FamilyChildReadModel{
			FamilyID: family, PersonID: person, PersonGivenName: "Child", PersonSurname: "Sample", RelationshipType: rel,
		}); err != nil {
			t.Fatalf("save family child: %v", err)
		}
	}
	link(domain.MainBranchID, families[0], people[0], domain.ChildBiological)
	link(domain.MainBranchID, families[0], people[1], domain.ChildBiological)
	link(domain.MainBranchID, families[1], people[2], domain.ChildBiological)
	link(domain.MainBranchID, families[2], people[3], domain.ChildBiological)
	link(branch, families[0], people[4], domain.ChildBiological)
	if err := store.DeleteFamilyChild(ctx, branch, families[0], people[1]); err != nil {
		t.Fatalf("unlink on branch: %v", err)
	}
	link(branch, families[1], people[2], domain.ChildAdopted)
	link(other, families[2], people[5], domain.ChildFoster)

	type key struct {
		family, person uuid.UUID
		rel            domain.ChildRelationType
	}
	keysOf := func(rows []repository.FamilyChildReadModel) []key {
		out := make([]key, 0, len(rows))
		for _, r := range rows {
			out = append(out, key{r.FamilyID, r.PersonID, r.RelationshipType})
		}
		return out
	}
	for _, tc := range []struct {
		name  string
		scope domain.BranchID
		want  []key
	}{
		{"main", domain.MainBranchID, []key{
			{families[0], people[0], domain.ChildBiological}, {families[0], people[1], domain.ChildBiological},
			{families[1], people[2], domain.ChildBiological}, {families[2], people[3], domain.ChildBiological},
		}},
		{"branch", branch, []key{
			{families[0], people[0], domain.ChildBiological}, {families[0], people[4], domain.ChildBiological},
			{families[1], people[2], domain.ChildAdopted}, {families[2], people[3], domain.ChildBiological},
		}},
	} {
		got, err := store.ListAllFamilyChildren(ctx, tc.scope)
		if err != nil {
			t.Fatalf("%s: list all family children: %v", tc.name, err)
		}
		want := append([]key(nil), tc.want...)
		sort.Slice(want, func(i, j int) bool {
			if want[i].family != want[j].family {
				return want[i].family.String() < want[j].family.String()
			}
			return want[i].person.String() < want[j].person.String()
		})
		if !reflect.DeepEqual(keysOf(got), want) {
			t.Errorf("%s: ListAllFamilyChildren = %v, want %v", tc.name, keysOf(got), want)
		}

		// Parity with the per-family read on the same scope.
		var perFamily []key
		for _, family := range families {
			rows, err := store.GetFamilyChildren(ctx, tc.scope, family)
			if err != nil {
				t.Fatalf("%s: get family children: %v", tc.name, err)
			}
			perFamily = append(perFamily, keysOf(rows)...)
		}
		sort.Slice(perFamily, func(i, j int) bool {
			if perFamily[i].family != perFamily[j].family {
				return perFamily[i].family.String() < perFamily[j].family.String()
			}
			return perFamily[i].person.String() < perFamily[j].person.String()
		})
		if !reflect.DeepEqual(perFamily, want) {
			t.Errorf("%s: per-family reads = %v, want %v", tc.name, perFamily, want)
		}
	}
}

// TestRenameFamilyMember_Parity runs the #1129 family-member rename scenario
// against the memory backend; the other backend packages run the same body (DB-001).
func TestRenameFamilyMember_Parity(t *testing.T) {
	runRenameFamilyMemberScenario(t, memory.NewReadModelStore())
}

// runRenameFamilyMemberScenario is the backend-agnostic body of the #1129
// family-member rename. Each backend package carries an identical copy
// (DB-001). It proves RenameFamilyMember rewrites the person's partner and
// child names, and its children's pedigree parent names, on the rows stored
// under the given scope only: a main rename leaves branch rows alone, a branch
// rename leaves main rows alone and writes no shadow for a family the branch
// holds no row of, and other people's names never change. Fixtures use neutral placeholder names only.
func runRenameFamilyMemberScenario(t *testing.T, store repository.ReadModelStore) {
	t.Helper()
	ctx := context.Background()
	branch := domain.BranchID(uuid.New())
	now := time.Now().UTC().Truncate(time.Second)
	person, other := uuid.New(), uuid.New()
	asPartner1, asPartner2, branchOnly, mainOnly := uuid.New(), uuid.New(), uuid.New(), uuid.New()

	saveFamily := func(scope domain.BranchID, id uuid.UUID, p1, p2 *uuid.UUID) {
		t.Helper()
		if err := store.SaveFamily(ctx, scope, &repository.FamilyReadModel{
			ID: id, Partner1ID: p1, Partner1GivenName: "Old", Partner1Surname: "Sample",
			Partner2ID: p2, Partner2GivenName: "Old", Partner2Surname: "Sample",
			RelationshipType: domain.RelationMarriage, Version: 1, UpdatedAt: now,
		}); err != nil {
			t.Fatalf("save family: %v", err)
		}
	}
	link := func(scope domain.BranchID, family, child uuid.UUID) {
		t.Helper()
		if err := store.SaveFamilyChild(ctx, scope, &repository.FamilyChildReadModel{
			FamilyID: family, PersonID: child, PersonGivenName: "Old", PersonSurname: "Sample", RelationshipType: domain.ChildBiological,
		}); err != nil {
			t.Fatalf("save family child: %v", err)
		}
	}
	saveFamily(domain.MainBranchID, asPartner1, &person, &other)
	saveFamily(domain.MainBranchID, asPartner2, &other, &person)
	saveFamily(branch, asPartner2, &other, &person)
	saveFamily(branch, branchOnly, &person, nil)
	saveFamily(domain.MainBranchID, mainOnly, nil, nil)
	link(domain.MainBranchID, mainOnly, person)
	link(domain.MainBranchID, mainOnly, other)
	link(branch, branchOnly, person)
	mainKid, branchKid := uuid.New(), uuid.New()
	for _, e := range []struct {
		scope domain.BranchID
		edge  *repository.PedigreeEdge
	}{
		{domain.MainBranchID, &repository.PedigreeEdge{PersonID: mainKid, FatherID: &person, FatherName: "Old Sample", MotherID: &other, MotherName: "Old Sample"}},
		{branch, &repository.PedigreeEdge{PersonID: branchKid, MotherID: &person, MotherName: "Old Sample"}},
	} {
		if err := store.SavePedigreeEdge(ctx, e.scope, e.edge); err != nil {
			t.Fatalf("save pedigree edge: %v", err)
		}
	}
	parents := func(scope domain.BranchID, child uuid.UUID) string {
		t.Helper()
		edge, err := store.GetPedigreeEdge(ctx, scope, child)
		if err != nil || edge == nil {
			t.Fatalf("GetPedigreeEdge %s = %v, %v", child, edge, err)
		}
		return edge.FatherName + " & " + edge.MotherName
	}

	family := func(scope domain.BranchID, id uuid.UUID) *repository.FamilyReadModel {
		t.Helper()
		f, err := store.GetFamily(ctx, scope, id)
		if err != nil || f == nil {
			t.Fatalf("GetFamily %s = %v, %v", id, f, err)
		}
		return f
	}
	childName := func(scope domain.BranchID, familyID, child uuid.UUID) string {
		t.Helper()
		children, err := store.GetFamilyChildren(ctx, scope, familyID)
		if err != nil {
			t.Fatalf("GetFamilyChildren: %v", err)
		}
		for _, c := range children {
			if c.PersonID == child {
				return c.PersonGivenName + " " + c.PersonSurname
			}
		}
		t.Fatalf("family %s has no child %s", familyID, child)
		return ""
	}
	partners := func(f *repository.FamilyReadModel) string {
		return f.Partner1GivenName + " " + f.Partner1Surname + " & " + f.Partner2GivenName + " " + f.Partner2Surname
	}

	if err := store.RenameFamilyMember(ctx, domain.MainBranchID, person, "New", "Main"); err != nil {
		t.Fatalf("main RenameFamilyMember: %v", err)
	}
	if got, want := partners(family(domain.MainBranchID, asPartner1)), "New Main & Old Sample"; got != want {
		t.Errorf("main partner-1 family = %q, want %q", got, want)
	}
	if got, want := partners(family(domain.MainBranchID, asPartner2)), "Old Sample & New Main"; got != want {
		t.Errorf("main partner-2 family = %q, want %q", got, want)
	}
	if got, want := childName(domain.MainBranchID, mainOnly, person), "New Main"; got != want {
		t.Errorf("main child = %q, want %q", got, want)
	}
	if got, want := childName(domain.MainBranchID, mainOnly, other), "Old Sample"; got != want {
		t.Errorf("main sibling = %q, want %q (another person)", got, want)
	}
	if got, want := parents(domain.MainBranchID, mainKid), "New Main & Old Sample"; got != want {
		t.Errorf("main pedigree parents = %q, want %q", got, want)
	}
	if got, want := parents(branch, branchKid), " & Old Sample"; got != want {
		t.Errorf("branch pedigree after main rename = %q, want %q (BR-003)", got, want)
	}
	if got, want := partners(family(branch, asPartner2)), "Old Sample & Old Sample"; got != want {
		t.Errorf("branch shadow after main rename = %q, want %q (BR-003)", got, want)
	}

	if err := store.RenameFamilyMember(ctx, branch, person, "New", "Branch"); err != nil {
		t.Fatalf("branch RenameFamilyMember: %v", err)
	}
	if got, want := partners(family(branch, asPartner2)), "Old Sample & New Branch"; got != want {
		t.Errorf("branch shadow = %q, want %q", got, want)
	}
	if got, want := partners(family(branch, branchOnly)), "New Branch & Old Sample"; got != want {
		t.Errorf("branch-only family = %q, want %q", got, want)
	}
	if got, want := childName(branch, branchOnly, person), "New Branch"; got != want {
		t.Errorf("branch child = %q, want %q", got, want)
	}
	if got, want := parents(branch, branchKid), " & New Branch"; got != want {
		t.Errorf("branch pedigree parents = %q, want %q", got, want)
	}
	if got, want := parents(domain.MainBranchID, mainKid), "New Main & Old Sample"; got != want {
		t.Errorf("main pedigree after branch rename = %q, want %q", got, want)
	}
	if got, want := partners(family(domain.MainBranchID, asPartner2)), "Old Sample & New Main"; got != want {
		t.Errorf("main family after branch rename = %q, want %q", got, want)
	}
	// The branch holds no row of asPartner1 or mainOnly: it still sees main's.
	if got, want := partners(family(branch, asPartner1)), "New Main & Old Sample"; got != want {
		t.Errorf("branch view of main's family = %q, want main's %q (no shadow written)", got, want)
	}
	if got, want := childName(branch, mainOnly, person), "New Main"; got != want {
		t.Errorf("branch view of main's child = %q, want main's %q (no shadow written)", got, want)
	}
}
