package integration_test

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// The Go and SQL forms of the privacy rule (ADR-008 §5) agree on one table of
// cases, on every backend, on the mainline and on a research branch. The Go
// form is repository.Privacy; the memory store applies it, and the SQL stores
// build their conditions from the same fields.

// parityCutoff is the living cutoff the table is evaluated at.
var parityCutoff = time.Date(1916, time.January, 1, 0, 0, 0, 0, time.UTC)

func parityDate(y int, m time.Month, d int) *time.Time {
	t := time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
	return &t
}

// parityCases are people who exercise each arm of the living rule and of the
// private test. Every one is named "<given> Parity", born and buried in
// places named for the table, so each list and search finds all of them
// unless the rule excludes them.
var parityCases = []struct {
	given string
	row   repository.PersonReadModel
}{
	{"NoDates", repository.PersonReadModel{}},
	{"Old", repository.PersonReadModel{LivingRefDate: parityDate(1900, time.June, 1)}},
	{"OnCutoff", repository.PersonReadModel{LivingRefDate: parityDate(1916, time.January, 1)}},
	{"DayBefore", repository.PersonReadModel{LivingRefDate: parityDate(1915, time.December, 31)}},
	{"BCClamped", repository.PersonReadModel{LivingRefDate: &domain.EarliestSortDate}},
	{"Recent", repository.PersonReadModel{LivingRefDate: parityDate(1990, time.March, 3)}},
	{"RecentDied", repository.PersonReadModel{LivingRefDate: parityDate(1990, time.March, 3), DeathEvidence: true}},
	{"UndatedDied", repository.PersonReadModel{DeathEvidence: true}},
	{"OverrideLiving", repository.PersonReadModel{LivingRefDate: parityDate(1800, time.May, 5), DeathEvidence: true, LivingOverride: domain.LivingOverrideLiving}},
	{"OverrideDeceased", repository.PersonReadModel{LivingRefDate: parityDate(1990, time.March, 3), LivingOverride: domain.LivingOverrideDeceased}},
	{"Confidential", repository.PersonReadModel{DeathEvidence: true, Restriction: "confidential"}},
	{"PrivacyUpper", repository.PersonReadModel{DeathEvidence: true, Restriction: "PRIVACY"}},
	{"LockedConfidential", repository.PersonReadModel{DeathEvidence: true, Restriction: "locked, Confidential"}},
	{"SpacedPrivacy", repository.PersonReadModel{DeathEvidence: true, Restriction: "privacy ,locked"}},
	{"Locked", repository.PersonReadModel{DeathEvidence: true, Restriction: "LOCKED"}},
	{"NotAValue", repository.PersonReadModel{DeathEvidence: true, Restriction: "confidentiality"}},
	{"PrivateLiving", repository.PersonReadModel{Restriction: "confidential"}},
}

// parityPrivacies are the principals' views the table is checked under.
var parityPrivacies = map[string]repository.Privacy{
	"nothing hidden": {},
	"private hidden": {HidePrivate: true},
	"living hidden":  {HideLiving: true, LivingCutoff: parityCutoff},
	"both hidden":    {HidePrivate: true, HideLiving: true, LivingCutoff: parityCutoff},
}

const (
	paritySurname  = "Parity"
	parityPlace    = "Paritytown"
	parityCemetery = "Parity Churchyard"
)

// saveParityPeople saves the table on branchID and returns the people saved.
func saveParityPeople(t *testing.T, ctx context.Context, rs repository.ReadModelStore, branchID domain.BranchID) []repository.PersonReadModel {
	t.Helper()
	now := time.Now().UTC().Truncate(time.Second)
	var saved []repository.PersonReadModel
	for _, c := range parityCases {
		p := c.row
		p.ID, p.GivenName, p.Surname = uuid.New(), c.given, paritySurname
		p.FullName = domain.JoinFullName(p.GivenName, p.Surname)
		p.BirthPlace, p.Version, p.UpdatedAt = parityPlace, 1, now
		if err := rs.SavePerson(ctx, branchID, &p); err != nil {
			t.Fatalf("save %s: %v", c.given, err)
		}
		burial := &repository.EventReadModel{ID: uuid.New(), OwnerType: "person", OwnerID: p.ID,
			FactType: domain.FactPersonBurial, Place: parityCemetery, Version: 1, CreatedAt: now}
		if err := rs.SaveEvent(ctx, branchID, burial); err != nil {
			t.Fatalf("save burial of %s: %v", c.given, err)
		}
		saved = append(saved, p)
	}
	return saved
}

