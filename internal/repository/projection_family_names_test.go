package repository_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// TestProjector_FamilyMemberNamesFollowThePerson proves the partner and child
// names a family shows follow the person (#1129, #817, #1207): a rename by
// PersonUpdated, by a primary NameUpdated or by a merge's resolved fields
// shows in them, and a person created after the family that names them (the
// order a branch merge can replay in) fills them in.
func TestProjector_FamilyMemberNamesFollowThePerson(t *testing.T) {
	for backend, readStore := range fullNameStores(t) {
		t.Run(backend, func(t *testing.T) {
			ctx := context.Background()
			projector := repository.NewProjector(readStore, nil)
			project := func(t *testing.T, branchID domain.BranchID, event domain.Event, version int64) {
				t.Helper()
				if err := projector.Project(ctx, event, version, branchID); err != nil {
					t.Fatalf("Project %s: %v", event.EventType(), err)
				}
			}
			onMain := func(t *testing.T, event domain.Event, version int64) {
				t.Helper()
				project(t, domain.MainBranchID, event, version)
			}
			person := func(t *testing.T, given, surname string) *domain.Person {
				t.Helper()
				p := domain.NewPerson(given, surname)
				onMain(t, domain.NewPersonCreated(p), 1)
				return p
			}
			// family links partner as partner 1 of one family and child as a
			// child of it, and child as partner 2 of a second family, so a
			// person shows up both as a partner and as a child.
			family := func(t *testing.T, partner, child *domain.Person) (parents, own *domain.Family) {
				t.Helper()
				parents = domain.NewFamilyWithPartners(&partner.ID, nil)
				onMain(t, domain.NewFamilyCreated(parents), 1)
				onMain(t, domain.NewChildLinkedToFamily(domain.NewFamilyChild(parents.ID, child.ID, domain.ChildBiological)), 2)
				own = domain.NewFamilyWithPartners(nil, &child.ID)
				onMain(t, domain.NewFamilyCreated(own), 1)
				return parents, own
			}

			t.Run("person update renames partner and child", func(t *testing.T) {
				parent := person(t, "Alden", "Original")
				child := person(t, "Wren", "Original")
				parents, own := family(t, parent, child)

				onMain(t, domain.NewPersonUpdated(parent.ID, map[string]any{"given_name": "Alder", "surname": "Renamed"}), 2)
				onMain(t, domain.NewPersonUpdated(child.ID, map[string]any{"surname": "Married"}), 2)

				assertPartner1(t, readStore, domain.MainBranchID, parents.ID, "Alder", "Renamed")
				assertChild(t, readStore, domain.MainBranchID, parents.ID, child.ID, "Wren", "Married")
				assertPartner2(t, readStore, domain.MainBranchID, own.ID, "Wren", "Married")
			})

			t.Run("primary name update renames partner and child", func(t *testing.T) {
				parent := person(t, "Briar", "Original")
				child := person(t, "Ellis", "Original")
				parents, own := family(t, parent, child)
				primary := domain.NewPersonName(child.ID, "Ellis", "Original")
				primary.IsPrimary = true
				onMain(t, domain.NewNameAdded(primary), 2)

				primary.GivenName, primary.Surname = "Elliot", "Renamed"
				onMain(t, domain.NewNameUpdated(primary), 3)

				assertChild(t, readStore, domain.MainBranchID, parents.ID, child.ID, "Elliot", "Renamed")
				assertPartner2(t, readStore, domain.MainBranchID, own.ID, "Elliot", "Renamed")
			})

			t.Run("merge resolution renames the survivor in its families", func(t *testing.T) {
				parent := person(t, "Casey", "Original")
				child := person(t, "Dana", "Original")
				parents, own := family(t, parent, child)
				duplicate := person(t, "Danielle", "Other")

				onMain(t, domain.NewPersonMerged(child.ID, duplicate.ID, map[string]any{},
					map[string]any{"given_name": "Danielle"}, nil, nil, nil, nil, nil), 2)

				assertChild(t, readStore, domain.MainBranchID, parents.ID, child.ID, "Danielle", "Original")
				assertPartner2(t, readStore, domain.MainBranchID, own.ID, "Danielle", "Original")
			})

			t.Run("person created after its families fills in the names", func(t *testing.T) {
				parent := domain.NewPerson("Frankie", "Late")
				child := domain.NewPerson("Gale", "Late")
				parents := domain.NewFamilyWithPartners(&parent.ID, nil)
				onMain(t, domain.NewFamilyCreated(parents), 1)
				onMain(t, domain.NewChildLinkedToFamily(domain.NewFamilyChild(parents.ID, child.ID, domain.ChildBiological)), 2)
				assertChild(t, readStore, domain.MainBranchID, parents.ID, child.ID, "", "")

				onMain(t, domain.NewPersonCreated(child), 1)
				onMain(t, domain.NewPersonCreated(parent), 1)

				assertPartner1(t, readStore, domain.MainBranchID, parents.ID, "Frankie", "Late")
				assertChild(t, readStore, domain.MainBranchID, parents.ID, child.ID, "Gale", "Late")
			})

			t.Run("parent rename refreshes the children's pedigree edges", func(t *testing.T) {
				father := domain.NewPerson("Lane", "Original")
				father.Gender = domain.GenderMale
				onMain(t, domain.NewPersonCreated(father), 1)
				mother := person(t, "Morgan", "Original")
				child := person(t, "Noel", "Original")
				parents := domain.NewFamilyWithPartners(&father.ID, &mother.ID)
				onMain(t, domain.NewFamilyCreated(parents), 1)
				onMain(t, domain.NewChildLinkedToFamily(domain.NewFamilyChild(parents.ID, child.ID, domain.ChildBiological)), 2)
				primary := domain.NewPersonName(mother.ID, "Morgan", "Original")
				primary.IsPrimary = true
				onMain(t, domain.NewNameAdded(primary), 2)

				onMain(t, domain.NewPersonUpdated(father.ID, map[string]any{"surname": "Renamed"}), 2)
				primary.Surname = "Renamed"
				onMain(t, domain.NewNameUpdated(primary), 3)

				edge, err := readStore.GetPedigreeEdge(ctx, domain.MainBranchID, child.ID)
				if err != nil || edge == nil {
					t.Fatalf("GetPedigreeEdge = %v, %v", edge, err)
				}
				if edge.FatherName != "Lane Renamed" || edge.MotherName != "Morgan Renamed" {
					t.Errorf("pedigree names = %q / %q, want %q / %q", edge.FatherName, edge.MotherName, "Lane Renamed", "Morgan Renamed")
				}
			})

			// A rename on a branch shows in every family the branch sees, edited
			// there or not, and main keeps its names. No family is shadowed just
			// to carry a name, so main's later edits to a family the branch never
			// edited stay visible on the branch (ADR-005 live overlay), and a
			// main rename shows in the branch's shadows (#1207).
			t.Run("rename on a branch shows only on the branch", func(t *testing.T) {
				parent := person(t, "Harper", "Original")
				child := person(t, "Indy", "Original")
				parents, own := family(t, parent, child)
				untouched := domain.NewFamilyWithPartners(&child.ID, nil)
				onMain(t, domain.NewFamilyCreated(untouched), 1)
				branch := domain.BranchID(uuid.New())
				project(t, branch, domain.NewFamilyUpdated(own.ID, map[string]any{"marriage_place": "Branchville"}), 2)
				project(t, branch, domain.NewChildLinkedToFamily(domain.NewFamilyChild(parents.ID, child.ID, domain.ChildAdopted)), 3)

				project(t, branch, domain.NewPersonUpdated(child.ID, map[string]any{"surname": "Branch"}), 2)

				assertChild(t, readStore, branch, parents.ID, child.ID, "Indy", "Branch")
				assertPartner2(t, readStore, branch, own.ID, "Indy", "Branch")
				assertPartner1(t, readStore, branch, untouched.ID, "Indy", "Branch")
				assertChild(t, readStore, domain.MainBranchID, parents.ID, child.ID, "Indy", "Original")
				assertPartner2(t, readStore, domain.MainBranchID, own.ID, "Indy", "Original")
				assertPartner1(t, readStore, domain.MainBranchID, untouched.ID, "Indy", "Original")

				onMain(t, domain.NewFamilyUpdated(untouched.ID, map[string]any{"marriage_place": "Mainville"}), 2)
				if got := mustFamily(t, readStore, branch, untouched.ID); got.MarriagePlace != "Mainville" || got.Version != 2 {
					t.Errorf("branch sees untouched family at place %q version %d, want main's Mainville at 2", got.MarriagePlace, got.Version)
				}

				onMain(t, domain.NewPersonUpdated(parent.ID, map[string]any{"surname": "Mainline"}), 2)
				assertPartner1(t, readStore, branch, parents.ID, "Harper", "Mainline")
			})
		})
	}
}

