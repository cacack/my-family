package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// ReadModelStore is a PostgreSQL implementation of repository.ReadModelStore.
type ReadModelStore struct {
	db *sql.DB
}

// NewReadModelStore creates a new PostgreSQL read model store.
func NewReadModelStore(db *sql.DB) (*ReadModelStore, error) {
	store := &ReadModelStore{db: db}
	if err := store.createTables(); err != nil {
		return nil, fmt.Errorf("failed to create tables: %w", err)
	}
	return store, nil
}

// createTables creates the read model schema if it doesn't exist.
func (s *ReadModelStore) createTables() error {
	_, err := s.db.Exec(`
		-- Enable pg_trgm extension for fuzzy search
		CREATE EXTENSION IF NOT EXISTS pg_trgm;

		-- Persons table
		CREATE TABLE IF NOT EXISTS persons (
			id UUID PRIMARY KEY,
			given_name VARCHAR(100) NOT NULL,
			surname VARCHAR(100) NOT NULL,
			full_name VARCHAR(200) GENERATED ALWAYS AS (given_name || ' ' || surname) STORED,
			gender VARCHAR(10),
			birth_date_raw VARCHAR(100),
			birth_date_sort DATE,
			birth_place VARCHAR(255),
			death_date_raw VARCHAR(100),
			death_date_sort DATE,
			death_place VARCHAR(255),
			notes TEXT,
			search_vector TSVECTOR,
			version BIGINT NOT NULL DEFAULT 1,
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);

		CREATE INDEX IF NOT EXISTS idx_persons_surname ON persons(surname, given_name);
		CREATE INDEX IF NOT EXISTS idx_persons_birth_date ON persons(birth_date_sort);
		CREATE INDEX IF NOT EXISTS idx_persons_search ON persons USING GIN(search_vector);
		CREATE INDEX IF NOT EXISTS idx_persons_surname_trgm ON persons USING GIN(surname gin_trgm_ops);
		CREATE INDEX IF NOT EXISTS idx_persons_given_name_trgm ON persons USING GIN(given_name gin_trgm_ops);

		-- Trigger to update search_vector
		CREATE OR REPLACE FUNCTION persons_search_trigger() RETURNS trigger AS $$
		BEGIN
			NEW.search_vector := to_tsvector('english', coalesce(NEW.given_name,'') || ' ' || coalesce(NEW.surname,''));
			RETURN NEW;
		END
		$$ LANGUAGE plpgsql;

		DROP TRIGGER IF EXISTS persons_search_update ON persons;
		CREATE TRIGGER persons_search_update BEFORE INSERT OR UPDATE ON persons
			FOR EACH ROW EXECUTE FUNCTION persons_search_trigger();

		-- Families table
		CREATE TABLE IF NOT EXISTS families (
			id UUID PRIMARY KEY,
			partner1_id UUID REFERENCES persons(id),
			partner1_name VARCHAR(200),
			partner2_id UUID REFERENCES persons(id),
			partner2_name VARCHAR(200),
			relationship_type VARCHAR(20),
			marriage_date_raw VARCHAR(100),
			marriage_date_sort DATE,
			marriage_place VARCHAR(255),
			child_count INTEGER NOT NULL DEFAULT 0,
			version BIGINT NOT NULL DEFAULT 1,
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);

		CREATE INDEX IF NOT EXISTS idx_families_partner1 ON families(partner1_id);
		CREATE INDEX IF NOT EXISTS idx_families_partner2 ON families(partner2_id);

		-- Family children table
		CREATE TABLE IF NOT EXISTS family_children (
			family_id UUID NOT NULL REFERENCES families(id) ON DELETE CASCADE,
			person_id UUID NOT NULL REFERENCES persons(id),
			person_name VARCHAR(200),
			relationship_type VARCHAR(20) NOT NULL DEFAULT 'biological',
			sequence INTEGER,
			PRIMARY KEY (family_id, person_id)
		);

		CREATE INDEX IF NOT EXISTS idx_family_children_person ON family_children(person_id);

		-- Pedigree edges table
		CREATE TABLE IF NOT EXISTS pedigree_edges (
			person_id UUID PRIMARY KEY REFERENCES persons(id) ON DELETE CASCADE,
			father_id UUID REFERENCES persons(id),
			mother_id UUID REFERENCES persons(id),
			father_name VARCHAR(200),
			mother_name VARCHAR(200)
		);

		CREATE INDEX IF NOT EXISTS idx_pedigree_father ON pedigree_edges(father_id);
		CREATE INDEX IF NOT EXISTS idx_pedigree_mother ON pedigree_edges(mother_id);
	`)
	return err
}

