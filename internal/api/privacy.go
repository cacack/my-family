package api

import "github.com/cacack/my-family/internal/domain"

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
