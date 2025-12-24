package domain

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
)

func TestPersonCreated_RoundTrip(t *testing.T) {
	p := NewPerson("John", "Doe")
	p.Gender = GenderMale
	p.SetBirthDate("1 JAN 1850")
	p.BirthPlace = "Springfield, IL"
	p.Notes = "Test person"

	event := NewPersonCreated(p)

	// Verify event type
	if event.EventType() != "PersonCreated" {
		t.Errorf("EventType() = %v, want PersonCreated", event.EventType())
	}

	// Verify aggregate ID
	if event.AggregateID() != p.ID {
		t.Errorf("AggregateID() = %v, want %v", event.AggregateID(), p.ID)
	}

	// JSON round-trip
	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var decoded PersonCreated
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if decoded.PersonID != event.PersonID {
		t.Errorf("PersonID = %v, want %v", decoded.PersonID, event.PersonID)
	}
	if decoded.GivenName != event.GivenName {
		t.Errorf("GivenName = %v, want %v", decoded.GivenName, event.GivenName)
	}
	if decoded.Surname != event.Surname {
		t.Errorf("Surname = %v, want %v", decoded.Surname, event.Surname)
	}
	if decoded.Gender != event.Gender {
		t.Errorf("Gender = %v, want %v", decoded.Gender, event.Gender)
	}
	if decoded.BirthPlace != event.BirthPlace {
		t.Errorf("BirthPlace = %v, want %v", decoded.BirthPlace, event.BirthPlace)
	}
}

func TestPersonUpdated_RoundTrip(t *testing.T) {
	personID := uuid.New()
	changes := map[string]any{
		"given_name": "Jane",
		"surname":    "Smith",
	}

	event := NewPersonUpdated(personID, changes)

	if event.EventType() != "PersonUpdated" {
		t.Errorf("EventType() = %v, want PersonUpdated", event.EventType())
	}

	// JSON round-trip
	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var decoded PersonUpdated
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if decoded.PersonID != personID {
		t.Errorf("PersonID = %v, want %v", decoded.PersonID, personID)
	}
	if decoded.Changes["given_name"] != "Jane" {
		t.Errorf("Changes[given_name] = %v, want Jane", decoded.Changes["given_name"])
	}
}

func TestPersonDeleted_RoundTrip(t *testing.T) {
	personID := uuid.New()
	event := NewPersonDeleted(personID, "Test deletion")

	if event.EventType() != "PersonDeleted" {
		t.Errorf("EventType() = %v, want PersonDeleted", event.EventType())
	}

	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var decoded PersonDeleted
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if decoded.PersonID != personID {
		t.Errorf("PersonID = %v, want %v", decoded.PersonID, personID)
	}
	if decoded.Reason != "Test deletion" {
		t.Errorf("Reason = %v, want Test deletion", decoded.Reason)
	}
}

func TestFamilyCreated_RoundTrip(t *testing.T) {
	p1 := uuid.New()
	p2 := uuid.New()
	f := NewFamilyWithPartners(&p1, &p2)
	f.RelationshipType = RelationMarriage
	f.SetMarriageDate("1 JAN 1870")
	f.MarriagePlace = "Springfield, IL"

	event := NewFamilyCreated(f)

	if event.EventType() != "FamilyCreated" {
		t.Errorf("EventType() = %v, want FamilyCreated", event.EventType())
	}

	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var decoded FamilyCreated
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if decoded.FamilyID != f.ID {
		t.Errorf("FamilyID = %v, want %v", decoded.FamilyID, f.ID)
	}
	if decoded.Partner1ID == nil || *decoded.Partner1ID != p1 {
		t.Error("Partner1ID not preserved")
	}
	if decoded.Partner2ID == nil || *decoded.Partner2ID != p2 {
		t.Error("Partner2ID not preserved")
	}
	if decoded.RelationshipType != RelationMarriage {
		t.Errorf("RelationshipType = %v, want marriage", decoded.RelationshipType)
	}
}

