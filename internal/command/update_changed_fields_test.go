package command_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/command"
	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository/memory"
)

// The remaining update commands record only the fields whose value differs
// from the current one too (#943, following #900). Each case submits the full
// form with the given fields edited: an unchanged form appends no event and
// returns the current version, and an edited one records only the edited keys.

// updateCase is one full-form submit with edit applied to it.
type updateCase[In any] struct {
	name string
	edit func(*In)
	want []string // recorded change keys; nil means no event
}

// updateFixture is a freshly created entity: its stream, version, the full
// form that matches it, and the update command to submit that form to.
type updateFixture[In any] struct {
	events  *memory.EventStore
	stream  uuid.UUID
	version int64
	form    In
	update  func(In) (int64, error)
}

func runUpdateCases[In any](t *testing.T, setup func(t *testing.T) updateFixture[In], cases []updateCase[In]) {
	t.Helper()
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			f := setup(t)
			input := f.form
			tt.edit(&input)
			got, err := f.update(input)
			if err != nil {
				t.Fatalf("update: %v", err)
			}
			if len(tt.want) == 0 && got != f.version {
				t.Errorf("version = %d, want current version %d for an unchanged form", got, f.version)
			}
			checkRecorded(t, f.events, f.stream, int(f.version), tt.want)
		})
	}
}

func newTestHandler() (context.Context, *memory.EventStore, *memory.ReadModelStore, *command.Handler) {
	events, read := memory.NewEventStore(), memory.NewReadModelStore()
	return context.Background(), events, read, command.NewHandler(events, read)
}

func TestUpdateCitation_RecordsOnlyChangedFields(t *testing.T) {
	setup := func(t *testing.T) updateFixture[command.UpdateCitationInput] {
		ctx, events, _, handler := newTestHandler()
		source, err := handler.CreateSource(ctx, command.CreateSourceInput{SourceType: "census", Title: "1880 Census"})
		if err != nil {
			t.Fatalf("CreateSource: %v", err)
		}
		owner := uuid.New()
		created, err := handler.CreateCitation(ctx, command.CreateCitationInput{
			SourceID: source.ID, FactType: "person_birth", FactOwnerID: owner, Page: "12", Volume: "3",
			SourceQuality: "original", InformantType: "primary", EvidenceType: "direct",
			QuotedText: "born 1850", Analysis: "reliable", TemplateID: "census-us",
			Fields: map[string]string{"line": "4"},
		})
		if err != nil {
			t.Fatalf("CreateCitation: %v", err)
		}
		return updateFixture[command.UpdateCitationInput]{
			events: events, stream: created.ID, version: created.Version,
			form: command.UpdateCitationInput{
				ID: created.ID, SourceID: &source.ID, FactType: ptr("person_birth"), FactOwnerID: &owner,
				Page: command.Set("12"), Volume: command.Set("3"), SourceQuality: command.Set("original"), InformantType: command.Set("primary"),
				EvidenceType: command.Set("direct"), QuotedText: command.Set("born 1850"), Analysis: command.Set("reliable"),
				TemplateID: command.Set("census-us"), Fields: command.Set(map[string]string{"line": "4"}), Version: created.Version,
			},
			update: func(in command.UpdateCitationInput) (int64, error) {
				r, err := handler.UpdateCitation(ctx, in)
				if err != nil {
					return 0, err
				}
				return r.Version, nil
			},
		}
	}
	runUpdateCases(t, setup, []updateCase[command.UpdateCitationInput]{
		{"unchanged form", func(*command.UpdateCitationInput) {}, nil},
		{"page only", func(in *command.UpdateCitationInput) { in.Page = command.Set("13") }, []string{"page"}},
		{"fields only", func(in *command.UpdateCitationInput) { in.Fields = command.Set(map[string]string{"line": "5"}) }, []string{"fields"}},
	})
}

