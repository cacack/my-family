package query

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/repository/memory"
)

// GPS artifact stream types, as the evidence commands name them.
const (
	streamTypeEvidenceAnalysis = "EvidenceAnalysis"
	streamTypeResearchLog      = "ResearchLog"
	streamTypeProofSummary     = "ProofSummary"
)

// archiveStreamTypes are the GPS artifacts a branch's research archive
// reconstructs (#836): the research log (including its negative "not found"
// outcomes), evidence analyses and proof summaries.
var archiveStreamTypes = map[string]bool{
	streamTypeEvidenceAnalysis: true,
	streamTypeResearchLog:      true,
	streamTypeProofSummary:     true,
}

// ArchivedResearchLog is a research log entry as the branch left it, with the
// display name of its subject and whether the entry was first written on the
// branch (rather than being a mainline entry the branch edited).
type ArchivedResearchLog struct {
	ResearchLogEntry
	SubjectName     string
	CreatedOnBranch bool
}

// ArchivedEvidenceAnalysis is an evidence analysis as the branch left it.
type ArchivedEvidenceAnalysis struct {
	EvidenceAnalysis
	SubjectName     string
	CreatedOnBranch bool
}

// ArchivedProofSummary is a proof summary as the branch left it.
type ArchivedProofSummary struct {
	ProofSummaryResult
	SubjectName     string
	CreatedOnBranch bool
}

// BranchResearchArchive is the read-only record of a branch's GPS artifacts
// (#836), rebuilt from the branch's own events rather than read from its
// overlay — the overlay of a closed or merged branch has been purged, but its
// events are retained (ES-002), so the research it recorded, including the
// searches that found nothing, stays reachable.
type BranchResearchArchive struct {
	Branch           *domain.Branch
	ResearchLogs     []ArchivedResearchLog
	EvidenceAnalyses []ArchivedEvidenceAnalysis
	ProofSummaries   []ArchivedProofSummary
	// DeletedCount is how many GPS artifacts the branch deleted (a mainline
	// artifact it removed, or one it created and removed again); they are not
	// listed.
	DeletedCount int
	// Truncated reports that a read hit maxComparisonEvents, so the archive
	// may be missing artifacts.
	Truncated bool
}

// archiveArtifact tracks one GPS artifact stream the branch touched.
type archiveArtifact struct {
	streamID   uuid.UUID
	streamType string
	createdOn  bool // the branch's first event for the stream was its Created
}

// BranchResearchArchive reconstructs the GPS artifacts a branch recorded
// (#836). It works on a branch in any status, but exists for the terminal
// ones: an active branch's artifacts can also be read live through its
// overlay.
//
// The reconstruction replays the artifacts' own events through the real
// projector into a throwaway in-memory read model, so each artifact reads as
// the branch recorded it on its own stream:
//
//  1. The branch's own events are read in position order and the streams of
//     its GPS artifacts collected.
//  2. For a stream the branch did not create (it edited or deleted a mainline
//     artifact), the mainline events before the branch's first event on it
//     are projected on main first. That is the copy-on-write base the
//     branch's edit applied to: the overlay copies main's row as it stands
//     when the branch first writes it, which may be after the fork.
//  3. The branch's GPS events are projected on the branch's scope.
//
// Only the touched streams are loaded: two set-based reads (the branch's
// events, main's events for the edited streams), each bounded by
// maxComparisonEvents, whatever the number of artifacts.
//
// A person merge on the branch (#834) is deliberately not replayed. On the
// overlay it re-points an artifact's subject to the survivor without an event
// on the artifact's stream; the archive keeps the subject the artifact was
// recorded about instead. A closed branch's person merge is part of the
// hypothesis it did not establish, and main never merged those persons, so
// the recorded subject is the one that is true on main — and the one a
// promotion must use (a log about a branch-only person merged away is then
// skipped as subject_not_on_main rather than attached to the survivor).
func (s *BranchService) BranchResearchArchive(ctx context.Context, branchID uuid.UUID) (*BranchResearchArchive, error) {
	branch, err := s.branchStore.Get(ctx, branchID)
	if err != nil {
		return nil, fmt.Errorf("get branch: %w", err)
	}
	archive := &BranchResearchArchive{Branch: branch}

	branchEvents, truncated, err := s.readBranchGPSEvents(ctx, branch)
	if err != nil {
		return nil, err
	}
	archive.Truncated = truncated

	artifacts, baseCutoffs := collectArchiveArtifacts(branchEvents)

	scratch := memory.NewReadModelStore()
	projector := repository.NewProjector(scratch, nil)

	if len(baseCutoffs) > 0 {
		truncated, err := s.projectArchiveBase(ctx, projector, artifacts, baseCutoffs)
		if err != nil {
			return nil, err
		}
		archive.Truncated = archive.Truncated || truncated
	}

	scope := domain.BranchID(branch.ID)
	for i := range branchEvents {
		if err := projectStored(ctx, projector, &branchEvents[i], scope); err != nil {
			return nil, err
		}
	}

	names, err := newArchiveNames(branchEvents)
	if err != nil {
		return nil, err
	}
	if err := s.collectArchive(ctx, scratch, scope, artifacts, names, archive); err != nil {
		return nil, err
	}
	if err := names.resolve(ctx, s.historyService); err != nil {
		return nil, err
	}
	names.apply(archive)
	return archive, nil
}

