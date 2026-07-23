package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/cacack/gedcom-go/v2/gedcom"
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

		-- Enable fuzzystrmatch extension for Soundex/metaphone search
		CREATE EXTENSION IF NOT EXISTS fuzzystrmatch;

		-- Persons table
		-- Branch-aware (ADR-005): (id, branch_id) is the row identity so main and
		-- a branch can each hold a shadow row for the same entity id; deleted marks
		-- a branch tombstone. branch_id defaults to the reserved main id (uuid.Nil).
		CREATE TABLE IF NOT EXISTS persons (
			id UUID NOT NULL,
			branch_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
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
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			deleted BOOLEAN NOT NULL DEFAULT FALSE,
			PRIMARY KEY (id, branch_id)
		);

		CREATE INDEX IF NOT EXISTS idx_persons_surname ON persons(surname, given_name);
		CREATE INDEX IF NOT EXISTS idx_persons_birth_date ON persons(birth_date_sort);
		CREATE INDEX IF NOT EXISTS idx_persons_search ON persons USING GIN(search_vector);
		CREATE INDEX IF NOT EXISTS idx_persons_surname_trgm ON persons USING GIN(surname gin_trgm_ops);
		CREATE INDEX IF NOT EXISTS idx_persons_given_name_trgm ON persons USING GIN(given_name gin_trgm_ops);
		CREATE INDEX IF NOT EXISTS idx_persons_research_status ON persons(research_status);
		-- Secondary index leading with branch_id so PurgeBranch's DELETE ... WHERE
		-- branch_id = ? (and the overlay's branch_id IN filter) is index-driven; the
		-- composite PK leads with id, leaving branch_id otherwise unindexed (#669).
		CREATE INDEX IF NOT EXISTS idx_persons_branch ON persons(branch_id);

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
		-- Branch-aware (ADR-005): (id, branch_id) row identity + deleted tombstone.
		-- Cross-table foreign keys to persons(id) are intentionally dropped because
		-- persons(id) is no longer unique under the branch overlay; cascade behavior
		-- is replicated in the Delete* methods to mirror the memory reference.
		CREATE TABLE IF NOT EXISTS families (
			id UUID NOT NULL,
			branch_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
			partner1_id UUID,
			partner1_given_name VARCHAR(200),
			partner1_surname VARCHAR(200),
			partner2_id UUID,
			partner2_given_name VARCHAR(200),
			partner2_surname VARCHAR(200),
			relationship_type VARCHAR(20),
			marriage_date_raw VARCHAR(100),
			marriage_date_sort DATE,
			marriage_place VARCHAR(255),
			child_count INTEGER NOT NULL DEFAULT 0,
			version BIGINT NOT NULL DEFAULT 1,
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			deleted BOOLEAN NOT NULL DEFAULT FALSE,
			PRIMARY KEY (id, branch_id)
		);

		CREATE INDEX IF NOT EXISTS idx_families_partner1 ON families(partner1_id);
		CREATE INDEX IF NOT EXISTS idx_families_partner2 ON families(partner2_id);
		CREATE INDEX IF NOT EXISTS idx_families_branch ON families(branch_id);

		-- Family children table
		-- Branch-aware (ADR-005): (family_id, person_id, branch_id) row identity +
		-- deleted tombstone. FKs to families/persons dropped (see families note).
		CREATE TABLE IF NOT EXISTS family_children (
			family_id UUID NOT NULL,
			person_id UUID NOT NULL,
			branch_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
			person_given_name VARCHAR(200),
			person_surname VARCHAR(200),
			relationship_type VARCHAR(20) NOT NULL DEFAULT 'biological',
			sequence INTEGER,
			deleted BOOLEAN NOT NULL DEFAULT FALSE,
			PRIMARY KEY (family_id, person_id, branch_id)
		);

		CREATE INDEX IF NOT EXISTS idx_family_children_person ON family_children(person_id);
		CREATE INDEX IF NOT EXISTS idx_family_children_branch ON family_children(branch_id);

		-- Pedigree edges table
		-- Branch-aware (ADR-005): (person_id, branch_id) row identity + deleted
		-- tombstone. FKs to persons dropped (see families note).
		CREATE TABLE IF NOT EXISTS pedigree_edges (
			person_id UUID NOT NULL,
			branch_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
			father_id UUID,
			mother_id UUID,
			father_name VARCHAR(200),
			mother_name VARCHAR(200),
			deleted BOOLEAN NOT NULL DEFAULT FALSE,
			PRIMARY KEY (person_id, branch_id)
		);

		CREATE INDEX IF NOT EXISTS idx_pedigree_father ON pedigree_edges(father_id);
		CREATE INDEX IF NOT EXISTS idx_pedigree_mother ON pedigree_edges(mother_id);
		CREATE INDEX IF NOT EXISTS idx_pedigree_edges_branch ON pedigree_edges(branch_id);

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
			repository_id UUID,
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
			fields_data JSONB,
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
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			-- GEDCOM 7.0 enhanced fields
			files JSONB,          -- Multiple file references (GEDCOM 7.0)
			format VARCHAR(100),  -- Primary format/MIME type (FORM)
			translations JSONB    -- Translated titles (GEDCOM 7.0)
		);

		CREATE INDEX IF NOT EXISTS idx_media_entity ON media(entity_type, entity_id);
		CREATE INDEX IF NOT EXISTS idx_media_type ON media(media_type);

		-- Person names table (for multiple name variants)
		-- Branch-aware (ADR-005): (id, branch_id) row identity + deleted tombstone.
		-- FK to persons dropped (see families note).
		CREATE TABLE IF NOT EXISTS person_names (
			id UUID NOT NULL,
			branch_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
			person_id UUID NOT NULL,
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
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			deleted BOOLEAN NOT NULL DEFAULT FALSE,
			PRIMARY KEY (id, branch_id)
		);

		CREATE INDEX IF NOT EXISTS idx_person_names_person ON person_names(person_id, branch_id);
		CREATE INDEX IF NOT EXISTS idx_person_names_primary ON person_names(person_id, is_primary);
		CREATE INDEX IF NOT EXISTS idx_person_names_search ON person_names USING GIN(search_vector);
		CREATE INDEX IF NOT EXISTS idx_person_names_given_trgm ON person_names USING GIN(given_name gin_trgm_ops);
		CREATE INDEX IF NOT EXISTS idx_person_names_surname_trgm ON person_names USING GIN(surname gin_trgm_ops);
		CREATE INDEX IF NOT EXISTS idx_person_names_branch ON person_names(branch_id);

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

		-- Person external identifiers (GEDCOM 7.0 EXID)
		-- Branch-aware (ADR-005): bucket-scoped by (person_id, branch_id); an empty
		-- branch bucket is represented by a single deleted marker row (tombstone).
		-- FK to persons dropped (see families note).
		CREATE TABLE IF NOT EXISTS person_external_ids (
			person_id UUID NOT NULL,
			sequence INTEGER NOT NULL,
			branch_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
			value TEXT NOT NULL,
			type TEXT NOT NULL DEFAULT '',
			deleted BOOLEAN NOT NULL DEFAULT FALSE,
			PRIMARY KEY (person_id, sequence, branch_id)
		);

		CREATE INDEX IF NOT EXISTS idx_person_external_ids_person ON person_external_ids(person_id, branch_id);
		CREATE INDEX IF NOT EXISTS idx_person_external_ids_branch ON person_external_ids(branch_id);

		-- Notes table (shared GEDCOM NOTE records)
		CREATE TABLE IF NOT EXISTS notes (
			id UUID PRIMARY KEY,
			text TEXT NOT NULL,
			mime VARCHAR(100),
			language VARCHAR(35),
			translations JSONB,
			gedcom_xref VARCHAR(50),
			version BIGINT NOT NULL DEFAULT 1,
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);

		CREATE INDEX IF NOT EXISTS idx_notes_gedcom_xref ON notes(gedcom_xref);

		-- Submitters table (GEDCOM SUBM records for file provenance)
		CREATE TABLE IF NOT EXISTS submitters (
			id UUID PRIMARY KEY,
			name VARCHAR(200) NOT NULL,
			address JSONB,
			phone JSONB,
			email JSONB,
			language VARCHAR(50),
			media_id UUID,
			gedcom_xref VARCHAR(50),
			version BIGINT NOT NULL DEFAULT 1,
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);

		CREATE INDEX IF NOT EXISTS idx_submitters_gedcom_xref ON submitters(gedcom_xref);

		-- Repositories table (GEDCOM REPO records for source document locations)
		CREATE TABLE IF NOT EXISTS repositories (
			id UUID PRIMARY KEY,
			name VARCHAR(200) NOT NULL,
			address JSONB,
			notes TEXT,
			gedcom_xref VARCHAR(50),
			version BIGINT NOT NULL DEFAULT 1,
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);

		CREATE INDEX IF NOT EXISTS idx_repositories_gedcom_xref ON repositories(gedcom_xref);

		-- Family external identifiers (GEDCOM 7.0 EXID)
		-- Branch-aware (ADR-005): bucket-scoped by (family_id, branch_id); an empty
		-- branch bucket is a single deleted marker row. FK to families dropped.
		CREATE TABLE IF NOT EXISTS family_external_ids (
			family_id UUID NOT NULL,
			sequence INTEGER NOT NULL,
			branch_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
			value TEXT NOT NULL,
			type TEXT NOT NULL DEFAULT '',
			deleted BOOLEAN NOT NULL DEFAULT FALSE,
			PRIMARY KEY (family_id, sequence, branch_id)
		);

		CREATE INDEX IF NOT EXISTS idx_family_external_ids_family ON family_external_ids(family_id, branch_id);
		CREATE INDEX IF NOT EXISTS idx_family_external_ids_branch ON family_external_ids(branch_id);

		-- Source external identifiers (GEDCOM 7.0 EXID)
		CREATE TABLE IF NOT EXISTS source_external_ids (
			source_id UUID NOT NULL REFERENCES sources(id) ON DELETE CASCADE,
			sequence INTEGER NOT NULL,
			value TEXT NOT NULL,
			type TEXT NOT NULL DEFAULT '',
			PRIMARY KEY (source_id, sequence)
		);

		CREATE INDEX IF NOT EXISTS idx_source_external_ids_source ON source_external_ids(source_id);

		-- Repository external identifiers (GEDCOM 7.0 EXID)
		CREATE TABLE IF NOT EXISTS repository_external_ids (
			repository_id UUID NOT NULL REFERENCES repositories(id) ON DELETE CASCADE,
			sequence INTEGER NOT NULL,
			value TEXT NOT NULL,
			type TEXT NOT NULL DEFAULT '',
			PRIMARY KEY (repository_id, sequence)
		);

		CREATE INDEX IF NOT EXISTS idx_repository_external_ids_repository ON repository_external_ids(repository_id);

		-- Associations table (GEDCOM ASSO records for non-family relationships)
		-- FK references to persons(id) dropped: persons(id) is not unique under the
		-- branch overlay (ADR-005). This table is not branch-scoped.
		CREATE TABLE IF NOT EXISTS associations (
			id UUID PRIMARY KEY,
			person_id UUID NOT NULL,
			person_name VARCHAR(200),
			associate_id UUID NOT NULL,
			associate_name VARCHAR(200),
			role VARCHAR(100) NOT NULL,
			phrase VARCHAR(500),
			notes TEXT,
			note_ids JSONB,
			gedcom_xref VARCHAR(50),
			version BIGINT NOT NULL DEFAULT 1,
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);

		CREATE INDEX IF NOT EXISTS idx_associations_person ON associations(person_id);
		CREATE INDEX IF NOT EXISTS idx_associations_associate ON associations(associate_id);
		CREATE INDEX IF NOT EXISTS idx_associations_role ON associations(role);

		-- Events table (life events for persons and families)
		CREATE TABLE IF NOT EXISTS events (
			id UUID PRIMARY KEY,
			owner_type VARCHAR(10) NOT NULL,
			owner_id UUID NOT NULL,
			fact_type VARCHAR(100) NOT NULL,
			date_raw VARCHAR(100),
			date_sort DATE,
			place VARCHAR(255),
			place_lat VARCHAR(20),
			place_long VARCHAR(20),
			address JSONB,
			description TEXT,
			cause TEXT,
			age VARCHAR(50),
			research_status VARCHAR(20),
			is_negated BOOLEAN NOT NULL DEFAULT FALSE,
			version BIGINT NOT NULL DEFAULT 1,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);

		CREATE INDEX IF NOT EXISTS idx_events_owner ON events(owner_type, owner_id);
		CREATE INDEX IF NOT EXISTS idx_events_fact_type ON events(fact_type);

		-- Attributes table (person attributes)
		-- FK reference to persons(id) dropped (see associations note). Not branch-scoped.
		CREATE TABLE IF NOT EXISTS attributes (
			id UUID PRIMARY KEY,
			person_id UUID NOT NULL,
			fact_type VARCHAR(100) NOT NULL,
			value TEXT NOT NULL DEFAULT '',
			date_raw VARCHAR(100),
			date_sort DATE,
			place VARCHAR(255),
			version BIGINT NOT NULL DEFAULT 1,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);

		CREATE INDEX IF NOT EXISTS idx_attributes_person ON attributes(person_id);
		CREATE INDEX IF NOT EXISTS idx_attributes_fact_type ON attributes(fact_type);

		-- LDS Ordinances table
		CREATE TABLE IF NOT EXISTS lds_ordinances (
			id UUID PRIMARY KEY,
			type VARCHAR(10) NOT NULL,
			type_label VARCHAR(50) NOT NULL,
			person_id UUID,
			person_name VARCHAR(200),
			family_id UUID,
			date_raw VARCHAR(100),
			date_sort DATE,
			place VARCHAR(255),
			temple VARCHAR(10),
			status VARCHAR(20),
			version BIGINT NOT NULL DEFAULT 1,
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);

		CREATE INDEX IF NOT EXISTS idx_lds_ordinances_person ON lds_ordinances(person_id);
		CREATE INDEX IF NOT EXISTS idx_lds_ordinances_family ON lds_ordinances(family_id);
		CREATE INDEX IF NOT EXISTS idx_lds_ordinances_type ON lds_ordinances(type);

		-- Evidence analyses table
		CREATE TABLE IF NOT EXISTS evidence_analyses (
			id UUID PRIMARY KEY,
			fact_type VARCHAR(50) NOT NULL,
			subject_id UUID NOT NULL,
			citation_ids JSONB,
			conclusion TEXT NOT NULL,
			research_status VARCHAR(20),
			notes TEXT,
			version BIGINT NOT NULL DEFAULT 1,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);

		CREATE INDEX IF NOT EXISTS idx_evidence_analyses_subject ON evidence_analyses(subject_id);
		CREATE INDEX IF NOT EXISTS idx_evidence_analyses_fact_type ON evidence_analyses(fact_type);

		-- Evidence conflicts table
		CREATE TABLE IF NOT EXISTS evidence_conflicts (
			id UUID PRIMARY KEY,
			fact_type VARCHAR(50) NOT NULL,
			subject_id UUID NOT NULL,
			analysis_ids JSONB,
			description TEXT NOT NULL,
			resolution TEXT,
			status VARCHAR(20) NOT NULL,
			version BIGINT NOT NULL DEFAULT 1,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);

		CREATE INDEX IF NOT EXISTS idx_evidence_conflicts_subject ON evidence_conflicts(subject_id);
		CREATE INDEX IF NOT EXISTS idx_evidence_conflicts_status ON evidence_conflicts(status);

		-- Research logs table
		CREATE TABLE IF NOT EXISTS research_logs (
			id UUID PRIMARY KEY,
			subject_id UUID NOT NULL,
			subject_type VARCHAR(20) NOT NULL,
			repository VARCHAR(255) NOT NULL,
			search_description TEXT NOT NULL,
			outcome VARCHAR(20) NOT NULL,
			notes TEXT,
			search_date TIMESTAMPTZ NOT NULL,
			version BIGINT NOT NULL DEFAULT 1,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);

		CREATE INDEX IF NOT EXISTS idx_research_logs_subject ON research_logs(subject_id);
		CREATE INDEX IF NOT EXISTS idx_research_logs_outcome ON research_logs(outcome);

		-- Proof summaries table
		CREATE TABLE IF NOT EXISTS proof_summaries (
			id UUID PRIMARY KEY,
			fact_type VARCHAR(50) NOT NULL,
			subject_id UUID NOT NULL,
			conclusion TEXT NOT NULL,
			argument TEXT NOT NULL,
			analysis_ids JSONB,
			research_status VARCHAR(20),
			version BIGINT NOT NULL DEFAULT 1,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);

		CREATE INDEX IF NOT EXISTS idx_proof_summaries_subject ON proof_summaries(subject_id);
		CREATE INDEX IF NOT EXISTS idx_proof_summaries_fact_type ON proof_summaries(fact_type);
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

	// Add place coordinate columns for geographic features (issue #105)
	_, _ = s.db.Exec(`ALTER TABLE persons ADD COLUMN IF NOT EXISTS birth_place_lat VARCHAR(20)`)
	_, _ = s.db.Exec(`ALTER TABLE persons ADD COLUMN IF NOT EXISTS birth_place_long VARCHAR(20)`)
	_, _ = s.db.Exec(`ALTER TABLE persons ADD COLUMN IF NOT EXISTS death_place_lat VARCHAR(20)`)
	_, _ = s.db.Exec(`ALTER TABLE persons ADD COLUMN IF NOT EXISTS death_place_long VARCHAR(20)`)
	_, _ = s.db.Exec(`ALTER TABLE families ADD COLUMN IF NOT EXISTS marriage_place_lat VARCHAR(20)`)
	_, _ = s.db.Exec(`ALTER TABLE families ADD COLUMN IF NOT EXISTS marriage_place_long VARCHAR(20)`)

	// Add brick wall columns for research tracking (issue #61)
	_, _ = s.db.Exec(`ALTER TABLE persons ADD COLUMN IF NOT EXISTS brick_wall_note TEXT DEFAULT ''`)
	_, _ = s.db.Exec(`ALTER TABLE persons ADD COLUMN IF NOT EXISTS brick_wall_since TIMESTAMPTZ`)
	_, _ = s.db.Exec(`ALTER TABLE persons ADD COLUMN IF NOT EXISTS brick_wall_resolved_at TIMESTAMPTZ`)
	_, _ = s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_persons_brick_wall ON persons(brick_wall_since) WHERE brick_wall_since IS NOT NULL`)

	// Add is_negated column for negative assertions / NO tags (issue #222)
	_, _ = s.db.Exec(`ALTER TABLE events ADD COLUMN IF NOT EXISTS is_negated BOOLEAN NOT NULL DEFAULT FALSE`)

	// Add GEDCOM 7.0 shared-note (SNOTE) metadata columns (issue #225)
	_, _ = s.db.Exec(`ALTER TABLE notes ADD COLUMN IF NOT EXISTS mime VARCHAR(100)`)
	_, _ = s.db.Exec(`ALTER TABLE notes ADD COLUMN IF NOT EXISTS language VARCHAR(35)`)
	_, _ = s.db.Exec(`ALTER TABLE notes ADD COLUMN IF NOT EXISTS translations JSONB`)

	// Split family partner names and family-child names into given_name / surname (issue #483).
	// Wrapped in a single transaction so a mid-migration crash leaves either the pre-migration
	// or post-migration state, never a half-state where legacy columns are dropped before the
	// new ones are populated. SQLite-side does NOT drop the legacy columns (project convention
	// never drops columns on SQLite), so the two backends diverge intentionally here.
	if tx, err := s.db.Begin(); err == nil {
		_, _ = tx.Exec(`ALTER TABLE families ADD COLUMN IF NOT EXISTS partner1_given_name VARCHAR(200)`)
		_, _ = tx.Exec(`ALTER TABLE families ADD COLUMN IF NOT EXISTS partner1_surname VARCHAR(200)`)
		_, _ = tx.Exec(`ALTER TABLE families ADD COLUMN IF NOT EXISTS partner2_given_name VARCHAR(200)`)
		_, _ = tx.Exec(`ALTER TABLE families ADD COLUMN IF NOT EXISTS partner2_surname VARCHAR(200)`)
		_, _ = tx.Exec(`ALTER TABLE family_children ADD COLUMN IF NOT EXISTS person_given_name VARCHAR(200)`)
		_, _ = tx.Exec(`ALTER TABLE family_children ADD COLUMN IF NOT EXISTS person_surname VARCHAR(200)`)

		// Backfill split fields from persons table; idempotent via IS NULL guard.
		_, _ = tx.Exec(`
			UPDATE families f SET
				partner1_given_name = p.given_name,
				partner1_surname    = p.surname
			FROM persons p WHERE f.partner1_id = p.id AND f.partner1_given_name IS NULL
		`)
		_, _ = tx.Exec(`
			UPDATE families f SET
				partner2_given_name = p.given_name,
				partner2_surname    = p.surname
			FROM persons p WHERE f.partner2_id = p.id AND f.partner2_given_name IS NULL
		`)
		_, _ = tx.Exec(`
			UPDATE family_children fc SET
				person_given_name = p.given_name,
				person_surname    = p.surname
			FROM persons p WHERE fc.person_id = p.id AND fc.person_given_name IS NULL
		`)

		// Drop legacy denormalized columns once the split is populated.
		_, _ = tx.Exec(`ALTER TABLE families DROP COLUMN IF EXISTS partner1_name, DROP COLUMN IF EXISTS partner2_name`)
		_, _ = tx.Exec(`ALTER TABLE family_children DROP COLUMN IF EXISTS person_name`)
		_ = tx.Commit()
	}

	// Add repository_id to sources for ID-based source→repository linkage (issue #525).
	_, _ = s.db.Exec(`ALTER TABLE sources ADD COLUMN IF NOT EXISTS repository_id UUID`)

	// Branch-aware read model (ADR-005 / issue #669). Add branch_id + deleted to
	// each slice table, drop cross-table FKs to persons(id)/families(id) (no longer
	// unique under the overlay), and re-key each table on a (…, branch_id) composite
	// PK. Existing rows backfill to the reserved main branch id (uuid.Nil) via the
	// column default. Best-effort like the migrations above: errors are ignored so a
	// DB already at the target shape is left untouched.
	s.runBranchMigration()
}

// runBranchMigration migrates an existing database to the branch-aware slice
// schema (ADR-005). Each table is migrated in its own transaction so a crash
// leaves the table either wholly pre- or post-migration. FK drops must precede
// the persons/families PK swap because a PK cannot be dropped while referenced.
func (s *ReadModelStore) runBranchMigration() {
	const mainDefault = `DEFAULT '00000000-0000-0000-0000-000000000000'`

	// Drop every foreign key that references persons(id) or families(id); these
	// span slice and non-slice tables. Done first (outside the per-table PK swaps)
	// so the referenced PKs are free to change.
	dropFKs := []string{
		`ALTER TABLE families DROP CONSTRAINT IF EXISTS families_partner1_id_fkey`,
		`ALTER TABLE families DROP CONSTRAINT IF EXISTS families_partner2_id_fkey`,
		`ALTER TABLE family_children DROP CONSTRAINT IF EXISTS family_children_family_id_fkey`,
		`ALTER TABLE family_children DROP CONSTRAINT IF EXISTS family_children_person_id_fkey`,
		`ALTER TABLE pedigree_edges DROP CONSTRAINT IF EXISTS pedigree_edges_person_id_fkey`,
		`ALTER TABLE pedigree_edges DROP CONSTRAINT IF EXISTS pedigree_edges_father_id_fkey`,
		`ALTER TABLE pedigree_edges DROP CONSTRAINT IF EXISTS pedigree_edges_mother_id_fkey`,
		`ALTER TABLE person_names DROP CONSTRAINT IF EXISTS person_names_person_id_fkey`,
		`ALTER TABLE person_external_ids DROP CONSTRAINT IF EXISTS person_external_ids_person_id_fkey`,
		`ALTER TABLE family_external_ids DROP CONSTRAINT IF EXISTS family_external_ids_family_id_fkey`,
		`ALTER TABLE associations DROP CONSTRAINT IF EXISTS associations_person_id_fkey`,
		`ALTER TABLE associations DROP CONSTRAINT IF EXISTS associations_associate_id_fkey`,
		`ALTER TABLE attributes DROP CONSTRAINT IF EXISTS attributes_person_id_fkey`,
	}
	for _, stmt := range dropFKs {
		_, _ = s.db.Exec(stmt)
	}

	// Per-table: add branch_id + deleted, then swap the primary key to include
	// branch_id. slicePK lists the non-branch key columns of each table's new PK.
	type sliceTable struct {
		name string
		pk   string // comma-separated key columns excluding branch_id
	}
	tables := []sliceTable{
		{"persons", "id"},
		{"families", "id"},
		{"family_children", "family_id, person_id"},
		{"pedigree_edges", "person_id"},
		{"person_names", "id"},
		{"person_external_ids", "person_id, sequence"},
		{"family_external_ids", "family_id, sequence"},
	}
	for _, t := range tables {
		// Column adds are idempotent and safe outside a transaction.
		_, _ = s.db.Exec(`ALTER TABLE ` + t.name + ` ADD COLUMN IF NOT EXISTS branch_id UUID NOT NULL ` + mainDefault)
		_, _ = s.db.Exec(`ALTER TABLE ` + t.name + ` ADD COLUMN IF NOT EXISTS deleted BOOLEAN NOT NULL DEFAULT FALSE`)

		// Swap the PK atomically. If branch_id is already part of the PK (target
		// shape reached) the ADD PRIMARY KEY fails and the tx rolls back, leaving
		// the already-correct PK in place.
		if tx, err := s.db.Begin(); err == nil {
			if _, err := tx.Exec(`ALTER TABLE ` + t.name + ` DROP CONSTRAINT IF EXISTS ` + t.name + `_pkey`); err != nil {
				_ = tx.Rollback()
				continue
			}
			if _, err := tx.Exec(`ALTER TABLE ` + t.name + ` ADD PRIMARY KEY (` + t.pk + `, branch_id)`); err != nil {
				_ = tx.Rollback()
				continue
			}
			_ = tx.Commit()
		}
	}

	// Refresh the collection-table indexes to include branch_id (the overlay filters
	// on parent + branch). Old single-column variants are replaced.
	_, _ = s.db.Exec(`DROP INDEX IF EXISTS idx_person_names_person`)
	_, _ = s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_person_names_person ON person_names(person_id, branch_id)`)
	_, _ = s.db.Exec(`DROP INDEX IF EXISTS idx_person_external_ids_person`)
	_, _ = s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_person_external_ids_person ON person_external_ids(person_id, branch_id)`)
	_, _ = s.db.Exec(`DROP INDEX IF EXISTS idx_family_external_ids_family`)
	_, _ = s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_family_external_ids_family ON family_external_ids(family_id, branch_id)`)

	// Secondary indexes leading with branch_id so PurgeBranch's DELETE ... WHERE
	// branch_id = ? (and the overlay's branch_id IN filter) is index-driven rather
	// than a full-table scan; the composite PK leads with id, so branch_id alone is
	// otherwise unindexed (issue #669).
	for _, tbl := range []string{
		"persons", "families", "family_children", "pedigree_edges",
		"person_names", "person_external_ids", "family_external_ids",
	} {
		_, _ = s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_` + tbl + `_branch ON ` + tbl + `(branch_id)`)
	}
}

// Column lists for the branch overlay queries (ADR-005). The overlay resolves a
// branch's view of a slice table in a single set-based query: an inner
// SELECT DISTINCT ON (<identity>) picks the branch's row over main's for each
// identity, and an OUTER "WHERE NOT deleted" drops identities whose winning row
// is a tombstone (so a branch tombstone suppresses the main fallback). The NOT
// deleted filter must be applied AFTER the DISTINCT ON, not inside it.
const (
	// personSelectCols is scanPerson's column order (unaliased).
	personSelectCols = `id, given_name, surname, full_name, gender,
		birth_date_raw, birth_date_sort, birth_place, birth_place_lat, birth_place_long,
		death_date_raw, death_date_sort, death_place, death_place_lat, death_place_long,
		notes, research_status, brick_wall_note, brick_wall_since, brick_wall_resolved_at,
		version, updated_at`

	// personInsertCols is personSelectCols without the generated full_name column,
	// for INSERT ... SELECT (a generated column cannot be written).
	personInsertCols = `id, given_name, surname, gender,
		birth_date_raw, birth_date_sort, birth_place, birth_place_lat, birth_place_long,
		death_date_raw, death_date_sort, death_place, death_place_lat, death_place_long,
		notes, research_status, brick_wall_note, brick_wall_since, brick_wall_resolved_at,
		version, updated_at`

	// personNameSelectCols is scanPersonName's column order (unaliased).
	personNameSelectCols = `id, person_id, given_name, surname, full_name, name_prefix, name_suffix,
		surname_prefix, nickname, name_type, is_primary, updated_at`

	// personNameOverlayCols are the person_names columns SearchPersons needs from
	// the resolved rpn CTE (matching against alternate names).
	personNameOverlayCols = `id, person_id, given_name, surname, full_name, nickname, is_primary, search_vector`

	// familySelectCols is scanFamily's column order (unaliased).
	familySelectCols = `id, partner1_id, partner1_given_name, partner1_surname,
		partner2_id, partner2_given_name, partner2_surname,
		relationship_type, marriage_date_raw, marriage_date_sort, marriage_place,
		marriage_place_lat, marriage_place_long,
		child_count, version, updated_at`

	// familyChildSelectCols matches GetFamilyChildren's scan order (unaliased).
	familyChildSelectCols = `family_id, person_id, person_given_name, person_surname, relationship_type, sequence`

	// pedigreeSelectCols matches GetPedigreeEdge's scan order (unaliased).
	pedigreeSelectCols = `person_id, father_id, mother_id, father_name, mother_name`
)

// GetPerson retrieves a person by ID within the branch overlay (ADR-005).
func (s *ReadModelStore) GetPerson(ctx context.Context, branchID domain.BranchID, id uuid.UUID) (*repository.PersonReadModel, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT `+personSelectCols+` FROM (
			SELECT DISTINCT ON (id) `+personSelectCols+`, deleted
			FROM persons WHERE id = $1 AND branch_id IN ($2, $3)
			ORDER BY id, (branch_id = $2) DESC
		) o WHERE NOT deleted
	`, id, branchID.UUID(), domain.MainBranchID.UUID())

	return scanPerson(row)
}

