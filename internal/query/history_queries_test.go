package query

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// mockEventStore implements repository.EventStore for testing.
type mockEventStore struct {
	readByStreamFunc     func(ctx context.Context, streamID uuid.UUID, limit, offset int) (*repository.HistoryPage, error)
	readGlobalByTimeFunc func(ctx context.Context, fromTime, toTime time.Time, eventTypes []string, limit, offset int) (*repository.HistoryPage, error)
}

func (m *mockEventStore) Append(ctx context.Context, streamID uuid.UUID, streamType string, events []domain.Event, expectedVersion int64) error {
	return nil
}

func (m *mockEventStore) ReadStream(ctx context.Context, streamID uuid.UUID) ([]repository.StoredEvent, error) {
	return nil, nil
}

func (m *mockEventStore) ReadAll(ctx context.Context, fromPosition int64, limit int) ([]repository.StoredEvent, error) {
	return nil, nil
}

func (m *mockEventStore) GetStreamVersion(ctx context.Context, streamID uuid.UUID) (int64, error) {
	return 0, nil
}

func (m *mockEventStore) ReadByStream(ctx context.Context, streamID uuid.UUID, limit, offset int) (*repository.HistoryPage, error) {
	if m.readByStreamFunc != nil {
		return m.readByStreamFunc(ctx, streamID, limit, offset)
	}
	return &repository.HistoryPage{}, nil
}

func (m *mockEventStore) ReadGlobalByTime(ctx context.Context, fromTime, toTime time.Time, eventTypes []string, limit, offset int) (*repository.HistoryPage, error) {
	if m.readGlobalByTimeFunc != nil {
		return m.readGlobalByTimeFunc(ctx, fromTime, toTime, eventTypes, limit, offset)
	}
	return &repository.HistoryPage{}, nil
}

// mockReadModelStore implements repository.ReadModelStore for testing.
type mockReadModelStore struct {
	getPersonFunc   func(ctx context.Context, id uuid.UUID) (*repository.PersonReadModel, error)
	getFamilyFunc   func(ctx context.Context, id uuid.UUID) (*repository.FamilyReadModel, error)
	getSourceFunc   func(ctx context.Context, id uuid.UUID) (*repository.SourceReadModel, error)
	getCitationFunc func(ctx context.Context, id uuid.UUID) (*repository.CitationReadModel, error)
}

func (m *mockReadModelStore) GetPerson(ctx context.Context, id uuid.UUID) (*repository.PersonReadModel, error) {
	if m.getPersonFunc != nil {
		return m.getPersonFunc(ctx, id)
	}
	return nil, repository.ErrStreamNotFound
}

func (m *mockReadModelStore) GetFamily(ctx context.Context, id uuid.UUID) (*repository.FamilyReadModel, error) {
	if m.getFamilyFunc != nil {
		return m.getFamilyFunc(ctx, id)
	}
	return nil, repository.ErrStreamNotFound
}

func (m *mockReadModelStore) GetSource(ctx context.Context, id uuid.UUID) (*repository.SourceReadModel, error) {
	if m.getSourceFunc != nil {
		return m.getSourceFunc(ctx, id)
	}
	return nil, repository.ErrStreamNotFound
}

func (m *mockReadModelStore) GetCitation(ctx context.Context, id uuid.UUID) (*repository.CitationReadModel, error) {
	if m.getCitationFunc != nil {
		return m.getCitationFunc(ctx, id)
	}
	return nil, repository.ErrStreamNotFound
}

// Stub methods for other ReadModelStore methods
func (m *mockReadModelStore) ListPersons(ctx context.Context, opts repository.ListOptions) ([]repository.PersonReadModel, int, error) {
	return nil, 0, nil
}
func (m *mockReadModelStore) SearchPersons(ctx context.Context, opts repository.SearchOptions) ([]repository.PersonReadModel, error) {
	return nil, nil
}
func (m *mockReadModelStore) SavePerson(ctx context.Context, person *repository.PersonReadModel) error {
	return nil
}
func (m *mockReadModelStore) DeletePerson(ctx context.Context, id uuid.UUID) error {
	return nil
}

// Person name stub methods
func (m *mockReadModelStore) SavePersonName(ctx context.Context, name *repository.PersonNameReadModel) error {
	return nil
}
func (m *mockReadModelStore) GetPersonName(ctx context.Context, nameID uuid.UUID) (*repository.PersonNameReadModel, error) {
	return nil, nil
}
func (m *mockReadModelStore) GetPersonNames(ctx context.Context, personID uuid.UUID) ([]repository.PersonNameReadModel, error) {
	return nil, nil
}
func (m *mockReadModelStore) DeletePersonName(ctx context.Context, nameID uuid.UUID) error {
	return nil
}

