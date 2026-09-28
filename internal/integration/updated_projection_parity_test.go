package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"sort"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/command"
	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// Live projection vs replay parity for every *Updated command (issue #848).
//
// A command projects its event synchronously, straight after the append; a
// rebuild, a branch merge or a resume repair projects the SAME event decoded
// back out of the log. UpdateFamily once put typed values (*uuid.UUID,
// RelationType, *GenDate) into its changes map while the projection only read
// strings, so the live row silently dropped the relationship type and marriage
// date and nulled the partners, while a replay of the same log applied them:
// the read model and the event log disagreed.
//
// This scenario drives every *Updated command with every field it can change,
// then replays the whole log — decoded from its stored JSON — into a FRESH read
// model of the same backend and requires every touched row to come out
// identical. It runs once per backend (DB-001), because each backend stores and
// reads the rows its own way.

// rowGetter reads one read-model row (or row set) out of a store.
type rowGetter func(ctx context.Context, rs repository.ReadModelStore) (any, error)

func TestUpdatedCommands_LiveProjectionMatchesReplay(t *testing.T) {
	for _, backend := range backends {
		t.Run(backend.name, func(t *testing.T) {
			ctx := context.Background()
			st := backend.setup(t)
			handler := command.NewHandlerWithBranches(st.events, st.read, st.branches, st.snapshots)

			rows := runEveryUpdate(t, ctx, handler, st.read)

			fresh := backend.setup(t).read
			replayLog(t, ctx, st.events, fresh)

			names := make([]string, 0, len(rows))
			for name := range rows {
				names = append(names, name)
			}
			sort.Strings(names)
			for _, name := range names {
				get := rows[name]
				t.Run(name, func(t *testing.T) {
					live, err := get(ctx, st.read)
					if err != nil {
						t.Fatalf("read live row: %v", err)
					}
					replayed, err := get(ctx, fresh)
					if err != nil {
						t.Fatalf("read replayed row: %v", err)
					}
					liveJSON, replayJSON := canonicalRowJSON(t, live), canonicalRowJSON(t, replayed)
					if liveJSON != replayJSON {
						t.Errorf("live projection differs from replay\n live:   %s\n replay: %s", liveJSON, replayJSON)
					}
				})
			}
		})
	}
}

