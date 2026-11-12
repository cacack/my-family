package api_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/api"
	"github.com/cacack/my-family/internal/config"
	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/repository/memory"
)

// detailCountingStore counts the person sub-resource reads that only full
// PersonDetail assembly makes, so a test can tell an existence check from a
// detail build.
type detailCountingStore struct {
	repository.ReadModelStore
	detailReads int
}

func (s *detailCountingStore) GetPersonNames(ctx context.Context, branchID domain.BranchID, personID uuid.UUID) ([]repository.PersonNameReadModel, error) {
	s.detailReads++
	return s.ReadModelStore.GetPersonNames(ctx, branchID, personID)
}

func (s *detailCountingStore) GetPersonExternalIDs(ctx context.Context, branchID domain.BranchID, personID uuid.UUID) ([]repository.PersonExternalIDReadModel, error) {
	s.detailReads++
	return s.ReadModelStore.GetPersonExternalIDs(ctx, branchID, personID)
}

// TestPersonExistenceChecks_SkipDetailAssembly: endpoints that only need to
// know a person exists check the read model directly instead of assembling a
// full PersonDetail (#612), and still answer 404 for an unknown person.
func TestPersonExistenceChecks_SkipDetailAssembly(t *testing.T) {
	cfg := &config.Config{Port: 8080, LogFormat: "text"}
	events := memory.NewEventStore()
	reads := &detailCountingStore{ReadModelStore: memory.NewReadModelStore()}
	server := api.NewServer(cfg, events, reads, memory.NewSnapshotStore(events), nil)

	personID, _ := createMergeTestPerson(t, server, "Ada", "Lovelace")
	unknownID := uuid.New().String()

	tests := []struct {
		name, method, path, body string
		want                     int
	}{
		{"history", http.MethodGet, "/api/v1/persons/" + personID + "/history", "", http.StatusOK},
		{"history unknown", http.MethodGet, "/api/v1/persons/" + unknownID + "/history", "", http.StatusNotFound},
		{"restore points", http.MethodGet, "/api/v1/persons/" + personID + "/restore-points", "", http.StatusOK},
		{"restore points unknown", http.MethodGet, "/api/v1/persons/" + unknownID + "/restore-points", "", http.StatusNotFound},
		{"rollback", http.MethodPost, "/api/v1/persons/" + personID + "/rollback", `{"target_version":0}`, http.StatusBadRequest},
		{"rollback unknown", http.MethodPost, "/api/v1/persons/" + unknownID + "/rollback", `{"target_version":1}`, http.StatusNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reads.detailReads = 0
			rec := do(t, server, tt.method, tt.path, tt.body)
			if rec.Code != tt.want {
				t.Fatalf("status = %d, want %d. Body: %s", rec.Code, tt.want, rec.Body.String())
			}
			if reads.detailReads != 0 {
				t.Errorf("detail sub-resource reads = %d, want 0 for an existence check", reads.detailReads)
			}
		})
	}
}
