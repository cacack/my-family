package memory

import (
	"context"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/cacack/gedcom-go/v2/gedcom"
	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// branchKey identifies a read-model row inside a specific branch's overlay
// (ADR-005). For single-row slice entities the id is the entity id; for
// collection slice entities (names, children, external ids) the id is the
// parent id and the value is that parent's whole bucket. The mainline uses
// branch == domain.MainBranchID; non-main branches hold copy-on-write shadow
// rows and tombstones layered over main.
type branchKey struct {
	branch domain.BranchID
	id     uuid.UUID
}

// resolveRow returns the overlay-resolved pointer for a single-row slice entity
// at (branch, id): the branch's own row when the branch has written one (a nil
// pointer is a tombstone), otherwise the main row. ok reports whether any row
// exists for the key (a tombstone counts as existing). When branch is
// MainBranchID only the main row is consulted, reproducing pre-branch behavior.
func resolveRow[T any](m map[branchKey]*T, branch domain.BranchID, id uuid.UUID) (row *T, ok bool) {
	if v, present := m[branchKey{branch, id}]; present {
		return v, true
	}
	if branch != domain.MainBranchID {
		if v, present := m[branchKey{domain.MainBranchID, id}]; present {
			return v, true
		}
	}
	return nil, false
}

// resolveRowsByIDs is resolveRow for many ids: copies of the rows visible on
// branch for ids, deduplicated and ordered by id (the order the SQL backends
// return their batched lookups in, #697). Ids with no visible row — never
// written, or hidden by a branch tombstone — are absent.
func resolveRowsByIDs[T any](m map[branchKey]*T, branch domain.BranchID, ids []uuid.UUID) []T {
	if len(ids) == 0 {
		return nil
	}
	sorted := slices.Clone(ids)
	slices.SortFunc(sorted, func(a, b uuid.UUID) int { return strings.Compare(a.String(), b.String()) })
	sorted = slices.Compact(sorted)
	var rows []T
	for _, id := range sorted {
		if row, _ := resolveRow(m, branch, id); row != nil {
			rows = append(rows, *row)
		}
	}
	return rows
}

// resolveAllRows returns the overlay-resolved rows for a single-row slice entity
// visible on branch: every main row, then the branch's shadow rows layered over
// them (a nil branch value is a tombstone that removes the id). When branch is
// MainBranchID this is exactly the set of main rows.
func resolveAllRows[T any](m map[branchKey]*T, branch domain.BranchID) []*T {
	resolved := make(map[uuid.UUID]*T)
	for k, v := range m {
		if k.branch == domain.MainBranchID && v != nil {
			resolved[k.id] = v
		}
	}
	if branch != domain.MainBranchID {
		for k, v := range m {
			if k.branch != branch {
				continue
			}
			if v == nil {
				delete(resolved, k.id) // tombstone hides the main row
			} else {
				resolved[k.id] = v
			}
		}
	}
	out := make([]*T, 0, len(resolved))
	for _, v := range resolved {
		out = append(out, v)
	}
	return out
}

// resolveBucket returns the overlay-resolved slice for a collection slice entity
// at (branch, parent): the branch's own bucket when present (an empty bucket is
// a tombstone that hides main), otherwise main's bucket. ok reports whether any
// bucket exists for the key.
func resolveBucket[T any](m map[branchKey][]T, branch domain.BranchID, parent uuid.UUID) (rows []T, ok bool) {
	if v, present := m[branchKey{branch, parent}]; present {
		return v, true
	}
	if branch != domain.MainBranchID {
		if v, present := m[branchKey{domain.MainBranchID, parent}]; present {
			return v, true
		}
	}
	return nil, false
}

// resolveAllBuckets returns the overlay-resolved bucket for every parent visible
// on branch: main's buckets, then the branch's buckets layered over them (a
// branch bucket wins wholesale; an empty branch bucket is a tombstone).
func resolveAllBuckets[T any](m map[branchKey][]T, branch domain.BranchID) map[uuid.UUID][]T {
	resolved := make(map[uuid.UUID][]T)
	for k, v := range m {
		if k.branch == domain.MainBranchID {
			resolved[k.id] = v
		}
	}
	if branch != domain.MainBranchID {
		for k, v := range m {
			if k.branch == branch {
				resolved[k.id] = v
			}
		}
	}
	return resolved
}

// bucketForWrite returns a fresh, mutable copy of the effective bucket for
// (branch, parent) to support copy-on-write: a branch mutation seeds from the
// branch's own bucket if it has one, else from main, so the first branch edit
// forks main's bucket rather than aliasing it.
func bucketForWrite[T any](m map[branchKey][]T, branch domain.BranchID, parent uuid.UUID) []T {
	v, _ := resolveBucket(m, branch, parent)
	if len(v) == 0 {
		return nil
	}
	cp := make([]T, len(v))
	copy(cp, v)
	return cp
}

// storeBucket writes a collection slice entity's bucket for (branch, parent). On
// main an empty bucket is a real removal (the key is deleted); on a non-main
// branch an empty bucket is stored as a present tombstone so the main fallback
// does not resurrect the parent's rows.
func storeBucket[T any](m map[branchKey][]T, branch domain.BranchID, parent uuid.UUID, rows []T) {
	key := branchKey{branch, parent}
	if branch == domain.MainBranchID && len(rows) == 0 {
		delete(m, key)
		return
	}
	m[key] = rows
}

// ReadModelStore is an in-memory implementation of repository.ReadModelStore for testing.
type ReadModelStore struct {
	mu sync.RWMutex
	// Branch-aware slice entities (ADR-005): keyed by (branch, id). Single-row
	// maps use a nil value as a tombstone; collection maps use a present-but-empty
	// bucket as a tombstone. See branchKey and the resolve* helpers above.
	persons               map[branchKey]*repository.PersonReadModel
	personNames           map[branchKey][]repository.PersonNameReadModel       // keyed by (branch, person ID)
	nameOwners            map[branchKey]uuid.UUID                              // (branch, name ID) -> person last saved under; a hint, see SavePersonName
	personExternalIDs     map[branchKey][]repository.PersonExternalIDReadModel // keyed by (branch, person ID)
	families              map[branchKey]*repository.FamilyReadModel
	familyChildren        map[branchKey][]repository.FamilyChildReadModel      // keyed by (branch, family ID)
	familyExternalIDs     map[branchKey][]repository.FamilyExternalIDReadModel // keyed by (branch, family ID)
	pedigreeEdges         map[branchKey]*repository.PedigreeEdge               // keyed by (branch, person ID)
	sources               map[branchKey]*repository.SourceReadModel            // branch-scoped (#758)
	sourceExternalIDs     map[branchKey][]repository.SourceExternalIDReadModel // branch-scoped bucket keyed by source ID (#758)
	citations             map[branchKey]*repository.CitationReadModel          // branch-scoped (#758)
	media                 map[branchKey]*mediaRow                              // branch-scoped metadata; bytes shared (#759)
	events                map[branchKey]*repository.EventReadModel             // branch-scoped (#757)
	attributes            map[branchKey]*repository.AttributeReadModel         // branch-scoped (#757)
	notes                 map[branchKey]*repository.NoteReadModel              // branch-scoped (#758)
	submitters            map[uuid.UUID]*repository.SubmitterReadModel
	repositories          map[uuid.UUID]*repository.RepositoryReadModel
	repositoryExternalIDs map[uuid.UUID][]repository.RepositoryExternalIDReadModel // keyed by repository ID
	associations          map[branchKey]*repository.AssociationReadModel           // branch-scoped (#757)
	ldsOrdinances         map[uuid.UUID]*repository.LDSOrdinanceReadModel
	evidenceAnalyses      map[branchKey]*repository.EvidenceAnalysisReadModel // branch-scoped (#760)
	evidenceConflicts     map[branchKey]*repository.EvidenceConflictReadModel // branch-scoped (#760)
	researchLogs          map[branchKey]*repository.ResearchLogReadModel      // branch-scoped (#760)
	proofSummaries        map[branchKey]*repository.ProofSummaryReadModel     // branch-scoped (#760)
}

// NewReadModelStore creates a new in-memory read model store.
func NewReadModelStore() *ReadModelStore {
	return &ReadModelStore{
		persons:               make(map[branchKey]*repository.PersonReadModel),
		personNames:           make(map[branchKey][]repository.PersonNameReadModel),
		nameOwners:            make(map[branchKey]uuid.UUID),
		personExternalIDs:     make(map[branchKey][]repository.PersonExternalIDReadModel),
		families:              make(map[branchKey]*repository.FamilyReadModel),
		familyChildren:        make(map[branchKey][]repository.FamilyChildReadModel),
		familyExternalIDs:     make(map[branchKey][]repository.FamilyExternalIDReadModel),
		pedigreeEdges:         make(map[branchKey]*repository.PedigreeEdge),
		sources:               make(map[branchKey]*repository.SourceReadModel),
		sourceExternalIDs:     make(map[branchKey][]repository.SourceExternalIDReadModel),
		citations:             make(map[branchKey]*repository.CitationReadModel),
		media:                 make(map[branchKey]*mediaRow),
		events:                make(map[branchKey]*repository.EventReadModel),
		attributes:            make(map[branchKey]*repository.AttributeReadModel),
		notes:                 make(map[branchKey]*repository.NoteReadModel),
		submitters:            make(map[uuid.UUID]*repository.SubmitterReadModel),
		repositories:          make(map[uuid.UUID]*repository.RepositoryReadModel),
		repositoryExternalIDs: make(map[uuid.UUID][]repository.RepositoryExternalIDReadModel),
		associations:          make(map[branchKey]*repository.AssociationReadModel),
		ldsOrdinances:         make(map[uuid.UUID]*repository.LDSOrdinanceReadModel),
		evidenceAnalyses:      make(map[branchKey]*repository.EvidenceAnalysisReadModel),
		evidenceConflicts:     make(map[branchKey]*repository.EvidenceConflictReadModel),
		researchLogs:          make(map[branchKey]*repository.ResearchLogReadModel),
		proofSummaries:        make(map[branchKey]*repository.ProofSummaryReadModel),
	}
}

// GetPerson retrieves a person by ID, resolving the branch overlay.
func (s *ReadModelStore) GetPerson(ctx context.Context, branchID domain.BranchID, id uuid.UUID) (*repository.PersonReadModel, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	p, ok := resolveRow(s.persons, branchID, id)
	if !ok || p == nil {
		return nil, nil
	}
	// Return a copy
	result := *p
	return &result, nil
}

// GetPersonsByIDs retrieves every visible person among ids on branchID (#697).
func (s *ReadModelStore) GetPersonsByIDs(ctx context.Context, branchID domain.BranchID, ids []uuid.UUID) ([]repository.PersonReadModel, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return resolveRowsByIDs(s.persons, branchID, ids), nil
}

// matchesResearchStatusFilter checks if a person matches the research status filter.
func matchesResearchStatusFilter(p *repository.PersonReadModel, filter *string) bool {
	if filter == nil {
		return true
	}
	if *filter == "unset" {
		return p.ResearchStatus == ""
	}
	return string(p.ResearchStatus) == *filter
}

// comparePersons compares two persons based on the sort field.
func comparePersons(a, b *repository.PersonReadModel, sortField string) int {
	switch sortField {
	case "given_name":
		return strings.Compare(a.GivenName, b.GivenName)
	case "birth_date":
		return compareBirthDates(a.BirthDateSort, b.BirthDateSort)
	case "updated_at":
		return compareTimestamps(a.UpdatedAt, b.UpdatedAt)
	default: // surname
		cmp := strings.Compare(a.Surname, b.Surname)
		if cmp == 0 {
			return strings.Compare(a.GivenName, b.GivenName)
		}
		return cmp
	}
}

// compareBirthDates compares two birth date pointers.
func compareBirthDates(a, b *time.Time) int {
	if a == nil && b == nil {
		return 0
	}
	if a == nil {
		return 1
	}
	if b == nil {
		return -1
	}
	return compareTimestamps(*a, *b)
}

// compareTimestamps compares two timestamps.
func compareTimestamps(a, b time.Time) int {
	if a.Before(b) {
		return -1
	}
	if a.After(b) {
		return 1
	}
	return 0
}

// ListPersons returns a paginated list of persons.
func (s *ReadModelStore) ListPersons(ctx context.Context, opts repository.ListOptions) ([]repository.PersonReadModel, int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Convert overlay-resolved rows to slice, applying research_status filter.
	resolved := resolveAllRows(s.persons, opts.BranchID)
	persons := make([]repository.PersonReadModel, 0, len(resolved))
	for _, p := range resolved {
		if matchesResearchStatusFilter(p, opts.ResearchStatus) {
			persons = append(persons, *p)
		}
	}

	// Sort
	sort.Slice(persons, func(i, j int) bool {
		cmp := comparePersons(&persons[i], &persons[j], opts.Sort)
		if opts.Order == "desc" {
			return cmp > 0
		}
		return cmp < 0
	})

	total := len(persons)

	// Paginate
	start := opts.Offset
	if start > len(persons) {
		start = len(persons)
	}
	end := start + opts.Limit
	if end > len(persons) {
		end = len(persons)
	}

	return persons[start:end], total, nil
}

// SearchPersons searches for persons by name, including alternate names.
func (s *ReadModelStore) SearchPersons(ctx context.Context, opts repository.SearchOptions) ([]repository.PersonReadModel, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	hasQuery := strings.TrimSpace(opts.Query) != ""
	hasDateFilter := opts.BirthDateFrom != nil || opts.BirthDateTo != nil ||
		opts.DeathDateFrom != nil || opts.DeathDateTo != nil
	hasPlaceFilter := strings.TrimSpace(opts.BirthPlace) != "" || strings.TrimSpace(opts.DeathPlace) != ""
	if !hasQuery && !hasDateFilter && !hasPlaceFilter {
		return nil, nil
	}

	queryLower := strings.ToLower(opts.Query)
	foundIDs := make(map[uuid.UUID]bool)
	var results []repository.PersonReadModel

	// Search the overlay-resolved persons for the requested branch.
	for _, p := range resolveAllRows(s.persons, opts.BranchID) {
		if !s.matchesSearchFilters(p, opts) {
			continue
		}
		if s.personMatchesQuery(p, queryLower, opts.Soundex) && !foundIDs[p.ID] {
			results = append(results, *p)
			foundIDs[p.ID] = true
		}
	}

	// Search in person_names table for alternate names (only if text query provided)
	if opts.Query != "" {
		s.searchAlternateNames(opts.BranchID, queryLower, opts, foundIDs, &results)
	}

	// Sort results to match postgres/sqlite behavior
	sortSearchResults(results, opts)

	// Apply limit after sorting
	if opts.Limit > 0 && len(results) > opts.Limit {
		results = results[:opts.Limit]
	}

	return results, nil
}

// personMatchesQuery checks if a person matches the text query (or returns true if no query).
func (s *ReadModelStore) personMatchesQuery(p *repository.PersonReadModel, queryLower string, soundex bool) bool {
	if queryLower == "" {
		return true
	}
	if strings.Contains(strings.ToLower(p.FullName), queryLower) ||
		strings.Contains(strings.ToLower(p.GivenName), queryLower) ||
		strings.Contains(strings.ToLower(p.Surname), queryLower) {
		return true
	}
	if soundex {
		for _, word := range strings.Fields(queryLower) {
			if repository.SoundexMatch(word, p.GivenName) || repository.SoundexMatch(word, p.Surname) {
				return true
			}
		}
	}
	return false
}

// searchAlternateNames searches person_names for alternate name matches.
func (s *ReadModelStore) searchAlternateNames(branchID domain.BranchID, queryLower string, opts repository.SearchOptions, foundIDs map[uuid.UUID]bool, results *[]repository.PersonReadModel) {
	for personID, names := range resolveAllBuckets(s.personNames, branchID) {
		if len(*results) >= opts.Limit {
			break
		}
		if foundIDs[personID] {
			continue
		}
		for _, name := range names {
			if altNameMatches(name, queryLower, opts.Soundex) {
				if p, ok := resolveRow(s.persons, branchID, personID); ok && p != nil && !foundIDs[personID] && s.matchesSearchFilters(p, opts) {
					*results = append(*results, *p)
					foundIDs[personID] = true
				}
				break
			}
		}
	}
}

// altNameMatches checks if a PersonNameReadModel matches via substring or Soundex.
func altNameMatches(name repository.PersonNameReadModel, queryLower string, soundex bool) bool {
	if nameMatchesQuery(name, queryLower) {
		return true
	}
	if soundex {
		for _, word := range strings.Fields(queryLower) {
			if repository.SoundexMatch(word, name.GivenName) ||
				repository.SoundexMatch(word, name.Surname) ||
				repository.SoundexMatch(word, name.Nickname) {
				return true
			}
		}
	}
	return false
}

// sortSearchResults sorts results by the requested field and direction.
func sortSearchResults(results []repository.PersonReadModel, opts repository.SearchOptions) {
	if opts.Sort == "" || opts.Sort == "relevance" {
		return // keep insertion order for relevance
	}
	desc := strings.EqualFold(opts.Order, "desc")
	sort.SliceStable(results, func(i, j int) bool {
		var cmp int
		switch opts.Sort {
		case "name":
			cmp = strings.Compare(results[i].Surname, results[j].Surname)
			if cmp == 0 {
				cmp = strings.Compare(results[i].GivenName, results[j].GivenName)
			}
		case "birth_date":
			cmp = compareTimePtr(results[i].BirthDateSort, results[j].BirthDateSort)
		case "death_date":
			cmp = compareTimePtr(results[i].DeathDateSort, results[j].DeathDateSort)
		default:
			return false
		}
		if desc {
			return cmp > 0
		}
		return cmp < 0
	})
}

// compareTimePtr does a three-way comparison of nullable times. Nil sorts last.
func compareTimePtr(a, b *time.Time) int {
	if a == nil && b == nil {
		return 0
	}
	if a == nil {
		return 1 // nil sorts last
	}
	if b == nil {
		return -1
	}
	return a.Compare(*b)
}

// nameMatchesQuery checks if a PersonNameReadModel matches the query.
func nameMatchesQuery(name repository.PersonNameReadModel, queryLower string) bool {
	return strings.Contains(strings.ToLower(name.FullName), queryLower) ||
		strings.Contains(strings.ToLower(name.GivenName), queryLower) ||
		strings.Contains(strings.ToLower(name.Surname), queryLower) ||
		strings.Contains(strings.ToLower(name.Nickname), queryLower)
}

// matchesSearchFilters checks if a person matches the date/place filters in SearchOptions.
func (s *ReadModelStore) matchesSearchFilters(p *repository.PersonReadModel, opts repository.SearchOptions) bool {
	if opts.BirthDateFrom != nil && (p.BirthDateSort == nil || p.BirthDateSort.Before(*opts.BirthDateFrom)) {
		return false
	}
	if opts.BirthDateTo != nil && (p.BirthDateSort == nil || p.BirthDateSort.After(*opts.BirthDateTo)) {
		return false
	}
	if opts.DeathDateFrom != nil && (p.DeathDateSort == nil || p.DeathDateSort.Before(*opts.DeathDateFrom)) {
		return false
	}
	if opts.DeathDateTo != nil && (p.DeathDateSort == nil || p.DeathDateSort.After(*opts.DeathDateTo)) {
		return false
	}
	if opts.BirthPlace != "" && !strings.Contains(strings.ToLower(p.BirthPlace), strings.ToLower(opts.BirthPlace)) {
		return false
	}
	if opts.DeathPlace != "" && !strings.Contains(strings.ToLower(p.DeathPlace), strings.ToLower(opts.DeathPlace)) {
		return false
	}
	return true
}

// SavePerson saves or updates a person on the given branch.
func (s *ReadModelStore) SavePerson(ctx context.Context, branchID domain.BranchID, person *repository.PersonReadModel) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	result := *person
	s.persons[branchKey{branchID, person.ID}] = &result
	return nil
}

