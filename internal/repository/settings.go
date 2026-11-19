package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
)

// SettingsStore keeps the site's settings: choices made in the app that are
// not genealogical facts. They are not events and not part of the read model,
// so a `myfamily rebuild` leaves them in place.
//
// The home person is one site-wide value until accounts exist (#1408). It then
// becomes a value of each account, and this one remains the value for open mode
// and the seed for the first owner's account (#1034).
type SettingsStore interface {
	// HomePerson returns the person marked as "me", or uuid.Nil when none is set.
	HomePerson(ctx context.Context) (uuid.UUID, error)

	// SetHomePerson marks id as the home person, replacing any earlier choice.
	// uuid.Nil clears it.
	SetHomePerson(ctx context.Context, id uuid.UUID) error
}

// SettingHomePerson is the settings key under which the SQL stores keep the
// home person's ID.
const SettingHomePerson = "home_person_id"

// The range an owner may set the living threshold to. Below the minimum the
// rule would presume people of working age dead and show them to the public;
// above the maximum it would protect no one it does not already protect.
const (
	MinLivingThresholdYears = 50
	MaxLivingThresholdYears = 150
)

// TreeSettings are a tree's privacy settings (ADR-008 §6), which its owner
// changes. Like accounts they are not tree data: they live in a plain table,
// never in the event log, and `myfamily rebuild` leaves them alone.
type TreeSettings struct {
	// LivingThresholdYears is N in the living rule: a person born more than
	// N years ago is not presumed living.
	LivingThresholdYears int
	// ViewersSeeLiving lets viewers see living people in full; when off they
	// see the placeholder anonymous visitors see.
	ViewersSeeLiving bool
}

// DefaultTreeSettings are the settings of a tree whose owner has changed none.
func DefaultTreeSettings() TreeSettings {
	return TreeSettings{LivingThresholdYears: domain.DefaultLivingThresholdYears, ViewersSeeLiving: true}
}

// Validate reports a setting out of range.
func (s TreeSettings) Validate() error {
	if s.LivingThresholdYears < MinLivingThresholdYears || s.LivingThresholdYears > MaxLivingThresholdYears {
		return fmt.Errorf("living threshold must be between %d and %d years, not %d",
			MinLivingThresholdYears, MaxLivingThresholdYears, s.LivingThresholdYears)
	}
	return nil
}

// LivingCutoff is the living rule's cutoff on now's date at these settings'
// threshold (domain.LivingCutoff).
func (s TreeSettings) LivingCutoff(now time.Time) time.Time {
	return domain.LivingCutoff(now, s.LivingThresholdYears)
}

// TreeSettingsStore keeps each tree's settings.
type TreeSettingsStore interface {
	// GetTreeSettings returns treeID's settings, or DefaultTreeSettings when
	// none were saved.
	GetTreeSettings(ctx context.Context, treeID uuid.UUID) (TreeSettings, error)

	// SaveTreeSettings stores treeID's settings, replacing any saved before.
	SaveTreeSettings(ctx context.Context, treeID uuid.UUID, settings TreeSettings) error
}

type treeSettingsKey struct{}

// ContextWithTreeSettings returns ctx carrying the settings of the tree a
// request reads, so the read side applies the owner's privacy rules.
func ContextWithTreeSettings(ctx context.Context, settings TreeSettings) context.Context {
	return context.WithValue(ctx, treeSettingsKey{}, settings)
}

// TreeSettingsFrom returns the settings on ctx, or DefaultTreeSettings when
// it carries none (the CLI, tests).
func TreeSettingsFrom(ctx context.Context) TreeSettings {
	if settings, ok := ctx.Value(treeSettingsKey{}).(TreeSettings); ok {
		return settings
	}
	return DefaultTreeSettings()
}
