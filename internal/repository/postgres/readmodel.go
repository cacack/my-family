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
			research_status VARCHAR(20),
			search_vector TSVECTOR,
			version BIGINT NOT NULL DEFAULT 1,
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);

		CREATE INDEX IF NOT EXISTS idx_persons_surname ON persons(surname, given_name);
		CREATE INDEX IF NOT EXISTS idx_persons_birth_date ON persons(birth_date_sort);
		CREATE INDEX IF NOT EXISTS idx_persons_search ON persons USING GIN(search_vector);
		CREATE INDEX IF NOT EXISTS idx_persons_surname_trgm ON persons USING GIN(surname gin_trgm_ops);
		CREATE INDEX IF NOT EXISTS idx_persons_given_name_trgm ON persons USING GIN(given_name gin_trgm_ops);
		CREATE INDEX IF NOT EXISTS idx_persons_research_status ON persons(research_status);

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

		-- Sources table
		CREATE TABLE IF NOT EXISTS sources (
			id UUID PRIMARY KEY,
			source_type VARCHAR(50) NOT NULL,
			title VARCHAR(500) NOT NULL,
			author VARCHAR(200),
			publisher VARCHAR(200),
			publish_date_raw VARCHAR(100),
			publish_date_sort DATE,
			url VARCHAR(500),
			repository_name VARCHAR(200),
			collection_name VARCHAR(200),
			call_number VARCHAR(100),
			notes TEXT,
			gedcom_xref VARCHAR(50),
			citation_count INTEGER NOT NULL DEFAULT 0,
			version BIGINT NOT NULL DEFAULT 1,
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);

		CREATE INDEX IF NOT EXISTS idx_sources_title ON sources(title);
		CREATE INDEX IF NOT EXISTS idx_sources_type ON sources(source_type);

		-- Citations table
		CREATE TABLE IF NOT EXISTS citations (
			id UUID PRIMARY KEY,
			source_id UUID NOT NULL REFERENCES sources(id),
			source_title VARCHAR(500),
			fact_type VARCHAR(100) NOT NULL,
			fact_owner_id UUID NOT NULL,
			page VARCHAR(100),
			volume VARCHAR(50),
			source_quality VARCHAR(20),
			informant_type VARCHAR(20),
			evidence_type VARCHAR(20),
			quoted_text TEXT,
			analysis TEXT,
			template_id VARCHAR(100),
			gedcom_xref VARCHAR(50),
			version BIGINT NOT NULL DEFAULT 1,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);

		CREATE INDEX IF NOT EXISTS idx_citations_source ON citations(source_id);
		CREATE INDEX IF NOT EXISTS idx_citations_fact ON citations(fact_type, fact_owner_id);
		CREATE INDEX IF NOT EXISTS idx_citations_owner ON citations(fact_owner_id);

		-- Media table
		CREATE TABLE IF NOT EXISTS media (
			id UUID PRIMARY KEY,
			entity_type VARCHAR(20) NOT NULL,
			entity_id UUID NOT NULL,
			title VARCHAR(500) NOT NULL,
			description TEXT,
			mime_type VARCHAR(100) NOT NULL,
			media_type VARCHAR(20) NOT NULL,
			filename VARCHAR(255) NOT NULL,
			file_size BIGINT NOT NULL,
			file_data BYTEA NOT NULL,
			thumbnail_data BYTEA,
			crop_left INTEGER,
			crop_top INTEGER,
			crop_width INTEGER,
			crop_height INTEGER,
			gedcom_xref VARCHAR(50),
			version BIGINT NOT NULL DEFAULT 1,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);

		CREATE INDEX IF NOT EXISTS idx_media_entity ON media(entity_type, entity_id);
		CREATE INDEX IF NOT EXISTS idx_media_type ON media(media_type);

		-- Person names table (for multiple name variants)
		CREATE TABLE IF NOT EXISTS person_names (
			id UUID PRIMARY KEY,
			person_id UUID NOT NULL REFERENCES persons(id) ON DELETE CASCADE,
			given_name VARCHAR(100) NOT NULL,
			surname VARCHAR(100) NOT NULL,
			full_name VARCHAR(200) GENERATED ALWAYS AS (given_name || ' ' || surname) STORED,
			name_prefix VARCHAR(50),
			name_suffix VARCHAR(50),
			surname_prefix VARCHAR(50),
			nickname VARCHAR(100),
			name_type VARCHAR(20) NOT NULL DEFAULT '',
			is_primary BOOLEAN NOT NULL DEFAULT FALSE,
			search_vector TSVECTOR,
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);

		CREATE INDEX IF NOT EXISTS idx_person_names_person ON person_names(person_id);
		CREATE INDEX IF NOT EXISTS idx_person_names_primary ON person_names(person_id, is_primary);
		CREATE INDEX IF NOT EXISTS idx_person_names_search ON person_names USING GIN(search_vector);
		CREATE INDEX IF NOT EXISTS idx_person_names_given_trgm ON person_names USING GIN(given_name gin_trgm_ops);
		CREATE INDEX IF NOT EXISTS idx_person_names_surname_trgm ON person_names USING GIN(surname gin_trgm_ops);

		-- Trigger to update search_vector for person_names
		CREATE OR REPLACE FUNCTION person_names_search_trigger() RETURNS trigger AS $$
		BEGIN
			NEW.search_vector := to_tsvector('english',
				coalesce(NEW.given_name,'') || ' ' ||
				coalesce(NEW.surname,'') || ' ' ||
				coalesce(NEW.nickname,''));
			RETURN NEW;
		END
		$$ LANGUAGE plpgsql;

		DROP TRIGGER IF EXISTS person_names_search_update ON person_names;
		CREATE TRIGGER person_names_search_update BEFORE INSERT OR UPDATE ON person_names
			FOR EACH ROW EXECUTE FUNCTION person_names_search_trigger();
	`)
	if err != nil {
		return err
	}

	// Run schema migrations for existing databases
	s.runMigrations()

	return nil
}

// runMigrations applies schema changes for existing databases.
func (s *ReadModelStore) runMigrations() {
	// Add research_status column if it doesn't exist (for databases created before this column was added)
	_, _ = s.db.Exec(`ALTER TABLE persons ADD COLUMN IF NOT EXISTS research_status VARCHAR(20)`)
	_, _ = s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_persons_research_status ON persons(research_status)`)
}

