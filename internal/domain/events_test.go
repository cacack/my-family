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
