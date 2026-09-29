package repository

import (
	"math"
	"testing"
)

// TestTrigramSimilarity_MatchesPgTrgm pins TrigramSimilarity to PostgreSQL. The
// expected values were produced by pg_trgm 1.6 on PostgreSQL 16:
//
//	SELECT a, b, similarity(a, b), a % b FROM (VALUES (...)) v(a, b);
//
// SQLite and in-memory fuzzy search call TrigramMatch; PostgreSQL uses `%`. If
// these drift, fuzzy search returns different people depending on the database
// (DB-005).
func TestTrigramSimilarity_MatchesPgTrgm(t *testing.T) {
	tests := []struct {
		a, b  string
		sim   float64
		match bool
	}{
		{"Smyth", "Smith", 0.33333334, true},
		{"Jon", "John", 0.2857143, false},
		{"Johnsen", "Johnson", 0.45454547, true},
		{"John", "John Smith", 0.45454547, true},
		{"Smyth", "Smith-Jones", 0.2, false},
		{"O'Brian", "O'Brien", 0.45454547, true},
		{"Müller", "MULLER", 0.4, true},
		{"Müller", "müller", 1, true},
		{"", "John", 0, false},
		{"!!", "!!", 0, false},
		{"abc", "abc abc", 1, true},
		{"Jo", "John", 0.33333334, true},
		{"Kathryn", "Catherine", 0.05882353, false},
		{"a", "a", 1, true},
		{"Mary Ann", "Maryann", 0.54545456, true},
		{"Joh", "John", 0.5, true},
		{"Élodie", "ELODIE", 0.4, true},
		{"x1", "X1", 1, true},
	}
	for _, tt := range tests {
		got := TrigramSimilarity(tt.a, tt.b)
		if math.Abs(got-tt.sim) > 1e-6 {
			t.Errorf("TrigramSimilarity(%q, %q) = %v, want %v", tt.a, tt.b, got, tt.sim)
		}
		if m := TrigramMatch(tt.a, tt.b); m != tt.match {
			t.Errorf("TrigramMatch(%q, %q) = %v, want %v", tt.a, tt.b, m, tt.match)
		}
		if rev := TrigramSimilarity(tt.b, tt.a); rev != got {
			t.Errorf("TrigramSimilarity is not symmetric for %q, %q: %v vs %v", tt.a, tt.b, got, rev)
		}
	}
}
