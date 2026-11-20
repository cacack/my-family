package query_test

import (
	"context"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/query"
	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/repository/memory"
)

// privacyTree is a small tree with a dead person, a private (dead) person and
// two living people, saved straight into a memory store, and the privacy
// decorator over it (ADR-008 §5).
type privacyTree struct {
	raw   *memory.ReadModelStore
	store *query.PrivacyStore

	dead, private, living, kid uuid.UUID
	// open has a dead and a living partner, and the kid; closed is private;
	// withPrivate has the dead and the private partner.
	open, closed, withPrivate uuid.UUID

	livingName, livingBirth, livingOccupation, deadBirth uuid.UUID
	livingCitation, deadCitation, deadPrivateCitation    uuid.UUID
	livingMedia, deadMedia, deadPrivateMedia, openMedia  uuid.UUID
	mediaGroup                                           uuid.UUID // every media item's group
	association, livingLDS, openLDS                      uuid.UUID
}

func newPrivacyTree(t *testing.T) *privacyTree {
	t.Helper()
	ctx := context.Background()
	raw := memory.NewReadModelStore()
	tr := &privacyTree{raw: raw, store: query.NewPrivacyStore(raw)}
	main := domain.MainBranchID
	now := time.Now().UTC()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	person := func(given string, deathEvidence bool, restriction domain.Restriction) uuid.UUID {
		p := &repository.PersonReadModel{ID: uuid.New(), GivenName: given, Surname: "Smith", FullName: given + " Smith",
			Gender: domain.GenderFemale, BirthPlace: "Springfield", DeathEvidence: deathEvidence, Restriction: restriction,
			Notes: "about " + given, Version: 1, UpdatedAt: now}
		must(raw.SavePerson(ctx, main, p))
		return p.ID
	}
	tr.dead = person("Dora", true, "")
	tr.private = person("Petra", true, "confidential")
	tr.living = person("Lena", false, "")
	tr.kid = person("Kit", false, "")

	family := func(p1, p2 uuid.UUID, restriction domain.Restriction) uuid.UUID {
		f := &repository.FamilyReadModel{ID: uuid.New(), Partner1ID: &p1, Partner2ID: &p2, RelationshipType: domain.RelationMarriage,
			MarriageDateRaw: "1 JAN 1950", MarriagePlace: "Chapel", Notes: "wed", Restriction: restriction, Version: 1, UpdatedAt: now}
		must(raw.SaveFamily(ctx, main, f))
		must(raw.ReplaceFamilyExternalIDs(ctx, main, f.ID, []repository.FamilyExternalIDReadModel{{FamilyID: f.ID, Value: "F-" + f.ID.String()}}))
		return f.ID
	}
	tr.open = family(tr.dead, tr.living, "")
	tr.closed = family(tr.dead, tr.dead, "privacy")
	tr.withPrivate = family(tr.dead, tr.private, "")
	must(raw.SaveFamilyChild(ctx, main, &repository.FamilyChildReadModel{FamilyID: tr.open, PersonID: tr.kid, RelationshipType: domain.ChildBiological}))
	must(raw.SaveFamilyChild(ctx, main, &repository.FamilyChildReadModel{FamilyID: tr.closed, PersonID: tr.dead, RelationshipType: domain.ChildBiological}))
	must(raw.SavePedigreeEdge(ctx, main, &repository.PedigreeEdge{PersonID: tr.kid, FatherID: &tr.dead, MotherID: &tr.living}))

	name := &repository.PersonNameReadModel{ID: uuid.New(), PersonID: tr.living, GivenName: "Lena", Surname: "Maiden", NameType: domain.NameTypeBirth, UpdatedAt: now}
	must(raw.SavePersonName(ctx, main, name))
	tr.livingName = name.ID
	for _, id := range []uuid.UUID{tr.dead, tr.living} {
		must(raw.ReplacePersonExternalIDs(ctx, main, id, []repository.PersonExternalIDReadModel{{PersonID: id, Value: "P-" + id.String()}}))
	}

	event := func(ownerType string, owner uuid.UUID, fact domain.FactType) uuid.UUID {
		e := &repository.EventReadModel{ID: uuid.New(), OwnerType: ownerType, OwnerID: owner, FactType: fact,
			DateRaw: "1990", Place: "Springfield", Version: 1, CreatedAt: now}
		must(raw.SaveEvent(ctx, main, e))
		return e.ID
	}
	tr.livingBirth = event("person", tr.living, domain.FactPersonBirth)
	tr.deadBirth = event("person", tr.dead, domain.FactPersonBirth)
	event("family", tr.open, domain.FactFamilyMarriage)
	event("family", tr.withPrivate, domain.FactFamilyMarriage)
	occupation := &repository.AttributeReadModel{ID: uuid.New(), PersonID: tr.living, FactType: domain.FactPersonOccupation, Value: "Pilot", Version: 1, CreatedAt: now}
	must(raw.SaveAttribute(ctx, main, occupation))
	tr.livingOccupation = occupation.ID

	source := &repository.SourceReadModel{ID: uuid.New(), SourceType: domain.SourceOther, Title: "Register", Version: 1, UpdatedAt: now}
	must(raw.SaveSource(ctx, main, source))
	citation := func(owner uuid.UUID, fact domain.FactType, restriction domain.Restriction) uuid.UUID {
		c := &repository.CitationReadModel{ID: uuid.New(), SourceID: source.ID, SourceTitle: source.Title, FactType: fact,
			FactOwnerID: owner, Restriction: restriction, Version: 1, CreatedAt: now}
		must(raw.SaveCitation(ctx, main, c))
		return c.ID
	}
	tr.livingCitation = citation(tr.living, domain.FactPersonBirth, "")
	tr.deadCitation = citation(tr.dead, domain.FactPersonBirth, "")
	tr.deadPrivateCitation = citation(tr.dead, domain.FactPersonBirth, "confidential")
	citation(tr.open, domain.FactFamilyMarriage, "")

	tr.mediaGroup = uuid.New()
	media := func(entityType string, owner uuid.UUID, restriction domain.Restriction) uuid.UUID {
		m := &repository.MediaReadModel{ID: uuid.New(), EntityType: entityType, EntityID: owner, Title: "photo", MimeType: "image/png",
			MediaType: domain.MediaPhoto, Filename: "p.png", FileSize: 1, FileData: []byte{1}, ThumbnailData: []byte{2},
			Restriction: restriction, GroupID: &tr.mediaGroup, Version: 1, CreatedAt: now, UpdatedAt: now}
		must(raw.SaveMedia(ctx, main, m))
		return m.ID
	}
	tr.livingMedia = media("person", tr.living, "")
	tr.deadMedia = media("person", tr.dead, "")
	tr.deadPrivateMedia = media("person", tr.dead, "confidential")
	tr.openMedia = media("family", tr.open, "")

	assoc := &repository.AssociationReadModel{ID: uuid.New(), PersonID: tr.dead, AssociateID: tr.living, Role: "godparent", Version: 1, UpdatedAt: now}
	must(raw.SaveAssociation(ctx, main, assoc))
	tr.association = assoc.ID
	must(raw.SaveAssociation(ctx, main, &repository.AssociationReadModel{ID: uuid.New(), PersonID: tr.dead, AssociateID: tr.dead, Role: "witness", Version: 1, UpdatedAt: now}))
	lds := func(person, family *uuid.UUID) uuid.UUID {
		o := &repository.LDSOrdinanceReadModel{ID: uuid.New(), Type: domain.LDSBaptism, TypeLabel: "Baptism (LDS)", PersonID: person, FamilyID: family, Version: 1, UpdatedAt: now}
		must(raw.SaveLDSOrdinance(ctx, o))
		return o.ID
	}
	tr.livingLDS = lds(&tr.living, nil)
	tr.openLDS = lds(nil, &tr.open)
	return tr
}

