package integration_test

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/api"
	"github.com/cacack/my-family/internal/command"
	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/gedcom"
	"github.com/cacack/my-family/internal/repository"
)

// Privacy marks and the living rule (ADR-008, #1412), on every backend.

// privacyGedcom carries a restriction notice on a person, a family, a source,
// a note and a media object (the last three as _RESN: GEDCOM 5.5.1 defines no
// RESN on them), and people who exercise each branch of the living rule.
const privacyGedcom = `0 HEAD
1 GEDC
2 VERS 5.5.1
1 CHAR UTF-8
0 @N1@ NOTE A private note
1 _RESN confidential
0 @S1@ SOUR
1 TITL Private register
1 _RESN privacy
0 @M1@ OBJE
1 FILE photos/baptism.jpg
2 FORM jpg
3 TYPE photo
2 TITL Private portrait
1 _RESN confidential
0 @I1@ INDI
1 RESN confidential
1 NAME Alive /Private/
1 SEX F
1 BIRT
2 DATE 1990
0 @I2@ INDI
1 RESN locked
1 NAME Buried /Undated/
1 SEX M
1 BURI
2 PLAC Old churchyard
0 @I3@ INDI
1 NAME Cremated /Undated/
1 SEX M
1 CREM Y
0 @I4@ INDI
1 NAME Baptized /Long/
1 SEX F
1 OBJE @M1@
1 BAPM
2 DATE 1850
0 @I5@ INDI
1 NAME Nodates /Person/
1 SEX U
0 @I6@ INDI
1 NAME Born /Range/
1 SEX U
1 BIRT
2 DATE BET 1800 AND 1820
0 @I7@ INDI
1 NAME Recent /Baptism/
1 SEX U
1 BAPM
2 DATE 2001
0 @I8@ INDI
1 NAME Died /Undated/
1 SEX U
1 BIRT
2 DATE 1990
1 DEAT Y
0 @I9@ INDI
1 NAME Cited /Undated/
1 SEX U
1 BIRT
2 DATE 1990
1 DEAT Y
2 SOUR @S1@
0 @F1@ FAM
1 RESN confidential
1 HUSB @I2@
1 WIFE @I1@
0 TRLR
`

// privacyPeople is what the living rule says of each person in privacyGedcom,
// and the restriction each carries.
var privacyPeople = []struct {
	given, restriction string
	living             bool
}{
	{"Alive", "confidential", true}, // born 1990
	{"Buried", "locked", false},     // a burial with no date is death evidence
	{"Cremated", "", false},         // so is a cremation
	{"Baptized", "", false},         // no birth: baptism 1850 is over 110 years ago
	{"Nodates", "", true},           // nothing to go on: fail safe
	{"Born", "", false},             // BET 1800 AND 1820 compares 1820
	{"Recent", "", true},            // baptism 2001 is recent
	{"Died", "", false},             // a bare DEAT Y is death evidence (#1436)
	{"Cited", "", false},            // so is a cited one
}

// importPrivacyTree imports privacyGedcom into st and returns the person IDs
// by given name.
func importPrivacyTree(t *testing.T, ctx context.Context, st stores) map[string]uuid.UUID {
	t.Helper()
	h := command.NewHandlerWithBranches(st.events, st.read, st.branches, st.snapshots)
	if _, err := h.ImportGedcom(ctx, command.ImportGedcomInput{Filename: "p.ged", Reader: strings.NewReader(privacyGedcom)}); err != nil {
		t.Fatalf("import: %v", err)
	}
	return personIDsByGivenName(t, ctx, st.read)
}

func personIDsByGivenName(t *testing.T, ctx context.Context, rs repository.ReadModelStore) map[string]uuid.UUID {
	t.Helper()
	people, _, err := rs.ListPersons(ctx, repository.ListOptions{Limit: 100})
	if err != nil {
		t.Fatalf("list persons: %v", err)
	}
	ids := make(map[string]uuid.UUID, len(people))
	for _, p := range people {
		ids[p.GivenName] = p.ID
	}
	return ids
}