func TestUpdateMedia_RecordsOnlyChangedFields(t *testing.T) {
	setup := func(t *testing.T) updateFixture[command.UpdateMediaInput] {
		ctx, events, _, handler := newTestHandler()
		person, err := handler.CreatePerson(ctx, command.CreatePersonInput{GivenName: "Ada", Surname: "Lovelace"})
		if err != nil {
			t.Fatalf("CreatePerson: %v", err)
		}
		uploaded, err := handler.UploadMedia(ctx, command.UploadMediaInput{
			EntityType: "person", EntityID: person.ID, Title: "Portrait", Description: "Studio",
			MediaType: "photo", Filename: "a.jpg", FileData: createTestJPEG(),
		})
		if err != nil {
			t.Fatalf("UploadMedia: %v", err)
		}
		form := command.UpdateMediaInput{
			ID: uploaded.ID, Title: ptr("Portrait"), Description: command.Set("Studio"), MediaType: ptr("photo"),
			CropLeft: command.Set(1), CropTop: command.Set(2), CropWidth: command.Set(30), CropHeight: command.Set(40), Version: uploaded.Version,
		}
		// Store a crop first so the unchanged form compares crop values too.
		cropped, err := handler.UpdateMedia(ctx, form)
		if err != nil {
			t.Fatalf("UpdateMedia (crop): %v", err)
		}
		form.Version = cropped.Version
		return updateFixture[command.UpdateMediaInput]{
			events: events, stream: uploaded.ID, version: cropped.Version, form: form,
			update: func(in command.UpdateMediaInput) (int64, error) {
				r, err := handler.UpdateMedia(ctx, in)
				if err != nil {
					return 0, err
				}
				return r.Version, nil
			},
		}
	}
	runUpdateCases(t, setup, []updateCase[command.UpdateMediaInput]{
		{"unchanged form", func(*command.UpdateMediaInput) {}, nil},
		{"crop width only", func(in *command.UpdateMediaInput) { in.CropWidth = command.Set(31) }, []string{"crop_width"}},
		{"title only", func(in *command.UpdateMediaInput) { in.Title = ptr("Portrait, 1840") }, []string{"title"}},
	})
}

func TestUpdateNote_RecordsOnlyChangedFields(t *testing.T) {
	setup := func(t *testing.T) updateFixture[command.UpdateNoteInput] {
		ctx, events, _, handler := newTestHandler()
		created, err := handler.CreateNote(ctx, command.CreateNoteInput{Text: "first"})
		if err != nil {
			t.Fatalf("CreateNote: %v", err)
		}
		return updateFixture[command.UpdateNoteInput]{
			events: events, stream: created.ID, version: created.Version,
			form: command.UpdateNoteInput{ID: created.ID, Text: ptr("first"), Version: created.Version},
			update: func(in command.UpdateNoteInput) (int64, error) {
				r, err := handler.UpdateNote(ctx, in)
				if err != nil {
					return 0, err
				}
				return r.Version, nil
			},
		}
	}
	runUpdateCases(t, setup, []updateCase[command.UpdateNoteInput]{
		{"unchanged form", func(*command.UpdateNoteInput) {}, nil},
		{"text only", func(in *command.UpdateNoteInput) { in.Text = ptr("second") }, []string{"text"}},
	})
}