// The principals of ADR-008 §4, as contexts.
var (
	ownerCtx   = repository.ContextWithPrincipal(context.Background(), repository.Principal{Role: repository.RoleOwner})
	editorCtx  = repository.ContextWithPrincipal(context.Background(), repository.Principal{Role: repository.RoleEditor})
	viewerCtx  = repository.ContextWithPrincipal(context.Background(), repository.Principal{Role: repository.RoleViewer})
	visitorCtx = repository.ContextWithPrincipal(context.Background(), repository.Principal{Role: repository.RoleViewer, Anonymous: true})
	// A viewer when the owner has turned "viewers see living" off.
	strictViewerCtx = func() context.Context {
		settings := repository.DefaultTreeSettings()
		settings.ViewersSeeLiving = false
		return repository.ContextWithTreeSettings(viewerCtx, settings)
	}()
)

func givenNames(people []repository.PersonReadModel) []string {
	var names []string
	for _, p := range people {
		names = append(names, p.GivenName)
	}
	slices.Sort(names)
	return names
}

func TestPrivacyStore_Persons(t *testing.T) {
	tr := newPrivacyTree(t)
	main := domain.MainBranchID
	tests := []struct {
		name string
		ctx  context.Context
		// listed are the people lists and searches return; shown are the
		// people GetPerson returns in full.
		listed      []string
		placeholder map[uuid.UUID]repository.Visibility
	}{
		{"owner", ownerCtx, []string{"Dora", "Kit", "Lena", "Petra"}, nil},
		{"editor", editorCtx, []string{"Dora", "Kit", "Lena", "Petra"}, nil},
		{"viewer", viewerCtx, []string{"Dora", "Kit", "Lena"}, map[uuid.UUID]repository.Visibility{tr.private: repository.Hidden}},
		{"viewer without living", strictViewerCtx, []string{"Dora"},
			map[uuid.UUID]repository.Visibility{tr.private: repository.Hidden, tr.living: repository.Placeholder, tr.kid: repository.Placeholder}},
		{"anonymous", visitorCtx, []string{"Dora"},
			map[uuid.UUID]repository.Visibility{tr.private: repository.Hidden, tr.living: repository.Placeholder, tr.kid: repository.Placeholder}},
		{"no principal", context.Background(), []string{"Dora"},
			map[uuid.UUID]repository.Visibility{tr.private: repository.Hidden, tr.living: repository.Placeholder, tr.kid: repository.Placeholder}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := tt.ctx
			list := repository.ListOptions{Limit: 10}
			for what, call := range map[string]func() ([]repository.PersonReadModel, int, error){
				"ListPersons": func() ([]repository.PersonReadModel, int, error) { return tr.store.ListPersons(ctx, list) },
				"SearchPersons": func() ([]repository.PersonReadModel, int, error) {
					return tr.store.SearchPersons(ctx, repository.SearchOptions{Query: "Smith", Limit: 10})
				},
				"GetPersonsBySurname": func() ([]repository.PersonReadModel, int, error) {
					return tr.store.GetPersonsBySurname(ctx, "Smith", list)
				},
				"GetPersonsByPlace": func() ([]repository.PersonReadModel, int, error) {
					return tr.store.GetPersonsByPlace(ctx, "Springfield", list)
				},
			} {
				people, total, err := call()
				if err != nil || total != len(tt.listed) || !slices.Equal(givenNames(people), tt.listed) {
					t.Errorf("%s = %v (total %d), %v; want %v", what, givenNames(people), total, err, tt.listed)
				}
			}

			all := []uuid.UUID{tr.dead, tr.private, tr.living, tr.kid}
			batch, err := tr.store.GetPersonsByIDs(ctx, main, all)
			if err != nil || len(batch) != len(all) {
				t.Fatalf("GetPersonsByIDs = %d people, %v; want every slot kept", len(batch), err)
			}
			for _, id := range all {
				p, err := tr.store.GetPerson(ctx, main, id)
				if err != nil || p == nil {
					t.Fatalf("GetPerson(%s) = %v, %v", id, p, err)
				}
				want, protected := tt.placeholder[id]
				if !protected {
					want = repository.Visible
				}
				if p.Visibility != want {
					t.Errorf("GetPerson(%s).Visibility = %v, want %v", p.GivenName, p.Visibility, want)
				}
				if protected && (p.GivenName != "Living" || p.Surname != "" || p.Notes != "" || p.BirthPlace != "" || p.Gender != domain.GenderFemale) {
					t.Errorf("GetPerson placeholder = %+v, want only the ID, sex and \"Living\"", p)
				}
				inBatch := batch[slices.IndexFunc(batch, func(b repository.PersonReadModel) bool { return b.ID == id })]
				if inBatch.Visibility != want {
					t.Errorf("GetPersonsByIDs(%s).Visibility = %v, want %v", inBatch.GivenName, inBatch.Visibility, want)
				}

				names, err := tr.store.GetPersonNames(ctx, main, id)
				ids, err2 := tr.store.GetPersonExternalIDs(ctx, main, id)
				events, err3 := tr.store.ListEventsForPerson(ctx, main, id)
				citations, err4 := tr.store.GetCitationsForPerson(ctx, main, id)
				if err != nil || err2 != nil || err3 != nil || err4 != nil {
					t.Fatalf("person side tables: %v, %v, %v, %v", err, err2, err3, err4)
				}
				if protected && (len(names) > 0 || len(ids) > 0 || len(events) > 0 || len(citations) > 0) {
					t.Errorf("%s is protected but has names %v, external IDs %v, events %v, citations %v", id, names, ids, events, citations)
				}
			}

			name, err := tr.store.GetPersonName(ctx, main, tr.livingName)
			if err != nil || (name == nil) != (tt.placeholder[tr.living] != repository.Visible) {
				t.Errorf("GetPersonName of the living person = %v, %v", name, err)
			}
			byOwner, err := tr.store.ListPersonExternalIDs(ctx, main)
			if _, ok := byOwner[tr.living]; err != nil || ok == (tt.placeholder[tr.living] != repository.Visible) {
				t.Errorf("ListPersonExternalIDs lists the living person: %v, %v", ok, err)
			}
			if _, ok := byOwner[tr.dead]; !ok {
				t.Error("ListPersonExternalIDs dropped the dead person")
			}
		})
	}
}