// recordRestrictions reads the restriction of every family, source and note
// in rs, and of the media item of the person named Baptized, keyed by kind.
func recordRestrictions(t *testing.T, ctx context.Context, rs repository.ReadModelStore) map[string]domain.Restriction {
	t.Helper()
	out := map[string]domain.Restriction{}
	families, _, err := rs.ListFamilies(ctx, repository.ListOptions{Limit: 10})
	if err != nil || len(families) != 1 {
		t.Fatalf("list families = %d, %v; want 1", len(families), err)
	}
	out["family"] = families[0].Restriction
	sources, _, err := rs.ListSources(ctx, repository.ListOptions{Limit: 10})
	if err != nil || len(sources) != 1 {
		t.Fatalf("list sources = %d, %v; want 1", len(sources), err)
	}
	out["source"] = sources[0].Restriction
	notes, _, err := rs.ListNotes(ctx, repository.ListOptions{Limit: 10})
	if err != nil || len(notes) != 1 {
		t.Fatalf("list notes = %d, %v; want 1", len(notes), err)
	}
	out["note"] = notes[0].Restriction
	media, _, err := rs.ListMediaForEntity(ctx, "person", personIDsByGivenName(t, ctx, rs)["Baptized"], repository.ListOptions{Limit: 10})
	if err != nil || len(media) != 1 {
		t.Fatalf("list media = %d, %v; want 1", len(media), err)
	}
	if media[0].Title != "Private portrait" || len(media[0].Files) != 1 || media[0].Files[0].Path != "photos/baptism.jpg" {
		t.Errorf("media = %q with files %+v, want Private portrait with photos/baptism.jpg", media[0].Title, media[0].Files)
	}
	out["media"] = media[0].Restriction
	return out
}

func assertPrivacyTree(t *testing.T, ctx context.Context, rs repository.ReadModelStore) {
	t.Helper()
	ids := personIDsByGivenName(t, ctx, rs)
	cutoff := domain.LivingCutoff(time.Now(), domain.DefaultLivingThresholdYears)
	for _, want := range privacyPeople {
		p, err := rs.GetPerson(ctx, domain.MainBranchID, ids[want.given])
		if err != nil || p == nil {
			t.Fatalf("get %s: %v, %v", want.given, p, err)
		}
		if string(p.Restriction) != want.restriction {
			t.Errorf("%s restriction = %q, want %q", want.given, p.Restriction, want.restriction)
		}
		if got := p.LivingFacts().IsLiving(cutoff); got != want.living {
			t.Errorf("%s living = %v, want %v (facts %+v)", want.given, got, want.living, p.LivingFacts())
		}
	}
	got := recordRestrictions(t, ctx, rs)
	want := map[string]domain.Restriction{"family": "confidential", "source": "privacy", "note": "confidential", "media": "confidential"}
	for kind, r := range want {
		if got[kind] != r {
			t.Errorf("%s restriction = %q, want %q", kind, got[kind], r)
		}
	}
}

// TestPrivacy_GedcomRestrictionAndLivingRuleSurviveReplayExportAndReimport
// imports RESN on a person, family, source, note and media item, replays the log into an
// empty read model, exports it and imports the export again. Each step keeps
// the restrictions and the living rule's answers.
func TestPrivacy_GedcomRestrictionAndLivingRuleSurviveReplayExportAndReimport(t *testing.T) {
	for _, backend := range backends {
		t.Run(backend.name, func(t *testing.T) {
			ctx := context.Background()
			st := backend.setup(t)
			importPrivacyTree(t, ctx, st)
			assertPrivacyTree(t, ctx, st.read)

			// The living rule's inputs and the restrictions come from the log.
			fresh := backend.setup(t)
			replayLog(t, ctx, st.events, fresh.read)
			assertPrivacyTree(t, ctx, fresh.read)

			var buf bytes.Buffer
			if _, err := gedcom.NewExporter(fresh.read).Export(ctx, &buf); err != nil {
				t.Fatalf("export: %v", err)
			}
			out := buf.String()
			for _, want := range []struct{ record, line string }{
				{"Alive /Private/", "1 RESN confidential"},
				{"Buried /Undated/", "1 RESN locked"},
				{"0 @F1@ FAM", "1 RESN confidential"},
				{"Private register", "1 _RESN privacy"},
				{"A private note", "1 _RESN confidential"},
				{"Private portrait", "1 _RESN confidential"},
				{"Baptized /Long/", "1 OBJE @O1@"},
				{"Died /Undated/", "1 DEAT Y"},
				{"Cited /Undated/", "1 DEAT Y"},
			} {
				if lines := recordLines(out, want.record); !slices.Contains(lines, want.line) {
					t.Errorf("record %q exported as %q, want a %q line; export:\n%s", want.record, lines, want.line, out)
				}
			}

			again := backend.setup(t)
			h := command.NewHandlerWithBranches(again.events, again.read, again.branches, again.snapshots)
			if _, err := h.ImportGedcom(ctx, command.ImportGedcomInput{Filename: "again.ged", Reader: strings.NewReader(out)}); err != nil {
				t.Fatalf("re-import: %v", err)
			}
			assertPrivacyTree(t, ctx, again.read)
		})
	}
}