func (m *mockReadModelStore) ListFamilies(ctx context.Context, opts repository.ListOptions) ([]repository.FamilyReadModel, int, error) {
	return nil, 0, nil
}
func (m *mockReadModelStore) GetFamiliesForPerson(ctx context.Context, personID uuid.UUID) ([]repository.FamilyReadModel, error) {
	return nil, nil
}
func (m *mockReadModelStore) SaveFamily(ctx context.Context, family *repository.FamilyReadModel) error {
	return nil
}
func (m *mockReadModelStore) DeleteFamily(ctx context.Context, id uuid.UUID) error {
	return nil
}
func (m *mockReadModelStore) GetFamilyChildren(ctx context.Context, familyID uuid.UUID) ([]repository.FamilyChildReadModel, error) {
	return nil, nil
}
func (m *mockReadModelStore) GetChildrenOfFamily(ctx context.Context, familyID uuid.UUID) ([]repository.PersonReadModel, error) {
	return nil, nil
}
func (m *mockReadModelStore) GetChildFamily(ctx context.Context, personID uuid.UUID) (*repository.FamilyReadModel, error) {
	return nil, nil
}
func (m *mockReadModelStore) SaveFamilyChild(ctx context.Context, child *repository.FamilyChildReadModel) error {
	return nil
}
func (m *mockReadModelStore) DeleteFamilyChild(ctx context.Context, familyID, personID uuid.UUID) error {
	return nil
}
func (m *mockReadModelStore) GetPedigreeEdge(ctx context.Context, personID uuid.UUID) (*repository.PedigreeEdge, error) {
	return nil, nil
}
func (m *mockReadModelStore) SavePedigreeEdge(ctx context.Context, edge *repository.PedigreeEdge) error {
	return nil
}
func (m *mockReadModelStore) DeletePedigreeEdge(ctx context.Context, personID uuid.UUID) error {
	return nil
}
func (m *mockReadModelStore) ListSources(ctx context.Context, opts repository.ListOptions) ([]repository.SourceReadModel, int, error) {
	return nil, 0, nil
}
func (m *mockReadModelStore) SearchSources(ctx context.Context, query string, limit int) ([]repository.SourceReadModel, error) {
	return nil, nil
}
func (m *mockReadModelStore) SaveSource(ctx context.Context, source *repository.SourceReadModel) error {
	return nil
}
func (m *mockReadModelStore) DeleteSource(ctx context.Context, id uuid.UUID) error {
	return nil
}
func (m *mockReadModelStore) ListCitations(ctx context.Context, opts repository.ListOptions) ([]repository.CitationReadModel, int, error) {
	return nil, 0, nil
}
func (m *mockReadModelStore) GetCitationsForSource(ctx context.Context, sourceID uuid.UUID) ([]repository.CitationReadModel, error) {
	return nil, nil
}
func (m *mockReadModelStore) GetCitationsForPerson(ctx context.Context, personID uuid.UUID) ([]repository.CitationReadModel, error) {
	return nil, nil
}
func (m *mockReadModelStore) GetCitationsForFact(ctx context.Context, factType domain.FactType, factOwnerID uuid.UUID) ([]repository.CitationReadModel, error) {
	return nil, nil
}
func (m *mockReadModelStore) SaveCitation(ctx context.Context, citation *repository.CitationReadModel) error {
	return nil
}
func (m *mockReadModelStore) DeleteCitation(ctx context.Context, id uuid.UUID) error {
	return nil
}

// Media stub methods
func (m *mockReadModelStore) GetMedia(ctx context.Context, id uuid.UUID) (*repository.MediaReadModel, error) {
	return nil, nil
}
func (m *mockReadModelStore) GetMediaWithData(ctx context.Context, id uuid.UUID) (*repository.MediaReadModel, error) {
	return nil, nil
}
func (m *mockReadModelStore) GetMediaThumbnail(ctx context.Context, id uuid.UUID) ([]byte, error) {
	return nil, nil
}
func (m *mockReadModelStore) ListMediaForEntity(ctx context.Context, entityType string, entityID uuid.UUID, opts repository.ListOptions) ([]repository.MediaReadModel, int, error) {
	return nil, 0, nil
}
func (m *mockReadModelStore) SaveMedia(ctx context.Context, media *repository.MediaReadModel) error {
	return nil
}
func (m *mockReadModelStore) DeleteMedia(ctx context.Context, id uuid.UUID) error {
	return nil
}

