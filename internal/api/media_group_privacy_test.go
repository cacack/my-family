package api_test

import (
	"bytes"
	"fmt"
	"mime/multipart"
	"net/http"
	"testing"

	"github.com/cacack/my-family/internal/repository"
)

// sharedMediaGedcom links one OBJE record from two dead people, so the import
// makes one media item for each (#1435).
const sharedMediaGedcom = `0 HEAD
1 GEDC
2 VERS 5.5.1
0 @M1@ OBJE
1 FILE photos/reunion.jpg
2 FORM jpg
2 TITL Reunion
0 @I1@ INDI
1 NAME Ada /Shared/
1 DEAT Y
1 OBJE @M1@
0 @I2@ INDI
1 NAME Bea /Shared/
1 DEAT Y
1 OBJE @M1@
0 TRLR
`

// TestMediaGroup_PrivateMarkHidesEveryCopyFromViewers marks the item of one
// person private and checks that a viewer sees the photo on neither person,
// while the owner still sees both copies, now private (#1478).
func TestMediaGroup_PrivateMarkHidesEveryCopyFromViewers(t *testing.T) {
	f, cookies := rolesFixture(t)
	owner, viewer := cookies[repository.RoleOwner], cookies[repository.RoleViewer]

	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	part, _ := w.CreateFormFile("file", "shared.ged")
	_, _ = part.Write([]byte(sharedMediaGedcom))
	_ = w.Close()
	if rec := f.do(t, request{method: http.MethodPost, path: "/api/v1/gedcom/import", body: body.String(), cookie: owner,
		header: map[string]string{"Content-Type": w.FormDataContentType()}}); rec.Code != http.StatusOK {
		t.Fatalf("import: %d %s", rec.Code, rec.Body)
	}

	persons := decode[map[string]any](t, f.do(t, request{method: http.MethodGet, path: "/api/v1/persons", cookie: owner}))
	var paths []string
	for _, p := range persons["items"].([]any) {
		paths = append(paths, "/api/v1/persons/"+p.(map[string]any)["id"].(string)+"/media")
	}
	if len(paths) != 2 {
		t.Fatalf("persons = %v, want 2", persons)
	}
	media := func(cookie *http.Cookie, path string) []any {
		t.Helper()
		rec := f.do(t, request{method: http.MethodGet, path: path, cookie: cookie})
		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s: %d %s", path, rec.Code, rec.Body)
		}
		return decode[map[string]any](t, rec)["items"].([]any)
	}
	for _, path := range paths {
		if got := media(viewer, path); len(got) != 1 {
			t.Fatalf("before: viewer %s = %v, want the photo", path, got)
		}
	}

	item := media(owner, paths[0])[0].(map[string]any)
	mark := fmt.Sprintf(`{"private":true,"version":%v}`, item["version"])
	if rec := f.do(t, request{method: http.MethodPut, path: "/api/v1/media/" + item["id"].(string), body: mark, cookie: owner}); rec.Code != http.StatusOK {
		t.Fatalf("mark private: %d %s", rec.Code, rec.Body)
	}

	for _, path := range paths {
		if got := media(viewer, path); len(got) != 0 {
			t.Errorf("viewer %s = %v, want no copy of the private photo", path, got)
		}
		got := media(owner, path)
		if len(got) != 1 || got[0].(map[string]any)["private"] != true {
			t.Errorf("owner %s = %v, want the photo marked private", path, got)
		}
	}
}
