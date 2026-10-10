package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/cacack/gedcom-go/v2/gedcom"
	"github.com/google/uuid"
	"github.com/lib/pq"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// ReadModelStore is a PostgreSQL implementation of repository.ReadModelStore.
type ReadModelStore struct {
	db *sql.DB
}

// ErrConflictingEventsTables is returned by NewReadModelStore when the current
// schema holds BOTH a pre-#733 read-model `events` table (one carrying owner_type)
// and a `life_events` table. Which of the two holds the live life facts cannot be
// decided from the schema, so the store refuses to open rather than guess: renaming
// would fail, and silently preferring either one risks serving an empty or stale
// table while the real rows sit orphaned.
var ErrConflictingEventsTables = errors.New(
	`this database holds both a pre-#733 read-model "events" table and a "life_events" table: ` +
		`the migration state is ambiguous — either could hold the live life facts, so the read model ` +
		`refuses to open. Inspect both tables manually, keep the one with the current rows as ` +
		`"life_events", and drop or archive the other (see issue #733)`)

// NewReadModelStore creates a new PostgreSQL read model store.
//
// It can fail at construction. On a pre-#733 database — one whose life-fact table is
// still named `events` — it performs the rename to `life_events` and returns an error
// if that rename cannot be completed, including ErrConflictingEventsTables when both
// table names are already taken. Nothing is served from a half-migrated schema.
//
// On such a pre-#733 database this store must be constructed BEFORE the event store:
// the event store refuses a database whose `events` table is the read model's
// (ErrReadModelEventsTable), and this constructor is what frees the name. On a fresh
// or already-migrated database the construction order does not matter.
func NewReadModelStore(db *sql.DB) (*ReadModelStore, error) {
	store := &ReadModelStore{db: db}
	if err := store.createTables(); err != nil {
		return nil, fmt.Errorf("failed to create tables: %w", err)
	}
	return store, nil
}

