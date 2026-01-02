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

func TestLifeEventCreatedFromModel_RoundTrip(t *testing.T) {
	personID := uuid.New()
	le := NewLifeEvent(personID, FactPersonBaptism)
	le.SetDate("1 JAN 1850")
	le.Place = "Springfield, IL"
	le.Description = "Baptized at First Church"
	le.Cause = ""
	le.Age = "0"
	le.GedcomXref = "@E1@"

	event := NewLifeEventCreatedFromModel(le)

	// Verify event type
	if event.EventType() != "LifeEventCreated" {
		t.Errorf("EventType() = %v, want LifeEventCreated", event.EventType())
	}

	// Verify aggregate ID
	if event.AggregateID() != le.ID {
		t.Errorf("AggregateID() = %v, want %v", event.AggregateID(), le.ID)
	}

	// JSON round-trip
	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var decoded LifeEventCreated
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if decoded.EventID != le.ID {
		t.Errorf("EventID = %v, want %v", decoded.EventID, le.ID)
	}
	if decoded.PersonID == nil || *decoded.PersonID != personID {
		t.Error("PersonID not preserved")
	}
	if decoded.FamilyID != nil {
		t.Error("FamilyID should be nil for person events")
	}
	if decoded.FactType != FactPersonBaptism {
		t.Errorf("FactType = %v, want %v", decoded.FactType, FactPersonBaptism)
	}
	if decoded.Place != "Springfield, IL" {
		t.Errorf("Place = %v, want Springfield, IL", decoded.Place)
	}
	if decoded.GedcomXref != "@E1@" {
		t.Errorf("GedcomXref = %v, want @E1@", decoded.GedcomXref)
	}
}

func TestLifeEventCreatedFromModel_FamilyEvent(t *testing.T) {
	familyID := uuid.New()
	le := NewFamilyLifeEvent(familyID, FactFamilyEngagement)
	le.SetDate("15 FEB 1870")
	le.Place = "Boston, MA"

	event := NewLifeEventCreatedFromModel(le)

	if event.PersonID != nil {
		t.Error("PersonID should be nil for family events")
	}
	if event.FamilyID == nil || *event.FamilyID != familyID {
		t.Error("FamilyID not preserved")
	}
}

func TestLifeEventUpdated_RoundTrip(t *testing.T) {
	eventID := uuid.New()
	changes := map[string]any{
		"place":       "New Location",
		"description": "Updated description",
	}

	event := NewLifeEventUpdated(eventID, changes)

	if event.EventType() != "LifeEventUpdated" {
		t.Errorf("EventType() = %v, want LifeEventUpdated", event.EventType())
	}

	if event.AggregateID() != eventID {
		t.Errorf("AggregateID() = %v, want %v", event.AggregateID(), eventID)
	}

	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var decoded LifeEventUpdated
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if decoded.EventID != eventID {
		t.Errorf("EventID = %v, want %v", decoded.EventID, eventID)
	}
	if decoded.Changes["place"] != "New Location" {
		t.Errorf("Changes[place] = %v, want New Location", decoded.Changes["place"])
	}
}

func TestLifeEventDeleted_RoundTrip(t *testing.T) {
	eventID := uuid.New()
	event := NewLifeEventDeleted(eventID, "Duplicate entry")

	if event.EventType() != "LifeEventDeleted" {
		t.Errorf("EventType() = %v, want LifeEventDeleted", event.EventType())
	}

	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var decoded LifeEventDeleted
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if decoded.EventID != eventID {
		t.Errorf("EventID = %v, want %v", decoded.EventID, eventID)
	}
	if decoded.Reason != "Duplicate entry" {
		t.Errorf("Reason = %v, want Duplicate entry", decoded.Reason)
	}
}