// sharedMediaGedcom links one OBJE record from two people, so the import
// makes one media item for each (#1435).
const sharedMediaGedcom = `0 HEAD
1 GEDC
2 VERS 5.5.1
0 @M1@ OBJE
1 FILE photos/reunion.jpg
2 FORM jpg
2 TITL Reunion
0 @I1@ INDI
1 NAME Ada /Shared/
1 OBJE @M1@
0 @I2@ INDI
1 NAME Bea /Shared/
1 OBJE @M1@
0 TRLR
`

// sharedMedia returns the one media item of each of the people named Ada and
// Bea, failing unless each has exactly one.
func sharedMedia(t *testing.T, ctx context.Context, rs repository.ReadModelStore) []repository.MediaReadModel {
	t.Helper()
	ids := personIDsByGivenName(t, ctx, rs)
	var out []repository.MediaReadModel
	for _, given := range []string{"Ada", "Bea"} {
		media, _, err := rs.ListMediaForEntity(ctx, "person", ids[given], repository.ListOptions{Limit: 10})
		if err != nil || len(media) != 1 || media[0].Title != "Reunion" {
			t.Fatalf("%s media = %+v, %v; want the Reunion photo", given, media, err)
		}
		out = append(out, media[0])
	}
	return out
}

// setMediaPrivate marks m private or not through the command handler.
func setMediaPrivate(t *testing.T, ctx context.Context, st stores, m repository.MediaReadModel, private bool) {
	t.Helper()
	h := command.NewHandlerWithBranches(st.events, st.read, st.branches, st.snapshots)
	if _, err := h.UpdateMedia(ctx, command.UpdateMediaInput{ID: m.ID, Private: &private, Version: m.Version}); err != nil {
		t.Fatalf("update media: %v", err)
	}
}

// assertSharedMediaPrivate checks that both copies of the shared photo are
// marked private, or both are not.
func assertSharedMediaPrivate(t *testing.T, ctx context.Context, rs repository.ReadModelStore, private bool) {
	t.Helper()
	for i, m := range sharedMedia(t, ctx, rs) {
		if m.Restriction.IsPrivate() != private {
			t.Errorf("copy %d restriction = %q, want private %v", i, m.Restriction, private)
		}
	}
}

