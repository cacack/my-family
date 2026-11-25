package integration_test

import (
	"archive/zip"
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/cacack/my-family/internal/backup"
	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/storage"
)

// Every event a release wrote must stay readable by every later version
// (#1802, ADR-006). Each directory under testdata/eventlogs holds the log one
// release recorded (scripts/record-event-log.sh), as a backup archive, with
// the read-model values that release's replay built from it. Replaying the
// log on this version must rebuild every one of those values; values this
// version adds pass.

// eventLogsDir holds one directory per recorded release, named by its tag.
const eventLogsDir = "../../testdata/eventlogs"

func TestRecordedEventLogs_Replay(t *testing.T) {
	releases := recordedReleases(t)
	for _, backend := range sqlBackends {
		for _, release := range releases {
			t.Run(backend.name+"/"+release, func(t *testing.T) {
				ctx := context.Background()
				dir := filepath.Join(eventLogsDir, release)
				want := readExpected(t, filepath.Join(dir, "expected.json"))

				cfg := backend.cfg(t)
				result, err := storage.Restore(ctx, cfg, filepath.Join(dir, "backup.zip"))
				if err != nil {
					t.Fatalf("restore and rebuild the %s log: %v", release, err)
				}
				if result.Rebuild.Failed > 0 {
					t.Fatalf("%d of the %s log's %d events failed to project", result.Rebuild.Failed, release, result.Rebuild.Events)
				}
				st := openStores(t, cfg, storage.AccessServe)
				defer closeStores(t, st)
				failures, err := st.ReadModel.ListProjectionFailures(ctx, 10)
				if err != nil {
					t.Fatal(err)
				}
				if len(failures) > 0 {
					t.Fatalf("replaying the %s log recorded projection failures: %+v", release, failures)
				}

				// PostgreSQL keeps microseconds, so a time the SQLite
				// recording kept in nanoseconds is compared at that.
				precision := time.Nanosecond
				if backend.name != "SQLite" {
					precision = time.Microsecond
				}
				for _, diff := range subsetDiffs("", want, replayDump(t, ctx, st), precision) {
					t.Error(diff)
				}
			})
		}
	}
}

// TestRecordedEventLogs_NewestCoversDecodedTypes holds the newest recording
// to every event type its release's DecodeEvent handled, so each type is
// replayed from a release's own bytes.
func TestRecordedEventLogs_NewestCoversDecodedTypes(t *testing.T) {
	releases := recordedReleases(t)
	newest := releases[len(releases)-1]
	dir := filepath.Join(eventLogsDir, newest)
	data, err := os.ReadFile(filepath.Join(dir, "event-types.json"))
	if err != nil {
		t.Fatal(err)
	}
	var handled []string
	if err := json.Unmarshal(data, &handled); err != nil {
		t.Fatal(err)
	}
	if len(handled) == 0 {
		t.Fatalf("%s/event-types.json names no event types", newest)
	}
	for _, missing := range missingEventTypes(handled, loggedEventTypes(t, filepath.Join(dir, "backup.zip"))) {
		t.Errorf("the %s log has no %s event, which %s's DecodeEvent handled; record one (scripts/record-event-log.sh)",
			newest, missing, newest)
	}
}

func TestMissingEventTypes(t *testing.T) {
	logged := map[string]bool{"PersonCreated": true, "FamilyCreated": true}
	if got := missingEventTypes([]string{"PersonCreated", "FamilyCreated"}, logged); len(got) != 0 {
		t.Errorf("a log with every type is missing %v", got)
	}
	got := missingEventTypes([]string{"PersonCreated", "PersonMerged", "FamilyCreated"}, logged)
	if !reflect.DeepEqual(got, []string{"PersonMerged"}) {
		t.Errorf("missing = %v, want [PersonMerged]", got)
	}
}

