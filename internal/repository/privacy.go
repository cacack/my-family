package repository

import (
	"context"
	"time"

	"github.com/cacack/my-family/internal/domain"
)

// Principal is who reads the tree, as ADR-008 §4 tells them apart: the role
// they hold, and whether they are an anonymous visitor (who holds the viewer
// role without an account).
type Principal struct {
	Role      Role
	Anonymous bool
}

type principalKey struct{}

// ContextWithPrincipal returns ctx carrying who makes the request, so the
// read side shows them what ADR-008 §4 allows.
func ContextWithPrincipal(ctx context.Context, p Principal) context.Context {
	return context.WithValue(ctx, principalKey{}, p)
}

// PrincipalFrom returns the principal on ctx. A context carrying none is an
// anonymous visitor's, so a missing principal fails safe (ADR-008 §4).
func PrincipalFrom(ctx context.Context) Principal {
	if p, ok := ctx.Value(principalKey{}).(Principal); ok {
		return p
	}
	return Principal{Anonymous: true}
}

// Visibility is how a record appears to a principal (ADR-008 §5).
type Visibility int

const (
	// Visible records are shown in full.
	Visible Visibility = iota
	// Placeholder people keep their ID, family links and sex and nothing else
	// (ADR-008 §3).
	Placeholder
	// Hidden records are omitted: absent from lists, not found on fetch.
	Hidden
)

// PlaceholderGivenName is the given name a protected person is shown with.
const PlaceholderGivenName = "Living"

// Privacy is what one principal may not see at one moment: the policy of
// ADR-008 §5 that both choke points apply. Its methods are the rule's Go form;
// each SQL store builds the same tests from its fields to exclude rows in the
// query (the SQL form), and a shared table of cases holds the forms together.
// The zero value hides nothing.
type Privacy struct {
	// HidePrivate hides records whose restriction makes them private.
	HidePrivate bool
	// HideLiving hides people the living rule finds living at LivingCutoff.
	HideLiving   bool
	LivingCutoff time.Time
	// HideSubmitters hides submitter records, which hold living researchers'
	// names and contact details (ADR-008 §4).
	HideSubmitters bool
}

// PrivacyFor returns what principal p may not see at now under settings
// (ADR-008 §4): editors and owners see everything; viewers see living people
// unless the owner turned that off, and no private records; anonymous
// visitors, and a principal with no role, see neither, nor submitters.
func PrivacyFor(p Principal, settings TreeSettings, now time.Time) Privacy {
	role := p.Role
	if p.Anonymous || !role.Valid() {
		role = ""
	}
	switch {
	case role.Includes(RoleEditor):
		return Privacy{}
	case role == RoleViewer && settings.ViewersSeeLiving:
		return Privacy{HidePrivate: true}
	default:
		return Privacy{HidePrivate: true, HideLiving: true, LivingCutoff: settings.LivingCutoff(now), HideSubmitters: role == ""}
	}
}

// PrivacyFrom returns what the principal on ctx may not see at now, under the
// tree settings on ctx.
func PrivacyFrom(ctx context.Context, now time.Time) Privacy {
	return PrivacyFor(PrincipalFrom(ctx), TreeSettingsFrom(ctx), now)
}

// SeesAll reports whether nothing is hidden.
func (v Privacy) SeesAll() bool {
	return !v.HidePrivate && !v.HideLiving && !v.HideSubmitters
}

// Hides reports whether a record with restriction r is hidden.
func (v Privacy) Hides(r domain.Restriction) bool {
	return v.HidePrivate && r.IsPrivate()
}

// Person returns how p appears: hidden when private, a placeholder when
// living, else visible. A person that cannot be resolved (nil) is hidden
// (ADR-008 §3).
func (v Privacy) Person(p *PersonReadModel) Visibility {
	switch {
	case p == nil, v.Hides(p.Restriction):
		return Hidden
	case v.HideLiving && p.LivingFacts().IsLiving(v.LivingCutoff):
		return Placeholder
	default:
		return Visible
	}
}

// Protects reports whether p may not be shown in full: it is hidden or a
// placeholder. Lists and searches exclude such people (ADR-008 §3).
func (v Privacy) Protects(p *PersonReadModel) bool {
	return v.Person(p) != Visible
}

// PlaceholderPerson returns the placeholder shown instead of p where its slot
// is needed: same ID and sex, given name "Living", and nothing else (ADR-008
// §3). It carries why (vis, Placeholder or Hidden), so the API marks it
// redacted and a person's own page is not found when it is Hidden.
func PlaceholderPerson(p *PersonReadModel, vis Visibility) *PersonReadModel {
	return &PersonReadModel{
		ID:         p.ID,
		GivenName:  PlaceholderGivenName,
		FullName:   PlaceholderGivenName,
		Gender:     p.Gender,
		Version:    p.Version,
		Visibility: vis,
	}
}
