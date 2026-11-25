// Command record writes the recorded event log of one release (#1802). It is
// not built with this module: scripts/record-event-log.sh copies it into a
// checkout of the release's tag and runs it there, so every event is written
// by that release's own code. It lives under testdata so the module's build,
// vet and lint skip it.
//
// Usage, from the root of the release's checkout:
//
//	go run ./cmd/record-event-log DB OUTDIR GEDCOM...
//
// It fills the empty SQLite database DB with synthetic data only (the demo
// seeder, the given GEDCOM files, and commands that reach every event type
// the release's DecodeEvent handles), rebuilds the read model so the rebuild's
// own backfill events are logged too, and writes to OUTDIR:
//
//   - expected.json: the read-model values the rebuild produced, keyed by
//     section and row (see dump; internal/integration/eventlog_replay_test.go
//     builds the same shape from the current code);
//   - event-types.json: every event type the release's DecodeEvent handles.
//
// The script then writes the log itself with `myfamily backup`.
//
// When a release's code no longer compiles this file, change it to the new
// API; when a release adds an event type, make it write one.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/command"
	"github.com/cacack/my-family/internal/config"
	"github.com/cacack/my-family/internal/demo"
	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/storage"
)

func main() {
	if len(os.Args) < 4 {
		log.Fatal("usage: record-event-log DB OUTDIR GEDCOM...")
	}
	db, outDir, gedcoms := os.Args[1], os.Args[2], os.Args[3:]
	ctx := context.Background()
	cfg := &config.Config{SQLitePath: db}

	handled, err := decodedEventTypes("internal/repository/eventstore.go")
	if err != nil {
		log.Fatalf("read DecodeEvent's event types: %v", err)
	}

	st, err := storage.Open(cfg, storage.AccessServe)
	if err != nil {
		log.Fatal(err)
	}
	if err := fill(ctx, st, gedcoms); err != nil {
		log.Fatalf("fill: %v", err)
	}
	if err := checkNoFailures(ctx, st); err != nil {
		log.Fatal(err)
	}
	if err := st.Close(); err != nil {
		log.Fatal(err)
	}

	// The rebuild logs the backfills of values fill left only in the read
	// model, and leaves the read model as this release's replay builds it.
	st, err = storage.Open(cfg, storage.AccessRebuild)
	if err != nil {
		log.Fatal(err)
	}
	result, err := st.RebuildReadModel(ctx)
	if err != nil {
		log.Fatalf("rebuild: %v", err)
	}
	if err := checkNoFailures(ctx, st); err != nil {
		log.Fatal(err)
	}
	if result.Backfilled == 0 || result.BackfilledBrickWalls == 0 {
		log.Fatalf("rebuild backfilled %d entities and %d brick walls, want some of each", result.Backfilled, result.BackfilledBrickWalls)
	}

	logged, err := repository.ListAll(ctx, 1000, func(ctx context.Context, opts repository.ListOptions) ([]repository.StoredEvent, int, error) {
		events, err := st.Events.ReadAll(ctx, int64(opts.Offset), opts.Limit)
		return events, -1, err
	})
	if err != nil {
		log.Fatal(err)
	}
	seen := map[string]bool{}
	for _, e := range logged {
		seen[e.EventType] = true
	}
	var missing []string
	for _, t := range handled {
		if !seen[t] {
			missing = append(missing, t)
		}
	}
	if len(missing) > 0 {
		log.Fatalf("the log lacks event types DecodeEvent handles: %v", missing)
	}

	expected, err := dump(ctx, st)
	if err != nil {
		log.Fatalf("dump: %v", err)
	}
	if err := st.Close(); err != nil {
		log.Fatal(err)
	}
	if err := writeJSON(filepath.Join(outDir, "expected.json"), expected); err != nil {
		log.Fatal(err)
	}
	if err := writeJSON(filepath.Join(outDir, "event-types.json"), handled); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("recorded %d events of %d types\n", len(logged), len(handled))
}

// checkNoFailures fails if a projection failure is recorded: the expected
// values must be what a clean projection of the log builds.
func checkNoFailures(ctx context.Context, st *storage.Stores) error {
	failures, err := st.ReadModel.ListProjectionFailures(ctx, 10)
	if err != nil {
		return err
	}
	if len(failures) > 0 {
		return fmt.Errorf("projection failures recorded: %+v", failures)
	}
	return nil
}

