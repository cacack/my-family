package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// ReadModelStore is a SQLite implementation of repository.ReadModelStore.
type ReadModelStore struct {
	db *sql.DB
}

// NewReadModelStore creates a new SQLite read model store.
func NewReadModelStore(db *sql.DB) (*ReadModelStore, error) {
	store := &ReadModelStore{db: db}
	if err := store.createTables(); err != nil {
		return nil, fmt.Errorf("failed to create tables: %w", err)
	}
	return store, nil
}

// createTables creates the read model schema if it doesn't exist.
func (s *ReadModelStore) createTables() error {
	// Create core tables
	_, err := s.db.Exec(`
		-- Persons table
		CREATE TABLE IF NOT EXISTS persons (
			id TEXT PRIMARY KEY,
			given_name TEXT NOT NULL,
			surname TEXT NOT NULL,
			full_name TEXT GENERATED ALWAYS AS (given_name || ' ' || surname) STORED,
			gender TEXT,
			birth_date_raw TEXT,
			birth_date_sort TEXT,
			birth_place TEXT,
			death_date_raw TEXT,
			death_date_sort TEXT,
			death_place TEXT,
			notes TEXT,
			research_status TEXT,
			version INTEGER NOT NULL DEFAULT 1,
			updated_at TEXT NOT NULL DEFAULT (datetime('now'))
		);

		CREATE INDEX IF NOT EXISTS idx_persons_surname ON persons(surname, given_name);
		CREATE INDEX IF NOT EXISTS idx_persons_birth_date ON persons(birth_date_sort);
		CREATE INDEX IF NOT EXISTS idx_persons_full_name ON persons(full_name);
		CREATE INDEX IF NOT EXISTS idx_persons_research_status ON persons(research_status);

		-- Families table
		CREATE TABLE IF NOT EXISTS families (
			id TEXT PRIMARY KEY,
			partner1_id TEXT,
			partner1_name TEXT,
			partner2_id TEXT,
			partner2_name TEXT,
			relationship_type TEXT,
			marriage_date_raw TEXT,
			marriage_date_sort TEXT,
			marriage_place TEXT,
			child_count INTEGER NOT NULL DEFAULT 0,
			version INTEGER NOT NULL DEFAULT 1,
			updated_at TEXT NOT NULL DEFAULT (datetime('now')),
			FOREIGN KEY (partner1_id) REFERENCES persons(id),
			FOREIGN KEY (partner2_id) REFERENCES persons(id)
		);

		CREATE INDEX IF NOT EXISTS idx_families_partner1 ON families(partner1_id);
		CREATE INDEX IF NOT EXISTS idx_families_partner2 ON families(partner2_id);

		-- Family children table
		CREATE TABLE IF NOT EXISTS family_children (
			family_id TEXT NOT NULL,
			person_id TEXT NOT NULL,
			person_name TEXT,
			relationship_type TEXT NOT NULL DEFAULT 'biological',
			sequence INTEGER,
			PRIMARY KEY (family_id, person_id),
			FOREIGN KEY (family_id) REFERENCES families(id) ON DELETE CASCADE,
			FOREIGN KEY (person_id) REFERENCES persons(id)
		);

		CREATE INDEX IF NOT EXISTS idx_family_children_person ON family_children(person_id);

		-- Pedigree edges table
		CREATE TABLE IF NOT EXISTS pedigree_edges (
			person_id TEXT PRIMARY KEY,
			father_id TEXT,
			mother_id TEXT,
			father_name TEXT,
			mother_name TEXT,
			FOREIGN KEY (person_id) REFERENCES persons(id) ON DELETE CASCADE,
			FOREIGN KEY (father_id) REFERENCES persons(id),
			FOREIGN KEY (mother_id) REFERENCES persons(id)
		);

		CREATE INDEX IF NOT EXISTS idx_pedigree_father ON pedigree_edges(father_id);
		CREATE INDEX IF NOT EXISTS idx_pedigree_mother ON pedigree_edges(mother_id);

		-- Sources table
		CREATE TABLE IF NOT EXISTS sources (
			id TEXT PRIMARY KEY,
			source_type TEXT NOT NULL,
			title TEXT NOT NULL,
			author TEXT,
			publisher TEXT,
			publish_date_raw TEXT,
			publish_date_sort TEXT,
			url TEXT,
			repository_name TEXT,
			collection_name TEXT,
			call_number TEXT,
			notes TEXT,
			gedcom_xref TEXT,
			citation_count INTEGER NOT NULL DEFAULT 0,
			version INTEGER NOT NULL DEFAULT 1,
			updated_at TEXT NOT NULL DEFAULT (datetime('now'))
		);

		CREATE INDEX IF NOT EXISTS idx_sources_title ON sources(title);
		CREATE INDEX IF NOT EXISTS idx_sources_type ON sources(source_type);

		-- Citations table
		CREATE TABLE IF NOT EXISTS citations (
			id TEXT PRIMARY KEY,
			source_id TEXT NOT NULL,
			source_title TEXT,
			fact_type TEXT NOT NULL,
			fact_owner_id TEXT NOT NULL,
			page TEXT,
			volume TEXT,
			source_quality TEXT,
			informant_type TEXT,
			evidence_type TEXT,
			quoted_text TEXT,
			analysis TEXT,
			template_id TEXT,
			gedcom_xref TEXT,
			version INTEGER NOT NULL DEFAULT 1,
			created_at TEXT NOT NULL DEFAULT (datetime('now')),
			FOREIGN KEY (source_id) REFERENCES sources(id)
		);

		CREATE INDEX IF NOT EXISTS idx_citations_source ON citations(source_id);
		CREATE INDEX IF NOT EXISTS idx_citations_fact ON citations(fact_type, fact_owner_id);
		CREATE INDEX IF NOT EXISTS idx_citations_owner ON citations(fact_owner_id);

		-- Media table
		CREATE TABLE IF NOT EXISTS media (
			id TEXT PRIMARY KEY,
			entity_type TEXT NOT NULL,
			entity_id TEXT NOT NULL,
			title TEXT NOT NULL,
			description TEXT,
			mime_type TEXT NOT NULL,
			media_type TEXT NOT NULL,
			filename TEXT NOT NULL,
			file_size INTEGER NOT NULL,
			file_data BLOB NOT NULL,
			thumbnail_data BLOB,
			crop_left INTEGER,
			crop_top INTEGER,
			crop_width INTEGER,
			crop_height INTEGER,
			gedcom_xref TEXT,
			version INTEGER NOT NULL DEFAULT 1,
			created_at TEXT NOT NULL DEFAULT (datetime('now')),
			updated_at TEXT NOT NULL DEFAULT (datetime('now')),
			-- GEDCOM 7.0 enhanced fields
			files TEXT,        -- JSON array of file references
			format TEXT,       -- Primary format/MIME type
			translations TEXT  -- JSON array of translated titles
		);

		CREATE INDEX IF NOT EXISTS idx_media_entity ON media(entity_type, entity_id);
		CREATE INDEX IF NOT EXISTS idx_media_type ON media(media_type);

		-- Person names table (for multiple name variants)
		CREATE TABLE IF NOT EXISTS person_names (
			id TEXT PRIMARY KEY,
			person_id TEXT NOT NULL,
			given_name TEXT NOT NULL,
			surname TEXT NOT NULL,
			full_name TEXT GENERATED ALWAYS AS (given_name || ' ' || surname) STORED,
			name_prefix TEXT,
			name_suffix TEXT,
			surname_prefix TEXT,
			nickname TEXT,
			name_type TEXT NOT NULL DEFAULT '',
			is_primary INTEGER NOT NULL DEFAULT 0,
			updated_at TEXT NOT NULL DEFAULT (datetime('now')),
			FOREIGN KEY (person_id) REFERENCES persons(id) ON DELETE CASCADE
		);

		CREATE INDEX IF NOT EXISTS idx_person_names_person ON person_names(person_id);
		CREATE INDEX IF NOT EXISTS idx_person_names_primary ON person_names(person_id, is_primary);

		-- Notes table (shared GEDCOM NOTE records)
		CREATE TABLE IF NOT EXISTS notes (
			id TEXT PRIMARY KEY,
			text TEXT NOT NULL,
			gedcom_xref TEXT,
			version INTEGER NOT NULL DEFAULT 1,
			updated_at TEXT NOT NULL DEFAULT (datetime('now'))
		);

		CREATE INDEX IF NOT EXISTS idx_notes_gedcom_xref ON notes(gedcom_xref);

		-- Submitters table (GEDCOM SUBM records for file provenance)
		CREATE TABLE IF NOT EXISTS submitters (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			address TEXT,
			phone TEXT,
			email TEXT,
			language TEXT,
			media_id TEXT,
			gedcom_xref TEXT,
			version INTEGER NOT NULL DEFAULT 1,
			updated_at TEXT NOT NULL DEFAULT (datetime('now'))
		);

		CREATE INDEX IF NOT EXISTS idx_submitters_gedcom_xref ON submitters(gedcom_xref);

		-- Associations table (GEDCOM ASSO records for non-family relationships)
		CREATE TABLE IF NOT EXISTS associations (
			id TEXT PRIMARY KEY,
			person_id TEXT NOT NULL,
			person_name TEXT,
			associate_id TEXT NOT NULL,
			associate_name TEXT,
			role TEXT NOT NULL,
			phrase TEXT,
			notes TEXT,
			note_ids TEXT,
			gedcom_xref TEXT,
			version INTEGER NOT NULL DEFAULT 1,
			updated_at TEXT NOT NULL DEFAULT (datetime('now')),
			FOREIGN KEY (person_id) REFERENCES persons(id) ON DELETE CASCADE,
			FOREIGN KEY (associate_id) REFERENCES persons(id) ON DELETE CASCADE
		);

		CREATE INDEX IF NOT EXISTS idx_associations_person ON associations(person_id);
		CREATE INDEX IF NOT EXISTS idx_associations_associate ON associations(associate_id);
		CREATE INDEX IF NOT EXISTS idx_associations_role ON associations(role);

		-- Events table (life events for persons and families)
		CREATE TABLE IF NOT EXISTS events (
			id TEXT PRIMARY KEY,
			owner_type TEXT NOT NULL,
			owner_id TEXT NOT NULL,
			fact_type TEXT NOT NULL,
			date_raw TEXT,
			date_sort TEXT,
			place TEXT,
			place_lat TEXT,
			place_long TEXT,
			address TEXT,
			description TEXT,
			cause TEXT,
			age TEXT,
			research_status TEXT,
			version INTEGER NOT NULL DEFAULT 1,
			created_at TEXT NOT NULL DEFAULT (datetime('now'))
		);

		CREATE INDEX IF NOT EXISTS idx_events_owner ON events(owner_type, owner_id);
		CREATE INDEX IF NOT EXISTS idx_events_fact_type ON events(fact_type);

		-- Attributes table (person attributes)
		CREATE TABLE IF NOT EXISTS attributes (
			id TEXT PRIMARY KEY,
			person_id TEXT NOT NULL,
			fact_type TEXT NOT NULL,
			value TEXT NOT NULL DEFAULT '',
			date_raw TEXT,
			date_sort TEXT,
			place TEXT,
			version INTEGER NOT NULL DEFAULT 1,
			created_at TEXT NOT NULL DEFAULT (datetime('now')),
			FOREIGN KEY (person_id) REFERENCES persons(id)
		);

		CREATE INDEX IF NOT EXISTS idx_attributes_person ON attributes(person_id);
		CREATE INDEX IF NOT EXISTS idx_attributes_fact_type ON attributes(fact_type);

		-- LDS Ordinances table
		CREATE TABLE IF NOT EXISTS lds_ordinances (
			id TEXT PRIMARY KEY,
			type TEXT NOT NULL,
			type_label TEXT NOT NULL,
			person_id TEXT,
			person_name TEXT,
			family_id TEXT,
			date_raw TEXT,
			date_sort TEXT,
			place TEXT,
			temple TEXT,
			status TEXT,
			version INTEGER NOT NULL DEFAULT 1,
			updated_at TEXT NOT NULL DEFAULT (datetime('now'))
		);

		CREATE INDEX IF NOT EXISTS idx_lds_ordinances_person ON lds_ordinances(person_id);
		CREATE INDEX IF NOT EXISTS idx_lds_ordinances_family ON lds_ordinances(family_id);
		CREATE INDEX IF NOT EXISTS idx_lds_ordinances_type ON lds_ordinances(type);
	`)
	if err != nil {
		return err
	}

	// Try to create FTS5 table (optional - falls back to LIKE if not available)
	s.tryCreateFTS5()

	// Run schema migrations for existing databases
	s.runMigrations()

	return nil
}

// runMigrations applies schema changes for existing databases.
func (s *ReadModelStore) runMigrations() {
	// Add research_status column if it doesn't exist (for databases created before this column was added)
	_, _ = s.db.Exec(`ALTER TABLE persons ADD COLUMN research_status TEXT`)
	_, _ = s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_persons_research_status ON persons(research_status)`)

	// Add place coordinate columns for geographic features (issue #105)
	_, _ = s.db.Exec(`ALTER TABLE persons ADD COLUMN birth_place_lat TEXT`)
	_, _ = s.db.Exec(`ALTER TABLE persons ADD COLUMN birth_place_long TEXT`)
	_, _ = s.db.Exec(`ALTER TABLE persons ADD COLUMN death_place_lat TEXT`)
	_, _ = s.db.Exec(`ALTER TABLE persons ADD COLUMN death_place_long TEXT`)
	_, _ = s.db.Exec(`ALTER TABLE families ADD COLUMN marriage_place_lat TEXT`)
	_, _ = s.db.Exec(`ALTER TABLE families ADD COLUMN marriage_place_long TEXT`)
}

// tryCreateFTS5 attempts to create FTS5 virtual table for full-text search.
// If FTS5 is not available, search will fall back to LIKE-based queries.
func (s *ReadModelStore) tryCreateFTS5() {
	// Try to create FTS5 virtual table for persons
	_, err := s.db.Exec(`
		CREATE VIRTUAL TABLE IF NOT EXISTS persons_fts USING fts5(
			given_name,
			surname,
			content='persons',
			content_rowid='rowid'
		)
	`)
	if err != nil {
		// FTS5 not available, search will use LIKE fallback
		return
	}

	// Create triggers to keep FTS in sync (errors non-critical with IF NOT EXISTS)
	_, _ = s.db.Exec(`
		CREATE TRIGGER IF NOT EXISTS persons_fts_insert AFTER INSERT ON persons BEGIN
			INSERT INTO persons_fts(rowid, given_name, surname)
			SELECT rowid, NEW.given_name, NEW.surname FROM persons WHERE id = NEW.id;
		END
	`)

	_, _ = s.db.Exec(`
		CREATE TRIGGER IF NOT EXISTS persons_fts_delete AFTER DELETE ON persons BEGIN
			INSERT INTO persons_fts(persons_fts, rowid, given_name, surname)
			VALUES('delete', OLD.rowid, OLD.given_name, OLD.surname);
		END
	`)

	_, _ = s.db.Exec(`
		CREATE TRIGGER IF NOT EXISTS persons_fts_update AFTER UPDATE ON persons BEGIN
			INSERT INTO persons_fts(persons_fts, rowid, given_name, surname)
			VALUES('delete', OLD.rowid, OLD.given_name, OLD.surname);
			INSERT INTO persons_fts(rowid, given_name, surname)
			SELECT rowid, NEW.given_name, NEW.surname FROM persons WHERE id = NEW.id;
		END
	`)

	// Create FTS5 virtual table for person_names
	_, _ = s.db.Exec(`
		CREATE VIRTUAL TABLE IF NOT EXISTS person_names_fts USING fts5(
			given_name,
			surname,
			nickname,
			content='person_names',
			content_rowid='rowid'
		)
	`)

	// Create triggers for person_names FTS
	_, _ = s.db.Exec(`
		CREATE TRIGGER IF NOT EXISTS person_names_fts_insert AFTER INSERT ON person_names BEGIN
			INSERT INTO person_names_fts(rowid, given_name, surname, nickname)
			SELECT rowid, NEW.given_name, NEW.surname, COALESCE(NEW.nickname, '')
			FROM person_names WHERE id = NEW.id;
		END
	`)

	_, _ = s.db.Exec(`
		CREATE TRIGGER IF NOT EXISTS person_names_fts_delete AFTER DELETE ON person_names BEGIN
			INSERT INTO person_names_fts(person_names_fts, rowid, given_name, surname, nickname)
			VALUES('delete', OLD.rowid, OLD.given_name, OLD.surname, COALESCE(OLD.nickname, ''));
		END
	`)

	_, _ = s.db.Exec(`
		CREATE TRIGGER IF NOT EXISTS person_names_fts_update AFTER UPDATE ON person_names BEGIN
			INSERT INTO person_names_fts(person_names_fts, rowid, given_name, surname, nickname)
			VALUES('delete', OLD.rowid, OLD.given_name, OLD.surname, COALESCE(OLD.nickname, ''));
			INSERT INTO person_names_fts(rowid, given_name, surname, nickname)
			SELECT rowid, NEW.given_name, NEW.surname, COALESCE(NEW.nickname, '')
			FROM person_names WHERE id = NEW.id;
		END
	`)
}

// GetPerson retrieves a person by ID.
func (s *ReadModelStore) GetPerson(ctx context.Context, id uuid.UUID) (*repository.PersonReadModel, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, given_name, surname, full_name, gender,
			   birth_date_raw, birth_date_sort, birth_place, birth_place_lat, birth_place_long,
			   death_date_raw, death_date_sort, death_place, death_place_lat, death_place_long,
			   notes, research_status, version, updated_at
		FROM persons WHERE id = ?
	`, id.String())

	return scanPerson(row)
}