// Event stub methods
func (m *mockReadModelStore) GetEvent(ctx context.Context, id uuid.UUID) (*repository.EventReadModel, error) {
	return nil, nil
}
func (m *mockReadModelStore) ListEvents(ctx context.Context, opts repository.ListOptions) ([]repository.EventReadModel, int, error) {
	return nil, 0, nil
}
func (m *mockReadModelStore) ListEventsForPerson(ctx context.Context, personID uuid.UUID) ([]repository.EventReadModel, error) {
	return nil, nil
}
func (m *mockReadModelStore) ListEventsForFamily(ctx context.Context, familyID uuid.UUID) ([]repository.EventReadModel, error) {
	return nil, nil
}
func (m *mockReadModelStore) SaveEvent(ctx context.Context, event *repository.EventReadModel) error {
	return nil
}
func (m *mockReadModelStore) DeleteEvent(ctx context.Context, id uuid.UUID) error {
	return nil
}

// Attribute stub methods
func (m *mockReadModelStore) GetAttribute(ctx context.Context, id uuid.UUID) (*repository.AttributeReadModel, error) {
	return nil, nil
}
func (m *mockReadModelStore) ListAttributes(ctx context.Context, opts repository.ListOptions) ([]repository.AttributeReadModel, int, error) {
	return nil, 0, nil
}
func (m *mockReadModelStore) ListAttributesForPerson(ctx context.Context, personID uuid.UUID) ([]repository.AttributeReadModel, error) {
	return nil, nil
}
func (m *mockReadModelStore) SaveAttribute(ctx context.Context, attribute *repository.AttributeReadModel) error {
	return nil
}
func (m *mockReadModelStore) DeleteAttribute(ctx context.Context, id uuid.UUID) error {
	return nil
}

// Browse stub methods
func (m *mockReadModelStore) GetSurnameIndex(ctx context.Context) ([]repository.SurnameEntry, []repository.LetterCount, error) {
	return nil, nil, nil
}
func (m *mockReadModelStore) GetSurnamesByLetter(ctx context.Context, letter string) ([]repository.SurnameEntry, error) {
	return nil, nil
}
func (m *mockReadModelStore) GetPersonsBySurname(ctx context.Context, surname string, opts repository.ListOptions) ([]repository.PersonReadModel, int, error) {
	return nil, 0, nil
}
func (m *mockReadModelStore) GetPlaceHierarchy(ctx context.Context, parent string) ([]repository.PlaceEntry, error) {
	return nil, nil
}
func (m *mockReadModelStore) GetPersonsByPlace(ctx context.Context, place string, opts repository.ListOptions) ([]repository.PersonReadModel, int, error) {
	return nil, 0, nil
}
func (m *mockReadModelStore) GetCemeteryIndex(ctx context.Context) ([]repository.CemeteryEntry, error) {
	return nil, nil
}
func (m *mockReadModelStore) GetPersonsByCemetery(ctx context.Context, place string, opts repository.ListOptions) ([]repository.PersonReadModel, int, error) {
	return nil, 0, nil
}
func (m *mockReadModelStore) GetMapLocations(ctx context.Context) ([]repository.MapLocation, error) {
	return nil, nil
}

// Brick wall stub methods
func (m *mockReadModelStore) SetBrickWall(ctx context.Context, personID uuid.UUID, note string) error {
	return nil
}
func (m *mockReadModelStore) ResolveBrickWall(ctx context.Context, personID uuid.UUID) error {
	return nil
}
func (m *mockReadModelStore) GetBrickWalls(ctx context.Context, includeResolved bool) ([]repository.BrickWallEntry, error) {
	return nil, nil
}

// Note stub methods
func (m *mockReadModelStore) GetNote(ctx context.Context, id uuid.UUID) (*repository.NoteReadModel, error) {
	return nil, nil
}
func (m *mockReadModelStore) ListNotes(ctx context.Context, opts repository.ListOptions) ([]repository.NoteReadModel, int, error) {
	return nil, 0, nil
}
func (m *mockReadModelStore) SaveNote(ctx context.Context, note *repository.NoteReadModel) error {
	return nil
}
func (m *mockReadModelStore) DeleteNote(ctx context.Context, id uuid.UUID) error {
	return nil
}

// Submitter stub methods
func (m *mockReadModelStore) GetSubmitter(ctx context.Context, id uuid.UUID) (*repository.SubmitterReadModel, error) {
	return nil, nil
}
func (m *mockReadModelStore) ListSubmitters(ctx context.Context, opts repository.ListOptions) ([]repository.SubmitterReadModel, int, error) {
	return nil, 0, nil
}
func (m *mockReadModelStore) SaveSubmitter(ctx context.Context, submitter *repository.SubmitterReadModel) error {
	return nil
}
func (m *mockReadModelStore) DeleteSubmitter(ctx context.Context, id uuid.UUID) error {
	return nil
}