// UpdateName records a NameUpdated event that carries the whole name rather
// than a changes map, so an edit is checked by the event appended and the
// value it stores.
func TestUpdateName_SkipsUnchangedForm(t *testing.T) {
	setup := func(t *testing.T) (updateFixture[command.UpdateNameInput], *memory.ReadModelStore) {
		ctx, events, read, handler := newTestHandler()
		person, err := handler.CreatePerson(ctx, command.CreatePersonInput{GivenName: "Ada", Surname: "Lovelace"})
		if err != nil {
			t.Fatalf("CreatePerson: %v", err)
		}
		added, err := handler.AddName(ctx, command.AddNameInput{
			PersonID: person.ID, GivenName: "Augusta", Surname: "Byron", NamePrefix: "Hon.",
			NameSuffix: "", SurnamePrefix: "", Nickname: "Ada", NameType: "birth",
		})
		if err != nil {
			t.Fatalf("AddName: %v", err)
		}
		current, err := read.GetPerson(ctx, domain.MainBranchID, person.ID)
		if err != nil {
			t.Fatalf("GetPerson: %v", err)
		}
		return updateFixture[command.UpdateNameInput]{
			events: events, stream: person.ID, version: current.Version,
			form: command.UpdateNameInput{
				PersonID: person.ID, NameID: added.ID, GivenName: ptr("Augusta"), Surname: ptr("Byron"),
				NamePrefix: command.Set("Hon."), NameSuffix: command.Set(""), SurnamePrefix: command.Set(""), Nickname: command.Set("Ada"),
				NameType: ptr("birth"), IsPrimary: ptr(false),
			},
			update: func(in command.UpdateNameInput) (int64, error) {
				if _, err := handler.UpdateName(ctx, in); err != nil {
					return 0, err
				}
				p, err := read.GetPerson(ctx, domain.MainBranchID, person.ID)
				if err != nil {
					return 0, err
				}
				return p.Version, nil
			},
		}, read
	}

	t.Run("unchanged form", func(t *testing.T) {
		f, _ := setup(t)
		got, err := f.update(f.form)
		if err != nil {
			t.Fatalf("UpdateName: %v", err)
		}
		if got != f.version {
			t.Errorf("person version = %d, want %d for an unchanged form", got, f.version)
		}
		checkRecorded(t, f.events, f.stream, int(f.version), nil)
	})
	t.Run("nickname only", func(t *testing.T) {
		f, read := setup(t)
		input := f.form
		input.Nickname = command.Set("Countess")
		if _, err := f.update(input); err != nil {
			t.Fatalf("UpdateName: %v", err)
		}
		stream, err := f.events.ReadStream(context.Background(), f.stream)
		if err != nil {
			t.Fatalf("ReadStream: %v", err)
		}
		if len(stream) != int(f.version)+1 || stream[len(stream)-1].EventType != "NameUpdated" {
			t.Fatalf("stream = %d events ending %q, want one NameUpdated appended", len(stream), stream[len(stream)-1].EventType)
		}
		name, err := read.GetPersonName(context.Background(), domain.MainBranchID, input.NameID)
		if err != nil || name == nil {
			t.Fatalf("GetPersonName: %v", err)
		}
		if name.Nickname != "Countess" || name.GivenName != "Augusta" {
			t.Errorf("name = %q/%q, want nickname edited only", name.GivenName, name.Nickname)
		}
	})
}

func TestUpdateSubmitter_RecordsOnlyChangedFields(t *testing.T) {
	setup := func(t *testing.T) updateFixture[command.UpdateSubmitterInput] {
		ctx, events, _, handler := newTestHandler()
		mediaID := uuid.New()
		created, err := handler.CreateSubmitter(ctx, command.CreateSubmitterInput{
			Name: "Jane", Address: &domain.Address{City: "Leeds"}, Phone: []string{"555"},
			Email: []string{"jane@example.org"}, Language: "en", MediaID: &mediaID,
		})
		if err != nil {
			t.Fatalf("CreateSubmitter: %v", err)
		}
		return updateFixture[command.UpdateSubmitterInput]{
			events: events, stream: created.ID, version: created.Version,
			form: command.UpdateSubmitterInput{
				ID: created.ID, Name: ptr("Jane"), Address: command.Set(domain.Address{City: "Leeds"}), Phone: command.Set([]string{"555"}),
				Email: command.Set([]string{"jane@example.org"}), Language: command.Set("en"), MediaID: command.Set(mediaID), Version: created.Version,
			},
			update: func(in command.UpdateSubmitterInput) (int64, error) {
				r, err := handler.UpdateSubmitter(ctx, in)
				if err != nil {
					return 0, err
				}
				return r.Version, nil
			},
		}
	}
	runUpdateCases(t, setup, []updateCase[command.UpdateSubmitterInput]{
		{"unchanged form", func(*command.UpdateSubmitterInput) {}, nil},
		{"email only", func(in *command.UpdateSubmitterInput) { in.Email = command.Set([]string{"jane@example.com"}) }, []string{"email"}},
		{"address only", func(in *command.UpdateSubmitterInput) { in.Address = command.Set(domain.Address{City: "York"}) }, []string{"address"}},
	})
}