// ListPersons returns a paginated list of persons.
func (s *ReadModelStore) ListPersons(ctx context.Context, opts repository.ListOptions) ([]repository.PersonReadModel, int, error) {
	// Build WHERE clause for research_status filter
	whereClause := ""
	var whereArgs []any
	if opts.ResearchStatus != nil {
		if *opts.ResearchStatus == "unset" {
			whereClause = "WHERE research_status IS NULL OR research_status = ''"
		} else {
			whereClause = "WHERE research_status = ?"
			whereArgs = append(whereArgs, *opts.ResearchStatus)
		}
	}

	// Count total (with filter if present)
	var total int
	countQuery := "SELECT COUNT(*) FROM persons " + whereClause
	err := s.db.QueryRowContext(ctx, countQuery, whereArgs...).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("count persons: %w", err)
	}

	// Build order clause
	orderColumn := "surname"
	switch opts.Sort {
	case "given_name":
		orderColumn = "given_name"
	case "birth_date":
		orderColumn = "birth_date_sort"
	case "updated_at":
		orderColumn = "updated_at"
	}
	orderDir := "ASC"
	if opts.Order == "desc" {
		orderDir = "DESC"
	}

	// Build query with filter
	// #nosec G201 -- orderColumn and orderDir are validated via switch/if above, not user input
	query := fmt.Sprintf(`
		SELECT id, given_name, surname, full_name, gender,
			   birth_date_raw, birth_date_sort, birth_place, birth_place_lat, birth_place_long,
			   death_date_raw, death_date_sort, death_place, death_place_lat, death_place_long,
			   notes, research_status, version, updated_at
		FROM persons
		%s
		ORDER BY %s %s, given_name %s
		LIMIT ? OFFSET ?
	`, whereClause, orderColumn, orderDir, orderDir)

	// Build args: where args + limit + offset
	queryArgs := append(whereArgs, opts.Limit, opts.Offset)
	rows, err := s.db.QueryContext(ctx, query, queryArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("query persons: %w", err)
	}
	defer rows.Close()

	var persons []repository.PersonReadModel
	for rows.Next() {
		p, err := scanPersonRow(rows)
		if err != nil {
			return nil, 0, err
		}
		persons = append(persons, *p)
	}

	return persons, total, rows.Err()
}

// SearchPersons searches for persons using FTS5, Soundex, date ranges, and place filters.
func (s *ReadModelStore) SearchPersons(ctx context.Context, opts repository.SearchOptions) ([]repository.PersonReadModel, error) {
	// Normalize limit
	limit := opts.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	hasQuery := opts.Query != ""
	hasDateFilter := opts.BirthDateFrom != nil || opts.BirthDateTo != nil ||
		opts.DeathDateFrom != nil || opts.DeathDateTo != nil
	hasPlaceFilter := opts.BirthPlace != "" || opts.DeathPlace != ""

	// Soundex: fetch candidates with SQL filters, then post-filter in Go
	if hasQuery && opts.Soundex {
		return s.searchPersonsSoundex(ctx, opts, limit)
	}

	// FTS5 or LIKE name matching combined with date/place SQL filters
	if hasQuery {
		return s.searchPersonsFTS(ctx, opts, limit)
	}

	// No text query — filter only by date/place
	if hasDateFilter || hasPlaceFilter {
		return s.searchPersonsFiltersOnly(ctx, opts, limit)
	}

	// No criteria at all — return empty
	return nil, nil
}

// searchPersonsFTS uses FTS5 (with LIKE fallback) combined with date/place SQL filters.
func (s *ReadModelStore) searchPersonsFTS(ctx context.Context, opts repository.SearchOptions, limit int) ([]repository.PersonReadModel, error) {
	// Build date/place filter conditions for the WHERE clause on p.*
	filterSQL, filterArgs := buildDatePlaceFilters(opts)

	ftsQuery := escapeFTS5Query(opts.Query)
	if opts.Fuzzy {
		ftsQuery += "*"
	}

	orderClause := searchOrderClause(opts, "", true)

	// Build the CTE query with optional date/place filters
	var sb strings.Builder
	var args []any

	sb.WriteString(`
		WITH matched_persons AS (
			SELECT p.id, p.given_name, p.surname, p.full_name, p.gender,
				   p.birth_date_raw, p.birth_date_sort, p.birth_place, p.birth_place_lat, p.birth_place_long,
				   p.death_date_raw, p.death_date_sort, p.death_place, p.death_place_lat, p.death_place_long,
				   p.notes, p.research_status, p.version, p.updated_at, 1 as is_primary, rank as search_rank
			FROM persons p
			JOIN persons_fts fts ON p.rowid = fts.rowid
			WHERE persons_fts MATCH ?`)
	args = append(args, ftsQuery)

	if filterSQL != "" {
		sb.WriteString(" AND " + filterSQL)
		args = append(args, filterArgs...)
	}

	sb.WriteString(`

			UNION

			SELECT p.id, p.given_name, p.surname, p.full_name, p.gender,
				   p.birth_date_raw, p.birth_date_sort, p.birth_place, p.birth_place_lat, p.birth_place_long,
				   p.death_date_raw, p.death_date_sort, p.death_place, p.death_place_lat, p.death_place_long,
				   p.notes, p.research_status, p.version, p.updated_at, pn.is_primary, nfts.rank as search_rank
			FROM persons p
			JOIN person_names pn ON p.id = pn.person_id
			JOIN person_names_fts nfts ON pn.rowid = nfts.rowid
			WHERE person_names_fts MATCH ?`)
	args = append(args, ftsQuery)

	if filterSQL != "" {
		sb.WriteString(" AND " + filterSQL)
		args = append(args, filterArgs...)
	}

	sb.WriteString(`
		)
		SELECT DISTINCT id, given_name, surname, full_name, gender,
			   birth_date_raw, birth_date_sort, birth_place, birth_place_lat, birth_place_long,
			   death_date_raw, death_date_sort, death_place, death_place_lat, death_place_long,
			   notes, research_status, version, updated_at
		FROM matched_persons
		ORDER BY ` + orderClause + `
		LIMIT ?`)
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, sb.String(), args...)
	if err != nil {
		// Fallback to LIKE if FTS5 query fails (e.g., for special characters)
		return s.searchPersonsLike(ctx, opts, limit)
	}
	defer rows.Close()

	persons, err := scanPersonRows(rows)
	if err != nil {
		return nil, err
	}

	// If no FTS results and fuzzy, try LIKE fallback
	if len(persons) == 0 && opts.Fuzzy {
		return s.searchPersonsLike(ctx, opts, limit)
	}

	return persons, nil
}

// searchPersonsLike is a fallback search using LIKE, including person_names and date/place filters.
func (s *ReadModelStore) searchPersonsLike(ctx context.Context, opts repository.SearchOptions, limit int) ([]repository.PersonReadModel, error) {
	likeQuery := "%" + strings.ToLower(opts.Query) + "%"
	filterSQL, filterArgs := buildDatePlaceFilters(opts)
	orderClause := searchOrderClause(opts, "p.", false)

	var sb strings.Builder
	var args []any

	sb.WriteString(`
		SELECT DISTINCT p.id, p.given_name, p.surname, p.full_name, p.gender,
			   p.birth_date_raw, p.birth_date_sort, p.birth_place, p.birth_place_lat, p.birth_place_long,
			   p.death_date_raw, p.death_date_sort, p.death_place, p.death_place_lat, p.death_place_long,
			   p.notes, p.research_status, p.version, p.updated_at
		FROM persons p
		LEFT JOIN person_names pn ON p.id = pn.person_id
		WHERE (LOWER(p.full_name) LIKE ? OR LOWER(p.given_name) LIKE ? OR LOWER(p.surname) LIKE ?
		   OR LOWER(pn.full_name) LIKE ? OR LOWER(pn.given_name) LIKE ? OR LOWER(pn.surname) LIKE ?
		   OR LOWER(pn.nickname) LIKE ?)`)
	args = append(args, likeQuery, likeQuery, likeQuery, likeQuery, likeQuery, likeQuery, likeQuery)

	if filterSQL != "" {
		sb.WriteString(" AND " + filterSQL)
		args = append(args, filterArgs...)
	}

	sb.WriteString(" ORDER BY " + orderClause + " LIMIT ?")
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, sb.String(), args...)
	if err != nil {
		return nil, fmt.Errorf("search persons: %w", err)
	}
	defer rows.Close()

	return scanPersonRows(rows)
}

// searchPersonsSoundex fetches candidates filtered by date/place, then post-filters using Soundex in Go.
func (s *ReadModelStore) searchPersonsSoundex(ctx context.Context, opts repository.SearchOptions, limit int) ([]repository.PersonReadModel, error) {
	filterSQL, filterArgs := buildDatePlaceFilters(opts)

	// Fetch a large candidate set (up to 1000) narrowed by date/place filters
	candidateLimit := 1000

	var sb strings.Builder
	var args []any

	sb.WriteString(`
		SELECT p.id, p.given_name, p.surname, p.full_name, p.gender,
			   p.birth_date_raw, p.birth_date_sort, p.birth_place, p.birth_place_lat, p.birth_place_long,
			   p.death_date_raw, p.death_date_sort, p.death_place, p.death_place_lat, p.death_place_long,
			   p.notes, p.research_status, p.version, p.updated_at
		FROM persons p`)

	if filterSQL != "" {
		sb.WriteString(" WHERE " + filterSQL)
		args = append(args, filterArgs...)
	}

	sb.WriteString(" LIMIT ?")
	args = append(args, candidateLimit)

	rows, err := s.db.QueryContext(ctx, sb.String(), args...)
	if err != nil {
		return nil, fmt.Errorf("search persons soundex: %w", err)
	}
	defer rows.Close()

	candidates, err := scanPersonRows(rows)
	if err != nil {
		return nil, err
	}

	// Split query into words for soundex comparison
	queryWords := strings.Fields(opts.Query)

	// Also load person_names for Soundex matching on alternate names
	namesByPerson := make(map[string][]nameEntry)
	if len(candidates) > 0 {
		var err error
		namesByPerson, err = s.loadPersonNamesForSoundex(ctx, candidates)
		if err != nil {
			return nil, fmt.Errorf("load person names for soundex: %w", err)
		}
	}

	// Post-filter: keep persons where any query word Soundex-matches given_name or surname
	var results []repository.PersonReadModel
	for _, p := range candidates {
		if personMatchesSoundex(p, queryWords, namesByPerson[p.ID.String()]) {
			results = append(results, p)
		}
	}

	// Sort before applying limit
	sortPersonResults(results, opts)
	if len(results) > limit {
		results = results[:limit]
	}

	return results, nil
}

// nameEntry holds name fields for Soundex comparison.
type nameEntry struct {
	GivenName string
	Surname   string
	Nickname  string
}

// loadPersonNamesForSoundex loads alternate names for a set of persons.
// Batches queries to stay within SQLite's 999 parameter limit.
func (s *ReadModelStore) loadPersonNamesForSoundex(ctx context.Context, persons []repository.PersonReadModel) (map[string][]nameEntry, error) {
	if len(persons) == 0 {
		return nil, nil
	}

	const batchSize = 900 // Stay well under SQLite's 999 parameter limit
	result := make(map[string][]nameEntry)

	for start := 0; start < len(persons); start += batchSize {
		end := start + batchSize
		if end > len(persons) {
			end = len(persons)
		}
		batch := persons[start:end]

		var sb strings.Builder
		var args []any
		sb.WriteString("SELECT person_id, given_name, surname, nickname FROM person_names WHERE person_id IN (")
		for i, p := range batch {
			if i > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString("?")
			args = append(args, p.ID.String())
		}
		sb.WriteString(")")

		rows, err := s.db.QueryContext(ctx, sb.String(), args...)
		if err != nil {
			return nil, err
		}

		for rows.Next() {
			var personID, givenName, surname string
			var nickname sql.NullString
			if err := rows.Scan(&personID, &givenName, &surname, &nickname); err != nil {
				rows.Close()
				return nil, err
			}
			result[personID] = append(result[personID], nameEntry{
				GivenName: givenName,
				Surname:   surname,
				Nickname:  nickname.String,
			})
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
	}

	return result, nil
}

// personMatchesSoundex checks if any query word Soundex-matches a person's names.
func personMatchesSoundex(p repository.PersonReadModel, queryWords []string, altNames []nameEntry) bool {
	for _, word := range queryWords {
		if repository.SoundexMatch(word, p.GivenName) || repository.SoundexMatch(word, p.Surname) {
			return true
		}
		for _, name := range altNames {
			if repository.SoundexMatch(word, name.GivenName) || repository.SoundexMatch(word, name.Surname) || repository.SoundexMatch(word, name.Nickname) {
				return true
			}
		}
	}
	return false
}

// searchPersonsFiltersOnly searches using only date/place filters (no text query).
func (s *ReadModelStore) searchPersonsFiltersOnly(ctx context.Context, opts repository.SearchOptions, limit int) ([]repository.PersonReadModel, error) {
	filterSQL, filterArgs := buildDatePlaceFilters(opts)
	orderClause := searchOrderClause(opts, "p.", false)

	var sb strings.Builder
	var args []any

	sb.WriteString(`
		SELECT p.id, p.given_name, p.surname, p.full_name, p.gender,
			   p.birth_date_raw, p.birth_date_sort, p.birth_place, p.birth_place_lat, p.birth_place_long,
			   p.death_date_raw, p.death_date_sort, p.death_place, p.death_place_lat, p.death_place_long,
			   p.notes, p.research_status, p.version, p.updated_at
		FROM persons p`)

	if filterSQL != "" {
		sb.WriteString(" WHERE " + filterSQL)
		args = append(args, filterArgs...)
	}

	sb.WriteString(" ORDER BY " + orderClause + " LIMIT ?")
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, sb.String(), args...)
	if err != nil {
		return nil, fmt.Errorf("search persons filters: %w", err)
	}
	defer rows.Close()

	return scanPersonRows(rows)
}

// buildDatePlaceFilters builds SQL WHERE conditions for date range and place filters.
// Returns the SQL fragment (without leading WHERE/AND) and args.
func buildDatePlaceFilters(opts repository.SearchOptions) (string, []any) {
	var conditions []string
	var args []any

	if opts.BirthDateFrom != nil {
		conditions = append(conditions, "p.birth_date_sort >= ?")
		args = append(args, opts.BirthDateFrom.Format("2006-01-02"))
	}
	if opts.BirthDateTo != nil {
		conditions = append(conditions, "p.birth_date_sort <= ?")
		args = append(args, opts.BirthDateTo.Format("2006-01-02"))
	}
	if opts.DeathDateFrom != nil {
		conditions = append(conditions, "p.death_date_sort >= ?")
		args = append(args, opts.DeathDateFrom.Format("2006-01-02"))
	}
	if opts.DeathDateTo != nil {
		conditions = append(conditions, "p.death_date_sort <= ?")
		args = append(args, opts.DeathDateTo.Format("2006-01-02"))
	}
	if opts.BirthPlace != "" {
		conditions = append(conditions, "p.birth_place LIKE '%' || ? || '%' COLLATE NOCASE")
		args = append(args, opts.BirthPlace)
	}
	if opts.DeathPlace != "" {
		conditions = append(conditions, "p.death_place LIKE '%' || ? || '%' COLLATE NOCASE")
		args = append(args, opts.DeathPlace)
	}

	if len(conditions) == 0 {
		return "", nil
	}
	return strings.Join(conditions, " AND "), args
}

// searchOrderClause returns the SQL ORDER BY columns for search results.
// prefix is the table alias prefix (e.g., "p." for JOINed queries, "" for CTEs).
// hasFTSRank indicates whether FTS rank columns are available.
func searchOrderClause(opts repository.SearchOptions, prefix string, hasFTSRank bool) string {
	dir := "ASC"
	if strings.EqualFold(opts.Order, "desc") {
		dir = "DESC"
	}

	switch opts.Sort {
	case "name":
		return prefix + "surname " + dir + ", " + prefix + "given_name " + dir
	case "birth_date":
		return prefix + "birth_date_sort " + dir
	case "death_date":
		return prefix + "death_date_sort " + dir
	case "relevance":
		if hasFTSRank {
			return "is_primary DESC, search_rank"
		}
		return prefix + "surname " + dir + ", " + prefix + "given_name " + dir
	default:
		if hasFTSRank {
			return "is_primary DESC, search_rank"
		}
		return prefix + "surname " + dir + ", " + prefix + "given_name " + dir
	}
}