// TestPrivacy_SharedMediaKeepsOnePrivateMark imports an OBJE record linked
// from two people and marks the copy of one private: both copies are private,
// after a replay of the log too, and an export writes them as one record that
// a re-import makes into copies that again change together (#1478).
func TestPrivacy_SharedMediaKeepsOnePrivateMark(t *testing.T) {
	for _, backend := range backends {
		t.Run(backend.name, func(t *testing.T) {
			ctx := context.Background()
			st := backend.setup(t)
			h := command.NewHandlerWithBranches(st.events, st.read, st.branches, st.snapshots)
			if _, err := h.ImportGedcom(ctx, command.ImportGedcomInput{Filename: "s.ged", Reader: strings.NewReader(sharedMediaGedcom)}); err != nil {
				t.Fatalf("import: %v", err)
			}
			assertSharedMediaPrivate(t, ctx, st.read, false)
			setMediaPrivate(t, ctx, st, sharedMedia(t, ctx, st.read)[0], true)
			assertSharedMediaPrivate(t, ctx, st.read, true)

			fresh := backend.setup(t)
			replayLog(t, ctx, st.events, fresh.read)
			assertSharedMediaPrivate(t, ctx, fresh.read, true)

			var buf bytes.Buffer
			if _, err := gedcom.NewExporter(fresh.read).Export(ctx, &buf); err != nil {
				t.Fatalf("export: %v", err)
			}
			out := buf.String()
			if n := strings.Count(out, " OBJE\n"); n != 1 {
				t.Errorf("export has %d OBJE records, want 1:\n%s", n, out)
			}
			for _, want := range []struct{ record, line string }{
				{"Reunion", "1 _RESN confidential"},
				{"Ada /Shared/", "1 OBJE @O1@"},
				{"Bea /Shared/", "1 OBJE @O1@"},
			} {
				if lines := recordLines(out, want.record); !slices.Contains(lines, want.line) {
					t.Errorf("record %q exported as %q, want a %q line; export:\n%s", want.record, lines, want.line, out)
				}
			}

			again := backend.setup(t)
			h = command.NewHandlerWithBranches(again.events, again.read, again.branches, again.snapshots)
			if _, err := h.ImportGedcom(ctx, command.ImportGedcomInput{Filename: "again.ged", Reader: strings.NewReader(out)}); err != nil {
				t.Fatalf("re-import: %v", err)
			}
			assertSharedMediaPrivate(t, ctx, again.read, true)
			setMediaPrivate(t, ctx, again, sharedMedia(t, ctx, again.read)[1], false)
			assertSharedMediaPrivate(t, ctx, again.read, false)
		})
	}
}

// TestPrivacy_StoresListMediaGroups saves media in groups on main and on a
// branch and lists each group as main and the branch see it, on every
// backend (#1478).
func TestPrivacy_StoresListMediaGroups(t *testing.T) {
	for _, backend := range backends {
		t.Run(backend.name, func(t *testing.T) {
			ctx := context.Background()
			rs := backend.setup(t).read
			main, branch := domain.MainBranchID, domain.BranchID(uuid.New())
			group, other := uuid.New(), uuid.New()
			now := time.Now().UTC().Truncate(time.Second)
			save := func(branchID domain.BranchID, groupID *uuid.UUID) uuid.UUID {
				t.Helper()
				m := &repository.MediaReadModel{ID: uuid.New(), EntityType: "person", EntityID: uuid.New(), Title: "M", MimeType: "image/png",
					MediaType: domain.MediaPhoto, Filename: "m.png", FileSize: 1, FileData: []byte{1}, GroupID: groupID, Version: 1, CreatedAt: now, UpdatedAt: now}
				if err := rs.SaveMedia(ctx, branchID, m); err != nil {
					t.Fatalf("save media: %v", err)
				}
				return m.ID
			}
			listed := func(branchID domain.BranchID, groupID uuid.UUID) []uuid.UUID {
				t.Helper()
				items, err := rs.ListMediaInGroup(ctx, branchID, groupID)
				if err != nil {
					t.Fatalf("list media group: %v", err)
				}
				var ids []uuid.UUID
				for _, m := range items {
					if m.GroupID == nil || *m.GroupID != groupID {
						t.Errorf("listed media %s has group %v, want %s", m.ID, m.GroupID, groupID)
					}
					ids = append(ids, m.ID)
				}
				slices.SortFunc(ids, func(a, b uuid.UUID) int { return strings.Compare(a.String(), b.String()) })
				return ids
			}
			sorted := func(ids ...uuid.UUID) []uuid.UUID {
				slices.SortFunc(ids, func(a, b uuid.UUID) int { return strings.Compare(a.String(), b.String()) })
				return ids
			}

			a, b := save(main, &group), save(main, &group)
			c := save(main, &other)
			ungrouped := save(main, nil)
			if m, err := rs.GetMedia(ctx, main, ungrouped); err != nil || m == nil || m.GroupID != nil {
				t.Errorf("ungrouped media = %+v, %v; want no group", m, err)
			}
			if err := rs.DeleteMedia(ctx, branch, b); err != nil {
				t.Fatalf("delete media on branch: %v", err)
			}
			e := save(branch, &group)

			for _, tc := range []struct {
				name     string
				branchID domain.BranchID
				groupID  uuid.UUID
				want     []uuid.UUID
			}{
				{"main", main, group, sorted(a, b)},
				{"main, other group", main, other, []uuid.UUID{c}},
				{"branch", branch, group, sorted(a, e)},
				{"no such group", main, uuid.New(), nil},
			} {
				if got := listed(tc.branchID, tc.groupID); !slices.Equal(got, tc.want) {
					t.Errorf("%s: group lists %v, want %v", tc.name, got, tc.want)
				}
			}
		})
	}
}