// decodedEventTypes returns the event types DecodeEvent's switch names in
// the source file at path, sorted.
func decodedEventTypes(path string) ([]string, error) {
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		return nil, err
	}
	var types []string
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "DecodeEvent" {
			continue
		}
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			clause, ok := n.(*ast.CaseClause)
			if !ok {
				return true
			}
			for _, expr := range clause.List {
				if lit, ok := expr.(*ast.BasicLit); ok && lit.Kind == token.STRING {
					name, err := strconv.Unquote(lit.Value)
					if err == nil {
						types = append(types, name)
					}
				}
			}
			return true
		})
	}
	if len(types) == 0 {
		return nil, errors.New("no DecodeEvent switch cases found")
	}
	sort.Strings(types)
	return types, nil
}

// failingStore fails the failAt-th mainline Append once armed, as a crash
// mid-merge would, so the merge can be resumed.
type failingStore struct {
	repository.EventStore
	mu     sync.Mutex
	armed  bool
	failAt int
	seen   int
}

func (s *failingStore) Append(ctx context.Context, streamID uuid.UUID, streamType string, events []domain.Event, expectedVersion int64, scope repository.AppendScope) error {
	s.mu.Lock()
	fail := false
	if s.armed && scope.BranchID.IsMain() {
		s.seen++
		fail = s.seen == s.failAt
	}
	s.mu.Unlock()
	if fail {
		return errors.New("injected failure")
	}
	return s.EventStore.Append(ctx, streamID, streamType, events, expectedVersion, scope)
}

