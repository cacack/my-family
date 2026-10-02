package sqlite

import (
	"bytes"
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

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// ReadModelStore is a SQLite implementation of repository.ReadModelStore.
type ReadModelStore struct {
	db *sql.DB
	// branchCapable reports whether the branch-scoped tables carry the composite
	// (id, branch_id) PRIMARY KEY that a branch's copy-on-write shadow row needs.
	// A freshly created schema always does; a database created before #669 (or,
	// for the person/family fact tables, before #757, for the evidence tables,
	// before #758, for media, before #759, or for the GPS artifacts, before #760)
	// keeps its pre-branch key
	// (SQLite cannot alter a PK in place), so branch writes
	// are refused with repository.ErrBranchesUnsupported rather than failing later
	// on an opaque constraint violation. See detectBranchCapable and issue #680.
	branchCapable bool
}

// mainBranchID is the string form of domain.MainBranchID (the all-zeros UUID),
// used as the default branch_id for mainline rows and as the fallback branch in
// the overlay resolution (ADR-005 / #669).
var mainBranchID = domain.MainBranchID.String()

// ErrConflictingEventsTables is returned by NewReadModelStore when a database holds
// BOTH a pre-#733 read-model `events` table (one carrying owner_type) and a
// `life_events` table. Which of the two holds the live life facts cannot be decided
// from the schema, so the store refuses to open rather than guess: renaming would
// fail, and silently preferring either one risks serving an empty or stale table
// while the real rows sit orphaned.
var ErrConflictingEventsTables = errors.New(
	`this database holds both a pre-#733 read-model "events" table and a "life_events" table: ` +
		`the migration state is ambiguous — either could hold the live life facts, so the read model ` +
		`refuses to open. Inspect both tables manually, keep the one with the current rows as ` +
		`"life_events", and drop or archive the other (see issue #733)`)

// NewReadModelStore creates a new SQLite read model store.
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
	store.branchCapable = store.detectBranchCapable()
	if !store.branchCapable {
		slog.Warn("sqlite read model predates branch support (single-column primary key): " +
			"mainline reads and writes work, branch writes are refused; " +
			"rebuild the read model from the event store to enable branches (see issue #680)")
	}
	return store, nil
}

// detectBranchCapable reports whether every table in branchKeyedTables carries
// branch_id in its PRIMARY KEY. persons stands for the seven #669 slice tables,
// which all gained their composite key in the same schema revision; the
// person/family fact tables gained theirs later (#757), the evidence tables
// later still (#758), then media (#759) and last the GPS artifact tables (#760),
// so a database built between two revisions has some
// branch-capable tables and some single-key ones.
// Such a database refuses EVERY branch write, not just the newer tables': a branch
// DeletePerson must tombstone the person's facts too, and letting half the
// branch model write would leave branches that cannot be deleted cleanly. On any
// error it returns false (fail closed — refusing a branch write is safer than
// attempting one the schema can't hold).
func (s *ReadModelStore) detectBranchCapable() bool {
	for _, table := range branchKeyedTables {
		if !s.hasBranchKey(table) {
			return false
		}
	}
	return true
}

// hasBranchKey reports whether branch_id is part of table's PRIMARY KEY. PRAGMA
// table_info marks each primary-key column with a non-zero `pk` ordinal. Checking
// for branch_id itself (rather than for a multi-column key) matters for tables
// such as source_external_ids whose pre-branch key was already composite
// (source_id, sequence). table must be a package constant.
func (s *ReadModelStore) hasBranchKey(table string) bool {
	// #nosec G202 -- table comes from the fixed branchKeyedTables list, not user input
	// nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query
	rows, err := s.db.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		return false
	}
	defer func() { _ = rows.Close() }()

	branchInKey := false
	for rows.Next() {
		var cid int
		var name, colType string
		var notNull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &colType, &notNull, &dflt, &pk); err != nil {
			return false
		}
		if pk > 0 && name == "branch_id" {
			branchInKey = true
		}
	}
	if rows.Err() != nil {
		return false
	}
	return branchInKey
}

// guardBranchWrite refuses a write scoped to a non-main branch when the schema
// cannot represent branch rows. Mainline writes are always allowed.
func (s *ReadModelStore) guardBranchWrite(branchID domain.BranchID) error {
	if branchID.IsMain() || s.branchCapable {
		return nil
	}
	return repository.ErrBranchesUnsupported
}

// createTables creates the read model schema if it doesn't exist.
func (s *ReadModelStore) createTables() error {
	// Rename a PRE-EXISTING read-model `events` table to `life_events` FIRST, before
	// the schema block below creates an empty `life_events` — after that the rename
	// would fail and strand the legacy rows in an orphaned table (#733).
	//
	// Its error is FATAL and returned immediately: unlike the ADD COLUMN migrations in
	// this file, this is a DESTRUCTIVE rename, so it must not follow runMigrations'
	// best-effort swallow idiom. See renameLegacyEventsTable for why.
	if err := s.renameLegacyEventsTable(); err != nil {
		return err
	}

	// Add the #669 branch columns to any PRE-EXISTING slice table FIRST. The schema
	// block below creates indexes on branch_id in the same batch as the tables, so on
	// an upgraded database (tables already exist without branch_id) those CREATE INDEX
	// statements would fail with "no such column: branch_id" and abort startup
	// entirely. Running the ALTERs up front makes the batch valid for both a fresh
	// database (the ALTERs no-op because the tables don't exist yet, then CREATE TABLE
	// defines branch_id inline) and an upgraded one.
	s.migrateBranchColumns()

	// Create core tables
	_, err := s.db.Exec(`
		-- Persons table
		CREATE TABLE IF NOT EXISTS persons (
			id TEXT NOT NULL,
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
			updated_at TEXT NOT NULL DEFAULT (datetime('now')),
			-- Branch scoping (ADR-005 / #669): (id, branch_id) is the row identity so
			-- a branch can hold a copy-on-write shadow row (or a deleted=1 tombstone)
			-- layered over the mainline row. branch_id defaults to MainBranchID (all-zeros).
			branch_id TEXT NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
			deleted INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY (id, branch_id)
		);

		-- Index leading with branch_id for PurgeBranch's DELETE ... WHERE branch_id = ?
		-- and the overlay's branch_id IN filter; the composite PK leads with id, so
		-- branch_id alone is otherwise unindexed (#669). No (id, branch_id) index is
		-- defined: the PK autoindex already serves that prefix.
		CREATE INDEX IF NOT EXISTS idx_persons_branch ON persons(branch_id);
		CREATE INDEX IF NOT EXISTS idx_persons_surname ON persons(surname, given_name);
		CREATE INDEX IF NOT EXISTS idx_persons_birth_date ON persons(birth_date_sort);
		CREATE INDEX IF NOT EXISTS idx_persons_full_name ON persons(full_name);
		CREATE INDEX IF NOT EXISTS idx_persons_research_status ON persons(research_status);

		-- Families table
		CREATE TABLE IF NOT EXISTS families (
			id TEXT NOT NULL,
			partner1_id TEXT,
			partner1_given_name TEXT,
			partner1_surname TEXT,
			partner2_id TEXT,
			partner2_given_name TEXT,
			partner2_surname TEXT,
			relationship_type TEXT,
			marriage_date_raw TEXT,
			marriage_date_sort TEXT,
			marriage_place TEXT,
			child_count INTEGER NOT NULL DEFAULT 0,
			version INTEGER NOT NULL DEFAULT 1,
			updated_at TEXT NOT NULL DEFAULT (datetime('now')),
			-- Branch scoping (ADR-005 / #669): identity is (id, branch_id). The partner
			-- foreign keys to persons(id) are dropped because persons is now keyed by
			-- (id, branch_id); read-model referential integrity is guaranteed write-side.
			branch_id TEXT NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
			deleted INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY (id, branch_id)
		);

		CREATE INDEX IF NOT EXISTS idx_families_branch ON families(branch_id);
		CREATE INDEX IF NOT EXISTS idx_families_partner1 ON families(partner1_id);
		CREATE INDEX IF NOT EXISTS idx_families_partner2 ON families(partner2_id);

		-- Family children table
		CREATE TABLE IF NOT EXISTS family_children (
			family_id TEXT NOT NULL,
			person_id TEXT NOT NULL,
			person_given_name TEXT,
			person_surname TEXT,
			relationship_type TEXT NOT NULL DEFAULT 'biological',
			sequence INTEGER,
			-- Branch scoping (ADR-005 / #669): identity is (family_id, person_id, branch_id).
			-- FKs dropped (parents keyed by (id, branch_id)); cascade is done in code.
			branch_id TEXT NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
			deleted INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY (family_id, person_id, branch_id)
		);

		CREATE INDEX IF NOT EXISTS idx_family_children_branch ON family_children(branch_id);
		CREATE INDEX IF NOT EXISTS idx_family_children_person ON family_children(person_id);

		-- Pedigree edges table
		CREATE TABLE IF NOT EXISTS pedigree_edges (
			person_id TEXT NOT NULL,
			father_id TEXT,
			mother_id TEXT,
			father_name TEXT,
			mother_name TEXT,
			-- Branch scoping (ADR-005 / #669): identity is (person_id, branch_id).
			-- FKs to persons(id) dropped (persons keyed by (id, branch_id)).
			branch_id TEXT NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
			deleted INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY (person_id, branch_id)
		);

		CREATE INDEX IF NOT EXISTS idx_pedigree_father ON pedigree_edges(father_id);
		CREATE INDEX IF NOT EXISTS idx_pedigree_mother ON pedigree_edges(mother_id);
		CREATE INDEX IF NOT EXISTS idx_pedigree_edges_branch ON pedigree_edges(branch_id);

		-- Sources table. Branch-aware (#758): (id, branch_id) row identity + deleted
		-- tombstone.
		CREATE TABLE IF NOT EXISTS sources (
			id TEXT NOT NULL,
			branch_id TEXT NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
			source_type TEXT NOT NULL,
			title TEXT NOT NULL,
			author TEXT,
			publisher TEXT,
			publish_date_raw TEXT,
			publish_date_sort TEXT,
			url TEXT,
			repository_id TEXT,
			repository_name TEXT,
			collection_name TEXT,
			call_number TEXT,
			notes TEXT,
			gedcom_xref TEXT,
			citation_count INTEGER NOT NULL DEFAULT 0,
			version INTEGER NOT NULL DEFAULT 1,
			updated_at TEXT NOT NULL DEFAULT (datetime('now')),
			deleted INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY (id, branch_id)
		);

		CREATE INDEX IF NOT EXISTS idx_sources_title ON sources(title);
		CREATE INDEX IF NOT EXISTS idx_sources_type ON sources(source_type);

		-- Citations table. Branch-aware (#758): (id, branch_id) row identity + deleted
		-- tombstone.
		CREATE TABLE IF NOT EXISTS citations (
			id TEXT NOT NULL,
			branch_id TEXT NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
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
			fields_data TEXT,
			gedcom_xref TEXT,
			version INTEGER NOT NULL DEFAULT 1,
			created_at TEXT NOT NULL DEFAULT (datetime('now')),
			deleted INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY (id, branch_id)
			-- FK to sources(id) dropped: sources is keyed by (id, branch_id) for branch
			-- scoping (#758). DeleteSource cascades citations in code.
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
			id TEXT NOT NULL,
			branch_id TEXT NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
			entity_type TEXT NOT NULL,
			entity_id TEXT NOT NULL,
			title TEXT NOT NULL,
			description TEXT,
			mime_type TEXT NOT NULL,
			media_type TEXT NOT NULL,
			filename TEXT NOT NULL,
			file_size INTEGER NOT NULL,
			file_data BLOB,
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
			translations TEXT, -- JSON array of translated titles
			deleted INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY (id, branch_id)
		);

		CREATE INDEX IF NOT EXISTS idx_media_entity ON media(entity_type, entity_id);
		CREATE INDEX IF NOT EXISTS idx_media_type ON media(media_type);

		-- Person names table (for multiple name variants)
		CREATE TABLE IF NOT EXISTS person_names (
			id TEXT NOT NULL,
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
			-- Branch scoping (ADR-005 / #669): identity is (id, branch_id); FK to
			-- persons(id) dropped (persons keyed by (id, branch_id)).
			branch_id TEXT NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
			deleted INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY (id, branch_id)
		);

		CREATE INDEX IF NOT EXISTS idx_person_names_person ON person_names(person_id, branch_id);
		CREATE INDEX IF NOT EXISTS idx_person_names_primary ON person_names(person_id, is_primary);
		CREATE INDEX IF NOT EXISTS idx_person_names_branch ON person_names(branch_id);

		-- Person external identifiers (GEDCOM 7.0 EXID)
		CREATE TABLE IF NOT EXISTS person_external_ids (
			person_id TEXT NOT NULL,
			sequence INTEGER NOT NULL,
			value TEXT NOT NULL,
			type TEXT NOT NULL DEFAULT '',
			-- Branch scoping (ADR-005 / #669): identity is (person_id, sequence, branch_id).
			-- EXIDs resolve per-parent bucket; a single deleted=1 marker row represents an
			-- empty branch bucket (tombstone hiding main's identifiers). FK dropped.
			branch_id TEXT NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
			deleted INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY (person_id, sequence, branch_id)
		);

		CREATE INDEX IF NOT EXISTS idx_person_external_ids_person ON person_external_ids(person_id, branch_id);
		CREATE INDEX IF NOT EXISTS idx_person_external_ids_branch ON person_external_ids(branch_id);

		-- Notes table (shared GEDCOM NOTE records). Branch-aware (#758): (id,
		-- branch_id) row identity + deleted tombstone.
		CREATE TABLE IF NOT EXISTS notes (
			id TEXT NOT NULL,
			branch_id TEXT NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
			text TEXT NOT NULL,
			mime TEXT,
			language TEXT,
			translations TEXT,
			gedcom_xref TEXT,
			version INTEGER NOT NULL DEFAULT 1,
			updated_at TEXT NOT NULL DEFAULT (datetime('now')),
			deleted INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY (id, branch_id)
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

		-- Repositories table (GEDCOM REPO records for source document locations)
		CREATE TABLE IF NOT EXISTS repositories (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			address TEXT,
			notes TEXT,
			gedcom_xref TEXT,
			version INTEGER NOT NULL DEFAULT 1,
			updated_at TEXT NOT NULL DEFAULT (datetime('now'))
		);

		CREATE INDEX IF NOT EXISTS idx_repositories_gedcom_xref ON repositories(gedcom_xref);

		-- Family external identifiers (GEDCOM 7.0 EXID)
		CREATE TABLE IF NOT EXISTS family_external_ids (
			family_id TEXT NOT NULL,
			sequence INTEGER NOT NULL,
			value TEXT NOT NULL,
			type TEXT NOT NULL DEFAULT '',
			-- Branch scoping (ADR-005 / #669): identity is (family_id, sequence, branch_id).
			-- Same per-parent bucket + empty-marker tombstone model as person_external_ids.
			branch_id TEXT NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
			deleted INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY (family_id, sequence, branch_id)
		);

		CREATE INDEX IF NOT EXISTS idx_family_external_ids_family ON family_external_ids(family_id, branch_id);
		CREATE INDEX IF NOT EXISTS idx_family_external_ids_branch ON family_external_ids(branch_id);

		-- Source external identifiers (GEDCOM 7.0 EXID)
		CREATE TABLE IF NOT EXISTS source_external_ids (
			source_id TEXT NOT NULL,
			sequence INTEGER NOT NULL,
			value TEXT NOT NULL,
			type TEXT NOT NULL DEFAULT '',
			-- Branch scoping (#758): identity is (source_id, sequence, branch_id), the
			-- same per-parent bucket + empty-marker tombstone model as
			-- person_external_ids. The FK to sources(id) is dropped (sources is keyed
			-- by (id, branch_id)); DeleteSource cascades the bucket in code.
			branch_id TEXT NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
			deleted INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY (source_id, sequence, branch_id)
		);

		CREATE INDEX IF NOT EXISTS idx_source_external_ids_source ON source_external_ids(source_id, branch_id);

		-- Repository external identifiers (GEDCOM 7.0 EXID)
		CREATE TABLE IF NOT EXISTS repository_external_ids (
			repository_id TEXT NOT NULL,
			sequence INTEGER NOT NULL,
			value TEXT NOT NULL,
			type TEXT NOT NULL DEFAULT '',
			PRIMARY KEY (repository_id, sequence),
			FOREIGN KEY (repository_id) REFERENCES repositories(id) ON DELETE CASCADE
		);

		CREATE INDEX IF NOT EXISTS idx_repository_external_ids_repository ON repository_external_ids(repository_id);

		-- Associations table (GEDCOM ASSO records for non-family relationships)
		-- Branch-aware (#757): (id, branch_id) row identity + deleted tombstone.
		CREATE TABLE IF NOT EXISTS associations (
			id TEXT NOT NULL,
			branch_id TEXT NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
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
			deleted INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY (id, branch_id)
			-- FKs to persons(id) dropped: persons is now keyed by (id, branch_id) for
			-- branch scoping (#669), which is incompatible with a lone-id foreign key.
			-- DeletePerson cascades associations in code to preserve mainline behavior.
		);

		CREATE INDEX IF NOT EXISTS idx_associations_person ON associations(person_id);
		CREATE INDEX IF NOT EXISTS idx_associations_associate ON associations(associate_id);
		CREATE INDEX IF NOT EXISTS idx_associations_role ON associations(role);

		-- Life events table (life events for persons and families). Named life_events
		-- rather than events so the read model can share one database with the event
		-- log, which owns the events table (#733). Branch-aware (#757): (id,
		-- branch_id) row identity + deleted tombstone.
		CREATE TABLE IF NOT EXISTS life_events (
			id TEXT NOT NULL,
			branch_id TEXT NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
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
			is_negated INTEGER NOT NULL DEFAULT 0,
			version INTEGER NOT NULL DEFAULT 1,
			created_at TEXT NOT NULL DEFAULT (datetime('now')),
			deleted INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY (id, branch_id)
		);

		CREATE INDEX IF NOT EXISTS idx_life_events_owner ON life_events(owner_type, owner_id);
		CREATE INDEX IF NOT EXISTS idx_life_events_fact_type ON life_events(fact_type);

		-- Attributes table (person attributes). Branch-aware (#757): (id, branch_id)
		-- row identity + deleted tombstone.
		CREATE TABLE IF NOT EXISTS attributes (
			id TEXT NOT NULL,
			branch_id TEXT NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
			person_id TEXT NOT NULL,
			fact_type TEXT NOT NULL,
			value TEXT NOT NULL DEFAULT '',
			date_raw TEXT,
			date_sort TEXT,
			place TEXT,
			version INTEGER NOT NULL DEFAULT 1,
			created_at TEXT NOT NULL DEFAULT (datetime('now')),
			deleted INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY (id, branch_id)
			-- FK to persons(id) dropped: persons is keyed by (id, branch_id) for branch
			-- scoping (#669). Integrity is enforced write-side; the read model is rebuildable.
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

		-- Evidence analyses table
		-- Branch-aware (#760): (id, branch_id) row identity + deleted tombstone, like
		-- every GPS artifact table below. subject_id has no foreign key; DeletePerson
		-- and DeleteFamily cascade in code.
		CREATE TABLE IF NOT EXISTS evidence_analyses (
			id TEXT NOT NULL,
			branch_id TEXT NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
			fact_type TEXT NOT NULL,
			subject_id TEXT NOT NULL,
			citation_ids TEXT,
			conclusion TEXT NOT NULL,
			research_status TEXT,
			notes TEXT,
			version INTEGER NOT NULL DEFAULT 1,
			created_at TEXT NOT NULL DEFAULT (datetime('now')),
			updated_at TEXT NOT NULL DEFAULT (datetime('now')),
			deleted INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY (id, branch_id)
		);

		CREATE INDEX IF NOT EXISTS idx_evidence_analyses_subject ON evidence_analyses(subject_id);
		CREATE INDEX IF NOT EXISTS idx_evidence_analyses_fact_type ON evidence_analyses(fact_type);

		-- Evidence conflicts table (branch-aware, #760)
		CREATE TABLE IF NOT EXISTS evidence_conflicts (
			id TEXT NOT NULL,
			branch_id TEXT NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
			fact_type TEXT NOT NULL,
			subject_id TEXT NOT NULL,
			analysis_ids TEXT,
			description TEXT NOT NULL,
			resolution TEXT,
			status TEXT NOT NULL,
			version INTEGER NOT NULL DEFAULT 1,
			created_at TEXT NOT NULL DEFAULT (datetime('now')),
			updated_at TEXT NOT NULL DEFAULT (datetime('now')),
			deleted INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY (id, branch_id)
		);

		CREATE INDEX IF NOT EXISTS idx_evidence_conflicts_subject ON evidence_conflicts(subject_id);
		CREATE INDEX IF NOT EXISTS idx_evidence_conflicts_status ON evidence_conflicts(status);

		-- Research logs table (branch-aware, #760)
		CREATE TABLE IF NOT EXISTS research_logs (
			id TEXT NOT NULL,
			branch_id TEXT NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
			subject_id TEXT NOT NULL,
			subject_type TEXT NOT NULL,
			repository TEXT NOT NULL,
			search_description TEXT NOT NULL,
			outcome TEXT NOT NULL,
			notes TEXT,
			search_date TEXT NOT NULL,
			version INTEGER NOT NULL DEFAULT 1,
			created_at TEXT NOT NULL DEFAULT (datetime('now')),
			updated_at TEXT NOT NULL DEFAULT (datetime('now')),
			deleted INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY (id, branch_id)
		);

		CREATE INDEX IF NOT EXISTS idx_research_logs_subject ON research_logs(subject_id);
		CREATE INDEX IF NOT EXISTS idx_research_logs_outcome ON research_logs(outcome);

		-- Proof summaries table (branch-aware, #760)
		CREATE TABLE IF NOT EXISTS proof_summaries (
			id TEXT NOT NULL,
			branch_id TEXT NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
			fact_type TEXT NOT NULL,
			subject_id TEXT NOT NULL,
			conclusion TEXT NOT NULL,
			argument TEXT NOT NULL,
			analysis_ids TEXT,
			research_status TEXT,
			version INTEGER NOT NULL DEFAULT 1,
			created_at TEXT NOT NULL DEFAULT (datetime('now')),
			updated_at TEXT NOT NULL DEFAULT (datetime('now')),
			deleted INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY (id, branch_id)
		);

		CREATE INDEX IF NOT EXISTS idx_proof_summaries_subject ON proof_summaries(subject_id);
		CREATE INDEX IF NOT EXISTS idx_proof_summaries_fact_type ON proof_summaries(fact_type);
	`)
	if err != nil {
		return err
	}

	if err := s.dropLegacyFTS5(); err != nil {
		return err
	}

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

	// Add brick wall columns for research tracking (issue #61)
	_, _ = s.db.Exec(`ALTER TABLE persons ADD COLUMN brick_wall_note TEXT DEFAULT ''`)
	_, _ = s.db.Exec(`ALTER TABLE persons ADD COLUMN brick_wall_since TEXT`)
	_, _ = s.db.Exec(`ALTER TABLE persons ADD COLUMN brick_wall_resolved_at TEXT`)
	_, _ = s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_persons_brick_wall ON persons(brick_wall_since)`)

	// Add is_negated column for negative assertions / NO tags (issue #222).
	// Targets life_events, not events: the latter is the event log's table (#733).
	_, _ = s.db.Exec(`ALTER TABLE life_events ADD COLUMN is_negated INTEGER NOT NULL DEFAULT 0`)

	// Add GEDCOM 7.0 shared-note (SNOTE) metadata columns (issue #225).
	// SQLite ALTER TABLE ADD COLUMN has no IF NOT EXISTS; rely on the _,_ = swallowing.
	_, _ = s.db.Exec(`ALTER TABLE notes ADD COLUMN mime TEXT`)
	_, _ = s.db.Exec(`ALTER TABLE notes ADD COLUMN language TEXT`)
	_, _ = s.db.Exec(`ALTER TABLE notes ADD COLUMN translations TEXT`)

	// Split family partner names and family-child names into given_name / surname (issue #483).
	// SQLite ALTER TABLE ADD COLUMN has no IF NOT EXISTS; rely on the existing _,_ = swallowing.
	// Legacy columns (partner1_name/partner2_name/person_name) are intentionally NOT dropped
	// on SQLite — project convention is to leave dead columns rather than rebuild tables.
	_, _ = s.db.Exec(`ALTER TABLE families ADD COLUMN partner1_given_name TEXT`)
	_, _ = s.db.Exec(`ALTER TABLE families ADD COLUMN partner1_surname TEXT`)
	_, _ = s.db.Exec(`ALTER TABLE families ADD COLUMN partner2_given_name TEXT`)
	_, _ = s.db.Exec(`ALTER TABLE families ADD COLUMN partner2_surname TEXT`)
	_, _ = s.db.Exec(`ALTER TABLE family_children ADD COLUMN person_given_name TEXT`)
	_, _ = s.db.Exec(`ALTER TABLE family_children ADD COLUMN person_surname TEXT`)

	// Backfill split fields from persons; idempotent via IS NULL guard.
	// COALESCE the subquery to '' so an orphan partner_id (no matching persons row)
	// converges to a written empty string rather than NULL, which would otherwise
	// re-trigger the backfill on every startup.
	_, _ = s.db.Exec(`
		UPDATE families SET
			partner1_given_name = COALESCE((SELECT given_name FROM persons WHERE id = partner1_id), ''),
			partner1_surname    = COALESCE((SELECT surname    FROM persons WHERE id = partner1_id), '')
		WHERE partner1_id IS NOT NULL AND partner1_given_name IS NULL
	`)
	_, _ = s.db.Exec(`
		UPDATE families SET
			partner2_given_name = COALESCE((SELECT given_name FROM persons WHERE id = partner2_id), ''),
			partner2_surname    = COALESCE((SELECT surname    FROM persons WHERE id = partner2_id), '')
		WHERE partner2_id IS NOT NULL AND partner2_given_name IS NULL
	`)
	_, _ = s.db.Exec(`
		UPDATE family_children SET
			person_given_name = COALESCE((SELECT given_name FROM persons WHERE id = person_id), ''),
			person_surname    = COALESCE((SELECT surname    FROM persons WHERE id = person_id), '')
		WHERE person_id IS NOT NULL AND person_given_name IS NULL
	`)

	// Add repository_id to sources for ID-based source→repository linkage (issue #525).
	_, _ = s.db.Exec(`ALTER TABLE sources ADD COLUMN repository_id TEXT`)

	// Branch scoping for the slice entities (ADR-005 / #669), the person/family
	// facts (#757) and the evidence (#758). Add branch_id + deleted to each table in branchScopedTables. SQLite ALTER TABLE ADD COLUMN has no
	// IF NOT EXISTS, so these bare execs swallow the "duplicate column" error on
	// databases that already have the columns (project convention).
	//
	// NOTE: SQLite cannot alter a table's PRIMARY KEY in place, so an existing
	// database created before #669 keeps its lone-id primary key and can only hold
	// mainline rows. Full branch overlay (a shadow row per branch) requires the
	// composite (id, branch_id) primary key defined in createTables above, which
	// only a freshly built read model has. Such a pre-#669 SQLite database therefore
	// stays main-only for these tables until the read model is recreated from the
	// event store; the branch_id/deleted columns below keep mainline reads working in
	// the meantime. NOTE: there is no automated read-model rebuild command in the repo
	// yet — recreation is currently manual (delete the read-model DB and let it
	// re-project on startup). TODO(#669 follow-up): add a rebuild command and link it
	// here once it exists.
	s.migrateBranchColumns()

	// Secondary indexes leading with branch_id so PurgeBranch's DELETE ... WHERE
	// branch_id = ? (and the overlay's branch_id IN filter) is index-driven rather
	// than a full-table scan; the composite PK leads with id (#669). No (id, branch_id)
	// indexes are created: the composite PK's autoindex already covers that prefix
	// (confirmed via EXPLAIN QUERY PLAN), so a separate one would only add write cost.
	for _, tbl := range branchScopedTables {
		// #nosec G202 -- tbl comes from the fixed branchScopedTables list, not user input.
		// nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query
		_, _ = s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_` + tbl + `_branch ON ` + tbl + `(branch_id)`)
	}

	// Refresh the collection-table indexes to include branch_id (the overlay filters
	// on parent + branch), matching the postgres schema. Old single-column variants
	// are replaced.
	_, _ = s.db.Exec(`DROP INDEX IF EXISTS idx_person_names_person`)
	_, _ = s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_person_names_person ON person_names(person_id, branch_id)`)
	_, _ = s.db.Exec(`DROP INDEX IF EXISTS idx_person_external_ids_person`)
	_, _ = s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_person_external_ids_person ON person_external_ids(person_id, branch_id)`)
	_, _ = s.db.Exec(`DROP INDEX IF EXISTS idx_family_external_ids_family`)
	_, _ = s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_family_external_ids_family ON family_external_ids(family_id, branch_id)`)
	_, _ = s.db.Exec(`DROP INDEX IF EXISTS idx_source_external_ids_source`)
	_, _ = s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_source_external_ids_source ON source_external_ids(source_id, branch_id)`)

	// Composite UNIQUE indexes matching every ON CONFLICT target used by the slice
	// upserts (SavePerson/SaveFamily/SavePersonName/SaveFamilyChild/SavePedigreeEdge
	// and the branch tombstones). A freshly built read model already gets these from
	// its composite PRIMARY KEY autoindex, but SQLite cannot add that composite PK to
	// a pre-#669 database in place, so an UPGRADED DB keeps its old single-column PK
	// and has no unique index for the (id, branch_id)/(family_id, person_id, branch_id)/
	// (person_id, branch_id) conflict targets — without which even a mainline projection
	// write (ON CONFLICT ... DO UPDATE) fails. Creating them here keeps upgraded DBs
	// writable; on fresh DBs the IF NOT EXISTS duplicate is harmless (a second unique
	// index over the same columns as the PK autoindex). The pre-#669 rows are all
	// branch_id = main and were already unique under the old PK, so uniqueness holds.
	// person_external_ids/family_external_ids/source_external_ids are omitted: they use plain DELETE+INSERT
	// (no ON CONFLICT) and carry multiple rows per (parent, branch), so a unique index
	// on the parent+branch would be wrong.
	for _, idx := range []struct{ name, table, cols string }{
		{"idx_persons_id_branch", "persons", "id, branch_id"},
		{"idx_families_id_branch", "families", "id, branch_id"},
		{"idx_person_names_id_branch", "person_names", "id, branch_id"},
		{"idx_family_children_fam_person_branch", "family_children", "family_id, person_id, branch_id"},
		{"idx_pedigree_edges_person_branch", "pedigree_edges", "person_id, branch_id"},
		// Person/family facts (#757): same reasoning for a database created before
		// these tables gained their composite key.
		{"idx_life_events_id_branch", "life_events", "id, branch_id"},
		{"idx_attributes_id_branch", "attributes", "id, branch_id"},
		{"idx_associations_id_branch", "associations", "id, branch_id"},
		// Evidence (#758): same reasoning for a database created before these
		// tables gained their composite key.
		{"idx_sources_id_branch", "sources", "id, branch_id"},
		{"idx_citations_id_branch", "citations", "id, branch_id"},
		{"idx_notes_id_branch", "notes", "id, branch_id"},
		// Media (#759): same reasoning for a database created before media gained
		// its composite key.
		{"idx_media_id_branch", "media", "id, branch_id"},
		// GPS artifacts (#760): same reasoning for a database created before
		// these tables gained their composite key.
		{"idx_evidence_analyses_id_branch", "evidence_analyses", "id, branch_id"},
		{"idx_evidence_conflicts_id_branch", "evidence_conflicts", "id, branch_id"},
		{"idx_research_logs_id_branch", "research_logs", "id, branch_id"},
		{"idx_proof_summaries_id_branch", "proof_summaries", "id, branch_id"},
	} {
		// #nosec G201 -- idx fields are internal constants, never user input.
		_, _ = s.db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS ` + idx.name + ` ON ` + idx.table + `(` + idx.cols + `)`)
	}
}

// branchScopedTables are every read-model table carrying branch_id/deleted: the
// seven #669 slice tables, the three person/family fact tables (#757), the
// four evidence tables (#758), media (#759) and the four GPS artifact tables
// (#760). PurgeBranch drops a branch's rows from each, and runMigrations gives
// each its branch_id-leading index.
var branchScopedTables = []string{
	"persons", "families", "family_children", "pedigree_edges",
	"person_names", "person_external_ids", "family_external_ids",
	"life_events", "attributes", "associations",
	"sources", "source_external_ids", "citations", "notes",
	"media",
	"evidence_analyses", "evidence_conflicts", "research_logs", "proof_summaries",
}

// branchKeyedTables are the branch-scoped tables whose row identity must include
// branch_id for a branch shadow row to exist. persons stands for the seven #669
// slice tables (they gained their composite key in one schema revision), the
// fact tables for #757, the evidence tables for #758, media for #759 and the GPS
// artifact tables for #760.
// detectBranchCapable
// requires branch_id in every one of their PRIMARY KEYs.
var branchKeyedTables = []string{
	"persons",
	"life_events", "attributes", "associations",
	"sources", "source_external_ids", "citations", "notes",
	"media",
	"evidence_analyses", "evidence_conflicts", "research_logs", "proof_summaries",
}

// migrateBranchColumns adds the #669 branch_id/deleted columns to any slice table
// that predates them. SQLite has no ALTER TABLE ... ADD COLUMN IF NOT EXISTS, so
// (per this file's convention) the duplicate-column error is swallowed; the same
// applies when the table does not exist yet on a fresh database. It is idempotent
// and safe to call more than once, and MUST run before any index on branch_id is
// created — see the note in createTables.
func (s *ReadModelStore) migrateBranchColumns() {
	for _, tbl := range branchScopedTables {
		// #nosec G201 -- tbl comes from the fixed branchScopedTables list, not user input.
		_, _ = s.db.Exec(`ALTER TABLE ` + tbl + ` ADD COLUMN branch_id TEXT NOT NULL DEFAULT '` + mainBranchID + `'`)
		// #nosec G201 -- tbl comes from the fixed branchScopedTables list, not user input.
		_, _ = s.db.Exec(`ALTER TABLE ` + tbl + ` ADD COLUMN deleted INTEGER NOT NULL DEFAULT 0`)
	}
}

// renameLegacyEventsTable renames a pre-#733 read-model `events` table to
// `life_events`, preserving its rows. It is idempotent and MUST run before
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
// The owner_type discriminator (eventsTableBelongsToReadModel) is the load-bearing
// guard: the event log's own `events` table lives in the same database once both
// stores share one SQLITE_PATH (ADR-002), and only the read model's table has that
// column. Without the check this would rename the event log's source of truth out
// from under it. Do not remove it, and do not treat a failed probe as "not legacy".
func (s *ReadModelStore) renameLegacyEventsTable() error {
	legacy, err := eventsTableBelongsToReadModel(s.db)
	if err != nil {
		return fmt.Errorf("check whether the events table belongs to the read model: %w", err)
	}
	if !legacy {
		// Fresh database, already migrated, or the table is the event log's: nothing to do.
		return nil
	}

	var existing int
	if err := s.db.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'life_events'`,
	).Scan(&existing); err != nil {
		return fmt.Errorf("check for an existing life_events table: %w", err)
	}
	if existing > 0 {
		return ErrConflictingEventsTables
	}

	// SQLite carries indexes across ALTER TABLE ... RENAME TO, so the old idx_events_*
	// names would end up attached to life_events beside the new idx_life_events_* ones.
	// Spelled out rather than looped over a slice so each statement is a static string
	// literal: nothing is concatenated into SQL here, which is both simpler to audit and
	// keeps SQL-injection scanners from flagging a query they cannot see is constant.
	if _, err := s.db.Exec(`DROP INDEX IF EXISTS idx_events_owner`); err != nil {
		return fmt.Errorf("drop stale read-model index idx_events_owner: %w", err)
	}
	if _, err := s.db.Exec(`DROP INDEX IF EXISTS idx_events_fact_type`); err != nil {
		return fmt.Errorf("drop stale read-model index idx_events_fact_type: %w", err)
	}
	if _, err := s.db.Exec(`ALTER TABLE events RENAME TO life_events`); err != nil {
		return fmt.Errorf("rename the read model's legacy events table to life_events: %w", err)
	}
	return nil
}