func TestPrivacyStore_PersonFacts(t *testing.T) {
	tr := newPrivacyTree(t)
	main := domain.MainBranchID
	for _, ctx := range []context.Context{viewerCtx, visitorCtx} {
		livingShown := ctx == viewerCtx
		e, err := tr.store.GetEvent(ctx, main, tr.livingBirth)
		if err != nil || (e != nil) != livingShown {
			t.Errorf("GetEvent of the living person's birth = %v, %v; want shown %v", e, err, livingShown)
		}
		if e, _ := tr.store.GetEvent(ctx, main, tr.deadBirth); e == nil {
			t.Error("GetEvent hid the dead person's birth")
		}
		a, err := tr.store.GetAttribute(ctx, main, tr.livingOccupation)
		attrs, err2 := tr.store.ListAttributesForPerson(ctx, main, tr.living)
		if err != nil || err2 != nil || (a != nil) != livingShown || (len(attrs) > 0) != livingShown {
			t.Errorf("living person's attribute = %v, %v (%v, %v); want shown %v", a, attrs, err, err2, livingShown)
		}

		citations, err := tr.store.GetCitationsForPerson(ctx, main, tr.dead)
		if err != nil || len(citations) != 1 || citations[0].ID != tr.deadCitation {
			t.Errorf("GetCitationsForPerson(dead) = %v, %v; want the public citation only", citations, err)
		}
		citations, err = tr.store.GetCitationsForFact(ctx, main, domain.FactPersonBirth, tr.dead)
		if err != nil || len(citations) != 1 {
			t.Errorf("GetCitationsForFact(dead birth) = %v, %v; want the public citation only", citations, err)
		}
		citations, err = tr.store.GetCitationsForFactInstance(ctx, main, domain.FactPersonBirth, tr.living, tr.living)
		if err != nil || len(citations) > 0 && !livingShown {
			t.Errorf("GetCitationsForFactInstance(living birth) = %v, %v; want shown %v", citations, err, livingShown)
		}

		associations, err := tr.store.ListAssociationsForPerson(ctx, main, tr.dead)
		if err != nil || len(associations) != map[bool]int{true: 2, false: 1}[livingShown] {
			t.Errorf("ListAssociationsForPerson(dead) = %d, %v; want the one with a living associate dropped unless living people are shown", len(associations), err)
		}
		associations, err = tr.store.ListAssociationsForPerson(ctx, main, tr.living)
		if err != nil || (len(associations) > 0) != livingShown {
			t.Errorf("ListAssociationsForPerson(living) = %v, %v; want shown %v", associations, err, livingShown)
		}
		ordinances, err := tr.store.ListLDSOrdinancesForPerson(ctx, tr.living)
		if err != nil || (len(ordinances) > 0) != livingShown {
			t.Errorf("ListLDSOrdinancesForPerson(living) = %v, %v; want shown %v", ordinances, err, livingShown)
		}
	}
}

