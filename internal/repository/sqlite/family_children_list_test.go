package sqlite_test

import (
	"context"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// TestListAllFamilyChildren_Parity runs the #838 whole-tree child-link scenario
// against the sqlite backend; the other backend packages run the same body (DB-001).
func TestListAllFamilyChildren_Parity(t *testing.T) {
	readStore, _, cleanup := setupBranchScenarioStores(t)
	defer cleanup()
	runListAllFamilyChildrenScenario(t, readStore)
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

// TestFamilyMemberNames_Parity runs the #1207 family-member name scenario
// against the sqlite backend; the other backend packages run the same body (DB-001).
func TestFamilyMemberNames_Parity(t *testing.T) {
	readStore, _, cleanup := setupBranchScenarioStores(t)
	defer cleanup()
	runFamilyMemberNamesScenario(t, readStore)
}

// runFamilyMemberNamesScenario is the backend-agnostic body of the #1207
// family-member name resolution. Each backend package carries an identical
// copy (DB-001). It proves every family, child-link and pedigree-edge read
// resolves member names from the person overlay of the scope it reads: a main
// rename shows on main and on a branch that has not renamed the person,
// whether or not the branch shadows the family; a branch rename shows on the
// branch only; a person the branch deleted reads with an empty name; and child
// links order by the resolved names. Fixtures use neutral placeholder names
// only.
func runFamilyMemberNamesScenario(t *testing.T, store repository.ReadModelStore) {
	t.Helper()
	ctx := context.Background()
	branch := domain.BranchID(uuid.New())
	now := time.Now().UTC().Truncate(time.Second)
	person, other := uuid.New(), uuid.New()
	asPartner1, asPartner2, branchOnly, withKids := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	mainKid, branchKid := uuid.New(), uuid.New()

	savePerson := func(scope domain.BranchID, id uuid.UUID, given, surname string) {
		t.Helper()
		if err := store.SavePerson(ctx, scope, &repository.PersonReadModel{
			ID: id, GivenName: given, Surname: surname, FullName: domain.JoinFullName(given, surname),
			Version: 1, UpdatedAt: now,
		}); err != nil {
			t.Fatalf("save person: %v", err)
		}
	}
	// The stored names are deliberately stale: reads must ignore them.
	saveFamily := func(scope domain.BranchID, id uuid.UUID, p1, p2 *uuid.UUID) {
		t.Helper()
		if err := store.SaveFamily(ctx, scope, &repository.FamilyReadModel{
			ID: id, Partner1ID: p1, Partner1GivenName: "Stale", Partner1Surname: "Stale",
			Partner2ID: p2, Partner2GivenName: "Stale", Partner2Surname: "Stale",
			RelationshipType: domain.RelationMarriage, Version: 1, UpdatedAt: now,
		}); err != nil {
			t.Fatalf("save family: %v", err)
		}
	}
	link := func(scope domain.BranchID, family, child uuid.UUID) {
		t.Helper()
		if err := store.SaveFamilyChild(ctx, scope, &repository.FamilyChildReadModel{
			FamilyID: family, PersonID: child, PersonGivenName: "Stale", PersonSurname: "Stale",
			RelationshipType: domain.ChildBiological,
		}); err != nil {
			t.Fatalf("save family child: %v", err)
		}
	}
	saveEdge := func(scope domain.BranchID, edge *repository.PedigreeEdge) {
		t.Helper()
		edge.FatherName, edge.MotherName = "Stale", "Stale"
		if err := store.SavePedigreeEdge(ctx, scope, edge); err != nil {
			t.Fatalf("save pedigree edge: %v", err)
		}
	}
	savePerson(domain.MainBranchID, person, "Old", "Main")
	savePerson(domain.MainBranchID, other, "Other", "Sample")
	saveFamily(domain.MainBranchID, asPartner1, &person, &other)
	saveFamily(domain.MainBranchID, asPartner2, &other, &person)
	saveFamily(branch, asPartner2, &other, &person) // the branch shadows this one
	saveFamily(branch, branchOnly, &person, nil)
	saveFamily(domain.MainBranchID, withKids, nil, nil)
	link(domain.MainBranchID, withKids, person)
	link(domain.MainBranchID, withKids, other)
	saveEdge(domain.MainBranchID, &repository.PedigreeEdge{PersonID: mainKid, FatherID: &person, MotherID: &other})
	saveEdge(branch, &repository.PedigreeEdge{PersonID: branchKid, MotherID: &person})

	partners := func(f repository.FamilyReadModel) string {
		return f.Partner1GivenName + " " + f.Partner1Surname + " & " + f.Partner2GivenName + " " + f.Partner2Surname
	}
	family := func(scope domain.BranchID, id uuid.UUID) string {
		t.Helper()
		f, err := store.GetFamily(ctx, scope, id)
		if err != nil || f == nil {
			t.Fatalf("GetFamily %s = %v, %v", id, f, err)
		}
		return partners(*f)
	}
	children := func(scope domain.BranchID) string {
		t.Helper()
		got, err := store.GetFamilyChildren(ctx, scope, withKids)
		if err != nil {
			t.Fatalf("GetFamilyChildren: %v", err)
		}
		var names []string
		for _, c := range got {
			names = append(names, c.PersonGivenName+" "+c.PersonSurname)
		}
		return strings.Join(names, ", ")
	}
	parents := func(scope domain.BranchID, child uuid.UUID) string {
		t.Helper()
		edge, err := store.GetPedigreeEdge(ctx, scope, child)
		if err != nil || edge == nil {
			t.Fatalf("GetPedigreeEdge %s = %v, %v", child, edge, err)
		}
		return edge.FatherName + " & " + edge.MotherName
	}
	check := func(what, got, want string) {
		t.Helper()
		if got != want {
			t.Errorf("%s = %q, want %q", what, got, want)
		}
	}

	// A main rename shows on main and on the branch, shadowed family or not.
	savePerson(domain.MainBranchID, person, "New", "Main")
	check("main partner-1 family", family(domain.MainBranchID, asPartner1), "New Main & Other Sample")
	check("main children", children(domain.MainBranchID), "New Main, Other Sample")
	check("main pedigree parents", parents(domain.MainBranchID, mainKid), "New Main & Other Sample")
	check("branch view of main's family", family(branch, asPartner1), "New Main & Other Sample")
	check("branch shadow after main rename", family(branch, asPartner2), "Other Sample & New Main")
	check("branch pedigree after main rename", parents(branch, branchKid), " & New Main")

	// A branch rename shows on the branch only, and re-orders its child links.
	savePerson(branch, person, "New", "Zed")
	check("branch view of main's family", family(branch, asPartner1), "New Zed & Other Sample")
	check("branch shadow", family(branch, asPartner2), "Other Sample & New Zed")
	check("branch-only family", family(branch, branchOnly), "New Zed &  ")
	check("branch children", children(branch), "Other Sample, New Zed")
	check("branch pedigree parents", parents(branch, branchKid), " & New Zed")
	check("branch view of main's pedigree", parents(branch, mainKid), "New Zed & Other Sample")
	check("main family after branch rename", family(domain.MainBranchID, asPartner2), "Other Sample & New Main")
	check("main children after branch rename", children(domain.MainBranchID), "New Main, Other Sample")

	// The batched and list reads resolve the same way.
	byIDs, err := store.GetFamiliesByIDs(ctx, branch, []uuid.UUID{asPartner1})
	if err != nil || len(byIDs) != 1 {
		t.Fatalf("GetFamiliesByIDs = %v, %v", byIDs, err)
	}
	check("GetFamiliesByIDs", partners(byIDs[0]), "New Zed & Other Sample")
	forPerson, err := store.GetFamiliesForPerson(ctx, branch, other)
	if err != nil || len(forPerson) != 2 {
		t.Fatalf("GetFamiliesForPerson = %v, %v", forPerson, err)
	}
	for _, f := range forPerson {
		if !strings.Contains(partners(f), "New Zed") {
			t.Errorf("GetFamiliesForPerson family %s = %q, want the branch name", f.ID, partners(f))
		}
	}
	forPersons, err := store.GetFamiliesForPersons(ctx, branch, []uuid.UUID{person})
	if err != nil || len(forPersons) != 3 {
		t.Fatalf("GetFamiliesForPersons = %v, %v", forPersons, err)
	}
	for _, f := range forPersons {
		if !strings.Contains(partners(f), "New Zed") {
			t.Errorf("GetFamiliesForPersons family %s = %q, want the branch name", f.ID, partners(f))
		}
	}
	listed, _, err := store.ListFamilies(ctx, repository.ListOptions{BranchID: branch, Limit: 10})
	if err != nil || len(listed) != 4 {
		t.Fatalf("ListFamilies = %v, %v", listed, err)
	}
	for _, f := range listed {
		if strings.Contains(partners(f), "Stale") || strings.Contains(partners(f), "New Main") {
			t.Errorf("ListFamilies family %s = %q, want only branch names", f.ID, partners(f))
		}
	}
	link(domain.MainBranchID, asPartner1, branchKid)
	childFamily, err := store.GetChildFamily(ctx, branch, branchKid)
	if err != nil || childFamily == nil {
		t.Fatalf("GetChildFamily = %v, %v", childFamily, err)
	}
	check("GetChildFamily", partners(*childFamily), "New Zed & Other Sample")
	all, err := store.ListAllFamilyChildren(ctx, branch)
	if err != nil {
		t.Fatalf("ListAllFamilyChildren: %v", err)
	}
	for _, c := range all {
		if c.PersonID == person {
			check("ListAllFamilyChildren", c.PersonGivenName+" "+c.PersonSurname, "New Zed")
		}
	}
	edges, err := store.GetPedigreeEdgesByPersonIDs(ctx, branch, []uuid.UUID{mainKid})
	if err != nil || len(edges) != 1 {
		t.Fatalf("GetPedigreeEdgesByPersonIDs = %v, %v", edges, err)
	}
	check("GetPedigreeEdgesByPersonIDs", edges[0].FatherName+" & "+edges[0].MotherName, "New Zed & Other Sample")

	// A person the branch deleted reads with an empty name there.
	if err := store.DeletePerson(ctx, branch, other); err != nil {
		t.Fatalf("DeletePerson: %v", err)
	}
	check("branch family after deleting a partner", family(branch, asPartner1), "New Zed &  ")
	check("main family after branch delete", family(domain.MainBranchID, asPartner1), "New Main & Other Sample")
}