// GetPerson retrieves a person by ID.
func (s *ReadModelStore) GetPerson(ctx context.Context, id uuid.UUID) (*repository.PersonReadModel, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, given_name, surname, full_name, gender,
			   birth_date_raw, birth_date_sort, birth_place,
			   death_date_raw, death_date_sort, death_place,
			   notes, research_status, version, updated_at
		FROM persons WHERE id = $1
	`, id)

	return scanPerson(row)
}

// ListPersons returns a paginated list of persons.
func (s *ReadModelStore) ListPersons(ctx context.Context, opts repository.ListOptions) ([]repository.PersonReadModel, int, error) {
	// Build WHERE clause for research_status filter
	whereClause := ""
	var whereArgs []any
	paramNum := 1
	if opts.ResearchStatus != nil {
		if *opts.ResearchStatus == "unset" {
			whereClause = "WHERE research_status IS NULL OR research_status = ''"
		} else {
			whereClause = fmt.Sprintf("WHERE research_status = $%d", paramNum)
			whereArgs = append(whereArgs, *opts.ResearchStatus)
			paramNum++
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
			   birth_date_raw, birth_date_sort, birth_place,
			   death_date_raw, death_date_sort, death_place,
			   notes, research_status, version, updated_at
		FROM persons
		%s
		ORDER BY %s %s NULLS LAST, given_name %s
		LIMIT $%d OFFSET $%d
	`, whereClause, orderColumn, orderDir, orderDir, paramNum, paramNum+1)

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

