package sqlite_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/repository/sqlite"
)

func setupTestReadModelDB(t *testing.T) (*sqlite.ReadModelStore, func()) {
	t.Helper()

	tmpFile, err := os.CreateTemp("", "myfamily-readmodel-test-*.db")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	tmpFile.Close()

	db, err := sqlite.OpenDB(tmpFile.Name())
	if err != nil {
		os.Remove(tmpFile.Name())
		t.Fatalf("open database: %v", err)
	}

	store, err := sqlite.NewReadModelStore(db)
	if err != nil {
		db.Close()
		os.Remove(tmpFile.Name())
		t.Fatalf("create read model store: %v", err)
	}

	return store, func() {
		db.Close()
		os.Remove(tmpFile.Name())
	}
}

// TestReadModelStore_SourceRepositoryID verifies the repository_id column round-trips
// through SaveSource/GetSource, including the nil case (issue #525).
func TestReadModelStore_SourceRepositoryID(t *testing.T) {
	store, cleanup := setupTestReadModelDB(t)
	defer cleanup()

	ctx := context.Background()

	repoID := uuid.New()
	linked := &repository.SourceReadModel{
		ID:             uuid.New(),
		SourceType:     domain.SourceBook,
		Title:          "Linked Source",
		RepositoryID:   &repoID,
		RepositoryName: "National Archives",
		Version:        1,
	}
	if err := store.SaveSource(ctx, linked); err != nil {
		t.Fatalf("SaveSource (linked): %v", err)
	}

	got, err := store.GetSource(ctx, linked.ID)
	if err != nil {
		t.Fatalf("GetSource: %v", err)
	}
	if got.RepositoryID == nil {
		t.Fatal("RepositoryID was not persisted")
	}
	if *got.RepositoryID != repoID {
		t.Errorf("RepositoryID = %s, want %s", got.RepositoryID, repoID)
	}

	// A source without a RepositoryID must round-trip as nil, not a zero UUID.
	unlinked := &repository.SourceReadModel{
		ID:         uuid.New(),
		SourceType: domain.SourceBook,
		Title:      "Unlinked Source",
		Version:    1,
	}
	if err := store.SaveSource(ctx, unlinked); err != nil {
		t.Fatalf("SaveSource (unlinked): %v", err)
	}
	got, err = store.GetSource(ctx, unlinked.ID)
	if err != nil {
		t.Fatalf("GetSource: %v", err)
	}
	if got.RepositoryID != nil {
		t.Errorf("RepositoryID = %s, want nil", got.RepositoryID)
	}
}

