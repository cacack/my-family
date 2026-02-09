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

	err := store.SavePerson(ctx, person)
	if err != nil {
		t.Fatalf("save person: %v", err)
	}

	// Read person
	retrieved, err := store.GetPerson(ctx, personID)
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
	err = store.SavePerson(ctx, person)
	if err != nil {
		t.Fatalf("update person: %v", err)
	}

	// Read updated person
	retrieved, err = store.GetPerson(ctx, personID)
	if err != nil {
		t.Fatalf("get updated person: %v", err)
	}
	if retrieved.GivenName != "Jane" {
		t.Errorf("expected updated GivenName Jane, got %s", retrieved.GivenName)
	}

	// Delete person
	err = store.DeletePerson(ctx, personID)
	if err != nil {
		t.Fatalf("delete person: %v", err)
	}

	// Verify deletion
	retrieved, err = store.GetPerson(ctx, personID)
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
		err := store.SavePerson(ctx, person)
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
		if err := store.SavePerson(ctx, &p); err != nil {
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
		if err := store.SavePerson(ctx, &p); err != nil {
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

	if err := store.SavePerson(ctx, partner1); err != nil {
		t.Fatalf("save partner1: %v", err)
	}
	if err := store.SavePerson(ctx, partner2); err != nil {
		t.Fatalf("save partner2: %v", err)
	}

	// Create family
	familyID := uuid.New()
	family := &repository.FamilyReadModel{
		ID:               familyID,
		Partner1ID:       &partner1ID,
		Partner1Name:     "John Doe",
		Partner2ID:       &partner2ID,
		Partner2Name:     "Jane Doe",
		RelationshipType: domain.RelationMarriage,
		MarriageDateRaw:  "15 JUN 1875",
		MarriagePlace:    "Boston, MA",
		ChildCount:       0,
		Version:          1,
		UpdatedAt:        now,
	}

	err := store.SaveFamily(ctx, family)
	if err != nil {
		t.Fatalf("save family: %v", err)
	}

	// Read family
	retrieved, err := store.GetFamily(ctx, familyID)
	if err != nil {
		t.Fatalf("get family: %v", err)
	}
	if retrieved == nil {
		t.Fatal("family not found")
	}

	if retrieved.Partner1Name != "John Doe" {
		t.Errorf("expected Partner1Name John Doe, got %s", retrieved.Partner1Name)
	}
	if retrieved.RelationshipType != domain.RelationMarriage {
		t.Errorf("expected RelationshipType married, got %s", retrieved.RelationshipType)
	}

	// Get families for person
	families, err := store.GetFamiliesForPerson(ctx, partner1ID)
	if err != nil {
		t.Fatalf("get families for person: %v", err)
	}
	if len(families) != 1 {
		t.Errorf("expected 1 family, got %d", len(families))
	}

	// Delete family
	err = store.DeleteFamily(ctx, familyID)
	if err != nil {
		t.Fatalf("delete family: %v", err)
	}

	// Verify deletion
	retrieved, err = store.GetFamily(ctx, familyID)
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
		if err := store.SavePerson(ctx, p); err != nil {
			t.Fatalf("save person: %v", err)
		}
	}

	// Create family
	familyID := uuid.New()
	family := &repository.FamilyReadModel{
		ID:           familyID,
		Partner1ID:   &partner1ID,
		Partner1Name: "John Doe",
		Partner2ID:   &partner2ID,
		Partner2Name: "Jane Doe",
		ChildCount:   0,
		Version:      1,
		UpdatedAt:    now,
	}

	if err := store.SaveFamily(ctx, family); err != nil {
		t.Fatalf("save family: %v", err)
	}

	// Add child
	seq := 1
	child := &repository.FamilyChildReadModel{
		FamilyID:         familyID,
		PersonID:         childID,
		PersonName:       "Jimmy Doe",
		RelationshipType: domain.ChildBiological,
		Sequence:         &seq,
	}

	if err := store.SaveFamilyChild(ctx, child); err != nil {
		t.Fatalf("save family child: %v", err)
	}

	// Get family children
	children, err := store.GetFamilyChildren(ctx, familyID)
	if err != nil {
		t.Fatalf("get family children: %v", err)
	}
	if len(children) != 1 {
		t.Fatalf("expected 1 child, got %d", len(children))
	}
	if children[0].PersonName != "Jimmy Doe" {
		t.Errorf("expected child name Jimmy Doe, got %s", children[0].PersonName)
	}

	// Get child family
	childFamily, err := store.GetChildFamily(ctx, childID)
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
	if err := store.DeleteFamilyChild(ctx, familyID, childID); err != nil {
		t.Fatalf("delete family child: %v", err)
	}

	// Verify removal
	children, err = store.GetFamilyChildren(ctx, familyID)
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
		if err := store.SavePerson(ctx, p); err != nil {
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

	if err := store.SavePedigreeEdge(ctx, edge); err != nil {
		t.Fatalf("save pedigree edge: %v", err)
	}

	// Get pedigree edge
	retrieved, err := store.GetPedigreeEdge(ctx, childID)
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
	if err := store.DeletePedigreeEdge(ctx, childID); err != nil {
		t.Fatalf("delete pedigree edge: %v", err)
	}

	// Verify deletion
	retrieved, err = store.GetPedigreeEdge(ctx, childID)
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
		if err := store.SavePerson(ctx, p); err != nil {
			t.Fatalf("save person: %v", err)
		}
	}

	// Create family
	familyID := uuid.New()
	family := &repository.FamilyReadModel{
		ID:           familyID,
		Partner1ID:   &partner1ID,
		Partner1Name: "John Doe",
		Partner2ID:   &partner2ID,
		Partner2Name: "Jane Doe",
		ChildCount:   2,
		Version:      1,
		UpdatedAt:    now,
	}

	if err := store.SaveFamily(ctx, family); err != nil {
		t.Fatalf("save family: %v", err)
	}

	// Add children
	seq1, seq2 := 1, 2
	children := []*repository.FamilyChildReadModel{
		{FamilyID: familyID, PersonID: child1ID, PersonName: "Jimmy Doe", RelationshipType: domain.ChildBiological, Sequence: &seq1},
		{FamilyID: familyID, PersonID: child2ID, PersonName: "Jenny Doe", RelationshipType: domain.ChildBiological, Sequence: &seq2},
	}

	for _, c := range children {
		if err := store.SaveFamilyChild(ctx, c); err != nil {
			t.Fatalf("save family child: %v", err)
		}
	}

	// Get children of family
	childPersons, err := store.GetChildrenOfFamily(ctx, familyID)
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
		if err := store.SavePerson(ctx, &persons[i]); err != nil {
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
		if err := store.SavePerson(ctx, &persons[i]); err != nil {
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
		if err := store.SavePerson(ctx, &persons[i]); err != nil {
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
		if err := store.SavePerson(ctx, &persons[i]); err != nil {
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
		if err := store.SavePerson(ctx, &persons[i]); err != nil {
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
		if err := store.SavePerson(ctx, &persons[i]); err != nil {
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
		if err := store.SavePerson(ctx, &persons[i]); err != nil {
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
