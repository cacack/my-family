package command

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestPreMergeSnapshotName(t *testing.T) {
	if got := preMergeSnapshotName("Byron theory"); got != "Before merging Byron theory" {
		t.Errorf("short name = %q", got)
	}
	for _, branchName := range []string{strings.Repeat("a", 100), strings.Repeat("é", 50), strings.Repeat("a", 84) + "語"} {
		got := preMergeSnapshotName(branchName)
		if len(got) > preMergeSnapshotNameMax {
			t.Errorf("name for a %d-byte branch name is %d bytes, over the limit", len(branchName), len(got))
		}
		if !utf8.ValidString(got) || !strings.HasPrefix(got, preMergeSnapshotPrefix) || !strings.HasSuffix(got, ellipsis) {
			t.Errorf("long name = %q, want a valid, prefixed, ellipsized name", got)
		}
	}
	exact := strings.Repeat("b", preMergeSnapshotNameMax-len(preMergeSnapshotPrefix))
	if got := preMergeSnapshotName(exact); got != preMergeSnapshotPrefix+exact {
		t.Errorf("a name that just fits was shortened: %q", got)
	}
}
