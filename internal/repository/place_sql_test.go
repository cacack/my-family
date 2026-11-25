package repository

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// placeRuleSQL matches SQL that transforms a place text or key column, which
// is how a store would implement a place-matching rule of its own, or walk
// the hierarchy right to left in SQL. The key columns (birth_place_key and
// the rest) hold PlaceKey's result and may only be compared whole.
var placeRuleSQL = regexp.MustCompile(`(?i)\b(lower|upper|trim|btrim|ltrim|rtrim|regexp_replace|replace|split_part|string_to_array|substr|substring|left|right|length)\s*\(\s*(\w+\.)?(birth_place|death_place|marriage_place|place)(_key)?\b`)

// TestNoSQLPlaceMatchingRule enforces PL-002 on the SQL stores' sources:
// PlaceKey is the only place-matching rule, so no statement case-folds, trims
// or splits a place text or key, and no store registers a place SQL function;
// the hierarchy is walked in Go (ADR-009 §4).
func TestNoSQLPlaceMatchingRule(t *testing.T) {
	for _, dir := range []string{"sqlite", "postgres"} {
		files, err := filepath.Glob(filepath.Join(dir, "*.go"))
		if err != nil || len(files) == 0 {
			t.Fatalf("list %s sources: %v (%d files)", dir, err, len(files))
		}
		for _, f := range files {
			if strings.HasSuffix(f, "_test.go") {
				continue
			}
			src, err := os.ReadFile(f) // #nosec G304 -- a path from the Glob above
			if err != nil {
				t.Fatalf("read %s: %v", f, err)
			}
			for _, m := range placeRuleSQL.FindAll(src, -1) {
				t.Errorf("%s: %q transforms a place text in SQL; compare the stored key instead (PL-002)", f, m)
			}
			if regexp.MustCompile(`Register\w*Function\(\s*"?\w*place`).Match(src) {
				t.Errorf("%s registers a place SQL function; compute the key with PlaceKey (PL-002)", f)
			}
		}
	}
}