// legacyFTS5Objects are the full-text triggers and tables earlier versions
// created when their SQLite driver had FTS5 compiled in. Search no longer reads
// them (ADR-002, #822), but a trigger left in place would keep writing to its
// index on every person save, so they are dropped. Triggers go first: they
// reference the tables.
var legacyFTS5Objects = []string{
	`DROP TRIGGER IF EXISTS persons_fts_insert`,
	`DROP TRIGGER IF EXISTS persons_fts_delete`,
	`DROP TRIGGER IF EXISTS persons_fts_update`,
	`DROP TRIGGER IF EXISTS person_names_fts_insert`,
	`DROP TRIGGER IF EXISTS person_names_fts_delete`,
	`DROP TRIGGER IF EXISTS person_names_fts_update`,
	`DROP TABLE IF EXISTS persons_fts`,
	`DROP TABLE IF EXISTS person_names_fts`,
}

// dropLegacyFTS5 removes the FTS5 search index an earlier version may have left
// in the database. It is a no-op on a database that never had one.
func (s *ReadModelStore) dropLegacyFTS5() error {
	for _, stmt := range legacyFTS5Objects {
		if _, err := s.db.Exec(stmt); err != nil {
			return fmt.Errorf("drop legacy full-text index (%s): %w", stmt, err)
		}
	}
	return nil
}

// personOverlaySubquery returns a parenthesized subquery that resolves the
// branch overlay for persons (ADR-005 / #669): every id resolves to the branch's
// own row when it has one, else the mainline row, with deleted=1 tombstones
// removed. For MainBranchID this is exactly the set of mainline rows, reproducing
// pre-branch behavior. The three placeholders bind (branch, branch, main).
func personOverlaySubquery(branchID domain.BranchID) (string, []any) {
	if branchID.IsMain() {
		// Main fast path (issue #669): main never shadows itself, so persons holds
		// exactly one row per id. Skip the ROW_NUMBER window (which materializes and
		// sorts the whole table, defeating index-driven pagination) and filter the
		// table directly; the plain subquery flattens into the caller so its ORDER
		// BY/LIMIT can use the sort indexes and short-circuit.
		return `(SELECT * FROM persons WHERE branch_id = ? AND deleted = 0)`, []any{mainBranchID}
	}
	sub := `(
			SELECT * FROM (
				SELECT *, ROW_NUMBER() OVER (PARTITION BY id ORDER BY (branch_id = ?) DESC) AS rn
				FROM persons
				WHERE branch_id IN (?, ?)
			) WHERE rn = 1 AND deleted = 0
		)`
	return sub, []any{branchID.String(), branchID.String(), mainBranchID}
}

// personOverlayCTE is personOverlaySubquery for a statement that reads the overlay
// more than once (the birth-place and death-place legs of GetPlaceHierarchy's UNION).
// Off main it hoists the overlay into a `resolved` CTE, which SQLite materializes
// because it is referenced more than once, so the ROW_NUMBER pass over the branch's
// rows plus all of main's runs once per call instead of once per leg.
//
// Main keeps the inline subquery (the #669 fast path): its plain indexed branch_id
// filter flattens into each leg and touches only that leg's column, so a mainline call
// must not start materializing a CTE it did not before.
//
// Returns the WITH head to prefix to the statement's own CTE list, the source to read
// FROM, the args bound once for the CTE, and the args bound at each FROM reference.
// Exactly one of cteArgs/legArgs is non-empty, so callers interleave both in
// statement order.
func personOverlayCTE(branchID domain.BranchID) (withClause, src string, cteArgs, legArgs []any) {
	sub, args := personOverlaySubquery(branchID)
	if branchID.IsMain() {
		return "WITH ", sub, nil, args
	}
	return "WITH resolved AS " + sub + ",\n\t\t\t", "resolved", args, nil
}

// GetPerson retrieves a person by ID within the branch overlay.
func (s *ReadModelStore) GetPerson(ctx context.Context, branchID domain.BranchID, id uuid.UUID) (*repository.PersonReadModel, error) {
	// #nosec G202 -- the query is built by factGetQuery from package constants; every value is a bound placeholder
	// nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query
	row := s.db.QueryRowContext(ctx, factGetQuery("persons", personSelectCols), factGetArgs(branchID, id)...)

	return scanPerson(row)
}

