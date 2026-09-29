package query_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/query"
	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/repository/memory"
)

// healthFixture is a read model with a mainline and one branch overlay, and
// the services that check them.
type healthFixture struct {
	ctx        context.Context
	store      *memory.ReadModelStore
	branch     domain.BranchID
	health     *query.BranchHealthService
	validation *query.ValidationService
	quality    *query.QualityService
}

func newHealthFixture() *healthFixture {
	store := memory.NewReadModelStore()
	validation := query.NewValidationService(store)
	quality := query.NewQualityService(store)
	return &healthFixture{
		ctx:        context.Background(),
		store:      store,
		branch:     domain.BranchID(uuid.New()),
		health:     query.NewBranchHealthService(validation, quality),
		validation: validation,
		quality:    quality,
	}
}

func (f *healthFixture) savePerson(t *testing.T, scope domain.BranchID, id uuid.UUID, given, birth, death string) {
	t.Helper()
	require.NoError(t, f.store.SavePerson(f.ctx, scope, &repository.PersonReadModel{
		ID: id, GivenName: given, Surname: "Sample", FullName: given + " Sample", Gender: domain.GenderFemale,
		BirthDateRaw: birth, BirthPlace: "Springfield", DeathDateRaw: death, Version: 1, UpdatedAt: time.Now(),
	}))
}

func TestBranchHealth_ReportsOnlyWhatTheBranchIntroduces(t *testing.T) {
	f := newHealthFixture()
	ada, bob, zed1, zed2, twin := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()

	// Mainline: Ada, Bob with no dates (standing issues), and a standing
	// duplicate pair of Zeds.
	f.savePerson(t, domain.MainBranchID, ada, "Ada", "1 JAN 1850", "")
	require.NoError(t, f.store.SavePerson(f.ctx, domain.MainBranchID, &repository.PersonReadModel{
		ID: bob, GivenName: "Bob", Surname: "Sample", FullName: "Bob Sample", Version: 1,
	}))
	f.savePerson(t, domain.MainBranchID, zed1, "Zed", "1 JAN 1900", "")
	f.savePerson(t, domain.MainBranchID, zed2, "Zed", "1 JAN 1900", "")

	// Branch: Ada lives to 140 (an impossible age), a twin of Ada appears (a
	// duplicate), and one Zed is deleted (resolving the Zed pair).
	f.savePerson(t, f.branch, ada, "Ada", "1 JAN 1850", "1 JAN 1990")
	f.savePerson(t, f.branch, twin, "Ada", "1 JAN 1850", "")
	require.NoError(t, f.store.DeletePerson(f.ctx, f.branch, zed2))

	health, err := f.health.BranchHealth(f.ctx, f.branch)
	require.NoError(t, err)

	var codes []string
	for _, issue := range health.ValidationIssues {
		codes = append(codes, issue.Code)
		require.NotNil(t, issue.RecordID, "issue %s has a record", issue.Code)
		assert.NotEqual(t, bob, *issue.RecordID, "Bob's issues are the mainline's")
		assert.NotEmpty(t, issue.Key)
	}
	assert.Contains(t, codes, "IMPOSSIBLE_AGE")
	assert.NotContains(t, codes, "POTENTIAL_DUPLICATE", "duplicates are reported as pairs")
	for _, issue := range health.ValidationIssues {
		if issue.Code == "IMPOSSIBLE_AGE" {
			assert.Equal(t, ada, *issue.RecordID)
			assert.Equal(t, "person", issue.RecordType)
			assert.Equal(t, "Ada Sample", issue.RecordName)
			assert.Equal(t, "warning", issue.Severity)
		}
	}
	assert.Equal(t, len(health.ValidationIssues), health.ErrorCount+health.WarningCount+health.InfoCount)
	assert.GreaterOrEqual(t, health.WarningCount, 1)

	require.Len(t, health.Duplicates, 1, "only the twin pair is new")
	pair := health.Duplicates[0]
	assert.ElementsMatch(t, []uuid.UUID{ada, twin}, []uuid.UUID{pair.Person1ID, pair.Person2ID})
	assert.Equal(t, "Ada Sample", pair.Person1Name)
	assert.Greater(t, pair.Confidence, 0.0)
	assert.NotEmpty(t, pair.MatchReasons)

	var twinOrphan bool
	for _, issue := range health.QualityIssues {
		assert.NotEqual(t, bob, issue.PersonID)
		if issue.PersonID == twin && issue.Issue == "No family connections" {
			twinOrphan = true
			assert.Equal(t, "Ada Sample", issue.PersonName)
		}
	}
	assert.True(t, twinOrphan, "quality issues = %+v", health.QualityIssues)
	assert.GreaterOrEqual(t, health.ResolvedCount, 1, "the Zed pair is resolved on the branch")

	// Keys are stable: the same check twice gives the same keys.
	again, err := f.health.BranchHealth(f.ctx, f.branch)
	require.NoError(t, err)
	assert.Equal(t, health, again)
}

