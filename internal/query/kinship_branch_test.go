package query

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/repository/memory"
)

// Branch-scoped descendancy, relationship and Ahnentafel reads (#829), run
// against every backend: the query layer is shared, but the overlay it reads
// through is backend-specific.

// kinshipBackends is every backend, memory included; memory has no statement
// counter.
func kinshipBackends() []sqlBackend {
	return append([]sqlBackend{{name: "memory", open: func(*testing.T) (repository.ReadModelStore, *statementCounter) {
		return memory.NewReadModelStore(), nil
	}}}, countedSQLBackends()...)
}

// kinshipTree writes read-model rows directly, the way the projection would.
type kinshipTree struct {
	t     *testing.T
	ctx   context.Context
	store repository.ReadModelStore
}

func (k kinshipTree) person(branch domain.BranchID, given string) uuid.UUID {
	k.t.Helper()
	id := uuid.New()
	require.NoError(k.t, k.store.SavePerson(k.ctx, branch, &repository.PersonReadModel{
		ID: id, GivenName: given, Surname: "Sample", FullName: given + " Sample", Version: 1,
	}))
	return id
}

func (k kinshipTree) family(branch domain.BranchID, father, mother uuid.UUID) uuid.UUID {
	k.t.Helper()
	id := uuid.New()
	require.NoError(k.t, k.store.SaveFamily(k.ctx, branch, &repository.FamilyReadModel{
		ID: id, Partner1ID: &father, Partner2ID: &mother, RelationshipType: domain.RelationMarriage, Version: 1,
	}))
	return id
}

func (k kinshipTree) child(branch domain.BranchID, familyID, father, mother, childID uuid.UUID) {
	k.t.Helper()
	require.NoError(k.t, k.store.SaveFamilyChild(k.ctx, branch, &repository.FamilyChildReadModel{
		FamilyID: familyID, PersonID: childID, RelationshipType: domain.ChildBiological,
	}))
	require.NoError(k.t, k.store.SavePedigreeEdge(k.ctx, branch, &repository.PedigreeEdge{
		PersonID: childID, FatherID: &father, MotherID: &mother,
	}))
}

func childNames(node *DescendancyNode) []string {
	var names []string
	for _, c := range node.Children {
		names = append(names, c.GivenName)
	}
	return names
}

func TestKinshipReads_FollowTheBranch(t *testing.T) {
	ctx := context.Background()
	for _, backend := range kinshipBackends() {
		t.Run(backend.name, func(t *testing.T) {
			store, _ := backend.open(t)
			k := kinshipTree{t: t, ctx: ctx, store: store}
			main := domain.MainBranchID
			branch := domain.BranchID(uuid.New())

			// Mainline: Father + Mother with children Alpha and Beta.
			father, mother := k.person(main, "Father"), k.person(main, "Mother")
			fam := k.family(main, father, mother)
			alpha, beta := k.person(main, "Alpha"), k.person(main, "Beta")
			k.child(main, fam, father, mother, alpha)
			k.child(main, fam, father, mother, beta)

			// Branch: a new child Gamma, and Alpha deleted with its links.
			gamma := k.person(branch, "Gamma")
			k.child(branch, fam, father, mother, gamma)
			require.NoError(t, store.DeletePerson(ctx, branch, alpha))
			require.NoError(t, store.DeleteFamilyChild(ctx, branch, fam, alpha))
			require.NoError(t, store.DeletePedigreeEdge(ctx, branch, alpha))

			descendancy := NewDescendancyService(store)
			onMain, err := descendancy.GetDescendancy(ctx, GetDescendancyInput{PersonID: father})
			require.NoError(t, err)
			assert.ElementsMatch(t, []string{"Alpha", "Beta"}, childNames(onMain.Root))
			require.Len(t, onMain.Root.Spouses, 1)
			assert.Equal(t, mother, onMain.Root.Spouses[0].ID)

			onBranch, err := descendancy.GetDescendancy(ctx, GetDescendancyInput{PersonID: father, BranchID: branch})
			require.NoError(t, err)
			assert.ElementsMatch(t, []string{"Beta", "Gamma"}, childNames(onBranch.Root))
			assert.Equal(t, 2, onBranch.TotalDescendants)

			_, err = descendancy.GetDescendancy(ctx, GetDescendancyInput{PersonID: gamma})
			assert.ErrorIs(t, err, ErrNotFound, "a branch-created person has no mainline descendancy")
			_, err = descendancy.GetDescendancy(ctx, GetDescendancyInput{PersonID: alpha, BranchID: branch})
			assert.ErrorIs(t, err, ErrNotFound, "a branch-deleted person has no branch descendancy")

			relationship := NewRelationshipService(store)
			rel, err := relationship.GetRelationship(ctx, branch, gamma, beta)
			require.NoError(t, err)
			assert.True(t, rel.IsRelated)
			assert.Equal(t, "sibling (via 2 paths)", rel.Summary, "full siblings share both parents")
			_, err = relationship.GetRelationship(ctx, main, gamma, beta)
			assert.ErrorIs(t, err, ErrNotFound)
			_, err = relationship.GetRelationship(ctx, branch, alpha, beta)
			assert.ErrorIs(t, err, ErrNotFound)
			rel, err = relationship.GetRelationship(ctx, main, alpha, beta)
			require.NoError(t, err)
			assert.Equal(t, "sibling (via 2 paths)", rel.Summary, "full siblings share both parents")

			ahnentafel := NewAhnentafelService(NewPedigreeService(store))
			report, err := ahnentafel.GetAhnentafel(ctx, GetAhnentafelInput{PersonID: gamma, BranchID: branch})
			require.NoError(t, err)
			require.Len(t, report.Entries, 3)
			assert.Equal(t, gamma, report.Entries[0].ID)
			assert.Equal(t, father, report.Entries[1].ID)
			assert.Equal(t, mother, report.Entries[2].ID)
			_, err = ahnentafel.GetAhnentafel(ctx, GetAhnentafelInput{PersonID: gamma})
			assert.ErrorIs(t, err, ErrNotFound)
		})
	}
}