// GetPerson retrieves a person by ID.
func (s *ReadModelStore) GetPerson(ctx context.Context, id uuid.UUID) (*repository.PersonReadModel, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, given_name, surname, full_name, gender,
			   birth_date_raw, birth_date_sort, birth_place,
			   death_date_raw, death_date_sort, death_place,
			   notes, version, updated_at
		FROM persons WHERE id = $1
	`, id)

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
		ORDER BY %s %s NULLS LAST, given_name %s
		LIMIT $1 OFFSET $2
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

// SearchPersons searches for persons by name using tsvector and trigram similarity.
func (s *ReadModelStore) SearchPersons(ctx context.Context, query string, fuzzy bool, limit int) ([]repository.PersonReadModel, error) {
	var rows *sql.Rows
	var err error

	if fuzzy {
		// Use trigram similarity for fuzzy matching
		rows, err = s.db.QueryContext(ctx, `
			SELECT id, given_name, surname, full_name, gender,
				   birth_date_raw, birth_date_sort, birth_place,
				   death_date_raw, death_date_sort, death_place,
				   notes, version, updated_at
			FROM persons
			WHERE given_name % $1 OR surname % $1 OR full_name % $1
			ORDER BY GREATEST(
				similarity(given_name, $1),
				similarity(surname, $1),
				similarity(full_name, $1)
			) DESC
			LIMIT $2
		`, query, limit)
	} else {
		// Use full-text search with tsvector
		rows, err = s.db.QueryContext(ctx, `
			SELECT id, given_name, surname, full_name, gender,
				   birth_date_raw, birth_date_sort, birth_place,
				   death_date_raw, death_date_sort, death_place,
				   notes, version, updated_at
			FROM persons
			WHERE search_vector @@ plainto_tsquery('english', $1)
			   OR full_name ILIKE '%' || $1 || '%'
			ORDER BY ts_rank(search_vector, plainto_tsquery('english', $1)) DESC
			LIMIT $2
		`, query, limit)
	}

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
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO persons (id, given_name, surname, gender, birth_date_raw, birth_date_sort, birth_place,
							 death_date_raw, death_date_sort, death_place, notes, version, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		ON CONFLICT(id) DO UPDATE SET
			given_name = EXCLUDED.given_name,
			surname = EXCLUDED.surname,
			gender = EXCLUDED.gender,
			birth_date_raw = EXCLUDED.birth_date_raw,
			birth_date_sort = EXCLUDED.birth_date_sort,
			birth_place = EXCLUDED.birth_place,
			death_date_raw = EXCLUDED.death_date_raw,
			death_date_sort = EXCLUDED.death_date_sort,
			death_place = EXCLUDED.death_place,
			notes = EXCLUDED.notes,
			version = EXCLUDED.version,
			updated_at = EXCLUDED.updated_at
	`, person.ID, person.GivenName, person.Surname, nullableGender(person.Gender),
		nullableString(person.BirthDateRaw), nullableTime(person.BirthDateSort), nullableString(person.BirthPlace),
		nullableString(person.DeathDateRaw), nullableTime(person.DeathDateSort), nullableString(person.DeathPlace),
		nullableString(person.Notes), person.Version, person.UpdatedAt)

	return err
}

// DeletePerson removes a person.
func (s *ReadModelStore) DeletePerson(ctx context.Context, id uuid.UUID) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM persons WHERE id = $1", id)
	return err
}

