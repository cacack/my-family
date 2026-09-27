package sqlite

// Test-only hooks into unexported search internals (issue #762).

// EscapeFTS5Query exposes escapeFTS5Query to the external test package.
func EscapeFTS5Query(query string, prefix bool) string {
	return escapeFTS5Query(query, prefix)
}

// FTS5Enabled reports whether the store created its FTS5 tables, i.e. whether
// SearchPersons takes the FTS5 path rather than the LIKE path.
func (s *ReadModelStore) FTS5Enabled() bool {
	return s.fts5
}