// SearchPersons searches for persons by name using tsvector and trigram similarity.
// Also searches in person_names table for alternate names.
func (s *ReadModelStore) SearchPersons(ctx context.Context, query string, fuzzy bool, limit int) ([]repository.PersonReadModel, error) {
	var rows *sql.Rows
	var err error

	if fuzzy {
		// Use trigram similarity for fuzzy matching across both tables
		rows, err = s.db.QueryContext(ctx, `
			WITH matched_persons AS (
				-- Match in main persons table
				SELECT p.id, p.given_name, p.surname, p.full_name, p.gender,
					   p.birth_date_raw, p.birth_date_sort, p.birth_place,
					   p.death_date_raw, p.death_date_sort, p.death_place,
					   p.notes, p.research_status, p.version, p.updated_at,
					   TRUE as is_primary,
					   GREATEST(
						   similarity(p.given_name, $1),
						   similarity(p.surname, $1),
						   similarity(p.full_name, $1)
					   ) as sim_score
				FROM persons p
				WHERE p.given_name % $1 OR p.surname % $1 OR p.full_name % $1

				UNION

				-- Match in person_names table
				SELECT p.id, p.given_name, p.surname, p.full_name, p.gender,
					   p.birth_date_raw, p.birth_date_sort, p.birth_place,
					   p.death_date_raw, p.death_date_sort, p.death_place,
					   p.notes, p.research_status, p.version, p.updated_at,
					   pn.is_primary,
					   GREATEST(
						   similarity(pn.given_name, $1),
						   similarity(pn.surname, $1),
						   similarity(pn.full_name, $1),
						   similarity(COALESCE(pn.nickname, ''), $1)
					   ) as sim_score
				FROM persons p
				JOIN person_names pn ON p.id = pn.person_id
				WHERE pn.given_name % $1 OR pn.surname % $1 OR pn.full_name % $1
				   OR pn.nickname % $1
			)
			SELECT DISTINCT ON (id) id, given_name, surname, full_name, gender,
				   birth_date_raw, birth_date_sort, birth_place,
				   death_date_raw, death_date_sort, death_place,
				   notes, research_status, version, updated_at
			FROM matched_persons
			ORDER BY id, is_primary DESC, sim_score DESC
			LIMIT $2
		`, query, limit)
	} else {
		// Use full-text search with tsvector across both tables
		rows, err = s.db.QueryContext(ctx, `
			WITH matched_persons AS (
				-- Match in main persons table
				SELECT p.id, p.given_name, p.surname, p.full_name, p.gender,
					   p.birth_date_raw, p.birth_date_sort, p.birth_place,
					   p.death_date_raw, p.death_date_sort, p.death_place,
					   p.notes, p.research_status, p.version, p.updated_at,
					   TRUE as is_primary,
					   ts_rank(p.search_vector, plainto_tsquery('english', $1)) as search_rank
				FROM persons p
				WHERE p.search_vector @@ plainto_tsquery('english', $1)
				   OR p.full_name ILIKE '%' || $1 || '%'

				UNION

				-- Match in person_names table
				SELECT p.id, p.given_name, p.surname, p.full_name, p.gender,
					   p.birth_date_raw, p.birth_date_sort, p.birth_place,
					   p.death_date_raw, p.death_date_sort, p.death_place,
					   p.notes, p.research_status, p.version, p.updated_at,
					   pn.is_primary,
					   ts_rank(pn.search_vector, plainto_tsquery('english', $1)) as search_rank
				FROM persons p
				JOIN person_names pn ON p.id = pn.person_id
				WHERE pn.search_vector @@ plainto_tsquery('english', $1)
				   OR pn.full_name ILIKE '%' || $1 || '%'
				   OR pn.nickname ILIKE '%' || $1 || '%'
			)
			SELECT DISTINCT ON (id) id, given_name, surname, full_name, gender,
				   birth_date_raw, birth_date_sort, birth_place,
				   death_date_raw, death_date_sort, death_place,
				   notes, research_status, version, updated_at
			FROM matched_persons
			ORDER BY id, is_primary DESC, search_rank DESC
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
							 death_date_raw, death_date_sort, death_place, notes, research_status, version, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
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
			research_status = EXCLUDED.research_status,
			version = EXCLUDED.version,
			updated_at = EXCLUDED.updated_at
	`, person.ID, person.GivenName, person.Surname, nullableGender(person.Gender),
		nullableString(person.BirthDateRaw), nullableTime(person.BirthDateSort), nullableString(person.BirthPlace),
		nullableString(person.DeathDateRaw), nullableTime(person.DeathDateSort), nullableString(person.DeathPlace),
		nullableString(person.Notes), nullableString(string(person.ResearchStatus)), person.Version, person.UpdatedAt)

	return err
}

// DeletePerson removes a person.
func (s *ReadModelStore) DeletePerson(ctx context.Context, id uuid.UUID) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM persons WHERE id = $1", id)
	return err
}

