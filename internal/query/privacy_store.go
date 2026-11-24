package query

import (
	"bytes"
	"context"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/gedcom"
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
//   - A private family is hidden, and so is the child-to-parents link its
//     pedigree edges carry; a family with a protected partner shows the
//     partner as a placeholder and hides its own facts, events, external IDs,
//     LDS ordinances and media.
//   - Private media are hidden, and so are media of a hidden owner, bytes and
//     thumbnails included.
//   - The surname, place and cemetery indexes, the place list, the map and
//     the brick walls leave protected people out of their counts and lists,
//     in the query; the place list also leaves out what belongs to a hidden
//     person or family (ADR-009 §10).
//   - Private sources, citations and notes are hidden. So is a citation of a
//     private source or of a fact whose owner is hidden, and a shared note
//     every record pointing to which is hidden (ADR-008 §3); a source's
//     citation count counts only the citations shown.
//   - An association, LDS ordinance, evidence record, research log, proof
//     summary, duplicate dismissal, life event or attribute is hidden when
//     a person or family it belongs to is.
//   - Submitters are hidden from anonymous visitors (ADR-008 §4).
//
// A list whose rule needs other records (an evidence record's subject, a
// shared note's referrers) is read whole, filtered, then paged (listShown),
// so its pages are full and its total counts only what is shown.
// Repositories hold no person data and pass through. The event log is the
// other choke point's (ADR-008 §5b).
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

// owner is a record others belong to: a person, family or source (typ), and
// its id.
type owner struct {
	typ string
	id  uuid.UUID
}

// ownersShown reports, for each of owners, whether v may see what belongs to
// it: a person shown in full, a family whose own facts are shown (see
// familiesShown), a source that is not private. Any other owner, or one that
// does not resolve, is not shown (ADR-008 §3).
func (s *PrivacyStore) ownersShown(ctx context.Context, v repository.Privacy, branchID domain.BranchID, owners []owner) (map[owner]bool, error) {
	var people, families, sources []uuid.UUID
	for _, o := range owners {
		switch o.typ {
		case "person":
			people = append(people, o.id)
		case "family":
			families = append(families, o.id)
		case "source":
			sources = append(sources, o.id)
		}
	}
	shown := make(map[owner]bool, len(owners))
	vis, err := s.visibilities(ctx, v, branchID, people)
	if err != nil {
		return nil, err
	}
	for id, pv := range vis {
		shown[owner{"person", id}] = pv == repository.Visible
	}
	familyShown, err := s.familiesShown(ctx, v, branchID, families)
	if err != nil {
		return nil, err
	}
	for id := range familyShown {
		shown[owner{"family", id}] = true
	}
	if len(sources) > 0 {
		rows, err := s.inner.GetSourcesByIDs(ctx, branchID, sources)
		if err != nil {
			return nil, err
		}
		for _, src := range rows {
			shown[owner{"source", src.ID}] = !v.Hides(src.Restriction)
		}
	}
	return shown, nil
}

// ownerShown reports whether v may see what belongs to the record ownerType
// and id name (see ownersShown).
func (s *PrivacyStore) ownerShown(ctx context.Context, v repository.Privacy, branchID domain.BranchID, ownerType string, id uuid.UUID) (bool, error) {
	o := owner{ownerType, id}
	shown, err := s.ownersShown(ctx, v, branchID, []owner{o})
	return shown[o], err
}

// keepShown returns the rows v may see every owner of, in order.
func keepShown[T any](ctx context.Context, s *PrivacyStore, v repository.Privacy, branchID domain.BranchID, rows []T, owners func(*T) []owner) ([]T, error) {
	var all []owner
	for i := range rows {
		all = append(all, owners(&rows[i])...)
	}
	shown, err := s.ownersShown(ctx, v, branchID, all)
	if err != nil {
		return nil, err
	}
	out := make([]T, 0, len(rows))
	for i := range rows {
		if !slices.ContainsFunc(owners(&rows[i]), func(o owner) bool { return !shown[o] }) {
			out = append(out, rows[i])
		}
	}
	return out, nil
}

// rowsShown returns the rows the principal on ctx may see every owner of.
func rowsShown[T any](ctx context.Context, s *PrivacyStore, branchID domain.BranchID, rows []T, err error, owners func(*T) []owner) ([]T, error) {
	v := s.privacy(ctx)
	if err != nil || v.SeesAll() {
		return rows, err
	}
	return keepShown(ctx, s, v, branchID, rows, owners)
}

// rowShown returns row when the principal on ctx may see every owner of it,
// else nil.
func rowShown[T any](ctx context.Context, s *PrivacyStore, branchID domain.BranchID, row *T, err error, owners func(*T) []owner) (*T, error) {
	if err != nil || row == nil {
		return row, err
	}
	rows, err := rowsShown(ctx, s, branchID, []T{*row}, nil, owners)
	if err != nil || len(rows) == 0 {
		return nil, err
	}
	return &rows[0], nil
}

// listPageSize is the page size listShown reads a whole list with.
const listPageSize = 1000

// listShown reads every row list returns for opts, with its filters and
// order, keeps those keep returns, and pages them as opts asks. The rule is
// applied to the whole list before paging, never to a page, so pages are full
// and the total counts only the rows kept (ADR-008 §5).
func listShown[T any](ctx context.Context, opts repository.ListOptions, list func(context.Context, repository.ListOptions) ([]T, int, error), keep func([]T) ([]T, error)) ([]T, int, error) {
	var all []T
	page := opts
	page.Limit = listPageSize
	for page.Offset = 0; ; page.Offset += listPageSize {
		rows, total, err := list(ctx, page)
		if err != nil {
			return nil, 0, err
		}
		all = append(all, rows...)
		if len(rows) < listPageSize || len(all) >= total {
			break
		}
	}
	kept, err := keep(all)
	if err != nil {
		return nil, 0, err
	}
	return repository.Page(kept, opts.Offset, opts.Limit), len(kept), nil
}

// pageShown is list for the principal on ctx: the page of the rows whose
// every owner they may see.
func pageShown[T any](ctx context.Context, s *PrivacyStore, opts repository.ListOptions, list func(context.Context, repository.ListOptions) ([]T, int, error), owners func(*T) []owner) ([]T, int, error) {
	v := s.privacy(ctx)
	if v.SeesAll() {
		return list(ctx, opts)
	}
	return listShown(ctx, opts, list, func(rows []T) ([]T, error) {
		return keepShown(ctx, s, v, opts.BranchID, rows, owners)
	})
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
func sourceRestriction(r *repository.SourceReadModel) domain.Restriction     { return r.Restriction }
func noteRestriction(n *repository.NoteReadModel) domain.Restriction         { return n.Restriction }

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

// redactEdges drops the parents an edge takes from a private family, so a
// chart shows a gap there as the family pages do, and shows protected parents
// by the placeholder name (ADR-008 §3).
func (s *PrivacyStore) redactEdges(ctx context.Context, v repository.Privacy, branchID domain.BranchID, edges []repository.PedigreeEdge) ([]repository.PedigreeEdge, error) {
	var ids []uuid.UUID
	for _, e := range edges {
		for _, id := range []*uuid.UUID{e.FatherID, e.MotherID} {
			if id != nil {
				ids = append(ids, *id)
			}
		}
	}
	private, err := s.privateParentage(ctx, v, branchID, ids)
	if err != nil {
		return nil, err
	}
	vis, err := s.visibilities(ctx, v, branchID, ids)
	if err != nil {
		return nil, err
	}
	for i := range edges {
		if private[edgeKey{edges[i].PersonID, partnerSet(edges[i].FatherID, edges[i].MotherID)}] {
			edges[i].FatherID, edges[i].FatherName = nil, ""
			edges[i].MotherID, edges[i].MotherName = nil, ""
			continue
		}
		if id := edges[i].FatherID; id != nil && vis[*id] != repository.Visible {
			edges[i].FatherName = repository.PlaceholderGivenName
		}
		if id := edges[i].MotherID; id != nil && vis[*id] != repository.Visible {
			edges[i].MotherName = repository.PlaceholderGivenName
		}
	}
	return edges, nil
}

// edgeKey names a child's link to a pair of parents: the child and the
// parents' partnerSet.
type edgeKey struct {
	child   uuid.UUID
	parents [2]uuid.UUID
}

// partnerSet returns a and b in a fixed order, with uuid.Nil for one that is
// absent, so an edge (father, mother) and a family (partner1, partner2) with
// the same people compare equal.
func partnerSet(a, b *uuid.UUID) [2]uuid.UUID {
	var x, y uuid.UUID
	if a != nil {
		x = *a
	}
	if b != nil {
		y = *b
	}
	if bytes.Compare(x[:], y[:]) > 0 {
		x, y = y, x
	}
	return [2]uuid.UUID{x, y}
}

// privateParentage returns the child-to-parents links that a private family
// of parentIDs records, which v may not see. A pedigree edge carries no
// family ID, so a link is matched by the child and the family's partners.
func (s *PrivacyStore) privateParentage(ctx context.Context, v repository.Privacy, branchID domain.BranchID, parentIDs []uuid.UUID) (map[edgeKey]bool, error) {
	out := make(map[edgeKey]bool)
	if !v.HidePrivate || len(parentIDs) == 0 {
		return out, nil
	}
	families, err := s.inner.GetFamiliesForPersons(ctx, branchID, parentIDs)
	if err != nil {
		return nil, err
	}
	partners := make(map[uuid.UUID][2]uuid.UUID)
	var private []uuid.UUID
	for _, f := range families {
		if v.Hides(f.Restriction) {
			partners[f.ID] = partnerSet(f.Partner1ID, f.Partner2ID)
			private = append(private, f.ID)
		}
	}
	if len(private) == 0 {
		return out, nil
	}
	children, err := s.inner.GetFamilyChildrenByFamilyIDs(ctx, branchID, private)
	if err != nil {
		return nil, err
	}
	for _, c := range children {
		out[edgeKey{c.PersonID, partners[c.FamilyID]}] = true
	}
	return out, nil
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
// Sources
// ============================================================================

// withShownCitationCounts sets each source's citation count to the number
// of its citations the principal on ctx may see.
func (s *PrivacyStore) withShownCitationCounts(ctx context.Context, branchID domain.BranchID, sources []repository.SourceReadModel) ([]repository.SourceReadModel, error) {
	ids := make([]uuid.UUID, len(sources))
	for i := range sources {
		ids[i] = sources[i].ID
	}
	counts, err := s.CountCitationsBySource(ctx, branchID, ids)
	if err != nil {
		return nil, err
	}
	for i := range sources {
		sources[i].CitationCount = counts[sources[i].ID]
	}
	return sources, nil
}

// shownSources drops the private sources and counts only the citations
// shown.
func (s *PrivacyStore) shownSources(ctx context.Context, v repository.Privacy, branchID domain.BranchID, sources []repository.SourceReadModel) ([]repository.SourceReadModel, error) {
	return s.withShownCitationCounts(ctx, branchID, withoutPrivate(v, sources, sourceRestriction))
}

func (s *PrivacyStore) GetSource(ctx context.Context, branchID domain.BranchID, id uuid.UUID) (*repository.SourceReadModel, error) {
	src, err := s.inner.GetSource(ctx, branchID, id)
	v := s.privacy(ctx)
	if err != nil || src == nil || v.SeesAll() {
		return src, err
	}
	shown, err := s.shownSources(ctx, v, branchID, []repository.SourceReadModel{*src})
	if err != nil || len(shown) == 0 {
		return nil, err
	}
	return &shown[0], nil
}

func (s *PrivacyStore) GetSourcesByIDs(ctx context.Context, branchID domain.BranchID, ids []uuid.UUID) ([]repository.SourceReadModel, error) {
	sources, err := s.inner.GetSourcesByIDs(ctx, branchID, ids)
	if v := s.privacy(ctx); err == nil && !v.SeesAll() {
		return s.shownSources(ctx, v, branchID, sources)
	}
	return sources, err
}

func (s *PrivacyStore) ListSources(ctx context.Context, opts repository.ListOptions) ([]repository.SourceReadModel, int, error) {
	v := s.privacy(ctx)
	if v.SeesAll() {
		return s.inner.ListSources(ctx, opts)
	}
	sources, total, err := listShown(ctx, opts, s.inner.ListSources, func(rows []repository.SourceReadModel) ([]repository.SourceReadModel, error) {
		return withoutPrivate(v, rows, sourceRestriction), nil
	})
	if err != nil {
		return nil, 0, err
	}
	sources, err = s.withShownCitationCounts(ctx, opts.BranchID, sources)
	return sources, total, err
}

// SearchSources asks the store for more matches until limit of them are
// shown or there are no more, so private sources do not shorten the result.
func (s *PrivacyStore) SearchSources(ctx context.Context, branchID domain.BranchID, query string, limit int) ([]repository.SourceReadModel, error) {
	v := s.privacy(ctx)
	if v.SeesAll() {
		return s.inner.SearchSources(ctx, branchID, query, limit)
	}
	for n := max(limit, 1); ; n *= 2 {
		matches, err := s.inner.SearchSources(ctx, branchID, query, n)
		if err != nil {
			return nil, err
		}
		shown := withoutPrivate(v, matches, sourceRestriction)
		if len(shown) >= limit || len(matches) < n {
			return s.withShownCitationCounts(ctx, branchID, shown[:min(len(shown), max(limit, 0))])
		}
	}
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
	if v := s.privacy(ctx); !v.SeesAll() {
		if shown, err := s.ownerShown(ctx, v, branchID, "source", sourceID); err != nil || !shown {
			return nil, err
		}
	}
	return s.inner.GetSourceExternalIDs(ctx, branchID, sourceID)
}

func (s *PrivacyStore) ListSourceExternalIDs(ctx context.Context, branchID domain.BranchID) (map[uuid.UUID][]repository.SourceExternalIDReadModel, error) {
	all, err := s.inner.ListSourceExternalIDs(ctx, branchID)
	v := s.privacy(ctx)
	if err != nil || v.SeesAll() {
		return all, err
	}
	owners := make([]owner, 0, len(all))
	for id := range all {
		owners = append(owners, owner{"source", id})
	}
	shown, err := s.ownersShown(ctx, v, branchID, owners)
	if err != nil {
		return nil, err
	}
	for id := range all {
		if !shown[owner{"source", id}] {
			delete(all, id)
		}
	}
	return all, nil
}

// ============================================================================
// Citations
// ============================================================================

// citationOwners are the records a citation belongs to: the owner of the
// cited fact, and the source.
func citationOwners(c *repository.CitationReadModel) []owner {
	return []owner{{factOwnerType(c.FactType), c.FactOwnerID}, {"source", c.SourceID}}
}

// citationsShown drops the private citations, and those of a source or a
// fact owner the principal on ctx may not see.
func (s *PrivacyStore) citationsShown(ctx context.Context, branchID domain.BranchID, citations []repository.CitationReadModel, err error) ([]repository.CitationReadModel, error) {
	if v := s.privacy(ctx); err == nil && !v.SeesAll() {
		citations = withoutPrivate(v, citations, citationRestriction)
	}
	return rowsShown(ctx, s, branchID, citations, err, citationOwners)
}

func (s *PrivacyStore) GetCitation(ctx context.Context, branchID domain.BranchID, id uuid.UUID) (*repository.CitationReadModel, error) {
	c, err := s.inner.GetCitation(ctx, branchID, id)
	if err != nil || c == nil {
		return c, err
	}
	shown, err := s.citationsShown(ctx, branchID, []repository.CitationReadModel{*c}, nil)
	if err != nil || len(shown) == 0 {
		return nil, err
	}
	return &shown[0], nil
}

func (s *PrivacyStore) GetCitationsByIDs(ctx context.Context, branchID domain.BranchID, ids []uuid.UUID) ([]repository.CitationReadModel, error) {
	citations, err := s.inner.GetCitationsByIDs(ctx, branchID, ids)
	return s.citationsShown(ctx, branchID, citations, err)
}

func (s *PrivacyStore) ListCitations(ctx context.Context, opts repository.ListOptions) ([]repository.CitationReadModel, int, error) {
	if v := s.privacy(ctx); v.SeesAll() {
		return s.inner.ListCitations(ctx, opts)
	}
	return listShown(ctx, opts, s.inner.ListCitations, func(rows []repository.CitationReadModel) ([]repository.CitationReadModel, error) {
		return s.citationsShown(ctx, opts.BranchID, rows, nil)
	})
}

func (s *PrivacyStore) GetCitationsForSource(ctx context.Context, branchID domain.BranchID, sourceID uuid.UUID) ([]repository.CitationReadModel, error) {
	citations, err := s.inner.GetCitationsForSource(ctx, branchID, sourceID)
	return s.citationsShown(ctx, branchID, citations, err)
}

// CountCitationsBySource counts only the citations shown, so it reads each
// source's citations.
func (s *PrivacyStore) CountCitationsBySource(ctx context.Context, branchID domain.BranchID, sourceIDs []uuid.UUID) (map[uuid.UUID]int, error) {
	if v := s.privacy(ctx); v.SeesAll() {
		return s.inner.CountCitationsBySource(ctx, branchID, sourceIDs)
	}
	counts := make(map[uuid.UUID]int, len(sourceIDs))
	for _, id := range sourceIDs {
		citations, err := s.GetCitationsForSource(ctx, branchID, id)
		if err != nil {
			return nil, err
		}
		if len(citations) > 0 {
			counts[id] = len(citations)
		}
	}
	return counts, nil
}

func (s *PrivacyStore) GetCitationsForPerson(ctx context.Context, branchID domain.BranchID, personID uuid.UUID) ([]repository.CitationReadModel, error) {
	citations, err := s.inner.GetCitationsForPerson(ctx, branchID, personID)
	return s.citationsShown(ctx, branchID, citations, err)
}

func (s *PrivacyStore) GetCitationsForFact(ctx context.Context, branchID domain.BranchID, factType domain.FactType, factOwnerID uuid.UUID) ([]repository.CitationReadModel, error) {
	citations, err := s.inner.GetCitationsForFact(ctx, branchID, factType, factOwnerID)
	return s.citationsShown(ctx, branchID, citations, err)
}

func (s *PrivacyStore) GetCitationsForFactInstance(ctx context.Context, branchID domain.BranchID, factType domain.FactType, factOwnerID, factID uuid.UUID) ([]repository.CitationReadModel, error) {
	citations, err := s.inner.GetCitationsForFactInstance(ctx, branchID, factType, factOwnerID, factID)
	return s.citationsShown(ctx, branchID, citations, err)
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
	if v := s.privacy(ctx); !v.SeesAll() {
		if c, err := s.GetCitation(ctx, branchID, citationID); err != nil || c == nil {
			return nil, err
		}
	}
	return s.inner.GetCitationExternalIDs(ctx, branchID, citationID)
}

func (s *PrivacyStore) ListCitationExternalIDs(ctx context.Context, branchID domain.BranchID) (map[uuid.UUID][]repository.CitationExternalIDReadModel, error) {
	all, err := s.inner.ListCitationExternalIDs(ctx, branchID)
	if v := s.privacy(ctx); err != nil || v.SeesAll() {
		return all, err
	}
	ids := make([]uuid.UUID, 0, len(all))
	for id := range all {
		ids = append(ids, id)
	}
	shown, err := s.GetCitationsByIDs(ctx, branchID, ids)
	if err != nil {
		return nil, err
	}
	out := make(map[uuid.UUID][]repository.CitationExternalIDReadModel, len(shown))
	for _, c := range shown {
		out[c.ID] = all[c.ID]
	}
	return out, nil
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

// ListAllMedia drops the private items and those whose owner a viewer may not
// see, checking every owner in one batch.
func (s *PrivacyStore) ListAllMedia(ctx context.Context, branchID domain.BranchID) ([]repository.MediaReadModel, error) {
	items, err := s.inner.ListAllMedia(ctx, branchID)
	v := s.privacy(ctx)
	if err != nil || v.SeesAll() {
		return items, err
	}
	items = slices.DeleteFunc(items, func(m repository.MediaReadModel) bool { return v.Hides(m.Restriction) })
	return keepShown(ctx, s, v, branchID, items, func(m *repository.MediaReadModel) []owner {
		return []owner{{m.EntityType, m.EntityID}}
	})
}

// ListMediaInGroup drops the items a viewer may not see.
func (s *PrivacyStore) ListMediaInGroup(ctx context.Context, branchID domain.BranchID, groupID uuid.UUID) ([]repository.MediaReadModel, error) {
	items, err := s.inner.ListMediaInGroup(ctx, branchID, groupID)
	v := s.privacy(ctx)
	if err != nil || v.SeesAll() {
		return items, err
	}
	shown := items[:0]
	for i := range items {
		ok, err := s.mediaShown(ctx, v, branchID, &items[i])
		if err != nil {
			return nil, err
		}
		if ok {
			shown = append(shown, items[i])
		}
	}
	return shown, nil
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

// eventOwners is the person or family an event belongs to.
func eventOwners(e *repository.EventReadModel) []owner { return []owner{{e.OwnerType, e.OwnerID}} }

// attributeOwners is the person an attribute belongs to.
func attributeOwners(a *repository.AttributeReadModel) []owner {
	return []owner{{"person", a.PersonID}}
}

func (s *PrivacyStore) ListEvents(ctx context.Context, opts repository.ListOptions) ([]repository.EventReadModel, int, error) {
	return pageShown(ctx, s, opts, s.inner.ListEvents, eventOwners)
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

func (s *PrivacyStore) ListAttributes(ctx context.Context, opts repository.ListOptions) ([]repository.AttributeReadModel, int, error) {
	return pageShown(ctx, s, opts, s.inner.ListAttributes, attributeOwners)
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
// Notes, submitters and repositories
// ============================================================================

// notesShown drops the private notes, and the shared notes every record
// pointing to which the principal may not see (ADR-008 §3). A note nothing
// points to is shown.
func (s *PrivacyStore) notesShown(ctx context.Context, v repository.Privacy, branchID domain.BranchID, notes []repository.NoteReadModel) ([]repository.NoteReadModel, error) {
	notes = withoutPrivate(v, notes, noteRestriction)
	if len(notes) == 0 {
		return notes, nil
	}
	refs, err := gedcom.NotesReferences(ctx, s.inner, branchID, notes)
	if err != nil {
		return nil, err
	}
	out := make([]repository.NoteReadModel, 0, len(notes))
	for _, n := range notes {
		shown := len(refs[n.ID]) == 0
		for _, ref := range refs[n.ID] {
			if shown, err = s.referenceShown(ctx, v, branchID, ref); err != nil {
				return nil, err
			}
			if shown {
				break
			}
		}
		if shown {
			out = append(out, n)
		}
	}
	return out, nil
}

// referenceShown reports whether v may see the record that ref names. A
// name, event or attribute is shown when its owner is, an association when
// both its people are; a repository holds no person data.
func (s *PrivacyStore) referenceShown(ctx context.Context, v repository.Privacy, branchID domain.BranchID, ref gedcom.NoteReference) (bool, error) {
	switch ref.Type {
	case "person", "family", "source":
		return s.ownerShown(ctx, v, branchID, ref.Type, ref.ID)
	case "repository":
		return true, nil
	case "name":
		n, err := s.inner.GetPersonName(ctx, branchID, ref.ID)
		if err != nil || n == nil {
			return false, err
		}
		return s.ownerShown(ctx, v, branchID, "person", n.PersonID)
	case "event":
		e, err := s.inner.GetEvent(ctx, branchID, ref.ID)
		if err != nil || e == nil {
			return false, err
		}
		return s.ownerShown(ctx, v, branchID, e.OwnerType, e.OwnerID)
	case "attribute":
		a, err := s.inner.GetAttribute(ctx, branchID, ref.ID)
		if err != nil || a == nil {
			return false, err
		}
		return s.ownerShown(ctx, v, branchID, "person", a.PersonID)
	case "association":
		a, err := s.GetAssociation(ctx, branchID, ref.ID)
		return a != nil, err
	default:
		return false, nil
	}
}

func (s *PrivacyStore) GetNote(ctx context.Context, branchID domain.BranchID, id uuid.UUID) (*repository.NoteReadModel, error) {
	n, err := s.inner.GetNote(ctx, branchID, id)
	v := s.privacy(ctx)
	if err != nil || n == nil || v.SeesAll() {
		return n, err
	}
	shown, err := s.notesShown(ctx, v, branchID, []repository.NoteReadModel{*n})
	if err != nil || len(shown) == 0 {
		return nil, err
	}
	return &shown[0], nil
}

func (s *PrivacyStore) ListNotes(ctx context.Context, opts repository.ListOptions) ([]repository.NoteReadModel, int, error) {
	v := s.privacy(ctx)
	if v.SeesAll() {
		return s.inner.ListNotes(ctx, opts)
	}
	return listShown(ctx, opts, s.inner.ListNotes, func(rows []repository.NoteReadModel) ([]repository.NoteReadModel, error) {
		return s.notesShown(ctx, v, opts.BranchID, rows)
	})
}

func (s *PrivacyStore) SaveNote(ctx context.Context, branchID domain.BranchID, note *repository.NoteReadModel) error {
	return s.inner.SaveNote(ctx, branchID, note)
}

func (s *PrivacyStore) DeleteNote(ctx context.Context, branchID domain.BranchID, id uuid.UUID) error {
	return s.inner.DeleteNote(ctx, branchID, id)
}

// GetSubmitter hides submitters from anonymous visitors (ADR-008 §4).
func (s *PrivacyStore) GetSubmitter(ctx context.Context, id uuid.UUID) (*repository.SubmitterReadModel, error) {
	if s.privacy(ctx).HideSubmitters {
		return nil, nil
	}
	return s.inner.GetSubmitter(ctx, id)
}

// ListSubmitters lists none to anonymous visitors (ADR-008 §4).
func (s *PrivacyStore) ListSubmitters(ctx context.Context, opts repository.ListOptions) ([]repository.SubmitterReadModel, int, error) {
	if s.privacy(ctx).HideSubmitters {
		return nil, 0, nil
	}
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

// associationOwners are an association's two people.
func associationOwners(a *repository.AssociationReadModel) []owner {
	return []owner{{"person", a.PersonID}, {"person", a.AssociateID}}
}

// ldsOwners are the person or family an ordinance belongs to.
func ldsOwners(o *repository.LDSOrdinanceReadModel) []owner {
	var owners []owner
	if o.PersonID != nil {
		owners = append(owners, owner{"person", *o.PersonID})
	}
	if o.FamilyID != nil {
		owners = append(owners, owner{"family", *o.FamilyID})
	}
	return owners
}

func (s *PrivacyStore) GetAssociation(ctx context.Context, branchID domain.BranchID, id uuid.UUID) (*repository.AssociationReadModel, error) {
	a, err := s.inner.GetAssociation(ctx, branchID, id)
	return rowShown(ctx, s, branchID, a, err, associationOwners)
}

func (s *PrivacyStore) ListAssociations(ctx context.Context, opts repository.ListOptions) ([]repository.AssociationReadModel, int, error) {
	return pageShown(ctx, s, opts, s.inner.ListAssociations, associationOwners)
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

// GetLDSOrdinance hides an ordinance whose person or family is hidden. LDS
// ordinances are mainline-only, so their owners are resolved on the
// mainline.
func (s *PrivacyStore) GetLDSOrdinance(ctx context.Context, id uuid.UUID) (*repository.LDSOrdinanceReadModel, error) {
	o, err := s.inner.GetLDSOrdinance(ctx, id)
	return rowShown(ctx, s, domain.MainBranchID, o, err, ldsOwners)
}

// ListLDSOrdinances is redacted as GetLDSOrdinance; its callers list the
// mainline.
func (s *PrivacyStore) ListLDSOrdinances(ctx context.Context, opts repository.ListOptions) ([]repository.LDSOrdinanceReadModel, int, error) {
	return pageShown(ctx, s, opts, s.inner.ListLDSOrdinances, ldsOwners)
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
// Evidence and duplicates
// ============================================================================

// Evidence records belong to their subject, a person or family; so does a
// research log. A duplicate dismissal belongs to its two people.

func analysisOwners(a *repository.EvidenceAnalysisReadModel) []owner {
	return []owner{{factOwnerType(a.FactType), a.SubjectID}}
}

func conflictOwners(c *repository.EvidenceConflictReadModel) []owner {
	return []owner{{factOwnerType(c.FactType), c.SubjectID}}
}

func researchLogOwners(l *repository.ResearchLogReadModel) []owner {
	return []owner{{l.SubjectType, l.SubjectID}}
}

func proofOwners(p *repository.ProofSummaryReadModel) []owner {
	return []owner{{factOwnerType(p.FactType), p.SubjectID}}
}

func dismissalOwners(d *repository.DuplicateDismissalReadModel) []owner {
	return []owner{{"person", d.Person1ID}, {"person", d.Person2ID}}
}

func (s *PrivacyStore) GetEvidenceAnalysis(ctx context.Context, branchID domain.BranchID, id uuid.UUID) (*repository.EvidenceAnalysisReadModel, error) {
	a, err := s.inner.GetEvidenceAnalysis(ctx, branchID, id)
	return rowShown(ctx, s, branchID, a, err, analysisOwners)
}

func (s *PrivacyStore) ListEvidenceAnalyses(ctx context.Context, opts repository.ListOptions) ([]repository.EvidenceAnalysisReadModel, int, error) {
	return pageShown(ctx, s, opts, s.inner.ListEvidenceAnalyses, analysisOwners)
}

func (s *PrivacyStore) GetAnalysesForFact(ctx context.Context, branchID domain.BranchID, factType domain.FactType, subjectID uuid.UUID) ([]repository.EvidenceAnalysisReadModel, error) {
	analyses, err := s.inner.GetAnalysesForFact(ctx, branchID, factType, subjectID)
	return rowsShown(ctx, s, branchID, analyses, err, analysisOwners)
}

func (s *PrivacyStore) GetAnalysesBySubject(ctx context.Context, branchID domain.BranchID, subjectID uuid.UUID) ([]repository.EvidenceAnalysisReadModel, error) {
	analyses, err := s.inner.GetAnalysesBySubject(ctx, branchID, subjectID)
	return rowsShown(ctx, s, branchID, analyses, err, analysisOwners)
}

func (s *PrivacyStore) SaveEvidenceAnalysis(ctx context.Context, branchID domain.BranchID, analysis *repository.EvidenceAnalysisReadModel) error {
	return s.inner.SaveEvidenceAnalysis(ctx, branchID, analysis)
}

func (s *PrivacyStore) DeleteEvidenceAnalysis(ctx context.Context, branchID domain.BranchID, id uuid.UUID) error {
	return s.inner.DeleteEvidenceAnalysis(ctx, branchID, id)
}

func (s *PrivacyStore) GetEvidenceConflict(ctx context.Context, branchID domain.BranchID, id uuid.UUID) (*repository.EvidenceConflictReadModel, error) {
	c, err := s.inner.GetEvidenceConflict(ctx, branchID, id)
	return rowShown(ctx, s, branchID, c, err, conflictOwners)
}

func (s *PrivacyStore) ListEvidenceConflicts(ctx context.Context, opts repository.ListOptions) ([]repository.EvidenceConflictReadModel, int, error) {
	return pageShown(ctx, s, opts, s.inner.ListEvidenceConflicts, conflictOwners)
}

func (s *PrivacyStore) GetConflictsForSubject(ctx context.Context, branchID domain.BranchID, subjectID uuid.UUID) ([]repository.EvidenceConflictReadModel, error) {
	conflicts, err := s.inner.GetConflictsForSubject(ctx, branchID, subjectID)
	return rowsShown(ctx, s, branchID, conflicts, err, conflictOwners)
}

func (s *PrivacyStore) ListUnresolvedConflicts(ctx context.Context, branchID domain.BranchID) ([]repository.EvidenceConflictReadModel, error) {
	conflicts, err := s.inner.ListUnresolvedConflicts(ctx, branchID)
	return rowsShown(ctx, s, branchID, conflicts, err, conflictOwners)
}

func (s *PrivacyStore) SaveEvidenceConflict(ctx context.Context, branchID domain.BranchID, conflict *repository.EvidenceConflictReadModel) error {
	return s.inner.SaveEvidenceConflict(ctx, branchID, conflict)
}

func (s *PrivacyStore) DeleteEvidenceConflict(ctx context.Context, branchID domain.BranchID, id uuid.UUID) error {
	return s.inner.DeleteEvidenceConflict(ctx, branchID, id)
}

func (s *PrivacyStore) GetResearchLog(ctx context.Context, branchID domain.BranchID, id uuid.UUID) (*repository.ResearchLogReadModel, error) {
	l, err := s.inner.GetResearchLog(ctx, branchID, id)
	return rowShown(ctx, s, branchID, l, err, researchLogOwners)
}

func (s *PrivacyStore) ListResearchLogs(ctx context.Context, opts repository.ListOptions) ([]repository.ResearchLogReadModel, int, error) {
	return pageShown(ctx, s, opts, s.inner.ListResearchLogs, researchLogOwners)
}

func (s *PrivacyStore) GetResearchLogsForSubject(ctx context.Context, branchID domain.BranchID, subjectID uuid.UUID) ([]repository.ResearchLogReadModel, error) {
	logs, err := s.inner.GetResearchLogsForSubject(ctx, branchID, subjectID)
	return rowsShown(ctx, s, branchID, logs, err, researchLogOwners)
}

func (s *PrivacyStore) SaveResearchLog(ctx context.Context, branchID domain.BranchID, log *repository.ResearchLogReadModel) error {
	return s.inner.SaveResearchLog(ctx, branchID, log)
}

func (s *PrivacyStore) DeleteResearchLog(ctx context.Context, branchID domain.BranchID, id uuid.UUID) error {
	return s.inner.DeleteResearchLog(ctx, branchID, id)
}

func (s *PrivacyStore) GetProofSummary(ctx context.Context, branchID domain.BranchID, id uuid.UUID) (*repository.ProofSummaryReadModel, error) {
	p, err := s.inner.GetProofSummary(ctx, branchID, id)
	return rowShown(ctx, s, branchID, p, err, proofOwners)
}

func (s *PrivacyStore) ListProofSummaries(ctx context.Context, opts repository.ListOptions) ([]repository.ProofSummaryReadModel, int, error) {
	return pageShown(ctx, s, opts, s.inner.ListProofSummaries, proofOwners)
}

func (s *PrivacyStore) GetProofSummariesForFact(ctx context.Context, branchID domain.BranchID, factType domain.FactType, subjectID uuid.UUID) ([]repository.ProofSummaryReadModel, error) {
	summaries, err := s.inner.GetProofSummariesForFact(ctx, branchID, factType, subjectID)
	return rowsShown(ctx, s, branchID, summaries, err, proofOwners)
}

func (s *PrivacyStore) GetProofSummariesBySubject(ctx context.Context, branchID domain.BranchID, subjectID uuid.UUID) ([]repository.ProofSummaryReadModel, error) {
	summaries, err := s.inner.GetProofSummariesBySubject(ctx, branchID, subjectID)
	return rowsShown(ctx, s, branchID, summaries, err, proofOwners)
}

func (s *PrivacyStore) SaveProofSummary(ctx context.Context, branchID domain.BranchID, summary *repository.ProofSummaryReadModel) error {
	return s.inner.SaveProofSummary(ctx, branchID, summary)
}

func (s *PrivacyStore) DeleteProofSummary(ctx context.Context, branchID domain.BranchID, id uuid.UUID) error {
	return s.inner.DeleteProofSummary(ctx, branchID, id)
}

func (s *PrivacyStore) GetDuplicateDismissal(ctx context.Context, branchID domain.BranchID, id uuid.UUID) (*repository.DuplicateDismissalReadModel, error) {
	d, err := s.inner.GetDuplicateDismissal(ctx, branchID, id)
	return rowShown(ctx, s, branchID, d, err, dismissalOwners)
}

func (s *PrivacyStore) ListDuplicateDismissals(ctx context.Context, opts repository.ListOptions) ([]repository.DuplicateDismissalReadModel, int, error) {
	return pageShown(ctx, s, opts, s.inner.ListDuplicateDismissals, dismissalOwners)
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

// The surname, place and cemetery indexes, the map and the brick walls count
// and name people in the store's aggregate queries, so each takes the
// principal's privacy and leaves protected people out in the query (ADR-008
// §5). The privacy a caller passes is replaced by the principal's.

func (s *PrivacyStore) GetSurnameIndex(ctx context.Context, branchID domain.BranchID, _ repository.Privacy) ([]repository.SurnameEntry, []repository.LetterCount, error) {
	return s.inner.GetSurnameIndex(ctx, branchID, s.privacy(ctx))
}

func (s *PrivacyStore) GetSurnamesByLetter(ctx context.Context, branchID domain.BranchID, letter string, _ repository.Privacy) ([]repository.SurnameEntry, error) {
	return s.inner.GetSurnamesByLetter(ctx, branchID, letter, s.privacy(ctx))
}

func (s *PrivacyStore) GetPersonsBySurname(ctx context.Context, surname string, opts repository.ListOptions) ([]repository.PersonReadModel, int, error) {
	opts.Privacy = s.privacy(ctx)
	return s.inner.GetPersonsBySurname(ctx, surname, opts)
}

func (s *PrivacyStore) GetPlaceHierarchy(ctx context.Context, branchID domain.BranchID, parent string, _ repository.Privacy) ([]repository.PlaceEntry, error) {
	return s.inner.GetPlaceHierarchy(ctx, branchID, parent, s.privacy(ctx))
}

func (s *PrivacyStore) GetPersonsByPlace(ctx context.Context, place string, opts repository.ListOptions) ([]repository.PersonReadModel, int, error) {
	opts.Privacy = s.privacy(ctx)
	return s.inner.GetPersonsByPlace(ctx, place, opts)
}

func (s *PrivacyStore) GetCemeteryIndex(ctx context.Context, branchID domain.BranchID, _ repository.Privacy) ([]repository.CemeteryEntry, error) {
	return s.inner.GetCemeteryIndex(ctx, branchID, s.privacy(ctx))
}

func (s *PrivacyStore) GetPersonsByCemetery(ctx context.Context, place string, opts repository.ListOptions) ([]repository.PersonReadModel, int, error) {
	opts.Privacy = s.privacy(ctx)
	return s.inner.GetPersonsByCemetery(ctx, place, opts)
}

func (s *PrivacyStore) ListPlaces(ctx context.Context, branchID domain.BranchID, _ repository.Privacy) ([]repository.PlaceReadModel, error) {
	return s.inner.ListPlaces(ctx, branchID, s.privacy(ctx))
}

func (s *PrivacyStore) GetMapLocations(ctx context.Context, branchID domain.BranchID, _ repository.Privacy) ([]repository.MapLocation, error) {
	return s.inner.GetMapLocations(ctx, branchID, s.privacy(ctx))
}

func (s *PrivacyStore) GetBrickWalls(ctx context.Context, includeResolved bool, _ repository.Privacy) ([]repository.BrickWallEntry, error) {
	return s.inner.GetBrickWalls(ctx, includeResolved, s.privacy(ctx))
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