// DeletePerson removes a person. On main this is a real removal; on a non-main
// branch it writes tombstones (for the person and its cascaded names/external
// IDs) so the main fallback does not resurrect the entity.
func (s *ReadModelStore) DeletePerson(ctx context.Context, branchID domain.BranchID, id uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if branchID == domain.MainBranchID {
		// Reproduce the pre-#669 ON DELETE CASCADE explicitly (read-model FKs were
		// dropped for branch scoping). person_names, person_external_ids,
		// pedigree_edges and associations (both person_id and associate_id) were
		// ON DELETE CASCADE. attributes referenced persons(id) with NO ON DELETE
		// (RESTRICT), which would have blocked the delete; blocking is not
		// reproducible against an append-only event log, so we cascade-delete
		// orphan attributes too. The person's own life events (owner_type
		// "person") go with it as well (#757), so no backend keeps counting a
		// deleted person's burial in the cemetery index.
		delete(s.persons, branchKey{domain.MainBranchID, id})
		delete(s.personNames, branchKey{domain.MainBranchID, id})
		delete(s.personExternalIDs, branchKey{domain.MainBranchID, id})
		delete(s.pedigreeEdges, branchKey{domain.MainBranchID, id})
		// family_children referenced persons(id) ON DELETE CASCADE (the child side),
		// so remove the deleted person from every mainline family it was a child of.
		for k, children := range s.familyChildren {
			if k.branch != domain.MainBranchID {
				continue
			}
			out := children[:0]
			for _, c := range children {
				if c.PersonID != id {
					out = append(out, c)
				}
			}
			storeBucket(s.familyChildren, k.branch, k.id, out)
		}
		s.cascadePersonFacts(domain.MainBranchID, id)
		s.cascadeMedia(domain.MainBranchID, "person", id)
		s.cascadeGPS(domain.MainBranchID, id)
		return nil
	}
	// Branch delete: tombstone the person and its branch-scoped dependents,
	// including the person/family facts it owns or is associated through.
	s.persons[branchKey{branchID, id}] = nil           // tombstone
	s.personNames[branchKey{branchID, id}] = nil       // cascade tombstone
	s.personExternalIDs[branchKey{branchID, id}] = nil // cascade tombstone
	s.pedigreeEdges[branchKey{branchID, id}] = nil     // cascade tombstone
	s.cascadePersonFacts(branchID, id)
	s.cascadeMedia(branchID, "person", id)
	s.cascadeGPS(branchID, id)
	return nil
}

// removeRow deletes the row (branch, id) from a single-row branch-scoped map: on
// main it is a real removal, on a non-main branch it stores a nil tombstone so
// the main fallback does not resurrect the row. Callers hold s.mu.
func removeRow[T any](m map[branchKey]*T, branch domain.BranchID, id uuid.UUID) {
	if branch == domain.MainBranchID {
		delete(m, branchKey{branch, id})
		return
	}
	m[branchKey{branch, id}] = nil
}

// cascadePersonFacts removes, on branch, every life event, attribute and
// association the person owns (associations on either side). Main rows are hard
// deleted; a branch tombstones each row visible on it (#757). Callers hold s.mu.
func (s *ReadModelStore) cascadePersonFacts(branch domain.BranchID, personID uuid.UUID) {
	for _, e := range resolveAllRows(s.events, branch) {
		if e.OwnerType == "person" && e.OwnerID == personID {
			removeRow(s.events, branch, e.ID)
		}
	}
	for _, a := range resolveAllRows(s.attributes, branch) {
		if a.PersonID == personID {
			removeRow(s.attributes, branch, a.ID)
		}
	}
	for _, a := range resolveAllRows(s.associations, branch) {
		if a.PersonID == personID || a.AssociateID == personID {
			removeRow(s.associations, branch, a.ID)
		}
	}
}

// SavePersonName saves or updates a person name variant on the given branch.
// On a non-main branch the person's name bucket is copied-on-write from main
// before the mutation is applied.
func (s *ReadModelStore) SavePersonName(ctx context.Context, branchID domain.BranchID, name *repository.PersonNameReadModel) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Compute full name if not set
	fullName := name.FullName
	if fullName == "" {
		fullName = name.GivenName + " " + name.Surname
	}

	nameCopy := *name
	nameCopy.FullName = fullName

	// A name saved under a different person than the one the scope currently
	// files it under has been re-owned (PersonMerged moves the merged person's
	// names to the survivor, #834). Names are bucketed per person, so take it
	// out of its old owner's bucket — copy-on-write on a branch — or it would
	// resolve under both, as the SQL backends' per-id overlay does not.
	//
	// The old owner comes from nameOwners, the person each name id was last
	// saved under in each scope, so this is O(1) per save rather than a scan
	// of every bucket (a GEDCOM import saves one name per NameAdded). The index
	// is only a hint: an entry left behind by a delete or purge names a bucket
	// that no longer holds the id, which the membership check below skips.
	for _, owner := range s.nameOwnerCandidates(branchID, name.ID) {
		if owner == name.PersonID {
			continue
		}
		current, _ := resolveBucket(s.personNames, branchID, owner)
		if !slices.ContainsFunc(current, func(n repository.PersonNameReadModel) bool { return n.ID == name.ID }) {
			continue
		}
		seeded := bucketForWrite(s.personNames, branchID, owner)
		kept := seeded[:0]
		for _, n := range seeded {
			if n.ID != name.ID {
				kept = append(kept, n)
			}
		}
		storeBucket(s.personNames, branchID, owner, kept)
	}
	s.nameOwners[branchKey{branchID, name.ID}] = name.PersonID

	names := bucketForWrite(s.personNames, branchID, name.PersonID)
	updated := false
	for i := range names {
		if names[i].ID == name.ID {
			names[i] = nameCopy
			updated = true
			break
		}
	}
	if !updated {
		names = append(names, nameCopy)
	}
	storeBucket(s.personNames, branchID, name.PersonID, names)
	return nil
}

// nameOwnerCandidates returns the persons a name id may currently be filed
// under in a scope: the one it was last saved under on the branch, and on main
// (a branch that never wrote the name resolves main's bucket for it).
func (s *ReadModelStore) nameOwnerCandidates(branchID domain.BranchID, nameID uuid.UUID) []uuid.UUID {
	var owners []uuid.UUID
	if owner, ok := s.nameOwners[branchKey{branchID, nameID}]; ok {
		owners = append(owners, owner)
	}
	if branchID != domain.MainBranchID {
		if owner, ok := s.nameOwners[branchKey{domain.MainBranchID, nameID}]; ok {
			owners = append(owners, owner)
		}
	}
	return owners
}

// GetPersonName retrieves a person name by ID within the branch overlay.
func (s *ReadModelStore) GetPersonName(ctx context.Context, branchID domain.BranchID, nameID uuid.UUID) (*repository.PersonNameReadModel, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, names := range resolveAllBuckets(s.personNames, branchID) {
		for _, n := range names {
			if n.ID == nameID {
				result := n
				return &result, nil
			}
		}
	}
	return nil, nil
}