func TestSubsetDiffs(t *testing.T) {
	recorded := map[string]any{"persons": map[string]any{"p1": map[string]any{
		"given_name": "Ada", "version": json.Number("2"), "citation_ids": []any{"c1", "c2"},
		"updated_at": "2026-11-23T10:00:00.123456789Z", "death_date": nil,
	}}}
	cases := []struct {
		name      string
		got       map[string]any
		precision time.Duration
		wantDiffs []string
	}{
		{"identical", recorded, time.Nanosecond, nil},
		{"a field, row and section this version adds", map[string]any{
			"persons": map[string]any{
				"p1": map[string]any{"given_name": "Ada", "version": json.Number("2"), "citation_ids": []any{"c1", "c2"},
					"updated_at": "2026-11-23T10:00:00.123456789Z", "nickname": "Addie"},
				"p2": map[string]any{"given_name": "New"},
			},
			"places": map[string]any{},
		}, time.Nanosecond, nil},
		{"a lost field", map[string]any{"persons": map[string]any{"p1": map[string]any{
			"version": json.Number("2"), "citation_ids": []any{"c1", "c2"}, "updated_at": "2026-11-23T10:00:00.123456789Z",
		}}}, time.Nanosecond, []string{"persons.p1.given_name: missing, want \"Ada\""}},
		{"a changed value", map[string]any{"persons": map[string]any{"p1": map[string]any{
			"given_name": "Adah", "version": json.Number("2"), "citation_ids": []any{"c1", "c2"}, "updated_at": "2026-11-23T10:00:00.123456789Z",
		}}}, time.Nanosecond, []string{"persons.p1.given_name: got \"Adah\", want \"Ada\""}},
		{"a list that lost an entry", map[string]any{"persons": map[string]any{"p1": map[string]any{
			"given_name": "Ada", "version": json.Number("2"), "citation_ids": []any{"c1"}, "updated_at": "2026-11-23T10:00:00.123456789Z",
		}}}, time.Nanosecond, []string{"persons.p1.citation_ids: got 1 entries, want 2"}},
		{"a lost row", map[string]any{"persons": map[string]any{}}, time.Nanosecond, []string{"persons.p1: missing"}},
		{"a lost section", map[string]any{}, time.Nanosecond, []string{"persons: missing"}},
		{"a time at PostgreSQL's precision", map[string]any{"persons": map[string]any{"p1": map[string]any{
			"given_name": "Ada", "version": json.Number("2"), "citation_ids": []any{"c1", "c2"}, "updated_at": "2026-11-23T10:00:00.123457Z",
		}}}, time.Microsecond, nil},
		{"a changed time", map[string]any{"persons": map[string]any{"p1": map[string]any{
			"given_name": "Ada", "version": json.Number("2"), "citation_ids": []any{"c1", "c2"}, "updated_at": "2026-11-23T10:00:01.123456789Z",
		}}}, time.Microsecond, []string{"persons.p1.updated_at: got \"2026-11-23T10:00:01.123456789Z\", want \"2026-11-23T10:00:00.123456789Z\""}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := subsetDiffs("", recorded, tc.got, tc.precision)
			if !reflect.DeepEqual(got, tc.wantDiffs) {
				t.Errorf("diffs = %q, want %q", got, tc.wantDiffs)
			}
		})
	}
}

// recordedReleases returns the recorded releases' tags, oldest first.
func recordedReleases(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(eventLogsDir)
	if err != nil {
		t.Fatal(err)
	}
	var releases []string
	for _, e := range entries {
		if e.IsDir() && strings.HasPrefix(e.Name(), "v") {
			releases = append(releases, e.Name())
		}
	}
	if len(releases) == 0 {
		t.Fatalf("no recorded event logs in %s", eventLogsDir)
	}
	sort.Slice(releases, func(i, j int) bool { return versionLess(t, releases[i], releases[j]) })
	return releases
}

// versionLess orders two vMAJOR.MINOR.PATCH tags.
func versionLess(t *testing.T, a, b string) bool {
	t.Helper()
	pa, pb := versionParts(t, a), versionParts(t, b)
	for i := range pa {
		if pa[i] != pb[i] {
			return pa[i] < pb[i]
		}
	}
	return false
}

func versionParts(t *testing.T, tag string) [3]int {
	t.Helper()
	var parts [3]int
	fields := strings.Split(strings.TrimPrefix(tag, "v"), ".")
	if len(fields) != 3 {
		t.Fatalf("%s is not a vMAJOR.MINOR.PATCH tag", tag)
	}
	for i, f := range fields {
		n, err := strconv.Atoi(f)
		if err != nil {
			t.Fatalf("%s is not a vMAJOR.MINOR.PATCH tag", tag)
		}
		parts[i] = n
	}
	return parts
}

func readExpected(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := decodeNumbers(data, &out); err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return out
}

// decodeNumbers decodes JSON keeping numbers as written, so a large integer
// is compared exactly.
func decodeNumbers(data []byte, v any) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	return dec.Decode(v)
}

// loggedEventTypes returns the event types in an archive's event log.
func loggedEventTypes(t *testing.T, archive string) map[string]bool {
	t.Helper()
	zr, err := zip.OpenReader(archive)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	rc, err := zr.Open(backup.EventsEntry)
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	types := map[string]bool{}
	lines := bufio.NewScanner(rc)
	lines.Buffer(nil, 16<<20)
	for lines.Scan() {
		var event struct {
			EventType string `json:"event_type"`
		}
		if err := json.Unmarshal(lines.Bytes(), &event); err != nil {
			t.Fatalf("read %s: %v", backup.EventsEntry, err)
		}
		types[event.EventType] = true
	}
	if err := lines.Err(); err != nil {
		t.Fatal(err)
	}
	return types
}

