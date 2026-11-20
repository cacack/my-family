package gedcom

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// NoteReference is a record that points to a shared note: its type ("person",
// "name", "family", "event", "attribute", "source", "repository" or
// "association") and id.
type NoteReference struct {
	Type string
	ID   uuid.UUID
}

// noteReferencePageSize is the page size NoteReferences lists records with.
const noteReferencePageSize = 1000

// NoteReferences lists every record on branchID that points to note (#1376).
// A record points to a note by the note's stored xref, as one of the
// pointers in a Notes value (the form import keeps, see recordNotes), and an
// association also by the note's id in NoteIDs. A note without a stored xref
// can only be pointed to by id. The result is sorted by type, then id.
func NoteReferences(ctx context.Context, store repository.ReadModelStore, branchID domain.BranchID, note *repository.NoteReadModel) ([]NoteReference, error) {
	refs, err := NotesReferences(ctx, store, branchID, []repository.NoteReadModel{*note})
	return refs[note.ID], err
}

// NotesReferences is NoteReferences for each of notes, keyed by note id, in
// one walk of the records on branchID whatever the number of notes. A note
// nothing points to has no key.
func NotesReferences(ctx context.Context, store repository.ReadModelStore, branchID domain.BranchID, notes []repository.NoteReadModel) (map[uuid.UUID][]NoteReference, error) {
	byXref := make(map[string][]uuid.UUID)
	byID := make(map[uuid.UUID]bool, len(notes))
	for _, n := range notes {
		byID[n.ID] = true
		if n.GedcomXref != "" {
			byXref[n.GedcomXref] = append(byXref[n.GedcomXref], n.ID)
		}
	}
	refs := make(map[uuid.UUID][]NoteReference)
	// add records ref under each note its Notes values point to, once per
	// note. values alternate a Notes value and its separator.
	add := func(ref NoteReference, values ...string) {
		seen := map[uuid.UUID]bool{}
		for i := 0; i+1 < len(values); i += 2 {
			for _, id := range pointedNotes(values[i], values[i+1], byXref) {
				if !seen[id] {
					seen[id] = true
					refs[id] = append(refs[id], ref)
				}
			}
		}
	}

	associations, err := repository.ListAllOn(ctx, branchID, noteReferencePageSize, store.ListAssociations)
	if err != nil {
		return nil, fmt.Errorf("listing associations: %w", err)
	}
	for _, a := range associations {
		ref := NoteReference{Type: "association", ID: a.ID}
		pointed := map[uuid.UUID]bool{}
		for _, id := range pointedNotes(a.Notes, associationNoteJoin, byXref) {
			pointed[id] = true
		}
		for _, id := range a.NoteIDs {
			if byID[id] {
				pointed[id] = true
			}
		}
		for id := range pointed {
			refs[id] = append(refs[id], ref)
		}
	}

	if len(byXref) > 0 {
		if err := noteXrefReferences(ctx, store, branchID, add); err != nil {
			return nil, err
		}
	}

	for _, r := range refs {
		sort.Slice(r, func(i, j int) bool {
			if r[i].Type != r[j].Type {
				return r[i].Type < r[j].Type
			}
			return r[i].ID.String() < r[j].ID.String()
		})
	}
	return refs, nil
}

// noteXrefReferences passes each record on branchID other than associations
// to add, with its Notes values and their separator.
func noteXrefReferences(ctx context.Context, store repository.ReadModelStore, branchID domain.BranchID, add func(ref NoteReference, values ...string)) error {
	persons, err := repository.ListAllOn(ctx, branchID, noteReferencePageSize, store.ListPersons)
	if err != nil {
		return fmt.Errorf("listing persons: %w", err)
	}
	for _, p := range persons {
		add(NoteReference{Type: "person", ID: p.ID}, p.Notes, noteJoin, p.BirthNotes, noteJoin, p.DeathNotes, noteJoin)
		names, err := store.GetPersonNames(ctx, branchID, p.ID)
		if err != nil {
			return fmt.Errorf("getting names of person %s: %w", p.ID, err)
		}
		for _, n := range names {
			add(NoteReference{Type: "name", ID: n.ID}, n.Notes, noteJoin)
		}
	}

	families, err := repository.ListAllOn(ctx, branchID, noteReferencePageSize, store.ListFamilies)
	if err != nil {
		return fmt.Errorf("listing families: %w", err)
	}
	for _, f := range families {
		add(NoteReference{Type: "family", ID: f.ID}, f.Notes, noteJoin, f.MarriageNotes, noteJoin)
	}

	events, err := repository.ListAllOn(ctx, branchID, noteReferencePageSize, store.ListEvents)
	if err != nil {
		return fmt.Errorf("listing events: %w", err)
	}
	for _, e := range events {
		add(NoteReference{Type: "event", ID: e.ID}, e.Notes, noteJoin)
	}

	attributes, err := repository.ListAllOn(ctx, branchID, noteReferencePageSize, store.ListAttributes)
	if err != nil {
		return fmt.Errorf("listing attributes: %w", err)
	}
	for _, a := range attributes {
		add(NoteReference{Type: "attribute", ID: a.ID}, a.Notes, noteJoin)
	}

	sources, err := repository.ListAllOn(ctx, branchID, noteReferencePageSize, store.ListSources)
	if err != nil {
		return fmt.Errorf("listing sources: %w", err)
	}
	for _, s := range sources {
		add(NoteReference{Type: "source", ID: s.ID}, s.Notes, noteJoin)
	}

	repositories, err := repository.ListAllOn(ctx, branchID, noteReferencePageSize, store.ListRepositories)
	if err != nil {
		return fmt.Errorf("listing repositories: %w", err)
	}
	for _, r := range repositories {
		add(NoteReference{Type: "repository", ID: r.ID}, r.Notes, noteJoin)
	}

	return nil
}

// pointedNotes returns the notes of byXref that notes, or one of its
// sep-separated parts, points to: the parts renameXrefs and remapNotes treat
// as a pointer.
func pointedNotes(notes, sep string, byXref map[string][]uuid.UUID) []uuid.UUID {
	if notes == "" || len(byXref) == 0 {
		return nil
	}
	ids := slices.Clone(byXref[notes])
	for _, part := range strings.Split(notes, sep) {
		if part != notes {
			ids = append(ids, byXref[part]...)
		}
	}
	return ids
}