// TestProjector_FamilyMemberNamesReplay proves a read model rebuilt from the
// log carries the same family rows as the one projected live (#1129).
func TestProjector_FamilyMemberNamesReplay(t *testing.T) {
	parent := domain.NewPerson("Jordan", "Original")
	child := domain.NewPerson("Kai", "Original")
	parents := domain.NewFamilyWithPartners(&parent.ID, nil)
	primary := domain.NewPersonName(child.ID, "Kai", "Original")
	primary.IsPrimary = true
	renamedPrimary := *primary
	renamedPrimary.GivenName, renamedPrimary.Surname = "Kit", "Primary"
	type entry struct {
		event   domain.Event
		version int64
	}
	log := []entry{
		{domain.NewFamilyCreated(parents), 1},
		{domain.NewChildLinkedToFamily(domain.NewFamilyChild(parents.ID, child.ID, domain.ChildBiological)), 2},
		{domain.NewPersonCreated(parent), 1},
		{domain.NewPersonCreated(child), 1},
		{domain.NewNameAdded(primary), 2},
		{domain.NewPersonUpdated(parent.ID, map[string]any{"surname": "Renamed"}), 2},
		{domain.NewNameUpdated(&renamedPrimary), 3},
	}

	ctx := context.Background()
	stores := fullNameStores(t)
	for backend, readStore := range stores {
		t.Run(backend, func(t *testing.T) {
			projector := repository.NewProjector(readStore, nil)
			for _, e := range log {
				if err := projector.Project(ctx, e.event, e.version, domain.MainBranchID); err != nil {
					t.Fatalf("Project %s: %v", e.event.EventType(), err)
				}
			}
			assertPartner1(t, readStore, domain.MainBranchID, parents.ID, "Jordan", "Renamed")
			assertChild(t, readStore, domain.MainBranchID, parents.ID, child.ID, "Kit", "Primary")
		})
	}
	// A second rebuild of the same log lands on the same rows.
	for backend, readStore := range fullNameStores(t) {
		t.Run(backend+" rebuilt", func(t *testing.T) {
			projector := repository.NewProjector(readStore, nil)
			for _, e := range log {
				if err := projector.Project(ctx, e.event, e.version, domain.MainBranchID); err != nil {
					t.Fatalf("Project %s: %v", e.event.EventType(), err)
				}
			}
			live := stores[backend]
			wantFamily, _ := live.GetFamily(ctx, domain.MainBranchID, parents.ID)
			gotFamily, _ := readStore.GetFamily(ctx, domain.MainBranchID, parents.ID)
			if wantFamily == nil || gotFamily == nil ||
				gotFamily.Partner1GivenName != wantFamily.Partner1GivenName || gotFamily.Partner1Surname != wantFamily.Partner1Surname ||
				gotFamily.ChildCount != wantFamily.ChildCount || gotFamily.Version != wantFamily.Version {
				t.Errorf("rebuilt family = %+v, want %+v", gotFamily, wantFamily)
			}
			wantChildren, _ := live.GetFamilyChildren(ctx, domain.MainBranchID, parents.ID)
			gotChildren, _ := readStore.GetFamilyChildren(ctx, domain.MainBranchID, parents.ID)
			if len(gotChildren) != 1 || len(wantChildren) != 1 || gotChildren[0] != wantChildren[0] {
				t.Errorf("rebuilt children = %+v, want %+v", gotChildren, wantChildren)
			}
		})
	}
}