func TestPrivacyStore_Families(t *testing.T) {
	tr := newPrivacyTree(t)
	main := domain.MainBranchID
	ctx := visitorCtx

	open, err := tr.store.GetFamily(ctx, main, tr.open)
	if err != nil || open == nil {
		t.Fatalf("GetFamily(open) = %v, %v", open, err)
	}
	if open.Partner1GivenName != "Dora" || open.Partner2GivenName != "Living" || open.Partner2Surname != "" {
		t.Errorf("open family partners = %q %q / %q %q, want Dora and the placeholder",
			open.Partner1GivenName, open.Partner1Surname, open.Partner2GivenName, open.Partner2Surname)
	}
	if open.MarriageDateRaw != "" || open.MarriagePlace != "" || open.Notes != "" {
		t.Errorf("a family with a living partner shows its facts: %+v", open)
	}
	if f, err := tr.store.GetFamily(ctx, main, tr.closed); err != nil || f != nil {
		t.Errorf("GetFamily(private) = %v, %v; want not found", f, err)
	}
	if f, _ := tr.store.GetFamily(ctx, main, tr.withPrivate); f == nil || f.Partner2GivenName != "Living" || f.MarriagePlace != "" {
		t.Errorf("GetFamily(private partner) = %+v; want the partner as placeholder and no facts", f)
	}
	if f, _ := tr.store.GetFamily(ownerCtx, main, tr.closed); f == nil || f.MarriagePlace != "Chapel" {
		t.Errorf("the owner's GetFamily(private) = %+v; want it in full", f)
	}

	families, total, err := tr.store.ListFamilies(ctx, repository.ListOptions{Limit: 10})
	if err != nil || total != 2 || len(families) != 2 {
		t.Errorf("ListFamilies = %d (total %d), %v; want the two that are not private", len(families), total, err)
	}
	for name, call := range map[string]func() ([]repository.FamilyReadModel, error){
		"GetFamiliesByIDs": func() ([]repository.FamilyReadModel, error) {
			return tr.store.GetFamiliesByIDs(ctx, main, []uuid.UUID{tr.open, tr.closed, tr.withPrivate})
		},
		"GetFamiliesForPerson": func() ([]repository.FamilyReadModel, error) { return tr.store.GetFamiliesForPerson(ctx, main, tr.dead) },
		"GetFamiliesForPersons": func() ([]repository.FamilyReadModel, error) {
			return tr.store.GetFamiliesForPersons(ctx, main, []uuid.UUID{tr.dead})
		},
	} {
		families, err := call()
		if err != nil || len(families) != 2 || slices.ContainsFunc(families, func(f repository.FamilyReadModel) bool { return f.ID == tr.closed }) {
			t.Errorf("%s = %d families, %v; want the private one dropped", name, len(families), err)
		}
	}
	if f, err := tr.store.GetChildFamily(ctx, main, tr.kid); err != nil || f == nil || f.Partner2GivenName != "Living" {
		t.Errorf("GetChildFamily(kid) = %+v, %v; want the open family, redacted", f, err)
	}
	if f, err := tr.store.GetChildFamily(ctx, main, tr.dead); err != nil || f != nil {
		t.Errorf("GetChildFamily(dead) = %+v, %v; want the private family hidden", f, err)
	}

	children, err := tr.store.GetFamilyChildren(ctx, main, tr.open)
	if err != nil || len(children) != 1 || children[0].PersonGivenName != "Living" {
		t.Errorf("GetFamilyChildren(open) = %+v, %v; want the kid as placeholder", children, err)
	}
	for name, call := range map[string]func() ([]repository.FamilyChildReadModel, error){
		"GetFamilyChildrenByFamilyIDs": func() ([]repository.FamilyChildReadModel, error) {
			return tr.store.GetFamilyChildrenByFamilyIDs(ctx, main, []uuid.UUID{tr.open, tr.closed})
		},
		"ListAllFamilyChildren": func() ([]repository.FamilyChildReadModel, error) { return tr.store.ListAllFamilyChildren(ctx, main) },
	} {
		children, err := call()
		if err != nil || len(children) != 1 || children[0].FamilyID != tr.open || children[0].PersonGivenName != "Living" {
			t.Errorf("%s = %+v, %v; want the open family's kid only, as placeholder", name, children, err)
		}
	}
	kids, err := tr.store.GetChildrenOfFamily(ctx, main, tr.open)
	if err != nil || len(kids) != 1 || kids[0].Visibility != repository.Placeholder {
		t.Errorf("GetChildrenOfFamily(open) = %+v, %v; want the kid as placeholder", kids, err)
	}
	if kids, err := tr.store.GetChildrenOfFamily(ctx, main, tr.closed); err != nil || kids != nil {
		t.Errorf("GetChildrenOfFamily(private) = %+v, %v; want none", kids, err)
	}

	edge, err := tr.store.GetPedigreeEdge(ctx, main, tr.kid)
	if err != nil || edge == nil || edge.MotherName != "Living" || edge.FatherName == "Living" {
		t.Errorf("GetPedigreeEdge(kid) = %+v, %v; want the living mother as placeholder", edge, err)
	}
	edges, err := tr.store.GetPedigreeEdgesByPersonIDs(ctx, main, []uuid.UUID{tr.kid})
	if err != nil || len(edges) != 1 || edges[0].MotherName != "Living" {
		t.Errorf("GetPedigreeEdgesByPersonIDs(kid) = %+v, %v", edges, err)
	}

	ids, err := tr.store.GetFamilyExternalIDs(ctx, main, tr.open)
	if err != nil || len(ids) != 0 {
		t.Errorf("GetFamilyExternalIDs(open) = %v, %v; want none", ids, err)
	}
	byFamily, err := tr.store.ListFamilyExternalIDs(ctx, main)
	if err != nil || len(byFamily) != 0 {
		t.Errorf("ListFamilyExternalIDs = %v, %v; want every family's hidden", byFamily, err)
	}
	if events, err := tr.store.ListEventsForFamily(ctx, main, tr.open); err != nil || len(events) != 0 {
		t.Errorf("ListEventsForFamily(open) = %v, %v; want none", events, err)
	}
	if citations, err := tr.store.GetCitationsForFact(ctx, main, domain.FactFamilyMarriage, tr.open); err != nil || len(citations) != 0 {
		t.Errorf("GetCitationsForFact(open marriage) = %v, %v; want none", citations, err)
	}
	if ordinances, err := tr.store.ListLDSOrdinancesForFamily(ctx, tr.open); err != nil || len(ordinances) != 0 {
		t.Errorf("ListLDSOrdinancesForFamily(open) = %v, %v; want none", ordinances, err)
	}

	// A viewer who sees living people sees the open family in full.
	if f, _ := tr.store.GetFamily(viewerCtx, main, tr.open); f == nil || f.MarriagePlace != "Chapel" || f.Partner2GivenName != "Lena" {
		t.Errorf("the viewer's GetFamily(open) = %+v; want it in full", f)
	}
	if events, _ := tr.store.ListEventsForFamily(viewerCtx, main, tr.open); len(events) != 1 {
		t.Errorf("the viewer's ListEventsForFamily(open) = %v; want the marriage", events)
	}
	if byFamily, _ := tr.store.ListFamilyExternalIDs(viewerCtx, main); len(byFamily) != 1 {
		t.Errorf("the viewer's ListFamilyExternalIDs = %v; want the open family's", byFamily)
	}
}

