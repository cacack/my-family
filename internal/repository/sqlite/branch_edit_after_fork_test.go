package sqlite_test

import (
	"context"
	"errors"
	"os"
	"reflect"
	"testing"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/command"
	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/query"
	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/repository/sqlite"
)

// TestBranchEditAfterMainCorrection runs the #844 scenario against the
// sqlite backend, wired into a real command handler.
func TestBranchEditAfterMainCorrection(t *testing.T) {
	tmp, err := os.CreateTemp("", "myfamily-branch-edit-*.db")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	tmp.Close()
	t.Cleanup(func() { os.Remove(tmp.Name()) })

	db, err := sqlite.OpenDB(tmp.Name())
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	eventStore, err := sqlite.NewEventStore(db)
	if err != nil {
		t.Fatalf("create event store: %v", err)
	}
	readStore, err := sqlite.NewReadModelStore(db)
	if err != nil {
		t.Fatalf("create read model store: %v", err)
	}
	branchStore, err := sqlite.NewBranchStore(db)
	if err != nil {
		t.Fatalf("create branch store: %v", err)
	}
	snapshots, err := sqlite.NewSnapshotStore(db)
	if err != nil {
		t.Fatalf("create snapshot store: %v", err)
	}

	runBranchEditAfterMainCorrection(t, command.NewHandlerWithBranches(eventStore, readStore, branchStore, snapshots), readStore)
}