// shownNames returns the given names of the people v does not protect: the
// Go form's answer.
func shownNames(v repository.Privacy, people []repository.PersonReadModel) []string {
	var names []string
	for i := range people {
		if !v.Protects(&people[i]) {
			names = append(names, people[i].GivenName)
		}
	}
	slices.Sort(names)
	return names
}

// listedNames returns the given names of the people of a page who are among
// people, and checks that the total counts the whole page. On a branch the
// mainline's people are listed too; they are told apart by ID.
func listedNames(t *testing.T, what string, people []repository.PersonReadModel, page []repository.PersonReadModel, total int, err error) []string {
	t.Helper()
	if err != nil {
		t.Fatalf("%s: %v", what, err)
	}
	if total != len(page) {
		t.Errorf("%s: total = %d, listed %d; the total must count only what is listed", what, total, len(page))
	}
	ours := map[uuid.UUID]bool{}
	for _, p := range people {
		ours[p.ID] = true
	}
	var names []string
	for _, p := range page {
		if ours[p.ID] {
			names = append(names, p.GivenName)
		}
	}
	slices.Sort(names)
	return names
}

func TestPrivacy_GoAndSQLFormsAgree(t *testing.T) {
	for _, backend := range backends {
		t.Run(backend.name, func(t *testing.T) {
			ctx := context.Background()
			rs := backend.setup(t).read
			branch := domain.BranchID(uuid.New())
			for scope, branchID := range map[string]domain.BranchID{"main": domain.MainBranchID, "branch": branch} {
				people := saveParityPeople(t, ctx, rs, branchID)
				for name, v := range parityPrivacies {
					want := shownNames(v, people)
					list := repository.ListOptions{Limit: 100, BranchID: branchID, Privacy: v}
					got := map[string][]string{}
					page, total, err := rs.ListPersons(ctx, list)
					got["ListPersons"] = listedNames(t, "ListPersons", people, page, total, err)
					page, total, err = rs.GetPersonsBySurname(ctx, paritySurname, list)
					got["GetPersonsBySurname"] = listedNames(t, "GetPersonsBySurname", people, page, total, err)
					page, total, err = rs.GetPersonsByPlace(ctx, parityPlace, list)
					got["GetPersonsByPlace"] = listedNames(t, "GetPersonsByPlace", people, page, total, err)
					page, total, err = rs.GetPersonsByCemetery(ctx, parityCemetery, list)
					got["GetPersonsByCemetery"] = listedNames(t, "GetPersonsByCemetery", people, page, total, err)
					for mode, search := range map[string]repository.SearchOptions{
						"SearchPersons":         {Query: paritySurname},
						"SearchPersons fuzzy":   {Query: paritySurname, Fuzzy: true},
						"SearchPersons soundex": {Query: paritySurname, Soundex: true},
						"SearchPersons place":   {BirthPlace: parityPlace},
					} {
						search.Limit, search.BranchID, search.Privacy = 100, branchID, v
						page, total, err = rs.SearchPersons(ctx, search)
						got[mode] = listedNames(t, mode, people, page, total, err)
					}
					for method, names := range got {
						if !slices.Equal(names, want) {
							t.Errorf("%s, %s, %s: got %v, want %v", scope, name, method, names, want)
						}
					}
				}
			}
		})
	}
}