// The family pages mark each person they name with that person's privacy
// marks as the reader sees them: a visitor sees a protected partner, parent or
// spouse as a placeholder, and the owner sees the living verdict and private
// flag (#1413).
func TestFamilyService_MarksTheRelativesItNames(t *testing.T) {
	tr := newPrivacyTree(t)
	main := domain.MainBranchID
	kidsMarriage := &repository.FamilyReadModel{ID: uuid.New(), Partner1ID: &tr.kid, Partner2ID: &tr.private,
		RelationshipType: domain.RelationMarriage, Version: 1, UpdatedAt: time.Now().UTC()}
	if err := tr.raw.SaveFamily(context.Background(), main, kidsMarriage); err != nil {
		t.Fatal(err)
	}
	service := query.NewFamilyService(tr.store)
	dead, living := query.PersonMarks{}, query.PersonMarks{Living: true}
	hidden := query.PersonMarks{Living: true, Redacted: true} // the placeholder's (empty) facts read as living
	private := query.PersonMarks{Private: true}

	for _, tc := range []struct {
		name                                   string
		ctx                                    context.Context
		livingPartner, privateSpouse, livingMa query.PersonMarks
	}{
		{"owner", ownerCtx, living, private, living},
		{"visitor", visitorCtx, hidden, hidden, hidden},
	} {
		t.Run(tc.name, func(t *testing.T) {
			family, err := service.GetFamily(tc.ctx, main, tr.open)
			if err != nil {
				t.Fatal(err)
			}
			if family.Partner1Marks != dead || family.Partner2Marks != tc.livingPartner {
				t.Errorf("partners = %+v, %+v; want %+v, %+v", family.Partner1Marks, family.Partner2Marks, dead, tc.livingPartner)
			}
			if len(family.Children) != 1 || family.Children[0].PersonMarks != tc.livingPartner {
				t.Errorf("children = %+v; want the kid marked %+v", family.Children, tc.livingPartner)
			}

			list, err := service.ListFamilies(tc.ctx, query.ListFamiliesInput{Limit: 10})
			if err != nil {
				t.Fatal(err)
			}
			listed := false
			for _, f := range list.Items {
				if f.ID != tr.open {
					continue
				}
				listed = true
				if f.Partner2Marks != tc.livingPartner {
					t.Errorf("listed partner2 = %+v, want %+v", f.Partner2Marks, tc.livingPartner)
				}
			}
			if !listed {
				t.Errorf("ListFamilies did not return family %s", tr.open)
			}

			sheet, err := service.GetGroupSheet(tc.ctx, main, tr.open)
			if err != nil {
				t.Fatal(err)
			}
			if len(sheet.Children) != 1 || sheet.Children[0].SpouseMarks == nil || *sheet.Children[0].SpouseMarks != tc.privateSpouse {
				t.Errorf("child's spouse marks = %+v; want %+v", sheet.Children, tc.privateSpouse)
			}
			sheet, err = service.GetGroupSheet(tc.ctx, main, kidsMarriage.ID)
			if err != nil {
				t.Fatal(err)
			}
			kid := sheet.Husband
			if kid == nil || kid.FatherMarks == nil || *kid.FatherMarks != dead || kid.MotherMarks == nil || *kid.MotherMarks != tc.livingMa {
				t.Errorf("kid's parents' marks = %+v; want father %+v, mother %+v", kid, dead, tc.livingMa)
			}
			if sheet.Wife == nil || sheet.Wife.FatherMarks != nil || sheet.Wife.MotherMarks != nil {
				t.Errorf("a partner with no parents has marks: %+v", sheet.Wife)
			}
		})
	}
}

