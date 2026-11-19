package api

import "github.com/cacack/my-family/internal/query"

// privateFlag is a record's private flag as the API sends it: true, or
// omitted when the record is not private (ADR-008 §2).
func privateFlag(private bool) *bool {
	if !private {
		return nil
	}
	return &private
}

// livingFields are a person's living verdict and override as the API sends
// them, with the empty (auto) override spelled "auto" (ADR-008 §1).
func livingFields(p query.Person) (*bool, *LivingOverride) {
	living := p.Living
	override := LivingOverrideAuto
	if p.LivingOverride != "" {
		override = LivingOverride(p.LivingOverride)
	}
	return &living, &override
}
