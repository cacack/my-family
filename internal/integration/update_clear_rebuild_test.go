package integration_test

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/command"
	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// TestUpdateClears_SurviveRebuild (#1359): an update that clears optional
// fields records the clears in its *Updated event, so the fields read back
// cleared both from the live projection and from a replay of the log into a
// fresh read model — the path `myfamily rebuild` takes — on every backend.
func TestUpdateClears_SurviveRebuild(t *testing.T) {
	for _, backend := range backends {
		t.Run(backend.name, func(t *testing.T) {
			ctx := context.Background()
			st := backend.setup(t)
			h := command.NewHandlerWithBranches(st.events, st.read, st.branches, st.snapshots)

			check := seedAndClearOptionalFields(t, ctx, h)
			t.Run("live", func(t *testing.T) { check(t, st.read) })

			fresh := backend.setup(t).read
			replayLog(t, ctx, st.events, fresh)
			t.Run("rebuilt", func(t *testing.T) { check(t, fresh) })
		})
	}
}

// seedAndClearOptionalFields creates one record per update command with its
// optional fields set, clears them with command.Clear, and returns the check
// that a read model holds every one of them cleared.
func seedAndClearOptionalFields(t *testing.T, ctx context.Context, h *command.Handler) func(*testing.T, repository.ReadModelStore) {
	t.Helper()
	must := func(step string, err error) {
		t.Helper()
		if err != nil {
			t.Fatalf("%s: %v", step, err)
		}
	}
	clearStr := command.Clear[string]

	p1, err := h.CreatePerson(ctx, command.CreatePersonInput{GivenName: "Avery", Surname: "Sample", Gender: "female",
		BirthDate: "1 JAN 1850", BirthPlace: "Testville", Notes: "person notes", ResearchStatus: "probable"})
	must("create person", err)
	p2, err := h.CreatePerson(ctx, command.CreatePersonInput{GivenName: "Blake", Surname: "Sample"})
	must("create person 2", err)
	_, err = h.UpdatePerson(ctx, command.UpdatePersonInput{ID: p1.ID, Surname: clearStr(), Gender: clearStr(),
		BirthDate: clearStr(), BirthPlace: clearStr(), Notes: clearStr(), ResearchStatus: clearStr(), Version: p1.Version})
	must("clear person", err)

	name, err := h.AddName(ctx, command.AddNameInput{PersonID: p2.ID, GivenName: "Bee", Surname: "Sample", NameType: "aka", Nickname: "B"})
	must("add name", err)
	_, err = h.UpdateName(ctx, command.UpdateNameInput{PersonID: p2.ID, NameID: name.ID, Nickname: clearStr()})
	must("clear name", err)

	fam, err := h.CreateFamily(ctx, command.CreateFamilyInput{Partner1ID: &p1.ID, Partner2ID: &p2.ID, RelationshipType: "marriage",
		MarriageDate: "1 JUN 1875", MarriagePlace: "Old Chapel"})
	must("create family", err)
	_, err = h.UpdateFamily(ctx, command.UpdateFamilyInput{ID: fam.ID, Partner2ID: command.Clear[uuid.UUID](),
		RelationshipType: clearStr(), MarriageDate: clearStr(), MarriagePlace: clearStr(), Version: fam.Version})
	must("clear family", err)

	src, err := h.CreateSource(ctx, command.CreateSourceInput{SourceType: "book", Title: "Register", Author: "A. Author", PublishDate: "1901"})
	must("create source", err)
	_, err = h.UpdateSource(ctx, command.UpdateSourceInput{ID: src.ID, Author: clearStr(), PublishDate: clearStr(), Version: src.Version})
	must("clear source", err)

	cit, err := h.CreateCitation(ctx, command.CreateCitationInput{SourceID: src.ID, FactType: "person_birth", FactOwnerID: p1.ID,
		Page: "12", SourceQuality: "original", Fields: map[string]string{"line": "7"}})
	must("create citation", err)
	_, err = h.UpdateCitation(ctx, command.UpdateCitationInput{ID: cit.ID, Page: clearStr(), SourceQuality: clearStr(),
		Fields: command.Clear[map[string]string](), Version: cit.Version})
	must("clear citation", err)

	med, err := h.UploadMedia(ctx, command.UploadMediaInput{EntityType: "person", EntityID: p1.ID, Title: "Portrait", MediaType: "photo",
		Description: "desc", Filename: "p.png", FileData: parityPNG(t)})
	must("upload media", err)
	cropped, err := h.UpdateMedia(ctx, command.UpdateMediaInput{ID: med.ID, CropLeft: command.Set(1), CropTop: command.Set(2),
		CropWidth: command.Set(30), CropHeight: command.Set(40), Version: med.Version})
	must("crop media", err)
	clearInt := command.Clear[int]
	_, err = h.UpdateMedia(ctx, command.UpdateMediaInput{ID: med.ID, Description: clearStr(), CropLeft: clearInt(), CropTop: clearInt(),
		CropWidth: clearInt(), CropHeight: clearInt(), Version: cropped.Version})
	must("clear media", err)

	repo, err := h.CreateRepository(ctx, command.CreateRepositoryInput{Name: "Archive", Address: &domain.Address{City: "Oldtown"}, Notes: "repo notes"})
	must("create repository", err)
	_, err = h.UpdateRepository(ctx, command.UpdateRepositoryInput{ID: repo.ID, Address: command.Clear[domain.Address](),
		Notes: clearStr(), Version: repo.Version})
	must("clear repository", err)

	sub, err := h.CreateSubmitter(ctx, command.CreateSubmitterInput{Name: "Submitter", Address: &domain.Address{City: "Subcity"},
		Phone: []string{"555-0100"}, Email: []string{"one@example.test"}, Language: "English", MediaID: &med.ID})
	must("create submitter", err)
	_, err = h.UpdateSubmitter(ctx, command.UpdateSubmitterInput{ID: sub.ID, Address: command.Clear[domain.Address](),
		Phone: command.Clear[[]string](), Email: command.Clear[[]string](), Language: clearStr(), MediaID: command.Clear[uuid.UUID](),
		Version: sub.Version})
	must("clear submitter", err)

	note, err := h.CreateNote(ctx, command.CreateNoteInput{Text: "linked"})
	must("create note", err)
	assoc, err := h.CreateAssociation(ctx, command.CreateAssociationInput{PersonID: p1.ID, AssociateID: p2.ID, Role: "godparent",
		Phrase: "stood", Notes: "assoc notes", NoteIDs: []uuid.UUID{note.ID}})
	must("create association", err)
	_, err = h.UpdateAssociation(ctx, command.UpdateAssociationInput{ID: assoc.ID, Phrase: clearStr(), Notes: clearStr(),
		NoteIDs: command.Clear[[]uuid.UUID](), Version: assoc.Version})
	must("clear association", err)

	lds, err := h.CreateLDSOrdinance(ctx, command.CreateLDSOrdinanceInput{Type: domain.LDSBaptism, PersonID: &p1.ID,
		Date: "1 JAN 1900", Place: "Temple City", Temple: "SLAKE", Status: "COMPLETED"})
	must("create lds ordinance", err)
	_, err = h.UpdateLDSOrdinance(ctx, command.UpdateLDSOrdinanceInput{ID: lds.ID, Date: clearStr(), Place: clearStr(),
		Temple: clearStr(), Status: clearStr(), Version: lds.Version})
	must("clear lds ordinance", err)

	ea, err := h.CreateEvidenceAnalysis(ctx, command.CreateEvidenceAnalysisInput{FactType: "person_birth", SubjectID: p1.ID,
		CitationIDs: []uuid.UUID{cit.ID}, Conclusion: "born 1850", ResearchStatus: "probable", Notes: "ea notes"})
	must("create evidence analysis", err)
	_, err = h.UpdateEvidenceAnalysis(ctx, command.UpdateEvidenceAnalysisInput{ID: ea.ID, CitationIDs: command.Clear[[]uuid.UUID](),
		ResearchStatus: clearStr(), Notes: clearStr(), Version: ea.Version})
	must("clear evidence analysis", err)

	rl, err := h.CreateResearchLog(ctx, command.CreateResearchLogInput{SubjectID: p1.ID, SubjectType: "person", Repository: "Archive",
		SearchDescription: "census", Outcome: "found", Notes: "rl notes", SearchDate: time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)})
	must("create research log", err)
	_, err = h.UpdateResearchLog(ctx, command.UpdateResearchLogInput{ID: rl.ID, Notes: clearStr(), Version: rl.Version})
	must("clear research log", err)

	ps, err := h.CreateProofSummary(ctx, command.CreateProofSummaryInput{FactType: "person_birth", SubjectID: p1.ID, Conclusion: "c",
		Argument: "a", AnalysisIDs: []uuid.UUID{ea.ID}, ResearchStatus: "possible"})
	must("create proof summary", err)
	_, err = h.UpdateProofSummary(ctx, command.UpdateProofSummaryInput{ID: ps.ID, AnalysisIDs: command.Clear[[]uuid.UUID](),
		ResearchStatus: clearStr(), Version: ps.Version})
	must("clear proof summary", err)

	main := domain.MainBranchID
	return func(t *testing.T, rs repository.ReadModelStore) {
		t.Helper()
		got := func(what string, v any, err error) {
			t.Helper()
			if err != nil {
				t.Fatalf("read %s: %v", what, err)
			}
			if rv := reflect.ValueOf(v); !rv.IsValid() || rv.IsNil() {
				t.Fatalf("read %s: not found", what)
			}
		}
		emptyList := func(s string) bool { return s == "" || s == "[]" || s == "null" }

		person, err := rs.GetPerson(ctx, main, p1.ID)
		got("person", person, err)
		if person.Surname != "" || person.Gender != "" || person.BirthDateRaw != "" || person.BirthDateSort != nil ||
			person.BirthPlace != "" || person.Notes != "" || person.ResearchStatus != "" {
			t.Errorf("person not cleared: %+v", person)
		}

		names, err := rs.GetPersonNames(ctx, main, p2.ID)
		got("names", names, err)
		for _, n := range names {
			if n.ID == name.ID && n.Nickname != "" {
				t.Errorf("name nickname = %q, want cleared", n.Nickname)
			}
		}

		family, err := rs.GetFamily(ctx, main, fam.ID)
		got("family", family, err)
		if family.Partner2ID != nil || family.RelationshipType != "" || family.MarriageDateRaw != "" ||
			family.MarriageDateSort != nil || family.MarriagePlace != "" {
			t.Errorf("family not cleared: %+v", family)
		}

		source, err := rs.GetSource(ctx, main, src.ID)
		got("source", source, err)
		if source.Author != "" || source.PublishDateRaw != "" {
			t.Errorf("source not cleared: %+v", source)
		}

		citation, err := rs.GetCitation(ctx, main, cit.ID)
		got("citation", citation, err)
		emptyFields := citation.FieldsJSON == "" || citation.FieldsJSON == "{}" || citation.FieldsJSON == "null"
		if citation.Page != "" || citation.SourceQuality != "" || !emptyFields {
			t.Errorf("citation not cleared: page=%q quality=%q fields=%q", citation.Page, citation.SourceQuality, citation.FieldsJSON)
		}

		media, err := rs.GetMedia(ctx, main, med.ID)
		got("media", media, err)
		if media.Description != "" || media.CropLeft != nil || media.CropTop != nil || media.CropWidth != nil || media.CropHeight != nil {
			t.Errorf("media not cleared: description=%q crop=%v,%v,%v,%v", media.Description,
				media.CropLeft, media.CropTop, media.CropWidth, media.CropHeight)
		}

		repoRow, err := rs.GetRepository(ctx, repo.ID)
		got("repository", repoRow, err)
		if repoRow.Address != nil || repoRow.Notes != "" {
			t.Errorf("repository not cleared: address=%+v notes=%q", repoRow.Address, repoRow.Notes)
		}

		submitter, err := rs.GetSubmitter(ctx, sub.ID)
		got("submitter", submitter, err)
		if submitter.Address != nil || len(submitter.Phone) != 0 || len(submitter.Email) != 0 ||
			submitter.Language != "" || submitter.MediaID != nil {
			t.Errorf("submitter not cleared: %+v", submitter)
		}

		association, err := rs.GetAssociation(ctx, main, assoc.ID)
		got("association", association, err)
		if association.Phrase != "" || association.Notes != "" || len(association.NoteIDs) != 0 {
			t.Errorf("association not cleared: %+v", association)
		}

		ordinance, err := rs.GetLDSOrdinance(ctx, lds.ID)
		got("lds ordinance", ordinance, err)
		if ordinance.DateRaw != "" || ordinance.Place != "" || ordinance.Temple != "" || ordinance.Status != "" {
			t.Errorf("lds ordinance not cleared: %+v", ordinance)
		}

		analysis, err := rs.GetEvidenceAnalysis(ctx, main, ea.ID)
		got("evidence analysis", analysis, err)
		if !emptyList(analysis.CitationIDsJSON) || analysis.ResearchStatus != "" || analysis.Notes != "" {
			t.Errorf("evidence analysis not cleared: citations=%q status=%q notes=%q",
				analysis.CitationIDsJSON, analysis.ResearchStatus, analysis.Notes)
		}

		log, err := rs.GetResearchLog(ctx, main, rl.ID)
		got("research log", log, err)
		if log.Notes != "" {
			t.Errorf("research log notes = %q, want cleared", log.Notes)
		}

		proof, err := rs.GetProofSummary(ctx, main, ps.ID)
		got("proof summary", proof, err)
		if !emptyList(proof.AnalysisIDsJSON) || proof.ResearchStatus != "" {
			t.Errorf("proof summary not cleared: analyses=%q status=%q", proof.AnalysisIDsJSON, proof.ResearchStatus)
		}
	}
}