// A private family's child-to-parents link is a gap in the pedigree, even
// when a visible family of the same partners links another child (ADR-008 §3).
func TestPrivacyStore_PrivateFamilyParentage(t *testing.T) {
	tr := newPrivacyTree(t)
	ctx := context.Background()
	main := domain.MainBranchID
	hal := &repository.PersonReadModel{ID: uuid.New(), GivenName: "Hal", Surname: "Smith", DeathEvidence: true, Version: 1}
	secret := &repository.FamilyReadModel{ID: uuid.New(), Partner1ID: &tr.living, Partner2ID: &tr.dead, Restriction: "privacy", Version: 1}
	for _, err := range []error{
		tr.raw.SavePerson(ctx, main, hal),
		tr.raw.SaveFamily(ctx, main, secret),
		tr.raw.SaveFamilyChild(ctx, main, &repository.FamilyChildReadModel{FamilyID: secret.ID, PersonID: hal.ID}),
		tr.raw.SavePedigreeEdge(ctx, main, &repository.PedigreeEdge{PersonID: hal.ID, FatherID: &tr.dead, MotherID: &tr.living, FatherName: "Dora", MotherName: "Lena"}),
	} {
		if err != nil {
			t.Fatal(err)
		}
	}

	for _, tc := range []struct {
		name    string
		ctx     context.Context
		seesHal bool
		seesKid bool
	}{
		{"owner", ownerCtx, true, true},
		{"viewer", viewerCtx, false, true},
		{"anonymous", visitorCtx, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			edge, err := tr.store.GetPedigreeEdge(tc.ctx, main, hal.ID)
			if err != nil || edge == nil || (edge.FatherID != nil) != tc.seesHal || (edge.MotherID != nil) != tc.seesHal {
				t.Errorf("GetPedigreeEdge(hal) = %+v, %v; want parents shown %v", edge, err, tc.seesHal)
			}
			if !tc.seesHal && (edge.FatherName != "" || edge.MotherName != "") {
				t.Errorf("GetPedigreeEdge(hal) = %+v; want no parent names", edge)
			}
			edges, err := tr.store.GetPedigreeEdgesByPersonIDs(tc.ctx, main, []uuid.UUID{hal.ID, tr.kid})
			if err != nil || len(edges) != 2 {
				t.Fatalf("GetPedigreeEdgesByPersonIDs = %+v, %v", edges, err)
			}
			for _, e := range edges {
				want := tc.seesKid
				if e.PersonID == hal.ID {
					want = tc.seesHal
				}
				if (e.FatherID != nil && e.MotherID != nil) != want {
					t.Errorf("edge %+v; want parents shown %v", e, want)
				}
			}
		})
	}
}