// seedWideTree writes a root couple and `generations` generations below it,
// each person of a generation having one family with `width` children. It
// returns the root's id and the id of one person in the deepest generation
// plus one in its first generation, for the relationship check.
func seedWideTree(t *testing.T, k kinshipTree, branch domain.BranchID, width, generations int) (root, deepest, first uuid.UUID) {
	t.Helper()
	root = k.person(branch, "Root")
	level := []uuid.UUID{root}
	for g := 1; g <= generations; g++ {
		var next []uuid.UUID
		for _, parent := range level {
			spouse := k.person(branch, fmt.Sprintf("Spouse%d", g))
			fam := k.family(branch, parent, spouse)
			for i := range width {
				c := k.person(branch, fmt.Sprintf("Child%d_%d", g, i))
				k.child(branch, fam, parent, spouse, c)
				next = append(next, c)
			}
		}
		if g == 1 {
			first = next[0]
		}
		level = next
	}
	return root, level[len(level)-1], first
}

// TestKinshipReads_SQLStatementCountDoesNotScale is the "no N+1" acceptance
// check (#829): on each SQL backend, walking a branch's descendancy or a
// relationship costs a fixed number of statements per generation, whatever the
// number of people in it.
func TestKinshipReads_SQLStatementCountDoesNotScale(t *testing.T) {
	ctx := context.Background()
	const generations = 3
	for _, backend := range countedSQLBackends() {
		counts := map[string][]int64{}
		for _, width := range []int{1, 4} {
			t.Run(fmt.Sprintf("%s/width %d", backend.name, width), func(t *testing.T) {
				store, counter := backend.open(t)
				k := kinshipTree{t: t, ctx: ctx, store: store}
				branch := domain.BranchID(uuid.New())
				root, deepest, first := seedWideTree(t, k, branch, width, generations)

				counter.reset()
				result, err := NewDescendancyService(store).GetDescendancy(ctx, GetDescendancyInput{
					PersonID: root, MaxGenerations: generations, BranchID: branch,
				})
				require.NoError(t, err)
				want := 0
				for g, n := 1, width; g <= generations; g, n = g+1, n*width {
					want += n
				}
				assert.Equal(t, want, result.TotalDescendants)
				counts["descendancy"] = append(counts["descendancy"], counter.count())

				counter.reset()
				rel, err := NewRelationshipService(store).GetRelationship(ctx, branch, deepest, first)
				require.NoError(t, err)
				assert.True(t, rel.IsRelated)
				counts["relationship"] = append(counts["relationship"], counter.count())
			})
		}
		for what, c := range counts {
			require.Len(t, c, 2, what)
			assert.Equal(t, c[0], c[1], "%s/%s: statement count must not grow with the number of people", backend.name, what)
		}
	}
}
