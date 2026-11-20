package query_test

import (
	"context"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// The decorator over records other than people and families: sources,
// citations, notes, submitters and the paged lists whose rule depends on
// other records (ADR-008 §3–5).

// textsOf returns f of each row, sorted.
func textsOf[T any](rows []T, f func(T) string) []string {
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, f(r))
	}
	slices.Sort(out)
	return out
}

func TestPrivacyStore_SourcesAndCitations(t *testing.T) {
	tr := newPrivacyTree(t)
	ctx, main, now := context.Background(), domain.MainBranchID, time.Now().UTC()
	// A private source with a citation of the dead person, and a run of
	// private sources that sort before the open one, so a search must ask
	// the store for more than its limit.
	private := &repository.SourceReadModel{ID: uuid.New(), SourceType: domain.SourceOther, Title: "A Register", Restriction: "privacy", CitationCount: 1, Version: 1, UpdatedAt: now}
	if err := tr.raw.SaveSource(ctx, main, private); err != nil {
		t.Fatal(err)
	}
	for i := range 3 {
		if err := tr.raw.SaveSource(ctx, main, &repository.SourceReadModel{ID: uuid.New(), SourceType: domain.SourceOther,
			Title: fmt.Sprintf("A%d Register", i), Restriction: "confidential", Version: 1, UpdatedAt: now}); err != nil {
			t.Fatal(err)
		}
	}
	onPrivate := &repository.CitationReadModel{ID: uuid.New(), SourceID: private.ID, FactType: domain.FactPersonBirth, FactOwnerID: tr.dead, Version: 1, CreatedAt: now}
	if err := tr.raw.SaveCitation(ctx, main, onPrivate); err != nil {
		t.Fatal(err)
	}
	if err := tr.raw.ReplaceSourceExternalIDs(ctx, main, private.ID, []repository.SourceExternalIDReadModel{{SourceID: private.ID, Value: "S-1"}}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []uuid.UUID{tr.deadCitation, tr.livingCitation} {
		if err := tr.raw.ReplaceCitationExternalIDs(ctx, main, id, []repository.CitationExternalIDReadModel{{CitationID: id, Value: "C-" + id.String()}}); err != nil {
			t.Fatal(err)
		}
	}
	all, _, _ := tr.raw.ListSources(ctx, repository.ListOptions{Limit: 10})
	open := all[len(all)-1].ID // "Register" sorts last

	tests := []struct {
		name                string
		ctx                 context.Context
		seesLiving bool
	}{
		{"viewer", viewerCtx, true},
		{"viewer without living", strictViewerCtx, false},
		{"visitor", visitorCtx, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Dora's public birth, and Lena's birth and the marriage of the
			// family she is a partner in when living people are shown.
			want := 1
			if tt.seesLiving {
				want = 3
			}
			if src, err := tr.store.GetSource(tt.ctx, main, private.ID); err != nil || src != nil {
				t.Errorf("GetSource(private) = %v, %v; want nil", src, err)
			}
			src, err := tr.store.GetSource(tt.ctx, main, open)
			if err != nil || src == nil || src.CitationCount != want {
				t.Fatalf("GetSource(open) = %+v, %v; want %d citations counted", src, err, want)
			}
			sources, total, err := tr.store.ListSources(tt.ctx, repository.ListOptions{Limit: 10})
			if err != nil || total != 1 || len(sources) != 1 || sources[0].CitationCount != want {
				t.Errorf("ListSources = %v (total %d), %v; want the open source alone", sources, total, err)
			}
			found, err := tr.store.SearchSources(tt.ctx, main, "Register", 1)
			if err != nil || len(found) != 1 || found[0].ID != open {
				t.Errorf("SearchSources(limit 1) = %v, %v; want the open source", found, err)
			}
			if byIDs, err := tr.store.GetSourcesByIDs(tt.ctx, main, []uuid.UUID{open, private.ID}); err != nil || len(byIDs) != 1 {
				t.Errorf("GetSourcesByIDs = %v, %v; want the open source alone", byIDs, err)
			}
			if ids, err := tr.store.GetSourceExternalIDs(tt.ctx, main, private.ID); err != nil || ids != nil {
				t.Errorf("GetSourceExternalIDs(private) = %v, %v; want none", ids, err)
			}
			if ids, err := tr.store.ListSourceExternalIDs(tt.ctx, main); err != nil || len(ids) != 0 {
				t.Errorf("ListSourceExternalIDs = %v, %v; want none", ids, err)
			}

			citations, err := tr.store.GetCitationsForSource(tt.ctx, main, open)
			if err != nil || len(citations) != want {
				t.Errorf("GetCitationsForSource = %d, %v; want %d", len(citations), err, want)
			}
			if citations, err := tr.store.GetCitationsForSource(tt.ctx, main, private.ID); err != nil || len(citations) != 0 {
				t.Errorf("GetCitationsForSource(private source) = %v, %v; want none", citations, err)
			}
			listed, total, err := tr.store.ListCitations(tt.ctx, repository.ListOptions{Limit: 1, Offset: 1})
			if err != nil || total != want || len(listed) != min(1, want-1) {
				t.Errorf("ListCitations(second page of 1) = %v (total %d), %v; want the second of %d", listed, total, err, want)
			}
			for id, shown := range map[uuid.UUID]bool{tr.deadCitation: true, tr.livingCitation: tt.seesLiving, tr.deadPrivateCitation: false, onPrivate.ID: false} {
				if c, err := tr.store.GetCitation(tt.ctx, main, id); err != nil || (c != nil) != shown {
					t.Errorf("GetCitation(%s) = %v, %v; want shown %v", id, c, err, shown)
				}
				if ids, err := tr.store.GetCitationExternalIDs(tt.ctx, main, id); err != nil || (len(ids) > 0) && !shown {
					t.Errorf("GetCitationExternalIDs(%s) = %v, %v; want shown %v", id, ids, err, shown)
				}
			}
			byIDs, err := tr.store.GetCitationsByIDs(tt.ctx, main, []uuid.UUID{tr.deadCitation, tr.livingCitation, onPrivate.ID})
			if wantIDs := map[bool]int{true: 2, false: 1}[tt.seesLiving]; err != nil || len(byIDs) != wantIDs {
				t.Errorf("GetCitationsByIDs = %v, %v; want %d", byIDs, err, wantIDs)
			}
			ext, err := tr.store.ListCitationExternalIDs(tt.ctx, main)
			if _, living := ext[tr.livingCitation]; err != nil || living != tt.seesLiving || len(ext[tr.deadCitation]) != 1 {
				t.Errorf("ListCitationExternalIDs = %v, %v; want the living citation's shown %v", ext, err, tt.seesLiving)
			}
			counts, err := tr.store.CountCitationsBySource(tt.ctx, main, []uuid.UUID{open, private.ID})
			if err != nil || counts[open] != want || counts[private.ID] != 0 {
				t.Errorf("CountCitationsBySource = %v, %v; want %d and 0", counts, err, want)
			}
		})
	}
}