// Association stub methods
func (m *mockReadModelStore) GetAssociation(ctx context.Context, id uuid.UUID) (*repository.AssociationReadModel, error) {
	return nil, nil
}
func (m *mockReadModelStore) ListAssociations(ctx context.Context, opts repository.ListOptions) ([]repository.AssociationReadModel, int, error) {
	return nil, 0, nil
}
func (m *mockReadModelStore) ListAssociationsForPerson(ctx context.Context, personID uuid.UUID) ([]repository.AssociationReadModel, error) {
	return nil, nil
}
func (m *mockReadModelStore) SaveAssociation(ctx context.Context, association *repository.AssociationReadModel) error {
	return nil
}
func (m *mockReadModelStore) DeleteAssociation(ctx context.Context, id uuid.UUID) error {
	return nil
}

// LDS Ordinance stub methods
func (m *mockReadModelStore) GetLDSOrdinance(ctx context.Context, id uuid.UUID) (*repository.LDSOrdinanceReadModel, error) {
	return nil, nil
}
func (m *mockReadModelStore) ListLDSOrdinances(ctx context.Context, opts repository.ListOptions) ([]repository.LDSOrdinanceReadModel, int, error) {
	return nil, 0, nil
}
func (m *mockReadModelStore) ListLDSOrdinancesForPerson(ctx context.Context, personID uuid.UUID) ([]repository.LDSOrdinanceReadModel, error) {
	return nil, nil
}
func (m *mockReadModelStore) ListLDSOrdinancesForFamily(ctx context.Context, familyID uuid.UUID) ([]repository.LDSOrdinanceReadModel, error) {
	return nil, nil
}
func (m *mockReadModelStore) SaveLDSOrdinance(ctx context.Context, ordinance *repository.LDSOrdinanceReadModel) error {
	return nil
}
func (m *mockReadModelStore) DeleteLDSOrdinance(ctx context.Context, id uuid.UUID) error {
	return nil
}

func TestNewHistoryService(t *testing.T) {
	eventStore := &mockEventStore{}
	readStore := &mockReadModelStore{}

	service := NewHistoryService(eventStore, readStore)

	assert.NotNil(t, service)
	assert.Equal(t, eventStore, service.eventStore)
	assert.Equal(t, readStore, service.readStore)
}

func TestGetEntityHistory(t *testing.T) {
	personID := uuid.New()
	now := time.Now().UTC()

	personCreatedEvent := domain.NewPersonCreated(&domain.Person{
		ID:        personID,
		GivenName: "John",
		Surname:   "Smith",
	})
	createdData, _ := json.Marshal(personCreatedEvent)

	tests := []struct {
		name           string
		entityType     string
		entityID       uuid.UUID
		limit          int
		offset         int
		mockEvents     []repository.StoredEvent
		mockTotalCount int
		mockHasMore    bool
		mockPerson     *repository.PersonReadModel
		wantEntries    int
		wantEntityType string
		wantAction     string
		wantName       string
	}{
		{
			name:       "person creation event",
			entityType: "person",
			entityID:   personID,
			limit:      20,
			offset:     0,
			mockEvents: []repository.StoredEvent{
				{
					ID:         uuid.New(),
					StreamID:   personID,
					StreamType: "person",
					EventType:  "PersonCreated",
					Data:       createdData,
					Version:    1,
					Position:   1,
					Timestamp:  now,
				},
			},
			mockTotalCount: 1,
			mockHasMore:    false,
			mockPerson: &repository.PersonReadModel{
				ID:       personID,
				FullName: "John Smith",
			},
			wantEntries:    1,
			wantEntityType: "person",
			wantAction:     "created",
			wantName:       "John Smith",
		},
		{
			name:           "empty history",
			entityType:     "person",
			entityID:       personID,
			limit:          20,
			offset:         0,
			mockEvents:     []repository.StoredEvent{},
			mockTotalCount: 0,
			mockHasMore:    false,
			wantEntries:    0,
		},
		{
			name:       "default limit",
			entityType: "person",
			entityID:   personID,
			limit:      0, // Should default to 20
			offset:     0,
			mockEvents: []repository.StoredEvent{
				{
					ID:         uuid.New(),
					StreamID:   personID,
					StreamType: "person",
					EventType:  "PersonCreated",
					Data:       createdData,
					Version:    1,
					Position:   1,
					Timestamp:  now,
				},
			},
			mockTotalCount: 1,
			mockHasMore:    false,
			mockPerson: &repository.PersonReadModel{
				ID:       personID,
				FullName: "John Smith",
			},
			wantEntries:    1,
			wantEntityType: "person",
			wantAction:     "created",
			wantName:       "John Smith",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eventStore := &mockEventStore{
				readByStreamFunc: func(ctx context.Context, streamID uuid.UUID, limit, offset int) (*repository.HistoryPage, error) {
					assert.Equal(t, tt.entityID, streamID)
					return &repository.HistoryPage{
						Events:     tt.mockEvents,
						TotalCount: tt.mockTotalCount,
						HasMore:    tt.mockHasMore,
					}, nil
				},
			}

			readStore := &mockReadModelStore{
				getPersonFunc: func(ctx context.Context, id uuid.UUID) (*repository.PersonReadModel, error) {
					if tt.mockPerson != nil && id == tt.entityID {
						return tt.mockPerson, nil
					}
					return nil, repository.ErrStreamNotFound
				},
			}

			service := NewHistoryService(eventStore, readStore)
			result, err := service.GetEntityHistory(context.Background(), tt.entityType, tt.entityID, tt.limit, tt.offset)

			require.NoError(t, err)
			assert.Equal(t, tt.wantEntries, len(result.Entries))
			assert.Equal(t, tt.mockTotalCount, result.TotalCount)
			assert.Equal(t, tt.mockHasMore, result.HasMore)

			if tt.wantEntries > 0 {
				entry := result.Entries[0]
				assert.Equal(t, tt.wantEntityType, entry.EntityType)
				assert.Equal(t, tt.wantAction, entry.Action)
				assert.Equal(t, tt.wantName, entry.EntityName)
			}
		})
	}
}

