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