// ListPersons returns a paginated list of persons within the branch overlay (ADR-005).
func (s *ReadModelStore) ListPersons(ctx context.Context, opts repository.ListOptions) ([]repository.PersonReadModel, int, error) {
	// Main-scope fast path (issue #669): main never shadows itself, so persons holds
	// exactly one row per id. The DISTINCT ON overlay is then pure overhead that also
	// materializes+sorts the whole table before ORDER BY/LIMIT can apply, defeating
	// index-driven pagination. Query persons directly so the planner can use the
	// sort/filter indexes and short-circuit at LIMIT. Non-main keeps the overlay.
	var cte, fromSrc string
	var conds []string
	var args []any
	var paramNum int
	if opts.BranchID.IsMain() {
		fromSrc = "persons"
		conds = append(conds, "branch_id = $1 AND NOT deleted")
		args = []any{domain.MainBranchID.UUID()}
		paramNum = 2
	} else {
		fromSrc = "resolved"
		cte = `WITH resolved AS (
			SELECT ` + personSelectCols + ` FROM (
				SELECT DISTINCT ON (id) ` + personSelectCols + `, deleted
				FROM persons WHERE branch_id IN ($1, $2)
				ORDER BY id, (branch_id = $1) DESC
			) o WHERE NOT deleted
		)`
		args = []any{opts.BranchID.UUID(), domain.MainBranchID.UUID()}
		paramNum = 3
	}

	// research_status filter (params continue after branch/main).
	if opts.ResearchStatus != nil {
		if *opts.ResearchStatus == "unset" {
			conds = append(conds, "(research_status IS NULL OR research_status = '')")
		} else {
			conds = append(conds, fmt.Sprintf("research_status = $%d", paramNum))
			args = append(args, *opts.ResearchStatus)
			paramNum++
		}
	}

	whereClause := ""
	if len(conds) > 0 {
		whereClause = "WHERE " + strings.Join(conds, " AND ")
	}

	// Count total (with filter if present)
	var total int
	// nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query -- whereClause uses parameterized placeholders, not user input
	countQuery := cte + " SELECT COUNT(*) FROM " + fromSrc + " " + whereClause
	err := s.db.QueryRowContext(ctx, countQuery, args...).Scan(&total)
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
	query := cte + fmt.Sprintf(`
		SELECT `+personSelectCols+`
		FROM `+fromSrc+`
		%s
		ORDER BY %s %s NULLS LAST, given_name %s
		LIMIT $%d OFFSET $%d
	`, whereClause, orderColumn, orderDir, orderDir, paramNum, paramNum+1)

	// Build args: branch/main + where args + limit + offset
	queryArgs := append(args, opts.Limit, opts.Offset)
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

// searchQueryParams tracks parameterized query building state.
type searchQueryParams struct {
	args   []any
	paramN int
}

func (p *searchQueryParams) add(val any) int {
	n := p.paramN
	p.args = append(p.args, val)
	p.paramN++
	return n
}

const personCols = `p.id, p.given_name, p.surname, p.full_name, p.gender,
	p.birth_date_raw, p.birth_date_sort, p.birth_place, p.birth_place_lat, p.birth_place_long,
	p.death_date_raw, p.death_date_sort, p.death_place, p.death_place_lat, p.death_place_long,
	p.notes, p.research_status, p.brick_wall_note, p.brick_wall_since, p.brick_wall_resolved_at,
	p.version, p.updated_at`

// SearchPersons searches for persons by name, date, and place using tsvector,
// trigram similarity, and Soundex matching. Also searches person_names for alternate names.
// All provided filters are ANDed together.
func (s *ReadModelStore) SearchPersons(ctx context.Context, opts repository.SearchOptions) ([]repository.PersonReadModel, error) {
	if opts.Limit <= 0 {
		opts.Limit = 20
	}
	if opts.Limit > 100 {
		opts.Limit = 100
	}

	hasQuery := strings.TrimSpace(opts.Query) != ""
	hasDateFilter := opts.BirthDateFrom != nil || opts.BirthDateTo != nil ||
		opts.DeathDateFrom != nil || opts.DeathDateTo != nil
	hasPlaceFilter := strings.TrimSpace(opts.BirthPlace) != "" || strings.TrimSpace(opts.DeathPlace) != ""

	if !hasQuery && !hasDateFilter && !hasPlaceFilter {
		return nil, nil
	}

	var qb strings.Builder
	params := &searchQueryParams{paramN: 1}

	// Resolve the branch overlay of persons (rp) and person_names (rpn) up front so
	// all matching runs against the branch's view, never the raw tables (ADR-005).
	// $branch/$main are the first two params; the name-match CTE and filters below
	// read from rp/rpn. This keeps the whole search a single set-based statement.
	branchN := params.add(opts.BranchID.UUID())
	if opts.BranchID.IsMain() {
		// Main fast path (issue #669): main never shadows itself, so rp/rpn are the
		// raw main-scoped rows and matching can use the GIN/trigram indexes directly
		// instead of paying the DISTINCT ON overlay's materialize+sort.
		writeResolvedPersonCTEsMain(&qb, branchN)
	} else {
		mainN := params.add(domain.MainBranchID.UUID())
		writeResolvedPersonCTEs(&qb, branchN, mainN)
	}

	if hasQuery {
		writeNameMatchCTE(&qb, opts, params)
		writeDedupSelect(&qb)
	} else {
		fmt.Fprintf(&qb, ` SELECT %s FROM rp p`, personCols)
	}

	writeDatePlaceFilters(&qb, opts, params)
	writeOrderBy(&qb, opts, hasQuery)

	fmt.Fprintf(&qb, " LIMIT $%d", params.add(opts.Limit))

	rows, err := s.db.QueryContext(ctx, qb.String(), params.args...)
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

// writeResolvedPersonCTEs writes the leading "WITH rp AS (...), rpn AS (...)"
// clause that resolves the branch overlay of persons and person_names for
// SearchPersons. branchN/mainN are the $-placeholder numbers for the branch and
// main ids. Downstream clauses read from rp/rpn instead of the raw tables so the
// entire search resolves the overlay in one set-based statement (no per-row
// branch lookups). When branch == main this collapses to the main rows.
func writeResolvedPersonCTEs(qb *strings.Builder, branchN, mainN int) {
	// rp also carries search_vector (needed by the full-text match), which
	// personSelectCols omits; the final SELECT only reads personCols so the extra
	// column is harmless.
	fmt.Fprintf(qb, `WITH rp AS (
		SELECT %s, search_vector FROM (
			SELECT DISTINCT ON (id) %s, search_vector, deleted
			FROM persons WHERE branch_id IN ($%d, $%d)
			ORDER BY id, (branch_id = $%d) DESC
		) o WHERE NOT deleted
	), rpn AS (
		SELECT %s FROM (
			SELECT DISTINCT ON (id) %s, deleted
			FROM person_names WHERE branch_id IN ($%d, $%d)
			ORDER BY id, (branch_id = $%d) DESC
		) o WHERE NOT deleted
	)`, personSelectCols, personSelectCols, branchN, mainN, branchN,
		personNameOverlayCols, personNameOverlayCols, branchN, mainN, branchN)
}

// writeResolvedPersonCTEsMain is the main-scope fast path for
// writeResolvedPersonCTEs (issue #669). For MainBranchID there is exactly one row
// per id, so the DISTINCT ON overlay is skipped: rp/rpn are the raw rows filtered
// to the main branch (tombstones excluded), letting the full-text/trigram indexes
// drive matching. branchN is the $-placeholder for the main branch id.
func writeResolvedPersonCTEsMain(qb *strings.Builder, branchN int) {
	// NOT MATERIALIZED: matched_persons references rp twice (direct match + the
	// rp JOIN rpn alt-name branch), which would otherwise trigger Postgres 12+'s
	// default to materialize a CTE used 2+ times. Materializing rp forces a full
	// branch-filtered scan of persons before the text predicates run, defeating the
	// GIN/trigram/tsvector indexes. Inlining pushes those predicates down to the base
	// tables so the indexes drive matching. Safe here because these are plain
	// branch-filtered selects; the non-main overlay's DISTINCT ON is left to
	// materialize (see writeResolvedPersonCTEs).
	fmt.Fprintf(qb, `WITH rp AS NOT MATERIALIZED (
		SELECT %s, search_vector FROM persons WHERE branch_id = $%d AND NOT deleted
	), rpn AS NOT MATERIALIZED (
		SELECT %s FROM person_names WHERE branch_id = $%d AND NOT deleted
	)`, personSelectCols, branchN, personNameOverlayCols, branchN)
}

// writeNameMatchCTE appends the matched_persons CTE for name matching (fuzzy,
// soundex, or full-text). It runs against the resolved rp/rpn CTEs written by
// writeResolvedPersonCTEs, so it opens with ", matched_persons AS (".
func writeNameMatchCTE(qb *strings.Builder, opts repository.SearchOptions, params *searchQueryParams) {
	query := strings.TrimSpace(opts.Query)
	n := params.add(query)

	switch {
	case opts.Fuzzy:
		fmt.Fprintf(qb, `, matched_persons AS (
			SELECT %s, TRUE as is_primary,
				GREATEST(similarity(p.given_name, $%d), similarity(p.surname, $%d), similarity(p.full_name, $%d)) as rank_score
			FROM rp p
			WHERE p.given_name %% $%d OR p.surname %% $%d OR p.full_name %% $%d
			UNION
			SELECT %s, pn.is_primary,
				GREATEST(similarity(pn.given_name, $%d), similarity(pn.surname, $%d), similarity(pn.full_name, $%d), similarity(COALESCE(pn.nickname, ''), $%d)) as rank_score
			FROM rp p JOIN rpn pn ON p.id = pn.person_id
			WHERE pn.given_name %% $%d OR pn.surname %% $%d OR pn.full_name %% $%d OR pn.nickname %% $%d
		)`, personCols, n, n, n, n, n, n,
			personCols, n, n, n, n, n, n, n, n)

	case opts.Soundex:
		fmt.Fprintf(qb, `, matched_persons AS (
			SELECT %s, TRUE as is_primary,
				GREATEST(difference(p.given_name, $%d), difference(p.surname, $%d))::float as rank_score
			FROM rp p
			WHERE difference(p.given_name, $%d) >= 3 OR difference(p.surname, $%d) >= 3
			UNION
			SELECT %s, pn.is_primary,
				GREATEST(difference(pn.given_name, $%d), difference(pn.surname, $%d))::float as rank_score
			FROM rp p JOIN rpn pn ON p.id = pn.person_id
			WHERE difference(pn.given_name, $%d) >= 3 OR difference(pn.surname, $%d) >= 3
		)`, personCols, n, n, n, n,
			personCols, n, n, n, n)

	default:
		fmt.Fprintf(qb, `, matched_persons AS (
			SELECT %s, TRUE as is_primary,
				ts_rank(p.search_vector, plainto_tsquery('english', $%d)) as rank_score
			FROM rp p
			WHERE p.search_vector @@ plainto_tsquery('english', $%d) OR p.full_name ILIKE '%%' || $%d || '%%'
			UNION
			SELECT %s, pn.is_primary,
				ts_rank(pn.search_vector, plainto_tsquery('english', $%d)) as rank_score
			FROM rp p JOIN rpn pn ON p.id = pn.person_id
			WHERE pn.search_vector @@ plainto_tsquery('english', $%d) OR pn.full_name ILIKE '%%' || $%d || '%%' OR pn.nickname ILIKE '%%' || $%d || '%%'
		)`, personCols, n, n, n,
			personCols, n, n, n, n)
	}
}

// writeDedupSelect writes the deduplication CTE and final SELECT.
func writeDedupSelect(qb *strings.Builder) {
	qb.WriteString(`, deduped AS (
		SELECT DISTINCT ON (id) id, given_name, surname, full_name, gender,
			birth_date_raw, birth_date_sort, birth_place, birth_place_lat, birth_place_long,
			death_date_raw, death_date_sort, death_place, death_place_lat, death_place_long,
			notes, research_status, brick_wall_note, brick_wall_since, brick_wall_resolved_at,
			version, updated_at, rank_score
		FROM matched_persons
		ORDER BY id, is_primary DESC, rank_score DESC
	)
	SELECT id, given_name, surname, full_name, gender,
		birth_date_raw, birth_date_sort, birth_place, birth_place_lat, birth_place_long,
		death_date_raw, death_date_sort, death_place, death_place_lat, death_place_long,
		notes, research_status, brick_wall_note, brick_wall_since, brick_wall_resolved_at,
		version, updated_at
	FROM deduped p`)
}

// writeDatePlaceFilters appends WHERE clauses for date and place filters.
func writeDatePlaceFilters(qb *strings.Builder, opts repository.SearchOptions, params *searchQueryParams) {
	var filters []string

	if opts.BirthDateFrom != nil {
		filters = append(filters, fmt.Sprintf("p.birth_date_sort >= $%d", params.add(*opts.BirthDateFrom)))
	}
	if opts.BirthDateTo != nil {
		filters = append(filters, fmt.Sprintf("p.birth_date_sort <= $%d", params.add(*opts.BirthDateTo)))
	}
	if opts.DeathDateFrom != nil {
		filters = append(filters, fmt.Sprintf("p.death_date_sort >= $%d", params.add(*opts.DeathDateFrom)))
	}
	if opts.DeathDateTo != nil {
		filters = append(filters, fmt.Sprintf("p.death_date_sort <= $%d", params.add(*opts.DeathDateTo)))
	}
	if bp := strings.TrimSpace(opts.BirthPlace); bp != "" {
		filters = append(filters, fmt.Sprintf("p.birth_place ILIKE '%%' || $%d || '%%'", params.add(bp)))
	}
	if dp := strings.TrimSpace(opts.DeathPlace); dp != "" {
		filters = append(filters, fmt.Sprintf("p.death_place ILIKE '%%' || $%d || '%%'", params.add(dp)))
	}

	if len(filters) > 0 {
		qb.WriteString(" WHERE " + strings.Join(filters, " AND "))
	}
}

// writeOrderBy appends the ORDER BY clause based on sort options.
func writeOrderBy(qb *strings.Builder, opts repository.SearchOptions, hasQuery bool) {
	orderDir := strings.ToUpper(opts.Order)
	if orderDir != "ASC" && orderDir != "DESC" {
		orderDir = ""
	}

	switch opts.Sort {
	case "name":
		if orderDir == "" {
			orderDir = "ASC"
		}
		fmt.Fprintf(qb, " ORDER BY p.surname %s, p.given_name %s", orderDir, orderDir)
	case "birth_date":
		if orderDir == "" {
			orderDir = "ASC"
		}
		fmt.Fprintf(qb, " ORDER BY p.birth_date_sort %s NULLS LAST", orderDir)
	case "death_date":
		if orderDir == "" {
			orderDir = "ASC"
		}
		fmt.Fprintf(qb, " ORDER BY p.death_date_sort %s NULLS LAST", orderDir)
	default:
		if hasQuery {
			if orderDir == "" {
				orderDir = "DESC"
			}
			fmt.Fprintf(qb, " ORDER BY rank_score %s", orderDir)
		} else {
			if orderDir == "" {
				orderDir = "ASC"
			}
			fmt.Fprintf(qb, " ORDER BY p.surname %s, p.given_name %s", orderDir, orderDir)
		}
	}
}

// SavePerson saves or updates a person on the given branch (ADR-005). The row is
// keyed by (id, branch_id); a save always clears any prior tombstone (deleted).
func (s *ReadModelStore) SavePerson(ctx context.Context, branchID domain.BranchID, person *repository.PersonReadModel) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO persons (id, branch_id, given_name, surname, gender, birth_date_raw, birth_date_sort, birth_place,
							 birth_place_lat, birth_place_long, death_date_raw, death_date_sort, death_place,
							 death_place_lat, death_place_long, notes, research_status,
							 brick_wall_note, brick_wall_since, brick_wall_resolved_at,
							 version, updated_at, deleted)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, FALSE)
		ON CONFLICT(id, branch_id) DO UPDATE SET
			given_name = EXCLUDED.given_name,
			surname = EXCLUDED.surname,
			gender = EXCLUDED.gender,
			birth_date_raw = EXCLUDED.birth_date_raw,
			birth_date_sort = EXCLUDED.birth_date_sort,
			birth_place = EXCLUDED.birth_place,
			birth_place_lat = EXCLUDED.birth_place_lat,
			birth_place_long = EXCLUDED.birth_place_long,
			death_date_raw = EXCLUDED.death_date_raw,
			death_date_sort = EXCLUDED.death_date_sort,
			death_place = EXCLUDED.death_place,
			death_place_lat = EXCLUDED.death_place_lat,
			death_place_long = EXCLUDED.death_place_long,
			notes = EXCLUDED.notes,
			research_status = EXCLUDED.research_status,
			brick_wall_note = EXCLUDED.brick_wall_note,
			brick_wall_since = EXCLUDED.brick_wall_since,
			brick_wall_resolved_at = EXCLUDED.brick_wall_resolved_at,
			version = EXCLUDED.version,
			updated_at = EXCLUDED.updated_at,
			deleted = FALSE
	`, person.ID, branchID.UUID(), person.GivenName, person.Surname, nullableGender(person.Gender),
		nullableString(person.BirthDateRaw), nullableTime(person.BirthDateSort), nullableString(person.BirthPlace),
		nullableStringPtr(person.BirthPlaceLat), nullableStringPtr(person.BirthPlaceLong),
		nullableString(person.DeathDateRaw), nullableTime(person.DeathDateSort), nullableString(person.DeathPlace),
		nullableStringPtr(person.DeathPlaceLat), nullableStringPtr(person.DeathPlaceLong),
		nullableString(person.Notes), nullableString(string(person.ResearchStatus)),
		nullableString(person.BrickWallNote), nullableTime(person.BrickWallSince), nullableTime(person.BrickWallResolvedAt),
		person.Version, person.UpdatedAt)

	return err
}

// DeletePerson removes a person (ADR-005). On main it is a real removal and
// cascades to the person's names and external IDs (matching the FK cascade that
// existed before the overlay). On a non-main branch it writes a tombstone for the
// person plus cascade tombstones for the person's names and external IDs, so the
// main fallback cannot resurrect them.
func (s *ReadModelStore) DeletePerson(ctx context.Context, branchID domain.BranchID, id uuid.UUID) error {
	main := domain.MainBranchID.UUID()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if branchID.IsMain() {
		// Reproduce the pre-#669 ON DELETE CASCADE explicitly: the read-model FKs
		// were dropped so persons could be keyed by (id, branch_id), so every
		// dependent the FKs used to cascade must be cleaned up by hand here.
		// person_names, person_external_ids, pedigree_edges and associations (both
		// person_id and associate_id) were ON DELETE CASCADE pre-#669. attributes
		// referenced persons(id) with NO ON DELETE (RESTRICT), which would have
		// blocked the delete; blocking is not reproducible against an append-only
		// event log (the event store is the source of truth), so we cascade-delete
		// orphan attributes too rather than leave dangling read-model rows.
		// associations/attributes are main-only (no branch_id column).
		for _, stmt := range []struct {
			sql  string
			args []any
		}{
			{"DELETE FROM persons WHERE id = $1 AND branch_id = $2", []any{id, main}},
			{"DELETE FROM person_names WHERE person_id = $1 AND branch_id = $2", []any{id, main}},
			{"DELETE FROM person_external_ids WHERE person_id = $1 AND branch_id = $2", []any{id, main}},
			{"DELETE FROM pedigree_edges WHERE person_id = $1 AND branch_id = $2", []any{id, main}},
			{"DELETE FROM associations WHERE person_id = $1 OR associate_id = $1", []any{id}},
			{"DELETE FROM attributes WHERE person_id = $1", []any{id}},
		} {
			if _, err := tx.ExecContext(ctx, stmt.sql, stmt.args...); err != nil {
				return fmt.Errorf("delete person: %w", err)
			}
		}
		return tx.Commit()
	}

	// Branch (non-main) delete: tombstone the person and its branch-scoped
	// dependents (names, external IDs, pedigree edge). associations/attributes are
	// main-only (not branch-scoped), so a branch delete does not touch them.

	// Tombstone the person on the branch (copy the resolved row, mark deleted).
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO persons (`+personInsertCols+`, branch_id, deleted)
		SELECT `+personInsertCols+`, $2, TRUE FROM (
			SELECT DISTINCT ON (id) `+personInsertCols+`
			FROM persons WHERE id = $1 AND branch_id IN ($2, $3)
			ORDER BY id, (branch_id = $2) DESC
		) o
		ON CONFLICT (id, branch_id) DO UPDATE SET deleted = TRUE
	`, id, branchID.UUID(), main); err != nil {
		return fmt.Errorf("tombstone person: %w", err)
	}

	// Cascade tombstone the person's names (every name visible on the branch).
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO person_names (id, person_id, given_name, surname, name_prefix, name_suffix,
								  surname_prefix, nickname, name_type, is_primary, updated_at, branch_id, deleted)
		SELECT id, person_id, given_name, surname, name_prefix, name_suffix,
			   surname_prefix, nickname, name_type, is_primary, updated_at, $2, TRUE
		FROM (
			SELECT DISTINCT ON (id) id, person_id, given_name, surname, name_prefix, name_suffix,
				   surname_prefix, nickname, name_type, is_primary, updated_at, deleted
			FROM person_names WHERE person_id = $1 AND branch_id IN ($2, $3)
			ORDER BY id, (branch_id = $2) DESC
		) o WHERE NOT o.deleted
		ON CONFLICT (id, branch_id) DO UPDATE SET deleted = TRUE
	`, id, branchID.UUID(), main); err != nil {
		return fmt.Errorf("cascade tombstone person names: %w", err)
	}

	// Cascade tombstone the person's external IDs as an empty branch bucket marker.
	if err := tombstoneExternalIDBucket(ctx, tx, "person_external_ids", "person_id", id, branchID.UUID()); err != nil {
		return err
	}

	// Cascade tombstone the person's pedigree edge so the mainline edge does not
	// resurrect through the overlay (mirrors DeletePedigreeEdge).
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO pedigree_edges (person_id, branch_id, deleted)
		VALUES ($1, $2, TRUE)
		ON CONFLICT(person_id, branch_id) DO UPDATE SET deleted = TRUE
	`, id, branchID.UUID()); err != nil {
		return fmt.Errorf("cascade tombstone pedigree edge: %w", err)
	}

	return tx.Commit()
}

