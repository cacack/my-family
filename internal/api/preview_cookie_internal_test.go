package api

import (
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// The web app's definitions of the preview cookie, matched loosely enough that
// a reformat (quote style, wrapping, a type annotation) is not mistaken for drift.
var (
	webPreviewCookie = regexp.MustCompile(`const PREVIEW_COOKIE\b[^=]*=\s*['"]([^'"]*)['"]`)
	webPreviewValues = regexp.MustCompile(`(?s)const PREVIEW_VALUES\b[^=]*=\s*\[([^\]]*)\]`)
	webStringLiteral = regexp.MustCompile(`['"]([^'"]*)['"]`)
)

// TestPreviewCookieMatchesWebApp fails when the preview cookie's name or
// values here drift from the web app's copy, which sets the cookie (#1497).
func TestPreviewCookieMatchesWebApp(t *testing.T) {
	const path = "../../web/src/lib/stores/auth.svelte.ts"
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read the web app's auth store: %v", err)
	}
	src := string(raw)

	name := webPreviewCookie.FindStringSubmatch(src)
	if name == nil {
		t.Fatalf("%s: no PREVIEW_COOKIE string constant found", path)
	}
	if name[1] != previewCookieName {
		t.Errorf("%s: PREVIEW_COOKIE = %q, auth.go has %q", path, name[1], previewCookieName)
	}

	list := webPreviewValues.FindStringSubmatch(src)
	if list == nil {
		t.Fatalf("%s: no PREVIEW_VALUES array found", path)
	}
	var got []string
	for _, m := range webStringLiteral.FindAllStringSubmatch(list[1], -1) {
		got = append(got, m[1])
	}
	want := []string{previewViewer, previewVisitor}
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Errorf("%s: PREVIEW_VALUES = [%s], auth.go has [%s]", path, strings.Join(got, ", "), strings.Join(want, ", "))
	}
}