// missingEventTypes returns the handled types the log has none of.
func missingEventTypes(handled []string, logged map[string]bool) []string {
	var missing []string
	for _, eventType := range handled {
		if !logged[eventType] {
			missing = append(missing, eventType)
		}
	}
	return missing
}

// subsetDiffs describes how got fails to hold want: every key of every
// object in want must be in got with an equal value, and a list must hold
// the same entries in the same order. A key only got has passes, as does a
// key want holds as null and got leaves out. Two times are equal at
// precision.
func subsetDiffs(path string, want, got any, precision time.Duration) []string {
	switch w := want.(type) {
	case map[string]any:
		g, ok := got.(map[string]any)
		if !ok {
			return []string{fmt.Sprintf("%s: got %s, want an object", path, describe(got))}
		}
		keys := make([]string, 0, len(w))
		for k := range w {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		var diffs []string
		for _, k := range keys {
			at := k
			if path != "" {
				at = path + "." + k
			}
			gv, ok := g[k]
			if !ok {
				switch w[k].(type) {
				case nil:
				case map[string]any:
					diffs = append(diffs, at+": missing")
				default:
					diffs = append(diffs, fmt.Sprintf("%s: missing, want %s", at, describe(w[k])))
				}
				continue
			}
			diffs = append(diffs, subsetDiffs(at, w[k], gv, precision)...)
		}
		return diffs
	case []any:
		g, ok := got.([]any)
		if !ok {
			return []string{fmt.Sprintf("%s: got %s, want a list", path, describe(got))}
		}
		if len(g) != len(w) {
			return []string{fmt.Sprintf("%s: got %d entries, want %d", path, len(g), len(w))}
		}
		var diffs []string
		for i := range w {
			diffs = append(diffs, subsetDiffs(fmt.Sprintf("%s[%d]", path, i), w[i], g[i], precision)...)
		}
		return diffs
	default:
		if sameValue(want, got, precision) {
			return nil
		}
		return []string{fmt.Sprintf("%s: got %s, want %s", path, describe(got), describe(want))}
	}
}

func sameValue(want, got any, precision time.Duration) bool {
	if reflect.DeepEqual(want, got) {
		return true
	}
	ws, ok1 := want.(string)
	gs, ok2 := got.(string)
	if !ok1 || !ok2 {
		return false
	}
	wt, err1 := time.Parse(time.RFC3339Nano, ws)
	gt, err2 := time.Parse(time.RFC3339Nano, gs)
	return err1 == nil && err2 == nil && wt.Round(precision).Equal(gt.Round(precision))
}

func describe(v any) string {
	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprint(v)
	}
	return string(data)
}

// replayDump returns the read-model values to compare, in the shape the
// recorder's dump writes expected.json (testdata/eventlogs/record/main.go):
// each section maps a row's key to the row as JSON. A section may grow here
// ahead of the recorder; one the recorder writes must be built here too.
func replayDump(t *testing.T, ctx context.Context, st *storage.Stores) map[string]any {
	t.Helper()
	rs := st.ReadModel
	out := map[string]any{}
	add := func(section string, rows any, err error, keys ...string) {
		t.Helper()
		if err != nil {
			t.Fatalf("dump %s: %v", section, err)
		}
		keyed, err := keyRows(rows, keys...)
		if err != nil {
			t.Fatalf("dump %s: %v", section, err)
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
			t.Fatalf("dump person_names: %v", err)
		}
		names = append(names, n...)
		m, _, err := rs.ListMediaForEntity(ctx, "person", p.ID, repository.ListOptions{Limit: 1000})
		if err != nil {
			t.Fatalf("dump media: %v", err)
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
	return out
}

// keyRows turns a slice of rows into a map from each row's key (its fields
// named by keys, joined by "/") to the row as decoded JSON.
func keyRows(rows any, keys ...string) (map[string]any, error) {
	data, err := json.Marshal(rows)
	if err != nil {
		return nil, err
	}
	var list []map[string]any
	if err := decodeNumbers(data, &list); err != nil {
		return nil, err
	}
	out := map[string]any{}
	for _, row := range list {
		parts := make([]string, len(keys))
		for i, k := range keys {
			parts[i] = fmt.Sprint(row[k])
		}
		key := strings.Join(parts, "/")
		if _, dup := out[key]; dup {
			return nil, errors.New("two rows share the key " + key)
		}
		out[key] = row
	}
	return out, nil
}