// runEveryUpdate seeds one of each entity, applies every *Updated command with
// every updatable field, asserts the fields that were broken or at risk landed
// in the live read model, and returns a getter per touched row.
func runEveryUpdate(t *testing.T, ctx context.Context, h *command.Handler, rs repository.ReadModelStore) map[string]rowGetter {
	t.Helper()
	must := func(step string, err error) {
		t.Helper()
		if err != nil {
			t.Fatalf("%s: %v", step, err)
		}
	}
	str := func(s string) *string { return &s }
	num := func(n int) *int { return &n }

	// --- Persons ---
	p1, err := h.CreatePerson(ctx, command.CreatePersonInput{GivenName: "Avery", Surname: "Placeholder", Gender: "unknown", BirthDate: "1 JAN 1850"})
	must("create person 1", err)
	p2, err := h.CreatePerson(ctx, command.CreatePersonInput{GivenName: "Blake", Surname: "Sample", Gender: "female"})
	must("create person 2", err)
	p3, err := h.CreatePerson(ctx, command.CreatePersonInput{GivenName: "Cameron", Surname: "Example", Gender: "male"})
	must("create person 3", err)

	_, err = h.UpdatePerson(ctx, command.UpdatePersonInput{
		ID: p1.ID, GivenName: str("Avery Jo"), Surname: str("Placeholder-Test"), Gender: str("male"),
		BirthDate: str("ABT 1851"), BirthPlace: str("Testville"), DeathDate: str("12 MAR 1920"),
		DeathPlace: str("Sampletown"), Notes: str("person notes"), ResearchStatus: str("probable"),
		Version: p1.Version,
	})
	must("update person", err)

	// --- Names (NameUpdated carries the whole name, not a changes map) ---
	alt, err := h.AddName(ctx, command.AddNameInput{PersonID: p2.ID, GivenName: "Bee", Surname: "Sample", NameType: "aka"})
	must("add name", err)
	_, err = h.UpdateName(ctx, command.UpdateNameInput{
		PersonID: p2.ID, NameID: alt.ID, GivenName: str("Beatrix"), Surname: str("Sample-Alt"),
		NamePrefix: str("Dr."), NameSuffix: str("Jr."), SurnamePrefix: str("van"), Nickname: str("Bea"),
		NameType: str("married"), IsPrimary: boolPtr(true),
	})
	must("update name", err)

	// --- Families: the issue's case. f1 changes every field; f2 clears the
	// marriage date; f3 is rolled back, which emits a compensating update. ---
	f1, err := h.CreateFamily(ctx, command.CreateFamilyInput{Partner1ID: &p1.ID, Partner2ID: &p2.ID, RelationshipType: "marriage", MarriageDate: "1 JUN 1875", MarriagePlace: "Old Chapel"})
	must("create family 1", err)
	_, err = h.UpdateFamily(ctx, command.UpdateFamilyInput{
		ID: f1.ID, Partner1ID: &p3.ID, Partner2ID: &p1.ID, RelationshipType: str("partnership"),
		MarriageDate: str("ABT 1880"), MarriagePlace: str("New Hall"), Version: f1.Version,
	})
	must("update family 1", err)

	f2, err := h.CreateFamily(ctx, command.CreateFamilyInput{Partner1ID: &p2.ID, RelationshipType: "marriage", MarriageDate: "2 FEB 1890"})
	must("create family 2", err)
	_, err = h.UpdateFamily(ctx, command.UpdateFamilyInput{ID: f2.ID, MarriageDate: str(""), Version: f2.Version})
	must("clear family 2 marriage date", err)

	f3, err := h.CreateFamily(ctx, command.CreateFamilyInput{Partner1ID: &p1.ID, Partner2ID: &p3.ID, RelationshipType: "marriage", MarriageDate: "3 MAR 1895", MarriagePlace: "Harbor"})
	must("create family 3", err)
	_, err = h.UpdateFamily(ctx, command.UpdateFamilyInput{
		ID: f3.ID, Partner1ID: &p2.ID, RelationshipType: str("unknown"), MarriageDate: str("1900"), Version: f3.Version,
	})
	must("update family 3", err)
	_, err = h.RollbackFamily(ctx, f3.ID, 1)
	must("roll back family 3", err)

	assertFamilyRow(t, ctx, rs, f1.ID, &p3.ID, &p1.ID, domain.RelationPartnership, "ABT 1880", "New Hall")
	assertFamilyRow(t, ctx, rs, f2.ID, &p2.ID, nil, domain.RelationMarriage, "", "")
	assertFamilyRow(t, ctx, rs, f3.ID, &p1.ID, &p3.ID, domain.RelationMarriage, "3 MAR 1895", "Harbor")

	// --- Sources and repository ---
	s1, err := h.CreateSource(ctx, command.CreateSourceInput{SourceType: "book", Title: "Register A"})
	must("create source 1", err)
	s2, err := h.CreateSource(ctx, command.CreateSourceInput{SourceType: "census", Title: "Register B"})
	must("create source 2", err)
	_, err = h.UpdateSource(ctx, command.UpdateSourceInput{
		ID: s1.ID, SourceType: str("archive"), Title: str("Register A (rev)"), Author: str("A. Author"),
		Publisher: str("Test Press"), PublishDate: str("1901"), URL: str("https://example.test/a"),
		RepositoryName: str("Test Archive"), CollectionName: str("Coll"), CallNumber: str("CN-1"),
		Notes: str("source notes"), Version: s1.Version,
	})
	must("update source", err)

	repo, err := h.CreateRepository(ctx, command.CreateRepositoryInput{Name: "Archive One", Address: &domain.Address{City: "Oldtown"}})
	must("create repository", err)
	_, err = h.UpdateRepository(ctx, command.UpdateRepositoryInput{
		ID: repo.ID, Name: str("Archive One (rev)"), Address: &domain.Address{Line1: "1 Test St", City: "Newtown", Country: "Testland"},
		Notes: str("repo notes"), GedcomXref: str("@R1@"), Version: repo.Version,
	})
	must("update repository", err)

	// --- Citation, every field including a source move and template fields ---
	cit, err := h.CreateCitation(ctx, command.CreateCitationInput{SourceID: s1.ID, FactType: "person_birth", FactOwnerID: p1.ID, Page: "1"})
	must("create citation", err)
	_, err = h.UpdateCitation(ctx, command.UpdateCitationInput{
		ID: cit.ID, SourceID: &s2.ID, FactType: str("person_death"), FactOwnerID: &p2.ID, Page: str("22"),
		Volume: str("III"), SourceQuality: str("original"), InformantType: str("primary"), EvidenceType: str("direct"),
		QuotedText: str("quoted"), Analysis: str("analysis"), TemplateID: str("census-1900"),
		Fields: map[string]string{"line": "7", "sheet": "2B"}, Version: cit.Version,
	})
	must("update citation", err)

	// --- Media, including the int crop fields ---
	med, err := h.UploadMedia(ctx, command.UploadMediaInput{EntityType: "person", EntityID: p1.ID, Title: "Portrait", MediaType: "photo", Filename: "p.png", FileData: parityPNG(t)})
	must("upload media", err)
	_, err = h.UpdateMedia(ctx, command.UpdateMediaInput{
		ID: med.ID, Title: str("Portrait (rev)"), Description: str("desc"), MediaType: str("document"),
		CropLeft: num(1), CropTop: num(2), CropWidth: num(30), CropHeight: num(40), Version: med.Version,
	})
	must("update media", err)

	// --- Note ---
	note, err := h.CreateNote(ctx, command.CreateNoteInput{Text: "first"})
	must("create note", err)
	_, err = h.UpdateNote(ctx, command.UpdateNoteInput{ID: note.ID, Text: str("second"), Version: note.Version})
	must("update note", err)

	// --- Submitter: address, string lists and a media ID ---
	sub, err := h.CreateSubmitter(ctx, command.CreateSubmitterInput{Name: "Submitter One"})
	must("create submitter", err)
	_, err = h.UpdateSubmitter(ctx, command.UpdateSubmitterInput{
		ID: sub.ID, Name: str("Submitter One (rev)"), Address: &domain.Address{City: "Subcity"},
		Phone: []string{"555-0100", "555-0101"}, Email: []string{"one@example.test"}, Language: str("English"),
		MediaID: &med.ID, Version: sub.Version,
	})
	must("update submitter", err)
	assertSubmitterRow(t, ctx, rs, sub.ID, med.ID)

	// --- Association, including the note ID list ---
	assoc, err := h.CreateAssociation(ctx, command.CreateAssociationInput{PersonID: p1.ID, AssociateID: p2.ID, Role: "godparent"})
	must("create association", err)
	_, err = h.UpdateAssociation(ctx, command.UpdateAssociationInput{
		ID: assoc.ID, Role: str("witness"), Phrase: str("witnessed"), Notes: str("assoc notes"),
		NoteIDs: &[]uuid.UUID{note.ID}, Version: assoc.Version,
	})
	must("update association", err)
	assertAssociationNotes(t, ctx, rs, assoc.ID, note.ID)

	// --- LDS ordinance ---
	lds, err := h.CreateLDSOrdinance(ctx, command.CreateLDSOrdinanceInput{Type: domain.LDSBaptism, PersonID: &p1.ID, Date: "1 JAN 1900", Temple: "SLAKE", Status: "COMPLETED"})
	must("create lds ordinance", err)
	_, err = h.UpdateLDSOrdinance(ctx, command.UpdateLDSOrdinanceInput{
		ID: lds.ID, Date: str("2 FEB 1902"), Place: str("Temple City"), Temple: str("LOGAN"), Status: str("SUBMITTED"), Version: lds.Version,
	})
	must("update lds ordinance", err)

	// --- GPS artifacts ---
	ea, err := h.CreateEvidenceAnalysis(ctx, command.CreateEvidenceAnalysisInput{FactType: "person_birth", SubjectID: p1.ID, CitationIDs: []uuid.UUID{cit.ID}, Conclusion: "born 1850"})
	must("create evidence analysis", err)
	_, err = h.UpdateEvidenceAnalysis(ctx, command.UpdateEvidenceAnalysisInput{
		ID: ea.ID, FactType: str("person_death"), SubjectID: &p2.ID, CitationIDs: []uuid.UUID{cit.ID},
		Conclusion: str("died 1920"), ResearchStatus: str("certain"), Notes: str("ea notes"), Version: ea.Version,
	})
	must("update evidence analysis", err)

	rl, err := h.CreateResearchLog(ctx, command.CreateResearchLogInput{SubjectID: p1.ID, SubjectType: "person", Repository: "Archive", SearchDescription: "census", Outcome: "found", SearchDate: time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)})
	must("create research log", err)
	searchDate := time.Date(2021, 6, 7, 8, 9, 10, 0, time.UTC)
	_, err = h.UpdateResearchLog(ctx, command.UpdateResearchLogInput{
		ID: rl.ID, SubjectID: &f1.ID, SubjectType: str("family"), Repository: str("Archive 2"),
		SearchDescription: str("parish"), Outcome: str("not_found"), Notes: str("rl notes"), SearchDate: &searchDate, Version: rl.Version,
	})
	must("update research log", err)

	ps, err := h.CreateProofSummary(ctx, command.CreateProofSummaryInput{FactType: "person_birth", SubjectID: p1.ID, Conclusion: "c", Argument: "a", AnalysisIDs: []uuid.UUID{ea.ID}})
	must("create proof summary", err)
	_, err = h.UpdateProofSummary(ctx, command.UpdateProofSummaryInput{
		ID: ps.ID, FactType: str("person_death"), SubjectID: &p2.ID, Conclusion: str("c2"), Argument: str("a2"),
		AnalysisIDs: []uuid.UUID{ea.ID}, ResearchStatus: str("possible"), Version: ps.Version,
	})
	must("update proof summary", err)

	branch := domain.MainBranchID
	return map[string]rowGetter{
		"person_1": func(ctx context.Context, rs repository.ReadModelStore) (any, error) {
			return rs.GetPerson(ctx, branch, p1.ID)
		},
		"person_2": func(ctx context.Context, rs repository.ReadModelStore) (any, error) {
			return rs.GetPerson(ctx, branch, p2.ID)
		},
		"person_2_names": func(ctx context.Context, rs repository.ReadModelStore) (any, error) {
			return rs.GetPersonNames(ctx, branch, p2.ID)
		},
		"family_1": func(ctx context.Context, rs repository.ReadModelStore) (any, error) {
			return rs.GetFamily(ctx, branch, f1.ID)
		},
		"family_2": func(ctx context.Context, rs repository.ReadModelStore) (any, error) {
			return rs.GetFamily(ctx, branch, f2.ID)
		},
		"family_3": func(ctx context.Context, rs repository.ReadModelStore) (any, error) {
			return rs.GetFamily(ctx, branch, f3.ID)
		},
		"source_1": func(ctx context.Context, rs repository.ReadModelStore) (any, error) {
			return rs.GetSource(ctx, branch, s1.ID)
		},
		"source_2": func(ctx context.Context, rs repository.ReadModelStore) (any, error) {
			return rs.GetSource(ctx, branch, s2.ID)
		},
		"repository": func(ctx context.Context, rs repository.ReadModelStore) (any, error) {
			return rs.GetRepository(ctx, repo.ID)
		},
		"citation": func(ctx context.Context, rs repository.ReadModelStore) (any, error) {
			return rs.GetCitation(ctx, branch, cit.ID)
		},
		"media": func(ctx context.Context, rs repository.ReadModelStore) (any, error) {
			return rs.GetMedia(ctx, branch, med.ID)
		},
		"note": func(ctx context.Context, rs repository.ReadModelStore) (any, error) {
			return rs.GetNote(ctx, branch, note.ID)
		},
		"submitter": func(ctx context.Context, rs repository.ReadModelStore) (any, error) {
			return rs.GetSubmitter(ctx, sub.ID)
		},
		"association": func(ctx context.Context, rs repository.ReadModelStore) (any, error) {
			return rs.GetAssociation(ctx, branch, assoc.ID)
		},
		"lds_ordinance": func(ctx context.Context, rs repository.ReadModelStore) (any, error) {
			return rs.GetLDSOrdinance(ctx, lds.ID)
		},
		"evidence_analysis": func(ctx context.Context, rs repository.ReadModelStore) (any, error) {
			return rs.GetEvidenceAnalysis(ctx, branch, ea.ID)
		},
		"research_log": func(ctx context.Context, rs repository.ReadModelStore) (any, error) {
			return rs.GetResearchLog(ctx, branch, rl.ID)
		},
		"proof_summary": func(ctx context.Context, rs repository.ReadModelStore) (any, error) {
			return rs.GetProofSummary(ctx, branch, ps.ID)
		},
	}
}