func TestGetGlobalHistory(t *testing.T) {
	fromTime := time.Now().Add(-24 * time.Hour).UTC()
	toTime := time.Now().UTC()
	personID := uuid.New()

	personCreatedEvent := domain.NewPersonCreated(&domain.Person{
		ID:        personID,
		GivenName: "John",
		Surname:   "Smith",
	})
	createdData, _ := json.Marshal(personCreatedEvent)

	tests := []struct {
		name           string
		input          GetGlobalHistoryInput
		mockEvents     []repository.StoredEvent
		mockTotalCount int
		mockHasMore    bool
		mockPerson     *repository.PersonReadModel
		wantEntries    int
		wantEntityType string
	}{
		{
			name: "global history with time range",
			input: GetGlobalHistoryInput{
				FromTime:   fromTime,
				ToTime:     toTime,
				EventTypes: []string{"PersonCreated"},
				Limit:      20,
				Offset:     0,
			},
			mockEvents: []repository.StoredEvent{
				{
					ID:         uuid.New(),
					StreamID:   personID,
					StreamType: "person",
					EventType:  "PersonCreated",
					Data:       createdData,
					Version:    1,
					Position:   1,
					Timestamp:  fromTime.Add(1 * time.Hour),
				},
			},
			mockTotalCount: 1,
			mockHasMore:    false,
			mockPerson: &repository.PersonReadModel{
				ID:       personID,
				FullName: "John Smith",
			},
			wantEntries:    1,
			wantEntityType: "person",
		},
		{
			name: "default limit",
			input: GetGlobalHistoryInput{
				FromTime:   fromTime,
				ToTime:     toTime,
				EventTypes: nil,
				Limit:      0, // Should default to 20
				Offset:     0,
			},
			mockEvents:     []repository.StoredEvent{},
			mockTotalCount: 0,
			mockHasMore:    false,
			wantEntries:    0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eventStore := &mockEventStore{
				readGlobalByTimeFunc: func(ctx context.Context, fromTime, toTime time.Time, eventTypes []string, limit, offset int) (*repository.HistoryPage, error) {
					return &repository.HistoryPage{
						Events:     tt.mockEvents,
						TotalCount: tt.mockTotalCount,
						HasMore:    tt.mockHasMore,
					}, nil
				},
			}

			readStore := &mockReadModelStore{
				getPersonFunc: func(ctx context.Context, id uuid.UUID) (*repository.PersonReadModel, error) {
					if tt.mockPerson != nil && id == personID {
						return tt.mockPerson, nil
					}
					return nil, repository.ErrStreamNotFound
				},
			}

			service := NewHistoryService(eventStore, readStore)
			result, err := service.GetGlobalHistory(context.Background(), tt.input)

			require.NoError(t, err)
			assert.Equal(t, tt.wantEntries, len(result.Entries))
			assert.Equal(t, tt.mockTotalCount, result.TotalCount)
			assert.Equal(t, tt.mockHasMore, result.HasMore)

			if tt.wantEntries > 0 {
				entry := result.Entries[0]
				assert.Equal(t, tt.wantEntityType, entry.EntityType)
			}
		})
	}
}

