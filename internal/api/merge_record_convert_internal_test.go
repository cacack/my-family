package api

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/query"
)

func TestConvertQueryMergeRecordToGenerated(t *testing.T) {
	if convertQueryMergeRecordToGenerated(nil) != nil {
		t.Error("a branch without a merge got a merge record")
	}

	stream, left := uuid.New(), uuid.New()
	count := 3
	record := &query.MergeRecord{
		ClaimID: uuid.New(), MergedAt: time.Now().UTC(), MergedAtPosition: 9, Note: "why",
		Recorded: true, ReplayedEventCount: &count, SkippedStreamIDs: []uuid.UUID{left}, ResumeCount: 1,
		Decisions: []query.MergeRecordDecision{
			{StreamID: stream, EntityType: "person", EntityName: "Ada", Kind: "delete_edit", Fields: []string{"surname"},
				DeletedBy: "main", Resolution: "main", Rationale: "register", DecidedAt: query.MergeDecidedAtMerge},
			{StreamID: left, EntityType: "person", Resolution: "branch", DecidedAt: query.MergeDecidedAtResume},
		},
		Exclusions: []query.MergeRecordExclusion{{StreamID: left, EntityType: "person", EntityName: "Allegra", Rationale: "unproven"}},
	}
	got := convertQueryMergeRecordToGenerated(record)
	if got.Note == nil || *got.Note != "why" || got.ReplayedEventCount == nil || *got.ReplayedEventCount != 3 ||
		len(got.SkippedStreamIds) != 1 || got.ResumeCount != 1 || !got.Recorded {
		t.Errorf("record = %+v", got)
	}
	full := got.Decisions[0]
	if full.Kind == nil || *full.Kind != "delete_edit" || full.Fields == nil || (*full.Fields)[0] != "surname" ||
		full.DeletedBy == nil || *full.DeletedBy != "main" || full.Rationale == nil || *full.Rationale != "register" ||
		full.DecidedAt != "merge" || full.Resolution != "main" || full.EntityName != "Ada" {
		t.Errorf("decision = %+v", full)
	}
	bare := got.Decisions[1]
	if bare.Kind != nil || bare.Fields != nil || bare.DeletedBy != nil || bare.Rationale != nil || bare.DecidedAt != "resume" {
		t.Errorf("a decision with nothing recorded but its side = %+v", bare)
	}
	if e := got.Exclusions[0]; e.EntityName != "Allegra" || e.Rationale == nil || *e.Rationale != "unproven" {
		t.Errorf("exclusion = %+v", e)
	}

	empty := convertQueryMergeRecordToGenerated(&query.MergeRecord{})
	if empty.SkippedStreamIds == nil || empty.Decisions == nil || empty.Exclusions == nil || empty.Note != nil {
		t.Errorf("an empty record must serialise its arrays as [] and omit the note: %+v", empty)
	}
}

func TestConvertQueryChangeEntryToGenerated_MergedFrom(t *testing.T) {
	origin := &query.MergeOrigin{BranchID: uuid.New(), BranchName: "Byron theory", Note: "why",
		MergedAt: time.Now().UTC(), OriginalTimestamp: time.Now().UTC().Add(-time.Hour)}
	got := convertQueryChangeEntryToGenerated(query.ChangeEntry{ID: uuid.New(), EntityType: "person", Action: "updated", MergedFrom: origin})
	if got.MergedFrom == nil || got.MergedFrom.BranchName != "Byron theory" || got.MergedFrom.Note == nil || *got.MergedFrom.Note != "why" ||
		!got.MergedFrom.OriginalTimestamp.Equal(origin.OriginalTimestamp) {
		t.Errorf("merged_from = %+v", got.MergedFrom)
	}
	origin.Note = ""
	if got := convertQueryChangeEntryToGenerated(query.ChangeEntry{MergedFrom: origin}); got.MergedFrom.Note != nil {
		t.Errorf("a merge without a note reported note %q", *got.MergedFrom.Note)
	}
	if got := convertQueryChangeEntryToGenerated(query.ChangeEntry{}); got.MergedFrom != nil {
		t.Error("an unmerged change reported a merge")
	}
}
