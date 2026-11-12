package integration_test

import (
	"context"
	"encoding/csv"
	"net/http"
	"strings"
	"testing"

	"github.com/cacack/my-family/internal/command"
)

// A life event's GEDCOM TYPE in API responses (issue #1282).
//
// Import stores an event's TYPE in LifeEventCreated (#1276); the life event
// export must return it as type_detail, in JSON and CSV, after the read model
// is rebuilt from the log, and omit it for an event that has none.

const lifeEventTypeDetailGedcom = `0 HEAD
1 GEDC
2 VERS 5.5.1
1 CHAR UTF-8
0 @I1@ INDI
1 NAME John /Mover/
1 SEX M
1 EVEN Moved west
2 TYPE Migration
2 DATE 1849
1 RESI
2 DATE 1900
2 PLAC Cambridge
0 TRLR
`

func TestAPI_LifeEventTypeDetailInExport(t *testing.T) {
	for _, backend := range backends {
		t.Run(backend.name, func(t *testing.T) {
			ctx := context.Background()
			st := backend.setup(t)
			h := command.NewHandlerWithBranches(st.events, st.read, st.branches, st.snapshots)
			if _, err := h.ImportGedcom(ctx, command.ImportGedcomInput{Filename: "t.ged", Reader: strings.NewReader(lifeEventTypeDetailGedcom)}); err != nil {
				t.Fatalf("import: %v", err)
			}

			// Serve from a read model rebuilt from the log, as `myfamily rebuild` does.
			rebuilt := backend.setup(t)
			replayLog(t, ctx, st.events, rebuilt.read)
			server := newServer(t, rebuilt)

			events := map[string]map[string]any{}
			for _, item := range jsonArray(t, mustDo(t, server, http.MethodGet, "/api/v1/export/events", "", http.StatusOK), "events") {
				m := item.(map[string]any)
				events[m["fact_type"].(string)] = m
			}
			if got := events["person_generic_event"]["type_detail"]; got != "Migration" {
				t.Errorf("JSON type_detail for EVEN = %v, want Migration", got)
			}
			if _, ok := events["person_residence"]["type_detail"]; ok {
				t.Errorf("JSON type_detail present for RESI without TYPE: %v", events["person_residence"])
			}

			rec := do(t, server, http.MethodGet, "/api/v1/export/events?format=csv&fields=fact_type,type_detail", "")
			if rec.Code != http.StatusOK {
				t.Fatalf("CSV export status = %d: %s", rec.Code, rec.Body.String())
			}
			rows, err := csv.NewReader(rec.Body).ReadAll()
			if err != nil {
				t.Fatalf("parse CSV: %v", err)
			}
			got := map[string]string{}
			for _, row := range rows[1:] {
				got[row[0]] = row[1]
			}
			if got["person_generic_event"] != "Migration" || got["person_residence"] != "" {
				t.Errorf("CSV type_detail by fact_type = %v, want person_generic_event=Migration, person_residence empty", got)
			}
		})
	}
}