func TestChildLinkedToFamily_RoundTrip(t *testing.T) {
	familyID := uuid.New()
	personID := uuid.New()
	seq := 1
	fc := &FamilyChild{
		FamilyID:         familyID,
		PersonID:         personID,
		RelationshipType: ChildAdopted,
		Sequence:         &seq,
	}

	event := NewChildLinkedToFamily(fc)

	if event.EventType() != "ChildLinkedToFamily" {
		t.Errorf("EventType() = %v, want ChildLinkedToFamily", event.EventType())
	}

	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var decoded ChildLinkedToFamily
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if decoded.FamilyID != familyID {
		t.Errorf("FamilyID = %v, want %v", decoded.FamilyID, familyID)
	}
	if decoded.PersonID != personID {
		t.Errorf("PersonID = %v, want %v", decoded.PersonID, personID)
	}
	if decoded.RelationshipType != ChildAdopted {
		t.Errorf("RelationshipType = %v, want adopted", decoded.RelationshipType)
	}
	if decoded.Sequence == nil || *decoded.Sequence != 1 {
		t.Error("Sequence not preserved")
	}
}

func TestChildUnlinkedFromFamily_RoundTrip(t *testing.T) {
	familyID := uuid.New()
	personID := uuid.New()

	event := NewChildUnlinkedFromFamily(familyID, personID)

	if event.EventType() != "ChildUnlinkedFromFamily" {
		t.Errorf("EventType() = %v, want ChildUnlinkedFromFamily", event.EventType())
	}

	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var decoded ChildUnlinkedFromFamily
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if decoded.FamilyID != familyID {
		t.Errorf("FamilyID = %v, want %v", decoded.FamilyID, familyID)
	}
	if decoded.PersonID != personID {
		t.Errorf("PersonID = %v, want %v", decoded.PersonID, personID)
	}
}

func TestGedcomImported_RoundTrip(t *testing.T) {
	warnings := []string{"Warning 1", "Warning 2"}
	errors := []string{"Error 1"}

	event := NewGedcomImported("test.ged", 12345, 100, 50, warnings, errors)

	if event.EventType() != "GedcomImported" {
		t.Errorf("EventType() = %v, want GedcomImported", event.EventType())
	}

	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var decoded GedcomImported
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if decoded.Filename != "test.ged" {
		t.Errorf("Filename = %v, want test.ged", decoded.Filename)
	}
	if decoded.FileSize != 12345 {
		t.Errorf("FileSize = %v, want 12345", decoded.FileSize)
	}
	if decoded.PersonsImported != 100 {
		t.Errorf("PersonsImported = %v, want 100", decoded.PersonsImported)
	}
	if decoded.FamiliesImported != 50 {
		t.Errorf("FamiliesImported = %v, want 50", decoded.FamiliesImported)
	}
	if len(decoded.Warnings) != 2 {
		t.Errorf("Warnings length = %v, want 2", len(decoded.Warnings))
	}
	if len(decoded.Errors) != 1 {
		t.Errorf("Errors length = %v, want 1", len(decoded.Errors))
	}
}

func TestSourceCreated_RoundTrip(t *testing.T) {
	s := NewSource("1850 US Census", SourceCensus)
	s.Author = "US Government"
	s.Publisher = "National Archives"
	pd := ParseGenDate("1850")
	s.PublishDate = &pd
	s.URL = "https://example.com/census"
	s.RepositoryName = "NARA"
	s.CollectionName = "Census Records"
	s.CallNumber = "M432"
	s.Notes = "Important census record"

	event := NewSourceCreated(s)

	// Verify event type
	if event.EventType() != "SourceCreated" {
		t.Errorf("EventType() = %v, want SourceCreated", event.EventType())
	}

	// Verify aggregate ID
	if event.AggregateID() != s.ID {
		t.Errorf("AggregateID() = %v, want %v", event.AggregateID(), s.ID)
	}

	// JSON round-trip
	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var decoded SourceCreated
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if decoded.SourceID != event.SourceID {
		t.Errorf("SourceID = %v, want %v", decoded.SourceID, event.SourceID)
	}
	if decoded.Title != event.Title {
		t.Errorf("Title = %v, want %v", decoded.Title, event.Title)
	}
	if decoded.SourceType != event.SourceType {
		t.Errorf("SourceType = %v, want %v", decoded.SourceType, event.SourceType)
	}
	if decoded.Author != event.Author {
		t.Errorf("Author = %v, want %v", decoded.Author, event.Author)
	}
	if decoded.URL != event.URL {
		t.Errorf("URL = %v, want %v", decoded.URL, event.URL)
	}
}