func TestPrivacyStore_SharedNotes(t *testing.T) {
	tr := newPrivacyTree(t)
	ctx, main, now := context.Background(), domain.MainBranchID, time.Now().UTC()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	notes := map[string]string{} // text by xref
	note := func(text, xref string, restriction domain.Restriction) uuid.UUID {
		n := &repository.NoteReadModel{ID: uuid.New(), Text: text, GedcomXref: xref, Restriction: restriction, Version: 1, UpdatedAt: now}
		must(tr.raw.SaveNote(ctx, main, n))
		notes[xref] = text
		return n.ID
	}
	note("unreferenced", "", "")
	note("private", "@P@", "confidential")
	byAssociation := note("by the living association", "", "")
	// Each record type points to a note of its own; the dead person also
	// shares one with the living person.
	for xref, text := range map[string]string{
		"@PERSON@": "by the living person", "@NAME@": "by the living name", "@EVENT@": "by the living birth",
		"@ATTR@": "by the living occupation", "@FAMILY@": "by the family with a living partner",
		"@SHARED@": "shared with the dead", "@SOURCE@": "by the private source", "@REPO@": "by a repository",
		"@DEADEVENT@": "by the dead birth",
	} {
		note(text, xref, "")
	}
	setNotes := func(get func() (any, error), set func(any) error) {
		t.Helper()
		row, err := get()
		must(err)
		must(set(row))
	}
	setNotes(func() (any, error) { return tr.raw.GetPerson(ctx, main, tr.living) }, func(r any) error {
		p := r.(*repository.PersonReadModel)
		p.Notes = "@PERSON@\n\n@SHARED@"
		return tr.raw.SavePerson(ctx, main, p)
	})
	setNotes(func() (any, error) { return tr.raw.GetPerson(ctx, main, tr.dead) }, func(r any) error {
		p := r.(*repository.PersonReadModel)
		p.DeathNotes = "@SHARED@"
		return tr.raw.SavePerson(ctx, main, p)
	})
	setNotes(func() (any, error) { return tr.raw.GetPersonName(ctx, main, tr.livingName) }, func(r any) error {
		n := r.(*repository.PersonNameReadModel)
		n.Notes = "@NAME@"
		return tr.raw.SavePersonName(ctx, main, n)
	})
	setNotes(func() (any, error) { return tr.raw.GetEvent(ctx, main, tr.livingBirth) }, func(r any) error {
		e := r.(*repository.EventReadModel)
		e.Notes = "@EVENT@"
		return tr.raw.SaveEvent(ctx, main, e)
	})
	setNotes(func() (any, error) { return tr.raw.GetEvent(ctx, main, tr.deadBirth) }, func(r any) error {
		e := r.(*repository.EventReadModel)
		e.Notes = "@DEADEVENT@"
		return tr.raw.SaveEvent(ctx, main, e)
	})
	setNotes(func() (any, error) { return tr.raw.GetAttribute(ctx, main, tr.livingOccupation) }, func(r any) error {
		a := r.(*repository.AttributeReadModel)
		a.Notes = "@ATTR@"
		return tr.raw.SaveAttribute(ctx, main, a)
	})
	setNotes(func() (any, error) { return tr.raw.GetFamily(ctx, main, tr.open) }, func(r any) error {
		f := r.(*repository.FamilyReadModel)
		f.MarriageNotes = "@FAMILY@"
		return tr.raw.SaveFamily(ctx, main, f)
	})
	setNotes(func() (any, error) { return tr.raw.GetAssociation(ctx, main, tr.association) }, func(r any) error {
		a := r.(*repository.AssociationReadModel)
		a.NoteIDs = []uuid.UUID{byAssociation}
		return tr.raw.SaveAssociation(ctx, main, a)
	})
	must(tr.raw.SaveSource(ctx, main, &repository.SourceReadModel{ID: uuid.New(), SourceType: domain.SourceOther, Title: "Secret", Notes: "@SOURCE@", Restriction: "privacy", Version: 1, UpdatedAt: now}))
	must(tr.raw.SaveRepository(ctx, &repository.RepositoryReadModel{ID: uuid.New(), Name: "Archive", Notes: "@REPO@", Version: 1, UpdatedAt: now}))

	everyone := []string{"unreferenced", "shared with the dead", "by a repository", "by the dead birth"}
	living := []string{"by the living person", "by the living name", "by the living birth", "by the living occupation",
		"by the family with a living partner", "by the living association"}
	tests := []struct {
		name string
		ctx  context.Context
		want []string
	}{
		{"editor", editorCtx, append(append(slices.Clone(everyone), living...), "private", "by the private source")},
		{"viewer", viewerCtx, append(slices.Clone(everyone), living...)},
		{"viewer without living", strictViewerCtx, everyone},
		{"visitor", visitorCtx, everyone},
	}
	text := func(n repository.NoteReadModel) string { return n.Text }
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			slices.Sort(tt.want)
			listed, total, err := tr.store.ListNotes(tt.ctx, repository.ListOptions{Limit: 100})
			if got := textsOf(listed, text); err != nil || !slices.Equal(got, tt.want) || total != len(tt.want) {
				t.Errorf("ListNotes = %v (total %d), %v; want %v", got, total, err, tt.want)
			}
			all, _, _ := tr.raw.ListNotes(ctx, repository.ListOptions{Limit: 100})
			for _, n := range all {
				got, err := tr.store.GetNote(tt.ctx, main, n.ID)
				if err != nil || (got != nil) != slices.Contains(tt.want, n.Text) {
					t.Errorf("GetNote(%q) = %v, %v; want shown %v", n.Text, got, err, slices.Contains(tt.want, n.Text))
				}
			}
		})
	}
}

