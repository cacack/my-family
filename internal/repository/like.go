package repository

import (
	"strings"
	"unicode"
)

// Plain name search and the place filters match with PostgreSQL's
// `value ILIKE '%' || query || '%'` on every backend (DB-005). The SQLite store
// calls ContainsPattern.Match through a registered SQL function and the memory
// store calls it directly, so the rules live here once:
//
//   - case is folded rune by rune with unicode.ToLower, so "MÜLLER", "Müller"
//     and "müller" all match each other (SQLite's own LOWER and LIKE fold only
//     ASCII);
//   - `%` in the query matches any run of characters and `_` any one character;
//   - a backslash makes the next character literal, as it is ILIKE's default
//     escape character; a trailing backslash escapes the closing `%`.

// likeKind is what one pattern element matches.
type likeKind uint8

const (
	likeRune likeKind = iota // exactly r
	likeOne                  // any one character (`_`)
	likeAny                  // any run of characters, including none (`%`)
)

// likeToken is one element of a compiled pattern.
type likeToken struct {
	kind likeKind
	r    rune
}

// ContainsPattern is a query compiled for ILIKE '%' || query || '%' matching.
type ContainsPattern struct {
	tokens []likeToken
	// plain is the lower-cased query when it has no wildcard or escape, so Match
	// is a substring test; hasPlain says whether it is set.
	plain    string
	hasPlain bool
}

// CompileContains compiles query for Match.
func CompileContains(query string) ContainsPattern {
	if !strings.ContainsAny(query, `%_\`) {
		return ContainsPattern{plain: strings.ToLower(query), hasPlain: true}
	}
	pattern := []rune("%" + query + "%")
	tokens := make([]likeToken, 0, len(pattern))
	for i := 0; i < len(pattern); i++ {
		switch r := pattern[i]; {
		case r == '\\' && i+1 < len(pattern):
			i++
			tokens = append(tokens, likeToken{kind: likeRune, r: unicode.ToLower(pattern[i])})
		case r == '%':
			if n := len(tokens); n > 0 && tokens[n-1].kind == likeAny {
				continue // "%%" is the same as "%"
			}
			tokens = append(tokens, likeToken{kind: likeAny})
		case r == '_':
			tokens = append(tokens, likeToken{kind: likeOne})
		default:
			tokens = append(tokens, likeToken{kind: likeRune, r: unicode.ToLower(r)})
		}
	}
	return ContainsPattern{tokens: tokens}
}

// Match reports whether value ILIKE '%' || query || '%'.
func (p ContainsPattern) Match(value string) bool {
	if p.hasPlain {
		return strings.Contains(strings.ToLower(value), p.plain)
	}
	s := []rune(strings.ToLower(value))
	// Greedy wildcard match with backtracking to the most recent `%`.
	si, ti := 0, 0
	star, mark := -1, 0
	for si < len(s) {
		switch {
		case ti < len(p.tokens) && p.tokens[ti].kind == likeAny:
			star, mark = ti, si
			ti++
		case ti < len(p.tokens) && (p.tokens[ti].kind == likeOne || p.tokens[ti].r == s[si]):
			si++
			ti++
		case star >= 0:
			ti = star + 1
			mark++
			si = mark
		default:
			return false
		}
	}
	for ti < len(p.tokens) && p.tokens[ti].kind == likeAny {
		ti++
	}
	return ti == len(p.tokens)
}

// ContainsFold reports whether value ILIKE '%' || query || '%', the plain-search
// and place-filter match PostgreSQL uses (DB-005).
func ContainsFold(value, query string) bool {
	return CompileContains(query).Match(value)
}