// SavePersonName saves or updates a person name variant on the given branch
// (ADR-005). Keyed by (id, branch_id); untouched names fall back to main via the
// overlay, so no copy-on-write of the whole bucket is needed here.
func (s *ReadModelStore) SavePersonName(ctx context.Context, branchID domain.BranchID, name *repository.PersonNameReadModel) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO person_names (id, branch_id, person_id, given_name, surname, name_prefix, name_suffix,
								  surname_prefix, nickname, name_type, is_primary, updated_at, deleted)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, FALSE)
		ON CONFLICT(id, branch_id) DO UPDATE SET
			person_id = EXCLUDED.person_id,
			given_name = EXCLUDED.given_name,
			surname = EXCLUDED.surname,
			name_prefix = EXCLUDED.name_prefix,
			name_suffix = EXCLUDED.name_suffix,
			surname_prefix = EXCLUDED.surname_prefix,
			nickname = EXCLUDED.nickname,
			name_type = EXCLUDED.name_type,
			is_primary = EXCLUDED.is_primary,
			updated_at = EXCLUDED.updated_at,
			deleted = FALSE
	`, name.ID, branchID.UUID(), name.PersonID, name.GivenName, name.Surname,
		nullableString(name.NamePrefix), nullableString(name.NameSuffix),
		nullableString(name.SurnamePrefix), nullableString(name.Nickname),
		// name_type is NOT NULL DEFAULT '' — bind the empty string, not NULL
		// (matches the SQLite backend; a nil here violates the constraint).
		string(name.NameType), name.IsPrimary, name.UpdatedAt)

	return err
}

// GetPersonName retrieves a person name by ID within the branch overlay (ADR-005).
func (s *ReadModelStore) GetPersonName(ctx context.Context, branchID domain.BranchID, nameID uuid.UUID) (*repository.PersonNameReadModel, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT `+personNameSelectCols+` FROM (
			SELECT DISTINCT ON (id) `+personNameSelectCols+`, deleted
			FROM person_names WHERE id = $1 AND branch_id IN ($2, $3)
			ORDER BY id, (branch_id = $2) DESC
		) o WHERE NOT deleted
	`, nameID, branchID.UUID(), domain.MainBranchID.UUID())

	return scanPersonName(row)
}

// GetPersonNames retrieves all name variants for a person within the branch overlay.
func (s *ReadModelStore) GetPersonNames(ctx context.Context, branchID domain.BranchID, personID uuid.UUID) ([]repository.PersonNameReadModel, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+personNameSelectCols+` FROM (
			SELECT DISTINCT ON (id) `+personNameSelectCols+`, deleted
			FROM person_names WHERE person_id = $1 AND branch_id IN ($2, $3)
			ORDER BY id, (branch_id = $2) DESC
		) o WHERE NOT deleted
		ORDER BY is_primary DESC, name_type
	`, personID, branchID.UUID(), domain.MainBranchID.UUID())
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

