package api_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/api"
	"github.com/cacack/my-family/internal/repository"
)

const homePersonPath = "/api/v1/settings/home-person"

// addPerson creates a mainline person as the holder of cookie, returning its
// ID and version.
func (f *authFixture) addPerson(t *testing.T, cookie *http.Cookie, given, surname string) (string, int64) {
	t.Helper()
	rec := f.do(t, request{method: http.MethodPost, path: "/api/v1/persons", cookie: cookie,
		body: fmt.Sprintf(`{"given_name":%q,"surname":%q}`, given, surname)})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create person: %d %s", rec.Code, rec.Body)
	}
	person := decode[api.Person](t, rec)
	return person.Id.String(), person.Version
}

// homeOf returns the home person cookie's holder sees, or "" for none.
func (f *authFixture) homeOf(t *testing.T, cookie *http.Cookie) string {
	t.Helper()
	rec := f.do(t, request{method: http.MethodGet, path: homePersonPath, cookie: cookie})
	if rec.Code != http.StatusOK {
		t.Fatalf("GET home person: %d %s", rec.Code, rec.Body)
	}
	home := decode[api.HomePerson](t, rec)
	if home.Person == nil {
		return ""
	}
	return home.Person.Id.String()
}

func (f *authFixture) setHome(t *testing.T, cookie *http.Cookie, personID string) {
	t.Helper()
	rec := f.do(t, request{method: http.MethodPut, path: homePersonPath, cookie: cookie, body: `{"person_id":"` + personID + `"}`})
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT home person: %d %s", rec.Code, rec.Body)
	}
}

// With sign-in on, each account has its own home person: setting or clearing
// one leaves the others' alone (#1443).
func TestHomePerson_EachAccountHasItsOwn(t *testing.T) {
	f := newAuthFixture(t)
	f.createOwner(t)
	owner := f.signIn(t)
	viewer := f.addMember(t, repository.RoleViewer)
	ada, _ := f.addPerson(t, owner, "Ada", "Byron")
	bob, _ := f.addPerson(t, owner, "Bob", "Byron")

	f.setHome(t, owner, ada)
	if got := f.homeOf(t, viewer); got != "" {
		t.Errorf("viewer's home person after the owner chose = %q, want none", got)
	}
	f.setHome(t, viewer, bob)
	if got := f.homeOf(t, owner); got != ada {
		t.Errorf("owner's home person = %q, want %s", got, ada)
	}
	if got := f.homeOf(t, viewer); got != bob {
		t.Errorf("viewer's home person = %q, want %s", got, bob)
	}

	if rec := f.do(t, request{method: http.MethodDelete, path: homePersonPath, cookie: viewer}); rec.Code != http.StatusNoContent {
		t.Fatalf("viewer DELETE: %d %s", rec.Code, rec.Body)
	}
	if got := f.homeOf(t, viewer); got != "" {
		t.Errorf("viewer's home person after clearing = %q, want none", got)
	}
	if got := f.homeOf(t, owner); got != ada {
		t.Errorf("owner's home person after the viewer cleared theirs = %q, want %s", got, ada)
	}
	if site, _ := f.settings.HomePerson(context.Background(), repository.SiteWide); site != uuid.Nil {
		t.Errorf("site home person = %v, want accounts to leave it alone", site)
	}
}

// An anonymous visitor has no home person: they read none, even when the
// site has one, and may not set or clear one.
func TestHomePerson_AnonymousVisitorHasNone(t *testing.T) {
	f := newAuthFixture(t)
	f.createOwner(t)
	owner := f.signIn(t)
	ada, _ := f.addPerson(t, owner, "Ada", "Byron")
	f.setHome(t, owner, ada)
	if err := f.settings.SetHomePerson(context.Background(), repository.SiteWide, uuid.MustParse(ada)); err != nil {
		t.Fatal(err)
	}
	f.openToVisitors(t)

	if got := f.homeOf(t, nil); got != "" {
		t.Errorf("visitor's home person = %q, want none", got)
	}
	for _, r := range []request{
		{method: http.MethodPut, path: homePersonPath, body: `{"person_id":"` + ada + `"}`},
		{method: http.MethodDelete, path: homePersonPath},
	} {
		if rec := f.do(t, r); !unauthenticated(rec) {
			t.Errorf("visitor %s: %d %s, want 401 UNAUTHENTICATED", r.method, rec.Code, rec.Body)
		}
	}
	if got := f.homeOf(t, owner); got != ada {
		t.Errorf("owner's home person after the visitor's attempts = %q, want %s", got, ada)
	}
}

// The first owner starts with the home person chosen while the server ran
// in open mode.
func TestHomePerson_FirstOwnerStartsWithSiteChoice(t *testing.T) {
	f := newAuthFixture(t)
	site := uuid.New()
	if err := f.settings.SetHomePerson(context.Background(), repository.SiteWide, site); err != nil {
		t.Fatal(err)
	}
	f.createOwner(t)
	owner, err := f.accounts.GetUserByUsername(context.Background(), "ada")
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := f.settings.HomePerson(context.Background(), owner.ID); got != site {
		t.Errorf("first owner's home person = %v, want the site's %v", got, site)
	}
}

// A mainline merge moves every account whose home person was merged away.
func TestHomePerson_MergeMovesEveryAccount(t *testing.T) {
	f := newAuthFixture(t)
	f.createOwner(t)
	owner := f.signIn(t)
	editor := f.addMember(t, repository.RoleEditor)
	viewer := f.addMember(t, repository.RoleViewer)
	survivor, sv := f.addPerson(t, owner, "Morgan", "Duplicate")
	merged, mv := f.addPerson(t, owner, "Morgan", "Duplicat")
	other, _ := f.addPerson(t, owner, "Other", "Person")
	f.setHome(t, owner, merged)
	f.setHome(t, editor, merged)
	f.setHome(t, viewer, other)

	body := fmt.Sprintf(`{"survivor_id":%q,"merged_id":%q,"survivor_version":%d,"merged_version":%d}`, survivor, merged, sv, mv)
	if rec := f.do(t, request{method: http.MethodPost, path: "/api/v1/persons/merge", cookie: editor, body: body}); rec.Code != http.StatusOK {
		t.Fatalf("merge: %d %s", rec.Code, rec.Body)
	}
	for name, tc := range map[string]struct {
		cookie *http.Cookie
		want   string
	}{"owner": {owner, survivor}, "editor": {editor, survivor}, "viewer": {viewer, other}} {
		if got := f.homeOf(t, tc.cookie); got != tc.want {
			t.Errorf("%s's home person after the merge = %q, want %s", name, got, tc.want)
		}
	}
}
