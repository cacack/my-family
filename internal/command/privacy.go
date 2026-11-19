package command

import (
	"fmt"

	"github.com/cacack/my-family/internal/domain"
)

// recordPrivate is the per-record step of an Update* command for the private
// flag (ADR-008 §2): when private is set and marking or unmarking the record
// changes its restriction, it records the new restriction under
// "restriction". Other RESN values, "locked" included, are kept.
func recordPrivate(changes map[string]any, private *bool, current domain.Restriction) {
	if private == nil {
		return
	}
	if next := current.WithPrivate(*private); next != current {
		changes["restriction"] = string(next)
	}
}

// recordLivingOverride records a change to a person's living override, where
// a clear means auto (ADR-008 §1). An unknown value is invalid input.
func recordLivingOverride(changes map[string]any, input Patch[string], current domain.LivingOverride) error {
	v, ok := input.Change()
	if !ok {
		return nil
	}
	next := domain.LivingOverride(v)
	if v == "auto" {
		next = domain.LivingOverrideAuto
	}
	if !next.IsValid() {
		return fmt.Errorf("%w: living_override: invalid value: %s", ErrInvalidInput, v)
	}
	if next != current {
		changes["living_override"] = string(next)
	}
	return nil
}