// ListPersons returns a paginated list of persons within the branch overlay.
func (s *ReadModelStore) ListPersons(ctx context.Context, opts repository.ListOptions) ([]repository.PersonReadModel, int, error) {
	// Resolve the branch overlay first, then filter/count/sort/paginate over it.
	overlay, overlayArgs := personOverlaySubquery(opts.BranchID)

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
	countQuery := "SELECT COUNT(*) FROM " + overlay + " " + whereClause
	countArgs := append(append([]any{}, overlayArgs...), whereArgs...)
	err := s.db.QueryRowContext(ctx, countQuery, countArgs...).Scan(&total)
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
			   notes, research_status, brick_wall_note, brick_wall_since, brick_wall_resolved_at,
			   version, updated_at
		FROM %s
		%s
		ORDER BY %s %s, given_name %s
		LIMIT ? OFFSET ?
	`, overlay, whereClause, orderColumn, orderDir, orderDir)

	// Build args: overlay args + where args + limit + offset
	queryArgs := append(append(append([]any{}, overlayArgs...), whereArgs...), opts.Limit, opts.Offset)
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

// SearchPersons searches for persons by name (substring, fuzzy or Soundex),
// date ranges, and place filters.
//
// Name matching follows the PostgreSQL store so both databases find the same
// people (DB-005, ADR-002): a plain query is a case-insensitive substring match,
// and a fuzzy query is pg_trgm trigram similarity, computed in Go here because
// SQLite has no trigram operator.
func (s *ReadModelStore) SearchPersons(ctx context.Context, opts repository.SearchOptions) ([]repository.PersonReadModel, error) {
	// Normalize limit
	limit := opts.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	// Trim the query and places as the PostgreSQL store does: a whitespace-only
	// query is "no query" (a filters-only search), a blank place is no filter, and
	// " London" filters on "London".
	opts.Query = strings.TrimSpace(opts.Query)
	opts.BirthPlace = strings.TrimSpace(opts.BirthPlace)
	opts.DeathPlace = strings.TrimSpace(opts.DeathPlace)
	hasQuery := opts.Query != ""
	hasDateFilter := opts.BirthDateFrom != nil || opts.BirthDateTo != nil ||
		opts.DeathDateFrom != nil || opts.DeathDateTo != nil
	hasPlaceFilter := opts.BirthPlace != "" || opts.DeathPlace != ""

	// Trigram similarity, post-filtered in Go, as PostgreSQL's `%` operator.
	// Checked before Soundex: PostgreSQL gives fuzzy precedence when both are set.
	if hasQuery && opts.Fuzzy {
		return s.searchPersonsFuzzy(ctx, opts, limit)
	}

	// Soundex: fetch candidates with SQL filters, then post-filter in Go
	if hasQuery && opts.Soundex {
		return s.searchPersonsSoundex(ctx, opts, limit)
	}

	// Substring name matching combined with date/place SQL filters.
	if hasQuery {
		return s.searchPersonsLike(ctx, opts, limit)
	}

	// No text query — filter only by date/place
	if hasDateFilter || hasPlaceFilter {
		return s.searchPersonsFiltersOnly(ctx, opts, limit)
	}

	// No criteria at all — return empty
	return nil, nil
}

// searchPersonsLike matches the query as a case-insensitive substring of a
// person's full name or of an alternate name's full name or nickname, with
// date/place filters: PostgreSQL's ILIKE arm (full_name ILIKE '%' || q || '%'),
// matched by ilike_contains so case folding and wildcards agree with it. This
// is the SQLite search strategy (ADR-002): a scan rather than an index, which
// keeps it identical to PostgreSQL's substring arm without a second search index
// to maintain. (Given name and surname need no test of their own: full_name is
// given_name || ' ' || surname, so it contains any substring of either.)
//
// Alternate names are matched in an uncorrelated IN subquery, which SQLite runs
// once, rather than joined per person: a LEFT JOIN against the names overlay
// scans every name for every person, which is quadratic in tree size.
func (s *ReadModelStore) searchPersonsLike(ctx context.Context, opts repository.SearchOptions, limit int) ([]repository.PersonReadModel, error) {
	query, args := searchPersonsLikeSQL(opts, limit)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("search persons: %w", err)
	}
	defer rows.Close()

	return scanPersonRows(rows)
}

// searchPersonsLikeSQL builds searchPersonsLike's statement and arguments.
func searchPersonsLikeSQL(opts repository.SearchOptions, limit int) (string, []any) {
	filterSQL, filterArgs := buildDatePlaceFilters(opts)
	orderClause := searchOrderClause(opts, "p.")
	overlay, overlayArgs := personOverlaySubquery(opts.BranchID)
	namesOverlay, namesArgs := personNamesOverlaySubquery(opts.BranchID)

	var sb strings.Builder
	var args []any
	args = append(args, overlayArgs...)

	sb.WriteString(`
		SELECT p.id, p.given_name, p.surname, p.full_name, p.gender,
			   p.birth_date_raw, p.birth_date_sort, p.birth_place, p.birth_place_lat, p.birth_place_long,
			   p.death_date_raw, p.death_date_sort, p.death_place, p.death_place_lat, p.death_place_long,
			   p.notes, p.research_status, p.brick_wall_note, p.brick_wall_since, p.brick_wall_resolved_at,
			   p.version, p.updated_at
		FROM ` + overlay + ` p
		WHERE (` + containsFoldFunc + `(p.full_name, ?)
		   OR p.id IN (SELECT person_id FROM ` + namesOverlay + `
			   WHERE ` + containsFoldFunc + `(full_name, ?) OR ` + containsFoldFunc + `(nickname, ?)))`)
	args = append(args, opts.Query)
	args = append(args, namesArgs...)
	args = append(args, opts.Query, opts.Query)

	if filterSQL != "" {
		sb.WriteString(" AND " + filterSQL)
		args = append(args, filterArgs...)
	}

	sb.WriteString(" ORDER BY " + orderClause + " LIMIT ?")
	args = append(args, limit)
	return sb.String(), args
}

// personNamesOverlaySubquery resolves person_names for a branch (partition by
// name id, branch row wins, tombstones excluded) so a branch-overridden name
// does not also match its shadowed mainline value. Off main the three
// placeholders bind (branch, branch, main); on main the one placeholder binds
// main.
func personNamesOverlaySubquery(branchID domain.BranchID) (string, []any) {
	if branchID.IsMain() {
		// Main fast path, as personOverlaySubquery (#669): main never shadows
		// itself, so person_names holds one row per id and the ROW_NUMBER window
		// over the whole table is unnecessary.
		return `(
			SELECT person_id, full_name, given_name, surname, nickname, is_primary
			FROM person_names WHERE branch_id = ? AND deleted = 0
		)`, []any{mainBranchID}
	}
	return `(
			SELECT person_id, full_name, given_name, surname, nickname, is_primary FROM (
				SELECT person_id, full_name, given_name, surname, nickname, is_primary, deleted,
					   ROW_NUMBER() OVER (PARTITION BY id ORDER BY (branch_id = ?) DESC) AS rrn
				FROM person_names WHERE branch_id IN (?, ?)
			) WHERE rrn = 1 AND deleted = 0
		)`, []any{branchID.String(), branchID.String(), mainBranchID}
}

// searchPersonsFuzzy matches names by trigram similarity, the semantics of
// PostgreSQL's pg_trgm `%` operator that the PostgreSQL store uses for fuzzy
// search: a person matches when the query is similar enough to their given
// name, surname or full name, or to the given name, surname, full name or
// nickname of one of their alternate names. SQLite has no trigram operator, so
// every person that passes the date/place filters is read and scored in Go;
// unlike the Soundex path there is no candidate cap, so a match is never missed
// because it sorted late. The scan reads only the columns that are scored or
// sorted on; the full rows are fetched for the people that make the limit.
//
// Results are ordered by best similarity (highest first, or lowest with
// order=asc), ties broken by surname, given name and id as on PostgreSQL,
// unless a name or date sort is requested.
func (s *ReadModelStore) searchPersonsFuzzy(ctx context.Context, opts repository.SearchOptions, limit int) ([]repository.PersonReadModel, error) {
	filterSQL, filterArgs := buildDatePlaceFilters(opts)
	overlay, overlayArgs := personOverlaySubquery(opts.BranchID)
	namesOverlay, namesArgs := personNamesOverlaySubquery(opts.BranchID)

	var sb strings.Builder
	args := append(append([]any{}, overlayArgs...), namesArgs...)
	sb.WriteString(`
		SELECT p.id, p.given_name, p.surname, p.full_name, p.birth_date_sort, p.death_date_sort,
			   pn.given_name, pn.surname, pn.full_name, pn.nickname, pn.is_primary
		FROM ` + overlay + ` p
		LEFT JOIN ` + namesOverlay + ` pn ON p.id = pn.person_id`)
	if filterSQL != "" {
		sb.WriteString(" WHERE " + filterSQL)
		args = append(args, filterArgs...)
	}

	rows, err := s.db.QueryContext(ctx, sb.String(), args...)
	if err != nil {
		return nil, fmt.Errorf("search persons fuzzy: %w", err)
	}
	matched, err := scoreFuzzyRows(rows, repository.NewTrigramQuery(opts.Query))
	rows.Close()
	if err != nil {
		return nil, fmt.Errorf("search persons fuzzy: %w", err)
	}
	ranked := orderFuzzyMatches(matched, opts)
	if len(ranked) > limit {
		ranked = ranked[:limit]
	}
	if len(ranked) == 0 {
		return nil, nil
	}

	ids := make([]uuid.UUID, len(ranked))
	for i, p := range ranked {
		ids[i] = p.ID
	}
	full, err := s.GetPersonsByIDs(ctx, opts.BranchID, ids)
	if err != nil {
		return nil, fmt.Errorf("search persons fuzzy: %w", err)
	}
	byID := make(map[uuid.UUID]repository.PersonReadModel, len(full))
	for _, p := range full {
		byID[p.ID] = p
	}
	results := make([]repository.PersonReadModel, 0, len(ranked))
	for _, id := range ids {
		if p, ok := byID[id]; ok {
			results = append(results, p)
		}
	}
	return results, nil
}

// fuzzyMatch is a person that matched a fuzzy search, with their best score.
// Only the fields searchPersonsFuzzy scores and sorts on are set.
type fuzzyMatch struct {
	person repository.PersonReadModel
	score  float64
}

// scoreFuzzyRows reads searchPersonsFuzzy's rows (id, given name, surname, full
// name, birth and death sort dates, then one alternate name's given name,
// surname, full name, nickname and primary flag, NULL when the person has none)
// and returns the people who match, in first-seen order.
//
// A person with several alternate names spans several rows; they are merged.
// The score follows PostgreSQL's dedup (DISTINCT ON (id) ORDER BY is_primary
// DESC, rank_score DESC), where the direct match on the person's own names
// counts as primary: the best score among the direct match and the primary
// alternate names, and only when none of those match, the best score among the
// non-primary alternate names.
func scoreFuzzyRows(rows *sql.Rows, query repository.TrigramQuery) ([]fuzzyMatch, error) {
	best := func(names ...string) (float64, bool) {
		top, hit := 0.0, false
		for _, n := range names {
			if sim := query.Similarity(n); sim >= repository.TrigramThreshold {
				hit, top = true, max(top, sim)
			}
		}
		return top, hit
	}
	parseSortDate := func(v sql.NullString) *time.Time {
		if !v.Valid {
			return nil
		}
		t, err := time.Parse("2006-01-02", v.String)
		if err != nil {
			return nil
		}
		return &t
	}

	// Each person keeps a score per tier: primary (the direct match and primary
	// alternate names) and other (non-primary alternate names).
	type entry struct {
		fuzzyMatch
		primary, other       float64
		primaryHit, otherHit bool
	}
	byID := make(map[string]*entry)
	var order []*entry
	for rows.Next() {
		var (
			idStr, given, surname, full          string
			birthSort, deathSort                 sql.NullString
			altGiven, altSurname, altFull, altNk sql.NullString
			altPrimary                           sql.NullBool
		)
		if err := rows.Scan(&idStr, &given, &surname, &full, &birthSort, &deathSort,
			&altGiven, &altSurname, &altFull, &altNk, &altPrimary); err != nil {
			return nil, err
		}
		e, seen := byID[idStr]
		if !seen {
			id, err := uuid.Parse(idStr)
			if err != nil {
				return nil, fmt.Errorf("parse person id %q: %w", idStr, err)
			}
			e = &entry{fuzzyMatch: fuzzyMatch{person: repository.PersonReadModel{
				ID: id, GivenName: given, Surname: surname, FullName: full,
				BirthDateSort: parseSortDate(birthSort), DeathDateSort: parseSortDate(deathSort),
			}}}
			e.primary, e.primaryHit = best(given, surname, full)
			byID[idStr] = e
			order = append(order, e)
		}
		if !altGiven.Valid {
			continue
		}
		isPrimary := altPrimary.Valid && altPrimary.Bool
		if !isPrimary && e.primaryHit {
			continue // a primary-tier match already decides the score
		}
		sim, hit := best(altGiven.String, altSurname.String, altFull.String, altNk.String)
		switch {
		case !hit:
		case isPrimary:
			e.primaryHit, e.primary = true, max(e.primary, sim)
		default:
			e.otherHit, e.other = true, max(e.other, sim)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	var matched []fuzzyMatch
	for _, e := range order {
		switch {
		case e.primaryHit:
			e.score = e.primary
		case e.otherHit:
			e.score = e.other
		default:
			continue
		}
		matched = append(matched, e.fuzzyMatch)
	}
	return matched, nil
}

// orderFuzzyMatches orders fuzzy results: by the requested name or date sort,
// else by score (best first, or worst first with order=asc) with ties broken by
// surname, given name and id (always ascending), the order PostgreSQL gives
// fuzzy results, so both return the same people when more match than the limit.
func orderFuzzyMatches(matched []fuzzyMatch, opts repository.SearchOptions) []repository.PersonReadModel {
	switch opts.Sort {
	case "name", "birth_date", "death_date":
	default:
		asc := strings.EqualFold(opts.Order, "asc")
		sort.SliceStable(matched, func(i, j int) bool {
			a, b := matched[i], matched[j]
			switch {
			case a.score != b.score && asc:
				return a.score < b.score
			case a.score != b.score:
				return a.score > b.score
			case a.person.Surname != b.person.Surname:
				return a.person.Surname < b.person.Surname
			case a.person.GivenName != b.person.GivenName:
				return a.person.GivenName < b.person.GivenName
			default:
				return bytes.Compare(a.person.ID[:], b.person.ID[:]) < 0
			}
		})
	}
	results := make([]repository.PersonReadModel, 0, len(matched))
	for _, m := range matched {
		results = append(results, m.person)
	}
	switch opts.Sort {
	case "name", "birth_date", "death_date":
		sortPersonResults(results, opts)
	}
	return results
}

// searchPersonsSoundex fetches candidates filtered by date/place, then post-filters using Soundex in Go.
func (s *ReadModelStore) searchPersonsSoundex(ctx context.Context, opts repository.SearchOptions, limit int) ([]repository.PersonReadModel, error) {
	filterSQL, filterArgs := buildDatePlaceFilters(opts)

	// Fetch a large candidate set (up to 1000) narrowed by date/place filters
	candidateLimit := 1000
	overlay, overlayArgs := personOverlaySubquery(opts.BranchID)

	var sb strings.Builder
	var args []any
	args = append(args, overlayArgs...)

	sb.WriteString(`
		SELECT p.id, p.given_name, p.surname, p.full_name, p.gender,
			   p.birth_date_raw, p.birth_date_sort, p.birth_place, p.birth_place_lat, p.birth_place_long,
			   p.death_date_raw, p.death_date_sort, p.death_place, p.death_place_lat, p.death_place_long,
			   p.notes, p.research_status, p.brick_wall_note, p.brick_wall_since, p.brick_wall_resolved_at,
			   p.version, p.updated_at
		FROM ` + overlay + ` p`)

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
		namesByPerson, err = s.loadPersonNamesForSoundex(ctx, opts.BranchID, candidates)
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
func (s *ReadModelStore) loadPersonNamesForSoundex(ctx context.Context, branchID domain.BranchID, persons []repository.PersonReadModel) (map[string][]nameEntry, error) {
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

		// Include both the branch's own and the mainline name rows (deleted=0) as
		// Soundex candidates; the person set itself is already branch-resolved by the
		// caller, so a loose union of alternate names is sufficient for matching.
		var sb strings.Builder
		var args []any
		sb.WriteString("SELECT person_id, given_name, surname, nickname FROM person_names WHERE deleted = 0 AND branch_id IN (?, ?) AND person_id IN (")
		args = append(args, branchID.String(), mainBranchID)
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
	orderClause := searchOrderClause(opts, "p.")
	overlay, overlayArgs := personOverlaySubquery(opts.BranchID)

	var sb strings.Builder
	var args []any
	args = append(args, overlayArgs...)

	sb.WriteString(`
		SELECT p.id, p.given_name, p.surname, p.full_name, p.gender,
			   p.birth_date_raw, p.birth_date_sort, p.birth_place, p.birth_place_lat, p.birth_place_long,
			   p.death_date_raw, p.death_date_sort, p.death_place, p.death_place_lat, p.death_place_long,
			   p.notes, p.research_status, p.brick_wall_note, p.brick_wall_since, p.brick_wall_resolved_at,
			   p.version, p.updated_at
		FROM ` + overlay + ` p`)

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
	// Places match as PostgreSQL's ILIKE '%' || place || '%' (see containsFoldFunc).
	// SearchPersons has already trimmed them.
	if opts.BirthPlace != "" {
		conditions = append(conditions, containsFoldFunc+"(p.birth_place, ?)")
		args = append(args, opts.BirthPlace)
	}
	if opts.DeathPlace != "" {
		conditions = append(conditions, containsFoldFunc+"(p.death_place, ?)")
		args = append(args, opts.DeathPlace)
	}

	if len(conditions) == 0 {
		return "", nil
	}
	return strings.Join(conditions, " AND "), args
}

// searchOrderClause returns the SQL ORDER BY columns for search results.
// prefix is the table alias prefix (e.g., "p." for JOINed queries, "" for CTEs).
// Relevance (and the default) orders by name: a substring match has no score.
func searchOrderClause(opts repository.SearchOptions, prefix string) string {
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
	default: // "relevance" or empty
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

// SavePerson saves or updates a person on the given branch. A non-main branch
// stores its own (id, branch_id) shadow row layered over the mainline.
func (s *ReadModelStore) SavePerson(ctx context.Context, branchID domain.BranchID, person *repository.PersonReadModel) error {
	if err := s.guardBranchWrite(branchID); err != nil {
		return err
	}
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

	// Convert brick wall timestamps to nullable strings
	var brickWallSince, brickWallResolvedAt sql.NullString
	if person.BrickWallSince != nil {
		brickWallSince = sql.NullString{String: formatTimestamp(*person.BrickWallSince), Valid: true}
	}
	if person.BrickWallResolvedAt != nil {
		brickWallResolvedAt = sql.NullString{String: formatTimestamp(*person.BrickWallResolvedAt), Valid: true}
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO persons (id, given_name, surname, gender, birth_date_raw, birth_date_sort, birth_place,
							 birth_place_lat, birth_place_long, death_date_raw, death_date_sort, death_place,
							 death_place_lat, death_place_long, notes, research_status,
							 brick_wall_note, brick_wall_since, brick_wall_resolved_at,
							 version, updated_at, branch_id, deleted)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0)
		ON CONFLICT(id, branch_id) DO UPDATE SET
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
			brick_wall_note = excluded.brick_wall_note,
			brick_wall_since = excluded.brick_wall_since,
			brick_wall_resolved_at = excluded.brick_wall_resolved_at,
			version = excluded.version,
			updated_at = excluded.updated_at,
			deleted = 0
	`, person.ID.String(), person.GivenName, person.Surname, string(person.Gender),
		person.BirthDateRaw, birthDateSort, person.BirthPlace, birthPlaceLat, birthPlaceLong,
		person.DeathDateRaw, deathDateSort, person.DeathPlace, deathPlaceLat, deathPlaceLong,
		person.Notes, string(person.ResearchStatus),
		person.BrickWallNote, brickWallSince, brickWallResolvedAt,
		person.Version, formatTimestamp(person.UpdatedAt), branchID.String())

	return err
}

// DeletePerson removes a person. On the mainline this is a real DELETE that
// cascades (in code, since the read-model FKs were dropped for branch scoping) to
// the person's names, external IDs, pedigree edge, and associations. On a
// non-main branch it writes a deleted=1 tombstone for the person plus cascade
// tombstones for its names and external IDs, so the mainline fallback cannot
// resurrect them (mirrors the memory backend).
func (s *ReadModelStore) DeletePerson(ctx context.Context, branchID domain.BranchID, id uuid.UUID) error {
	if err := s.guardBranchWrite(branchID); err != nil {
		return err
	}
	if branchID.IsMain() {
		return s.deletePersonMain(ctx, id)
	}
	return s.tombstonePersonBranch(ctx, branchID, id)
}

// deletePersonMain removes the mainline person row and its dependents. This
// reproduces the pre-branch DELETE + ON DELETE CASCADE behavior explicitly, now
// that the read-model foreign keys have been dropped to allow branch scoping.
// person_names, person_external_ids, pedigree_edges and associations (both
// person_id and associate_id) were ON DELETE CASCADE pre-#669. attributes
// referenced persons(id) with NO ON DELETE (RESTRICT), which would have blocked
// the delete; blocking is not reproducible against an append-only event log, so
// we cascade-delete orphan attributes too. The person's own life events go with it
// as well (#757), and so does its media (#759), under the blob rule (see
// cascadeMedia), and every GPS artifact about the person (#760).
func (s *ReadModelStore) deletePersonMain(ctx context.Context, id uuid.UUID) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	idStr := id.String()
	stmts := []struct {
		sql  string
		args []any
	}{
		{"DELETE FROM person_names WHERE person_id = ? AND branch_id = ?", []any{idStr, mainBranchID}},
		{"DELETE FROM person_external_ids WHERE person_id = ? AND branch_id = ?", []any{idStr, mainBranchID}},
		{"DELETE FROM pedigree_edges WHERE person_id = ? AND branch_id = ?", []any{idStr, mainBranchID}},
		// family_children referenced persons(id) ON DELETE CASCADE on the child side,
		// so drop the deleted person from every mainline family it was a child of.
		{"DELETE FROM family_children WHERE person_id = ? AND branch_id = ?", []any{idStr, mainBranchID}},
		{"DELETE FROM persons WHERE id = ? AND branch_id = ?", []any{idStr, mainBranchID}},
	}
	for _, st := range stmts {
		if _, err := tx.ExecContext(ctx, st.sql, st.args...); err != nil {
			return fmt.Errorf("delete person (main): %w", err)
		}
	}
	if err := cascadePersonFacts(ctx, tx, domain.MainBranchID, id); err != nil {
		return err
	}
	if err := cascadeMedia(ctx, tx, personMediaFilter, domain.MainBranchID, id); err != nil {
		return err
	}
	if err := cascadeGPS(ctx, tx, domain.MainBranchID, id); err != nil {
		return err
	}
	return tx.Commit()
}

// tombstonePersonBranch writes branch tombstones for a person and its branch-scoped
// dependents (names, external IDs, pedigree edge, the life events, attributes
// and associations it owns or appears in, its media and the GPS artifacts about
// it) so the mainline fallback does not resurrect the entity.
func (s *ReadModelStore) tombstonePersonBranch(ctx context.Context, branchID domain.BranchID, id uuid.UUID) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO persons (id, given_name, surname, branch_id, deleted)
		VALUES (?, '', '', ?, 1)
		ON CONFLICT(id, branch_id) DO UPDATE SET deleted = 1
	`, id.String(), branchID.String()); err != nil {
		return fmt.Errorf("tombstone person: %w", err)
	}
	if err := tombstoneNamesBranch(ctx, tx, id, branchID); err != nil {
		return err
	}
	if err := tombstoneExternalIDsBranch(ctx, tx, "person_external_ids", "person_id", id, branchID); err != nil {
		return err
	}
	// Cascade tombstone the person's pedigree edge (mirrors DeletePedigreeEdge).
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO pedigree_edges (person_id, branch_id, deleted)
		VALUES (?, ?, 1)
		ON CONFLICT(person_id, branch_id) DO UPDATE SET deleted = 1
	`, id.String(), branchID.String()); err != nil {
		return fmt.Errorf("tombstone pedigree edge: %w", err)
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

// tombstoneNamesBranch hides every name of a person on a non-main branch: it
// drops the branch's own shadow names for the person and writes a deleted=1
// tombstone for each mainline name id, so the per-row name overlay resolves the
// person's names as absent on the branch.
func tombstoneNamesBranch(ctx context.Context, tx *sql.Tx, personID uuid.UUID, branchID domain.BranchID) error {
	// The branch's own rows for the person become tombstones in place, which
	// also hides a mainline name the branch had re-owned TO this person.
	if _, err := tx.ExecContext(ctx,
		"UPDATE person_names SET deleted = 1 WHERE person_id = ? AND branch_id = ?",
		personID.String(), branchID.String()); err != nil {
		return fmt.Errorf("tombstone branch names: %w", err)
	}
	// Then the mainline names the branch has not shadowed. A name the branch
	// re-owned AWAY from this person (PersonMerged moves the merged person's
	// names to the survivor, #834) has a branch row under its new owner and
	// must keep it, so it is skipped rather than tombstoned over.
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO person_names (id, person_id, given_name, surname, branch_id, deleted)
		SELECT id, person_id, '', '', ?, 1 FROM person_names
		WHERE person_id = ? AND branch_id = ?
		  AND id NOT IN (SELECT id FROM person_names WHERE branch_id = ?)
		ON CONFLICT(id, branch_id) DO UPDATE SET deleted = 1
	`, branchID.String(), personID.String(), mainBranchID, branchID.String()); err != nil {
		return fmt.Errorf("tombstone names: %w", err)
	}
	return nil
}

// tombstoneChildrenBranch hides every child of a family on a non-main branch,
// analogously to tombstoneNamesBranch, keyed by (family_id, person_id).
func tombstoneChildrenBranch(ctx context.Context, tx *sql.Tx, familyID uuid.UUID, branchID domain.BranchID) error {
	if _, err := tx.ExecContext(ctx,
		"DELETE FROM family_children WHERE family_id = ? AND branch_id = ?",
		familyID.String(), branchID.String()); err != nil {
		return fmt.Errorf("clear branch children: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO family_children (family_id, person_id, branch_id, deleted)
		SELECT family_id, person_id, ?, 1 FROM family_children WHERE family_id = ? AND branch_id = ?
		ON CONFLICT(family_id, person_id, branch_id) DO UPDATE SET deleted = 1
	`, branchID.String(), familyID.String(), mainBranchID); err != nil {
		return fmt.Errorf("tombstone children: %w", err)
	}
	return nil
}

// tombstoneExternalIDsBranch writes an empty-bucket tombstone for an external-id
// collection on a non-main branch: it removes the branch's own rows for the
// parent and inserts a single deleted=1 marker row. Bucket presence (the marker)
// makes GetXExternalIDs resolve the branch's empty bucket instead of falling back
// to the mainline identifiers.
func tombstoneExternalIDsBranch(ctx context.Context, tx *sql.Tx, table, parentCol string, parentID uuid.UUID, branchID domain.BranchID) error {
	// #nosec G201 G202 -- table and parentCol are internal constants, never user input.
	if _, err := tx.ExecContext(ctx,
		"DELETE FROM "+table+" WHERE "+parentCol+" = ? AND branch_id = ?",
		parentID.String(), branchID.String()); err != nil {
		return fmt.Errorf("clear branch external ids: %w", err)
	}
	// #nosec G201 G202 -- table and parentCol are internal constants, never user input.
	if _, err := tx.ExecContext(ctx,
		"INSERT INTO "+table+" ("+parentCol+", sequence, value, type, branch_id, deleted) VALUES (?, 0, '', '', ?, 1)",
		parentID.String(), branchID.String()); err != nil {
		return fmt.Errorf("tombstone external ids: %w", err)
	}
	return nil
}

// SavePersonName saves or updates a person name variant on the given branch. A
// non-main branch writes its own (id, branch_id) shadow row; the per-name overlay
// layers it over the mainline name of the same id.
func (s *ReadModelStore) SavePersonName(ctx context.Context, branchID domain.BranchID, name *repository.PersonNameReadModel) error {
	if err := s.guardBranchWrite(branchID); err != nil {
		return err
	}
	isPrimary := 0
	if name.IsPrimary {
		isPrimary = 1
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO person_names (id, person_id, given_name, surname, name_prefix, name_suffix,
								  surname_prefix, nickname, name_type, is_primary, updated_at, branch_id, deleted)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0)
		ON CONFLICT(id, branch_id) DO UPDATE SET
			person_id = excluded.person_id,
			given_name = excluded.given_name,
			surname = excluded.surname,
			name_prefix = excluded.name_prefix,
			name_suffix = excluded.name_suffix,
			surname_prefix = excluded.surname_prefix,
			nickname = excluded.nickname,
			name_type = excluded.name_type,
			is_primary = excluded.is_primary,
			updated_at = excluded.updated_at,
			deleted = 0
	`, name.ID.String(), name.PersonID.String(), name.GivenName, name.Surname,
		nullableString(name.NamePrefix), nullableString(name.NameSuffix),
		nullableString(name.SurnamePrefix), nullableString(name.Nickname),
		string(name.NameType), isPrimary, formatTimestamp(name.UpdatedAt), branchID.String())

	return err
}

// GetPersonName retrieves a person name by ID within the branch overlay.
func (s *ReadModelStore) GetPersonName(ctx context.Context, branchID domain.BranchID, nameID uuid.UUID) (*repository.PersonNameReadModel, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, person_id, given_name, surname, full_name, name_prefix, name_suffix,
			   surname_prefix, nickname, name_type, is_primary, updated_at
		FROM (
			SELECT *, ROW_NUMBER() OVER (PARTITION BY id ORDER BY (branch_id = ?) DESC) AS rn
			FROM person_names WHERE id = ? AND branch_id IN (?, ?)
		) WHERE rn = 1 AND deleted = 0
	`, branchID.String(), nameID.String(), branchID.String(), mainBranchID)

	return scanPersonName(row)
}

// GetPersonNames retrieves all name variants for a person within the branch
// overlay (each name id resolves to the branch's row if present, else mainline;
// tombstones are excluded).
func (s *ReadModelStore) GetPersonNames(ctx context.Context, branchID domain.BranchID, personID uuid.UUID) ([]repository.PersonNameReadModel, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, person_id, given_name, surname, full_name, name_prefix, name_suffix,
			   surname_prefix, nickname, name_type, is_primary, updated_at
		FROM (
			SELECT *, ROW_NUMBER() OVER (PARTITION BY id ORDER BY (branch_id = ?) DESC) AS rn
			FROM person_names
			WHERE id IN (SELECT id FROM person_names WHERE person_id = ? AND branch_id IN (?, ?))
			  AND branch_id IN (?, ?)
		)
		WHERE rn = 1 AND deleted = 0 AND person_id = ?
		ORDER BY is_primary DESC, name_type
	`, branchID.String(), personID.String(), branchID.String(), mainBranchID,
		branchID.String(), mainBranchID, personID.String())
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

// DeletePersonName removes a person name. On the mainline this is a real DELETE;
// on a non-main branch it writes a deleted=1 tombstone for the name id (seeded
// from the resolved row so the required person_id is populated) so the overlay
// hides the mainline name.
func (s *ReadModelStore) DeletePersonName(ctx context.Context, branchID domain.BranchID, nameID uuid.UUID) error {
	if err := s.guardBranchWrite(branchID); err != nil {
		return err
	}
	if branchID.IsMain() {
		_, err := s.db.ExecContext(ctx,
			"DELETE FROM person_names WHERE id = ? AND branch_id = ?", nameID.String(), mainBranchID)
		return err
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO person_names (id, person_id, given_name, surname, branch_id, deleted)
		SELECT id, person_id, '', '', ?, 1 FROM person_names
		WHERE id = ? AND branch_id IN (?, ?)
		ORDER BY (branch_id = ?) DESC LIMIT 1
		ON CONFLICT(id, branch_id) DO UPDATE SET deleted = 1
	`, branchID.String(), nameID.String(), branchID.String(), mainBranchID, branchID.String())
	return err
}