// GetPersonNames retrieves all name variants for a person within the branch overlay.
func (s *ReadModelStore) GetPersonNames(ctx context.Context, branchID domain.BranchID, personID uuid.UUID) ([]repository.PersonNameReadModel, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	names, ok := resolveBucket(s.personNames, branchID, personID)
	if !ok || len(names) == 0 {
		return nil, nil
	}
	result := make([]repository.PersonNameReadModel, len(names))
	copy(result, names)

	// Sort by is_primary DESC, then name_type
	sort.Slice(result, func(i, j int) bool {
		if result[i].IsPrimary != result[j].IsPrimary {
			return result[i].IsPrimary // true comes before false
		}
		return result[i].NameType < result[j].NameType
	})

	return result, nil
}

// DeletePersonName removes a person name within the branch overlay. On a
// non-main branch the owning person's name bucket is copied-on-write before the
// name is removed, so the branch shadows main without mutating it.
func (s *ReadModelStore) DeletePersonName(ctx context.Context, branchID domain.BranchID, nameID uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for personID, names := range resolveAllBuckets(s.personNames, branchID) {
		for _, n := range names {
			if n.ID != nameID {
				continue
			}
			seeded := bucketForWrite(s.personNames, branchID, personID)
			out := seeded[:0]
			for _, sn := range seeded {
				if sn.ID != nameID {
					out = append(out, sn)
				}
			}
			storeBucket(s.personNames, branchID, personID, out)
			return nil
		}
	}
	return nil
}

// ReplacePersonExternalIDs replaces all external identifiers for a person on the
// given branch. On a non-main branch an empty set writes a tombstone bucket so
// the main fallback does not resurrect the identifiers.
func (s *ReadModelStore) ReplacePersonExternalIDs(ctx context.Context, branchID domain.BranchID, personID uuid.UUID, ids []repository.PersonExternalIDReadModel) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(ids) == 0 {
		storeBucket(s.personExternalIDs, branchID, personID, nil)
		return nil
	}
	stored := make([]repository.PersonExternalIDReadModel, len(ids))
	for i, id := range ids {
		id.PersonID = personID
		id.Sequence = i
		stored[i] = id
	}
	storeBucket(s.personExternalIDs, branchID, personID, stored)
	return nil
}

// GetPersonExternalIDs retrieves all external identifiers for a person within the
// branch overlay, ordered by their original sequence.
func (s *ReadModelStore) GetPersonExternalIDs(ctx context.Context, branchID domain.BranchID, personID uuid.UUID) ([]repository.PersonExternalIDReadModel, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	ids, ok := resolveBucket(s.personExternalIDs, branchID, personID)
	if !ok || len(ids) == 0 {
		return nil, nil
	}
	result := make([]repository.PersonExternalIDReadModel, len(ids))
	copy(result, ids)
	sort.Slice(result, func(i, j int) bool {
		return result[i].Sequence < result[j].Sequence
	})
	return result, nil
}

// ReplaceFamilyExternalIDs replaces all external identifiers for a family on the
// given branch. On a non-main branch an empty set writes a tombstone bucket so
// the main fallback does not resurrect the identifiers.
func (s *ReadModelStore) ReplaceFamilyExternalIDs(ctx context.Context, branchID domain.BranchID, familyID uuid.UUID, ids []repository.FamilyExternalIDReadModel) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(ids) == 0 {
		storeBucket(s.familyExternalIDs, branchID, familyID, nil)
		return nil
	}
	stored := make([]repository.FamilyExternalIDReadModel, len(ids))
	for i, id := range ids {
		id.FamilyID = familyID
		id.Sequence = i
		stored[i] = id
	}
	storeBucket(s.familyExternalIDs, branchID, familyID, stored)
	return nil
}

// GetFamilyExternalIDs retrieves all external identifiers for a family within the
// branch overlay, ordered by their original sequence.
func (s *ReadModelStore) GetFamilyExternalIDs(ctx context.Context, branchID domain.BranchID, familyID uuid.UUID) ([]repository.FamilyExternalIDReadModel, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	ids, ok := resolveBucket(s.familyExternalIDs, branchID, familyID)
	if !ok || len(ids) == 0 {
		return nil, nil
	}
	result := make([]repository.FamilyExternalIDReadModel, len(ids))
	copy(result, ids)
	sort.Slice(result, func(i, j int) bool {
		return result[i].Sequence < result[j].Sequence
	})
	return result, nil
}

// ReplaceSourceExternalIDs replaces all external identifiers for a source on the
// given branch (#758). On a non-main branch an empty set writes a tombstone
// bucket so the main fallback does not resurrect the identifiers.
func (s *ReadModelStore) ReplaceSourceExternalIDs(ctx context.Context, branchID domain.BranchID, sourceID uuid.UUID, ids []repository.SourceExternalIDReadModel) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(ids) == 0 {
		storeBucket(s.sourceExternalIDs, branchID, sourceID, nil)
		return nil
	}
	stored := make([]repository.SourceExternalIDReadModel, len(ids))
	for i, id := range ids {
		id.SourceID = sourceID
		id.Sequence = i
		stored[i] = id
	}
	storeBucket(s.sourceExternalIDs, branchID, sourceID, stored)
	return nil
}

// GetSourceExternalIDs retrieves all external identifiers for a source within the
// branch overlay, ordered by their original sequence.
func (s *ReadModelStore) GetSourceExternalIDs(ctx context.Context, branchID domain.BranchID, sourceID uuid.UUID) ([]repository.SourceExternalIDReadModel, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	ids, ok := resolveBucket(s.sourceExternalIDs, branchID, sourceID)
	if !ok || len(ids) == 0 {
		return nil, nil
	}
	result := make([]repository.SourceExternalIDReadModel, len(ids))
	copy(result, ids)
	sort.Slice(result, func(i, j int) bool {
		return result[i].Sequence < result[j].Sequence
	})
	return result, nil
}

// ReplaceRepositoryExternalIDs replaces all external identifiers for a repository.
func (s *ReadModelStore) ReplaceRepositoryExternalIDs(ctx context.Context, repositoryID uuid.UUID, ids []repository.RepositoryExternalIDReadModel) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(ids) == 0 {
		delete(s.repositoryExternalIDs, repositoryID)
		return nil
	}
	stored := make([]repository.RepositoryExternalIDReadModel, len(ids))
	for i, id := range ids {
		id.RepositoryID = repositoryID
		id.Sequence = i
		stored[i] = id
	}
	s.repositoryExternalIDs[repositoryID] = stored
	return nil
}

// GetRepositoryExternalIDs retrieves all external identifiers for a repository,
// ordered by their original sequence.
func (s *ReadModelStore) GetRepositoryExternalIDs(ctx context.Context, repositoryID uuid.UUID) ([]repository.RepositoryExternalIDReadModel, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	ids := s.repositoryExternalIDs[repositoryID]
	if len(ids) == 0 {
		return nil, nil
	}
	result := make([]repository.RepositoryExternalIDReadModel, len(ids))
	copy(result, ids)
	sort.Slice(result, func(i, j int) bool {
		return result[i].Sequence < result[j].Sequence
	})
	return result, nil
}

// GetFamily retrieves a family by ID, resolving the branch overlay.
func (s *ReadModelStore) GetFamily(ctx context.Context, branchID domain.BranchID, id uuid.UUID) (*repository.FamilyReadModel, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	f, ok := resolveRow(s.families, branchID, id)
	if !ok || f == nil {
		return nil, nil
	}
	result := *f
	return &result, nil
}

// GetFamiliesByIDs retrieves every visible family among ids on branchID (#697).
func (s *ReadModelStore) GetFamiliesByIDs(ctx context.Context, branchID domain.BranchID, ids []uuid.UUID) ([]repository.FamilyReadModel, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return resolveRowsByIDs(s.families, branchID, ids), nil
}

// ListFamilies returns a paginated list of families for the requested branch.
func (s *ReadModelStore) ListFamilies(ctx context.Context, opts repository.ListOptions) ([]repository.FamilyReadModel, int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	resolved := resolveAllRows(s.families, opts.BranchID)
	families := make([]repository.FamilyReadModel, 0, len(resolved))
	for _, f := range resolved {
		families = append(families, *f)
	}

	total := len(families)

	// Paginate
	start := opts.Offset
	if start > len(families) {
		start = len(families)
	}
	end := start + opts.Limit
	if end > len(families) {
		end = len(families)
	}

	return families[start:end], total, nil
}

// GetFamiliesForPerson returns all families where the person is a partner,
// resolving the branch overlay.
func (s *ReadModelStore) GetFamiliesForPerson(ctx context.Context, branchID domain.BranchID, personID uuid.UUID) ([]repository.FamilyReadModel, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var results []repository.FamilyReadModel
	for _, f := range resolveAllRows(s.families, branchID) {
		if (f.Partner1ID != nil && *f.Partner1ID == personID) ||
			(f.Partner2ID != nil && *f.Partner2ID == personID) {
			results = append(results, *f)
		}
	}
	return results, nil
}

// SaveFamily saves or updates a family on the given branch.
func (s *ReadModelStore) SaveFamily(ctx context.Context, branchID domain.BranchID, family *repository.FamilyReadModel) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	result := *family
	s.families[branchKey{branchID, family.ID}] = &result
	return nil
}

// DeleteFamily removes a family. On main this is a real removal; on a non-main
// branch it writes tombstones (for the family and its cascaded children/external
// IDs) so the main fallback does not resurrect the entity.
func (s *ReadModelStore) DeleteFamily(ctx context.Context, branchID domain.BranchID, id uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// The family's own life events (owner_type "family") cascade with it (#757),
	// and so does its media (#759).
	for _, e := range resolveAllRows(s.events, branchID) {
		if e.OwnerType == "family" && e.OwnerID == id {
			removeRow(s.events, branchID, e.ID)
		}
	}
	s.cascadeMedia(branchID, "family", id)
	// So do the GPS artifacts whose subject is the family (#760).
	s.cascadeGPS(branchID, id)
	if branchID == domain.MainBranchID {
		delete(s.families, branchKey{domain.MainBranchID, id})
		delete(s.familyChildren, branchKey{domain.MainBranchID, id})
		delete(s.familyExternalIDs, branchKey{domain.MainBranchID, id})
		return nil
	}
	s.families[branchKey{branchID, id}] = nil          // tombstone
	s.familyChildren[branchKey{branchID, id}] = nil    // cascade tombstone
	s.familyExternalIDs[branchKey{branchID, id}] = nil // cascade tombstone
	return nil
}

// GetFamilyChildren returns all children for a family within the branch overlay
// (ADR-005). It is GetFamilyChildrenByFamilyIDs for one family, so the family
// group sheet and the descendancy walk always list siblings in the same order
// (sequence with unsequenced children last, then bytewise surname, given name
// and person id) on every backend.
func (s *ReadModelStore) GetFamilyChildren(ctx context.Context, branchID domain.BranchID, familyID uuid.UUID) ([]repository.FamilyChildReadModel, error) {
	return s.GetFamilyChildrenByFamilyIDs(ctx, branchID, []uuid.UUID{familyID})
}

// ListAllFamilyChildren returns every child link branchID sees, resolving each
// family's children bucket through the overlay as GetFamilyChildren does,
// ordered by family id then person id.
func (s *ReadModelStore) ListAllFamilyChildren(ctx context.Context, branchID domain.BranchID) ([]repository.FamilyChildReadModel, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []repository.FamilyChildReadModel
	for _, children := range resolveAllBuckets(s.familyChildren, branchID) {
		result = append(result, children...)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].FamilyID != result[j].FamilyID {
			return result[i].FamilyID.String() < result[j].FamilyID.String()
		}
		return result[i].PersonID.String() < result[j].PersonID.String()
	})
	return result, nil
}

// GetChildrenOfFamily returns person read models for all children in a family,
// resolving both the children bucket and each person through the branch overlay.
func (s *ReadModelStore) GetChildrenOfFamily(ctx context.Context, branchID domain.BranchID, familyID uuid.UUID) ([]repository.PersonReadModel, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	children, _ := resolveBucket(s.familyChildren, branchID, familyID)
	var result []repository.PersonReadModel
	for _, child := range children {
		if p, ok := resolveRow(s.persons, branchID, child.PersonID); ok && p != nil {
			result = append(result, *p)
		}
	}
	return result, nil
}

// GetChildFamily returns the family where the person is a child, resolving the
// branch overlay.
func (s *ReadModelStore) GetChildFamily(ctx context.Context, branchID domain.BranchID, personID uuid.UUID) (*repository.FamilyReadModel, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for familyID, children := range resolveAllBuckets(s.familyChildren, branchID) {
		for _, child := range children {
			if child.PersonID == personID {
				if f, ok := resolveRow(s.families, branchID, familyID); ok && f != nil {
					result := *f
					return &result, nil
				}
			}
		}
	}
	return nil, nil
}

// SaveFamilyChild saves a family child relationship on the given branch. On a
// non-main branch the family's children bucket is copied-on-write from main
// before the mutation is applied.
func (s *ReadModelStore) SaveFamilyChild(ctx context.Context, branchID domain.BranchID, child *repository.FamilyChildReadModel) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	children := bucketForWrite(s.familyChildren, branchID, child.FamilyID)
	updated := false
	for i := range children {
		if children[i].PersonID == child.PersonID {
			children[i] = *child
			updated = true
			break
		}
	}
	if !updated {
		children = append(children, *child)
	}
	storeBucket(s.familyChildren, branchID, child.FamilyID, children)
	return nil
}

// DeleteFamilyChild removes a family child relationship within the branch
// overlay. It is a no-op when the child is absent; otherwise, on a non-main
// branch the family's children bucket is copied-on-write before removal.
func (s *ReadModelStore) DeleteFamilyChild(ctx context.Context, branchID domain.BranchID, familyID, personID uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	children := bucketForWrite(s.familyChildren, branchID, familyID)
	out := children[:0]
	removed := false
	for _, c := range children {
		if c.PersonID == personID {
			removed = true
			continue
		}
		out = append(out, c)
	}
	if !removed {
		return nil
	}
	storeBucket(s.familyChildren, branchID, familyID, out)
	return nil
}