func TestBranchHealth_UntouchedBranchIntroducesNothing(t *testing.T) {
	f := newHealthFixture()
	f.savePerson(t, domain.MainBranchID, uuid.New(), "Ada", "", "")
	health, err := f.health.BranchHealth(f.ctx, f.branch)
	require.NoError(t, err)
	assert.Empty(t, health.ValidationIssues)
	assert.Empty(t, health.QualityIssues)
	assert.Empty(t, health.Duplicates)
	assert.Zero(t, health.ResolvedCount)
}

func TestBranchHealth_RefusesTheMainline(t *testing.T) {
	f := newHealthFixture()
	_, err := f.health.BranchHealth(f.ctx, domain.MainBranchID)
	assert.Error(t, err)
}

// The quality checks read the branch overlay, family links included, through
// set-based reads (ADR-005, #838).
func TestQualityAndValidation_ReadTheBranchOverlay(t *testing.T) {
	f := newHealthFixture()
	parent, child, family := uuid.New(), uuid.New(), uuid.New()
	f.savePerson(t, domain.MainBranchID, parent, "Pat", "1 JAN 1850", "")
	f.savePerson(t, domain.MainBranchID, child, "Cal", "1 JAN 1880", "")
	require.NoError(t, f.store.SaveFamily(f.ctx, domain.MainBranchID, &repository.FamilyReadModel{
		ID: family, Partner1ID: &parent, Partner1GivenName: "Pat", Partner1Surname: "Sample", Version: 1,
	}))
	// Only the branch links the child.
	require.NoError(t, f.store.SaveFamilyChild(f.ctx, f.branch, &repository.FamilyChildReadModel{FamilyID: family, PersonID: child}))
	// And only the branch knows Dee.
	f.savePerson(t, f.branch, uuid.New(), "Dee", "1 JAN 1890", "")

	mainQuality, err := f.quality.GetPersonQuality(f.ctx, child)
	require.NoError(t, err)
	assert.Contains(t, mainQuality.Issues, "No family connections")
	branchQuality, err := f.quality.GetPersonQualityOn(f.ctx, f.branch, child)
	require.NoError(t, err)
	assert.NotContains(t, branchQuality.Issues, "No family connections")

	mainOverview, err := f.quality.GetQualityOverview(f.ctx)
	require.NoError(t, err)
	branchOverview, err := f.quality.GetQualityOverviewOn(f.ctx, f.branch)
	require.NoError(t, err)
	assert.Equal(t, 2, mainOverview.TotalPersons)
	assert.Equal(t, 3, branchOverview.TotalPersons)

	branchStats, err := f.quality.GetStatisticsOn(f.ctx, f.branch)
	require.NoError(t, err)
	assert.Equal(t, 3, branchStats.TotalPersons)
	assert.Equal(t, 1, branchStats.TotalFamilies)

	mainFeed, err := f.quality.GetDiscoveryFeed(f.ctx, 50)
	require.NoError(t, err)
	branchFeed, err := f.quality.GetDiscoveryFeedOn(f.ctx, f.branch, 50)
	require.NoError(t, err)
	orphans := func(feed *query.DiscoveryFeed) int {
		n := 0
		for _, item := range feed.Items {
			if item.Type == "orphan" {
				n++
			}
		}
		return n
	}
	assert.Equal(t, 1, orphans(mainFeed), "Cal is unlinked on the mainline")
	assert.Equal(t, 1, orphans(branchFeed), "Dee is unlinked on the branch; Cal is linked there")

	mainReport, err := f.validation.GetQualityReport(f.ctx)
	require.NoError(t, err)
	branchReport, err := f.validation.GetQualityReportOn(f.ctx, f.branch)
	require.NoError(t, err)
	assert.Equal(t, 2, mainReport.TotalIndividuals)
	assert.Equal(t, 3, branchReport.TotalIndividuals)

	page, err := f.validation.GetValidationIssuesOn(f.ctx, f.branch, "", 0, 0)
	require.NoError(t, err)
	assert.Equal(t, page.ErrorCount+page.WarningCount+page.InfoCount, page.Total)

	_, total, err := f.validation.FindDuplicatesOn(f.ctx, f.branch, 10, 0)
	require.NoError(t, err)
	assert.Zero(t, total)
}