// A branch that marks a mainline person private hides them on that branch
// only, and one that unmarks a private person shows them there: the rule
// tests the row the branch resolves to, never a row it shadows.
func TestPrivacy_ListsTestTheBranchRow(t *testing.T) {
	for _, backend := range backends {
		t.Run(backend.name, func(t *testing.T) {
			ctx := context.Background()
			rs := backend.setup(t).read
			branch := domain.BranchID(uuid.New())
			now := time.Now().UTC().Truncate(time.Second)
			public := repository.PersonReadModel{ID: uuid.New(), GivenName: "MarkedOnBranch", Surname: paritySurname, DeathEvidence: true, Version: 1, UpdatedAt: now}
			private := repository.PersonReadModel{ID: uuid.New(), GivenName: "UnmarkedOnBranch", Surname: paritySurname, DeathEvidence: true, Restriction: "confidential", Version: 1, UpdatedAt: now}
			for _, p := range []repository.PersonReadModel{public, private} {
				if err := rs.SavePerson(ctx, domain.MainBranchID, &p); err != nil {
					t.Fatalf("save on main: %v", err)
				}
			}
			public.Restriction, private.Restriction = "confidential", ""
			for _, p := range []repository.PersonReadModel{public, private} {
				if err := rs.SavePerson(ctx, branch, &p); err != nil {
					t.Fatalf("save on branch: %v", err)
				}
			}
			v := repository.Privacy{HidePrivate: true}
			for scope, want := range map[domain.BranchID]string{domain.MainBranchID: "MarkedOnBranch", branch: "UnmarkedOnBranch"} {
				people := []repository.PersonReadModel{public, private}
				page, total, err := rs.ListPersons(ctx, repository.ListOptions{Limit: 10, BranchID: scope, Privacy: v})
				if got := listedNames(t, "ListPersons", people, page, total, err); !slices.Equal(got, []string{want}) {
					t.Errorf("ListPersons on %s = %v, want [%s]", scope, got, want)
				}
				found, total, err := rs.SearchPersons(ctx, repository.SearchOptions{Query: paritySurname, Limit: 10, BranchID: scope, Privacy: v})
				if got := listedNames(t, "SearchPersons", people, found, total, err); !slices.Equal(got, []string{want}) {
					t.Errorf("SearchPersons on %s = %v, want [%s]", scope, got, want)
				}
			}
		})
	}
}

// ListFamilies and ListMediaForEntity exclude private records in the query,
// so their totals count only what they list.
func TestPrivacy_ListsExcludePrivateFamiliesAndMedia(t *testing.T) {
	for _, backend := range backends {
		t.Run(backend.name, func(t *testing.T) {
			ctx := context.Background()
			rs := backend.setup(t).read
			now := time.Now().UTC().Truncate(time.Second)
			owner := uuid.New()
			for _, branchID := range []domain.BranchID{domain.MainBranchID, domain.BranchID(uuid.New())} {
				var families []uuid.UUID
				for _, r := range []domain.Restriction{"", "locked", "confidential", "PRIVACY"} {
					f := &repository.FamilyReadModel{ID: uuid.New(), RelationshipType: domain.RelationMarriage, Restriction: r, Version: 1, UpdatedAt: now}
					m := &repository.MediaReadModel{ID: uuid.New(), EntityType: "person", EntityID: owner, Title: string(r), MimeType: "image/png",
						MediaType: domain.MediaPhoto, Filename: "m.png", FileSize: 1, FileData: []byte{1}, Restriction: r, Version: 1, CreatedAt: now, UpdatedAt: now}
					if err := rs.SaveFamily(ctx, branchID, f); err != nil {
						t.Fatalf("save family: %v", err)
					}
					if err := rs.SaveMedia(ctx, branchID, m); err != nil {
						t.Fatalf("save media: %v", err)
					}
					families = append(families, f.ID)
				}
				for _, v := range []repository.Privacy{{}, {HidePrivate: true}} {
					want := 4
					if v.HidePrivate {
						want = 2
					}
					list := repository.ListOptions{Limit: 100, BranchID: branchID, Privacy: v}
					page, total, err := rs.ListFamilies(ctx, list)
					if err != nil {
						t.Fatalf("ListFamilies: %v", err)
					}
					listed := 0
					for _, f := range page {
						if slices.Contains(families, f.ID) {
							listed++
						}
					}
					if listed != want || total != len(page) {
						t.Errorf("ListFamilies on %s with %+v: %d of ours, total %d of %d; want %d of ours and total = listed",
							branchID, v, listed, total, len(page), want)
					}
					media, total, err := rs.ListMediaForEntity(ctx, "person", owner, list)
					if err != nil {
						t.Fatalf("ListMediaForEntity: %v", err)
					}
					// The branch sees the mainline's media too.
					if want := want * branchFactor(branchID); len(media) != want || total != want {
						t.Errorf("ListMediaForEntity on %s with %+v: %d, total %d; want %d", branchID, v, len(media), total, want)
					}
				}
			}
		})
	}
}

// branchFactor is how many scopes' media a scope sees in
// TestPrivacy_ListsExcludePrivateFamiliesAndMedia: the mainline its own, a
// branch its own and the mainline's.
func branchFactor(branchID domain.BranchID) int {
	if branchID.IsMain() {
		return 1
	}
	return 2
}
