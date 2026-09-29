package query

import (
	"context"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/repository/memory"
)

// streamReadRecorder records every stream id a multi-stream read asks for.
type streamReadRecorder struct {
	*memory.EventStore
	mu    sync.Mutex
	asked map[uuid.UUID]bool
}

func (r *streamReadRecorder) ReadStreamsForBranch(ctx context.Context, streamIDs []uuid.UUID, branchID domain.BranchID, fromPosition int64, limit int) ([]repository.StoredEvent, error) {
	r.mu.Lock()
	for _, id := range streamIDs {
		r.asked[id] = true
	}
	r.mu.Unlock()
	return r.EventStore.ReadStreamsForBranch(ctx, streamIDs, branchID, fromPosition, limit)
}

// NameEntities asks the read model first and folds only what it cannot name:
// a media item the read model holds is named from its metadata, so its
// history (whose MediaCreated carries the file bytes) is never read (#831).
func TestNameEntities_FoldsOnlyWhatTheReadModelCannotName(t *testing.T) {
	ctx := context.Background()
	events := &streamReadRecorder{EventStore: memory.NewEventStore(), asked: map[uuid.UUID]bool{}}
	readStore := memory.NewReadModelStore()
	service := NewBranchService(memory.NewBranchStore(), events, NewHistoryService(events, readStore))

	// A person and a media item the read model holds.
	held := domain.NewPerson("Ada", "Lovelace")
	require.NoError(t, readStore.SavePerson(ctx, domain.MainBranchID, &repository.PersonReadModel{ID: held.ID, GivenName: "Ada", Surname: "Lovelace", FullName: "Ada Lovelace"}))
	photo := domain.NewMedia("Portrait", "person", held.ID)
	require.NoError(t, events.Append(ctx, photo.ID, "Media", []domain.Event{domain.NewMediaCreated(photo)}, anyVersion, repository.MainScope))
	require.NoError(t, readStore.SaveMedia(ctx, domain.MainBranchID, &repository.MediaReadModel{ID: photo.ID, Title: "Portrait", EntityType: "person", EntityID: held.ID}))
	untitled := uuid.New()
	require.NoError(t, readStore.SaveMedia(ctx, domain.MainBranchID, &repository.MediaReadModel{ID: untitled, Filename: "scan.png", EntityType: "person", EntityID: held.ID}))

	// A person and a media item only the log still holds.
	gone := domain.NewPerson("Grace", "Hopper")
	require.NoError(t, events.Append(ctx, gone.ID, "Person", []domain.Event{domain.NewPersonCreated(gone)}, anyVersion, repository.MainScope))
	lost := domain.NewMedia("Lost letter", "person", gone.ID)
	require.NoError(t, events.Append(ctx, lost.ID, "Media", []domain.Event{domain.NewMediaCreated(lost)}, anyVersion, repository.MainScope))

	branch, err := domain.NewBranch("naming", "", 0)
	require.NoError(t, err)
	names, err := service.NameEntities(ctx, domain.BranchID(branch.ID), []EntityRef{
		{EntityType: "person", ID: held.ID},
		{EntityType: "media", ID: photo.ID},
		{EntityType: "media", ID: untitled},
		{EntityType: "person", ID: gone.ID},
		{EntityType: "media", ID: lost.ID},
	})
	require.NoError(t, err)
	assert.Equal(t, "Ada Lovelace", names[EntityRef{EntityType: "person", ID: held.ID}])
	assert.Equal(t, "Portrait", names[EntityRef{EntityType: "media", ID: photo.ID}])
	assert.Equal(t, "scan.png", names[EntityRef{EntityType: "media", ID: untitled}])
	assert.Equal(t, "Grace Hopper", names[EntityRef{EntityType: "person", ID: gone.ID}])
	assert.Equal(t, "Lost letter", names[EntityRef{EntityType: "media", ID: lost.ID}])

	assert.False(t, events.asked[held.ID], "a person the read model names must not be folded")
	assert.False(t, events.asked[photo.ID], "a media item the read model names must not have its history read")
	assert.True(t, events.asked[gone.ID], "a person gone from the read model is named from its stream")
	assert.True(t, events.asked[lost.ID], "a media item gone from the read model is named from its stream")
}
