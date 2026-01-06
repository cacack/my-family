package sqlite

import (
	"context"
	"database/sql"
	"fmt"
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
			version INTEGER NOT NULL DEFAULT 1,
			updated_at TEXT NOT NULL DEFAULT (datetime('now'))
		);

		CREATE INDEX IF NOT EXISTS idx_persons_surname ON persons(surname, given_name);
		CREATE INDEX IF NOT EXISTS idx_persons_birth_date ON persons(birth_date_sort);
		CREATE INDEX IF NOT EXISTS idx_persons_full_name ON persons(full_name);

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
			updated_at TEXT NOT NULL DEFAULT (datetime('now'))
		);

		CREATE INDEX IF NOT EXISTS idx_media_entity ON media(entity_type, entity_id);
		CREATE INDEX IF NOT EXISTS idx_media_type ON media(media_type);
	`)
	if err != nil {
		return err
	}

	// Try to create FTS5 table (optional - falls back to LIKE if not available)
	s.tryCreateFTS5()

	return nil
}

// tryCreateFTS5 attempts to create FTS5 virtual table for full-text search.
// If FTS5 is not available, search will fall back to LIKE-based queries.
func (s *ReadModelStore) tryCreateFTS5() {
	// Try to create FTS5 virtual table
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
}

// GetPerson retrieves a person by ID.
func (s *ReadModelStore) GetPerson(ctx context.Context, id uuid.UUID) (*repository.PersonReadModel, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, given_name, surname, full_name, gender,
			   birth_date_raw, birth_date_sort, birth_place,
			   death_date_raw, death_date_sort, death_place,
			   notes, version, updated_at
		FROM persons WHERE id = ?
	`, id.String())

	return scanPerson(row)
}

