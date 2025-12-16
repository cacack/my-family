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

	// Create triggers to keep FTS in sync
	s.db.Exec(`
		CREATE TRIGGER IF NOT EXISTS persons_fts_insert AFTER INSERT ON persons BEGIN
			INSERT INTO persons_fts(rowid, given_name, surname)
			SELECT rowid, NEW.given_name, NEW.surname FROM persons WHERE id = NEW.id;
		END
	`)

	s.db.Exec(`
		CREATE TRIGGER IF NOT EXISTS persons_fts_delete AFTER DELETE ON persons BEGIN
			INSERT INTO persons_fts(persons_fts, rowid, given_name, surname)
			VALUES('delete', OLD.rowid, OLD.given_name, OLD.surname);
		END
	`)

	s.db.Exec(`
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
		ftsQuery = ftsQuery + "*"
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
