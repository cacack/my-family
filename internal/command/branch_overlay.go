package command

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// overlayVersions reads one aggregate's row version as main serves it and as a
// branch serves it (0 for no row), in that order.
type overlayVersions func(ctx context.Context, rs repository.ReadModelStore, branchID domain.BranchID, id uuid.UUID) (mainVersion, branchVersion int64, err error)

// rowVersions builds an overlayVersions from a branch-scoped single-row getter.
// Main is read FIRST: when the branch serves main's row, a main write landing
// between the two reads makes the branch read the newer row, never an older one.
func rowVersions[T any](get func(repository.ReadModelStore, context.Context, domain.BranchID, uuid.UUID) (*T, error), version func(*T) int64) overlayVersions {
	return func(ctx context.Context, rs repository.ReadModelStore, branchID domain.BranchID, id uuid.UUID) (int64, int64, error) {
		read := func(scope domain.BranchID) (int64, error) {
			row, err := get(rs, ctx, scope, id)
			if err != nil || row == nil {
				return 0, err
			}
			return version(row), nil
		}
		mainVersion, err := read(domain.MainBranchID)
		if err != nil {
			return 0, 0, err
		}
		branchVersion, err := read(branchID)
		if err != nil {
			return 0, 0, err
		}
		return mainVersion, branchVersion, nil
	}
}

// branchOverlayStreams maps every branch-scoped aggregate's stream type (lower
// case) to how its row versions are read. It covers every aggregate a branch
// appends to through execute, not only those a cross-stream projection writes
// today (a source, whose citation count a branch citation changes), so a future
// projection that shadows another aggregate cannot reintroduce #844 for it.
var branchOverlayStreams = map[string]overlayVersions{
	"person":           rowVersions(repository.ReadModelStore.GetPerson, func(r *repository.PersonReadModel) int64 { return r.Version }),
	familyStreamType:   rowVersions(repository.ReadModelStore.GetFamily, func(r *repository.FamilyReadModel) int64 { return r.Version }),
	"source":           rowVersions(repository.ReadModelStore.GetSource, func(r *repository.SourceReadModel) int64 { return r.Version }),
	"citation":         rowVersions(repository.ReadModelStore.GetCitation, func(r *repository.CitationReadModel) int64 { return r.Version }),
	"note":             rowVersions(repository.ReadModelStore.GetNote, func(r *repository.NoteReadModel) int64 { return r.Version }),
	"media":            rowVersions(repository.ReadModelStore.GetMedia, func(r *repository.MediaReadModel) int64 { return r.Version }),
	"association":      rowVersions(repository.ReadModelStore.GetAssociation, func(r *repository.AssociationReadModel) int64 { return r.Version }),
	"evidenceanalysis": rowVersions(repository.ReadModelStore.GetEvidenceAnalysis, func(r *repository.EvidenceAnalysisReadModel) int64 { return r.Version }),
	"evidenceconflict": rowVersions(repository.ReadModelStore.GetEvidenceConflict, func(r *repository.EvidenceConflictReadModel) int64 { return r.Version }),
	"researchlog":      rowVersions(repository.ReadModelStore.GetResearchLog, func(r *repository.ResearchLogReadModel) int64 { return r.Version }),
	"proofsummary":     rowVersions(repository.ReadModelStore.GetProofSummary, func(r *repository.ProofSummaryReadModel) int64 { return r.Version }),
}

// branchOverlayVersion is the repository.AppendScope.OverlayVersion of a branch
// append to the aggregate id: the version of the row the branch's read serves
// for it when that row is NOT main's (a cross-stream shadow, #844), else 0.
//
// A branch row whose version differs from main's is either the branch's own
// line (the branch has appended to the stream — the event store then ignores
// OverlayVersion) or a cross-stream shadow that kept the version main had when
// it was copied. When the two versions agree, the branch serves main's row (or a
// shadow indistinguishable from it), and 0 lets the event store seed from main's
// current version, read inside the append.
func (h *Handler) branchOverlayVersion(ctx context.Context, streamType string, id uuid.UUID) (int64, error) {
	versions, ok := branchOverlayStreams[strings.ToLower(streamType)]
	if !ok {
		return 0, nil
	}
	mainVersion, branchVersion, err := versions(ctx, h.readStore, h.branchID, id)
	if err != nil {
		return 0, fmt.Errorf("reading %s %s on branch %s: %w", streamType, id, h.branchID, err)
	}
	if branchVersion == 0 || branchVersion == mainVersion {
		return 0, nil
	}
	return branchVersion, nil
}