// DeletePersonName removes a person name (ADR-005). On main it is a real removal;
// on a non-main branch it writes a tombstone (copying the resolved name row) so
// the main fallback does not resurrect it.
func (s *ReadModelStore) DeletePersonName(ctx context.Context, branchID domain.BranchID, nameID uuid.UUID) error {
	if branchID.IsMain() {
		_, err := s.db.ExecContext(ctx, "DELETE FROM person_names WHERE id = $1 AND branch_id = $2", nameID, domain.MainBranchID.UUID())
		return err
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO person_names (id, person_id, given_name, surname, name_prefix, name_suffix,
								  surname_prefix, nickname, name_type, is_primary, updated_at, branch_id, deleted)
		SELECT id, person_id, given_name, surname, name_prefix, name_suffix,
			   surname_prefix, nickname, name_type, is_primary, updated_at, $2, TRUE
		FROM (
			SELECT DISTINCT ON (id) id, person_id, given_name, surname, name_prefix, name_suffix,
				   surname_prefix, nickname, name_type, is_primary, updated_at
			FROM person_names WHERE id = $1 AND branch_id IN ($2, $3)
			ORDER BY id, (branch_id = $2) DESC
		) o
		ON CONFLICT (id, branch_id) DO UPDATE SET deleted = TRUE
	`, nameID, branchID.UUID(), domain.MainBranchID.UUID())
	return err
}

// tombstoneExternalIDBucket writes an empty branch bucket marker for an external
// ID table: it clears the branch's rows for the parent and inserts a single
// deleted marker row so the branch bucket is present-but-empty (hiding main),
// mirroring the memory backend's empty-bucket tombstone. Used by branch deletes.
func tombstoneExternalIDBucket(ctx context.Context, tx *sql.Tx, table, parentCol string, parentID, branchID uuid.UUID) error {
	// #nosec G201 G202 -- table and parentCol are package-internal literals, not user input
	if _, err := tx.ExecContext(ctx, "DELETE FROM "+table+" WHERE "+parentCol+" = $1 AND branch_id = $2", parentID, branchID); err != nil {
		return fmt.Errorf("clear %s branch bucket: %w", table, err)
	}
	// #nosec G201 G202 -- table and parentCol are package-internal literals, not user input
	if _, err := tx.ExecContext(ctx, "INSERT INTO "+table+" ("+parentCol+", sequence, branch_id, value, type, deleted) VALUES ($1, 0, $2, '', '', TRUE)", parentID, branchID); err != nil {
		return fmt.Errorf("mark %s empty branch bucket: %w", table, err)
	}
	return nil
}

// ReplacePersonExternalIDs replaces all external identifiers (GEDCOM 7.0 EXID)
// for a person on the given branch within a single transaction (ADR-005). This is
// a bucket-scoped replace: it clears only the branch's rows and writes the new
// set as branch rows. On a non-main branch an empty set writes a present-but-empty
// tombstone bucket so main's identifiers are hidden; on main an empty set is a
// plain removal.
func (s *ReadModelStore) ReplacePersonExternalIDs(ctx context.Context, branchID domain.BranchID, personID uuid.UUID, ids []repository.PersonExternalIDReadModel) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, "DELETE FROM person_external_ids WHERE person_id = $1 AND branch_id = $2", personID, branchID.UUID()); err != nil {
		return fmt.Errorf("delete person external ids: %w", err)
	}
	if len(ids) == 0 {
		if !branchID.IsMain() {
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO person_external_ids (person_id, sequence, branch_id, value, type, deleted)
				VALUES ($1, 0, $2, '', '', TRUE)
			`, personID, branchID.UUID()); err != nil {
				return fmt.Errorf("mark empty person external id bucket: %w", err)
			}
		}
		return tx.Commit()
	}
	for i, id := range ids {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO person_external_ids (person_id, sequence, branch_id, value, type, deleted)
			VALUES ($1, $2, $3, $4, $5, FALSE)
		`, personID, i, branchID.UUID(), id.Value, id.Type); err != nil {
			return fmt.Errorf("insert person external id: %w", err)
		}
	}
	return tx.Commit()
}

// GetPersonExternalIDs retrieves all external identifiers for a person within the
// branch overlay, ordered by their original sequence (ADR-005). Bucket-scoped: if
// the branch has any rows for the person (including an empty-bucket tombstone
// marker) the branch bucket wins wholesale; otherwise it falls back to main.
func (s *ReadModelStore) GetPersonExternalIDs(ctx context.Context, branchID domain.BranchID, personID uuid.UUID) ([]repository.PersonExternalIDReadModel, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT sequence, value, type FROM person_external_ids
		WHERE person_id = $1 AND NOT deleted AND branch_id = (
			CASE WHEN EXISTS (SELECT 1 FROM person_external_ids WHERE person_id = $1 AND branch_id = $2)
			     THEN $2 ELSE $3 END)
		ORDER BY sequence
	`, personID, branchID.UUID(), domain.MainBranchID.UUID())
	if err != nil {
		return nil, fmt.Errorf("query person external ids: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var result []repository.PersonExternalIDReadModel
	for rows.Next() {
		var (
			seq   int
			value string
			typ   string
		)
		if err := rows.Scan(&seq, &value, &typ); err != nil {
			return nil, fmt.Errorf("scan person external id: %w", err)
		}
		result = append(result, repository.PersonExternalIDReadModel{
			PersonID: personID,
			Sequence: seq,
			Value:    value,
			Type:     typ,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate person external ids: %w", err)
	}
	return result, nil
}

// ReplaceFamilyExternalIDs replaces all external identifiers (GEDCOM 7.0 EXID)
// for a family on the given branch within a single transaction (ADR-005). Same
// bucket-scoped semantics as ReplacePersonExternalIDs.
func (s *ReadModelStore) ReplaceFamilyExternalIDs(ctx context.Context, branchID domain.BranchID, familyID uuid.UUID, ids []repository.FamilyExternalIDReadModel) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, "DELETE FROM family_external_ids WHERE family_id = $1 AND branch_id = $2", familyID, branchID.UUID()); err != nil {
		return fmt.Errorf("delete family external ids: %w", err)
	}
	if len(ids) == 0 {
		if !branchID.IsMain() {
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO family_external_ids (family_id, sequence, branch_id, value, type, deleted)
				VALUES ($1, 0, $2, '', '', TRUE)
			`, familyID, branchID.UUID()); err != nil {
				return fmt.Errorf("mark empty family external id bucket: %w", err)
			}
		}
		return tx.Commit()
	}
	for i, id := range ids {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO family_external_ids (family_id, sequence, branch_id, value, type, deleted)
			VALUES ($1, $2, $3, $4, $5, FALSE)
		`, familyID, i, branchID.UUID(), id.Value, id.Type); err != nil {
			return fmt.Errorf("insert family external id: %w", err)
		}
	}
	return tx.Commit()
}

// GetFamilyExternalIDs retrieves all external identifiers for a family within the
// branch overlay, ordered by their original sequence (ADR-005). Bucket-scoped
// resolution, matching GetPersonExternalIDs.
func (s *ReadModelStore) GetFamilyExternalIDs(ctx context.Context, branchID domain.BranchID, familyID uuid.UUID) ([]repository.FamilyExternalIDReadModel, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT sequence, value, type FROM family_external_ids
		WHERE family_id = $1 AND NOT deleted AND branch_id = (
			CASE WHEN EXISTS (SELECT 1 FROM family_external_ids WHERE family_id = $1 AND branch_id = $2)
			     THEN $2 ELSE $3 END)
		ORDER BY sequence
	`, familyID, branchID.UUID(), domain.MainBranchID.UUID())
	if err != nil {
		return nil, fmt.Errorf("query family external ids: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var result []repository.FamilyExternalIDReadModel
	for rows.Next() {
		var (
			seq   int
			value string
			typ   string
		)
		if err := rows.Scan(&seq, &value, &typ); err != nil {
			return nil, fmt.Errorf("scan family external id: %w", err)
		}
		result = append(result, repository.FamilyExternalIDReadModel{
			FamilyID: familyID,
			Sequence: seq,
			Value:    value,
			Type:     typ,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate family external ids: %w", err)
	}
	return result, nil
}

// ReplaceSourceExternalIDs replaces all external identifiers (GEDCOM 7.0 EXID)
// for a source within a single transaction.
func (s *ReadModelStore) ReplaceSourceExternalIDs(ctx context.Context, sourceID uuid.UUID, ids []repository.SourceExternalIDReadModel) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, "DELETE FROM source_external_ids WHERE source_id = $1", sourceID); err != nil {
		return fmt.Errorf("delete source external ids: %w", err)
	}
	for i, id := range ids {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO source_external_ids (source_id, sequence, value, type)
			VALUES ($1, $2, $3, $4)
		`, sourceID, i, id.Value, id.Type); err != nil {
			return fmt.Errorf("insert source external id: %w", err)
		}
	}
	return tx.Commit()
}

// GetSourceExternalIDs retrieves all external identifiers for a source, ordered
// by their original sequence.
func (s *ReadModelStore) GetSourceExternalIDs(ctx context.Context, sourceID uuid.UUID) ([]repository.SourceExternalIDReadModel, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT sequence, value, type FROM source_external_ids
		WHERE source_id = $1 ORDER BY sequence
	`, sourceID)
	if err != nil {
		return nil, fmt.Errorf("query source external ids: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var result []repository.SourceExternalIDReadModel
	for rows.Next() {
		var (
			seq   int
			value string
			typ   string
		)
		if err := rows.Scan(&seq, &value, &typ); err != nil {
			return nil, fmt.Errorf("scan source external id: %w", err)
		}
		result = append(result, repository.SourceExternalIDReadModel{
			SourceID: sourceID,
			Sequence: seq,
			Value:    value,
			Type:     typ,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate source external ids: %w", err)
	}
	return result, nil
}

// ReplaceRepositoryExternalIDs replaces all external identifiers (GEDCOM 7.0
// EXID) for a repository within a single transaction.
func (s *ReadModelStore) ReplaceRepositoryExternalIDs(ctx context.Context, repositoryID uuid.UUID, ids []repository.RepositoryExternalIDReadModel) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, "DELETE FROM repository_external_ids WHERE repository_id = $1", repositoryID); err != nil {
		return fmt.Errorf("delete repository external ids: %w", err)
	}
	for i, id := range ids {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO repository_external_ids (repository_id, sequence, value, type)
			VALUES ($1, $2, $3, $4)
		`, repositoryID, i, id.Value, id.Type); err != nil {
			return fmt.Errorf("insert repository external id: %w", err)
		}
	}
	return tx.Commit()
}

// GetRepositoryExternalIDs retrieves all external identifiers for a repository,
// ordered by their original sequence.
func (s *ReadModelStore) GetRepositoryExternalIDs(ctx context.Context, repositoryID uuid.UUID) ([]repository.RepositoryExternalIDReadModel, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT sequence, value, type FROM repository_external_ids
		WHERE repository_id = $1 ORDER BY sequence
	`, repositoryID)
	if err != nil {
		return nil, fmt.Errorf("query repository external ids: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var result []repository.RepositoryExternalIDReadModel
	for rows.Next() {
		var (
			seq   int
			value string
			typ   string
		)
		if err := rows.Scan(&seq, &value, &typ); err != nil {
			return nil, fmt.Errorf("scan repository external id: %w", err)
		}
		result = append(result, repository.RepositoryExternalIDReadModel{
			RepositoryID: repositoryID,
			Sequence:     seq,
			Value:        value,
			Type:         typ,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate repository external ids: %w", err)
	}
	return result, nil
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

// GetFamily retrieves a family by ID within the branch overlay (ADR-005).
func (s *ReadModelStore) GetFamily(ctx context.Context, branchID domain.BranchID, id uuid.UUID) (*repository.FamilyReadModel, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT `+familySelectCols+` FROM (
			SELECT DISTINCT ON (id) `+familySelectCols+`, deleted
			FROM families WHERE id = $1 AND branch_id IN ($2, $3)
			ORDER BY id, (branch_id = $2) DESC
		) o WHERE NOT deleted
	`, id, branchID.UUID(), domain.MainBranchID.UUID())

	return scanFamily(row)
}

// ListFamilies returns a paginated list of families within the branch overlay (ADR-005).
func (s *ReadModelStore) ListFamilies(ctx context.Context, opts repository.ListOptions) ([]repository.FamilyReadModel, int, error) {
	// Main-scope fast path (issue #669): main never shadows itself, so query families
	// directly and let the planner short-circuit at LIMIT instead of materializing+
	// sorting the whole table through the DISTINCT ON overlay. Non-main keeps the overlay.
	var cte, fromSrc, whereClause string
	var args []any
	var limitParam int
	if opts.BranchID.IsMain() {
		fromSrc = "families"
		whereClause = "WHERE branch_id = $1 AND NOT deleted"
		args = []any{domain.MainBranchID.UUID()}
		limitParam = 2
	} else {
		fromSrc = "resolved"
		cte = `WITH resolved AS (
			SELECT ` + familySelectCols + ` FROM (
				SELECT DISTINCT ON (id) ` + familySelectCols + `, deleted
				FROM families WHERE branch_id IN ($1, $2)
				ORDER BY id, (branch_id = $1) DESC
			) o WHERE NOT deleted
		)`
		args = []any{opts.BranchID.UUID(), domain.MainBranchID.UUID()}
		limitParam = 3
	}

	var total int
	// nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query -- whereClause uses parameterized placeholders, not user input
	err := s.db.QueryRowContext(ctx, cte+" SELECT COUNT(*) FROM "+fromSrc+" "+whereClause, args...).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("count families: %w", err)
	}

	// #nosec G201 -- fromSrc/whereClause/limitParam are internal, not user input
	query := cte + fmt.Sprintf(`
		SELECT `+familySelectCols+`
		FROM `+fromSrc+`
		%s
		ORDER BY updated_at DESC
		LIMIT $%d OFFSET $%d
	`, whereClause, limitParam, limitParam+1)
	queryArgs := append(args, opts.Limit, opts.Offset)
	rows, err := s.db.QueryContext(ctx, query, queryArgs...)
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

// GetFamiliesForPerson returns all families where the person is a partner, within
// the branch overlay (ADR-005).
func (s *ReadModelStore) GetFamiliesForPerson(ctx context.Context, branchID domain.BranchID, personID uuid.UUID) ([]repository.FamilyReadModel, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+familySelectCols+` FROM (
			SELECT DISTINCT ON (id) `+familySelectCols+`, deleted
			FROM families WHERE branch_id IN ($2, $3)
			ORDER BY id, (branch_id = $2) DESC
		) o WHERE NOT deleted AND (partner1_id = $1 OR partner2_id = $1)
	`, personID, branchID.UUID(), domain.MainBranchID.UUID())
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

// SaveFamily saves or updates a family on the given branch (ADR-005). Keyed by
// (id, branch_id); a save clears any prior tombstone.
func (s *ReadModelStore) SaveFamily(ctx context.Context, branchID domain.BranchID, family *repository.FamilyReadModel) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO families (id, branch_id, partner1_id, partner1_given_name, partner1_surname,
							  partner2_id, partner2_given_name, partner2_surname,
							  relationship_type, marriage_date_raw, marriage_date_sort, marriage_place,
							  marriage_place_lat, marriage_place_long,
							  child_count, version, updated_at, deleted)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, FALSE)
		ON CONFLICT(id, branch_id) DO UPDATE SET
			partner1_id = EXCLUDED.partner1_id,
			partner1_given_name = EXCLUDED.partner1_given_name,
			partner1_surname = EXCLUDED.partner1_surname,
			partner2_id = EXCLUDED.partner2_id,
			partner2_given_name = EXCLUDED.partner2_given_name,
			partner2_surname = EXCLUDED.partner2_surname,
			relationship_type = EXCLUDED.relationship_type,
			marriage_date_raw = EXCLUDED.marriage_date_raw,
			marriage_date_sort = EXCLUDED.marriage_date_sort,
			marriage_place = EXCLUDED.marriage_place,
			marriage_place_lat = EXCLUDED.marriage_place_lat,
			marriage_place_long = EXCLUDED.marriage_place_long,
			child_count = EXCLUDED.child_count,
			version = EXCLUDED.version,
			updated_at = EXCLUDED.updated_at,
			deleted = FALSE
	`, family.ID, branchID.UUID(),
		nullableUUID(family.Partner1ID), nullableString(family.Partner1GivenName), nullableString(family.Partner1Surname),
		nullableUUID(family.Partner2ID), nullableString(family.Partner2GivenName), nullableString(family.Partner2Surname),
		// relationship_type is NOT NULL DEFAULT 'biological' — bind the string value,
		// not NULL (matches the SQLite backend; a nil here violates the constraint).
		string(family.RelationshipType), nullableString(family.MarriageDateRaw),
		nullableTime(family.MarriageDateSort), nullableString(family.MarriagePlace),
		nullableStringPtr(family.MarriagePlaceLat), nullableStringPtr(family.MarriagePlaceLong),
		family.ChildCount, family.Version, family.UpdatedAt)

	return err
}

// DeleteFamily removes a family (ADR-005). On main it is a real removal and
// cascades to the family's children and external IDs. On a non-main branch it
// writes a tombstone for the family plus cascade tombstones for its children and
// external IDs, so the main fallback cannot resurrect them.
func (s *ReadModelStore) DeleteFamily(ctx context.Context, branchID domain.BranchID, id uuid.UUID) error {
	main := domain.MainBranchID.UUID()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if branchID.IsMain() {
		for _, stmt := range []string{
			"DELETE FROM families WHERE id = $1 AND branch_id = $2",
			"DELETE FROM family_children WHERE family_id = $1 AND branch_id = $2",
			"DELETE FROM family_external_ids WHERE family_id = $1 AND branch_id = $2",
		} {
			if _, err := tx.ExecContext(ctx, stmt, id, main); err != nil {
				return fmt.Errorf("delete family: %w", err)
			}
		}
		return tx.Commit()
	}

	// Tombstone the family on the branch (copy the resolved row, mark deleted).
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO families (`+familySelectCols+`, branch_id, deleted)
		SELECT `+familySelectCols+`, $2, TRUE FROM (
			SELECT DISTINCT ON (id) `+familySelectCols+`
			FROM families WHERE id = $1 AND branch_id IN ($2, $3)
			ORDER BY id, (branch_id = $2) DESC
		) o
		ON CONFLICT (id, branch_id) DO UPDATE SET deleted = TRUE
	`, id, branchID.UUID(), main); err != nil {
		return fmt.Errorf("tombstone family: %w", err)
	}

	// Cascade tombstone the family's children (every child visible on the branch).
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO family_children (family_id, person_id, person_given_name, person_surname,
									 relationship_type, sequence, branch_id, deleted)
		SELECT family_id, person_id, person_given_name, person_surname, relationship_type, sequence, $2, TRUE
		FROM (
			SELECT DISTINCT ON (family_id, person_id) family_id, person_id, person_given_name,
				   person_surname, relationship_type, sequence, deleted
			FROM family_children WHERE family_id = $1 AND branch_id IN ($2, $3)
			ORDER BY family_id, person_id, (branch_id = $2) DESC
		) o WHERE NOT o.deleted
		ON CONFLICT (family_id, person_id, branch_id) DO UPDATE SET deleted = TRUE
	`, id, branchID.UUID(), main); err != nil {
		return fmt.Errorf("cascade tombstone family children: %w", err)
	}

	// Cascade tombstone the family's external IDs as an empty branch bucket marker.
	if err := tombstoneExternalIDBucket(ctx, tx, "family_external_ids", "family_id", id, branchID.UUID()); err != nil {
		return err
	}

	return tx.Commit()
}

// GetFamilyChildren returns all children for a family within the branch overlay (ADR-005).
func (s *ReadModelStore) GetFamilyChildren(ctx context.Context, branchID domain.BranchID, familyID uuid.UUID) ([]repository.FamilyChildReadModel, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+familyChildSelectCols+` FROM (
			SELECT DISTINCT ON (family_id, person_id) `+familyChildSelectCols+`, deleted
			FROM family_children WHERE family_id = $1 AND branch_id IN ($2, $3)
			ORDER BY family_id, person_id, (branch_id = $2) DESC
		) o WHERE NOT deleted
		ORDER BY sequence NULLS LAST, person_surname, person_given_name
	`, familyID, branchID.UUID(), domain.MainBranchID.UUID())
	if err != nil {
		return nil, fmt.Errorf("query family children: %w", err)
	}
	defer rows.Close()

	var children []repository.FamilyChildReadModel
	for rows.Next() {
		var (
			familyID, personID             uuid.UUID
			personGivenName, personSurname sql.NullString
			relType                        string
			sequence                       sql.NullInt64
		)
		err := rows.Scan(&familyID, &personID, &personGivenName, &personSurname, &relType, &sequence)
		if err != nil {
			return nil, fmt.Errorf("scan family child: %w", err)
		}

		child := repository.FamilyChildReadModel{
			FamilyID:         familyID,
			PersonID:         personID,
			PersonGivenName:  personGivenName.String,
			PersonSurname:    personSurname.String,
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

// GetChildrenOfFamily returns person read models for all children in a family,
// resolving both the children and each person through the branch overlay (ADR-005).
func (s *ReadModelStore) GetChildrenOfFamily(ctx context.Context, branchID domain.BranchID, familyID uuid.UUID) ([]repository.PersonReadModel, error) {
	rows, err := s.db.QueryContext(ctx, `
		WITH rc AS (
			SELECT `+familyChildSelectCols+` FROM (
				SELECT DISTINCT ON (family_id, person_id) `+familyChildSelectCols+`, deleted
				FROM family_children WHERE family_id = $1 AND branch_id IN ($2, $3)
				ORDER BY family_id, person_id, (branch_id = $2) DESC
			) o WHERE NOT deleted
		), rp AS (
			SELECT `+personSelectCols+` FROM (
				SELECT DISTINCT ON (id) `+personSelectCols+`, deleted
				FROM persons WHERE branch_id IN ($2, $3)
				ORDER BY id, (branch_id = $2) DESC
			) o WHERE NOT deleted
		)
		SELECT `+personCols+`
		FROM rp p
		JOIN rc ON p.id = rc.person_id
		ORDER BY rc.sequence NULLS LAST, p.given_name
	`, familyID, branchID.UUID(), domain.MainBranchID.UUID())
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

// GetChildFamily returns the family where the person is a child, resolving both
// the child link and the family through the branch overlay (ADR-005).
func (s *ReadModelStore) GetChildFamily(ctx context.Context, branchID domain.BranchID, personID uuid.UUID) (*repository.FamilyReadModel, error) {
	row := s.db.QueryRowContext(ctx, `
		WITH rc AS (
			SELECT family_id, person_id FROM (
				SELECT DISTINCT ON (family_id, person_id) family_id, person_id, deleted
				FROM family_children WHERE person_id = $1 AND branch_id IN ($2, $3)
				ORDER BY family_id, person_id, (branch_id = $2) DESC
			) o WHERE NOT deleted
		), rf AS (
			SELECT `+familySelectCols+` FROM (
				SELECT DISTINCT ON (id) `+familySelectCols+`, deleted
				FROM families WHERE branch_id IN ($2, $3)
				ORDER BY id, (branch_id = $2) DESC
			) o WHERE NOT deleted
		)
		SELECT f.id, f.partner1_id, f.partner1_given_name, f.partner1_surname,
			   f.partner2_id, f.partner2_given_name, f.partner2_surname,
			   f.relationship_type, f.marriage_date_raw, f.marriage_date_sort, f.marriage_place,
			   f.marriage_place_lat, f.marriage_place_long,
			   f.child_count, f.version, f.updated_at
		FROM rf f
		JOIN rc ON f.id = rc.family_id
		LIMIT 1
	`, personID, branchID.UUID(), domain.MainBranchID.UUID())

	return scanFamily(row)
}

// SaveFamilyChild saves a family child relationship on the given branch (ADR-005).
// Keyed by (family_id, person_id, branch_id); untouched children fall back to main.
func (s *ReadModelStore) SaveFamilyChild(ctx context.Context, branchID domain.BranchID, child *repository.FamilyChildReadModel) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO family_children (family_id, person_id, branch_id, person_given_name, person_surname, relationship_type, sequence, deleted)
		VALUES ($1, $2, $3, $4, $5, $6, $7, FALSE)
		ON CONFLICT(family_id, person_id, branch_id) DO UPDATE SET
			person_given_name = EXCLUDED.person_given_name,
			person_surname = EXCLUDED.person_surname,
			relationship_type = EXCLUDED.relationship_type,
			sequence = EXCLUDED.sequence,
			deleted = FALSE
	`, child.FamilyID, child.PersonID, branchID.UUID(), nullableString(child.PersonGivenName), nullableString(child.PersonSurname),
		string(child.RelationshipType), nullableInt(child.Sequence))

	return err
}