// SavePersonName saves or updates a person name variant.
func (s *ReadModelStore) SavePersonName(ctx context.Context, name *repository.PersonNameReadModel) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO person_names (id, person_id, given_name, surname, name_prefix, name_suffix,
								  surname_prefix, nickname, name_type, is_primary, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		ON CONFLICT(id) DO UPDATE SET
			person_id = EXCLUDED.person_id,
			given_name = EXCLUDED.given_name,
			surname = EXCLUDED.surname,
			name_prefix = EXCLUDED.name_prefix,
			name_suffix = EXCLUDED.name_suffix,
			surname_prefix = EXCLUDED.surname_prefix,
			nickname = EXCLUDED.nickname,
			name_type = EXCLUDED.name_type,
			is_primary = EXCLUDED.is_primary,
			updated_at = EXCLUDED.updated_at
	`, name.ID, name.PersonID, name.GivenName, name.Surname,
		nullableString(name.NamePrefix), nullableString(name.NameSuffix),
		nullableString(name.SurnamePrefix), nullableString(name.Nickname),
		nullableString(string(name.NameType)), name.IsPrimary, name.UpdatedAt)

	return err
}

// GetPersonName retrieves a person name by ID.
func (s *ReadModelStore) GetPersonName(ctx context.Context, nameID uuid.UUID) (*repository.PersonNameReadModel, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, person_id, given_name, surname, full_name, name_prefix, name_suffix,
			   surname_prefix, nickname, name_type, is_primary, updated_at
		FROM person_names WHERE id = $1
	`, nameID)

	return scanPersonName(row)
}

// GetPersonNames retrieves all name variants for a person.
func (s *ReadModelStore) GetPersonNames(ctx context.Context, personID uuid.UUID) ([]repository.PersonNameReadModel, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, person_id, given_name, surname, full_name, name_prefix, name_suffix,
			   surname_prefix, nickname, name_type, is_primary, updated_at
		FROM person_names
		WHERE person_id = $1
		ORDER BY is_primary DESC, name_type
	`, personID)
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
	_, err := s.db.ExecContext(ctx, "DELETE FROM person_names WHERE id = $1", nameID)
	return err
}

// scanPersonName scans a single person name row.
func scanPersonName(row rowScanner) (*repository.PersonNameReadModel, error) {
	var (
		id, personID                                    uuid.UUID
		givenName, surname, fullName                    string
		namePrefix, nameSuffix, surnamePrefix, nickname sql.NullString
		nameType                                        sql.NullString
		isPrimary                                       bool
		updatedAt                                       time.Time
	)

	err := row.Scan(&id, &personID, &givenName, &surname, &fullName,
		&namePrefix, &nameSuffix, &surnamePrefix, &nickname,
		&nameType, &isPrimary, &updatedAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan person name: %w", err)
	}

	return &repository.PersonNameReadModel{
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
		IsPrimary:     isPrimary,
		UpdatedAt:     updatedAt,
	}, nil
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
			   p.notes, p.research_status, p.version, p.updated_at
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
		researchStatus                   sql.NullString
		birthDateSort, deathDateSort     sql.NullTime
		version                          int64
		updatedAt                        time.Time
	)

	err := row.Scan(&id, &givenName, &surname, &fullName, &gender,
		&birthDateRaw, &birthDateSort, &birthPlace,
		&deathDateRaw, &deathDateSort, &deathPlace,
		&notes, &researchStatus, &version, &updatedAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan person: %w", err)
	}

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
		UpdatedAt:      updatedAt,
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

// GetSource retrieves a source by ID.
func (s *ReadModelStore) GetSource(ctx context.Context, id uuid.UUID) (*repository.SourceReadModel, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, source_type, title, author, publisher, publish_date_raw, publish_date_sort,
			   url, repository_name, collection_name, call_number, notes, gedcom_xref,
			   citation_count, version, updated_at
		FROM sources WHERE id = $1
	`, id)

	return scanSourceRow(row)
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
		LIMIT $1 OFFSET $2
	`, opts.Limit, opts.Offset)
	if err != nil {
		return nil, 0, fmt.Errorf("query sources: %w", err)
	}
	defer rows.Close()

	var sources []repository.SourceReadModel
	for rows.Next() {
		src, err := scanSourceRows(rows)
		if err != nil {
			return nil, 0, err
		}
		sources = append(sources, *src)
	}

	return sources, total, rows.Err()
}

// SearchSources searches for sources by title or author.
func (s *ReadModelStore) SearchSources(ctx context.Context, query string, limit int) ([]repository.SourceReadModel, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, source_type, title, author, publisher, publish_date_raw, publish_date_sort,
			   url, repository_name, collection_name, call_number, notes, gedcom_xref,
			   citation_count, version, updated_at
		FROM sources
		WHERE title ILIKE '%' || $1 || '%' OR author ILIKE '%' || $1 || '%'
		ORDER BY title ASC
		LIMIT $2
	`, query, limit)
	if err != nil {
		return nil, fmt.Errorf("search sources: %w", err)
	}
	defer rows.Close()

	var sources []repository.SourceReadModel
	for rows.Next() {
		src, err := scanSourceRows(rows)
		if err != nil {
			return nil, err
		}
		sources = append(sources, *src)
	}

	return sources, rows.Err()
}