func TestAttributeCreatedFromModel_RoundTrip(t *testing.T) {
	personID := uuid.New()
	attr := NewAttribute(personID, FactPersonOccupation, "Blacksmith")
	attr.SetDate("FROM 1850 TO 1875")
	attr.Place = "Springfield, IL"
	attr.GedcomXref = "@A1@"

	event := NewAttributeCreatedFromModel(attr)

	// Verify event type
	if event.EventType() != "AttributeCreated" {
		t.Errorf("EventType() = %v, want AttributeCreated", event.EventType())
	}

	// Verify aggregate ID
	if event.AggregateID() != attr.ID {
		t.Errorf("AggregateID() = %v, want %v", event.AggregateID(), attr.ID)
	}

	// JSON round-trip
	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var decoded AttributeCreated
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if decoded.AttributeID != attr.ID {
		t.Errorf("AttributeID = %v, want %v", decoded.AttributeID, attr.ID)
	}
	if decoded.PersonID != personID {
		t.Errorf("PersonID = %v, want %v", decoded.PersonID, personID)
	}
	if decoded.FactType != FactPersonOccupation {
		t.Errorf("FactType = %v, want %v", decoded.FactType, FactPersonOccupation)
	}
	if decoded.Value != "Blacksmith" {
		t.Errorf("Value = %v, want Blacksmith", decoded.Value)
	}
	if decoded.Place != "Springfield, IL" {
		t.Errorf("Place = %v, want Springfield, IL", decoded.Place)
	}
	if decoded.GedcomXref != "@A1@" {
		t.Errorf("GedcomXref = %v, want @A1@", decoded.GedcomXref)
	}
}

func TestAttributeUpdated_RoundTrip(t *testing.T) {
	attributeID := uuid.New()
	changes := map[string]any{
		"value": "Farmer",
		"place": "New Location",
	}

	event := NewAttributeUpdated(attributeID, changes)

	if event.EventType() != "AttributeUpdated" {
		t.Errorf("EventType() = %v, want AttributeUpdated", event.EventType())
	}

	if event.AggregateID() != attributeID {
		t.Errorf("AggregateID() = %v, want %v", event.AggregateID(), attributeID)
	}

	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var decoded AttributeUpdated
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if decoded.AttributeID != attributeID {
		t.Errorf("AttributeID = %v, want %v", decoded.AttributeID, attributeID)
	}
	if decoded.Changes["value"] != "Farmer" {
		t.Errorf("Changes[value] = %v, want Farmer", decoded.Changes["value"])
	}
}

func TestAttributeDeleted_RoundTrip(t *testing.T) {
	attributeID := uuid.New()
	event := NewAttributeDeleted(attributeID, "No longer valid")

	if event.EventType() != "AttributeDeleted" {
		t.Errorf("EventType() = %v, want AttributeDeleted", event.EventType())
	}

	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var decoded AttributeDeleted
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if decoded.AttributeID != attributeID {
		t.Errorf("AttributeID = %v, want %v", decoded.AttributeID, attributeID)
	}
	if decoded.Reason != "No longer valid" {
		t.Errorf("Reason = %v, want No longer valid", decoded.Reason)
	}
}

func TestBaseEvent_OccurredAt(t *testing.T) {
	event := NewBaseEvent()

	occurredAt := event.OccurredAt()
	if occurredAt.IsZero() {
		t.Error("OccurredAt() returned zero time")
	}
	if occurredAt != event.Timestamp {
		t.Errorf("OccurredAt() = %v, want %v", occurredAt, event.Timestamp)
	}
}

func TestFamilyUpdated_RoundTrip(t *testing.T) {
	familyID := uuid.New()
	changes := map[string]any{
		"relationship_type": "marriage",
		"marriage_place":    "New York, NY",
	}

	event := NewFamilyUpdated(familyID, changes)

	if event.EventType() != "FamilyUpdated" {
		t.Errorf("EventType() = %v, want FamilyUpdated", event.EventType())
	}

	if event.AggregateID() != familyID {
		t.Errorf("AggregateID() = %v, want %v", event.AggregateID(), familyID)
	}

	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var decoded FamilyUpdated
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if decoded.FamilyID != familyID {
		t.Errorf("FamilyID = %v, want %v", decoded.FamilyID, familyID)
	}
	if decoded.Changes["relationship_type"] != "marriage" {
		t.Errorf("Changes[relationship_type] = %v, want marriage", decoded.Changes["relationship_type"])
	}
	if decoded.Changes["marriage_place"] != "New York, NY" {
		t.Errorf("Changes[marriage_place] = %v, want New York, NY", decoded.Changes["marriage_place"])
	}
}

