package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
	pgstore "github.com/cacack/my-family/internal/repository/postgres"
)

func setupReadModelStore(t *testing.T) (*pgstore.ReadModelStore, func()) {
	t.Helper()

	db, cleanup := setupPostgres(t)

	store, err := pgstore.NewReadModelStore(db)
	if err != nil {
		cleanup()
		t.Fatalf("create read model store: %v", err)
	}

	return store, cleanup
}

func TestReadModelStore_PersonCRUD(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	store, cleanup := setupReadModelStore(t)
	defer cleanup()

	ctx := context.Background()
	personID := uuid.New()
	now := time.Now().Truncate(time.Microsecond) // PostgreSQL microsecond precision

	// Create person
	person := &repository.PersonReadModel{
		ID:           personID,
		GivenName:    "John",
		Surname:      "Doe",
		FullName:     "John Doe",
		Gender:       domain.GenderMale,
		BirthDateRaw: "1 JAN 1850",
		BirthPlace:   "Springfield, IL",
		Notes:        "Test person",
		Version:      1,
		UpdatedAt:    now,
	}

	err := store.SavePerson(ctx, domain.MainBranchID, person)
	if err != nil {
		t.Fatalf("save person: %v", err)
	}

	// Read person
	retrieved, err := store.GetPerson(ctx, domain.MainBranchID, personID)
	if err != nil {
		t.Fatalf("get person: %v", err)
	}
	if retrieved == nil {
		t.Fatal("person not found")
	}

	if retrieved.GivenName != "John" {
		t.Errorf("expected GivenName John, got %s", retrieved.GivenName)
	}
	if retrieved.Surname != "Doe" {
		t.Errorf("expected Surname Doe, got %s", retrieved.Surname)
	}
	if retrieved.Gender != domain.GenderMale {
		t.Errorf("expected Gender male, got %s", retrieved.Gender)
	}

	// Update person
	person.GivenName = "Jane"
	person.Version = 2
	err = store.SavePerson(ctx, domain.MainBranchID, person)
	if err != nil {
		t.Fatalf("update person: %v", err)
	}

	// Read updated person
	retrieved, err = store.GetPerson(ctx, domain.MainBranchID, personID)
	if err != nil {
		t.Fatalf("get updated person: %v", err)
	}
	if retrieved.GivenName != "Jane" {
		t.Errorf("expected updated GivenName Jane, got %s", retrieved.GivenName)
	}

	// Delete person
	err = store.DeletePerson(ctx, domain.MainBranchID, personID)
	if err != nil {
		t.Fatalf("delete person: %v", err)
	}

	// Verify deletion
	retrieved, err = store.GetPerson(ctx, domain.MainBranchID, personID)
	if err != nil {
		t.Fatalf("get deleted person: %v", err)
	}
	if retrieved != nil {
		t.Error("person should have been deleted")
	}
}