// readBranchGPSEvents reads the branch's own events for the archived GPS
// artifacts and for the persons and families it created (their names label
// subjects that never reached main). It pages like readBranchMutations, so
// the cap counts the events kept.
func (s *BranchService) readBranchGPSEvents(ctx context.Context, branch *domain.Branch) ([]repository.StoredEvent, bool, error) {
	from := branch.BasePosition
	var kept []repository.StoredEvent
	for {
		page, err := s.eventStore.ReadBranch(ctx, domain.BranchID(branch.ID), from, maxComparisonEvents)
		if err != nil {
			return nil, false, fmt.Errorf("read branch events: %w", err)
		}
		for _, evt := range page {
			if archiveStreamTypes[evt.StreamType] || evt.EventType == "PersonCreated" || evt.EventType == "FamilyCreated" {
				kept = append(kept, evt)
			}
		}
		if len(kept) >= maxComparisonEvents {
			return kept[:maxComparisonEvents], true, nil
		}
		if len(page) < maxComparisonEvents {
			return kept, false, nil
		}
		from = page[len(page)-1].Position
	}
}

// collectArchiveArtifacts lists the GPS streams the branch touched, in the
// order it first touched them, and — for the ones whose base must be read from
// main (the branch's first event on them was not a Created) — the position of
// that first event: main's events before it are the base the branch edited.
func collectArchiveArtifacts(events []repository.StoredEvent) ([]archiveArtifact, map[uuid.UUID]int64) {
	seen := map[uuid.UUID]bool{}
	var artifacts []archiveArtifact
	cutoffs := map[uuid.UUID]int64{}
	for _, evt := range events {
		if !archiveStreamTypes[evt.StreamType] || seen[evt.StreamID] {
			continue
		}
		seen[evt.StreamID] = true
		created := strings.HasSuffix(evt.EventType, "Created")
		artifacts = append(artifacts, archiveArtifact{streamID: evt.StreamID, streamType: evt.StreamType, createdOn: created})
		if !created {
			cutoffs[evt.StreamID] = evt.Position
		}
	}
	return artifacts, cutoffs
}

// projectArchiveBase projects, on main, each edited stream's mainline events
// that precede the branch's first event on it. One set-based read covers every
// stream; it reports truncation when the read is full and may have stopped
// short of a needed event.
func (s *BranchService) projectArchiveBase(ctx context.Context, projector *repository.Projector,
	artifacts []archiveArtifact, cutoffs map[uuid.UUID]int64) (bool, error) {
	streams := make([]uuid.UUID, 0, len(cutoffs))
	var lastCutoff int64
	for _, a := range artifacts {
		if cutoff, ok := cutoffs[a.streamID]; ok {
			streams = append(streams, a.streamID)
			lastCutoff = max(lastCutoff, cutoff)
		}
	}
	mainEvents, err := s.eventStore.ReadStreamsForBranch(ctx, streams, domain.MainBranchID, 0, maxComparisonEvents)
	if err != nil {
		return false, fmt.Errorf("read main base of branch research: %w", err)
	}
	for i := range mainEvents {
		if mainEvents[i].Position >= cutoffs[mainEvents[i].StreamID] {
			continue
		}
		if err := projectStored(ctx, projector, &mainEvents[i], domain.MainBranchID); err != nil {
			return false, err
		}
	}
	truncated := len(mainEvents) >= maxComparisonEvents && mainEvents[len(mainEvents)-1].Position < lastCutoff
	return truncated, nil
}

// projectStored decodes a stored event and projects it on scope. Persons and
// families are only read for their names, not projected.
func projectStored(ctx context.Context, projector *repository.Projector, evt *repository.StoredEvent, scope domain.BranchID) error {
	if !archiveStreamTypes[evt.StreamType] {
		return nil
	}
	decoded, err := evt.DecodeEvent()
	if err != nil {
		return fmt.Errorf("decode %s event %s: %w", evt.EventType, evt.ID, err)
	}
	if err := projector.Project(ctx, decoded, evt.Version, scope); err != nil {
		return fmt.Errorf("replay %s event %s: %w", evt.EventType, evt.ID, err)
	}
	return nil
}