// replayLog projects every event in the log, decoded from its stored JSON, into
// rs — the path a rebuild takes.
func replayLog(t *testing.T, ctx context.Context, events repository.EventStore, rs repository.ReadModelStore) {
	t.Helper()
	projector := repository.NewProjector(rs, nil)
	var from int64
	for {
		page, err := events.ReadAll(ctx, from, 500)
		if err != nil {
			t.Fatalf("read log from %d: %v", from, err)
		}
		if len(page) == 0 {
			return
		}
		for _, stored := range page {
			decoded, err := stored.DecodeEvent()
			if err != nil {
				t.Fatalf("decode %s at %d: %v", stored.EventType, stored.Position, err)
			}
			if err := projector.Project(ctx, decoded, stored.Version, stored.BranchID); err != nil {
				t.Fatalf("replay %s at %d: %v", stored.EventType, stored.Position, err)
			}
			from = stored.Position
		}
	}
}

// canonicalRowJSON renders a row for comparison: JSON with every timestamp
// normalized to UTC, so an in-memory time.Now() and the same instant decoded
// from the log compare equal regardless of the process's local zone.
func canonicalRowJSON(t *testing.T, row any) string {
	t.Helper()
	b, err := json.Marshal(row)
	if err != nil {
		t.Fatalf("marshal row: %v", err)
	}
	var generic any
	if err := json.Unmarshal(b, &generic); err != nil {
		t.Fatalf("unmarshal row: %v", err)
	}
	out, err := json.Marshal(normalizeTimes(generic))
	if err != nil {
		t.Fatalf("re-marshal row: %v", err)
	}
	return string(out)
}

