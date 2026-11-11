package integration_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/cacack/my-family/internal/command"
)

// Place coordinates in life event API responses (issue #1290).
//
// Import keeps a life event's PLAC MAP coordinates in the read model (#1225);
// the life event export must return them, and omit them for an event that has
// none.

const lifeEventCoordinatesGedcom = `0 HEAD
1 GEDC
2 VERS 5.5.1
1 CHAR UTF-8
0 @I1@ INDI
1 NAME John /Mapped/
1 SEX M
1 BURI
2 DATE 1910
2 PLAC Mount Auburn Cemetery
3 MAP
4 LATI N42.3709
4 LONG W71.1449
1 RESI
2 DATE 1900
2 PLAC Cambridge
0 TRLR
`

var lifeEventCoordinateFields = []string{"place_latitude", "place_longitude"}

func TestAPI_PlaceCoordinatesInLifeEventResponses(t *testing.T) {
	for _, backend := range backends {
		t.Run(backend.name, func(t *testing.T) {
			ctx := context.Background()
			st := backend.setup(t)
			h := command.NewHandlerWithBranches(st.events, st.read, st.branches, st.snapshots)
			if _, err := h.ImportGedcom(ctx, command.ImportGedcomInput{Filename: "t.ged", Reader: strings.NewReader(lifeEventCoordinatesGedcom)}); err != nil {
				t.Fatalf("import: %v", err)
			}
			server := newServer(t, st)

			events := map[string]map[string]any{}
			for _, item := range jsonArray(t, mustDo(t, server, http.MethodGet, "/api/v1/export/events", "", http.StatusOK), "events") {
				m := item.(map[string]any)
				events[m["fact_type"].(string)] = m
			}

			assertCoordinates(t, "burial export", events["person_burial"], lifeEventCoordinateFields,
				map[string]string{"place_latitude": "N42.3709", "place_longitude": "W71.1449"})
			assertCoordinates(t, "residence export without coordinates", events["person_residence"], lifeEventCoordinateFields, nil)
		})
	}
}