// sortPersonResults sorts person results in-place for Soundex (Go-level sorting).
func sortPersonResults(persons []repository.PersonReadModel, opts repository.SearchOptions) {
	if len(persons) <= 1 {
		return
	}

	less := func(i, j int) bool {
		a, b := persons[i], persons[j]
		switch opts.Sort {
		case "birth_date":
			if a.BirthDateSort == nil && b.BirthDateSort == nil {
				return false
			}
			if a.BirthDateSort == nil {
				return false
			}
			if b.BirthDateSort == nil {
				return true
			}
			return a.BirthDateSort.Before(*b.BirthDateSort)
		case "death_date":
			if a.DeathDateSort == nil && b.DeathDateSort == nil {
				return false
			}
			if a.DeathDateSort == nil {
				return false
			}
			if b.DeathDateSort == nil {
				return true
			}
			return a.DeathDateSort.Before(*b.DeathDateSort)
		default: // "name", "relevance", or empty
			if a.Surname != b.Surname {
				return a.Surname < b.Surname
			}
			return a.GivenName < b.GivenName
		}
	}

	if strings.EqualFold(opts.Order, "desc") {
		sort.Slice(persons, func(i, j int) bool { return less(j, i) })
	} else {
		sort.Slice(persons, less)
	}
}

// scanPersonRows scans all rows into PersonReadModel slice.
func scanPersonRows(rows *sql.Rows) ([]repository.PersonReadModel, error) {
	var persons []repository.PersonReadModel
	for rows.Next() {
		p, err := scanPersonRow(rows)
		if err != nil {
			return nil, err
		}
		persons = append(persons, *p)
	}
	return persons, rows.Err()
}

// SavePerson saves or updates a person.
func (s *ReadModelStore) SavePerson(ctx context.Context, person *repository.PersonReadModel) error {
	var birthDateSort, deathDateSort sql.NullString
	if person.BirthDateSort != nil {
		birthDateSort = sql.NullString{String: person.BirthDateSort.Format("2006-01-02"), Valid: true}
	}
	if person.DeathDateSort != nil {
		deathDateSort = sql.NullString{String: person.DeathDateSort.Format("2006-01-02"), Valid: true}
	}

	// Convert coordinate pointers to nullable strings
	var birthPlaceLat, birthPlaceLong, deathPlaceLat, deathPlaceLong sql.NullString
	if person.BirthPlaceLat != nil {
		birthPlaceLat = sql.NullString{String: *person.BirthPlaceLat, Valid: true}
	}
	if person.BirthPlaceLong != nil {
		birthPlaceLong = sql.NullString{String: *person.BirthPlaceLong, Valid: true}
	}
	if person.DeathPlaceLat != nil {
		deathPlaceLat = sql.NullString{String: *person.DeathPlaceLat, Valid: true}
	}
	if person.DeathPlaceLong != nil {
		deathPlaceLong = sql.NullString{String: *person.DeathPlaceLong, Valid: true}
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO persons (id, given_name, surname, gender, birth_date_raw, birth_date_sort, birth_place,
							 birth_place_lat, birth_place_long, death_date_raw, death_date_sort, death_place,
							 death_place_lat, death_place_long, notes, research_status, version, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			given_name = excluded.given_name,
			surname = excluded.surname,
			gender = excluded.gender,
			birth_date_raw = excluded.birth_date_raw,
			birth_date_sort = excluded.birth_date_sort,
			birth_place = excluded.birth_place,
			birth_place_lat = excluded.birth_place_lat,
			birth_place_long = excluded.birth_place_long,
			death_date_raw = excluded.death_date_raw,
			death_date_sort = excluded.death_date_sort,
			death_place = excluded.death_place,
			death_place_lat = excluded.death_place_lat,
			death_place_long = excluded.death_place_long,
			notes = excluded.notes,
			research_status = excluded.research_status,
			version = excluded.version,
			updated_at = excluded.updated_at
	`, person.ID.String(), person.GivenName, person.Surname, string(person.Gender),
		person.BirthDateRaw, birthDateSort, person.BirthPlace, birthPlaceLat, birthPlaceLong,
		person.DeathDateRaw, deathDateSort, person.DeathPlace, deathPlaceLat, deathPlaceLong,
		person.Notes, string(person.ResearchStatus), person.Version, formatTimestamp(person.UpdatedAt))

	return err
}

// DeletePerson removes a person.
func (s *ReadModelStore) DeletePerson(ctx context.Context, id uuid.UUID) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM persons WHERE id = ?", id.String())
	return err
}

// SavePersonName saves or updates a person name variant.
func (s *ReadModelStore) SavePersonName(ctx context.Context, name *repository.PersonNameReadModel) error {
	isPrimary := 0
	if name.IsPrimary {
		isPrimary = 1
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO person_names (id, person_id, given_name, surname, name_prefix, name_suffix,
								  surname_prefix, nickname, name_type, is_primary, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			person_id = excluded.person_id,
			given_name = excluded.given_name,
			surname = excluded.surname,
			name_prefix = excluded.name_prefix,
			name_suffix = excluded.name_suffix,
			surname_prefix = excluded.surname_prefix,
			nickname = excluded.nickname,
			name_type = excluded.name_type,
			is_primary = excluded.is_primary,
			updated_at = excluded.updated_at
	`, name.ID.String(), name.PersonID.String(), name.GivenName, name.Surname,
		nullableString(name.NamePrefix), nullableString(name.NameSuffix),
		nullableString(name.SurnamePrefix), nullableString(name.Nickname),
		string(name.NameType), isPrimary, formatTimestamp(name.UpdatedAt))

	return err
}

// GetPersonName retrieves a person name by ID.
func (s *ReadModelStore) GetPersonName(ctx context.Context, nameID uuid.UUID) (*repository.PersonNameReadModel, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, person_id, given_name, surname, full_name, name_prefix, name_suffix,
			   surname_prefix, nickname, name_type, is_primary, updated_at
		FROM person_names WHERE id = ?
	`, nameID.String())

	return scanPersonName(row)
}

// GetPersonNames retrieves all name variants for a person.
func (s *ReadModelStore) GetPersonNames(ctx context.Context, personID uuid.UUID) ([]repository.PersonNameReadModel, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, person_id, given_name, surname, full_name, name_prefix, name_suffix,
			   surname_prefix, nickname, name_type, is_primary, updated_at
		FROM person_names
		WHERE person_id = ?
		ORDER BY is_primary DESC, name_type
	`, personID.String())
	if err != nil {
		return nil, fmt.Errorf("query person names: %w", err)
	}
	defer rows.Close()

	var names []repository.PersonNameReadModel
	for rows.Next() {
		n, err := scanPersonNameRow(rows)
		if err != nil {
			return nil, err
		}
		names = append(names, *n)
	}

	return names, rows.Err()
}

// DeletePersonName removes a person name.
func (s *ReadModelStore) DeletePersonName(ctx context.Context, nameID uuid.UUID) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM person_names WHERE id = ?", nameID.String())
	return err
}

// scanPersonName scans a single person name row.
func scanPersonName(row rowScanner) (*repository.PersonNameReadModel, error) {
	var (
		idStr, personIDStr, givenName, surname, fullName string
		namePrefix, nameSuffix, surnamePrefix, nickname  sql.NullString
		nameType                                         sql.NullString
		isPrimary                                        int
		updatedAt                                        string
	)

	err := row.Scan(&idStr, &personIDStr, &givenName, &surname, &fullName,
		&namePrefix, &nameSuffix, &surnamePrefix, &nickname,
		&nameType, &isPrimary, &updatedAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan person name: %w", err)
	}

	id, _ := uuid.Parse(idStr)
	personID, _ := uuid.Parse(personIDStr)

	n := &repository.PersonNameReadModel{
		ID:            id,
		PersonID:      personID,
		GivenName:     givenName,
		Surname:       surname,
		FullName:      fullName,
		NamePrefix:    namePrefix.String,
		NameSuffix:    nameSuffix.String,
		SurnamePrefix: surnamePrefix.String,
		Nickname:      nickname.String,
		NameType:      domain.NameType(nameType.String),
		IsPrimary:     isPrimary == 1,
	}

	if t, err := parseTimestamp(updatedAt); err == nil {
		n.UpdatedAt = t
	}

	return n, nil
}

// scanPersonNameRow scans a person name from rows.
func scanPersonNameRow(rows *sql.Rows) (*repository.PersonNameReadModel, error) {
	return scanPersonName(rows)
}

// GetFamily retrieves a family by ID.
func (s *ReadModelStore) GetFamily(ctx context.Context, id uuid.UUID) (*repository.FamilyReadModel, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, partner1_id, partner1_name, partner2_id, partner2_name,
			   relationship_type, marriage_date_raw, marriage_date_sort, marriage_place,
			   marriage_place_lat, marriage_place_long,
			   child_count, version, updated_at
		FROM families WHERE id = ?
	`, id.String())

	return scanFamily(row)
}

// ListFamilies returns a paginated list of families.
func (s *ReadModelStore) ListFamilies(ctx context.Context, opts repository.ListOptions) ([]repository.FamilyReadModel, int, error) {
	var total int
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM families").Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("count families: %w", err)
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, partner1_id, partner1_name, partner2_id, partner2_name,
			   relationship_type, marriage_date_raw, marriage_date_sort, marriage_place,
			   marriage_place_lat, marriage_place_long,
			   child_count, version, updated_at
		FROM families
		ORDER BY updated_at DESC
		LIMIT ? OFFSET ?
	`, opts.Limit, opts.Offset)
	if err != nil {
		return nil, 0, fmt.Errorf("query families: %w", err)
	}
	defer rows.Close()

	var families []repository.FamilyReadModel
	for rows.Next() {
		f, err := scanFamilyRow(rows)
		if err != nil {
			return nil, 0, err
		}
		families = append(families, *f)
	}

	return families, total, rows.Err()
}

// GetFamiliesForPerson returns all families where the person is a partner.
func (s *ReadModelStore) GetFamiliesForPerson(ctx context.Context, personID uuid.UUID) ([]repository.FamilyReadModel, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, partner1_id, partner1_name, partner2_id, partner2_name,
			   relationship_type, marriage_date_raw, marriage_date_sort, marriage_place,
			   marriage_place_lat, marriage_place_long,
			   child_count, version, updated_at
		FROM families
		WHERE partner1_id = ? OR partner2_id = ?
	`, personID.String(), personID.String())
	if err != nil {
		return nil, fmt.Errorf("query families for person: %w", err)
	}
	defer rows.Close()

	var families []repository.FamilyReadModel
	for rows.Next() {
		f, err := scanFamilyRow(rows)
		if err != nil {
			return nil, err
		}
		families = append(families, *f)
	}

	return families, rows.Err()
}

// SaveFamily saves or updates a family.
func (s *ReadModelStore) SaveFamily(ctx context.Context, family *repository.FamilyReadModel) error {
	var partner1ID, partner2ID sql.NullString
	if family.Partner1ID != nil {
		partner1ID = sql.NullString{String: family.Partner1ID.String(), Valid: true}
	}
	if family.Partner2ID != nil {
		partner2ID = sql.NullString{String: family.Partner2ID.String(), Valid: true}
	}

	var marriageDateSort sql.NullString
	if family.MarriageDateSort != nil {
		marriageDateSort = sql.NullString{String: family.MarriageDateSort.Format("2006-01-02"), Valid: true}
	}

	// Convert coordinate pointers to nullable strings
	var marriagePlaceLat, marriagePlaceLong sql.NullString
	if family.MarriagePlaceLat != nil {
		marriagePlaceLat = sql.NullString{String: *family.MarriagePlaceLat, Valid: true}
	}
	if family.MarriagePlaceLong != nil {
		marriagePlaceLong = sql.NullString{String: *family.MarriagePlaceLong, Valid: true}
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO families (id, partner1_id, partner1_name, partner2_id, partner2_name,
							  relationship_type, marriage_date_raw, marriage_date_sort, marriage_place,
							  marriage_place_lat, marriage_place_long,
							  child_count, version, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			partner1_id = excluded.partner1_id,
			partner1_name = excluded.partner1_name,
			partner2_id = excluded.partner2_id,
			partner2_name = excluded.partner2_name,
			relationship_type = excluded.relationship_type,
			marriage_date_raw = excluded.marriage_date_raw,
			marriage_date_sort = excluded.marriage_date_sort,
			marriage_place = excluded.marriage_place,
			marriage_place_lat = excluded.marriage_place_lat,
			marriage_place_long = excluded.marriage_place_long,
			child_count = excluded.child_count,
			version = excluded.version,
			updated_at = excluded.updated_at
	`, family.ID.String(), partner1ID, family.Partner1Name, partner2ID, family.Partner2Name,
		string(family.RelationshipType), family.MarriageDateRaw, marriageDateSort, family.MarriagePlace,
		marriagePlaceLat, marriagePlaceLong,
		family.ChildCount, family.Version, formatTimestamp(family.UpdatedAt))

	return err
}

// DeleteFamily removes a family.
func (s *ReadModelStore) DeleteFamily(ctx context.Context, id uuid.UUID) error {
	// Children are deleted via ON DELETE CASCADE
	_, err := s.db.ExecContext(ctx, "DELETE FROM families WHERE id = ?", id.String())
	return err
}

// GetFamilyChildren returns all children for a family.
func (s *ReadModelStore) GetFamilyChildren(ctx context.Context, familyID uuid.UUID) ([]repository.FamilyChildReadModel, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT family_id, person_id, person_name, relationship_type, sequence
		FROM family_children
		WHERE family_id = ?
		ORDER BY sequence, person_name
	`, familyID.String())
	if err != nil {
		return nil, fmt.Errorf("query family children: %w", err)
	}
	defer rows.Close()

	var children []repository.FamilyChildReadModel
	for rows.Next() {
		var (
			familyIDStr, personIDStr, personName, relType string
			sequence                                      sql.NullInt64
		)
		err := rows.Scan(&familyIDStr, &personIDStr, &personName, &relType, &sequence)
		if err != nil {
			return nil, fmt.Errorf("scan family child: %w", err)
		}

		fID, _ := uuid.Parse(familyIDStr)
		pID, _ := uuid.Parse(personIDStr)

		child := repository.FamilyChildReadModel{
			FamilyID:         fID,
			PersonID:         pID,
			PersonName:       personName,
			RelationshipType: domain.ChildRelationType(relType),
		}
		if sequence.Valid {
			seq := int(sequence.Int64)
			child.Sequence = &seq
		}
		children = append(children, child)
	}

	return children, rows.Err()
}

// GetChildrenOfFamily returns person read models for all children in a family.
func (s *ReadModelStore) GetChildrenOfFamily(ctx context.Context, familyID uuid.UUID) ([]repository.PersonReadModel, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT p.id, p.given_name, p.surname, p.full_name, p.gender,
			   p.birth_date_raw, p.birth_date_sort, p.birth_place, p.birth_place_lat, p.birth_place_long,
			   p.death_date_raw, p.death_date_sort, p.death_place, p.death_place_lat, p.death_place_long,
			   p.notes, p.research_status, p.version, p.updated_at
		FROM persons p
		JOIN family_children fc ON p.id = fc.person_id
		WHERE fc.family_id = ?
		ORDER BY fc.sequence, p.given_name
	`, familyID.String())
	if err != nil {
		return nil, fmt.Errorf("query children of family: %w", err)
	}
	defer rows.Close()

	var persons []repository.PersonReadModel
	for rows.Next() {
		p, err := scanPersonRow(rows)
		if err != nil {
			return nil, err
		}
		persons = append(persons, *p)
	}

	return persons, rows.Err()
}

// GetChildFamily returns the family where the person is a child.
func (s *ReadModelStore) GetChildFamily(ctx context.Context, personID uuid.UUID) (*repository.FamilyReadModel, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT f.id, f.partner1_id, f.partner1_name, f.partner2_id, f.partner2_name,
			   f.relationship_type, f.marriage_date_raw, f.marriage_date_sort, f.marriage_place,
			   f.marriage_place_lat, f.marriage_place_long,
			   f.child_count, f.version, f.updated_at
		FROM families f
		JOIN family_children fc ON f.id = fc.family_id
		WHERE fc.person_id = ?
	`, personID.String())

	return scanFamily(row)
}

// SaveFamilyChild saves a family child relationship.
func (s *ReadModelStore) SaveFamilyChild(ctx context.Context, child *repository.FamilyChildReadModel) error {
	var sequence sql.NullInt64
	if child.Sequence != nil {
		sequence = sql.NullInt64{Int64: int64(*child.Sequence), Valid: true}
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO family_children (family_id, person_id, person_name, relationship_type, sequence)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(family_id, person_id) DO UPDATE SET
			person_name = excluded.person_name,
			relationship_type = excluded.relationship_type,
			sequence = excluded.sequence
	`, child.FamilyID.String(), child.PersonID.String(), child.PersonName, string(child.RelationshipType), sequence)

	return err
}

