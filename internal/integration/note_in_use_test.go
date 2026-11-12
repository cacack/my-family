package integration_test

import (
	"context"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/api"
	"github.com/cacack/my-family/internal/command"
)

// Deleting a shared note that records still point to (issue #1376).
//
// DeleteNote refuses with 409 NOTE_IN_USE and lists every record that points
// to the note, appending no event; a note nothing points to still deletes.

// noteInUseGedcom points @N1@ from every kind of record that can hold a NOTE
// pointer; @N2@ is pointed to by nothing.
const noteInUseGedcom = `0 HEAD
1 GEDC
2 VERS 5.5.1
1 CHAR UTF-8
0 @N1@ NOTE Shared research note.
0 @N2@ NOTE Nobody points here.
0 @I1@ INDI
1 NAME John /Doe/
2 NOTE @N1@
1 SEX M
1 NOTE @N1@
1 BURI
2 PLAC Salem
2 NOTE @N1@
1 OCCU Farmer
2 NOTE @N1@
1 ASSO @I2@
2 RELA Godfather
2 NOTE @N1@
0 @I2@ INDI
1 NAME Mary /Roe/
1 SEX F
0 @F1@ FAM
1 HUSB @I1@
1 WIFE @I2@
1 NOTE @N1@
0 @R1@ REPO
1 NAME County Archive
1 NOTE @N1@
0 @S1@ SOUR
1 TITL Parish Register
1 REPO @R1@
1 NOTE @N1@
0 TRLR
`

func TestDeleteNote_RefusedWhileReferenced(t *testing.T) {
	for _, backend := range backends {
		t.Run(backend.name, func(t *testing.T) {
			ctx := context.Background()
			st := backend.setup(t)
			h := command.NewHandlerWithBranches(st.events, st.read, st.branches, st.snapshots)
			if _, err := h.ImportGedcom(ctx, command.ImportGedcomInput{Filename: "t.ged", Reader: strings.NewReader(noteInUseGedcom)}); err != nil {
				t.Fatalf("import: %v", err)
			}
			server := newServer(t, st)
			shared := noteIDByXref(t, server, "@N1@")
			unused := noteIDByXref(t, server, "@N2@")

			rec := do(t, server, http.MethodDelete, "/api/v1/notes/"+shared+"?version=1", "")
			if rec.Code != http.StatusConflict {
				t.Fatalf("delete referenced note: status = %d, want 409. Body: %s", rec.Code, rec.Body.String())
			}
			body := decodeJSON(t, rec)
			if body["code"] != api.CodeNoteInUse {
				t.Errorf("code = %v, want %s", body["code"], api.CodeNoteInUse)
			}
			details, _ := body["details"].(map[string]any)
			refs, _ := details["references"].([]any)
			var gotTypes []string
			for _, r := range refs {
				ref, _ := r.(map[string]any)
				id, _ := ref["id"].(string)
				if _, err := uuid.Parse(id); err != nil {
					t.Errorf("reference %v: id is not a uuid", ref)
				}
				gotTypes = append(gotTypes, ref["type"].(string))
			}
			wantTypes := []string{"association", "attribute", "event", "family", "name", "person", "repository", "source"}
			if !slices.Equal(gotTypes, wantTypes) {
				t.Errorf("reference types = %v, want %v", gotTypes, wantTypes)
			}

			// Nothing changed: the note is still there and its stream has
			// only its creation event.
			mustDo(t, server, http.MethodGet, "/api/v1/notes/"+shared, "", http.StatusOK)
			stream, err := st.events.ReadStream(ctx, uuid.MustParse(shared))
			if err != nil {
				t.Fatalf("read note stream: %v", err)
			}
			if len(stream) != 1 {
				t.Errorf("note stream has %d events after the refusal, want 1", len(stream))
			}

			// A note nothing points to still deletes.
			mustDo(t, server, http.MethodDelete, "/api/v1/notes/"+unused+"?version=1", "", http.StatusNoContent)
			mustDo(t, server, http.MethodGet, "/api/v1/notes/"+unused, "", http.StatusNotFound)
		})
	}
}

// noteIDByXref returns the id of the note whose gedcom_xref is xref.
func noteIDByXref(t *testing.T, server *api.Server, xref string) string {
	t.Helper()
	resp := mustDo(t, server, http.MethodGet, "/api/v1/notes?limit=100", "", http.StatusOK)
	items, _ := resp["notes"].([]any)
	for _, item := range items {
		n, _ := item.(map[string]any)
		if n["gedcom_xref"] == xref {
			return mustString(t, n, "id")
		}
	}
	t.Fatalf("no note with xref %s in %v", xref, resp)
	return ""
}
