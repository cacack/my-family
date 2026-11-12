package integration_test

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// Batched external-ID reads (#617). Each List*ExternalIDs must give, for every
// parent at once, exactly what Get*ExternalIDs gives for that parent — on main
// and through a branch overlay — on every backend.

// exidKind adapts one external-ID bucket type to strings ("seq:value:type") so
// the scenario below is written once for all of them.
type exidKind struct {
	name     string
	branched bool // false for repositories, which are main-only
	replace  func(ctx context.Context, b domain.BranchID, parent uuid.UUID, values []string) error
	get      func(ctx context.Context, b domain.BranchID, parent uuid.UUID) ([]string, error)
	list     func(ctx context.Context, b domain.BranchID) (map[uuid.UUID][]string, error)
}

// newExidKind builds an exidKind from a bucket type's typed methods. mk builds
// an ID from a value; row reports an ID's parent, sequence, value and type.
func newExidKind[T any](name string, branched bool,
	replace func(context.Context, domain.BranchID, uuid.UUID, []T) error,
	get func(context.Context, domain.BranchID, uuid.UUID) ([]T, error),
	list func(context.Context, domain.BranchID) (map[uuid.UUID][]T, error),
	mk func(value string) T,
	row func(T) (uuid.UUID, int, string, string),
) exidKind {
	format := func(parent uuid.UUID, ids []T) ([]string, error) {
		var out []string
		for _, id := range ids {
			p, seq, value, typ := row(id)
			if p != parent {
				return nil, fmt.Errorf("%s id %q carries parent %s, want %s", name, value, p, parent)
			}
			out = append(out, fmt.Sprintf("%d:%s:%s", seq, value, typ))
		}
		return out, nil
	}
	return exidKind{
		name:     name,
		branched: branched,
		replace: func(ctx context.Context, b domain.BranchID, parent uuid.UUID, values []string) error {
			ids := make([]T, len(values))
			for i, v := range values {
				ids[i] = mk(v)
			}
			return replace(ctx, b, parent, ids)
		},
		get: func(ctx context.Context, b domain.BranchID, parent uuid.UUID) ([]string, error) {
			ids, err := get(ctx, b, parent)
			if err != nil {
				return nil, err
			}
			return format(parent, ids)
		},
		list: func(ctx context.Context, b domain.BranchID) (map[uuid.UUID][]string, error) {
			byParent, err := list(ctx, b)
			if err != nil {
				return nil, err
			}
			out := make(map[uuid.UUID][]string, len(byParent))
			for parent, ids := range byParent {
				if out[parent], err = format(parent, ids); err != nil {
					return nil, err
				}
			}
			return out, nil
		},
	}
}

func exidKinds(t *testing.T, rs repository.ReadModelStore) []exidKind {
	mainOnly := func(b domain.BranchID) {
		if !b.IsMain() {
			t.Fatalf("repository external IDs are main-only")
		}
	}
	return []exidKind{
		newExidKind("person", true, rs.ReplacePersonExternalIDs, rs.GetPersonExternalIDs, rs.ListPersonExternalIDs,
			func(v string) repository.PersonExternalIDReadModel {
				return repository.PersonExternalIDReadModel{Value: v, Type: "t-" + v}
			},
			func(x repository.PersonExternalIDReadModel) (uuid.UUID, int, string, string) {
				return x.PersonID, x.Sequence, x.Value, x.Type
			}),
		newExidKind("family", true, rs.ReplaceFamilyExternalIDs, rs.GetFamilyExternalIDs, rs.ListFamilyExternalIDs,
			func(v string) repository.FamilyExternalIDReadModel {
				return repository.FamilyExternalIDReadModel{Value: v, Type: "t-" + v}
			},
			func(x repository.FamilyExternalIDReadModel) (uuid.UUID, int, string, string) {
				return x.FamilyID, x.Sequence, x.Value, x.Type
			}),
		newExidKind("source", true, rs.ReplaceSourceExternalIDs, rs.GetSourceExternalIDs, rs.ListSourceExternalIDs,
			func(v string) repository.SourceExternalIDReadModel {
				return repository.SourceExternalIDReadModel{Value: v, Type: "t-" + v}
			},
			func(x repository.SourceExternalIDReadModel) (uuid.UUID, int, string, string) {
				return x.SourceID, x.Sequence, x.Value, x.Type
			}),
		newExidKind("citation", true, rs.ReplaceCitationExternalIDs, rs.GetCitationExternalIDs, rs.ListCitationExternalIDs,
			func(v string) repository.CitationExternalIDReadModel {
				return repository.CitationExternalIDReadModel{Value: v, Type: "t-" + v}
			},
			func(x repository.CitationExternalIDReadModel) (uuid.UUID, int, string, string) {
				return x.CitationID, x.Sequence, x.Value, x.Type
			}),
		newExidKind("repository", false,
			func(ctx context.Context, b domain.BranchID, parent uuid.UUID, ids []repository.RepositoryExternalIDReadModel) error {
				mainOnly(b)
				// repository_external_ids has a foreign key to its repository.
				if err := rs.SaveRepository(ctx, &repository.RepositoryReadModel{ID: parent, Name: "Archive", Version: 1, UpdatedAt: time.Now()}); err != nil {
					return err
				}
				return rs.ReplaceRepositoryExternalIDs(ctx, parent, ids)
			},
			func(ctx context.Context, b domain.BranchID, parent uuid.UUID) ([]repository.RepositoryExternalIDReadModel, error) {
				mainOnly(b)
				return rs.GetRepositoryExternalIDs(ctx, parent)
			},
			func(ctx context.Context, b domain.BranchID) (map[uuid.UUID][]repository.RepositoryExternalIDReadModel, error) {
				mainOnly(b)
				return rs.ListRepositoryExternalIDs(ctx)
			},
			func(v string) repository.RepositoryExternalIDReadModel {
				return repository.RepositoryExternalIDReadModel{Value: v, Type: "t-" + v}
			},
			func(x repository.RepositoryExternalIDReadModel) (uuid.UUID, int, string, string) {
				return x.RepositoryID, x.Sequence, x.Value, x.Type
			}),
	}
}

