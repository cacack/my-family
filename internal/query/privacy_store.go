package query

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// PrivacyStore is the read-model choke point of ADR-008 §5: a ReadModelStore
// that wraps the real one and shows each principal only what ADR-008 §4 lets
// them see. Every query service and read handler reads through it; command
// handlers, projections and the CLI use the real store.
//
// It does not embed the store it wraps, so a method added to ReadModelStore
// does not compile here until someone decides how it is redacted. For a
// principal who sees everything (repository.Privacy.SeesAll) every method
// passes through. Otherwise:
//
//   - A protected person is a placeholder (repository.PlaceholderPerson) where
//     a record names them or a chart needs their slot, and is excluded, in the
//     query, from person lists and searches (ListOptions.Privacy,
//     SearchOptions.Privacy). Their names, external IDs, events, attributes,
//     citations, associations, LDS ordinances and media are hidden.
//   - A private family is hidden; a family with a protected partner shows the
//     partner as a placeholder and hides its own facts, events, external IDs,
//     LDS ordinances and media.
//   - Private media are hidden, and so are media of a hidden owner, bytes and
//     thumbnails included.
//
// The methods marked "not yet redacted" pass through for every principal.
// They serve records other than people and families (sources, citations by
// source, notes, submitters, repositories, evidence), aggregates (surname,
// place and cemetery indexes, the map, brick walls) and whole-tree exports,
// whose redaction is still to come (#1412). Until it lands anonymous viewing
// stays off (repository.AnonymousViewingProtected).
type PrivacyStore struct {
	inner repository.ReadModelStore
	now   func() time.Time
}

var _ repository.ReadModelStore = (*PrivacyStore)(nil)

// NewPrivacyStore wraps inner so that its reads honour the privacy of the
// principal and tree settings on each call's context.
func NewPrivacyStore(inner repository.ReadModelStore) *PrivacyStore {
	return &PrivacyStore{inner: inner, now: time.Now}
}

// privacy is what the principal on ctx may not see now.
func (s *PrivacyStore) privacy(ctx context.Context) repository.Privacy {
	return repository.PrivacyFrom(ctx, s.now())
}

// ============================================================================
// Shared checks
// ============================================================================