func TestMapEventTypeToEntityAndAction(t *testing.T) {
	service := &HistoryService{}

	tests := []struct {
		eventType      string
		wantEntityType string
		wantAction     string
	}{
		{"PersonCreated", "person", "created"},
		{"PersonUpdated", "person", "updated"},
		{"PersonDeleted", "person", "deleted"},
		{"FamilyCreated", "family", "created"},
		{"FamilyUpdated", "family", "updated"},
		{"FamilyDeleted", "family", "deleted"},
		{"ChildLinkedToFamily", "family", "linked"},
		{"ChildUnlinkedFromFamily", "family", "unlinked"},
		{"SourceCreated", "source", "created"},
		{"SourceUpdated", "source", "updated"},
		{"SourceDeleted", "source", "deleted"},
		{"CitationCreated", "citation", "created"},
		{"CitationUpdated", "citation", "updated"},
		{"CitationDeleted", "citation", "deleted"},
		{"GedcomImported", "import", "created"},
		{"UnknownEvent", "unknown", "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.eventType, func(t *testing.T) {
			entityType, action := service.mapEventTypeToEntityAndAction(tt.eventType)
			assert.Equal(t, tt.wantEntityType, entityType)
			assert.Equal(t, tt.wantAction, action)
		})
	}
}

func TestExtractChanges(t *testing.T) {
	service := &HistoryService{}

	tests := []struct {
		name        string
		event       repository.StoredEvent
		wantChanges bool
		wantFields  []string
	}{
		{
			name: "person updated event",
			event: repository.StoredEvent{
				EventType: "PersonUpdated",
				Data: mustMarshal(domain.PersonUpdated{
					PersonID: uuid.New(),
					Changes: map[string]any{
						"given_name": "Jane",
						"surname":    "Doe",
					},
				}),
			},
			wantChanges: true,
			wantFields:  []string{"given_name", "surname"},
		},
		{
			name: "family updated event",
			event: repository.StoredEvent{
				EventType: "FamilyUpdated",
				Data: mustMarshal(domain.FamilyUpdated{
					FamilyID: uuid.New(),
					Changes: map[string]any{
						"marriage_place": "New York",
					},
				}),
			},
			wantChanges: true,
			wantFields:  []string{"marriage_place"},
		},
		{
			name: "creation event has no changes",
			event: repository.StoredEvent{
				EventType: "PersonCreated",
				Data: mustMarshal(domain.PersonCreated{
					PersonID:  uuid.New(),
					GivenName: "John",
					Surname:   "Smith",
				}),
			},
			wantChanges: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			changes, err := service.extractChanges(tt.event)
			require.NoError(t, err)

			if tt.wantChanges {
				assert.NotNil(t, changes)
				assert.Equal(t, len(tt.wantFields), len(changes))
				for _, field := range tt.wantFields {
					assert.Contains(t, changes, field)
				}
			} else {
				assert.Nil(t, changes)
			}
		})
	}
}