func normalizeTimes(v any) any {
	switch val := v.(type) {
	case map[string]any:
		for k, item := range val {
			val[k] = normalizeTimes(item)
		}
		return val
	case []any:
		for i, item := range val {
			val[i] = normalizeTimes(item)
		}
		return val
	case string:
		if ts, err := time.Parse(time.RFC3339Nano, val); err == nil {
			return ts.UTC().Format(time.RFC3339Nano)
		}
		return val
	default:
		return v
	}
}

func assertFamilyRow(t *testing.T, ctx context.Context, rs repository.ReadModelStore, id uuid.UUID, partner1, partner2 *uuid.UUID, rel domain.RelationType, marriageDate, marriagePlace string) {
	t.Helper()
	family, err := rs.GetFamily(ctx, domain.MainBranchID, id)
	if err != nil || family == nil {
		t.Fatalf("get family %s: %v (nil=%v)", id, err, family == nil)
	}
	if got, want := fmtID(family.Partner1ID), fmtID(partner1); got != want {
		t.Errorf("family %s partner1 = %s, want %s", id, got, want)
	}
	if got, want := fmtID(family.Partner2ID), fmtID(partner2); got != want {
		t.Errorf("family %s partner2 = %s, want %s", id, got, want)
	}
	if family.RelationshipType != rel {
		t.Errorf("family %s relationship_type = %q, want %q", id, family.RelationshipType, rel)
	}
	if family.MarriageDateRaw != marriageDate {
		t.Errorf("family %s marriage date = %q, want %q", id, family.MarriageDateRaw, marriageDate)
	}
	if (marriageDate == "") != (family.MarriageDateSort == nil) {
		t.Errorf("family %s marriage date sort = %v, want set iff the date is", id, family.MarriageDateSort)
	}
	if family.MarriagePlace != marriagePlace {
		t.Errorf("family %s marriage place = %q, want %q", id, family.MarriagePlace, marriagePlace)
	}
	if partner1 != nil && family.Partner1GivenName == "" {
		t.Errorf("family %s partner1 name not denormalized", id)
	}
}