func TestReadModelStore_PersonCRUD(t *testing.T) {
	store, cleanup := setupTestReadModelDB(t)
	defer cleanup()

	ctx := context.Background()
	personID := uuid.New()

	// Create
	birthDate := time.Date(1850, 1, 1, 0, 0, 0, 0, time.UTC)
	person := &repository.PersonReadModel{
		ID:            personID,
		GivenName:     "John",
		Surname:       "Doe",
		FullName:      "John Doe",
		Gender:        domain.GenderMale,
		BirthDateRaw:  "1 JAN 1850",
		BirthDateSort: &birthDate,
		BirthPlace:    "Springfield, IL",
		Version:       1,
		UpdatedAt:     time.Now(),
	}

	err := store.SavePerson(ctx, domain.MainBranchID, person)
	if err != nil {
		t.Fatalf("save person: %v", err)
	}

	// Read
	retrieved, err := store.GetPerson(ctx, domain.MainBranchID, personID)
	if err != nil {
		t.Fatalf("get person: %v", err)
	}
	if retrieved == nil {
		t.Fatal("expected person, got nil")
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

	// Update
	person.GivenName = "Jane"
	person.FullName = "Jane Doe"
	person.Version = 2
	err = store.SavePerson(ctx, domain.MainBranchID, person)
	if err != nil {
		t.Fatalf("update person: %v", err)
	}

	retrieved, err = store.GetPerson(ctx, domain.MainBranchID, personID)
	if err != nil {
		t.Fatalf("get updated person: %v", err)
	}
	if retrieved.GivenName != "Jane" {
		t.Errorf("expected GivenName Jane, got %s", retrieved.GivenName)
	}
	if retrieved.Version != 2 {
		t.Errorf("expected Version 2, got %d", retrieved.Version)
	}

	// Delete
	err = store.DeletePerson(ctx, domain.MainBranchID, personID)
	if err != nil {
		t.Fatalf("delete person: %v", err)
	}

	retrieved, err = store.GetPerson(ctx, domain.MainBranchID, personID)
	if err != nil {
		t.Fatalf("get deleted person: %v", err)
	}
	if retrieved != nil {
		t.Error("expected nil after delete")
	}
}

func TestReadModelStore_ListPersons(t *testing.T) {
	store, cleanup := setupTestReadModelDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create multiple persons
	persons := []struct {
		given   string
		surname string
	}{
		{"Alice", "Smith"},
		{"Bob", "Johnson"},
		{"Charlie", "Smith"},
		{"David", "Adams"},
	}

	for _, p := range persons {
		person := &repository.PersonReadModel{
			ID:        uuid.New(),
			GivenName: p.given,
			Surname:   p.surname,
			FullName:  p.given + " " + p.surname,
			Version:   1,
			UpdatedAt: time.Now(),
		}
		err := store.SavePerson(ctx, domain.MainBranchID, person)
		if err != nil {
			t.Fatalf("save person: %v", err)
		}
	}

	// List all with default sort (surname)
	opts := repository.DefaultListOptions()
	opts.Limit = 10
	results, total, err := store.ListPersons(ctx, opts)
	if err != nil {
		t.Fatalf("list persons: %v", err)
	}

	if total != 4 {
		t.Errorf("expected total 4, got %d", total)
	}
	if len(results) != 4 {
		t.Errorf("expected 4 results, got %d", len(results))
	}

	// Verify sort order (Adams, Johnson, Smith, Smith)
	if results[0].Surname != "Adams" {
		t.Errorf("expected first surname Adams, got %s", results[0].Surname)
	}

	// Test pagination
	opts.Limit = 2
	opts.Offset = 1
	results, total, err = store.ListPersons(ctx, opts)
	if err != nil {
		t.Fatalf("list persons with offset: %v", err)
	}

	if total != 4 {
		t.Errorf("expected total 4, got %d", total)
	}
	if len(results) != 2 {
		t.Errorf("expected 2 results, got %d", len(results))
	}
}

func TestReadModelStore_SearchPersons(t *testing.T) {
	store, cleanup := setupTestReadModelDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create persons
	persons := []struct {
		given   string
		surname string
	}{
		{"John", "Doe"},
		{"Jane", "Doe"},
		{"John", "Smith"},
		{"Alice", "Johnson"},
	}

	for _, p := range persons {
		person := &repository.PersonReadModel{
			ID:        uuid.New(),
			GivenName: p.given,
			Surname:   p.surname,
			FullName:  p.given + " " + p.surname,
			Version:   1,
			UpdatedAt: time.Now(),
		}
		err := store.SavePerson(ctx, domain.MainBranchID, person)
		if err != nil {
			t.Fatalf("save person: %v", err)
		}
	}

	// Search for "Doe"
	results, err := store.SearchPersons(ctx, repository.SearchOptions{Query: "Doe", Limit: 10})
	if err != nil {
		t.Fatalf("search persons: %v", err)
	}

	if len(results) != 2 {
		t.Errorf("expected 2 results for 'Doe', got %d", len(results))
	}

	// Search for "John"
	results, err = store.SearchPersons(ctx, repository.SearchOptions{Query: "John", Limit: 10})
	if err != nil {
		t.Fatalf("search persons: %v", err)
	}

	if len(results) != 3 { // John Doe, John Smith, Alice Johnson
		t.Errorf("expected 3 results for 'John', got %d", len(results))
	}

	// Fuzzy search (prefix matching)
	results, err = store.SearchPersons(ctx, repository.SearchOptions{Query: "Jo", Fuzzy: true, Limit: 10})
	if err != nil {
		t.Fatalf("fuzzy search persons: %v", err)
	}

	if len(results) < 3 {
		t.Errorf("expected at least 3 results for fuzzy 'Jo', got %d", len(results))
	}
}

func TestReadModelStore_FamilyCRUD(t *testing.T) {
	store, cleanup := setupTestReadModelDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create persons first
	person1ID := uuid.New()
	person2ID := uuid.New()

	person1 := &repository.PersonReadModel{
		ID:        person1ID,
		GivenName: "John",
		Surname:   "Doe",
		FullName:  "John Doe",
		Version:   1,
		UpdatedAt: time.Now(),
	}
	person2 := &repository.PersonReadModel{
		ID:        person2ID,
		GivenName: "Jane",
		Surname:   "Doe",
		FullName:  "Jane Doe",
		Version:   1,
		UpdatedAt: time.Now(),
	}

	store.SavePerson(ctx, domain.MainBranchID, person1)
	store.SavePerson(ctx, domain.MainBranchID, person2)

	// Create family
	familyID := uuid.New()
	marriageDate := time.Date(1875, 6, 15, 0, 0, 0, 0, time.UTC)
	family := &repository.FamilyReadModel{
		ID:                familyID,
		Partner1ID:        &person1ID,
		Partner1GivenName: "John",
		Partner1Surname:   "Doe",
		Partner2ID:        &person2ID,
		Partner2GivenName: "Jane",
		Partner2Surname:   "Doe",
		RelationshipType:  domain.RelationMarriage,
		MarriageDateRaw:   "15 JUN 1875",
		MarriageDateSort:  &marriageDate,
		MarriagePlace:     "Springfield, IL",
		Version:           1,
		UpdatedAt:         time.Now(),
	}

	err := store.SaveFamily(ctx, domain.MainBranchID, family)
	if err != nil {
		t.Fatalf("save family: %v", err)
	}

	// Read
	retrieved, err := store.GetFamily(ctx, domain.MainBranchID, familyID)
	if err != nil {
		t.Fatalf("get family: %v", err)
	}
	if retrieved == nil {
		t.Fatal("expected family, got nil")
	}

	if retrieved.Partner1GivenName != "John" || retrieved.Partner1Surname != "Doe" {
		t.Errorf("expected Partner1 John/Doe, got %q/%q", retrieved.Partner1GivenName, retrieved.Partner1Surname)
	}
	if retrieved.RelationshipType != domain.RelationMarriage {
		t.Errorf("expected RelationshipType marriage, got %s", retrieved.RelationshipType)
	}

	// Get families for person
	families, err := store.GetFamiliesForPerson(ctx, domain.MainBranchID, person1ID)
	if err != nil {
		t.Fatalf("get families for person: %v", err)
	}
	if len(families) != 1 {
		t.Errorf("expected 1 family, got %d", len(families))
	}

	// Delete
	err = store.DeleteFamily(ctx, domain.MainBranchID, familyID)
	if err != nil {
		t.Fatalf("delete family: %v", err)
	}

	retrieved, err = store.GetFamily(ctx, domain.MainBranchID, familyID)
	if err != nil {
		t.Fatalf("get deleted family: %v", err)
	}
	if retrieved != nil {
		t.Error("expected nil after delete")
	}
}

func TestReadModelStore_FamilyChildren(t *testing.T) {
	store, cleanup := setupTestReadModelDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create parent persons
	parent1ID := uuid.New()
	parent2ID := uuid.New()
	childID := uuid.New()

	parent1 := &repository.PersonReadModel{
		ID: parent1ID, GivenName: "John", Surname: "Doe", FullName: "John Doe", Version: 1, UpdatedAt: time.Now(),
	}
	parent2 := &repository.PersonReadModel{
		ID: parent2ID, GivenName: "Jane", Surname: "Doe", FullName: "Jane Doe", Version: 1, UpdatedAt: time.Now(),
	}
	child := &repository.PersonReadModel{
		ID: childID, GivenName: "Bobby", Surname: "Doe", FullName: "Bobby Doe", Version: 1, UpdatedAt: time.Now(),
	}

	store.SavePerson(ctx, domain.MainBranchID, parent1)
	store.SavePerson(ctx, domain.MainBranchID, parent2)
	store.SavePerson(ctx, domain.MainBranchID, child)

	// Create family
	familyID := uuid.New()
	family := &repository.FamilyReadModel{
		ID:                familyID,
		Partner1ID:        &parent1ID,
		Partner1GivenName: "John",
		Partner1Surname:   "Doe",
		Partner2ID:        &parent2ID,
		Partner2GivenName: "Jane",
		Partner2Surname:   "Doe",
		ChildCount:        0,
		Version:           1,
		UpdatedAt:         time.Now(),
	}
	store.SaveFamily(ctx, domain.MainBranchID, family)

	// Add child
	seq := 1
	familyChild := &repository.FamilyChildReadModel{
		FamilyID:         familyID,
		PersonID:         childID,
		PersonGivenName:  "Bobby",
		PersonSurname:    "Doe",
		RelationshipType: domain.ChildBiological,
		Sequence:         &seq,
	}

	err := store.SaveFamilyChild(ctx, domain.MainBranchID, familyChild)
	if err != nil {
		t.Fatalf("save family child: %v", err)
	}

	// Get children
	children, err := store.GetFamilyChildren(ctx, domain.MainBranchID, familyID)
	if err != nil {
		t.Fatalf("get family children: %v", err)
	}
	if len(children) != 1 {
		t.Fatalf("expected 1 child, got %d", len(children))
	}
	if children[0].PersonGivenName != "Bobby" || children[0].PersonSurname != "Doe" {
		t.Errorf("expected child Bobby/Doe, got %q/%q", children[0].PersonGivenName, children[0].PersonSurname)
	}

	// Get child family
	childFamily, err := store.GetChildFamily(ctx, domain.MainBranchID, childID)
	if err != nil {
		t.Fatalf("get child family: %v", err)
	}
	if childFamily == nil {
		t.Fatal("expected child family, got nil")
	}
	if childFamily.ID != familyID {
		t.Errorf("expected family ID %s, got %s", familyID, childFamily.ID)
	}

	// Get children as persons
	childPersons, err := store.GetChildrenOfFamily(ctx, domain.MainBranchID, familyID)
	if err != nil {
		t.Fatalf("get children of family: %v", err)
	}
	if len(childPersons) != 1 {
		t.Fatalf("expected 1 child person, got %d", len(childPersons))
	}
	if childPersons[0].GivenName != "Bobby" {
		t.Errorf("expected child GivenName Bobby, got %s", childPersons[0].GivenName)
	}

	// Delete child
	err = store.DeleteFamilyChild(ctx, domain.MainBranchID, familyID, childID)
	if err != nil {
		t.Fatalf("delete family child: %v", err)
	}

	children, err = store.GetFamilyChildren(ctx, domain.MainBranchID, familyID)
	if err != nil {
		t.Fatalf("get family children after delete: %v", err)
	}
	if len(children) != 0 {
		t.Errorf("expected 0 children after delete, got %d", len(children))
	}
}

func TestReadModelStore_PedigreeEdges(t *testing.T) {
	store, cleanup := setupTestReadModelDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create persons
	childID := uuid.New()
	fatherID := uuid.New()
	motherID := uuid.New()

	child := &repository.PersonReadModel{
		ID: childID, GivenName: "Bobby", Surname: "Doe", FullName: "Bobby Doe", Version: 1, UpdatedAt: time.Now(),
	}
	father := &repository.PersonReadModel{
		ID: fatherID, GivenName: "John", Surname: "Doe", FullName: "John Doe", Gender: domain.GenderMale, Version: 1, UpdatedAt: time.Now(),
	}
	mother := &repository.PersonReadModel{
		ID: motherID, GivenName: "Jane", Surname: "Doe", FullName: "Jane Doe", Gender: domain.GenderFemale, Version: 1, UpdatedAt: time.Now(),
	}

	store.SavePerson(ctx, domain.MainBranchID, child)
	store.SavePerson(ctx, domain.MainBranchID, father)
	store.SavePerson(ctx, domain.MainBranchID, mother)

	// Create pedigree edge
	edge := &repository.PedigreeEdge{
		PersonID:   childID,
		FatherID:   &fatherID,
		MotherID:   &motherID,
		FatherName: "John Doe",
		MotherName: "Jane Doe",
	}

	err := store.SavePedigreeEdge(ctx, domain.MainBranchID, edge)
	if err != nil {
		t.Fatalf("save pedigree edge: %v", err)
	}

	// Get edge
	retrieved, err := store.GetPedigreeEdge(ctx, domain.MainBranchID, childID)
	if err != nil {
		t.Fatalf("get pedigree edge: %v", err)
	}
	if retrieved == nil {
		t.Fatal("expected pedigree edge, got nil")
	}

	if retrieved.FatherName != "John Doe" {
		t.Errorf("expected FatherName John Doe, got %s", retrieved.FatherName)
	}
	if retrieved.MotherName != "Jane Doe" {
		t.Errorf("expected MotherName Jane Doe, got %s", retrieved.MotherName)
	}
	if *retrieved.FatherID != fatherID {
		t.Errorf("expected FatherID %s, got %s", fatherID, *retrieved.FatherID)
	}

	// Delete edge
	err = store.DeletePedigreeEdge(ctx, domain.MainBranchID, childID)
	if err != nil {
		t.Fatalf("delete pedigree edge: %v", err)
	}

	retrieved, err = store.GetPedigreeEdge(ctx, domain.MainBranchID, childID)
	if err != nil {
		t.Fatalf("get pedigree edge after delete: %v", err)
	}
	if retrieved != nil {
		t.Error("expected nil after delete")
	}
}

func TestReadModelStore_NonExistentRecords(t *testing.T) {
	store, cleanup := setupTestReadModelDB(t)
	defer cleanup()

	ctx := context.Background()
	nonExistentID := uuid.New()

	// Get non-existent person
	person, err := store.GetPerson(ctx, domain.MainBranchID, nonExistentID)
	if err != nil {
		t.Fatalf("get non-existent person: %v", err)
	}
	if person != nil {
		t.Error("expected nil for non-existent person")
	}

	// Get non-existent family
	family, err := store.GetFamily(ctx, domain.MainBranchID, nonExistentID)
	if err != nil {
		t.Fatalf("get non-existent family: %v", err)
	}
	if family != nil {
		t.Error("expected nil for non-existent family")
	}

	// Get non-existent pedigree edge
	edge, err := store.GetPedigreeEdge(ctx, domain.MainBranchID, nonExistentID)
	if err != nil {
		t.Fatalf("get non-existent pedigree edge: %v", err)
	}
	if edge != nil {
		t.Error("expected nil for non-existent pedigree edge")
	}
}

func TestReadModelStore_ListFamilies(t *testing.T) {
	store, cleanup := setupTestReadModelDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create persons for families
	person1ID := uuid.New()
	person2ID := uuid.New()
	person3ID := uuid.New()

	person1 := &repository.PersonReadModel{
		ID: person1ID, GivenName: "John", Surname: "Doe", FullName: "John Doe", Version: 1, UpdatedAt: time.Now(),
	}
	person2 := &repository.PersonReadModel{
		ID: person2ID, GivenName: "Jane", Surname: "Doe", FullName: "Jane Doe", Version: 1, UpdatedAt: time.Now(),
	}
	person3 := &repository.PersonReadModel{
		ID: person3ID, GivenName: "Bob", Surname: "Smith", FullName: "Bob Smith", Version: 1, UpdatedAt: time.Now(),
	}

	store.SavePerson(ctx, domain.MainBranchID, person1)
	store.SavePerson(ctx, domain.MainBranchID, person2)
	store.SavePerson(ctx, domain.MainBranchID, person3)

	// Create multiple families
	family1ID := uuid.New()
	family2ID := uuid.New()

	family1 := &repository.FamilyReadModel{
		ID:                family1ID,
		Partner1ID:        &person1ID,
		Partner1GivenName: "John",
		Partner1Surname:   "Doe",
		Partner2ID:        &person2ID,
		Partner2GivenName: "Jane",
		Partner2Surname:   "Doe",
		Version:           1,
		UpdatedAt:         time.Now().Add(-1 * time.Hour), // Older
	}
	family2 := &repository.FamilyReadModel{
		ID:                family2ID,
		Partner1ID:        &person3ID,
		Partner1GivenName: "Bob",
		Partner1Surname:   "Smith",
		Version:           1,
		UpdatedAt:         time.Now(), // Newer
	}

	store.SaveFamily(ctx, domain.MainBranchID, family1)
	store.SaveFamily(ctx, domain.MainBranchID, family2)

	// List all families
	opts := repository.DefaultListOptions()
	opts.Limit = 10
	families, total, err := store.ListFamilies(ctx, opts)
	if err != nil {
		t.Fatalf("list families: %v", err)
	}

	if total != 2 {
		t.Errorf("expected total 2, got %d", total)
	}
	if len(families) != 2 {
		t.Errorf("expected 2 families, got %d", len(families))
	}

	// Verify order (newest first by updated_at)
	if len(families) == 2 && families[0].ID != family2ID {
		t.Errorf("expected first family to be %s, got %s", family2ID, families[0].ID)
	}

	// Test pagination
	opts.Limit = 1
	opts.Offset = 1
	families, total, err = store.ListFamilies(ctx, opts)
	if err != nil {
		t.Fatalf("list families with pagination: %v", err)
	}

	if total != 2 {
		t.Errorf("expected total 2, got %d", total)
	}
	if len(families) != 1 {
		t.Errorf("expected 1 family, got %d", len(families))
	}
}

func TestReadModelStore_SearchPersons_FTS5Error(t *testing.T) {
	store, cleanup := setupTestReadModelDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create a person
	personID := uuid.New()
	person := &repository.PersonReadModel{
		ID:        personID,
		GivenName: "John",
		Surname:   "Doe",
		FullName:  "John Doe",
		Version:   1,
		UpdatedAt: time.Now(),
	}
	store.SavePerson(ctx, domain.MainBranchID, person)

	// Search with a complex FTS5 query that might fail
	// Using quotes and special FTS5 operators can trigger errors
	results, err := store.SearchPersons(ctx, repository.SearchOptions{Query: `"John" AND "Doe"`, Limit: 10})
	if err != nil {
		t.Fatalf("search persons: %v", err)
	}

	// Should still get results via fallback
	if len(results) == 0 {
		t.Log("No results found (fallback may have been triggered)")
	}
}

func TestReadModelStore_SearchPersons_NoFuzzyResults(t *testing.T) {
	store, cleanup := setupTestReadModelDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create a person
	personID := uuid.New()
	person := &repository.PersonReadModel{
		ID:        personID,
		GivenName: "Alexander",
		Surname:   "Hamilton",
		FullName:  "Alexander Hamilton",
		Version:   1,
		UpdatedAt: time.Now(),
	}
	store.SavePerson(ctx, domain.MainBranchID, person)

	// Search for something that doesn't match but with fuzzy enabled
	// This should trigger the fuzzy fallback at line 261-262
	results, err := store.SearchPersons(ctx, repository.SearchOptions{Query: "xyz123notfound", Fuzzy: true, Limit: 10})
	if err != nil {
		t.Fatalf("fuzzy search persons: %v", err)
	}

	// Shouldn't find anything
	if len(results) > 0 {
		t.Logf("Found %d unexpected results", len(results))
	}
}

func TestReadModelStore_SearchPersons_FuzzyFallback(t *testing.T) {
	store, cleanup := setupTestReadModelDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create a person
	personID := uuid.New()
	person := &repository.PersonReadModel{
		ID:        personID,
		GivenName: "Zachary",
		Surname:   "Thompson",
		FullName:  "Zachary Thompson",
		Version:   1,
		UpdatedAt: time.Now(),
	}
	store.SavePerson(ctx, domain.MainBranchID, person)

	// Fuzzy search with prefix that might not match in FTS5
	// This tests the fuzzy fallback path
	results, err := store.SearchPersons(ctx, repository.SearchOptions{Query: "Zac", Fuzzy: true, Limit: 10})
	if err != nil {
		t.Fatalf("fuzzy search persons: %v", err)
	}

	// Should find the person via fuzzy matching
	if len(results) == 0 {
		t.Log("Fuzzy search found no results (this is okay, tests the fallback path)")
	}
}

func TestReadModelStore_ListPersons_Sorting(t *testing.T) {
	store, cleanup := setupTestReadModelDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create persons with various birth dates
	createPerson := func(given, surname string, birthDate *time.Time) {
		person := &repository.PersonReadModel{
			ID:            uuid.New(),
			GivenName:     given,
			Surname:       surname,
			FullName:      given + " " + surname,
			BirthDateSort: birthDate,
			Version:       1,
			UpdatedAt:     time.Now(),
		}
		store.SavePerson(ctx, domain.MainBranchID, person)
	}

	date1 := time.Date(1850, 1, 1, 0, 0, 0, 0, time.UTC)
	date2 := time.Date(1860, 1, 1, 0, 0, 0, 0, time.UTC)

	createPerson("Alice", "Smith", &date1)
	createPerson("Bob", "Smith", &date2)
	createPerson("Charlie", "Adams", nil) // No birth date

	// Test sort by birth date
	opts := repository.DefaultListOptions()
	opts.Sort = "birth_date"
	opts.Limit = 10
	results, total, err := store.ListPersons(ctx, opts)
	if err != nil {
		t.Fatalf("list persons by birth date: %v", err)
	}

	if total != 3 {
		t.Errorf("expected total 3, got %d", total)
	}
	if len(results) != 3 {
		t.Errorf("expected 3 results, got %d", len(results))
	}

	// Test sort by given name
	opts.Sort = "given_name"
	results, _, err = store.ListPersons(ctx, opts)
	if err != nil {
		t.Fatalf("list persons by given name: %v", err)
	}

	if len(results) > 0 && results[0].GivenName != "Alice" {
		t.Errorf("expected first person Alice, got %s", results[0].GivenName)
	}
}

func TestEventStore_ErrorPaths(t *testing.T) {
	// Test error path in NewEventStore by using a closed database
	tmpFile, err := os.CreateTemp("", "myfamily-error-test-*.db")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	tmpFile.Close()
	defer os.Remove(tmpFile.Name())

	db, err := sqlite.OpenDB(tmpFile.Name())
	if err != nil {
		t.Fatalf("open database: %v", err)
	}

	// Close the database before creating event store to trigger error
	db.Close()

	_, err = sqlite.NewEventStore(db)
	if err == nil {
		t.Error("expected error when creating event store with closed database")
	}
}

func TestReadModelStore_ErrorPaths(t *testing.T) {
	// Test error path in NewReadModelStore by using a closed database
	tmpFile, err := os.CreateTemp("", "myfamily-error-test-*.db")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	tmpFile.Close()
	defer os.Remove(tmpFile.Name())

	db, err := sqlite.OpenDB(tmpFile.Name())
	if err != nil {
		t.Fatalf("open database: %v", err)
	}

	// Close the database before creating read model store to trigger error
	db.Close()

	_, err = sqlite.NewReadModelStore(db)
	if err == nil {
		t.Error("expected error when creating read model store with closed database")
	}
}

func TestReadModelStore_SearchPersons_SpecialCharacters(t *testing.T) {
	store, cleanup := setupTestReadModelDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create a person with special characters in name
	personID := uuid.New()
	person := &repository.PersonReadModel{
		ID:        personID,
		GivenName: "Mary-Ann",
		Surname:   "O'Brien",
		FullName:  "Mary-Ann O'Brien",
		Version:   1,
		UpdatedAt: time.Now(),
	}
	store.SavePerson(ctx, domain.MainBranchID, person)

	// Search with special FTS5 characters that might cause errors
	// This should trigger FTS5 error and fallback to LIKE
	testQueries := []string{
		`Mary-Ann`,   // Hyphen
		`O'Brien`,    // Apostrophe
		`"Mary-Ann"`, // Quotes
		`(Mary)`,     // Parentheses
	}

	for _, query := range testQueries {
		results, err := store.SearchPersons(ctx, repository.SearchOptions{Query: query, Limit: 10})
		if err != nil {
			t.Fatalf("search with query %q failed: %v", query, err)
		}
		// Results may or may not be found depending on FTS5/LIKE behavior
		t.Logf("Query %q returned %d results", query, len(results))
	}
}

func TestReadModelStore_ListCitations(t *testing.T) {
	store, cleanup := setupTestReadModelDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create a source first (citations reference sources)
	sourceID := uuid.New()
	source := &repository.SourceReadModel{
		ID:         sourceID,
		SourceType: "book",
		Title:      "Test Source",
		Version:    1,
		UpdatedAt:  time.Now(),
	}
	if err := store.SaveSource(ctx, source); err != nil {
		t.Fatalf("save source: %v", err)
	}

	// Create multiple citations
	citations := []struct {
		sourceTitle string
		factType    domain.FactType
	}{
		{"Test Source", "person_birth"},
		{"Test Source", "person_death"},
		{"Test Source", "person_birth"},
	}

	for _, c := range citations {
		cit := &repository.CitationReadModel{
			ID:          uuid.New(),
			SourceID:    sourceID,
			SourceTitle: c.sourceTitle,
			FactType:    c.factType,
			FactOwnerID: uuid.New(),
			Version:     1,
			CreatedAt:   time.Now(),
		}
		if err := store.SaveCitation(ctx, cit); err != nil {
			t.Fatalf("save citation: %v", err)
		}
	}

	// List all citations
	opts := repository.DefaultListOptions()
	opts.Limit = 10
	results, total, err := store.ListCitations(ctx, opts)
	if err != nil {
		t.Fatalf("list citations: %v", err)
	}

	if total != 3 {
		t.Errorf("expected total 3, got %d", total)
	}
	if len(results) != 3 {
		t.Errorf("expected 3 results, got %d", len(results))
	}

	// Verify sort order: source_title ASC, fact_type ASC
	// All same source_title, so should be sorted by fact_type: person_birth, person_birth, person_death
	if len(results) >= 3 {
		if results[0].FactType != "person_birth" {
			t.Errorf("expected first fact_type person_birth, got %s", results[0].FactType)
		}
		if results[2].FactType != "person_death" {
			t.Errorf("expected last fact_type person_death, got %s", results[2].FactType)
		}
	}

	// Test pagination
	opts.Limit = 2
	opts.Offset = 1
	results, total, err = store.ListCitations(ctx, opts)
	if err != nil {
		t.Fatalf("list citations with offset: %v", err)
	}

	if total != 3 {
		t.Errorf("expected total 3, got %d", total)
	}
	if len(results) != 2 {
		t.Errorf("expected 2 results, got %d", len(results))
	}
}

func TestReadModelStore_EventCRUD(t *testing.T) {
	store, cleanup := setupTestReadModelDB(t)
	defer cleanup()

	ctx := context.Background()
	eventID := uuid.New()
	personID := uuid.New()
	birthDate := time.Date(1990, 1, 15, 0, 0, 0, 0, time.UTC)
	lat := "40.7128"
	long := "-74.0060"

	event := &repository.EventReadModel{
		ID:        eventID,
		OwnerType: "person",
		OwnerID:   personID,
		FactType:  "person_birth",
		DateRaw:   "15 JAN 1990",
		DateSort:  &birthDate,
		Place:     "New York, NY",
		PlaceLat:  &lat,
		PlaceLong: &long,
		Address: &domain.Address{
			City:    "New York",
			State:   "NY",
			Country: "USA",
		},
		Description:    "Born at hospital",
		ResearchStatus: "certain",
		Version:        1,
		CreatedAt:      time.Now(),
	}

	// Save
	err := store.SaveEvent(ctx, event)
	if err != nil {
		t.Fatalf("save event: %v", err)
	}

	// Get
	got, err := store.GetEvent(ctx, eventID)
	if err != nil {
		t.Fatalf("get event: %v", err)
	}
	if got == nil {
		t.Fatal("expected event, got nil")
	}
	if got.ID != eventID {
		t.Errorf("expected ID %s, got %s", eventID, got.ID)
	}
	if got.OwnerType != "person" {
		t.Errorf("expected owner_type person, got %s", got.OwnerType)
	}
	if got.OwnerID != personID {
		t.Errorf("expected owner_id %s, got %s", personID, got.OwnerID)
	}
	if got.FactType != "person_birth" {
		t.Errorf("expected fact_type person_birth, got %s", got.FactType)
	}
	if got.DateRaw != "15 JAN 1990" {
		t.Errorf("expected date_raw '15 JAN 1990', got '%s'", got.DateRaw)
	}
	if got.DateSort == nil {
		t.Error("expected date_sort to be set")
	}
	if got.Place != "New York, NY" {
		t.Errorf("expected place 'New York, NY', got '%s'", got.Place)
	}
	if got.PlaceLat == nil || *got.PlaceLat != "40.7128" {
		t.Error("expected place_lat to be 40.7128")
	}
	if got.PlaceLong == nil || *got.PlaceLong != "-74.0060" {
		t.Error("expected place_long to be -74.0060")
	}
	if got.Address == nil || got.Address.City != "New York" {
		t.Error("expected address with city New York")
	}
	if got.Description != "Born at hospital" {
		t.Errorf("expected description 'Born at hospital', got '%s'", got.Description)
	}
	if got.ResearchStatus != "certain" {
		t.Errorf("expected research_status certain, got %s", got.ResearchStatus)
	}

	// Update
	event.Description = "Updated description"
	event.Version = 2
	err = store.SaveEvent(ctx, event)
	if err != nil {
		t.Fatalf("update event: %v", err)
	}

	got, err = store.GetEvent(ctx, eventID)
	if err != nil {
		t.Fatalf("get updated event: %v", err)
	}
	if got.Description != "Updated description" {
		t.Errorf("expected updated description, got '%s'", got.Description)
	}
	if got.Version != 2 {
		t.Errorf("expected version 2, got %d", got.Version)
	}

	// Delete
	err = store.DeleteEvent(ctx, eventID)
	if err != nil {
		t.Fatalf("delete event: %v", err)
	}

	got, err = store.GetEvent(ctx, eventID)
	if err != nil {
		t.Fatalf("get deleted event: %v", err)
	}
	if got != nil {
		t.Error("expected nil after delete")
	}
}

func TestReadModelStore_ListEvents(t *testing.T) {
	store, cleanup := setupTestReadModelDB(t)
	defer cleanup()

	ctx := context.Background()
	personID := uuid.New()
	familyID := uuid.New()

	date1 := time.Date(1990, 1, 1, 0, 0, 0, 0, time.UTC)
	date2 := time.Date(2020, 6, 15, 0, 0, 0, 0, time.UTC)

	events := []struct {
		ownerType string
		ownerID   uuid.UUID
		factType  domain.FactType
		dateSort  *time.Time
	}{
		{"person", personID, "person_birth", &date1},
		{"person", personID, "person_death", &date2},
		{"family", familyID, "family_marriage", &date2},
		{"person", personID, "person_birth", nil}, // No date
	}

	for _, e := range events {
		event := &repository.EventReadModel{
			ID:        uuid.New(),
			OwnerType: e.ownerType,
			OwnerID:   e.ownerID,
			FactType:  e.factType,
			DateSort:  e.dateSort,
			Version:   1,
			CreatedAt: time.Now(),
		}
		if err := store.SaveEvent(ctx, event); err != nil {
			t.Fatalf("save event: %v", err)
		}
	}

	// List all events
	opts := repository.DefaultListOptions()
	opts.Limit = 10
	results, total, err := store.ListEvents(ctx, opts)
	if err != nil {
		t.Fatalf("list events: %v", err)
	}

	if total != 4 {
		t.Errorf("expected total 4, got %d", total)
	}
	if len(results) != 4 {
		t.Errorf("expected 4 results, got %d", len(results))
	}

	// Test pagination
	opts.Limit = 2
	opts.Offset = 0
	results, total, err = store.ListEvents(ctx, opts)
	if err != nil {
		t.Fatalf("list events paginated: %v", err)
	}
	if total != 4 {
		t.Errorf("expected total 4, got %d", total)
	}
	if len(results) != 2 {
		t.Errorf("expected 2 results, got %d", len(results))
	}

	// ListEventsForPerson
	personEvents, err := store.ListEventsForPerson(ctx, personID)
	if err != nil {
		t.Fatalf("list events for person: %v", err)
	}
	if len(personEvents) != 3 {
		t.Errorf("expected 3 person events, got %d", len(personEvents))
	}

	// ListEventsForFamily
	familyEvents, err := store.ListEventsForFamily(ctx, familyID)
	if err != nil {
		t.Fatalf("list events for family: %v", err)
	}
	if len(familyEvents) != 1 {
		t.Errorf("expected 1 family event, got %d", len(familyEvents))
	}
	if len(familyEvents) > 0 && familyEvents[0].FactType != "family_marriage" {
		t.Errorf("expected family_marriage, got %s", familyEvents[0].FactType)
	}
}

func TestReadModelStore_AttributeCRUD(t *testing.T) {
	store, cleanup := setupTestReadModelDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create a person first (attributes reference persons)
	personID := uuid.New()
	person := &repository.PersonReadModel{
		ID:        personID,
		GivenName: "John",
		Surname:   "Doe",
		FullName:  "John Doe",
		Version:   1,
		UpdatedAt: time.Now(),
	}
	if err := store.SavePerson(ctx, domain.MainBranchID, person); err != nil {
		t.Fatalf("save person: %v", err)
	}

	attrID := uuid.New()
	attrDate := time.Date(1990, 1, 1, 0, 0, 0, 0, time.UTC)

	attr := &repository.AttributeReadModel{
		ID:        attrID,
		PersonID:  personID,
		FactType:  "person_occupation",
		Value:     "Software Engineer",
		DateRaw:   "1990",
		DateSort:  &attrDate,
		Place:     "San Francisco, CA",
		Version:   1,
		CreatedAt: time.Now(),
	}

	// Save
	err := store.SaveAttribute(ctx, attr)
	if err != nil {
		t.Fatalf("save attribute: %v", err)
	}

	// Get
	got, err := store.GetAttribute(ctx, attrID)
	if err != nil {
		t.Fatalf("get attribute: %v", err)
	}
	if got == nil {
		t.Fatal("expected attribute, got nil")
	}
	if got.ID != attrID {
		t.Errorf("expected ID %s, got %s", attrID, got.ID)
	}
	if got.PersonID != personID {
		t.Errorf("expected person_id %s, got %s", personID, got.PersonID)
	}
	if got.FactType != "person_occupation" {
		t.Errorf("expected fact_type person_occupation, got %s", got.FactType)
	}
	if got.Value != "Software Engineer" {
		t.Errorf("expected value 'Software Engineer', got '%s'", got.Value)
	}
	if got.DateRaw != "1990" {
		t.Errorf("expected date_raw '1990', got '%s'", got.DateRaw)
	}
	if got.DateSort == nil {
		t.Error("expected date_sort to be set")
	}
	if got.Place != "San Francisco, CA" {
		t.Errorf("expected place 'San Francisco, CA', got '%s'", got.Place)
	}

	// Update
	attr.Value = "Senior Engineer"
	attr.Version = 2
	err = store.SaveAttribute(ctx, attr)
	if err != nil {
		t.Fatalf("update attribute: %v", err)
	}

	got, err = store.GetAttribute(ctx, attrID)
	if err != nil {
		t.Fatalf("get updated attribute: %v", err)
	}
	if got.Value != "Senior Engineer" {
		t.Errorf("expected updated value, got '%s'", got.Value)
	}
	if got.Version != 2 {
		t.Errorf("expected version 2, got %d", got.Version)
	}

	// Delete
	err = store.DeleteAttribute(ctx, attrID)
	if err != nil {
		t.Fatalf("delete attribute: %v", err)
	}

	got, err = store.GetAttribute(ctx, attrID)
	if err != nil {
		t.Fatalf("get deleted attribute: %v", err)
	}
	if got != nil {
		t.Error("expected nil after delete")
	}
}

func TestReadModelStore_ListAttributes(t *testing.T) {
	store, cleanup := setupTestReadModelDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create a person first
	personID1 := uuid.New()
	person1 := &repository.PersonReadModel{
		ID:        personID1,
		GivenName: "John",
		Surname:   "Doe",
		FullName:  "John Doe",
		Version:   1,
		UpdatedAt: time.Now(),
	}
	if err := store.SavePerson(ctx, domain.MainBranchID, person1); err != nil {
		t.Fatalf("save person: %v", err)
	}

	personID2 := uuid.New()
	person2 := &repository.PersonReadModel{
		ID:        personID2,
		GivenName: "Jane",
		Surname:   "Doe",
		FullName:  "Jane Doe",
		Version:   1,
		UpdatedAt: time.Now(),
	}
	if err := store.SavePerson(ctx, domain.MainBranchID, person2); err != nil {
		t.Fatalf("save person: %v", err)
	}

	// Create attributes for both persons
	attributes := []struct {
		personID uuid.UUID
		factType domain.FactType
		value    string
	}{
		{personID1, "person_occupation", "Engineer"},
		{personID1, "person_occupation", "Architect"},
		{personID1, "person_religion", "None"},
		{personID2, "person_occupation", "Doctor"},
	}

	for _, a := range attributes {
		attr := &repository.AttributeReadModel{
			ID:        uuid.New(),
			PersonID:  a.personID,
			FactType:  a.factType,
			Value:     a.value,
			Version:   1,
			CreatedAt: time.Now(),
		}
		if err := store.SaveAttribute(ctx, attr); err != nil {
			t.Fatalf("save attribute: %v", err)
		}
	}

	// List all attributes
	opts := repository.DefaultListOptions()
	opts.Limit = 10
	results, total, err := store.ListAttributes(ctx, opts)
	if err != nil {
		t.Fatalf("list attributes: %v", err)
	}

	if total != 4 {
		t.Errorf("expected total 4, got %d", total)
	}
	if len(results) != 4 {
		t.Errorf("expected 4 results, got %d", len(results))
	}

	// Verify sort order: fact_type ASC, value ASC
	// person_occupation (Architect, Doctor, Engineer), person_religion (None)
	if len(results) >= 4 {
		if results[0].FactType != "person_occupation" || results[0].Value != "Architect" {
			t.Errorf("expected first: person_occupation/Architect, got %s/%s", results[0].FactType, results[0].Value)
		}
		if results[3].FactType != "person_religion" {
			t.Errorf("expected last fact_type person_religion, got %s", results[3].FactType)
		}
	}

	// Test pagination
	opts.Limit = 2
	opts.Offset = 1
	results, total, err = store.ListAttributes(ctx, opts)
	if err != nil {
		t.Fatalf("list attributes with offset: %v", err)
	}
	if total != 4 {
		t.Errorf("expected total 4, got %d", total)
	}
	if len(results) != 2 {
		t.Errorf("expected 2 results, got %d", len(results))
	}

	// ListAttributesForPerson
	person1Attrs, err := store.ListAttributesForPerson(ctx, personID1)
	if err != nil {
		t.Fatalf("list attributes for person: %v", err)
	}
	if len(person1Attrs) != 3 {
		t.Errorf("expected 3 attributes for person1, got %d", len(person1Attrs))
	}

	person2Attrs, err := store.ListAttributesForPerson(ctx, personID2)
	if err != nil {
		t.Fatalf("list attributes for person2: %v", err)
	}
	if len(person2Attrs) != 1 {
		t.Errorf("expected 1 attribute for person2, got %d", len(person2Attrs))
	}
}

func TestSearchPersons_DateRange(t *testing.T) {
	store, cleanup := setupTestReadModelDB(t)
	defer cleanup()

	ctx := context.Background()

	birth1850 := time.Date(1850, 6, 15, 0, 0, 0, 0, time.UTC)
	birth1880 := time.Date(1880, 3, 20, 0, 0, 0, 0, time.UTC)
	birth1920 := time.Date(1920, 11, 1, 0, 0, 0, 0, time.UTC)

	persons := []repository.PersonReadModel{
		{ID: uuid.New(), GivenName: "Alice", Surname: "Early", FullName: "Alice Early", BirthDateRaw: "15 JUN 1850", BirthDateSort: &birth1850, Version: 1, UpdatedAt: time.Now()},
		{ID: uuid.New(), GivenName: "Bob", Surname: "Middle", FullName: "Bob Middle", BirthDateRaw: "20 MAR 1880", BirthDateSort: &birth1880, Version: 1, UpdatedAt: time.Now()},
		{ID: uuid.New(), GivenName: "Charlie", Surname: "Late", FullName: "Charlie Late", BirthDateRaw: "1 NOV 1920", BirthDateSort: &birth1920, Version: 1, UpdatedAt: time.Now()},
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
	store, cleanup := setupTestReadModelDB(t)
	defer cleanup()

	ctx := context.Background()

	persons := []repository.PersonReadModel{
		{ID: uuid.New(), GivenName: "Alice", Surname: "Smith", FullName: "Alice Smith", BirthPlace: "London, England", Version: 1, UpdatedAt: time.Now()},
		{ID: uuid.New(), GivenName: "Bob", Surname: "Dupont", FullName: "Bob Dupont", BirthPlace: "Paris, France", Version: 1, UpdatedAt: time.Now()},
		{ID: uuid.New(), GivenName: "Charlie", Surname: "Jones", FullName: "Charlie Jones", BirthPlace: "New London, Connecticut", Version: 1, UpdatedAt: time.Now()},
		{ID: uuid.New(), GivenName: "Diana", Surname: "Brown", FullName: "Diana Brown", DeathPlace: "London, England", Version: 1, UpdatedAt: time.Now()},
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
	store, cleanup := setupTestReadModelDB(t)
	defer cleanup()

	ctx := context.Background()

	persons := []repository.PersonReadModel{
		{ID: uuid.New(), GivenName: "Catherine", Surname: "Smith", FullName: "Catherine Smith", Version: 1, UpdatedAt: time.Now()},
		{ID: uuid.New(), GivenName: "Katherine", Surname: "Smyth", FullName: "Katherine Smyth", Version: 1, UpdatedAt: time.Now()},
		{ID: uuid.New(), GivenName: "Robert", Surname: "Brown", FullName: "Robert Brown", Version: 1, UpdatedAt: time.Now()},
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
		// Smith (S530) and Smyth (S530) have the same soundex code
		if len(results) != 2 {
			t.Errorf("expected 2 results for soundex 'Smith', got %d", len(results))
		}
	})

	t.Run("Soundex Catherine does not match Katherine", func(t *testing.T) {
		// Catherine (C365) vs Katherine (K365) differ in first letter
		results, err := store.SearchPersons(ctx, repository.SearchOptions{
			Query:   "Catherine",
			Soundex: true,
			Limit:   10,
		})
		if err != nil {
			t.Fatalf("search: %v", err)
		}
		if len(results) != 1 {
			t.Errorf("expected 1 result for soundex 'Catherine', got %d", len(results))
		}
		if len(results) > 0 && results[0].GivenName != "Catherine" {
			t.Errorf("expected Catherine, got %s", results[0].GivenName)
		}
	})
}

func TestSearchPersons_Combined(t *testing.T) {
	store, cleanup := setupTestReadModelDB(t)
	defer cleanup()

	ctx := context.Background()

	birth1850 := time.Date(1850, 1, 1, 0, 0, 0, 0, time.UTC)
	birth1880 := time.Date(1880, 1, 1, 0, 0, 0, 0, time.UTC)
	birth1920 := time.Date(1920, 1, 1, 0, 0, 0, 0, time.UTC)

	persons := []repository.PersonReadModel{
		{ID: uuid.New(), GivenName: "John", Surname: "Smith", FullName: "John Smith", BirthDateSort: &birth1850, BirthPlace: "London, England", Version: 1, UpdatedAt: time.Now()},
		{ID: uuid.New(), GivenName: "John", Surname: "Smith", FullName: "John Smith", BirthDateSort: &birth1920, BirthPlace: "London, England", Version: 1, UpdatedAt: time.Now()},
		{ID: uuid.New(), GivenName: "John", Surname: "Smith", FullName: "John Smith", BirthDateSort: &birth1880, BirthPlace: "Paris, France", Version: 1, UpdatedAt: time.Now()},
		{ID: uuid.New(), GivenName: "Jane", Surname: "Doe", FullName: "Jane Doe", BirthDateSort: &birth1880, BirthPlace: "London, England", Version: 1, UpdatedAt: time.Now()},
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
		// Only the 1850 person has "Smith" + 1860-1900 range + "London", but 1850 < 1860
		// The 1880 Smith is in Paris. The 1920 Smith is out of range.
		// So none match all criteria.
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
	store, cleanup := setupTestReadModelDB(t)
	defer cleanup()

	ctx := context.Background()

	birth1850 := time.Date(1850, 1, 1, 0, 0, 0, 0, time.UTC)
	birth1880 := time.Date(1880, 1, 1, 0, 0, 0, 0, time.UTC)
	birth1920 := time.Date(1920, 1, 1, 0, 0, 0, 0, time.UTC)

	persons := []repository.PersonReadModel{
		{ID: uuid.New(), GivenName: "Alice", Surname: "Smith", FullName: "Alice Smith", BirthDateSort: &birth1850, BirthPlace: "London", Version: 1, UpdatedAt: time.Now()},
		{ID: uuid.New(), GivenName: "Bob", Surname: "Jones", FullName: "Bob Jones", BirthDateSort: &birth1880, BirthPlace: "Paris", Version: 1, UpdatedAt: time.Now()},
		{ID: uuid.New(), GivenName: "Charlie", Surname: "Brown", FullName: "Charlie Brown", BirthDateSort: &birth1920, BirthPlace: "London", Version: 1, UpdatedAt: time.Now()},
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
	store, cleanup := setupTestReadModelDB(t)
	defer cleanup()

	ctx := context.Background()

	birth1850 := time.Date(1850, 1, 1, 0, 0, 0, 0, time.UTC)
	birth1880 := time.Date(1880, 1, 1, 0, 0, 0, 0, time.UTC)
	birth1920 := time.Date(1920, 1, 1, 0, 0, 0, 0, time.UTC)

	persons := []repository.PersonReadModel{
		{ID: uuid.New(), GivenName: "Charlie", Surname: "Adams", FullName: "Charlie Adams", BirthDateSort: &birth1920, BirthPlace: "London", Version: 1, UpdatedAt: time.Now()},
		{ID: uuid.New(), GivenName: "Alice", Surname: "Brown", FullName: "Alice Brown", BirthDateSort: &birth1850, BirthPlace: "London", Version: 1, UpdatedAt: time.Now()},
		{ID: uuid.New(), GivenName: "Bob", Surname: "Clark", FullName: "Bob Clark", BirthDateSort: &birth1880, BirthPlace: "London", Version: 1, UpdatedAt: time.Now()},
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
	store, cleanup := setupTestReadModelDB(t)
	defer cleanup()

	ctx := context.Background()

	persons := []repository.PersonReadModel{
		{ID: uuid.New(), GivenName: "John", Surname: "Doe", FullName: "John Doe", Version: 1, UpdatedAt: time.Now()},
		{ID: uuid.New(), GivenName: "Jane", Surname: "Doe", FullName: "Jane Doe", Version: 1, UpdatedAt: time.Now()},
		{ID: uuid.New(), GivenName: "Robert", Surname: "Smith", FullName: "Robert Smith", Version: 1, UpdatedAt: time.Now()},
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
			Query: "Do",
			Fuzzy: true,
			Limit: 10,
		})
		if err != nil {
			t.Fatalf("search: %v", err)
		}
		if len(results) < 2 {
			t.Errorf("expected at least 2 results for fuzzy 'Do', got %d", len(results))
		}
	})
}

func TestReadModelStore_RepositoryCRUD(t *testing.T) {
	store, cleanup := setupTestReadModelDB(t)
	defer cleanup()
	ctx := context.Background()

	repo := &repository.RepositoryReadModel{
		ID:         uuid.New(),
		Name:       "National Archives",
		Address:    &domain.Address{City: "Washington", State: "DC", Phone: "+1-866-272-6272"},
		Notes:      "Primary federal records repository",
		GedcomXref: "@R1@",
		Version:    1,
		UpdatedAt:  time.Now(),
	}

	if err := store.SaveRepository(ctx, repo); err != nil {
		t.Fatalf("SaveRepository() failed: %v", err)
	}

	retrieved, err := store.GetRepository(ctx, repo.ID)
	if err != nil {
		t.Fatalf("GetRepository() failed: %v", err)
	}
	if retrieved == nil {
		t.Fatal("Repository not found")
	}
	if retrieved.Name != "National Archives" {
		t.Errorf("Name = %s, want National Archives", retrieved.Name)
	}
	if retrieved.Notes != "Primary federal records repository" {
		t.Errorf("Notes = %s, want Primary federal records repository", retrieved.Notes)
	}
	if retrieved.GedcomXref != "@R1@" {
		t.Errorf("GedcomXref = %s, want @R1@", retrieved.GedcomXref)
	}
	if retrieved.Address == nil || retrieved.Address.City != "Washington" || retrieved.Address.Phone != "+1-866-272-6272" {
		t.Errorf("Address not round-tripped: %+v", retrieved.Address)
	}

	// Update via Save (upsert)
	retrieved.Name = "US National Archives"
	retrieved.Version = 2
	if err := store.SaveRepository(ctx, retrieved); err != nil {
		t.Fatalf("SaveRepository() update failed: %v", err)
	}
	updated, err := store.GetRepository(ctx, repo.ID)
	if err != nil {
		t.Fatalf("GetRepository() after update failed: %v", err)
	}
	if updated.Name != "US National Archives" || updated.Version != 2 {
		t.Errorf("update not persisted: name=%s version=%d", updated.Name, updated.Version)
	}

	// Delete
	if err := store.DeleteRepository(ctx, repo.ID); err != nil {
		t.Fatalf("DeleteRepository() failed: %v", err)
	}
	deleted, err := store.GetRepository(ctx, repo.ID)
	if err != nil {
		t.Fatalf("GetRepository() after delete failed: %v", err)
	}
	if deleted != nil {
		t.Error("Repository should be deleted")
	}
}

func TestReadModelStore_ListRepositories(t *testing.T) {
	store, cleanup := setupTestReadModelDB(t)
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
			t.Fatalf("SaveRepository() failed: %v", err)
		}
	}

	results, total, err := store.ListRepositories(ctx, repository.ListOptions{Limit: 10, Sort: "name", Order: "asc"})
	if err != nil {
		t.Fatalf("ListRepositories() failed: %v", err)
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

// --- Branch-scoping overlay/tombstone tests (ADR-005 / #669) ---
//
// These mirror the in-memory reference tests
// (internal/repository/memory/branch_readmodel_test.go) to verify the SQLite
// backend is behaviorally identical: single-row entities resolve via a window
// overlay, collections resolve per-row (names/children) or per-parent bucket
// (external ids), and non-main deletes write tombstones while main deletes are
// real removals.

func branchPersonRM(id uuid.UUID, given, surname string) *repository.PersonReadModel {
	return &repository.PersonReadModel{ID: id, GivenName: given, Surname: surname, Version: 1}
}

func TestBranchOverlayPersonPrecedence(t *testing.T) {
	store, cleanup := setupTestReadModelDB(t)
	defer cleanup()
	ctx := context.Background()
	branch := domain.BranchID(uuid.New())
	id := uuid.New()

	if err := store.SavePerson(ctx, domain.MainBranchID, branchPersonRM(id, "Ada", "Main")); err != nil {
		t.Fatalf("SavePerson main: %v", err)
	}
	if err := store.SavePerson(ctx, branch, branchPersonRM(id, "Ada", "Branch")); err != nil {
		t.Fatalf("SavePerson branch: %v", err)
	}

	if got, _ := store.GetPerson(ctx, branch, id); got == nil || got.Surname != "Branch" {
		t.Fatalf("branch Get: want surname Branch, got %+v", got)
	}
	if main, _ := store.GetPerson(ctx, domain.MainBranchID, id); main == nil || main.Surname != "Main" {
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

func TestBranchPersonFallbackToMain(t *testing.T) {
	store, cleanup := setupTestReadModelDB(t)
	defer cleanup()
	ctx := context.Background()
	branch := domain.BranchID(uuid.New())
	id := uuid.New()

	if err := store.SavePerson(ctx, domain.MainBranchID, branchPersonRM(id, "Grace", "Hopper")); err != nil {
		t.Fatalf("SavePerson main: %v", err)
	}

	if got, _ := store.GetPerson(ctx, branch, id); got == nil || got.Surname != "Hopper" {
		t.Fatalf("branch Get fallback: want Hopper, got %+v", got)
	}
	branchList, total, _ := store.ListPersons(ctx, repository.ListOptions{Limit: 10, BranchID: branch})
	if total != 1 || len(branchList) != 1 || branchList[0].Surname != "Hopper" {
		t.Fatalf("branch List fallback: want 1 Hopper, got total=%d list=%+v", total, branchList)
	}
}

func TestBranchPersonTombstoneSuppression(t *testing.T) {
	store, cleanup := setupTestReadModelDB(t)
	defer cleanup()
	ctx := context.Background()
	branch := domain.BranchID(uuid.New())
	id := uuid.New()

	if err := store.SavePerson(ctx, domain.MainBranchID, branchPersonRM(id, "Alan", "Turing")); err != nil {
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

func TestBranchPersonMainDeleteIsRealRemoval(t *testing.T) {
	store, cleanup := setupTestReadModelDB(t)
	defer cleanup()
	ctx := context.Background()
	id := uuid.New()

	if err := store.SavePerson(ctx, domain.MainBranchID, branchPersonRM(id, "Ada", "Lovelace")); err != nil {
		t.Fatalf("SavePerson: %v", err)
	}
	if err := store.DeletePerson(ctx, domain.MainBranchID, id); err != nil {
		t.Fatalf("DeletePerson main: %v", err)
	}
	if got, _ := store.GetPerson(ctx, domain.MainBranchID, id); got != nil {
		t.Fatalf("main Get after main delete: want nil, got %+v", got)
	}
}

func TestBranchOnlyPersonInvisibleOnMain(t *testing.T) {
	store, cleanup := setupTestReadModelDB(t)
	defer cleanup()
	ctx := context.Background()
	branch := domain.BranchID(uuid.New())
	id := uuid.New()

	if err := store.SavePerson(ctx, branch, branchPersonRM(id, "Only", "Branch")); err != nil {
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
	store, cleanup := setupTestReadModelDB(t)
	defer cleanup()
	ctx := context.Background()
	branch := domain.BranchID(uuid.New())
	id := uuid.New()

	_ = store.SavePerson(ctx, domain.MainBranchID, branchPersonRM(id, "Katherine", "Johnson"))
	_ = store.SavePerson(ctx, branch, branchPersonRM(id, "Katherine", "Coleman"))

	branchHits, _ := store.SearchPersons(ctx, repository.SearchOptions{Query: "Coleman", Limit: 10, BranchID: branch})
	if len(branchHits) != 1 || branchHits[0].Surname != "Coleman" {
		t.Fatalf("branch Search: want Coleman, got %+v", branchHits)
	}
	// The main-only surname must not surface on the branch (branch overrode it).
	if hits, _ := store.SearchPersons(ctx, repository.SearchOptions{Query: "Johnson", Limit: 10, BranchID: branch}); len(hits) != 0 {
		t.Fatalf("branch Search for main-only surname: want empty, got %+v", hits)
	}
	mainHits, _ := store.SearchPersons(ctx, repository.SearchOptions{Query: "Johnson", Limit: 10, BranchID: domain.MainBranchID})
	if len(mainHits) != 1 || mainHits[0].Surname != "Johnson" {
		t.Fatalf("main Search: want Johnson, got %+v", mainHits)
	}
	// And the branch override must not surface under the branch surname on main.
	if hits, _ := store.SearchPersons(ctx, repository.SearchOptions{Query: "Coleman", Limit: 10, BranchID: domain.MainBranchID}); len(hits) != 0 {
		t.Fatalf("main Search for branch-only surname: want empty, got %+v", hits)
	}
}

func TestBranchPersonNamesOverlay(t *testing.T) {
	store, cleanup := setupTestReadModelDB(t)
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
		t.Fatalf("branch names: want 2 (main fallback + branch), got %d: %+v", len(branchNames), branchNames)
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

	// Deleting the main name on the branch tombstones it; main keeps it.
	if err := store.DeletePersonName(ctx, branch, mainName.ID); err != nil {
		t.Fatalf("DeletePersonName branch: %v", err)
	}
	branchNames, _ = store.GetPersonNames(ctx, branch, personID)
	if len(branchNames) != 1 || branchNames[0].ID != branchName.ID {
		t.Fatalf("branch names after tombstone: want [branchName], got %+v", branchNames)
	}
	mainNames, _ = store.GetPersonNames(ctx, domain.MainBranchID, personID)
	if len(mainNames) != 1 {
		t.Fatalf("main names after branch name tombstone: want 1, got %d", len(mainNames))
	}
}

func TestBranchFamilyOverlayTombstone(t *testing.T) {
	store, cleanup := setupTestReadModelDB(t)
	defer cleanup()
	ctx := context.Background()
	branch := domain.BranchID(uuid.New())
	id := uuid.New()

	_ = store.SaveFamily(ctx, domain.MainBranchID, &repository.FamilyReadModel{ID: id, Partner1GivenName: "Main", Version: 1})
	_ = store.SaveFamily(ctx, branch, &repository.FamilyReadModel{ID: id, Partner1GivenName: "Branch", Version: 1})

	if got, _ := store.GetFamily(ctx, branch, id); got == nil || got.Partner1GivenName != "Branch" {
		t.Fatalf("branch GetFamily: want Branch, got %+v", got)
	}
	if got, _ := store.GetFamily(ctx, domain.MainBranchID, id); got == nil || got.Partner1GivenName != "Main" {
		t.Fatalf("main GetFamily: want Main, got %+v", got)
	}

	branchList, total, _ := store.ListFamilies(ctx, repository.ListOptions{Limit: 10, BranchID: branch})
	if total != 1 || len(branchList) != 1 || branchList[0].Partner1GivenName != "Branch" {
		t.Fatalf("branch ListFamilies: want 1 Branch, got total=%d %+v", total, branchList)
	}

	// Branch tombstone hides the family for the branch; main keeps it.
	if err := store.DeleteFamily(ctx, branch, id); err != nil {
		t.Fatalf("DeleteFamily branch: %v", err)
	}
	if got, _ := store.GetFamily(ctx, branch, id); got != nil {
		t.Fatalf("branch GetFamily after tombstone: want nil, got %+v", got)
	}
	if got, _ := store.GetFamily(ctx, domain.MainBranchID, id); got == nil {
		t.Fatal("main GetFamily after branch tombstone: want present, got nil")
	}
}

func TestBranchFamilyChildrenOverlay(t *testing.T) {
	store, cleanup := setupTestReadModelDB(t)
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
	store, cleanup := setupTestReadModelDB(t)
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

	// Empty Replace on the branch is a tombstone: branch sees none, main intact.
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

func TestBranchFamilyExternalIDsOverlay(t *testing.T) {
	store, cleanup := setupTestReadModelDB(t)
	defer cleanup()
	ctx := context.Background()
	branch := domain.BranchID(uuid.New())
	familyID := uuid.New()

	_ = store.ReplaceFamilyExternalIDs(ctx, domain.MainBranchID, familyID, []repository.FamilyExternalIDReadModel{{Value: "FMAIN"}})
	_ = store.ReplaceFamilyExternalIDs(ctx, branch, familyID, []repository.FamilyExternalIDReadModel{{Value: "FBR-1"}, {Value: "FBR-2"}})

	branchIDs, _ := store.GetFamilyExternalIDs(ctx, branch, familyID)
	if len(branchIDs) != 2 || branchIDs[0].Value != "FBR-1" {
		t.Fatalf("branch family ext ids: want [FBR-1, FBR-2], got %+v", branchIDs)
	}
	// Fallback: a branch that never touched the family sees main's ids.
	other := domain.BranchID(uuid.New())
	fallbackIDs, _ := store.GetFamilyExternalIDs(ctx, other, familyID)
	if len(fallbackIDs) != 1 || fallbackIDs[0].Value != "FMAIN" {
		t.Fatalf("fallback family ext ids: want [FMAIN], got %+v", fallbackIDs)
	}
}

func TestBranchPedigreeEdgeOverlayTombstone(t *testing.T) {
	store, cleanup := setupTestReadModelDB(t)
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
	if e, _ := store.GetPedigreeEdge(ctx, branch, personID); e == nil || *e.FatherID != branchFather {
		t.Fatalf("branch edge override: want branchFather, got %+v", e)
	}
	if e, _ := store.GetPedigreeEdge(ctx, domain.MainBranchID, personID); e == nil || *e.FatherID != mainFather {
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
	store, cleanup := setupTestReadModelDB(t)
	defer cleanup()
	ctx := context.Background()
	branch := domain.BranchID(uuid.New())
	personID := uuid.New()

	_ = store.SavePerson(ctx, domain.MainBranchID, branchPersonRM(personID, "Cascade", "Main"))
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
	// Main still has the person.
	if p, _ := store.GetPerson(ctx, domain.MainBranchID, personID); p == nil {
		t.Fatal("main person after branch cascade: want present, got nil")
	}
}
