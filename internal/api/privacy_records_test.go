package api_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// The read-surface table of ADR-008 for the records other than people and
// families (sources, citations, notes, submitters, associations, LDS
// ordinances, evidence, duplicates and validation), driven through the API as
// every principal of ADR-008 §4. Each record carries a marker in its text
// that names who may see it, so a response shows a record exactly when its
// marker is in the body.

// audience is who may see a record.
type audience int

const (
	everyone       audience = iota
	livingSeers             // what belongs to a living person
	privateSeers            // a private record, or what belongs to one
	signedInPeople          // submitters: anyone but a visitor
)

// sees reports whether p may see a record meant for a.
func (p privacyPrincipal) sees(a audience) bool {
	switch a {
	case livingSeers:
		return p.seesLiving
	case privateSeers:
		return p.seesPrivate
	case signedInPeople:
		return p.cookie != nil
	default:
		return true
	}
}

// markedRecord is one record of the tree: the marker in its text, who may see
// it, and the path that fetches it alone ("" when there is none).
type markedRecord struct {
	marker string
	who    audience
	path   string
}

// recordTree is the records added to the redaction tree, by list path.
type recordTree struct {
	openSource string
	records    map[string][]markedRecord
}

// buildRecordTree writes the records to the read model: the API takes no
// RESN on citations, no note pointers and no submitters.
func buildRecordTree(t *testing.T, f *authFixture, tr redactionTree) recordTree {
	t.Helper()
	ctx := context.Background()
	main := domain.MainBranchID
	now := time.Now().UTC()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	dead, private, living, family := uuid.MustParse(tr.dead), uuid.MustParse(tr.private), uuid.MustParse(tr.living), uuid.MustParse(tr.family)
	out := recordTree{records: map[string][]markedRecord{}}
	add := func(list string, r markedRecord) { out.records[list] = append(out.records[list], r) }

	source := func(title string, restriction domain.Restriction) uuid.UUID {
		s := &repository.SourceReadModel{ID: uuid.New(), SourceType: domain.SourceOther, Title: title, Restriction: restriction, Version: 1, UpdatedAt: now}
		must(f.read.SaveSource(ctx, main, s))
		return s.ID
	}
	openSource := source("OPEN-source", "")
	out.openSource = openSource.String()
	privateSource := source("PRIVATE-source", "confidential")
	add("/api/v1/sources", markedRecord{"OPEN-source", everyone, "/api/v1/sources/" + openSource.String()})
	add("/api/v1/sources", markedRecord{"PRIVATE-source", privateSeers, "/api/v1/sources/" + privateSource.String()})

	citation := func(marker string, who audience, src, owner uuid.UUID, fact domain.FactType, restriction domain.Restriction) {
		c := &repository.CitationReadModel{ID: uuid.New(), SourceID: src, SourceTitle: "OPEN-source", FactType: fact,
			FactOwnerID: owner, Page: marker, Restriction: restriction, Version: 1, CreatedAt: now}
		must(f.read.SaveCitation(ctx, main, c))
		add("/api/v1/sources/"+src.String()+"/citations", markedRecord{marker, who, "/api/v1/citations/" + c.ID.String()})
		// The projection keeps the count; the read model is written here.
		s, err := f.read.GetSource(ctx, main, src)
		must(err)
		s.CitationCount++
		must(f.read.SaveSource(ctx, main, s))
	}
	citation("OPEN-citation", everyone, openSource, dead, domain.FactPersonBirth, "")
	citation("LIVING-citation", livingSeers, openSource, living, domain.FactPersonBirth, "")
	citation("LIVING-family-citation", livingSeers, openSource, family, domain.FactFamilyMarriage, "")
	citation("PRIVATE-citation", privateSeers, openSource, dead, domain.FactPersonBirth, "confidential")
	citation("PRIVATE-person-citation", privateSeers, openSource, private, domain.FactPersonBirth, "")
	citation("PRIVATE-source-citation", privateSeers, privateSource, dead, domain.FactPersonBirth, "")

	// Shared notes: one nothing points to, a private one, one only the living
	// person points to, one the dead person shares with her, and one an
	// association with the living person points to by id.
	note := func(marker string, who audience, xref string, restriction domain.Restriction) uuid.UUID {
		n := &repository.NoteReadModel{ID: uuid.New(), Text: marker, GedcomXref: xref, Restriction: restriction, Version: 1, UpdatedAt: now}
		must(f.read.SaveNote(ctx, main, n))
		add("/api/v1/notes", markedRecord{marker, who, "/api/v1/notes/" + n.ID.String()})
		return n.ID
	}
	note("OPEN-note", everyone, "", "")
	note("PRIVATE-note", privateSeers, "", "confidential")
	note("LIVING-note", livingSeers, "@N1@", "")
	note("OPEN-shared-note", everyone, "@N2@", "")
	associationNote := note("LIVING-association-note", livingSeers, "", "")
	for id, notes := range map[uuid.UUID]string{living: "@N1@\n\n@N2@", dead: "@N2@"} {
		p, err := f.read.GetPerson(ctx, main, id)
		must(err)
		p.Notes = notes
		must(f.read.SavePerson(ctx, main, p))
	}

	s := &repository.SubmitterReadModel{ID: uuid.New(), Name: "SIGNED-IN-submitter", Version: 1, UpdatedAt: now}
	must(f.read.SaveSubmitter(ctx, s))
	add("/api/v1/submitters", markedRecord{"SIGNED-IN-submitter", signedInPeople, "/api/v1/submitters/" + s.ID.String()})

	association := func(marker string, who audience, a, b uuid.UUID, notes []uuid.UUID) {
		r := &repository.AssociationReadModel{ID: uuid.New(), PersonID: a, AssociateID: b, Role: "witness", Phrase: marker, NoteIDs: notes, Version: 1, UpdatedAt: now}
		must(f.read.SaveAssociation(ctx, main, r))
		add("/api/v1/associations", markedRecord{marker, who, "/api/v1/associations/" + r.ID.String()})
	}
	association("OPEN-association", everyone, dead, dead, nil)
	association("LIVING-association", livingSeers, dead, living, []uuid.UUID{associationNote})
	association("PRIVATE-association", privateSeers, private, dead, nil)

	lds := func(marker string, who audience, person, fam *uuid.UUID) {
		o := &repository.LDSOrdinanceReadModel{ID: uuid.New(), Type: domain.LDSBaptism, TypeLabel: "Baptism (LDS)", PersonID: person, FamilyID: fam, Temple: marker, Version: 1, UpdatedAt: now}
		must(f.read.SaveLDSOrdinance(ctx, o))
		add("/api/v1/lds-ordinances", markedRecord{marker, who, "/api/v1/lds-ordinances/" + o.ID.String()})
	}
	lds("OPEN-lds", everyone, &dead, nil)
	lds("LIVING-lds", livingSeers, &living, nil)
	lds("LIVING-family-lds", livingSeers, nil, &family)
	lds("PRIVATE-lds", privateSeers, &private, nil)

	// Evidence about the dead, living and private person, and the family
	// with a living partner.
	subjects := []struct {
		prefix  string
		who     audience
		subject uuid.UUID
		fact    domain.FactType
	}{
		{"OPEN", everyone, dead, domain.FactPersonBirth},
		{"LIVING", livingSeers, living, domain.FactPersonBirth},
		{"LIVING-family", livingSeers, family, domain.FactFamilyMarriage},
		{"PRIVATE", privateSeers, private, domain.FactPersonBirth},
	}
	for _, sub := range subjects {
		subjectType := strings.SplitN(string(sub.fact), "_", 2)[0]
		a := &repository.EvidenceAnalysisReadModel{ID: uuid.New(), FactType: sub.fact, SubjectID: sub.subject, Conclusion: sub.prefix + "-analysis", Version: 1, CreatedAt: now, UpdatedAt: now}
		must(f.read.SaveEvidenceAnalysis(ctx, main, a))
		add("/api/v1/evidence-analyses", markedRecord{a.Conclusion, sub.who, "/api/v1/evidence-analyses/" + a.ID.String()})
		add(fmt.Sprintf("/api/v1/evidence-analyses/by-fact?factType=%s&subjectId=%s", sub.fact, sub.subject), markedRecord{a.Conclusion, sub.who, ""})

		c := &repository.EvidenceConflictReadModel{ID: uuid.New(), FactType: sub.fact, SubjectID: sub.subject, Description: sub.prefix + "-conflict", Status: domain.ConflictStatusOpen, Version: 1, CreatedAt: now, UpdatedAt: now}
		must(f.read.SaveEvidenceConflict(ctx, main, c))
		add("/api/v1/evidence-conflicts", markedRecord{c.Description, sub.who, "/api/v1/evidence-conflicts/" + c.ID.String()})
		add("/api/v1/evidence-conflicts/by-subject/"+sub.subject.String(), markedRecord{c.Description, sub.who, ""})

		l := &repository.ResearchLogReadModel{ID: uuid.New(), SubjectID: sub.subject, SubjectType: subjectType, Repository: "Archive", SearchDescription: sub.prefix + "-research-log",
			Outcome: domain.ResearchOutcomeFound, SearchDate: now, Version: 1, CreatedAt: now, UpdatedAt: now}
		must(f.read.SaveResearchLog(ctx, main, l))
		add("/api/v1/research-logs", markedRecord{l.SearchDescription, sub.who, "/api/v1/research-logs/" + l.ID.String()})
		add("/api/v1/research-logs/by-subject/"+sub.subject.String(), markedRecord{l.SearchDescription, sub.who, ""})

		p := &repository.ProofSummaryReadModel{ID: uuid.New(), FactType: sub.fact, SubjectID: sub.subject, Conclusion: sub.prefix + "-proof", Argument: "because", Version: 1, CreatedAt: now, UpdatedAt: now}
		must(f.read.SaveProofSummary(ctx, main, p))
		add("/api/v1/proof-summaries", markedRecord{p.Conclusion, sub.who, "/api/v1/proof-summaries/" + p.ID.String()})
		add(fmt.Sprintf("/api/v1/proof-summaries/by-fact?factType=%s&subjectId=%s", sub.fact, sub.subject), markedRecord{p.Conclusion, sub.who, ""})
	}

	dismiss := func(marker string, who audience, a, b uuid.UUID) {
		pair, err := domain.NewDuplicatePair(a, b)
		must(err)
		d := &repository.DuplicateDismissalReadModel{ID: pair.ID(), Person1ID: pair.Person1ID, Person2ID: pair.Person2ID, Reason: marker, Version: 1, CreatedAt: now, UpdatedAt: now}
		must(f.read.SaveDuplicateDismissal(ctx, main, d))
		add("/api/v1/persons/duplicates/dismissed", markedRecord{marker, who, ""})
	}
	dismiss("LIVING-dismissal", livingSeers, dead, living)
	dismiss("PRIVATE-dismissal", privateSeers, dead, private)
	return out
}