func TestGetEntityName(t *testing.T) {
	personID := uuid.New()
	familyID := uuid.New()
	sourceID := uuid.New()
	citationID := uuid.New()
	importID := uuid.New()

	readStore := &mockReadModelStore{
		getPersonFunc: func(ctx context.Context, id uuid.UUID) (*repository.PersonReadModel, error) {
			if id == personID {
				return &repository.PersonReadModel{
					ID:       personID,
					FullName: "John Smith",
				}, nil
			}
			return nil, repository.ErrStreamNotFound
		},
		getFamilyFunc: func(ctx context.Context, id uuid.UUID) (*repository.FamilyReadModel, error) {
			if id == familyID {
				return &repository.FamilyReadModel{
					ID:           familyID,
					Partner1Name: "John Smith",
					Partner2Name: "Jane Doe",
				}, nil
			}
			return nil, repository.ErrStreamNotFound
		},
		getSourceFunc: func(ctx context.Context, id uuid.UUID) (*repository.SourceReadModel, error) {
			if id == sourceID {
				return &repository.SourceReadModel{
					ID:    sourceID,
					Title: "1900 Census",
				}, nil
			}
			return nil, repository.ErrStreamNotFound
		},
		getCitationFunc: func(ctx context.Context, id uuid.UUID) (*repository.CitationReadModel, error) {
			if id == citationID {
				return &repository.CitationReadModel{
					ID:          citationID,
					SourceTitle: "1900 Census",
					FactType:    domain.FactPersonBirth,
				}, nil
			}
			return nil, repository.ErrStreamNotFound
		},
	}

	service := NewHistoryService(&mockEventStore{}, readStore)

	tests := []struct {
		name       string
		entityType string
		entityID   uuid.UUID
		event      *repository.StoredEvent
		wantName   string
	}{
		{
			name:       "person from read model",
			entityType: "person",
			entityID:   personID,
			event:      &repository.StoredEvent{EventType: "PersonCreated"},
			wantName:   "John Smith",
		},
		{
			name:       "family from read model",
			entityType: "family",
			entityID:   familyID,
			event:      &repository.StoredEvent{EventType: "FamilyCreated"},
			wantName:   "John Smith & Jane Doe",
		},
		{
			name:       "source from read model",
			entityType: "source",
			entityID:   sourceID,
			event:      &repository.StoredEvent{EventType: "SourceCreated"},
			wantName:   "1900 Census",
		},
		{
			name:       "citation from read model",
			entityType: "citation",
			entityID:   citationID,
			event:      &repository.StoredEvent{EventType: "CitationCreated"},
			wantName:   "1900 Census (person_birth)",
		},
		{
			name:       "import event",
			entityType: "import",
			entityID:   importID,
			event: &repository.StoredEvent{
				EventType: "GedcomImported",
				Data:      mustMarshal(domain.NewGedcomImported("family.ged", 1024, 10, 5, nil, nil)),
			},
			wantName: "GEDCOM Import: family.ged",
		},
		{
			name:       "unknown entity type",
			entityType: "unknown",
			entityID:   uuid.New(),
			event:      &repository.StoredEvent{EventType: "UnknownEvent"},
			wantName:   "", // Will be the UUID string
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			name := service.getEntityName(context.Background(), tt.entityType, tt.entityID, tt.event)
			if tt.wantName != "" {
				assert.Equal(t, tt.wantName, name)
			} else {
				// For unknown types, we expect the UUID
				assert.Equal(t, tt.entityID.String(), name)
			}
		})
	}
}

func TestGetPersonNameFallback(t *testing.T) {
	personID := uuid.New()
	deletedPersonID := uuid.New()

	readStore := &mockReadModelStore{
		getPersonFunc: func(ctx context.Context, id uuid.UUID) (*repository.PersonReadModel, error) {
			// Person exists in read model
			if id == personID {
				return &repository.PersonReadModel{
					ID:       personID,
					FullName: "John Smith",
				}, nil
			}
			// Deleted person not in read model
			return nil, repository.ErrStreamNotFound
		},
	}

	service := NewHistoryService(&mockEventStore{}, readStore)

	t.Run("person found in read model", func(t *testing.T) {
		evt := &repository.StoredEvent{
			EventType: "PersonCreated",
		}
		name := service.getPersonName(context.Background(), personID, evt)
		assert.Equal(t, "John Smith", name)
	})

	t.Run("person deleted, extract from creation event", func(t *testing.T) {
		evt := &repository.StoredEvent{
			EventType: "PersonCreated",
			Data: mustMarshal(domain.PersonCreated{
				PersonID:  deletedPersonID,
				GivenName: "Jane",
				Surname:   "Doe",
			}),
		}
		name := service.getPersonName(context.Background(), deletedPersonID, evt)
		assert.Equal(t, "Jane Doe", name)
	})

	t.Run("person deleted, no creation event data", func(t *testing.T) {
		evt := &repository.StoredEvent{
			EventType: "PersonDeleted",
		}
		name := service.getPersonName(context.Background(), deletedPersonID, evt)
		assert.Equal(t, deletedPersonID.String(), name)
	})
}

func TestGetFamilyNameVariations(t *testing.T) {
	familyID := uuid.New()

	tests := []struct {
		name     string
		family   *repository.FamilyReadModel
		wantName string
	}{
		{
			name: "both partners",
			family: &repository.FamilyReadModel{
				ID:           familyID,
				Partner1Name: "John Smith",
				Partner2Name: "Jane Doe",
			},
			wantName: "John Smith & Jane Doe",
		},
		{
			name: "partner1 only",
			family: &repository.FamilyReadModel{
				ID:           familyID,
				Partner1Name: "John Smith",
				Partner2Name: "",
			},
			wantName: "John Smith",
		},
		{
			name: "partner2 only",
			family: &repository.FamilyReadModel{
				ID:           familyID,
				Partner1Name: "",
				Partner2Name: "Jane Doe",
			},
			wantName: "Jane Doe",
		},
		{
			name: "no partners (deleted)",
			family: &repository.FamilyReadModel{
				ID:           familyID,
				Partner1Name: "",
				Partner2Name: "",
			},
			wantName: familyID.String(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			readStore := &mockReadModelStore{
				getFamilyFunc: func(ctx context.Context, id uuid.UUID) (*repository.FamilyReadModel, error) {
					if id == familyID {
						return tt.family, nil
					}
					return nil, repository.ErrStreamNotFound
				},
			}

			service := NewHistoryService(&mockEventStore{}, readStore)
			evt := &repository.StoredEvent{EventType: "FamilyCreated"}
			name := service.getFamilyName(context.Background(), familyID, evt)
			assert.Equal(t, tt.wantName, name)
		})
	}
}