// fill writes the synthetic tree: the demo seeder, the GEDCOM files, then a
// command for each event type neither writes.
func fill(ctx context.Context, st *storage.Stores, gedcoms []string) error {
	events := &failingStore{EventStore: st.Events}
	h := command.NewHandlerWithBranches(events, st.ReadModel, st.Branches, st.Snapshots)
	rs := st.ReadModel

	if err := demo.SeedDemoData(ctx, h, rs); err != nil {
		return fmt.Errorf("seed: %w", err)
	}
	for _, path := range gedcoms {
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		_, err = h.ImportGedcom(ctx, command.ImportGedcomInput{Filename: filepath.Base(path), Reader: f})
		_ = f.Close()
		if err != nil {
			return fmt.Errorf("import %s: %w", path, err)
		}
	}

	// Persons, names, families and children.
	a, err := h.CreatePerson(ctx, command.CreatePersonInput{GivenName: "Replay", Surname: "Alpha", Gender: "female", BirthDate: "3 MAR 1850", BirthPlace: "Springfield"})
	if err != nil {
		return err
	}
	b, err := h.CreatePerson(ctx, command.CreatePersonInput{GivenName: "Replay", Surname: "Beta", Gender: "male", BirthDate: "1848"})
	if err != nil {
		return err
	}
	c, err := h.CreatePerson(ctx, command.CreatePersonInput{GivenName: "Replay", Surname: "Child", BirthDate: "ABT 1875"})
	if err != nil {
		return err
	}
	gone, err := h.CreatePerson(ctx, command.CreatePersonInput{GivenName: "Replay", Surname: "Gone"})
	if err != nil {
		return err
	}
	dupe, err := h.CreatePerson(ctx, command.CreatePersonInput{GivenName: "Replay", Surname: "Alpha", BirthDate: "1850"})
	if err != nil {
		return err
	}
	upd, err := h.UpdatePerson(ctx, command.UpdatePersonInput{ID: a.ID, DeathDate: command.Set("12 DEC 1920"), DeathPlace: command.Set("Shelbyville"),
		Notes: command.Set("updated for the replay log"), ResearchStatus: command.Set("probable"), Version: a.Version})
	if err != nil {
		return err
	}
	a.Version = upd.Version
	if err := h.DeletePerson(ctx, command.DeletePersonInput{ID: gone.ID, Version: gone.Version, Reason: "replay log"}); err != nil {
		return err
	}
	name, err := h.AddName(ctx, command.AddNameInput{PersonID: a.ID, GivenName: "Rae", Surname: "Alpha", NameType: "aka"})
	if err != nil {
		return err
	}
	if _, err := h.UpdateName(ctx, command.UpdateNameInput{PersonID: a.ID, NameID: name.ID, Nickname: command.Set("Ray")}); err != nil {
		return err
	}
	extra, err := h.AddName(ctx, command.AddNameInput{PersonID: a.ID, GivenName: "R.", Surname: "Alpha", NameType: "aka"})
	if err != nil {
		return err
	}
	if err := h.DeleteName(ctx, command.DeleteNameInput{PersonID: a.ID, NameID: extra.ID}); err != nil {
		return err
	}
	fam, err := h.CreateFamily(ctx, command.CreateFamilyInput{Partner1ID: &a.ID, Partner2ID: &b.ID, RelationshipType: "marriage", MarriageDate: "1872"})
	if err != nil {
		return err
	}
	famUpd, err := h.UpdateFamily(ctx, command.UpdateFamilyInput{ID: fam.ID, MarriagePlace: command.Set("Springfield"), Version: fam.Version})
	if err != nil {
		return err
	}
	_ = famUpd
	if _, err := h.LinkChild(ctx, command.LinkChildInput{FamilyID: fam.ID, ChildID: c.ID, RelationType: "biological"}); err != nil {
		return err
	}
	if _, err := h.LinkChild(ctx, command.LinkChildInput{FamilyID: fam.ID, ChildID: dupe.ID, RelationType: "adopted"}); err != nil {
		return err
	}
	if err := h.UnlinkChild(ctx, command.UnlinkChildInput{FamilyID: fam.ID, ChildID: dupe.ID}); err != nil {
		return err
	}
	spare, err := h.CreateFamily(ctx, command.CreateFamilyInput{Partner1ID: &c.ID})
	if err != nil {
		return err
	}
	if err := h.DeleteFamily(ctx, command.DeleteFamilyInput{ID: spare.ID, Version: spare.Version}); err != nil {
		return err
	}

	// Duplicates and a person merge.
	if err := h.DismissDuplicate(ctx, command.DismissDuplicateInput{Person1ID: a.ID, Person2ID: dupe.ID, Reason: "checking"}); err != nil {
		return err
	}
	if err := h.RestoreDuplicate(ctx, a.ID, dupe.ID); err != nil {
		return err
	}
	if err := h.DismissDuplicate(ctx, command.DismissDuplicateInput{Person1ID: b.ID, Person2ID: c.ID, Reason: "father and son"}); err != nil {
		return err
	}
	aNow, err := rs.GetPerson(ctx, domain.MainBranchID, a.ID)
	if err != nil {
		return err
	}
	dupeNow, err := rs.GetPerson(ctx, domain.MainBranchID, dupe.ID)
	if err != nil {
		return err
	}
	if _, err := h.MergePersons(ctx, command.MergePersonsInput{SurvivorID: a.ID, MergedID: dupe.ID, SurvivorVersion: aNow.Version, MergedVersion: dupeNow.Version}); err != nil {
		return fmt.Errorf("merge persons: %w", err)
	}

	// Brick walls.
	if err := h.SetBrickWall(ctx, b.ID, "no parents found"); err != nil {
		return err
	}
	if err := h.ResolveBrickWall(ctx, b.ID); err != nil {
		return err
	}

	// Sources, citations, repositories, notes, submitters, media.
	src, err := h.CreateSource(ctx, command.CreateSourceInput{SourceType: "book", Title: "Replay Parish Register", Author: "A. Clerk"})
	if err != nil {
		return err
	}
	if _, err := h.UpdateSource(ctx, command.UpdateSourceInput{ID: src.ID, Publisher: command.Set("Replay Press"), Version: src.Version}); err != nil {
		return err
	}
	cit, err := h.CreateCitation(ctx, command.CreateCitationInput{SourceID: src.ID, FactType: "person_birth", FactOwnerID: a.ID, Page: "p. 12",
		SourceQuality: "original", InformantType: "primary", EvidenceType: "direct"})
	if err != nil {
		return err
	}
	if _, err := h.UpdateCitation(ctx, command.UpdateCitationInput{ID: cit.ID, Volume: command.Set("2"), Version: cit.Version}); err != nil {
		return err
	}
	spareSrc, err := h.CreateSource(ctx, command.CreateSourceInput{SourceType: "webpage", Title: "Replay Spare"})
	if err != nil {
		return err
	}
	spareCit, err := h.CreateCitation(ctx, command.CreateCitationInput{SourceID: spareSrc.ID, FactType: "person_death", FactOwnerID: a.ID})
	if err != nil {
		return err
	}
	if err := h.DeleteCitation(ctx, spareCit.ID, spareCit.Version, "replay log"); err != nil {
		return err
	}
	if err := h.DeleteSource(ctx, spareSrc.ID, spareSrc.Version, "replay log"); err != nil {
		return err
	}
	repo, err := h.CreateRepository(ctx, command.CreateRepositoryInput{Name: "Replay Archive", Address: &domain.Address{City: "Springfield"}})
	if err != nil {
		return err
	}
	if _, err := h.UpdateRepository(ctx, command.UpdateRepositoryInput{ID: repo.ID, Notes: command.Set("open Mondays"), Version: repo.Version}); err != nil {
		return err
	}
	spareRepo, err := h.CreateRepository(ctx, command.CreateRepositoryInput{Name: "Replay Spare Archive"})
	if err != nil {
		return err
	}
	if err := h.DeleteRepository(ctx, spareRepo.ID, spareRepo.Version, "replay log"); err != nil {
		return err
	}
	note, err := h.CreateNote(ctx, command.CreateNoteInput{Text: "A shared note."})
	if err != nil {
		return err
	}
	text := "A shared note, revised."
	if _, err := h.UpdateNote(ctx, command.UpdateNoteInput{ID: note.ID, Text: &text, Version: note.Version}); err != nil {
		return err
	}
	spareNote, err := h.CreateNote(ctx, command.CreateNoteInput{Text: "A spare note."})
	if err != nil {
		return err
	}
	if err := h.DeleteNote(ctx, spareNote.ID, spareNote.Version, "replay log"); err != nil {
		return err
	}
	sub, err := h.CreateSubmitter(ctx, command.CreateSubmitterInput{Name: "Replay Submitter", Email: []string{"submitter@example.org"}})
	if err != nil {
		return err
	}
	if _, err := h.UpdateSubmitter(ctx, command.UpdateSubmitterInput{ID: sub.ID, Language: command.Set("English"), Version: sub.Version}); err != nil {
		return err
	}
	spareSub, err := h.CreateSubmitter(ctx, command.CreateSubmitterInput{Name: "Replay Spare Submitter"})
	if err != nil {
		return err
	}
	if err := h.DeleteSubmitter(ctx, spareSub.ID, spareSub.Version, "replay log"); err != nil {
		return err
	}
	media, err := h.UploadMedia(ctx, command.UploadMediaInput{EntityType: "person", EntityID: a.ID, Title: "Portrait", MediaType: "photo",
		Filename: "portrait.png", FileData: pixelPNG})
	if err != nil {
		return fmt.Errorf("upload media: %w", err)
	}
	if _, err := h.UpdateMedia(ctx, command.UpdateMediaInput{ID: media.ID, Description: command.Set("about 1870"), Version: media.Version}); err != nil {
		return err
	}
	spareMedia, err := h.UploadMedia(ctx, command.UploadMediaInput{EntityType: "person", EntityID: b.ID, Title: "Spare", MediaType: "photo",
		Filename: "spare.png", FileData: pixelPNG})
	if err != nil {
		return err
	}
	if err := h.DeleteMedia(ctx, spareMedia.ID, spareMedia.Version, "replay log"); err != nil {
		return err
	}

	// Associations and LDS ordinances.
	assoc, err := h.CreateAssociation(ctx, command.CreateAssociationInput{PersonID: a.ID, AssociateID: b.ID, Role: "witness"})
	if err != nil {
		return err
	}
	if _, err := h.UpdateAssociation(ctx, command.UpdateAssociationInput{ID: assoc.ID, Phrase: command.Set("witness at the baptism"), Version: assoc.Version}); err != nil {
		return err
	}
	spareAssoc, err := h.CreateAssociation(ctx, command.CreateAssociationInput{PersonID: b.ID, AssociateID: c.ID, Role: "godparent"})
	if err != nil {
		return err
	}
	if err := h.DeleteAssociation(ctx, spareAssoc.ID, spareAssoc.Version, "replay log"); err != nil {
		return err
	}
	lds, err := h.CreateLDSOrdinance(ctx, command.CreateLDSOrdinanceInput{Type: domain.LDSBaptism, PersonID: &a.ID, Date: "1900", Temple: "SLAKE"})
	if err != nil {
		return err
	}
	if _, err := h.UpdateLDSOrdinance(ctx, command.UpdateLDSOrdinanceInput{ID: lds.ID, Status: command.Set("COMPLETED"), Version: lds.Version}); err != nil {
		return err
	}
	spareLDS, err := h.CreateLDSOrdinance(ctx, command.CreateLDSOrdinanceInput{Type: domain.LDSSealingSpouse, FamilyID: &fam.ID, Date: "1901"})
	if err != nil {
		return err
	}
	if err := h.DeleteLDSOrdinance(ctx, spareLDS.ID, spareLDS.Version, "replay log"); err != nil {
		return err
	}

	// Evidence: analyses, a conflict, a research log, a proof summary.
	ana, err := h.CreateEvidenceAnalysis(ctx, command.CreateEvidenceAnalysisInput{FactType: "person_birth", SubjectID: a.ID, CitationIDs: []uuid.UUID{cit.ID},
		Conclusion: "born 3 March 1850", ResearchStatus: "probable"})
	if err != nil {
		return err
	}
	anaUpd, err := h.UpdateEvidenceAnalysis(ctx, command.UpdateEvidenceAnalysisInput{ID: ana.ID, Notes: command.Set("one original record"), Version: ana.Version})
	if err != nil {
		return err
	}
	_ = anaUpd
	rival, err := h.CreateEvidenceAnalysis(ctx, command.CreateEvidenceAnalysisInput{FactType: "person_birth", SubjectID: a.ID, Conclusion: "born 1851", ResearchStatus: "possible"})
	if err != nil {
		return err
	}
	if rival.ConflictID == nil {
		return errors.New("the rival analysis detected no conflict")
	}
	conflict, err := rs.GetEvidenceConflict(ctx, domain.MainBranchID, *rival.ConflictID)
	if err != nil || conflict == nil {
		return fmt.Errorf("read conflict: %v", err)
	}
	if _, err := h.ResolveEvidenceConflict(ctx, conflict.ID, "the 1850 register is the original", conflict.Version); err != nil {
		return err
	}
	rivalNow, err := rs.GetEvidenceAnalysis(ctx, domain.MainBranchID, rival.ID)
	if err != nil || rivalNow == nil {
		return fmt.Errorf("read rival analysis: %v", err)
	}
	if err := h.DeleteEvidenceAnalysis(ctx, rival.ID, rivalNow.Version, "replay log"); err != nil {
		return fmt.Errorf("delete analysis: %w", err)
	}
	rlog, err := h.CreateResearchLog(ctx, command.CreateResearchLogInput{SubjectID: a.ID, SubjectType: "person", Repository: "Replay Archive",
		SearchDescription: "baptisms 1845-1855", Outcome: "found", SearchDate: time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)})
	if err != nil {
		return err
	}
	if _, err := h.UpdateResearchLog(ctx, command.UpdateResearchLogInput{ID: rlog.ID, Notes: command.Set("entry 41"), Version: rlog.Version}); err != nil {
		return err
	}
	spareLog, err := h.CreateResearchLog(ctx, command.CreateResearchLogInput{SubjectID: b.ID, SubjectType: "person", Repository: "Replay Archive",
		SearchDescription: "burials", Outcome: "not_found", SearchDate: time.Date(2026, 1, 16, 0, 0, 0, 0, time.UTC)})
	if err != nil {
		return err
	}
	if err := h.DeleteResearchLog(ctx, spareLog.ID, spareLog.Version, "replay log"); err != nil {
		return err
	}
	proof, err := h.CreateProofSummary(ctx, command.CreateProofSummaryInput{FactType: "person_birth", SubjectID: a.ID, Conclusion: "born 3 March 1850",
		Argument: "the original register outweighs the census", AnalysisIDs: []uuid.UUID{ana.ID}, ResearchStatus: "probable"})
	if err != nil {
		return err
	}
	if _, err := h.UpdateProofSummary(ctx, command.UpdateProofSummaryInput{ID: proof.ID, ResearchStatus: command.Set("certain"), Version: proof.Version}); err != nil {
		return err
	}
	spareProof, err := h.CreateProofSummary(ctx, command.CreateProofSummaryInput{FactType: "person_death", SubjectID: a.ID, Conclusion: "died 1920",
		Argument: "a spare argument"})
	if err != nil {
		return err
	}
	if err := h.DeleteProofSummary(ctx, spareProof.ID, spareProof.Version, "replay log"); err != nil {
		return err
	}

	// Life events and attributes: no command of this release updates or
	// deletes them, so their events are appended as the projection reads them.
	lifeEvents, err := repository.ListAll(ctx, 1000, rs.ListEvents)
	if err != nil {
		return err
	}
	attributes, err := repository.ListAll(ctx, 1000, rs.ListAttributes)
	if err != nil {
		return err
	}
	if len(lifeEvents) < 2 || len(attributes) < 2 {
		return fmt.Errorf("imported %d life events and %d attributes, want at least 2 of each", len(lifeEvents), len(attributes))
	}
	sort.Slice(lifeEvents, func(i, j int) bool { return lifeEvents[i].ID.String() < lifeEvents[j].ID.String() })
	if err := appendAndProject(ctx, st, lifeEvents[0].ID, "event", lifeEvents[0].Version,
		domain.NewLifeEventUpdated(lifeEvents[0].ID, map[string]any{"place": "Replay Town", "description": "updated for the replay log"})); err != nil {
		return err
	}
	if err := appendAndProject(ctx, st, lifeEvents[1].ID, "event", lifeEvents[1].Version, domain.NewLifeEventDeleted(lifeEvents[1].ID, "replay log")); err != nil {
		return err
	}
	sort.Slice(attributes, func(i, j int) bool { return attributes[i].ID.String() < attributes[j].ID.String() })
	if err := appendAndProject(ctx, st, attributes[0].ID, "attribute", attributes[0].Version,
		domain.NewAttributeUpdated(attributes[0].ID, map[string]any{"value": "Replay value"})); err != nil {
		return err
	}
	if err := appendAndProject(ctx, st, attributes[1].ID, "attribute", attributes[1].Version, domain.NewAttributeDeleted(attributes[1].ID, "replay log")); err != nil {
		return err
	}

	// Snapshots.
	if _, err := h.CreateSnapshot(ctx, "replay kept", "stays"); err != nil {
		return err
	}
	spareSnap, err := h.CreateSnapshot(ctx, "replay spare", "")
	if err != nil {
		return err
	}
	if err := h.DeleteSnapshot(ctx, spareSnap.ID); err != nil {
		return err
	}

	// Branches: one merged, one whose merge fails part-way and is resumed,
	// one closed, and one left open with a change of its own.
	merged, err := h.CreateBranch(ctx, "replay merged", "")
	if err != nil {
		return err
	}
	if _, err := h.UpdateBranch(ctx, command.UpdateBranchInput{BranchID: merged.ID, Hypothesis: command.Set("Beta was born in 1848")}); err != nil {
		return err
	}
	if err := branchUpdate(ctx, h, rs, merged, b.ID, "Beta", "Merged"); err != nil {
		return err
	}
	if _, err := h.MergeBranch(ctx, command.MergeBranchInput{BranchID: merged.ID, Note: "replay merge"}); err != nil {
		return fmt.Errorf("merge branch: %w", err)
	}
	resumed, err := h.CreateBranch(ctx, "replay resumed", "")
	if err != nil {
		return err
	}
	if err := branchUpdate(ctx, h, rs, resumed, a.ID, "Replay", "Resumed"); err != nil {
		return err
	}
	if err := branchUpdate(ctx, h, rs, resumed, c.ID, "Replay", "Resumed"); err != nil {
		return err
	}
	events.mu.Lock()
	events.armed, events.failAt, events.seen = true, 2, 0
	events.mu.Unlock()
	_, mergeErr := h.MergeBranch(ctx, command.MergeBranchInput{BranchID: resumed.ID})
	events.mu.Lock()
	events.armed = false
	events.mu.Unlock()
	if mergeErr == nil {
		return errors.New("the merge meant to fail part-way succeeded")
	}
	// Main moves on the person the merge did not reach, so the resume needs,
	// and records, a decision for it.
	pending := c.ID
	if landed, err := rs.GetPerson(ctx, domain.MainBranchID, c.ID); err != nil {
		return err
	} else if landed.Surname == "Resumed" {
		pending = a.ID
	}
	pendingNow, err := rs.GetPerson(ctx, domain.MainBranchID, pending)
	if err != nil {
		return err
	}
	if _, err := h.UpdatePerson(ctx, command.UpdatePersonInput{ID: pending, Notes: command.Set("changed on main mid-merge"), Version: pendingNow.Version}); err != nil {
		return err
	}
	if _, err := h.ResumeMerge(ctx, command.ResumeMergeInput{BranchID: resumed.ID,
		Resolutions: map[uuid.UUID]command.MergeResolution{pending: command.ResolveBranch},
		Rationales:  map[uuid.UUID]string{pending: "the branch's names are the researched ones"}}); err != nil {
		return fmt.Errorf("resume merge: %w", err)
	}
	closed, err := h.CreateBranch(ctx, "replay closed", "")
	if err != nil {
		return err
	}
	if err := h.CloseBranch(ctx, closed.ID, domain.BranchOutcomeDisproved, "replay log"); err != nil {
		return err
	}
	open, err := h.CreateBranch(ctx, "replay open", "left open")
	if err != nil {
		return err
	}
	if err := branchUpdate(ctx, h, rs, open, c.ID, "Replay", "Open"); err != nil {
		return err
	}

	// Values only the read model holds, as releases before #802 and #1225
	// left them: the rebuild logs them as backfill events.
	since := time.Date(2020, 5, 1, 0, 0, 0, 0, time.UTC)
	child, err := rs.GetPerson(ctx, domain.MainBranchID, c.ID)
	if err != nil {
		return err
	}
	child.BrickWallNote, child.BrickWallSince = "marked before #802", &since
	lat, long := "39.78", "-89.65"
	child.BirthPlace, child.BirthPlaceLat, child.BirthPlaceLong = "Springfield", &lat, &long
	return rs.SavePerson(ctx, domain.MainBranchID, child)
}