// GetPedigreeEdge returns the pedigree edge for a person, resolving the branch overlay.
func (s *ReadModelStore) GetPedigreeEdge(ctx context.Context, branchID domain.BranchID, personID uuid.UUID) (*repository.PedigreeEdge, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	edge, ok := resolveRow(s.pedigreeEdges, branchID, personID)
	if !ok || edge == nil {
		return nil, nil
	}
	result := *edge
	return &result, nil
}

// SavePedigreeEdge saves a pedigree edge on the given branch.
func (s *ReadModelStore) SavePedigreeEdge(ctx context.Context, branchID domain.BranchID, edge *repository.PedigreeEdge) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	result := *edge
	s.pedigreeEdges[branchKey{branchID, edge.PersonID}] = &result
	return nil
}

// DeletePedigreeEdge removes a pedigree edge. On main this is a real removal; on
// a non-main branch it writes a tombstone so the main fallback does not
// resurrect the edge.
func (s *ReadModelStore) DeletePedigreeEdge(ctx context.Context, branchID domain.BranchID, personID uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if branchID == domain.MainBranchID {
		delete(s.pedigreeEdges, branchKey{domain.MainBranchID, personID})
		return nil
	}
	s.pedigreeEdges[branchKey{branchID, personID}] = nil // tombstone
	return nil
}

// PurgeBranch hard-deletes every map entry keyed to branchID across the
// branch-scoped entities (the seven #669 slice entities, life events,
// attributes and associations (#757), and sources, source external IDs,
// citations and notes (#758), media (#759) and the GPS artifacts (#760)). It is
// a no-op for the mainline (domain.MainBranchID), which is never purged. See
// ADR-005 and the branch-delete projection handler.
func (s *ReadModelStore) PurgeBranch(ctx context.Context, branchID domain.BranchID) error {
	if branchID == domain.MainBranchID {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	deleteBranchRows(s.persons, branchID)
	deleteBranchRows(s.personNames, branchID)
	deleteBranchRows(s.nameOwners, branchID)
	deleteBranchRows(s.personExternalIDs, branchID)
	deleteBranchRows(s.families, branchID)
	deleteBranchRows(s.familyExternalIDs, branchID)
	deleteBranchRows(s.familyChildren, branchID)
	deleteBranchRows(s.pedigreeEdges, branchID)
	deleteBranchRows(s.events, branchID)
	deleteBranchRows(s.attributes, branchID)
	deleteBranchRows(s.associations, branchID)
	deleteBranchRows(s.sources, branchID)
	deleteBranchRows(s.sourceExternalIDs, branchID)
	deleteBranchRows(s.citations, branchID)
	deleteBranchRows(s.notes, branchID)
	deleteBranchRows(s.media, branchID)
	deleteBranchRows(s.evidenceAnalyses, branchID)
	deleteBranchRows(s.evidenceConflicts, branchID)
	deleteBranchRows(s.researchLogs, branchID)
	deleteBranchRows(s.proofSummaries, branchID)
	// Drop main media tombstones kept alive only for this branch's shadows (#759).
	s.collectMainMediaTombstones()
	return nil
}

// deleteBranchRows removes every entry of a branch-keyed slice map whose key
// belongs to branch. It works for both single-row (*T) and bucket ([]T) slice
// maps because it only inspects the key.
func deleteBranchRows[V any](m map[branchKey]V, branch domain.BranchID) {
	for k := range m {
		if k.branch == branch {
			delete(m, k)
		}
	}
}

// Reset clears all data.
func (s *ReadModelStore) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.persons = make(map[branchKey]*repository.PersonReadModel)
	s.personNames = make(map[branchKey][]repository.PersonNameReadModel)
	s.nameOwners = make(map[branchKey]uuid.UUID)
	s.families = make(map[branchKey]*repository.FamilyReadModel)
	s.familyChildren = make(map[branchKey][]repository.FamilyChildReadModel)
	s.pedigreeEdges = make(map[branchKey]*repository.PedigreeEdge)
	s.sources = make(map[branchKey]*repository.SourceReadModel)
	s.sourceExternalIDs = make(map[branchKey][]repository.SourceExternalIDReadModel)
	s.citations = make(map[branchKey]*repository.CitationReadModel)
	s.media = make(map[branchKey]*mediaRow)
	s.events = make(map[branchKey]*repository.EventReadModel)
	s.attributes = make(map[branchKey]*repository.AttributeReadModel)
	s.notes = make(map[branchKey]*repository.NoteReadModel)
	s.submitters = make(map[uuid.UUID]*repository.SubmitterReadModel)
	s.repositories = make(map[uuid.UUID]*repository.RepositoryReadModel)
	s.associations = make(map[branchKey]*repository.AssociationReadModel)
	s.ldsOrdinances = make(map[uuid.UUID]*repository.LDSOrdinanceReadModel)
	s.evidenceAnalyses = make(map[branchKey]*repository.EvidenceAnalysisReadModel)
	s.evidenceConflicts = make(map[branchKey]*repository.EvidenceConflictReadModel)
	s.researchLogs = make(map[branchKey]*repository.ResearchLogReadModel)
	s.proofSummaries = make(map[branchKey]*repository.ProofSummaryReadModel)
}

// GetSource retrieves a source by ID within the branch overlay (ADR-005, #758).
func (s *ReadModelStore) GetSource(ctx context.Context, branchID domain.BranchID, id uuid.UUID) (*repository.SourceReadModel, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	src, _ := resolveRow(s.sources, branchID, id)
	if src == nil {
		return nil, nil
	}
	result := *src
	return &result, nil
}

// GetSourcesByIDs retrieves every visible source among ids on branchID (#697).
func (s *ReadModelStore) GetSourcesByIDs(ctx context.Context, branchID domain.BranchID, ids []uuid.UUID) ([]repository.SourceReadModel, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return resolveRowsByIDs(s.sources, branchID, ids), nil
}

// compareSources orders sources by title, then id — the order the SQL backends
// return them in.
func compareSources(a, b *repository.SourceReadModel) int {
	if cmp := strings.Compare(a.Title, b.Title); cmp != 0 {
		return cmp
	}
	return strings.Compare(a.ID.String(), b.ID.String())
}

// ListSources returns a paginated list of the sources visible on opts.BranchID.
func (s *ReadModelStore) ListSources(ctx context.Context, opts repository.ListOptions) ([]repository.SourceReadModel, int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	resolved := resolveAllRows(s.sources, opts.BranchID)
	sources := make([]repository.SourceReadModel, 0, len(resolved))
	for _, src := range resolved {
		sources = append(sources, *src)
	}

	// Sort by title (id breaks ties)
	sort.Slice(sources, func(i, j int) bool {
		cmp := compareSources(&sources[i], &sources[j])
		if opts.Order == "desc" {
			return cmp > 0
		}
		return cmp < 0
	})

	total := len(sources)
	start, end := pageBounds(len(sources), opts)
	return sources[start:end], total, nil
}

// SearchSources searches the sources visible on branchID by title or author
// (case-insensitive substring). The overlay is resolved first and the query is
// matched against the winning rows, so a branch retitle is found under its new
// title only and a branch-deleted source is never returned.
func (s *ReadModelStore) SearchSources(ctx context.Context, branchID domain.BranchID, query string, limit int) ([]repository.SourceReadModel, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query = strings.ToLower(query)
	var results []repository.SourceReadModel
	for _, src := range resolveAllRows(s.sources, branchID) {
		if strings.Contains(strings.ToLower(src.Title), query) || strings.Contains(strings.ToLower(src.Author), query) {
			results = append(results, *src)
		}
	}
	sort.Slice(results, func(i, j int) bool { return compareSources(&results[i], &results[j]) < 0 })
	if limit >= 0 && len(results) > limit {
		results = results[:limit]
	}
	return results, nil
}

// SaveSource saves or updates a source on the given branch (ADR-005, #758).
func (s *ReadModelStore) SaveSource(ctx context.Context, branchID domain.BranchID, source *repository.SourceReadModel) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	result := *source
	s.sources[branchKey{branchID, source.ID}] = &result
	return nil
}

// DeleteSource removes a source on the given branch: a real removal on main, a
// tombstone on a non-main branch. The source's external identifiers and its
// citations and media go with it on the same branch (#758, #759) — the manual cascade that
// replaces the dropped foreign keys; other branches are untouched.
func (s *ReadModelStore) DeleteSource(ctx context.Context, branchID domain.BranchID, id uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, c := range resolveAllRows(s.citations, branchID) {
		if c.SourceID == id {
			removeRow(s.citations, branchID, c.ID)
		}
	}
	s.cascadeMedia(branchID, "source", id)
	storeBucket(s.sourceExternalIDs, branchID, id, nil)
	removeRow(s.sources, branchID, id)
	return nil
}

// GetCitation retrieves a citation by ID within the branch overlay (ADR-005, #758).
func (s *ReadModelStore) GetCitation(ctx context.Context, branchID domain.BranchID, id uuid.UUID) (*repository.CitationReadModel, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	cit, _ := resolveRow(s.citations, branchID, id)
	if cit == nil {
		return nil, nil
	}
	result := *cit
	return &result, nil
}

// GetCitationsByIDs retrieves every visible citation among ids on branchID (#697).
func (s *ReadModelStore) GetCitationsByIDs(ctx context.Context, branchID domain.BranchID, ids []uuid.UUID) ([]repository.CitationReadModel, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return resolveRowsByIDs(s.citations, branchID, ids), nil
}

// compareCitations orders citations by source title, then fact type, then id —
// the order the SQL backends return them in.
func compareCitations(a, b *repository.CitationReadModel) int {
	cmp := strings.Compare(a.SourceTitle, b.SourceTitle)
	if cmp == 0 {
		cmp = strings.Compare(string(a.FactType), string(b.FactType))
	}
	if cmp == 0 {
		cmp = strings.Compare(a.ID.String(), b.ID.String())
	}
	return cmp
}

// ListCitations returns the citations visible on opts.BranchID with pagination.
func (s *ReadModelStore) ListCitations(ctx context.Context, opts repository.ListOptions) ([]repository.CitationReadModel, int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	resolved := resolveAllRows(s.citations, opts.BranchID)
	citations := make([]repository.CitationReadModel, 0, len(resolved))
	for _, cit := range resolved {
		citations = append(citations, *cit)
	}

	// Sort by source title, then by fact type, then by ID for deterministic ordering
	sort.Slice(citations, func(i, j int) bool {
		cmp := compareCitations(&citations[i], &citations[j])
		if opts.Order == "desc" {
			return cmp > 0
		}
		return cmp < 0
	})

	total := len(citations)
	start, end := pageBounds(len(citations), opts)
	return citations[start:end], total, nil
}

// filterCitations returns the citations visible on branchID that keep accepts,
// in compareCitations order. The filter is applied to the winning row, so a
// branch that re-pointed a citation lists it under its new source/owner only.
// Callers hold s.mu.
func (s *ReadModelStore) filterCitations(branchID domain.BranchID, keep func(*repository.CitationReadModel) bool) []repository.CitationReadModel {
	var results []repository.CitationReadModel
	for _, cit := range resolveAllRows(s.citations, branchID) {
		if keep(cit) {
			results = append(results, *cit)
		}
	}
	sort.Slice(results, func(i, j int) bool { return compareCitations(&results[i], &results[j]) < 0 })
	return results
}

// GetCitationsForSource returns all citations of a source within the branch overlay.
func (s *ReadModelStore) GetCitationsForSource(ctx context.Context, branchID domain.BranchID, sourceID uuid.UUID) ([]repository.CitationReadModel, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.filterCitations(branchID, func(c *repository.CitationReadModel) bool {
		return c.SourceID == sourceID
	}), nil
}

// CountCitationsBySource counts the visible citations of each of sourceIDs on
// branchID (see the interface).
func (s *ReadModelStore) CountCitationsBySource(ctx context.Context, branchID domain.BranchID, sourceIDs []uuid.UUID) (map[uuid.UUID]int, error) {
	counts := make(map[uuid.UUID]int)
	if len(sourceIDs) == 0 {
		return counts, nil
	}
	wanted := make(map[uuid.UUID]bool, len(sourceIDs))
	for _, id := range sourceIDs {
		wanted[id] = true
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, cit := range resolveAllRows(s.citations, branchID) {
		if wanted[cit.SourceID] {
			counts[cit.SourceID]++
		}
	}
	return counts, nil
}

// GetCitationsForPerson returns all citations of a person's facts within the
// branch overlay.
func (s *ReadModelStore) GetCitationsForPerson(ctx context.Context, branchID domain.BranchID, personID uuid.UUID) ([]repository.CitationReadModel, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.filterCitations(branchID, func(c *repository.CitationReadModel) bool {
		return c.FactOwnerID == personID && strings.HasPrefix(string(c.FactType), "person_")
	}), nil
}

// GetCitationsForFact returns all citations of a specific fact within the branch
// overlay.
func (s *ReadModelStore) GetCitationsForFact(ctx context.Context, branchID domain.BranchID, factType domain.FactType, factOwnerID uuid.UUID) ([]repository.CitationReadModel, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.filterCitations(branchID, func(c *repository.CitationReadModel) bool {
		return c.FactType == factType && c.FactOwnerID == factOwnerID
	}), nil
}

// SaveCitation saves or updates a citation on the given branch (ADR-005, #758).
func (s *ReadModelStore) SaveCitation(ctx context.Context, branchID domain.BranchID, citation *repository.CitationReadModel) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	result := *citation
	s.citations[branchKey{branchID, citation.ID}] = &result
	return nil
}