func TestUpdateAssociation_RecordsOnlyChangedFields(t *testing.T) {
	setup := func(t *testing.T) updateFixture[command.UpdateAssociationInput] {
		ctx, events, _, handler := newTestHandler()
		p1, err := handler.CreatePerson(ctx, command.CreatePersonInput{GivenName: "Ada", Surname: "Lovelace"})
		if err != nil {
			t.Fatalf("CreatePerson: %v", err)
		}
		p2, err := handler.CreatePerson(ctx, command.CreatePersonInput{GivenName: "Charles", Surname: "Babbage"})
		if err != nil {
			t.Fatalf("CreatePerson: %v", err)
		}
		noteIDs := []uuid.UUID{uuid.New()}
		created, err := handler.CreateAssociation(ctx, command.CreateAssociationInput{
			PersonID: p1.ID, AssociateID: p2.ID, Role: "godparent", Phrase: "friend", Notes: "notes", NoteIDs: noteIDs,
		})
		if err != nil {
			t.Fatalf("CreateAssociation: %v", err)
		}
		return updateFixture[command.UpdateAssociationInput]{
			events: events, stream: created.ID, version: created.Version,
			form: command.UpdateAssociationInput{
				ID: created.ID, Role: ptr("godparent"), Phrase: command.Set("friend"), Notes: command.Set("notes"),
				NoteIDs: command.Set(noteIDs), Version: created.Version,
			},
			update: func(in command.UpdateAssociationInput) (int64, error) {
				r, err := handler.UpdateAssociation(ctx, in)
				if err != nil {
					return 0, err
				}
				return r.Version, nil
			},
		}
	}
	runUpdateCases(t, setup, []updateCase[command.UpdateAssociationInput]{
		{"unchanged form", func(*command.UpdateAssociationInput) {}, nil},
		{"phrase only", func(in *command.UpdateAssociationInput) { in.Phrase = command.Set("colleague") }, []string{"phrase"}},
	})
}

func TestUpdateLDSOrdinance_RecordsOnlyChangedFields(t *testing.T) {
	setup := func(t *testing.T) updateFixture[command.UpdateLDSOrdinanceInput] {
		ctx, events, _, handler := newTestHandler()
		person, err := handler.CreatePerson(ctx, command.CreatePersonInput{GivenName: "Ada", Surname: "Lovelace"})
		if err != nil {
			t.Fatalf("CreatePerson: %v", err)
		}
		created, err := handler.CreateLDSOrdinance(ctx, command.CreateLDSOrdinanceInput{
			Type: domain.LDSBaptism, PersonID: &person.ID, Date: "1 JAN 1900", Place: "Utah",
			Temple: "SLAKE", Status: "COMPLETED",
		})
		if err != nil {
			t.Fatalf("CreateLDSOrdinance: %v", err)
		}
		return updateFixture[command.UpdateLDSOrdinanceInput]{
			events: events, stream: created.ID, version: created.Version,
			form: command.UpdateLDSOrdinanceInput{
				ID: created.ID, Date: command.Set("1 JAN 1900"), Place: command.Set("Utah"), Temple: command.Set("SLAKE"),
				Status: command.Set("COMPLETED"), Version: created.Version,
			},
			update: func(in command.UpdateLDSOrdinanceInput) (int64, error) {
				r, err := handler.UpdateLDSOrdinance(ctx, in)
				if err != nil {
					return 0, err
				}
				return r.Version, nil
			},
		}
	}
	runUpdateCases(t, setup, []updateCase[command.UpdateLDSOrdinanceInput]{
		{"unchanged form", func(*command.UpdateLDSOrdinanceInput) {}, nil},
		{"temple only", func(in *command.UpdateLDSOrdinanceInput) { in.Temple = command.Set("LOGAN") }, []string{"temple"}},
	})
}

func TestUpdateEvidenceAnalysis_RecordsOnlyChangedFields(t *testing.T) {
	setup := func(t *testing.T) updateFixture[command.UpdateEvidenceAnalysisInput] {
		ctx, events, _, handler := newTestHandler()
		subject := uuid.New()
		citations := []uuid.UUID{uuid.New(), uuid.New()}
		created, err := handler.CreateEvidenceAnalysis(ctx, command.CreateEvidenceAnalysisInput{
			FactType: "person_birth", SubjectID: subject, CitationIDs: citations,
			Conclusion: "Born 1850", ResearchStatus: "probable", Notes: "notes",
		})
		if err != nil {
			t.Fatalf("CreateEvidenceAnalysis: %v", err)
		}
		return updateFixture[command.UpdateEvidenceAnalysisInput]{
			events: events, stream: created.ID, version: created.Version,
			form: command.UpdateEvidenceAnalysisInput{
				ID: created.ID, FactType: ptr("person_birth"), SubjectID: &subject,
				CitationIDs: command.Set(append([]uuid.UUID(nil), citations...)), Conclusion: ptr("Born 1850"),
				ResearchStatus: command.Set("probable"), Notes: command.Set("notes"), Version: created.Version,
			},
			update: func(in command.UpdateEvidenceAnalysisInput) (int64, error) {
				r, err := handler.UpdateEvidenceAnalysis(ctx, in)
				if err != nil {
					return 0, err
				}
				return r.Version, nil
			},
		}
	}
	runUpdateCases(t, setup, []updateCase[command.UpdateEvidenceAnalysisInput]{
		{"unchanged form", func(*command.UpdateEvidenceAnalysisInput) {}, nil},
		{"conclusion only", func(in *command.UpdateEvidenceAnalysisInput) { in.Conclusion = ptr("Born 1851") }, []string{"conclusion"}},
		{"citations only", func(in *command.UpdateEvidenceAnalysisInput) {
			ids, _ := in.CitationIDs.Value()
			in.CitationIDs = command.Set(ids[:1])
		}, []string{"citation_ids"}},
	})
}

