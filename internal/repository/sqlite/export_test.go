package sqlite

import "github.com/cacack/my-family/internal/repository"

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

// SearchPersonsPlan returns SQLite's query plan for a plain (substring) name
// search, so a test can pin that alternate names are not rescanned per person.
func (s *ReadModelStore) SearchPersonsPlan(opts repository.SearchOptions) ([]string, error) {
	query, args := searchPersonsLikeSQL(opts, 20)
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
