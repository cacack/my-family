package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
)

// SettingsStore keeps the choices made in the app that are not genealogical
// facts: the site's and each account's. They are not events and not part of
// the read model, so a `myfamily rebuild` leaves them in place.
//
// The home person ("me") is a value of each account (#1443). The site-wide
// value, kept under SiteWide, is the home person in open mode, where no one
// signs in, and the starting value of the first owner's account (#1034).
type SettingsStore interface {
	// HomePerson returns the person accountID marked as "me", or uuid.Nil when
	// none is set. accountID is a user's ID, or SiteWide.
	HomePerson(ctx context.Context, accountID uuid.UUID) (uuid.UUID, error)

	// SetHomePerson marks personID as accountID's home person, replacing any
	// earlier choice. uuid.Nil clears it; an account remembers that it cleared
	// its choice, so SeedHomePerson leaves it cleared.
	SetHomePerson(ctx context.Context, accountID, personID uuid.UUID) error

	// SeedHomePerson gives accountID the site-wide home person, unless
	// accountID has chosen one or cleared its choice. It does nothing while
	// the site has none. A clear made before #1474 left no record, so it
	// counts as never chosen.
	SeedHomePerson(ctx context.Context, accountID uuid.UUID) error

	// MoveHomePersons makes survivors[p] the home person of the site and of
	// every account whose home person is p, as when merges remove each p. It
	// looks each home person up once, so a chain of merges is not followed.
	MoveHomePersons(ctx context.Context, survivors map[uuid.UUID]uuid.UUID) error
}

// SiteWide is the account ID under which SettingsStore keeps the site's own
// home person rather than a user's. No user has it.
var SiteWide = uuid.Nil

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

// AnonymousAccess is what someone who is not signed in may do in a tree
// (ADR-007, "Anonymous access").
type AnonymousAccess string

// The anonymous-access values. With none, every tree operation needs a
// session; with viewer, a visitor may call what a viewer may and sees what
// ADR-008 §4 allows anonymous principals.
const (
	AnonymousAccessNone   AnonymousAccess = "none"
	AnonymousAccessViewer AnonymousAccess = "viewer"
)

// Valid reports whether a is one of the anonymous-access values.
func (a AnonymousAccess) Valid() bool {
	return a == AnonymousAccessNone || a == AnonymousAccessViewer
}

// AnonymousViewingProtected reports whether the read side redacts living
// people and private records for anonymous principals (ADR-008 §5). It does
// (#1412), so owners may let visitors in. It is a kill switch: setting it to
// false refuses anonymous viewing again, in the settings API and for a viewer
// setting already stored, should a surface be found to leak to visitors.
const AnonymousViewingProtected = true

// ErrAnonymousViewingUnprotected refuses turning anonymous viewing on while
// AnonymousViewingProtected is false.
var ErrAnonymousViewingUnprotected = errors.New(
	"anonymous viewing cannot be turned on: living people and private records are not protected from visitors")

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
	// AnonymousAccess is what visitors who are not signed in may do.
	AnonymousAccess AnonymousAccess
}

// DefaultTreeSettings are the settings of a tree whose owner has changed none.
func DefaultTreeSettings() TreeSettings {
	return TreeSettings{
		LivingThresholdYears: domain.DefaultLivingThresholdYears,
		ViewersSeeLiving:     true,
		AnonymousAccess:      AnonymousAccessNone,
	}
}

// Validate reports a setting out of range, and anonymous viewing while it
// is unprotected (ErrAnonymousViewingUnprotected).
func (s TreeSettings) Validate() error {
	if s.LivingThresholdYears < MinLivingThresholdYears || s.LivingThresholdYears > MaxLivingThresholdYears {
		return fmt.Errorf("living threshold must be between %d and %d years, not %d",
			MinLivingThresholdYears, MaxLivingThresholdYears, s.LivingThresholdYears)
	}
	if !s.AnonymousAccess.Valid() {
		return fmt.Errorf("anonymous access must be %q or %q, not %q", AnonymousAccessNone, AnonymousAccessViewer, s.AnonymousAccess)
	}
	if s.AnonymousAccess == AnonymousAccessViewer && !AnonymousViewingProtected {
		return ErrAnonymousViewingUnprotected
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