// ListPersons returns a paginated list of persons.
func (s *ReadModelStore) ListPersons(ctx context.Context, opts repository.ListOptions) ([]repository.PersonReadModel, int, error) {
	// Count total
	var total int
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM persons").Scan(&total)
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

	// #nosec G201 -- orderColumn and orderDir are validated via switch/if above, not user input
	query := fmt.Sprintf(`
		SELECT id, given_name, surname, full_name, gender,
			   birth_date_raw, birth_date_sort, birth_place,
			   death_date_raw, death_date_sort, death_place,
			   notes, version, updated_at
		FROM persons
		ORDER BY %s %s, given_name %s
		LIMIT ? OFFSET ?
	`, orderColumn, orderDir, orderDir)

	rows, err := s.db.QueryContext(ctx, query, opts.Limit, opts.Offset)
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

// SearchPersons searches for persons by name using FTS5.
func (s *ReadModelStore) SearchPersons(ctx context.Context, query string, fuzzy bool, limit int) ([]repository.PersonReadModel, error) {
	// Escape and prepare FTS5 query
	ftsQuery := escapeFTS5Query(query)

	// For fuzzy matching, use prefix matching
	if fuzzy {
		ftsQuery += "*"
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT p.id, p.given_name, p.surname, p.full_name, p.gender,
			   p.birth_date_raw, p.birth_date_sort, p.birth_place,
			   p.death_date_raw, p.death_date_sort, p.death_place,
			   p.notes, p.version, p.updated_at
		FROM persons p
		JOIN persons_fts fts ON p.rowid = fts.rowid
		WHERE persons_fts MATCH ?
		ORDER BY rank
		LIMIT ?
	`, ftsQuery, limit)
	if err != nil {
		// Fallback to LIKE if FTS5 query fails (e.g., for special characters)
		return s.searchPersonsLike(ctx, query, limit)
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

	// If no FTS results and fuzzy, try LIKE fallback
	if len(persons) == 0 && fuzzy {
		return s.searchPersonsLike(ctx, query, limit)
	}

	return persons, rows.Err()
}

// searchPersonsLike is a fallback search using LIKE.
func (s *ReadModelStore) searchPersonsLike(ctx context.Context, query string, limit int) ([]repository.PersonReadModel, error) {
	likeQuery := "%" + strings.ToLower(query) + "%"

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, given_name, surname, full_name, gender,
			   birth_date_raw, birth_date_sort, birth_place,
			   death_date_raw, death_date_sort, death_place,
			   notes, version, updated_at
		FROM persons
		WHERE LOWER(full_name) LIKE ? OR LOWER(given_name) LIKE ? OR LOWER(surname) LIKE ?
		LIMIT ?
	`, likeQuery, likeQuery, likeQuery, limit)
	if err != nil {
		return nil, fmt.Errorf("search persons: %w", err)
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

// SavePerson saves or updates a person.
func (s *ReadModelStore) SavePerson(ctx context.Context, person *repository.PersonReadModel) error {
	var birthDateSort, deathDateSort sql.NullString
	if person.BirthDateSort != nil {
		birthDateSort = sql.NullString{String: person.BirthDateSort.Format("2006-01-02"), Valid: true}
	}
	if person.DeathDateSort != nil {
		deathDateSort = sql.NullString{String: person.DeathDateSort.Format("2006-01-02"), Valid: true}
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO persons (id, given_name, surname, gender, birth_date_raw, birth_date_sort, birth_place,
							 death_date_raw, death_date_sort, death_place, notes, version, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			given_name = excluded.given_name,
			surname = excluded.surname,
			gender = excluded.gender,
			birth_date_raw = excluded.birth_date_raw,
			birth_date_sort = excluded.birth_date_sort,
			birth_place = excluded.birth_place,
			death_date_raw = excluded.death_date_raw,
			death_date_sort = excluded.death_date_sort,
			death_place = excluded.death_place,
			notes = excluded.notes,
			version = excluded.version,
			updated_at = excluded.updated_at
	`, person.ID.String(), person.GivenName, person.Surname, string(person.Gender),
		person.BirthDateRaw, birthDateSort, person.BirthPlace,
		person.DeathDateRaw, deathDateSort, person.DeathPlace,
		person.Notes, person.Version, formatTimestamp(person.UpdatedAt))

	return err
}

// DeletePerson removes a person.
func (s *ReadModelStore) DeletePerson(ctx context.Context, id uuid.UUID) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM persons WHERE id = ?", id.String())
	return err
}

// GetFamily retrieves a family by ID.
func (s *ReadModelStore) GetFamily(ctx context.Context, id uuid.UUID) (*repository.FamilyReadModel, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, partner1_id, partner1_name, partner2_id, partner2_name,
			   relationship_type, marriage_date_raw, marriage_date_sort, marriage_place,
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

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO families (id, partner1_id, partner1_name, partner2_id, partner2_name,
							  relationship_type, marriage_date_raw, marriage_date_sort, marriage_place,
							  child_count, version, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			partner1_id = excluded.partner1_id,
			partner1_name = excluded.partner1_name,
			partner2_id = excluded.partner2_id,
			partner2_name = excluded.partner2_name,
			relationship_type = excluded.relationship_type,
			marriage_date_raw = excluded.marriage_date_raw,
			marriage_date_sort = excluded.marriage_date_sort,
			marriage_place = excluded.marriage_place,
			child_count = excluded.child_count,
			version = excluded.version,
			updated_at = excluded.updated_at
	`, family.ID.String(), partner1ID, family.Partner1Name, partner2ID, family.Partner2Name,
		string(family.RelationshipType), family.MarriageDateRaw, marriageDateSort, family.MarriagePlace,
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
			   p.birth_date_raw, p.birth_date_sort, p.birth_place,
			   p.death_date_raw, p.death_date_sort, p.death_place,
			   p.notes, p.version, p.updated_at
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
		deathDateRaw, deathDateSort, deathPlace, notes  sql.NullString
		version                                         int64
		updatedAt                                       string
	)

	err := row.Scan(&idStr, &givenName, &surname, &fullName, &gender,
		&birthDateRaw, &birthDateSort, &birthPlace,
		&deathDateRaw, &deathDateSort, &deathPlace,
		&notes, &version, &updatedAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan person: %w", err)
	}

	id, _ := uuid.Parse(idStr)
	p := &repository.PersonReadModel{
		ID:           id,
		GivenName:    givenName,
		Surname:      surname,
		FullName:     fullName,
		Gender:       domain.Gender(gender.String),
		BirthDateRaw: birthDateRaw.String,
		BirthPlace:   birthPlace.String,
		DeathDateRaw: deathDateRaw.String,
		DeathPlace:   deathPlace.String,
		Notes:        notes.String,
		Version:      version,
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
		childCount                                                int
		version                                                   int64
		updatedAt                                                 string
	)

	err := row.Scan(&idStr, &partner1ID, &partner1Name, &partner2ID, &partner2Name,
		&relType, &marriageDateRaw, &marriageDateSort, &marriagePlace,
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

// GetMedia retrieves media metadata by ID (excludes FileData and ThumbnailData).
func (s *ReadModelStore) GetMedia(ctx context.Context, id uuid.UUID) (*repository.MediaReadModel, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, entity_type, entity_id, title, description, mime_type, media_type,
			   filename, file_size, crop_left, crop_top, crop_width, crop_height,
			   gedcom_xref, version, created_at, updated_at
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
			   gedcom_xref, version, created_at, updated_at
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
			   gedcom_xref, version, created_at, updated_at
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
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO media (id, entity_type, entity_id, title, description, mime_type, media_type,
						  filename, file_size, file_data, thumbnail_data,
						  crop_left, crop_top, crop_width, crop_height,
						  gedcom_xref, version, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
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
			updated_at = excluded.updated_at
	`, media.ID.String(), media.EntityType, media.EntityID.String(), media.Title,
		nullableString(media.Description), media.MimeType, string(media.MediaType),
		media.Filename, media.FileSize, media.FileData, media.ThumbnailData,
		nullableInt(media.CropLeft), nullableInt(media.CropTop),
		nullableInt(media.CropWidth), nullableInt(media.CropHeight),
		nullableString(media.GedcomXref), media.Version,
		formatTimestamp(media.CreatedAt), formatTimestamp(media.UpdatedAt))

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
	)

	err := row.Scan(&idStr, &entityType, &entityIDStr, &title, &description,
		&mimeType, &mediaType, &filename, &fileSize,
		&cropLeft, &cropTop, &cropWidth, &cropHeight,
		&gedcomXref, &version, &createdAt, &updatedAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan media metadata: %w", err)
	}

	id, _ := uuid.Parse(idStr)
	entityID, _ := uuid.Parse(entityIDStr)

	m := &repository.MediaReadModel{
		ID:          id,
		EntityType:  entityType,
		EntityID:    entityID,
		Title:       title,
		Description: description.String,
		MimeType:    mimeType,
		MediaType:   domain.MediaType(mediaType),
		Filename:    filename,
		FileSize:    fileSize,
		GedcomXref:  gedcomXref.String,
		Version:     version,
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
	)

	err := row.Scan(&idStr, &entityType, &entityIDStr, &title, &description,
		&mimeType, &mediaType, &filename, &fileSize, &fileData, &thumbnailData,
		&cropLeft, &cropTop, &cropWidth, &cropHeight,
		&gedcomXref, &version, &createdAt, &updatedAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan media full: %w", err)
	}

	id, _ := uuid.Parse(idStr)
	entityID, _ := uuid.Parse(entityIDStr)

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
			   birth_date_raw, birth_date_sort, birth_place,
			   death_date_raw, death_date_sort, death_place,
			   notes, version, updated_at
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
			   birth_date_raw, birth_date_sort, birth_place,
			   death_date_raw, death_date_sort, death_place,
			   notes, version, updated_at
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