func TestUpdateResearchLog_RecordsOnlyChangedFields(t *testing.T) {
	setup := func(t *testing.T) updateFixture[command.UpdateResearchLogInput] {
		ctx, events, _, handler := newTestHandler()
		subject := uuid.New()
		searched := time.Date(2025, 6, 15, 0, 0, 0, 0, time.UTC)
		created, err := handler.CreateResearchLog(ctx, command.CreateResearchLogInput{
			SubjectID: subject, SubjectType: "person", Repository: "Archives",
			SearchDescription: "Census", Outcome: "found", Notes: "notes", SearchDate: searched,
		})
		if err != nil {
			t.Fatalf("CreateResearchLog: %v", err)
		}
		return updateFixture[command.UpdateResearchLogInput]{
			events: events, stream: created.ID, version: created.Version,
			form: command.UpdateResearchLogInput{
				ID: created.ID, SubjectID: &subject, SubjectType: ptr("person"), Repository: ptr("Archives"),
				SearchDescription: ptr("Census"), Outcome: ptr("found"), Notes: command.Set("notes"),
				SearchDate: &searched, Version: created.Version,
			},
			update: func(in command.UpdateResearchLogInput) (int64, error) {
				r, err := handler.UpdateResearchLog(ctx, in)
				if err != nil {
					return 0, err
				}
				return r.Version, nil
			},
		}
	}
	runUpdateCases(t, setup, []updateCase[command.UpdateResearchLogInput]{
		{"unchanged form", func(*command.UpdateResearchLogInput) {}, nil},
		{"outcome only", func(in *command.UpdateResearchLogInput) { in.Outcome = ptr("not_found") }, []string{"outcome"}},
		{"search date only", func(in *command.UpdateResearchLogInput) {
			in.SearchDate = ptr(time.Date(2025, 6, 16, 0, 0, 0, 0, time.UTC))
		}, []string{"search_date"}},
	})
}

func TestUpdateProofSummary_RecordsOnlyChangedFields(t *testing.T) {
	setup := func(t *testing.T) updateFixture[command.UpdateProofSummaryInput] {
		ctx, events, _, handler := newTestHandler()
		subject := uuid.New()
		analyses := []uuid.UUID{uuid.New()}
		created, err := handler.CreateProofSummary(ctx, command.CreateProofSummaryInput{
			FactType: "person_birth", SubjectID: subject, Conclusion: "Born 1850",
			Argument: "Two records agree", AnalysisIDs: analyses, ResearchStatus: "probable",
		})
		if err != nil {
			t.Fatalf("CreateProofSummary: %v", err)
		}
		return updateFixture[command.UpdateProofSummaryInput]{
			events: events, stream: created.ID, version: created.Version,
			form: command.UpdateProofSummaryInput{
				ID: created.ID, FactType: ptr("person_birth"), SubjectID: &subject, Conclusion: ptr("Born 1850"),
				Argument: ptr("Two records agree"), AnalysisIDs: command.Set(append([]uuid.UUID(nil), analyses...)),
				ResearchStatus: command.Set("probable"), Version: created.Version,
			},
			update: func(in command.UpdateProofSummaryInput) (int64, error) {
				r, err := handler.UpdateProofSummary(ctx, in)
				if err != nil {
					return 0, err
				}
				return r.Version, nil
			},
		}
	}
	runUpdateCases(t, setup, []updateCase[command.UpdateProofSummaryInput]{
		{"unchanged form", func(*command.UpdateProofSummaryInput) {}, nil},
		{"argument only", func(in *command.UpdateProofSummaryInput) { in.Argument = ptr("Three records agree") }, []string{"argument"}},
	})
}