// DeleteFamilyChild removes a family child relationship.
func (s *ReadModelStore) DeleteFamilyChild(ctx context.Context, familyID, personID uuid.UUID) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM family_children WHERE family_id = ? AND person_id = ?",
		familyID.String(), personID.String())
	return err
}

// GetPedigreeEdge returns the pedigree edge for a person.
func (s *ReadModelStore) GetPedigreeEdge(ctx context.Context, personID uuid.UUID) (*repository.PedigreeEdge, error) {
	var (
		personIDStr, fatherIDStr, motherIDStr, fatherName, motherName sql.NullString
	)

	err := s.db.QueryRowContext(ctx, `
		SELECT person_id, father_id, mother_id, father_name, mother_name
		FROM pedigree_edges
		WHERE person_id = ?
	`, personID.String()).Scan(&personIDStr, &fatherIDStr, &motherIDStr, &fatherName, &motherName)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("query pedigree edge: %w", err)
	}

	pID, _ := uuid.Parse(personIDStr.String)
	edge := &repository.PedigreeEdge{
		PersonID:   pID,
		FatherName: fatherName.String,
		MotherName: motherName.String,
	}

	if fatherIDStr.Valid {
		fID, _ := uuid.Parse(fatherIDStr.String)
		edge.FatherID = &fID
	}
	if motherIDStr.Valid {
		mID, _ := uuid.Parse(motherIDStr.String)
		edge.MotherID = &mID
	}

	return edge, nil
}

// SavePedigreeEdge saves a pedigree edge.
func (s *ReadModelStore) SavePedigreeEdge(ctx context.Context, edge *repository.PedigreeEdge) error {
	var fatherID, motherID sql.NullString
	if edge.FatherID != nil {
		fatherID = sql.NullString{String: edge.FatherID.String(), Valid: true}
	}
	if edge.MotherID != nil {
		motherID = sql.NullString{String: edge.MotherID.String(), Valid: true}
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO pedigree_edges (person_id, father_id, mother_id, father_name, mother_name)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(person_id) DO UPDATE SET
			father_id = excluded.father_id,
			mother_id = excluded.mother_id,
			father_name = excluded.father_name,
			mother_name = excluded.mother_name
	`, edge.PersonID.String(), fatherID, motherID, edge.FatherName, edge.MotherName)

	return err
}

// DeletePedigreeEdge removes a pedigree edge.
func (s *ReadModelStore) DeletePedigreeEdge(ctx context.Context, personID uuid.UUID) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM pedigree_edges WHERE person_id = ?", personID.String())
	return err
}

// Helper functions for scanning rows

type rowScanner interface {
	Scan(dest ...any) error
}

func scanPerson(row rowScanner) (*repository.PersonReadModel, error) {
	var (
		idStr, givenName, surname, fullName             string
		gender, birthDateRaw, birthDateSort, birthPlace sql.NullString
		birthPlaceLat, birthPlaceLong                   sql.NullString
		deathDateRaw, deathDateSort, deathPlace, notes  sql.NullString
		deathPlaceLat, deathPlaceLong                   sql.NullString
		researchStatus                                  sql.NullString
		version                                         int64
		updatedAt                                       string
	)

	err := row.Scan(&idStr, &givenName, &surname, &fullName, &gender,
		&birthDateRaw, &birthDateSort, &birthPlace, &birthPlaceLat, &birthPlaceLong,
		&deathDateRaw, &deathDateSort, &deathPlace, &deathPlaceLat, &deathPlaceLong,
		&notes, &researchStatus, &version, &updatedAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan person: %w", err)
	}

	id, _ := uuid.Parse(idStr)
	p := &repository.PersonReadModel{
		ID:             id,
		GivenName:      givenName,
		Surname:        surname,
		FullName:       fullName,
		Gender:         domain.Gender(gender.String),
		BirthDateRaw:   birthDateRaw.String,
		BirthPlace:     birthPlace.String,
		DeathDateRaw:   deathDateRaw.String,
		DeathPlace:     deathPlace.String,
		Notes:          notes.String,
		ResearchStatus: domain.ResearchStatus(researchStatus.String),
		Version:        version,
	}

	// Set coordinate pointers if values are present
	if birthPlaceLat.Valid && birthPlaceLat.String != "" {
		p.BirthPlaceLat = &birthPlaceLat.String
	}
	if birthPlaceLong.Valid && birthPlaceLong.String != "" {
		p.BirthPlaceLong = &birthPlaceLong.String
	}
	if deathPlaceLat.Valid && deathPlaceLat.String != "" {
		p.DeathPlaceLat = &deathPlaceLat.String
	}
	if deathPlaceLong.Valid && deathPlaceLong.String != "" {
		p.DeathPlaceLong = &deathPlaceLong.String
	}

	if birthDateSort.Valid {
		if t, err := time.Parse("2006-01-02", birthDateSort.String); err == nil {
			p.BirthDateSort = &t
		}
	}
	if deathDateSort.Valid {
		if t, err := time.Parse("2006-01-02", deathDateSort.String); err == nil {
			p.DeathDateSort = &t
		}
	}
	if t, err := parseTimestamp(updatedAt); err == nil {
		p.UpdatedAt = t
	}

	return p, nil
}

func scanPersonRow(rows *sql.Rows) (*repository.PersonReadModel, error) {
	return scanPerson(rows)
}

func scanFamily(row rowScanner) (*repository.FamilyReadModel, error) {
	var (
		idStr                                                     string
		partner1ID, partner1Name, partner2ID, partner2Name        sql.NullString
		relType, marriageDateRaw, marriageDateSort, marriagePlace sql.NullString
		marriagePlaceLat, marriagePlaceLong                       sql.NullString
		childCount                                                int
		version                                                   int64
		updatedAt                                                 string
	)

	err := row.Scan(&idStr, &partner1ID, &partner1Name, &partner2ID, &partner2Name,
		&relType, &marriageDateRaw, &marriageDateSort, &marriagePlace,
		&marriagePlaceLat, &marriagePlaceLong,
		&childCount, &version, &updatedAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan family: %w", err)
	}

	id, _ := uuid.Parse(idStr)
	f := &repository.FamilyReadModel{
		ID:               id,
		Partner1Name:     partner1Name.String,
		Partner2Name:     partner2Name.String,
		RelationshipType: domain.RelationType(relType.String),
		MarriageDateRaw:  marriageDateRaw.String,
		MarriagePlace:    marriagePlace.String,
		ChildCount:       childCount,
		Version:          version,
	}

	if partner1ID.Valid {
		p1ID, _ := uuid.Parse(partner1ID.String)
		f.Partner1ID = &p1ID
	}
	if partner2ID.Valid {
		p2ID, _ := uuid.Parse(partner2ID.String)
		f.Partner2ID = &p2ID
	}
	if marriageDateSort.Valid {
		if t, err := time.Parse("2006-01-02", marriageDateSort.String); err == nil {
			f.MarriageDateSort = &t
		}
	}
	// Set coordinate pointers if values are present
	if marriagePlaceLat.Valid && marriagePlaceLat.String != "" {
		f.MarriagePlaceLat = &marriagePlaceLat.String
	}
	if marriagePlaceLong.Valid && marriagePlaceLong.String != "" {
		f.MarriagePlaceLong = &marriagePlaceLong.String
	}
	if t, err := parseTimestamp(updatedAt); err == nil {
		f.UpdatedAt = t
	}

	return f, nil
}

func scanFamilyRow(rows *sql.Rows) (*repository.FamilyReadModel, error) {
	return scanFamily(rows)
}

// escapeFTS5Query escapes special characters for FTS5 queries.
func escapeFTS5Query(query string) string {
	// FTS5 special characters that need escaping
	specialChars := []string{"*", "+", "-", "\"", "(", ")", ":", "^"}
	result := query
	for _, char := range specialChars {
		result = strings.ReplaceAll(result, char, "\""+char+"\"")
	}
	return result
}

// soundex computes the American Soundex code for a string.
// Returns a 4-character code (letter + 3 digits), or "" for empty input.
// Soundex and SoundexMatch are in the shared repository package.

// GetSource retrieves a source by ID.
func (s *ReadModelStore) GetSource(ctx context.Context, id uuid.UUID) (*repository.SourceReadModel, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, source_type, title, author, publisher, publish_date_raw, publish_date_sort,
			   url, repository_name, collection_name, call_number, notes, gedcom_xref,
			   citation_count, version, updated_at
		FROM sources WHERE id = ?
	`, id.String())

	return scanSource(row)
}

// ListSources returns a paginated list of sources.
func (s *ReadModelStore) ListSources(ctx context.Context, opts repository.ListOptions) ([]repository.SourceReadModel, int, error) {
	var total int
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM sources").Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("count sources: %w", err)
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, source_type, title, author, publisher, publish_date_raw, publish_date_sort,
			   url, repository_name, collection_name, call_number, notes, gedcom_xref,
			   citation_count, version, updated_at
		FROM sources
		ORDER BY title ASC
		LIMIT ? OFFSET ?
	`, opts.Limit, opts.Offset)
	if err != nil {
		return nil, 0, fmt.Errorf("query sources: %w", err)
	}
	defer rows.Close()

	var sources []repository.SourceReadModel
	for rows.Next() {
		src, err := scanSourceRow(rows)
		if err != nil {
			return nil, 0, err
		}
		sources = append(sources, *src)
	}

	return sources, total, rows.Err()
}

// SearchSources searches for sources by title or author.
func (s *ReadModelStore) SearchSources(ctx context.Context, query string, limit int) ([]repository.SourceReadModel, error) {
	likeQuery := "%" + strings.ToLower(query) + "%"

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, source_type, title, author, publisher, publish_date_raw, publish_date_sort,
			   url, repository_name, collection_name, call_number, notes, gedcom_xref,
			   citation_count, version, updated_at
		FROM sources
		WHERE LOWER(title) LIKE ? OR LOWER(author) LIKE ?
		LIMIT ?
	`, likeQuery, likeQuery, limit)
	if err != nil {
		return nil, fmt.Errorf("search sources: %w", err)
	}
	defer rows.Close()

	var sources []repository.SourceReadModel
	for rows.Next() {
		src, err := scanSourceRow(rows)
		if err != nil {
			return nil, err
		}
		sources = append(sources, *src)
	}

	return sources, rows.Err()
}

// SaveSource saves or updates a source.
func (s *ReadModelStore) SaveSource(ctx context.Context, source *repository.SourceReadModel) error {
	var publishDateSort sql.NullString
	if source.PublishDateSort != nil {
		publishDateSort = sql.NullString{String: source.PublishDateSort.Format("2006-01-02"), Valid: true}
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO sources (id, source_type, title, author, publisher, publish_date_raw, publish_date_sort,
							 url, repository_name, collection_name, call_number, notes, gedcom_xref,
							 citation_count, version, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			source_type = excluded.source_type,
			title = excluded.title,
			author = excluded.author,
			publisher = excluded.publisher,
			publish_date_raw = excluded.publish_date_raw,
			publish_date_sort = excluded.publish_date_sort,
			url = excluded.url,
			repository_name = excluded.repository_name,
			collection_name = excluded.collection_name,
			call_number = excluded.call_number,
			notes = excluded.notes,
			gedcom_xref = excluded.gedcom_xref,
			citation_count = excluded.citation_count,
			version = excluded.version,
			updated_at = excluded.updated_at
	`, source.ID.String(), string(source.SourceType), source.Title, source.Author, source.Publisher,
		source.PublishDateRaw, publishDateSort, source.URL, source.RepositoryName, source.CollectionName,
		source.CallNumber, source.Notes, source.GedcomXref, source.CitationCount, source.Version,
		formatTimestamp(source.UpdatedAt))

	return err
}

// DeleteSource removes a source.
func (s *ReadModelStore) DeleteSource(ctx context.Context, id uuid.UUID) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM sources WHERE id = ?", id.String())
	return err
}

// GetCitation retrieves a citation by ID.
func (s *ReadModelStore) GetCitation(ctx context.Context, id uuid.UUID) (*repository.CitationReadModel, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, source_id, source_title, fact_type, fact_owner_id, page, volume,
			   source_quality, informant_type, evidence_type, quoted_text, analysis,
			   template_id, gedcom_xref, version, created_at
		FROM citations WHERE id = ?
	`, id.String())

	return scanCitation(row)
}

// GetCitationsForSource returns all citations for a source.
func (s *ReadModelStore) GetCitationsForSource(ctx context.Context, sourceID uuid.UUID) ([]repository.CitationReadModel, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, source_id, source_title, fact_type, fact_owner_id, page, volume,
			   source_quality, informant_type, evidence_type, quoted_text, analysis,
			   template_id, gedcom_xref, version, created_at
		FROM citations
		WHERE source_id = ?
	`, sourceID.String())
	if err != nil {
		return nil, fmt.Errorf("query citations for source: %w", err)
	}
	defer rows.Close()

	var citations []repository.CitationReadModel
	for rows.Next() {
		cit, err := scanCitationRow(rows)
		if err != nil {
			return nil, err
		}
		citations = append(citations, *cit)
	}

	return citations, rows.Err()
}

// GetCitationsForPerson returns all citations for a person.
func (s *ReadModelStore) GetCitationsForPerson(ctx context.Context, personID uuid.UUID) ([]repository.CitationReadModel, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, source_id, source_title, fact_type, fact_owner_id, page, volume,
			   source_quality, informant_type, evidence_type, quoted_text, analysis,
			   template_id, gedcom_xref, version, created_at
		FROM citations
		WHERE fact_owner_id = ? AND fact_type LIKE 'person_%'
	`, personID.String())
	if err != nil {
		return nil, fmt.Errorf("query citations for person: %w", err)
	}
	defer rows.Close()

	var citations []repository.CitationReadModel
	for rows.Next() {
		cit, err := scanCitationRow(rows)
		if err != nil {
			return nil, err
		}
		citations = append(citations, *cit)
	}

	return citations, rows.Err()
}

// GetCitationsForFact returns all citations for a specific fact.
func (s *ReadModelStore) GetCitationsForFact(ctx context.Context, factType domain.FactType, factOwnerID uuid.UUID) ([]repository.CitationReadModel, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, source_id, source_title, fact_type, fact_owner_id, page, volume,
			   source_quality, informant_type, evidence_type, quoted_text, analysis,
			   template_id, gedcom_xref, version, created_at
		FROM citations
		WHERE fact_type = ? AND fact_owner_id = ?
	`, string(factType), factOwnerID.String())
	if err != nil {
		return nil, fmt.Errorf("query citations for fact: %w", err)
	}
	defer rows.Close()

	var citations []repository.CitationReadModel
	for rows.Next() {
		cit, err := scanCitationRow(rows)
		if err != nil {
			return nil, err
		}
		citations = append(citations, *cit)
	}

	return citations, rows.Err()
}

// ListCitations returns a paginated list of citations.
func (s *ReadModelStore) ListCitations(ctx context.Context, opts repository.ListOptions) ([]repository.CitationReadModel, int, error) {
	// Count total
	var total int
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM citations").Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("count citations: %w", err)
	}

	// Sort by source_title ASC, fact_type ASC, id ASC for deterministic ordering
	query := `
		SELECT id, source_id, source_title, fact_type, fact_owner_id, page, volume,
			   source_quality, informant_type, evidence_type, quoted_text, analysis,
			   template_id, gedcom_xref, version, created_at
		FROM citations
		ORDER BY source_title ASC, fact_type ASC, id ASC
		LIMIT ? OFFSET ?
	`

	rows, err := s.db.QueryContext(ctx, query, opts.Limit, opts.Offset)
	if err != nil {
		return nil, 0, fmt.Errorf("query citations: %w", err)
	}
	defer rows.Close()

	var citations []repository.CitationReadModel
	for rows.Next() {
		cit, err := scanCitationRow(rows)
		if err != nil {
			return nil, 0, err
		}
		citations = append(citations, *cit)
	}

	return citations, total, rows.Err()
}

// SaveCitation saves or updates a citation.
func (s *ReadModelStore) SaveCitation(ctx context.Context, citation *repository.CitationReadModel) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO citations (id, source_id, source_title, fact_type, fact_owner_id, page, volume,
							   source_quality, informant_type, evidence_type, quoted_text, analysis,
							   template_id, gedcom_xref, version, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			source_id = excluded.source_id,
			source_title = excluded.source_title,
			fact_type = excluded.fact_type,
			fact_owner_id = excluded.fact_owner_id,
			page = excluded.page,
			volume = excluded.volume,
			source_quality = excluded.source_quality,
			informant_type = excluded.informant_type,
			evidence_type = excluded.evidence_type,
			quoted_text = excluded.quoted_text,
			analysis = excluded.analysis,
			template_id = excluded.template_id,
			gedcom_xref = excluded.gedcom_xref,
			version = excluded.version
	`, citation.ID.String(), citation.SourceID.String(), citation.SourceTitle,
		string(citation.FactType), citation.FactOwnerID.String(), citation.Page, citation.Volume,
		string(citation.SourceQuality), string(citation.InformantType), string(citation.EvidenceType),
		citation.QuotedText, citation.Analysis, citation.TemplateID, citation.GedcomXref,
		citation.Version, formatTimestamp(citation.CreatedAt))

	return err
}