// branchUpdate changes a person's names on branch b.
func branchUpdate(ctx context.Context, h *command.Handler, rs repository.ReadModelStore, b *domain.Branch, id uuid.UUID, given, surname string) error {
	p, err := rs.GetPerson(ctx, domain.BranchID(b.ID), id)
	if err != nil {
		return err
	}
	if p == nil {
		if p, err = rs.GetPerson(ctx, domain.MainBranchID, id); err != nil {
			return err
		}
	}
	_, err = h.WithBranch(b).UpdatePerson(ctx, command.UpdatePersonInput{ID: id, GivenName: &given, Surname: command.Set(surname), Version: p.Version})
	return err
}

// appendAndProject appends event to a stream at version and projects it, as
// a command would.
func appendAndProject(ctx context.Context, st *storage.Stores, id uuid.UUID, streamType string, version int64, event domain.Event) error {
	if err := st.Events.Append(ctx, id, streamType, []domain.Event{event}, version, repository.MainScope); err != nil {
		return fmt.Errorf("append %s: %w", event.EventType(), err)
	}
	return repository.NewProjector(st.ReadModel, st.Branches).Project(ctx, event, version+1, domain.MainBranchID)
}

// pixelPNG is a 1x1 PNG.
var pixelPNG = []byte{
	0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52,
	0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01, 0x08, 0x06, 0x00, 0x00, 0x00, 0x1f, 0x15, 0xc4,
	0x89, 0x00, 0x00, 0x00, 0x0d, 0x49, 0x44, 0x41, 0x54, 0x78, 0x9c, 0x63, 0xf8, 0xcf, 0xc0, 0xf0,
	0x1f, 0x00, 0x05, 0x00, 0x01, 0xff, 0x89, 0x99, 0x3d, 0x1d, 0x00, 0x00, 0x00, 0x00, 0x49, 0x45,
	0x4e, 0x44, 0xae, 0x42, 0x60, 0x82,
}