func assertSubmitterRow(t *testing.T, ctx context.Context, rs repository.ReadModelStore, id, mediaID uuid.UUID) {
	t.Helper()
	sub, err := rs.GetSubmitter(ctx, id)
	if err != nil || sub == nil {
		t.Fatalf("get submitter: %v", err)
	}
	if sub.Address == nil || sub.Address.City != "Subcity" {
		t.Errorf("submitter address = %+v, want city Subcity", sub.Address)
	}
	if fmt.Sprint(sub.Phone) != "[555-0100 555-0101]" || fmt.Sprint(sub.Email) != "[one@example.test]" {
		t.Errorf("submitter phone/email = %v / %v", sub.Phone, sub.Email)
	}
	if fmtID(sub.MediaID) != mediaID.String() {
		t.Errorf("submitter media_id = %s, want %s", fmtID(sub.MediaID), mediaID)
	}
}

func assertAssociationNotes(t *testing.T, ctx context.Context, rs repository.ReadModelStore, id, noteID uuid.UUID) {
	t.Helper()
	assoc, err := rs.GetAssociation(ctx, domain.MainBranchID, id)
	if err != nil || assoc == nil {
		t.Fatalf("get association: %v", err)
	}
	if len(assoc.NoteIDs) != 1 || assoc.NoteIDs[0] != noteID {
		t.Errorf("association note_ids = %v, want [%s]", assoc.NoteIDs, noteID)
	}
}

func fmtID(id *uuid.UUID) string {
	if id == nil {
		return "<nil>"
	}
	return id.String()
}

func boolPtr(b bool) *bool { return &b }

// parityPNG is a tiny valid image for the media upload.
func parityPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			img.Set(x, y, color.RGBA{R: 10, G: 20, B: 30, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return buf.Bytes()
}