// DeleteCitation removes a citation.
func (s *ReadModelStore) DeleteCitation(ctx context.Context, id uuid.UUID) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM citations WHERE id = ?", id.String())
	return err
}

// GetEvent retrieves an event by ID.
func (s *ReadModelStore) GetEvent(ctx context.Context, id uuid.UUID) (*repository.EventReadModel, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, owner_type, owner_id, fact_type, date_raw, date_sort,
		       place, place_lat, place_long, address, description, cause,
		       age, research_status, version, created_at
		FROM events WHERE id = ?
	`, id.String())

	return scanEvent(row)
}

// ListEvents returns a paginated list of events.
func (s *ReadModelStore) ListEvents(ctx context.Context, opts repository.ListOptions) ([]repository.EventReadModel, int, error) {
	// Count total
	var total int
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM events").Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("count events: %w", err)
	}

	// Sort by fact_type ASC, date_sort ASC NULLS LAST, id ASC for deterministic ordering
	query := `
		SELECT id, owner_type, owner_id, fact_type, date_raw, date_sort,
		       place, place_lat, place_long, address, description, cause,
		       age, research_status, version, created_at
		FROM events
		ORDER BY fact_type ASC, CASE WHEN date_sort IS NULL THEN 1 ELSE 0 END, date_sort ASC, id ASC
		LIMIT ? OFFSET ?
	`

	rows, err := s.db.QueryContext(ctx, query, opts.Limit, opts.Offset)
	if err != nil {
		return nil, 0, fmt.Errorf("query events: %w", err)
	}
	defer rows.Close()

	var events []repository.EventReadModel
	for rows.Next() {
		event, err := scanEventRow(rows)
		if err != nil {
			return nil, 0, err
		}
		events = append(events, *event)
	}

	return events, total, rows.Err()
}

// ListEventsForPerson returns all events for a given person.
func (s *ReadModelStore) ListEventsForPerson(ctx context.Context, personID uuid.UUID) ([]repository.EventReadModel, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, owner_type, owner_id, fact_type, date_raw, date_sort,
		       place, place_lat, place_long, address, description, cause,
		       age, research_status, version, created_at
		FROM events
		WHERE owner_type = 'person' AND owner_id = ?
		ORDER BY fact_type ASC, CASE WHEN date_sort IS NULL THEN 1 ELSE 0 END, date_sort ASC, id ASC
	`, personID.String())
	if err != nil {
		return nil, fmt.Errorf("query events for person: %w", err)
	}
	defer rows.Close()

	var events []repository.EventReadModel
	for rows.Next() {
		event, err := scanEventRow(rows)
		if err != nil {
			return nil, err
		}
		events = append(events, *event)
	}

	return events, rows.Err()
}

// ListEventsForFamily returns all events for a given family.
func (s *ReadModelStore) ListEventsForFamily(ctx context.Context, familyID uuid.UUID) ([]repository.EventReadModel, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, owner_type, owner_id, fact_type, date_raw, date_sort,
		       place, place_lat, place_long, address, description, cause,
		       age, research_status, version, created_at
		FROM events
		WHERE owner_type = 'family' AND owner_id = ?
		ORDER BY fact_type ASC, CASE WHEN date_sort IS NULL THEN 1 ELSE 0 END, date_sort ASC, id ASC
	`, familyID.String())
	if err != nil {
		return nil, fmt.Errorf("query events for family: %w", err)
	}
	defer rows.Close()

	var events []repository.EventReadModel
	for rows.Next() {
		event, err := scanEventRow(rows)
		if err != nil {
			return nil, err
		}
		events = append(events, *event)
	}

	return events, rows.Err()
}

// SaveEvent saves or updates an event.
func (s *ReadModelStore) SaveEvent(ctx context.Context, event *repository.EventReadModel) error {
	var dateSort, placeLat, placeLong, addressJSON interface{}
	var description, cause, age, researchStatus interface{}

	if event.DateSort != nil {
		dateSort = event.DateSort.Format(time.RFC3339)
	}
	if event.PlaceLat != nil {
		placeLat = *event.PlaceLat
	}
	if event.PlaceLong != nil {
		placeLong = *event.PlaceLong
	}
	if event.Address != nil {
		if data, err := json.Marshal(event.Address); err == nil {
			addressJSON = string(data)
		}
	}
	if event.Description != "" {
		description = event.Description
	}
	if event.Cause != "" {
		cause = event.Cause
	}
	if event.Age != "" {
		age = event.Age
	}
	if event.ResearchStatus != "" {
		researchStatus = string(event.ResearchStatus)
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO events (id, owner_type, owner_id, fact_type, date_raw, date_sort,
		                    place, place_lat, place_long, address, description, cause,
		                    age, research_status, version, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET
			owner_type = excluded.owner_type,
			owner_id = excluded.owner_id,
			fact_type = excluded.fact_type,
			date_raw = excluded.date_raw,
			date_sort = excluded.date_sort,
			place = excluded.place,
			place_lat = excluded.place_lat,
			place_long = excluded.place_long,
			address = excluded.address,
			description = excluded.description,
			cause = excluded.cause,
			age = excluded.age,
			research_status = excluded.research_status,
			version = excluded.version
	`, event.ID.String(), event.OwnerType, event.OwnerID.String(), string(event.FactType),
		event.DateRaw, dateSort, event.Place, placeLat, placeLong, addressJSON,
		description, cause, age, researchStatus, event.Version,
		formatTimestamp(event.CreatedAt))
	if err != nil {
		return fmt.Errorf("save event: %w", err)
	}
	return nil
}

// DeleteEvent deletes an event by ID.
func (s *ReadModelStore) DeleteEvent(ctx context.Context, id uuid.UUID) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM events WHERE id = ?", id.String())
	if err != nil {
		return fmt.Errorf("delete event: %w", err)
	}
	return nil
}

// GetAttribute retrieves an attribute by ID.
func (s *ReadModelStore) GetAttribute(ctx context.Context, id uuid.UUID) (*repository.AttributeReadModel, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, person_id, fact_type, value, date_raw, date_sort, place, version, created_at
		FROM attributes WHERE id = ?
	`, id.String())

	return scanAttribute(row)
}

// ListAttributes returns a paginated list of attributes.
func (s *ReadModelStore) ListAttributes(ctx context.Context, opts repository.ListOptions) ([]repository.AttributeReadModel, int, error) {
	// Count total
	var total int
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM attributes").Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("count attributes: %w", err)
	}

	// Sort by fact_type ASC, value ASC, id ASC for deterministic ordering
	query := `
		SELECT id, person_id, fact_type, value, date_raw, date_sort, place, version, created_at
		FROM attributes
		ORDER BY fact_type ASC, value ASC, id ASC
		LIMIT ? OFFSET ?
	`

	rows, err := s.db.QueryContext(ctx, query, opts.Limit, opts.Offset)
	if err != nil {
		return nil, 0, fmt.Errorf("query attributes: %w", err)
	}
	defer rows.Close()

	var attributes []repository.AttributeReadModel
	for rows.Next() {
		attr, err := scanAttributeRow(rows)
		if err != nil {
			return nil, 0, err
		}
		attributes = append(attributes, *attr)
	}

	return attributes, total, rows.Err()
}

// ListAttributesForPerson returns all attributes for a given person.
func (s *ReadModelStore) ListAttributesForPerson(ctx context.Context, personID uuid.UUID) ([]repository.AttributeReadModel, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, person_id, fact_type, value, date_raw, date_sort, place, version, created_at
		FROM attributes
		WHERE person_id = ?
		ORDER BY fact_type ASC, value ASC, id ASC
	`, personID.String())
	if err != nil {
		return nil, fmt.Errorf("query attributes for person: %w", err)
	}
	defer rows.Close()

	var attributes []repository.AttributeReadModel
	for rows.Next() {
		attr, err := scanAttributeRow(rows)
		if err != nil {
			return nil, err
		}
		attributes = append(attributes, *attr)
	}

	return attributes, rows.Err()
}

// SaveAttribute saves or updates an attribute.
func (s *ReadModelStore) SaveAttribute(ctx context.Context, attribute *repository.AttributeReadModel) error {
	var dateSort interface{}
	if attribute.DateSort != nil {
		dateSort = attribute.DateSort.Format(time.RFC3339)
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO attributes (id, person_id, fact_type, value, date_raw, date_sort, place, version, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET
			person_id = excluded.person_id,
			fact_type = excluded.fact_type,
			value = excluded.value,
			date_raw = excluded.date_raw,
			date_sort = excluded.date_sort,
			place = excluded.place,
			version = excluded.version
	`, attribute.ID.String(), attribute.PersonID.String(), string(attribute.FactType),
		attribute.Value, attribute.DateRaw, dateSort, attribute.Place,
		attribute.Version, formatTimestamp(attribute.CreatedAt))
	if err != nil {
		return fmt.Errorf("save attribute: %w", err)
	}
	return nil
}

// DeleteAttribute deletes an attribute by ID.
func (s *ReadModelStore) DeleteAttribute(ctx context.Context, id uuid.UUID) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM attributes WHERE id = ?", id.String())
	if err != nil {
		return fmt.Errorf("delete attribute: %w", err)
	}
	return nil
}

// Scanning functions for sources and citations

func scanSource(row rowScanner) (*repository.SourceReadModel, error) {
	var (
		idStr, sourceType, title                            string
		author, publisher, publishDateRaw, publishDateSort  sql.NullString
		url, repoName, collName, callNum, notes, gedcomXref sql.NullString
		citationCount                                       int
		version                                             int64
		updatedAt                                           string
	)

	err := row.Scan(&idStr, &sourceType, &title, &author, &publisher, &publishDateRaw, &publishDateSort,
		&url, &repoName, &collName, &callNum, &notes, &gedcomXref,
		&citationCount, &version, &updatedAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan source: %w", err)
	}

	id, _ := uuid.Parse(idStr)
	src := &repository.SourceReadModel{
		ID:             id,
		SourceType:     domain.SourceType(sourceType),
		Title:          title,
		Author:         author.String,
		Publisher:      publisher.String,
		PublishDateRaw: publishDateRaw.String,
		URL:            url.String,
		RepositoryName: repoName.String,
		CollectionName: collName.String,
		CallNumber:     callNum.String,
		Notes:          notes.String,
		GedcomXref:     gedcomXref.String,
		CitationCount:  citationCount,
		Version:        version,
	}

	if publishDateSort.Valid {
		if t, err := time.Parse("2006-01-02", publishDateSort.String); err == nil {
			src.PublishDateSort = &t
		}
	}
	if t, err := parseTimestamp(updatedAt); err == nil {
		src.UpdatedAt = t
	}

	return src, nil
}

func scanSourceRow(rows *sql.Rows) (*repository.SourceReadModel, error) {
	return scanSource(rows)
}

func scanCitation(row rowScanner) (*repository.CitationReadModel, error) {
	var (
		idStr, sourceIDStr, sourceTitle, factType, factOwnerIDStr string
		page, volume, sourceQuality, informantType, evidenceType  sql.NullString
		quotedText, analysis, templateID, gedcomXref              sql.NullString
		version                                                   int64
		createdAt                                                 string
	)

	err := row.Scan(&idStr, &sourceIDStr, &sourceTitle, &factType, &factOwnerIDStr,
		&page, &volume, &sourceQuality, &informantType, &evidenceType,
		&quotedText, &analysis, &templateID, &gedcomXref, &version, &createdAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan citation: %w", err)
	}

	id, _ := uuid.Parse(idStr)
	sourceID, _ := uuid.Parse(sourceIDStr)
	factOwnerID, _ := uuid.Parse(factOwnerIDStr)

	cit := &repository.CitationReadModel{
		ID:            id,
		SourceID:      sourceID,
		SourceTitle:   sourceTitle,
		FactType:      domain.FactType(factType),
		FactOwnerID:   factOwnerID,
		Page:          page.String,
		Volume:        volume.String,
		SourceQuality: domain.SourceQuality(sourceQuality.String),
		InformantType: domain.InformantType(informantType.String),
		EvidenceType:  domain.EvidenceType(evidenceType.String),
		QuotedText:    quotedText.String,
		Analysis:      analysis.String,
		TemplateID:    templateID.String,
		GedcomXref:    gedcomXref.String,
		Version:       version,
	}

	if t, err := parseTimestamp(createdAt); err == nil {
		cit.CreatedAt = t
	}

	return cit, nil
}

func scanCitationRow(rows *sql.Rows) (*repository.CitationReadModel, error) {
	return scanCitation(rows)
}

func scanEvent(row rowScanner) (*repository.EventReadModel, error) {
	var (
		idStr, ownerType, ownerIDStr, factType string
		dateRaw, place                         sql.NullString
		dateSort                               sql.NullString
		placeLat, placeLong                    sql.NullString
		addressJSON                            []byte
		description, cause, age                sql.NullString
		researchStatus                         sql.NullString
		version                                int64
		createdAt                              string
	)

	err := row.Scan(&idStr, &ownerType, &ownerIDStr, &factType, &dateRaw, &dateSort,
		&place, &placeLat, &placeLong, &addressJSON, &description, &cause,
		&age, &researchStatus, &version, &createdAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan event: %w", err)
	}

	id, _ := uuid.Parse(idStr)
	ownerID, _ := uuid.Parse(ownerIDStr)

	event := &repository.EventReadModel{
		ID:          id,
		OwnerType:   ownerType,
		OwnerID:     ownerID,
		FactType:    domain.FactType(factType),
		DateRaw:     dateRaw.String,
		Place:       place.String,
		Description: description.String,
		Cause:       cause.String,
		Age:         age.String,
		Version:     version,
	}

	if dateSort.Valid {
		if t, err := parseTimestamp(dateSort.String); err == nil {
			event.DateSort = &t
		}
	}
	if placeLat.Valid {
		s := placeLat.String
		event.PlaceLat = &s
	}
	if placeLong.Valid {
		s := placeLong.String
		event.PlaceLong = &s
	}
	if len(addressJSON) > 0 {
		var addr domain.Address
		if err := json.Unmarshal(addressJSON, &addr); err == nil {
			event.Address = &addr
		}
	}
	if researchStatus.Valid {
		event.ResearchStatus = domain.ResearchStatus(researchStatus.String)
	}
	if t, err := parseTimestamp(createdAt); err == nil {
		event.CreatedAt = t
	}

	return event, nil
}

func scanEventRow(rows *sql.Rows) (*repository.EventReadModel, error) {
	return scanEvent(rows)
}

func scanAttribute(row rowScanner) (*repository.AttributeReadModel, error) {
	var (
		idStr, personIDStr, factType string
		value                        string
		dateRaw, place               sql.NullString
		dateSort                     sql.NullString
		version                      int64
		createdAt                    string
	)

	err := row.Scan(&idStr, &personIDStr, &factType, &value, &dateRaw, &dateSort,
		&place, &version, &createdAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan attribute: %w", err)
	}

	id, _ := uuid.Parse(idStr)
	personID, _ := uuid.Parse(personIDStr)

	attr := &repository.AttributeReadModel{
		ID:       id,
		PersonID: personID,
		FactType: domain.FactType(factType),
		Value:    value,
		DateRaw:  dateRaw.String,
		Place:    place.String,
		Version:  version,
	}

	if dateSort.Valid {
		if t, err := parseTimestamp(dateSort.String); err == nil {
			attr.DateSort = &t
		}
	}
	if t, err := parseTimestamp(createdAt); err == nil {
		attr.CreatedAt = t
	}

	return attr, nil
}

func scanAttributeRow(rows *sql.Rows) (*repository.AttributeReadModel, error) {
	return scanAttribute(rows)
}

// GetMedia retrieves media metadata by ID (excludes FileData and ThumbnailData).
func (s *ReadModelStore) GetMedia(ctx context.Context, id uuid.UUID) (*repository.MediaReadModel, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, entity_type, entity_id, title, description, mime_type, media_type,
			   filename, file_size, crop_left, crop_top, crop_width, crop_height,
			   gedcom_xref, version, created_at, updated_at,
			   files, format, translations
		FROM media WHERE id = ?
	`, id.String())

	return scanMediaMetadata(row)
}

// GetMediaWithData retrieves full media record including FileData and ThumbnailData.
func (s *ReadModelStore) GetMediaWithData(ctx context.Context, id uuid.UUID) (*repository.MediaReadModel, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, entity_type, entity_id, title, description, mime_type, media_type,
			   filename, file_size, file_data, thumbnail_data,
			   crop_left, crop_top, crop_width, crop_height,
			   gedcom_xref, version, created_at, updated_at,
			   files, format, translations
		FROM media WHERE id = ?
	`, id.String())

	return scanMediaFull(row)
}

// GetMediaThumbnail retrieves just the thumbnail bytes for efficient serving.
func (s *ReadModelStore) GetMediaThumbnail(ctx context.Context, id uuid.UUID) ([]byte, error) {
	var thumbnail []byte
	err := s.db.QueryRowContext(ctx, `
		SELECT thumbnail_data FROM media WHERE id = ?
	`, id.String()).Scan(&thumbnail)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	return thumbnail, err
}

// ListMediaForEntity returns a paginated list of media for an entity.
func (s *ReadModelStore) ListMediaForEntity(ctx context.Context, entityType string, entityID uuid.UUID, opts repository.ListOptions) ([]repository.MediaReadModel, int, error) {
	// Count total
	var total int
	err := s.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM media WHERE entity_type = ? AND entity_id = ?",
		entityType, entityID.String()).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("count media: %w", err)
	}

	// Query with pagination (metadata only, ordered by created_at DESC)
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, entity_type, entity_id, title, description, mime_type, media_type,
			   filename, file_size, crop_left, crop_top, crop_width, crop_height,
			   gedcom_xref, version, created_at, updated_at,
			   files, format, translations
		FROM media
		WHERE entity_type = ? AND entity_id = ?
		ORDER BY created_at DESC
		LIMIT ? OFFSET ?
	`, entityType, entityID.String(), opts.Limit, opts.Offset)
	if err != nil {
		return nil, 0, fmt.Errorf("query media: %w", err)
	}
	defer rows.Close()

	var items []repository.MediaReadModel
	for rows.Next() {
		m, err := scanMediaMetadataRow(rows)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, *m)
	}

	return items, total, rows.Err()
}