// visibilities returns how each of ids appears under v. An ID no person
// resolves to is Hidden (ADR-008 §3).
func (s *PrivacyStore) visibilities(ctx context.Context, v repository.Privacy, branchID domain.BranchID, ids []uuid.UUID) (map[uuid.UUID]repository.Visibility, error) {
	out := make(map[uuid.UUID]repository.Visibility, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	people, err := s.inner.GetPersonsByIDs(ctx, branchID, ids)
	if err != nil {
		return nil, err
	}
	for _, id := range ids {
		out[id] = repository.Hidden
	}
	for i := range people {
		out[people[i].ID] = v.Person(&people[i])
	}
	return out, nil
}

// personProtected reports whether the person id may not be shown in full.
func (s *PrivacyStore) personProtected(ctx context.Context, v repository.Privacy, branchID domain.BranchID, id uuid.UUID) (bool, error) {
	vis, err := s.visibilities(ctx, v, branchID, []uuid.UUID{id})
	return vis[id] != repository.Visible, err
}

// partnerIDs returns the partner IDs the families name.
func partnerIDs(families []repository.FamilyReadModel) []uuid.UUID {
	var ids []uuid.UUID
	for _, f := range families {
		for _, id := range []*uuid.UUID{f.Partner1ID, f.Partner2ID} {
			if id != nil {
				ids = append(ids, *id)
			}
		}
	}
	return ids
}

// redactFamilies drops the private families and redacts the rest (see
// redactFamily).
func (s *PrivacyStore) redactFamilies(ctx context.Context, v repository.Privacy, branchID domain.BranchID, families []repository.FamilyReadModel) ([]repository.FamilyReadModel, error) {
	vis, err := s.visibilities(ctx, v, branchID, partnerIDs(families))
	if err != nil {
		return nil, err
	}
	out := make([]repository.FamilyReadModel, 0, len(families))
	for _, f := range families {
		if v.Hides(f.Restriction) {
			continue
		}
		redacted, _ := redactFamily(f, vis)
		out = append(out, redacted)
	}
	return out, nil
}

// redactFamily shows each protected partner as the placeholder and, when
// there is one, hides the family's own facts (ADR-008 §3). It reports whether
// the facts were hidden. vis must hold both partners.
func redactFamily(f repository.FamilyReadModel, vis map[uuid.UUID]repository.Visibility) (repository.FamilyReadModel, bool) {
	protected := false
	if f.Partner1ID != nil && vis[*f.Partner1ID] != repository.Visible {
		f.Partner1GivenName, f.Partner1Surname = repository.PlaceholderGivenName, ""
		protected = true
	}
	if f.Partner2ID != nil && vis[*f.Partner2ID] != repository.Visible {
		f.Partner2GivenName, f.Partner2Surname = repository.PlaceholderGivenName, ""
		protected = true
	}
	if protected {
		f.MarriageDateRaw, f.MarriageDateSort = "", nil
		f.MarriagePlace, f.MarriagePlaceLat, f.MarriagePlaceLong = "", nil, nil
		f.MarriageNotes, f.MarriageTypeDetail, f.Notes = "", "", ""
	}
	return f, protected
}

// familiesShown returns the IDs of the families whose own facts v may see:
// they resolve, are not private and have no protected partner.
func (s *PrivacyStore) familiesShown(ctx context.Context, v repository.Privacy, branchID domain.BranchID, ids []uuid.UUID) (map[uuid.UUID]bool, error) {
	shown := make(map[uuid.UUID]bool, len(ids))
	if len(ids) == 0 {
		return shown, nil
	}
	families, err := s.inner.GetFamiliesByIDs(ctx, branchID, ids)
	if err != nil {
		return nil, err
	}
	vis, err := s.visibilities(ctx, v, branchID, partnerIDs(families))
	if err != nil {
		return nil, err
	}
	for _, f := range families {
		if _, factsHidden := redactFamily(f, vis); !factsHidden && !v.Hides(f.Restriction) {
			shown[f.ID] = true
		}
	}
	return shown, nil
}

// familyShown reports whether v may see the family id's own facts.
func (s *PrivacyStore) familyShown(ctx context.Context, v repository.Privacy, branchID domain.BranchID, id uuid.UUID) (bool, error) {
	shown, err := s.familiesShown(ctx, v, branchID, []uuid.UUID{id})
	return shown[id], err
}

// ownerShown reports whether v may see what is attached to a person or family
// (ownerType "person" or "family"). Other owners are not redacted yet.
func (s *PrivacyStore) ownerShown(ctx context.Context, v repository.Privacy, branchID domain.BranchID, ownerType string, id uuid.UUID) (bool, error) {
	switch ownerType {
	case "person":
		protected, err := s.personProtected(ctx, v, branchID, id)
		return !protected, err
	case "family":
		return s.familyShown(ctx, v, branchID, id)
	default:
		return true, nil
	}
}

// factOwnerType returns the kind of record a fact type belongs to.
func factOwnerType(f domain.FactType) string {
	owner, _, _ := strings.Cut(string(f), "_")
	return owner
}

// withoutPrivate drops the records v hides.
func withoutPrivate[T any](v repository.Privacy, records []T, restriction func(*T) domain.Restriction) []T {
	out := make([]T, 0, len(records))
	for i := range records {
		if !v.Hides(restriction(&records[i])) {
			out = append(out, records[i])
		}
	}
	return out
}

func citationRestriction(c *repository.CitationReadModel) domain.Restriction { return c.Restriction }

// placeholderName returns the given name and surname to show for a person
// who appears as vis.
func placeholderName(vis repository.Visibility, given, surname string) (string, string) {
	if vis != repository.Visible {
		return repository.PlaceholderGivenName, ""
	}
	return given, surname
}

// redactChildren drops the rows of private families and shows protected
// children by the placeholder name.
func (s *PrivacyStore) redactChildren(ctx context.Context, v repository.Privacy, branchID domain.BranchID, children []repository.FamilyChildReadModel) ([]repository.FamilyChildReadModel, error) {
	if len(children) == 0 {
		return children, nil
	}
	var familyIDs, personIDs []uuid.UUID
	for _, c := range children {
		familyIDs = append(familyIDs, c.FamilyID)
		personIDs = append(personIDs, c.PersonID)
	}
	families, err := s.inner.GetFamiliesByIDs(ctx, branchID, familyIDs)
	if err != nil {
		return nil, err
	}
	listed := make(map[uuid.UUID]bool, len(families))
	for _, f := range families {
		listed[f.ID] = !v.Hides(f.Restriction)
	}
	vis, err := s.visibilities(ctx, v, branchID, personIDs)
	if err != nil {
		return nil, err
	}
	out := make([]repository.FamilyChildReadModel, 0, len(children))
	for _, c := range children {
		if !listed[c.FamilyID] {
			continue
		}
		c.PersonGivenName, c.PersonSurname = placeholderName(vis[c.PersonID], c.PersonGivenName, c.PersonSurname)
		out = append(out, c)
	}
	return out, nil
}

// redactEdges shows protected parents by the placeholder name.
func (s *PrivacyStore) redactEdges(ctx context.Context, v repository.Privacy, branchID domain.BranchID, edges []repository.PedigreeEdge) ([]repository.PedigreeEdge, error) {
	var ids []uuid.UUID
	for _, e := range edges {
		for _, id := range []*uuid.UUID{e.FatherID, e.MotherID} {
			if id != nil {
				ids = append(ids, *id)
			}
		}
	}
	vis, err := s.visibilities(ctx, v, branchID, ids)
	if err != nil {
		return nil, err
	}
	for i := range edges {
		if id := edges[i].FatherID; id != nil && vis[*id] != repository.Visible {
			edges[i].FatherName = repository.PlaceholderGivenName
		}
		if id := edges[i].MotherID; id != nil && vis[*id] != repository.Visible {
			edges[i].MotherName = repository.PlaceholderGivenName
		}
	}
	return edges, nil
}

// placeholders replaces each person v protects by the placeholder.
func placeholders(v repository.Privacy, people []repository.PersonReadModel) []repository.PersonReadModel {
	for i := range people {
		if vis := v.Person(&people[i]); vis != repository.Visible {
			people[i] = *repository.PlaceholderPerson(&people[i], vis)
		}
	}
	return people
}

// ============================================================================
// Persons
// ============================================================================

// GetPerson returns the placeholder for a protected person, marked Hidden for
// a private one so that the person's own page is not found.
func (s *PrivacyStore) GetPerson(ctx context.Context, branchID domain.BranchID, id uuid.UUID) (*repository.PersonReadModel, error) {
	p, err := s.inner.GetPerson(ctx, branchID, id)
	v := s.privacy(ctx)
	if err != nil || p == nil || v.SeesAll() {
		return p, err
	}
	if vis := v.Person(p); vis != repository.Visible {
		return repository.PlaceholderPerson(p, vis), nil
	}
	return p, nil
}

func (s *PrivacyStore) GetPersonsByIDs(ctx context.Context, branchID domain.BranchID, ids []uuid.UUID) ([]repository.PersonReadModel, error) {
	people, err := s.inner.GetPersonsByIDs(ctx, branchID, ids)
	if v := s.privacy(ctx); err == nil && !v.SeesAll() {
		people = placeholders(v, people)
	}
	return people, err
}

func (s *PrivacyStore) ListPersons(ctx context.Context, opts repository.ListOptions) ([]repository.PersonReadModel, int, error) {
	opts.Privacy = s.privacy(ctx)
	return s.inner.ListPersons(ctx, opts)
}

func (s *PrivacyStore) SearchPersons(ctx context.Context, opts repository.SearchOptions) ([]repository.PersonReadModel, int, error) {
	opts.Privacy = s.privacy(ctx)
	return s.inner.SearchPersons(ctx, opts)
}

func (s *PrivacyStore) SavePerson(ctx context.Context, branchID domain.BranchID, person *repository.PersonReadModel) error {
	return s.inner.SavePerson(ctx, branchID, person)
}

func (s *PrivacyStore) DeletePerson(ctx context.Context, branchID domain.BranchID, id uuid.UUID) error {
	return s.inner.DeletePerson(ctx, branchID, id)
}

func (s *PrivacyStore) SavePersonName(ctx context.Context, branchID domain.BranchID, name *repository.PersonNameReadModel) error {
	return s.inner.SavePersonName(ctx, branchID, name)
}

func (s *PrivacyStore) GetPersonName(ctx context.Context, branchID domain.BranchID, nameID uuid.UUID) (*repository.PersonNameReadModel, error) {
	name, err := s.inner.GetPersonName(ctx, branchID, nameID)
	v := s.privacy(ctx)
	if err != nil || name == nil || v.SeesAll() {
		return name, err
	}
	if protected, err := s.personProtected(ctx, v, branchID, name.PersonID); err != nil || protected {
		return nil, err
	}
	return name, nil
}

func (s *PrivacyStore) GetPersonNames(ctx context.Context, branchID domain.BranchID, personID uuid.UUID) ([]repository.PersonNameReadModel, error) {
	if v := s.privacy(ctx); !v.SeesAll() {
		if protected, err := s.personProtected(ctx, v, branchID, personID); err != nil || protected {
			return nil, err
		}
	}
	return s.inner.GetPersonNames(ctx, branchID, personID)
}

func (s *PrivacyStore) DeletePersonName(ctx context.Context, branchID domain.BranchID, nameID uuid.UUID) error {
	return s.inner.DeletePersonName(ctx, branchID, nameID)
}

func (s *PrivacyStore) ReplacePersonExternalIDs(ctx context.Context, branchID domain.BranchID, personID uuid.UUID, ids []repository.PersonExternalIDReadModel) error {
	return s.inner.ReplacePersonExternalIDs(ctx, branchID, personID, ids)
}

func (s *PrivacyStore) GetPersonExternalIDs(ctx context.Context, branchID domain.BranchID, personID uuid.UUID) ([]repository.PersonExternalIDReadModel, error) {
	if v := s.privacy(ctx); !v.SeesAll() {
		if protected, err := s.personProtected(ctx, v, branchID, personID); err != nil || protected {
			return nil, err
		}
	}
	return s.inner.GetPersonExternalIDs(ctx, branchID, personID)
}

func (s *PrivacyStore) ListPersonExternalIDs(ctx context.Context, branchID domain.BranchID) (map[uuid.UUID][]repository.PersonExternalIDReadModel, error) {
	all, err := s.inner.ListPersonExternalIDs(ctx, branchID)
	v := s.privacy(ctx)
	if err != nil || v.SeesAll() {
		return all, err
	}
	ids := make([]uuid.UUID, 0, len(all))
	for id := range all {
		ids = append(ids, id)
	}
	vis, err := s.visibilities(ctx, v, branchID, ids)
	if err != nil {
		return nil, err
	}
	for id := range all {
		if vis[id] != repository.Visible {
			delete(all, id)
		}
	}
	return all, nil
}

// ============================================================================
// Families
// ============================================================================

func (s *PrivacyStore) GetFamily(ctx context.Context, branchID domain.BranchID, id uuid.UUID) (*repository.FamilyReadModel, error) {
	f, err := s.inner.GetFamily(ctx, branchID, id)
	v := s.privacy(ctx)
	if err != nil || f == nil || v.SeesAll() {
		return f, err
	}
	families, err := s.redactFamilies(ctx, v, branchID, []repository.FamilyReadModel{*f})
	if err != nil || len(families) == 0 {
		return nil, err
	}
	return &families[0], nil
}

func (s *PrivacyStore) GetFamiliesByIDs(ctx context.Context, branchID domain.BranchID, ids []uuid.UUID) ([]repository.FamilyReadModel, error) {
	families, err := s.inner.GetFamiliesByIDs(ctx, branchID, ids)
	if v := s.privacy(ctx); err == nil && !v.SeesAll() {
		return s.redactFamilies(ctx, v, branchID, families)
	}
	return families, err
}

// ListFamilies excludes private families in the query, so the total counts
// only the families listed, and redacts the rest.
func (s *PrivacyStore) ListFamilies(ctx context.Context, opts repository.ListOptions) ([]repository.FamilyReadModel, int, error) {
	v := s.privacy(ctx)
	opts.Privacy = v
	families, total, err := s.inner.ListFamilies(ctx, opts)
	if err != nil || v.SeesAll() {
		return families, total, err
	}
	families, err = s.redactFamilies(ctx, v, opts.BranchID, families)
	return families, total, err
}

func (s *PrivacyStore) GetFamiliesForPerson(ctx context.Context, branchID domain.BranchID, personID uuid.UUID) ([]repository.FamilyReadModel, error) {
	families, err := s.inner.GetFamiliesForPerson(ctx, branchID, personID)
	if v := s.privacy(ctx); err == nil && !v.SeesAll() {
		return s.redactFamilies(ctx, v, branchID, families)
	}
	return families, err
}

func (s *PrivacyStore) GetFamiliesForPersons(ctx context.Context, branchID domain.BranchID, personIDs []uuid.UUID) ([]repository.FamilyReadModel, error) {
	families, err := s.inner.GetFamiliesForPersons(ctx, branchID, personIDs)
	if v := s.privacy(ctx); err == nil && !v.SeesAll() {
		return s.redactFamilies(ctx, v, branchID, families)
	}
	return families, err
}

func (s *PrivacyStore) SaveFamily(ctx context.Context, branchID domain.BranchID, family *repository.FamilyReadModel) error {
	return s.inner.SaveFamily(ctx, branchID, family)
}

func (s *PrivacyStore) DeleteFamily(ctx context.Context, branchID domain.BranchID, id uuid.UUID) error {
	return s.inner.DeleteFamily(ctx, branchID, id)
}

func (s *PrivacyStore) ReplaceFamilyExternalIDs(ctx context.Context, branchID domain.BranchID, familyID uuid.UUID, ids []repository.FamilyExternalIDReadModel) error {
	return s.inner.ReplaceFamilyExternalIDs(ctx, branchID, familyID, ids)
}

func (s *PrivacyStore) GetFamilyExternalIDs(ctx context.Context, branchID domain.BranchID, familyID uuid.UUID) ([]repository.FamilyExternalIDReadModel, error) {
	if v := s.privacy(ctx); !v.SeesAll() {
		if shown, err := s.familyShown(ctx, v, branchID, familyID); err != nil || !shown {
			return nil, err
		}
	}
	return s.inner.GetFamilyExternalIDs(ctx, branchID, familyID)
}

func (s *PrivacyStore) ListFamilyExternalIDs(ctx context.Context, branchID domain.BranchID) (map[uuid.UUID][]repository.FamilyExternalIDReadModel, error) {
	all, err := s.inner.ListFamilyExternalIDs(ctx, branchID)
	v := s.privacy(ctx)
	if err != nil || v.SeesAll() {
		return all, err
	}
	ids := make([]uuid.UUID, 0, len(all))
	for id := range all {
		ids = append(ids, id)
	}
	shown, err := s.familiesShown(ctx, v, branchID, ids)
	if err != nil {
		return nil, err
	}
	for id := range all {
		if !shown[id] {
			delete(all, id)
		}
	}
	return all, nil
}

func (s *PrivacyStore) GetFamilyChildren(ctx context.Context, branchID domain.BranchID, familyID uuid.UUID) ([]repository.FamilyChildReadModel, error) {
	children, err := s.inner.GetFamilyChildren(ctx, branchID, familyID)
	if v := s.privacy(ctx); err == nil && !v.SeesAll() {
		return s.redactChildren(ctx, v, branchID, children)
	}
	return children, err
}

func (s *PrivacyStore) GetFamilyChildrenByFamilyIDs(ctx context.Context, branchID domain.BranchID, familyIDs []uuid.UUID) ([]repository.FamilyChildReadModel, error) {
	children, err := s.inner.GetFamilyChildrenByFamilyIDs(ctx, branchID, familyIDs)
	if v := s.privacy(ctx); err == nil && !v.SeesAll() {
		return s.redactChildren(ctx, v, branchID, children)
	}
	return children, err
}

func (s *PrivacyStore) ListAllFamilyChildren(ctx context.Context, branchID domain.BranchID) ([]repository.FamilyChildReadModel, error) {
	children, err := s.inner.ListAllFamilyChildren(ctx, branchID)
	if v := s.privacy(ctx); err == nil && !v.SeesAll() {
		return s.redactChildren(ctx, v, branchID, children)
	}
	return children, err
}

func (s *PrivacyStore) GetChildrenOfFamily(ctx context.Context, branchID domain.BranchID, familyID uuid.UUID) ([]repository.PersonReadModel, error) {
	v := s.privacy(ctx)
	if !v.SeesAll() {
		f, err := s.inner.GetFamily(ctx, branchID, familyID)
		if err != nil || f == nil || v.Hides(f.Restriction) {
			return nil, err
		}
	}
	children, err := s.inner.GetChildrenOfFamily(ctx, branchID, familyID)
	if err == nil && !v.SeesAll() {
		children = placeholders(v, children)
	}
	return children, err
}

func (s *PrivacyStore) GetChildFamily(ctx context.Context, branchID domain.BranchID, personID uuid.UUID) (*repository.FamilyReadModel, error) {
	f, err := s.inner.GetChildFamily(ctx, branchID, personID)
	v := s.privacy(ctx)
	if err != nil || f == nil || v.SeesAll() {
		return f, err
	}
	families, err := s.redactFamilies(ctx, v, branchID, []repository.FamilyReadModel{*f})
	if err != nil || len(families) == 0 {
		return nil, err
	}
	return &families[0], nil
}

func (s *PrivacyStore) SaveFamilyChild(ctx context.Context, branchID domain.BranchID, child *repository.FamilyChildReadModel) error {
	return s.inner.SaveFamilyChild(ctx, branchID, child)
}

func (s *PrivacyStore) DeleteFamilyChild(ctx context.Context, branchID domain.BranchID, familyID, personID uuid.UUID) error {
	return s.inner.DeleteFamilyChild(ctx, branchID, familyID, personID)
}

// ============================================================================
// Pedigree edges
// ============================================================================

func (s *PrivacyStore) GetPedigreeEdge(ctx context.Context, branchID domain.BranchID, personID uuid.UUID) (*repository.PedigreeEdge, error) {
	edge, err := s.inner.GetPedigreeEdge(ctx, branchID, personID)
	v := s.privacy(ctx)
	if err != nil || edge == nil || v.SeesAll() {
		return edge, err
	}
	edges, err := s.redactEdges(ctx, v, branchID, []repository.PedigreeEdge{*edge})
	if err != nil {
		return nil, err
	}
	return &edges[0], nil
}

func (s *PrivacyStore) GetPedigreeEdgesByPersonIDs(ctx context.Context, branchID domain.BranchID, personIDs []uuid.UUID) ([]repository.PedigreeEdge, error) {
	edges, err := s.inner.GetPedigreeEdgesByPersonIDs(ctx, branchID, personIDs)
	if v := s.privacy(ctx); err == nil && !v.SeesAll() {
		return s.redactEdges(ctx, v, branchID, edges)
	}
	return edges, err
}

func (s *PrivacyStore) SavePedigreeEdge(ctx context.Context, branchID domain.BranchID, edge *repository.PedigreeEdge) error {
	return s.inner.SavePedigreeEdge(ctx, branchID, edge)
}

func (s *PrivacyStore) DeletePedigreeEdge(ctx context.Context, branchID domain.BranchID, personID uuid.UUID) error {
	return s.inner.DeletePedigreeEdge(ctx, branchID, personID)
}

func (s *PrivacyStore) PurgeBranch(ctx context.Context, branchID domain.BranchID) error {
	return s.inner.PurgeBranch(ctx, branchID)
}

// ============================================================================
// Sources (not yet redacted)
// ============================================================================

func (s *PrivacyStore) GetSource(ctx context.Context, branchID domain.BranchID, id uuid.UUID) (*repository.SourceReadModel, error) {
	return s.inner.GetSource(ctx, branchID, id)
}

func (s *PrivacyStore) GetSourcesByIDs(ctx context.Context, branchID domain.BranchID, ids []uuid.UUID) ([]repository.SourceReadModel, error) {
	return s.inner.GetSourcesByIDs(ctx, branchID, ids)
}

func (s *PrivacyStore) ListSources(ctx context.Context, opts repository.ListOptions) ([]repository.SourceReadModel, int, error) {
	return s.inner.ListSources(ctx, opts)
}

func (s *PrivacyStore) SearchSources(ctx context.Context, branchID domain.BranchID, query string, limit int) ([]repository.SourceReadModel, error) {
	return s.inner.SearchSources(ctx, branchID, query, limit)
}

func (s *PrivacyStore) SaveSource(ctx context.Context, branchID domain.BranchID, source *repository.SourceReadModel) error {
	return s.inner.SaveSource(ctx, branchID, source)
}

func (s *PrivacyStore) DeleteSource(ctx context.Context, branchID domain.BranchID, id uuid.UUID) error {
	return s.inner.DeleteSource(ctx, branchID, id)
}

func (s *PrivacyStore) ReplaceSourceExternalIDs(ctx context.Context, branchID domain.BranchID, sourceID uuid.UUID, ids []repository.SourceExternalIDReadModel) error {
	return s.inner.ReplaceSourceExternalIDs(ctx, branchID, sourceID, ids)
}

func (s *PrivacyStore) GetSourceExternalIDs(ctx context.Context, branchID domain.BranchID, sourceID uuid.UUID) ([]repository.SourceExternalIDReadModel, error) {
	return s.inner.GetSourceExternalIDs(ctx, branchID, sourceID)
}

func (s *PrivacyStore) ListSourceExternalIDs(ctx context.Context, branchID domain.BranchID) (map[uuid.UUID][]repository.SourceExternalIDReadModel, error) {
	return s.inner.ListSourceExternalIDs(ctx, branchID)
}

// ============================================================================
// Citations
// ============================================================================

// Not yet redacted: GetCitation, GetCitationsByIDs, ListCitations,
// GetCitationsForSource, CountCitationsBySource and the citation external IDs.

func (s *PrivacyStore) GetCitation(ctx context.Context, branchID domain.BranchID, id uuid.UUID) (*repository.CitationReadModel, error) {
	return s.inner.GetCitation(ctx, branchID, id)
}

func (s *PrivacyStore) GetCitationsByIDs(ctx context.Context, branchID domain.BranchID, ids []uuid.UUID) ([]repository.CitationReadModel, error) {
	return s.inner.GetCitationsByIDs(ctx, branchID, ids)
}

func (s *PrivacyStore) ListCitations(ctx context.Context, opts repository.ListOptions) ([]repository.CitationReadModel, int, error) {
	return s.inner.ListCitations(ctx, opts)
}

func (s *PrivacyStore) GetCitationsForSource(ctx context.Context, branchID domain.BranchID, sourceID uuid.UUID) ([]repository.CitationReadModel, error) {
	return s.inner.GetCitationsForSource(ctx, branchID, sourceID)
}

func (s *PrivacyStore) CountCitationsBySource(ctx context.Context, branchID domain.BranchID, sourceIDs []uuid.UUID) (map[uuid.UUID]int, error) {
	return s.inner.CountCitationsBySource(ctx, branchID, sourceIDs)
}

// GetCitationsForPerson hides a protected person's citations and drops
// private ones.
func (s *PrivacyStore) GetCitationsForPerson(ctx context.Context, branchID domain.BranchID, personID uuid.UUID) ([]repository.CitationReadModel, error) {
	v := s.privacy(ctx)
	if v.SeesAll() {
		return s.inner.GetCitationsForPerson(ctx, branchID, personID)
	}
	if protected, err := s.personProtected(ctx, v, branchID, personID); err != nil || protected {
		return nil, err
	}
	citations, err := s.inner.GetCitationsForPerson(ctx, branchID, personID)
	return withoutPrivate(v, citations, citationRestriction), err
}

// GetCitationsForFact hides the citations of a protected person's facts and
// of a family's facts it hides, and drops private ones.
func (s *PrivacyStore) GetCitationsForFact(ctx context.Context, branchID domain.BranchID, factType domain.FactType, factOwnerID uuid.UUID) ([]repository.CitationReadModel, error) {
	v := s.privacy(ctx)
	if v.SeesAll() {
		return s.inner.GetCitationsForFact(ctx, branchID, factType, factOwnerID)
	}
	if shown, err := s.ownerShown(ctx, v, branchID, factOwnerType(factType), factOwnerID); err != nil || !shown {
		return nil, err
	}
	citations, err := s.inner.GetCitationsForFact(ctx, branchID, factType, factOwnerID)
	return withoutPrivate(v, citations, citationRestriction), err
}

// GetCitationsForFactInstance is redacted as GetCitationsForFact.
func (s *PrivacyStore) GetCitationsForFactInstance(ctx context.Context, branchID domain.BranchID, factType domain.FactType, factOwnerID, factID uuid.UUID) ([]repository.CitationReadModel, error) {
	v := s.privacy(ctx)
	if v.SeesAll() {
		return s.inner.GetCitationsForFactInstance(ctx, branchID, factType, factOwnerID, factID)
	}
	if shown, err := s.ownerShown(ctx, v, branchID, factOwnerType(factType), factOwnerID); err != nil || !shown {
		return nil, err
	}
	citations, err := s.inner.GetCitationsForFactInstance(ctx, branchID, factType, factOwnerID, factID)
	return withoutPrivate(v, citations, citationRestriction), err
}

func (s *PrivacyStore) SaveCitation(ctx context.Context, branchID domain.BranchID, citation *repository.CitationReadModel) error {
	return s.inner.SaveCitation(ctx, branchID, citation)
}

func (s *PrivacyStore) DeleteCitation(ctx context.Context, branchID domain.BranchID, id uuid.UUID) error {
	return s.inner.DeleteCitation(ctx, branchID, id)
}

func (s *PrivacyStore) ReplaceCitationExternalIDs(ctx context.Context, branchID domain.BranchID, citationID uuid.UUID, ids []repository.CitationExternalIDReadModel) error {
	return s.inner.ReplaceCitationExternalIDs(ctx, branchID, citationID, ids)
}

func (s *PrivacyStore) GetCitationExternalIDs(ctx context.Context, branchID domain.BranchID, citationID uuid.UUID) ([]repository.CitationExternalIDReadModel, error) {
	return s.inner.GetCitationExternalIDs(ctx, branchID, citationID)
}

func (s *PrivacyStore) ListCitationExternalIDs(ctx context.Context, branchID domain.BranchID) (map[uuid.UUID][]repository.CitationExternalIDReadModel, error) {
	return s.inner.ListCitationExternalIDs(ctx, branchID)
}

// ============================================================================
// Media
// ============================================================================

// mediaShown reports whether v may see m: it is not private and its owner
// is shown.
func (s *PrivacyStore) mediaShown(ctx context.Context, v repository.Privacy, branchID domain.BranchID, m *repository.MediaReadModel) (bool, error) {
	if v.Hides(m.Restriction) {
		return false, nil
	}
	return s.ownerShown(ctx, v, branchID, m.EntityType, m.EntityID)
}

func (s *PrivacyStore) GetMedia(ctx context.Context, branchID domain.BranchID, id uuid.UUID) (*repository.MediaReadModel, error) {
	m, err := s.inner.GetMedia(ctx, branchID, id)
	v := s.privacy(ctx)
	if err != nil || m == nil || v.SeesAll() {
		return m, err
	}
	if shown, err := s.mediaShown(ctx, v, branchID, m); err != nil || !shown {
		return nil, err
	}
	return m, nil
}

// GetMediaWithData checks the media row before reading its bytes.
func (s *PrivacyStore) GetMediaWithData(ctx context.Context, branchID domain.BranchID, id uuid.UUID) (*repository.MediaReadModel, error) {
	if m, err := s.GetMedia(ctx, branchID, id); err != nil || m == nil {
		return nil, err
	}
	return s.inner.GetMediaWithData(ctx, branchID, id)
}

// GetMediaThumbnail returns bytes with no owner, so it checks the media row
// first (ADR-008 §5).
func (s *PrivacyStore) GetMediaThumbnail(ctx context.Context, branchID domain.BranchID, id uuid.UUID) ([]byte, error) {
	if m, err := s.GetMedia(ctx, branchID, id); err != nil || m == nil {
		return nil, err
	}
	return s.inner.GetMediaThumbnail(ctx, branchID, id)
}

// ListMediaForEntity hides the media of an owner it hides, and excludes
// private media in the query.
func (s *PrivacyStore) ListMediaForEntity(ctx context.Context, entityType string, entityID uuid.UUID, opts repository.ListOptions) ([]repository.MediaReadModel, int, error) {
	v := s.privacy(ctx)
	if !v.SeesAll() {
		if shown, err := s.ownerShown(ctx, v, opts.BranchID, entityType, entityID); err != nil || !shown {
			return nil, 0, err
		}
	}
	opts.Privacy = v
	return s.inner.ListMediaForEntity(ctx, entityType, entityID, opts)
}

func (s *PrivacyStore) SaveMedia(ctx context.Context, branchID domain.BranchID, media *repository.MediaReadModel) error {
	return s.inner.SaveMedia(ctx, branchID, media)
}

func (s *PrivacyStore) DeleteMedia(ctx context.Context, branchID domain.BranchID, id uuid.UUID) error {
	return s.inner.DeleteMedia(ctx, branchID, id)
}

// ============================================================================
// Life events and attributes
// ============================================================================

func (s *PrivacyStore) GetEvent(ctx context.Context, branchID domain.BranchID, id uuid.UUID) (*repository.EventReadModel, error) {
	e, err := s.inner.GetEvent(ctx, branchID, id)
	v := s.privacy(ctx)
	if err != nil || e == nil || v.SeesAll() {
		return e, err
	}
	if shown, err := s.ownerShown(ctx, v, branchID, e.OwnerType, e.OwnerID); err != nil || !shown {
		return nil, err
	}
	return e, nil
}

// ListEvents is not yet redacted; only the editor-only exports call it.
func (s *PrivacyStore) ListEvents(ctx context.Context, opts repository.ListOptions) ([]repository.EventReadModel, int, error) {
	return s.inner.ListEvents(ctx, opts)
}

func (s *PrivacyStore) ListEventsForPerson(ctx context.Context, branchID domain.BranchID, personID uuid.UUID) ([]repository.EventReadModel, error) {
	if v := s.privacy(ctx); !v.SeesAll() {
		if protected, err := s.personProtected(ctx, v, branchID, personID); err != nil || protected {
			return nil, err
		}
	}
	return s.inner.ListEventsForPerson(ctx, branchID, personID)
}

func (s *PrivacyStore) ListEventsForFamily(ctx context.Context, branchID domain.BranchID, familyID uuid.UUID) ([]repository.EventReadModel, error) {
	if v := s.privacy(ctx); !v.SeesAll() {
		if shown, err := s.familyShown(ctx, v, branchID, familyID); err != nil || !shown {
			return nil, err
		}
	}
	return s.inner.ListEventsForFamily(ctx, branchID, familyID)
}

func (s *PrivacyStore) SaveEvent(ctx context.Context, branchID domain.BranchID, event *repository.EventReadModel) error {
	return s.inner.SaveEvent(ctx, branchID, event)
}

func (s *PrivacyStore) DeleteEvent(ctx context.Context, branchID domain.BranchID, id uuid.UUID) error {
	return s.inner.DeleteEvent(ctx, branchID, id)
}

func (s *PrivacyStore) GetAttribute(ctx context.Context, branchID domain.BranchID, id uuid.UUID) (*repository.AttributeReadModel, error) {
	a, err := s.inner.GetAttribute(ctx, branchID, id)
	v := s.privacy(ctx)
	if err != nil || a == nil || v.SeesAll() {
		return a, err
	}
	if protected, err := s.personProtected(ctx, v, branchID, a.PersonID); err != nil || protected {
		return nil, err
	}
	return a, nil
}

// ListAttributes is not yet redacted; only the editor-only exports call it.
func (s *PrivacyStore) ListAttributes(ctx context.Context, opts repository.ListOptions) ([]repository.AttributeReadModel, int, error) {
	return s.inner.ListAttributes(ctx, opts)
}

func (s *PrivacyStore) ListAttributesForPerson(ctx context.Context, branchID domain.BranchID, personID uuid.UUID) ([]repository.AttributeReadModel, error) {
	if v := s.privacy(ctx); !v.SeesAll() {
		if protected, err := s.personProtected(ctx, v, branchID, personID); err != nil || protected {
			return nil, err
		}
	}
	return s.inner.ListAttributesForPerson(ctx, branchID, personID)
}

func (s *PrivacyStore) SaveAttribute(ctx context.Context, branchID domain.BranchID, attribute *repository.AttributeReadModel) error {
	return s.inner.SaveAttribute(ctx, branchID, attribute)
}

func (s *PrivacyStore) DeleteAttribute(ctx context.Context, branchID domain.BranchID, id uuid.UUID) error {
	return s.inner.DeleteAttribute(ctx, branchID, id)
}

// ============================================================================
// Notes, submitters and repositories (not yet redacted)
// ============================================================================

func (s *PrivacyStore) GetNote(ctx context.Context, branchID domain.BranchID, id uuid.UUID) (*repository.NoteReadModel, error) {
	return s.inner.GetNote(ctx, branchID, id)
}

func (s *PrivacyStore) ListNotes(ctx context.Context, opts repository.ListOptions) ([]repository.NoteReadModel, int, error) {
	return s.inner.ListNotes(ctx, opts)
}

func (s *PrivacyStore) SaveNote(ctx context.Context, branchID domain.BranchID, note *repository.NoteReadModel) error {
	return s.inner.SaveNote(ctx, branchID, note)
}

func (s *PrivacyStore) DeleteNote(ctx context.Context, branchID domain.BranchID, id uuid.UUID) error {
	return s.inner.DeleteNote(ctx, branchID, id)
}

func (s *PrivacyStore) GetSubmitter(ctx context.Context, id uuid.UUID) (*repository.SubmitterReadModel, error) {
	return s.inner.GetSubmitter(ctx, id)
}

func (s *PrivacyStore) ListSubmitters(ctx context.Context, opts repository.ListOptions) ([]repository.SubmitterReadModel, int, error) {
	return s.inner.ListSubmitters(ctx, opts)
}

func (s *PrivacyStore) SaveSubmitter(ctx context.Context, submitter *repository.SubmitterReadModel) error {
	return s.inner.SaveSubmitter(ctx, submitter)
}

func (s *PrivacyStore) DeleteSubmitter(ctx context.Context, id uuid.UUID) error {
	return s.inner.DeleteSubmitter(ctx, id)
}

func (s *PrivacyStore) GetRepository(ctx context.Context, id uuid.UUID) (*repository.RepositoryReadModel, error) {
	return s.inner.GetRepository(ctx, id)
}

func (s *PrivacyStore) ListRepositories(ctx context.Context, opts repository.ListOptions) ([]repository.RepositoryReadModel, int, error) {
	return s.inner.ListRepositories(ctx, opts)
}

func (s *PrivacyStore) SaveRepository(ctx context.Context, repo *repository.RepositoryReadModel) error {
	return s.inner.SaveRepository(ctx, repo)
}

func (s *PrivacyStore) DeleteRepository(ctx context.Context, id uuid.UUID) error {
	return s.inner.DeleteRepository(ctx, id)
}

func (s *PrivacyStore) ReplaceRepositoryExternalIDs(ctx context.Context, repositoryID uuid.UUID, ids []repository.RepositoryExternalIDReadModel) error {
	return s.inner.ReplaceRepositoryExternalIDs(ctx, repositoryID, ids)
}

func (s *PrivacyStore) GetRepositoryExternalIDs(ctx context.Context, repositoryID uuid.UUID) ([]repository.RepositoryExternalIDReadModel, error) {
	return s.inner.GetRepositoryExternalIDs(ctx, repositoryID)
}

func (s *PrivacyStore) ListRepositoryExternalIDs(ctx context.Context) (map[uuid.UUID][]repository.RepositoryExternalIDReadModel, error) {
	return s.inner.ListRepositoryExternalIDs(ctx)
}

// ============================================================================
// Associations and LDS ordinances
// ============================================================================

// Not yet redacted: GetAssociation, ListAssociations, GetLDSOrdinance and
// ListLDSOrdinances.

func (s *PrivacyStore) GetAssociation(ctx context.Context, branchID domain.BranchID, id uuid.UUID) (*repository.AssociationReadModel, error) {
	return s.inner.GetAssociation(ctx, branchID, id)
}

func (s *PrivacyStore) ListAssociations(ctx context.Context, opts repository.ListOptions) ([]repository.AssociationReadModel, int, error) {
	return s.inner.ListAssociations(ctx, opts)
}

// ListAssociationsForPerson hides a protected person's associations, and
// drops those whose other end is protected.
func (s *PrivacyStore) ListAssociationsForPerson(ctx context.Context, branchID domain.BranchID, personID uuid.UUID) ([]repository.AssociationReadModel, error) {
	associations, err := s.inner.ListAssociationsForPerson(ctx, branchID, personID)
	v := s.privacy(ctx)
	if err != nil || v.SeesAll() {
		return associations, err
	}
	ids := []uuid.UUID{personID}
	for _, a := range associations {
		ids = append(ids, a.PersonID, a.AssociateID)
	}
	vis, err := s.visibilities(ctx, v, branchID, ids)
	if err != nil || vis[personID] != repository.Visible {
		return nil, err
	}
	out := make([]repository.AssociationReadModel, 0, len(associations))
	for _, a := range associations {
		if vis[a.PersonID] == repository.Visible && vis[a.AssociateID] == repository.Visible {
			out = append(out, a)
		}
	}
	return out, nil
}

func (s *PrivacyStore) SaveAssociation(ctx context.Context, branchID domain.BranchID, association *repository.AssociationReadModel) error {
	return s.inner.SaveAssociation(ctx, branchID, association)
}

func (s *PrivacyStore) DeleteAssociation(ctx context.Context, branchID domain.BranchID, id uuid.UUID) error {
	return s.inner.DeleteAssociation(ctx, branchID, id)
}

func (s *PrivacyStore) GetLDSOrdinance(ctx context.Context, id uuid.UUID) (*repository.LDSOrdinanceReadModel, error) {
	return s.inner.GetLDSOrdinance(ctx, id)
}

func (s *PrivacyStore) ListLDSOrdinances(ctx context.Context, opts repository.ListOptions) ([]repository.LDSOrdinanceReadModel, int, error) {
	return s.inner.ListLDSOrdinances(ctx, opts)
}

// ListLDSOrdinancesForPerson hides a protected person's ordinances. LDS
// ordinances are mainline-only, so the person is resolved on the mainline.
func (s *PrivacyStore) ListLDSOrdinancesForPerson(ctx context.Context, personID uuid.UUID) ([]repository.LDSOrdinanceReadModel, error) {
	if v := s.privacy(ctx); !v.SeesAll() {
		if protected, err := s.personProtected(ctx, v, domain.MainBranchID, personID); err != nil || protected {
			return nil, err
		}
	}
	return s.inner.ListLDSOrdinancesForPerson(ctx, personID)
}

// ListLDSOrdinancesForFamily hides the ordinances of a family whose facts it
// hides.
func (s *PrivacyStore) ListLDSOrdinancesForFamily(ctx context.Context, familyID uuid.UUID) ([]repository.LDSOrdinanceReadModel, error) {
	if v := s.privacy(ctx); !v.SeesAll() {
		if shown, err := s.familyShown(ctx, v, domain.MainBranchID, familyID); err != nil || !shown {
			return nil, err
		}
	}
	return s.inner.ListLDSOrdinancesForFamily(ctx, familyID)
}

func (s *PrivacyStore) SaveLDSOrdinance(ctx context.Context, ordinance *repository.LDSOrdinanceReadModel) error {
	return s.inner.SaveLDSOrdinance(ctx, ordinance)
}

func (s *PrivacyStore) DeleteLDSOrdinance(ctx context.Context, id uuid.UUID) error {
	return s.inner.DeleteLDSOrdinance(ctx, id)
}

// ============================================================================
// Evidence and duplicates (not yet redacted)
// ============================================================================

func (s *PrivacyStore) GetEvidenceAnalysis(ctx context.Context, branchID domain.BranchID, id uuid.UUID) (*repository.EvidenceAnalysisReadModel, error) {
	return s.inner.GetEvidenceAnalysis(ctx, branchID, id)
}

func (s *PrivacyStore) ListEvidenceAnalyses(ctx context.Context, opts repository.ListOptions) ([]repository.EvidenceAnalysisReadModel, int, error) {
	return s.inner.ListEvidenceAnalyses(ctx, opts)
}

func (s *PrivacyStore) GetAnalysesForFact(ctx context.Context, branchID domain.BranchID, factType domain.FactType, subjectID uuid.UUID) ([]repository.EvidenceAnalysisReadModel, error) {
	return s.inner.GetAnalysesForFact(ctx, branchID, factType, subjectID)
}

func (s *PrivacyStore) GetAnalysesBySubject(ctx context.Context, branchID domain.BranchID, subjectID uuid.UUID) ([]repository.EvidenceAnalysisReadModel, error) {
	return s.inner.GetAnalysesBySubject(ctx, branchID, subjectID)
}

func (s *PrivacyStore) SaveEvidenceAnalysis(ctx context.Context, branchID domain.BranchID, analysis *repository.EvidenceAnalysisReadModel) error {
	return s.inner.SaveEvidenceAnalysis(ctx, branchID, analysis)
}

func (s *PrivacyStore) DeleteEvidenceAnalysis(ctx context.Context, branchID domain.BranchID, id uuid.UUID) error {
	return s.inner.DeleteEvidenceAnalysis(ctx, branchID, id)
}

func (s *PrivacyStore) GetEvidenceConflict(ctx context.Context, branchID domain.BranchID, id uuid.UUID) (*repository.EvidenceConflictReadModel, error) {
	return s.inner.GetEvidenceConflict(ctx, branchID, id)
}

func (s *PrivacyStore) ListEvidenceConflicts(ctx context.Context, opts repository.ListOptions) ([]repository.EvidenceConflictReadModel, int, error) {
	return s.inner.ListEvidenceConflicts(ctx, opts)
}

func (s *PrivacyStore) GetConflictsForSubject(ctx context.Context, branchID domain.BranchID, subjectID uuid.UUID) ([]repository.EvidenceConflictReadModel, error) {
	return s.inner.GetConflictsForSubject(ctx, branchID, subjectID)
}

func (s *PrivacyStore) ListUnresolvedConflicts(ctx context.Context, branchID domain.BranchID) ([]repository.EvidenceConflictReadModel, error) {
	return s.inner.ListUnresolvedConflicts(ctx, branchID)
}

func (s *PrivacyStore) SaveEvidenceConflict(ctx context.Context, branchID domain.BranchID, conflict *repository.EvidenceConflictReadModel) error {
	return s.inner.SaveEvidenceConflict(ctx, branchID, conflict)
}

func (s *PrivacyStore) DeleteEvidenceConflict(ctx context.Context, branchID domain.BranchID, id uuid.UUID) error {
	return s.inner.DeleteEvidenceConflict(ctx, branchID, id)
}

func (s *PrivacyStore) GetResearchLog(ctx context.Context, branchID domain.BranchID, id uuid.UUID) (*repository.ResearchLogReadModel, error) {
	return s.inner.GetResearchLog(ctx, branchID, id)
}

func (s *PrivacyStore) ListResearchLogs(ctx context.Context, opts repository.ListOptions) ([]repository.ResearchLogReadModel, int, error) {
	return s.inner.ListResearchLogs(ctx, opts)
}

func (s *PrivacyStore) GetResearchLogsForSubject(ctx context.Context, branchID domain.BranchID, subjectID uuid.UUID) ([]repository.ResearchLogReadModel, error) {
	return s.inner.GetResearchLogsForSubject(ctx, branchID, subjectID)
}

func (s *PrivacyStore) SaveResearchLog(ctx context.Context, branchID domain.BranchID, log *repository.ResearchLogReadModel) error {
	return s.inner.SaveResearchLog(ctx, branchID, log)
}

func (s *PrivacyStore) DeleteResearchLog(ctx context.Context, branchID domain.BranchID, id uuid.UUID) error {
	return s.inner.DeleteResearchLog(ctx, branchID, id)
}

func (s *PrivacyStore) GetProofSummary(ctx context.Context, branchID domain.BranchID, id uuid.UUID) (*repository.ProofSummaryReadModel, error) {
	return s.inner.GetProofSummary(ctx, branchID, id)
}

func (s *PrivacyStore) ListProofSummaries(ctx context.Context, opts repository.ListOptions) ([]repository.ProofSummaryReadModel, int, error) {
	return s.inner.ListProofSummaries(ctx, opts)
}

func (s *PrivacyStore) GetProofSummariesForFact(ctx context.Context, branchID domain.BranchID, factType domain.FactType, subjectID uuid.UUID) ([]repository.ProofSummaryReadModel, error) {
	return s.inner.GetProofSummariesForFact(ctx, branchID, factType, subjectID)
}

func (s *PrivacyStore) GetProofSummariesBySubject(ctx context.Context, branchID domain.BranchID, subjectID uuid.UUID) ([]repository.ProofSummaryReadModel, error) {
	return s.inner.GetProofSummariesBySubject(ctx, branchID, subjectID)
}

func (s *PrivacyStore) SaveProofSummary(ctx context.Context, branchID domain.BranchID, summary *repository.ProofSummaryReadModel) error {
	return s.inner.SaveProofSummary(ctx, branchID, summary)
}

func (s *PrivacyStore) DeleteProofSummary(ctx context.Context, branchID domain.BranchID, id uuid.UUID) error {
	return s.inner.DeleteProofSummary(ctx, branchID, id)
}

func (s *PrivacyStore) GetDuplicateDismissal(ctx context.Context, branchID domain.BranchID, id uuid.UUID) (*repository.DuplicateDismissalReadModel, error) {
	return s.inner.GetDuplicateDismissal(ctx, branchID, id)
}

func (s *PrivacyStore) ListDuplicateDismissals(ctx context.Context, opts repository.ListOptions) ([]repository.DuplicateDismissalReadModel, int, error) {
	return s.inner.ListDuplicateDismissals(ctx, opts)
}

func (s *PrivacyStore) SaveDuplicateDismissal(ctx context.Context, branchID domain.BranchID, dismissal *repository.DuplicateDismissalReadModel) error {
	return s.inner.SaveDuplicateDismissal(ctx, branchID, dismissal)
}

func (s *PrivacyStore) DeleteDuplicateDismissal(ctx context.Context, branchID domain.BranchID, id uuid.UUID) error {
	return s.inner.DeleteDuplicateDismissal(ctx, branchID, id)
}

// ============================================================================
// Browse, map and brick walls
// ============================================================================

// Not yet redacted: the surname, place and cemetery indexes, the map and the
// brick walls count and name people in the store's aggregate queries.

func (s *PrivacyStore) GetSurnameIndex(ctx context.Context, branchID domain.BranchID) ([]repository.SurnameEntry, []repository.LetterCount, error) {
	return s.inner.GetSurnameIndex(ctx, branchID)
}

func (s *PrivacyStore) GetSurnamesByLetter(ctx context.Context, branchID domain.BranchID, letter string) ([]repository.SurnameEntry, error) {
	return s.inner.GetSurnamesByLetter(ctx, branchID, letter)
}

func (s *PrivacyStore) GetPersonsBySurname(ctx context.Context, surname string, opts repository.ListOptions) ([]repository.PersonReadModel, int, error) {
	opts.Privacy = s.privacy(ctx)
	return s.inner.GetPersonsBySurname(ctx, surname, opts)
}

func (s *PrivacyStore) GetPlaceHierarchy(ctx context.Context, branchID domain.BranchID, parent string) ([]repository.PlaceEntry, error) {
	return s.inner.GetPlaceHierarchy(ctx, branchID, parent)
}

func (s *PrivacyStore) GetPersonsByPlace(ctx context.Context, place string, opts repository.ListOptions) ([]repository.PersonReadModel, int, error) {
	opts.Privacy = s.privacy(ctx)
	return s.inner.GetPersonsByPlace(ctx, place, opts)
}

func (s *PrivacyStore) GetCemeteryIndex(ctx context.Context, branchID domain.BranchID) ([]repository.CemeteryEntry, error) {
	return s.inner.GetCemeteryIndex(ctx, branchID)
}

func (s *PrivacyStore) GetPersonsByCemetery(ctx context.Context, place string, opts repository.ListOptions) ([]repository.PersonReadModel, int, error) {
	opts.Privacy = s.privacy(ctx)
	return s.inner.GetPersonsByCemetery(ctx, place, opts)
}

func (s *PrivacyStore) GetMapLocations(ctx context.Context, branchID domain.BranchID) ([]repository.MapLocation, error) {
	return s.inner.GetMapLocations(ctx, branchID)
}

func (s *PrivacyStore) GetBrickWalls(ctx context.Context, includeResolved bool) ([]repository.BrickWallEntry, error) {
	return s.inner.GetBrickWalls(ctx, includeResolved)
}

// ============================================================================
// Read-model administration (no tree data)
// ============================================================================

func (s *PrivacyStore) ResetReadModel(ctx context.Context) error {
	return s.inner.ResetReadModel(ctx)
}

func (s *PrivacyStore) GetSchemaVersion(ctx context.Context) (int, error) {
	return s.inner.GetSchemaVersion(ctx)
}

func (s *PrivacyStore) SetSchemaVersion(ctx context.Context, version int) error {
	return s.inner.SetSchemaVersion(ctx, version)
}

func (s *PrivacyStore) SaveRebuildState(ctx context.Context, state []byte) error {
	return s.inner.SaveRebuildState(ctx, state)
}

func (s *PrivacyStore) GetRebuildState(ctx context.Context) ([]byte, error) {
	return s.inner.GetRebuildState(ctx)
}

func (s *PrivacyStore) ClearRebuildState(ctx context.Context) error {
	return s.inner.ClearRebuildState(ctx)
}

func (s *PrivacyStore) RecordProjectionFailure(ctx context.Context, failure repository.ProjectionFailure) error {
	return s.inner.RecordProjectionFailure(ctx, failure)
}

func (s *PrivacyStore) ListProjectionFailures(ctx context.Context, limit int) ([]repository.ProjectionFailure, error) {
	return s.inner.ListProjectionFailures(ctx, limit)
}