func TestFamilyDeleted_RoundTrip(t *testing.T) {
	familyID := uuid.New()
	event := NewFamilyDeleted(familyID, "Family dissolved")

	if event.EventType() != "FamilyDeleted" {
		t.Errorf("EventType() = %v, want FamilyDeleted", event.EventType())
	}

	if event.AggregateID() != familyID {
		t.Errorf("AggregateID() = %v, want %v", event.AggregateID(), familyID)
	}

	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var decoded FamilyDeleted
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if decoded.FamilyID != familyID {
		t.Errorf("FamilyID = %v, want %v", decoded.FamilyID, familyID)
	}
	if decoded.Reason != "Family dissolved" {
		t.Errorf("Reason = %v, want Family dissolved", decoded.Reason)
	}
}

func TestMediaCreated_RoundTrip(t *testing.T) {
	entityID := uuid.New()
	m := NewMedia("Family Photo 1920", "person", entityID)
	m.Description = "Family gathering at homestead"
	m.MimeType = "image/jpeg"
	m.MediaType = MediaPhoto
	m.Filename = "family_1920.jpg"
	m.FileSize = 1024000
	m.FileData = []byte{0x01, 0x02, 0x03}
	m.ThumbnailData = []byte{0x04, 0x05}
	m.GedcomXref = "@M1@"

	event := NewMediaCreated(m)

	if event.EventType() != "MediaCreated" {
		t.Errorf("EventType() = %v, want MediaCreated", event.EventType())
	}

	if event.AggregateID() != m.ID {
		t.Errorf("AggregateID() = %v, want %v", event.AggregateID(), m.ID)
	}

	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var decoded MediaCreated
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if decoded.MediaID != m.ID {
		t.Errorf("MediaID = %v, want %v", decoded.MediaID, m.ID)
	}
	if decoded.EntityType != "person" {
		t.Errorf("EntityType = %v, want person", decoded.EntityType)
	}
	if decoded.EntityID != entityID {
		t.Errorf("EntityID = %v, want %v", decoded.EntityID, entityID)
	}
	if decoded.Title != "Family Photo 1920" {
		t.Errorf("Title = %v, want Family Photo 1920", decoded.Title)
	}
	if decoded.Description != "Family gathering at homestead" {
		t.Errorf("Description = %v, want Family gathering at homestead", decoded.Description)
	}
	if decoded.MimeType != "image/jpeg" {
		t.Errorf("MimeType = %v, want image/jpeg", decoded.MimeType)
	}
	if decoded.MediaType != MediaPhoto {
		t.Errorf("MediaType = %v, want %v", decoded.MediaType, MediaPhoto)
	}
	if decoded.Filename != "family_1920.jpg" {
		t.Errorf("Filename = %v, want family_1920.jpg", decoded.Filename)
	}
	if decoded.FileSize != 1024000 {
		t.Errorf("FileSize = %v, want 1024000", decoded.FileSize)
	}
	if decoded.GedcomXref != "@M1@" {
		t.Errorf("GedcomXref = %v, want @M1@", decoded.GedcomXref)
	}
}

func TestMediaUpdated_RoundTrip(t *testing.T) {
	mediaID := uuid.New()
	changes := map[string]any{
		"title":       "Updated Title",
		"description": "New description",
	}

	event := NewMediaUpdated(mediaID, changes)

	if event.EventType() != "MediaUpdated" {
		t.Errorf("EventType() = %v, want MediaUpdated", event.EventType())
	}

	if event.AggregateID() != mediaID {
		t.Errorf("AggregateID() = %v, want %v", event.AggregateID(), mediaID)
	}

	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var decoded MediaUpdated
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if decoded.MediaID != mediaID {
		t.Errorf("MediaID = %v, want %v", decoded.MediaID, mediaID)
	}
	if decoded.Changes["title"] != "Updated Title" {
		t.Errorf("Changes[title] = %v, want Updated Title", decoded.Changes["title"])
	}
	if decoded.Changes["description"] != "New description" {
		t.Errorf("Changes[description] = %v, want New description", decoded.Changes["description"])
	}
}

func TestMediaDeleted_RoundTrip(t *testing.T) {
	mediaID := uuid.New()
	event := NewMediaDeleted(mediaID, "Duplicate file")

	if event.EventType() != "MediaDeleted" {
		t.Errorf("EventType() = %v, want MediaDeleted", event.EventType())
	}

	if event.AggregateID() != mediaID {
		t.Errorf("AggregateID() = %v, want %v", event.AggregateID(), mediaID)
	}

	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var decoded MediaDeleted
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if decoded.MediaID != mediaID {
		t.Errorf("MediaID = %v, want %v", decoded.MediaID, mediaID)
	}
	if decoded.Reason != "Duplicate file" {
		t.Errorf("Reason = %v, want Duplicate file", decoded.Reason)
	}
}