func TestPrivacyRedaction_RecordSurfaces(t *testing.T) {
	for _, viewersSeeLiving := range []bool{true, false} {
		t.Run(fmt.Sprintf("viewers see living %v", viewersSeeLiving), func(t *testing.T) {
			f, tr, principals := redactionFixture(t, viewersSeeLiving)
			records := buildRecordTree(t, f, tr)
			for _, p := range principals {
				t.Run(p.name, func(t *testing.T) {
					for list, rs := range records.records {
						rec := p.get(t, f, list)
						text := rec.Body.String()
						if rec.Code == http.StatusNotFound {
							// A hidden source's citations: not found, as it is.
							text = ""
						} else if rec.Code != http.StatusOK {
							t.Fatalf("GET %s: %d %s", list, rec.Code, rec.Body)
						}
						shown := 0
						for _, r := range rs {
							if p.sees(r.who) {
								shown++
							}
							if strings.Contains(text, r.marker) != p.sees(r.who) {
								t.Errorf("GET %s: shows %s %v, want %v", list, r.marker, !p.sees(r.who), p.sees(r.who))
							}
							if r.path == "" {
								continue
							}
							if rec := p.get(t, f, r.path); (rec.Code == http.StatusOK) != p.sees(r.who) || (rec.Code != http.StatusOK && rec.Code != http.StatusNotFound) {
								t.Errorf("GET %s (%s): %d, want shown %v", r.path, r.marker, rec.Code, p.sees(r.who))
							}
						}
						// A list's total counts only what it shows.
						var body struct{ Total *int }
						if err := json.Unmarshal(rec.Body.Bytes(), &body); err == nil && body.Total != nil && *body.Total != shown {
							t.Errorf("GET %s: total %d, want %d", list, *body.Total, shown)
						}
					}

					// The open source counts only the citations shown, and
					// a hidden one is not found through the citation paths.
					citations := 0
					for _, r := range records.records["/api/v1/sources/"+records.openSource+"/citations"] {
						if p.sees(r.who) {
							citations++
						}
						if r.path != "" {
							if rec := p.get(t, f, r.path+"/format"); (rec.Code == http.StatusOK) != p.sees(r.who) {
								t.Errorf("GET %s/format (%s): %d, want shown %v", r.path, r.marker, rec.Code, p.sees(r.who))
							}
						}
					}
					if got := p.getJSON(t, f, "/api/v1/sources/"+records.openSource)["citation_count"]; got != float64(citations) {
						t.Errorf("open source citation_count = %v, want %d", got, citations)
					}
					for _, path := range []string{"/api/v1/sources/search?q=source", "/api/v1/sources?q=source"} {
						if text := p.get(t, f, path).Body.String(); strings.Contains(text, "PRIVATE-source") != p.seesPrivate {
							t.Errorf("GET %s shows the private source %v, want %v", path, !p.seesPrivate, p.seesPrivate)
						}
					}

					// Duplicates and validation never name a hidden person,
					// nor point to one as a broken reference.
					hidden := p.hiddenPeople()
					if !p.seesLiving {
						hidden = append(hidden, tr.living, tr.kid)
					}
					if !p.seesPrivate {
						hidden = append(hidden, tr.private)
					}
					for _, path := range []string{"/api/v1/persons/duplicates", "/api/v1/quality/validation"} {
						rec := p.get(t, f, path)
						if rec.Code != http.StatusOK {
							t.Fatalf("GET %s: %d %s", path, rec.Code, rec.Body)
						}
						// The raw body, IDs and all: hidden is given names,
						// each with a letter no hex ID holds, and the hidden
						// people's whole IDs, which no other random ID equals.
						if len(hidden) > 0 && containsAny(rec.Body.String(), hidden...) {
							t.Errorf("GET %s names one of %v: %s", path, hidden, rec.Body)
						}
					}
				})
			}
		})
	}
}