// recordLines returns the lines of the level-0 record of out that contains a
// line containing marker.
func recordLines(out, marker string) []string {
	var record []string
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "0 ") {
			if slices.ContainsFunc(record, func(l string) bool { return strings.Contains(l, marker) }) {
				return record
			}
			record = nil
		}
		record = append(record, strings.TrimRight(line, "\r"))
	}
	return nil
}

// TestPrivacy_APIMarksPrivateAndOverridesLiving drives the API: the person
// response carries the rule's verdict, an editor overrides it and marks
// records private, and both are kept on every backend.
func TestPrivacy_APIMarksPrivateAndOverridesLiving(t *testing.T) {
	for _, backend := range backends {
		t.Run(backend.name, func(t *testing.T) {
			ctx := context.Background()
			st := backend.setup(t)
			ids := importPrivacyTree(t, ctx, st)
			server := newServer(t, st)

			for _, want := range privacyPeople {
				resp := getEntity(t, server, "/api/v1/persons/"+ids[want.given].String(), "")
				if resp["living"] != want.living {
					t.Errorf("%s living = %v, want %v", want.given, resp["living"], want.living)
				}
				if got := resp["private"] == true; got != (want.restriction == "confidential") {
					t.Errorf("%s private = %v, want %v", want.given, resp["private"], !got)
				}
			}

			baptized := "/api/v1/persons/" + ids["Baptized"].String()
			for _, step := range []struct {
				body     string
				living   bool
				override string
			}{
				{`"living_override":"living"`, true, "living"},
				{`"living_override":"deceased"`, false, "deceased"},
				{`"living_override":null`, false, "auto"},
			} {
				body := fmt.Sprintf(`{%s,"version":%d}`, step.body, entityVersion(t, server, baptized, ""))
				mustDo(t, server, http.MethodPut, baptized, body, http.StatusOK)
				resp := getEntity(t, server, baptized, "")
				if resp["living"] != step.living || resp["living_override"] != step.override {
					t.Errorf("after %s: living = %v, override = %v; want %v, %v",
						step.body, resp["living"], resp["living_override"], step.living, step.override)
				}
			}
			bad := fmt.Sprintf(`{"living_override":"undead","version":%d}`, entityVersion(t, server, baptized, ""))
			if rec := do(t, server, http.MethodPut, baptized, bad); rec.Code != http.StatusBadRequest {
				t.Errorf("unknown living_override: status = %d, want 400. Body: %s", rec.Code, rec.Body.String())
			}

			// Marking a locked person private keeps the lock.
			buried := "/api/v1/persons/" + ids["Buried"].String()
			setPrivate(t, server, buried, true)
			if p, _ := st.read.GetPerson(ctx, domain.MainBranchID, ids["Buried"]); p == nil || p.Restriction != "locked, confidential" {
				t.Errorf("restriction after marking private = %+v, want %q", p, "locked, confidential")
			}
			setPrivate(t, server, buried, false)
			if p, _ := st.read.GetPerson(ctx, domain.MainBranchID, ids["Buried"]); p == nil || p.Restriction != "locked" {
				t.Errorf("restriction after unmarking = %+v, want %q", p, "locked")
			}

			// The other record types take the flag the same way.
			families := mustDo(t, server, http.MethodGet, "/api/v1/families", "", http.StatusOK)
			familyPath := "/api/v1/families/" + jsonArray(t, families, "items")[0].(map[string]any)["id"].(string)
			sources := mustDo(t, server, http.MethodGet, "/api/v1/sources", "", http.StatusOK)
			sourceID := jsonArray(t, sources, "sources")[0].(map[string]any)["id"].(string)
			notes := mustDo(t, server, http.MethodGet, "/api/v1/notes", "", http.StatusOK)
			notePath := "/api/v1/notes/" + jsonArray(t, notes, "notes")[0].(map[string]any)["id"].(string)
			citation := mustDo(t, server, http.MethodPost, "/api/v1/citations", fmt.Sprintf(
				`{"source_id":%q,"fact_type":"person_birth","fact_owner_id":%q}`, sourceID, ids["Alive"]), http.StatusCreated)
			citationPath := "/api/v1/citations/" + mustString(t, citation, "id")
			for _, path := range []string{familyPath, "/api/v1/sources/" + sourceID, notePath, citationPath} {
				for _, private := range []bool{false, true} {
					setPrivate(t, server, path, private)
					if got := getEntity(t, server, path, "")["private"] == true; got != private {
						t.Errorf("%s private = %v after setting %v", path, got, private)
					}
				}
			}
		})
	}
}

