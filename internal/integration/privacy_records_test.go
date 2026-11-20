package integration_test

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/query"
	"github.com/cacack/my-family/internal/repository"
)

// The privacy decorator over the records other than people and families
// (ADR-008 §3–4, #1514), on every backend: private
// sources, citations and notes are hidden, so are the records of a living
// person and a shared note only they point to, and submitters are hidden from
// visitors. A research branch shows the mainline's records the same way. The
// rule reads the store's lists whole before paging, so a page
// is full and its total counts only what is shown.
func TestPrivacy_RecordSurfacesOnEveryBackend(t *testing.T) {
	visitor := repository.ContextWithPrincipal(context.Background(), repository.Principal{Role: repository.RoleViewer, Anonymous: true})
	settings := repository.DefaultTreeSettings()
	settings.ViewersSeeLiving = false
	strictViewer := repository.ContextWithTreeSettings(
		repository.ContextWithPrincipal(context.Background(), repository.Principal{Role: repository.RoleViewer}), settings)
	editor := repository.ContextWithPrincipal(context.Background(), repository.Principal{Role: repository.RoleEditor})

	for _, backend := range backends {
		t.Run(backend.name, func(t *testing.T) {
			ctx := context.Background()
			raw := backend.setup(t).read
			store := query.NewPrivacyStore(raw)
			now := time.Now().UTC().Truncate(time.Second)
			must := func(err error) {
				t.Helper()
				if err != nil {
					t.Fatal(err)
				}
			}
			// The records are saved on the mainline; a research branch sees
			// them through its overlay.
			branch := domain.BranchID(uuid.New())
			main := domain.MainBranchID
			save := func(p repository.PersonReadModel) uuid.UUID {
				p.ID, p.Surname, p.Version, p.UpdatedAt = uuid.New(), "Record", 1, now
				p.FullName = p.GivenName + " Record"
				must(raw.SavePerson(ctx, main, &p))
				return p.ID
			}
			dead := save(repository.PersonReadModel{GivenName: "Dead", DeathEvidence: true, DeathNotes: "@SHARED@"})
			living := save(repository.PersonReadModel{GivenName: "Living", Notes: "@LIVING@\n\n@SHARED@"})
			for _, src := range []repository.SourceReadModel{
				{ID: uuid.New(), Title: "Open register"},
				{ID: uuid.New(), Title: "Private register", Restriction: "privacy"},
			} {
				src.SourceType, src.Version, src.UpdatedAt = domain.SourceOther, 1, now
				must(raw.SaveSource(ctx, main, &src))
				for owner, page := range map[uuid.UUID]string{dead: "dead", living: "living"} {
					must(raw.SaveCitation(ctx, main, &repository.CitationReadModel{ID: uuid.New(), SourceID: src.ID, SourceTitle: src.Title,
						FactType: domain.FactPersonBirth, FactOwnerID: owner, Page: src.Title + " " + page, Version: 1, CreatedAt: now}))
				}
			}
			for text, xref := range map[string]string{"living note": "@LIVING@", "shared note": "@SHARED@", "private note": ""} {
				n := &repository.NoteReadModel{ID: uuid.New(), Text: text, GedcomXref: xref, Version: 1, UpdatedAt: now}
				if xref == "" {
					n.Restriction = "confidential"
				}
				must(raw.SaveNote(ctx, main, n))
			}
			for role, other := range map[string]uuid.UUID{"with dead": dead, "with living": living} {
				must(raw.SaveAssociation(ctx, main, &repository.AssociationReadModel{ID: uuid.New(), PersonID: dead, AssociateID: other, Role: role, Version: 1, UpdatedAt: now}))
			}
			for conclusion, subject := range map[string]uuid.UUID{"dead": dead, "living": living} {
				must(raw.SaveEvidenceAnalysis(ctx, main, &repository.EvidenceAnalysisReadModel{ID: uuid.New(), FactType: domain.FactPersonBirth,
					SubjectID: subject, Conclusion: conclusion, Version: 1, CreatedAt: now, UpdatedAt: now}))
			}
			must(raw.SaveSubmitter(ctx, &repository.SubmitterReadModel{ID: uuid.New(), Name: "Researcher", Version: 1, UpdatedAt: now}))

			one := repository.ListOptions{Limit: 1}
			for _, scope := range []domain.BranchID{domain.MainBranchID, branch} {
				one.BranchID = scope
				for name, c := range map[string]struct {
					ctx                context.Context
					sources, citations []string
					notes, roles       []string
					analyses           []string
					submitters         int
				}{
					"editor": {editor, []string{"Open register", "Private register"}, []string{"Open register dead", "Open register living"},
						[]string{"living note", "private note", "shared note"}, []string{"with dead", "with living"}, []string{"dead", "living"}, 1},
					"viewer without living": {strictViewer, []string{"Open register"}, []string{"Open register dead"},
						[]string{"shared note"}, []string{"with dead"}, []string{"dead"}, 1},
					"visitor": {visitor, []string{"Open register"}, []string{"Open register dead"},
						[]string{"shared note"}, []string{"with dead"}, []string{"dead"}, 0},
				} {
					sources, err := repository.ListAllOn(c.ctx, scope, 1, store.ListSources)
					if got := textsOf(sources, func(s repository.SourceReadModel) string { return s.Title }); err != nil || !slices.Equal(got, c.sources) {
						t.Errorf("%s on %s: ListSources = %v, %v; want %v", name, scope, got, err, c.sources)
					}
					// The first page is the open source. The store keeps no
					// count for the citations saved here, so only the
					// redacted count is checked.
					page, n, err := store.ListSources(c.ctx, one)
					if err != nil || n != len(c.sources) || len(page) != 1 {
						t.Fatalf("%s on %s: ListSources page = %+v (total %d), %v", name, scope, page, n, err)
					}
					if name != "editor" && page[0].CitationCount != len(c.citations) {
						t.Errorf("%s on %s: open source counts %d citations, want %d", name, scope, page[0].CitationCount, len(c.citations))
					}
					citations, err := store.GetCitationsForSource(c.ctx, scope, page[0].ID)
					if got := textsOf(citations, func(c repository.CitationReadModel) string { return c.Page }); err != nil || !slices.Equal(got, c.citations) {
						t.Errorf("%s on %s: GetCitationsForSource = %v, %v; want %v", name, scope, got, err, c.citations)
					}
					notes, err := repository.ListAllOn(c.ctx, scope, 1, store.ListNotes)
					if got := textsOf(notes, func(n repository.NoteReadModel) string { return n.Text }); err != nil || !slices.Equal(got, c.notes) {
						t.Errorf("%s on %s: ListNotes = %v, %v; want %v", name, scope, got, err, c.notes)
					}
					associations, err := repository.ListAllOn(c.ctx, scope, 1, store.ListAssociations)
					if got := textsOf(associations, func(a repository.AssociationReadModel) string { return a.Role }); err != nil || !slices.Equal(got, c.roles) {
						t.Errorf("%s on %s: ListAssociations = %v, %v; want %v", name, scope, got, err, c.roles)
					}
					analyses, err := repository.ListAllOn(c.ctx, scope, 1, store.ListEvidenceAnalyses)
					if got := textsOf(analyses, func(a repository.EvidenceAnalysisReadModel) string { return a.Conclusion }); err != nil || !slices.Equal(got, c.analyses) {
						t.Errorf("%s on %s: ListEvidenceAnalyses = %v, %v; want %v", name, scope, got, err, c.analyses)
					}
					if submitters, n, err := store.ListSubmitters(c.ctx, repository.ListOptions{Limit: 10}); err != nil || n != c.submitters || len(submitters) != c.submitters {
						t.Errorf("%s: ListSubmitters = %v (total %d), %v; want %d", name, submitters, n, err, c.submitters)
					}
				}
			}
		})
	}
}

// textsOf returns f of each row, sorted.
func textsOf[T any](rows []T, f func(T) string) []string {
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, f(r))
	}
	slices.Sort(out)
	return out
}