// collectArchive reads each touched artifact back from the scratch read model
// on the branch scope, in first-touched order. An artifact that no longer
// resolves was deleted on the branch.
func (s *BranchService) collectArchive(ctx context.Context, scratch *memory.ReadModelStore, scope domain.BranchID,
	artifacts []archiveArtifact, names *archiveNames, archive *BranchResearchArchive) error {
	for _, a := range artifacts {
		switch a.streamType {
		case streamTypeResearchLog:
			rm, err := scratch.GetResearchLog(ctx, scope, a.streamID)
			if err != nil {
				return fmt.Errorf("read archived research log %s: %w", a.streamID, err)
			}
			if rm == nil {
				archive.DeletedCount++
				continue
			}
			names.want(rm.SubjectType, rm.SubjectID)
			archive.ResearchLogs = append(archive.ResearchLogs, ArchivedResearchLog{
				ResearchLogEntry: convertReadModelToResearchLog(*rm),
				CreatedOnBranch:  a.createdOn,
			})
		case streamTypeEvidenceAnalysis:
			rm, err := scratch.GetEvidenceAnalysis(ctx, scope, a.streamID)
			if err != nil {
				return fmt.Errorf("read archived evidence analysis %s: %w", a.streamID, err)
			}
			if rm == nil {
				archive.DeletedCount++
				continue
			}
			names.want("", rm.SubjectID)
			archive.EvidenceAnalyses = append(archive.EvidenceAnalyses, ArchivedEvidenceAnalysis{
				EvidenceAnalysis: convertReadModelToEvidenceAnalysis(*rm),
				CreatedOnBranch:  a.createdOn,
			})
		case streamTypeProofSummary:
			rm, err := scratch.GetProofSummary(ctx, scope, a.streamID)
			if err != nil {
				return fmt.Errorf("read archived proof summary %s: %w", a.streamID, err)
			}
			if rm == nil {
				archive.DeletedCount++
				continue
			}
			names.want("", rm.SubjectID)
			archive.ProofSummaries = append(archive.ProofSummaries, ArchivedProofSummary{
				ProofSummaryResult: convertReadModelToProofSummary(*rm),
				CreatedOnBranch:    a.createdOn,
			})
		}
	}
	return nil
}

// archiveNames resolves the subjects of archived artifacts to display names:
// first through main as it stands now (in one batched lookup per type), then,
// for a person or family that only ever existed on the branch, through the
// name its Created event carried. A subject that resolves neither way keeps
// an empty name.
type archiveNames struct {
	typed    map[uuid.UUID]string // subject id -> "person" / "family" / "" (unknown)
	resolved map[uuid.UUID]string
	onBranch map[uuid.UUID]string
}

func newArchiveNames(branchEvents []repository.StoredEvent) (*archiveNames, error) {
	n := &archiveNames{
		typed:    map[uuid.UUID]string{},
		resolved: map[uuid.UUID]string{},
		onBranch: map[uuid.UUID]string{},
	}
	for _, evt := range branchEvents {
		switch evt.EventType {
		case "PersonCreated":
			var e domain.PersonCreated
			if err := json.Unmarshal(evt.Data, &e); err != nil {
				return nil, fmt.Errorf("decode PersonCreated event %s: %w", evt.ID, err)
			}
			n.onBranch[e.PersonID] = strings.TrimSpace(e.GivenName + " " + e.Surname)
		case "FamilyCreated":
			n.onBranch[evt.StreamID] = "Family created on this branch"
		}
	}
	return n, nil
}

func (n *archiveNames) want(subjectType string, id uuid.UUID) {
	if _, ok := n.typed[id]; !ok || n.typed[id] == "" {
		n.typed[id] = subjectType
	}
}

// resolve looks the wanted subjects up on main. A subject of unknown type (an
// analysis or proof summary's subject may be a person or a family) is looked
// up as both.
func (n *archiveNames) resolve(ctx context.Context, history *HistoryService) error {
	if history == nil || history.readStore == nil || len(n.typed) == 0 {
		return nil
	}
	refs := newEntityRefs()
	for id, typ := range n.typed {
		switch typ {
		case "person":
			refs.add("person", id)
		case "family":
			refs.add("family", id)
		default:
			refs.add("person", id)
			refs.add("family", id)
		}
	}
	names, err := history.resolveEntityNamesOn(ctx, domain.MainBranchID, refs)
	if err != nil {
		return fmt.Errorf("resolve archived research subjects: %w", err)
	}
	for id := range n.typed {
		switch {
		case names.persons[id] != nil:
			n.resolved[id] = names.personName(id, nil)
		case names.families[id] != nil:
			n.resolved[id] = names.familyName(id, nil)
		}
	}
	return nil
}

func (n *archiveNames) name(id uuid.UUID) string {
	if name, ok := n.resolved[id]; ok {
		return name
	}
	return n.onBranch[id]
}

func (n *archiveNames) apply(archive *BranchResearchArchive) {
	for i := range archive.ResearchLogs {
		archive.ResearchLogs[i].SubjectName = n.name(archive.ResearchLogs[i].SubjectID)
	}
	for i := range archive.EvidenceAnalyses {
		archive.EvidenceAnalyses[i].SubjectName = n.name(archive.EvidenceAnalyses[i].SubjectID)
	}
	for i := range archive.ProofSummaries {
		archive.ProofSummaries[i].SubjectName = n.name(archive.ProofSummaries[i].SubjectID)
	}
}