// TestPrivacy_APIMarksProtectedPeopleOnEverySurface reads a living, private
// person and a deceased one through search, the charts, the group sheet and
// the map: each surface carries the living verdict and the private flag, so
// the UI can mark the person wherever they appear (#1413).
func TestPrivacy_APIMarksProtectedPeopleOnEverySurface(t *testing.T) {
	for _, backend := range backends {
		t.Run(backend.name, func(t *testing.T) {
			ctx := context.Background()
			st := backend.setup(t)
			ids := importPrivacyTree(t, ctx, st)
			server := newServer(t, st)
			alive, buried, recent := ids["Alive"].String(), ids["Buried"].String(), ids["Recent"].String()

			// Pin the two partners on the map; the API has no write for coordinates.
			for _, id := range []uuid.UUID{ids["Alive"], ids["Buried"]} {
				p, err := st.read.GetPerson(ctx, domain.MainBranchID, id)
				if err != nil || p == nil {
					t.Fatalf("get person: %v, %v", p, err)
				}
				lat, long := "N39.8", "W89.6"
				p.BirthPlace, p.BirthPlaceLat, p.BirthPlaceLong = "Springfield", &lat, &long
				if err := st.read.SavePerson(ctx, domain.MainBranchID, p); err != nil {
					t.Fatalf("save person: %v", err)
				}
			}

			families := mustDo(t, server, http.MethodGet, "/api/v1/families", "", http.StatusOK)
			familyID := jsonArray(t, families, "items")[0].(map[string]any)["id"].(string)
			mustDo(t, server, http.MethodPost, "/api/v1/families/"+familyID+"/children",
				fmt.Sprintf(`{"person_id":%q}`, recent), http.StatusCreated)

			type mark struct{ living, private any }
			got := map[string]mark{}
			note := func(surface string, person map[string]any) {
				got[surface+" "+person["id"].(string)] = mark{person["living"], person["private"]}
			}
			search := mustDo(t, server, http.MethodGet, "/api/v1/search?q=Alive", "", http.StatusOK)
			note("search", jsonArray(t, search, "items")[0].(map[string]any))
			pedigree := getEntity(t, server, "/api/v1/pedigree/"+recent, "")
			root := pedigree["root"].(map[string]any)
			note("pedigree", root)
			note("pedigree", root["father"].(map[string]any))
			note("pedigree", root["mother"].(map[string]any))
			descendancy := getEntity(t, server, "/api/v1/descendancy/"+buried, "")
			droot := descendancy["root"].(map[string]any)
			note("descendancy", droot)
			note("descendancy", jsonArray(t, droot, "children")[0].(map[string]any))
			note("descendancy spouse", jsonArray(t, droot, "spouses")[0].(map[string]any))
			sheet := getEntity(t, server, "/api/v1/families/"+familyID+"/group-sheet", "")
			note("group sheet", sheet["husband"].(map[string]any))
			note("group sheet", sheet["wife"].(map[string]any))
			note("group sheet", jsonArray(t, sheet, "children")[0].(map[string]any))
			locations := mustDo(t, server, http.MethodGet, "/api/v1/map/locations", "", http.StatusOK)
			for _, loc := range jsonArray(t, locations, "items") {
				for _, person := range jsonArray(t, loc.(map[string]any), "persons") {
					note("map", person.(map[string]any))
				}
			}

			livingPrivate, deceased, living := mark{true, true}, mark{false, nil}, mark{true, nil}
			want := map[string]mark{
				"search " + alive:             livingPrivate,
				"pedigree " + recent:          living,
				"pedigree " + buried:          deceased,
				"pedigree " + alive:           livingPrivate,
				"descendancy " + buried:       deceased,
				"descendancy " + recent:       living,
				"descendancy spouse " + alive: livingPrivate,
				"group sheet " + buried:       deceased,
				"group sheet " + alive:        livingPrivate,
				"group sheet " + recent:       living,
				"map " + alive:                livingPrivate,
				"map " + buried:               deceased,
			}
			for key, w := range want {
				if g, ok := got[key]; !ok || g != w {
					t.Errorf("%s: living, private = %v, want %v (present %v)", key, g, w, ok)
				}
			}
		})
	}
}

