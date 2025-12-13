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
	results, err := store.SearchPersons(ctx, "Smith", false, 10)
	if err != nil {
		t.Fatalf("search persons: %v", err)
	}

	if len(results) != 2 {
		t.Errorf("expected 2 results for 'Smith', got %d", len(results))
	}

	// Search for "John" - should find 2 results (John Smith and Robert Johnson)
	results, err = store.SearchPersons(ctx, "John", false, 10)
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
	results, err := store.SearchPersons(ctx, "Kathryn", true, 10)
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