func TestPrivacyStore_Media(t *testing.T) {
	tr := newPrivacyTree(t)
	main := domain.MainBranchID
	tests := []struct {
		name  string
		ctx   context.Context
		shown []uuid.UUID
	}{
		{"owner", ownerCtx, []uuid.UUID{tr.livingMedia, tr.deadMedia, tr.deadPrivateMedia, tr.openMedia}},
		{"viewer", viewerCtx, []uuid.UUID{tr.livingMedia, tr.deadMedia, tr.openMedia}},
		{"anonymous", visitorCtx, []uuid.UUID{tr.deadMedia}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, id := range []uuid.UUID{tr.livingMedia, tr.deadMedia, tr.deadPrivateMedia, tr.openMedia} {
				want := slices.Contains(tt.shown, id)
				m, err := tr.store.GetMedia(tt.ctx, main, id)
				data, err2 := tr.store.GetMediaWithData(tt.ctx, main, id)
				thumb, err3 := tr.store.GetMediaThumbnail(tt.ctx, main, id)
				if err != nil || err2 != nil || err3 != nil {
					t.Fatalf("media %s: %v, %v, %v", id, err, err2, err3)
				}
				if (m != nil) != want || (data != nil) != want || (len(thumb) > 0) != want {
					t.Errorf("media %s: metadata %v, bytes %v, thumbnail %v; want shown %v", id, m != nil, data != nil, len(thumb) > 0, want)
				}
			}
			for owner, entityType := range map[uuid.UUID]string{tr.living: "person", tr.dead: "person", tr.open: "family"} {
				items, total, err := tr.store.ListMediaForEntity(tt.ctx, entityType, owner, repository.ListOptions{Limit: 10})
				if err != nil || total != len(items) {
					t.Fatalf("ListMediaForEntity = %d (total %d), %v", len(items), total, err)
				}
				for _, m := range items {
					if !slices.Contains(tt.shown, m.ID) {
						t.Errorf("ListMediaForEntity(%s) lists hidden media %s", entityType, m.ID)
					}
				}
			}
			group, err := tr.store.ListMediaInGroup(tt.ctx, main, tr.mediaGroup)
			if err != nil {
				t.Fatalf("ListMediaInGroup: %v", err)
			}
			var listed []uuid.UUID
			for _, m := range group {
				listed = append(listed, m.ID)
			}
			if len(listed) != len(tt.shown) || slices.ContainsFunc(listed, func(id uuid.UUID) bool { return !slices.Contains(tt.shown, id) }) {
				t.Errorf("ListMediaInGroup = %v, want %v", listed, tt.shown)
			}
		})
	}
}