// extIDValue is the branch-agnostic payload of an external identifier, shared by
// the person and family external-id replace helpers.
type extIDValue struct {
	Value string
	Type  string
}

func externalIDValues(ids []repository.PersonExternalIDReadModel) []extIDValue {
	out := make([]extIDValue, len(ids))
	for i, id := range ids {
		out[i] = extIDValue{Value: id.Value, Type: id.Type}
	}
	return out
}

func familyExternalIDValues(ids []repository.FamilyExternalIDReadModel) []extIDValue {
	out := make([]extIDValue, len(ids))
	for i, id := range ids {
		out[i] = extIDValue{Value: id.Value, Type: id.Type}
	}
	return out
}

// externalIDsOverlayQuery returns the single-statement per-parent bucket overlay
// for an external-id table. It resolves the branch's bucket when the branch has
// any row for the parent (including a deleted=1 empty-bucket marker), else the
// mainline bucket. Bind args: parentID, parentID, branch, branch, main.
func externalIDsOverlayQuery(table, parentCol string) string {
	// #nosec G201 G202 -- table and parentCol are internal constants, never user input.
	return "SELECT sequence, value, type FROM " + table + `
		WHERE ` + parentCol + ` = ? AND deleted = 0
		  AND branch_id = (
			CASE WHEN EXISTS(SELECT 1 FROM ` + table + " WHERE " + parentCol + ` = ? AND branch_id = ?)
				 THEN ? ELSE ? END)
		ORDER BY sequence`
}

// replaceExternalIDs replaces a parent's external-id bucket on the given branch
// within one transaction. On the mainline an empty set removes the bucket (real
// delete). On a non-main branch an empty set writes a single deleted=1 marker row
// (a present-but-empty tombstone that hides the mainline identifiers).
func replaceExternalIDs(ctx context.Context, db *sql.DB, table, parentCol string, branchID domain.BranchID, parentID uuid.UUID, ids []extIDValue) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// #nosec G201 G202 -- table and parentCol are internal constants, never user input.
	if _, err := tx.ExecContext(ctx,
		"DELETE FROM "+table+" WHERE "+parentCol+" = ? AND branch_id = ?",
		parentID.String(), branchID.String()); err != nil {
		return fmt.Errorf("delete external ids: %w", err)
	}

	if len(ids) == 0 {
		if !branchID.IsMain() {
			// #nosec G201 G202 -- table and parentCol are internal constants, never user input.
			if _, err := tx.ExecContext(ctx,
				"INSERT INTO "+table+" ("+parentCol+", sequence, value, type, branch_id, deleted) VALUES (?, 0, '', '', ?, 1)",
				parentID.String(), branchID.String()); err != nil {
				return fmt.Errorf("tombstone external ids: %w", err)
			}
		}
		return tx.Commit()
	}

	for i, id := range ids {
		// #nosec G201 G202 -- table and parentCol are internal constants, never user input.
		if _, err := tx.ExecContext(ctx,
			"INSERT INTO "+table+" ("+parentCol+", sequence, value, type, branch_id, deleted) VALUES (?, ?, ?, ?, ?, 0)",
			parentID.String(), i, id.Value, id.Type, branchID.String()); err != nil {
			return fmt.Errorf("insert external id: %w", err)
		}
	}
	return tx.Commit()
}

// ReplacePersonExternalIDs replaces all external identifiers (GEDCOM 7.0 EXID)
// for a person within a single transaction, scoped to the branch.
func (s *ReadModelStore) ReplacePersonExternalIDs(ctx context.Context, branchID domain.BranchID, personID uuid.UUID, ids []repository.PersonExternalIDReadModel) error {
	if err := s.guardBranchWrite(branchID); err != nil {
		return err
	}
	return replaceExternalIDs(ctx, s.db, "person_external_ids", "person_id", branchID, personID, externalIDValues(ids))
}

// GetPersonExternalIDs retrieves all external identifiers for a person within the
// branch overlay, ordered by their original sequence. External IDs resolve as a
// per-parent bucket (a present branch bucket wins wholesale over the mainline; an
// empty branch bucket — a single deleted=1 marker row — is a tombstone).
func (s *ReadModelStore) GetPersonExternalIDs(ctx context.Context, branchID domain.BranchID, personID uuid.UUID) ([]repository.PersonExternalIDReadModel, error) {
	rows, err := s.db.QueryContext(ctx, externalIDsOverlayQuery("person_external_ids", "person_id"),
		personID.String(), personID.String(), branchID.String(), branchID.String(), mainBranchID)
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
// for a family within a single transaction.
func (s *ReadModelStore) ReplaceFamilyExternalIDs(ctx context.Context, branchID domain.BranchID, familyID uuid.UUID, ids []repository.FamilyExternalIDReadModel) error {
	if err := s.guardBranchWrite(branchID); err != nil {
		return err
	}
	return replaceExternalIDs(ctx, s.db, "family_external_ids", "family_id", branchID, familyID, familyExternalIDValues(ids))
}

// GetFamilyExternalIDs retrieves all external identifiers for a family within the
// branch overlay, ordered by their original sequence (per-parent bucket overlay,
// same model as GetPersonExternalIDs).
func (s *ReadModelStore) GetFamilyExternalIDs(ctx context.Context, branchID domain.BranchID, familyID uuid.UUID) ([]repository.FamilyExternalIDReadModel, error) {
	rows, err := s.db.QueryContext(ctx, externalIDsOverlayQuery("family_external_ids", "family_id"),
		familyID.String(), familyID.String(), branchID.String(), branchID.String(), mainBranchID)
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
// for a source within a single transaction, scoped to the branch (#758): the
// same per-parent bucket model as ReplacePersonExternalIDs.
func (s *ReadModelStore) ReplaceSourceExternalIDs(ctx context.Context, branchID domain.BranchID, sourceID uuid.UUID, ids []repository.SourceExternalIDReadModel) error {
	if err := s.guardBranchWrite(branchID); err != nil {
		return err
	}
	values := make([]extIDValue, len(ids))
	for i, id := range ids {
		values[i] = extIDValue{Value: id.Value, Type: id.Type}
	}
	return replaceExternalIDs(ctx, s.db, "source_external_ids", "source_id", branchID, sourceID, values)
}

// GetSourceExternalIDs retrieves all external identifiers for a source within the
// branch overlay, ordered by their original sequence (per-parent bucket overlay,
// same model as GetPersonExternalIDs).
func (s *ReadModelStore) GetSourceExternalIDs(ctx context.Context, branchID domain.BranchID, sourceID uuid.UUID) ([]repository.SourceExternalIDReadModel, error) {
	rows, err := s.db.QueryContext(ctx, externalIDsOverlayQuery("source_external_ids", "source_id"),
		sourceID.String(), sourceID.String(), branchID.String(), branchID.String(), mainBranchID)
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

	if _, err := tx.ExecContext(ctx, "DELETE FROM repository_external_ids WHERE repository_id = ?", repositoryID.String()); err != nil {
		return fmt.Errorf("delete repository external ids: %w", err)
	}
	for i, id := range ids {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO repository_external_ids (repository_id, sequence, value, type)
			VALUES (?, ?, ?, ?)
		`, repositoryID.String(), i, id.Value, id.Type); err != nil {
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
		WHERE repository_id = ? ORDER BY sequence
	`, repositoryID.String())
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

// familyOverlaySubquery returns a parenthesized subquery that resolves the branch
// overlay for families, mirroring personOverlaySubquery. Bind order: (branch,
// branch, main).
func familyOverlaySubquery(branchID domain.BranchID) (string, []any) {
	if branchID.IsMain() {
		// Main fast path (issue #669): see personOverlaySubquery. Skip the window and
		// filter families directly so the caller's ORDER BY/LIMIT stays index-driven.
		return `(SELECT * FROM families WHERE branch_id = ? AND deleted = 0)`, []any{mainBranchID}
	}
	sub := `(
			SELECT * FROM (
				SELECT *, ROW_NUMBER() OVER (PARTITION BY id ORDER BY (branch_id = ?) DESC) AS rn
				FROM families
				WHERE branch_id IN (?, ?)
			) WHERE rn = 1 AND deleted = 0
		)`
	return sub, []any{branchID.String(), branchID.String(), mainBranchID}
}

// GetFamily retrieves a family by ID within the branch overlay.
func (s *ReadModelStore) GetFamily(ctx context.Context, branchID domain.BranchID, id uuid.UUID) (*repository.FamilyReadModel, error) {
	// #nosec G202 -- the query is built by factGetQuery from package constants; every value is a bound placeholder
	// nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query
	row := s.db.QueryRowContext(ctx, factGetQuery("families", familySelectCols), factGetArgs(branchID, id)...)

	return scanFamily(row)
}

// ListFamilies returns a paginated list of families within the branch overlay.
func (s *ReadModelStore) ListFamilies(ctx context.Context, opts repository.ListOptions) ([]repository.FamilyReadModel, int, error) {
	overlay, overlayArgs := familyOverlaySubquery(opts.BranchID)

	var total int
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+overlay, overlayArgs...).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("count families: %w", err)
	}

	queryArgs := append(append([]any{}, overlayArgs...), opts.Limit, opts.Offset)
	// #nosec G202 -- overlay is a hardcoded SQL fragment from familyOverlaySubquery; branch/entity values are bound parameters, not user input
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, partner1_id, partner1_given_name, partner1_surname,
			   partner2_id, partner2_given_name, partner2_surname,
			   relationship_type, marriage_date_raw, marriage_date_sort, marriage_place,
			   marriage_place_lat, marriage_place_long,
			   child_count, version, updated_at
		FROM `+overlay+`
		ORDER BY updated_at DESC
		LIMIT ? OFFSET ?
	`, queryArgs...)
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
// the branch overlay.
func (s *ReadModelStore) GetFamiliesForPerson(ctx context.Context, branchID domain.BranchID, personID uuid.UUID) ([]repository.FamilyReadModel, error) {
	overlay, overlayArgs := familyOverlaySubquery(branchID)
	args := append(append([]any{}, overlayArgs...), personID.String(), personID.String())
	// #nosec G202 -- overlay is a hardcoded SQL fragment from familyOverlaySubquery; branch/entity values are bound parameters, not user input
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, partner1_id, partner1_given_name, partner1_surname,
			   partner2_id, partner2_given_name, partner2_surname,
			   relationship_type, marriage_date_raw, marriage_date_sort, marriage_place,
			   marriage_place_lat, marriage_place_long,
			   child_count, version, updated_at
		FROM `+overlay+`
		WHERE partner1_id = ? OR partner2_id = ?
	`, args...)
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

// SaveFamily saves or updates a family on the given branch.
func (s *ReadModelStore) SaveFamily(ctx context.Context, branchID domain.BranchID, family *repository.FamilyReadModel) error {
	if err := s.guardBranchWrite(branchID); err != nil {
		return err
	}
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
		INSERT INTO families (id, partner1_id, partner1_given_name, partner1_surname,
							  partner2_id, partner2_given_name, partner2_surname,
							  relationship_type, marriage_date_raw, marriage_date_sort, marriage_place,
							  marriage_place_lat, marriage_place_long,
							  child_count, version, updated_at, branch_id, deleted)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0)
		ON CONFLICT(id, branch_id) DO UPDATE SET
			partner1_id = excluded.partner1_id,
			partner1_given_name = excluded.partner1_given_name,
			partner1_surname = excluded.partner1_surname,
			partner2_id = excluded.partner2_id,
			partner2_given_name = excluded.partner2_given_name,
			partner2_surname = excluded.partner2_surname,
			relationship_type = excluded.relationship_type,
			marriage_date_raw = excluded.marriage_date_raw,
			marriage_date_sort = excluded.marriage_date_sort,
			marriage_place = excluded.marriage_place,
			marriage_place_lat = excluded.marriage_place_lat,
			marriage_place_long = excluded.marriage_place_long,
			child_count = excluded.child_count,
			version = excluded.version,
			updated_at = excluded.updated_at,
			deleted = 0
	`, family.ID.String(),
		partner1ID, family.Partner1GivenName, family.Partner1Surname,
		partner2ID, family.Partner2GivenName, family.Partner2Surname,
		string(family.RelationshipType), family.MarriageDateRaw, marriageDateSort, family.MarriagePlace,
		marriagePlaceLat, marriagePlaceLong,
		family.ChildCount, family.Version, formatTimestamp(family.UpdatedAt), branchID.String())

	return err
}

// DeleteFamily removes a family. On the mainline this is a real DELETE that
// cascades (in code) to the family's children, external IDs, life events and
// media. On a non-main
// branch it writes a deleted=1 tombstone for the family plus cascade tombstones
// for its children and external IDs (mirrors the memory backend).
func (s *ReadModelStore) DeleteFamily(ctx context.Context, branchID domain.BranchID, id uuid.UUID) error {
	if err := s.guardBranchWrite(branchID); err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// The family's own life events cascade with it on either scope (#757), and
	// so do its media (#759) and the GPS artifacts about it (#760).
	if err := cascadeFactRows(ctx, tx, "life_events", eventSelectCols, familyEventsFilter, branchID, id); err != nil {
		return err
	}
	if err := cascadeMedia(ctx, tx, familyMediaFilter, branchID, id); err != nil {
		return err
	}
	if err := cascadeGPS(ctx, tx, branchID, id); err != nil {
		return err
	}

	if branchID.IsMain() {
		stmts := []struct {
			sql  string
			args []any
		}{
			{"DELETE FROM family_children WHERE family_id = ? AND branch_id = ?", []any{id.String(), mainBranchID}},
			{"DELETE FROM family_external_ids WHERE family_id = ? AND branch_id = ?", []any{id.String(), mainBranchID}},
			{"DELETE FROM families WHERE id = ? AND branch_id = ?", []any{id.String(), mainBranchID}},
		}
		for _, st := range stmts {
			if _, err := tx.ExecContext(ctx, st.sql, st.args...); err != nil {
				return fmt.Errorf("delete family (main): %w", err)
			}
		}
		return tx.Commit()
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO families (id, branch_id, deleted)
		VALUES (?, ?, 1)
		ON CONFLICT(id, branch_id) DO UPDATE SET deleted = 1
	`, id.String(), branchID.String()); err != nil {
		return fmt.Errorf("tombstone family: %w", err)
	}
	if err := tombstoneChildrenBranch(ctx, tx, id, branchID); err != nil {
		return err
	}
	if err := tombstoneExternalIDsBranch(ctx, tx, "family_external_ids", "family_id", id, branchID); err != nil {
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
	rows, err := s.db.QueryContext(ctx, `
		SELECT family_id, person_id, person_given_name, person_surname, relationship_type, sequence
		FROM (
			SELECT *, ROW_NUMBER() OVER (PARTITION BY family_id, person_id ORDER BY (branch_id = ?) DESC) AS rn
			FROM family_children WHERE branch_id IN (?, ?)
		)
		WHERE rn = 1 AND deleted = 0
		ORDER BY family_id, person_id
	`, branchID.String(), branchID.String(), mainBranchID)
	if err != nil {
		return nil, fmt.Errorf("query all family children: %w", err)
	}
	defer rows.Close()
	return scanFamilyChildRows(rows)
}