// SaveMedia saves or updates a media record.
func (s *ReadModelStore) SaveMedia(ctx context.Context, media *repository.MediaReadModel) error {
	// Serialize JSON fields
	filesJSON, err := domain.MarshalFilesToJSON(media.Files)
	if err != nil {
		return fmt.Errorf("marshal files: %w", err)
	}
	translationsJSON, err := domain.MarshalTranslationsToJSON(media.Translations)
	if err != nil {
		return fmt.Errorf("marshal translations: %w", err)
	}

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO media (id, entity_type, entity_id, title, description, mime_type, media_type,
						  filename, file_size, file_data, thumbnail_data,
						  crop_left, crop_top, crop_width, crop_height,
						  gedcom_xref, version, created_at, updated_at,
						  files, format, translations)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			entity_type = excluded.entity_type,
			entity_id = excluded.entity_id,
			title = excluded.title,
			description = excluded.description,
			mime_type = excluded.mime_type,
			media_type = excluded.media_type,
			filename = excluded.filename,
			file_size = excluded.file_size,
			file_data = excluded.file_data,
			thumbnail_data = excluded.thumbnail_data,
			crop_left = excluded.crop_left,
			crop_top = excluded.crop_top,
			crop_width = excluded.crop_width,
			crop_height = excluded.crop_height,
			gedcom_xref = excluded.gedcom_xref,
			version = excluded.version,
			updated_at = excluded.updated_at,
			files = excluded.files,
			format = excluded.format,
			translations = excluded.translations
	`, media.ID.String(), media.EntityType, media.EntityID.String(), media.Title,
		nullableString(media.Description), media.MimeType, string(media.MediaType),
		media.Filename, media.FileSize, media.FileData, media.ThumbnailData,
		nullableInt(media.CropLeft), nullableInt(media.CropTop),
		nullableInt(media.CropWidth), nullableInt(media.CropHeight),
		nullableString(media.GedcomXref), media.Version,
		formatTimestamp(media.CreatedAt), formatTimestamp(media.UpdatedAt),
		nullableBytes(filesJSON), nullableString(media.Format), nullableBytes(translationsJSON))

	return err
}

// DeleteMedia removes a media record.
func (s *ReadModelStore) DeleteMedia(ctx context.Context, id uuid.UUID) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM media WHERE id = ?", id.String())
	return err
}

// Media scanner helpers

func scanMediaMetadata(row rowScanner) (*repository.MediaReadModel, error) {
	var (
		idStr, entityType, entityIDStr string
		title, mimeType, mediaType     string
		filename                       string
		description, gedcomXref        sql.NullString
		fileSize, version              int64
		cropLeft, cropTop              sql.NullInt64
		cropWidth, cropHeight          sql.NullInt64
		createdAt, updatedAt           string
		// GEDCOM 7.0 enhanced fields
		filesJSON, translationsJSON sql.NullString
		format                      sql.NullString
	)

	err := row.Scan(&idStr, &entityType, &entityIDStr, &title, &description,
		&mimeType, &mediaType, &filename, &fileSize,
		&cropLeft, &cropTop, &cropWidth, &cropHeight,
		&gedcomXref, &version, &createdAt, &updatedAt,
		&filesJSON, &format, &translationsJSON)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan media metadata: %w", err)
	}

	id, _ := uuid.Parse(idStr)
	entityID, _ := uuid.Parse(entityIDStr)

	// Deserialize JSON fields
	files, err := domain.UnmarshalFilesFromJSON([]byte(filesJSON.String))
	if err != nil {
		return nil, fmt.Errorf("unmarshal files: %w", err)
	}
	translations, err := domain.UnmarshalTranslationsFromJSON([]byte(translationsJSON.String))
	if err != nil {
		return nil, fmt.Errorf("unmarshal translations: %w", err)
	}

	m := &repository.MediaReadModel{
		ID:           id,
		EntityType:   entityType,
		EntityID:     entityID,
		Title:        title,
		Description:  description.String,
		MimeType:     mimeType,
		MediaType:    domain.MediaType(mediaType),
		Filename:     filename,
		FileSize:     fileSize,
		GedcomXref:   gedcomXref.String,
		Version:      version,
		Files:        files,
		Format:       format.String,
		Translations: translations,
	}

	if cropLeft.Valid {
		v := int(cropLeft.Int64)
		m.CropLeft = &v
	}
	if cropTop.Valid {
		v := int(cropTop.Int64)
		m.CropTop = &v
	}
	if cropWidth.Valid {
		v := int(cropWidth.Int64)
		m.CropWidth = &v
	}
	if cropHeight.Valid {
		v := int(cropHeight.Int64)
		m.CropHeight = &v
	}

	if t, err := parseTimestamp(createdAt); err == nil {
		m.CreatedAt = t
	}
	if t, err := parseTimestamp(updatedAt); err == nil {
		m.UpdatedAt = t
	}

	return m, nil
}

func scanMediaMetadataRow(rows *sql.Rows) (*repository.MediaReadModel, error) {
	return scanMediaMetadata(rows)
}

func scanMediaFull(row rowScanner) (*repository.MediaReadModel, error) {
	var (
		idStr, entityType, entityIDStr string
		title, mimeType, mediaType     string
		filename                       string
		description, gedcomXref        sql.NullString
		fileSize, version              int64
		fileData, thumbnailData        []byte
		cropLeft, cropTop              sql.NullInt64
		cropWidth, cropHeight          sql.NullInt64
		createdAt, updatedAt           string
		// GEDCOM 7.0 enhanced fields
		filesJSON, translationsJSON sql.NullString
		format                      sql.NullString
	)

	err := row.Scan(&idStr, &entityType, &entityIDStr, &title, &description,
		&mimeType, &mediaType, &filename, &fileSize, &fileData, &thumbnailData,
		&cropLeft, &cropTop, &cropWidth, &cropHeight,
		&gedcomXref, &version, &createdAt, &updatedAt,
		&filesJSON, &format, &translationsJSON)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan media full: %w", err)
	}

	id, _ := uuid.Parse(idStr)
	entityID, _ := uuid.Parse(entityIDStr)

	// Deserialize JSON fields
	files, err := domain.UnmarshalFilesFromJSON([]byte(filesJSON.String))
	if err != nil {
		return nil, fmt.Errorf("unmarshal files: %w", err)
	}
	translations, err := domain.UnmarshalTranslationsFromJSON([]byte(translationsJSON.String))
	if err != nil {
		return nil, fmt.Errorf("unmarshal translations: %w", err)
	}

	m := &repository.MediaReadModel{
		ID:            id,
		EntityType:    entityType,
		EntityID:      entityID,
		Title:         title,
		Description:   description.String,
		MimeType:      mimeType,
		MediaType:     domain.MediaType(mediaType),
		Filename:      filename,
		FileSize:      fileSize,
		FileData:      fileData,
		ThumbnailData: thumbnailData,
		GedcomXref:    gedcomXref.String,
		Version:       version,
		Files:         files,
		Format:        format.String,
		Translations:  translations,
	}

	if cropLeft.Valid {
		v := int(cropLeft.Int64)
		m.CropLeft = &v
	}
	if cropTop.Valid {
		v := int(cropTop.Int64)
		m.CropTop = &v
	}
	if cropWidth.Valid {
		v := int(cropWidth.Int64)
		m.CropWidth = &v
	}
	if cropHeight.Valid {
		v := int(cropHeight.Int64)
		m.CropHeight = &v
	}

	if t, err := parseTimestamp(createdAt); err == nil {
		m.CreatedAt = t
	}
	if t, err := parseTimestamp(updatedAt); err == nil {
		m.UpdatedAt = t
	}

	return m, nil
}

// GetSurnameIndex returns all unique surnames with counts and letter counts.
func (s *ReadModelStore) GetSurnameIndex(ctx context.Context) ([]repository.SurnameEntry, []repository.LetterCount, error) {
	// Get surname counts
	rows, err := s.db.QueryContext(ctx, `
		SELECT surname, COUNT(*) as count
		FROM persons
		GROUP BY surname
		ORDER BY surname ASC
	`)
	if err != nil {
		return nil, nil, fmt.Errorf("query surname index: %w", err)
	}
	defer rows.Close()

	var surnames []repository.SurnameEntry
	for rows.Next() {
		var entry repository.SurnameEntry
		if err := rows.Scan(&entry.Surname, &entry.Count); err != nil {
			return nil, nil, fmt.Errorf("scan surname entry: %w", err)
		}
		surnames = append(surnames, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}

	// Get letter counts
	letterRows, err := s.db.QueryContext(ctx, `
		SELECT UPPER(SUBSTR(surname, 1, 1)) as letter, COUNT(DISTINCT surname) as count
		FROM persons
		WHERE surname != ''
		GROUP BY UPPER(SUBSTR(surname, 1, 1))
		ORDER BY letter ASC
	`)
	if err != nil {
		return nil, nil, fmt.Errorf("query letter counts: %w", err)
	}
	defer letterRows.Close()

	var letterCounts []repository.LetterCount
	for letterRows.Next() {
		var entry repository.LetterCount
		if err := letterRows.Scan(&entry.Letter, &entry.Count); err != nil {
			return nil, nil, fmt.Errorf("scan letter count: %w", err)
		}
		letterCounts = append(letterCounts, entry)
	}

	return surnames, letterCounts, letterRows.Err()
}

// GetSurnamesByLetter returns surnames starting with a specific letter.
func (s *ReadModelStore) GetSurnamesByLetter(ctx context.Context, letter string) ([]repository.SurnameEntry, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT surname, COUNT(*) as count
		FROM persons
		WHERE UPPER(SUBSTR(surname, 1, 1)) = UPPER(?)
		GROUP BY surname
		ORDER BY surname ASC
	`, letter)
	if err != nil {
		return nil, fmt.Errorf("query surnames by letter: %w", err)
	}
	defer rows.Close()

	var surnames []repository.SurnameEntry
	for rows.Next() {
		var entry repository.SurnameEntry
		if err := rows.Scan(&entry.Surname, &entry.Count); err != nil {
			return nil, fmt.Errorf("scan surname entry: %w", err)
		}
		surnames = append(surnames, entry)
	}

	return surnames, rows.Err()
}

// GetPersonsBySurname returns persons with a specific surname.
func (s *ReadModelStore) GetPersonsBySurname(ctx context.Context, surname string, opts repository.ListOptions) ([]repository.PersonReadModel, int, error) {
	// Count total
	var total int
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM persons WHERE LOWER(surname) = LOWER(?)", surname).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("count persons by surname: %w", err)
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, given_name, surname, full_name, gender,
			   birth_date_raw, birth_date_sort, birth_place, birth_place_lat, birth_place_long,
			   death_date_raw, death_date_sort, death_place, death_place_lat, death_place_long,
			   notes, research_status, version, updated_at
		FROM persons
		WHERE LOWER(surname) = LOWER(?)
		ORDER BY given_name ASC
		LIMIT ? OFFSET ?
	`, surname, opts.Limit, opts.Offset)
	if err != nil {
		return nil, 0, fmt.Errorf("query persons by surname: %w", err)
	}
	defer rows.Close()

	var persons []repository.PersonReadModel
	for rows.Next() {
		p, err := scanPersonRow(rows)
		if err != nil {
			return nil, 0, err
		}
		persons = append(persons, *p)
	}

	return persons, total, rows.Err()
}

// GetPlaceHierarchy returns places at a given level in the hierarchy.
// Places are parsed from comma-separated strings like "City, County, State, Country"
// working from right to left (Country is top level).
func (s *ReadModelStore) GetPlaceHierarchy(ctx context.Context, parent string) ([]repository.PlaceEntry, error) {
	var rows *sql.Rows
	var err error

	if parent == "" {
		// Top-level: get unique countries/top-level places (rightmost part after last comma)
		rows, err = s.db.QueryContext(ctx, `
			WITH all_places AS (
				SELECT DISTINCT birth_place as place FROM persons WHERE birth_place != '' AND birth_place IS NOT NULL
				UNION
				SELECT DISTINCT death_place as place FROM persons WHERE death_place != '' AND death_place IS NOT NULL
			),
			parsed AS (
				SELECT
					place,
					CASE
						WHEN INSTR(place, ',') > 0
						THEN TRIM(SUBSTR(place, LENGTH(place) - LENGTH(REPLACE(SUBSTR(place, INSTR(place, ',')), ',', '')) + 1))
						ELSE TRIM(place)
					END as top_level
				FROM all_places
			)
			SELECT
				top_level as place_name,
				top_level as full_name,
				COUNT(DISTINCT place) as count,
				CASE
					WHEN COUNT(DISTINCT place) > (SELECT COUNT(*) FROM parsed p2 WHERE p2.top_level = parsed.top_level AND p2.place = p2.top_level)
					THEN 1
					ELSE 0
				END as has_children
			FROM parsed
			WHERE top_level != ''
			GROUP BY top_level
			ORDER BY top_level ASC
		`)
	} else {
		// Child level: get places that end with parent
		rows, err = s.db.QueryContext(ctx, `
			WITH all_places AS (
				SELECT DISTINCT birth_place as place FROM persons WHERE birth_place LIKE '%' || ? AND birth_place != ''
				UNION
				SELECT DISTINCT death_place as place FROM persons WHERE death_place LIKE '%' || ? AND death_place != ''
			),
			parsed AS (
				SELECT
					place,
					CASE
						WHEN place = ? THEN ''
						ELSE TRIM(REPLACE(place, ', ' || ?, ''))
					END as remainder
				FROM all_places
			),
			next_level AS (
				SELECT
					place,
					remainder,
					CASE
						WHEN remainder = '' THEN ''
						WHEN INSTR(remainder, ',') > 0
						THEN TRIM(SUBSTR(remainder, LENGTH(remainder) - LENGTH(REPLACE(SUBSTR(remainder, INSTR(remainder, ',')), ',', '')) + 1))
						ELSE TRIM(remainder)
					END as level_name
				FROM parsed
			)
			SELECT
				level_name as place_name,
				level_name || ', ' || ? as full_name,
				COUNT(DISTINCT place) as count,
				CASE
					WHEN COUNT(DISTINCT place) > COUNT(DISTINCT CASE WHEN remainder = level_name THEN place END)
					THEN 1
					ELSE 0
				END as has_children
			FROM next_level
			WHERE level_name != '' AND level_name != ?
			GROUP BY level_name
			ORDER BY level_name ASC
		`, parent, parent, parent, parent, parent, parent)
	}
	if err != nil {
		return nil, fmt.Errorf("query place hierarchy: %w", err)
	}
	defer rows.Close()

	var places []repository.PlaceEntry
	for rows.Next() {
		var entry repository.PlaceEntry
		var hasChildrenInt int
		if err := rows.Scan(&entry.Name, &entry.FullName, &entry.Count, &hasChildrenInt); err != nil {
			return nil, fmt.Errorf("scan place entry: %w", err)
		}
		entry.HasChildren = hasChildrenInt == 1
		places = append(places, entry)
	}

	return places, rows.Err()
}

// GetPersonsByPlace returns persons associated with a place.
func (s *ReadModelStore) GetPersonsByPlace(ctx context.Context, place string, opts repository.ListOptions) ([]repository.PersonReadModel, int, error) {
	// Count total - match place at any position in birth_place or death_place
	var total int
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM persons
		WHERE birth_place LIKE '%' || ? || '%' OR death_place LIKE '%' || ? || '%'
	`, place, place).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("count persons by place: %w", err)
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, given_name, surname, full_name, gender,
			   birth_date_raw, birth_date_sort, birth_place, birth_place_lat, birth_place_long,
			   death_date_raw, death_date_sort, death_place, death_place_lat, death_place_long,
			   notes, research_status, version, updated_at
		FROM persons
		WHERE birth_place LIKE '%' || ? || '%' OR death_place LIKE '%' || ? || '%'
		ORDER BY surname ASC, given_name ASC
		LIMIT ? OFFSET ?
	`, place, place, opts.Limit, opts.Offset)
	if err != nil {
		return nil, 0, fmt.Errorf("query persons by place: %w", err)
	}
	defer rows.Close()

	var persons []repository.PersonReadModel
	for rows.Next() {
		p, err := scanPersonRow(rows)
		if err != nil {
			return nil, 0, err
		}
		persons = append(persons, *p)
	}

	return persons, total, rows.Err()
}

// GetCemeteryIndex returns unique burial/cremation places with person counts.
func (s *ReadModelStore) GetCemeteryIndex(ctx context.Context) ([]repository.CemeteryEntry, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT place, COUNT(DISTINCT owner_id) as count
		FROM events
		WHERE fact_type IN (?, ?) AND place != '' AND place IS NOT NULL
		GROUP BY place
		ORDER BY place ASC
	`, string(domain.FactPersonBurial), string(domain.FactPersonCremation))
	if err != nil {
		return nil, fmt.Errorf("query cemetery index: %w", err)
	}
	defer rows.Close()

	var entries []repository.CemeteryEntry
	for rows.Next() {
		var entry repository.CemeteryEntry
		if err := rows.Scan(&entry.Place, &entry.Count); err != nil {
			return nil, fmt.Errorf("scan cemetery entry: %w", err)
		}
		entries = append(entries, entry)
	}

	return entries, rows.Err()
}