// DeleteCitation removes a citation: a real removal on main, a tombstone on a
// non-main branch. Other branches' rows are untouched.
func (s *ReadModelStore) DeleteCitation(ctx context.Context, branchID domain.BranchID, id uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	removeRow(s.citations, branchID, id)
	return nil
}

// mediaRow is one stored media row (ADR-005, #759). Unlike the other
// single-row maps, a tombstone is a row with deleted set rather than a nil
// entry, because a MAIN tombstone must keep its bytes while a branch shadow
// still borrows them (the blob rule on repository.ReadModelStore). FileData and
// ThumbnailData on a branch shadow row of a mainline item are always nil.
type mediaRow struct {
	m       repository.MediaReadModel
	deleted bool
}

// mediaWinner returns the row branch resolves for id — its own row, else main's
// — or nil when neither exists. Callers hold s.mu.
func (s *ReadModelStore) mediaWinner(branch domain.BranchID, id uuid.UUID) *mediaRow {
	if r, ok := s.media[branchKey{branch, id}]; ok {
		return r
	}
	if branch != domain.MainBranchID {
		return s.media[branchKey{domain.MainBranchID, id}]
	}
	return nil
}

// visibleMedia returns every live media row branch sees, one per id. Callers
// hold s.mu.
func (s *ReadModelStore) visibleMedia(branch domain.BranchID) []*mediaRow {
	out := make([]*mediaRow, 0)
	seen := make(map[uuid.UUID]bool)
	for k := range s.media {
		if (k.branch != branch && k.branch != domain.MainBranchID) || seen[k.id] {
			continue
		}
		seen[k.id] = true
		if r := s.mediaWinner(branch, k.id); r != nil && !r.deleted {
			out = append(out, r)
		}
	}
	return out
}

// metadataOnly returns a copy of a stored row without its bytes.
func metadataOnly(r *mediaRow) *repository.MediaReadModel {
	result := r.m
	result.FileData = nil
	result.ThumbnailData = nil
	return &result
}

// GetMedia retrieves media metadata by ID within the branch overlay (ADR-005,
// #759); FileData and ThumbnailData are left empty.
func (s *ReadModelStore) GetMedia(ctx context.Context, branchID domain.BranchID, id uuid.UUID) (*repository.MediaReadModel, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	r := s.mediaWinner(branchID, id)
	if r == nil || r.deleted {
		return nil, nil
	}
	return metadataOnly(r), nil
}

// sharedMediaBytes returns the bytes branch reads for the winning row w: w's
// own, else main's (#759). Callers hold s.mu.
func (s *ReadModelStore) sharedMediaBytes(w *mediaRow, id uuid.UUID) (file, thumb []byte) {
	file, thumb = w.m.FileData, w.m.ThumbnailData
	if main, ok := s.media[branchKey{domain.MainBranchID, id}]; ok && main != w {
		if file == nil {
			file = main.m.FileData
		}
		if thumb == nil {
			thumb = main.m.ThumbnailData
		}
	}
	return file, thumb
}

// GetMediaWithData retrieves the full media record within the branch overlay:
// the winning row's metadata and the shared bytes (#759).
func (s *ReadModelStore) GetMediaWithData(ctx context.Context, branchID domain.BranchID, id uuid.UUID) (*repository.MediaReadModel, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	r := s.mediaWinner(branchID, id)
	if r == nil || r.deleted {
		return nil, nil
	}
	result := r.m
	result.FileData, result.ThumbnailData = s.sharedMediaBytes(r, id)
	return &result, nil
}

// GetMediaThumbnail retrieves just the thumbnail bytes of a media item visible
// on branchID (#759).
func (s *ReadModelStore) GetMediaThumbnail(ctx context.Context, branchID domain.BranchID, id uuid.UUID) ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	r := s.mediaWinner(branchID, id)
	if r == nil || r.deleted {
		return nil, nil
	}
	_, thumb := s.sharedMediaBytes(r, id)
	return thumb, nil
}

// ListMediaForEntity returns a paginated list of the media attached to an
// entity as opts.BranchID sees it (ADR-005, #759), newest first; the entity
// filter is decided on each item's winning row.
func (s *ReadModelStore) ListMediaForEntity(ctx context.Context, entityType string, entityID uuid.UUID, opts repository.ListOptions) ([]repository.MediaReadModel, int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var results []repository.MediaReadModel
	for _, r := range s.visibleMedia(opts.BranchID) {
		if r.m.EntityType == entityType && r.m.EntityID == entityID {
			results = append(results, *metadataOnly(r))
		}
	}

	total := len(results)

	// Sort by created_at DESC, id DESC (the SQL backends' order).
	sort.Slice(results, func(i, j int) bool {
		cmp := compareTimestamps(results[i].CreatedAt, results[j].CreatedAt)
		if cmp == 0 {
			cmp = strings.Compare(results[i].ID.String(), results[j].ID.String())
		}
		return cmp > 0
	})

	// Apply pagination
	if opts.Offset > 0 {
		if opts.Offset >= len(results) {
			results = nil
		} else {
			results = results[opts.Offset:]
		}
	}

	if opts.Limit > 0 && len(results) > opts.Limit {
		results = results[:opts.Limit]
	}

	return results, total, nil
}

// SaveMedia saves or updates a media record on the given branch (ADR-005,
// #759), clearing any prior tombstone. It enforces the blob rule: on a non-main
// branch an id that has a main row stores no bytes whatever the caller passes,
// and nil bytes never overwrite stored ones.
func (s *ReadModelStore) SaveMedia(ctx context.Context, branchID domain.BranchID, media *repository.MediaReadModel) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := branchKey{branchID, media.ID}
	row := &mediaRow{m: *media}
	_, hasMain := s.media[branchKey{domain.MainBranchID, media.ID}]
	prev, hasOwn := s.media[key]
	// A byte-less branch save of an id with no row anywhere is a metadata edit
	// of an item main has since deleted: store nothing rather than a shadow
	// with no bytes (parity with the SQL backends).
	if branchID != domain.MainBranchID && len(media.FileData) == 0 && !hasMain && !hasOwn {
		return nil
	}
	if branchID != domain.MainBranchID && hasMain {
		row.m.FileData, row.m.ThumbnailData = nil, nil
	}
	if hasOwn {
		if row.m.FileData == nil {
			row.m.FileData = prev.m.FileData
		}
		if row.m.ThumbnailData == nil {
			row.m.ThumbnailData = prev.m.ThumbnailData
		}
	}
	s.media[key] = row
	if branchID == domain.MainBranchID {
		s.releaseBranchMediaBytes(media.ID, row)
	}
	return nil
}

// releaseBranchMediaBytes clears the byte columns of every branch row of id
// once main's row holds the same bytes — a branch upload merged into main —
// so the bytes are stored exactly once and the branch reads them through the
// main fallback (#759). Callers hold s.mu.
func (s *ReadModelStore) releaseBranchMediaBytes(id uuid.UUID, main *mediaRow) {
	for k, r := range s.media {
		if k.id != id || k.branch == domain.MainBranchID {
			continue
		}
		if main.m.FileData != nil {
			r.m.FileData = nil
		}
		if main.m.ThumbnailData != nil {
			r.m.ThumbnailData = nil
		}
	}
}

// DeleteMedia removes a media item on the given branch (ADR-005, #759): a
// metadata-only tombstone off main; on main a real removal, unless a branch
// still shows the item through a live shadow row, in which case main's row
// stays as a tombstone that keeps the shared bytes.
func (s *ReadModelStore) DeleteMedia(ctx context.Context, branchID domain.BranchID, id uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.removeMedia(branchID, id)
	s.collectMainMediaTombstones()
	return nil
}

// cascadeMedia removes, on branch, every media item the branch sees attached
// to (entityType, entityID) — the manual cascade of DeletePerson, DeleteFamily
// and DeleteSource (#759). Callers hold s.mu.
func (s *ReadModelStore) cascadeMedia(branch domain.BranchID, entityType string, entityID uuid.UUID) {
	for _, r := range s.visibleMedia(branch) {
		if r.m.EntityType == entityType && r.m.EntityID == entityID {
			s.removeMedia(branch, r.m.ID)
		}
	}
	s.collectMainMediaTombstones()
}

// removeMedia deletes one media id on branch under the blob rule. Callers hold
// s.mu.
func (s *ReadModelStore) removeMedia(branch domain.BranchID, id uuid.UUID) {
	if branch == domain.MainBranchID {
		main, ok := s.media[branchKey{domain.MainBranchID, id}]
		if !ok {
			return
		}
		if s.hasLiveMediaShadow(id, domain.MainBranchID) {
			main.deleted = true
			return
		}
		delete(s.media, branchKey{domain.MainBranchID, id})
		return
	}
	w := s.mediaWinner(branch, id)
	if w == nil || w.deleted {
		return // nothing visible to hide
	}
	key := branchKey{branch, id}
	if own, ok := s.media[key]; ok {
		own.deleted = true
		return
	}
	s.media[key] = &mediaRow{m: *metadataOnly(w), deleted: true}
}

// hasLiveMediaShadow reports whether a non-main branch other than except shows
// id through a live row of its own. Callers hold s.mu.
func (s *ReadModelStore) hasLiveMediaShadow(id uuid.UUID, except domain.BranchID) bool {
	for k, r := range s.media {
		if k.id == id && k.branch != domain.MainBranchID && k.branch != except && !r.deleted {
			return true
		}
	}
	return false
}

// collectMainMediaTombstones drops every main media tombstone that no live
// branch shadow needs any more. Callers hold s.mu.
func (s *ReadModelStore) collectMainMediaTombstones() {
	for k, r := range s.media {
		if k.branch == domain.MainBranchID && r.deleted && !s.hasLiveMediaShadow(k.id, domain.MainBranchID) {
			delete(s.media, k)
		}
	}
}

// GetEvent retrieves a life event by ID within the branch overlay (ADR-005).
func (s *ReadModelStore) GetEvent(ctx context.Context, branchID domain.BranchID, id uuid.UUID) (*repository.EventReadModel, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	e, _ := resolveRow(s.events, branchID, id)
	if e == nil {
		return nil, nil
	}
	eventCopy := *e
	return &eventCopy, nil
}

// compareEvents orders life events by fact type, then date (undated last), then
// id — the order the SQL backends return them in.
func compareEvents(a, b *repository.EventReadModel) int {
	cmp := strings.Compare(string(a.FactType), string(b.FactType))
	if cmp == 0 {
		switch {
		case a.DateSort != nil && b.DateSort != nil:
			cmp = a.DateSort.Compare(*b.DateSort)
		case a.DateSort == nil && b.DateSort != nil:
			cmp = 1 // nil dates sort after non-nil
		case a.DateSort != nil && b.DateSort == nil:
			cmp = -1
		}
	}
	if cmp == 0 {
		cmp = strings.Compare(a.ID.String(), b.ID.String())
	}
	return cmp
}

// listOwnerEvents returns the life events of one owner visible on branchID,
// in compareEvents order.
func (s *ReadModelStore) listOwnerEvents(branchID domain.BranchID, ownerType string, ownerID uuid.UUID) []repository.EventReadModel {
	var results []repository.EventReadModel
	for _, e := range resolveAllRows(s.events, branchID) {
		if e.OwnerType == ownerType && e.OwnerID == ownerID {
			results = append(results, *e)
		}
	}
	sort.Slice(results, func(i, j int) bool { return compareEvents(&results[i], &results[j]) < 0 })
	return results
}

// ListEventsForPerson returns all life events of a person within the branch overlay.
func (s *ReadModelStore) ListEventsForPerson(ctx context.Context, branchID domain.BranchID, personID uuid.UUID) ([]repository.EventReadModel, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.listOwnerEvents(branchID, "person", personID), nil
}

// ListEventsForFamily returns all life events of a family within the branch overlay.
func (s *ReadModelStore) ListEventsForFamily(ctx context.Context, branchID domain.BranchID, familyID uuid.UUID) ([]repository.EventReadModel, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.listOwnerEvents(branchID, "family", familyID), nil
}

// ListEvents returns the life events visible on opts.BranchID with pagination.
func (s *ReadModelStore) ListEvents(ctx context.Context, opts repository.ListOptions) ([]repository.EventReadModel, int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	resolved := resolveAllRows(s.events, opts.BranchID)
	events := make([]repository.EventReadModel, 0, len(resolved))
	for _, e := range resolved {
		events = append(events, *e)
	}

	// Sort by fact type, then by date, then by ID for deterministic ordering
	sort.Slice(events, func(i, j int) bool {
		cmp := compareEvents(&events[i], &events[j])
		if opts.Order == "desc" {
			return cmp > 0
		}
		return cmp < 0
	})

	total := len(events)
	start, end := pageBounds(len(events), opts)
	return events[start:end], total, nil
}

// pageBounds clamps opts' offset/limit window to a slice of length n.
func pageBounds(n int, opts repository.ListOptions) (start, end int) {
	start = opts.Offset
	if start > n {
		start = n
	}
	if start < 0 {
		start = 0
	}
	end = start + opts.Limit
	if end > n || opts.Limit < 0 {
		end = n
	}
	return start, end
}

// SaveEvent saves or updates a life event on the given branch (ADR-005).
func (s *ReadModelStore) SaveEvent(ctx context.Context, branchID domain.BranchID, event *repository.EventReadModel) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	eventCopy := *event
	s.events[branchKey{branchID, event.ID}] = &eventCopy
	return nil
}

// DeleteEvent removes a life event: a real removal on main, a tombstone on a
// non-main branch so the main fallback does not resurrect it.
func (s *ReadModelStore) DeleteEvent(ctx context.Context, branchID domain.BranchID, id uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	removeRow(s.events, branchID, id)
	return nil
}

