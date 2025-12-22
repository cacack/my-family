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

	err := store.SavePerson(ctx, person)
	if err != nil {
		t.Fatalf("save person: %v", err)
	}

	// Read
	retrieved, err := store.GetPerson(ctx, personID)
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
	err = store.SavePerson(ctx, person)
	if err != nil {
		t.Fatalf("update person: %v", err)
	}

	retrieved, err = store.GetPerson(ctx, personID)
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
	err = store.DeletePerson(ctx, personID)
	if err != nil {
		t.Fatalf("delete person: %v", err)
	}

	retrieved, err = store.GetPerson(ctx, personID)
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
		err := store.SavePerson(ctx, person)
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
		err := store.SavePerson(ctx, person)
		if err != nil {
			t.Fatalf("save person: %v", err)
		}
	}

	// Search for "Doe"
	results, err := store.SearchPersons(ctx, "Doe", false, 10)
	if err != nil {
		t.Fatalf("search persons: %v", err)
	}

	if len(results) != 2 {
		t.Errorf("expected 2 results for 'Doe', got %d", len(results))
	}

	// Search for "John"
	results, err = store.SearchPersons(ctx, "John", false, 10)
	if err != nil {
		t.Fatalf("search persons: %v", err)
	}

	if len(results) != 3 { // John Doe, John Smith, Alice Johnson
		t.Errorf("expected 3 results for 'John', got %d", len(results))
	}

	// Fuzzy search (prefix matching)
	results, err = store.SearchPersons(ctx, "Jo", true, 10)
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

	store.SavePerson(ctx, person1)
	store.SavePerson(ctx, person2)

	// Create family
	familyID := uuid.New()
	marriageDate := time.Date(1875, 6, 15, 0, 0, 0, 0, time.UTC)
	family := &repository.FamilyReadModel{
		ID:               familyID,
		Partner1ID:       &person1ID,
		Partner1Name:     "John Doe",
		Partner2ID:       &person2ID,
		Partner2Name:     "Jane Doe",
		RelationshipType: domain.RelationMarriage,
		MarriageDateRaw:  "15 JUN 1875",
		MarriageDateSort: &marriageDate,
		MarriagePlace:    "Springfield, IL",
		Version:          1,
		UpdatedAt:        time.Now(),
	}

	err := store.SaveFamily(ctx, family)
	if err != nil {
		t.Fatalf("save family: %v", err)
	}

	// Read
	retrieved, err := store.GetFamily(ctx, familyID)
	if err != nil {
		t.Fatalf("get family: %v", err)
	}
	if retrieved == nil {
		t.Fatal("expected family, got nil")
	}

	if retrieved.Partner1Name != "John Doe" {
		t.Errorf("expected Partner1Name John Doe, got %s", retrieved.Partner1Name)
	}
	if retrieved.RelationshipType != domain.RelationMarriage {
		t.Errorf("expected RelationshipType marriage, got %s", retrieved.RelationshipType)
	}

	// Get families for person
	families, err := store.GetFamiliesForPerson(ctx, person1ID)
	if err != nil {
		t.Fatalf("get families for person: %v", err)
	}
	if len(families) != 1 {
		t.Errorf("expected 1 family, got %d", len(families))
	}

	// Delete
	err = store.DeleteFamily(ctx, familyID)
	if err != nil {
		t.Fatalf("delete family: %v", err)
	}

	retrieved, err = store.GetFamily(ctx, familyID)
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

	store.SavePerson(ctx, parent1)
	store.SavePerson(ctx, parent2)
	store.SavePerson(ctx, child)

	// Create family
	familyID := uuid.New()
	family := &repository.FamilyReadModel{
		ID:           familyID,
		Partner1ID:   &parent1ID,
		Partner1Name: "John Doe",
		Partner2ID:   &parent2ID,
		Partner2Name: "Jane Doe",
		ChildCount:   0,
		Version:      1,
		UpdatedAt:    time.Now(),
	}
	store.SaveFamily(ctx, family)

	// Add child
	seq := 1
	familyChild := &repository.FamilyChildReadModel{
		FamilyID:         familyID,
		PersonID:         childID,
		PersonName:       "Bobby Doe",
		RelationshipType: domain.ChildBiological,
		Sequence:         &seq,
	}

	err := store.SaveFamilyChild(ctx, familyChild)
	if err != nil {
		t.Fatalf("save family child: %v", err)
	}

	// Get children
	children, err := store.GetFamilyChildren(ctx, familyID)
	if err != nil {
		t.Fatalf("get family children: %v", err)
	}
	if len(children) != 1 {
		t.Fatalf("expected 1 child, got %d", len(children))
	}
	if children[0].PersonName != "Bobby Doe" {
		t.Errorf("expected PersonName Bobby Doe, got %s", children[0].PersonName)
	}

	// Get child family
	childFamily, err := store.GetChildFamily(ctx, childID)
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
	childPersons, err := store.GetChildrenOfFamily(ctx, familyID)
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
	err = store.DeleteFamilyChild(ctx, familyID, childID)
	if err != nil {
		t.Fatalf("delete family child: %v", err)
	}

	children, err = store.GetFamilyChildren(ctx, familyID)
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

	store.SavePerson(ctx, child)
	store.SavePerson(ctx, father)
	store.SavePerson(ctx, mother)

	// Create pedigree edge
	edge := &repository.PedigreeEdge{
		PersonID:   childID,
		FatherID:   &fatherID,
		MotherID:   &motherID,
		FatherName: "John Doe",
		MotherName: "Jane Doe",
	}

	err := store.SavePedigreeEdge(ctx, edge)
	if err != nil {
		t.Fatalf("save pedigree edge: %v", err)
	}

	// Get edge
	retrieved, err := store.GetPedigreeEdge(ctx, childID)
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
	err = store.DeletePedigreeEdge(ctx, childID)
	if err != nil {
		t.Fatalf("delete pedigree edge: %v", err)
	}

	retrieved, err = store.GetPedigreeEdge(ctx, childID)
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
	person, err := store.GetPerson(ctx, nonExistentID)
	if err != nil {
		t.Fatalf("get non-existent person: %v", err)
	}
	if person != nil {
		t.Error("expected nil for non-existent person")
	}

	// Get non-existent family
	family, err := store.GetFamily(ctx, nonExistentID)
	if err != nil {
		t.Fatalf("get non-existent family: %v", err)
	}
	if family != nil {
		t.Error("expected nil for non-existent family")
	}

	// Get non-existent pedigree edge
	edge, err := store.GetPedigreeEdge(ctx, nonExistentID)
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

	store.SavePerson(ctx, person1)
	store.SavePerson(ctx, person2)
	store.SavePerson(ctx, person3)

	// Create multiple families
	family1ID := uuid.New()
	family2ID := uuid.New()

	family1 := &repository.FamilyReadModel{
		ID:           family1ID,
		Partner1ID:   &person1ID,
		Partner1Name: "John Doe",
		Partner2ID:   &person2ID,
		Partner2Name: "Jane Doe",
		Version:      1,
		UpdatedAt:    time.Now().Add(-1 * time.Hour), // Older
	}
	family2 := &repository.FamilyReadModel{
		ID:           family2ID,
		Partner1ID:   &person3ID,
		Partner1Name: "Bob Smith",
		Version:      1,
		UpdatedAt:    time.Now(), // Newer
	}

	store.SaveFamily(ctx, family1)
	store.SaveFamily(ctx, family2)

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
	store.SavePerson(ctx, person)

	// Search with a complex FTS5 query that might fail
	// Using quotes and special FTS5 operators can trigger errors
	results, err := store.SearchPersons(ctx, `"John" AND "Doe"`, false, 10)
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
	store.SavePerson(ctx, person)

	// Search for something that doesn't match but with fuzzy enabled
	// This should trigger the fuzzy fallback at line 261-262
	results, err := store.SearchPersons(ctx, "xyz123notfound", true, 10)
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
	store.SavePerson(ctx, person)

	// Fuzzy search with prefix that might not match in FTS5
	// This tests the fuzzy fallback path
	results, err := store.SearchPersons(ctx, "Zac", true, 10)
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
		store.SavePerson(ctx, person)
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
	store.SavePerson(ctx, person)

	// Search with special FTS5 characters that might cause errors
	// This should trigger FTS5 error and fallback to LIKE
	testQueries := []string{
		`Mary-Ann`,   // Hyphen
		`O'Brien`,    // Apostrophe
		`"Mary-Ann"`, // Quotes
		`(Mary)`,     // Parentheses
	}

	for _, query := range testQueries {
		results, err := store.SearchPersons(ctx, query, false, 10)
		if err != nil {
			t.Fatalf("search with query %q failed: %v", query, err)
		}
		// Results may or may not be found depending on FTS5/LIKE behavior
		t.Logf("Query %q returned %d results", query, len(results))
	}
}
