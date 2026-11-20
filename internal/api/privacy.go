package api

import (
	"context"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// privateFlag is a record's private flag as the API sends it: true, or
// omitted when the record is not private (ADR-008 §2).
func privateFlag(private bool) *bool {
	if !private {
		return nil
	}
	return &private
}

// redactedFlag is a person's redacted mark as the API sends it: true on the
// placeholder shown instead of a protected person, else omitted (ADR-008 §3).
func redactedFlag(redacted bool) *bool {
	if !redacted {
		return nil
	}
	return &redacted
}

// livingFields are a person's living verdict and override as the API sends
// them, with the empty (auto) override spelled "auto" (ADR-008 §1).
func livingFields(living bool, recorded domain.LivingOverride) (*bool, *LivingOverride) {
	override := LivingOverrideAuto
	if recorded != domain.LivingOverrideAuto {
		override = LivingOverride(recorded)
	}
	return &living, &override
}

// familyEventLogHidden reports whether the change history and restore points
// of family id are hidden from the principal on ctx: the family is not shown
// to them, or a partner is protected, so such a family's log is not found
// (ADR-008 read-surface table).
func (s *Server) familyEventLogHidden(ctx context.Context, branchID domain.BranchID, id uuid.UUID) (bool, error) {
	family, err := s.readStore.GetFamily(ctx, branchID, id)
	if err != nil || family == nil {
		return true, err
	}
	var partners []uuid.UUID
	for _, p := range []*uuid.UUID{family.Partner1ID, family.Partner2ID} {
		if p != nil {
			partners = append(partners, *p)
		}
	}
	if len(partners) == 0 {
		return false, nil
	}
	people, err := s.readStore.GetPersonsByIDs(ctx, branchID, partners)
	if err != nil {
		return true, err
	}
	shown := 0
	for i := range people {
		if people[i].Visibility == repository.Visible {
			shown++
		}
	}
	return shown != len(partners), nil
}

// personFound is the existence check of the routes under /persons/{id}: a
// private person, whom the read store returns as a Hidden placeholder, is not
// found there, as on their own page (ADR-008 §3).
func personFound(p *repository.PersonReadModel) bool {
	return p != nil && p.Visibility != repository.Hidden
}