// GetAttribute retrieves an attribute by ID within the branch overlay (ADR-005).
func (s *ReadModelStore) GetAttribute(ctx context.Context, branchID domain.BranchID, id uuid.UUID) (*repository.AttributeReadModel, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	a, _ := resolveRow(s.attributes, branchID, id)
	if a == nil {
		return nil, nil
	}
	attrCopy := *a
	return &attrCopy, nil
}

// compareAttributes orders attributes by fact type, then value, then id — the
// order the SQL backends return them in.
func compareAttributes(a, b *repository.AttributeReadModel) int {
	cmp := strings.Compare(string(a.FactType), string(b.FactType))
	if cmp == 0 {
		cmp = strings.Compare(a.Value, b.Value)
	}
	if cmp == 0 {
		cmp = strings.Compare(a.ID.String(), b.ID.String())
	}
	return cmp
}

// ListAttributesForPerson returns all attributes of a person within the branch overlay.
func (s *ReadModelStore) ListAttributesForPerson(ctx context.Context, branchID domain.BranchID, personID uuid.UUID) ([]repository.AttributeReadModel, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var results []repository.AttributeReadModel
	for _, a := range resolveAllRows(s.attributes, branchID) {
		if a.PersonID == personID {
			results = append(results, *a)
		}
	}
	sort.Slice(results, func(i, j int) bool { return compareAttributes(&results[i], &results[j]) < 0 })
	return results, nil
}

// ListAttributes returns the attributes visible on opts.BranchID with pagination.
func (s *ReadModelStore) ListAttributes(ctx context.Context, opts repository.ListOptions) ([]repository.AttributeReadModel, int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	resolved := resolveAllRows(s.attributes, opts.BranchID)
	attributes := make([]repository.AttributeReadModel, 0, len(resolved))
	for _, a := range resolved {
		attributes = append(attributes, *a)
	}

	// Sort by fact type, then by value, then by ID for deterministic ordering
	sort.Slice(attributes, func(i, j int) bool {
		cmp := compareAttributes(&attributes[i], &attributes[j])
		if opts.Order == "desc" {
			return cmp > 0
		}
		return cmp < 0
	})

	total := len(attributes)
	start, end := pageBounds(len(attributes), opts)
	return attributes[start:end], total, nil
}

// SaveAttribute saves or updates an attribute on the given branch (ADR-005).
func (s *ReadModelStore) SaveAttribute(ctx context.Context, branchID domain.BranchID, attribute *repository.AttributeReadModel) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	attrCopy := *attribute
	s.attributes[branchKey{branchID, attribute.ID}] = &attrCopy
	return nil
}

// DeleteAttribute removes an attribute: a real removal on main, a tombstone on a
// non-main branch.
func (s *ReadModelStore) DeleteAttribute(ctx context.Context, branchID domain.BranchID, id uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	removeRow(s.attributes, branchID, id)
	return nil
}

// GetSurnameIndex returns a list of unique surnames with counts and letter
// distribution, aggregated over branchID's overlay of persons (ADR-005).
func (s *ReadModelStore) GetSurnameIndex(ctx context.Context, branchID domain.BranchID) ([]repository.SurnameEntry, []repository.LetterCount, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	surnameCount := make(map[string]int)
	surnamesByLetter := make(map[string]map[string]bool) // letter -> set of surnames

	for _, p := range resolveAllRows(s.persons, branchID) {
		surname := p.Surname
		surnameCount[surname]++
		if surname != "" {
			letter := strings.ToUpper(string(surname[0]))
			if surnamesByLetter[letter] == nil {
				surnamesByLetter[letter] = make(map[string]bool)
			}
			surnamesByLetter[letter][surname] = true
		}
	}

	surnames := make([]repository.SurnameEntry, 0, len(surnameCount))
	for name, count := range surnameCount {
		surnames = append(surnames, repository.SurnameEntry{Surname: name, Count: count})
	}
	sort.Slice(surnames, func(i, j int) bool {
		return surnames[i].Surname < surnames[j].Surname
	})

	letters := make([]repository.LetterCount, 0, len(surnamesByLetter))
	for letter, surnameSet := range surnamesByLetter {
		letters = append(letters, repository.LetterCount{Letter: letter, Count: len(surnameSet)})
	}
	sort.Slice(letters, func(i, j int) bool {
		return letters[i].Letter < letters[j].Letter
	})

	return surnames, letters, nil
}

// GetSurnamesByLetter returns surnames starting with a specific letter, aggregated
// over branchID's overlay of persons (ADR-005).
func (s *ReadModelStore) GetSurnamesByLetter(ctx context.Context, branchID domain.BranchID, letter string) ([]repository.SurnameEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	surnameCount := make(map[string]int)
	for _, p := range resolveAllRows(s.persons, branchID) {
		surname := p.Surname
		if surname != "" && strings.EqualFold(string(surname[0]), letter) {
			surnameCount[surname]++
		}
	}

	surnames := make([]repository.SurnameEntry, 0, len(surnameCount))
	for name, count := range surnameCount {
		surnames = append(surnames, repository.SurnameEntry{Surname: name, Count: count})
	}
	sort.Slice(surnames, func(i, j int) bool {
		return surnames[i].Surname < surnames[j].Surname
	})

	return surnames, nil
}

// GetPersonsBySurname returns persons with a specific surname from opts.BranchID's
// overlay of persons (ADR-005).
func (s *ReadModelStore) GetPersonsBySurname(ctx context.Context, surname string, opts repository.ListOptions) ([]repository.PersonReadModel, int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var results []repository.PersonReadModel
	for _, p := range resolveAllRows(s.persons, opts.BranchID) {
		if strings.EqualFold(p.Surname, surname) {
			results = append(results, *p)
		}
	}

	total := len(results)

	// Sort by GivenName
	sort.Slice(results, func(i, j int) bool {
		return results[i].GivenName < results[j].GivenName
	})

	// Apply pagination
	if opts.Offset > 0 && opts.Offset < len(results) {
		results = results[opts.Offset:]
	} else if opts.Offset >= len(results) {
		results = nil
	}
	if opts.Limit > 0 && opts.Limit < len(results) {
		results = results[:opts.Limit]
	}

	return results, total, nil
}

// GetPlaceHierarchy returns places at a given level of hierarchy, aggregated over
// branchID's overlay of persons (ADR-005).
func (s *ReadModelStore) GetPlaceHierarchy(ctx context.Context, branchID domain.BranchID, parent string) ([]repository.PlaceEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Simple implementation: extract unique places
	placeCount := make(map[string]int)

	for _, p := range resolveAllRows(s.persons, branchID) {
		for _, place := range []string{p.BirthPlace, p.DeathPlace} {
			if place != "" {
				placeCount[place]++
			}
		}
	}

	entries := make([]repository.PlaceEntry, 0, len(placeCount))
	for place, count := range placeCount {
		entries = append(entries, repository.PlaceEntry{
			Name:     place,
			FullName: place,
			Count:    count,
		})
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name < entries[j].Name
	})

	return entries, nil
}

// GetPersonsByPlace returns persons associated with a specific place from
// opts.BranchID's overlay of persons (ADR-005).
func (s *ReadModelStore) GetPersonsByPlace(ctx context.Context, place string, opts repository.ListOptions) ([]repository.PersonReadModel, int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var results []repository.PersonReadModel
	for _, p := range resolveAllRows(s.persons, opts.BranchID) {
		if strings.Contains(p.BirthPlace, place) || strings.Contains(p.DeathPlace, place) {
			results = append(results, *p)
		}
	}

	total := len(results)

	// Sort by surname
	sort.Slice(results, func(i, j int) bool {
		if results[i].Surname != results[j].Surname {
			return results[i].Surname < results[j].Surname
		}
		return results[i].GivenName < results[j].GivenName
	})

	// Apply pagination
	if opts.Offset > 0 && opts.Offset < len(results) {
		results = results[opts.Offset:]
	} else if opts.Offset >= len(results) {
		results = nil
	}
	if opts.Limit > 0 && opts.Limit < len(results) {
		results = results[:opts.Limit]
	}

	return results, total, nil
}

// GetCemeteryIndex returns burial/cremation places with the number of distinct
// owners placed there, counted over branchID's overlay of life events (ADR-005).
// A branch that tombstoned a person also tombstoned that person's life events
// (DeletePerson's cascade), so the index agrees with GetPersonsByCemetery.
func (s *ReadModelStore) GetCemeteryIndex(ctx context.Context, branchID domain.BranchID) ([]repository.CemeteryEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Count distinct persons per place for burial/cremation events
	placePersons := make(map[string]map[uuid.UUID]struct{})
	for _, e := range resolveAllRows(s.events, branchID) {
		if e.Place == "" {
			continue
		}
		if e.FactType != domain.FactPersonBurial && e.FactType != domain.FactPersonCremation {
			continue
		}
		if _, ok := placePersons[e.Place]; !ok {
			placePersons[e.Place] = make(map[uuid.UUID]struct{})
		}
		placePersons[e.Place][e.OwnerID] = struct{}{}
	}

	entries := make([]repository.CemeteryEntry, 0, len(placePersons))
	for place, persons := range placePersons {
		entries = append(entries, repository.CemeteryEntry{
			Place: place,
			Count: len(persons),
		})
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Place < entries[j].Place
	})

	return entries, nil
}

