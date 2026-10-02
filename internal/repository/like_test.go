package repository

import "testing"

// TestContainsFold_MatchesPostgresILIKE pins ContainsFold to PostgreSQL. The
// expected values were produced on PostgreSQL 16 (C.UTF-8) with:
//
//	SELECT v, q, v ILIKE '%' || q || '%' FROM (VALUES (...)) t(v, q);
//
// SQLite and in-memory plain search call ContainsFold; PostgreSQL uses ILIKE.
// If these drift, plain search returns different people depending on the
// database (DB-005).
func TestContainsFold_MatchesPostgresILIKE(t *testing.T) {
	tests := []struct {
		value, query string
		want         bool
	}{
		// Case folds for every letter, not only ASCII.
		{"Karl MÜLLER", "müller", true},
		{"Karl MÜLLER", "MÜLLER", true},
		{"Hans Müller", "MÜLLER", true},
		{"Ana ÑÚÑEZ", "ñúñez", true},
		{"ÅBERG", "åberg", true},
		{"John Smith", "hn S", true},
		{"John Smith", "x", false},
		{"", "", true},
		// Backslash escapes the next character.
		{"John Smith", `\s`, true},
		{`Back\slash`, `\\`, true},
		{"Johns", `\\`, false},
		{"100% Smith", `100\%`, true},
		{"A_B", `\_`, true},
		{"AxB", `\_`, false},
		// A trailing backslash escapes the closing %.
		{"John Smith", `Smith\`, false},
		{"John Smith%", `Smith\`, true},
		// % and _ are wildcards.
		{"John Smith", "%", true},
		{"John Smith", "_", true},
		{"John Smith", "J_hn", true},
		{"John Smith", "J%h", true},
		{"John Smith", "h%s", true},
		{"John Smith", "s%h", true},
		{"abc", "c%%", true},
		{"aaa", "a_a_", false},
		{"aaaa", "a_a_", true},
		{"Ölund", "öl_nd", true},
	}
	for _, tt := range tests {
		if got := ContainsFold(tt.value, tt.query); got != tt.want {
			t.Errorf("ContainsFold(%q, %q) = %v, want %v", tt.value, tt.query, got, tt.want)
		}
	}
}