// For a principal who sees everything the decorator is the store it wraps:
// every method returns what the wrapped store returns, or panics where it
// panics, called with zero arguments on a seeded tree.
func TestPrivacyStore_EditorsSeeTheStoreUnchanged(t *testing.T) {
	tr := newPrivacyTree(t)
	iface := reflect.TypeOf((*repository.ReadModelStore)(nil)).Elem()
	decorated := reflect.ValueOf(tr.store)
	raw := reflect.ValueOf(tr.raw)
	// Reads first, so the writes' zero arguments change nothing they compare.
	var reads, writes []string
	for i := range iface.NumMethod() {
		name := iface.Method(i).Name
		if slices.ContainsFunc([]string{"Save", "Delete", "Replace", "Reset", "Set", "Clear", "Purge", "Record"}, func(prefix string) bool {
			return strings.HasPrefix(name, prefix)
		}) {
			writes = append(writes, name)
		} else {
			reads = append(reads, name)
		}
	}
	for _, name := range append(reads, writes...) {
		m, _ := iface.MethodByName(name)
		args := make([]reflect.Value, m.Type.NumIn())
		for i := range args {
			args[i] = reflect.Zero(m.Type.In(i))
		}
		args[0] = reflect.ValueOf(editorCtx)
		got, gotPanic := call(decorated.MethodByName(name), args)
		want, wantPanic := call(raw.MethodByName(name), args)
		if gotPanic != wantPanic {
			t.Errorf("%s: decorator panicked %v, store panicked %v", name, gotPanic, wantPanic)
			continue
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s: decorator returned %v, store %v", name, got, want)
		}
	}
}

// call calls m and returns its results as values, or reports that it
// panicked.
func call(m reflect.Value, args []reflect.Value) (out []any, panicked bool) {
	defer func() {
		if recover() != nil {
			panicked = true
		}
	}()
	for _, v := range m.Call(args) {
		out = append(out, v.Interface())
	}
	return out, false
}