// SaveSource saves or updates a source.
func (s *ReadModelStore) SaveSource(ctx context.Context, source *repository.SourceReadModel) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO sources (id, source_type, title, author, publisher, publish_date_raw, publish_date_sort,
							 url, repository_name, collection_name, call_number, notes, gedcom_xref,
							 citation_count, version, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
		ON CONFLICT(id) DO UPDATE SET
			source_type = EXCLUDED.source_type,
			title = EXCLUDED.title,
			author = EXCLUDED.author,
			publisher = EXCLUDED.publisher,
			publish_date_raw = EXCLUDED.publish_date_raw,
			publish_date_sort = EXCLUDED.publish_date_sort,
			url = EXCLUDED.url,
			repository_name = EXCLUDED.repository_name,
			collection_name = EXCLUDED.collection_name,
			call_number = EXCLUDED.call_number,
			notes = EXCLUDED.notes,
			gedcom_xref = EXCLUDED.gedcom_xref,
			citation_count = EXCLUDED.citation_count,
			version = EXCLUDED.version,
			updated_at = EXCLUDED.updated_at
	`, source.ID, nullableString(string(source.SourceType)), source.Title,
		nullableString(source.Author), nullableString(source.Publisher),
		nullableString(source.PublishDateRaw), nullableTime(source.PublishDateSort),
		nullableString(source.URL), nullableString(source.RepositoryName),
		nullableString(source.CollectionName), nullableString(source.CallNumber),
		nullableString(source.Notes), nullableString(source.GedcomXref),
		source.CitationCount, source.Version, source.UpdatedAt)

	return err
}

// DeleteSource removes a source.
func (s *ReadModelStore) DeleteSource(ctx context.Context, id uuid.UUID) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM sources WHERE id = $1", id)
	return err
}

// GetCitation retrieves a citation by ID.
func (s *ReadModelStore) GetCitation(ctx context.Context, id uuid.UUID) (*repository.CitationReadModel, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, source_id, source_title, fact_type, fact_owner_id, page, volume,
			   source_quality, informant_type, evidence_type, quoted_text, analysis,
			   template_id, gedcom_xref, version, created_at
		FROM citations WHERE id = $1
	`, id)

	return scanCitationRow(row)
}