// GetFamily retrieves a family by ID.
func (s *ReadModelStore) GetFamily(ctx context.Context, id uuid.UUID) (*repository.FamilyReadModel, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, partner1_id, partner1_name, partner2_id, partner2_name,
			   relationship_type, marriage_date_raw, marriage_date_sort, marriage_place,
			   child_count, version, updated_at
		FROM families WHERE id = $1
	`, id)

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
		LIMIT $1 OFFSET $2
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
		WHERE partner1_id = $1 OR partner2_id = $1
	`, personID)
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
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO families (id, partner1_id, partner1_name, partner2_id, partner2_name,
							  relationship_type, marriage_date_raw, marriage_date_sort, marriage_place,
							  child_count, version, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		ON CONFLICT(id) DO UPDATE SET
			partner1_id = EXCLUDED.partner1_id,
			partner1_name = EXCLUDED.partner1_name,
			partner2_id = EXCLUDED.partner2_id,
			partner2_name = EXCLUDED.partner2_name,
			relationship_type = EXCLUDED.relationship_type,
			marriage_date_raw = EXCLUDED.marriage_date_raw,
			marriage_date_sort = EXCLUDED.marriage_date_sort,
			marriage_place = EXCLUDED.marriage_place,
			child_count = EXCLUDED.child_count,
			version = EXCLUDED.version,
			updated_at = EXCLUDED.updated_at
	`, family.ID, nullableUUID(family.Partner1ID), nullableString(family.Partner1Name),
		nullableUUID(family.Partner2ID), nullableString(family.Partner2Name),
		nullableString(string(family.RelationshipType)), nullableString(family.MarriageDateRaw),
		nullableTime(family.MarriageDateSort), nullableString(family.MarriagePlace),
		family.ChildCount, family.Version, family.UpdatedAt)

	return err
}

// DeleteFamily removes a family.
func (s *ReadModelStore) DeleteFamily(ctx context.Context, id uuid.UUID) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM families WHERE id = $1", id)
	return err
}

// GetFamilyChildren returns all children for a family.
func (s *ReadModelStore) GetFamilyChildren(ctx context.Context, familyID uuid.UUID) ([]repository.FamilyChildReadModel, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT family_id, person_id, person_name, relationship_type, sequence
		FROM family_children
		WHERE family_id = $1
		ORDER BY sequence NULLS LAST, person_name
	`, familyID)
	if err != nil {
		return nil, fmt.Errorf("query family children: %w", err)
	}
	defer rows.Close()

	var children []repository.FamilyChildReadModel
	for rows.Next() {
		var (
			familyID, personID  uuid.UUID
			personName, relType string
			sequence            sql.NullInt64
		)
		err := rows.Scan(&familyID, &personID, &personName, &relType, &sequence)
		if err != nil {
			return nil, fmt.Errorf("scan family child: %w", err)
		}

		child := repository.FamilyChildReadModel{
			FamilyID:         familyID,
			PersonID:         personID,
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
		WHERE fc.family_id = $1
		ORDER BY fc.sequence NULLS LAST, p.given_name
	`, familyID)
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
		WHERE fc.person_id = $1
	`, personID)

	return scanFamily(row)
}

// SaveFamilyChild saves a family child relationship.
func (s *ReadModelStore) SaveFamilyChild(ctx context.Context, child *repository.FamilyChildReadModel) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO family_children (family_id, person_id, person_name, relationship_type, sequence)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT(family_id, person_id) DO UPDATE SET
			person_name = EXCLUDED.person_name,
			relationship_type = EXCLUDED.relationship_type,
			sequence = EXCLUDED.sequence
	`, child.FamilyID, child.PersonID, child.PersonName, string(child.RelationshipType), nullableInt(child.Sequence))

	return err
}

// DeleteFamilyChild removes a family child relationship.
func (s *ReadModelStore) DeleteFamilyChild(ctx context.Context, familyID, personID uuid.UUID) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM family_children WHERE family_id = $1 AND person_id = $2",
		familyID, personID)
	return err
}

// GetPedigreeEdge returns the pedigree edge for a person.
func (s *ReadModelStore) GetPedigreeEdge(ctx context.Context, personID uuid.UUID) (*repository.PedigreeEdge, error) {
	var (
		pID                    uuid.UUID
		fatherID, motherID     sql.NullString
		fatherName, motherName sql.NullString
	)

	err := s.db.QueryRowContext(ctx, `
		SELECT person_id, father_id, mother_id, father_name, mother_name
		FROM pedigree_edges
		WHERE person_id = $1
	`, personID).Scan(&pID, &fatherID, &motherID, &fatherName, &motherName)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("query pedigree edge: %w", err)
	}

	edge := &repository.PedigreeEdge{
		PersonID:   pID,
		FatherName: fatherName.String,
		MotherName: motherName.String,
	}

	if fatherID.Valid {
		fID, _ := uuid.Parse(fatherID.String)
		edge.FatherID = &fID
	}
	if motherID.Valid {
		mID, _ := uuid.Parse(motherID.String)
		edge.MotherID = &mID
	}

	return edge, nil
}

// SavePedigreeEdge saves a pedigree edge.
func (s *ReadModelStore) SavePedigreeEdge(ctx context.Context, edge *repository.PedigreeEdge) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO pedigree_edges (person_id, father_id, mother_id, father_name, mother_name)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT(person_id) DO UPDATE SET
			father_id = EXCLUDED.father_id,
			mother_id = EXCLUDED.mother_id,
			father_name = EXCLUDED.father_name,
			mother_name = EXCLUDED.mother_name
	`, edge.PersonID, nullableUUID(edge.FatherID), nullableUUID(edge.MotherID),
		nullableString(edge.FatherName), nullableString(edge.MotherName))

	return err
}

