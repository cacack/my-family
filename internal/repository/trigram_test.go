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
		// Word characters are Unicode Alphabetic plus digits, as glibc's
		// iswalnum: Indic vowel signs and Arabic harakat (combining marks) and
		// letter numbers are part of a word, not separators. The virama (्) is
		// not Alphabetic and still splits words.
		{"राजेश", "सुरेश", 0.09090909, false},
		{"शर्मा", "वर्मा", 0.33333334, true},
		{"विष्णु", "कृष्ण", 0.083333336, false},
		{"हिन्दी", "हिंदी", 0.3, true},
		{"فاطمة", "فَاطِمَة", 0.071428575, false},
		{"مُحَمَّد", "محمد", 0.07692308, false},
		{"Ⅻ", "Ⅻ", 1, true},
		{"ⅫⅢ", "ⅻⅲ", 1, true},
		{"\u0345", "\u0345", 1, true},
		{"Ⓐlice", "alice", 0.33333334, true},
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
