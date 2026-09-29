package query

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// TestCompareSnapshotToPosition_AllBackends pins the bounded comparison #833
// links a merge to: from a snapshot up to a chosen log position, however far
// the log has moved since.
func TestCompareSnapshotToPosition_AllBackends(t *testing.T) {
	for _, backend := range snapshotBackends() {
		t.Run(backend.name, func(t *testing.T) {
			ctx := context.Background()
			es, rs, ss := backend.open(t)
			service := NewSnapshotService(ss, es, NewHistoryService(es, rs))
			w := &logWriter{t: t, es: es, versions: map[string]int64{}}

			p := domain.NewPerson("Ada", "Lovelace")
			w.append(domain.MainBranchID, p.ID, domain.NewPersonCreated(p))
			head, err := ss.GetMaxPosition(ctx)
			require.NoError(t, err)
			snapshot, err := domain.NewSnapshot("Before", "", head)
			require.NoError(t, err)
			require.NoError(t, ss.Upsert(ctx, snapshot))

			w.update(domain.MainBranchID, p.ID, "Byron")
			w.update(domain.MainBranchID, p.ID, "King")
			until, err := ss.GetMaxPosition(ctx)
			require.NoError(t, err)
			w.update(domain.MainBranchID, p.ID, "Later")

			bounded, err := service.CompareSnapshotToPosition(ctx, domain.MainBranchID, snapshot.ID, &until)
			require.NoError(t, err)
			assert.Len(t, bounded.Changes, 2)
			assert.Equal(t, until, bounded.ToPosition)
			assert.Greater(t, bounded.HeadPosition, until)

			past := bounded.HeadPosition + 100
			clamped, err := service.CompareSnapshotToPosition(ctx, domain.MainBranchID, snapshot.ID, &past)
			require.NoError(t, err)
			assert.Len(t, clamped.Changes, 3)
			assert.Equal(t, clamped.HeadPosition, clamped.ToPosition)

			at := snapshot.Position
			empty, err := service.CompareSnapshotToPosition(ctx, domain.MainBranchID, snapshot.ID, &at)
			require.NoError(t, err)
			assert.Empty(t, empty.Changes)

			before := snapshot.Position - 1
			_, err = service.CompareSnapshotToPosition(ctx, domain.MainBranchID, snapshot.ID, &before)
			assert.True(t, errors.Is(err, ErrComparisonEndBeforeSnapshot), "err = %v", err)

			now, err := service.CompareSnapshotToCurrent(ctx, domain.MainBranchID, snapshot.ID)
			require.NoError(t, err)
			assert.Equal(t, now.HeadPosition, now.ToPosition)
		})
	}
}

func TestLastReplayedPosition(t *testing.T) {
	claim := uuid.New()
	stamped := func(position int64, claimID uuid.UUID) repository.StoredEvent {
		meta, err := json.Marshal(domain.EventMetadata{MergedFromBranch: &domain.MergeProvenance{ClaimID: claimID}})
		require.NoError(t, err)
		return repository.StoredEvent{Position: position, Metadata: meta}
	}
	events := []repository.StoredEvent{
		stamped(3, claim),
		{Position: 4}, // unstamped
		{Position: 5, Metadata: json.RawMessage(`{`)},  // undecodable
		{Position: 6, Metadata: json.RawMessage(`{}`)}, // no provenance
		stamped(7, claim),
		stamped(9, uuid.New()), // another merge
	}
	position, ok, count := lastReplayedPosition(claim, events)
	assert.True(t, ok)
	assert.Equal(t, int64(7), position)
	assert.Equal(t, 2, count)

	_, ok, count = lastReplayedPosition(uuid.New(), events[:4])
	assert.False(t, ok)
	assert.Zero(t, count)
}

// TestReplayedThroughPosition covers when the merge's effect can be bounded
// exactly: a main tail cut short by later mainline edits still bounds it when
// every copy the plan called for was found, and a plan that replays nothing
// ends at its own claim.
func TestReplayedThroughPosition(t *testing.T) {
	claimID := uuid.New()
	replayed, kept := uuid.New(), uuid.New()
	stamped := func(position int64) repository.StoredEvent {
		meta, err := json.Marshal(domain.EventMetadata{MergedFromBranch: &domain.MergeProvenance{ClaimID: claimID}})
		require.NoError(t, err)
		return repository.StoredEvent{StreamID: replayed, Position: position, Metadata: meta}
	}
	markers := func(plan map[uuid.UUID]int64, resumes ...domain.BranchMergeResumed) *mergeMarkers {
		claim := domain.BranchMerged{ReplayStreamVersions: plan}
		claim.ID = claimID
		return &mergeMarkers{claim: claim, claimPosition: 20, resumes: resumes}
	}
	branchEvents := []repository.StoredEvent{{StreamID: replayed}, {StreamID: replayed}, {StreamID: kept}}
	// Two copies, then later mainline edits to the same entity that fill the scan.
	tail := []repository.StoredEvent{stamped(21), stamped(22), {StreamID: replayed, Position: 30}, {StreamID: replayed, Position: 31}}
	plan := map[uuid.UUID]int64{replayed: 0}

	tests := []struct {
		name    string
		markers *mergeMarkers
		diff    branchDiffSources
		want    int64
		wantOK  bool
	}{
		{"complete tail", markers(plan), branchDiffSources{branchEvents: branchEvents, mainEvents: tail}, 22, true},
		{"cut-short tail holding every copy", markers(plan),
			branchDiffSources{branchEvents: branchEvents, mainEvents: tail, mainTruncated: true}, 22, true},
		{"cut-short tail missing a copy", markers(plan),
			branchDiffSources{branchEvents: branchEvents, mainEvents: tail[1:], mainTruncated: true}, 0, false},
		{"cut-short tail, branch scan cut short too", markers(plan),
			branchDiffSources{branchEvents: branchEvents, mainEvents: tail, mainTruncated: true, branchTruncated: true}, 0, false},
		{"cut-short tail, no recorded plan", markers(nil),
			branchDiffSources{branchEvents: branchEvents, mainEvents: tail, mainTruncated: true}, 0, false},
		{"resume's plan is the one counted", markers(plan, domain.BranchMergeResumed{ReplayStreamVersions: map[uuid.UUID]int64{replayed: 0, kept: 0}}),
			branchDiffSources{branchEvents: branchEvents, mainEvents: tail, mainTruncated: true}, 0, false},
		{"plan replays nothing", markers(map[uuid.UUID]int64{}),
			branchDiffSources{branchEvents: branchEvents, mainEvents: tail[2:], mainTruncated: true}, 20, true},
		{"no copy found", markers(plan), branchDiffSources{branchEvents: branchEvents, mainEvents: tail[2:]}, 0, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := replayedThroughPosition(tc.markers, &tc.diff)
			assert.Equal(t, tc.wantOK, ok)
			assert.Equal(t, tc.want, got)
		})
	}
}
