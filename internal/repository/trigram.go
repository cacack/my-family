package repository

import (
	"unicode"
)

// TrigramThreshold is PostgreSQL pg_trgm's default similarity threshold
// (pg_trgm.similarity_threshold): the `%` operator is true when two strings'
// similarity is at least this value.
const TrigramThreshold = 0.3

// trigram is three consecutive runes of a padded word.
type trigram [3]rune

// trigrams returns the set of trigrams pg_trgm extracts from s: the string is
// lower-cased and split into words of word characters (see isTrigramWordRune;
// every other rune is a separator), each word is padded with two spaces in
// front and one behind, and every run of three consecutive runes of a padded
// word is a trigram. Duplicates collapse, so the result is a set.
func trigrams(s string) map[trigram]struct{} {
	set := make(map[trigram]struct{})
	var word []rune
	flush := func() {
		if len(word) == 0 {
			return
		}
		padded := make([]rune, 0, len(word)+3)
		padded = append(padded, ' ', ' ')
		padded = append(padded, word...)
		padded = append(padded, ' ')
		for i := 0; i+3 <= len(padded); i++ {
			set[trigram{padded[i], padded[i+1], padded[i+2]}] = struct{}{}
		}
		word = word[:0]
	}
	for _, r := range s {
		if isTrigramWordRune(r) {
			word = append(word, unicode.ToLower(r))
			continue
		}
		flush()
	}
	flush()
	return set
}

// isTrigramWordRune reports whether pg_trgm treats r as part of a word. pg_trgm
// asks the database ctype (iswalnum); under glibc's UTF-8 locales, the ones
// PostgreSQL deployments use, that is Unicode's Alphabetic property plus
// digits: letters, letter numbers (Ⅻ) and the Other_Alphabetic marks and
// symbols, such as Indic vowel signs (the ा in शर्मा) and circled letters.
// Checked against every code point PostgreSQL 16 on C.UTF-8 accepts; runes
// newer than Go's Unicode tables are the only ones left out.
func isTrigramWordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) ||
		unicode.Is(unicode.Nl, r) || unicode.Is(unicode.Other_Alphabetic, r)
}

// TrigramSimilarity reports how similar a and b are, from 0 (no trigram in
// common) to 1 (the same trigram sets), matching PostgreSQL pg_trgm's
// similarity(): shared trigrams divided by the size of the union. A string with
// no word characters has no trigrams and is similar to nothing.
func TrigramSimilarity(a, b string) float64 {
	return NewTrigramQuery(a).Similarity(b)
}

// TrigramQuery is a search string with its trigrams extracted once, for scoring
// it against many names.
type TrigramQuery struct {
	set map[trigram]struct{}
}

// NewTrigramQuery prepares query for repeated Similarity calls.
func NewTrigramQuery(query string) TrigramQuery {
	return TrigramQuery{set: trigrams(query)}
}

// Similarity is TrigramSimilarity(query, s).
//
// The ratio is computed in float32, as pg_trgm computes it in float4, so a
// value on the TrigramThreshold boundary compares the same way on every backend.
func (q TrigramQuery) Similarity(s string) float64 {
	other := trigrams(s)
	if len(q.set) == 0 || len(other) == 0 {
		return 0
	}
	shared := 0
	for t := range q.set {
		if _, ok := other[t]; ok {
			shared++
		}
	}
	return float64(float32(shared) / float32(len(q.set)+len(other)-shared))
}

// TrigramMatch reports whether a and b are similar enough for pg_trgm's `%`
// operator, which fuzzy person search uses on every backend (DB-005).
func TrigramMatch(a, b string) bool {
	return TrigramSimilarity(a, b) >= TrigramThreshold
}