// DeletePedigreeEdge removes a pedigree edge.
func (s *ReadModelStore) DeletePedigreeEdge(ctx context.Context, personID uuid.UUID) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM pedigree_edges WHERE person_id = $1", personID)
	return err
}

// Helper functions

type rowScanner interface {
	Scan(dest ...any) error
}

func scanPerson(row rowScanner) (*repository.PersonReadModel, error) {
	var (
		id                               uuid.UUID
		givenName, surname, fullName     string
		gender, birthDateRaw, birthPlace sql.NullString
		deathDateRaw, deathPlace, notes  sql.NullString
		birthDateSort, deathDateSort     sql.NullTime
		version                          int64
		updatedAt                        time.Time
	)

	err := row.Scan(&id, &givenName, &surname, &fullName, &gender,
		&birthDateRaw, &birthDateSort, &birthPlace,
		&deathDateRaw, &deathDateSort, &deathPlace,
		&notes, &version, &updatedAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan person: %w", err)
	}

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
		UpdatedAt:    updatedAt,
	}

	if birthDateSort.Valid {
		p.BirthDateSort = &birthDateSort.Time
	}
	if deathDateSort.Valid {
		p.DeathDateSort = &deathDateSort.Time
	}

	return p, nil
}

func scanPersonRow(rows *sql.Rows) (*repository.PersonReadModel, error) {
	return scanPerson(rows)
}

func scanFamily(row rowScanner) (*repository.FamilyReadModel, error) {
	var (
		id                                      uuid.UUID
		partner1ID, partner2ID                  sql.NullString
		partner1Name, partner2Name              sql.NullString
		relType, marriageDateRaw, marriagePlace sql.NullString
		marriageDateSort                        sql.NullTime
		childCount                              int
		version                                 int64
		updatedAt                               time.Time
	)

	err := row.Scan(&id, &partner1ID, &partner1Name, &partner2ID, &partner2Name,
		&relType, &marriageDateRaw, &marriageDateSort, &marriagePlace,
		&childCount, &version, &updatedAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan family: %w", err)
	}

	f := &repository.FamilyReadModel{
		ID:               id,
		Partner1Name:     partner1Name.String,
		Partner2Name:     partner2Name.String,
		RelationshipType: domain.RelationType(relType.String),
		MarriageDateRaw:  marriageDateRaw.String,
		MarriagePlace:    marriagePlace.String,
		ChildCount:       childCount,
		Version:          version,
		UpdatedAt:        updatedAt,
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
		f.MarriageDateSort = &marriageDateSort.Time
	}

	return f, nil
}

func scanFamilyRow(rows *sql.Rows) (*repository.FamilyReadModel, error) {
	return scanFamily(rows)
}

func nullableString(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}

func nullableGender(g domain.Gender) sql.NullString {
	if g == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: string(g), Valid: true}
}

func nullableTime(t *time.Time) sql.NullTime {
	if t == nil {
		return sql.NullTime{}
	}
	return sql.NullTime{Time: *t, Valid: true}
}

func nullableUUID(id *uuid.UUID) sql.NullString {
	if id == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: id.String(), Valid: true}
}

func nullableInt(i *int) sql.NullInt64 {
	if i == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: int64(*i), Valid: true}
}