func TestGetSourceNameFallback(t *testing.T) {
	sourceID := uuid.New()
	deletedSourceID := uuid.New()

	readStore := &mockReadModelStore{
		getSourceFunc: func(ctx context.Context, id uuid.UUID) (*repository.SourceReadModel, error) {
			// Source exists in read model
			if id == sourceID {
				return &repository.SourceReadModel{
					ID:    sourceID,
					Title: "1900 Census",
				}, nil
			}
			// Deleted source not in read model
			return nil, repository.ErrStreamNotFound
		},
	}

	service := NewHistoryService(&mockEventStore{}, readStore)

	t.Run("source found in read model", func(t *testing.T) {
		evt := &repository.StoredEvent{
			EventType: "SourceCreated",
		}
		name := service.getSourceName(context.Background(), sourceID, evt)
		assert.Equal(t, "1900 Census", name)
	})

	t.Run("source deleted, extract from creation event", func(t *testing.T) {
		evt := &repository.StoredEvent{
			EventType: "SourceCreated",
			Data: mustMarshal(domain.SourceCreated{
				SourceID: deletedSourceID,
				Title:    "1920 Census",
			}),
		}
		name := service.getSourceName(context.Background(), deletedSourceID, evt)
		assert.Equal(t, "1920 Census", name)
	})

	t.Run("source deleted, no creation event data", func(t *testing.T) {
		evt := &repository.StoredEvent{
			EventType: "SourceDeleted",
		}
		name := service.getSourceName(context.Background(), deletedSourceID, evt)
		assert.Equal(t, deletedSourceID.String(), name)
	})
}

func TestTransformStoredEvents(t *testing.T) {
	personID := uuid.New()
	now := time.Now().UTC()

	personCreatedEvent := domain.NewPersonCreated(&domain.Person{
		ID:        personID,
		GivenName: "John",
		Surname:   "Smith",
	})
	createdData, _ := json.Marshal(personCreatedEvent)

	personUpdatedEvent := domain.NewPersonUpdated(personID, map[string]any{
		"given_name": "Jane",
	})
	updatedData, _ := json.Marshal(personUpdatedEvent)

	userID := "user-123"
	metadata, _ := json.Marshal(domain.EventMetadata{
		UserID: userID,
	})

	readStore := &mockReadModelStore{
		getPersonFunc: func(ctx context.Context, id uuid.UUID) (*repository.PersonReadModel, error) {
			if id == personID {
				return &repository.PersonReadModel{
					ID:       personID,
					FullName: "John Smith",
				}, nil
			}
			return nil, repository.ErrStreamNotFound
		},
	}

	service := NewHistoryService(&mockEventStore{}, readStore)

	events := []repository.StoredEvent{
		{
			ID:         uuid.New(),
			StreamID:   personID,
			StreamType: "person",
			EventType:  "PersonCreated",
			Data:       createdData,
			Metadata:   metadata,
			Version:    1,
			Position:   1,
			Timestamp:  now,
		},
		{
			ID:         uuid.New(),
			StreamID:   personID,
			StreamType: "person",
			EventType:  "PersonUpdated",
			Data:       updatedData,
			Version:    2,
			Position:   2,
			Timestamp:  now.Add(1 * time.Hour),
		},
	}

	entries, err := service.transformStoredEvents(context.Background(), events)

	require.NoError(t, err)
	assert.Equal(t, 2, len(entries))

	// Check first entry (created)
	assert.Equal(t, "person", entries[0].EntityType)
	assert.Equal(t, "created", entries[0].Action)
	assert.Equal(t, "John Smith", entries[0].EntityName)
	assert.Equal(t, personID, entries[0].EntityID)
	assert.Nil(t, entries[0].Changes)
	assert.NotNil(t, entries[0].UserID)
	assert.Equal(t, userID, *entries[0].UserID)

	// Check second entry (updated)
	assert.Equal(t, "person", entries[1].EntityType)
	assert.Equal(t, "updated", entries[1].Action)
	assert.Equal(t, "John Smith", entries[1].EntityName)
	assert.NotNil(t, entries[1].Changes)
	assert.Contains(t, entries[1].Changes, "given_name")
}

// Helper function to marshal data for tests
func mustMarshal(v any) json.RawMessage {
	data, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return data
}