// setPrivate marks the record at path private or not.
func setPrivate(t *testing.T, server *api.Server, path string, private bool) {
	t.Helper()
	body := fmt.Sprintf(`{"private":%v,"version":%d}`, private, entityVersion(t, server, path, ""))
	mustDo(t, server, http.MethodPut, path, body, http.StatusOK)
}

// TestPrivacy_LivingRuleFollowsTheBranch records a death on a research
// branch: the person is deceased on that branch only (ADR-008 §1).
func TestPrivacy_LivingRuleFollowsTheBranch(t *testing.T) {
	forEachBackend(t, func(t *testing.T, server *api.Server) {
		personPath := "/api/v1/persons/" + createPerson(t, server, "Branch", "Living")
		branchID := createBranch(t, server, "records a death")
		body := fmt.Sprintf(`{"death_date":"1 JAN 2020","version":%d}`, entityVersion(t, server, personPath, branchID))
		mustDo(t, server, http.MethodPut, scoped(personPath, branchID), body, http.StatusOK)

		if got := getEntity(t, server, personPath, branchID)["living"]; got != false {
			t.Errorf("on the branch: living = %v, want false", got)
		}
		if got := getEntity(t, server, personPath, "")["living"]; got != true {
			t.Errorf("on main: living = %v, want true", got)
		}
	})
}