// scanFamilyChildRows scans family_id, person_id, person_given_name,
// person_surname, relationship_type, sequence rows.
func scanFamilyChildRows(rows *sql.Rows) ([]repository.FamilyChildReadModel, error) {
	var children []repository.FamilyChildReadModel
	for rows.Next() {
		var (
			familyIDStr, personIDStr, relType string
			personGivenName, personSurname    sql.NullString
			sequence                          sql.NullInt64
		)
		err := rows.Scan(&familyIDStr, &personIDStr, &personGivenName, &personSurname, &relType, &sequence)
		if err != nil {
			return nil, fmt.Errorf("scan family child: %w", err)
		}

		fID, _ := uuid.Parse(familyIDStr)
		pID, _ := uuid.Parse(personIDStr)

		child := repository.FamilyChildReadModel{
			FamilyID:         fID,
			PersonID:         pID,
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
// resolving both the children set and each person through the branch overlay.
func (s *ReadModelStore) GetChildrenOfFamily(ctx context.Context, branchID domain.BranchID, familyID uuid.UUID) ([]repository.PersonReadModel, error) {
	personOverlay, personArgs := personOverlaySubquery(branchID)
	// #nosec G201 G202 -- personOverlay is a fixed internal subquery, not user input.
	query := `
		SELECT p.id, p.given_name, p.surname, p.full_name, p.gender,
			   p.birth_date_raw, p.birth_date_sort, p.birth_place, p.birth_place_lat, p.birth_place_long,
			   p.death_date_raw, p.death_date_sort, p.death_place, p.death_place_lat, p.death_place_long,
			   p.notes, p.research_status, p.brick_wall_note, p.brick_wall_since, p.brick_wall_resolved_at,
			   p.version, p.updated_at
		FROM ` + personOverlay + ` p
		JOIN (
			SELECT person_id, sequence FROM (
				SELECT person_id, sequence, deleted,
					   ROW_NUMBER() OVER (PARTITION BY family_id, person_id ORDER BY (branch_id = ?) DESC) AS rn
				FROM family_children WHERE family_id = ? AND branch_id IN (?, ?)
			) WHERE rn = 1 AND deleted = 0
		) fc ON p.id = fc.person_id
		ORDER BY fc.sequence, p.given_name`
	args := append(append([]any{}, personArgs...), branchID.String(), familyID.String(), branchID.String(), mainBranchID)
	rows, err := s.db.QueryContext(ctx, query, args...)
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
// the child link and the family through the branch overlay.
func (s *ReadModelStore) GetChildFamily(ctx context.Context, branchID domain.BranchID, personID uuid.UUID) (*repository.FamilyReadModel, error) {
	familyOverlay, familyArgs := familyOverlaySubquery(branchID)
	// #nosec G201 G202 -- familyOverlay is a fixed internal subquery, not user input.
	query := `
		SELECT f.id, f.partner1_id, f.partner1_given_name, f.partner1_surname,
			   f.partner2_id, f.partner2_given_name, f.partner2_surname,
			   f.relationship_type, f.marriage_date_raw, f.marriage_date_sort, f.marriage_place,
			   f.marriage_place_lat, f.marriage_place_long,
			   f.child_count, f.version, f.updated_at
		FROM ` + familyOverlay + ` f
		JOIN (
			SELECT family_id FROM (
				SELECT family_id, deleted,
					   ROW_NUMBER() OVER (PARTITION BY family_id, person_id ORDER BY (branch_id = ?) DESC) AS rn
				FROM family_children WHERE person_id = ? AND branch_id IN (?, ?)
			) WHERE rn = 1 AND deleted = 0
		) fc ON f.id = fc.family_id
		LIMIT 1`
	args := append(append([]any{}, familyArgs...), branchID.String(), personID.String(), branchID.String(), mainBranchID)
	row := s.db.QueryRowContext(ctx, query, args...)

	return scanFamily(row)
}

// SaveFamilyChild saves a family child relationship on the given branch.
func (s *ReadModelStore) SaveFamilyChild(ctx context.Context, branchID domain.BranchID, child *repository.FamilyChildReadModel) error {
	if err := s.guardBranchWrite(branchID); err != nil {
		return err
	}
	var sequence sql.NullInt64
	if child.Sequence != nil {
		sequence = sql.NullInt64{Int64: int64(*child.Sequence), Valid: true}
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO family_children (family_id, person_id, person_given_name, person_surname, relationship_type, sequence, branch_id, deleted)
		VALUES (?, ?, ?, ?, ?, ?, ?, 0)
		ON CONFLICT(family_id, person_id, branch_id) DO UPDATE SET
			person_given_name = excluded.person_given_name,
			person_surname = excluded.person_surname,
			relationship_type = excluded.relationship_type,
			sequence = excluded.sequence,
			deleted = 0
	`, child.FamilyID.String(), child.PersonID.String(),
		child.PersonGivenName, child.PersonSurname,
		string(child.RelationshipType), sequence, branchID.String())

	return err
}

// DeleteFamilyChild removes a family child relationship. On the mainline this is a
// real DELETE; on a non-main branch it writes a deleted=1 tombstone for the
// (family_id, person_id) so the overlay hides the mainline child.
func (s *ReadModelStore) DeleteFamilyChild(ctx context.Context, branchID domain.BranchID, familyID, personID uuid.UUID) error {
	if err := s.guardBranchWrite(branchID); err != nil {
		return err
	}
	if branchID.IsMain() {
		_, err := s.db.ExecContext(ctx,
			"DELETE FROM family_children WHERE family_id = ? AND person_id = ? AND branch_id = ?",
			familyID.String(), personID.String(), mainBranchID)
		return err
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO family_children (family_id, person_id, branch_id, deleted)
		VALUES (?, ?, ?, 1)
		ON CONFLICT(family_id, person_id, branch_id) DO UPDATE SET deleted = 1
	`, familyID.String(), personID.String(), branchID.String())
	return err
}

// GetPedigreeEdge returns the pedigree edge for a person within the branch overlay.
func (s *ReadModelStore) GetPedigreeEdge(ctx context.Context, branchID domain.BranchID, personID uuid.UUID) (*repository.PedigreeEdge, error) {
	var (
		personIDStr, fatherIDStr, motherIDStr, fatherName, motherName sql.NullString
	)

	err := s.db.QueryRowContext(ctx, `
		SELECT person_id, father_id, mother_id, father_name, mother_name
		FROM (
			SELECT *, ROW_NUMBER() OVER (PARTITION BY person_id ORDER BY (branch_id = ?) DESC) AS rn
			FROM pedigree_edges WHERE person_id = ? AND branch_id IN (?, ?)
		)
		WHERE rn = 1 AND deleted = 0
	`, branchID.String(), personID.String(), branchID.String(), mainBranchID).Scan(&personIDStr, &fatherIDStr, &motherIDStr, &fatherName, &motherName)

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

// SavePedigreeEdge saves a pedigree edge on the given branch.
func (s *ReadModelStore) SavePedigreeEdge(ctx context.Context, branchID domain.BranchID, edge *repository.PedigreeEdge) error {
	if err := s.guardBranchWrite(branchID); err != nil {
		return err
	}
	var fatherID, motherID sql.NullString
	if edge.FatherID != nil {
		fatherID = sql.NullString{String: edge.FatherID.String(), Valid: true}
	}
	if edge.MotherID != nil {
		motherID = sql.NullString{String: edge.MotherID.String(), Valid: true}
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO pedigree_edges (person_id, father_id, mother_id, father_name, mother_name, branch_id, deleted)
		VALUES (?, ?, ?, ?, ?, ?, 0)
		ON CONFLICT(person_id, branch_id) DO UPDATE SET
			father_id = excluded.father_id,
			mother_id = excluded.mother_id,
			father_name = excluded.father_name,
			mother_name = excluded.mother_name,
			deleted = 0
	`, edge.PersonID.String(), fatherID, motherID, edge.FatherName, edge.MotherName, branchID.String())

	return err
}

// DeletePedigreeEdge removes a pedigree edge. On the mainline this is a real
// DELETE; on a non-main branch it writes a deleted=1 tombstone for the person.
func (s *ReadModelStore) DeletePedigreeEdge(ctx context.Context, branchID domain.BranchID, personID uuid.UUID) error {
	if err := s.guardBranchWrite(branchID); err != nil {
		return err
	}
	if branchID.IsMain() {
		_, err := s.db.ExecContext(ctx,
			"DELETE FROM pedigree_edges WHERE person_id = ? AND branch_id = ?", personID.String(), mainBranchID)
		return err
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO pedigree_edges (person_id, branch_id, deleted)
		VALUES (?, ?, 1)
		ON CONFLICT(person_id, branch_id) DO UPDATE SET deleted = 1
	`, personID.String(), branchID.String())
	return err
}

// PurgeBranch hard-deletes every row for branchID across the branch-scoped
// tables (branchScopedTables). It is a no-op for the mainline (domain.MainBranchID), which is
// never purged. See ADR-005 and the branch-delete projection handler.
//
// Unlike every other branch-scoped write it is NOT guarded by guardBranchWrite:
// a database built between #669 and #757 is no longer branch-capable yet may
// already hold live branch rows in its slice tables, and those branches must
// stay deletable and mergeable. The DELETE is safe on any schema, because
// migrateBranchColumns gives every branchScopedTables table a branch_id column
// and a lone-id table simply holds no rows for the branch.
func (s *ReadModelStore) PurgeBranch(ctx context.Context, branchID domain.BranchID) error {
	if branchID.IsMain() {
		return nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	branchStr := branchID.String()
	// Main media tombstones kept alive only for this branch's shadows go first,
	// while the branch rows that name them still exist (#759).
	if err := gcMainMedia(ctx, tx, gcMainMediaBeforePurge, mainBranchID, branchStr, branchStr, mainBranchID); err != nil {
		return err
	}
	for _, table := range branchScopedTables {
		// #nosec G202 -- table is from the fixed branchScopedTables list, not user input
		// nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query
		if _, err := tx.ExecContext(ctx, "DELETE FROM "+table+" WHERE branch_id = ?", branchStr); err != nil {
			return fmt.Errorf("purge branch %s: %w", table, err)
		}
	}
	return tx.Commit()
}

// Helper functions for scanning rows

type rowScanner interface {
	Scan(dest ...any) error
}

// scanPerson scans the person columns (personSelectCols order) and then, into
// extra, any columns the query selects after them.
func scanPerson(row rowScanner, extra ...any) (*repository.PersonReadModel, error) {
	var (
		idStr, givenName, surname, fullName             string
		gender, birthDateRaw, birthDateSort, birthPlace sql.NullString
		birthPlaceLat, birthPlaceLong                   sql.NullString
		deathDateRaw, deathDateSort, deathPlace, notes  sql.NullString
		deathPlaceLat, deathPlaceLong                   sql.NullString
		researchStatus                                  sql.NullString
		brickWallNote                                   sql.NullString
		brickWallSince, brickWallResolvedAt             sql.NullString
		version                                         int64
		updatedAt                                       string
	)

	dest := []any{&idStr, &givenName, &surname, &fullName, &gender,
		&birthDateRaw, &birthDateSort, &birthPlace, &birthPlaceLat, &birthPlaceLong,
		&deathDateRaw, &deathDateSort, &deathPlace, &deathPlaceLat, &deathPlaceLong,
		&notes, &researchStatus, &brickWallNote, &brickWallSince, &brickWallResolvedAt,
		&version, &updatedAt}
	err := row.Scan(append(dest, extra...)...)

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
		BrickWallNote:  brickWallNote.String,
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
	if brickWallSince.Valid {
		if t, err := parseTimestamp(brickWallSince.String); err == nil {
			p.BrickWallSince = &t
		}
	}
	if brickWallResolvedAt.Valid {
		if t, err := parseTimestamp(brickWallResolvedAt.String); err == nil {
			p.BrickWallResolvedAt = &t
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
		partner1ID, partner2ID                                    sql.NullString
		partner1GivenName, partner1Surname                        sql.NullString
		partner2GivenName, partner2Surname                        sql.NullString
		relType, marriageDateRaw, marriageDateSort, marriagePlace sql.NullString
		marriagePlaceLat, marriagePlaceLong                       sql.NullString
		childCount                                                int
		version                                                   int64
		updatedAt                                                 string
	)

	err := row.Scan(&idStr,
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

	id, _ := uuid.Parse(idStr)
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

// soundex computes the American Soundex code for a string.
// Returns a 4-character code (letter + 3 digits), or "" for empty input.
// Soundex and SoundexMatch are in the shared repository package.

// Evidence tables (#758): sources, citations and notes are branch-scoped the
// same way as the person/family facts (ADR-005) and reuse their overlay helpers
// (factOverlaySubquery, factGetQuery, tombstoneFactRows, cascadeFactRows).

const (
	// sourceSelectCols is scanSource's column order (unaliased).
	sourceSelectCols = `id, source_type, title, author, publisher, publish_date_raw, publish_date_sort,
		url, repository_id, repository_name, collection_name, call_number, notes, gedcom_xref,
		citation_count, version, updated_at`

	// citationSelectCols is scanCitation's column order (unaliased).
	citationSelectCols = `id, source_id, source_title, fact_type, fact_owner_id, page, volume,
		source_quality, informant_type, evidence_type, quoted_text, analysis,
		template_id, fields_data, gedcom_xref, version, created_at`

	// noteSelectCols is scanNote's column order (unaliased).
	noteSelectCols = `id, text, mime, language, translations, gedcom_xref, version, updated_at`

	// citationOrder is the deterministic citation order every backend returns.
	citationOrder = `source_title ASC, fact_type ASC, id ASC`

	// Per-parent filters for citations; each binds its values once per ?.
	sourceCitationsFilter = `source_id = ?`
	personCitationsFilter = `fact_owner_id = ? AND fact_type LIKE 'person_%'`
	factCitationsFilter   = `fact_type = ? AND fact_owner_id = ?`
)

// scanSources drains rows of sourceSelectCols.
func scanSources(rows *sql.Rows) ([]repository.SourceReadModel, error) {
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

// scanCitations drains rows of citationSelectCols.
func scanCitations(rows *sql.Rows) ([]repository.CitationReadModel, error) {
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

// GetSource retrieves a source by ID within the branch overlay (ADR-005, #758).
func (s *ReadModelStore) GetSource(ctx context.Context, branchID domain.BranchID, id uuid.UUID) (*repository.SourceReadModel, error) {
	// #nosec G202 -- the query is built by factGetQuery from package constants; every value is a bound placeholder
	// nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query
	row := s.db.QueryRowContext(ctx, factGetQuery("sources", sourceSelectCols), factGetArgs(branchID, id)...)
	return scanSource(row)
}

// ListSources returns a paginated list of the sources visible on opts.BranchID
// (ADR-005, #758).
func (s *ReadModelStore) ListSources(ctx context.Context, opts repository.ListOptions) ([]repository.SourceReadModel, int, error) {
	sub, args := factOverlaySubquery("sources", "", nil, opts.BranchID)

	var total int
	// #nosec G202 -- sub is built from package constants; every value is a bound ? placeholder
	// nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+sub+" src", args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count sources: %w", err)
	}

	// #nosec G202 -- sub is built from package constants; every value is a bound ? placeholder
	// nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+sourceSelectCols+` FROM `+sub+` src
		ORDER BY title ASC, id ASC
		LIMIT ? OFFSET ?
	`, append(args, opts.Limit, opts.Offset)...)
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
// overlay is resolved FIRST (the subquery) and the query is matched against the
// winning rows only, so a branch retitle is found under its new title alone and
// a branch-deleted source never matches (#758).
func (s *ReadModelStore) SearchSources(ctx context.Context, branchID domain.BranchID, query string, limit int) ([]repository.SourceReadModel, error) {
	likeQuery := "%" + strings.ToLower(query) + "%"
	sub, args := factOverlaySubquery("sources", "", nil, branchID)

	// #nosec G202 -- sub is built from package constants; every value is a bound ? placeholder
	// nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+sourceSelectCols+` FROM `+sub+` src
		WHERE LOWER(title) LIKE ? OR LOWER(author) LIKE ?
		ORDER BY title ASC, id ASC
		LIMIT ?
	`, append(args, likeQuery, likeQuery, limit)...)
	if err != nil {
		return nil, fmt.Errorf("search sources: %w", err)
	}
	return scanSources(rows)
}

// SaveSource saves or updates a source on the given branch (ADR-005, #758). The
// row is keyed by (id, branch_id); a save always clears any prior tombstone.
func (s *ReadModelStore) SaveSource(ctx context.Context, branchID domain.BranchID, source *repository.SourceReadModel) error {
	if err := s.guardBranchWrite(branchID); err != nil {
		return err
	}

	var publishDateSort sql.NullString
	if source.PublishDateSort != nil {
		publishDateSort = sql.NullString{String: source.PublishDateSort.Format("2006-01-02"), Valid: true}
	}

	var repositoryID sql.NullString
	if source.RepositoryID != nil {
		repositoryID = sql.NullString{String: source.RepositoryID.String(), Valid: true}
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO sources (id, branch_id, source_type, title, author, publisher, publish_date_raw, publish_date_sort,
							 url, repository_id, repository_name, collection_name, call_number, notes, gedcom_xref,
							 citation_count, version, updated_at, deleted)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0)
		ON CONFLICT(id, branch_id) DO UPDATE SET
			source_type = excluded.source_type,
			title = excluded.title,
			author = excluded.author,
			publisher = excluded.publisher,
			publish_date_raw = excluded.publish_date_raw,
			publish_date_sort = excluded.publish_date_sort,
			url = excluded.url,
			repository_id = excluded.repository_id,
			repository_name = excluded.repository_name,
			collection_name = excluded.collection_name,
			call_number = excluded.call_number,
			notes = excluded.notes,
			gedcom_xref = excluded.gedcom_xref,
			citation_count = excluded.citation_count,
			version = excluded.version,
			updated_at = excluded.updated_at,
			deleted = 0
	`, source.ID.String(), branchID.String(), string(source.SourceType), source.Title, source.Author, source.Publisher,
		source.PublishDateRaw, publishDateSort, source.URL, repositoryID, source.RepositoryName, source.CollectionName,
		source.CallNumber, source.Notes, source.GedcomXref, source.CitationCount, source.Version,
		formatTimestamp(source.UpdatedAt))
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
	if err := s.guardBranchWrite(branchID); err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := cascadeFactRows(ctx, tx, "citations", citationSelectCols, sourceCitationsFilter, branchID, id); err != nil {
		return err
	}
	if err := cascadeMedia(ctx, tx, sourceMediaFilter, branchID, id); err != nil {
		return err
	}
	if branchID.IsMain() {
		if _, err := tx.ExecContext(ctx, "DELETE FROM source_external_ids WHERE source_id = ? AND branch_id = ?", id.String(), mainBranchID); err != nil {
			return fmt.Errorf("delete source external ids: %w", err)
		}
		if _, err := tx.ExecContext(ctx, "DELETE FROM sources WHERE id = ? AND branch_id = ?", id.String(), mainBranchID); err != nil {
			return fmt.Errorf("delete source: %w", err)
		}
		return tx.Commit()
	}
	if err := tombstoneExternalIDsBranch(ctx, tx, "source_external_ids", "source_id", id, branchID); err != nil {
		return err
	}
	sub, args := factOverlaySubquery("sources", "id = ?", []any{id.String()}, branchID)
	if err := tombstoneFactRows(ctx, tx, "sources", sourceSelectCols, branchID, sub, args); err != nil {
		return fmt.Errorf("tombstone source: %w", err)
	}
	return tx.Commit()
}

// GetCitation retrieves a citation by ID within the branch overlay (ADR-005, #758).
func (s *ReadModelStore) GetCitation(ctx context.Context, branchID domain.BranchID, id uuid.UUID) (*repository.CitationReadModel, error) {
	// #nosec G202 -- the query is built by factGetQuery from package constants; every value is a bound placeholder
	// nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query
	row := s.db.QueryRowContext(ctx, factGetQuery("citations", citationSelectCols), factGetArgs(branchID, id)...)
	return scanCitation(row)
}

// listFilteredCitations returns the citations visible on branchID that filter
// (a package constant, binding filterArgs) selects, decided on the winning row.
func (s *ReadModelStore) listFilteredCitations(ctx context.Context, branchID domain.BranchID, filter string, filterArgs []any) ([]repository.CitationReadModel, error) {
	sub, args := factOverlaySubquery("citations", filter, filterArgs, branchID)
	// #nosec G202 -- sub is built from package constants; every value is a bound ? placeholder
	// nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query
	rows, err := s.db.QueryContext(ctx, `SELECT `+citationSelectCols+` FROM `+sub+` c ORDER BY `+citationOrder, args...)
	if err != nil {
		return nil, err
	}
	return scanCitations(rows)
}

// GetCitationsForSource returns all citations of a source within the branch overlay.
func (s *ReadModelStore) GetCitationsForSource(ctx context.Context, branchID domain.BranchID, sourceID uuid.UUID) ([]repository.CitationReadModel, error) {
	citations, err := s.listFilteredCitations(ctx, branchID, sourceCitationsFilter, []any{sourceID.String()})
	if err != nil {
		return nil, fmt.Errorf("query citations for source: %w", err)
	}
	return citations, nil
}

// GetCitationsForPerson returns all citations of a person's facts within the
// branch overlay.
func (s *ReadModelStore) GetCitationsForPerson(ctx context.Context, branchID domain.BranchID, personID uuid.UUID) ([]repository.CitationReadModel, error) {
	citations, err := s.listFilteredCitations(ctx, branchID, personCitationsFilter, []any{personID.String()})
	if err != nil {
		return nil, fmt.Errorf("query citations for person: %w", err)
	}
	return citations, nil
}

// GetCitationsForFact returns all citations of a specific fact within the branch
// overlay.
func (s *ReadModelStore) GetCitationsForFact(ctx context.Context, branchID domain.BranchID, factType domain.FactType, factOwnerID uuid.UUID) ([]repository.CitationReadModel, error) {
	citations, err := s.listFilteredCitations(ctx, branchID, factCitationsFilter, []any{string(factType), factOwnerID.String()})
	if err != nil {
		return nil, fmt.Errorf("query citations for fact: %w", err)
	}
	return citations, nil
}

// ListCitations returns a paginated list of the citations visible on
// opts.BranchID (ADR-005, #758).
func (s *ReadModelStore) ListCitations(ctx context.Context, opts repository.ListOptions) ([]repository.CitationReadModel, int, error) {
	sub, args := factOverlaySubquery("citations", "", nil, opts.BranchID)

	var total int
	// #nosec G202 -- sub is built from package constants; every value is a bound ? placeholder
	// nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+sub+" c", args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count citations: %w", err)
	}

	// #nosec G202 -- sub is built from package constants; every value is a bound ? placeholder
	// nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+citationSelectCols+` FROM `+sub+` c
		ORDER BY `+citationOrder+`
		LIMIT ? OFFSET ?
	`, append(args, opts.Limit, opts.Offset)...)
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
	if err := s.guardBranchWrite(branchID); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO citations (id, branch_id, source_id, source_title, fact_type, fact_owner_id, page, volume,
							   source_quality, informant_type, evidence_type, quoted_text, analysis,
							   template_id, fields_data, gedcom_xref, version, created_at, deleted)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0)
		ON CONFLICT(id, branch_id) DO UPDATE SET
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
			fields_data = excluded.fields_data,
			gedcom_xref = excluded.gedcom_xref,
			version = excluded.version,
			deleted = 0
	`, citation.ID.String(), branchID.String(), citation.SourceID.String(), citation.SourceTitle,
		string(citation.FactType), citation.FactOwnerID.String(), citation.Page, citation.Volume,
		string(citation.SourceQuality), string(citation.InformantType), string(citation.EvidenceType),
		citation.QuotedText, citation.Analysis, citation.TemplateID, citation.FieldsJSON,
		citation.GedcomXref, citation.Version, formatTimestamp(citation.CreatedAt))
	if err != nil {
		return fmt.Errorf("save citation: %w", err)
	}
	return nil
}

// DeleteCitation removes a citation (ADR-005, #758): a real removal on main, a
// tombstone on a non-main branch. Other branches' rows are untouched.
func (s *ReadModelStore) DeleteCitation(ctx context.Context, branchID domain.BranchID, id uuid.UUID) error {
	if err := s.deleteFactRow(ctx, "citations", citationSelectCols, branchID, id); err != nil {
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
	// eventSelectCols is scanEvent's column order (unaliased).
	eventSelectCols = `id, owner_type, owner_id, fact_type, date_raw, date_sort,
		place, place_lat, place_long, address, description, cause,
		age, research_status, is_negated, version, created_at`

	// attributeSelectCols is scanAttribute's column order (unaliased).
	attributeSelectCols = `id, person_id, fact_type, value, date_raw, date_sort, place, version, created_at`

	// associationSelectCols is scanAssociation's column order (unaliased).
	associationSelectCols = `id, person_id, person_name, associate_id, associate_name,
		role, phrase, notes, note_ids, gedcom_xref, version, updated_at`

	// eventOrder is the deterministic life-event order every backend returns.
	eventOrder = `fact_type ASC, CASE WHEN date_sort IS NULL THEN 1 ELSE 0 END, date_sort ASC, id ASC`

	// Per-owner filters for the fact tables; each binds the owner id once per ?.
	personEventsFilter      = `owner_type = 'person' AND owner_id = ?`
	familyEventsFilter      = `owner_type = 'family' AND owner_id = ?`
	personAttributesFilter  = `person_id = ?`
	personAssociationFilter = `(person_id = ? OR associate_id = ?)`
)

// factOverlaySubquery returns a parenthesized subquery holding branchID's
// resolved view of an id-keyed branch-scoped table, and the args it binds in
// order. table and filter must be package constants: nothing here is user input.
//
// filter optionally narrows the rows (e.g. to one owner) and binds filterArgs.
// Off main it is applied twice: once to choose the candidate ids (any row of the
// id on either side matches), so the ROW_NUMBER window sorts only those ids
// rather than the whole table, and once to the winning row, so the answer is
// decided by the row the branch actually sees. The deleted filter stays outside
// the window so a branch tombstone suppresses the main fallback.
//
// Main takes the fast path (issue #669): main never shadows itself, so the plain
// branch-filtered subquery flattens into the caller and keeps its indexes.
func factOverlaySubquery(table, filter string, filterArgs []any, branchID domain.BranchID) (string, []any) {
	return overlayColsSubquery(table, "*", filter, filterArgs, branchID)
}

// overlayColsSubquery is factOverlaySubquery projected to cols ("*" for every
// column). An explicit column list keeps the columns it omits out of the
// ROW_NUMBER window entirely, which is how the media reads avoid ever touching
// the byte columns (#759). cols must be a package constant.
func overlayColsSubquery(table, cols, filter string, filterArgs []any, branchID domain.BranchID) (string, []any) {
	if branchID.IsMain() {
		// #nosec G202 -- table, cols and filter are package constants, not user input
		// nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query
		sub := "(SELECT " + cols + " FROM " + table + " WHERE branch_id = ? AND deleted = 0"
		args := []any{mainBranchID}
		if filter != "" {
			sub += " AND " + filter
			args = append(args, filterArgs...)
		}
		return sub + ")", args
	}
	branch := branchID.String()
	args := []any{branch, branch, mainBranchID}
	candidates, outer := "", ""
	if filter != "" {
		// #nosec G202 -- table, cols and filter are package constants, not user input
		// nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query
		candidates = " AND id IN (SELECT id FROM " + table + " WHERE branch_id IN (?, ?) AND " + filter + ")"
		outer = " AND " + filter
		args = append(args, branch, mainBranchID)
		args = append(args, filterArgs...)
		args = append(args, filterArgs...)
	}
	// The window needs deleted beside cols; "*" already carries it.
	inner := cols
	if cols != "*" {
		inner = cols + ", deleted"
	}
	// #nosec G202 -- table, cols and filter are package constants, not user input
	// nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query
	return "(SELECT " + cols + " FROM (SELECT " + inner + ", ROW_NUMBER() OVER (PARTITION BY id ORDER BY (branch_id = ?) DESC) AS rn FROM " +
		table + " WHERE branch_id IN (?, ?)" + candidates + ") WHERE rn = 1 AND deleted = 0" + outer + ")", args
}

// factGetQuery returns the single-row overlay lookup for an id-keyed
// branch-scoped table; bind factGetArgs. table and cols must be package constants.
func factGetQuery(table, cols string) string {
	// #nosec G202 -- table, cols and filter are package constants, not user input
	// nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query
	return "SELECT " + cols + " FROM (SELECT *, ROW_NUMBER() OVER (PARTITION BY id ORDER BY (branch_id = ?) DESC) AS rn FROM " +
		table + " WHERE id = ? AND branch_id IN (?, ?)) WHERE rn = 1 AND deleted = 0"
}

// factGetArgs are the args factGetQuery binds.
func factGetArgs(branchID domain.BranchID, id uuid.UUID) []any {
	return []any{branchID.String(), id.String(), branchID.String(), mainBranchID}
}

// sqlExecer is the ExecContext subset shared by *sql.DB and *sql.Tx.
type sqlExecer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// tombstoneFactRows writes a deleted=1 shadow row on branchID for every row of
// an id-keyed fact table that the overlay subquery sub (with its args) resolves,
// copying the resolved row so the table's NOT NULL columns hold values. One
// set-based statement; the WHERE true disambiguates SQLite's INSERT ... SELECT
// ... ON CONFLICT parse. table and cols must be package constants.
func tombstoneFactRows(ctx context.Context, db sqlExecer, table, cols string, branchID domain.BranchID, sub string, subArgs []any) error {
	// #nosec G202 -- table, cols and sub are built from package constants; every value is a bound ? placeholder
	// nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query
	_, err := db.ExecContext(ctx, `
		INSERT INTO `+table+` (`+cols+`, branch_id, deleted)
		SELECT `+cols+`, ?, 1 FROM `+sub+` r WHERE true
		ON CONFLICT(id, branch_id) DO UPDATE SET deleted = 1
	`, append([]any{branchID.String()}, subArgs...)...)
	return err
}

// deleteFactRow removes one row of an id-keyed branch-scoped fact table
// (ADR-005): a real removal on main; on a non-main branch a tombstone copying the
// row the branch currently resolves (with no visible row there is nothing to hide
// and nothing is written). table and cols must be package constants.
func (s *ReadModelStore) deleteFactRow(ctx context.Context, table, cols string, branchID domain.BranchID, id uuid.UUID) error {
	if err := s.guardBranchWrite(branchID); err != nil {
		return err
	}
	if branchID.IsMain() {
		// #nosec G202 -- table is a package constant, not user input
		// nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query
		_, err := s.db.ExecContext(ctx, "DELETE FROM "+table+" WHERE id = ? AND branch_id = ?", id.String(), mainBranchID)
		return err
	}
	sub, args := factOverlaySubquery(table, "id = ?", []any{id.String()}, branchID)
	return tombstoneFactRows(ctx, s.db, table, cols, branchID, sub, args)
}

// cascadeFactRows removes, on branchID, every row of an id-keyed fact table that
// ownerFilter selects for ownerID — the manual cascade DeletePerson/DeleteFamily
// run now that the read-model foreign keys are gone (#669, #757). On main the rows
// are deleted; off main each row visible on the branch is tombstoned in one
// set-based statement. table, cols and ownerFilter must be package constants.
func cascadeFactRows(ctx context.Context, tx *sql.Tx, table, cols, ownerFilter string, branchID domain.BranchID, ownerID uuid.UUID) error {
	ownerArgs := make([]any, strings.Count(ownerFilter, "?"))
	for i := range ownerArgs {
		ownerArgs[i] = ownerID.String()
	}
	if branchID.IsMain() {
		// #nosec G202 -- table and ownerFilter are package constants; values are bound ? placeholders
		// nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query
		if _, err := tx.ExecContext(ctx, "DELETE FROM "+table+" WHERE branch_id = ? AND "+ownerFilter,
			append([]any{mainBranchID}, ownerArgs...)...); err != nil {
			return fmt.Errorf("cascade delete %s: %w", table, err)
		}
		return nil
	}
	sub, args := factOverlaySubquery(table, ownerFilter, ownerArgs, branchID)
	if err := tombstoneFactRows(ctx, tx, table, cols, branchID, sub, args); err != nil {
		return fmt.Errorf("cascade tombstone %s: %w", table, err)
	}
	return nil
}

// cascadePersonFacts removes, on branchID, the person's life events and
// attributes and every association naming the person on either side (#757).
func cascadePersonFacts(ctx context.Context, tx *sql.Tx, branchID domain.BranchID, personID uuid.UUID) error {
	if err := cascadeFactRows(ctx, tx, "life_events", eventSelectCols, personEventsFilter, branchID, personID); err != nil {
		return err
	}
	if err := cascadeFactRows(ctx, tx, "attributes", attributeSelectCols, personAttributesFilter, branchID, personID); err != nil {
		return err
	}
	return cascadeFactRows(ctx, tx, "associations", associationSelectCols, personAssociationFilter, branchID, personID)
}

// GetEvent retrieves a life event by ID within the branch overlay (ADR-005).
func (s *ReadModelStore) GetEvent(ctx context.Context, branchID domain.BranchID, id uuid.UUID) (*repository.EventReadModel, error) {
	// #nosec G202 -- the query is built by factGetQuery from package constants; every value is a bound placeholder
	// nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query
	row := s.db.QueryRowContext(ctx, factGetQuery("life_events", eventSelectCols), factGetArgs(branchID, id)...)
	return scanEvent(row)
}

// ListEvents returns a paginated list of the life events visible on
// opts.BranchID (ADR-005).
func (s *ReadModelStore) ListEvents(ctx context.Context, opts repository.ListOptions) ([]repository.EventReadModel, int, error) {
	sub, args := factOverlaySubquery("life_events", "", nil, opts.BranchID)

	var total int
	// #nosec G202 -- sub is built from package constants; every value is a bound ? placeholder
	// nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+sub+" e", args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count events: %w", err)
	}

	// #nosec G202 -- sub is built from package constants; every value is a bound ? placeholder
	// nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+eventSelectCols+` FROM `+sub+` e
		ORDER BY `+eventOrder+`
		LIMIT ? OFFSET ?
	`, append(args, opts.Limit, opts.Offset)...)
	if err != nil {
		return nil, 0, fmt.Errorf("query events: %w", err)
	}
	events, err := scanLifeEvents(rows)
	if err != nil {
		return nil, 0, err
	}
	return events, total, nil
}

// scanLifeEvents drains rows of eventSelectCols.
func scanLifeEvents(rows *sql.Rows) ([]repository.EventReadModel, error) {
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

// listOwnerEvents returns one owner's life events visible on branchID.
func (s *ReadModelStore) listOwnerEvents(ctx context.Context, branchID domain.BranchID, ownerFilter string, ownerID uuid.UUID) ([]repository.EventReadModel, error) {
	sub, args := factOverlaySubquery("life_events", ownerFilter, []any{ownerID.String()}, branchID)
	// #nosec G202 -- sub is built from package constants; every value is a bound ? placeholder
	// nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query
	rows, err := s.db.QueryContext(ctx, `SELECT `+eventSelectCols+` FROM `+sub+` e ORDER BY `+eventOrder, args...)
	if err != nil {
		return nil, err
	}
	return scanLifeEvents(rows)
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
	if err := s.guardBranchWrite(branchID); err != nil {
		return err
	}

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
		data, err := json.Marshal(event.Address)
		if err != nil {
			return fmt.Errorf("marshal event address: %w", err)
		}
		addressJSON = string(data)
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
		INSERT INTO life_events (id, branch_id, owner_type, owner_id, fact_type, date_raw, date_sort,
		                    place, place_lat, place_long, address, description, cause,
		                    age, research_status, is_negated, version, created_at, deleted)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0)
		ON CONFLICT (id, branch_id) DO UPDATE SET
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
			is_negated = excluded.is_negated,
			version = excluded.version,
			deleted = 0
	`, event.ID.String(), branchID.String(), event.OwnerType, event.OwnerID.String(), string(event.FactType),
		event.DateRaw, dateSort, event.Place, placeLat, placeLong, addressJSON,
		description, cause, age, researchStatus, event.IsNegated, event.Version,
		formatTimestamp(event.CreatedAt))
	if err != nil {
		return fmt.Errorf("save event: %w", err)
	}
	return nil
}

// DeleteEvent removes a life event (ADR-005): a real removal on main, a
// tombstone on a non-main branch.
func (s *ReadModelStore) DeleteEvent(ctx context.Context, branchID domain.BranchID, id uuid.UUID) error {
	if err := s.deleteFactRow(ctx, "life_events", eventSelectCols, branchID, id); err != nil {
		return fmt.Errorf("delete event: %w", err)
	}
	return nil
}

// GetAttribute retrieves an attribute by ID within the branch overlay (ADR-005).
func (s *ReadModelStore) GetAttribute(ctx context.Context, branchID domain.BranchID, id uuid.UUID) (*repository.AttributeReadModel, error) {
	// #nosec G202 -- the query is built by factGetQuery from package constants; every value is a bound placeholder
	// nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query
	row := s.db.QueryRowContext(ctx, factGetQuery("attributes", attributeSelectCols), factGetArgs(branchID, id)...)
	return scanAttribute(row)
}

// scanAttributes drains rows of attributeSelectCols.
func scanAttributes(rows *sql.Rows) ([]repository.AttributeReadModel, error) {
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

// ListAttributes returns a paginated list of the attributes visible on
// opts.BranchID (ADR-005).
func (s *ReadModelStore) ListAttributes(ctx context.Context, opts repository.ListOptions) ([]repository.AttributeReadModel, int, error) {
	sub, args := factOverlaySubquery("attributes", "", nil, opts.BranchID)

	var total int
	// #nosec G202 -- sub is built from package constants; every value is a bound ? placeholder
	// nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+sub+" a", args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count attributes: %w", err)
	}

	// Sort by fact_type ASC, value ASC, id ASC for deterministic ordering
	// #nosec G202 -- sub is built from package constants; every value is a bound ? placeholder
	// nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+attributeSelectCols+` FROM `+sub+` a
		ORDER BY fact_type ASC, value ASC, id ASC
		LIMIT ? OFFSET ?
	`, append(args, opts.Limit, opts.Offset)...)
	if err != nil {
		return nil, 0, fmt.Errorf("query attributes: %w", err)
	}
	attributes, err := scanAttributes(rows)
	if err != nil {
		return nil, 0, err
	}
	return attributes, total, nil
}

// ListAttributesForPerson returns all attributes of a person within the branch overlay.
func (s *ReadModelStore) ListAttributesForPerson(ctx context.Context, branchID domain.BranchID, personID uuid.UUID) ([]repository.AttributeReadModel, error) {
	sub, args := factOverlaySubquery("attributes", personAttributesFilter, []any{personID.String()}, branchID)
	// #nosec G202 -- sub is built from package constants; every value is a bound ? placeholder
	// nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+attributeSelectCols+` FROM `+sub+` a
		ORDER BY fact_type ASC, value ASC, id ASC
	`, args...)
	if err != nil {
		return nil, fmt.Errorf("query attributes for person: %w", err)
	}
	return scanAttributes(rows)
}

// SaveAttribute saves or updates an attribute on the given branch (ADR-005). A
// save always clears any prior tombstone.
func (s *ReadModelStore) SaveAttribute(ctx context.Context, branchID domain.BranchID, attribute *repository.AttributeReadModel) error {
	if err := s.guardBranchWrite(branchID); err != nil {
		return err
	}

	var dateSort interface{}
	if attribute.DateSort != nil {
		dateSort = attribute.DateSort.Format(time.RFC3339)
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO attributes (id, branch_id, person_id, fact_type, value, date_raw, date_sort, place, version, created_at, deleted)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0)
		ON CONFLICT (id, branch_id) DO UPDATE SET
			person_id = excluded.person_id,
			fact_type = excluded.fact_type,
			value = excluded.value,
			date_raw = excluded.date_raw,
			date_sort = excluded.date_sort,
			place = excluded.place,
			version = excluded.version,
			deleted = 0
	`, attribute.ID.String(), branchID.String(), attribute.PersonID.String(), string(attribute.FactType),
		attribute.Value, attribute.DateRaw, dateSort, attribute.Place,
		attribute.Version, formatTimestamp(attribute.CreatedAt))
	if err != nil {
		return fmt.Errorf("save attribute: %w", err)
	}
	return nil
}

// DeleteAttribute removes an attribute (ADR-005): a real removal on main, a
// tombstone on a non-main branch.
func (s *ReadModelStore) DeleteAttribute(ctx context.Context, branchID domain.BranchID, id uuid.UUID) error {
	if err := s.deleteFactRow(ctx, "attributes", attributeSelectCols, branchID, id); err != nil {
		return fmt.Errorf("delete attribute: %w", err)
	}
	return nil
}

// Scanning functions for sources and citations

func scanSource(row rowScanner) (*repository.SourceReadModel, error) {
	var (
		idStr, sourceType, title                                    string
		author, publisher, publishDateRaw, publishDateSort          sql.NullString
		url, repoID, repoName, collName, callNum, notes, gedcomXref sql.NullString
		citationCount                                               int
		version                                                     int64
		updatedAt                                                   string
	)

	err := row.Scan(&idStr, &sourceType, &title, &author, &publisher, &publishDateRaw, &publishDateSort,
		&url, &repoID, &repoName, &collName, &callNum, &notes, &gedcomXref,
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

	if repoID.Valid {
		if rid, err := uuid.Parse(repoID.String); err == nil {
			src.RepositoryID = &rid
		}
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
		quotedText, analysis, templateID, fieldsData, gedcomXref  sql.NullString
		version                                                   int64
		createdAt                                                 string
	)

	err := row.Scan(&idStr, &sourceIDStr, &sourceTitle, &factType, &factOwnerIDStr,
		&page, &volume, &sourceQuality, &informantType, &evidenceType,
		&quotedText, &analysis, &templateID, &fieldsData, &gedcomXref, &version, &createdAt)

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
		FieldsJSON:    fieldsData.String,
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
		isNegated                              bool
		version                                int64
		createdAt                              string
	)

	err := row.Scan(&idStr, &ownerType, &ownerIDStr, &factType, &dateRaw, &dateSort,
		&place, &placeLat, &placeLong, &addressJSON, &description, &cause,
		&age, &researchStatus, &isNegated, &version, &createdAt)

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
		IsNegated:   isNegated,
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

// Media (#759) is branch-scoped for its METADATA only; the file bytes are
// shared. See the blob rule on repository.ReadModelStore: the bytes live on the
// item's origin row (the row MediaCreated wrote), a branch shadow row of a
// mainline item holds NULL bytes, and the byte reads fall back from the winning
// row to the main row. Every media read resolves the overlay over
// mediaSelectCols, which names no byte column, so the ROW_NUMBER window never
// carries a blob and GetMedia and ListMediaForEntity never read one.

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
	// its bytes (o ranks the rows by (id, branch_id) alone), joins it back as w,
	// and joins main's row as m only when the winner is not main's own row, so
	// the byte columns can COALESCE from the winner to main. Bind
	// mediaWinnerArgs.
	mediaWinnerJoin = `FROM (
			SELECT id, branch_id, deleted,
				ROW_NUMBER() OVER (PARTITION BY id ORDER BY (branch_id = ?) DESC) AS rn
			FROM media WHERE id = ? AND branch_id IN (?, ?)
		) o
		JOIN media w ON w.id = o.id AND w.branch_id = o.branch_id
		LEFT JOIN media m ON m.id = o.id AND m.branch_id = ? AND o.branch_id <> ?
		WHERE o.rn = 1 AND o.deleted = 0`

	// Per-owner media filters; each binds the owner id once.
	personMediaFilter = `entity_type = 'person' AND entity_id = ?`
	familyMediaFilter = `entity_type = 'family' AND entity_id = ?`
	sourceMediaFilter = `entity_type = 'source' AND entity_id = ?`
	// entityMediaFilter backs ListMediaForEntity; it binds (entity type, entity id).
	entityMediaFilter = `entity_type = ? AND entity_id = ?`
	// mediaIDFilter selects one media id; it binds the id.
	mediaIDFilter = `id = ?`

	// mediaLiveShadow holds when a non-main branch still shows the main row m
	// through a live shadow row; it binds main.
	mediaLiveShadow = `EXISTS (SELECT 1 FROM media b WHERE b.id = m.id AND b.branch_id <> ? AND b.deleted = 0)`

	// Main-tombstone collection after a branch-side delete: main rows kept alive
	// only for a shadow the branch has now tombstoned. Binds (main, branch, main).
	gcMainMediaAfterDelete = `
		DELETE FROM media AS m
		WHERE m.branch_id = ? AND m.deleted = 1
		  AND m.id IN (SELECT id FROM media WHERE branch_id = ? AND deleted = 1)
		  AND NOT EXISTS (SELECT 1 FROM media b WHERE b.id = m.id AND b.branch_id <> ? AND b.deleted = 0)`

	// Main-tombstone collection before PurgeBranch drops the branch's rows: main
	// rows kept alive only for this branch's shadows. Binds (main, branch,
	// branch, main).
	gcMainMediaBeforePurge = `
		DELETE FROM media AS m
		WHERE m.branch_id = ? AND m.deleted = 1
		  AND m.id IN (SELECT id FROM media WHERE branch_id = ?)
		  AND NOT EXISTS (SELECT 1 FROM media b WHERE b.id = m.id AND b.branch_id NOT IN (?, ?) AND b.deleted = 0)`
)

// mediaWinnerArgs are the args mediaWinnerJoin binds.
func mediaWinnerArgs(branchID domain.BranchID, id uuid.UUID) []any {
	branch := branchID.String()
	return []any{branch, id.String(), branch, mainBranchID, mainBranchID, mainBranchID}
}

// GetMedia retrieves media metadata by ID within the branch overlay (ADR-005,
// #759). It never reads the file bytes.
func (s *ReadModelStore) GetMedia(ctx context.Context, branchID domain.BranchID, id uuid.UUID) (*repository.MediaReadModel, error) {
	sub, args := overlayColsSubquery("media", mediaSelectCols, mediaIDFilter, []any{id.String()}, branchID)
	// #nosec G202 -- sub is built from package constants; every value is a bound ? placeholder
	// nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query
	row := s.db.QueryRowContext(ctx, "SELECT "+mediaSelectCols+" FROM "+sub+" md", args...)
	return scanMediaRow(row, false)
}

// GetMediaWithData retrieves the full media record within the branch overlay:
// the winning row's metadata, and the shared bytes from the winning row else the
// main row (#759).
func (s *ReadModelStore) GetMediaWithData(ctx context.Context, branchID domain.BranchID, id uuid.UUID) (*repository.MediaReadModel, error) {
	// #nosec G202 -- the query is assembled from package constants; every value is a bound ? placeholder
	// nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query
	row := s.db.QueryRowContext(ctx, `SELECT `+mediaWinnerCols+`,
			COALESCE(w.file_data, m.file_data), COALESCE(w.thumbnail_data, m.thumbnail_data)
		`+mediaWinnerJoin, mediaWinnerArgs(branchID, id)...)
	return scanMediaRow(row, true)
}

// GetMediaThumbnail retrieves just the thumbnail bytes of a media item visible
// on branchID, read from the winning row else the main row (#759). It returns
// nil when the item is absent or tombstoned on the branch.
func (s *ReadModelStore) GetMediaThumbnail(ctx context.Context, branchID domain.BranchID, id uuid.UUID) ([]byte, error) {
	var thumbnail []byte
	// #nosec G202 -- the query is assembled from package constants; every value is a bound ? placeholder
	// nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query
	err := s.db.QueryRowContext(ctx, `SELECT COALESCE(w.thumbnail_data, m.thumbnail_data) `+mediaWinnerJoin,
		mediaWinnerArgs(branchID, id)...).Scan(&thumbnail)
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
	sub, args := overlayColsSubquery("media", mediaSelectCols, entityMediaFilter,
		[]any{entityType, entityID.String()}, opts.BranchID)

	var total int
	// #nosec G202 -- sub is built from package constants; every value is a bound ? placeholder
	// nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+sub+" md", args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count media: %w", err)
	}

	// #nosec G202 -- sub is built from package constants; every value is a bound ? placeholder
	// nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+mediaSelectCols+`
		FROM `+sub+` md
		ORDER BY created_at DESC, id DESC
		LIMIT ? OFFSET ?
	`, append(args, opts.Limit, opts.Offset)...)
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
// deleted meanwhile) inserts nothing, so it can never leave a shadow with no
// bytes; being one statement, and SQLite serializing writers, that check cannot
// interleave with a concurrent delete. (The COALESCE with the row's own stored
// bytes keeps a pre-#759 database, whose file_data is still NOT NULL, writable
// for a mainline metadata edit.)
func (s *ReadModelStore) SaveMedia(ctx context.Context, branchID domain.BranchID, media *repository.MediaReadModel) error {
	if err := s.guardBranchWrite(branchID); err != nil {
		return err
	}
	// Serialize JSON fields
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

	// Numbered parameters: ?1 id, ?2 branch, ?3 main, ?4 file bytes, ?5 thumbnail
	// bytes, then the metadata columns in order.
	_, err = tx.ExecContext(ctx, `
		INSERT INTO media (id, branch_id, file_data, thumbnail_data,
						  entity_type, entity_id, title, description, mime_type, media_type,
						  filename, file_size,
						  crop_left, crop_top, crop_width, crop_height,
						  gedcom_xref, version, created_at, updated_at,
						  files, format, translations, deleted)
		SELECT ?1, ?2,
			CASE WHEN ?2 <> ?3 AND EXISTS (SELECT 1 FROM media WHERE id = ?1 AND branch_id = ?3) THEN NULL
				ELSE COALESCE(?4, (SELECT file_data FROM media WHERE id = ?1 AND branch_id = ?2)) END,
			CASE WHEN ?2 <> ?3 AND EXISTS (SELECT 1 FROM media WHERE id = ?1 AND branch_id = ?3) THEN NULL
				ELSE ?5 END,
			?6, ?7, ?8, ?9, ?10, ?11, ?12, ?13, ?14, ?15, ?16, ?17, ?18, ?19, ?20, ?21, ?22, ?23, ?24, 0
		WHERE NOT (?2 <> ?3 AND ?4 IS NULL
			AND NOT EXISTS (SELECT 1 FROM media WHERE id = ?1 AND branch_id IN (?2, ?3)))
		ON CONFLICT(id, branch_id) DO UPDATE SET
			entity_type = excluded.entity_type,
			entity_id = excluded.entity_id,
			title = excluded.title,
			description = excluded.description,
			mime_type = excluded.mime_type,
			media_type = excluded.media_type,
			filename = excluded.filename,
			file_size = excluded.file_size,
			file_data = COALESCE(excluded.file_data, media.file_data),
			thumbnail_data = COALESCE(excluded.thumbnail_data, media.thumbnail_data),
			crop_left = excluded.crop_left,
			crop_top = excluded.crop_top,
			crop_width = excluded.crop_width,
			crop_height = excluded.crop_height,
			gedcom_xref = excluded.gedcom_xref,
			version = excluded.version,
			updated_at = excluded.updated_at,
			files = excluded.files,
			format = excluded.format,
			translations = excluded.translations,
			deleted = 0
	`, media.ID.String(), branchID.String(), mainBranchID,
		nullableBlob(media.FileData), nullableBlob(media.ThumbnailData),
		media.EntityType, media.EntityID.String(), media.Title,
		nullableString(media.Description), media.MimeType, string(media.MediaType),
		media.Filename, media.FileSize,
		nullableInt(media.CropLeft), nullableInt(media.CropTop),
		nullableInt(media.CropWidth), nullableInt(media.CropHeight),
		nullableString(media.GedcomXref), media.Version,
		formatTimestamp(media.CreatedAt), formatTimestamp(media.UpdatedAt),
		nullableBytes(filesJSON), nullableString(media.Format), nullableBytes(translationsJSON))
	if err != nil {
		return fmt.Errorf("save media: %w", err)
	}
	if branchID.IsMain() {
		// A branch upload merged into main: main's row now holds the bytes, so
		// the branch's origin row becomes a shadow and drops its copy (#759).
		if _, err := tx.ExecContext(ctx, releaseBranchMediaBytes, media.ID.String(), mainBranchID); err != nil {
			return fmt.Errorf("release branch media bytes: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit save media: %w", err)
	}
	return nil
}

// releaseBranchMediaBytes clears, on every non-main row of media ?1, each byte
// column main's row (?2) now also holds, so an item's bytes are stored exactly
// once (ADR-005, #759).
const releaseBranchMediaBytes = `UPDATE media SET
		file_data = CASE WHEN EXISTS (SELECT 1 FROM media m WHERE m.id = ?1 AND m.branch_id = ?2 AND m.file_data IS NOT NULL)
			THEN NULL ELSE file_data END,
		thumbnail_data = CASE WHEN EXISTS (SELECT 1 FROM media m WHERE m.id = ?1 AND m.branch_id = ?2 AND m.thumbnail_data IS NOT NULL)
			THEN NULL ELSE thumbnail_data END
	WHERE id = ?1 AND branch_id <> ?2 AND (file_data IS NOT NULL OR thumbnail_data IS NOT NULL)`

// DeleteMedia removes a media item on the given branch (ADR-005, #759). On a
// non-main branch it writes a metadata-only tombstone and never touches main's
// row or bytes. On main it is a real removal — unless a branch still shows the
// item through a live shadow row, in which case main's row is kept as a
// tombstone so that shadow keeps its bytes.
func (s *ReadModelStore) DeleteMedia(ctx context.Context, branchID domain.BranchID, id uuid.UUID) error {
	if err := s.guardBranchWrite(branchID); err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := cascadeMedia(ctx, tx, mediaIDFilter, branchID, id); err != nil {
		return fmt.Errorf("delete media: %w", err)
	}
	return tx.Commit()
}

// cascadeMedia removes, on branchID, every media row filter selects for value —
// one id for DeleteMedia, or every item attached to an owner for the manual
// cascade DeletePerson/DeleteFamily/DeleteSource run (media never had a foreign
// key to its owner; #759). Off main each item the branch sees is tombstoned
// (metadata only), then any main tombstone no live shadow needs any more is
// dropped; on main the rows go through deleteMainMedia so a live branch shadow
// keeps its bytes. filter must be a package constant binding value once.
func cascadeMedia(ctx context.Context, tx *sql.Tx, filter string, branchID domain.BranchID, value uuid.UUID) error {
	if branchID.IsMain() {
		return deleteMainMedia(ctx, tx, filter, value)
	}
	sub, args := overlayColsSubquery("media", mediaSelectCols, filter, []any{value.String()}, branchID)
	if err := tombstoneFactRows(ctx, tx, "media", mediaSelectCols, branchID, sub, args); err != nil {
		return fmt.Errorf("tombstone media: %w", err)
	}
	return gcMainMedia(ctx, tx, gcMainMediaAfterDelete, mainBranchID, branchID.String(), mainBranchID)
}

// deleteMainMedia removes the main rows filter selects for value: each is
// hard-deleted, except that one a non-main branch still shows through a live
// shadow row becomes a main tombstone instead, keeping the bytes that shadow
// borrows (#759). filter must be a package constant binding value once.
func deleteMainMedia(ctx context.Context, tx *sql.Tx, filter string, value uuid.UUID) error {
	// #nosec G202 -- filter and mediaLiveShadow are package constants; every value is a bound ? placeholder
	// nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query
	if _, err := tx.ExecContext(ctx, `UPDATE media AS m SET deleted = 1
		WHERE m.branch_id = ? AND m.deleted = 0 AND `+filter+` AND `+mediaLiveShadow,
		mainBranchID, value.String(), mainBranchID); err != nil {
		return fmt.Errorf("tombstone shared media on main: %w", err)
	}
	// #nosec G202 -- filter and mediaLiveShadow are package constants; every value is a bound ? placeholder
	// nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query
	if _, err := tx.ExecContext(ctx, `DELETE FROM media AS m
		WHERE m.branch_id = ? AND `+filter+` AND NOT `+mediaLiveShadow,
		mainBranchID, value.String(), mainBranchID); err != nil {
		return fmt.Errorf("delete media on main: %w", err)
	}
	return nil
}

// gcMainMedia drops the main media tombstones that no live branch shadow needs
// any more; stmt is gcMainMediaAfterDelete or gcMainMediaBeforePurge.
func gcMainMedia(ctx context.Context, db sqlExecer, stmt string, args ...any) error {
	if _, err := db.ExecContext(ctx, stmt, args...); err != nil {
		return fmt.Errorf("collect main media tombstones: %w", err)
	}
	return nil
}

// scanMediaRow scans one mediaSelectCols row, followed by the file and
// thumbnail bytes when withData is set. It returns (nil, nil) for
// sql.ErrNoRows so single-row lookups report absence as nil.
func scanMediaRow(row rowScanner, withData bool) (*repository.MediaReadModel, error) {
	var (
		m                             repository.MediaReadModel
		idStr, entityIDStr, mediaType string
		description, gedcomXref       sql.NullString
		cropLeft, cropTop             sql.NullInt64
		cropWidth, cropHeight         sql.NullInt64
		createdAt, updatedAt          string
		filesJSON, translationsJSON   sql.NullString
		format                        sql.NullString
	)

	dest := []any{&idStr, &m.EntityType, &entityIDStr, &m.Title, &description,
		&m.MimeType, &mediaType, &m.Filename, &m.FileSize,
		&cropLeft, &cropTop, &cropWidth, &cropHeight,
		&gedcomXref, &m.Version, &createdAt, &updatedAt,
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

	if m.ID, err = uuid.Parse(idStr); err != nil {
		return nil, fmt.Errorf("parse media id %q: %w", idStr, err)
	}
	if m.EntityID, err = uuid.Parse(entityIDStr); err != nil {
		return nil, fmt.Errorf("parse media entity id %q: %w", entityIDStr, err)
	}

	// Deserialize JSON fields
	if m.Files, err = domain.UnmarshalFilesFromJSON([]byte(filesJSON.String)); err != nil {
		return nil, fmt.Errorf("unmarshal files: %w", err)
	}
	if m.Translations, err = domain.UnmarshalTranslationsFromJSON([]byte(translationsJSON.String)); err != nil {
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
	if t, err := parseTimestamp(createdAt); err == nil {
		m.CreatedAt = t
	}
	if t, err := parseTimestamp(updatedAt); err == nil {
		m.UpdatedAt = t
	}
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

// GetSurnameIndex returns unique surnames with counts and letter counts within
// the branch overlay (ADR-005 / #756): a branch sees its own shadow rows in place
// of main's and never counts a tombstoned person.
func (s *ReadModelStore) GetSurnameIndex(ctx context.Context, branchID domain.BranchID) ([]repository.SurnameEntry, []repository.LetterCount, error) {
	overlay, overlayArgs := personOverlaySubquery(branchID)

	// Get surname counts
	// #nosec G202 -- overlay is one of two constant subquery literals returned by personOverlaySubquery; every value is a bound ? placeholder
	rows, err := s.db.QueryContext(ctx, `
		SELECT surname, COUNT(*) as count
		FROM `+overlay+`
		GROUP BY surname
		ORDER BY surname ASC
	`, overlayArgs...)
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
	// #nosec G202 -- overlay is one of two constant subquery literals returned by personOverlaySubquery; every value is a bound ? placeholder
	letterRows, err := s.db.QueryContext(ctx, `
		SELECT UPPER(SUBSTR(surname, 1, 1)) as letter, COUNT(DISTINCT surname) as count
		FROM `+overlay+`
		WHERE surname != ''
		GROUP BY UPPER(SUBSTR(surname, 1, 1))
		ORDER BY letter ASC
	`, overlayArgs...)
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

// GetSurnamesByLetter returns surnames starting with a specific letter within the
// branch overlay (ADR-005 / #756).
func (s *ReadModelStore) GetSurnamesByLetter(ctx context.Context, branchID domain.BranchID, letter string) ([]repository.SurnameEntry, error) {
	overlay, overlayArgs := personOverlaySubquery(branchID)

	args := append(append([]any{}, overlayArgs...), letter)
	// #nosec G202 -- overlay is one of two constant subquery literals returned by personOverlaySubquery; every value is a bound ? placeholder
	rows, err := s.db.QueryContext(ctx, `
		SELECT surname, COUNT(*) as count
		FROM `+overlay+`
		WHERE UPPER(SUBSTR(surname, 1, 1)) = UPPER(?)
		GROUP BY surname
		ORDER BY surname ASC
	`, args...)
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

// GetPersonsBySurname returns persons with a specific surname within the branch
// overlay carried on opts.BranchID (ADR-005 / #756). Count and page resolve the
// same overlay so they always agree on scope.
func (s *ReadModelStore) GetPersonsBySurname(ctx context.Context, surname string, opts repository.ListOptions) ([]repository.PersonReadModel, int, error) {
	overlay, overlayArgs := personOverlaySubquery(opts.BranchID)

	// Count total
	var total int
	countArgs := append(append([]any{}, overlayArgs...), surname)
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+overlay+" WHERE LOWER(surname) = LOWER(?)", countArgs...).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("count persons by surname: %w", err)
	}

	queryArgs := append(append([]any{}, overlayArgs...), surname, opts.Limit, opts.Offset)
	// #nosec G202 -- overlay is one of two constant subquery literals returned by personOverlaySubquery; every value is a bound ? placeholder
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, given_name, surname, full_name, gender,
			   birth_date_raw, birth_date_sort, birth_place, birth_place_lat, birth_place_long,
			   death_date_raw, death_date_sort, death_place, death_place_lat, death_place_long,
			   notes, research_status, brick_wall_note, brick_wall_since, brick_wall_resolved_at,
			   version, updated_at
		FROM `+overlay+`
		WHERE LOWER(surname) = LOWER(?)
		ORDER BY given_name ASC
		LIMIT ? OFFSET ?
	`, queryArgs...)
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
// Scoped to the branch overlay (ADR-005 / #756): the birth-place and death-place
// legs of each UNION both read the overlay, so a branch sees its own places and
// tombstoned persons contribute none.
func (s *ReadModelStore) GetPlaceHierarchy(ctx context.Context, branchID domain.BranchID, parent string) ([]repository.PlaceEntry, error) {
	var rows *sql.Rows
	var err error

	withClause, src, cteArgs, legArgs := personOverlayCTE(branchID)

	if parent == "" {
		// Top-level: get unique countries/top-level places (rightmost part after last comma)
		// Args in statement order: the CTE's binds (off main), then each leg's own
		// overlay binds (main only, where the subquery is still inlined per leg).
		args := append(append([]any{}, cteArgs...), legArgs...)
		args = append(args, legArgs...)
		// #nosec G202 -- withClause/src are built from the constant literals returned by personOverlayCTE; every value is a bound ? placeholder
		rows, err = s.db.QueryContext(ctx, `
			`+withClause+`all_places AS (
				SELECT DISTINCT birth_place as place FROM `+src+` WHERE birth_place != '' AND birth_place IS NOT NULL
				UNION
				SELECT DISTINCT death_place as place FROM `+src+` WHERE death_place != '' AND death_place IS NOT NULL
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
		`, args...)
	} else {
		// Child level: get places that end with parent. Args in statement order: the
		// CTE's binds (off main), then per leg its own overlay binds (main only) plus
		// that leg's parent bind, then the four remaining parent binds.
		args := append(append([]any{}, cteArgs...), legArgs...)
		args = append(args, parent)
		args = append(args, legArgs...)
		args = append(args, parent, parent, parent, parent, parent)
		// #nosec G202 -- withClause/src are built from the constant literals returned by personOverlayCTE; every value is a bound ? placeholder
		rows, err = s.db.QueryContext(ctx, `
			`+withClause+`all_places AS (
				SELECT DISTINCT birth_place as place FROM `+src+` WHERE birth_place LIKE '%' || ? AND birth_place != ''
				UNION
				SELECT DISTINCT death_place as place FROM `+src+` WHERE death_place LIKE '%' || ? AND death_place != ''
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
		`, args...)
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

// GetPersonsByPlace returns persons associated with a place within the branch
// overlay carried on opts.BranchID (ADR-005 / #756).
func (s *ReadModelStore) GetPersonsByPlace(ctx context.Context, place string, opts repository.ListOptions) ([]repository.PersonReadModel, int, error) {
	overlay, overlayArgs := personOverlaySubquery(opts.BranchID)

	// Count total - match place at any position in birth_place or death_place
	var total int
	countArgs := append(append([]any{}, overlayArgs...), place, place)
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM `+overlay+`
		WHERE birth_place LIKE '%' || ? || '%' OR death_place LIKE '%' || ? || '%'
	`, countArgs...).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("count persons by place: %w", err)
	}

	queryArgs := append(append([]any{}, overlayArgs...), place, place, opts.Limit, opts.Offset)
	// #nosec G202 -- overlay is one of two constant subquery literals returned by personOverlaySubquery; every value is a bound ? placeholder
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, given_name, surname, full_name, gender,
			   birth_date_raw, birth_date_sort, birth_place, birth_place_lat, birth_place_long,
			   death_date_raw, death_date_sort, death_place, death_place_lat, death_place_long,
			   notes, research_status, brick_wall_note, brick_wall_since, brick_wall_resolved_at,
			   version, updated_at
		FROM `+overlay+`
		WHERE birth_place LIKE '%' || ? || '%' OR death_place LIKE '%' || ? || '%'
		ORDER BY surname ASC, given_name ASC
		LIMIT ? OFFSET ?
	`, queryArgs...)
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

// GetCemeteryIndex returns unique burial/cremation places with person counts,
// counted over branchID's overlay of life events (ADR-005, #757). A branch that
// tombstoned a person also tombstoned that person's life events (DeletePerson's
// cascade), so the counts agree with GetPersonsByCemetery on the same scope.
func (s *ReadModelStore) GetCemeteryIndex(ctx context.Context, branchID domain.BranchID) ([]repository.CemeteryEntry, error) {
	sub, args := factOverlaySubquery("life_events", "fact_type IN (?, ?)",
		[]any{string(domain.FactPersonBurial), string(domain.FactPersonCremation)}, branchID)
	// #nosec G202 -- sub is built from package constants; every value is a bound ? placeholder
	// nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query
	rows, err := s.db.QueryContext(ctx, `
		SELECT place, COUNT(DISTINCT owner_id) as count
		FROM `+sub+` e
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
// returned are the branch's view of them.
func (s *ReadModelStore) GetPersonsByCemetery(ctx context.Context, place string, opts repository.ListOptions) ([]repository.PersonReadModel, int, error) {
	overlay, overlayArgs := personOverlaySubquery(opts.BranchID)
	events, eventArgs := factOverlaySubquery("life_events", "fact_type IN (?, ?) AND LOWER(place) = LOWER(?)",
		[]any{string(domain.FactPersonBurial), string(domain.FactPersonCremation), place}, opts.BranchID)
	joinArgs := append(append([]any{}, overlayArgs...), eventArgs...)

	// Count total distinct persons
	var total int
	// #nosec G202 -- overlay/events are built from package constants; every value is a bound ? placeholder
	// nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(DISTINCT p.id)
		FROM `+overlay+` p
		INNER JOIN `+events+` e ON e.owner_id = p.id
	`, joinArgs...).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("count persons by cemetery: %w", err)
	}

	// #nosec G202 -- overlay/events are built from package constants; every value is a bound ? placeholder
	// nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query
	rows, err := s.db.QueryContext(ctx, `
		SELECT DISTINCT p.id, p.given_name, p.surname, p.full_name, p.gender,
			   p.birth_date_raw, p.birth_date_sort, p.birth_place, p.birth_place_lat, p.birth_place_long,
			   p.death_date_raw, p.death_date_sort, p.death_place, p.death_place_lat, p.death_place_long,
			   p.notes, p.research_status, p.brick_wall_note, p.brick_wall_since, p.brick_wall_resolved_at,
			   p.version, p.updated_at
		FROM `+overlay+` p
		INNER JOIN `+events+` e ON e.owner_id = p.id
		ORDER BY p.surname ASC, p.given_name ASC
		LIMIT ? OFFSET ?
	`, append(joinArgs, opts.Limit, opts.Offset)...)
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
// coordinates within the branch overlay (ADR-005 / #756).
func (s *ReadModelStore) GetMapLocations(ctx context.Context, branchID domain.BranchID) ([]repository.MapLocation, error) {
	overlay, overlayArgs := personOverlaySubquery(branchID)

	// Query birth locations
	// #nosec G202 -- overlay is one of two constant subquery literals returned by personOverlaySubquery; every value is a bound ? placeholder
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, birth_place, birth_place_lat, birth_place_long
		FROM `+overlay+`
		WHERE birth_place_lat IS NOT NULL AND birth_place_long IS NOT NULL
		  AND birth_place_lat != '' AND birth_place_long != ''
		ORDER BY birth_place ASC
	`, overlayArgs...)
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
		var idStr, place, latStr, lonStr string
		if err := rows.Scan(&idStr, &place, &latStr, &lonStr); err != nil {
			return nil, fmt.Errorf("scan birth map location: %w", err)
		}
		personID, err := uuid.Parse(idStr)
		if err != nil {
			continue
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
	// #nosec G202 -- overlay is one of two constant subquery literals returned by personOverlaySubquery; every value is a bound ? placeholder
	rows2, err := s.db.QueryContext(ctx, `
		SELECT id, death_place, death_place_lat, death_place_long
		FROM `+overlay+`
		WHERE death_place_lat IS NOT NULL AND death_place_long IS NOT NULL
		  AND death_place_lat != '' AND death_place_long != ''
		ORDER BY death_place ASC
	`, overlayArgs...)
	if err != nil {
		return nil, fmt.Errorf("query death map locations: %w", err)
	}
	defer rows2.Close()

	for rows2.Next() {
		var idStr, place, latStr, lonStr string
		if err := rows2.Scan(&idStr, &place, &latStr, &lonStr); err != nil {
			return nil, fmt.Errorf("scan death map location: %w", err)
		}
		personID, err := uuid.Parse(idStr)
		if err != nil {
			continue
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

// Brick-wall state is written straight to the read model rather than projected from
// events, so there is no overlay to resolve. All three methods are MAIN-ONLY and say
// so with an explicit branch_id predicate: without it the UPDATEs would rewrite every
// branch's shadow row for the person and the SELECT would list shadows and tombstones
// as extra people (BR-003). Whether brick walls should become branch-aware at all waits
// on deciding whether they become event-sourced, as #624 did for snapshots (ADR-005,
// "Entities that stay main-only").

// SetBrickWall marks a person as a brick wall with a note (main only).
func (s *ReadModelStore) SetBrickWall(ctx context.Context, personID uuid.UUID, note string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE persons SET brick_wall_note = ?, brick_wall_since = ?, brick_wall_resolved_at = NULL
		WHERE id = ? AND branch_id = ?
	`, note, formatTimestamp(time.Now()), personID.String(), mainBranchID)
	return err
}

// ResolveBrickWall marks a brick wall as resolved (main only).
func (s *ReadModelStore) ResolveBrickWall(ctx context.Context, personID uuid.UUID) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE persons SET brick_wall_resolved_at = ?
		WHERE id = ? AND branch_id = ?
	`, formatTimestamp(time.Now()), personID.String(), mainBranchID)
	return err
}

// GetBrickWalls returns persons with brick wall status (main only).
func (s *ReadModelStore) GetBrickWalls(ctx context.Context, includeResolved bool) ([]repository.BrickWallEntry, error) {
	query := `
		SELECT id, full_name, brick_wall_note, brick_wall_since, brick_wall_resolved_at
		FROM persons
		WHERE branch_id = ? AND deleted = 0
		  AND brick_wall_since IS NOT NULL AND brick_wall_since != ''`
	if !includeResolved {
		query += ` AND (brick_wall_resolved_at IS NULL OR brick_wall_resolved_at = '')`
	}
	query += ` ORDER BY brick_wall_since DESC`

	rows, err := s.db.QueryContext(ctx, query, mainBranchID)
	if err != nil {
		return nil, fmt.Errorf("query brick walls: %w", err)
	}
	defer rows.Close()

	var entries []repository.BrickWallEntry
	for rows.Next() {
		var (
			idStr, fullName string
			note            sql.NullString
			sinceStr        string
			resolvedAtStr   sql.NullString
		)
		if err := rows.Scan(&idStr, &fullName, &note, &sinceStr, &resolvedAtStr); err != nil {
			return nil, fmt.Errorf("scan brick wall: %w", err)
		}
		id, _ := uuid.Parse(idStr)
		since, _ := parseTimestamp(sinceStr)
		entry := repository.BrickWallEntry{
			PersonID:   id,
			PersonName: fullName,
			Note:       note.String,
			Since:      since,
		}
		if resolvedAtStr.Valid && resolvedAtStr.String != "" {
			if t, err := parseTimestamp(resolvedAtStr.String); err == nil {
				entry.ResolvedAt = &t
			}
		}
		entries = append(entries, entry)
	}

	return entries, rows.Err()
}

// scanNote scans one noteSelectCols row. It returns (nil, nil) for
// sql.ErrNoRows so single-row lookups report absence as nil.
func scanNote(row rowScanner) (*repository.NoteReadModel, error) {
	var note repository.NoteReadModel
	var idStr string
	var mime, language, translations, gedcomXref sql.NullString
	var updatedAtStr string

	err := row.Scan(
		&idStr,
		&note.Text,
		&mime,
		&language,
		&translations,
		&gedcomXref,
		&note.Version,
		&updatedAtStr,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan note: %w", err)
	}

	if note.ID, err = uuid.Parse(idStr); err != nil {
		return nil, fmt.Errorf("parse note id: %w", err)
	}
	note.MIME = mime.String
	note.Language = language.String
	note.Translations = repository.UnmarshalNoteTranslations(translations.String)
	note.GedcomXref = gedcomXref.String
	if t, err := parseTimestamp(updatedAtStr); err == nil {
		note.UpdatedAt = t
	}
	return &note, nil
}

// GetNote retrieves a note by ID within the branch overlay (ADR-005, #758).
func (s *ReadModelStore) GetNote(ctx context.Context, branchID domain.BranchID, id uuid.UUID) (*repository.NoteReadModel, error) {
	// #nosec G202 -- the query is built by factGetQuery from package constants; every value is a bound placeholder
	// nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query
	row := s.db.QueryRowContext(ctx, factGetQuery("notes", noteSelectCols), factGetArgs(branchID, id)...)
	return scanNote(row)
}

// ListNotes returns a paginated list of the notes visible on opts.BranchID
// (ADR-005, #758).
func (s *ReadModelStore) ListNotes(ctx context.Context, opts repository.ListOptions) ([]repository.NoteReadModel, int, error) {
	sub, args := factOverlaySubquery("notes", "", nil, opts.BranchID)

	var total int
	// #nosec G202 -- sub is built from package constants; every value is a bound ? placeholder
	// nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+sub+" n", args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count notes: %w", err)
	}

	// Build order clause
	orderDir := "DESC"
	if opts.Order == "asc" {
		orderDir = "ASC"
	}

	// #nosec G201 G202 -- orderDir is one of two literals chosen above; sub is built from package constants
	// nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query
	query := fmt.Sprintf(`
		SELECT `+noteSelectCols+`
		FROM %s n
		ORDER BY updated_at %s, id %s
		LIMIT ? OFFSET ?
	`, sub, orderDir, orderDir)

	rows, err := s.db.QueryContext(ctx, query, append(args, opts.Limit, opts.Offset)...)
	if err != nil {
		return nil, 0, fmt.Errorf("query notes: %w", err)
	}
	defer rows.Close()

	var notes []repository.NoteReadModel
	for rows.Next() {
		note, err := scanNote(rows)
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
	if err := s.guardBranchWrite(branchID); err != nil {
		return err
	}
	var gedcomXref any
	if note.GedcomXref != "" {
		gedcomXref = note.GedcomXref
	}
	var mime any
	if note.MIME != "" {
		mime = note.MIME
	}
	var language any
	if note.Language != "" {
		language = note.Language
	}
	var translations any
	if len(note.Translations) > 0 {
		translations = repository.MarshalNoteTranslations(note.Translations)
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO notes (id, branch_id, text, mime, language, translations, gedcom_xref, version, updated_at, deleted)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 0)
		ON CONFLICT (id, branch_id) DO UPDATE SET
			text = excluded.text,
			mime = excluded.mime,
			language = excluded.language,
			translations = excluded.translations,
			gedcom_xref = excluded.gedcom_xref,
			version = excluded.version,
			updated_at = excluded.updated_at,
			deleted = 0
	`, note.ID.String(), branchID.String(), note.Text, mime, language, translations, gedcomXref, note.Version, note.UpdatedAt.Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("save note: %w", err)
	}
	return nil
}

// DeleteNote removes a note (ADR-005, #758): a real removal on main, a tombstone
// on a non-main branch.
func (s *ReadModelStore) DeleteNote(ctx context.Context, branchID domain.BranchID, id uuid.UUID) error {
	if err := s.deleteFactRow(ctx, "notes", noteSelectCols, branchID, id); err != nil {
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

// GetRepository retrieves a repository by ID.
func (s *ReadModelStore) GetRepository(ctx context.Context, id uuid.UUID) (*repository.RepositoryReadModel, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, name, address, notes, gedcom_xref, version, updated_at
		FROM repositories WHERE id = ?
	`, id.String())

	var repo repository.RepositoryReadModel
	var idStr string
	var addressJSON []byte
	var notes, gedcomXref sql.NullString
	var updatedAtStr string

	err := row.Scan(
		&idStr,
		&repo.Name,
		&addressJSON,
		&notes,
		&gedcomXref,
		&repo.Version,
		&updatedAtStr,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan repository: %w", err)
	}

	repo.ID, _ = uuid.Parse(idStr)
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
	if t, err := parseTimestamp(updatedAtStr); err == nil {
		repo.UpdatedAt = t
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
		LIMIT ? OFFSET ?
	`, orderColumn, orderDir, orderDir)

	rows, err := s.db.QueryContext(ctx, query, opts.Limit, opts.Offset)
	if err != nil {
		return nil, 0, fmt.Errorf("query repositories: %w", err)
	}
	defer rows.Close()

	var repositories []repository.RepositoryReadModel
	for rows.Next() {
		var repo repository.RepositoryReadModel
		var idStr string
		var addressJSON []byte
		var notes, gedcomXref sql.NullString
		var updatedAtStr string

		if err := rows.Scan(
			&idStr,
			&repo.Name,
			&addressJSON,
			&notes,
			&gedcomXref,
			&repo.Version,
			&updatedAtStr,
		); err != nil {
			return nil, 0, fmt.Errorf("scan repository: %w", err)
		}

		repo.ID, _ = uuid.Parse(idStr)
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
		if t, err := parseTimestamp(updatedAtStr); err == nil {
			repo.UpdatedAt = t
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

	var notes any
	if repo.Notes != "" {
		notes = repo.Notes
	}
	var gedcomXref any
	if repo.GedcomXref != "" {
		gedcomXref = repo.GedcomXref
	}

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO repositories (id, name, address, notes, gedcom_xref, version, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET
			name = excluded.name,
			address = excluded.address,
			notes = excluded.notes,
			gedcom_xref = excluded.gedcom_xref,
			version = excluded.version,
			updated_at = excluded.updated_at
	`, repo.ID.String(), repo.Name, addressJSON, notes, gedcomXref,
		repo.Version, repo.UpdatedAt.Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("save repository: %w", err)
	}
	return nil
}

// DeleteRepository deletes a repository by ID.
func (s *ReadModelStore) DeleteRepository(ctx context.Context, id uuid.UUID) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM repositories WHERE id = ?", id.String())
	if err != nil {
		return fmt.Errorf("delete repository: %w", err)
	}
	return nil
}

// scanAssociation scans one associationSelectCols row. It returns (nil, nil) for
// sql.ErrNoRows so single-row lookups report absence as nil.
func scanAssociation(row rowScanner) (*repository.AssociationReadModel, error) {
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
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan association: %w", err)
	}
	if assoc.ID, err = uuid.Parse(idStr); err != nil {
		return nil, fmt.Errorf("parse association id: %w", err)
	}
	if assoc.PersonID, err = uuid.Parse(personIDStr); err != nil {
		return nil, fmt.Errorf("parse association person_id: %w", err)
	}
	if assoc.AssociateID, err = uuid.Parse(associateIDStr); err != nil {
		return nil, fmt.Errorf("parse association associate_id: %w", err)
	}
	assoc.PersonName = personName.String
	assoc.AssociateName = associateName.String
	assoc.Phrase = phrase.String
	assoc.Notes = notes.String
	assoc.GedcomXref = gedcomXref.String
	if noteIDsJSON.Valid && noteIDsJSON.String != "" {
		if err := json.Unmarshal([]byte(noteIDsJSON.String), &assoc.NoteIDs); err != nil {
			return nil, fmt.Errorf("decode association note_ids: %w", err)
		}
	}
	if t, err := parseTimestamp(updatedAtStr); err == nil {
		assoc.UpdatedAt = t
	}
	return &assoc, nil
}

// scanAssociations drains rows of associationSelectCols.
func scanAssociations(rows *sql.Rows) ([]repository.AssociationReadModel, error) {
	defer rows.Close()
	var associations []repository.AssociationReadModel
	for rows.Next() {
		assoc, err := scanAssociation(rows)
		if err != nil {
			return nil, err
		}
		associations = append(associations, *assoc)
	}
	return associations, rows.Err()
}

// GetAssociation retrieves an association by ID within the branch overlay (ADR-005).
func (s *ReadModelStore) GetAssociation(ctx context.Context, branchID domain.BranchID, id uuid.UUID) (*repository.AssociationReadModel, error) {
	// #nosec G202 -- the query is built by factGetQuery from package constants; every value is a bound placeholder
	// nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query
	row := s.db.QueryRowContext(ctx, factGetQuery("associations", associationSelectCols), factGetArgs(branchID, id)...)
	return scanAssociation(row)
}

// ListAssociations returns a paginated list of the associations visible on
// opts.BranchID (ADR-005).
func (s *ReadModelStore) ListAssociations(ctx context.Context, opts repository.ListOptions) ([]repository.AssociationReadModel, int, error) {
	sub, args := factOverlaySubquery("associations", "", nil, opts.BranchID)

	var total int
	// #nosec G202 -- sub is built from package constants; every value is a bound ? placeholder
	// nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+sub+" a", args...).Scan(&total); err != nil {
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

	// #nosec G201 G202 -- orderColumn and orderDir are validated via switch/if above; sub is internal
	// nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query
	query := fmt.Sprintf(`
		SELECT `+associationSelectCols+`
		FROM %s a
		ORDER BY %s %s, id ASC
		LIMIT ? OFFSET ?
	`, sub, orderColumn, orderDir)

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
	sub, args := factOverlaySubquery("associations", personAssociationFilter,
		[]any{personID.String(), personID.String()}, branchID)
	// #nosec G202 -- sub is built from package constants; every value is a bound ? placeholder
	// nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+associationSelectCols+` FROM `+sub+` a
		ORDER BY role, updated_at DESC
	`, args...)
	if err != nil {
		return nil, fmt.Errorf("query associations for person: %w", err)
	}
	return scanAssociations(rows)
}

// SaveAssociation saves or updates an association on the given branch
// (ADR-005). A save always clears any prior tombstone.
func (s *ReadModelStore) SaveAssociation(ctx context.Context, branchID domain.BranchID, assoc *repository.AssociationReadModel) error {
	if err := s.guardBranchWrite(branchID); err != nil {
		return err
	}

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
		INSERT INTO associations (id, branch_id, person_id, person_name, associate_id, associate_name,
		                         role, phrase, notes, note_ids, gedcom_xref, version, updated_at, deleted)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0)
		ON CONFLICT (id, branch_id) DO UPDATE SET
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
			updated_at = excluded.updated_at,
			deleted = 0
	`, assoc.ID.String(), branchID.String(), assoc.PersonID.String(), personName, assoc.AssociateID.String(), associateName,
		assoc.Role, phrase, notes, noteIDsJSON, gedcomXref, assoc.Version, assoc.UpdatedAt.Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("save association: %w", err)
	}
	return nil
}

// DeleteAssociation removes an association (ADR-005): a real removal on main, a
// tombstone on a non-main branch.
func (s *ReadModelStore) DeleteAssociation(ctx context.Context, branchID domain.BranchID, id uuid.UUID) error {
	if err := s.deleteFactRow(ctx, "associations", associationSelectCols, branchID, id); err != nil {
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

// GPS artifacts (#760): evidence_analyses, evidence_conflicts, research_logs and
// proof_summaries are id-keyed branch-scoped tables with the life-event shape:
// every read goes through factOverlaySubquery / factGetQuery (one set-based
// ROW_NUMBER window with the main-scope fast path) and every filtered list
// applies its predicate to the winning row, so a branch-side resolution or
// re-pointed subject decides what the branch lists.
const (
	// analysisSelectCols is scanAnalysis's column order (unaliased).
	analysisSelectCols = `id, fact_type, subject_id, citation_ids, conclusion, research_status, notes,
		version, created_at, updated_at`

	// conflictSelectCols is scanConflict's column order (unaliased).
	conflictSelectCols = `id, fact_type, subject_id, analysis_ids, description, resolution, status,
		version, created_at, updated_at`

	// researchLogSelectCols is scanResearchLog's column order (unaliased).
	researchLogSelectCols = `id, subject_id, subject_type, repository, search_description, outcome, notes,
		search_date, version, created_at, updated_at`

	// proofSummarySelectCols is scanProofSummary's column order (unaliased).
	proofSummarySelectCols = `id, fact_type, subject_id, conclusion, argument, analysis_ids, research_status,
		version, created_at, updated_at`

	// GPS artifact filters; each binds its values once per ?.
	gpsSubjectFilter        = `subject_id = ?`
	gpsFactFilter           = `fact_type = ? AND subject_id = ?`
	gpsConflictStatusFilter = `status = ?`

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
		if err := cascadeFactRows(ctx, tx, t.name, t.cols, gpsSubjectFilter, branchID, subjectID); err != nil {
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
// package constant, or "") is applied to each id's winning row, binding values
// in order.
func (s *ReadModelStore) queryGPSPage(ctx context.Context, table, cols, filter string, opts repository.ListOptions, scan func(*sql.Rows) error, values ...any) (int, error) {
	sub, args := factOverlaySubquery(table, filter, values, opts.BranchID)

	var total int
	// #nosec G202 -- sub is built from package constants; every value is a bound ? placeholder
	// nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+sub+" g", args...).Scan(&total); err != nil {
		return 0, fmt.Errorf("count %s: %w", table, err)
	}

	// #nosec G202 -- sub, cols and the ORDER BY are internal constants; limit/offset stay bound parameters
	// nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query
	rows, err := s.db.QueryContext(ctx, "SELECT "+cols+" FROM "+sub+" g ORDER BY "+gpsListOrder(opts)+" LIMIT ? OFFSET ?",
		append(args, opts.Limit, opts.Offset)...)
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
// package constant) selects, binding values in order, calling scan once per row
// in gpsSubjectOrder.
func (s *ReadModelStore) queryGPSFiltered(ctx context.Context, table, cols, filter string, branchID domain.BranchID, scan func(*sql.Rows) error, values ...any) error {
	sub, args := factOverlaySubquery(table, filter, values, branchID)
	// #nosec G202 -- sub is built from package constants; every value is a bound ? placeholder
	// nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query
	rows, err := s.db.QueryContext(ctx, "SELECT "+cols+" FROM "+sub+" g ORDER BY "+gpsSubjectOrder, args...)
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
	// #nosec G202 -- the query is built by factGetQuery from package constants; every value is a bound placeholder
	// nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query
	return s.db.QueryRowContext(ctx, factGetQuery(table, cols), factGetArgs(branchID, id)...)
}

// parseGPSIDs parses the id and subject id columns of a GPS artifact row.
func parseGPSIDs(table, idStr, subjectStr string) (id, subject uuid.UUID, err error) {
	if id, err = uuid.Parse(idStr); err != nil {
		return uuid.Nil, uuid.Nil, fmt.Errorf("parse %s id: %w", table, err)
	}
	if subject, err = uuid.Parse(subjectStr); err != nil {
		return uuid.Nil, uuid.Nil, fmt.Errorf("parse %s subject id: %w", table, err)
	}
	return id, subject, nil
}

// parseGPSTimes parses the created_at/updated_at columns of a GPS artifact row.
func parseGPSTimes(table, createdStr, updatedStr string) (created, updated time.Time, err error) {
	if created, err = parseTimestamp(createdStr); err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("parse %s created_at: %w", table, err)
	}
	if updated, err = parseTimestamp(updatedStr); err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("parse %s updated_at: %w", table, err)
	}
	return created, updated, nil
}

func scanAnalysis(row rowScanner) (*repository.EvidenceAnalysisReadModel, error) {
	var a repository.EvidenceAnalysisReadModel
	var idStr, subjectStr, createdStr, updatedStr string
	var citationIDs, researchStatus, notes sql.NullString
	err := row.Scan(&idStr, &a.FactType, &subjectStr, &citationIDs, &a.Conclusion, &researchStatus, &notes,
		&a.Version, &createdStr, &updatedStr)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan evidence_analysis: %w", err)
	}
	if a.ID, a.SubjectID, err = parseGPSIDs("evidence_analysis", idStr, subjectStr); err != nil {
		return nil, err
	}
	if a.CreatedAt, a.UpdatedAt, err = parseGPSTimes("evidence_analysis", createdStr, updatedStr); err != nil {
		return nil, err
	}
	a.CitationIDsJSON = citationIDs.String
	a.ResearchStatus = domain.ResearchStatus(researchStatus.String)
	a.Notes = notes.String
	return &a, nil
}

func scanConflict(row rowScanner) (*repository.EvidenceConflictReadModel, error) {
	var c repository.EvidenceConflictReadModel
	var idStr, subjectStr, createdStr, updatedStr string
	var analysisIDs, resolution sql.NullString
	err := row.Scan(&idStr, &c.FactType, &subjectStr, &analysisIDs, &c.Description, &resolution, &c.Status,
		&c.Version, &createdStr, &updatedStr)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan evidence_conflict: %w", err)
	}
	if c.ID, c.SubjectID, err = parseGPSIDs("evidence_conflict", idStr, subjectStr); err != nil {
		return nil, err
	}
	if c.CreatedAt, c.UpdatedAt, err = parseGPSTimes("evidence_conflict", createdStr, updatedStr); err != nil {
		return nil, err
	}
	c.AnalysisIDsJSON = analysisIDs.String
	c.Resolution = resolution.String
	return &c, nil
}

func scanResearchLog(row rowScanner) (*repository.ResearchLogReadModel, error) {
	var l repository.ResearchLogReadModel
	var idStr, subjectStr, searchDateStr, createdStr, updatedStr string
	var notes sql.NullString
	err := row.Scan(&idStr, &subjectStr, &l.SubjectType, &l.Repository, &l.SearchDescription, &l.Outcome, &notes,
		&searchDateStr, &l.Version, &createdStr, &updatedStr)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan research_log: %w", err)
	}
	if l.ID, l.SubjectID, err = parseGPSIDs("research_log", idStr, subjectStr); err != nil {
		return nil, err
	}
	if l.CreatedAt, l.UpdatedAt, err = parseGPSTimes("research_log", createdStr, updatedStr); err != nil {
		return nil, err
	}
	if l.SearchDate, err = parseTimestamp(searchDateStr); err != nil {
		return nil, fmt.Errorf("parse research_log search_date: %w", err)
	}
	l.Notes = notes.String
	return &l, nil
}

func scanProofSummary(row rowScanner) (*repository.ProofSummaryReadModel, error) {
	var ps repository.ProofSummaryReadModel
	var idStr, subjectStr, createdStr, updatedStr string
	var analysisIDs, researchStatus sql.NullString
	err := row.Scan(&idStr, &ps.FactType, &subjectStr, &ps.Conclusion, &ps.Argument, &analysisIDs, &researchStatus,
		&ps.Version, &createdStr, &updatedStr)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan proof_summary: %w", err)
	}
	if ps.ID, ps.SubjectID, err = parseGPSIDs("proof_summary", idStr, subjectStr); err != nil {
		return nil, err
	}
	if ps.CreatedAt, ps.UpdatedAt, err = parseGPSTimes("proof_summary", createdStr, updatedStr); err != nil {
		return nil, err
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
	return scanAnalysis(s.getGPSRow(ctx, "evidence_analyses", analysisSelectCols, branchID, id))
}

// ListEvidenceAnalyses returns a paginated list of the evidence analyses visible
// on opts.BranchID.
func (s *ReadModelStore) ListEvidenceAnalyses(ctx context.Context, opts repository.ListOptions) ([]repository.EvidenceAnalysisReadModel, int, error) {
	var results []repository.EvidenceAnalysisReadModel
	total, err := s.queryGPSPage(ctx, "evidence_analyses", analysisSelectCols, "", opts, collectRows(&results, scanAnalysis))
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
		collectRows(&results, scanAnalysis), string(factType), subjectID.String()); err != nil {
		return nil, fmt.Errorf("analyses for fact: %w", err)
	}
	return results, nil
}

// GetAnalysesBySubject returns the evidence analyses of a subject visible on
// branchID, regardless of fact type.
func (s *ReadModelStore) GetAnalysesBySubject(ctx context.Context, branchID domain.BranchID, subjectID uuid.UUID) ([]repository.EvidenceAnalysisReadModel, error) {
	var results []repository.EvidenceAnalysisReadModel
	if err := s.queryGPSFiltered(ctx, "evidence_analyses", analysisSelectCols, gpsSubjectFilter, branchID,
		collectRows(&results, scanAnalysis), subjectID.String()); err != nil {
		return nil, fmt.Errorf("analyses by subject: %w", err)
	}
	return results, nil
}

// SaveEvidenceAnalysis saves or updates an evidence analysis on the given
// branch (ADR-005); a save always clears any prior tombstone.
func (s *ReadModelStore) SaveEvidenceAnalysis(ctx context.Context, branchID domain.BranchID, analysis *repository.EvidenceAnalysisReadModel) error {
	if err := s.guardBranchWrite(branchID); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO evidence_analyses (id, branch_id, fact_type, subject_id, citation_ids, conclusion, research_status,
		                               notes, version, created_at, updated_at, deleted)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0)
		ON CONFLICT (id, branch_id) DO UPDATE SET
			fact_type = excluded.fact_type,
			subject_id = excluded.subject_id,
			citation_ids = excluded.citation_ids,
			conclusion = excluded.conclusion,
			research_status = excluded.research_status,
			notes = excluded.notes,
			version = excluded.version,
			updated_at = excluded.updated_at,
			deleted = 0
	`, analysis.ID.String(), branchID.String(), string(analysis.FactType), analysis.SubjectID.String(),
		nullableString(analysis.CitationIDsJSON), analysis.Conclusion, nullableString(string(analysis.ResearchStatus)),
		nullableString(analysis.Notes), analysis.Version,
		analysis.CreatedAt.Format(time.RFC3339), analysis.UpdatedAt.Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("save evidence_analysis: %w", err)
	}
	return nil
}

// DeleteEvidenceAnalysis removes an evidence analysis (ADR-005): a real removal
// on main, a tombstone on a non-main branch.
func (s *ReadModelStore) DeleteEvidenceAnalysis(ctx context.Context, branchID domain.BranchID, id uuid.UUID) error {
	if err := s.deleteFactRow(ctx, "evidence_analyses", analysisSelectCols, branchID, id); err != nil {
		return fmt.Errorf("delete evidence_analysis: %w", err)
	}
	return nil
}

// GetEvidenceConflict retrieves an evidence conflict by ID within the branch
// overlay (ADR-005, #760).
func (s *ReadModelStore) GetEvidenceConflict(ctx context.Context, branchID domain.BranchID, id uuid.UUID) (*repository.EvidenceConflictReadModel, error) {
	return scanConflict(s.getGPSRow(ctx, "evidence_conflicts", conflictSelectCols, branchID, id))
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
		collectRows(&results, scanConflict), values...)
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
		collectRows(&results, scanConflict), subjectID.String()); err != nil {
		return nil, fmt.Errorf("conflicts for subject: %w", err)
	}
	return results, nil
}

// ListUnresolvedConflicts returns the open evidence conflicts visible on
// branchID. The overlay window resolves each conflict first and the status
// predicate is applied to the winning row, so a conflict a branch resolved is
// not listed on that branch even though main's row for it is still open.
func (s *ReadModelStore) ListUnresolvedConflicts(ctx context.Context, branchID domain.BranchID) ([]repository.EvidenceConflictReadModel, error) {
	var results []repository.EvidenceConflictReadModel
	if err := s.queryGPSFiltered(ctx, "evidence_conflicts", conflictSelectCols, gpsConflictStatusFilter, branchID,
		collectRows(&results, scanConflict), string(domain.ConflictStatusOpen)); err != nil {
		return nil, fmt.Errorf("unresolved conflicts: %w", err)
	}
	return results, nil
}

// SaveEvidenceConflict saves or updates an evidence conflict on the given branch
// (ADR-005); a save always clears any prior tombstone.
func (s *ReadModelStore) SaveEvidenceConflict(ctx context.Context, branchID domain.BranchID, conflict *repository.EvidenceConflictReadModel) error {
	if err := s.guardBranchWrite(branchID); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO evidence_conflicts (id, branch_id, fact_type, subject_id, analysis_ids, description, resolution,
		                                status, version, created_at, updated_at, deleted)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0)
		ON CONFLICT (id, branch_id) DO UPDATE SET
			fact_type = excluded.fact_type,
			subject_id = excluded.subject_id,
			analysis_ids = excluded.analysis_ids,
			description = excluded.description,
			resolution = excluded.resolution,
			status = excluded.status,
			version = excluded.version,
			updated_at = excluded.updated_at,
			deleted = 0
	`, conflict.ID.String(), branchID.String(), string(conflict.FactType), conflict.SubjectID.String(),
		nullableString(conflict.AnalysisIDsJSON), conflict.Description, nullableString(conflict.Resolution),
		string(conflict.Status), conflict.Version,
		conflict.CreatedAt.Format(time.RFC3339), conflict.UpdatedAt.Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("save evidence_conflict: %w", err)
	}
	return nil
}

// DeleteEvidenceConflict removes an evidence conflict (ADR-005): a real removal
// on main, a tombstone on a non-main branch.
func (s *ReadModelStore) DeleteEvidenceConflict(ctx context.Context, branchID domain.BranchID, id uuid.UUID) error {
	if err := s.deleteFactRow(ctx, "evidence_conflicts", conflictSelectCols, branchID, id); err != nil {
		return fmt.Errorf("delete evidence_conflict: %w", err)
	}
	return nil
}

// GetResearchLog retrieves a research log by ID within the branch overlay
// (ADR-005, #760).
func (s *ReadModelStore) GetResearchLog(ctx context.Context, branchID domain.BranchID, id uuid.UUID) (*repository.ResearchLogReadModel, error) {
	return scanResearchLog(s.getGPSRow(ctx, "research_logs", researchLogSelectCols, branchID, id))
}

// ListResearchLogs returns a paginated list of the research logs visible on
// opts.BranchID.
func (s *ReadModelStore) ListResearchLogs(ctx context.Context, opts repository.ListOptions) ([]repository.ResearchLogReadModel, int, error) {
	var results []repository.ResearchLogReadModel
	total, err := s.queryGPSPage(ctx, "research_logs", researchLogSelectCols, "", opts, collectRows(&results, scanResearchLog))
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
		collectRows(&results, scanResearchLog), subjectID.String()); err != nil {
		return nil, fmt.Errorf("research logs for subject: %w", err)
	}
	return results, nil
}

// SaveResearchLog saves or updates a research log on the given branch
// (ADR-005); a save always clears any prior tombstone.
func (s *ReadModelStore) SaveResearchLog(ctx context.Context, branchID domain.BranchID, log *repository.ResearchLogReadModel) error {
	if err := s.guardBranchWrite(branchID); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO research_logs (id, branch_id, subject_id, subject_type, repository, search_description, outcome,
		                           notes, search_date, version, created_at, updated_at, deleted)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0)
		ON CONFLICT (id, branch_id) DO UPDATE SET
			subject_id = excluded.subject_id,
			subject_type = excluded.subject_type,
			repository = excluded.repository,
			search_description = excluded.search_description,
			outcome = excluded.outcome,
			notes = excluded.notes,
			search_date = excluded.search_date,
			version = excluded.version,
			updated_at = excluded.updated_at,
			deleted = 0
	`, log.ID.String(), branchID.String(), log.SubjectID.String(), log.SubjectType, log.Repository,
		log.SearchDescription, string(log.Outcome), nullableString(log.Notes),
		log.SearchDate.Format(time.RFC3339), log.Version,
		log.CreatedAt.Format(time.RFC3339), log.UpdatedAt.Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("save research_log: %w", err)
	}
	return nil
}

// DeleteResearchLog removes a research log (ADR-005): a real removal on main, a
// tombstone on a non-main branch.
func (s *ReadModelStore) DeleteResearchLog(ctx context.Context, branchID domain.BranchID, id uuid.UUID) error {
	if err := s.deleteFactRow(ctx, "research_logs", researchLogSelectCols, branchID, id); err != nil {
		return fmt.Errorf("delete research_log: %w", err)
	}
	return nil
}

// GetProofSummary retrieves a proof summary by ID within the branch overlay
// (ADR-005, #760).
func (s *ReadModelStore) GetProofSummary(ctx context.Context, branchID domain.BranchID, id uuid.UUID) (*repository.ProofSummaryReadModel, error) {
	return scanProofSummary(s.getGPSRow(ctx, "proof_summaries", proofSummarySelectCols, branchID, id))
}

// ListProofSummaries returns a paginated list of the proof summaries visible on
// opts.BranchID.
func (s *ReadModelStore) ListProofSummaries(ctx context.Context, opts repository.ListOptions) ([]repository.ProofSummaryReadModel, int, error) {
	var results []repository.ProofSummaryReadModel
	total, err := s.queryGPSPage(ctx, "proof_summaries", proofSummarySelectCols, "", opts, collectRows(&results, scanProofSummary))
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
		collectRows(&results, scanProofSummary), string(factType), subjectID.String()); err != nil {
		return nil, fmt.Errorf("proof summaries for fact: %w", err)
	}
	return results, nil
}

// GetProofSummariesBySubject returns the proof summaries of a subject visible on
// branchID, regardless of fact type.
func (s *ReadModelStore) GetProofSummariesBySubject(ctx context.Context, branchID domain.BranchID, subjectID uuid.UUID) ([]repository.ProofSummaryReadModel, error) {
	var results []repository.ProofSummaryReadModel
	if err := s.queryGPSFiltered(ctx, "proof_summaries", proofSummarySelectCols, gpsSubjectFilter, branchID,
		collectRows(&results, scanProofSummary), subjectID.String()); err != nil {
		return nil, fmt.Errorf("proof summaries by subject: %w", err)
	}
	return results, nil
}

// SaveProofSummary saves or updates a proof summary on the given branch
// (ADR-005); a save always clears any prior tombstone.
func (s *ReadModelStore) SaveProofSummary(ctx context.Context, branchID domain.BranchID, summary *repository.ProofSummaryReadModel) error {
	if err := s.guardBranchWrite(branchID); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO proof_summaries (id, branch_id, fact_type, subject_id, conclusion, argument, analysis_ids,
		                             research_status, version, created_at, updated_at, deleted)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0)
		ON CONFLICT (id, branch_id) DO UPDATE SET
			fact_type = excluded.fact_type,
			subject_id = excluded.subject_id,
			conclusion = excluded.conclusion,
			argument = excluded.argument,
			analysis_ids = excluded.analysis_ids,
			research_status = excluded.research_status,
			version = excluded.version,
			updated_at = excluded.updated_at,
			deleted = 0
	`, summary.ID.String(), branchID.String(), string(summary.FactType), summary.SubjectID.String(),
		summary.Conclusion, summary.Argument, nullableString(summary.AnalysisIDsJSON),
		nullableString(string(summary.ResearchStatus)), summary.Version,
		summary.CreatedAt.Format(time.RFC3339), summary.UpdatedAt.Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("save proof_summary: %w", err)
	}
	return nil
}

// DeleteProofSummary removes a proof summary (ADR-005): a real removal on main,
// a tombstone on a non-main branch.
func (s *ReadModelStore) DeleteProofSummary(ctx context.Context, branchID domain.BranchID, id uuid.UUID) error {
	if err := s.deleteFactRow(ctx, "proof_summaries", proofSummarySelectCols, branchID, id); err != nil {
		return fmt.Errorf("delete proof_summary: %w", err)
	}
	return nil
}