// DeleteFamilyChild removes a family child relationship (ADR-005). On main it is a
// real removal; on a non-main branch it writes a tombstone (copying the resolved
// child row) so the main fallback does not resurrect it.
func (s *ReadModelStore) DeleteFamilyChild(ctx context.Context, branchID domain.BranchID, familyID, personID uuid.UUID) error {
	if branchID.IsMain() {
		_, err := s.db.ExecContext(ctx, "DELETE FROM family_children WHERE family_id = $1 AND person_id = $2 AND branch_id = $3",
			familyID, personID, domain.MainBranchID.UUID())
		return err
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO family_children (family_id, person_id, person_given_name, person_surname,
									 relationship_type, sequence, branch_id, deleted)
		SELECT family_id, person_id, person_given_name, person_surname, relationship_type, sequence, $3, TRUE
		FROM (
			SELECT DISTINCT ON (family_id, person_id) family_id, person_id, person_given_name,
				   person_surname, relationship_type, sequence
			FROM family_children WHERE family_id = $1 AND person_id = $2 AND branch_id IN ($3, $4)
			ORDER BY family_id, person_id, (branch_id = $3) DESC
		) o
		ON CONFLICT (family_id, person_id, branch_id) DO UPDATE SET deleted = TRUE
	`, familyID, personID, branchID.UUID(), domain.MainBranchID.UUID())
	return err
}

// GetPedigreeEdge returns the pedigree edge for a person within the branch overlay (ADR-005).
func (s *ReadModelStore) GetPedigreeEdge(ctx context.Context, branchID domain.BranchID, personID uuid.UUID) (*repository.PedigreeEdge, error) {
	var (
		pID                    uuid.UUID
		fatherID, motherID     sql.NullString
		fatherName, motherName sql.NullString
	)

	err := s.db.QueryRowContext(ctx, `
		SELECT `+pedigreeSelectCols+` FROM (
			SELECT DISTINCT ON (person_id) `+pedigreeSelectCols+`, deleted
			FROM pedigree_edges WHERE person_id = $1 AND branch_id IN ($2, $3)
			ORDER BY person_id, (branch_id = $2) DESC
		) o WHERE NOT deleted
	`, personID, branchID.UUID(), domain.MainBranchID.UUID()).Scan(&pID, &fatherID, &motherID, &fatherName, &motherName)

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

// SavePedigreeEdge saves a pedigree edge on the given branch (ADR-005). Keyed by
// (person_id, branch_id); a save clears any prior tombstone.
func (s *ReadModelStore) SavePedigreeEdge(ctx context.Context, branchID domain.BranchID, edge *repository.PedigreeEdge) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO pedigree_edges (person_id, branch_id, father_id, mother_id, father_name, mother_name, deleted)
		VALUES ($1, $2, $3, $4, $5, $6, FALSE)
		ON CONFLICT(person_id, branch_id) DO UPDATE SET
			father_id = EXCLUDED.father_id,
			mother_id = EXCLUDED.mother_id,
			father_name = EXCLUDED.father_name,
			mother_name = EXCLUDED.mother_name,
			deleted = FALSE
	`, edge.PersonID, branchID.UUID(), nullableUUID(edge.FatherID), nullableUUID(edge.MotherID),
		nullableString(edge.FatherName), nullableString(edge.MotherName))

	return err
}

// DeletePedigreeEdge removes a pedigree edge (ADR-005). On main it is a real
// removal; on a non-main branch it writes a tombstone so the main fallback does
// not resurrect the edge.
func (s *ReadModelStore) DeletePedigreeEdge(ctx context.Context, branchID domain.BranchID, personID uuid.UUID) error {
	if branchID.IsMain() {
		_, err := s.db.ExecContext(ctx, "DELETE FROM pedigree_edges WHERE person_id = $1 AND branch_id = $2", personID, domain.MainBranchID.UUID())
		return err
	}
	// Always record a tombstone for the branch (person_id is the only NOT NULL
	// column besides branch_id), mirroring the memory backend which tombstones
	// regardless of whether a resolved edge currently exists.
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO pedigree_edges (person_id, branch_id, deleted)
		VALUES ($1, $2, TRUE)
		ON CONFLICT(person_id, branch_id) DO UPDATE SET deleted = TRUE
	`, personID, branchID.UUID())
	return err
}

// PurgeBranch hard-deletes every row for branchID.UUID() across the seven branch-scoped
// slice tables. It is a no-op for the mainline (domain.MainBranchID), which is
// never purged. See ADR-005 and the branch-delete projection handler.
func (s *ReadModelStore) PurgeBranch(ctx context.Context, branchID domain.BranchID) error {
	if branchID.IsMain() {
		return nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// associations/attributes are intentionally excluded: they are main-scoped
	// (no branch_id column), so a branch-only entity cannot own them and purging
	// them here would delete mainline data.
	for _, table := range []string{
		"persons",
		"person_names",
		"person_external_ids",
		"families",
		"family_external_ids",
		"family_children",
		"pedigree_edges",
	} {
		// #nosec G202 -- table is from a hardcoded slice of slice-table names, not user input
		if _, err := tx.ExecContext(ctx, "DELETE FROM "+table+" WHERE branch_id = $1", branchID.UUID()); err != nil {
			return fmt.Errorf("purge branch %s: %w", table, err)
		}
	}
	return tx.Commit()
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
		birthPlaceLat, birthPlaceLong    sql.NullString
		deathDateRaw, deathPlace, notes  sql.NullString
		deathPlaceLat, deathPlaceLong    sql.NullString
		researchStatus                   sql.NullString
		brickWallNote                    sql.NullString
		brickWallSince                   sql.NullTime
		brickWallResolvedAt              sql.NullTime
		birthDateSort, deathDateSort     sql.NullTime
		version                          int64
		updatedAt                        time.Time
	)

	err := row.Scan(&id, &givenName, &surname, &fullName, &gender,
		&birthDateRaw, &birthDateSort, &birthPlace, &birthPlaceLat, &birthPlaceLong,
		&deathDateRaw, &deathDateSort, &deathPlace, &deathPlaceLat, &deathPlaceLong,
		&notes, &researchStatus, &brickWallNote, &brickWallSince, &brickWallResolvedAt,
		&version, &updatedAt)

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
		BrickWallNote:  brickWallNote.String,
		Version:        version,
		UpdatedAt:      updatedAt,
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
		p.BirthDateSort = &birthDateSort.Time
	}
	if deathDateSort.Valid {
		p.DeathDateSort = &deathDateSort.Time
	}
	if brickWallSince.Valid {
		p.BrickWallSince = &brickWallSince.Time
	}
	if brickWallResolvedAt.Valid {
		p.BrickWallResolvedAt = &brickWallResolvedAt.Time
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
		partner1GivenName, partner1Surname      sql.NullString
		partner2GivenName, partner2Surname      sql.NullString
		relType, marriageDateRaw, marriagePlace sql.NullString
		marriagePlaceLat, marriagePlaceLong     sql.NullString
		marriageDateSort                        sql.NullTime
		childCount                              int
		version                                 int64
		updatedAt                               time.Time
	)

	err := row.Scan(&id,
		&partner1ID, &partner1GivenName, &partner1Surname,
		&partner2ID, &partner2GivenName, &partner2Surname,
		&relType, &marriageDateRaw, &marriageDateSort, &marriagePlace,
		&marriagePlaceLat, &marriagePlaceLong,
		&childCount, &version, &updatedAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan family: %w", err)
	}

	f := &repository.FamilyReadModel{
		ID:                id,
		Partner1GivenName: partner1GivenName.String,
		Partner1Surname:   partner1Surname.String,
		Partner2GivenName: partner2GivenName.String,
		Partner2Surname:   partner2Surname.String,
		RelationshipType:  domain.RelationType(relType.String),
		MarriageDateRaw:   marriageDateRaw.String,
		MarriagePlace:     marriagePlace.String,
		ChildCount:        childCount,
		Version:           version,
		UpdatedAt:         updatedAt,
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
	// Set coordinate pointers if values are present
	if marriagePlaceLat.Valid && marriagePlaceLat.String != "" {
		f.MarriagePlaceLat = &marriagePlaceLat.String
	}
	if marriagePlaceLong.Valid && marriagePlaceLong.String != "" {
		f.MarriagePlaceLong = &marriagePlaceLong.String
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

func nullableStringPtr(s *string) sql.NullString {
	if s == nil || *s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: *s, Valid: true}
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

func nullableBytes(b []byte) any {
	if len(b) == 0 {
		return nil
	}
	return b
}

// GetSource retrieves a source by ID.
func (s *ReadModelStore) GetSource(ctx context.Context, id uuid.UUID) (*repository.SourceReadModel, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, source_type, title, author, publisher, publish_date_raw, publish_date_sort,
			   url, repository_id, repository_name, collection_name, call_number, notes, gedcom_xref,
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
			   url, repository_id, repository_name, collection_name, call_number, notes, gedcom_xref,
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
			   url, repository_id, repository_name, collection_name, call_number, notes, gedcom_xref,
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
							 url, repository_id, repository_name, collection_name, call_number, notes, gedcom_xref,
							 citation_count, version, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)
		ON CONFLICT(id) DO UPDATE SET
			source_type = EXCLUDED.source_type,
			title = EXCLUDED.title,
			author = EXCLUDED.author,
			publisher = EXCLUDED.publisher,
			publish_date_raw = EXCLUDED.publish_date_raw,
			publish_date_sort = EXCLUDED.publish_date_sort,
			url = EXCLUDED.url,
			repository_id = EXCLUDED.repository_id,
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
		nullableString(source.URL), nullableUUID(source.RepositoryID), nullableString(source.RepositoryName),
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
			   template_id, fields_data, gedcom_xref, version, created_at
		FROM citations WHERE id = $1
	`, id)

	return scanCitationRow(row)
}

// GetCitationsForSource returns all citations for a source.
func (s *ReadModelStore) GetCitationsForSource(ctx context.Context, sourceID uuid.UUID) ([]repository.CitationReadModel, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, source_id, source_title, fact_type, fact_owner_id, page, volume,
			   source_quality, informant_type, evidence_type, quoted_text, analysis,
			   template_id, fields_data, gedcom_xref, version, created_at
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
			   template_id, fields_data, gedcom_xref, version, created_at
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
			   template_id, fields_data, gedcom_xref, version, created_at
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
			   template_id, fields_data, gedcom_xref, version, created_at
		FROM citations
		ORDER BY source_title ASC, fact_type ASC, id ASC
		LIMIT $1 OFFSET $2
	`

	rows, err := s.db.QueryContext(ctx, query, opts.Limit, opts.Offset)
	if err != nil {
		return nil, 0, fmt.Errorf("query citations: %w", err)
	}
	defer rows.Close()

	var citations []repository.CitationReadModel
	for rows.Next() {
		cit, err := scanCitationRows(rows)
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
							   template_id, fields_data, gedcom_xref, version, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)
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
			fields_data = EXCLUDED.fields_data,
			gedcom_xref = EXCLUDED.gedcom_xref,
			version = EXCLUDED.version
	`, citation.ID, citation.SourceID, nullableString(citation.SourceTitle),
		nullableString(string(citation.FactType)), citation.FactOwnerID,
		nullableString(citation.Page), nullableString(citation.Volume),
		nullableString(string(citation.SourceQuality)), nullableString(string(citation.InformantType)),
		nullableString(string(citation.EvidenceType)), nullableString(citation.QuotedText),
		nullableString(citation.Analysis), nullableString(citation.TemplateID),
		nullableString(citation.FieldsJSON), nullableString(citation.GedcomXref),
		citation.Version, citation.CreatedAt)

	return err
}

// DeleteCitation removes a citation.
func (s *ReadModelStore) DeleteCitation(ctx context.Context, id uuid.UUID) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM citations WHERE id = $1", id)
	return err
}

// GetEvent retrieves an event by ID.
func (s *ReadModelStore) GetEvent(ctx context.Context, id uuid.UUID) (*repository.EventReadModel, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, owner_type, owner_id, fact_type, date_raw, date_sort,
		       place, place_lat, place_long, address, description, cause,
		       age, research_status, is_negated, version, created_at
		FROM events WHERE id = $1
	`, id)

	return scanEventRow(row)
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
		       age, research_status, is_negated, version, created_at
		FROM events
		ORDER BY fact_type ASC, date_sort ASC NULLS LAST, id ASC
		LIMIT $1 OFFSET $2
	`

	rows, err := s.db.QueryContext(ctx, query, opts.Limit, opts.Offset)
	if err != nil {
		return nil, 0, fmt.Errorf("query events: %w", err)
	}
	defer rows.Close()

	var events []repository.EventReadModel
	for rows.Next() {
		event, err := scanEventRows(rows)
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
		       age, research_status, is_negated, version, created_at
		FROM events
		WHERE owner_type = 'person' AND owner_id = $1
		ORDER BY fact_type ASC, date_sort ASC NULLS LAST, id ASC
	`, personID)
	if err != nil {
		return nil, fmt.Errorf("query events for person: %w", err)
	}
	defer rows.Close()

	var events []repository.EventReadModel
	for rows.Next() {
		event, err := scanEventRows(rows)
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
		       age, research_status, is_negated, version, created_at
		FROM events
		WHERE owner_type = 'family' AND owner_id = $1
		ORDER BY fact_type ASC, date_sort ASC NULLS LAST, id ASC
	`, familyID)
	if err != nil {
		return nil, fmt.Errorf("query events for family: %w", err)
	}
	defer rows.Close()

	var events []repository.EventReadModel
	for rows.Next() {
		event, err := scanEventRows(rows)
		if err != nil {
			return nil, err
		}
		events = append(events, *event)
	}

	return events, rows.Err()
}

// SaveEvent saves or updates an event.
func (s *ReadModelStore) SaveEvent(ctx context.Context, event *repository.EventReadModel) error {
	var addressJSON interface{}
	if event.Address != nil {
		if data, err := json.Marshal(event.Address); err == nil {
			addressJSON = string(data)
		}
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO events (id, owner_type, owner_id, fact_type, date_raw, date_sort,
		                    place, place_lat, place_long, address, description, cause,
		                    age, research_status, is_negated, version, created_at)
		VALUES ($1, $2, $3, $4, NULLIF($5, ''), $6, NULLIF($7, ''), NULLIF($8, ''), NULLIF($9, ''),
		        $10, NULLIF($11, ''), NULLIF($12, ''), NULLIF($13, ''), NULLIF($14, ''), $15, $16, $17)
		ON CONFLICT (id) DO UPDATE SET
			owner_type = EXCLUDED.owner_type,
			owner_id = EXCLUDED.owner_id,
			fact_type = EXCLUDED.fact_type,
			date_raw = EXCLUDED.date_raw,
			date_sort = EXCLUDED.date_sort,
			place = EXCLUDED.place,
			place_lat = EXCLUDED.place_lat,
			place_long = EXCLUDED.place_long,
			address = EXCLUDED.address,
			description = EXCLUDED.description,
			cause = EXCLUDED.cause,
			age = EXCLUDED.age,
			research_status = EXCLUDED.research_status,
			is_negated = EXCLUDED.is_negated,
			version = EXCLUDED.version
	`, event.ID, event.OwnerType, event.OwnerID, string(event.FactType),
		event.DateRaw, nullableTime(event.DateSort), event.Place,
		nullableStringPtr(event.PlaceLat), nullableStringPtr(event.PlaceLong),
		addressJSON, event.Description, event.Cause, event.Age,
		nullableString(string(event.ResearchStatus)), event.IsNegated, event.Version, event.CreatedAt)
	if err != nil {
		return fmt.Errorf("save event: %w", err)
	}
	return nil
}

// DeleteEvent deletes an event by ID.
func (s *ReadModelStore) DeleteEvent(ctx context.Context, id uuid.UUID) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM events WHERE id = $1", id)
	if err != nil {
		return fmt.Errorf("delete event: %w", err)
	}
	return nil
}

// GetAttribute retrieves an attribute by ID.
func (s *ReadModelStore) GetAttribute(ctx context.Context, id uuid.UUID) (*repository.AttributeReadModel, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, person_id, fact_type, value, date_raw, date_sort, place, version, created_at
		FROM attributes WHERE id = $1
	`, id)

	return scanAttributeRow(row)
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
		LIMIT $1 OFFSET $2
	`

	rows, err := s.db.QueryContext(ctx, query, opts.Limit, opts.Offset)
	if err != nil {
		return nil, 0, fmt.Errorf("query attributes: %w", err)
	}
	defer rows.Close()

	var attributes []repository.AttributeReadModel
	for rows.Next() {
		attr, err := scanAttributeRows(rows)
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
		WHERE person_id = $1
		ORDER BY fact_type ASC, value ASC, id ASC
	`, personID)
	if err != nil {
		return nil, fmt.Errorf("query attributes for person: %w", err)
	}
	defer rows.Close()

	var attributes []repository.AttributeReadModel
	for rows.Next() {
		attr, err := scanAttributeRows(rows)
		if err != nil {
			return nil, err
		}
		attributes = append(attributes, *attr)
	}

	return attributes, rows.Err()
}