func TestRepositoryCreated_RoundTrip(t *testing.T) {
	r := NewRepository("National Archives")
	r.Address = "700 Pennsylvania Avenue NW"
	r.City = "Washington"
	r.State = "DC"
	r.PostalCode = "20408"
	r.Country = "USA"
	r.Phone = "202-357-5000"
	r.Email = "inquire@nara.gov"
	r.Website = "https://www.archives.gov"
	r.Notes = "Primary federal archives"
	r.GedcomXref = "@R1@"

	event := NewRepositoryCreated(r)

	if event.EventType() != "RepositoryCreated" {
		t.Errorf("EventType() = %v, want RepositoryCreated", event.EventType())
	}

	if event.AggregateID() != r.ID {
		t.Errorf("AggregateID() = %v, want %v", event.AggregateID(), r.ID)
	}

	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var decoded RepositoryCreated
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if decoded.RepositoryID != r.ID {
		t.Errorf("RepositoryID = %v, want %v", decoded.RepositoryID, r.ID)
	}
	if decoded.Name != "National Archives" {
		t.Errorf("Name = %v, want National Archives", decoded.Name)
	}
	if decoded.Address != "700 Pennsylvania Avenue NW" {
		t.Errorf("Address = %v, want 700 Pennsylvania Avenue NW", decoded.Address)
	}
	if decoded.City != "Washington" {
		t.Errorf("City = %v, want Washington", decoded.City)
	}
	if decoded.State != "DC" {
		t.Errorf("State = %v, want DC", decoded.State)
	}
	if decoded.PostalCode != "20408" {
		t.Errorf("PostalCode = %v, want 20408", decoded.PostalCode)
	}
	if decoded.Country != "USA" {
		t.Errorf("Country = %v, want USA", decoded.Country)
	}
	if decoded.Phone != "202-357-5000" {
		t.Errorf("Phone = %v, want 202-357-5000", decoded.Phone)
	}
	if decoded.Email != "inquire@nara.gov" {
		t.Errorf("Email = %v, want inquire@nara.gov", decoded.Email)
	}
	if decoded.Website != "https://www.archives.gov" {
		t.Errorf("Website = %v, want https://www.archives.gov", decoded.Website)
	}
	if decoded.Notes != "Primary federal archives" {
		t.Errorf("Notes = %v, want Primary federal archives", decoded.Notes)
	}
	if decoded.GedcomXref != "@R1@" {
		t.Errorf("GedcomXref = %v, want @R1@", decoded.GedcomXref)
	}
}

func TestRepositoryUpdated_RoundTrip(t *testing.T) {
	repositoryID := uuid.New()
	changes := map[string]any{
		"name":    "Updated Repository Name",
		"address": "New Address",
		"phone":   "555-1234",
	}

	event := NewRepositoryUpdated(repositoryID, changes)

	if event.EventType() != "RepositoryUpdated" {
		t.Errorf("EventType() = %v, want RepositoryUpdated", event.EventType())
	}

	if event.AggregateID() != repositoryID {
		t.Errorf("AggregateID() = %v, want %v", event.AggregateID(), repositoryID)
	}

	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var decoded RepositoryUpdated
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if decoded.RepositoryID != repositoryID {
		t.Errorf("RepositoryID = %v, want %v", decoded.RepositoryID, repositoryID)
	}
	if decoded.Changes["name"] != "Updated Repository Name" {
		t.Errorf("Changes[name] = %v, want Updated Repository Name", decoded.Changes["name"])
	}
	if decoded.Changes["address"] != "New Address" {
		t.Errorf("Changes[address] = %v, want New Address", decoded.Changes["address"])
	}
	if decoded.Changes["phone"] != "555-1234" {
		t.Errorf("Changes[phone] = %v, want 555-1234", decoded.Changes["phone"])
	}
}

