package command_test

import (
	"context"
	"encoding/json"
	"slices"
	"testing"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/command"
	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository/memory"
)

// The edit forms send every field, so an update only records the fields whose
// value differs from the current one (#900). Each case submits the full form
// with the given fields edited and checks the stored event's change keys; no
// edited fields means no event is appended at all.

// lastChangeKeys returns the sorted change keys of a stream's last event and
// the stream's length.
func lastChangeKeys(t *testing.T, events *memory.EventStore, id uuid.UUID) (keys []string, length int) {
	t.Helper()
	stream, err := events.ReadStream(context.Background(), id)
	if err != nil {
		t.Fatalf("ReadStream: %v", err)
	}
	var payload struct {
		Changes map[string]json.RawMessage `json:"changes"`
	}
	if err := json.Unmarshal(stream[len(stream)-1].Data, &payload); err != nil {
		t.Fatalf("decode stored event: %v", err)
	}
	for key := range payload.Changes {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys, len(stream)
}

// checkRecorded asserts an update appended one event carrying exactly want,
// or nothing when want is empty.
func checkRecorded(t *testing.T, events *memory.EventStore, id uuid.UUID, before int, want []string) {
	t.Helper()
	keys, after := lastChangeKeys(t, events, id)
	if len(want) == 0 {
		if after != before {
			t.Errorf("stream grew from %d to %d events, want no event for an unchanged form", before, after)
		}
		return
	}
	if after != before+1 {
		t.Fatalf("stream grew from %d to %d events, want one update", before, after)
	}
	if !slices.Equal(keys, want) {
		t.Errorf("recorded changes = %v, want %v", keys, want)
	}
}

func TestUpdatePerson_RecordsOnlyChangedFields(t *testing.T) {
	tests := []struct {
		name string
		edit func(*command.UpdatePersonInput)
		want []string
	}{
		{"unchanged form", func(*command.UpdatePersonInput) {}, nil},
		{"birth place only", func(in *command.UpdatePersonInput) { in.BirthPlace = ptr("Boston") }, []string{"birth_place"}},
		{"birth date only", func(in *command.UpdatePersonInput) { in.BirthDate = ptr("1876") }, []string{"birth_date"}},
		{"gender and status", func(in *command.UpdatePersonInput) {
			in.Gender = ptr("male")
			in.ResearchStatus = ptr("certain")
		}, []string{"gender", "research_status"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			events, read := memory.NewEventStore(), memory.NewReadModelStore()
			handler := command.NewHandler(events, read)
			created, err := handler.CreatePerson(ctx, command.CreatePersonInput{
				GivenName: "Ada", Surname: "Lovelace", Gender: "female",
				BirthDate: "1875", BirthPlace: "London", DeathDate: "1952", DeathPlace: "Leeds",
				Notes: "notes", ResearchStatus: "probable",
			})
			if err != nil {
				t.Fatalf("CreatePerson: %v", err)
			}
			input := command.UpdatePersonInput{
				ID: created.ID, GivenName: ptr("Ada"), Surname: ptr("Lovelace"), Gender: ptr("female"),
				BirthDate: ptr("1875"), BirthPlace: ptr("London"), DeathDate: ptr("1952"), DeathPlace: ptr("Leeds"),
				Notes: ptr("notes"), ResearchStatus: ptr("probable"), Version: created.Version,
			}
			tt.edit(&input)
			if _, err := handler.UpdatePerson(ctx, input); err != nil {
				t.Fatalf("UpdatePerson: %v", err)
			}
			checkRecorded(t, events, created.ID, 1, tt.want)
		})
	}
}

func TestUpdateFamily_RecordsOnlyChangedFields(t *testing.T) {
	tests := []struct {
		name string
		edit func(*command.UpdateFamilyInput, *familyFixture)
		want []string
	}{
		{"unchanged form", func(*command.UpdateFamilyInput, *familyFixture) {}, nil},
		{"marriage place only", func(in *command.UpdateFamilyInput, _ *familyFixture) { in.MarriagePlace = ptr("York") }, []string{"marriage_place"}},
		{"partner only", func(in *command.UpdateFamilyInput, f *familyFixture) { in.Partner2ID = &f.p3 }, []string{"partner2_id"}},
		{"type and date", func(in *command.UpdateFamilyInput, _ *familyFixture) {
			in.RelationshipType = ptr("partnership")
			in.MarriageDate = ptr("")
		}, []string{"marriage_date", "relationship_type"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFamilyFixture(t)
			input := command.UpdateFamilyInput{
				ID: f.familyID, Partner1ID: &f.p1, Partner2ID: &f.p2, RelationshipType: ptr("marriage"),
				MarriageDate: ptr("1 JUN 1875"), MarriagePlace: ptr(""), Version: f.version,
			}
			tt.edit(&input, f)
			if _, err := f.handler.UpdateFamily(f.ctx, input); err != nil {
				t.Fatalf("UpdateFamily: %v", err)
			}
			checkRecorded(t, f.events, f.familyID, 1, tt.want)
		})
	}
}

func TestUpdateSource_RecordsOnlyChangedFields(t *testing.T) {
	tests := []struct {
		name string
		edit func(*command.UpdateSourceInput)
		want []string
	}{
		{"unchanged form", func(*command.UpdateSourceInput) {}, nil},
		{"title only", func(in *command.UpdateSourceInput) { in.Title = ptr("1881 Census") }, []string{"title"}},
		{"type and date", func(in *command.UpdateSourceInput) {
			in.SourceType = ptr("book")
			in.PublishDate = ptr("1882")
		}, []string{"publish_date", "source_type"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			events, read := memory.NewEventStore(), memory.NewReadModelStore()
			handler := command.NewHandler(events, read)
			created, err := handler.CreateSource(ctx, command.CreateSourceInput{
				SourceType: "census", Title: "1880 Census", Author: "Bureau", Publisher: "GPO",
				PublishDate: "1881", URL: "https://example.org", RepositoryName: "Archive",
				CollectionName: "Population", CallNumber: "T9", Notes: "notes",
			})
			if err != nil {
				t.Fatalf("CreateSource: %v", err)
			}
			input := command.UpdateSourceInput{
				ID: created.ID, SourceType: ptr("census"), Title: ptr("1880 Census"), Author: ptr("Bureau"),
				Publisher: ptr("GPO"), PublishDate: ptr("1881"), URL: ptr("https://example.org"),
				RepositoryName: ptr("Archive"), CollectionName: ptr("Population"), CallNumber: ptr("T9"),
				Notes: ptr("notes"), Version: created.Version,
			}
			tt.edit(&input)
			if _, err := handler.UpdateSource(ctx, input); err != nil {
				t.Fatalf("UpdateSource: %v", err)
			}
			checkRecorded(t, events, created.ID, 1, tt.want)
		})
	}
}

func TestUpdateRepository_RecordsOnlyChangedFields(t *testing.T) {
	tests := []struct {
		name    string
		address *domain.Address // the repository's address at creation
		edit    func(*command.UpdateRepositoryInput)
		want    []string
	}{
		{"unchanged form", &domain.Address{City: "Salt Lake City"}, func(*command.UpdateRepositoryInput) {}, nil},
		{"blank address on none", nil, func(in *command.UpdateRepositoryInput) { in.Address = &domain.Address{} }, nil},
		{"address only", &domain.Address{City: "Salt Lake City"}, func(in *command.UpdateRepositoryInput) {
			in.Address = &domain.Address{City: "Provo"}
		}, []string{"address"}},
		{"name only", &domain.Address{City: "Salt Lake City"}, func(in *command.UpdateRepositoryInput) { in.Name = ptr("FHL") }, []string{"name"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			events, read := memory.NewEventStore(), memory.NewReadModelStore()
			handler := command.NewHandler(events, read)
			created, err := handler.CreateRepository(ctx, command.CreateRepositoryInput{
				Name: "Family History Library", Address: tt.address, Notes: "notes",
			})
			if err != nil {
				t.Fatalf("CreateRepository: %v", err)
			}
			input := command.UpdateRepositoryInput{
				ID: created.ID, Name: ptr("Family History Library"), Notes: ptr("notes"), Version: created.Version,
			}
			if tt.address != nil {
				addr := *tt.address
				input.Address = &addr
			}
			tt.edit(&input)
			if _, err := handler.UpdateRepository(ctx, input); err != nil {
				t.Fatalf("UpdateRepository: %v", err)
			}
			checkRecorded(t, events, created.ID, 1, tt.want)
		})
	}
}
