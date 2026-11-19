package repository

import (
	"context"

	"github.com/google/uuid"
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