// A branch introducing errors and warnings of several codes, several quality
// issues and two duplicate pairs gets them back in review order, with the
// validation issues counted by severity.
func TestBranchHealth_OrdersAndCountsWhatTheBranchIntroduces(t *testing.T) {
	f := newHealthFixture()
	f.savePerson(t, domain.MainBranchID, uuid.New(), "Main", "1 JAN 1800", "")

	// Errors: two deaths before births. Warnings: two impossible ages.
	f.savePerson(t, f.branch, uuid.New(), "Wes", "1 JAN 1900", "1 JAN 1890")
	f.savePerson(t, f.branch, uuid.New(), "Ann", "1 JAN 1910", "1 JAN 1905")
	f.savePerson(t, f.branch, uuid.New(), "Old", "1 JAN 1700", "1 JAN 1900")
	f.savePerson(t, f.branch, uuid.New(), "Eld", "1 JAN 1650", "1 JAN 1850")
	// Two duplicate pairs: the Kits and the Lous.
	kit1, kit2, lou1, lou2 := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	f.savePerson(t, f.branch, kit1, "Kit", "1 JAN 1880", "")
	f.savePerson(t, f.branch, kit2, "Kit", "1 JAN 1880", "")
	f.savePerson(t, f.branch, lou1, "Lou", "1 JAN 1881", "")
	f.savePerson(t, f.branch, lou2, "Lou", "1 JAN 1881", "")

	health, err := f.health.BranchHealth(f.ctx, f.branch)
	require.NoError(t, err)

	var severities, codes []string
	for _, issue := range health.ValidationIssues {
		severities = append(severities, issue.Severity)
		codes = append(codes, issue.Code)
	}
	assert.Equal(t, []string{"error", "error", "warning", "warning"}, severities, "codes = %v", codes)
	assert.Equal(t, []string{"DEATH_BEFORE_BIRTH", "DEATH_BEFORE_BIRTH", "IMPOSSIBLE_AGE", "IMPOSSIBLE_AGE"}, codes)
	assert.Equal(t, "Ann Sample", health.ValidationIssues[0].RecordName, "same code: by record name")
	assert.Equal(t, "Wes Sample", health.ValidationIssues[1].RecordName)
	assert.Equal(t, "Eld Sample", health.ValidationIssues[2].RecordName)
	assert.Equal(t, "Old Sample", health.ValidationIssues[3].RecordName)
	assert.Equal(t, 2, health.ErrorCount)
	assert.Equal(t, 2, health.WarningCount)
	assert.Zero(t, health.InfoCount)

	// Every branch person is unlinked and lacks a death detail: two quality
	// issues each, by name, then key.
	require.Len(t, health.QualityIssues, 16)
	for i := 1; i < len(health.QualityIssues); i++ {
		a, b := health.QualityIssues[i-1], health.QualityIssues[i]
		assert.True(t, a.PersonName < b.PersonName || (a.PersonName == b.PersonName && a.Key < b.Key),
			"quality issues out of order: %q/%q before %q/%q", a.PersonName, a.Key, b.PersonName, b.Key)
	}

	require.Len(t, health.Duplicates, 2)
	pairs := [][]uuid.UUID{}
	for _, d := range health.Duplicates {
		pairs = append(pairs, []uuid.UUID{d.Person1ID, d.Person2ID})
	}
	assert.ElementsMatch(t, []uuid.UUID{kit1, kit2}, pairs[slicesIndexOfPair(pairs, kit1)])
	assert.ElementsMatch(t, []uuid.UUID{lou1, lou2}, pairs[slicesIndexOfPair(pairs, lou1)])
	a, b := health.Duplicates[0], health.Duplicates[1]
	assert.True(t, a.Confidence > b.Confidence || (a.Confidence == b.Confidence && a.Key < b.Key),
		"duplicates out of order: %v/%s before %v/%s", a.Confidence, a.Key, b.Confidence, b.Key)
}