// SaveAttribute saves or updates an attribute.
func (s *ReadModelStore) SaveAttribute(ctx context.Context, attribute *repository.AttributeReadModel) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO attributes (id, person_id, fact_type, value, date_raw, date_sort, place, version, created_at)
		VALUES ($1, $2, $3, $4, NULLIF($5, ''), $6, NULLIF($7, ''), $8, $9)
		ON CONFLICT (id) DO UPDATE SET
			person_id = EXCLUDED.person_id,
			fact_type = EXCLUDED.fact_type,
			value = EXCLUDED.value,
			date_raw = EXCLUDED.date_raw,
			date_sort = EXCLUDED.date_sort,
			place = EXCLUDED.place,
			version = EXCLUDED.version
	`, attribute.ID, attribute.PersonID, string(attribute.FactType),
		attribute.Value, attribute.DateRaw, nullableTime(attribute.DateSort),
		attribute.Place, attribute.Version, attribute.CreatedAt)
	if err != nil {
		return fmt.Errorf("save attribute: %w", err)
	}
	return nil
}

// DeleteAttribute deletes an attribute by ID.
func (s *ReadModelStore) DeleteAttribute(ctx context.Context, id uuid.UUID) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM attributes WHERE id = $1", id)
	if err != nil {
		return fmt.Errorf("delete attribute: %w", err)
	}
	return nil
}

// Scanning functions for sources and citations

func scanSourceRow(row rowScanner) (*repository.SourceReadModel, error) {
	var (
		id                                uuid.UUID
		sourceType, title                 string
		author, publisher, publishDateRaw sql.NullString
		url, repoID, repoName, collName   sql.NullString
		callNum, notes, gedcomXref        sql.NullString
		publishDateSort                   sql.NullTime
		citationCount                     int
		version                           int64
		updatedAt                         time.Time
	)

	err := row.Scan(&id, &sourceType, &title, &author, &publisher, &publishDateRaw, &publishDateSort,
		&url, &repoID, &repoName, &collName, &callNum, &notes, &gedcomXref,
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

	if repoID.Valid {
		if rid, err := uuid.Parse(repoID.String); err == nil {
			src.RepositoryID = &rid
		}
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
		fieldsData, gedcomXref           sql.NullString
		version                          int64
		createdAt                        time.Time
	)

	err := row.Scan(&id, &sourceID, &sourceTitle, &factType, &factOwnerID,
		&page, &volume, &sourceQuality, &informantType, &evidenceType,
		&quotedText, &analysis, &templateID, &fieldsData, &gedcomXref, &version, &createdAt)

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
		FieldsJSON:    fieldsData.String,
		GedcomXref:    gedcomXref.String,
		Version:       version,
		CreatedAt:     createdAt,
	}

	return cit, nil
}

func scanCitationRows(rows *sql.Rows) (*repository.CitationReadModel, error) {
	return scanCitationRow(rows)
}

func scanEventRow(row rowScanner) (*repository.EventReadModel, error) {
	var (
		id, ownerID             uuid.UUID
		ownerType, factType     string
		dateRaw, place          sql.NullString
		dateSort                sql.NullTime
		placeLat, placeLong     sql.NullString
		addressJSON             []byte
		description, cause, age sql.NullString
		researchStatus          sql.NullString
		isNegated               bool
		version                 int64
		createdAt               time.Time
	)

	err := row.Scan(&id, &ownerType, &ownerID, &factType, &dateRaw, &dateSort,
		&place, &placeLat, &placeLong, &addressJSON, &description, &cause,
		&age, &researchStatus, &isNegated, &version, &createdAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan event: %w", err)
	}

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
		IsNegated:   isNegated,
		Version:     version,
		CreatedAt:   createdAt,
	}

	if dateSort.Valid {
		event.DateSort = &dateSort.Time
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

	return event, nil
}

func scanEventRows(rows *sql.Rows) (*repository.EventReadModel, error) {
	return scanEventRow(rows)
}

func scanAttributeRow(row rowScanner) (*repository.AttributeReadModel, error) {
	var (
		id, personID    uuid.UUID
		factType, value string
		dateRaw, place  sql.NullString
		dateSort        sql.NullTime
		version         int64
		createdAt       time.Time
	)

	err := row.Scan(&id, &personID, &factType, &value, &dateRaw, &dateSort,
		&place, &version, &createdAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan attribute: %w", err)
	}

	attr := &repository.AttributeReadModel{
		ID:        id,
		PersonID:  personID,
		FactType:  domain.FactType(factType),
		Value:     value,
		DateRaw:   dateRaw.String,
		Place:     place.String,
		Version:   version,
		CreatedAt: createdAt,
	}

	if dateSort.Valid {
		attr.DateSort = &dateSort.Time
	}

	return attr, nil
}

func scanAttributeRows(rows *sql.Rows) (*repository.AttributeReadModel, error) {
	return scanAttributeRow(rows)
}

// GetMedia retrieves media metadata by ID (excludes FileData and ThumbnailData).
func (s *ReadModelStore) GetMedia(ctx context.Context, id uuid.UUID) (*repository.MediaReadModel, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, entity_type, entity_id, title, description, mime_type, media_type,
			   filename, file_size, crop_left, crop_top, crop_width, crop_height,
			   gedcom_xref, version, created_at, updated_at,
			   files, format, translations
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
			   gedcom_xref, version, created_at, updated_at,
			   files, format, translations
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
			   gedcom_xref, version, created_at, updated_at,
			   files, format, translations
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
	// Serialize JSONB fields
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
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22)
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
			updated_at = EXCLUDED.updated_at,
			files = EXCLUDED.files,
			format = EXCLUDED.format,
			translations = EXCLUDED.translations
	`, media.ID, media.EntityType, media.EntityID, media.Title,
		nullableString(media.Description), media.MimeType, string(media.MediaType),
		media.Filename, media.FileSize, media.FileData, media.ThumbnailData,
		nullableInt(media.CropLeft), nullableInt(media.CropTop),
		nullableInt(media.CropWidth), nullableInt(media.CropHeight),
		nullableString(media.GedcomXref), media.Version, media.CreatedAt, media.UpdatedAt,
		nullableBytes(filesJSON), nullableString(media.Format), nullableBytes(translationsJSON))

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
		// GEDCOM 7.0 enhanced fields
		filesJSON, translationsJSON []byte
		format                      sql.NullString
	)

	err := row.Scan(&id, &entityType, &entityID, &title, &description,
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

	// Deserialize JSONB fields
	files, err := domain.UnmarshalFilesFromJSON(filesJSON)
	if err != nil {
		return nil, fmt.Errorf("unmarshal files: %w", err)
	}
	translations, err := domain.UnmarshalTranslationsFromJSON(translationsJSON)
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
		CreatedAt:    createdAt,
		UpdatedAt:    updatedAt,
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
		// GEDCOM 7.0 enhanced fields
		filesJSON, translationsJSON []byte
		format                      sql.NullString
	)

	err := row.Scan(&id, &entityType, &entityID, &title, &description,
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

	// Deserialize JSONB fields
	files, err := domain.UnmarshalFilesFromJSON(filesJSON)
	if err != nil {
		return nil, fmt.Errorf("unmarshal files: %w", err)
	}
	translations, err := domain.UnmarshalTranslationsFromJSON(translationsJSON)
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
		CreatedAt:     createdAt,
		UpdatedAt:     updatedAt,
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
			   birth_date_raw, birth_date_sort, birth_place, birth_place_lat, birth_place_long,
			   death_date_raw, death_date_sort, death_place, death_place_lat, death_place_long,
			   notes, research_status, brick_wall_note, brick_wall_since, brick_wall_resolved_at,
			   version, updated_at
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
			   birth_date_raw, birth_date_sort, birth_place, birth_place_lat, birth_place_long,
			   death_date_raw, death_date_sort, death_place, death_place_lat, death_place_long,
			   notes, research_status, brick_wall_note, brick_wall_since, brick_wall_resolved_at,
			   version, updated_at
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

// GetCemeteryIndex returns unique burial/cremation places with person counts.
func (s *ReadModelStore) GetCemeteryIndex(ctx context.Context) ([]repository.CemeteryEntry, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT place, COUNT(DISTINCT owner_id) as count
		FROM events
		WHERE fact_type IN ($1, $2) AND place != '' AND place IS NOT NULL
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
		WHERE e.fact_type IN ($1, $2) AND LOWER(e.place) = LOWER($3)
	`, string(domain.FactPersonBurial), string(domain.FactPersonCremation), place).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("count persons by cemetery: %w", err)
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT DISTINCT p.id, p.given_name, p.surname, p.full_name, p.gender,
			   p.birth_date_raw, p.birth_date_sort, p.birth_place, p.birth_place_lat, p.birth_place_long,
			   p.death_date_raw, p.death_date_sort, p.death_place, p.death_place_lat, p.death_place_long,
			   p.notes, p.research_status, p.brick_wall_note, p.brick_wall_since, p.brick_wall_resolved_at,
			   p.version, p.updated_at
		FROM persons p
		INNER JOIN events e ON e.owner_id = p.id
		WHERE e.fact_type IN ($1, $2) AND LOWER(e.place) = LOWER($3)
		ORDER BY p.surname ASC, p.given_name ASC
		LIMIT $4 OFFSET $5
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

// GetMapLocations returns aggregated geographic locations from person birth/death coordinates.
func (s *ReadModelStore) GetMapLocations(ctx context.Context) ([]repository.MapLocation, error) {
	// Query birth locations — individual rows, aggregate in Go
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, birth_place, birth_place_lat, birth_place_long
		FROM persons
		WHERE birth_place_lat IS NOT NULL AND birth_place_long IS NOT NULL
		  AND birth_place_lat != '' AND birth_place_long != ''
		ORDER BY birth_place ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("query birth map locations: %w", err)
	}
	defer rows.Close()

	type locKey struct {
		place     string
		eventType string
	}
	type locData struct {
		lat       float64
		lon       float64
		personIDs []uuid.UUID
	}
	agg := make(map[locKey]*locData)

	for rows.Next() {
		var personID uuid.UUID
		var place, latStr, lonStr string
		if err := rows.Scan(&personID, &place, &latStr, &lonStr); err != nil {
			return nil, fmt.Errorf("scan birth map location: %w", err)
		}
		lat, errLat := gedcom.ParseCoordinate(latStr)
		lon, errLon := gedcom.ParseCoordinate(lonStr)
		if errLat != nil || errLon != nil {
			continue
		}
		key := locKey{place: place, eventType: "birth"}
		if d, ok := agg[key]; ok {
			d.personIDs = append(d.personIDs, personID)
		} else {
			agg[key] = &locData{lat: lat, lon: lon, personIDs: []uuid.UUID{personID}}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Query death locations
	rows2, err := s.db.QueryContext(ctx, `
		SELECT id, death_place, death_place_lat, death_place_long
		FROM persons
		WHERE death_place_lat IS NOT NULL AND death_place_long IS NOT NULL
		  AND death_place_lat != '' AND death_place_long != ''
		ORDER BY death_place ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("query death map locations: %w", err)
	}
	defer rows2.Close()

	for rows2.Next() {
		var personID uuid.UUID
		var place, latStr, lonStr string
		if err := rows2.Scan(&personID, &place, &latStr, &lonStr); err != nil {
			return nil, fmt.Errorf("scan death map location: %w", err)
		}
		lat, errLat := gedcom.ParseCoordinate(latStr)
		lon, errLon := gedcom.ParseCoordinate(lonStr)
		if errLat != nil || errLon != nil {
			continue
		}
		key := locKey{place: place, eventType: "death"}
		if d, ok := agg[key]; ok {
			d.personIDs = append(d.personIDs, personID)
		} else {
			agg[key] = &locData{lat: lat, lon: lon, personIDs: []uuid.UUID{personID}}
		}
	}
	if err := rows2.Err(); err != nil {
		return nil, err
	}

	results := make([]repository.MapLocation, 0, len(agg))
	for key, data := range agg {
		results = append(results, repository.MapLocation{
			Place:     key.place,
			Latitude:  data.lat,
			Longitude: data.lon,
			EventType: key.eventType,
			Count:     len(data.personIDs),
			PersonIDs: data.personIDs,
		})
	}
	sort.Slice(results, func(i, j int) bool {
		if results[i].Place != results[j].Place {
			return results[i].Place < results[j].Place
		}
		return results[i].EventType < results[j].EventType
	})

	return results, nil
}

// SetBrickWall marks a person as a brick wall with a note.
func (s *ReadModelStore) SetBrickWall(ctx context.Context, personID uuid.UUID, note string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE persons SET brick_wall_note = $1, brick_wall_since = NOW(), brick_wall_resolved_at = NULL
		WHERE id = $2
	`, note, personID)
	return err
}

// ResolveBrickWall marks a brick wall as resolved.
func (s *ReadModelStore) ResolveBrickWall(ctx context.Context, personID uuid.UUID) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE persons SET brick_wall_resolved_at = NOW()
		WHERE id = $1
	`, personID)
	return err
}

// GetBrickWalls returns persons with brick wall status.
func (s *ReadModelStore) GetBrickWalls(ctx context.Context, includeResolved bool) ([]repository.BrickWallEntry, error) {
	query := `
		SELECT id, full_name, brick_wall_note, brick_wall_since, brick_wall_resolved_at
		FROM persons
		WHERE brick_wall_since IS NOT NULL`
	if !includeResolved {
		query += ` AND brick_wall_resolved_at IS NULL`
	}
	query += ` ORDER BY brick_wall_since DESC`

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query brick walls: %w", err)
	}
	defer rows.Close()

	var entries []repository.BrickWallEntry
	for rows.Next() {
		var (
			id         uuid.UUID
			fullName   string
			note       sql.NullString
			since      time.Time
			resolvedAt sql.NullTime
		)
		if err := rows.Scan(&id, &fullName, &note, &since, &resolvedAt); err != nil {
			return nil, fmt.Errorf("scan brick wall: %w", err)
		}
		entry := repository.BrickWallEntry{
			PersonID:   id,
			PersonName: fullName,
			Note:       note.String,
			Since:      since,
		}
		if resolvedAt.Valid {
			entry.ResolvedAt = &resolvedAt.Time
		}
		entries = append(entries, entry)
	}

	return entries, rows.Err()
}

// GetNote retrieves a note by ID.
func (s *ReadModelStore) GetNote(ctx context.Context, id uuid.UUID) (*repository.NoteReadModel, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, text, mime, language, translations, gedcom_xref, version, updated_at
		FROM notes WHERE id = $1
	`, id)

	var note repository.NoteReadModel
	var mime, language, gedcomXref sql.NullString
	var translations []byte
	err := row.Scan(
		&note.ID,
		&note.Text,
		&mime,
		&language,
		&translations,
		&gedcomXref,
		&note.Version,
		&note.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan note: %w", err)
	}
	note.MIME = mime.String
	note.Language = language.String
	note.Translations = repository.UnmarshalNoteTranslations(string(translations))
	if gedcomXref.Valid {
		note.GedcomXref = gedcomXref.String
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
		SELECT id, text, mime, language, translations, gedcom_xref, version, updated_at
		FROM notes
		ORDER BY %s %s
		LIMIT $1 OFFSET $2
	`, orderColumn, orderDir)

	rows, err := s.db.QueryContext(ctx, query, opts.Limit, opts.Offset)
	if err != nil {
		return nil, 0, fmt.Errorf("query notes: %w", err)
	}
	defer rows.Close()

	var notes []repository.NoteReadModel
	for rows.Next() {
		var note repository.NoteReadModel
		var mime, language, gedcomXref sql.NullString
		var translations []byte
		if err := rows.Scan(
			&note.ID,
			&note.Text,
			&mime,
			&language,
			&translations,
			&gedcomXref,
			&note.Version,
			&note.UpdatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("scan note: %w", err)
		}
		note.MIME = mime.String
		note.Language = language.String
		note.Translations = repository.UnmarshalNoteTranslations(string(translations))
		if gedcomXref.Valid {
			note.GedcomXref = gedcomXref.String
		}
		notes = append(notes, note)
	}

	return notes, total, rows.Err()
}

// SaveNote saves or updates a note.
func (s *ReadModelStore) SaveNote(ctx context.Context, note *repository.NoteReadModel) error {
	var translations any
	if len(note.Translations) > 0 {
		translations = repository.MarshalNoteTranslations(note.Translations)
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO notes (id, text, mime, language, translations, gedcom_xref, version, updated_at)
		VALUES ($1, $2, NULLIF($3, ''), NULLIF($4, ''), $5, NULLIF($6, ''), $7, $8)
		ON CONFLICT (id) DO UPDATE SET
			text = EXCLUDED.text,
			mime = EXCLUDED.mime,
			language = EXCLUDED.language,
			translations = EXCLUDED.translations,
			gedcom_xref = EXCLUDED.gedcom_xref,
			version = EXCLUDED.version,
			updated_at = EXCLUDED.updated_at
	`, note.ID, note.Text, note.MIME, note.Language, translations, note.GedcomXref, note.Version, note.UpdatedAt)
	if err != nil {
		return fmt.Errorf("save note: %w", err)
	}
	return nil
}

// DeleteNote deletes a note by ID.
func (s *ReadModelStore) DeleteNote(ctx context.Context, id uuid.UUID) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM notes WHERE id = $1", id)
	if err != nil {
		return fmt.Errorf("delete note: %w", err)
	}
	return nil
}

// GetSubmitter retrieves a submitter by ID.
func (s *ReadModelStore) GetSubmitter(ctx context.Context, id uuid.UUID) (*repository.SubmitterReadModel, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, name, address, phone, email, language, media_id, gedcom_xref, version, updated_at
		FROM submitters WHERE id = $1
	`, id)

	var submitter repository.SubmitterReadModel
	var addressJSON, phoneJSON, emailJSON []byte
	var gedcomXref sql.NullString
	var mediaID sql.NullString
	var language sql.NullString
	err := row.Scan(
		&submitter.ID,
		&submitter.Name,
		&addressJSON,
		&phoneJSON,
		&emailJSON,
		&language,
		&mediaID,
		&gedcomXref,
		&submitter.Version,
		&submitter.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan submitter: %w", err)
	}
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
		LIMIT $1 OFFSET $2
	`, orderColumn, orderDir)

	rows, err := s.db.QueryContext(ctx, query, opts.Limit, opts.Offset)
	if err != nil {
		return nil, 0, fmt.Errorf("query submitters: %w", err)
	}
	defer rows.Close()

	var submitters []repository.SubmitterReadModel
	for rows.Next() {
		var submitter repository.SubmitterReadModel
		var addressJSON, phoneJSON, emailJSON []byte
		var gedcomXref sql.NullString
		var mediaID sql.NullString
		var language sql.NullString
		if err := rows.Scan(
			&submitter.ID,
			&submitter.Name,
			&addressJSON,
			&phoneJSON,
			&emailJSON,
			&language,
			&mediaID,
			&gedcomXref,
			&submitter.Version,
			&submitter.UpdatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("scan submitter: %w", err)
		}
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

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO submitters (id, name, address, phone, email, language, media_id, gedcom_xref, version, updated_at)
		VALUES ($1, $2, $3, $4, $5, NULLIF($6, ''), $7, NULLIF($8, ''), $9, $10)
		ON CONFLICT (id) DO UPDATE SET
			name = EXCLUDED.name,
			address = EXCLUDED.address,
			phone = EXCLUDED.phone,
			email = EXCLUDED.email,
			language = EXCLUDED.language,
			media_id = EXCLUDED.media_id,
			gedcom_xref = EXCLUDED.gedcom_xref,
			version = EXCLUDED.version,
			updated_at = EXCLUDED.updated_at
	`, submitter.ID, submitter.Name, addressJSON, phoneJSON, emailJSON,
		submitter.Language, mediaID, submitter.GedcomXref, submitter.Version, submitter.UpdatedAt)
	if err != nil {
		return fmt.Errorf("save submitter: %w", err)
	}
	return nil
}

// DeleteSubmitter deletes a submitter by ID.
func (s *ReadModelStore) DeleteSubmitter(ctx context.Context, id uuid.UUID) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM submitters WHERE id = $1", id)
	if err != nil {
		return fmt.Errorf("delete submitter: %w", err)
	}
	return nil
}