func TestPrivacyStore_Submitters(t *testing.T) {
	tr := newPrivacyTree(t)
	s := &repository.SubmitterReadModel{ID: uuid.New(), Name: "Researcher", Version: 1, UpdatedAt: time.Now()}
	if err := tr.raw.SaveSubmitter(context.Background(), s); err != nil {
		t.Fatal(err)
	}
	for name, ctx := range map[string]context.Context{"viewer": viewerCtx, "viewer without living": strictViewerCtx, "visitor": visitorCtx, "no principal": context.Background()} {
		shown := ctx == viewerCtx || ctx == strictViewerCtx
		got, err := tr.store.GetSubmitter(ctx, s.ID)
		listed, total, err2 := tr.store.ListSubmitters(ctx, repository.ListOptions{Limit: 10})
		if err != nil || err2 != nil || (got != nil) != shown || (len(listed) == 1 && total == 1) != shown {
			t.Errorf("%s: submitter %v, list %v (total %d), %v, %v; want shown %v", name, got, listed, total, err, err2, shown)
		}
	}
}

// The paged lists whose rule depends on other records: each row is dropped
// when a person or family it belongs to is hidden, the page is full and the
// total counts only the rows shown.
func TestPrivacyStore_ListsOfDependentRecords(t *testing.T) {
	tr := newPrivacyTree(t)
	ctx, main, now := context.Background(), domain.MainBranchID, time.Now().UTC()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	subjects := map[string]uuid.UUID{"dead": tr.dead, "private": tr.private, "living": tr.living}
	for name, id := range subjects {
		must(tr.raw.SaveEvidenceAnalysis(ctx, main, &repository.EvidenceAnalysisReadModel{ID: uuid.New(), FactType: domain.FactPersonBirth, SubjectID: id, Conclusion: name, Version: 1, CreatedAt: now, UpdatedAt: now}))
		must(tr.raw.SaveEvidenceConflict(ctx, main, &repository.EvidenceConflictReadModel{ID: uuid.New(), FactType: domain.FactPersonBirth, SubjectID: id, Description: name, Status: domain.ConflictStatusOpen, Version: 1, CreatedAt: now, UpdatedAt: now}))
		must(tr.raw.SaveResearchLog(ctx, main, &repository.ResearchLogReadModel{ID: uuid.New(), SubjectID: id, SubjectType: "person", SearchDescription: name, Outcome: domain.ResearchOutcomeFound, SearchDate: now, Version: 1, CreatedAt: now, UpdatedAt: now}))
		must(tr.raw.SaveProofSummary(ctx, main, &repository.ProofSummaryReadModel{ID: uuid.New(), FactType: domain.FactPersonBirth, SubjectID: id, Conclusion: name, Version: 1, CreatedAt: now, UpdatedAt: now}))
		if id != tr.dead {
			pair, err := domain.NewDuplicatePair(tr.dead, id)
			must(err)
			must(tr.raw.SaveDuplicateDismissal(ctx, main, &repository.DuplicateDismissalReadModel{ID: pair.ID(), Person1ID: pair.Person1ID, Person2ID: pair.Person2ID, Reason: name, Version: 1, CreatedAt: now, UpdatedAt: now}))
		}
	}
	// A research log about a record kind the rule does not know is hidden.
	must(tr.raw.SaveResearchLog(ctx, main, &repository.ResearchLogReadModel{ID: uuid.New(), SubjectID: uuid.New(), SubjectType: "place", SearchDescription: "unknown", Outcome: domain.ResearchOutcomeFound, SearchDate: now, Version: 1, CreatedAt: now, UpdatedAt: now}))

	page := repository.ListOptions{Limit: 1, Offset: 0}
	type list struct {
		name string
		call func(context.Context) ([]string, int, error)
		// shown by a viewer, and by a viewer without living people
		viewer, strict int
	}
	lists := []list{
		{"ListEvidenceAnalyses", func(c context.Context) ([]string, int, error) {
			rows, n, err := tr.store.ListEvidenceAnalyses(c, page)
			return textsOf(rows, func(r repository.EvidenceAnalysisReadModel) string { return r.Conclusion }), n, err
		}, 2, 1},
		{"ListEvidenceConflicts", func(c context.Context) ([]string, int, error) {
			rows, n, err := tr.store.ListEvidenceConflicts(c, page)
			return textsOf(rows, func(r repository.EvidenceConflictReadModel) string { return r.Description }), n, err
		}, 2, 1},
		{"ListResearchLogs", func(c context.Context) ([]string, int, error) {
			rows, n, err := tr.store.ListResearchLogs(c, page)
			return textsOf(rows, func(r repository.ResearchLogReadModel) string { return r.SearchDescription }), n, err
		}, 2, 1},
		{"ListProofSummaries", func(c context.Context) ([]string, int, error) {
			rows, n, err := tr.store.ListProofSummaries(c, page)
			return textsOf(rows, func(r repository.ProofSummaryReadModel) string { return r.Conclusion }), n, err
		}, 2, 1},
		{"ListDuplicateDismissals", func(c context.Context) ([]string, int, error) {
			rows, n, err := tr.store.ListDuplicateDismissals(c, page)
			return textsOf(rows, func(r repository.DuplicateDismissalReadModel) string { return r.Reason }), n, err
		}, 1, 0},
		// Dead–living and dead–dead.
		{"ListAssociations", func(c context.Context) ([]string, int, error) {
			rows, n, err := tr.store.ListAssociations(c, page)
			return textsOf(rows, func(r repository.AssociationReadModel) string { return r.Role }), n, err
		}, 2, 1},
		// The living person's and the open family's.
		{"ListLDSOrdinances", func(c context.Context) ([]string, int, error) {
			rows, n, err := tr.store.ListLDSOrdinances(c, page)
			return textsOf(rows, func(r repository.LDSOrdinanceReadModel) string { return r.ID.String() }), n, err
		}, 2, 0},
		// Births of the dead and living person, and the marriages of the open
		// family and the family with the private partner.
		{"ListEvents", func(c context.Context) ([]string, int, error) {
			rows, n, err := tr.store.ListEvents(c, page)
			return textsOf(rows, func(r repository.EventReadModel) string { return r.ID.String() }), n, err
		}, 3, 1},
		{"ListAttributes", func(c context.Context) ([]string, int, error) {
			rows, n, err := tr.store.ListAttributes(c, page)
			return textsOf(rows, func(r repository.AttributeReadModel) string { return r.Value }), n, err
		}, 1, 0},
	}
	for _, l := range lists {
		for _, c := range []struct {
			ctx  context.Context
			want int
		}{{viewerCtx, l.viewer}, {strictViewerCtx, l.strict}, {visitorCtx, l.strict}} {
			rows, total, err := l.call(c.ctx)
			if err != nil || total != c.want || len(rows) != min(c.want, 1) {
				t.Errorf("%s: %v (total %d), %v; want a full page of %d", l.name, rows, total, err, c.want)
			}
			if c.want > 0 && slices.ContainsFunc(rows, func(s string) bool { return s == "private" || s == "unknown" }) {
				t.Errorf("%s shows a hidden row: %v", l.name, rows)
			}
		}
	}

	// The single reads and reads by subject follow the same rule.
	for name, id := range subjects {
		shown := map[string]bool{"dead": true, "living": false, "private": false}[name]
		for read, got := range map[string]int{
			"GetAnalysesForFact":         lenOf(tr.store.GetAnalysesForFact(visitorCtx, main, domain.FactPersonBirth, id)),
			"GetAnalysesBySubject":       lenOf(tr.store.GetAnalysesBySubject(visitorCtx, main, id)),
			"GetConflictsForSubject":     lenOf(tr.store.GetConflictsForSubject(visitorCtx, main, id)),
			"GetResearchLogsForSubject":  lenOf(tr.store.GetResearchLogsForSubject(visitorCtx, main, id)),
			"GetProofSummariesForFact":   lenOf(tr.store.GetProofSummariesForFact(visitorCtx, main, domain.FactPersonBirth, id)),
			"GetProofSummariesBySubject": lenOf(tr.store.GetProofSummariesBySubject(visitorCtx, main, id)),
		} {
			if (got > 0) != shown {
				t.Errorf("%s(%s) as a visitor: %d rows; want shown %v", read, name, got, shown)
			}
		}
	}
	analyses, _, _ := tr.raw.ListEvidenceAnalyses(ctx, repository.ListOptions{Limit: 10})
	conflicts, _ := tr.raw.ListUnresolvedConflicts(ctx, main)
	logs, _, _ := tr.raw.ListResearchLogs(ctx, repository.ListOptions{Limit: 10})
	proofs, _, _ := tr.raw.ListProofSummaries(ctx, repository.ListOptions{Limit: 10})
	dismissals, _, _ := tr.raw.ListDuplicateDismissals(ctx, repository.ListOptions{Limit: 10})
	for i := range analyses {
		shown := analyses[i].Conclusion == "dead"
		if got, err := tr.store.GetEvidenceAnalysis(visitorCtx, main, analyses[i].ID); err != nil || (got != nil) != shown {
			t.Errorf("GetEvidenceAnalysis(%s) = %v, %v; want shown %v", analyses[i].Conclusion, got, err, shown)
		}
	}
	for i := range conflicts {
		shown := conflicts[i].Description == "dead"
		if got, err := tr.store.GetEvidenceConflict(visitorCtx, main, conflicts[i].ID); err != nil || (got != nil) != shown {
			t.Errorf("GetEvidenceConflict(%s) = %v, %v; want shown %v", conflicts[i].Description, got, err, shown)
		}
	}
	if got := lenOf(tr.store.ListUnresolvedConflicts(visitorCtx, main)); got != 1 {
		t.Errorf("ListUnresolvedConflicts as a visitor: %d; want the dead person's alone", got)
	}
	for i := range logs {
		shown := logs[i].SearchDescription == "dead"
		if got, err := tr.store.GetResearchLog(visitorCtx, main, logs[i].ID); err != nil || (got != nil) != shown {
			t.Errorf("GetResearchLog(%s) = %v, %v; want shown %v", logs[i].SearchDescription, got, err, shown)
		}
	}
	for i := range proofs {
		shown := proofs[i].Conclusion == "dead"
		if got, err := tr.store.GetProofSummary(visitorCtx, main, proofs[i].ID); err != nil || (got != nil) != shown {
			t.Errorf("GetProofSummary(%s) = %v, %v; want shown %v", proofs[i].Conclusion, got, err, shown)
		}
	}
	for i := range dismissals {
		if got, err := tr.store.GetDuplicateDismissal(viewerCtx, main, dismissals[i].ID); err != nil || (got != nil) != (dismissals[i].Reason == "living") {
			t.Errorf("GetDuplicateDismissal(%s) as a viewer = %v, %v", dismissals[i].Reason, got, err)
		}
	}
	if a, err := tr.store.GetAssociation(visitorCtx, main, tr.association); err != nil || a != nil {
		t.Errorf("GetAssociation(dead–living) as a visitor = %v, %v; want nil", a, err)
	}
	if o, err := tr.store.GetLDSOrdinance(visitorCtx, tr.openLDS); err != nil || o != nil {
		t.Errorf("GetLDSOrdinance(open family) as a visitor = %v, %v; want nil", o, err)
	}
	if o, err := tr.store.GetLDSOrdinance(viewerCtx, tr.openLDS); err != nil || o == nil {
		t.Errorf("GetLDSOrdinance(open family) as a viewer = %v, %v; want it", o, err)
	}
}

// lenOf returns the length of rows, or -1 on an error.
func lenOf[T any](rows []T, err error) int {
	if err != nil {
		return -1
	}
	return len(rows)
}
