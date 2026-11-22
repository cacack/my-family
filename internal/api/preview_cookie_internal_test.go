package api

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

// TestPreviewCookieMatchesWebApp fails when the preview cookie's name or
// values here drift from the web app's copy, which sets the cookie (#1497).
func TestPreviewCookieMatchesWebApp(t *testing.T) {
	src, err := os.ReadFile("../../web/src/lib/stores/auth.svelte.ts")
	if err != nil {
		t.Fatalf("read the web app's auth store: %v", err)
	}
	for _, want := range []string{
		fmt.Sprintf("const PREVIEW_COOKIE = '%s';", previewCookieName),
		fmt.Sprintf("const PREVIEW_VALUES: readonly PreviewAs[] = ['%s', '%s'];", previewViewer, previewVisitor),
	} {
		if !strings.Contains(string(src), want) {
			t.Errorf("web/src/lib/stores/auth.svelte.ts lacks %q: keep it in step with auth.go", want)
		}
	}
}