func mustFamily(t *testing.T, rs repository.ReadModelStore, branchID domain.BranchID, id uuid.UUID) *repository.FamilyReadModel {
	t.Helper()
	f, err := rs.GetFamily(context.Background(), branchID, id)
	if err != nil || f == nil {
		t.Fatalf("GetFamily %s = %v, %v", id, f, err)
	}
	return f
}

func assertPartner1(t *testing.T, rs repository.ReadModelStore, branchID domain.BranchID, familyID uuid.UUID, given, surname string) {
	t.Helper()
	f := mustFamily(t, rs, branchID, familyID)
	if f.Partner1GivenName != given || f.Partner1Surname != surname {
		t.Errorf("partner 1 = %q %q, want %q %q", f.Partner1GivenName, f.Partner1Surname, given, surname)
	}
}

func assertPartner2(t *testing.T, rs repository.ReadModelStore, branchID domain.BranchID, familyID uuid.UUID, given, surname string) {
	t.Helper()
	f := mustFamily(t, rs, branchID, familyID)
	if f.Partner2GivenName != given || f.Partner2Surname != surname {
		t.Errorf("partner 2 = %q %q, want %q %q", f.Partner2GivenName, f.Partner2Surname, given, surname)
	}
}

func assertChild(t *testing.T, rs repository.ReadModelStore, branchID domain.BranchID, familyID, personID uuid.UUID, given, surname string) {
	t.Helper()
	children, err := rs.GetFamilyChildren(context.Background(), branchID, familyID)
	if err != nil {
		t.Fatalf("GetFamilyChildren: %v", err)
	}
	for _, c := range children {
		if c.PersonID == personID {
			if c.PersonGivenName != given || c.PersonSurname != surname {
				t.Errorf("child = %q %q, want %q %q", c.PersonGivenName, c.PersonSurname, given, surname)
			}
			return
		}
	}
	t.Errorf("family %s has no child %s", familyID, personID)
}