// runBranchEditAfterMainCorrection is the backend-agnostic body of the #844
// scenario: on a branch, an entity main edited AFTER the fork (and the branch
// has not touched) must be editable with the version the branch read shows,
// which is main's current version (ADR-005's live overlay). Each backend package
// carries an identical copy of this body (there is no shared test harness in
// this repo); keeping it byte-identical is the DB-001 parity guarantee. Fixtures
// use neutral placeholder names only (public repo — no real PII).
func runBranchEditAfterMainCorrection(t *testing.T, h *command.Handler, readStore repository.ReadModelStore) {
	t.Helper()
	ctx := context.Background()
	str := func(s string) *string { return &s }

	// --- Main: a person at version 2 before the fork (the issue's steps 1-2),
	// plus a family and a source at version 1, to prove the fix is generic. ---
	person, err := h.CreatePerson(ctx, command.CreatePersonInput{GivenName: "Alex", Surname: "Placeholder"})
	if err != nil {
		t.Fatalf("CreatePerson: %v", err)
	}
	if _, err := h.UpdatePerson(ctx, command.UpdatePersonInput{ID: person.ID, BirthPlace: str("Placeville"), Version: 1}); err != nil {
		t.Fatalf("pre-fork UpdatePerson: %v", err)
	}
	family, err := h.CreateFamily(ctx, command.CreateFamilyInput{Partner1ID: &person.ID, RelationshipType: "marriage"})
	if err != nil {
		t.Fatalf("CreateFamily: %v", err)
	}
	source, err := h.CreateSource(ctx, command.CreateSourceInput{Title: "Placeholder Register", SourceType: "book"})
	if err != nil {
		t.Fatalf("CreateSource: %v", err)
	}
	cited, err := h.CreateSource(ctx, command.CreateSourceInput{Title: "Placeholder Census", SourceType: "book"})
	if err != nil {
		t.Fatalf("CreateSource (cited): %v", err)
	}

	branch, err := h.CreateBranch(ctx, "post-fork-edits", "")
	if err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}
	onBranch := h.WithBranch(branch)
	branchID := domain.BranchID(branch.ID)

	// --- Main corrects every entity AFTER the fork (step 3). ---
	if _, err := h.UpdatePerson(ctx, command.UpdatePersonInput{ID: person.ID, Surname: str("Mainfix"), Version: 2}); err != nil {
		t.Fatalf("post-fork main UpdatePerson: %v", err)
	}
	if _, err := h.UpdateFamily(ctx, command.UpdateFamilyInput{ID: family.ID, MarriagePlace: str("Mainville"), Version: 1}); err != nil {
		t.Fatalf("post-fork main UpdateFamily: %v", err)
	}
	if _, err := h.UpdateSource(ctx, command.UpdateSourceInput{ID: source.ID, Author: str("Main Author"), Version: 1}); err != nil {
		t.Fatalf("post-fork main UpdateSource: %v", err)
	}

	// --- The branch reads main's CURRENT row for the untouched person (step 4). ---
	shown, err := readStore.GetPerson(ctx, branchID, person.ID)
	if err != nil || shown == nil {
		t.Fatalf("branch GetPerson = %v (err %v), want main's current row", shown, err)
	}
	if shown.Version != 3 || shown.Surname != "Mainfix" {
		t.Fatalf("branch read = version %d surname %q, want version 3 surname %q", shown.Version, shown.Surname, "Mainfix")
	}

	// --- The as-of-fork version is stale: the branch never showed it. ---
	if _, err := onBranch.UpdatePerson(ctx, command.UpdatePersonInput{ID: person.ID, Surname: str("Stale"), Version: 2}); !errors.Is(err, repository.ErrConcurrencyConflict) {
		t.Fatalf("branch UpdatePerson at the as-of-fork version: want ErrConcurrencyConflict, got %v", err)
	}

	// --- Step 5: the edit succeeds with the version the branch displayed. ---
	res, err := onBranch.UpdatePerson(ctx, command.UpdatePersonInput{ID: person.ID, Surname: str("Branchfix"), Version: shown.Version})
	if err != nil {
		t.Fatalf("branch UpdatePerson at the displayed version: %v", err)
	}
	if res.Version != 4 {
		t.Fatalf("branch UpdatePerson version = %d, want 4", res.Version)
	}
	after, err := readStore.GetPerson(ctx, branchID, person.ID)
	if err != nil || after == nil {
		t.Fatalf("branch GetPerson after edit = %v (err %v)", after, err)
	}
	if after.Version != 4 || after.Surname != "Branchfix" || after.BirthPlace != "Placeville" {
		t.Fatalf("branch row after edit = version %d surname %q birth place %q, want 4 %q %q",
			after.Version, after.Surname, after.BirthPlace, "Branchfix", "Placeville")
	}
	onMain, err := readStore.GetPerson(ctx, domain.MainBranchID, person.ID)
	if err != nil || onMain == nil {
		t.Fatalf("main GetPerson = %v (err %v)", onMain, err)
	}
	if onMain.Version != 3 || onMain.Surname != "Mainfix" {
		t.Fatalf("main row after branch edit = version %d surname %q, want 3 %q (isolated)", onMain.Version, onMain.Surname, "Mainfix")
	}

	// --- A second edit on the branch continues the branch's own line. ---
	if _, err := onBranch.UpdatePerson(ctx, command.UpdatePersonInput{ID: person.ID, Notes: str("branch note"), Version: 4}); err != nil {
		t.Fatalf("second branch UpdatePerson: %v", err)
	}

	// --- Family and source: the same rule, through the same append path.
	// UpdateFamily checks no version of its own before appending, so these
	// refusals and acceptances come from the event store's append alone. ---
	shownFamily, err := readStore.GetFamily(ctx, branchID, family.ID)
	if err != nil || shownFamily == nil || shownFamily.Version != 2 {
		t.Fatalf("branch GetFamily = %+v (err %v), want main's current version 2", shownFamily, err)
	}
	if _, err := onBranch.UpdateFamily(ctx, command.UpdateFamilyInput{ID: family.ID, MarriagePlace: str("Branchtown"), Version: 1}); !errors.Is(err, repository.ErrConcurrencyConflict) {
		t.Fatalf("branch UpdateFamily at the as-of-fork version: want ErrConcurrencyConflict, got %v", err)
	}
	// A main write landing between the branch's read and its write makes the
	// displayed version stale, exactly as it would on main.
	if _, err := h.UpdateFamily(ctx, command.UpdateFamilyInput{ID: family.ID, MarriagePlace: str("Mainville Again"), Version: 2}); err != nil {
		t.Fatalf("racing main UpdateFamily: %v", err)
	}
	if _, err := onBranch.UpdateFamily(ctx, command.UpdateFamilyInput{ID: family.ID, MarriagePlace: str("Branchtown"), Version: shownFamily.Version}); !errors.Is(err, repository.ErrConcurrencyConflict) {
		t.Fatalf("branch UpdateFamily after a racing main write: want ErrConcurrencyConflict, got %v", err)
	}
	if _, err := onBranch.UpdateFamily(ctx, command.UpdateFamilyInput{ID: family.ID, MarriagePlace: str("Branchtown"), Version: 3}); err != nil {
		t.Fatalf("branch UpdateFamily at the re-read version: %v", err)
	}
	shownSource, err := readStore.GetSource(ctx, branchID, source.ID)
	if err != nil || shownSource == nil || shownSource.Version != 2 {
		t.Fatalf("branch GetSource = %+v (err %v), want main's current version 2", shownSource, err)
	}
	if _, err := onBranch.UpdateSource(ctx, command.UpdateSourceInput{ID: source.ID, CallNumber: str("B-1"), Version: shownSource.Version}); err != nil {
		t.Fatalf("branch UpdateSource at the displayed version: %v", err)
	}

	// --- Main keeps editing the source after the branch did (another field). ---
	if _, err := h.UpdateSource(ctx, command.UpdateSourceInput{ID: source.ID, Notes: str("main again"), Version: 2}); err != nil {
		t.Fatalf("main UpdateSource after branch edit: %v", err)
	}

	// --- A cross-stream shadow: a branch citation bumps its source's citation
	// count by saving a branch copy of the source row, and that copy keeps the
	// version the source had when it was copied. Main then corrects the source,
	// but the branch keeps serving its copy — so the branch has appended nothing
	// to the source's stream, yet reads a row older than main's. Its first edit
	// must expect the copy's version, not main's current one. ---
	if _, err := onBranch.CreateCitation(ctx, command.CreateCitationInput{SourceID: cited.ID, FactType: "person_birth", FactOwnerID: person.ID}); err != nil {
		t.Fatalf("branch CreateCitation: %v", err)
	}
	if _, err := h.UpdateSource(ctx, command.UpdateSourceInput{ID: cited.ID, Author: str("Main Author"), Version: 1}); err != nil {
		t.Fatalf("post-citation main UpdateSource: %v", err)
	}
	shownCited, err := readStore.GetSource(ctx, branchID, cited.ID)
	if err != nil || shownCited == nil {
		t.Fatalf("branch GetSource (cited) = %v (err %v)", shownCited, err)
	}
	if shownCited.Version != 1 || shownCited.CitationCount != 1 || shownCited.Author != "" {
		t.Fatalf("branch cited source = version %d citations %d author %q, want the branch's copy: 1, 1, %q",
			shownCited.Version, shownCited.CitationCount, shownCited.Author, "")
	}
	if _, err := onBranch.UpdateSource(ctx, command.UpdateSourceInput{ID: cited.ID, CallNumber: str("B-2"), Version: 2}); !errors.Is(err, repository.ErrConcurrencyConflict) {
		t.Fatalf("branch UpdateSource at main's version, which the branch never showed: want ErrConcurrencyConflict, got %v", err)
	}
	citedRes, err := onBranch.UpdateSource(ctx, command.UpdateSourceInput{ID: cited.ID, CallNumber: str("B-2"), Version: shownCited.Version})
	if err != nil {
		t.Fatalf("branch UpdateSource of a shadowed source at the displayed version: %v", err)
	}
	if citedRes.Version != 2 {
		t.Fatalf("branch UpdateSource (cited) version = %d, want 2", citedRes.Version)
	}
	afterCited, err := readStore.GetSource(ctx, branchID, cited.ID)
	if err != nil || afterCited == nil {
		t.Fatalf("branch GetSource (cited) after edit = %v (err %v)", afterCited, err)
	}
	if afterCited.Version != 2 || afterCited.CallNumber != "B-2" || afterCited.CitationCount != 1 {
		t.Fatalf("branch cited source after edit = version %d call number %q citations %d, want 2 %q 1",
			afterCited.Version, afterCited.CallNumber, afterCited.CitationCount, "B-2")
	}
	if _, err := onBranch.UpdateSource(ctx, command.UpdateSourceInput{ID: cited.ID, Notes: str("stale"), Version: 1}); !errors.Is(err, repository.ErrConcurrencyConflict) {
		t.Fatalf("branch UpdateSource (cited) at the now-stale version: want ErrConcurrencyConflict, got %v", err)
	}

	// --- Merge: main's post-fork events are main changes (they sit after the
	// base position), so where the branch then changed the same field — the
	// person's surname, the family's marriage place — the compare reports an
	// edit/edit conflict. The source edits touched different fields on each
	// side, so the source merges cleanly. ---
	merge, err := h.MergeBranch(ctx, command.MergeBranchInput{BranchID: branch.ID})
	if !errors.Is(err, command.ErrMergeConflicts) {
		t.Fatalf("MergeBranch error = %v, want ErrMergeConflicts", err)
	}
	if merge == nil {
		t.Fatal("MergeBranch returned no result alongside ErrMergeConflicts")
	}
	wantFields := map[uuid.UUID][]string{person.ID: {"surname"}, family.ID: {"marriage_place"}}
	if len(merge.Conflicts) != len(wantFields) {
		t.Fatalf("MergeBranch conflicts = %+v, want the person's and the family's", merge.Conflicts)
	}
	for _, conflict := range merge.Conflicts {
		want, ok := wantFields[conflict.StreamID]
		if !ok || conflict.Kind != query.ConflictEditEdit || !reflect.DeepEqual(conflict.Fields, want) {
			t.Fatalf("conflict = %+v, want edit_edit on %v", conflict, want)
		}
	}

	merged, err := h.MergeBranch(ctx, command.MergeBranchInput{
		BranchID: branch.ID,
		Resolutions: map[uuid.UUID]command.MergeResolution{
			person.ID: command.ResolveBranch,
			family.ID: command.ResolveMain,
		},
	})
	if err != nil {
		t.Fatalf("MergeBranch with the conflict resolved: %v", err)
	}
	if merged.Branch.Status != domain.BranchStatusMerged {
		t.Fatalf("branch status after merge = %q, want merged", merged.Branch.Status)
	}

	// Main holds the resolved edits, and both sides' edits where they did not collide.
	mainPerson, err := readStore.GetPerson(ctx, domain.MainBranchID, person.ID)
	if err != nil || mainPerson == nil {
		t.Fatalf("main GetPerson after merge = %v (err %v)", mainPerson, err)
	}
	if mainPerson.Surname != "Branchfix" || mainPerson.Notes != "branch note" || mainPerson.BirthPlace != "Placeville" {
		t.Fatalf("main person after merge = surname %q notes %q birth place %q", mainPerson.Surname, mainPerson.Notes, mainPerson.BirthPlace)
	}
	mainFamily, err := readStore.GetFamily(ctx, domain.MainBranchID, family.ID)
	if err != nil || mainFamily == nil {
		t.Fatalf("main GetFamily after merge = %v (err %v)", mainFamily, err)
	}
	if mainFamily.MarriagePlace != "Mainville Again" {
		t.Fatalf("main family after merge = place %q, want main's resolved %q", mainFamily.MarriagePlace, "Mainville Again")
	}
	mainSource, err := readStore.GetSource(ctx, domain.MainBranchID, source.ID)
	if err != nil || mainSource == nil {
		t.Fatalf("main GetSource after merge = %v (err %v)", mainSource, err)
	}
	if mainSource.CallNumber != "B-1" || mainSource.Author != "Main Author" || mainSource.Notes != "main again" {
		t.Fatalf("main source after merge = call number %q author %q notes %q, want both sides' edits",
			mainSource.CallNumber, mainSource.Author, mainSource.Notes)
	}
	mainCited, err := readStore.GetSource(ctx, domain.MainBranchID, cited.ID)
	if err != nil || mainCited == nil {
		t.Fatalf("main GetSource (cited) after merge = %v (err %v)", mainCited, err)
	}
	if mainCited.CallNumber != "B-2" || mainCited.Author != "Main Author" || mainCited.CitationCount != 1 {
		t.Fatalf("main cited source after merge = call number %q author %q citations %d, want %q %q 1",
			mainCited.CallNumber, mainCited.Author, mainCited.CitationCount, "B-2", "Main Author")
	}
}