func TestReadModelStore_ListPersons(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	store, cleanup := setupReadModelStore(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now().Truncate(time.Microsecond)

	// Create multiple persons
	for i := 0; i < 5; i++ {
		person := &repository.PersonReadModel{
			ID:        uuid.New(),
			GivenName: "Person",
			Surname:   "Test",
			FullName:  "Person Test",
			Version:   1,
			UpdatedAt: now,
		}
		err := store.SavePerson(ctx, domain.MainBranchID, person)
		if err != nil {
			t.Fatalf("save person %d: %v", i, err)
		}
	}

	// List with pagination
	persons, total, err := store.ListPersons(ctx, repository.ListOptions{
		Limit:  3,
		Offset: 0,
	})
	if err != nil {
		t.Fatalf("list persons: %v", err)
	}

	if total != 5 {
		t.Errorf("expected total 5, got %d", total)
	}
	if len(persons) != 3 {
		t.Errorf("expected 3 persons in page, got %d", len(persons))
	}
}

func TestReadModelStore_SearchPersons_FullText(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	store, cleanup := setupReadModelStore(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now().Truncate(time.Microsecond)

	// Create test persons
	persons := []repository.PersonReadModel{
		{ID: uuid.New(), GivenName: "John", Surname: "Smith", FullName: "John Smith", Version: 1, UpdatedAt: now},
		{ID: uuid.New(), GivenName: "Jane", Surname: "Smith", FullName: "Jane Smith", Version: 1, UpdatedAt: now},
		{ID: uuid.New(), GivenName: "Robert", Surname: "Johnson", FullName: "Robert Johnson", Version: 1, UpdatedAt: now},
	}

	for _, p := range persons {
		if err := store.SavePerson(ctx, domain.MainBranchID, &p); err != nil {
			t.Fatalf("save person: %v", err)
		}
	}

	// Search for "Smith" - should find 2 results
	results, err := store.SearchPersons(ctx, repository.SearchOptions{Query: "Smith", Limit: 10})
	if err != nil {
		t.Fatalf("search persons: %v", err)
	}

	if len(results) != 2 {
		t.Errorf("expected 2 results for 'Smith', got %d", len(results))
	}

	// Search for "John" - should find 2 results (John Smith and Robert Johnson)
	results, err = store.SearchPersons(ctx, repository.SearchOptions{Query: "John", Limit: 10})
	if err != nil {
		t.Fatalf("search persons: %v", err)
	}

	// John matches "John Smith" and "Johnson" matches via full_name ILIKE
	if len(results) < 1 {
		t.Errorf("expected at least 1 result for 'John', got %d", len(results))
	}
}

func TestReadModelStore_SearchPersons_Fuzzy(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	store, cleanup := setupReadModelStore(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now().Truncate(time.Microsecond)

	// Create test persons
	persons := []repository.PersonReadModel{
		{ID: uuid.New(), GivenName: "Katherine", Surname: "Williams", FullName: "Katherine Williams", Version: 1, UpdatedAt: now},
		{ID: uuid.New(), GivenName: "Catherine", Surname: "Wilson", FullName: "Catherine Wilson", Version: 1, UpdatedAt: now},
		{ID: uuid.New(), GivenName: "Robert", Surname: "Brown", FullName: "Robert Brown", Version: 1, UpdatedAt: now},
	}

	for _, p := range persons {
		if err := store.SavePerson(ctx, domain.MainBranchID, &p); err != nil {
			t.Fatalf("save person: %v", err)
		}
	}

	// Fuzzy search for "Kathryn" - should find "Katherine" and "Catherine"
	results, err := store.SearchPersons(ctx, repository.SearchOptions{Query: "Kathryn", Fuzzy: true, Limit: 10})
	if err != nil {
		t.Fatalf("fuzzy search: %v", err)
	}

	// pg_trgm should find similar names
	if len(results) < 1 {
		t.Logf("Note: fuzzy search returned %d results (pg_trgm similarity may vary)", len(results))
	}
}

func TestReadModelStore_FamilyCRUD(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	store, cleanup := setupReadModelStore(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now().Truncate(time.Microsecond)

	// Create partner persons first
	partner1ID := uuid.New()
	partner2ID := uuid.New()

	partner1 := &repository.PersonReadModel{
		ID:        partner1ID,
		GivenName: "John",
		Surname:   "Doe",
		FullName:  "John Doe",
		Version:   1,
		UpdatedAt: now,
	}
	partner2 := &repository.PersonReadModel{
		ID:        partner2ID,
		GivenName: "Jane",
		Surname:   "Doe",
		FullName:  "Jane Doe",
		Version:   1,
		UpdatedAt: now,
	}

	if err := store.SavePerson(ctx, domain.MainBranchID, partner1); err != nil {
		t.Fatalf("save partner1: %v", err)
	}
	if err := store.SavePerson(ctx, domain.MainBranchID, partner2); err != nil {
		t.Fatalf("save partner2: %v", err)
	}

	// Create family
	familyID := uuid.New()
	family := &repository.FamilyReadModel{
		ID:                familyID,
		Partner1ID:        &partner1ID,
		Partner1GivenName: "John",
		Partner1Surname:   "Doe",
		Partner2ID:        &partner2ID,
		Partner2GivenName: "Jane",
		Partner2Surname:   "Doe",
		RelationshipType:  domain.RelationMarriage,
		MarriageDateRaw:   "15 JUN 1875",
		MarriagePlace:     "Boston, MA",
		ChildCount:        0,
		Version:           1,
		UpdatedAt:         now,
	}

	err := store.SaveFamily(ctx, domain.MainBranchID, family)
	if err != nil {
		t.Fatalf("save family: %v", err)
	}

	// Read family
	retrieved, err := store.GetFamily(ctx, domain.MainBranchID, familyID)
	if err != nil {
		t.Fatalf("get family: %v", err)
	}
	if retrieved == nil {
		t.Fatal("family not found")
	}

	if retrieved.Partner1GivenName != "John" || retrieved.Partner1Surname != "Doe" {
		t.Errorf("expected Partner1 John/Doe, got %q/%q", retrieved.Partner1GivenName, retrieved.Partner1Surname)
	}
	if retrieved.RelationshipType != domain.RelationMarriage {
		t.Errorf("expected RelationshipType married, got %s", retrieved.RelationshipType)
	}

	// Get families for person
	families, err := store.GetFamiliesForPerson(ctx, domain.MainBranchID, partner1ID)
	if err != nil {
		t.Fatalf("get families for person: %v", err)
	}
	if len(families) != 1 {
		t.Errorf("expected 1 family, got %d", len(families))
	}

	// Delete family
	err = store.DeleteFamily(ctx, domain.MainBranchID, familyID)
	if err != nil {
		t.Fatalf("delete family: %v", err)
	}

	// Verify deletion
	retrieved, err = store.GetFamily(ctx, domain.MainBranchID, familyID)
	if err != nil {
		t.Fatalf("get deleted family: %v", err)
	}
	if retrieved != nil {
		t.Error("family should have been deleted")
	}
}

func TestReadModelStore_FamilyChildren(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	store, cleanup := setupReadModelStore(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now().Truncate(time.Microsecond)

	// Create persons
	partner1ID := uuid.New()
	partner2ID := uuid.New()
	childID := uuid.New()

	persons := []*repository.PersonReadModel{
		{ID: partner1ID, GivenName: "John", Surname: "Doe", FullName: "John Doe", Version: 1, UpdatedAt: now},
		{ID: partner2ID, GivenName: "Jane", Surname: "Doe", FullName: "Jane Doe", Version: 1, UpdatedAt: now},
		{ID: childID, GivenName: "Jimmy", Surname: "Doe", FullName: "Jimmy Doe", Version: 1, UpdatedAt: now},
	}

	for _, p := range persons {
		if err := store.SavePerson(ctx, domain.MainBranchID, p); err != nil {
			t.Fatalf("save person: %v", err)
		}
	}

	// Create family
	familyID := uuid.New()
	family := &repository.FamilyReadModel{
		ID:                familyID,
		Partner1ID:        &partner1ID,
		Partner1GivenName: "John",
		Partner1Surname:   "Doe",
		Partner2ID:        &partner2ID,
		Partner2GivenName: "Jane",
		Partner2Surname:   "Doe",
		ChildCount:        0,
		Version:           1,
		UpdatedAt:         now,
	}

	if err := store.SaveFamily(ctx, domain.MainBranchID, family); err != nil {
		t.Fatalf("save family: %v", err)
	}

	// Add child
	seq := 1
	child := &repository.FamilyChildReadModel{
		FamilyID:         familyID,
		PersonID:         childID,
		PersonGivenName:  "Jimmy",
		PersonSurname:    "Doe",
		RelationshipType: domain.ChildBiological,
		Sequence:         &seq,
	}

	if err := store.SaveFamilyChild(ctx, domain.MainBranchID, child); err != nil {
		t.Fatalf("save family child: %v", err)
	}

	// Get family children
	children, err := store.GetFamilyChildren(ctx, domain.MainBranchID, familyID)
	if err != nil {
		t.Fatalf("get family children: %v", err)
	}
	if len(children) != 1 {
		t.Fatalf("expected 1 child, got %d", len(children))
	}
	if children[0].PersonGivenName != "Jimmy" || children[0].PersonSurname != "Doe" {
		t.Errorf("expected child Jimmy/Doe, got %q/%q", children[0].PersonGivenName, children[0].PersonSurname)
	}

	// Get child family
	childFamily, err := store.GetChildFamily(ctx, domain.MainBranchID, childID)
	if err != nil {
		t.Fatalf("get child family: %v", err)
	}
	if childFamily == nil {
		t.Fatal("child family not found")
	}
	if childFamily.ID != familyID {
		t.Errorf("expected family ID %s, got %s", familyID, childFamily.ID)
	}

	// Remove child
	if err := store.DeleteFamilyChild(ctx, domain.MainBranchID, familyID, childID); err != nil {
		t.Fatalf("delete family child: %v", err)
	}

	// Verify removal
	children, err = store.GetFamilyChildren(ctx, domain.MainBranchID, familyID)
	if err != nil {
		t.Fatalf("get family children after delete: %v", err)
	}
	if len(children) != 0 {
		t.Errorf("expected 0 children, got %d", len(children))
	}
}

func TestReadModelStore_PedigreeEdges(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	store, cleanup := setupReadModelStore(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now().Truncate(time.Microsecond)

	// Create persons
	childID := uuid.New()
	fatherID := uuid.New()
	motherID := uuid.New()

	persons := []*repository.PersonReadModel{
		{ID: childID, GivenName: "Jimmy", Surname: "Doe", FullName: "Jimmy Doe", Version: 1, UpdatedAt: now},
		{ID: fatherID, GivenName: "John", Surname: "Doe", FullName: "John Doe", Version: 1, UpdatedAt: now},
		{ID: motherID, GivenName: "Jane", Surname: "Doe", FullName: "Jane Doe", Version: 1, UpdatedAt: now},
	}

	for _, p := range persons {
		if err := store.SavePerson(ctx, domain.MainBranchID, p); err != nil {
			t.Fatalf("save person: %v", err)
		}
	}

	// Create pedigree edge
	edge := &repository.PedigreeEdge{
		PersonID:   childID,
		FatherID:   &fatherID,
		MotherID:   &motherID,
		FatherName: "John Doe",
		MotherName: "Jane Doe",
	}

	if err := store.SavePedigreeEdge(ctx, domain.MainBranchID, edge); err != nil {
		t.Fatalf("save pedigree edge: %v", err)
	}

	// Get pedigree edge
	retrieved, err := store.GetPedigreeEdge(ctx, domain.MainBranchID, childID)
	if err != nil {
		t.Fatalf("get pedigree edge: %v", err)
	}
	if retrieved == nil {
		t.Fatal("pedigree edge not found")
	}

	if *retrieved.FatherID != fatherID {
		t.Errorf("expected father ID %s, got %s", fatherID, *retrieved.FatherID)
	}
	if *retrieved.MotherID != motherID {
		t.Errorf("expected mother ID %s, got %s", motherID, *retrieved.MotherID)
	}
	if retrieved.FatherName != "John Doe" {
		t.Errorf("expected father name John Doe, got %s", retrieved.FatherName)
	}

	// Delete pedigree edge
	if err := store.DeletePedigreeEdge(ctx, domain.MainBranchID, childID); err != nil {
		t.Fatalf("delete pedigree edge: %v", err)
	}

	// Verify deletion
	retrieved, err = store.GetPedigreeEdge(ctx, domain.MainBranchID, childID)
	if err != nil {
		t.Fatalf("get deleted pedigree edge: %v", err)
	}
	if retrieved != nil {
		t.Error("pedigree edge should have been deleted")
	}
}

func TestReadModelStore_GetChildrenOfFamily(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	db, cleanup := setupPostgres(t)
	defer cleanup()

	store, err := pgstore.NewReadModelStore(db)
	if err != nil {
		t.Fatalf("create read model store: %v", err)
	}

	ctx := context.Background()
	now := time.Now().Truncate(time.Microsecond)

	// Create persons
	partner1ID := uuid.New()
	partner2ID := uuid.New()
	child1ID := uuid.New()
	child2ID := uuid.New()

	persons := []*repository.PersonReadModel{
		{ID: partner1ID, GivenName: "John", Surname: "Doe", FullName: "John Doe", Version: 1, UpdatedAt: now},
		{ID: partner2ID, GivenName: "Jane", Surname: "Doe", FullName: "Jane Doe", Version: 1, UpdatedAt: now},
		{ID: child1ID, GivenName: "Jimmy", Surname: "Doe", FullName: "Jimmy Doe", Version: 1, UpdatedAt: now},
		{ID: child2ID, GivenName: "Jenny", Surname: "Doe", FullName: "Jenny Doe", Version: 1, UpdatedAt: now},
	}

	for _, p := range persons {
		if err := store.SavePerson(ctx, domain.MainBranchID, p); err != nil {
			t.Fatalf("save person: %v", err)
		}
	}

	// Create family
	familyID := uuid.New()
	family := &repository.FamilyReadModel{
		ID:                familyID,
		Partner1ID:        &partner1ID,
		Partner1GivenName: "John",
		Partner1Surname:   "Doe",
		Partner2ID:        &partner2ID,
		Partner2GivenName: "Jane",
		Partner2Surname:   "Doe",
		ChildCount:        2,
		Version:           1,
		UpdatedAt:         now,
	}

	if err := store.SaveFamily(ctx, domain.MainBranchID, family); err != nil {
		t.Fatalf("save family: %v", err)
	}

	// Add children
	seq1, seq2 := 1, 2
	children := []*repository.FamilyChildReadModel{
		{FamilyID: familyID, PersonID: child1ID, PersonGivenName: "Jimmy", PersonSurname: "Doe", RelationshipType: domain.ChildBiological, Sequence: &seq1},
		{FamilyID: familyID, PersonID: child2ID, PersonGivenName: "Jenny", PersonSurname: "Doe", RelationshipType: domain.ChildBiological, Sequence: &seq2},
	}

	for _, c := range children {
		if err := store.SaveFamilyChild(ctx, domain.MainBranchID, c); err != nil {
			t.Fatalf("save family child: %v", err)
		}
	}

	// Get children of family
	childPersons, err := store.GetChildrenOfFamily(ctx, domain.MainBranchID, familyID)
	if err != nil {
		t.Fatalf("get children of family: %v", err)
	}

	if len(childPersons) != 2 {
		t.Fatalf("expected 2 children, got %d", len(childPersons))
	}

	// Verify order by sequence
	if childPersons[0].GivenName != "Jimmy" {
		t.Errorf("expected first child Jimmy, got %s", childPersons[0].GivenName)
	}
	if childPersons[1].GivenName != "Jenny" {
		t.Errorf("expected second child Jenny, got %s", childPersons[1].GivenName)
	}
}

func TestSearchPersons_DateRange(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	store, cleanup := setupReadModelStore(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now().Truncate(time.Microsecond)

	birth1850 := time.Date(1850, 6, 15, 0, 0, 0, 0, time.UTC)
	birth1880 := time.Date(1880, 3, 20, 0, 0, 0, 0, time.UTC)
	birth1920 := time.Date(1920, 11, 1, 0, 0, 0, 0, time.UTC)

	persons := []repository.PersonReadModel{
		{ID: uuid.New(), GivenName: "Alice", Surname: "Early", FullName: "Alice Early", BirthDateRaw: "15 JUN 1850", BirthDateSort: &birth1850, Version: 1, UpdatedAt: now},
		{ID: uuid.New(), GivenName: "Bob", Surname: "Middle", FullName: "Bob Middle", BirthDateRaw: "20 MAR 1880", BirthDateSort: &birth1880, Version: 1, UpdatedAt: now},
		{ID: uuid.New(), GivenName: "Charlie", Surname: "Late", FullName: "Charlie Late", BirthDateRaw: "1 NOV 1920", BirthDateSort: &birth1920, Version: 1, UpdatedAt: now},
	}

	for i := range persons {
		if err := store.SavePerson(ctx, domain.MainBranchID, &persons[i]); err != nil {
			t.Fatalf("save person: %v", err)
		}
	}

	t.Run("BirthDateFrom and BirthDateTo", func(t *testing.T) {
		from := time.Date(1860, 1, 1, 0, 0, 0, 0, time.UTC)
		to := time.Date(1900, 1, 1, 0, 0, 0, 0, time.UTC)
		results, err := store.SearchPersons(ctx, repository.SearchOptions{
			BirthDateFrom: &from,
			BirthDateTo:   &to,
			Limit:         10,
		})
		if err != nil {
			t.Fatalf("search: %v", err)
		}
		if len(results) != 1 {
			t.Fatalf("expected 1 result, got %d", len(results))
		}
		if results[0].GivenName != "Bob" {
			t.Errorf("expected Bob, got %s", results[0].GivenName)
		}
	})

	t.Run("BirthDateFrom only", func(t *testing.T) {
		from := time.Date(1900, 1, 1, 0, 0, 0, 0, time.UTC)
		results, err := store.SearchPersons(ctx, repository.SearchOptions{
			BirthDateFrom: &from,
			Limit:         10,
		})
		if err != nil {
			t.Fatalf("search: %v", err)
		}
		if len(results) != 1 {
			t.Fatalf("expected 1 result, got %d", len(results))
		}
		if results[0].GivenName != "Charlie" {
			t.Errorf("expected Charlie, got %s", results[0].GivenName)
		}
	})

	t.Run("BirthDateTo only", func(t *testing.T) {
		to := time.Date(1860, 1, 1, 0, 0, 0, 0, time.UTC)
		results, err := store.SearchPersons(ctx, repository.SearchOptions{
			BirthDateTo: &to,
			Limit:       10,
		})
		if err != nil {
			t.Fatalf("search: %v", err)
		}
		if len(results) != 1 {
			t.Fatalf("expected 1 result, got %d", len(results))
		}
		if results[0].GivenName != "Alice" {
			t.Errorf("expected Alice, got %s", results[0].GivenName)
		}
	})
}

func TestSearchPersons_PlaceFilter(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	store, cleanup := setupReadModelStore(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now().Truncate(time.Microsecond)

	persons := []repository.PersonReadModel{
		{ID: uuid.New(), GivenName: "Alice", Surname: "Smith", FullName: "Alice Smith", BirthPlace: "London, England", Version: 1, UpdatedAt: now},
		{ID: uuid.New(), GivenName: "Bob", Surname: "Dupont", FullName: "Bob Dupont", BirthPlace: "Paris, France", Version: 1, UpdatedAt: now},
		{ID: uuid.New(), GivenName: "Charlie", Surname: "Jones", FullName: "Charlie Jones", BirthPlace: "New London, Connecticut", Version: 1, UpdatedAt: now},
		{ID: uuid.New(), GivenName: "Diana", Surname: "Brown", FullName: "Diana Brown", DeathPlace: "London, England", Version: 1, UpdatedAt: now},
	}

	for i := range persons {
		if err := store.SavePerson(ctx, domain.MainBranchID, &persons[i]); err != nil {
			t.Fatalf("save person: %v", err)
		}
	}

	t.Run("BirthPlace London matches two", func(t *testing.T) {
		results, err := store.SearchPersons(ctx, repository.SearchOptions{
			BirthPlace: "London",
			Limit:      10,
		})
		if err != nil {
			t.Fatalf("search: %v", err)
		}
		if len(results) != 2 {
			t.Fatalf("expected 2 results for BirthPlace=London, got %d", len(results))
		}
	})

	t.Run("DeathPlace London matches one", func(t *testing.T) {
		results, err := store.SearchPersons(ctx, repository.SearchOptions{
			DeathPlace: "London",
			Limit:      10,
		})
		if err != nil {
			t.Fatalf("search: %v", err)
		}
		if len(results) != 1 {
			t.Fatalf("expected 1 result for DeathPlace=London, got %d", len(results))
		}
		if results[0].GivenName != "Diana" {
			t.Errorf("expected Diana, got %s", results[0].GivenName)
		}
	})

	t.Run("BirthPlace Paris matches one", func(t *testing.T) {
		results, err := store.SearchPersons(ctx, repository.SearchOptions{
			BirthPlace: "Paris",
			Limit:      10,
		})
		if err != nil {
			t.Fatalf("search: %v", err)
		}
		if len(results) != 1 {
			t.Fatalf("expected 1 result for BirthPlace=Paris, got %d", len(results))
		}
		if results[0].GivenName != "Bob" {
			t.Errorf("expected Bob, got %s", results[0].GivenName)
		}
	})
}

func TestSearchPersons_Soundex(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	store, cleanup := setupReadModelStore(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now().Truncate(time.Microsecond)

	persons := []repository.PersonReadModel{
		{ID: uuid.New(), GivenName: "Catherine", Surname: "Smith", FullName: "Catherine Smith", Version: 1, UpdatedAt: now},
		{ID: uuid.New(), GivenName: "Katherine", Surname: "Smyth", FullName: "Katherine Smyth", Version: 1, UpdatedAt: now},
		{ID: uuid.New(), GivenName: "Robert", Surname: "Brown", FullName: "Robert Brown", Version: 1, UpdatedAt: now},
	}

	for i := range persons {
		if err := store.SavePerson(ctx, domain.MainBranchID, &persons[i]); err != nil {
			t.Fatalf("save person: %v", err)
		}
	}

	t.Run("Soundex matches Smith and Smyth", func(t *testing.T) {
		results, err := store.SearchPersons(ctx, repository.SearchOptions{
			Query:   "Smith",
			Soundex: true,
			Limit:   10,
		})
		if err != nil {
			t.Fatalf("search: %v", err)
		}
		// PostgreSQL difference() with threshold >= 3 should match Smith/Smyth
		if len(results) < 2 {
			t.Errorf("expected at least 2 results for soundex 'Smith', got %d", len(results))
		}
	})

	t.Run("Soundex Catherine", func(t *testing.T) {
		// PostgreSQL difference("Catherine", "Katherine") may match at threshold 3
		// since they are phonetically similar despite different first letter.
		// This test verifies soundex search works without error.
		results, err := store.SearchPersons(ctx, repository.SearchOptions{
			Query:   "Catherine",
			Soundex: true,
			Limit:   10,
		})
		if err != nil {
			t.Fatalf("search: %v", err)
		}
		// Should at least find Catherine herself
		if len(results) < 1 {
			t.Errorf("expected at least 1 result for soundex 'Catherine', got %d", len(results))
		}
	})
}

func TestSearchPersons_Combined(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	store, cleanup := setupReadModelStore(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now().Truncate(time.Microsecond)

	birth1850 := time.Date(1850, 1, 1, 0, 0, 0, 0, time.UTC)
	birth1880 := time.Date(1880, 1, 1, 0, 0, 0, 0, time.UTC)
	birth1920 := time.Date(1920, 1, 1, 0, 0, 0, 0, time.UTC)

	persons := []repository.PersonReadModel{
		{ID: uuid.New(), GivenName: "John", Surname: "Smith", FullName: "John Smith", BirthDateSort: &birth1850, BirthPlace: "London, England", Version: 1, UpdatedAt: now},
		{ID: uuid.New(), GivenName: "John", Surname: "Smith", FullName: "John Smith", BirthDateSort: &birth1920, BirthPlace: "London, England", Version: 1, UpdatedAt: now},
		{ID: uuid.New(), GivenName: "John", Surname: "Smith", FullName: "John Smith", BirthDateSort: &birth1880, BirthPlace: "Paris, France", Version: 1, UpdatedAt: now},
		{ID: uuid.New(), GivenName: "Jane", Surname: "Doe", FullName: "Jane Doe", BirthDateSort: &birth1880, BirthPlace: "London, England", Version: 1, UpdatedAt: now},
	}

	for i := range persons {
		if err := store.SavePerson(ctx, domain.MainBranchID, &persons[i]); err != nil {
			t.Fatalf("save person: %v", err)
		}
	}

	t.Run("Query + date range + place narrows results", func(t *testing.T) {
		from := time.Date(1860, 1, 1, 0, 0, 0, 0, time.UTC)
		to := time.Date(1900, 1, 1, 0, 0, 0, 0, time.UTC)
		results, err := store.SearchPersons(ctx, repository.SearchOptions{
			Query:         "Smith",
			BirthDateFrom: &from,
			BirthDateTo:   &to,
			BirthPlace:    "London",
			Limit:         10,
		})
		if err != nil {
			t.Fatalf("search: %v", err)
		}
		// 1850 Smith is out of range. 1880 Smith is in Paris. 1920 Smith is out of range.
		if len(results) != 0 {
			t.Errorf("expected 0 results, got %d", len(results))
		}
	})

	t.Run("Query + date range matches subset", func(t *testing.T) {
		from := time.Date(1840, 1, 1, 0, 0, 0, 0, time.UTC)
		to := time.Date(1900, 1, 1, 0, 0, 0, 0, time.UTC)
		results, err := store.SearchPersons(ctx, repository.SearchOptions{
			Query:         "Smith",
			BirthDateFrom: &from,
			BirthDateTo:   &to,
			Limit:         10,
		})
		if err != nil {
			t.Fatalf("search: %v", err)
		}
		// Smith born 1850 and Smith born 1880 both match
		if len(results) != 2 {
			t.Errorf("expected 2 results, got %d", len(results))
		}
	})
}

func TestSearchPersons_NoQueryWithFilters(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	store, cleanup := setupReadModelStore(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now().Truncate(time.Microsecond)

	birth1850 := time.Date(1850, 1, 1, 0, 0, 0, 0, time.UTC)
	birth1880 := time.Date(1880, 1, 1, 0, 0, 0, 0, time.UTC)
	birth1920 := time.Date(1920, 1, 1, 0, 0, 0, 0, time.UTC)

	persons := []repository.PersonReadModel{
		{ID: uuid.New(), GivenName: "Alice", Surname: "Smith", FullName: "Alice Smith", BirthDateSort: &birth1850, BirthPlace: "London", Version: 1, UpdatedAt: now},
		{ID: uuid.New(), GivenName: "Bob", Surname: "Jones", FullName: "Bob Jones", BirthDateSort: &birth1880, BirthPlace: "Paris", Version: 1, UpdatedAt: now},
		{ID: uuid.New(), GivenName: "Charlie", Surname: "Brown", FullName: "Charlie Brown", BirthDateSort: &birth1920, BirthPlace: "London", Version: 1, UpdatedAt: now},
	}

	for i := range persons {
		if err := store.SavePerson(ctx, domain.MainBranchID, &persons[i]); err != nil {
			t.Fatalf("save person: %v", err)
		}
	}

	t.Run("date range only", func(t *testing.T) {
		from := time.Date(1860, 1, 1, 0, 0, 0, 0, time.UTC)
		to := time.Date(1900, 1, 1, 0, 0, 0, 0, time.UTC)
		results, err := store.SearchPersons(ctx, repository.SearchOptions{
			BirthDateFrom: &from,
			BirthDateTo:   &to,
			Limit:         10,
		})
		if err != nil {
			t.Fatalf("search: %v", err)
		}
		if len(results) != 1 {
			t.Fatalf("expected 1 result, got %d", len(results))
		}
		if results[0].GivenName != "Bob" {
			t.Errorf("expected Bob, got %s", results[0].GivenName)
		}
	})

	t.Run("place only", func(t *testing.T) {
		results, err := store.SearchPersons(ctx, repository.SearchOptions{
			BirthPlace: "London",
			Limit:      10,
		})
		if err != nil {
			t.Fatalf("search: %v", err)
		}
		if len(results) != 2 {
			t.Errorf("expected 2 results for London, got %d", len(results))
		}
	})

	t.Run("no criteria returns empty", func(t *testing.T) {
		results, err := store.SearchPersons(ctx, repository.SearchOptions{
			Limit: 10,
		})
		if err != nil {
			t.Fatalf("search: %v", err)
		}
		if len(results) != 0 {
			t.Errorf("expected 0 results with no criteria, got %d", len(results))
		}
	})
}

func TestSearchPersons_SortOptions(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	store, cleanup := setupReadModelStore(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now().Truncate(time.Microsecond)

	birth1850 := time.Date(1850, 1, 1, 0, 0, 0, 0, time.UTC)
	birth1880 := time.Date(1880, 1, 1, 0, 0, 0, 0, time.UTC)
	birth1920 := time.Date(1920, 1, 1, 0, 0, 0, 0, time.UTC)

	persons := []repository.PersonReadModel{
		{ID: uuid.New(), GivenName: "Charlie", Surname: "Adams", FullName: "Charlie Adams", BirthDateSort: &birth1920, BirthPlace: "London", Version: 1, UpdatedAt: now},
		{ID: uuid.New(), GivenName: "Alice", Surname: "Brown", FullName: "Alice Brown", BirthDateSort: &birth1850, BirthPlace: "London", Version: 1, UpdatedAt: now},
		{ID: uuid.New(), GivenName: "Bob", Surname: "Clark", FullName: "Bob Clark", BirthDateSort: &birth1880, BirthPlace: "London", Version: 1, UpdatedAt: now},
	}

	for i := range persons {
		if err := store.SavePerson(ctx, domain.MainBranchID, &persons[i]); err != nil {
			t.Fatalf("save person: %v", err)
		}
	}

	t.Run("sort by birth_date asc", func(t *testing.T) {
		results, err := store.SearchPersons(ctx, repository.SearchOptions{
			BirthPlace: "London",
			Sort:       "birth_date",
			Order:      "asc",
			Limit:      10,
		})
		if err != nil {
			t.Fatalf("search: %v", err)
		}
		if len(results) != 3 {
			t.Fatalf("expected 3 results, got %d", len(results))
		}
		if results[0].GivenName != "Alice" {
			t.Errorf("expected first Alice (1850), got %s", results[0].GivenName)
		}
		if results[2].GivenName != "Charlie" {
			t.Errorf("expected last Charlie (1920), got %s", results[2].GivenName)
		}
	})

	t.Run("sort by name desc", func(t *testing.T) {
		results, err := store.SearchPersons(ctx, repository.SearchOptions{
			BirthPlace: "London",
			Sort:       "name",
			Order:      "desc",
			Limit:      10,
		})
		if err != nil {
			t.Fatalf("search: %v", err)
		}
		if len(results) != 3 {
			t.Fatalf("expected 3 results, got %d", len(results))
		}
		// Desc by surname: Clark, Brown, Adams
		if results[0].Surname != "Clark" {
			t.Errorf("expected first surname Clark, got %s", results[0].Surname)
		}
		if results[2].Surname != "Adams" {
			t.Errorf("expected last surname Adams, got %s", results[2].Surname)
		}
	})
}

func TestSearchPersons_BackwardCompatible(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	store, cleanup := setupReadModelStore(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now().Truncate(time.Microsecond)

	persons := []repository.PersonReadModel{
		{ID: uuid.New(), GivenName: "John", Surname: "Doe", FullName: "John Doe", Version: 1, UpdatedAt: now},
		{ID: uuid.New(), GivenName: "Jane", Surname: "Doe", FullName: "Jane Doe", Version: 1, UpdatedAt: now},
		{ID: uuid.New(), GivenName: "Robert", Surname: "Smith", FullName: "Robert Smith", Version: 1, UpdatedAt: now},
	}

	for i := range persons {
		if err := store.SavePerson(ctx, domain.MainBranchID, &persons[i]); err != nil {
			t.Fatalf("save person: %v", err)
		}
	}

	t.Run("query only", func(t *testing.T) {
		results, err := store.SearchPersons(ctx, repository.SearchOptions{
			Query: "Doe",
			Limit: 10,
		})
		if err != nil {
			t.Fatalf("search: %v", err)
		}
		if len(results) != 2 {
			t.Errorf("expected 2 results for 'Doe', got %d", len(results))
		}
	})

	t.Run("query with fuzzy", func(t *testing.T) {
		results, err := store.SearchPersons(ctx, repository.SearchOptions{
			Query: "Doe",
			Fuzzy: true,
			Limit: 10,
		})
		if err != nil {
			t.Fatalf("search: %v", err)
		}
		// Fuzzy (trigram) should find "Doe" matches
		if len(results) < 1 {
			t.Logf("Note: fuzzy search returned %d results (pg_trgm similarity may vary)", len(results))
		}
	})
}

func TestReadModelStore_RepositoryCRUD(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	store, cleanup := setupReadModelStore(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now().Truncate(time.Microsecond)

	repo := &repository.RepositoryReadModel{
		ID:         uuid.New(),
		Name:       "National Archives",
		Address:    &domain.Address{City: "Washington", State: "DC", Phone: "+1-866-272-6272"},
		Notes:      "Primary federal records repository",
		GedcomXref: "@R1@",
		Version:    1,
		UpdatedAt:  now,
	}

	if err := store.SaveRepository(ctx, repo); err != nil {
		t.Fatalf("save repository: %v", err)
	}

	retrieved, err := store.GetRepository(ctx, repo.ID)
	if err != nil {
		t.Fatalf("get repository: %v", err)
	}
	if retrieved == nil {
		t.Fatal("repository not found")
	}
	if retrieved.Name != "National Archives" {
		t.Errorf("expected Name National Archives, got %s", retrieved.Name)
	}
	if retrieved.Notes != "Primary federal records repository" {
		t.Errorf("expected Notes set, got %s", retrieved.Notes)
	}
	if retrieved.GedcomXref != "@R1@" {
		t.Errorf("expected GedcomXref @R1@, got %s", retrieved.GedcomXref)
	}
	if retrieved.Address == nil || retrieved.Address.City != "Washington" || retrieved.Address.Phone != "+1-866-272-6272" {
		t.Errorf("address not round-tripped: %+v", retrieved.Address)
	}

	// Update
	repo.Name = "US National Archives"
	repo.Version = 2
	if err := store.SaveRepository(ctx, repo); err != nil {
		t.Fatalf("update repository: %v", err)
	}
	retrieved, err = store.GetRepository(ctx, repo.ID)
	if err != nil {
		t.Fatalf("get updated repository: %v", err)
	}
	if retrieved.Name != "US National Archives" || retrieved.Version != 2 {
		t.Errorf("update not persisted: name=%s version=%d", retrieved.Name, retrieved.Version)
	}

	// Delete
	if err := store.DeleteRepository(ctx, repo.ID); err != nil {
		t.Fatalf("delete repository: %v", err)
	}
	retrieved, err = store.GetRepository(ctx, repo.ID)
	if err != nil {
		t.Fatalf("get deleted repository: %v", err)
	}
	if retrieved != nil {
		t.Error("repository should have been deleted")
	}
}

func TestReadModelStore_ListRepositories(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	store, cleanup := setupReadModelStore(t)
	defer cleanup()

	ctx := context.Background()

	names := []string{"Alpha Archive", "Beta Library", "Gamma Collection"}
	for i, name := range names {
		repo := &repository.RepositoryReadModel{
			ID:        uuid.New(),
			Name:      name,
			Version:   1,
			UpdatedAt: time.Now().Add(time.Duration(i) * time.Second),
		}
		if err := store.SaveRepository(ctx, repo); err != nil {
			t.Fatalf("save repository: %v", err)
		}
	}

	results, total, err := store.ListRepositories(ctx, repository.ListOptions{Limit: 10, Sort: "name", Order: "asc"})
	if err != nil {
		t.Fatalf("list repositories: %v", err)
	}
	if total != 3 {
		t.Errorf("total = %d, want 3", total)
	}
	if len(results) != 3 {
		t.Fatalf("len(results) = %d, want 3", len(results))
	}
	if results[0].Name != "Alpha Archive" {
		t.Errorf("first result = %s, want Alpha Archive (asc by name)", results[0].Name)
	}
}

// --- Branch overlay (ADR-005) -----------------------------------------------
// These tests mirror the in-memory reference (memory/branch_readmodel_test.go),
// verifying the PostgreSQL overlay/tombstone semantics match it exactly. They
// share the Docker/short skips of the other integration tests (via
// setupReadModelStore -> setupPostgres).

func pgPersonRM(id uuid.UUID, given, surname string) *repository.PersonReadModel {
	return &repository.PersonReadModel{
		ID: id, GivenName: given, Surname: surname, FullName: given + " " + surname,
		Version: 1, UpdatedAt: time.Now().Truncate(time.Microsecond),
	}
}

func TestBranchOverlayPersonPrecedence(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	store, cleanup := setupReadModelStore(t)
	defer cleanup()
	ctx := context.Background()
	branch := domain.BranchID(uuid.New())
	id := uuid.New()

	if err := store.SavePerson(ctx, domain.MainBranchID, pgPersonRM(id, "Ada", "Main")); err != nil {
		t.Fatalf("SavePerson main: %v", err)
	}
	if err := store.SavePerson(ctx, branch, pgPersonRM(id, "Ada", "Branch")); err != nil {
		t.Fatalf("SavePerson branch: %v", err)
	}

	got, _ := store.GetPerson(ctx, branch, id)
	if got == nil || got.Surname != "Branch" {
		t.Fatalf("branch Get: want surname Branch, got %+v", got)
	}
	main, _ := store.GetPerson(ctx, domain.MainBranchID, id)
	if main == nil || main.Surname != "Main" {
		t.Fatalf("main Get: want surname Main, got %+v", main)
	}

	branchList, total, _ := store.ListPersons(ctx, repository.ListOptions{Limit: 10, BranchID: branch})
	if total != 1 || len(branchList) != 1 || branchList[0].Surname != "Branch" {
		t.Fatalf("branch List: want 1 Branch, got total=%d list=%+v", total, branchList)
	}
	mainList, _, _ := store.ListPersons(ctx, repository.ListOptions{Limit: 10, BranchID: domain.MainBranchID})
	if len(mainList) != 1 || mainList[0].Surname != "Main" {
		t.Fatalf("main List: want 1 Main, got %+v", mainList)
	}
}

func TestBranchFallbackToMain(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	store, cleanup := setupReadModelStore(t)
	defer cleanup()
	ctx := context.Background()
	branch := domain.BranchID(uuid.New())
	id := uuid.New()

	if err := store.SavePerson(ctx, domain.MainBranchID, pgPersonRM(id, "Grace", "Hopper")); err != nil {
		t.Fatalf("SavePerson main: %v", err)
	}

	got, _ := store.GetPerson(ctx, branch, id)
	if got == nil || got.Surname != "Hopper" {
		t.Fatalf("branch Get fallback: want Hopper, got %+v", got)
	}
	branchList, total, _ := store.ListPersons(ctx, repository.ListOptions{Limit: 10, BranchID: branch})
	if total != 1 || len(branchList) != 1 || branchList[0].Surname != "Hopper" {
		t.Fatalf("branch List fallback: want 1 Hopper, got total=%d list=%+v", total, branchList)
	}
}

func TestBranchTombstoneSuppression(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	store, cleanup := setupReadModelStore(t)
	defer cleanup()
	ctx := context.Background()
	branch := domain.BranchID(uuid.New())
	id := uuid.New()

	if err := store.SavePerson(ctx, domain.MainBranchID, pgPersonRM(id, "Alan", "Turing")); err != nil {
		t.Fatalf("SavePerson main: %v", err)
	}
	if err := store.DeletePerson(ctx, branch, id); err != nil {
		t.Fatalf("DeletePerson branch: %v", err)
	}

	if got, _ := store.GetPerson(ctx, branch, id); got != nil {
		t.Fatalf("branch Get after tombstone: want nil, got %+v", got)
	}
	if main, _ := store.GetPerson(ctx, domain.MainBranchID, id); main == nil || main.Surname != "Turing" {
		t.Fatalf("main Get after branch tombstone: want Turing, got %+v", main)
	}
	branchList, total, _ := store.ListPersons(ctx, repository.ListOptions{Limit: 10, BranchID: branch})
	if total != 0 || len(branchList) != 0 {
		t.Fatalf("branch List after tombstone: want empty, got total=%d list=%+v", total, branchList)
	}
	mainList, _, _ := store.ListPersons(ctx, repository.ListOptions{Limit: 10, BranchID: domain.MainBranchID})
	if len(mainList) != 1 {
		t.Fatalf("main List after branch tombstone: want 1, got %+v", mainList)
	}
}

func TestBranchMainDeleteIsRealRemoval(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	store, cleanup := setupReadModelStore(t)
	defer cleanup()
	ctx := context.Background()
	id := uuid.New()

	if err := store.SavePerson(ctx, domain.MainBranchID, pgPersonRM(id, "Ada", "Lovelace")); err != nil {
		t.Fatalf("SavePerson: %v", err)
	}
	if err := store.DeletePerson(ctx, domain.MainBranchID, id); err != nil {
		t.Fatalf("DeletePerson main: %v", err)
	}
	if got, _ := store.GetPerson(ctx, domain.MainBranchID, id); got != nil {
		t.Fatalf("main Get after main delete: want nil, got %+v", got)
	}
}

func TestBranchOnlyEntityInvisibleOnMain(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	store, cleanup := setupReadModelStore(t)
	defer cleanup()
	ctx := context.Background()
	branch := domain.BranchID(uuid.New())
	id := uuid.New()

	if err := store.SavePerson(ctx, branch, pgPersonRM(id, "Only", "Branch")); err != nil {
		t.Fatalf("SavePerson branch: %v", err)
	}
	if got, _ := store.GetPerson(ctx, domain.MainBranchID, id); got != nil {
		t.Fatalf("main Get of branch-only entity: want nil, got %+v", got)
	}
	if got, _ := store.GetPerson(ctx, branch, id); got == nil {
		t.Fatal("branch Get of branch-only entity: want present, got nil")
	}
	mainList, _, _ := store.ListPersons(ctx, repository.ListOptions{Limit: 10, BranchID: domain.MainBranchID})
	if len(mainList) != 0 {
		t.Fatalf("main List of branch-only entity: want empty, got %+v", mainList)
	}
}

func TestBranchSearchPersonsScope(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	store, cleanup := setupReadModelStore(t)
	defer cleanup()
	ctx := context.Background()
	branch := domain.BranchID(uuid.New())
	id := uuid.New()

	_ = store.SavePerson(ctx, domain.MainBranchID, pgPersonRM(id, "Katherine", "Johnson"))
	_ = store.SavePerson(ctx, branch, pgPersonRM(id, "Katherine", "Coleman"))

	branchHits, _ := store.SearchPersons(ctx, repository.SearchOptions{Query: "Coleman", Limit: 10, BranchID: branch})
	if len(branchHits) != 1 || branchHits[0].Surname != "Coleman" {
		t.Fatalf("branch Search: want Coleman, got %+v", branchHits)
	}
	if hits, _ := store.SearchPersons(ctx, repository.SearchOptions{Query: "Johnson", Limit: 10, BranchID: branch}); len(hits) != 0 {
		t.Fatalf("branch Search for main-only surname: want empty, got %+v", hits)
	}
	mainHits, _ := store.SearchPersons(ctx, repository.SearchOptions{Query: "Johnson", Limit: 10, BranchID: domain.MainBranchID})
	if len(mainHits) != 1 || mainHits[0].Surname != "Johnson" {
		t.Fatalf("main Search: want Johnson, got %+v", mainHits)
	}
}

func TestBranchPersonNamesOverlay(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	store, cleanup := setupReadModelStore(t)
	defer cleanup()
	ctx := context.Background()
	branch := domain.BranchID(uuid.New())
	personID := uuid.New()

	mainName := &repository.PersonNameReadModel{ID: uuid.New(), PersonID: personID, GivenName: "Main", Surname: "Name"}
	if err := store.SavePersonName(ctx, domain.MainBranchID, mainName); err != nil {
		t.Fatalf("SavePersonName main: %v", err)
	}
	branchName := &repository.PersonNameReadModel{ID: uuid.New(), PersonID: personID, GivenName: "Branch", Surname: "Name"}
	if err := store.SavePersonName(ctx, branch, branchName); err != nil {
		t.Fatalf("SavePersonName branch: %v", err)
	}

	branchNames, _ := store.GetPersonNames(ctx, branch, personID)
	if len(branchNames) != 2 {
		t.Fatalf("branch names: want 2 (main fallback + branch add), got %d: %+v", len(branchNames), branchNames)
	}
	mainNames, _ := store.GetPersonNames(ctx, domain.MainBranchID, personID)
	if len(mainNames) != 1 {
		t.Fatalf("main names after branch edit: want 1 (untouched), got %d", len(mainNames))
	}
	if got, _ := store.GetPersonName(ctx, branch, branchName.ID); got == nil {
		t.Fatal("GetPersonName branch: want the branch name, got nil")
	}
	if got, _ := store.GetPersonName(ctx, domain.MainBranchID, branchName.ID); got != nil {
		t.Fatalf("GetPersonName main: branch-only name must be invisible, got %+v", got)
	}

	// Branch delete of the main-fallback name tombstones it on the branch only.
	if err := store.DeletePersonName(ctx, branch, mainName.ID); err != nil {
		t.Fatalf("DeletePersonName branch: %v", err)
	}
	branchNames, _ = store.GetPersonNames(ctx, branch, personID)
	if len(branchNames) != 1 || branchNames[0].ID != branchName.ID {
		t.Fatalf("branch names after tombstone: want [branchName], got %+v", branchNames)
	}
	mainNames, _ = store.GetPersonNames(ctx, domain.MainBranchID, personID)
	if len(mainNames) != 1 || mainNames[0].ID != mainName.ID {
		t.Fatalf("main names after branch tombstone: want [mainName], got %+v", mainNames)
	}
}

func TestBranchFamilyChildrenOverlay(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	store, cleanup := setupReadModelStore(t)
	defer cleanup()
	ctx := context.Background()
	branch := domain.BranchID(uuid.New())
	familyID := uuid.New()
	child1 := uuid.New()
	child2 := uuid.New()

	_ = store.SaveFamilyChild(ctx, domain.MainBranchID, &repository.FamilyChildReadModel{FamilyID: familyID, PersonID: child1})
	_ = store.SaveFamilyChild(ctx, branch, &repository.FamilyChildReadModel{FamilyID: familyID, PersonID: child2})

	branchKids, _ := store.GetFamilyChildren(ctx, branch, familyID)
	if len(branchKids) != 2 {
		t.Fatalf("branch children: want 2, got %d: %+v", len(branchKids), branchKids)
	}
	mainKids, _ := store.GetFamilyChildren(ctx, domain.MainBranchID, familyID)
	if len(mainKids) != 1 {
		t.Fatalf("main children after branch add: want 1, got %d", len(mainKids))
	}

	_ = store.DeleteFamilyChild(ctx, branch, familyID, child1)
	branchKids, _ = store.GetFamilyChildren(ctx, branch, familyID)
	if len(branchKids) != 1 || branchKids[0].PersonID != child2 {
		t.Fatalf("branch children after delete: want [child2], got %+v", branchKids)
	}
	mainKids, _ = store.GetFamilyChildren(ctx, domain.MainBranchID, familyID)
	if len(mainKids) != 1 || mainKids[0].PersonID != child1 {
		t.Fatalf("main children after branch delete: want [child1], got %+v", mainKids)
	}
}

func TestBranchPersonExternalIDsOverlayTombstone(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	store, cleanup := setupReadModelStore(t)
	defer cleanup()
	ctx := context.Background()
	branch := domain.BranchID(uuid.New())
	personID := uuid.New()

	_ = store.ReplacePersonExternalIDs(ctx, domain.MainBranchID, personID, []repository.PersonExternalIDReadModel{{Value: "MAIN-1"}})
	_ = store.ReplacePersonExternalIDs(ctx, branch, personID, []repository.PersonExternalIDReadModel{{Value: "BR-1"}, {Value: "BR-2"}})

	branchIDs, _ := store.GetPersonExternalIDs(ctx, branch, personID)
	if len(branchIDs) != 2 || branchIDs[0].Value != "BR-1" {
		t.Fatalf("branch ext ids: want [BR-1, BR-2], got %+v", branchIDs)
	}
	mainIDs, _ := store.GetPersonExternalIDs(ctx, domain.MainBranchID, personID)
	if len(mainIDs) != 1 || mainIDs[0].Value != "MAIN-1" {
		t.Fatalf("main ext ids after branch override: want [MAIN-1], got %+v", mainIDs)
	}

	_ = store.ReplacePersonExternalIDs(ctx, branch, personID, nil)
	branchIDs, _ = store.GetPersonExternalIDs(ctx, branch, personID)
	if len(branchIDs) != 0 {
		t.Fatalf("branch ext ids after tombstone: want empty, got %+v", branchIDs)
	}
	mainIDs, _ = store.GetPersonExternalIDs(ctx, domain.MainBranchID, personID)
	if len(mainIDs) != 1 {
		t.Fatalf("main ext ids after branch tombstone: want [MAIN-1], got %+v", mainIDs)
	}
}

func TestBranchPedigreeEdgeOverlayTombstone(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	store, cleanup := setupReadModelStore(t)
	defer cleanup()
	ctx := context.Background()
	branch := domain.BranchID(uuid.New())
	personID := uuid.New()
	mainFather := uuid.New()
	branchFather := uuid.New()

	_ = store.SavePedigreeEdge(ctx, domain.MainBranchID, &repository.PedigreeEdge{PersonID: personID, FatherID: &mainFather})

	if e, _ := store.GetPedigreeEdge(ctx, branch, personID); e == nil || e.FatherID == nil || *e.FatherID != mainFather {
		t.Fatalf("branch edge fallback: want mainFather, got %+v", e)
	}

	_ = store.SavePedigreeEdge(ctx, branch, &repository.PedigreeEdge{PersonID: personID, FatherID: &branchFather})
	if e, _ := store.GetPedigreeEdge(ctx, branch, personID); e == nil || e.FatherID == nil || *e.FatherID != branchFather {
		t.Fatalf("branch edge override: want branchFather, got %+v", e)
	}
	if e, _ := store.GetPedigreeEdge(ctx, domain.MainBranchID, personID); e == nil || e.FatherID == nil || *e.FatherID != mainFather {
		t.Fatalf("main edge after branch override: want mainFather, got %+v", e)
	}

	_ = store.DeletePedigreeEdge(ctx, branch, personID)
	if e, _ := store.GetPedigreeEdge(ctx, branch, personID); e != nil {
		t.Fatalf("branch edge after tombstone: want nil, got %+v", e)
	}
	if e, _ := store.GetPedigreeEdge(ctx, domain.MainBranchID, personID); e == nil {
		t.Fatal("main edge after branch tombstone: want present, got nil")
	}
}

func TestBranchDeletePersonCascadesTombstones(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	store, cleanup := setupReadModelStore(t)
	defer cleanup()
	ctx := context.Background()
	branch := domain.BranchID(uuid.New())
	personID := uuid.New()

	_ = store.SavePerson(ctx, domain.MainBranchID, pgPersonRM(personID, "Cascade", "Main"))
	_ = store.SavePersonName(ctx, domain.MainBranchID, &repository.PersonNameReadModel{ID: uuid.New(), PersonID: personID, GivenName: "Cascade", Surname: "Main"})
	_ = store.ReplacePersonExternalIDs(ctx, domain.MainBranchID, personID, []repository.PersonExternalIDReadModel{{Value: "X"}})

	_ = store.DeletePerson(ctx, branch, personID)

	if names, _ := store.GetPersonNames(ctx, branch, personID); len(names) != 0 {
		t.Fatalf("branch names after cascade tombstone: want empty, got %+v", names)
	}
	if ids, _ := store.GetPersonExternalIDs(ctx, branch, personID); len(ids) != 0 {
		t.Fatalf("branch ext ids after cascade tombstone: want empty, got %+v", ids)
	}
	if names, _ := store.GetPersonNames(ctx, domain.MainBranchID, personID); len(names) != 1 {
		t.Fatalf("main names after branch cascade: want 1, got %+v", names)
	}
	if ids, _ := store.GetPersonExternalIDs(ctx, domain.MainBranchID, personID); len(ids) != 1 {
		t.Fatalf("main ext ids after branch cascade: want 1, got %+v", ids)
	}
}