// createTables creates the read model schema if it doesn't exist.
func (s *ReadModelStore) createTables() error {
	// Must run before the DDL batch below: once CREATE TABLE IF NOT EXISTS
	// life_events has made an empty table, the rename can no longer happen and the
	// legacy rows are stranded (issue #733).
	//
	// Its error is FATAL and returned immediately: unlike the ADD COLUMN migrations
	// in this file, this is a DESTRUCTIVE rename, so it must not follow
	// runMigrations' best-effort swallow idiom. See renameLegacyEventsTable.
	if err := s.renameLegacyEventsTable(); err != nil {
		return err
	}

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

		-- Sources table. Branch-aware (#758): (id, branch_id) row identity + deleted
		-- tombstone; the branch_id index is created by runBranchMigration.
		CREATE TABLE IF NOT EXISTS sources (
			id UUID NOT NULL,
			branch_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
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
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			deleted BOOLEAN NOT NULL DEFAULT FALSE,
			PRIMARY KEY (id, branch_id)
		);

		CREATE INDEX IF NOT EXISTS idx_sources_title ON sources(title);
		CREATE INDEX IF NOT EXISTS idx_sources_type ON sources(source_type);

		-- Citations table
		-- Branch-aware (#758): (id, branch_id) row identity + deleted tombstone. The FK
		-- to sources(id) is dropped (sources(id) is not unique under the overlay);
		-- DeleteSource cascades citations in code.
		CREATE TABLE IF NOT EXISTS citations (
			id UUID NOT NULL,
			branch_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
			source_id UUID NOT NULL,
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
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			deleted BOOLEAN NOT NULL DEFAULT FALSE,
			PRIMARY KEY (id, branch_id)
		);

		CREATE INDEX IF NOT EXISTS idx_citations_source ON citations(source_id);
		CREATE INDEX IF NOT EXISTS idx_citations_fact ON citations(fact_type, fact_owner_id);
		CREATE INDEX IF NOT EXISTS idx_citations_owner ON citations(fact_owner_id);

		-- Media table
		-- Branch-aware for METADATA only (#759): (id, branch_id) row identity +
		-- deleted tombstone. file_data/thumbnail_data are nullable because a branch
		-- shadow row of a mainline item carries none: the bytes stay on the item's
		-- origin row and are shared (see the blob rule on repository.ReadModelStore).
		CREATE TABLE IF NOT EXISTS media (
			id UUID NOT NULL,
			branch_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
			entity_type VARCHAR(20) NOT NULL,
			entity_id UUID NOT NULL,
			title VARCHAR(500) NOT NULL,
			description TEXT,
			mime_type VARCHAR(100) NOT NULL,
			media_type VARCHAR(20) NOT NULL,
			filename VARCHAR(255) NOT NULL,
			file_size BIGINT NOT NULL,
			file_data BYTEA,
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
			translations JSONB,   -- Translated titles (GEDCOM 7.0)
			deleted BOOLEAN NOT NULL DEFAULT FALSE,
			PRIMARY KEY (id, branch_id)
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
		-- Branch-aware (#758): (id, branch_id) row identity + deleted tombstone.
		CREATE TABLE IF NOT EXISTS notes (
			id UUID NOT NULL,
			branch_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
			text TEXT NOT NULL,
			mime VARCHAR(100),
			language VARCHAR(35),
			translations JSONB,
			gedcom_xref VARCHAR(50),
			version BIGINT NOT NULL DEFAULT 1,
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			deleted BOOLEAN NOT NULL DEFAULT FALSE,
			PRIMARY KEY (id, branch_id)
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
		-- Branch-aware (#758): identity is (source_id, sequence, branch_id), the same
		-- per-parent bucket + empty-marker tombstone model as person_external_ids.
		-- The FK to sources(id) is dropped; DeleteSource cascades the bucket in code.
		CREATE TABLE IF NOT EXISTS source_external_ids (
			source_id UUID NOT NULL,
			sequence INTEGER NOT NULL,
			branch_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
			value TEXT NOT NULL,
			type TEXT NOT NULL DEFAULT '',
			deleted BOOLEAN NOT NULL DEFAULT FALSE,
			PRIMARY KEY (source_id, sequence, branch_id)
		);

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
		-- branch overlay (ADR-005). Branch-aware (#757): (id, branch_id) row identity
		-- + deleted tombstone. The branch_id index is created by runBranchMigration,
		-- after the column is guaranteed to exist on an upgraded database.
		CREATE TABLE IF NOT EXISTS associations (
			id UUID NOT NULL,
			branch_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
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
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			deleted BOOLEAN NOT NULL DEFAULT FALSE,
			PRIMARY KEY (id, branch_id)
		);

		CREATE INDEX IF NOT EXISTS idx_associations_person ON associations(person_id);
		CREATE INDEX IF NOT EXISTS idx_associations_associate ON associations(associate_id);
		CREATE INDEX IF NOT EXISTS idx_associations_role ON associations(role);

		-- Life events table (life events for persons and families).
		-- Named life_events, not events: the event log owns the events table and both
		-- stores can share one database (issue #733). Branch-aware (#757): (id,
		-- branch_id) row identity + deleted tombstone; see the associations note.
		CREATE TABLE IF NOT EXISTS life_events (
			id UUID NOT NULL,
			branch_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
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
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			deleted BOOLEAN NOT NULL DEFAULT FALSE,
			PRIMARY KEY (id, branch_id)
		);

		CREATE INDEX IF NOT EXISTS idx_life_events_owner ON life_events(owner_type, owner_id);
		CREATE INDEX IF NOT EXISTS idx_life_events_fact_type ON life_events(fact_type);

		-- Attributes table (person attributes)
		-- FK reference to persons(id) dropped (see associations note). Branch-aware
		-- (#757): (id, branch_id) row identity + deleted tombstone.
		CREATE TABLE IF NOT EXISTS attributes (
			id UUID NOT NULL,
			branch_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
			person_id UUID NOT NULL,
			fact_type VARCHAR(100) NOT NULL,
			value TEXT NOT NULL DEFAULT '',
			date_raw VARCHAR(100),
			date_sort DATE,
			place VARCHAR(255),
			version BIGINT NOT NULL DEFAULT 1,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			deleted BOOLEAN NOT NULL DEFAULT FALSE,
			PRIMARY KEY (id, branch_id)
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
		-- Branch-aware (#760): (id, branch_id) row identity + deleted tombstone, like
		-- every GPS artifact table below. subject_id has no foreign key; DeletePerson
		-- and DeleteFamily cascade in code.
		CREATE TABLE IF NOT EXISTS evidence_analyses (
			id UUID NOT NULL,
			branch_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
			fact_type VARCHAR(50) NOT NULL,
			subject_id UUID NOT NULL,
			citation_ids JSONB,
			conclusion TEXT NOT NULL,
			research_status VARCHAR(20),
			notes TEXT,
			version BIGINT NOT NULL DEFAULT 1,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			deleted BOOLEAN NOT NULL DEFAULT FALSE,
			PRIMARY KEY (id, branch_id)
		);

		CREATE INDEX IF NOT EXISTS idx_evidence_analyses_subject ON evidence_analyses(subject_id);
		CREATE INDEX IF NOT EXISTS idx_evidence_analyses_fact_type ON evidence_analyses(fact_type);

		-- Evidence conflicts table (branch-aware, #760)
		CREATE TABLE IF NOT EXISTS evidence_conflicts (
			id UUID NOT NULL,
			branch_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
			fact_type VARCHAR(50) NOT NULL,
			subject_id UUID NOT NULL,
			analysis_ids JSONB,
			description TEXT NOT NULL,
			resolution TEXT,
			status VARCHAR(20) NOT NULL,
			version BIGINT NOT NULL DEFAULT 1,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			deleted BOOLEAN NOT NULL DEFAULT FALSE,
			PRIMARY KEY (id, branch_id)
		);

		CREATE INDEX IF NOT EXISTS idx_evidence_conflicts_subject ON evidence_conflicts(subject_id);
		CREATE INDEX IF NOT EXISTS idx_evidence_conflicts_status ON evidence_conflicts(status);

		-- Research logs table (branch-aware, #760)
		CREATE TABLE IF NOT EXISTS research_logs (
			id UUID NOT NULL,
			branch_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
			subject_id UUID NOT NULL,
			subject_type VARCHAR(20) NOT NULL,
			repository VARCHAR(255) NOT NULL,
			search_description TEXT NOT NULL,
			outcome VARCHAR(20) NOT NULL,
			notes TEXT,
			search_date TIMESTAMPTZ NOT NULL,
			version BIGINT NOT NULL DEFAULT 1,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			deleted BOOLEAN NOT NULL DEFAULT FALSE,
			PRIMARY KEY (id, branch_id)
		);

		CREATE INDEX IF NOT EXISTS idx_research_logs_subject ON research_logs(subject_id);
		CREATE INDEX IF NOT EXISTS idx_research_logs_outcome ON research_logs(outcome);

		-- Proof summaries table (branch-aware, #760)
		CREATE TABLE IF NOT EXISTS proof_summaries (
			id UUID NOT NULL,
			branch_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
			fact_type VARCHAR(50) NOT NULL,
			subject_id UUID NOT NULL,
			conclusion TEXT NOT NULL,
			argument TEXT NOT NULL,
			analysis_ids JSONB,
			research_status VARCHAR(20),
			version BIGINT NOT NULL DEFAULT 1,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			deleted BOOLEAN NOT NULL DEFAULT FALSE,
			PRIMARY KEY (id, branch_id)
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

// renameLegacyEventsTable renames a pre-#733 read model events table to
// life_events, preserving its rows. It is idempotent and MUST run before
// createTables' DDL batch — see the note there.
//
// EVERY failure is reported, none swallowed. This is a DESTRUCTIVE rename, not one of
// this file's additive ADD COLUMN migrations, so it deliberately does NOT follow
// runMigrations' best-effort idiom. If a needed rename is skipped, the DDL batch
// creates an empty `life_events` beside the still-populated legacy table,
// NewReadModelStore returns success, and every GetEvent/ListEvents answers zero rows
// while the real data sits orphaned. Worse, it never recovers: the next open sees
// `life_events` and takes the already-migrated path. A startup failure is
// recoverable; a silently empty read model is not.
//
// Three details are load-bearing and must not be simplified away:
//
//   - The owner_type guard (eventsTableBelongsToReadModel). The event log
//     (eventstore.go) also owns a table named events, and ADR-002 puts both stores in
//     one database. Only the read model's table has an owner_type column, so probing
//     for it is what keeps this migration from renaming the event log's source of
//     truth out from under it. Do not treat a failed probe as "not legacy".
//   - The primary-key constraint rename. Constraint names are schema-global, so the
//     read model's id UUID PRIMARY KEY left behind an events_pkey that a table rename
//     does not touch. Without renaming it, the event log's own CREATE TABLE IF NOT
//     EXISTS events (id UUID PRIMARY KEY ...) in the same database fails with
//     "relation events_pkey already exists". The name is resolved from pg_constraint
//     rather than assumed to be events_pkey: a restored or renamed schema can differ.
//   - The single schema of reference. Every step here — the probe, the life_events
//     existence check, and the DDL — resolves against CURRENT_SCHEMA(), the schema
//     this store's own unqualified CREATE TABLE statements land in. An earlier form
//     mixed resolutions: it probed information_schema scoped to CURRENT_SCHEMA() but
//     tested life_events with to_regclass, which searches the WHOLE search_path. With
//     a DSN carrying search_path=app,public (lib/pq forwards it, so DATABASE_URL can
//     express it) a life_events in public suppressed the rename of app.events and
//     stranded its rows. The ALTER statements are schema-qualified for the same
//     reason: unqualified, they could resolve to another schema's events.
func (s *ReadModelStore) renameLegacyEventsTable() error {
	legacy, err := eventsTableBelongsToReadModel(s.db)
	if err != nil {
		return fmt.Errorf("check whether the events table belongs to the read model: %w", err)
	}
	if !legacy {
		// Fresh database, already migrated, or the table is the event log's: nothing to do.
		return nil
	}

	// CURRENT_SCHEMA() is NULL when the search_path names no existing schema. The
	// probe above could not have matched in that case, so reaching here with a NULL
	// means the schema vanished mid-migration; refuse rather than emit bare DDL.
	var schema sql.NullString
	if err := s.db.QueryRow(`SELECT CURRENT_SCHEMA()`).Scan(&schema); err != nil {
		return fmt.Errorf("resolve the current schema: %w", err)
	}
	if !schema.Valid {
		return fmt.Errorf("resolve the current schema: search_path names no existing schema")
	}
	qualifiedSchema := pq.QuoteIdentifier(schema.String)

	var lifeEventsExists bool
	if err := s.db.QueryRow(`
		SELECT EXISTS (
			SELECT 1 FROM information_schema.tables
			WHERE table_schema = $1 AND table_name = 'life_events'
		)`, schema.String).Scan(&lifeEventsExists); err != nil {
		return fmt.Errorf("check for an existing life_events table: %w", err)
	}
	if lifeEventsExists {
		return ErrConflictingEventsTables
	}

	// Postgres carries indexes across a table rename, so the old idx_events_* names
	// would end up attached to life_events beside the new idx_life_events_* ones.
	// An identifier cannot be a bind parameter, so schema-qualifying this DDL requires
	// building the statement as a string. Every interpolated part is either a literal
	// from the list below or an identifier passed through pq.QuoteIdentifier, which is
	// the correct defense for identifiers; no external value reaches these statements.
	for _, idx := range []string{"idx_events_owner", "idx_events_fact_type"} {
		// #nosec G201 -- idx comes from this fixed literal list and the schema from
		// pq.QuoteIdentifier over CURRENT_SCHEMA(); neither is user input.
		// nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query
		if _, err := s.db.Exec(`DROP INDEX IF EXISTS ` + qualifiedSchema + `.` + pq.QuoteIdentifier(idx)); err != nil {
			return fmt.Errorf("drop stale read-model index %s: %w", idx, err)
		}
	}

	// #nosec G201 -- qualifiedSchema comes from pq.QuoteIdentifier over CURRENT_SCHEMA(), not user input.
	// nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query
	if _, err := s.db.Exec(`ALTER TABLE ` + qualifiedSchema + `.events RENAME TO life_events`); err != nil {
		return fmt.Errorf("rename the read model's legacy events table to life_events: %w", err)
	}

	// Bound as $1, not interpolated: to_regclass takes the qualified name as a value.
	qualifiedTable := qualifiedSchema + `.life_events`
	var pkName string
	// nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query
	err = s.db.QueryRow(`
		SELECT conname FROM pg_constraint
		WHERE conrelid = to_regclass($1) AND contype = 'p'
	`, qualifiedTable).Scan(&pkName)
	if errors.Is(err, sql.ErrNoRows) {
		// No primary key to rename, so no events_pkey is occupying the name the event
		// log needs. Nothing failed; the rename is complete.
		return nil
	}
	if err != nil {
		return fmt.Errorf("resolve the life_events primary key name: %w", err)
	}
	if pkName == "life_events_pkey" {
		return nil
	}
	// #nosec G201 -- pkName comes from pg_constraint for a fixed internal table, not user input.
	// nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query
	if _, err := s.db.Exec(`ALTER TABLE ` + qualifiedTable + ` RENAME CONSTRAINT ` + pq.QuoteIdentifier(pkName) + ` TO life_events_pkey`); err != nil {
		return fmt.Errorf("rename the life_events primary key %s to life_events_pkey: %w", pkName, err)
	}
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
	_, _ = s.db.Exec(`ALTER TABLE life_events ADD COLUMN IF NOT EXISTS is_negated BOOLEAN NOT NULL DEFAULT FALSE`)

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

		// The backfill joins persons, which is branch-overlaid (ADR-005): with no branch
		// predicate the join is ambiguous once shadow rows exist and can copy a branch
		// row's name onto the main family (issue #756). Pin both sides to main. The
		// column adds repeat runBranchMigration's idempotent statements because this
		// transaction runs before it, so a pre-#669 database has no branch_id column yet
		// and the predicate below would abort the whole migration.
		//
		// The pin only prevents new bad copies; it cannot repair old ones. The IS NULL
		// guard that makes the backfill idempotent also makes it skip every row the
		// unpinned version already wrote, which is exactly the set that could be wrong.
		// The window is narrow: a family whose denormalized partner name was still NULL
		// at the moment shadow rows existed for that partner -- i.e. a database that ran
		// the #669 branch migration, then created a branch that edited the partner, and
		// only then ran this backfill for the first time. A repair migration is a larger
		// decision and is deliberately not attempted here.
		for _, t := range []string{"persons", "families", "family_children"} {
			_, _ = tx.Exec(`ALTER TABLE ` + t + ` ADD COLUMN IF NOT EXISTS branch_id UUID NOT NULL ` + mainBranchDefault)
		}

		// Backfill split fields from persons table; idempotent via IS NULL guard.
		main := domain.MainBranchID.UUID()
		_, _ = tx.Exec(`
			UPDATE families f SET
				partner1_given_name = p.given_name,
				partner1_surname    = p.surname
			FROM persons p WHERE f.partner1_id = p.id AND f.partner1_given_name IS NULL
			  AND f.branch_id = $1 AND p.branch_id = $1
		`, main)
		_, _ = tx.Exec(`
			UPDATE families f SET
				partner2_given_name = p.given_name,
				partner2_surname    = p.surname
			FROM persons p WHERE f.partner2_id = p.id AND f.partner2_given_name IS NULL
			  AND f.branch_id = $1 AND p.branch_id = $1
		`, main)
		_, _ = tx.Exec(`
			UPDATE family_children fc SET
				person_given_name = p.given_name,
				person_surname    = p.surname
			FROM persons p WHERE fc.person_id = p.id AND fc.person_given_name IS NULL
			  AND fc.branch_id = $1 AND p.branch_id = $1
		`, main)

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
		// Evidence (#758): sources(id) stops being unique once it joins the overlay,
		// so the FKs referencing it go before its primary key is swapped.
		`ALTER TABLE citations DROP CONSTRAINT IF EXISTS citations_source_id_fkey`,
		`ALTER TABLE source_external_ids DROP CONSTRAINT IF EXISTS source_external_ids_source_id_fkey`,
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
		// Person/family facts (#757).
		{"life_events", "id"},
		{"attributes", "id"},
		{"associations", "id"},
		// Evidence (#758).
		{"sources", "id"},
		{"source_external_ids", "source_id, sequence"},
		{"citations", "id"},
		{"notes", "id"},
		// Media metadata (#759).
		{"media", "id"},
		// GPS artifacts (#760).
		{"evidence_analyses", "id"},
		{"evidence_conflicts", "id"},
		{"research_logs", "id"},
		{"proof_summaries", "id"},
	}
	for _, t := range tables {
		// Column adds are idempotent and safe outside a transaction.
		// #nosec G202 -- t.name comes from the fixed `tables` literal above and
		// mainBranchDefault is a package constant; no external input reaches this DDL.
		// nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query
		_, _ = s.db.Exec(`ALTER TABLE ` + t.name + ` ADD COLUMN IF NOT EXISTS branch_id UUID NOT NULL ` + mainBranchDefault)
		_, _ = s.db.Exec(`ALTER TABLE ` + t.name + ` ADD COLUMN IF NOT EXISTS deleted BOOLEAN NOT NULL DEFAULT FALSE`)

		// Already the target shape? Nothing to do. Checked explicitly so the swap
		// below is only attempted when it is actually needed, and so "already
		// migrated" is not indistinguishable from "swap failed".
		var branchInPK bool
		if err := s.db.QueryRow(`
			SELECT EXISTS (
				SELECT 1 FROM pg_index i
				JOIN pg_attribute a ON a.attrelid = i.indrelid AND a.attnum = ANY(i.indkey)
				WHERE i.indrelid = $1::regclass AND i.indisprimary AND a.attname = 'branch_id'
			)`, t.name).Scan(&branchInPK); err != nil {
			slog.Warn("read model migration: cannot inspect primary key; branch writes may fail until the read model is rebuilt (see issue #680)",
				"table", t.name, "error", err)
			continue
		}
		if branchInPK {
			continue
		}

		// Swap the PK atomically. Resolve the constraint name from the catalog
		// rather than assuming the default <table>_pkey: a restored or renamed
		// schema can carry a different name, in which case a DROP CONSTRAINT IF
		// EXISTS <table>_pkey is a silent no-op and the following ADD PRIMARY KEY
		// fails with "multiple primary keys are not allowed" — leaving the table on
		// its single-column key while every writer uses ON CONFLICT (…, branch_id).
		// Failures are logged, never swallowed, so a half-migrated database is
		// visible at startup instead of surfacing later as opaque projection errors.
		var pkName string
		if err := s.db.QueryRow(`
			SELECT conname FROM pg_constraint
			WHERE conrelid = $1::regclass AND contype = 'p'
		`, t.name).Scan(&pkName); err != nil && !errors.Is(err, sql.ErrNoRows) {
			slog.Warn("read model migration: cannot resolve primary key name; branch writes may fail until the read model is rebuilt (see issue #680)",
				"table", t.name, "error", err)
			continue
		}

		tx, err := s.db.Begin()
		if err != nil {
			slog.Warn("read model migration: cannot begin primary key swap", "table", t.name, "error", err)
			continue
		}
		if pkName != "" {
			// #nosec G201 -- pkName comes from pg_constraint for a fixed internal table, not user input.
			if _, err := tx.Exec(`ALTER TABLE ` + t.name + ` DROP CONSTRAINT ` + pq.QuoteIdentifier(pkName)); err != nil {
				_ = tx.Rollback()
				slog.Warn("read model migration: dropping old primary key failed; branch writes may fail until the read model is rebuilt (see issue #680)",
					"table", t.name, "constraint", pkName, "error", err)
				continue
			}
		}
		if _, err := tx.Exec(`ALTER TABLE ` + t.name + ` ADD PRIMARY KEY (` + t.pk + `, branch_id)`); err != nil {
			_ = tx.Rollback()
			slog.Warn("read model migration: adding composite primary key failed; branch writes may fail until the read model is rebuilt (see issue #680)",
				"table", t.name, "error", err)
			continue
		}
		if err := tx.Commit(); err != nil {
			slog.Warn("read model migration: committing primary key swap failed", "table", t.name, "error", err)
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
	_, _ = s.db.Exec(`DROP INDEX IF EXISTS idx_source_external_ids_source`)
	_, _ = s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_source_external_ids_source ON source_external_ids(source_id, branch_id)`)

	// Media (#759): a branch shadow row of a mainline item stores no bytes, so
	// file_data can no longer be NOT NULL. Logged, not swallowed: until it
	// succeeds every branch edit of mainline media fails its insert.
	if _, err := s.db.Exec(`ALTER TABLE media ALTER COLUMN file_data DROP NOT NULL`); err != nil {
		slog.Warn("read model migration: cannot make media.file_data nullable; branch media edits will fail until the read model is rebuilt (see issue #680)",
			"error", err)
	}

	// Secondary indexes leading with branch_id so PurgeBranch's DELETE ... WHERE
	// branch_id = ? (and the overlay's branch_id IN filter) is index-driven rather
	// than a full-table scan; the composite PK leads with id, so branch_id alone is
	// otherwise unindexed (issue #669).
	for _, tbl := range branchScopedTables {
		// #nosec G202 -- tbl comes from the fixed branchScopedTables list, not user input.
		// nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query
		_, _ = s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_` + tbl + `_branch ON ` + tbl + `(branch_id)`)
	}
}

// branchScopedTables are every read-model table carrying branch_id/deleted: the
// seven #669 slice tables, the three person/family fact tables (#757), the
// four evidence tables (#758), media (#759) and the four GPS artifact tables
// (#760). PurgeBranch drops a branch's rows from each, and runBranchMigration
// gives each its branch_id-leading index.
var branchScopedTables = []string{
	"persons", "person_names", "person_external_ids",
	"families", "family_external_ids", "family_children", "pedigree_edges",
	"life_events", "attributes", "associations",
	"sources", "source_external_ids", "citations", "notes",
	"media",
	"evidence_analyses", "evidence_conflicts", "research_logs", "proof_summaries",
}

// mainBranchDefault is the column default that backfills existing rows to the
// reserved main branch id (domain.MainBranchID / uuid.Nil) when branch_id is added
// to a slice table. A DDL default cannot be parameterized, so the literal is kept
// here rather than repeated at each ALTER TABLE.
const mainBranchDefault = `DEFAULT '00000000-0000-0000-0000-000000000000'`

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
	// #nosec G201 G202 -- orderColumn/orderDir are validated via switch/if above; cte/personSelectCols/fromSrc are internal SQL fragments, not user input
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
			// Ties break on name then id, always ascending, so the people that
			// make the limit are deterministic; the SQLite store breaks fuzzy
			// score ties the same way (DB-005).
			fmt.Fprintf(qb, " ORDER BY rank_score %s, p.surname ASC, p.given_name ASC, p.id ASC", orderDir)
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
// existed before the overlay), its facts (#757) and its media (#759, under the
// blob rule: see cascadeMedia). On a non-main branch it writes a tombstone for the
// person plus cascade tombstones for the person's names and external IDs, so the
// main fallback cannot resurrect them.
func (s *ReadModelStore) DeletePerson(ctx context.Context, branchID domain.BranchID, id uuid.UUID) error {
	main := domain.MainBranchID.UUID()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	// Take the media lock before any row lock (see lockMediaBlobs).
	if err := lockMediaBlobs(ctx, tx); err != nil {
		return err
	}

	if branchID.IsMain() {
		// Reproduce the pre-#669 ON DELETE CASCADE explicitly: the read-model FKs
		// were dropped so persons could be keyed by (id, branch_id), so every
		// dependent the FKs used to cascade must be cleaned up by hand here.
		// person_names, person_external_ids, pedigree_edges and associations (both
		// person_id and associate_id) were ON DELETE CASCADE pre-#669. attributes
		// referenced persons(id) with NO ON DELETE (RESTRICT), which would have
		// blocked the delete; blocking is not reproducible against an append-only
		// event log (the event store is the source of truth), so we cascade-delete
		// orphan attributes too rather than leave dangling read-model rows. The
		// person's own life events go with it as well (#757).
		for _, stmt := range []struct {
			sql  string
			args []any
		}{
			{"DELETE FROM persons WHERE id = $1 AND branch_id = $2", []any{id, main}},
			{"DELETE FROM person_names WHERE person_id = $1 AND branch_id = $2", []any{id, main}},
			{"DELETE FROM person_external_ids WHERE person_id = $1 AND branch_id = $2", []any{id, main}},
			{"DELETE FROM pedigree_edges WHERE person_id = $1 AND branch_id = $2", []any{id, main}},
			// family_children referenced persons(id) ON DELETE CASCADE on the child side,
			// so drop the deleted person from every mainline family it was a child of.
			{"DELETE FROM family_children WHERE person_id = $1 AND branch_id = $2", []any{id, main}},
		} {
			if _, err := tx.ExecContext(ctx, stmt.sql, stmt.args...); err != nil {
				return fmt.Errorf("delete person: %w", err)
			}
		}
		if err := cascadePersonFacts(ctx, tx, branchID, id); err != nil {
			return err
		}
		if err := cascadeMedia(ctx, tx, personMediaFilter, branchID, id); err != nil {
			return err
		}
		if err := cascadeGPS(ctx, tx, branchID, id); err != nil {
			return err
		}
		return tx.Commit()
	}

	// Branch (non-main) delete: tombstone the person and its branch-scoped
	// dependents (names, external IDs, pedigree edge, and the life events,
	// attributes and associations it owns or appears in), its media (#759) and
	// the GPS artifacts about it (#760).
	if err := cascadePersonFacts(ctx, tx, branchID, id); err != nil {
		return err
	}
	if err := cascadeMedia(ctx, tx, personMediaFilter, branchID, id); err != nil {
		return err
	}
	if err := cascadeGPS(ctx, tx, branchID, id); err != nil {
		return err
	}

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

	// Cascade tombstone the person's names: every name the branch resolves to
	// this person. The overlay is resolved per name id BEFORE the owner filter,
	// so a name the branch re-owned away (PersonMerged moves the merged
	// person's names to the survivor, #834) keeps its branch row.
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO person_names (id, person_id, given_name, surname, name_prefix, name_suffix,
								  surname_prefix, nickname, name_type, is_primary, updated_at, branch_id, deleted)
		SELECT id, person_id, given_name, surname, name_prefix, name_suffix,
			   surname_prefix, nickname, name_type, is_primary, updated_at, $2, TRUE
		FROM (
			SELECT DISTINCT ON (id) id, person_id, given_name, surname, name_prefix, name_suffix,
				   surname_prefix, nickname, name_type, is_primary, updated_at, deleted
			FROM person_names
			WHERE id IN (SELECT id FROM person_names WHERE person_id = $1 AND branch_id IN ($2, $3))
			  AND branch_id IN ($2, $3)
			ORDER BY id, (branch_id = $2) DESC
		) o WHERE NOT o.deleted AND o.person_id = $1
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

// cascadePersonFacts removes, on branchID, the person's life events and
// attributes and every association naming the person on either side (#757).
func cascadePersonFacts(ctx context.Context, tx *sql.Tx, branchID domain.BranchID, personID uuid.UUID) error {
	if err := cascadeOverlayRows(ctx, tx, "life_events", eventSelectCols, personEventsFilter, branchID, personID); err != nil {
		return err
	}
	if err := cascadeOverlayRows(ctx, tx, "attributes", attributeSelectCols, personAttributesFilter, branchID, personID); err != nil {
		return err
	}
	return cascadeOverlayRows(ctx, tx, "associations", associationSelectCols, personAssociationFilter, branchID, personID)
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
			FROM person_names
			WHERE id IN (SELECT id FROM person_names WHERE person_id = $1 AND branch_id IN ($2, $3))
			  AND branch_id IN ($2, $3)
			ORDER BY id, (branch_id = $2) DESC
		) o WHERE NOT deleted AND person_id = $1
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
// for a source on the given branch within a single transaction (ADR-005, #758).
// Same bucket-scoped semantics as ReplacePersonExternalIDs.
func (s *ReadModelStore) ReplaceSourceExternalIDs(ctx context.Context, branchID domain.BranchID, sourceID uuid.UUID, ids []repository.SourceExternalIDReadModel) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, "DELETE FROM source_external_ids WHERE source_id = $1 AND branch_id = $2", sourceID, branchID.UUID()); err != nil {
		return fmt.Errorf("delete source external ids: %w", err)
	}
	if len(ids) == 0 {
		if !branchID.IsMain() {
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO source_external_ids (source_id, sequence, branch_id, value, type, deleted)
				VALUES ($1, 0, $2, '', '', TRUE)
			`, sourceID, branchID.UUID()); err != nil {
				return fmt.Errorf("mark empty source external id bucket: %w", err)
			}
		}
		return tx.Commit()
	}
	for i, id := range ids {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO source_external_ids (source_id, sequence, branch_id, value, type, deleted)
			VALUES ($1, $2, $3, $4, $5, FALSE)
		`, sourceID, i, branchID.UUID(), id.Value, id.Type); err != nil {
			return fmt.Errorf("insert source external id: %w", err)
		}
	}
	return tx.Commit()
}

// GetSourceExternalIDs retrieves all external identifiers for a source within the
// branch overlay, ordered by their original sequence (ADR-005, #758).
// Bucket-scoped resolution, matching GetPersonExternalIDs.
func (s *ReadModelStore) GetSourceExternalIDs(ctx context.Context, branchID domain.BranchID, sourceID uuid.UUID) ([]repository.SourceExternalIDReadModel, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT sequence, value, type FROM source_external_ids
		WHERE source_id = $1 AND NOT deleted AND branch_id = (
			CASE WHEN EXISTS (SELECT 1 FROM source_external_ids WHERE source_id = $1 AND branch_id = $2)
			     THEN $2 ELSE $3 END)
		ORDER BY sequence
	`, sourceID, branchID.UUID(), domain.MainBranchID.UUID())
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

	// #nosec G201 G202 -- fromSrc/whereClause/limitParam/cte/familySelectCols are internal SQL fragments, not user input
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
// cascades to the family's children, external IDs, life events and media. On a non-main branch it
// writes a tombstone for the family plus cascade tombstones for its children and
// external IDs, so the main fallback cannot resurrect them.
func (s *ReadModelStore) DeleteFamily(ctx context.Context, branchID domain.BranchID, id uuid.UUID) error {
	main := domain.MainBranchID.UUID()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	// Take the media lock before any row lock (see lockMediaBlobs).
	if err := lockMediaBlobs(ctx, tx); err != nil {
		return err
	}

	// The family's own life events cascade with it on either scope (#757), and
	// so do its media (#759) and the GPS artifacts about it (#760).
	if err := cascadeOverlayRows(ctx, tx, "life_events", eventSelectCols, familyEventsFilter, branchID, id); err != nil {
		return err
	}
	if err := cascadeMedia(ctx, tx, familyMediaFilter, branchID, id); err != nil {
		return err
	}
	if err := cascadeGPS(ctx, tx, branchID, id); err != nil {
		return err
	}

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

// GetFamilyChildren returns all children for a family within the branch overlay
// (ADR-005). It is GetFamilyChildrenByFamilyIDs for one family, so the family
// group sheet and the descendancy walk always list siblings in the same order
// (sequence with unsequenced children last, then bytewise surname, given name
// and person id) on every backend.
func (s *ReadModelStore) GetFamilyChildren(ctx context.Context, branchID domain.BranchID, familyID uuid.UUID) ([]repository.FamilyChildReadModel, error) {
	return s.GetFamilyChildrenByFamilyIDs(ctx, branchID, []uuid.UUID{familyID})
}

// ListAllFamilyChildren returns every child link branchID sees, in one query:
// the same per-(family, person) overlay as GetFamilyChildren, over every family.
func (s *ReadModelStore) ListAllFamilyChildren(ctx context.Context, branchID domain.BranchID) ([]repository.FamilyChildReadModel, error) {
	// #nosec G202 -- familyChildSelectCols is a package constant; every value is a $-placeholder
	// nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+familyChildSelectCols+` FROM (
			SELECT DISTINCT ON (family_id, person_id) `+familyChildSelectCols+`, deleted
			FROM family_children WHERE branch_id IN ($1, $2)
			ORDER BY family_id, person_id, (branch_id = $1) DESC
		) o WHERE NOT deleted
		ORDER BY family_id, person_id
	`, branchID.UUID(), domain.MainBranchID.UUID())
	if err != nil {
		return nil, fmt.Errorf("query all family children: %w", err)
	}
	defer rows.Close()
	return scanFamilyChildRows(rows)
}

// scanFamilyChildRows scans familyChildSelectCols rows.
func scanFamilyChildRows(rows *sql.Rows) ([]repository.FamilyChildReadModel, error) {
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

// PurgeBranch hard-deletes every row for branchID across the branch-scoped
// tables (branchScopedTables). It is a no-op for the mainline (domain.MainBranchID), which is
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
	// Take the media lock before any row lock (see lockMediaBlobs).
	if err := lockMediaBlobs(ctx, tx); err != nil {
		return err
	}

	// Main media tombstones kept alive only for this branch's shadows go first,
	// while the branch rows that name them still exist (#759).
	if err := gcMainMedia(ctx, tx, gcMainMediaBeforePurge, branchID); err != nil {
		return err
	}
	for _, table := range branchScopedTables {
		// #nosec G202 -- table is from the fixed branchScopedTables list, not user input
		// nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query
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

// Evidence tables (#758): sources, citations and notes are branch-scoped the
// same way as the person/family facts (ADR-005) and reuse their overlay helpers
// (overlayArgs, overlaySrc, overlayGetQuery, deleteOverlayRow,
// cascadeOverlayRows).

const (
	// sourceSelectCols is scanSourceRow's column order (unaliased).
	sourceSelectCols = `id, source_type, title, author, publisher, publish_date_raw, publish_date_sort,
		url, repository_id, repository_name, collection_name, call_number, notes, gedcom_xref,
		citation_count, version, updated_at`

	// citationSelectCols is scanCitationRow's column order (unaliased).
	citationSelectCols = `id, source_id, source_title, fact_type, fact_owner_id, page, volume,
		source_quality, informant_type, evidence_type, quoted_text, analysis,
		template_id, fields_data, gedcom_xref, version, created_at`

	// noteSelectCols is scanNoteRow's column order (unaliased).
	noteSelectCols = `id, text, mime, language, translations, gedcom_xref, version, updated_at`

	// citationOrder is the deterministic citation order every backend returns.
	citationOrder = `source_title ASC, fact_type ASC, id ASC`

	// Per-parent citation filters. %[1]d (and %[2]d) are the placeholder numbers
	// of the values, which the caller binds after overlayArgs.
	sourceCitationsFilter = `source_id = $%[1]d`
	personCitationsFilter = `fact_owner_id = $%[1]d AND fact_type LIKE 'person_%%'`
	factCitationsFilter   = `fact_type = $%[1]d AND fact_owner_id = $%[2]d`
)

// scanSources drains rows of sourceSelectCols.
func scanSources(rows *sql.Rows) ([]repository.SourceReadModel, error) {
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

// scanCitations drains rows of citationSelectCols.
func scanCitations(rows *sql.Rows) ([]repository.CitationReadModel, error) {
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

// GetSource retrieves a source by ID within the branch overlay (ADR-005, #758).
func (s *ReadModelStore) GetSource(ctx context.Context, branchID domain.BranchID, id uuid.UUID) (*repository.SourceReadModel, error) {
	// #nosec G202 -- the query is built by overlayGetQuery from package constants; every value is a bound placeholder
	// nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query
	row := s.db.QueryRowContext(ctx, overlayGetQuery("sources", sourceSelectCols),
		id, branchID.UUID(), domain.MainBranchID.UUID())
	return scanSourceRow(row)
}

// ListSources returns a paginated list of the sources visible on opts.BranchID
// (ADR-005, #758).
func (s *ReadModelStore) ListSources(ctx context.Context, opts repository.ListOptions) ([]repository.SourceReadModel, int, error) {
	args, n := overlayArgs(opts.BranchID)
	src := overlaySrc("sources", sourceSelectCols, "", opts.BranchID)

	var total int
	// #nosec G202 -- src is built from package constants carrying only $-placeholders
	// nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+src+" s", args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count sources: %w", err)
	}

	// #nosec G201 G202 -- src and n are internal; limit/offset stay bound parameters
	// nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query
	query := fmt.Sprintf(`
		SELECT `+sourceSelectCols+`
		FROM %s s
		ORDER BY title ASC, id ASC
		LIMIT $%d OFFSET $%d
	`, src, n, n+1)
	rows, err := s.db.QueryContext(ctx, query, append(args, opts.Limit, opts.Offset)...)
	if err != nil {
		return nil, 0, fmt.Errorf("query sources: %w", err)
	}
	sources, err := scanSources(rows)
	if err != nil {
		return nil, 0, err
	}
	return sources, total, nil
}

// SearchSources searches the sources visible on branchID by title or author. The
// overlay is resolved FIRST (the DISTINCT ON source) and the query is matched
// against the winning rows only, so a branch retitle is found under its new
// title alone and a branch-deleted source never matches (#758).
func (s *ReadModelStore) SearchSources(ctx context.Context, branchID domain.BranchID, query string, limit int) ([]repository.SourceReadModel, error) {
	args, n := overlayArgs(branchID)
	src := overlaySrc("sources", sourceSelectCols, "", branchID)

	// #nosec G201 G202 -- src and n are internal; the query text and limit stay bound parameters
	// nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query
	q := fmt.Sprintf(`
		SELECT `+sourceSelectCols+`
		FROM %[1]s s
		WHERE title ILIKE '%%' || $%[2]d || '%%' OR author ILIKE '%%' || $%[2]d || '%%'
		ORDER BY title ASC, id ASC
		LIMIT $%[3]d
	`, src, n, n+1)
	rows, err := s.db.QueryContext(ctx, q, append(args, query, limit)...)
	if err != nil {
		return nil, fmt.Errorf("search sources: %w", err)
	}
	return scanSources(rows)
}

// SaveSource saves or updates a source on the given branch (ADR-005, #758). The
// row is keyed by (id, branch_id); a save always clears any prior tombstone.
func (s *ReadModelStore) SaveSource(ctx context.Context, branchID domain.BranchID, source *repository.SourceReadModel) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO sources (id, branch_id, source_type, title, author, publisher, publish_date_raw, publish_date_sort,
							 url, repository_id, repository_name, collection_name, call_number, notes, gedcom_xref,
							 citation_count, version, updated_at, deleted)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, FALSE)
		ON CONFLICT(id, branch_id) DO UPDATE SET
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
			updated_at = EXCLUDED.updated_at,
			deleted = FALSE
	`, source.ID, branchID.UUID(), nullableString(string(source.SourceType)), source.Title,
		nullableString(source.Author), nullableString(source.Publisher),
		nullableString(source.PublishDateRaw), nullableTime(source.PublishDateSort),
		nullableString(source.URL), nullableUUID(source.RepositoryID), nullableString(source.RepositoryName),
		nullableString(source.CollectionName), nullableString(source.CallNumber),
		nullableString(source.Notes), nullableString(source.GedcomXref),
		source.CitationCount, source.Version, source.UpdatedAt)
	if err != nil {
		return fmt.Errorf("save source: %w", err)
	}
	return nil
}

// DeleteSource removes a source on the given branch (ADR-005, #758) together
// with its external identifiers, its citations and its media (#759) — the manual cascade that
// replaces the dropped foreign keys. On main the rows are deleted; off main the
// source and every citation of it the branch sees are tombstoned, and its
// external-id bucket is replaced by an empty tombstone bucket. Only branchID's
// rows are written; other branches are untouched.
func (s *ReadModelStore) DeleteSource(ctx context.Context, branchID domain.BranchID, id uuid.UUID) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	// Take the media lock before any row lock (see lockMediaBlobs).
	if err := lockMediaBlobs(ctx, tx); err != nil {
		return err
	}

	if err := cascadeOverlayRows(ctx, tx, "citations", citationSelectCols, sourceCitationsFilter, branchID, id); err != nil {
		return err
	}
	if err := cascadeMedia(ctx, tx, sourceMediaFilter, branchID, id); err != nil {
		return err
	}
	if branchID.IsMain() {
		if _, err := tx.ExecContext(ctx, "DELETE FROM source_external_ids WHERE source_id = $1 AND branch_id = $2", id, domain.MainBranchID.UUID()); err != nil {
			return fmt.Errorf("delete source external ids: %w", err)
		}
	} else if err := tombstoneExternalIDBucket(ctx, tx, "source_external_ids", "source_id", id, branchID.UUID()); err != nil {
		return err
	}
	if err := deleteOverlayRow(ctx, tx, "sources", sourceSelectCols, branchID, id); err != nil {
		return fmt.Errorf("delete source: %w", err)
	}
	return tx.Commit()
}

// GetCitation retrieves a citation by ID within the branch overlay (ADR-005, #758).
func (s *ReadModelStore) GetCitation(ctx context.Context, branchID domain.BranchID, id uuid.UUID) (*repository.CitationReadModel, error) {
	// #nosec G202 -- the query is built by overlayGetQuery from package constants; every value is a bound placeholder
	// nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query
	row := s.db.QueryRowContext(ctx, overlayGetQuery("citations", citationSelectCols),
		id, branchID.UUID(), domain.MainBranchID.UUID())
	return scanCitationRow(row)
}

// listFilteredCitations returns the citations visible on branchID that filter
// selects, decided on the winning row. filter is a package constant whose %[1]d
// (and %[2]d) placeholders number the values, bound in order after overlayArgs.
func (s *ReadModelStore) listFilteredCitations(ctx context.Context, branchID domain.BranchID, filter string, values ...any) ([]repository.CitationReadModel, error) {
	args, n := overlayArgs(branchID)
	src := overlaySrc("citations", citationSelectCols, fmt.Sprintf(filter, n, n+1), branchID)
	// #nosec G202 -- src is built from package constants carrying only $-placeholders
	// nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query
	rows, err := s.db.QueryContext(ctx, `SELECT `+citationSelectCols+` FROM `+src+` c ORDER BY `+citationOrder,
		append(args, values...)...)
	if err != nil {
		return nil, err
	}
	return scanCitations(rows)
}

// GetCitationsForSource returns all citations of a source within the branch overlay.
func (s *ReadModelStore) GetCitationsForSource(ctx context.Context, branchID domain.BranchID, sourceID uuid.UUID) ([]repository.CitationReadModel, error) {
	citations, err := s.listFilteredCitations(ctx, branchID, sourceCitationsFilter, sourceID)
	if err != nil {
		return nil, fmt.Errorf("query citations for source: %w", err)
	}
	return citations, nil
}

// GetCitationsForPerson returns all citations of a person's facts within the
// branch overlay.
func (s *ReadModelStore) GetCitationsForPerson(ctx context.Context, branchID domain.BranchID, personID uuid.UUID) ([]repository.CitationReadModel, error) {
	citations, err := s.listFilteredCitations(ctx, branchID, personCitationsFilter, personID)
	if err != nil {
		return nil, fmt.Errorf("query citations for person: %w", err)
	}
	return citations, nil
}

// GetCitationsForFact returns all citations of a specific fact within the branch
// overlay.
func (s *ReadModelStore) GetCitationsForFact(ctx context.Context, branchID domain.BranchID, factType domain.FactType, factOwnerID uuid.UUID) ([]repository.CitationReadModel, error) {
	citations, err := s.listFilteredCitations(ctx, branchID, factCitationsFilter, string(factType), factOwnerID)
	if err != nil {
		return nil, fmt.Errorf("query citations for fact: %w", err)
	}
	return citations, nil
}

// ListCitations returns a paginated list of the citations visible on
// opts.BranchID (ADR-005, #758).
func (s *ReadModelStore) ListCitations(ctx context.Context, opts repository.ListOptions) ([]repository.CitationReadModel, int, error) {
	args, n := overlayArgs(opts.BranchID)
	src := overlaySrc("citations", citationSelectCols, "", opts.BranchID)

	var total int
	// #nosec G202 -- src is built from package constants carrying only $-placeholders
	// nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+src+" c", args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count citations: %w", err)
	}

	// #nosec G201 G202 -- src and n are internal; limit/offset stay bound parameters
	// nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query
	query := fmt.Sprintf(`
		SELECT `+citationSelectCols+`
		FROM %s c
		ORDER BY `+citationOrder+`
		LIMIT $%d OFFSET $%d
	`, src, n, n+1)
	rows, err := s.db.QueryContext(ctx, query, append(args, opts.Limit, opts.Offset)...)
	if err != nil {
		return nil, 0, fmt.Errorf("query citations: %w", err)
	}
	citations, err := scanCitations(rows)
	if err != nil {
		return nil, 0, err
	}
	return citations, total, nil
}

// SaveCitation saves or updates a citation on the given branch (ADR-005, #758).
// A save always clears any prior tombstone.
func (s *ReadModelStore) SaveCitation(ctx context.Context, branchID domain.BranchID, citation *repository.CitationReadModel) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO citations (id, branch_id, source_id, source_title, fact_type, fact_owner_id, page, volume,
							   source_quality, informant_type, evidence_type, quoted_text, analysis,
							   template_id, fields_data, gedcom_xref, version, created_at, deleted)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, FALSE)
		ON CONFLICT(id, branch_id) DO UPDATE SET
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
			version = EXCLUDED.version,
			deleted = FALSE
	`, citation.ID, branchID.UUID(), citation.SourceID, nullableString(citation.SourceTitle),
		nullableString(string(citation.FactType)), citation.FactOwnerID,
		nullableString(citation.Page), nullableString(citation.Volume),
		nullableString(string(citation.SourceQuality)), nullableString(string(citation.InformantType)),
		nullableString(string(citation.EvidenceType)), nullableString(citation.QuotedText),
		nullableString(citation.Analysis), nullableString(citation.TemplateID),
		nullableString(citation.FieldsJSON), nullableString(citation.GedcomXref),
		citation.Version, citation.CreatedAt)
	if err != nil {
		return fmt.Errorf("save citation: %w", err)
	}
	return nil
}

// DeleteCitation removes a citation (ADR-005, #758): a real removal on main, a
// tombstone on a non-main branch. Other branches' rows are untouched.
func (s *ReadModelStore) DeleteCitation(ctx context.Context, branchID domain.BranchID, id uuid.UUID) error {
	if err := deleteOverlayRow(ctx, s.db, "citations", citationSelectCols, branchID, id); err != nil {
		return fmt.Errorf("delete citation: %w", err)
	}
	return nil
}

// Person/family fact tables (#757): life_events, attributes and associations are
// branch-scoped the same way as the #669 slice (ADR-005). Each is keyed by
// (id, branch_id) and resolves per id: the branch's row wins, else main's, and a
// winning tombstone hides the id. The per-owner lists are resolved over every id
// the owner has on either side, then re-filtered on the WINNING row, so a branch
// row that re-owned an id (PersonMerged moves life events and attributes) is
// listed under its new owner only.

const (
	// eventSelectCols is scanEventRow's column order (unaliased).
	eventSelectCols = `id, owner_type, owner_id, fact_type, date_raw, date_sort,
		place, place_lat, place_long, address, description, cause,
		age, research_status, is_negated, version, created_at`

	// attributeSelectCols is scanAttributeRow's column order (unaliased).
	attributeSelectCols = `id, person_id, fact_type, value, date_raw, date_sort, place, version, created_at`

	// associationSelectCols is scanAssociationRow's column order (unaliased).
	associationSelectCols = `id, person_id, person_name, associate_id, associate_name,
		role, phrase, notes, note_ids, gedcom_xref, version, updated_at`

	// Per-owner filters for the fact tables. %[1]d is the placeholder number of
	// the owner id, which the caller binds after overlayArgs.
	personEventsFilter      = `owner_type = 'person' AND owner_id = $%[1]d`
	familyEventsFilter      = `owner_type = 'family' AND owner_id = $%[1]d`
	personAttributesFilter  = `person_id = $%[1]d`
	personAssociationFilter = `(person_id = $%[1]d OR associate_id = $%[1]d)`
)

// overlayArgs returns the leading query args bound by an overlaySrc source and
// the first free $-placeholder after them. $1 is the requested scope; off main,
// $2 is main. resolvedPersonsSrc binds the same leading args, so a statement can
// join a persons source and a fact-table source on one set of placeholders.
func overlayArgs(branchID domain.BranchID) ([]any, int) {
	if branchID.IsMain() {
		return []any{domain.MainBranchID.UUID()}, 2
	}
	return []any{branchID.UUID(), domain.MainBranchID.UUID()}, 3
}

// overlaySrc returns a parenthesized FROM source holding branchID's resolved view
// of an id-keyed branch-scoped table, projected to cols. table, cols and filter
// must be package constants (filter already formatted with the caller's
// placeholder numbers): nothing here is user input.
//
// filter optionally narrows the rows, e.g. to one owner. Off main it is applied
// twice: once to choose the candidate ids (any row of the id on either side
// matches), so the DISTINCT ON sorts only those ids rather than the whole
// table, and once to the winning row, so the answer is decided by the row the
// branch actually sees. The NOT deleted filter stays outside the DISTINCT ON so
// a branch tombstone suppresses the main fallback.
//
// Main takes the fast path (issue #669): main never shadows itself, so the plain
// branch-filtered subquery is flattened by the planner and keeps the plan it had
// before branches existed.
func overlaySrc(table, cols, filter string, branchID domain.BranchID) string {
	if branchID.IsMain() {
		// #nosec G202 -- table, cols and filter are package constants, not user input
		// nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query
		src := "(SELECT " + cols + " FROM " + table + " WHERE branch_id = $1 AND NOT deleted"
		if filter != "" {
			src += " AND " + filter
		}
		return src + ")"
	}
	candidates, outer := "", ""
	if filter != "" {
		// #nosec G202 -- table, cols and filter are package constants, not user input
		// nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query
		candidates = " AND id IN (SELECT id FROM " + table + " WHERE branch_id IN ($1, $2) AND " + filter + ")"
		outer = " AND " + filter
	}
	// #nosec G202 -- table, cols and filter are package constants, not user input
	// nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query
	return "(SELECT " + cols + " FROM (SELECT DISTINCT ON (id) " + cols + ", deleted FROM " + table +
		" WHERE branch_id IN ($1, $2)" + candidates +
		" ORDER BY id, (branch_id = $1) DESC) o WHERE NOT deleted" + outer + ")"
}

// overlayGetQuery returns the single-row overlay lookup for an id-keyed
// branch-scoped table: bind (id, branch, main). On main $2 = $3 and the query
// degenerates to the main row. table and cols must be package constants.
func overlayGetQuery(table, cols string) string {
	// #nosec G202 -- table, cols and filter are package constants, not user input
	// nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query
	return "SELECT " + cols + " FROM (SELECT DISTINCT ON (id) " + cols + ", deleted FROM " + table +
		" WHERE id = $1 AND branch_id IN ($2, $3) ORDER BY id, (branch_id = $2) DESC) o WHERE NOT deleted"
}

// sqlExecer is the ExecContext subset shared by *sql.DB and *sql.Tx.
type sqlExecer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// deleteOverlayRow removes one row of an id-keyed branch-scoped table (ADR-005).
// On main it is a real removal. On a non-main branch it writes a tombstone that
// copies the row the branch currently resolves (its NOT NULL columns need
// values), so the main fallback cannot resurrect it; with no visible row there is
// nothing to hide and nothing is written. table and cols must be package
// constants; cols must name every column of the row except branch_id/deleted.
func deleteOverlayRow(ctx context.Context, db sqlExecer, table, cols string, branchID domain.BranchID, id uuid.UUID) error {
	if branchID.IsMain() {
		// #nosec G202 -- table is a package constant, not user input
		// nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query
		_, err := db.ExecContext(ctx, "DELETE FROM "+table+" WHERE id = $1 AND branch_id = $2", id, domain.MainBranchID.UUID())
		return err
	}
	// #nosec G202 -- table and cols are package constants, not user input
	// nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query
	_, err := db.ExecContext(ctx, `
		INSERT INTO `+table+` (`+cols+`, branch_id, deleted)
		SELECT `+cols+`, $2, TRUE FROM (
			SELECT DISTINCT ON (id) `+cols+`
			FROM `+table+` WHERE id = $1 AND branch_id IN ($2, $3)
			ORDER BY id, (branch_id = $2) DESC
		) o
		ON CONFLICT (id, branch_id) DO UPDATE SET deleted = TRUE
	`, id, branchID.UUID(), domain.MainBranchID.UUID())
	return err
}

// cascadeOverlayRows removes, on branchID, every row of an id-keyed
// branch-scoped fact table that ownerFilter selects for ownerID — the manual
// cascade DeletePerson/DeleteFamily run now that the read-model foreign keys are
// gone (#669, #757). On main the rows are deleted; off main each row visible on
// the branch is tombstoned in one set-based statement. table, cols and
// ownerFilter must be package constants.
func cascadeOverlayRows(ctx context.Context, tx *sql.Tx, table, cols, ownerFilter string, branchID domain.BranchID, ownerID uuid.UUID) error {
	args, n := overlayArgs(branchID)
	filter := fmt.Sprintf(ownerFilter, n)
	args = append(args, ownerID)
	if branchID.IsMain() {
		// #nosec G201 G202 -- table and filter are package constants carrying only $-placeholders
		// nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query
		if _, err := tx.ExecContext(ctx, "DELETE FROM "+table+" WHERE branch_id = $1 AND "+filter, args...); err != nil {
			return fmt.Errorf("cascade delete %s: %w", table, err)
		}
		return nil
	}
	// #nosec G201 G202 -- table, cols and filter are package constants carrying only $-placeholders
	// nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO `+table+` (`+cols+`, branch_id, deleted)
		SELECT `+cols+`, $1, TRUE FROM `+overlaySrc(table, cols, filter, branchID)+` r
		ON CONFLICT (id, branch_id) DO UPDATE SET deleted = TRUE
	`, args...); err != nil {
		return fmt.Errorf("cascade tombstone %s: %w", table, err)
	}
	return nil
}

// GetEvent retrieves a life event by ID within the branch overlay (ADR-005).
func (s *ReadModelStore) GetEvent(ctx context.Context, branchID domain.BranchID, id uuid.UUID) (*repository.EventReadModel, error) {
	// #nosec G202 -- the query is built by overlayGetQuery from package constants; every value is a bound placeholder
	// nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query
	row := s.db.QueryRowContext(ctx, overlayGetQuery("life_events", eventSelectCols),
		id, branchID.UUID(), domain.MainBranchID.UUID())
	return scanEventRow(row)
}

// ListEvents returns a paginated list of the life events visible on
// opts.BranchID (ADR-005).
func (s *ReadModelStore) ListEvents(ctx context.Context, opts repository.ListOptions) ([]repository.EventReadModel, int, error) {
	args, n := overlayArgs(opts.BranchID)
	src := overlaySrc("life_events", eventSelectCols, "", opts.BranchID)

	var total int
	// #nosec G202 -- src is built from package constants carrying only $-placeholders
	// nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+src+" e", args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count events: %w", err)
	}

	// Sort by fact_type ASC, date_sort ASC NULLS LAST, id ASC for deterministic ordering
	// #nosec G201 G202 -- src and n are internal; limit/offset stay bound parameters
	// nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query
	query := fmt.Sprintf(`
		SELECT `+eventSelectCols+`
		FROM %s e
		ORDER BY fact_type ASC, date_sort ASC NULLS LAST, id ASC
		LIMIT $%d OFFSET $%d
	`, src, n, n+1)

	rows, err := s.db.QueryContext(ctx, query, append(args, opts.Limit, opts.Offset)...)
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

// listOwnerEvents returns one owner's life events visible on branchID.
func (s *ReadModelStore) listOwnerEvents(ctx context.Context, branchID domain.BranchID, ownerFilter string, ownerID uuid.UUID) ([]repository.EventReadModel, error) {
	args, n := overlayArgs(branchID)
	src := overlaySrc("life_events", eventSelectCols, fmt.Sprintf(ownerFilter, n), branchID)
	// #nosec G202 -- src is built from package constants carrying only $-placeholders
	// nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+eventSelectCols+` FROM `+src+` e
		ORDER BY fact_type ASC, date_sort ASC NULLS LAST, id ASC
	`, append(args, ownerID)...)
	if err != nil {
		return nil, err
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

// ListEventsForPerson returns all life events of a person within the branch overlay.
func (s *ReadModelStore) ListEventsForPerson(ctx context.Context, branchID domain.BranchID, personID uuid.UUID) ([]repository.EventReadModel, error) {
	events, err := s.listOwnerEvents(ctx, branchID, personEventsFilter, personID)
	if err != nil {
		return nil, fmt.Errorf("query events for person: %w", err)
	}
	return events, nil
}

// ListEventsForFamily returns all life events of a family within the branch overlay.
func (s *ReadModelStore) ListEventsForFamily(ctx context.Context, branchID domain.BranchID, familyID uuid.UUID) ([]repository.EventReadModel, error) {
	events, err := s.listOwnerEvents(ctx, branchID, familyEventsFilter, familyID)
	if err != nil {
		return nil, fmt.Errorf("query events for family: %w", err)
	}
	return events, nil
}

// SaveEvent saves or updates a life event on the given branch (ADR-005). The row
// is keyed by (id, branch_id); a save always clears any prior tombstone.
func (s *ReadModelStore) SaveEvent(ctx context.Context, branchID domain.BranchID, event *repository.EventReadModel) error {
	var addressJSON interface{}
	if event.Address != nil {
		data, err := json.Marshal(event.Address)
		if err != nil {
			return fmt.Errorf("marshal event address: %w", err)
		}
		addressJSON = string(data)
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO life_events (id, branch_id, owner_type, owner_id, fact_type, date_raw, date_sort,
		                    place, place_lat, place_long, address, description, cause,
		                    age, research_status, is_negated, version, created_at, deleted)
		VALUES ($1, $2, $3, $4, $5, NULLIF($6, ''), $7, NULLIF($8, ''), NULLIF($9, ''), NULLIF($10, ''),
		        $11, NULLIF($12, ''), NULLIF($13, ''), NULLIF($14, ''), NULLIF($15, ''), $16, $17, $18, FALSE)
		ON CONFLICT (id, branch_id) DO UPDATE SET
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
			version = EXCLUDED.version,
			deleted = FALSE
	`, event.ID, branchID.UUID(), event.OwnerType, event.OwnerID, string(event.FactType),
		event.DateRaw, nullableTime(event.DateSort), event.Place,
		nullableStringPtr(event.PlaceLat), nullableStringPtr(event.PlaceLong),
		addressJSON, event.Description, event.Cause, event.Age,
		nullableString(string(event.ResearchStatus)), event.IsNegated, event.Version, event.CreatedAt)
	if err != nil {
		return fmt.Errorf("save event: %w", err)
	}
	return nil
}

// DeleteEvent removes a life event (ADR-005): a real removal on main, a
// tombstone on a non-main branch.
func (s *ReadModelStore) DeleteEvent(ctx context.Context, branchID domain.BranchID, id uuid.UUID) error {
	if err := deleteOverlayRow(ctx, s.db, "life_events", eventSelectCols, branchID, id); err != nil {
		return fmt.Errorf("delete event: %w", err)
	}
	return nil
}

// GetAttribute retrieves an attribute by ID within the branch overlay (ADR-005).
func (s *ReadModelStore) GetAttribute(ctx context.Context, branchID domain.BranchID, id uuid.UUID) (*repository.AttributeReadModel, error) {
	// #nosec G202 -- the query is built by overlayGetQuery from package constants; every value is a bound placeholder
	// nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query
	row := s.db.QueryRowContext(ctx, overlayGetQuery("attributes", attributeSelectCols),
		id, branchID.UUID(), domain.MainBranchID.UUID())
	return scanAttributeRow(row)
}

// ListAttributes returns a paginated list of the attributes visible on
// opts.BranchID (ADR-005).
func (s *ReadModelStore) ListAttributes(ctx context.Context, opts repository.ListOptions) ([]repository.AttributeReadModel, int, error) {
	args, n := overlayArgs(opts.BranchID)
	src := overlaySrc("attributes", attributeSelectCols, "", opts.BranchID)

	var total int
	// #nosec G202 -- src is built from package constants carrying only $-placeholders
	// nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+src+" a", args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count attributes: %w", err)
	}

	// Sort by fact_type ASC, value ASC, id ASC for deterministic ordering
	// #nosec G201 G202 -- src and n are internal; limit/offset stay bound parameters
	// nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query
	query := fmt.Sprintf(`
		SELECT `+attributeSelectCols+`
		FROM %s a
		ORDER BY fact_type ASC, value ASC, id ASC
		LIMIT $%d OFFSET $%d
	`, src, n, n+1)

	rows, err := s.db.QueryContext(ctx, query, append(args, opts.Limit, opts.Offset)...)
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

// ListAttributesForPerson returns all attributes of a person within the branch overlay.
func (s *ReadModelStore) ListAttributesForPerson(ctx context.Context, branchID domain.BranchID, personID uuid.UUID) ([]repository.AttributeReadModel, error) {
	args, n := overlayArgs(branchID)
	src := overlaySrc("attributes", attributeSelectCols, fmt.Sprintf(personAttributesFilter, n), branchID)
	// #nosec G202 -- src is built from package constants carrying only $-placeholders
	// nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+attributeSelectCols+` FROM `+src+` a
		ORDER BY fact_type ASC, value ASC, id ASC
	`, append(args, personID)...)
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

// SaveAttribute saves or updates an attribute on the given branch (ADR-005). A
// save always clears any prior tombstone.
func (s *ReadModelStore) SaveAttribute(ctx context.Context, branchID domain.BranchID, attribute *repository.AttributeReadModel) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO attributes (id, branch_id, person_id, fact_type, value, date_raw, date_sort, place, version, created_at, deleted)
		VALUES ($1, $2, $3, $4, $5, NULLIF($6, ''), $7, NULLIF($8, ''), $9, $10, FALSE)
		ON CONFLICT (id, branch_id) DO UPDATE SET
			person_id = EXCLUDED.person_id,
			fact_type = EXCLUDED.fact_type,
			value = EXCLUDED.value,
			date_raw = EXCLUDED.date_raw,
			date_sort = EXCLUDED.date_sort,
			place = EXCLUDED.place,
			version = EXCLUDED.version,
			deleted = FALSE
	`, attribute.ID, branchID.UUID(), attribute.PersonID, string(attribute.FactType),
		attribute.Value, attribute.DateRaw, nullableTime(attribute.DateSort),
		attribute.Place, attribute.Version, attribute.CreatedAt)
	if err != nil {
		return fmt.Errorf("save attribute: %w", err)
	}
	return nil
}

// DeleteAttribute removes an attribute (ADR-005): a real removal on main, a
// tombstone on a non-main branch.
func (s *ReadModelStore) DeleteAttribute(ctx context.Context, branchID domain.BranchID, id uuid.UUID) error {
	if err := deleteOverlayRow(ctx, s.db, "attributes", attributeSelectCols, branchID, id); err != nil {
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
		factType                         string
		sourceTitle                      sql.NullString // SaveCitation stores an empty title as NULL
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
		SourceTitle:   sourceTitle.String,
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

// Media (#759) is branch-scoped for its METADATA only; the file bytes are
// shared. See the blob rule on repository.ReadModelStore: the bytes live on the
// item's origin row (the row MediaCreated wrote), a branch shadow row of a
// mainline item holds NULL bytes, and the byte reads fall back from the winning
// row to the main row. GetMedia and ListMediaForEntity project mediaSelectCols,
// which names no byte column, so neither ever reads file_data or
// thumbnail_data.

const (
	// mediaSelectCols is scanMediaRow's column order (unaliased). It deliberately
	// omits file_data and thumbnail_data.
	mediaSelectCols = `id, entity_type, entity_id, title, description, mime_type, media_type,
		filename, file_size, crop_left, crop_top, crop_width, crop_height,
		gedcom_xref, version, created_at, updated_at, files, format, translations`

	// mediaWinnerCols is mediaSelectCols qualified by the w alias the byte reads
	// join the winning row under.
	mediaWinnerCols = `w.id, w.entity_type, w.entity_id, w.title, w.description, w.mime_type, w.media_type,
		w.filename, w.file_size, w.crop_left, w.crop_top, w.crop_width, w.crop_height,
		w.gedcom_xref, w.version, w.created_at, w.updated_at, w.files, w.format, w.translations`

	// mediaWinnerJoin resolves the winning row of one media id WITHOUT reading
	// its bytes (o picks the row by (id, branch_id) alone), joins it back as w,
	// and joins main's row as m only when the winner is not main's own row, so
	// the byte columns can COALESCE from the winner to main. Bind (id, branch,
	// main); on main $2 = $3 and m never joins.
	mediaWinnerJoin = `FROM (
			SELECT DISTINCT ON (id) id, branch_id, deleted FROM media
			WHERE id = $1 AND branch_id IN ($2, $3)
			ORDER BY id, (branch_id = $2) DESC
		) o
		JOIN media w ON w.id = o.id AND w.branch_id = o.branch_id
		LEFT JOIN media m ON m.id = o.id AND m.branch_id = $3 AND o.branch_id <> $3
		WHERE NOT o.deleted`

	// Per-owner media filters. %[1]d is the placeholder number of the owner id,
	// which the caller binds after overlayArgs.
	personMediaFilter = `entity_type = 'person' AND entity_id = $%[1]d`
	familyMediaFilter = `entity_type = 'family' AND entity_id = $%[1]d`
	sourceMediaFilter = `entity_type = 'source' AND entity_id = $%[1]d`
	// entityMediaFilter backs ListMediaForEntity: %[1]d numbers the entity type,
	// %[2]d the entity id.
	entityMediaFilter = `entity_type = $%[1]d AND entity_id = $%[2]d`
	// mediaIDFilter selects one media id for deleteMainMedia.
	mediaIDFilter = `id = $%[1]d`

	// Main-tombstone collection after a branch-side delete: main rows kept alive
	// only for a shadow the branch has now tombstoned. Bind (branch, main).
	gcMainMediaAfterDelete = `
		DELETE FROM media m
		WHERE m.branch_id = $2 AND m.deleted
		  AND m.id IN (SELECT id FROM media WHERE branch_id = $1 AND deleted)
		  AND NOT EXISTS (SELECT 1 FROM media b WHERE b.id = m.id AND b.branch_id <> $2 AND NOT b.deleted)`

	// Main-tombstone collection before PurgeBranch drops the branch's rows: main
	// rows kept alive only for this branch's shadows. Bind (branch, main).
	gcMainMediaBeforePurge = `
		DELETE FROM media m
		WHERE m.branch_id = $2 AND m.deleted
		  AND m.id IN (SELECT id FROM media WHERE branch_id = $1)
		  AND NOT EXISTS (SELECT 1 FROM media b WHERE b.id = m.id AND b.branch_id NOT IN ($1, $2) AND NOT b.deleted)`
)

// GetMedia retrieves media metadata by ID within the branch overlay (ADR-005,
// #759). It never reads the file bytes.
func (s *ReadModelStore) GetMedia(ctx context.Context, branchID domain.BranchID, id uuid.UUID) (*repository.MediaReadModel, error) {
	// #nosec G202 -- the query is built by overlayGetQuery from package constants; every value is a bound placeholder
	// nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query
	row := s.db.QueryRowContext(ctx, overlayGetQuery("media", mediaSelectCols),
		id, branchID.UUID(), domain.MainBranchID.UUID())
	return scanMediaRow(row, false)
}

// GetMediaWithData retrieves the full media record within the branch overlay:
// the winning row's metadata, and the shared bytes from the winning row else the
// main row (#759).
func (s *ReadModelStore) GetMediaWithData(ctx context.Context, branchID domain.BranchID, id uuid.UUID) (*repository.MediaReadModel, error) {
	// #nosec G202 -- the query is assembled from package constants; every value is a bound placeholder
	// nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query
	row := s.db.QueryRowContext(ctx, `SELECT `+mediaWinnerCols+`,
			COALESCE(w.file_data, m.file_data), COALESCE(w.thumbnail_data, m.thumbnail_data)
		`+mediaWinnerJoin,
		id, branchID.UUID(), domain.MainBranchID.UUID())
	return scanMediaRow(row, true)
}

// GetMediaThumbnail retrieves just the thumbnail bytes of a media item visible
// on branchID, read from the winning row else the main row (#759). It returns
// nil when the item is absent or tombstoned on the branch.
func (s *ReadModelStore) GetMediaThumbnail(ctx context.Context, branchID domain.BranchID, id uuid.UUID) ([]byte, error) {
	var thumbnail []byte
	// #nosec G202 -- the query is assembled from package constants; every value is a bound placeholder
	// nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query
	err := s.db.QueryRowContext(ctx, `SELECT COALESCE(w.thumbnail_data, m.thumbnail_data) `+mediaWinnerJoin,
		id, branchID.UUID(), domain.MainBranchID.UUID()).Scan(&thumbnail)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get media thumbnail: %w", err)
	}
	return thumbnail, nil
}

// ListMediaForEntity returns a paginated list of the media attached to an
// entity as opts.BranchID sees it (ADR-005, #759), newest first. The entity
// filter is decided on each item's winning row, and no byte column is read.
func (s *ReadModelStore) ListMediaForEntity(ctx context.Context, entityType string, entityID uuid.UUID, opts repository.ListOptions) ([]repository.MediaReadModel, int, error) {
	args, n := overlayArgs(opts.BranchID)
	src := overlaySrc("media", mediaSelectCols, fmt.Sprintf(entityMediaFilter, n, n+1), opts.BranchID)
	args = append(args, entityType, entityID)

	var total int
	// #nosec G202 -- src is built from package constants carrying only $-placeholders
	// nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+src+" md", args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count media: %w", err)
	}

	// #nosec G201 G202 -- src and n are internal; limit/offset stay bound parameters
	// nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query
	query := fmt.Sprintf(`
		SELECT `+mediaSelectCols+`
		FROM %s md
		ORDER BY created_at DESC, id DESC
		LIMIT $%d OFFSET $%d
	`, src, n+2, n+3)

	rows, err := s.db.QueryContext(ctx, query, append(args, opts.Limit, opts.Offset)...)
	if err != nil {
		return nil, 0, fmt.Errorf("query media: %w", err)
	}
	defer rows.Close()

	var items []repository.MediaReadModel
	for rows.Next() {
		m, err := scanMediaRow(rows, false)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, *m)
	}

	return items, total, rows.Err()
}

// SaveMedia saves or updates a media record on the given branch (ADR-005, #759).
// A save always clears any prior tombstone. It enforces the blob rule in the
// statement itself: on a non-main branch, an id that has a main row gets NULL
// bytes whatever the caller passes (the shadow borrows main's), and nil bytes
// never overwrite stored ones. A byte-less save on a non-main branch of an id
// that has no row there and none on main (a metadata edit of an item main
// deleted meanwhile) is a no-op, so it can never leave a shadow with no bytes.
// It runs under lockMediaBlobs, so those checks cannot interleave with a
// concurrent mainline delete.
func (s *ReadModelStore) SaveMedia(ctx context.Context, branchID domain.BranchID, media *repository.MediaReadModel) error {
	// Serialize JSONB fields
	filesJSON, err := domain.MarshalFilesToJSON(media.Files)
	if err != nil {
		return fmt.Errorf("marshal files: %w", err)
	}
	translationsJSON, err := domain.MarshalTranslationsToJSON(media.Translations)
	if err != nil {
		return fmt.Errorf("marshal translations: %w", err)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := lockMediaBlobs(ctx, tx); err != nil {
		return err
	}
	if !branchID.IsMain() && len(media.FileData) == 0 {
		var known bool
		if err := tx.QueryRowContext(ctx,
			`SELECT EXISTS (SELECT 1 FROM media WHERE id = $1 AND branch_id IN ($2, $3))`,
			media.ID, branchID.UUID(), domain.MainBranchID.UUID()).Scan(&known); err != nil {
			return fmt.Errorf("check media rows: %w", err)
		}
		if !known {
			return nil
		}
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO media (id, branch_id, entity_type, entity_id, title, description, mime_type, media_type,
						  filename, file_size, file_data, thumbnail_data,
						  crop_left, crop_top, crop_width, crop_height,
						  gedcom_xref, version, created_at, updated_at,
						  files, format, translations, deleted)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10,
			CASE WHEN $2::uuid <> $23::uuid AND EXISTS (SELECT 1 FROM media WHERE id = $1 AND branch_id = $23::uuid)
				THEN NULL ELSE $11::bytea END,
			CASE WHEN $2::uuid <> $23::uuid AND EXISTS (SELECT 1 FROM media WHERE id = $1 AND branch_id = $23::uuid)
				THEN NULL ELSE $12::bytea END,
			$13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $24, FALSE)
		ON CONFLICT (id, branch_id) DO UPDATE SET
			entity_type = EXCLUDED.entity_type,
			entity_id = EXCLUDED.entity_id,
			title = EXCLUDED.title,
			description = EXCLUDED.description,
			mime_type = EXCLUDED.mime_type,
			media_type = EXCLUDED.media_type,
			filename = EXCLUDED.filename,
			file_size = EXCLUDED.file_size,
			file_data = COALESCE(EXCLUDED.file_data, media.file_data),
			thumbnail_data = COALESCE(EXCLUDED.thumbnail_data, media.thumbnail_data),
			crop_left = EXCLUDED.crop_left,
			crop_top = EXCLUDED.crop_top,
			crop_width = EXCLUDED.crop_width,
			crop_height = EXCLUDED.crop_height,
			gedcom_xref = EXCLUDED.gedcom_xref,
			version = EXCLUDED.version,
			updated_at = EXCLUDED.updated_at,
			files = EXCLUDED.files,
			format = EXCLUDED.format,
			translations = EXCLUDED.translations,
			deleted = FALSE
	`, media.ID, branchID.UUID(), media.EntityType, media.EntityID, media.Title,
		nullableString(media.Description), media.MimeType, string(media.MediaType),
		media.Filename, media.FileSize, nullableBytes(media.FileData), nullableBytes(media.ThumbnailData),
		nullableInt(media.CropLeft), nullableInt(media.CropTop),
		nullableInt(media.CropWidth), nullableInt(media.CropHeight),
		nullableString(media.GedcomXref), media.Version, media.CreatedAt, media.UpdatedAt,
		nullableBytes(filesJSON), nullableString(media.Format), domain.MainBranchID.UUID(), nullableBytes(translationsJSON))
	if err != nil {
		return fmt.Errorf("save media: %w", err)
	}
	if branchID.IsMain() {
		// A branch upload merged into main: main's row now holds the bytes, so
		// the branch's origin row becomes a shadow and drops its copy (#759).
		if _, err := tx.ExecContext(ctx, releaseBranchMediaBytes, media.ID, domain.MainBranchID.UUID()); err != nil {
			return fmt.Errorf("release branch media bytes: %w", err)
		}
	}
	return tx.Commit()
}

// releaseBranchMediaBytes clears, on every non-main row of media $1, each byte
// column main's row ($2) now also holds, so an item's bytes are stored exactly
// once (ADR-005, #759).
const releaseBranchMediaBytes = `UPDATE media b SET
		file_data = CASE WHEN m.file_data IS NOT NULL THEN NULL ELSE b.file_data END,
		thumbnail_data = CASE WHEN m.thumbnail_data IS NOT NULL THEN NULL ELSE b.thumbnail_data END
	FROM media m
	WHERE b.id = $1 AND b.branch_id <> $2 AND m.id = $1 AND m.branch_id = $2
		AND ((b.file_data IS NOT NULL AND m.file_data IS NOT NULL)
			OR (b.thumbnail_data IS NOT NULL AND m.thumbnail_data IS NOT NULL))`

// DeleteMedia removes a media item on the given branch (ADR-005, #759). On a
// non-main branch it writes a metadata-only tombstone and never touches main's
// row or bytes. On main it is a real removal — unless a branch still shows the
// item through a live shadow row, in which case main's row is kept as a
// tombstone so that shadow keeps its bytes.
func (s *ReadModelStore) DeleteMedia(ctx context.Context, branchID domain.BranchID, id uuid.UUID) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := lockMediaBlobs(ctx, tx); err != nil {
		return err
	}

	if branchID.IsMain() {
		if err := deleteMainMedia(ctx, tx, mediaIDFilter, id); err != nil {
			return err
		}
		return tx.Commit()
	}
	if err := deleteOverlayRow(ctx, tx, "media", mediaSelectCols, branchID, id); err != nil {
		return fmt.Errorf("delete media: %w", err)
	}
	if err := gcMainMedia(ctx, tx, gcMainMediaAfterDelete, branchID); err != nil {
		return err
	}
	return tx.Commit()
}

// cascadeMedia removes, on branchID, every media item ownerFilter attaches to
// ownerID — the manual cascade DeletePerson/DeleteFamily/DeleteSource run for
// media (#759), which never had a foreign key to its owner. Off main each item
// the branch sees is tombstoned (metadata only); on main the rows go through
// deleteMainMedia so a live branch shadow keeps its bytes.
func cascadeMedia(ctx context.Context, tx *sql.Tx, ownerFilter string, branchID domain.BranchID, ownerID uuid.UUID) error {
	if err := lockMediaBlobs(ctx, tx); err != nil {
		return err
	}
	if branchID.IsMain() {
		return deleteMainMedia(ctx, tx, ownerFilter, ownerID)
	}
	if err := cascadeOverlayRows(ctx, tx, "media", mediaSelectCols, ownerFilter, branchID, ownerID); err != nil {
		return err
	}
	return gcMainMedia(ctx, tx, gcMainMediaAfterDelete, branchID)
}

// deleteMainMedia removes the main rows filter selects for value ($2): each is
// hard-deleted, except that one a non-main branch still shows through a live
// shadow row becomes a main tombstone instead, keeping the bytes that shadow
// borrows (#759). filter must be a package constant whose %[1]d is the value's
// placeholder.
func deleteMainMedia(ctx context.Context, tx *sql.Tx, filter string, value any) error {
	if err := lockMediaBlobs(ctx, tx); err != nil {
		return err
	}
	f := fmt.Sprintf(filter, 2)
	const liveShadow = `EXISTS (SELECT 1 FROM media b WHERE b.id = m.id AND b.branch_id <> $1 AND NOT b.deleted)`
	main := domain.MainBranchID.UUID()
	// #nosec G202 -- filter and liveShadow are package constants carrying only $-placeholders
	// nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query
	if _, err := tx.ExecContext(ctx, `UPDATE media m SET deleted = TRUE
		WHERE m.branch_id = $1 AND NOT m.deleted AND `+f+` AND `+liveShadow, main, value); err != nil {
		return fmt.Errorf("tombstone shared media on main: %w", err)
	}
	// #nosec G202 -- filter and liveShadow are package constants carrying only $-placeholders
	// nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query
	if _, err := tx.ExecContext(ctx, `DELETE FROM media m
		WHERE m.branch_id = $1 AND `+f+` AND NOT `+liveShadow, main, value); err != nil {
		return fmt.Errorf("delete media on main: %w", err)
	}
	return nil
}

// gcMainMedia drops the main media tombstones that no live branch shadow needs
// any more; stmt is gcMainMediaAfterDelete or gcMainMediaBeforePurge.
func gcMainMedia(ctx context.Context, db sqlExecer, stmt string, branchID domain.BranchID) error {
	if err := lockMediaBlobs(ctx, db); err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, stmt, branchID.UUID(), domain.MainBranchID.UUID()); err != nil {
		return fmt.Errorf("collect main media tombstones: %w", err)
	}
	return nil
}

// mediaBlobLockKey is the transaction-scoped advisory lock (pg_advisory_xact_lock)
// every media write that reads OTHER branches' rows takes first: SaveMedia's
// "does main have this id" and "is this id known at all" checks, and the
// mainline delete and tombstone collection's "does a live branch shadow exist"
// checks. Under READ COMMITTED those checks and the writes that depend on them
// would otherwise interleave across concurrent requests on different branches —
// a branch shadow committing between a main delete's check and its commit would
// be left with no bytes, or the delete would be skipped. One key for all ids
// keeps it simple and deadlock-free; media writes are infrequent, and SQLite
// already serializes every writer. The value is arbitrary ("media" in ASCII).
const mediaBlobLockKey int64 = 0x6d65646961

// lockMediaBlobs takes mediaBlobLockKey for the rest of db's transaction. It
// is re-entrant within a transaction, so nested callers may each take it.
// Every transaction that takes it must do so before its first row write, so
// the advisory lock is always ordered before row locks: a DeletePerson that
// locked a family_children row and then waited here, while a DeleteFamily
// holding the lock waited for that row, would otherwise deadlock.
func lockMediaBlobs(ctx context.Context, db sqlExecer) error {
	if _, err := db.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1)`, mediaBlobLockKey); err != nil {
		return fmt.Errorf("lock media blobs: %w", err)
	}
	return nil
}

// scanMediaRow scans one mediaSelectCols row, followed by the file and
// thumbnail bytes when withData is set. It returns (nil, nil) for
// sql.ErrNoRows so single-row lookups report absence as nil.
func scanMediaRow(row rowScanner, withData bool) (*repository.MediaReadModel, error) {
	var (
		m                           repository.MediaReadModel
		mediaType                   string
		description, gedcomXref     sql.NullString
		cropLeft, cropTop           sql.NullInt64
		cropWidth, cropHeight       sql.NullInt64
		filesJSON, translationsJSON []byte
		format                      sql.NullString
	)

	dest := []any{&m.ID, &m.EntityType, &m.EntityID, &m.Title, &description,
		&m.MimeType, &mediaType, &m.Filename, &m.FileSize,
		&cropLeft, &cropTop, &cropWidth, &cropHeight,
		&gedcomXref, &m.Version, &m.CreatedAt, &m.UpdatedAt,
		&filesJSON, &format, &translationsJSON}
	if withData {
		dest = append(dest, &m.FileData, &m.ThumbnailData)
	}

	err := row.Scan(dest...)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan media: %w", err)
	}

	// Deserialize JSONB fields
	if m.Files, err = domain.UnmarshalFilesFromJSON(filesJSON); err != nil {
		return nil, fmt.Errorf("unmarshal files: %w", err)
	}
	if m.Translations, err = domain.UnmarshalTranslationsFromJSON(translationsJSON); err != nil {
		return nil, fmt.Errorf("unmarshal translations: %w", err)
	}

	m.MediaType = domain.MediaType(mediaType)
	m.Description = description.String
	m.GedcomXref = gedcomXref.String
	m.Format = format.String
	m.CropLeft = nullIntPtr(cropLeft)
	m.CropTop = nullIntPtr(cropTop)
	m.CropWidth = nullIntPtr(cropWidth)
	m.CropHeight = nullIntPtr(cropHeight)
	return &m, nil
}

// nullIntPtr converts a nullable integer column to an *int.
func nullIntPtr(v sql.NullInt64) *int {
	if !v.Valid {
		return nil
	}
	i := int(v.Int64)
	return &i
}

// resolvedPersonsSrc builds the persons source for a branch-scoped browse/map
// aggregate (ADR-005). It returns a parenthesized subquery yielding cols from the
// branch's resolved view of persons, the leading query args, and the next free
// $-placeholder number for the caller's own parameters. Callers alias it
// (FROM <src> p) and read unqualified column names out of it, so cols must be
// unqualified (personSelectCols, not personCols).
//
// Off main the whole overlay resolves in ONE set-based pass: the inner
// SELECT DISTINCT ON (id) prefers the branch's row over main's for each identity,
// and the OUTER "WHERE NOT deleted" drops identities whose winning row is a
// tombstone. The NOT deleted filter must stay outside the DISTINCT ON, or a branch
// tombstone would fail to suppress the main fallback.
//
// Main takes the fast path (issue #669): main never shadows itself, so persons holds
// exactly one row per id and the overlay is pure overhead that would materialize and
// sort the table before the aggregate runs. The plain branch-filtered subquery is
// inlined by the planner, so a mainline aggregate keeps the plan it had before
// branches existed. Main rows are hard-deleted (see DeletePerson), so NOT deleted is
// a no-op there and only guards a rebuilt-from-branch row.
func resolvedPersonsSrc(cols string, branchID domain.BranchID) (string, []any, int) {
	if branchID.IsMain() {
		return "(SELECT " + cols + " FROM persons WHERE branch_id = $1 AND NOT deleted)",
			[]any{domain.MainBranchID.UUID()}, 2
	}
	return `(
			SELECT ` + cols + ` FROM (
				SELECT DISTINCT ON (id) ` + cols + `, deleted
				FROM persons WHERE branch_id IN ($1, $2)
				ORDER BY id, (branch_id = $1) DESC
			) o WHERE NOT deleted
		)`, []any{branchID.UUID(), domain.MainBranchID.UUID()}, 3
}

// resolvedPersonsCTE is resolvedPersonsSrc for a statement that reads the overlay
// more than once (the birth-place and death-place legs of GetPlaceHierarchy's UNION).
// Off main it hoists the overlay into a `resolved` CTE and returns the WITH head the
// caller prefixes to its own CTE list, so the DISTINCT ON pass over the branch's rows
// plus all of main's is planned and executed once per call instead of once per leg.
// A CTE referenced more than once is materialized by default in PG12+, which is the
// behavior wanted here.
//
// Main keeps the inline subquery and gets a bare "WITH " head: its plain branch_id
// filter is index-driven and flattens into each leg, touching only that leg's column,
// so a mainline call must not start materializing a CTE it did not before (#669).
//
// Returns the WITH head, the source to read FROM, the leading query args, and the next
// free $-placeholder number. The args are the same either way -- $-placeholders are
// reused, not repeated, so hoisting does not change the arg list.
func resolvedPersonsCTE(cols string, branchID domain.BranchID) (string, string, []any, int) {
	src, args, n := resolvedPersonsSrc(cols, branchID)
	if branchID.IsMain() {
		return "WITH ", src, args, n
	}
	return "WITH resolved AS " + src + ",\n\t\t\t", "resolved", args, n
}

// GetSurnameIndex returns all unique surnames with counts and letter counts,
// resolved through the branch overlay (ADR-005).
func (s *ReadModelStore) GetSurnameIndex(ctx context.Context, branchID domain.BranchID) ([]repository.SurnameEntry, []repository.LetterCount, error) {
	// Both queries read the same resolved source so the counts agree on scope.
	src, args, _ := resolvedPersonsSrc("surname", branchID)

	// Get surname counts
	// #nosec G201 G202 -- src is an internal SQL fragment carrying only $-placeholders, not user input
	// nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query
	rows, err := s.db.QueryContext(ctx, `
		SELECT surname, COUNT(*) as count
		FROM `+src+` p
		GROUP BY surname
		ORDER BY surname ASC
	`, args...)
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
	// #nosec G201 G202 -- src is an internal SQL fragment carrying only $-placeholders, not user input
	// nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query
	letterRows, err := s.db.QueryContext(ctx, `
		SELECT UPPER(SUBSTRING(surname, 1, 1)) as letter, COUNT(DISTINCT surname) as count
		FROM `+src+` p
		WHERE surname != ''
		GROUP BY UPPER(SUBSTRING(surname, 1, 1))
		ORDER BY letter ASC
	`, args...)
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

// GetSurnamesByLetter returns surnames starting with a specific letter, resolved
// through the branch overlay (ADR-005).
func (s *ReadModelStore) GetSurnamesByLetter(ctx context.Context, branchID domain.BranchID, letter string) ([]repository.SurnameEntry, error) {
	src, args, n := resolvedPersonsSrc("surname", branchID)

	// #nosec G201 -- src/n are internal SQL fragments; letter stays a bound parameter
	// nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query
	query := fmt.Sprintf(`
		SELECT surname, COUNT(*) as count
		FROM %s p
		WHERE UPPER(SUBSTRING(surname, 1, 1)) = UPPER($%d)
		GROUP BY surname
		ORDER BY surname ASC
	`, src, n)
	rows, err := s.db.QueryContext(ctx, query, append(args, letter)...)
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

// GetPersonsBySurname returns persons with a specific surname, scoped to
// opts.BranchID through the branch overlay (ADR-005).
func (s *ReadModelStore) GetPersonsBySurname(ctx context.Context, surname string, opts repository.ListOptions) ([]repository.PersonReadModel, int, error) {
	// One resolved source for both statements so the count and the page agree on scope.
	src, args, n := resolvedPersonsSrc(personSelectCols, opts.BranchID)
	countArgs := append(args, surname)

	// Count total
	var total int
	// #nosec G201 -- src/n are internal SQL fragments; surname stays a bound parameter
	// nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM %s p WHERE LOWER(surname) = LOWER($%d)", src, n)
	err := s.db.QueryRowContext(ctx, countQuery, countArgs...).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("count persons by surname: %w", err)
	}

	// #nosec G201 -- src/n/personSelectCols are internal SQL fragments, not user input
	// nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query
	query := fmt.Sprintf(`
		SELECT `+personSelectCols+`
		FROM %s p
		WHERE LOWER(surname) = LOWER($%d)
		ORDER BY given_name ASC
		LIMIT $%d OFFSET $%d
	`, src, n, n+1, n+2)
	rows, err := s.db.QueryContext(ctx, query, append(countArgs, opts.Limit, opts.Offset)...)
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
func (s *ReadModelStore) GetPlaceHierarchy(ctx context.Context, branchID domain.BranchID, parent string) ([]repository.PlaceEntry, error) {
	var rows *sql.Rows
	var err error

	// Both levels read persons twice (birth + death). Off main the overlay is hoisted
	// into a `resolved` CTE so both UNION legs share one resolution pass; main keeps the
	// inline indexed filter. Either way the UNION is branch-scoped end to end (ADR-005).
	// Names sort with the "C" collation so the order is bytewise, as it is on SQLite.
	withClause, src, args, n := resolvedPersonsCTE("birth_place, death_place", branchID)

	if parent == "" {
		// Top-level: get unique countries/top-level places (rightmost part after last comma)
		// #nosec G201 -- withClause/src are internal SQL fragments carrying only $-placeholders, not user input
		// nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query
		rows, err = s.db.QueryContext(ctx, fmt.Sprintf(`
			%[1]sall_places AS (
				SELECT DISTINCT birth_place as place FROM %[2]s p WHERE birth_place != '' AND birth_place IS NOT NULL
				UNION
				SELECT DISTINCT death_place as place FROM %[2]s p WHERE death_place != '' AND death_place IS NOT NULL
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
			ORDER BY top_level COLLATE "C" ASC
		`, withClause, src), args...)
	} else {
		// Child level: places whose last parts are parent's parts. parent is a
		// full_name built with ", " while GEDCOM places may omit the space, so both
		// sides compare with ", " collapsed to ",".
		// #nosec G201 -- withClause/src/n are internal SQL fragments; parent stays a bound parameter
		// nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query
		rows, err = s.db.QueryContext(ctx, fmt.Sprintf(`
			%[1]sall_places AS (
				SELECT DISTINCT birth_place as place FROM %[2]s p WHERE birth_place != '' AND birth_place IS NOT NULL
				UNION
				SELECT DISTINCT death_place as place FROM %[2]s p WHERE death_place != '' AND death_place IS NOT NULL
			),
			normalized AS (
				SELECT
					place,
					REPLACE(place, ', ', ',') as norm,
					REPLACE($%[3]d, ', ', ',') as norm_parent
				FROM all_places
			),
			parsed AS (
				SELECT
					place,
					TRIM(LEFT(norm, LENGTH(norm) - LENGTH(norm_parent) - 1)) as remainder
				FROM normalized
				WHERE RIGHT(norm, LENGTH(norm_parent) + 1) = ',' || norm_parent
			),
			next_level AS (
				SELECT
					place,
					remainder,
					CASE
						WHEN POSITION(',' IN remainder) > 0
						THEN TRIM(SPLIT_PART(remainder, ',', ARRAY_LENGTH(STRING_TO_ARRAY(remainder, ','), 1)))
						ELSE TRIM(remainder)
					END as level_name
				FROM parsed
			)
			SELECT
				level_name as place_name,
				level_name || ', ' || $%[3]d as full_name,
				COUNT(DISTINCT place) as count,
				CASE
					WHEN COUNT(DISTINCT place) > COUNT(DISTINCT CASE WHEN remainder = level_name THEN place END)
					THEN true
					ELSE false
				END as has_children
			FROM next_level
			WHERE level_name != ''
			GROUP BY level_name
			ORDER BY level_name COLLATE "C" ASC
		`, withClause, src, n), append(args, parent)...)
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

// GetPersonsByPlace returns persons associated with a place, scoped to
// opts.BranchID through the branch overlay (ADR-005).
func (s *ReadModelStore) GetPersonsByPlace(ctx context.Context, place string, opts repository.ListOptions) ([]repository.PersonReadModel, int, error) {
	// One resolved source for both statements so the count and the page agree on scope.
	src, args, n := resolvedPersonsSrc(personSelectCols, opts.BranchID)
	countArgs := append(args, place)

	// Count total - match place at any position in birth_place or death_place.
	// place is usually a GetPlaceHierarchy full_name (parts joined with ", "), so
	// both sides compare with ", " collapsed to "," as the hierarchy does.
	var total int
	// #nosec G201 -- src/n are internal SQL fragments; place stays a bound parameter
	// nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query
	countQuery := fmt.Sprintf(`
		SELECT COUNT(*) FROM %[1]s p
		WHERE REPLACE(birth_place, ', ', ',') ILIKE '%%' || REPLACE($%[2]d, ', ', ',') || '%%'
		   OR REPLACE(death_place, ', ', ',') ILIKE '%%' || REPLACE($%[2]d, ', ', ',') || '%%'
	`, src, n)
	err := s.db.QueryRowContext(ctx, countQuery, countArgs...).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("count persons by place: %w", err)
	}

	// #nosec G201 -- src/n/personSelectCols are internal SQL fragments, not user input
	// nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query
	query := fmt.Sprintf(`
		SELECT `+personSelectCols+`
		FROM %[1]s p
		WHERE REPLACE(birth_place, ', ', ',') ILIKE '%%' || REPLACE($%[2]d, ', ', ',') || '%%'
		   OR REPLACE(death_place, ', ', ',') ILIKE '%%' || REPLACE($%[2]d, ', ', ',') || '%%'
		ORDER BY surname ASC, given_name ASC
		LIMIT $%[3]d OFFSET $%[4]d
	`, src, n, n+1, n+2)
	rows, err := s.db.QueryContext(ctx, query, append(countArgs, opts.Limit, opts.Offset)...)
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

// cemeteryEventCols are the life_events columns the cemetery pair reads.
const cemeteryEventCols = `owner_id, fact_type, place`

// GetCemeteryIndex returns unique burial/cremation places with person counts,
// counted over branchID's overlay of life events (ADR-005, #757). A branch that
// tombstoned a person also tombstoned that person's life events (DeletePerson's
// cascade), so the counts agree with GetPersonsByCemetery on the same scope.
func (s *ReadModelStore) GetCemeteryIndex(ctx context.Context, branchID domain.BranchID) ([]repository.CemeteryEntry, error) {
	args, n := overlayArgs(branchID)
	src := overlaySrc("life_events", cemeteryEventCols, fmt.Sprintf("fact_type IN ($%d, $%d)", n, n+1), branchID)
	args = append(args, string(domain.FactPersonBurial), string(domain.FactPersonCremation))

	// #nosec G202 -- src is built from package constants carrying only $-placeholders
	// nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query
	rows, err := s.db.QueryContext(ctx, `
		SELECT place, COUNT(DISTINCT owner_id) as count
		FROM `+src+` e
		WHERE place != '' AND place IS NOT NULL
		GROUP BY place
		ORDER BY place ASC
	`, args...)
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

// GetPersonsByCemetery returns persons with burial/cremation events at the given
// place. Both sides of the join resolve through opts.BranchID's overlay
// (ADR-005): the branch-visible life events select the owners, and the persons
// returned are the branch's view of them. resolvedPersonsSrc and overlaySrc bind
// the same leading args, so both sources share $1 (and $2 off main).
func (s *ReadModelStore) GetPersonsByCemetery(ctx context.Context, place string, opts repository.ListOptions) ([]repository.PersonReadModel, int, error) {
	args, n := overlayArgs(opts.BranchID)
	eventsSrc := overlaySrc("life_events", cemeteryEventCols,
		fmt.Sprintf("fact_type IN ($%d, $%d) AND LOWER(place) = LOWER($%d)", n, n+1, n+2), opts.BranchID)
	args = append(args, string(domain.FactPersonBurial), string(domain.FactPersonCremation), place)

	// Count total distinct persons
	countSrc, _, _ := resolvedPersonsSrc("id", opts.BranchID)
	var total int
	// #nosec G202 -- countSrc/eventsSrc are internal SQL fragments; fact types and place stay bound parameters
	// nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query
	countQuery := `
		SELECT COUNT(DISTINCT p.id)
		FROM ` + countSrc + ` p
		INNER JOIN ` + eventsSrc + ` e ON e.owner_id = p.id`
	if err := s.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count persons by cemetery: %w", err)
	}

	src, _, _ := resolvedPersonsSrc(personSelectCols, opts.BranchID)
	// #nosec G201 -- src/eventsSrc/personCols are internal SQL fragments, not user input
	// nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query
	query := fmt.Sprintf(`
		SELECT DISTINCT `+personCols+`
		FROM %s p
		INNER JOIN %s e ON e.owner_id = p.id
		ORDER BY p.surname ASC, p.given_name ASC
		LIMIT $%d OFFSET $%d
	`, src, eventsSrc, n+3, n+4)
	rows, err := s.db.QueryContext(ctx, query, append(args, opts.Limit, opts.Offset)...)
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

// GetMapLocations returns aggregated geographic locations from person birth/death
// coordinates, resolved through the branch overlay (ADR-005).
func (s *ReadModelStore) GetMapLocations(ctx context.Context, branchID domain.BranchID) ([]repository.MapLocation, error) {
	// Query birth locations — individual rows, aggregate in Go
	birthSrc, birthArgs, _ := resolvedPersonsSrc("id, birth_place, birth_place_lat, birth_place_long", branchID)
	// #nosec G201 G202 -- birthSrc is an internal SQL fragment carrying only $-placeholders, not user input
	// nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, birth_place, birth_place_lat, birth_place_long
		FROM `+birthSrc+` p
		WHERE birth_place_lat IS NOT NULL AND birth_place_long IS NOT NULL
		  AND birth_place_lat != '' AND birth_place_long != ''
		ORDER BY birth_place ASC
	`, birthArgs...)
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
	deathSrc, deathArgs, _ := resolvedPersonsSrc("id, death_place, death_place_lat, death_place_long", branchID)
	// #nosec G201 G202 -- deathSrc is an internal SQL fragment carrying only $-placeholders, not user input
	// nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query
	rows2, err := s.db.QueryContext(ctx, `
		SELECT id, death_place, death_place_lat, death_place_long
		FROM `+deathSrc+` p
		WHERE death_place_lat IS NOT NULL AND death_place_long IS NOT NULL
		  AND death_place_lat != '' AND death_place_long != ''
		ORDER BY death_place ASC
	`, deathArgs...)
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
//
// Main-pinned: brick-wall state is not branch data today, so the UPDATE targets the
// main row only. Without the branch predicate it rewrites every branch's shadow row
// for the person. Whether brick walls should become branch-scoped waits on deciding
// whether they become event-sourced, as #624 did for snapshots (ADR-005, "Entities
// that stay main-only").
func (s *ReadModelStore) SetBrickWall(ctx context.Context, personID uuid.UUID, note string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE persons SET brick_wall_note = $1, brick_wall_since = NOW(), brick_wall_resolved_at = NULL
		WHERE id = $2 AND branch_id = $3
	`, note, personID, domain.MainBranchID.UUID())
	return err
}

// ResolveBrickWall marks a brick wall as resolved. Main-pinned, like SetBrickWall.
func (s *ReadModelStore) ResolveBrickWall(ctx context.Context, personID uuid.UUID) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE persons SET brick_wall_resolved_at = NOW()
		WHERE id = $1 AND branch_id = $2
	`, personID, domain.MainBranchID.UUID())
	return err
}

// GetBrickWalls returns persons with brick wall status. Main-pinned, like
// SetBrickWall: a branch shadow row must not surface as a second entry for the same
// person, and a main row is never a tombstone (see DeletePerson).
func (s *ReadModelStore) GetBrickWalls(ctx context.Context, includeResolved bool) ([]repository.BrickWallEntry, error) {
	query := `
		SELECT id, full_name, brick_wall_note, brick_wall_since, brick_wall_resolved_at
		FROM persons
		WHERE branch_id = $1 AND NOT deleted AND brick_wall_since IS NOT NULL`
	if !includeResolved {
		query += ` AND brick_wall_resolved_at IS NULL`
	}
	query += ` ORDER BY brick_wall_since DESC`

	rows, err := s.db.QueryContext(ctx, query, domain.MainBranchID.UUID())
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

// scanNoteRow scans one noteSelectCols row. It returns (nil, nil) for
// sql.ErrNoRows so single-row lookups report absence as nil.
func scanNoteRow(row rowScanner) (*repository.NoteReadModel, error) {
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
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan note: %w", err)
	}
	note.MIME = mime.String
	note.Language = language.String
	note.Translations = repository.UnmarshalNoteTranslations(string(translations))
	note.GedcomXref = gedcomXref.String
	return &note, nil
}

// GetNote retrieves a note by ID within the branch overlay (ADR-005, #758).
func (s *ReadModelStore) GetNote(ctx context.Context, branchID domain.BranchID, id uuid.UUID) (*repository.NoteReadModel, error) {
	// #nosec G202 -- the query is built by overlayGetQuery from package constants; every value is a bound placeholder
	// nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query
	row := s.db.QueryRowContext(ctx, overlayGetQuery("notes", noteSelectCols),
		id, branchID.UUID(), domain.MainBranchID.UUID())
	return scanNoteRow(row)
}

// ListNotes returns a paginated list of the notes visible on opts.BranchID
// (ADR-005, #758).
func (s *ReadModelStore) ListNotes(ctx context.Context, opts repository.ListOptions) ([]repository.NoteReadModel, int, error) {
	args, n := overlayArgs(opts.BranchID)
	src := overlaySrc("notes", noteSelectCols, "", opts.BranchID)

	var total int
	// #nosec G202 -- src is built from package constants carrying only $-placeholders
	// nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+src+" n", args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count notes: %w", err)
	}

	// Build order clause
	orderDir := "DESC"
	if opts.Order == "asc" {
		orderDir = "ASC"
	}

	// #nosec G201 G202 -- orderDir is one of two literals chosen above; src and n are internal
	// nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query
	query := fmt.Sprintf(`
		SELECT `+noteSelectCols+`
		FROM %s n
		ORDER BY updated_at %s, id %s
		LIMIT $%d OFFSET $%d
	`, src, orderDir, orderDir, n, n+1)

	rows, err := s.db.QueryContext(ctx, query, append(args, opts.Limit, opts.Offset)...)
	if err != nil {
		return nil, 0, fmt.Errorf("query notes: %w", err)
	}
	defer rows.Close()

	var notes []repository.NoteReadModel
	for rows.Next() {
		note, err := scanNoteRow(rows)
		if err != nil {
			return nil, 0, err
		}
		notes = append(notes, *note)
	}

	return notes, total, rows.Err()
}

// SaveNote saves or updates a note on the given branch (ADR-005, #758). A save
// always clears any prior tombstone.
func (s *ReadModelStore) SaveNote(ctx context.Context, branchID domain.BranchID, note *repository.NoteReadModel) error {
	var translations any
	if len(note.Translations) > 0 {
		translations = repository.MarshalNoteTranslations(note.Translations)
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO notes (id, branch_id, text, mime, language, translations, gedcom_xref, version, updated_at, deleted)
		VALUES ($1, $2, $3, NULLIF($4, ''), NULLIF($5, ''), $6, NULLIF($7, ''), $8, $9, FALSE)
		ON CONFLICT (id, branch_id) DO UPDATE SET
			text = EXCLUDED.text,
			mime = EXCLUDED.mime,
			language = EXCLUDED.language,
			translations = EXCLUDED.translations,
			gedcom_xref = EXCLUDED.gedcom_xref,
			version = EXCLUDED.version,
			updated_at = EXCLUDED.updated_at,
			deleted = FALSE
	`, note.ID, branchID.UUID(), note.Text, note.MIME, note.Language, translations, note.GedcomXref, note.Version, note.UpdatedAt)
	if err != nil {
		return fmt.Errorf("save note: %w", err)
	}
	return nil
}

// DeleteNote removes a note (ADR-005, #758): a real removal on main, a tombstone
// on a non-main branch.
func (s *ReadModelStore) DeleteNote(ctx context.Context, branchID domain.BranchID, id uuid.UUID) error {
	if err := deleteOverlayRow(ctx, s.db, "notes", noteSelectCols, branchID, id); err != nil {
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

// scanAssociationRow scans one associationSelectCols row. It returns (nil, nil)
// for sql.ErrNoRows so single-row lookups report absence as nil.
func scanAssociationRow(row rowScanner) (*repository.AssociationReadModel, error) {
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
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan association: %w", err)
	}
	assoc.PersonName = personName.String
	assoc.AssociateName = associateName.String
	assoc.Phrase = phrase.String
	assoc.Notes = notes.String
	assoc.GedcomXref = gedcomXref.String
	if len(noteIDsJSON) > 0 {
		if err := json.Unmarshal(noteIDsJSON, &assoc.NoteIDs); err != nil {
			return nil, fmt.Errorf("decode association note_ids: %w", err)
		}
	}
	return &assoc, nil
}

// scanAssociations drains rows of associationSelectCols.
func scanAssociations(rows *sql.Rows) ([]repository.AssociationReadModel, error) {
	defer rows.Close()
	var associations []repository.AssociationReadModel
	for rows.Next() {
		assoc, err := scanAssociationRow(rows)
		if err != nil {
			return nil, err
		}
		associations = append(associations, *assoc)
	}
	return associations, rows.Err()
}

// GetAssociation retrieves an association by ID within the branch overlay (ADR-005).
func (s *ReadModelStore) GetAssociation(ctx context.Context, branchID domain.BranchID, id uuid.UUID) (*repository.AssociationReadModel, error) {
	// #nosec G202 -- the query is built by overlayGetQuery from package constants; every value is a bound placeholder
	// nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query
	row := s.db.QueryRowContext(ctx, overlayGetQuery("associations", associationSelectCols),
		id, branchID.UUID(), domain.MainBranchID.UUID())
	return scanAssociationRow(row)
}

// ListAssociations returns a paginated list of the associations visible on
// opts.BranchID (ADR-005).
func (s *ReadModelStore) ListAssociations(ctx context.Context, opts repository.ListOptions) ([]repository.AssociationReadModel, int, error) {
	args, n := overlayArgs(opts.BranchID)
	src := overlaySrc("associations", associationSelectCols, "", opts.BranchID)

	var total int
	// #nosec G202 -- src is built from package constants carrying only $-placeholders
	// nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+src+" a", args...).Scan(&total); err != nil {
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

	// #nosec G201 G202 -- orderColumn and orderDir are validated via switch/if above; src is internal
	// nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query
	query := fmt.Sprintf(`
		SELECT `+associationSelectCols+`
		FROM %s a
		ORDER BY %s %s, id ASC
		LIMIT $%d OFFSET $%d
	`, src, orderColumn, orderDir, n, n+1)

	rows, err := s.db.QueryContext(ctx, query, append(args, opts.Limit, opts.Offset)...)
	if err != nil {
		return nil, 0, fmt.Errorf("query associations: %w", err)
	}
	associations, err := scanAssociations(rows)
	if err != nil {
		return nil, 0, err
	}
	return associations, total, nil
}

// ListAssociationsForPerson returns all associations visible on branchID in
// which the person is either the subject or the associate.
func (s *ReadModelStore) ListAssociationsForPerson(ctx context.Context, branchID domain.BranchID, personID uuid.UUID) ([]repository.AssociationReadModel, error) {
	args, n := overlayArgs(branchID)
	src := overlaySrc("associations", associationSelectCols, fmt.Sprintf(personAssociationFilter, n), branchID)
	// #nosec G202 -- src is built from package constants carrying only $-placeholders
	// nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+associationSelectCols+` FROM `+src+` a
		ORDER BY role, updated_at DESC
	`, append(args, personID)...)
	if err != nil {
		return nil, fmt.Errorf("query associations for person: %w", err)
	}
	return scanAssociations(rows)
}

// SaveAssociation saves or updates an association on the given branch
// (ADR-005). A save always clears any prior tombstone.
func (s *ReadModelStore) SaveAssociation(ctx context.Context, branchID domain.BranchID, assoc *repository.AssociationReadModel) error {
	var noteIDsJSON []byte
	var err error

	if len(assoc.NoteIDs) > 0 {
		noteIDsJSON, err = json.Marshal(assoc.NoteIDs)
		if err != nil {
			return fmt.Errorf("marshal note_ids: %w", err)
		}
	}

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO associations (id, branch_id, person_id, person_name, associate_id, associate_name,
		                         role, phrase, notes, note_ids, gedcom_xref, version, updated_at, deleted)
		VALUES ($1, $2, $3, NULLIF($4, ''), $5, NULLIF($6, ''), $7, NULLIF($8, ''), NULLIF($9, ''), $10, NULLIF($11, ''), $12, $13, FALSE)
		ON CONFLICT (id, branch_id) DO UPDATE SET
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
			updated_at = EXCLUDED.updated_at,
			deleted = FALSE
	`, assoc.ID, branchID.UUID(), assoc.PersonID, assoc.PersonName, assoc.AssociateID, assoc.AssociateName,
		assoc.Role, assoc.Phrase, assoc.Notes, noteIDsJSON, assoc.GedcomXref, assoc.Version, assoc.UpdatedAt)
	if err != nil {
		return fmt.Errorf("save association: %w", err)
	}
	return nil
}

// DeleteAssociation removes an association (ADR-005): a real removal on main, a
// tombstone on a non-main branch.
func (s *ReadModelStore) DeleteAssociation(ctx context.Context, branchID domain.BranchID, id uuid.UUID) error {
	if err := deleteOverlayRow(ctx, s.db, "associations", associationSelectCols, branchID, id); err != nil {
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

// GPS artifacts (#760): evidence_analyses, evidence_conflicts, research_logs and
// proof_summaries are id-keyed branch-scoped tables with the life-event shape:
// every read goes through overlaySrc / overlayGetQuery (one set-based DISTINCT ON
// query with the main-scope fast path) and every filtered list applies its
// predicate to the winning row, so a branch-side resolution or re-pointed
// subject decides what the branch lists.
const (
	// analysisSelectCols is scanAnalysisRow's column order (unaliased).
	analysisSelectCols = `id, fact_type, subject_id, citation_ids, conclusion, research_status, notes,
		version, created_at, updated_at`

	// conflictSelectCols is scanConflictRow's column order (unaliased).
	conflictSelectCols = `id, fact_type, subject_id, analysis_ids, description, resolution, status,
		version, created_at, updated_at`

	// researchLogSelectCols is scanResearchLogRow's column order (unaliased).
	researchLogSelectCols = `id, subject_id, subject_type, repository, search_description, outcome, notes,
		search_date, version, created_at, updated_at`

	// proofSummarySelectCols is scanProofSummaryRow's column order (unaliased).
	proofSummarySelectCols = `id, fact_type, subject_id, conclusion, argument, analysis_ids, research_status,
		version, created_at, updated_at`

	// GPS artifact filters. %[1]d (and %[2]d) are the placeholder numbers the
	// caller binds after overlayArgs.
	gpsSubjectFilter        = `subject_id = $%[1]d`
	gpsFactFilter           = `fact_type = $%[1]d AND subject_id = $%[2]d`
	gpsConflictStatusFilter = `status = $%[1]d`

	// gpsSubjectOrder is the deterministic order of every per-subject and
	// per-fact GPS list, on every backend.
	gpsSubjectOrder = `created_at ASC, id ASC`
)

// gpsTable names one GPS artifact table and the column list its rows carry.
type gpsTable struct {
	name, cols string
}

// gpsTables are the four GPS artifact tables DeletePerson/DeleteFamily cascade.
var gpsTables = []gpsTable{
	{"evidence_analyses", analysisSelectCols},
	{"evidence_conflicts", conflictSelectCols},
	{"research_logs", researchLogSelectCols},
	{"proof_summaries", proofSummarySelectCols},
}

// cascadeGPS removes, on branchID, every GPS artifact whose subject is
// subjectID (#760): deleted on main, tombstoned on a branch. The tables have no
// foreign key to their subject, so DeletePerson and DeleteFamily run this.
func cascadeGPS(ctx context.Context, tx *sql.Tx, branchID domain.BranchID, subjectID uuid.UUID) error {
	for _, t := range gpsTables {
		if err := cascadeOverlayRows(ctx, tx, t.name, t.cols, gpsSubjectFilter, branchID, subjectID); err != nil {
			return err
		}
	}
	return nil
}

// gpsListOrder returns the ORDER BY of a paged GPS list. Both parts are chosen
// from constants here, never copied from the request.
func gpsListOrder(opts repository.ListOptions) string {
	dir := "DESC"
	if opts.Order == "asc" {
		dir = "ASC"
	}
	col := "updated_at"
	if opts.Sort == "created_at" {
		col = "created_at"
	}
	return col + " " + dir + ", id " + dir
}

// queryGPSPage runs the COUNT and the paged SELECT of a GPS list over
// opts.BranchID's resolved view of table, calling scan once per row. filter (a
// package constant with %[n]d placeholders, or "") is applied to each id's
// winning row, binding values in order.
func (s *ReadModelStore) queryGPSPage(ctx context.Context, table, cols, filter string, opts repository.ListOptions, scan func(*sql.Rows) error, values ...any) (int, error) {
	args, n := overlayArgs(opts.BranchID)
	if filter != "" {
		placeholders := make([]any, len(values))
		for i := range values {
			placeholders[i] = n + i
		}
		filter = fmt.Sprintf(filter, placeholders...)
		args = append(args, values...)
		n += len(values)
	}
	src := overlaySrc(table, cols, filter, opts.BranchID)

	var total int
	// #nosec G202 -- src is built from package constants carrying only $-placeholders
	// nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+src+" g", args...).Scan(&total); err != nil {
		return 0, fmt.Errorf("count %s: %w", table, err)
	}

	// #nosec G201 G202 -- src, cols and the ORDER BY are internal constants; limit/offset stay bound parameters
	// nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query
	query := fmt.Sprintf("SELECT %s FROM %s g ORDER BY %s LIMIT $%d OFFSET $%d", cols, src, gpsListOrder(opts), n, n+1)
	rows, err := s.db.QueryContext(ctx, query, append(args, opts.Limit, opts.Offset)...)
	if err != nil {
		return 0, fmt.Errorf("query %s: %w", table, err)
	}
	defer rows.Close()
	for rows.Next() {
		if err := scan(rows); err != nil {
			return 0, err
		}
	}
	return total, rows.Err()
}

// queryGPSFiltered returns branchID's resolved rows of table that filter (a
// package constant with %[n]d placeholders) selects, binding values in order,
// calling scan once per row in gpsSubjectOrder.
func (s *ReadModelStore) queryGPSFiltered(ctx context.Context, table, cols, filter string, branchID domain.BranchID, scan func(*sql.Rows) error, values ...any) error {
	args, n := overlayArgs(branchID)
	placeholders := make([]any, len(values))
	for i := range values {
		placeholders[i] = n + i
	}
	src := overlaySrc(table, cols, fmt.Sprintf(filter, placeholders...), branchID)
	// #nosec G202 -- src is built from package constants carrying only $-placeholders
	// nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query
	rows, err := s.db.QueryContext(ctx, "SELECT "+cols+" FROM "+src+" g ORDER BY "+gpsSubjectOrder, append(args, values...)...)
	if err != nil {
		return fmt.Errorf("query %s: %w", table, err)
	}
	defer rows.Close()
	for rows.Next() {
		if err := scan(rows); err != nil {
			return err
		}
	}
	return rows.Err()
}

// getGPSRow runs the single-row overlay lookup of an id-keyed GPS table.
func (s *ReadModelStore) getGPSRow(ctx context.Context, table, cols string, branchID domain.BranchID, id uuid.UUID) *sql.Row {
	// #nosec G202 -- the query is built by overlayGetQuery from package constants; every value is a bound placeholder
	// nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query
	return s.db.QueryRowContext(ctx, overlayGetQuery(table, cols), id, branchID.UUID(), domain.MainBranchID.UUID())
}

func scanAnalysisRow(row rowScanner) (*repository.EvidenceAnalysisReadModel, error) {
	var a repository.EvidenceAnalysisReadModel
	var citationIDs, researchStatus, notes sql.NullString
	err := row.Scan(&a.ID, &a.FactType, &a.SubjectID, &citationIDs, &a.Conclusion, &researchStatus, &notes,
		&a.Version, &a.CreatedAt, &a.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan evidence_analysis: %w", err)
	}
	a.CitationIDsJSON = citationIDs.String
	a.ResearchStatus = domain.ResearchStatus(researchStatus.String)
	a.Notes = notes.String
	return &a, nil
}

func scanConflictRow(row rowScanner) (*repository.EvidenceConflictReadModel, error) {
	var c repository.EvidenceConflictReadModel
	var analysisIDs, resolution sql.NullString
	err := row.Scan(&c.ID, &c.FactType, &c.SubjectID, &analysisIDs, &c.Description, &resolution, &c.Status,
		&c.Version, &c.CreatedAt, &c.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan evidence_conflict: %w", err)
	}
	c.AnalysisIDsJSON = analysisIDs.String
	c.Resolution = resolution.String
	return &c, nil
}

func scanResearchLogRow(row rowScanner) (*repository.ResearchLogReadModel, error) {
	var l repository.ResearchLogReadModel
	var notes sql.NullString
	err := row.Scan(&l.ID, &l.SubjectID, &l.SubjectType, &l.Repository, &l.SearchDescription, &l.Outcome, &notes,
		&l.SearchDate, &l.Version, &l.CreatedAt, &l.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan research_log: %w", err)
	}
	l.Notes = notes.String
	return &l, nil
}

func scanProofSummaryRow(row rowScanner) (*repository.ProofSummaryReadModel, error) {
	var ps repository.ProofSummaryReadModel
	var analysisIDs, researchStatus sql.NullString
	err := row.Scan(&ps.ID, &ps.FactType, &ps.SubjectID, &ps.Conclusion, &ps.Argument, &analysisIDs, &researchStatus,
		&ps.Version, &ps.CreatedAt, &ps.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan proof_summary: %w", err)
	}
	ps.AnalysisIDsJSON = analysisIDs.String
	ps.ResearchStatus = domain.ResearchStatus(researchStatus.String)
	return &ps, nil
}

// collectRows adapts a row scanner into a queryGPS* callback appending to out.
func collectRows[T any](out *[]T, scan func(rowScanner) (*T, error)) func(*sql.Rows) error {
	return func(rows *sql.Rows) error {
		row, err := scan(rows)
		if err != nil {
			return err
		}
		*out = append(*out, *row)
		return nil
	}
}

// GetEvidenceAnalysis retrieves an evidence analysis by ID within the branch
// overlay (ADR-005, #760).
func (s *ReadModelStore) GetEvidenceAnalysis(ctx context.Context, branchID domain.BranchID, id uuid.UUID) (*repository.EvidenceAnalysisReadModel, error) {
	return scanAnalysisRow(s.getGPSRow(ctx, "evidence_analyses", analysisSelectCols, branchID, id))
}

// ListEvidenceAnalyses returns a paginated list of the evidence analyses visible
// on opts.BranchID.
func (s *ReadModelStore) ListEvidenceAnalyses(ctx context.Context, opts repository.ListOptions) ([]repository.EvidenceAnalysisReadModel, int, error) {
	var results []repository.EvidenceAnalysisReadModel
	total, err := s.queryGPSPage(ctx, "evidence_analyses", analysisSelectCols, "", opts, collectRows(&results, scanAnalysisRow))
	if err != nil {
		return nil, 0, err
	}
	return results, total, nil
}

// GetAnalysesForFact returns the evidence analyses of a fact type and subject
// visible on branchID, matched on each id's winning row.
func (s *ReadModelStore) GetAnalysesForFact(ctx context.Context, branchID domain.BranchID, factType domain.FactType, subjectID uuid.UUID) ([]repository.EvidenceAnalysisReadModel, error) {
	var results []repository.EvidenceAnalysisReadModel
	if err := s.queryGPSFiltered(ctx, "evidence_analyses", analysisSelectCols, gpsFactFilter, branchID,
		collectRows(&results, scanAnalysisRow), string(factType), subjectID); err != nil {
		return nil, fmt.Errorf("analyses for fact: %w", err)
	}
	return results, nil
}

// GetAnalysesBySubject returns the evidence analyses of a subject visible on
// branchID, regardless of fact type.
func (s *ReadModelStore) GetAnalysesBySubject(ctx context.Context, branchID domain.BranchID, subjectID uuid.UUID) ([]repository.EvidenceAnalysisReadModel, error) {
	var results []repository.EvidenceAnalysisReadModel
	if err := s.queryGPSFiltered(ctx, "evidence_analyses", analysisSelectCols, gpsSubjectFilter, branchID,
		collectRows(&results, scanAnalysisRow), subjectID); err != nil {
		return nil, fmt.Errorf("analyses by subject: %w", err)
	}
	return results, nil
}

// SaveEvidenceAnalysis saves or updates an evidence analysis on the given
// branch (ADR-005); a save always clears any prior tombstone.
func (s *ReadModelStore) SaveEvidenceAnalysis(ctx context.Context, branchID domain.BranchID, analysis *repository.EvidenceAnalysisReadModel) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO evidence_analyses (id, branch_id, fact_type, subject_id, citation_ids, conclusion, research_status,
		                               notes, version, created_at, updated_at, deleted)
		VALUES ($1, $2, $3, $4, NULLIF($5, '')::JSONB, $6, NULLIF($7, ''), NULLIF($8, ''), $9, $10, $11, FALSE)
		ON CONFLICT (id, branch_id) DO UPDATE SET
			fact_type = EXCLUDED.fact_type,
			subject_id = EXCLUDED.subject_id,
			citation_ids = EXCLUDED.citation_ids,
			conclusion = EXCLUDED.conclusion,
			research_status = EXCLUDED.research_status,
			notes = EXCLUDED.notes,
			version = EXCLUDED.version,
			updated_at = EXCLUDED.updated_at,
			deleted = FALSE
	`, analysis.ID, branchID.UUID(), string(analysis.FactType), analysis.SubjectID, analysis.CitationIDsJSON,
		analysis.Conclusion, string(analysis.ResearchStatus), analysis.Notes,
		analysis.Version, analysis.CreatedAt, analysis.UpdatedAt)
	if err != nil {
		return fmt.Errorf("save evidence_analysis: %w", err)
	}
	return nil
}

// DeleteEvidenceAnalysis removes an evidence analysis (ADR-005): a real removal
// on main, a tombstone on a non-main branch.
func (s *ReadModelStore) DeleteEvidenceAnalysis(ctx context.Context, branchID domain.BranchID, id uuid.UUID) error {
	if err := deleteOverlayRow(ctx, s.db, "evidence_analyses", analysisSelectCols, branchID, id); err != nil {
		return fmt.Errorf("delete evidence_analysis: %w", err)
	}
	return nil
}

// GetEvidenceConflict retrieves an evidence conflict by ID within the branch
// overlay (ADR-005, #760).
func (s *ReadModelStore) GetEvidenceConflict(ctx context.Context, branchID domain.BranchID, id uuid.UUID) (*repository.EvidenceConflictReadModel, error) {
	return scanConflictRow(s.getGPSRow(ctx, "evidence_conflicts", conflictSelectCols, branchID, id))
}

// ListEvidenceConflicts returns a paginated list of the evidence conflicts
// visible on opts.BranchID.
func (s *ReadModelStore) ListEvidenceConflicts(ctx context.Context, opts repository.ListOptions) ([]repository.EvidenceConflictReadModel, int, error) {
	var results []repository.EvidenceConflictReadModel
	filter, values := "", []any(nil)
	if opts.ConflictStatus != nil {
		filter, values = gpsConflictStatusFilter, []any{string(*opts.ConflictStatus)}
	}
	total, err := s.queryGPSPage(ctx, "evidence_conflicts", conflictSelectCols, filter, opts,
		collectRows(&results, scanConflictRow), values...)
	if err != nil {
		return nil, 0, err
	}
	return results, total, nil
}

// GetConflictsForSubject returns the evidence conflicts of a subject visible on
// branchID.
func (s *ReadModelStore) GetConflictsForSubject(ctx context.Context, branchID domain.BranchID, subjectID uuid.UUID) ([]repository.EvidenceConflictReadModel, error) {
	var results []repository.EvidenceConflictReadModel
	if err := s.queryGPSFiltered(ctx, "evidence_conflicts", conflictSelectCols, gpsSubjectFilter, branchID,
		collectRows(&results, scanConflictRow), subjectID); err != nil {
		return nil, fmt.Errorf("conflicts for subject: %w", err)
	}
	return results, nil
}

// ListUnresolvedConflicts returns the open evidence conflicts visible on
// branchID. overlaySrc resolves each conflict first and applies the status
// predicate to the winning row, so a conflict a branch resolved is not listed on
// that branch even though main's row for it is still open.
func (s *ReadModelStore) ListUnresolvedConflicts(ctx context.Context, branchID domain.BranchID) ([]repository.EvidenceConflictReadModel, error) {
	var results []repository.EvidenceConflictReadModel
	if err := s.queryGPSFiltered(ctx, "evidence_conflicts", conflictSelectCols, gpsConflictStatusFilter, branchID,
		collectRows(&results, scanConflictRow), string(domain.ConflictStatusOpen)); err != nil {
		return nil, fmt.Errorf("unresolved conflicts: %w", err)
	}
	return results, nil
}

// SaveEvidenceConflict saves or updates an evidence conflict on the given branch
// (ADR-005); a save always clears any prior tombstone.
func (s *ReadModelStore) SaveEvidenceConflict(ctx context.Context, branchID domain.BranchID, conflict *repository.EvidenceConflictReadModel) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO evidence_conflicts (id, branch_id, fact_type, subject_id, analysis_ids, description, resolution,
		                                status, version, created_at, updated_at, deleted)
		VALUES ($1, $2, $3, $4, NULLIF($5, '')::JSONB, $6, NULLIF($7, ''), $8, $9, $10, $11, FALSE)
		ON CONFLICT (id, branch_id) DO UPDATE SET
			fact_type = EXCLUDED.fact_type,
			subject_id = EXCLUDED.subject_id,
			analysis_ids = EXCLUDED.analysis_ids,
			description = EXCLUDED.description,
			resolution = EXCLUDED.resolution,
			status = EXCLUDED.status,
			version = EXCLUDED.version,
			updated_at = EXCLUDED.updated_at,
			deleted = FALSE
	`, conflict.ID, branchID.UUID(), string(conflict.FactType), conflict.SubjectID, conflict.AnalysisIDsJSON,
		conflict.Description, conflict.Resolution, string(conflict.Status),
		conflict.Version, conflict.CreatedAt, conflict.UpdatedAt)
	if err != nil {
		return fmt.Errorf("save evidence_conflict: %w", err)
	}
	return nil
}

// DeleteEvidenceConflict removes an evidence conflict (ADR-005): a real removal
// on main, a tombstone on a non-main branch.
func (s *ReadModelStore) DeleteEvidenceConflict(ctx context.Context, branchID domain.BranchID, id uuid.UUID) error {
	if err := deleteOverlayRow(ctx, s.db, "evidence_conflicts", conflictSelectCols, branchID, id); err != nil {
		return fmt.Errorf("delete evidence_conflict: %w", err)
	}
	return nil
}

// GetResearchLog retrieves a research log by ID within the branch overlay
// (ADR-005, #760).
func (s *ReadModelStore) GetResearchLog(ctx context.Context, branchID domain.BranchID, id uuid.UUID) (*repository.ResearchLogReadModel, error) {
	return scanResearchLogRow(s.getGPSRow(ctx, "research_logs", researchLogSelectCols, branchID, id))
}

// ListResearchLogs returns a paginated list of the research logs visible on
// opts.BranchID.
func (s *ReadModelStore) ListResearchLogs(ctx context.Context, opts repository.ListOptions) ([]repository.ResearchLogReadModel, int, error) {
	var results []repository.ResearchLogReadModel
	total, err := s.queryGPSPage(ctx, "research_logs", researchLogSelectCols, "", opts, collectRows(&results, scanResearchLogRow))
	if err != nil {
		return nil, 0, err
	}
	return results, total, nil
}

// GetResearchLogsForSubject returns the research logs of a subject visible on
// branchID.
func (s *ReadModelStore) GetResearchLogsForSubject(ctx context.Context, branchID domain.BranchID, subjectID uuid.UUID) ([]repository.ResearchLogReadModel, error) {
	var results []repository.ResearchLogReadModel
	if err := s.queryGPSFiltered(ctx, "research_logs", researchLogSelectCols, gpsSubjectFilter, branchID,
		collectRows(&results, scanResearchLogRow), subjectID); err != nil {
		return nil, fmt.Errorf("research logs for subject: %w", err)
	}
	return results, nil
}

// SaveResearchLog saves or updates a research log on the given branch
// (ADR-005); a save always clears any prior tombstone.
func (s *ReadModelStore) SaveResearchLog(ctx context.Context, branchID domain.BranchID, log *repository.ResearchLogReadModel) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO research_logs (id, branch_id, subject_id, subject_type, repository, search_description, outcome,
		                           notes, search_date, version, created_at, updated_at, deleted)
		VALUES ($1, $2, $3, $4, $5, $6, $7, NULLIF($8, ''), $9, $10, $11, $12, FALSE)
		ON CONFLICT (id, branch_id) DO UPDATE SET
			subject_id = EXCLUDED.subject_id,
			subject_type = EXCLUDED.subject_type,
			repository = EXCLUDED.repository,
			search_description = EXCLUDED.search_description,
			outcome = EXCLUDED.outcome,
			notes = EXCLUDED.notes,
			search_date = EXCLUDED.search_date,
			version = EXCLUDED.version,
			updated_at = EXCLUDED.updated_at,
			deleted = FALSE
	`, log.ID, branchID.UUID(), log.SubjectID, log.SubjectType, log.Repository, log.SearchDescription,
		string(log.Outcome), log.Notes, log.SearchDate, log.Version, log.CreatedAt, log.UpdatedAt)
	if err != nil {
		return fmt.Errorf("save research_log: %w", err)
	}
	return nil
}

// DeleteResearchLog removes a research log (ADR-005): a real removal on main, a
// tombstone on a non-main branch.
func (s *ReadModelStore) DeleteResearchLog(ctx context.Context, branchID domain.BranchID, id uuid.UUID) error {
	if err := deleteOverlayRow(ctx, s.db, "research_logs", researchLogSelectCols, branchID, id); err != nil {
		return fmt.Errorf("delete research_log: %w", err)
	}
	return nil
}

// GetProofSummary retrieves a proof summary by ID within the branch overlay
// (ADR-005, #760).
func (s *ReadModelStore) GetProofSummary(ctx context.Context, branchID domain.BranchID, id uuid.UUID) (*repository.ProofSummaryReadModel, error) {
	return scanProofSummaryRow(s.getGPSRow(ctx, "proof_summaries", proofSummarySelectCols, branchID, id))
}

// ListProofSummaries returns a paginated list of the proof summaries visible on
// opts.BranchID.
func (s *ReadModelStore) ListProofSummaries(ctx context.Context, opts repository.ListOptions) ([]repository.ProofSummaryReadModel, int, error) {
	var results []repository.ProofSummaryReadModel
	total, err := s.queryGPSPage(ctx, "proof_summaries", proofSummarySelectCols, "", opts, collectRows(&results, scanProofSummaryRow))
	if err != nil {
		return nil, 0, err
	}
	return results, total, nil
}

// GetProofSummariesForFact returns the proof summaries of a fact type and
// subject visible on branchID.
func (s *ReadModelStore) GetProofSummariesForFact(ctx context.Context, branchID domain.BranchID, factType domain.FactType, subjectID uuid.UUID) ([]repository.ProofSummaryReadModel, error) {
	var results []repository.ProofSummaryReadModel
	if err := s.queryGPSFiltered(ctx, "proof_summaries", proofSummarySelectCols, gpsFactFilter, branchID,
		collectRows(&results, scanProofSummaryRow), string(factType), subjectID); err != nil {
		return nil, fmt.Errorf("proof summaries for fact: %w", err)
	}
	return results, nil
}

// GetProofSummariesBySubject returns the proof summaries of a subject visible on
// branchID, regardless of fact type.
func (s *ReadModelStore) GetProofSummariesBySubject(ctx context.Context, branchID domain.BranchID, subjectID uuid.UUID) ([]repository.ProofSummaryReadModel, error) {
	var results []repository.ProofSummaryReadModel
	if err := s.queryGPSFiltered(ctx, "proof_summaries", proofSummarySelectCols, gpsSubjectFilter, branchID,
		collectRows(&results, scanProofSummaryRow), subjectID); err != nil {
		return nil, fmt.Errorf("proof summaries by subject: %w", err)
	}
	return results, nil
}

// SaveProofSummary saves or updates a proof summary on the given branch
// (ADR-005); a save always clears any prior tombstone.
func (s *ReadModelStore) SaveProofSummary(ctx context.Context, branchID domain.BranchID, summary *repository.ProofSummaryReadModel) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO proof_summaries (id, branch_id, fact_type, subject_id, conclusion, argument, analysis_ids,
		                             research_status, version, created_at, updated_at, deleted)
		VALUES ($1, $2, $3, $4, $5, $6, NULLIF($7, '')::JSONB, NULLIF($8, ''), $9, $10, $11, FALSE)
		ON CONFLICT (id, branch_id) DO UPDATE SET
			fact_type = EXCLUDED.fact_type,
			subject_id = EXCLUDED.subject_id,
			conclusion = EXCLUDED.conclusion,
			argument = EXCLUDED.argument,
			analysis_ids = EXCLUDED.analysis_ids,
			research_status = EXCLUDED.research_status,
			version = EXCLUDED.version,
			updated_at = EXCLUDED.updated_at,
			deleted = FALSE
	`, summary.ID, branchID.UUID(), string(summary.FactType), summary.SubjectID, summary.Conclusion,
		summary.Argument, summary.AnalysisIDsJSON, string(summary.ResearchStatus),
		summary.Version, summary.CreatedAt, summary.UpdatedAt)
	if err != nil {
		return fmt.Errorf("save proof_summary: %w", err)
	}
	return nil
}

// DeleteProofSummary removes a proof summary (ADR-005): a real removal on main,
// a tombstone on a non-main branch.
func (s *ReadModelStore) DeleteProofSummary(ctx context.Context, branchID domain.BranchID, id uuid.UUID) error {
	if err := deleteOverlayRow(ctx, s.db, "proof_summaries", proofSummarySelectCols, branchID, id); err != nil {
		return fmt.Errorf("delete proof_summary: %w", err)
	}
	return nil
}