// GetPersonsByCemetery returns persons with burial/cremation events at the given place.
func (s *ReadModelStore) GetPersonsByCemetery(ctx context.Context, place string, opts repository.ListOptions) ([]repository.PersonReadModel, int, error) {
	// Count total distinct persons
	var total int
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(DISTINCT p.id)
		FROM persons p
		INNER JOIN events e ON e.owner_id = p.id
		WHERE e.fact_type IN (?, ?) AND LOWER(e.place) = LOWER(?)
	`, string(domain.FactPersonBurial), string(domain.FactPersonCremation), place).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("count persons by cemetery: %w", err)
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT DISTINCT p.id, p.given_name, p.surname, p.full_name, p.gender,
			   p.birth_date_raw, p.birth_date_sort, p.birth_place, p.birth_place_lat, p.birth_place_long,
			   p.death_date_raw, p.death_date_sort, p.death_place, p.death_place_lat, p.death_place_long,
			   p.notes, p.research_status, p.version, p.updated_at
		FROM persons p
		INNER JOIN events e ON e.owner_id = p.id
		WHERE e.fact_type IN (?, ?) AND LOWER(e.place) = LOWER(?)
		ORDER BY p.surname ASC, p.given_name ASC
		LIMIT ? OFFSET ?
	`, string(domain.FactPersonBurial), string(domain.FactPersonCremation), place, opts.Limit, opts.Offset)
	if err != nil {
		return nil, 0, fmt.Errorf("query persons by cemetery: %w", err)
	}
	defer rows.Close()

	var persons []repository.PersonReadModel
	for rows.Next() {
		p, err := scanPersonRow(rows)
		if err != nil {
			return nil, 0, err
		}
		persons = append(persons, *p)
	}

	return persons, total, rows.Err()
}

// GetNote retrieves a note by ID.
func (s *ReadModelStore) GetNote(ctx context.Context, id uuid.UUID) (*repository.NoteReadModel, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, text, gedcom_xref, version, updated_at
		FROM notes WHERE id = ?
	`, id.String())

	var note repository.NoteReadModel
	var idStr string
	var gedcomXref sql.NullString
	var updatedAtStr string

	err := row.Scan(
		&idStr,
		&note.Text,
		&gedcomXref,
		&note.Version,
		&updatedAtStr,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan note: %w", err)
	}

	note.ID, _ = uuid.Parse(idStr)
	if gedcomXref.Valid {
		note.GedcomXref = gedcomXref.String
	}
	if t, err := parseTimestamp(updatedAtStr); err == nil {
		note.UpdatedAt = t
	}

	return &note, nil
}

// ListNotes returns a paginated list of notes.
func (s *ReadModelStore) ListNotes(ctx context.Context, opts repository.ListOptions) ([]repository.NoteReadModel, int, error) {
	// Count total
	var total int
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM notes").Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("count notes: %w", err)
	}

	// Build order clause
	orderColumn := "updated_at"
	orderDir := "DESC"
	if opts.Order == "asc" {
		orderDir = "ASC"
	}

	// #nosec G201 -- orderColumn and orderDir are validated via switch/if above, not user input
	query := fmt.Sprintf(`
		SELECT id, text, gedcom_xref, version, updated_at
		FROM notes
		ORDER BY %s %s
		LIMIT ? OFFSET ?
	`, orderColumn, orderDir)

	rows, err := s.db.QueryContext(ctx, query, opts.Limit, opts.Offset)
	if err != nil {
		return nil, 0, fmt.Errorf("query notes: %w", err)
	}
	defer rows.Close()

	var notes []repository.NoteReadModel
	for rows.Next() {
		var note repository.NoteReadModel
		var idStr string
		var gedcomXref sql.NullString
		var updatedAtStr string

		if err := rows.Scan(
			&idStr,
			&note.Text,
			&gedcomXref,
			&note.Version,
			&updatedAtStr,
		); err != nil {
			return nil, 0, fmt.Errorf("scan note: %w", err)
		}

		note.ID, _ = uuid.Parse(idStr)
		if gedcomXref.Valid {
			note.GedcomXref = gedcomXref.String
		}
		if t, err := parseTimestamp(updatedAtStr); err == nil {
			note.UpdatedAt = t
		}
		notes = append(notes, note)
	}

	return notes, total, rows.Err()
}

// SaveNote saves or updates a note.
func (s *ReadModelStore) SaveNote(ctx context.Context, note *repository.NoteReadModel) error {
	var gedcomXref any
	if note.GedcomXref != "" {
		gedcomXref = note.GedcomXref
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO notes (id, text, gedcom_xref, version, updated_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET
			text = excluded.text,
			gedcom_xref = excluded.gedcom_xref,
			version = excluded.version,
			updated_at = excluded.updated_at
	`, note.ID.String(), note.Text, gedcomXref, note.Version, note.UpdatedAt.Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("save note: %w", err)
	}
	return nil
}

// DeleteNote deletes a note by ID.
func (s *ReadModelStore) DeleteNote(ctx context.Context, id uuid.UUID) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM notes WHERE id = ?", id.String())
	if err != nil {
		return fmt.Errorf("delete note: %w", err)
	}
	return nil
}

// GetSubmitter retrieves a submitter by ID.
func (s *ReadModelStore) GetSubmitter(ctx context.Context, id uuid.UUID) (*repository.SubmitterReadModel, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, name, address, phone, email, language, media_id, gedcom_xref, version, updated_at
		FROM submitters WHERE id = ?
	`, id.String())

	var submitter repository.SubmitterReadModel
	var idStr string
	var addressJSON, phoneJSON, emailJSON []byte
	var gedcomXref sql.NullString
	var mediaID sql.NullString
	var language sql.NullString
	var updatedAtStr string

	err := row.Scan(
		&idStr,
		&submitter.Name,
		&addressJSON,
		&phoneJSON,
		&emailJSON,
		&language,
		&mediaID,
		&gedcomXref,
		&submitter.Version,
		&updatedAtStr,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan submitter: %w", err)
	}

	submitter.ID, _ = uuid.Parse(idStr)
	if gedcomXref.Valid {
		submitter.GedcomXref = gedcomXref.String
	}
	if language.Valid {
		submitter.Language = language.String
	}
	if mediaID.Valid {
		if id, err := uuid.Parse(mediaID.String); err == nil {
			submitter.MediaID = &id
		}
	}
	if len(addressJSON) > 0 {
		var addr domain.Address
		if err := json.Unmarshal(addressJSON, &addr); err == nil {
			submitter.Address = &addr
		}
	}
	if len(phoneJSON) > 0 {
		_ = json.Unmarshal(phoneJSON, &submitter.Phone)
	}
	if len(emailJSON) > 0 {
		_ = json.Unmarshal(emailJSON, &submitter.Email)
	}
	if t, err := parseTimestamp(updatedAtStr); err == nil {
		submitter.UpdatedAt = t
	}

	return &submitter, nil
}

// ListSubmitters returns a paginated list of submitters.
func (s *ReadModelStore) ListSubmitters(ctx context.Context, opts repository.ListOptions) ([]repository.SubmitterReadModel, int, error) {
	// Count total
	var total int
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM submitters").Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("count submitters: %w", err)
	}

	// Build order clause
	orderColumn := "updated_at"
	if opts.Sort == "name" {
		orderColumn = "name"
	}
	orderDir := "DESC"
	if opts.Order == "asc" {
		orderDir = "ASC"
	}

	// #nosec G201 -- orderColumn and orderDir are validated via switch/if above, not user input
	query := fmt.Sprintf(`
		SELECT id, name, address, phone, email, language, media_id, gedcom_xref, version, updated_at
		FROM submitters
		ORDER BY %s %s
		LIMIT ? OFFSET ?
	`, orderColumn, orderDir)

	rows, err := s.db.QueryContext(ctx, query, opts.Limit, opts.Offset)
	if err != nil {
		return nil, 0, fmt.Errorf("query submitters: %w", err)
	}
	defer rows.Close()

	var submitters []repository.SubmitterReadModel
	for rows.Next() {
		var submitter repository.SubmitterReadModel
		var idStr string
		var addressJSON, phoneJSON, emailJSON []byte
		var gedcomXref sql.NullString
		var mediaID sql.NullString
		var language sql.NullString
		var updatedAtStr string

		if err := rows.Scan(
			&idStr,
			&submitter.Name,
			&addressJSON,
			&phoneJSON,
			&emailJSON,
			&language,
			&mediaID,
			&gedcomXref,
			&submitter.Version,
			&updatedAtStr,
		); err != nil {
			return nil, 0, fmt.Errorf("scan submitter: %w", err)
		}

		submitter.ID, _ = uuid.Parse(idStr)
		if gedcomXref.Valid {
			submitter.GedcomXref = gedcomXref.String
		}
		if language.Valid {
			submitter.Language = language.String
		}
		if mediaID.Valid {
			if id, err := uuid.Parse(mediaID.String); err == nil {
				submitter.MediaID = &id
			}
		}
		if len(addressJSON) > 0 {
			var addr domain.Address
			if err := json.Unmarshal(addressJSON, &addr); err == nil {
				submitter.Address = &addr
			}
		}
		if len(phoneJSON) > 0 {
			_ = json.Unmarshal(phoneJSON, &submitter.Phone)
		}
		if len(emailJSON) > 0 {
			_ = json.Unmarshal(emailJSON, &submitter.Email)
		}
		if t, err := parseTimestamp(updatedAtStr); err == nil {
			submitter.UpdatedAt = t
		}
		submitters = append(submitters, submitter)
	}

	return submitters, total, rows.Err()
}

// SaveSubmitter saves or updates a submitter.
func (s *ReadModelStore) SaveSubmitter(ctx context.Context, submitter *repository.SubmitterReadModel) error {
	var addressJSON, phoneJSON, emailJSON []byte
	var err error

	if submitter.Address != nil {
		addressJSON, err = json.Marshal(submitter.Address)
		if err != nil {
			return fmt.Errorf("marshal address: %w", err)
		}
	}
	if len(submitter.Phone) > 0 {
		phoneJSON, err = json.Marshal(submitter.Phone)
		if err != nil {
			return fmt.Errorf("marshal phone: %w", err)
		}
	}
	if len(submitter.Email) > 0 {
		emailJSON, err = json.Marshal(submitter.Email)
		if err != nil {
			return fmt.Errorf("marshal email: %w", err)
		}
	}

	var mediaID any
	if submitter.MediaID != nil {
		mediaID = submitter.MediaID.String()
	}
	var language any
	if submitter.Language != "" {
		language = submitter.Language
	}
	var gedcomXref any
	if submitter.GedcomXref != "" {
		gedcomXref = submitter.GedcomXref
	}

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO submitters (id, name, address, phone, email, language, media_id, gedcom_xref, version, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET
			name = excluded.name,
			address = excluded.address,
			phone = excluded.phone,
			email = excluded.email,
			language = excluded.language,
			media_id = excluded.media_id,
			gedcom_xref = excluded.gedcom_xref,
			version = excluded.version,
			updated_at = excluded.updated_at
	`, submitter.ID.String(), submitter.Name, addressJSON, phoneJSON, emailJSON,
		language, mediaID, gedcomXref, submitter.Version, submitter.UpdatedAt.Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("save submitter: %w", err)
	}
	return nil
}

// DeleteSubmitter deletes a submitter by ID.
func (s *ReadModelStore) DeleteSubmitter(ctx context.Context, id uuid.UUID) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM submitters WHERE id = ?", id.String())
	if err != nil {
		return fmt.Errorf("delete submitter: %w", err)
	}
	return nil
}

// GetAssociation retrieves an association by ID.
func (s *ReadModelStore) GetAssociation(ctx context.Context, id uuid.UUID) (*repository.AssociationReadModel, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, person_id, person_name, associate_id, associate_name,
		       role, phrase, notes, note_ids, gedcom_xref, version, updated_at
		FROM associations WHERE id = ?
	`, id.String())

	var assoc repository.AssociationReadModel
	var idStr, personIDStr, associateIDStr string
	var personName, associateName, phrase, notes sql.NullString
	var noteIDsJSON sql.NullString
	var gedcomXref sql.NullString
	var updatedAtStr string
	err := row.Scan(
		&idStr,
		&personIDStr,
		&personName,
		&associateIDStr,
		&associateName,
		&assoc.Role,
		&phrase,
		&notes,
		&noteIDsJSON,
		&gedcomXref,
		&assoc.Version,
		&updatedAtStr,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan association: %w", err)
	}
	assoc.ID, _ = uuid.Parse(idStr)
	assoc.PersonID, _ = uuid.Parse(personIDStr)
	assoc.AssociateID, _ = uuid.Parse(associateIDStr)
	if personName.Valid {
		assoc.PersonName = personName.String
	}
	if associateName.Valid {
		assoc.AssociateName = associateName.String
	}
	if phrase.Valid {
		assoc.Phrase = phrase.String
	}
	if notes.Valid {
		assoc.Notes = notes.String
	}
	if gedcomXref.Valid {
		assoc.GedcomXref = gedcomXref.String
	}
	if noteIDsJSON.Valid && noteIDsJSON.String != "" {
		_ = json.Unmarshal([]byte(noteIDsJSON.String), &assoc.NoteIDs)
	}
	if t, err := parseTimestamp(updatedAtStr); err == nil {
		assoc.UpdatedAt = t
	}
	return &assoc, nil
}

// ListAssociations returns a paginated list of associations.
func (s *ReadModelStore) ListAssociations(ctx context.Context, opts repository.ListOptions) ([]repository.AssociationReadModel, int, error) {
	// Count total
	var total int
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM associations").Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("count associations: %w", err)
	}

	// Build order clause
	orderColumn := "updated_at"
	if opts.Sort == "role" {
		orderColumn = "role"
	}
	orderDir := "DESC"
	if opts.Order == "asc" {
		orderDir = "ASC"
	}

	// #nosec G201 -- orderColumn and orderDir are validated via switch/if above, not user input
	query := fmt.Sprintf(`
		SELECT id, person_id, person_name, associate_id, associate_name,
		       role, phrase, notes, note_ids, gedcom_xref, version, updated_at
		FROM associations
		ORDER BY %s %s
		LIMIT ? OFFSET ?
	`, orderColumn, orderDir)

	rows, err := s.db.QueryContext(ctx, query, opts.Limit, opts.Offset)
	if err != nil {
		return nil, 0, fmt.Errorf("query associations: %w", err)
	}
	defer rows.Close()

	var associations []repository.AssociationReadModel
	for rows.Next() {
		var assoc repository.AssociationReadModel
		var idStr, personIDStr, associateIDStr string
		var personName, associateName, phrase, notes sql.NullString
		var noteIDsJSON sql.NullString
		var gedcomXref sql.NullString
		var updatedAtStr string
		if err := rows.Scan(
			&idStr,
			&personIDStr,
			&personName,
			&associateIDStr,
			&associateName,
			&assoc.Role,
			&phrase,
			&notes,
			&noteIDsJSON,
			&gedcomXref,
			&assoc.Version,
			&updatedAtStr,
		); err != nil {
			return nil, 0, fmt.Errorf("scan association: %w", err)
		}
		assoc.ID, _ = uuid.Parse(idStr)
		assoc.PersonID, _ = uuid.Parse(personIDStr)
		assoc.AssociateID, _ = uuid.Parse(associateIDStr)
		if personName.Valid {
			assoc.PersonName = personName.String
		}
		if associateName.Valid {
			assoc.AssociateName = associateName.String
		}
		if phrase.Valid {
			assoc.Phrase = phrase.String
		}
		if notes.Valid {
			assoc.Notes = notes.String
		}
		if gedcomXref.Valid {
			assoc.GedcomXref = gedcomXref.String
		}
		if noteIDsJSON.Valid && noteIDsJSON.String != "" {
			_ = json.Unmarshal([]byte(noteIDsJSON.String), &assoc.NoteIDs)
		}
		if t, err := parseTimestamp(updatedAtStr); err == nil {
			assoc.UpdatedAt = t
		}
		associations = append(associations, assoc)
	}

	return associations, total, rows.Err()
}

// ListAssociationsForPerson returns all associations for a given person.
func (s *ReadModelStore) ListAssociationsForPerson(ctx context.Context, personID uuid.UUID) ([]repository.AssociationReadModel, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, person_id, person_name, associate_id, associate_name,
		       role, phrase, notes, note_ids, gedcom_xref, version, updated_at
		FROM associations
		WHERE person_id = ? OR associate_id = ?
		ORDER BY role, updated_at DESC
	`, personID.String(), personID.String())
	if err != nil {
		return nil, fmt.Errorf("query associations for person: %w", err)
	}
	defer rows.Close()

	var associations []repository.AssociationReadModel
	for rows.Next() {
		var assoc repository.AssociationReadModel
		var idStr, personIDStr, associateIDStr string
		var personName, associateName, phrase, notes sql.NullString
		var noteIDsJSON sql.NullString
		var gedcomXref sql.NullString
		var updatedAtStr string
		if err := rows.Scan(
			&idStr,
			&personIDStr,
			&personName,
			&associateIDStr,
			&associateName,
			&assoc.Role,
			&phrase,
			&notes,
			&noteIDsJSON,
			&gedcomXref,
			&assoc.Version,
			&updatedAtStr,
		); err != nil {
			return nil, fmt.Errorf("scan association: %w", err)
		}
		assoc.ID, _ = uuid.Parse(idStr)
		assoc.PersonID, _ = uuid.Parse(personIDStr)
		assoc.AssociateID, _ = uuid.Parse(associateIDStr)
		if personName.Valid {
			assoc.PersonName = personName.String
		}
		if associateName.Valid {
			assoc.AssociateName = associateName.String
		}
		if phrase.Valid {
			assoc.Phrase = phrase.String
		}
		if notes.Valid {
			assoc.Notes = notes.String
		}
		if gedcomXref.Valid {
			assoc.GedcomXref = gedcomXref.String
		}
		if noteIDsJSON.Valid && noteIDsJSON.String != "" {
			_ = json.Unmarshal([]byte(noteIDsJSON.String), &assoc.NoteIDs)
		}
		if t, err := parseTimestamp(updatedAtStr); err == nil {
			assoc.UpdatedAt = t
		}
		associations = append(associations, assoc)
	}

	return associations, rows.Err()
}