// GetPersonsByCemetery returns persons with burial/cremation events at the given
// place. Both sides of the join resolve through opts.BranchID's overlay (ADR-005):
// the branch-visible life events pick the owners, the branch-visible persons are
// returned.
func (s *ReadModelStore) GetPersonsByCemetery(ctx context.Context, place string, opts repository.ListOptions) ([]repository.PersonReadModel, int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Find distinct person IDs with matching burial/cremation events (exact
	// case-insensitive match).
	matchedIDs := make(map[uuid.UUID]struct{})
	for _, e := range resolveAllRows(s.events, opts.BranchID) {
		if e.FactType != domain.FactPersonBurial && e.FactType != domain.FactPersonCremation {
			continue
		}
		if strings.EqualFold(e.Place, place) {
			matchedIDs[e.OwnerID] = struct{}{}
		}
	}

	var results []repository.PersonReadModel
	for _, p := range resolveAllRows(s.persons, opts.BranchID) {
		if _, ok := matchedIDs[p.ID]; ok {
			results = append(results, *p)
		}
	}

	total := len(results)

	// Sort by surname, then given name
	sort.Slice(results, func(i, j int) bool {
		if results[i].Surname != results[j].Surname {
			return results[i].Surname < results[j].Surname
		}
		return results[i].GivenName < results[j].GivenName
	})

	// Apply pagination
	if opts.Offset > 0 && opts.Offset < len(results) {
		results = results[opts.Offset:]
	} else if opts.Offset >= len(results) {
		results = nil
	}
	if opts.Limit > 0 && opts.Limit < len(results) {
		results = results[:opts.Limit]
	}

	return results, total, nil
}

// GetMapLocations returns aggregated geographic locations from the birth/death
// coordinates of branchID's overlay of persons (ADR-005).
func (s *ReadModelStore) GetMapLocations(ctx context.Context, branchID domain.BranchID) ([]repository.MapLocation, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Key by "place|eventType" to aggregate
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

	for _, p := range resolveAllRows(s.persons, branchID) {
		// Birth location
		if p.BirthPlaceLat != nil && p.BirthPlaceLong != nil && *p.BirthPlaceLat != "" && *p.BirthPlaceLong != "" {
			lat, errLat := gedcom.ParseCoordinate(*p.BirthPlaceLat)
			lon, errLon := gedcom.ParseCoordinate(*p.BirthPlaceLong)
			if errLat == nil && errLon == nil {
				key := locKey{place: p.BirthPlace, eventType: "birth"}
				if d, ok := agg[key]; ok {
					d.personIDs = append(d.personIDs, p.ID)
				} else {
					agg[key] = &locData{lat: lat, lon: lon, personIDs: []uuid.UUID{p.ID}}
				}
			}
		}
		// Death location
		if p.DeathPlaceLat != nil && p.DeathPlaceLong != nil && *p.DeathPlaceLat != "" && *p.DeathPlaceLong != "" {
			lat, errLat := gedcom.ParseCoordinate(*p.DeathPlaceLat)
			lon, errLon := gedcom.ParseCoordinate(*p.DeathPlaceLong)
			if errLat == nil && errLon == nil {
				key := locKey{place: p.DeathPlace, eventType: "death"}
				if d, ok := agg[key]; ok {
					d.personIDs = append(d.personIDs, p.ID)
				} else {
					agg[key] = &locData{lat: lat, lon: lon, personIDs: []uuid.UUID{p.ID}}
				}
			}
		}
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
// MAIN-ONLY on purpose: brick-wall state is written straight to the read model rather
// than projected from an event, so there is no overlay to copy-on-write into and the
// main row is the only row to write. Whether brick walls should become branch-aware
// waits on deciding whether they become event-sourced, as #624 did for snapshots
// (ADR-005, "Entities that stay main-only") -- keep the main pin until that is decided.
func (s *ReadModelStore) SetBrickWall(ctx context.Context, personID uuid.UUID, note string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	p, exists := s.persons[branchKey{domain.MainBranchID, personID}]
	if !exists {
		return nil
	}
	now := time.Now()
	p.BrickWallNote = note
	p.BrickWallSince = &now
	p.BrickWallResolvedAt = nil
	return nil
}

// ResolveBrickWall marks a brick wall as resolved.
//
// MAIN-ONLY on purpose, for the same reason as SetBrickWall: written outside the
// event store, so there is no overlay to resolve (sub-issue F of #676, #761).
func (s *ReadModelStore) ResolveBrickWall(ctx context.Context, personID uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	p, exists := s.persons[branchKey{domain.MainBranchID, personID}]
	if !exists {
		return nil
	}
	now := time.Now()
	p.BrickWallResolvedAt = &now
	return nil
}

// GetBrickWalls returns persons with brick wall status.
//
// MAIN-ONLY for symmetry with SetBrickWall/ResolveBrickWall, which only ever write
// the main row: reading a branch overlay here would report shadow rows whose
// brick-wall fields no writer maintains (sub-issue F of #676, #761).
func (s *ReadModelStore) GetBrickWalls(ctx context.Context, includeResolved bool) ([]repository.BrickWallEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var entries []repository.BrickWallEntry
	for _, p := range resolveAllRows(s.persons, domain.MainBranchID) {
		if p.BrickWallSince == nil {
			continue
		}
		if !includeResolved && p.BrickWallResolvedAt != nil {
			continue
		}
		entries = append(entries, repository.BrickWallEntry{
			PersonID:   p.ID,
			PersonName: p.FullName,
			Note:       p.BrickWallNote,
			Since:      *p.BrickWallSince,
			ResolvedAt: p.BrickWallResolvedAt,
		})
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Since.After(entries[j].Since)
	})

	return entries, nil
}

// GetNote retrieves a note by ID within the branch overlay (ADR-005, #758).
func (s *ReadModelStore) GetNote(ctx context.Context, branchID domain.BranchID, id uuid.UUID) (*repository.NoteReadModel, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	n, _ := resolveRow(s.notes, branchID, id)
	if n == nil {
		return nil, nil
	}
	result := *n
	return &result, nil
}

// ListNotes returns a paginated list of the notes visible on opts.BranchID.
func (s *ReadModelStore) ListNotes(ctx context.Context, opts repository.ListOptions) ([]repository.NoteReadModel, int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var results []repository.NoteReadModel
	for _, n := range resolveAllRows(s.notes, opts.BranchID) {
		results = append(results, *n)
	}

	total := len(results)

	// Sort by updated_at (id breaks ties so pages are stable)
	asc := opts.Order == "asc"
	sort.Slice(results, func(i, j int) bool {
		cmp := compareTimestamps(results[i].UpdatedAt, results[j].UpdatedAt)
		if cmp == 0 {
			cmp = strings.Compare(results[i].ID.String(), results[j].ID.String())
		}
		if asc {
			return cmp < 0
		}
		return cmp > 0
	})

	// Apply pagination
	if opts.Offset > 0 && opts.Offset < len(results) {
		results = results[opts.Offset:]
	} else if opts.Offset >= len(results) {
		results = nil
	}
	if opts.Limit > 0 && opts.Limit < len(results) {
		results = results[:opts.Limit]
	}

	return results, total, nil
}

// SaveNote saves or updates a note on the given branch (ADR-005, #758).
func (s *ReadModelStore) SaveNote(ctx context.Context, branchID domain.BranchID, note *repository.NoteReadModel) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	result := *note
	s.notes[branchKey{branchID, note.ID}] = &result
	return nil
}

// DeleteNote removes a note: a real removal on main, a tombstone on a non-main
// branch.
func (s *ReadModelStore) DeleteNote(ctx context.Context, branchID domain.BranchID, id uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	removeRow(s.notes, branchID, id)
	return nil
}

// GetSubmitter retrieves a submitter by ID.
func (s *ReadModelStore) GetSubmitter(ctx context.Context, id uuid.UUID) (*repository.SubmitterReadModel, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	sub, exists := s.submitters[id]
	if !exists {
		return nil, nil
	}
	result := *sub
	return &result, nil
}

// ListSubmitters returns a paginated list of submitters.
func (s *ReadModelStore) ListSubmitters(ctx context.Context, opts repository.ListOptions) ([]repository.SubmitterReadModel, int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var results []repository.SubmitterReadModel
	for _, sub := range s.submitters {
		results = append(results, *sub)
	}

	total := len(results)

	// Sort by name
	asc := opts.Order == "asc"
	sort.Slice(results, func(i, j int) bool {
		if asc {
			return results[i].Name < results[j].Name
		}
		return results[i].Name > results[j].Name
	})

	// Apply pagination
	if opts.Offset > 0 && opts.Offset < len(results) {
		results = results[opts.Offset:]
	} else if opts.Offset >= len(results) {
		results = nil
	}
	if opts.Limit > 0 && opts.Limit < len(results) {
		results = results[:opts.Limit]
	}

	return results, total, nil
}

// SaveSubmitter saves or updates a submitter.
func (s *ReadModelStore) SaveSubmitter(ctx context.Context, submitter *repository.SubmitterReadModel) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	result := *submitter
	s.submitters[submitter.ID] = &result
	return nil
}

// DeleteSubmitter removes a submitter.
func (s *ReadModelStore) DeleteSubmitter(ctx context.Context, id uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.submitters, id)
	return nil
}

// GetRepository retrieves a repository by ID.
func (s *ReadModelStore) GetRepository(ctx context.Context, id uuid.UUID) (*repository.RepositoryReadModel, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	repo, exists := s.repositories[id]
	if !exists {
		return nil, nil
	}
	result := *repo
	return &result, nil
}

// ListRepositories returns a paginated list of repositories.
func (s *ReadModelStore) ListRepositories(ctx context.Context, opts repository.ListOptions) ([]repository.RepositoryReadModel, int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var results []repository.RepositoryReadModel
	for _, repo := range s.repositories {
		results = append(results, *repo)
	}

	total := len(results)

	// Sort by name or updated_at (matches the postgres/sqlite implementations,
	// which default to updated_at and switch to name when opts.Sort == "name").
	sortField := opts.Sort
	if sortField == "" {
		sortField = "updated_at"
	}
	asc := opts.Order == "asc"
	sort.Slice(results, func(i, j int) bool {
		var cmp int
		if sortField == "name" {
			cmp = strings.Compare(results[i].Name, results[j].Name)
		} else {
			cmp = compareTimestamps(results[i].UpdatedAt, results[j].UpdatedAt)
		}
		// id is a stable tie-breaker so pagination is deterministic when sort
		// keys collide, matching the postgres/sqlite implementations.
		if cmp == 0 {
			cmp = strings.Compare(results[i].ID.String(), results[j].ID.String())
		}
		if asc {
			return cmp < 0
		}
		return cmp > 0
	})

	// Apply pagination
	if opts.Offset > 0 && opts.Offset < len(results) {
		results = results[opts.Offset:]
	} else if opts.Offset >= len(results) {
		results = nil
	}
	if opts.Limit > 0 && opts.Limit < len(results) {
		results = results[:opts.Limit]
	}

	return results, total, nil
}

// SaveRepository saves or updates a repository.
func (s *ReadModelStore) SaveRepository(ctx context.Context, repo *repository.RepositoryReadModel) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	result := *repo
	s.repositories[repo.ID] = &result
	return nil
}

// DeleteRepository removes a repository.
func (s *ReadModelStore) DeleteRepository(ctx context.Context, id uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.repositories, id)
	delete(s.repositoryExternalIDs, id)
	return nil
}

// GetAssociation retrieves an association by ID within the branch overlay (ADR-005).
func (s *ReadModelStore) GetAssociation(ctx context.Context, branchID domain.BranchID, id uuid.UUID) (*repository.AssociationReadModel, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	assoc, _ := resolveRow(s.associations, branchID, id)
	if assoc == nil {
		return nil, nil
	}
	result := *assoc
	return &result, nil
}

// ListAssociations returns a paginated list of the associations visible on
// opts.BranchID.
func (s *ReadModelStore) ListAssociations(ctx context.Context, opts repository.ListOptions) ([]repository.AssociationReadModel, int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var results []repository.AssociationReadModel
	for _, assoc := range resolveAllRows(s.associations, opts.BranchID) {
		results = append(results, *assoc)
	}

	total := len(results)

	// Sort by role or updated_at
	sortField := opts.Sort
	if sortField == "" {
		sortField = "updated_at"
	}
	asc := opts.Order == "asc"
	sort.Slice(results, func(i, j int) bool {
		var cmp int
		if sortField == "role" {
			cmp = strings.Compare(results[i].Role, results[j].Role)
		} else {
			cmp = compareTimestamps(results[i].UpdatedAt, results[j].UpdatedAt)
		}
		if asc {
			return cmp < 0
		}
		return cmp > 0
	})

	// Apply pagination
	if opts.Offset > 0 && opts.Offset < len(results) {
		results = results[opts.Offset:]
	} else if opts.Offset >= len(results) {
		results = nil
	}
	if opts.Limit > 0 && opts.Limit < len(results) {
		results = results[:opts.Limit]
	}

	return results, total, nil
}

// ListAssociationsForPerson returns all associations visible on branchID in
// which the person is either the subject or the associate.
func (s *ReadModelStore) ListAssociationsForPerson(ctx context.Context, branchID domain.BranchID, personID uuid.UUID) ([]repository.AssociationReadModel, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var results []repository.AssociationReadModel
	for _, assoc := range resolveAllRows(s.associations, branchID) {
		if assoc.PersonID == personID || assoc.AssociateID == personID {
			results = append(results, *assoc)
		}
	}

	// Sort by role, then updated_at
	sort.Slice(results, func(i, j int) bool {
		cmp := strings.Compare(results[i].Role, results[j].Role)
		if cmp != 0 {
			return cmp < 0
		}
		return results[i].UpdatedAt.After(results[j].UpdatedAt)
	})

	return results, nil
}

// SaveAssociation saves or updates an association on the given branch (ADR-005).
func (s *ReadModelStore) SaveAssociation(ctx context.Context, branchID domain.BranchID, assoc *repository.AssociationReadModel) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	result := *assoc
	s.associations[branchKey{branchID, assoc.ID}] = &result
	return nil
}

// DeleteAssociation removes an association: a real removal on main, a tombstone
// on a non-main branch.
func (s *ReadModelStore) DeleteAssociation(ctx context.Context, branchID domain.BranchID, id uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	removeRow(s.associations, branchID, id)
	return nil
}

// GetLDSOrdinance retrieves an LDS ordinance by ID.
func (s *ReadModelStore) GetLDSOrdinance(ctx context.Context, id uuid.UUID) (*repository.LDSOrdinanceReadModel, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	ord, exists := s.ldsOrdinances[id]
	if !exists {
		return nil, nil
	}
	result := *ord
	return &result, nil
}

// ListLDSOrdinances returns a paginated list of LDS ordinances.
func (s *ReadModelStore) ListLDSOrdinances(ctx context.Context, opts repository.ListOptions) ([]repository.LDSOrdinanceReadModel, int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var results []repository.LDSOrdinanceReadModel
	for _, ord := range s.ldsOrdinances {
		results = append(results, *ord)
	}

	total := len(results)

	// Sort by type or updated_at
	sortField := opts.Sort
	if sortField == "" {
		sortField = "updated_at"
	}
	asc := opts.Order == "asc"
	sort.Slice(results, func(i, j int) bool {
		var cmp int
		if sortField == "type" {
			cmp = strings.Compare(string(results[i].Type), string(results[j].Type))
		} else {
			cmp = compareTimestamps(results[i].UpdatedAt, results[j].UpdatedAt)
		}
		if asc {
			return cmp < 0
		}
		return cmp > 0
	})

	// Apply pagination
	if opts.Offset > 0 && opts.Offset < len(results) {
		results = results[opts.Offset:]
	} else if opts.Offset >= len(results) {
		results = nil
	}
	if opts.Limit > 0 && opts.Limit < len(results) {
		results = results[:opts.Limit]
	}

	return results, total, nil
}

// ListLDSOrdinancesForPerson returns all LDS ordinances for a given person.
func (s *ReadModelStore) ListLDSOrdinancesForPerson(ctx context.Context, personID uuid.UUID) ([]repository.LDSOrdinanceReadModel, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var results []repository.LDSOrdinanceReadModel
	for _, ord := range s.ldsOrdinances {
		if ord.PersonID != nil && *ord.PersonID == personID {
			results = append(results, *ord)
		}
	}

	// Sort by type
	sort.Slice(results, func(i, j int) bool {
		return string(results[i].Type) < string(results[j].Type)
	})

	return results, nil
}

// ListLDSOrdinancesForFamily returns all LDS ordinances for a given family.
func (s *ReadModelStore) ListLDSOrdinancesForFamily(ctx context.Context, familyID uuid.UUID) ([]repository.LDSOrdinanceReadModel, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var results []repository.LDSOrdinanceReadModel
	for _, ord := range s.ldsOrdinances {
		if ord.FamilyID != nil && *ord.FamilyID == familyID {
			results = append(results, *ord)
		}
	}

	// Sort by type
	sort.Slice(results, func(i, j int) bool {
		return string(results[i].Type) < string(results[j].Type)
	})

	return results, nil
}

// SaveLDSOrdinance saves or updates an LDS ordinance.
func (s *ReadModelStore) SaveLDSOrdinance(ctx context.Context, ord *repository.LDSOrdinanceReadModel) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	result := *ord
	s.ldsOrdinances[ord.ID] = &result
	return nil
}

// DeleteLDSOrdinance removes an LDS ordinance.
func (s *ReadModelStore) DeleteLDSOrdinance(ctx context.Context, id uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.ldsOrdinances, id)
	return nil
}

// GPS artifacts (#760): evidence analyses, evidence conflicts, research logs and
// proof summaries are branch-scoped like the life events. Every read resolves the
// per-id overlay (resolveRow / resolveAllRows) first and only then applies its
// filter to the winning row, so a branch-side edit (a resolution, a re-pointed
// subject) or tombstone decides what the branch lists.