// GetCitationsForSource returns all citations for a source.
func (s *ReadModelStore) GetCitationsForSource(ctx context.Context, sourceID uuid.UUID) ([]repository.CitationReadModel, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, source_id, source_title, fact_type, fact_owner_id, page, volume,
			   source_quality, informant_type, evidence_type, quoted_text, analysis,
			   template_id, gedcom_xref, version, created_at
		FROM citations
		WHERE source_id = $1
	`, sourceID)
	if err != nil {
		return nil, fmt.Errorf("query citations for source: %w", err)
	}
	defer rows.Close()

	var citations []repository.CitationReadModel
	for rows.Next() {
		cit, err := scanCitationRows(rows)
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
		WHERE fact_owner_id = $1 AND fact_type LIKE 'person_%'
	`, personID)
	if err != nil {
		return nil, fmt.Errorf("query citations for person: %w", err)
	}
	defer rows.Close()

	var citations []repository.CitationReadModel
	for rows.Next() {
		cit, err := scanCitationRows(rows)
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
		WHERE fact_type = $1 AND fact_owner_id = $2
	`, string(factType), factOwnerID)
	if err != nil {
		return nil, fmt.Errorf("query citations for fact: %w", err)
	}
	defer rows.Close()

	var citations []repository.CitationReadModel
	for rows.Next() {
		cit, err := scanCitationRows(rows)
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
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
		ON CONFLICT(id) DO UPDATE SET
			source_id = EXCLUDED.source_id,
			source_title = EXCLUDED.source_title,
			fact_type = EXCLUDED.fact_type,
			fact_owner_id = EXCLUDED.fact_owner_id,
			page = EXCLUDED.page,
			volume = EXCLUDED.volume,
			source_quality = EXCLUDED.source_quality,
			informant_type = EXCLUDED.informant_type,
			evidence_type = EXCLUDED.evidence_type,
			quoted_text = EXCLUDED.quoted_text,
			analysis = EXCLUDED.analysis,
			template_id = EXCLUDED.template_id,
			gedcom_xref = EXCLUDED.gedcom_xref,
			version = EXCLUDED.version
	`, citation.ID, citation.SourceID, nullableString(citation.SourceTitle),
		nullableString(string(citation.FactType)), citation.FactOwnerID,
		nullableString(citation.Page), nullableString(citation.Volume),
		nullableString(string(citation.SourceQuality)), nullableString(string(citation.InformantType)),
		nullableString(string(citation.EvidenceType)), nullableString(citation.QuotedText),
		nullableString(citation.Analysis), nullableString(citation.TemplateID),
		nullableString(citation.GedcomXref), citation.Version, citation.CreatedAt)

	return err
}

// DeleteCitation removes a citation.
func (s *ReadModelStore) DeleteCitation(ctx context.Context, id uuid.UUID) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM citations WHERE id = $1", id)
	return err
}

// Scanning functions for sources and citations

func scanSourceRow(row rowScanner) (*repository.SourceReadModel, error) {
	var (
		id                                uuid.UUID
		sourceType, title                 string
		author, publisher, publishDateRaw sql.NullString
		url, repoName, collName, callNum  sql.NullString
		notes, gedcomXref                 sql.NullString
		publishDateSort                   sql.NullTime
		citationCount                     int
		version                           int64
		updatedAt                         time.Time
	)

	err := row.Scan(&id, &sourceType, &title, &author, &publisher, &publishDateRaw, &publishDateSort,
		&url, &repoName, &collName, &callNum, &notes, &gedcomXref,
		&citationCount, &version, &updatedAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan source: %w", err)
	}

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
		UpdatedAt:      updatedAt,
	}

	if publishDateSort.Valid {
		src.PublishDateSort = &publishDateSort.Time
	}

	return src, nil
}

func scanSourceRows(rows *sql.Rows) (*repository.SourceReadModel, error) {
	return scanSourceRow(rows)
}

func scanCitationRow(row rowScanner) (*repository.CitationReadModel, error) {
	var (
		id, sourceID, factOwnerID        uuid.UUID
		sourceTitle, factType            string
		page, volume, sourceQuality      sql.NullString
		informantType, evidenceType      sql.NullString
		quotedText, analysis, templateID sql.NullString
		gedcomXref                       sql.NullString
		version                          int64
		createdAt                        time.Time
	)

	err := row.Scan(&id, &sourceID, &sourceTitle, &factType, &factOwnerID,
		&page, &volume, &sourceQuality, &informantType, &evidenceType,
		&quotedText, &analysis, &templateID, &gedcomXref, &version, &createdAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan citation: %w", err)
	}

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
		CreatedAt:     createdAt,
	}

	return cit, nil
}

func scanCitationRows(rows *sql.Rows) (*repository.CitationReadModel, error) {
	return scanCitationRow(rows)
}