func TestRepositoryDeleted_RoundTrip(t *testing.T) {
	repositoryID := uuid.New()
	event := NewRepositoryDeleted(repositoryID, "Repository closed")

	if event.EventType() != "RepositoryDeleted" {
		t.Errorf("EventType() = %v, want RepositoryDeleted", event.EventType())
	}

	if event.AggregateID() != repositoryID {
		t.Errorf("AggregateID() = %v, want %v", event.AggregateID(), repositoryID)
	}

	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var decoded RepositoryDeleted
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if decoded.RepositoryID != repositoryID {
		t.Errorf("RepositoryID = %v, want %v", decoded.RepositoryID, repositoryID)
	}
	if decoded.Reason != "Repository closed" {
		t.Errorf("Reason = %v, want Repository closed", decoded.Reason)
	}
}

// Tests for AggregateID methods that were previously uncovered
func TestEventAggregateIDs(t *testing.T) {
	tests := []struct {
		name        string
		eventFunc   func() (Event, uuid.UUID)
		wantType    string
	}{
		{
			name: "PersonUpdated",
			eventFunc: func() (Event, uuid.UUID) {
				id := uuid.New()
				return NewPersonUpdated(id, nil), id
			},
			wantType: "PersonUpdated",
		},
		{
			name: "PersonDeleted",
			eventFunc: func() (Event, uuid.UUID) {
				id := uuid.New()
				return NewPersonDeleted(id, "test"), id
			},
			wantType: "PersonDeleted",
		},
		{
			name: "FamilyCreated",
			eventFunc: func() (Event, uuid.UUID) {
				p1 := uuid.New()
				f := NewFamilyWithPartners(&p1, nil)
				return NewFamilyCreated(f), f.ID
			},
			wantType: "FamilyCreated",
		},
		{
			name: "ChildLinkedToFamily",
			eventFunc: func() (Event, uuid.UUID) {
				familyID := uuid.New()
				fc := &FamilyChild{FamilyID: familyID, PersonID: uuid.New()}
				return NewChildLinkedToFamily(fc), familyID
			},
			wantType: "ChildLinkedToFamily",
		},
		{
			name: "ChildUnlinkedFromFamily",
			eventFunc: func() (Event, uuid.UUID) {
				familyID := uuid.New()
				return NewChildUnlinkedFromFamily(familyID, uuid.New()), familyID
			},
			wantType: "ChildUnlinkedFromFamily",
		},
		{
			name: "GedcomImported",
			eventFunc: func() (Event, uuid.UUID) {
				event := NewGedcomImported("test.ged", 100, 10, 5, nil, nil)
				return event, event.ImportID
			},
			wantType: "GedcomImported",
		},
		{
			name: "SourceUpdated",
			eventFunc: func() (Event, uuid.UUID) {
				id := uuid.New()
				return NewSourceUpdated(id, nil), id
			},
			wantType: "SourceUpdated",
		},
		{
			name: "SourceDeleted",
			eventFunc: func() (Event, uuid.UUID) {
				id := uuid.New()
				return NewSourceDeleted(id, "test"), id
			},
			wantType: "SourceDeleted",
		},
		{
			name: "CitationUpdated",
			eventFunc: func() (Event, uuid.UUID) {
				id := uuid.New()
				return NewCitationUpdated(id, nil), id
			},
			wantType: "CitationUpdated",
		},
		{
			name: "CitationDeleted",
			eventFunc: func() (Event, uuid.UUID) {
				id := uuid.New()
				return NewCitationDeleted(id, "test"), id
			},
			wantType: "CitationDeleted",
		},
		{
			name: "LifeEventDeleted",
			eventFunc: func() (Event, uuid.UUID) {
				id := uuid.New()
				return NewLifeEventDeleted(id, "test"), id
			},
			wantType: "LifeEventDeleted",
		},
		{
			name: "AttributeDeleted",
			eventFunc: func() (Event, uuid.UUID) {
				id := uuid.New()
				return NewAttributeDeleted(id, "test"), id
			},
			wantType: "AttributeDeleted",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event, expectedID := tt.eventFunc()

			if event.EventType() != tt.wantType {
				t.Errorf("EventType() = %v, want %v", event.EventType(), tt.wantType)
			}

			if event.AggregateID() != expectedID {
				t.Errorf("AggregateID() = %v, want %v", event.AggregateID(), expectedID)
			}

			if event.OccurredAt().IsZero() {
				t.Error("OccurredAt() returned zero time")
			}
		})
	}
}