// TestPrivacy_StoresKeepRestrictionAndLivingFacts saves each record type with
// a restriction, and a person with the living rule's inputs, and reads them
// back from every backend, on main and on a branch.
func TestPrivacy_StoresKeepRestrictionAndLivingFacts(t *testing.T) {
	for _, backend := range backends {
		t.Run(backend.name, func(t *testing.T) {
			ctx := context.Background()
			rs := backend.setup(t).read
			now := time.Now().UTC().Truncate(time.Second)
			refDate := time.Date(1890, time.December, 31, 0, 0, 0, 0, time.UTC)
			for i, branch := range []domain.BranchID{domain.MainBranchID, domain.BranchID(uuid.New())} {
				name := fmt.Sprintf("Kept%d", i)
				lat, long := "N39.8", "W89.6"
				person := &repository.PersonReadModel{ID: uuid.New(), GivenName: name, Restriction: "CONFIDENTIAL, LOCKED",
					LivingOverride: domain.LivingOverrideDeceased, DeathEvidence: true, LivingRefDate: &refDate,
					DeathPlace: "Springfield", DeathPlaceLat: &lat, DeathPlaceLong: &long, Version: 1, UpdatedAt: now}
				if err := rs.SavePerson(ctx, branch, person); err != nil {
					t.Fatalf("save person: %v", err)
				}
				got, err := rs.GetPerson(ctx, branch, person.ID)
				if err != nil || got == nil {
					t.Fatalf("get person: %v, %v", got, err)
				}
				if got.Restriction != person.Restriction || got.LivingOverride != person.LivingOverride ||
					!got.DeathEvidence || got.LivingRefDate == nil || !got.LivingRefDate.Equal(refDate) {
					t.Errorf("person read back as %+v", got)
				}
				searched, _, err := rs.SearchPersons(ctx, repository.SearchOptions{Query: name, BranchID: branch, Limit: 10})
				if err != nil || len(searched) != 1 || searched[0].LivingRefDate == nil || searched[0].Restriction != person.Restriction {
					t.Errorf("search = %+v, %v; want the person with its privacy fields", searched, err)
				}
				locations, err := rs.GetMapLocations(ctx, branch, repository.Privacy{})
				if err != nil || len(locations) != 1 {
					t.Fatalf("map locations = %+v, %v; want the death place", locations, err)
				}
				pinned := locations[0].Persons[slices.IndexFunc(locations[0].Persons,
					func(m repository.MapPerson) bool { return m.ID == person.ID })]
				if facts := pinned.LivingFacts; pinned.Restriction != person.Restriction || facts.Override != person.LivingOverride ||
					!facts.DeathEvidence || facts.ReferenceDate == nil || !facts.ReferenceDate.Equal(refDate) {
					t.Errorf("map person = %+v, want the person's privacy fields", pinned)
				}

				family := &repository.FamilyReadModel{ID: uuid.New(), RelationshipType: domain.RelationMarriage, Restriction: "privacy", Version: 1, UpdatedAt: now}
				source := &repository.SourceReadModel{ID: uuid.New(), SourceType: domain.SourceOther, Title: "S", Restriction: "confidential", Version: 1, UpdatedAt: now}
				citation := &repository.CitationReadModel{ID: uuid.New(), SourceID: source.ID, SourceTitle: "S", FactType: domain.FactPersonBirth,
					FactOwnerID: person.ID, Restriction: "confidential", Version: 1, CreatedAt: now}
				note := &repository.NoteReadModel{ID: uuid.New(), Text: "N", Restriction: "confidential", Version: 1, UpdatedAt: now}
				media := &repository.MediaReadModel{ID: uuid.New(), EntityType: "person", EntityID: person.ID, Title: "M", MimeType: "image/png",
					MediaType: domain.MediaPhoto, Filename: "m.png", FileSize: 1, FileData: []byte{1}, Restriction: "privacy", Version: 1, CreatedAt: now, UpdatedAt: now}
				for _, err := range []error{
					rs.SaveFamily(ctx, branch, family), rs.SaveSource(ctx, branch, source), rs.SaveCitation(ctx, branch, citation),
					rs.SaveNote(ctx, branch, note), rs.SaveMedia(ctx, branch, media),
				} {
					if err != nil {
						t.Fatalf("save: %v", err)
					}
				}
				gotFamily, _ := rs.GetFamily(ctx, branch, family.ID)
				gotSource, _ := rs.GetSource(ctx, branch, source.ID)
				gotCitation, _ := rs.GetCitation(ctx, branch, citation.ID)
				gotNote, _ := rs.GetNote(ctx, branch, note.ID)
				gotMedia, _ := rs.GetMedia(ctx, branch, media.ID)
				gotMediaData, _ := rs.GetMediaWithData(ctx, branch, media.ID)
				for name, got := range map[string]domain.Restriction{
					"family":            restrictionOf(gotFamily, func(f *repository.FamilyReadModel) domain.Restriction { return f.Restriction }),
					"source":            restrictionOf(gotSource, func(s *repository.SourceReadModel) domain.Restriction { return s.Restriction }),
					"citation":          restrictionOf(gotCitation, func(c *repository.CitationReadModel) domain.Restriction { return c.Restriction }),
					"note":              restrictionOf(gotNote, func(n *repository.NoteReadModel) domain.Restriction { return n.Restriction }),
					"media":             restrictionOf(gotMedia, func(m *repository.MediaReadModel) domain.Restriction { return m.Restriction }),
					"media (with data)": restrictionOf(gotMediaData, func(m *repository.MediaReadModel) domain.Restriction { return m.Restriction }),
				} {
					want := map[string]domain.Restriction{"family": "privacy", "media": "privacy", "media (with data)": "privacy"}[name]
					if want == "" {
						want = "confidential"
					}
					if got != want {
						t.Errorf("branch %s: %s restriction = %q, want %q", branch, name, got, want)
					}
				}
			}
		})
	}
}

func restrictionOf[T any](row *T, field func(*T) domain.Restriction) domain.Restriction {
	if row == nil {
		return "<missing>"
	}
	return field(row)
}