func TestListExternalIDs_MatchesPerParentReads(t *testing.T) {
	for _, backend := range backends {
		t.Run(backend.name, func(t *testing.T) {
			ctx := context.Background()
			rs := backend.setup(t).read
			main := domain.MainBranchID
			branch := domain.BranchID(uuid.New())

			for _, k := range exidKinds(t, rs) {
				t.Run(k.name, func(t *testing.T) {
					// check asserts List(b) equals want and agrees with Get(b) for every parent.
					check := func(b domain.BranchID, want map[uuid.UUID][]string, parents ...uuid.UUID) {
						t.Helper()
						got, err := k.list(ctx, b)
						if err != nil {
							t.Fatalf("list on %s: %v", b, err)
						}
						if !maps.EqualFunc(got, want, slices.Equal) {
							t.Errorf("list on %s = %v, want %v", b, got, want)
						}
						for _, p := range parents {
							single, err := k.get(ctx, b, p)
							if err != nil {
								t.Fatalf("get on %s: %v", b, err)
							}
							if !slices.Equal(single, got[p]) {
								t.Errorf("get(%s) on %s = %v, list says %v", p, b, single, got[p])
							}
						}
					}

					check(main, map[uuid.UUID][]string{})

					a, bp, empty := uuid.New(), uuid.New(), uuid.New()
					if err := k.replace(ctx, main, a, []string{"A2", "A1"}); err != nil {
						t.Fatalf("replace: %v", err)
					}
					if err := k.replace(ctx, main, bp, []string{"B1"}); err != nil {
						t.Fatalf("replace: %v", err)
					}
					if err := k.replace(ctx, main, empty, nil); err != nil {
						t.Fatalf("replace: %v", err)
					}
					onMain := map[uuid.UUID][]string{
						a:  {"0:A2:t-A2", "1:A1:t-A1"},
						bp: {"0:B1:t-B1"},
					}
					check(main, onMain, a, bp, empty)

					if !k.branched {
						return
					}
					// A branch overrides a's bucket, empties bp's (a tombstone) and
					// adds one of its own; main is untouched.
					onlyBranch := uuid.New()
					for parent, values := range map[uuid.UUID][]string{a: {"BA"}, bp: nil, onlyBranch: {"BX"}} {
						if err := k.replace(ctx, branch, parent, values); err != nil {
							t.Fatalf("replace on branch: %v", err)
						}
					}
					check(branch, map[uuid.UUID][]string{
						a:          {"0:BA:t-BA"},
						onlyBranch: {"0:BX:t-BX"},
					}, a, bp, empty, onlyBranch)
					check(main, onMain, a, bp, empty, onlyBranch)
				})
			}
		})
	}
}