// GetRepository retrieves a repository by ID.
func (s *ReadModelStore) GetRepository(ctx context.Context, id uuid.UUID) (*repository.RepositoryReadModel, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, name, address, notes, gedcom_xref, version, updated_at
		FROM repositories WHERE id = $1
	`, id)

	var repo repository.RepositoryReadModel
	var addressJSON []byte
	var notes, gedcomXref sql.NullString
	err := row.Scan(
		&repo.ID,
		&repo.Name,
		&addressJSON,
		&notes,
		&gedcomXref,
		&repo.Version,
		&repo.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan repository: %w", err)
	}
	if notes.Valid {
		repo.Notes = notes.String
	}
	if gedcomXref.Valid {
		repo.GedcomXref = gedcomXref.String
	}
	if len(addressJSON) > 0 {
		var addr domain.Address
		if err := json.Unmarshal(addressJSON, &addr); err == nil {
			repo.Address = &addr
		}
	}
	return &repo, nil
}

// ListRepositories returns a paginated list of repositories.
func (s *ReadModelStore) ListRepositories(ctx context.Context, opts repository.ListOptions) ([]repository.RepositoryReadModel, int, error) {
	// Count total
	var total int
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM repositories").Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("count repositories: %w", err)
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
	// id is a stable tie-breaker so LIMIT/OFFSET pagination is deterministic when sort keys collide.
	query := fmt.Sprintf(`
		SELECT id, name, address, notes, gedcom_xref, version, updated_at
		FROM repositories
		ORDER BY %s %s, id %s
		LIMIT $1 OFFSET $2
	`, orderColumn, orderDir, orderDir)

	rows, err := s.db.QueryContext(ctx, query, opts.Limit, opts.Offset)
	if err != nil {
		return nil, 0, fmt.Errorf("query repositories: %w", err)
	}
	defer rows.Close()

	var repositories []repository.RepositoryReadModel
	for rows.Next() {
		var repo repository.RepositoryReadModel
		var addressJSON []byte
		var notes, gedcomXref sql.NullString
		if err := rows.Scan(
			&repo.ID,
			&repo.Name,
			&addressJSON,
			&notes,
			&gedcomXref,
			&repo.Version,
			&repo.UpdatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("scan repository: %w", err)
		}
		if notes.Valid {
			repo.Notes = notes.String
		}
		if gedcomXref.Valid {
			repo.GedcomXref = gedcomXref.String
		}
		if len(addressJSON) > 0 {
			var addr domain.Address
			if err := json.Unmarshal(addressJSON, &addr); err == nil {
				repo.Address = &addr
			}
		}
		repositories = append(repositories, repo)
	}

	return repositories, total, rows.Err()
}

// SaveRepository saves or updates a repository.
func (s *ReadModelStore) SaveRepository(ctx context.Context, repo *repository.RepositoryReadModel) error {
	var addressJSON []byte
	var err error

	if repo.Address != nil {
		addressJSON, err = json.Marshal(repo.Address)
		if err != nil {
			return fmt.Errorf("marshal address: %w", err)
		}
	}

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO repositories (id, name, address, notes, gedcom_xref, version, updated_at)
		VALUES ($1, $2, $3, NULLIF($4, ''), NULLIF($5, ''), $6, $7)
		ON CONFLICT (id) DO UPDATE SET
			name = EXCLUDED.name,
			address = EXCLUDED.address,
			notes = EXCLUDED.notes,
			gedcom_xref = EXCLUDED.gedcom_xref,
			version = EXCLUDED.version,
			updated_at = EXCLUDED.updated_at
	`, repo.ID, repo.Name, addressJSON, repo.Notes, repo.GedcomXref, repo.Version, repo.UpdatedAt)
	if err != nil {
		return fmt.Errorf("save repository: %w", err)
	}
	return nil
}

// DeleteRepository deletes a repository by ID.
func (s *ReadModelStore) DeleteRepository(ctx context.Context, id uuid.UUID) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM repositories WHERE id = $1", id)
	if err != nil {
		return fmt.Errorf("delete repository: %w", err)
	}
	return nil
}

// GetAssociation retrieves an association by ID.
func (s *ReadModelStore) GetAssociation(ctx context.Context, id uuid.UUID) (*repository.AssociationReadModel, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, person_id, person_name, associate_id, associate_name,
		       role, phrase, notes, note_ids, gedcom_xref, version, updated_at
		FROM associations WHERE id = $1
	`, id)

	var assoc repository.AssociationReadModel
	var personName, associateName, phrase, notes sql.NullString
	var noteIDsJSON []byte
	var gedcomXref sql.NullString
	err := row.Scan(
		&assoc.ID,
		&assoc.PersonID,
		&personName,
		&assoc.AssociateID,
		&associateName,
		&assoc.Role,
		&phrase,
		&notes,
		&noteIDsJSON,
		&gedcomXref,
		&assoc.Version,
		&assoc.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan association: %w", err)
	}
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
	if len(noteIDsJSON) > 0 {
		_ = json.Unmarshal(noteIDsJSON, &assoc.NoteIDs)
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
		LIMIT $1 OFFSET $2
	`, orderColumn, orderDir)

	rows, err := s.db.QueryContext(ctx, query, opts.Limit, opts.Offset)
	if err != nil {
		return nil, 0, fmt.Errorf("query associations: %w", err)
	}
	defer rows.Close()

	var associations []repository.AssociationReadModel
	for rows.Next() {
		var assoc repository.AssociationReadModel
		var personName, associateName, phrase, notes sql.NullString
		var noteIDsJSON []byte
		var gedcomXref sql.NullString
		if err := rows.Scan(
			&assoc.ID,
			&assoc.PersonID,
			&personName,
			&assoc.AssociateID,
			&associateName,
			&assoc.Role,
			&phrase,
			&notes,
			&noteIDsJSON,
			&gedcomXref,
			&assoc.Version,
			&assoc.UpdatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("scan association: %w", err)
		}
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
		if len(noteIDsJSON) > 0 {
			_ = json.Unmarshal(noteIDsJSON, &assoc.NoteIDs)
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
		WHERE person_id = $1 OR associate_id = $1
		ORDER BY role, updated_at DESC
	`, personID)
	if err != nil {
		return nil, fmt.Errorf("query associations for person: %w", err)
	}
	defer rows.Close()

	var associations []repository.AssociationReadModel
	for rows.Next() {
		var assoc repository.AssociationReadModel
		var personName, associateName, phrase, notes sql.NullString
		var noteIDsJSON []byte
		var gedcomXref sql.NullString
		if err := rows.Scan(
			&assoc.ID,
			&assoc.PersonID,
			&personName,
			&assoc.AssociateID,
			&associateName,
			&assoc.Role,
			&phrase,
			&notes,
			&noteIDsJSON,
			&gedcomXref,
			&assoc.Version,
			&assoc.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan association: %w", err)
		}
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
		if len(noteIDsJSON) > 0 {
			_ = json.Unmarshal(noteIDsJSON, &assoc.NoteIDs)
		}
		associations = append(associations, assoc)
	}

	return associations, rows.Err()
}

// SaveAssociation saves or updates an association.
func (s *ReadModelStore) SaveAssociation(ctx context.Context, assoc *repository.AssociationReadModel) error {
	var noteIDsJSON []byte
	var err error

	if len(assoc.NoteIDs) > 0 {
		noteIDsJSON, err = json.Marshal(assoc.NoteIDs)
		if err != nil {
			return fmt.Errorf("marshal note_ids: %w", err)
		}
	}

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO associations (id, person_id, person_name, associate_id, associate_name,
		                         role, phrase, notes, note_ids, gedcom_xref, version, updated_at)
		VALUES ($1, $2, NULLIF($3, ''), $4, NULLIF($5, ''), $6, NULLIF($7, ''), NULLIF($8, ''), $9, NULLIF($10, ''), $11, $12)
		ON CONFLICT (id) DO UPDATE SET
			person_id = EXCLUDED.person_id,
			person_name = EXCLUDED.person_name,
			associate_id = EXCLUDED.associate_id,
			associate_name = EXCLUDED.associate_name,
			role = EXCLUDED.role,
			phrase = EXCLUDED.phrase,
			notes = EXCLUDED.notes,
			note_ids = EXCLUDED.note_ids,
			gedcom_xref = EXCLUDED.gedcom_xref,
			version = EXCLUDED.version,
			updated_at = EXCLUDED.updated_at
	`, assoc.ID, assoc.PersonID, assoc.PersonName, assoc.AssociateID, assoc.AssociateName,
		assoc.Role, assoc.Phrase, assoc.Notes, noteIDsJSON, assoc.GedcomXref, assoc.Version, assoc.UpdatedAt)
	if err != nil {
		return fmt.Errorf("save association: %w", err)
	}
	return nil
}

// DeleteAssociation deletes an association by ID.
func (s *ReadModelStore) DeleteAssociation(ctx context.Context, id uuid.UUID) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM associations WHERE id = $1", id)
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
		FROM lds_ordinances WHERE id = $1
	`, id)

	var ordinance repository.LDSOrdinanceReadModel
	var personID, familyID sql.NullString
	var personName, dateRaw, place, temple, status sql.NullString
	var dateSort sql.NullTime
	err := row.Scan(
		&ordinance.ID,
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
		&ordinance.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan lds_ordinance: %w", err)
	}
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
		ordinance.DateSort = &dateSort.Time
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
		LIMIT $1 OFFSET $2
	`, orderColumn, orderDir)

	rows, err := s.db.QueryContext(ctx, query, opts.Limit, opts.Offset)
	if err != nil {
		return nil, 0, fmt.Errorf("query lds_ordinances: %w", err)
	}
	defer rows.Close()

	var ordinances []repository.LDSOrdinanceReadModel
	for rows.Next() {
		var ordinance repository.LDSOrdinanceReadModel
		var personID, familyID sql.NullString
		var personName, dateRaw, place, temple, status sql.NullString
		var dateSort sql.NullTime
		if err := rows.Scan(
			&ordinance.ID,
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
			&ordinance.UpdatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("scan lds_ordinance: %w", err)
		}
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
			ordinance.DateSort = &dateSort.Time
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
		ordinances = append(ordinances, ordinance)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate lds_ordinances: %w", err)
	}

	return ordinances, total, nil
}

// ListLDSOrdinancesForPerson returns all LDS ordinances for a given person.
func (s *ReadModelStore) ListLDSOrdinancesForPerson(ctx context.Context, personID uuid.UUID) ([]repository.LDSOrdinanceReadModel, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, type, type_label, person_id, person_name, family_id,
		       date_raw, date_sort, place, temple, status, version, updated_at
		FROM lds_ordinances
		WHERE person_id = $1
		ORDER BY type, date_sort
	`, personID)
	if err != nil {
		return nil, fmt.Errorf("query lds_ordinances for person: %w", err)
	}
	defer rows.Close()

	var ordinances []repository.LDSOrdinanceReadModel
	for rows.Next() {
		var ordinance repository.LDSOrdinanceReadModel
		var personIDNull, familyID sql.NullString
		var personName, dateRaw, place, temple, status sql.NullString
		var dateSort sql.NullTime
		if err := rows.Scan(
			&ordinance.ID,
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
			&ordinance.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan lds_ordinance: %w", err)
		}
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
			ordinance.DateSort = &dateSort.Time
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
		ordinances = append(ordinances, ordinance)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate lds_ordinances: %w", err)
	}

	return ordinances, nil
}

// ListLDSOrdinancesForFamily returns all LDS ordinances for a given family.
func (s *ReadModelStore) ListLDSOrdinancesForFamily(ctx context.Context, familyID uuid.UUID) ([]repository.LDSOrdinanceReadModel, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, type, type_label, person_id, person_name, family_id,
		       date_raw, date_sort, place, temple, status, version, updated_at
		FROM lds_ordinances
		WHERE family_id = $1
		ORDER BY type, date_sort
	`, familyID)
	if err != nil {
		return nil, fmt.Errorf("query lds_ordinances for family: %w", err)
	}
	defer rows.Close()

	var ordinances []repository.LDSOrdinanceReadModel
	for rows.Next() {
		var ordinance repository.LDSOrdinanceReadModel
		var personID, familyIDNull sql.NullString
		var personName, dateRaw, place, temple, status sql.NullString
		var dateSort sql.NullTime
		if err := rows.Scan(
			&ordinance.ID,
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
			&ordinance.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan lds_ordinance: %w", err)
		}
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
			ordinance.DateSort = &dateSort.Time
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
		ordinances = append(ordinances, ordinance)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate lds_ordinances: %w", err)
	}

	return ordinances, nil
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

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO lds_ordinances (id, type, type_label, person_id, person_name, family_id,
		                           date_raw, date_sort, place, temple, status, version, updated_at)
		VALUES ($1, $2, $3, $4, NULLIF($5, ''), $6, NULLIF($7, ''), $8, NULLIF($9, ''), NULLIF($10, ''), NULLIF($11, ''), $12, $13)
		ON CONFLICT (id) DO UPDATE SET
			type = EXCLUDED.type,
			type_label = EXCLUDED.type_label,
			person_id = EXCLUDED.person_id,
			person_name = EXCLUDED.person_name,
			family_id = EXCLUDED.family_id,
			date_raw = EXCLUDED.date_raw,
			date_sort = EXCLUDED.date_sort,
			place = EXCLUDED.place,
			temple = EXCLUDED.temple,
			status = EXCLUDED.status,
			version = EXCLUDED.version,
			updated_at = EXCLUDED.updated_at
	`, ordinance.ID, ordinance.Type, ordinance.TypeLabel, personID, ordinance.PersonName, familyID,
		ordinance.DateRaw, ordinance.DateSort, ordinance.Place, ordinance.Temple, ordinance.Status,
		ordinance.Version, ordinance.UpdatedAt)
	if err != nil {
		return fmt.Errorf("save lds_ordinance: %w", err)
	}
	return nil
}

// DeleteLDSOrdinance deletes an LDS ordinance by ID.
func (s *ReadModelStore) DeleteLDSOrdinance(ctx context.Context, id uuid.UUID) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM lds_ordinances WHERE id = $1", id)
	if err != nil {
		return fmt.Errorf("delete lds_ordinance: %w", err)
	}
	return nil
}

// GetEvidenceAnalysis retrieves an evidence analysis by ID.
func (s *ReadModelStore) GetEvidenceAnalysis(ctx context.Context, id uuid.UUID) (*repository.EvidenceAnalysisReadModel, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, fact_type, subject_id, citation_ids, conclusion, research_status, notes, version, created_at, updated_at
		FROM evidence_analyses WHERE id = $1
	`, id)

	var a repository.EvidenceAnalysisReadModel
	var citationIDs, researchStatus, notes sql.NullString
	err := row.Scan(
		&a.ID, &a.FactType, &a.SubjectID, &citationIDs,
		&a.Conclusion, &researchStatus, &notes,
		&a.Version, &a.CreatedAt, &a.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan evidence_analysis: %w", err)
	}
	if citationIDs.Valid {
		a.CitationIDsJSON = citationIDs.String
	}
	if researchStatus.Valid {
		a.ResearchStatus = domain.ResearchStatus(researchStatus.String)
	}
	if notes.Valid {
		a.Notes = notes.String
	}
	return &a, nil
}

// ListEvidenceAnalyses returns a paginated list of evidence analyses.
func (s *ReadModelStore) ListEvidenceAnalyses(ctx context.Context, opts repository.ListOptions) ([]repository.EvidenceAnalysisReadModel, int, error) {
	var total int
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM evidence_analyses").Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("count evidence_analyses: %w", err)
	}

	orderDir := "DESC"
	if opts.Order == "asc" {
		orderDir = "ASC"
	}

	sortCol := "updated_at"
	if opts.Sort == "created_at" {
		sortCol = "created_at"
	}

	// #nosec G201 -- orderDir and sortCol are validated above, not user input
	query := fmt.Sprintf(`
		SELECT id, fact_type, subject_id, citation_ids, conclusion, research_status, notes, version, created_at, updated_at
		FROM evidence_analyses
		ORDER BY %s %s, id %s
		LIMIT $1 OFFSET $2
	`, sortCol, orderDir, orderDir)

	rows, err := s.db.QueryContext(ctx, query, opts.Limit, opts.Offset)
	if err != nil {
		return nil, 0, fmt.Errorf("query evidence_analyses: %w", err)
	}
	defer rows.Close()

	var results []repository.EvidenceAnalysisReadModel
	for rows.Next() {
		var a repository.EvidenceAnalysisReadModel
		var citationIDs, researchStatus, notes sql.NullString
		if err := rows.Scan(
			&a.ID, &a.FactType, &a.SubjectID, &citationIDs,
			&a.Conclusion, &researchStatus, &notes,
			&a.Version, &a.CreatedAt, &a.UpdatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("scan evidence_analysis: %w", err)
		}
		if citationIDs.Valid {
			a.CitationIDsJSON = citationIDs.String
		}
		if researchStatus.Valid {
			a.ResearchStatus = domain.ResearchStatus(researchStatus.String)
		}
		if notes.Valid {
			a.Notes = notes.String
		}
		results = append(results, a)
	}

	return results, total, rows.Err()
}

// GetAnalysesForFact returns all evidence analyses for a given fact type and subject.
func (s *ReadModelStore) GetAnalysesForFact(ctx context.Context, factType domain.FactType, subjectID uuid.UUID) ([]repository.EvidenceAnalysisReadModel, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, fact_type, subject_id, citation_ids, conclusion, research_status, notes, version, created_at, updated_at
		FROM evidence_analyses
		WHERE fact_type = $1 AND subject_id = $2
	`, string(factType), subjectID)
	if err != nil {
		return nil, fmt.Errorf("query analyses for fact: %w", err)
	}
	defer rows.Close()

	var results []repository.EvidenceAnalysisReadModel
	for rows.Next() {
		var a repository.EvidenceAnalysisReadModel
		var citationIDs, researchStatus, notes sql.NullString
		if err := rows.Scan(
			&a.ID, &a.FactType, &a.SubjectID, &citationIDs,
			&a.Conclusion, &researchStatus, &notes,
			&a.Version, &a.CreatedAt, &a.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan evidence_analysis: %w", err)
		}
		if citationIDs.Valid {
			a.CitationIDsJSON = citationIDs.String
		}
		if researchStatus.Valid {
			a.ResearchStatus = domain.ResearchStatus(researchStatus.String)
		}
		if notes.Valid {
			a.Notes = notes.String
		}
		results = append(results, a)
	}

	return results, rows.Err()
}

// GetAnalysesBySubject returns all evidence analyses for a given subject, regardless of fact type.
func (s *ReadModelStore) GetAnalysesBySubject(ctx context.Context, subjectID uuid.UUID) ([]repository.EvidenceAnalysisReadModel, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, fact_type, subject_id, citation_ids, conclusion, research_status, notes, version, created_at, updated_at
		FROM evidence_analyses
		WHERE subject_id = $1
	`, subjectID)
	if err != nil {
		return nil, fmt.Errorf("query analyses by subject: %w", err)
	}
	defer rows.Close()

	var results []repository.EvidenceAnalysisReadModel
	for rows.Next() {
		var a repository.EvidenceAnalysisReadModel
		var citationIDs, researchStatus, notes sql.NullString
		if err := rows.Scan(
			&a.ID, &a.FactType, &a.SubjectID, &citationIDs,
			&a.Conclusion, &researchStatus, &notes,
			&a.Version, &a.CreatedAt, &a.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan evidence_analysis: %w", err)
		}
		if citationIDs.Valid {
			a.CitationIDsJSON = citationIDs.String
		}
		if researchStatus.Valid {
			a.ResearchStatus = domain.ResearchStatus(researchStatus.String)
		}
		if notes.Valid {
			a.Notes = notes.String
		}
		results = append(results, a)
	}

	return results, rows.Err()
}