// gpsTimes extracts the fields every GPS artifact list sorts by.
type gpsTimes[T any] func(*T) (id uuid.UUID, createdAt, updatedAt time.Time)

// resolveGPS returns the rows of m visible on branch that keep accepts (nil keeps
// every row), as copies, sorted by created_at then id so the per-subject lists
// are deterministic on every backend. Callers hold s.mu.
func resolveGPS[T any](m map[branchKey]*T, branch domain.BranchID, keep func(*T) bool, times gpsTimes[T]) []T {
	var out []T
	for _, row := range resolveAllRows(m, branch) {
		if keep == nil || keep(row) {
			out = append(out, *row)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		idI, createdI, _ := times(&out[i])
		idJ, createdJ, _ := times(&out[j])
		if !createdI.Equal(createdJ) {
			return createdI.Before(createdJ)
		}
		return idI.String() < idJ.String()
	})
	return out
}

// pageGPS sorts rows by opts.Sort ("created_at", else updated_at) in opts.Order
// (id breaks ties in the same direction) and returns the requested page and the
// unpaged total. A non-positive limit returns every row from the offset on.
func pageGPS[T any](rows []T, opts repository.ListOptions, times gpsTimes[T]) ([]T, int) {
	asc := opts.Order == "asc"
	sort.Slice(rows, func(i, j int) bool {
		idI, createdI, updatedI := times(&rows[i])
		idJ, createdJ, updatedJ := times(&rows[j])
		ti, tj := updatedI, updatedJ
		if opts.Sort == "created_at" {
			ti, tj = createdI, createdJ
		}
		if ti.Equal(tj) {
			if asc {
				return idI.String() < idJ.String()
			}
			return idI.String() > idJ.String()
		}
		if asc {
			return ti.Before(tj)
		}
		return ti.After(tj)
	})

	total := len(rows)
	if opts.Offset > 0 && opts.Offset < len(rows) {
		rows = rows[opts.Offset:]
	} else if opts.Offset >= len(rows) {
		rows = nil
	}
	if opts.Limit > 0 && opts.Limit < len(rows) {
		rows = rows[:opts.Limit]
	}
	return rows, total
}

// getGPS returns a copy of the row id resolves to on branch, or nil.
func getGPS[T any](m map[branchKey]*T, branch domain.BranchID, id uuid.UUID) *T {
	row, _ := resolveRow(m, branch, id)
	if row == nil {
		return nil
	}
	result := *row
	return &result
}

func analysisTimes(a *repository.EvidenceAnalysisReadModel) (uuid.UUID, time.Time, time.Time) {
	return a.ID, a.CreatedAt, a.UpdatedAt
}

func conflictTimes(c *repository.EvidenceConflictReadModel) (uuid.UUID, time.Time, time.Time) {
	return c.ID, c.CreatedAt, c.UpdatedAt
}

func researchLogTimes(l *repository.ResearchLogReadModel) (uuid.UUID, time.Time, time.Time) {
	return l.ID, l.CreatedAt, l.UpdatedAt
}

func proofSummaryTimes(p *repository.ProofSummaryReadModel) (uuid.UUID, time.Time, time.Time) {
	return p.ID, p.CreatedAt, p.UpdatedAt
}

// cascadeGPS removes, on branch, every GPS artifact whose subject is subjectID —
// the manual cascade DeletePerson and DeleteFamily run (#760). Main rows are hard
// deleted; a branch tombstones each row visible on it. Callers hold s.mu.
func (s *ReadModelStore) cascadeGPS(branch domain.BranchID, subjectID uuid.UUID) {
	for _, a := range resolveAllRows(s.evidenceAnalyses, branch) {
		if a.SubjectID == subjectID {
			removeRow(s.evidenceAnalyses, branch, a.ID)
		}
	}
	for _, c := range resolveAllRows(s.evidenceConflicts, branch) {
		if c.SubjectID == subjectID {
			removeRow(s.evidenceConflicts, branch, c.ID)
		}
	}
	for _, l := range resolveAllRows(s.researchLogs, branch) {
		if l.SubjectID == subjectID {
			removeRow(s.researchLogs, branch, l.ID)
		}
	}
	for _, p := range resolveAllRows(s.proofSummaries, branch) {
		if p.SubjectID == subjectID {
			removeRow(s.proofSummaries, branch, p.ID)
		}
	}
}

// GetEvidenceAnalysis retrieves an evidence analysis by ID within the branch
// overlay (ADR-005, #760).
func (s *ReadModelStore) GetEvidenceAnalysis(ctx context.Context, branchID domain.BranchID, id uuid.UUID) (*repository.EvidenceAnalysisReadModel, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return getGPS(s.evidenceAnalyses, branchID, id), nil
}

// ListEvidenceAnalyses returns a paginated list of the evidence analyses visible
// on opts.BranchID.
func (s *ReadModelStore) ListEvidenceAnalyses(ctx context.Context, opts repository.ListOptions) ([]repository.EvidenceAnalysisReadModel, int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rows, total := pageGPS(resolveGPS(s.evidenceAnalyses, opts.BranchID, nil, analysisTimes), opts, analysisTimes)
	return rows, total, nil
}

// GetAnalysesForFact returns the evidence analyses for a fact type and subject
// visible on branchID, filtered on each id's winning row.
func (s *ReadModelStore) GetAnalysesForFact(ctx context.Context, branchID domain.BranchID, factType domain.FactType, subjectID uuid.UUID) ([]repository.EvidenceAnalysisReadModel, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return resolveGPS(s.evidenceAnalyses, branchID, func(a *repository.EvidenceAnalysisReadModel) bool {
		return a.FactType == factType && a.SubjectID == subjectID
	}, analysisTimes), nil
}

// GetAnalysesBySubject returns the evidence analyses for a subject visible on
// branchID, regardless of fact type.
func (s *ReadModelStore) GetAnalysesBySubject(ctx context.Context, branchID domain.BranchID, subjectID uuid.UUID) ([]repository.EvidenceAnalysisReadModel, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return resolveGPS(s.evidenceAnalyses, branchID, func(a *repository.EvidenceAnalysisReadModel) bool {
		return a.SubjectID == subjectID
	}, analysisTimes), nil
}

// SaveEvidenceAnalysis saves or updates an evidence analysis on the given
// branch; a save clears any prior tombstone.
func (s *ReadModelStore) SaveEvidenceAnalysis(ctx context.Context, branchID domain.BranchID, analysis *repository.EvidenceAnalysisReadModel) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := *analysis
	s.evidenceAnalyses[branchKey{branchID, analysis.ID}] = &result
	return nil
}

// DeleteEvidenceAnalysis removes an evidence analysis: a real removal on main, a
// tombstone on a non-main branch.
func (s *ReadModelStore) DeleteEvidenceAnalysis(ctx context.Context, branchID domain.BranchID, id uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	removeRow(s.evidenceAnalyses, branchID, id)
	return nil
}

// GetEvidenceConflict retrieves an evidence conflict by ID within the branch
// overlay (ADR-005, #760).
func (s *ReadModelStore) GetEvidenceConflict(ctx context.Context, branchID domain.BranchID, id uuid.UUID) (*repository.EvidenceConflictReadModel, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return getGPS(s.evidenceConflicts, branchID, id), nil
}

// ListEvidenceConflicts returns a paginated list of the evidence conflicts
// visible on opts.BranchID.
func (s *ReadModelStore) ListEvidenceConflicts(ctx context.Context, opts repository.ListOptions) ([]repository.EvidenceConflictReadModel, int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var keep func(*repository.EvidenceConflictReadModel) bool
	if opts.ConflictStatus != nil {
		status := *opts.ConflictStatus
		keep = func(c *repository.EvidenceConflictReadModel) bool { return c.Status == status }
	}
	rows, total := pageGPS(resolveGPS(s.evidenceConflicts, opts.BranchID, keep, conflictTimes), opts, conflictTimes)
	return rows, total, nil
}

// GetConflictsForSubject returns the evidence conflicts for a subject visible on
// branchID.
func (s *ReadModelStore) GetConflictsForSubject(ctx context.Context, branchID domain.BranchID, subjectID uuid.UUID) ([]repository.EvidenceConflictReadModel, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return resolveGPS(s.evidenceConflicts, branchID, func(c *repository.EvidenceConflictReadModel) bool {
		return c.SubjectID == subjectID
	}, conflictTimes), nil
}

// ListUnresolvedConflicts returns the open evidence conflicts visible on
// branchID. The overlay is resolved first and the status predicate applied to
// the winning row, so a conflict the branch resolved is not listed there even
// though main's row for it is still open.
func (s *ReadModelStore) ListUnresolvedConflicts(ctx context.Context, branchID domain.BranchID) ([]repository.EvidenceConflictReadModel, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return resolveGPS(s.evidenceConflicts, branchID, func(c *repository.EvidenceConflictReadModel) bool {
		return c.Status == domain.ConflictStatusOpen
	}, conflictTimes), nil
}

// SaveEvidenceConflict saves or updates an evidence conflict on the given
// branch; a save clears any prior tombstone.
func (s *ReadModelStore) SaveEvidenceConflict(ctx context.Context, branchID domain.BranchID, conflict *repository.EvidenceConflictReadModel) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := *conflict
	s.evidenceConflicts[branchKey{branchID, conflict.ID}] = &result
	return nil
}

// DeleteEvidenceConflict removes an evidence conflict: a real removal on main, a
// tombstone on a non-main branch.
func (s *ReadModelStore) DeleteEvidenceConflict(ctx context.Context, branchID domain.BranchID, id uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	removeRow(s.evidenceConflicts, branchID, id)
	return nil
}

// GetResearchLog retrieves a research log by ID within the branch overlay
// (ADR-005, #760).
func (s *ReadModelStore) GetResearchLog(ctx context.Context, branchID domain.BranchID, id uuid.UUID) (*repository.ResearchLogReadModel, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return getGPS(s.researchLogs, branchID, id), nil
}

// ListResearchLogs returns a paginated list of the research logs visible on
// opts.BranchID.
func (s *ReadModelStore) ListResearchLogs(ctx context.Context, opts repository.ListOptions) ([]repository.ResearchLogReadModel, int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rows, total := pageGPS(resolveGPS(s.researchLogs, opts.BranchID, nil, researchLogTimes), opts, researchLogTimes)
	return rows, total, nil
}

// GetResearchLogsForSubject returns the research logs for a subject visible on
// branchID.
func (s *ReadModelStore) GetResearchLogsForSubject(ctx context.Context, branchID domain.BranchID, subjectID uuid.UUID) ([]repository.ResearchLogReadModel, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return resolveGPS(s.researchLogs, branchID, func(l *repository.ResearchLogReadModel) bool {
		return l.SubjectID == subjectID
	}, researchLogTimes), nil
}

// SaveResearchLog saves or updates a research log on the given branch; a save
// clears any prior tombstone.
func (s *ReadModelStore) SaveResearchLog(ctx context.Context, branchID domain.BranchID, log *repository.ResearchLogReadModel) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := *log
	s.researchLogs[branchKey{branchID, log.ID}] = &result
	return nil
}

// DeleteResearchLog removes a research log: a real removal on main, a tombstone
// on a non-main branch.
func (s *ReadModelStore) DeleteResearchLog(ctx context.Context, branchID domain.BranchID, id uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	removeRow(s.researchLogs, branchID, id)
	return nil
}

// GetProofSummary retrieves a proof summary by ID within the branch overlay
// (ADR-005, #760).
func (s *ReadModelStore) GetProofSummary(ctx context.Context, branchID domain.BranchID, id uuid.UUID) (*repository.ProofSummaryReadModel, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return getGPS(s.proofSummaries, branchID, id), nil
}

// ListProofSummaries returns a paginated list of the proof summaries visible on
// opts.BranchID.
func (s *ReadModelStore) ListProofSummaries(ctx context.Context, opts repository.ListOptions) ([]repository.ProofSummaryReadModel, int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rows, total := pageGPS(resolveGPS(s.proofSummaries, opts.BranchID, nil, proofSummaryTimes), opts, proofSummaryTimes)
	return rows, total, nil
}

// GetProofSummariesForFact returns the proof summaries for a fact type and
// subject visible on branchID.
func (s *ReadModelStore) GetProofSummariesForFact(ctx context.Context, branchID domain.BranchID, factType domain.FactType, subjectID uuid.UUID) ([]repository.ProofSummaryReadModel, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return resolveGPS(s.proofSummaries, branchID, func(p *repository.ProofSummaryReadModel) bool {
		return p.FactType == factType && p.SubjectID == subjectID
	}, proofSummaryTimes), nil
}

// GetProofSummariesBySubject returns the proof summaries for a subject visible on
// branchID, regardless of fact type.
func (s *ReadModelStore) GetProofSummariesBySubject(ctx context.Context, branchID domain.BranchID, subjectID uuid.UUID) ([]repository.ProofSummaryReadModel, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return resolveGPS(s.proofSummaries, branchID, func(p *repository.ProofSummaryReadModel) bool {
		return p.SubjectID == subjectID
	}, proofSummaryTimes), nil
}

// SaveProofSummary saves or updates a proof summary on the given branch; a save
// clears any prior tombstone.
func (s *ReadModelStore) SaveProofSummary(ctx context.Context, branchID domain.BranchID, summary *repository.ProofSummaryReadModel) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := *summary
	s.proofSummaries[branchKey{branchID, summary.ID}] = &result
	return nil
}

// DeleteProofSummary removes a proof summary: a real removal on main, a
// tombstone on a non-main branch.
func (s *ReadModelStore) DeleteProofSummary(ctx context.Context, branchID domain.BranchID, id uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	removeRow(s.proofSummaries, branchID, id)
	return nil
}
