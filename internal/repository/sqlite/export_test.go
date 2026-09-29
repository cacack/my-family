package sqlite

import "github.com/cacack/my-family/internal/repository"

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

// GlobalHistoryPlan returns SQLite's query plan for ReadGlobalHistory's page
// and count queries, so a test can pin that they walk an index (#739 review).
func (s *EventStore) GlobalHistoryPlan(q repository.GlobalHistoryQuery) (page, count []string, err error) {
	countQuery, pageQuery, args := globalHistorySQL(q)
	explain := func(query string, args ...any) ([]string, error) {
		rows, err := s.db.Query("EXPLAIN QUERY PLAN "+query, args...)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var steps []string
		for rows.Next() {
			var id, parent, unused int
			var detail string
			if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
				return nil, err
			}
			steps = append(steps, detail)
		}
		return steps, rows.Err()
	}
	if page, err = explain(pageQuery, append(append([]any{}, args...), 20, 0)...); err != nil {
		return nil, nil, err
	}
	count, err = explain(countQuery, args...)
	return page, count, err
}