// SaveEvidenceAnalysis saves or updates an evidence analysis.
func (s *ReadModelStore) SaveEvidenceAnalysis(ctx context.Context, analysis *repository.EvidenceAnalysisReadModel) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO evidence_analyses (id, fact_type, subject_id, citation_ids, conclusion, research_status, notes, version, created_at, updated_at)
		VALUES ($1, $2, $3, NULLIF($4, '')::JSONB, $5, NULLIF($6, ''), NULLIF($7, ''), $8, $9, $10)
		ON CONFLICT (id) DO UPDATE SET
			fact_type = EXCLUDED.fact_type,
			subject_id = EXCLUDED.subject_id,
			citation_ids = EXCLUDED.citation_ids,
			conclusion = EXCLUDED.conclusion,
			research_status = EXCLUDED.research_status,
			notes = EXCLUDED.notes,
			version = EXCLUDED.version,
			updated_at = EXCLUDED.updated_at
	`, analysis.ID, analysis.FactType, analysis.SubjectID, analysis.CitationIDsJSON,
		analysis.Conclusion, string(analysis.ResearchStatus), analysis.Notes,
		analysis.Version, analysis.CreatedAt, analysis.UpdatedAt)
	if err != nil {
		return fmt.Errorf("save evidence_analysis: %w", err)
	}
	return nil
}

// DeleteEvidenceAnalysis deletes an evidence analysis by ID.
func (s *ReadModelStore) DeleteEvidenceAnalysis(ctx context.Context, id uuid.UUID) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM evidence_analyses WHERE id = $1", id)
	if err != nil {
		return fmt.Errorf("delete evidence_analysis: %w", err)
	}
	return nil
}

// GetEvidenceConflict retrieves an evidence conflict by ID.
func (s *ReadModelStore) GetEvidenceConflict(ctx context.Context, id uuid.UUID) (*repository.EvidenceConflictReadModel, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, fact_type, subject_id, analysis_ids, description, resolution, status, version, created_at, updated_at
		FROM evidence_conflicts WHERE id = $1
	`, id)

	var c repository.EvidenceConflictReadModel
	var analysisIDs, resolution sql.NullString
	err := row.Scan(
		&c.ID, &c.FactType, &c.SubjectID, &analysisIDs,
		&c.Description, &resolution, &c.Status,
		&c.Version, &c.CreatedAt, &c.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan evidence_conflict: %w", err)
	}
	if analysisIDs.Valid {
		c.AnalysisIDsJSON = analysisIDs.String
	}
	if resolution.Valid {
		c.Resolution = resolution.String
	}
	return &c, nil
}

// ListEvidenceConflicts returns a paginated list of evidence conflicts.
func (s *ReadModelStore) ListEvidenceConflicts(ctx context.Context, opts repository.ListOptions) ([]repository.EvidenceConflictReadModel, int, error) {
	var total int
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM evidence_conflicts").Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("count evidence_conflicts: %w", err)
	}

	orderDir := "DESC"
	if opts.Order == "asc" {
		orderDir = "ASC"
	}

	sortCol := "updated_at"
	if opts.Sort == "created_at" {
		sortCol = "created_at"
	}

	// #nosec G201 -- orderDir and sortCol are validated above, not user input
	query := fmt.Sprintf(`
		SELECT id, fact_type, subject_id, analysis_ids, description, resolution, status, version, created_at, updated_at
		FROM evidence_conflicts
		ORDER BY %s %s, id %s
		LIMIT $1 OFFSET $2
	`, sortCol, orderDir, orderDir)

	rows, err := s.db.QueryContext(ctx, query, opts.Limit, opts.Offset)
	if err != nil {
		return nil, 0, fmt.Errorf("query evidence_conflicts: %w", err)
	}
	defer rows.Close()

	var results []repository.EvidenceConflictReadModel
	for rows.Next() {
		var c repository.EvidenceConflictReadModel
		var analysisIDs, resolution sql.NullString
		if err := rows.Scan(
			&c.ID, &c.FactType, &c.SubjectID, &analysisIDs,
			&c.Description, &resolution, &c.Status,
			&c.Version, &c.CreatedAt, &c.UpdatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("scan evidence_conflict: %w", err)
		}
		if analysisIDs.Valid {
			c.AnalysisIDsJSON = analysisIDs.String
		}
		if resolution.Valid {
			c.Resolution = resolution.String
		}
		results = append(results, c)
	}

	return results, total, rows.Err()
}

// GetConflictsForSubject returns all evidence conflicts for a given subject.
func (s *ReadModelStore) GetConflictsForSubject(ctx context.Context, subjectID uuid.UUID) ([]repository.EvidenceConflictReadModel, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, fact_type, subject_id, analysis_ids, description, resolution, status, version, created_at, updated_at
		FROM evidence_conflicts
		WHERE subject_id = $1
	`, subjectID)
	if err != nil {
		return nil, fmt.Errorf("query conflicts for subject: %w", err)
	}
	defer rows.Close()

	var results []repository.EvidenceConflictReadModel
	for rows.Next() {
		var c repository.EvidenceConflictReadModel
		var analysisIDs, resolution sql.NullString
		if err := rows.Scan(
			&c.ID, &c.FactType, &c.SubjectID, &analysisIDs,
			&c.Description, &resolution, &c.Status,
			&c.Version, &c.CreatedAt, &c.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan evidence_conflict: %w", err)
		}
		if analysisIDs.Valid {
			c.AnalysisIDsJSON = analysisIDs.String
		}
		if resolution.Valid {
			c.Resolution = resolution.String
		}
		results = append(results, c)
	}

	return results, rows.Err()
}

// ListUnresolvedConflicts returns all unresolved (open) evidence conflicts.
func (s *ReadModelStore) ListUnresolvedConflicts(ctx context.Context) ([]repository.EvidenceConflictReadModel, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, fact_type, subject_id, analysis_ids, description, resolution, status, version, created_at, updated_at
		FROM evidence_conflicts
		WHERE status = $1
	`, string(domain.ConflictStatusOpen))
	if err != nil {
		return nil, fmt.Errorf("query unresolved conflicts: %w", err)
	}
	defer rows.Close()

	var results []repository.EvidenceConflictReadModel
	for rows.Next() {
		var c repository.EvidenceConflictReadModel
		var analysisIDs, resolution sql.NullString
		if err := rows.Scan(
			&c.ID, &c.FactType, &c.SubjectID, &analysisIDs,
			&c.Description, &resolution, &c.Status,
			&c.Version, &c.CreatedAt, &c.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan evidence_conflict: %w", err)
		}
		if analysisIDs.Valid {
			c.AnalysisIDsJSON = analysisIDs.String
		}
		if resolution.Valid {
			c.Resolution = resolution.String
		}
		results = append(results, c)
	}

	return results, rows.Err()
}

// SaveEvidenceConflict saves or updates an evidence conflict.
func (s *ReadModelStore) SaveEvidenceConflict(ctx context.Context, conflict *repository.EvidenceConflictReadModel) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO evidence_conflicts (id, fact_type, subject_id, analysis_ids, description, resolution, status, version, created_at, updated_at)
		VALUES ($1, $2, $3, NULLIF($4, '')::JSONB, $5, NULLIF($6, ''), $7, $8, $9, $10)
		ON CONFLICT (id) DO UPDATE SET
			fact_type = EXCLUDED.fact_type,
			subject_id = EXCLUDED.subject_id,
			analysis_ids = EXCLUDED.analysis_ids,
			description = EXCLUDED.description,
			resolution = EXCLUDED.resolution,
			status = EXCLUDED.status,
			version = EXCLUDED.version,
			updated_at = EXCLUDED.updated_at
	`, conflict.ID, conflict.FactType, conflict.SubjectID, conflict.AnalysisIDsJSON,
		conflict.Description, conflict.Resolution, conflict.Status,
		conflict.Version, conflict.CreatedAt, conflict.UpdatedAt)
	if err != nil {
		return fmt.Errorf("save evidence_conflict: %w", err)
	}
	return nil
}

// DeleteEvidenceConflict deletes an evidence conflict by ID.
func (s *ReadModelStore) DeleteEvidenceConflict(ctx context.Context, id uuid.UUID) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM evidence_conflicts WHERE id = $1", id)
	if err != nil {
		return fmt.Errorf("delete evidence_conflict: %w", err)
	}
	return nil
}

// GetResearchLog retrieves a research log by ID.
func (s *ReadModelStore) GetResearchLog(ctx context.Context, id uuid.UUID) (*repository.ResearchLogReadModel, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, subject_id, subject_type, repository, search_description, outcome, notes, search_date, version, created_at, updated_at
		FROM research_logs WHERE id = $1
	`, id)

	var l repository.ResearchLogReadModel
	var notes sql.NullString
	err := row.Scan(
		&l.ID, &l.SubjectID, &l.SubjectType, &l.Repository,
		&l.SearchDescription, &l.Outcome, &notes,
		&l.SearchDate, &l.Version, &l.CreatedAt, &l.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan research_log: %w", err)
	}
	if notes.Valid {
		l.Notes = notes.String
	}
	return &l, nil
}

// ListResearchLogs returns a paginated list of research logs.
func (s *ReadModelStore) ListResearchLogs(ctx context.Context, opts repository.ListOptions) ([]repository.ResearchLogReadModel, int, error) {
	var total int
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM research_logs").Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("count research_logs: %w", err)
	}

	orderDir := "DESC"
	if opts.Order == "asc" {
		orderDir = "ASC"
	}

	sortCol := "updated_at"
	if opts.Sort == "created_at" {
		sortCol = "created_at"
	}

	// #nosec G201 -- orderDir and sortCol are validated above, not user input
	query := fmt.Sprintf(`
		SELECT id, subject_id, subject_type, repository, search_description, outcome, notes, search_date, version, created_at, updated_at
		FROM research_logs
		ORDER BY %s %s, id %s
		LIMIT $1 OFFSET $2
	`, sortCol, orderDir, orderDir)

	rows, err := s.db.QueryContext(ctx, query, opts.Limit, opts.Offset)
	if err != nil {
		return nil, 0, fmt.Errorf("query research_logs: %w", err)
	}
	defer rows.Close()

	var results []repository.ResearchLogReadModel
	for rows.Next() {
		var l repository.ResearchLogReadModel
		var notes sql.NullString
		if err := rows.Scan(
			&l.ID, &l.SubjectID, &l.SubjectType, &l.Repository,
			&l.SearchDescription, &l.Outcome, &notes,
			&l.SearchDate, &l.Version, &l.CreatedAt, &l.UpdatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("scan research_log: %w", err)
		}
		if notes.Valid {
			l.Notes = notes.String
		}
		results = append(results, l)
	}

	return results, total, rows.Err()
}

// GetResearchLogsForSubject returns all research logs for a given subject.
func (s *ReadModelStore) GetResearchLogsForSubject(ctx context.Context, subjectID uuid.UUID) ([]repository.ResearchLogReadModel, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, subject_id, subject_type, repository, search_description, outcome, notes, search_date, version, created_at, updated_at
		FROM research_logs
		WHERE subject_id = $1
	`, subjectID)
	if err != nil {
		return nil, fmt.Errorf("query research logs for subject: %w", err)
	}
	defer rows.Close()

	var results []repository.ResearchLogReadModel
	for rows.Next() {
		var l repository.ResearchLogReadModel
		var notes sql.NullString
		if err := rows.Scan(
			&l.ID, &l.SubjectID, &l.SubjectType, &l.Repository,
			&l.SearchDescription, &l.Outcome, &notes,
			&l.SearchDate, &l.Version, &l.CreatedAt, &l.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan research_log: %w", err)
		}
		if notes.Valid {
			l.Notes = notes.String
		}
		results = append(results, l)
	}

	return results, rows.Err()
}

// SaveResearchLog saves or updates a research log.
func (s *ReadModelStore) SaveResearchLog(ctx context.Context, log *repository.ResearchLogReadModel) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO research_logs (id, subject_id, subject_type, repository, search_description, outcome, notes, search_date, version, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, NULLIF($7, ''), $8, $9, $10, $11)
		ON CONFLICT (id) DO UPDATE SET
			subject_id = EXCLUDED.subject_id,
			subject_type = EXCLUDED.subject_type,
			repository = EXCLUDED.repository,
			search_description = EXCLUDED.search_description,
			outcome = EXCLUDED.outcome,
			notes = EXCLUDED.notes,
			search_date = EXCLUDED.search_date,
			version = EXCLUDED.version,
			updated_at = EXCLUDED.updated_at
	`, log.ID, log.SubjectID, log.SubjectType, log.Repository,
		log.SearchDescription, log.Outcome, log.Notes,
		log.SearchDate, log.Version, log.CreatedAt, log.UpdatedAt)
	if err != nil {
		return fmt.Errorf("save research_log: %w", err)
	}
	return nil
}

// DeleteResearchLog deletes a research log by ID.
func (s *ReadModelStore) DeleteResearchLog(ctx context.Context, id uuid.UUID) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM research_logs WHERE id = $1", id)
	if err != nil {
		return fmt.Errorf("delete research_log: %w", err)
	}
	return nil
}

// GetProofSummary retrieves a proof summary by ID.
func (s *ReadModelStore) GetProofSummary(ctx context.Context, id uuid.UUID) (*repository.ProofSummaryReadModel, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, fact_type, subject_id, conclusion, argument, analysis_ids, research_status, version, created_at, updated_at
		FROM proof_summaries WHERE id = $1
	`, id)

	var ps repository.ProofSummaryReadModel
	var analysisIDs, researchStatus sql.NullString
	err := row.Scan(
		&ps.ID, &ps.FactType, &ps.SubjectID, &ps.Conclusion,
		&ps.Argument, &analysisIDs, &researchStatus,
		&ps.Version, &ps.CreatedAt, &ps.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan proof_summary: %w", err)
	}
	if analysisIDs.Valid {
		ps.AnalysisIDsJSON = analysisIDs.String
	}
	if researchStatus.Valid {
		ps.ResearchStatus = domain.ResearchStatus(researchStatus.String)
	}
	return &ps, nil
}

// ListProofSummaries returns a paginated list of proof summaries.
func (s *ReadModelStore) ListProofSummaries(ctx context.Context, opts repository.ListOptions) ([]repository.ProofSummaryReadModel, int, error) {
	var total int
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM proof_summaries").Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("count proof_summaries: %w", err)
	}

	orderDir := "DESC"
	if opts.Order == "asc" {
		orderDir = "ASC"
	}

	sortCol := "updated_at"
	if opts.Sort == "created_at" {
		sortCol = "created_at"
	}

	// #nosec G201 -- orderDir and sortCol are validated above, not user input
	query := fmt.Sprintf(`
		SELECT id, fact_type, subject_id, conclusion, argument, analysis_ids, research_status, version, created_at, updated_at
		FROM proof_summaries
		ORDER BY %s %s, id %s
		LIMIT $1 OFFSET $2
	`, sortCol, orderDir, orderDir)

	rows, err := s.db.QueryContext(ctx, query, opts.Limit, opts.Offset)
	if err != nil {
		return nil, 0, fmt.Errorf("query proof_summaries: %w", err)
	}
	defer rows.Close()

	var results []repository.ProofSummaryReadModel
	for rows.Next() {
		var ps repository.ProofSummaryReadModel
		var analysisIDs, researchStatus sql.NullString
		if err := rows.Scan(
			&ps.ID, &ps.FactType, &ps.SubjectID, &ps.Conclusion,
			&ps.Argument, &analysisIDs, &researchStatus,
			&ps.Version, &ps.CreatedAt, &ps.UpdatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("scan proof_summary: %w", err)
		}
		if analysisIDs.Valid {
			ps.AnalysisIDsJSON = analysisIDs.String
		}
		if researchStatus.Valid {
			ps.ResearchStatus = domain.ResearchStatus(researchStatus.String)
		}
		results = append(results, ps)
	}

	return results, total, rows.Err()
}

// GetProofSummariesForFact returns all proof summaries for a given fact type and subject.
func (s *ReadModelStore) GetProofSummariesForFact(ctx context.Context, factType domain.FactType, subjectID uuid.UUID) ([]repository.ProofSummaryReadModel, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, fact_type, subject_id, conclusion, argument, analysis_ids, research_status, version, created_at, updated_at
		FROM proof_summaries
		WHERE fact_type = $1 AND subject_id = $2
	`, string(factType), subjectID)
	if err != nil {
		return nil, fmt.Errorf("query proof summaries for fact: %w", err)
	}
	defer rows.Close()

	var results []repository.ProofSummaryReadModel
	for rows.Next() {
		var ps repository.ProofSummaryReadModel
		var analysisIDs, researchStatus sql.NullString
		if err := rows.Scan(
			&ps.ID, &ps.FactType, &ps.SubjectID, &ps.Conclusion,
			&ps.Argument, &analysisIDs, &researchStatus,
			&ps.Version, &ps.CreatedAt, &ps.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan proof_summary: %w", err)
		}
		if analysisIDs.Valid {
			ps.AnalysisIDsJSON = analysisIDs.String
		}
		if researchStatus.Valid {
			ps.ResearchStatus = domain.ResearchStatus(researchStatus.String)
		}
		results = append(results, ps)
	}

	return results, rows.Err()
}

// GetProofSummariesBySubject returns all proof summaries for a given subject, regardless of fact type.
func (s *ReadModelStore) GetProofSummariesBySubject(ctx context.Context, subjectID uuid.UUID) ([]repository.ProofSummaryReadModel, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, fact_type, subject_id, conclusion, argument, analysis_ids, research_status, version, created_at, updated_at
		FROM proof_summaries
		WHERE subject_id = $1
	`, subjectID)
	if err != nil {
		return nil, fmt.Errorf("query proof summaries by subject: %w", err)
	}
	defer rows.Close()

	var results []repository.ProofSummaryReadModel
	for rows.Next() {
		var ps repository.ProofSummaryReadModel
		var analysisIDs, researchStatus sql.NullString
		if err := rows.Scan(
			&ps.ID, &ps.FactType, &ps.SubjectID, &ps.Conclusion,
			&ps.Argument, &analysisIDs, &researchStatus,
			&ps.Version, &ps.CreatedAt, &ps.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan proof_summary: %w", err)
		}
		if analysisIDs.Valid {
			ps.AnalysisIDsJSON = analysisIDs.String
		}
		if researchStatus.Valid {
			ps.ResearchStatus = domain.ResearchStatus(researchStatus.String)
		}
		results = append(results, ps)
	}

	return results, rows.Err()
}

// SaveProofSummary saves or updates a proof summary.
func (s *ReadModelStore) SaveProofSummary(ctx context.Context, summary *repository.ProofSummaryReadModel) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO proof_summaries (id, fact_type, subject_id, conclusion, argument, analysis_ids, research_status, version, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, NULLIF($6, '')::JSONB, NULLIF($7, ''), $8, $9, $10)
		ON CONFLICT (id) DO UPDATE SET
			fact_type = EXCLUDED.fact_type,
			subject_id = EXCLUDED.subject_id,
			conclusion = EXCLUDED.conclusion,
			argument = EXCLUDED.argument,
			analysis_ids = EXCLUDED.analysis_ids,
			research_status = EXCLUDED.research_status,
			version = EXCLUDED.version,
			updated_at = EXCLUDED.updated_at
	`, summary.ID, summary.FactType, summary.SubjectID, summary.Conclusion,
		summary.Argument, summary.AnalysisIDsJSON, string(summary.ResearchStatus),
		summary.Version, summary.CreatedAt, summary.UpdatedAt)
	if err != nil {
		return fmt.Errorf("save proof_summary: %w", err)
	}
	return nil
}

// DeleteProofSummary deletes a proof summary by ID.
func (s *ReadModelStore) DeleteProofSummary(ctx context.Context, id uuid.UUID) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM proof_summaries WHERE id = $1", id)
	if err != nil {
		return fmt.Errorf("delete proof_summary: %w", err)
	}
	return nil
}