// SaveAssociation saves or updates an association.
func (s *ReadModelStore) SaveAssociation(ctx context.Context, assoc *repository.AssociationReadModel) error {
	var noteIDsJSON any
	if len(assoc.NoteIDs) > 0 {
		jsonBytes, err := json.Marshal(assoc.NoteIDs)
		if err != nil {
			return fmt.Errorf("marshal note_ids: %w", err)
		}
		noteIDsJSON = string(jsonBytes)
	}

	var personName, associateName, phrase, notes, gedcomXref any
	if assoc.PersonName != "" {
		personName = assoc.PersonName
	}
	if assoc.AssociateName != "" {
		associateName = assoc.AssociateName
	}
	if assoc.Phrase != "" {
		phrase = assoc.Phrase
	}
	if assoc.Notes != "" {
		notes = assoc.Notes
	}
	if assoc.GedcomXref != "" {
		gedcomXref = assoc.GedcomXref
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO associations (id, person_id, person_name, associate_id, associate_name,
		                         role, phrase, notes, note_ids, gedcom_xref, version, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET
			person_id = excluded.person_id,
			person_name = excluded.person_name,
			associate_id = excluded.associate_id,
			associate_name = excluded.associate_name,
			role = excluded.role,
			phrase = excluded.phrase,
			notes = excluded.notes,
			note_ids = excluded.note_ids,
			gedcom_xref = excluded.gedcom_xref,
			version = excluded.version,
			updated_at = excluded.updated_at
	`, assoc.ID.String(), assoc.PersonID.String(), personName, assoc.AssociateID.String(), associateName,
		assoc.Role, phrase, notes, noteIDsJSON, gedcomXref, assoc.Version, assoc.UpdatedAt.Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("save association: %w", err)
	}
	return nil
}

// DeleteAssociation deletes an association by ID.
func (s *ReadModelStore) DeleteAssociation(ctx context.Context, id uuid.UUID) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM associations WHERE id = ?", id.String())
	if err != nil {
		return fmt.Errorf("delete association: %w", err)
	}
	return nil
}

// GetLDSOrdinance retrieves an LDS ordinance by ID.
func (s *ReadModelStore) GetLDSOrdinance(ctx context.Context, id uuid.UUID) (*repository.LDSOrdinanceReadModel, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, type, type_label, person_id, person_name, family_id,
		       date_raw, date_sort, place, temple, status, version, updated_at
		FROM lds_ordinances WHERE id = ?
	`, id.String())

	var ordinance repository.LDSOrdinanceReadModel
	var idStr string
	var personID, familyID sql.NullString
	var personName, dateRaw, place, temple, status sql.NullString
	var dateSort sql.NullString
	var updatedAtStr string

	err := row.Scan(
		&idStr,
		&ordinance.Type,
		&ordinance.TypeLabel,
		&personID,
		&personName,
		&familyID,
		&dateRaw,
		&dateSort,
		&place,
		&temple,
		&status,
		&ordinance.Version,
		&updatedAtStr,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan lds_ordinance: %w", err)
	}

	ordinance.ID, _ = uuid.Parse(idStr)
	if personID.Valid {
		id, _ := uuid.Parse(personID.String)
		ordinance.PersonID = &id
	}
	if familyID.Valid {
		id, _ := uuid.Parse(familyID.String)
		ordinance.FamilyID = &id
	}
	if personName.Valid {
		ordinance.PersonName = personName.String
	}
	if dateRaw.Valid {
		ordinance.DateRaw = dateRaw.String
	}
	if dateSort.Valid {
		if t, err := parseTimestamp(dateSort.String); err == nil {
			ordinance.DateSort = &t
		}
	}
	if place.Valid {
		ordinance.Place = place.String
	}
	if temple.Valid {
		ordinance.Temple = temple.String
	}
	if status.Valid {
		ordinance.Status = status.String
	}
	if t, err := parseTimestamp(updatedAtStr); err == nil {
		ordinance.UpdatedAt = t
	}
	return &ordinance, nil
}

// ListLDSOrdinances returns a paginated list of LDS ordinances.
func (s *ReadModelStore) ListLDSOrdinances(ctx context.Context, opts repository.ListOptions) ([]repository.LDSOrdinanceReadModel, int, error) {
	// Count total
	var total int
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM lds_ordinances").Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("count lds_ordinances: %w", err)
	}

	// Build order clause
	orderColumn := "updated_at"
	switch opts.Sort {
	case "type":
		orderColumn = "type"
	case "date":
		orderColumn = "date_sort"
	}
	orderDir := "DESC"
	if opts.Order == "asc" {
		orderDir = "ASC"
	}

	// #nosec G201 -- orderColumn and orderDir are validated via switch/if above, not user input
	query := fmt.Sprintf(`
		SELECT id, type, type_label, person_id, person_name, family_id,
		       date_raw, date_sort, place, temple, status, version, updated_at
		FROM lds_ordinances
		ORDER BY %s %s
		LIMIT ? OFFSET ?
	`, orderColumn, orderDir)

	rows, err := s.db.QueryContext(ctx, query, opts.Limit, opts.Offset)
	if err != nil {
		return nil, 0, fmt.Errorf("query lds_ordinances: %w", err)
	}
	defer rows.Close()

	var ordinances []repository.LDSOrdinanceReadModel
	for rows.Next() {
		var ordinance repository.LDSOrdinanceReadModel
		var idStr string
		var personID, familyID sql.NullString
		var personName, dateRaw, place, temple, status sql.NullString
		var dateSort sql.NullString
		var updatedAtStr string
		if err := rows.Scan(
			&idStr,
			&ordinance.Type,
			&ordinance.TypeLabel,
			&personID,
			&personName,
			&familyID,
			&dateRaw,
			&dateSort,
			&place,
			&temple,
			&status,
			&ordinance.Version,
			&updatedAtStr,
		); err != nil {
			return nil, 0, fmt.Errorf("scan lds_ordinance: %w", err)
		}
		ordinance.ID, _ = uuid.Parse(idStr)
		if personID.Valid {
			id, _ := uuid.Parse(personID.String)
			ordinance.PersonID = &id
		}
		if familyID.Valid {
			id, _ := uuid.Parse(familyID.String)
			ordinance.FamilyID = &id
		}
		if personName.Valid {
			ordinance.PersonName = personName.String
		}
		if dateRaw.Valid {
			ordinance.DateRaw = dateRaw.String
		}
		if dateSort.Valid {
			if t, err := parseTimestamp(dateSort.String); err == nil {
				ordinance.DateSort = &t
			}
		}
		if place.Valid {
			ordinance.Place = place.String
		}
		if temple.Valid {
			ordinance.Temple = temple.String
		}
		if status.Valid {
			ordinance.Status = status.String
		}
		if t, err := parseTimestamp(updatedAtStr); err == nil {
			ordinance.UpdatedAt = t
		}
		ordinances = append(ordinances, ordinance)
	}

	return ordinances, total, rows.Err()
}

// ListLDSOrdinancesForPerson returns all LDS ordinances for a given person.
func (s *ReadModelStore) ListLDSOrdinancesForPerson(ctx context.Context, personID uuid.UUID) ([]repository.LDSOrdinanceReadModel, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, type, type_label, person_id, person_name, family_id,
		       date_raw, date_sort, place, temple, status, version, updated_at
		FROM lds_ordinances
		WHERE person_id = ?
		ORDER BY type, date_sort
	`, personID.String())
	if err != nil {
		return nil, fmt.Errorf("query lds_ordinances for person: %w", err)
	}
	defer rows.Close()

	var ordinances []repository.LDSOrdinanceReadModel
	for rows.Next() {
		var ordinance repository.LDSOrdinanceReadModel
		var idStr string
		var personIDNull, familyID sql.NullString
		var personName, dateRaw, place, temple, status sql.NullString
		var dateSort sql.NullString
		var updatedAtStr string
		if err := rows.Scan(
			&idStr,
			&ordinance.Type,
			&ordinance.TypeLabel,
			&personIDNull,
			&personName,
			&familyID,
			&dateRaw,
			&dateSort,
			&place,
			&temple,
			&status,
			&ordinance.Version,
			&updatedAtStr,
		); err != nil {
			return nil, fmt.Errorf("scan lds_ordinance: %w", err)
		}
		ordinance.ID, _ = uuid.Parse(idStr)
		if personIDNull.Valid {
			id, _ := uuid.Parse(personIDNull.String)
			ordinance.PersonID = &id
		}
		if familyID.Valid {
			id, _ := uuid.Parse(familyID.String)
			ordinance.FamilyID = &id
		}
		if personName.Valid {
			ordinance.PersonName = personName.String
		}
		if dateRaw.Valid {
			ordinance.DateRaw = dateRaw.String
		}
		if dateSort.Valid {
			if t, err := parseTimestamp(dateSort.String); err == nil {
				ordinance.DateSort = &t
			}
		}
		if place.Valid {
			ordinance.Place = place.String
		}
		if temple.Valid {
			ordinance.Temple = temple.String
		}
		if status.Valid {
			ordinance.Status = status.String
		}
		if t, err := parseTimestamp(updatedAtStr); err == nil {
			ordinance.UpdatedAt = t
		}
		ordinances = append(ordinances, ordinance)
	}

	return ordinances, rows.Err()
}

// ListLDSOrdinancesForFamily returns all LDS ordinances for a given family.
func (s *ReadModelStore) ListLDSOrdinancesForFamily(ctx context.Context, familyID uuid.UUID) ([]repository.LDSOrdinanceReadModel, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, type, type_label, person_id, person_name, family_id,
		       date_raw, date_sort, place, temple, status, version, updated_at
		FROM lds_ordinances
		WHERE family_id = ?
		ORDER BY type, date_sort
	`, familyID.String())
	if err != nil {
		return nil, fmt.Errorf("query lds_ordinances for family: %w", err)
	}
	defer rows.Close()

	var ordinances []repository.LDSOrdinanceReadModel
	for rows.Next() {
		var ordinance repository.LDSOrdinanceReadModel
		var idStr string
		var personID, familyIDNull sql.NullString
		var personName, dateRaw, place, temple, status sql.NullString
		var dateSort sql.NullString
		var updatedAtStr string
		if err := rows.Scan(
			&idStr,
			&ordinance.Type,
			&ordinance.TypeLabel,
			&personID,
			&personName,
			&familyIDNull,
			&dateRaw,
			&dateSort,
			&place,
			&temple,
			&status,
			&ordinance.Version,
			&updatedAtStr,
		); err != nil {
			return nil, fmt.Errorf("scan lds_ordinance: %w", err)
		}
		ordinance.ID, _ = uuid.Parse(idStr)
		if personID.Valid {
			id, _ := uuid.Parse(personID.String)
			ordinance.PersonID = &id
		}
		if familyIDNull.Valid {
			id, _ := uuid.Parse(familyIDNull.String)
			ordinance.FamilyID = &id
		}
		if personName.Valid {
			ordinance.PersonName = personName.String
		}
		if dateRaw.Valid {
			ordinance.DateRaw = dateRaw.String
		}
		if dateSort.Valid {
			if t, err := parseTimestamp(dateSort.String); err == nil {
				ordinance.DateSort = &t
			}
		}
		if place.Valid {
			ordinance.Place = place.String
		}
		if temple.Valid {
			ordinance.Temple = temple.String
		}
		if status.Valid {
			ordinance.Status = status.String
		}
		if t, err := parseTimestamp(updatedAtStr); err == nil {
			ordinance.UpdatedAt = t
		}
		ordinances = append(ordinances, ordinance)
	}

	return ordinances, rows.Err()
}

// SaveLDSOrdinance saves or updates an LDS ordinance.
func (s *ReadModelStore) SaveLDSOrdinance(ctx context.Context, ordinance *repository.LDSOrdinanceReadModel) error {
	var personID, familyID interface{}
	if ordinance.PersonID != nil {
		personID = ordinance.PersonID.String()
	}
	if ordinance.FamilyID != nil {
		familyID = ordinance.FamilyID.String()
	}

	var personName, dateRaw, dateSort, place, temple, status interface{}
	if ordinance.PersonName != "" {
		personName = ordinance.PersonName
	}
	if ordinance.DateRaw != "" {
		dateRaw = ordinance.DateRaw
	}
	if ordinance.DateSort != nil {
		dateSort = ordinance.DateSort.Format(time.RFC3339)
	}
	if ordinance.Place != "" {
		place = ordinance.Place
	}
	if ordinance.Temple != "" {
		temple = ordinance.Temple
	}
	if ordinance.Status != "" {
		status = ordinance.Status
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO lds_ordinances (id, type, type_label, person_id, person_name, family_id,
		                           date_raw, date_sort, place, temple, status, version, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET
			type = excluded.type,
			type_label = excluded.type_label,
			person_id = excluded.person_id,
			person_name = excluded.person_name,
			family_id = excluded.family_id,
			date_raw = excluded.date_raw,
			date_sort = excluded.date_sort,
			place = excluded.place,
			temple = excluded.temple,
			status = excluded.status,
			version = excluded.version,
			updated_at = excluded.updated_at
	`, ordinance.ID.String(), ordinance.Type, ordinance.TypeLabel, personID, personName, familyID,
		dateRaw, dateSort, place, temple, status, ordinance.Version, ordinance.UpdatedAt.Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("save lds_ordinance: %w", err)
	}
	return nil
}

// DeleteLDSOrdinance deletes an LDS ordinance by ID.
func (s *ReadModelStore) DeleteLDSOrdinance(ctx context.Context, id uuid.UUID) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM lds_ordinances WHERE id = ?", id.String())
	if err != nil {
		return fmt.Errorf("delete lds_ordinance: %w", err)
	}
	return nil
}