// dump returns the read-model values to compare: each section maps a row's
// key to the row as JSON. Keep it in step with replayDump in
// internal/integration/eventlog_replay_test.go: a section or key this
// writes that the test does not build fails the test.
func dump(ctx context.Context, st *storage.Stores) (map[string]map[string]any, error) {
	rs := st.ReadModel
	out := map[string]map[string]any{}
	var errs []error
	add := func(section string, rows any, err error, keys ...string) {
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", section, err))
			return
		}
		keyed, err := keyRows(rows, keys...)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", section, err))
			return
		}
		out[section] = keyed
	}

	persons, err := repository.ListAll(ctx, 1000, rs.ListPersons)
	add("persons", persons, err, "id")
	families, err := repository.ListAll(ctx, 1000, rs.ListFamilies)
	add("families", families, err, "id")
	children, err := rs.ListAllFamilyChildren(ctx, domain.MainBranchID)
	add("family_children", children, err, "family_id", "person_id")
	var names []repository.PersonNameReadModel
	var media []repository.MediaReadModel
	for _, p := range persons {
		n, err := rs.GetPersonNames(ctx, domain.MainBranchID, p.ID)
		if err != nil {
			errs = append(errs, err)
		}
		names = append(names, n...)
		m, _, err := rs.ListMediaForEntity(ctx, "person", p.ID, repository.ListOptions{Limit: 1000})
		if err != nil {
			errs = append(errs, err)
		}
		media = append(media, m...)
	}
	add("person_names", names, nil, "id")
	add("media", media, nil, "id")
	sources, err := repository.ListAll(ctx, 1000, rs.ListSources)
	add("sources", sources, err, "id")
	citations, err := repository.ListAll(ctx, 1000, rs.ListCitations)
	add("citations", citations, err, "id")
	lifeEvents, err := repository.ListAll(ctx, 1000, rs.ListEvents)
	add("life_events", lifeEvents, err, "id")
	attributes, err := repository.ListAll(ctx, 1000, rs.ListAttributes)
	add("attributes", attributes, err, "id")
	notes, err := repository.ListAll(ctx, 1000, rs.ListNotes)
	add("notes", notes, err, "id")
	submitters, err := repository.ListAll(ctx, 1000, rs.ListSubmitters)
	add("submitters", submitters, err, "id")
	repositories, err := repository.ListAll(ctx, 1000, rs.ListRepositories)
	add("repositories", repositories, err, "id")
	associations, err := repository.ListAll(ctx, 1000, rs.ListAssociations)
	add("associations", associations, err, "id")
	ordinances, err := repository.ListAll(ctx, 1000, rs.ListLDSOrdinances)
	add("lds_ordinances", ordinances, err, "id")
	analyses, err := repository.ListAll(ctx, 1000, rs.ListEvidenceAnalyses)
	add("evidence_analyses", analyses, err, "id")
	conflicts, err := repository.ListAll(ctx, 1000, rs.ListEvidenceConflicts)
	add("evidence_conflicts", conflicts, err, "id")
	logs, err := repository.ListAll(ctx, 1000, rs.ListResearchLogs)
	add("research_logs", logs, err, "id")
	proofs, err := repository.ListAll(ctx, 1000, rs.ListProofSummaries)
	add("proof_summaries", proofs, err, "id")
	dismissals, err := repository.ListAll(ctx, 1000, rs.ListDuplicateDismissals)
	add("duplicate_dismissals", dismissals, err, "id")
	walls, err := rs.GetBrickWalls(ctx, true, repository.Privacy{})
	add("brick_walls", walls, err, "person_id")
	branches, err := st.Branches.List(ctx)
	add("branches", branches, err, "id")
	snapshots, err := st.Snapshots.List(ctx, domain.MainBranchID)
	add("snapshots", snapshots, err, "id")
	for _, b := range branches {
		if b.Status != domain.BranchStatusActive {
			continue
		}
		onBranch, err := repository.ListAllOn(ctx, domain.BranchID(b.ID), 1000, rs.ListPersons)
		add("branch_persons/"+b.ID.String(), onBranch, err, "id")
	}
	return out, errors.Join(errs...)
}

// keyRows turns a slice of rows into a map from each row's key (its fields
// named by keys, joined by "/") to the row as decoded JSON.
func keyRows(rows any, keys ...string) (map[string]any, error) {
	data, err := json.Marshal(rows)
	if err != nil {
		return nil, err
	}
	var list []map[string]any
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	if err := dec.Decode(&list); err != nil {
		return nil, err
	}
	out := map[string]any{}
	for _, row := range list {
		key := ""
		for i, k := range keys {
			if i > 0 {
				key += "/"
			}
			key += fmt.Sprint(row[k])
		}
		if _, dup := out[key]; dup {
			return nil, fmt.Errorf("two rows share the key %s", key)
		}
		out[key] = row
	}
	return out, nil
}

func writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o600)
}