func slicesIndexOfPair(pairs [][]uuid.UUID, id uuid.UUID) int {
	for i, pair := range pairs {
		if pair[0] == id || pair[1] == id {
			return i
		}
	}
	return -1
}

// countingStore counts the whole-tree list reads a check makes.
type countingStore struct {
	repository.ReadModelStore
	persons, families, sources, links int
}

func (c *countingStore) ListPersons(ctx context.Context, opts repository.ListOptions) ([]repository.PersonReadModel, int, error) {
	c.persons++
	return c.ReadModelStore.ListPersons(ctx, opts)
}

func (c *countingStore) ListFamilies(ctx context.Context, opts repository.ListOptions) ([]repository.FamilyReadModel, int, error) {
	c.families++
	return c.ReadModelStore.ListFamilies(ctx, opts)
}

func (c *countingStore) ListSources(ctx context.Context, opts repository.ListOptions) ([]repository.SourceReadModel, int, error) {
	c.sources++
	return c.ReadModelStore.ListSources(ctx, opts)
}

func (c *countingStore) ListAllFamilyChildren(ctx context.Context, branchID domain.BranchID) ([]repository.FamilyChildReadModel, error) {
	c.links++
	return c.ReadModelStore.ListAllFamilyChildren(ctx, branchID)
}

// The validation view and the quality checks share one read of each list per
// scope: a health check of a small tree reads each list once for the mainline
// and once for the branch.
func TestBranchHealth_ReadsEachListOncePerScope(t *testing.T) {
	f := newHealthFixture()
	parent, child, family := uuid.New(), uuid.New(), uuid.New()
	f.savePerson(t, domain.MainBranchID, parent, "Pat", "1 JAN 1850", "")
	f.savePerson(t, f.branch, child, "Cal", "1 JAN 1880", "")
	require.NoError(t, f.store.SaveFamily(f.ctx, domain.MainBranchID, &repository.FamilyReadModel{
		ID: family, Partner1ID: &parent, Version: 1,
	}))
	require.NoError(t, f.store.SaveFamilyChild(f.ctx, f.branch, &repository.FamilyChildReadModel{FamilyID: family, PersonID: child}))

	store := &countingStore{ReadModelStore: f.store}
	health := query.NewBranchHealthService(query.NewValidationService(store), query.NewQualityService(store))
	result, err := health.BranchHealth(f.ctx, f.branch)
	require.NoError(t, err)
	for _, issue := range result.QualityIssues {
		assert.NotEqual(t, "No family connections", issue.Issue, "Cal is linked on the branch, so is not an orphan")
	}

	assert.Equal(t, 2, store.persons, "persons")
	assert.Equal(t, 2, store.families, "families")
	assert.Equal(t, 2, store.sources, "sources")
	assert.Equal(t, 2, store.links, "child links")
}