func TestSourceUpdated_RoundTrip(t *testing.T) {
	sourceID := uuid.New()
	changes := map[string]any{
		"title":  "Updated Title",
		"author": "New Author",
	}

	event := NewSourceUpdated(sourceID, changes)

	if event.EventType() != "SourceUpdated" {
		t.Errorf("EventType() = %v, want SourceUpdated", event.EventType())
	}

	// JSON round-trip
	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var decoded SourceUpdated
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if decoded.SourceID != sourceID {
		t.Errorf("SourceID = %v, want %v", decoded.SourceID, sourceID)
	}
	if decoded.Changes["title"] != "Updated Title" {
		t.Errorf("Changes[title] = %v, want Updated Title", decoded.Changes["title"])
	}
}

func TestSourceDeleted_RoundTrip(t *testing.T) {
	sourceID := uuid.New()
	event := NewSourceDeleted(sourceID, "Test deletion")

	if event.EventType() != "SourceDeleted" {
		t.Errorf("EventType() = %v, want SourceDeleted", event.EventType())
	}

	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var decoded SourceDeleted
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if decoded.SourceID != sourceID {
		t.Errorf("SourceID = %v, want %v", decoded.SourceID, sourceID)
	}
	if decoded.Reason != "Test deletion" {
		t.Errorf("Reason = %v, want Test deletion", decoded.Reason)
	}
}

func TestCitationCreated_RoundTrip(t *testing.T) {
	sourceID := uuid.New()
	factOwnerID := uuid.New()
	c := NewCitation(sourceID, FactPersonBirth, factOwnerID)
	c.Page = "123"
	c.Volume = "1"
	c.SourceQuality = SourceOriginal
	c.InformantType = InformantPrimary
	c.EvidenceType = EvidenceDirect
	c.QuotedText = "Born on Jan 1, 1850"
	c.Analysis = "Primary evidence of birth"

	event := NewCitationCreated(c)

	// Verify event type
	if event.EventType() != "CitationCreated" {
		t.Errorf("EventType() = %v, want CitationCreated", event.EventType())
	}

	// Verify aggregate ID
	if event.AggregateID() != c.ID {
		t.Errorf("AggregateID() = %v, want %v", event.AggregateID(), c.ID)
	}

	// JSON round-trip
	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var decoded CitationCreated
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if decoded.CitationID != event.CitationID {
		t.Errorf("CitationID = %v, want %v", decoded.CitationID, event.CitationID)
	}
	if decoded.SourceID != event.SourceID {
		t.Errorf("SourceID = %v, want %v", decoded.SourceID, event.SourceID)
	}
	if decoded.FactType != event.FactType {
		t.Errorf("FactType = %v, want %v", decoded.FactType, event.FactType)
	}
	if decoded.FactOwnerID != event.FactOwnerID {
		t.Errorf("FactOwnerID = %v, want %v", decoded.FactOwnerID, event.FactOwnerID)
	}
	if decoded.SourceQuality != event.SourceQuality {
		t.Errorf("SourceQuality = %v, want %v", decoded.SourceQuality, event.SourceQuality)
	}
	if decoded.InformantType != event.InformantType {
		t.Errorf("InformantType = %v, want %v", decoded.InformantType, event.InformantType)
	}
	if decoded.EvidenceType != event.EvidenceType {
		t.Errorf("EvidenceType = %v, want %v", decoded.EvidenceType, event.EvidenceType)
	}
}

func TestCitationUpdated_RoundTrip(t *testing.T) {
	citationID := uuid.New()
	changes := map[string]any{
		"page":     "456",
		"analysis": "Updated analysis",
	}

	event := NewCitationUpdated(citationID, changes)

	if event.EventType() != "CitationUpdated" {
		t.Errorf("EventType() = %v, want CitationUpdated", event.EventType())
	}

	// JSON round-trip
	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var decoded CitationUpdated
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if decoded.CitationID != citationID {
		t.Errorf("CitationID = %v, want %v", decoded.CitationID, citationID)
	}
	if decoded.Changes["page"] != "456" {
		t.Errorf("Changes[page] = %v, want 456", decoded.Changes["page"])
	}
}

func TestCitationDeleted_RoundTrip(t *testing.T) {
	citationID := uuid.New()
	event := NewCitationDeleted(citationID, "No longer relevant")

	if event.EventType() != "CitationDeleted" {
		t.Errorf("EventType() = %v, want CitationDeleted", event.EventType())
	}

	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var decoded CitationDeleted
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if decoded.CitationID != citationID {
		t.Errorf("CitationID = %v, want %v", decoded.CitationID, citationID)
	}
	if decoded.Reason != "No longer relevant" {
		t.Errorf("Reason = %v, want No longer relevant", decoded.Reason)
	}
}
