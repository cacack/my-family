package integration_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/api"
	"github.com/cacack/my-family/internal/domain"
)

// TestFamilyUpdate_PartnersTypeAndDate covers issue #848 over the real HTTP
// API on every backend: updating a family's partners, relationship type and
// marriage date is visible on the very next read (the synchronous projection),
// survives a branch merge (the decoded-replay projection) with the same values,
// and a full replay of the log into a fresh read model yields the same row.
func TestFamilyUpdate_PartnersTypeAndDate(t *testing.T) {
	for _, backend := range backends {
		t.Run(backend.name, func(t *testing.T) {
			st := backend.setup(t)
			runFamilyUpdate(t, newServer(t, st), st, backend)
		})
	}
}

func runFamilyUpdate(t *testing.T, server *api.Server, st stores, backend backend) {
	t.Helper()
	ctx := context.Background()

	alpha := createPerson(t, server, "Alpha", "Placeholder")
	bravo := createPerson(t, server, "Bravo", "Sample")
	charlie := createPerson(t, server, "Charlie", "Example")
	familyID := createFamily(t, server, alpha, bravo)
	familyPath := "/api/v1/families/" + familyID

	// --- Live: a mainline update is reflected on the next read. ---
	mustDo(t, server, http.MethodPut, familyPath,
		fmt.Sprintf(`{"partner1_id":%q,"partner2_id":%q,"relationship_type":"partnership","marriage_date":"ABT 1880","marriage_place":"New Hall","version":%d}`,
			charlie, alpha, entityVersion(t, server, familyPath, "")),
		http.StatusOK)
	assertFamilyAPI(t, getEntity(t, server, familyPath, ""), charlie, "Charlie", alpha, "partnership", "ABT 1880", "New Hall")

	// --- Merge: the same kind of update made on a branch reaches main through
	// the decoded replay, and lands identically. ---
	branchID := createBranch(t, server, "family-fix")
	mustDo(t, server, http.MethodPut, scoped(familyPath, branchID),
		fmt.Sprintf(`{"partner2_id":%q,"relationship_type":"marriage","marriage_date":"2 FEB 1890","version":%d}`,
			bravo, entityVersion(t, server, familyPath, branchID)),
		http.StatusOK)
	assertFamilyAPI(t, getEntity(t, server, familyPath, branchID), charlie, "Charlie", bravo, "marriage", "2 FEB 1890", "New Hall")
	assertFamilyAPI(t, getEntity(t, server, familyPath, ""), charlie, "Charlie", alpha, "partnership", "ABT 1880", "New Hall")

	mustDo(t, server, http.MethodPost, mergePath(branchID), mergeBody("partner correction"), http.StatusOK)
	assertFamilyAPI(t, getEntity(t, server, familyPath, ""), charlie, "Charlie", bravo, "marriage", "2 FEB 1890", "New Hall")

	// --- Clearing the marriage date live clears it in the read model. ---
	mustDo(t, server, http.MethodPut, familyPath,
		fmt.Sprintf(`{"marriage_date":"","version":%d}`, entityVersion(t, server, familyPath, "")),
		http.StatusOK)
	cleared := getEntity(t, server, familyPath, "")
	if md, ok := cleared["marriage_date"]; ok && md != nil {
		t.Errorf("marriage_date after clearing = %v, want absent", md)
	}

	// --- Replay: rebuilding the read model from the log gives the same row. ---
	fresh := backend.setup(t).read
	replayLog(t, ctx, st.events, fresh)
	id := uuid.MustParse(familyID)
	live, err := st.read.GetFamily(ctx, domain.MainBranchID, id)
	if err != nil {
		t.Fatalf("read live family: %v", err)
	}
	replayed, err := fresh.GetFamily(ctx, domain.MainBranchID, id)
	if err != nil {
		t.Fatalf("read replayed family: %v", err)
	}
	if a, b := canonicalRowJSON(t, live), canonicalRowJSON(t, replayed); a != b {
		t.Errorf("live family row differs from replay\n live:   %s\n replay: %s", a, b)
	}
}

// assertFamilyAPI checks the fields issue #848 lost on a family GET response.
func assertFamilyAPI(t *testing.T, family map[string]any, partner1, partner1Given, partner2, relType, marriageDate, marriagePlace string) {
	t.Helper()
	if got := family["partner1_id"]; got != partner1 {
		t.Errorf("partner1_id = %v, want %s", got, partner1)
	}
	if p1, _ := family["partner1"].(map[string]any); p1 == nil || p1["given_name"] != partner1Given {
		t.Errorf("partner1 = %v, want given_name %s", family["partner1"], partner1Given)
	}
	if got := family["partner2_id"]; got != partner2 {
		t.Errorf("partner2_id = %v, want %s", got, partner2)
	}
	if got := family["relationship_type"]; got != relType {
		t.Errorf("relationship_type = %v, want %s", got, relType)
	}
	md, _ := family["marriage_date"].(map[string]any)
	if md == nil || md["raw"] != marriageDate {
		t.Errorf("marriage_date = %v, want raw %q", family["marriage_date"], marriageDate)
	}
	if got := family["marriage_place"]; got != marriagePlace {
		t.Errorf("marriage_place = %v, want %s", got, marriagePlace)
	}
}
