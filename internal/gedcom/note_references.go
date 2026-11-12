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
	var refs []NoteReference
	xref := note.GedcomXref
	points := func(notes, sep string) bool {
		return xref != "" && hasXref(notes, sep, xref)
	}

	associations, err := repository.ListAllOn(ctx, branchID, noteReferencePageSize, store.ListAssociations)
	if err != nil {
		return nil, fmt.Errorf("listing associations: %w", err)
	}
	for _, a := range associations {
		if points(a.Notes, associationNoteJoin) || slices.Contains(a.NoteIDs, note.ID) {
			refs = append(refs, NoteReference{Type: "association", ID: a.ID})
		}
	}

	if xref != "" {
		xrefRefs, err := noteXrefReferences(ctx, store, branchID, points)
		if err != nil {
			return nil, err
		}
		refs = append(refs, xrefRefs...)
	}

	sort.Slice(refs, func(i, j int) bool {
		if refs[i].Type != refs[j].Type {
			return refs[i].Type < refs[j].Type
		}
		return refs[i].ID.String() < refs[j].ID.String()
	})
	return refs, nil
}

// noteXrefReferences lists the records on branchID other than associations
// whose Notes values hold a pointer for which points reports true.
func noteXrefReferences(ctx context.Context, store repository.ReadModelStore, branchID domain.BranchID, points func(notes, sep string) bool) ([]NoteReference, error) {
	var refs []NoteReference

	persons, err := repository.ListAllOn(ctx, branchID, noteReferencePageSize, store.ListPersons)
	if err != nil {
		return nil, fmt.Errorf("listing persons: %w", err)
	}
	for _, p := range persons {
		if points(p.Notes, noteJoin) || points(p.BirthNotes, noteJoin) || points(p.DeathNotes, noteJoin) {
			refs = append(refs, NoteReference{Type: "person", ID: p.ID})
		}
		names, err := store.GetPersonNames(ctx, branchID, p.ID)
		if err != nil {
			return nil, fmt.Errorf("getting names of person %s: %w", p.ID, err)
		}
		for _, n := range names {
			if points(n.Notes, noteJoin) {
				refs = append(refs, NoteReference{Type: "name", ID: n.ID})
			}
		}
	}

	families, err := repository.ListAllOn(ctx, branchID, noteReferencePageSize, store.ListFamilies)
	if err != nil {
		return nil, fmt.Errorf("listing families: %w", err)
	}
	for _, f := range families {
		if points(f.Notes, noteJoin) || points(f.MarriageNotes, noteJoin) {
			refs = append(refs, NoteReference{Type: "family", ID: f.ID})
		}
	}

	events, err := repository.ListAllOn(ctx, branchID, noteReferencePageSize, store.ListEvents)
	if err != nil {
		return nil, fmt.Errorf("listing events: %w", err)
	}
	for _, e := range events {
		if points(e.Notes, noteJoin) {
			refs = append(refs, NoteReference{Type: "event", ID: e.ID})
		}
	}

	attributes, err := repository.ListAllOn(ctx, branchID, noteReferencePageSize, store.ListAttributes)
	if err != nil {
		return nil, fmt.Errorf("listing attributes: %w", err)
	}
	for _, a := range attributes {
		if points(a.Notes, noteJoin) {
			refs = append(refs, NoteReference{Type: "attribute", ID: a.ID})
		}
	}

	sources, err := repository.ListAllOn(ctx, branchID, noteReferencePageSize, store.ListSources)
	if err != nil {
		return nil, fmt.Errorf("listing sources: %w", err)
	}
	for _, s := range sources {
		if points(s.Notes, noteJoin) {
			refs = append(refs, NoteReference{Type: "source", ID: s.ID})
		}
	}

	repositories, err := repository.ListAllOn(ctx, branchID, noteReferencePageSize, store.ListRepositories)
	if err != nil {
		return nil, fmt.Errorf("listing repositories: %w", err)
	}
	for _, r := range repositories {
		if points(r.Notes, noteJoin) {
			refs = append(refs, NoteReference{Type: "repository", ID: r.ID})
		}
	}

	return refs, nil
}

// hasXref reports whether notes, or one of its sep-separated parts, is xref:
// the parts renameXrefs and remapNotes treat as a pointer.
func hasXref(notes, sep, xref string) bool {
	if notes == xref {
		return true
	}
	for _, part := range strings.Split(notes, sep) {
		if part == xref {
			return true
		}
	}
	return false
}