// GetMedia retrieves media metadata by ID (excludes FileData and ThumbnailData).
func (s *ReadModelStore) GetMedia(ctx context.Context, id uuid.UUID) (*repository.MediaReadModel, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, entity_type, entity_id, title, description, mime_type, media_type,
			   filename, file_size, crop_left, crop_top, crop_width, crop_height,
			   gedcom_xref, version, created_at, updated_at
		FROM media WHERE id = $1
	`, id)

	return scanMediaMetadata(row)
}

// GetMediaWithData retrieves full media record including FileData and ThumbnailData.
func (s *ReadModelStore) GetMediaWithData(ctx context.Context, id uuid.UUID) (*repository.MediaReadModel, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, entity_type, entity_id, title, description, mime_type, media_type,
			   filename, file_size, file_data, thumbnail_data,
			   crop_left, crop_top, crop_width, crop_height,
			   gedcom_xref, version, created_at, updated_at
		FROM media WHERE id = $1
	`, id)

	return scanMediaFull(row)
}

// GetMediaThumbnail retrieves just the thumbnail bytes for efficient serving.
func (s *ReadModelStore) GetMediaThumbnail(ctx context.Context, id uuid.UUID) ([]byte, error) {
	var thumbnail []byte
	err := s.db.QueryRowContext(ctx, `
		SELECT thumbnail_data FROM media WHERE id = $1
	`, id).Scan(&thumbnail)

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
		"SELECT COUNT(*) FROM media WHERE entity_type = $1 AND entity_id = $2",
		entityType, entityID).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("count media: %w", err)
	}

	// Query with pagination (metadata only, ordered by created_at DESC)
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, entity_type, entity_id, title, description, mime_type, media_type,
			   filename, file_size, crop_left, crop_top, crop_width, crop_height,
			   gedcom_xref, version, created_at, updated_at
		FROM media
		WHERE entity_type = $1 AND entity_id = $2
		ORDER BY created_at DESC
		LIMIT $3 OFFSET $4
	`, entityType, entityID, opts.Limit, opts.Offset)
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
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19)
		ON CONFLICT(id) DO UPDATE SET
			entity_type = EXCLUDED.entity_type,
			entity_id = EXCLUDED.entity_id,
			title = EXCLUDED.title,
			description = EXCLUDED.description,
			mime_type = EXCLUDED.mime_type,
			media_type = EXCLUDED.media_type,
			filename = EXCLUDED.filename,
			file_size = EXCLUDED.file_size,
			file_data = EXCLUDED.file_data,
			thumbnail_data = EXCLUDED.thumbnail_data,
			crop_left = EXCLUDED.crop_left,
			crop_top = EXCLUDED.crop_top,
			crop_width = EXCLUDED.crop_width,
			crop_height = EXCLUDED.crop_height,
			gedcom_xref = EXCLUDED.gedcom_xref,
			version = EXCLUDED.version,
			updated_at = EXCLUDED.updated_at
	`, media.ID, media.EntityType, media.EntityID, media.Title,
		nullableString(media.Description), media.MimeType, string(media.MediaType),
		media.Filename, media.FileSize, media.FileData, media.ThumbnailData,
		nullableInt(media.CropLeft), nullableInt(media.CropTop),
		nullableInt(media.CropWidth), nullableInt(media.CropHeight),
		nullableString(media.GedcomXref), media.Version, media.CreatedAt, media.UpdatedAt)

	return err
}

// DeleteMedia removes a media record.
func (s *ReadModelStore) DeleteMedia(ctx context.Context, id uuid.UUID) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM media WHERE id = $1", id)
	return err
}

// Media scanner helpers

func scanMediaMetadata(row rowScanner) (*repository.MediaReadModel, error) {
	var (
		id, entityID                uuid.UUID
		entityType, title, mimeType string
		mediaType, filename         string
		description, gedcomXref     sql.NullString
		fileSize, version           int64
		cropLeft, cropTop           sql.NullInt64
		cropWidth, cropHeight       sql.NullInt64
		createdAt, updatedAt        time.Time
	)

	err := row.Scan(&id, &entityType, &entityID, &title, &description,
		&mimeType, &mediaType, &filename, &fileSize,
		&cropLeft, &cropTop, &cropWidth, &cropHeight,
		&gedcomXref, &version, &createdAt, &updatedAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan media metadata: %w", err)
	}

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
		CreatedAt:   createdAt,
		UpdatedAt:   updatedAt,
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

	return m, nil
}

func scanMediaMetadataRow(rows *sql.Rows) (*repository.MediaReadModel, error) {
	return scanMediaMetadata(rows)
}

func scanMediaFull(row rowScanner) (*repository.MediaReadModel, error) {
	var (
		id, entityID                uuid.UUID
		entityType, title, mimeType string
		mediaType, filename         string
		description, gedcomXref     sql.NullString
		fileSize, version           int64
		fileData, thumbnailData     []byte
		cropLeft, cropTop           sql.NullInt64
		cropWidth, cropHeight       sql.NullInt64
		createdAt, updatedAt        time.Time
	)

	err := row.Scan(&id, &entityType, &entityID, &title, &description,
		&mimeType, &mediaType, &filename, &fileSize, &fileData, &thumbnailData,
		&cropLeft, &cropTop, &cropWidth, &cropHeight,
		&gedcomXref, &version, &createdAt, &updatedAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan media full: %w", err)
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
		CreatedAt:     createdAt,
		UpdatedAt:     updatedAt,
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
		SELECT UPPER(SUBSTRING(surname, 1, 1)) as letter, COUNT(DISTINCT surname) as count
		FROM persons
		WHERE surname != ''
		GROUP BY UPPER(SUBSTRING(surname, 1, 1))
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
		WHERE UPPER(SUBSTRING(surname, 1, 1)) = UPPER($1)
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
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM persons WHERE LOWER(surname) = LOWER($1)", surname).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("count persons by surname: %w", err)
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, given_name, surname, full_name, gender,
			   birth_date_raw, birth_date_sort, birth_place,
			   death_date_raw, death_date_sort, death_place,
			   notes, research_status, version, updated_at
		FROM persons
		WHERE LOWER(surname) = LOWER($1)
		ORDER BY given_name ASC
		LIMIT $2 OFFSET $3
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
						WHEN POSITION(',' IN place) > 0
						THEN TRIM(SPLIT_PART(place, ',', ARRAY_LENGTH(STRING_TO_ARRAY(place, ','), 1)))
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
					THEN true
					ELSE false
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
				SELECT DISTINCT birth_place as place FROM persons WHERE birth_place LIKE '%' || $1 AND birth_place != ''
				UNION
				SELECT DISTINCT death_place as place FROM persons WHERE death_place LIKE '%' || $1 AND death_place != ''
			),
			parsed AS (
				SELECT
					place,
					CASE
						WHEN place = $1 THEN ''
						ELSE TRIM(REPLACE(place, ', ' || $1, ''))
					END as remainder
				FROM all_places
			),
			next_level AS (
				SELECT
					place,
					remainder,
					CASE
						WHEN remainder = '' THEN ''
						WHEN POSITION(',' IN remainder) > 0
						THEN TRIM(SPLIT_PART(remainder, ',', ARRAY_LENGTH(STRING_TO_ARRAY(remainder, ','), 1)))
						ELSE TRIM(remainder)
					END as level_name
				FROM parsed
			)
			SELECT
				level_name as place_name,
				level_name || ', ' || $1 as full_name,
				COUNT(DISTINCT place) as count,
				CASE
					WHEN COUNT(DISTINCT place) > COUNT(DISTINCT CASE WHEN remainder = level_name THEN place END)
					THEN true
					ELSE false
				END as has_children
			FROM next_level
			WHERE level_name != '' AND level_name != $1
			GROUP BY level_name
			ORDER BY level_name ASC
		`, parent)
	}
	if err != nil {
		return nil, fmt.Errorf("query place hierarchy: %w", err)
	}
	defer rows.Close()

	var places []repository.PlaceEntry
	for rows.Next() {
		var entry repository.PlaceEntry
		if err := rows.Scan(&entry.Name, &entry.FullName, &entry.Count, &entry.HasChildren); err != nil {
			return nil, fmt.Errorf("scan place entry: %w", err)
		}
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
		WHERE birth_place ILIKE '%' || $1 || '%' OR death_place ILIKE '%' || $1 || '%'
	`, place).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("count persons by place: %w", err)
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, given_name, surname, full_name, gender,
			   birth_date_raw, birth_date_sort, birth_place,
			   death_date_raw, death_date_sort, death_place,
			   notes, research_status, version, updated_at
		FROM persons
		WHERE birth_place ILIKE '%' || $1 || '%' OR death_place ILIKE '%' || $1 || '%'
		ORDER BY surname ASC, given_name ASC
		LIMIT $2 OFFSET $3
	`, place, opts.Limit, opts.Offset)
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
