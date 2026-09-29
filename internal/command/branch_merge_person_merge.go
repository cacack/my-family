package command

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/query"
	"github.com/cacack/my-family/internal/repository"
)

// A person merge made on a branch (#834) is one PersonMerged on the SURVIVOR's
// stream, but its projection reaches across aggregates: it re-links the merged
// person's families, child link, names, facts, citations, media and GPS
// artifacts to the survivor and deletes the merged person, without writing to
// any of their streams. Replaying it onto main is still one event — main's
// projection does the same re-linking against main's rows — so what a branch
// merge needs is to get the ORDER and the CHECKS around it right:
//
//   - Order (orderPersonMergesForReplay). Everything the branch wrote that
//     names the merged person — their own stream (their creation or edits), a
//     family that links them, a media upload or GPS artifact about them — must
//     be on main before the merge re-links it, or it lands after the merge,
//     pointing at a person main no longer has.
//   - References (personReferences, branchPersonMerges). The survivor is a
//     person the replayed event makes main point at, so a survivor main has
//     removed since the fork is a dangling reference like any other. And a
//     reference to the merged person counts as present when the replay merges
//     them into a survivor main will have: the merge re-links it.
//   - Staleness (validateMergedPersonsNotStale). The merged person's stream is
//     not replayed, but the conflict verdict covered main's changes to it
//     (query.mergedPersonsMainChanged), so it is pinned like a replayed stream.

// personMergeOf is one PersonMerged a stream carries.
type personMergeOf struct {
	survivorID uuid.UUID
	mergedID   uuid.UUID
	position   int64
}

// personMergesIn decodes the PersonMerged events of one stream, in order.
func personMergesIn(group streamGroup) ([]personMergeOf, error) {
	var merges []personMergeOf
	for i := range group.events {
		evt := group.events[i]
		if evt.EventType != "PersonMerged" {
			continue
		}
		var payload struct {
			SurvivorID uuid.UUID `json:"survivor_id"`
			MergedID   uuid.UUID `json:"merged_id"`
		}
		if err := json.Unmarshal(evt.Data, &payload); err != nil {
			return nil, fmt.Errorf("decoding person merge on stream %s: %w", evt.StreamID, err)
		}
		merges = append(merges, personMergeOf{survivorID: payload.SurvivorID, mergedID: payload.MergedID, position: evt.Position})
	}
	return merges, nil
}

// branchPersonMerges maps every person the given streams merge away to the
// person they are merged into.
func branchPersonMerges(groups []streamGroup) (map[uuid.UUID]uuid.UUID, error) {
	var into map[uuid.UUID]uuid.UUID
	for _, group := range groups {
		merges, err := personMergesIn(group)
		if err != nil {
			return nil, err
		}
		for _, m := range merges {
			if into == nil {
				into = make(map[uuid.UUID]uuid.UUID)
			}
			into[m.mergedID] = m.survivorID
		}
	}
	return into, nil
}

// finalMergeSurvivor follows a person through a chain of merges (A into B,
// then B into C) to the person that finally holds their data. ok is false
// when the person is not merged away at all. The walk is bounded by the size
// of the map, so a malformed cycle cannot loop forever.
func finalMergeSurvivor(personID uuid.UUID, into map[uuid.UUID]uuid.UUID) (uuid.UUID, bool) {
	current, merged := into[personID]
	if !merged {
		return uuid.Nil, false
	}
	for range len(into) {
		next, again := into[current]
		if !again {
			break
		}
		current = next
	}
	return current, true
}

// survivorLookup adapts a merge map to the callback findDanglingReferences
// takes. A nil map follows nothing.
func survivorLookup(into map[uuid.UUID]uuid.UUID) func(uuid.UUID) (uuid.UUID, bool) {
	if len(into) == 0 {
		return nil
	}
	return func(personID uuid.UUID) (uuid.UUID, bool) { return finalMergeSurvivor(personID, into) }
}

// orderPersonMergesForReplay reorders the replay so that every stream that
// mentions a person a PersonMerged merges away lands before the stream that
// carries the merge, while every stream that mentions an entity the branch
// CREATED still lands after the stream that creates it. Streams no constraint
// moves keep their relative order.
//
// The replay is one Append per stream, and the branch's first-touch order can
// put the survivor's stream first: a branch that edits the survivor, links the
// merged person into a family, then merges touches the survivor before the
// family. Replayed in that order, the merge would run on main before the link
// lands, re-link nothing, delete the merged person — and the family's link
// would then land on a person main no longer has. On the branch the link came
// first and the merge moved it, so replaying the merge after it reproduces the
// branch's result.
//
// Moving the merge later is not enough on its own: when the branch created the
// survivor, the survivor's stream holds that creation too, and a family or
// child link naming the survivor that the move jumps over would land before
// the survivor exists — the projections denormalize a partner's name and a
// child's parents from the person row, so main would get a blank partner and
// a parentless pedigree edge the branch never had. Hence the second
// constraint. The two are solved together as a stable topological order
// (Kahn's algorithm, always taking the ready stream that came first).
//
// "Mentions" is deliberately generic: a stream mentions an entity when it is
// that entity's own stream or any of its events carries the entity's id.
// Every cross-aggregate reference a branch can write (a partner or child link,
// an association, a media owner, a GPS subject, a citation's fact owner)
// carries the id in its payload, so this cannot silently miss a reference
// shape a future event adds; a false positive only adds a constraint.
//
// When the constraints contradict each other — one stream both needs a
// branch-created survivor to exist and mentions the person merged into them
// (an association between the two, say) — no stream order reproduces the
// branch, and the merge is refused (ErrMergeDanglingReference) rather than
// replayed into a main that differs from it. A stream whose own events
// interleave with the merge lands whole on one side of it; that is the
// one-Append-per-stream granularity every replay ordering here shares.
func orderPersonMergesForReplay(groups []streamGroup) ([]streamGroup, error) {
	merged := make(map[int][]uuid.UUID)
	for i, group := range groups {
		found, err := personMergesIn(group)
		if err != nil {
			return nil, err
		}
		for _, m := range found {
			merged[i] = append(merged[i], m.mergedID)
		}
	}
	if len(merged) == 0 {
		return groups, nil
	}

	graph := newReplayGraph(len(groups))
	mentioned := make([]map[uuid.UUID]bool, len(groups))
	index := make(map[uuid.UUID]int, len(groups))
	for i := range groups {
		mentioned[i] = idsMentioned(groups[i])
		index[groups[i].streamID] = i
	}
	// A stream that names an entity the branch created lands after its creation.
	for j := range groups {
		for id := range mentioned[j] {
			if i, ok := index[id]; ok && i < j && createsAggregate(groups[i]) {
				graph.add(i, j)
			}
		}
	}
	// A stream that mentions a merged person lands before the merge.
	for at, personIDs := range merged {
		for j := range groups {
			if slices.ContainsFunc(personIDs, func(id uuid.UUID) bool { return groups[j].streamID == id || mentioned[j][id] }) {
				graph.add(j, at)
			}
		}
	}

	order, blocked := graph.stableOrder()
	if blocked < 0 {
		ordered := make([]streamGroup, len(order))
		for i, at := range order {
			ordered[i] = groups[at]
		}
		return ordered, nil
	}
	for at := range groups {
		if graph.indegree[at] > 0 && len(merged[at]) > 0 {
			blocked = at
			break
		}
	}
	return nil, fmt.Errorf(
		"%w: stream %s merges %s away, but another stream the branch wrote both mentions that person "+
			"and needs an entity this branch created on a stream the merge must follow; no replay order lands it "+
			"after that creation and before the merge, so main would not match the branch. "+
			"Remove that reference on the branch before merging",
		ErrMergeDanglingReference, groups[blocked].streamID, joinPersonIDs(merged[blocked]))
}

// replayGraph is the "must land before" relation orderPersonMergesForReplay
// sorts the replay by, over stream indexes in first-touch order.
type replayGraph struct {
	after    [][]int // after[i]: the streams that must land after i
	indegree []int
	edges    map[[2]int]bool
}

func newReplayGraph(n int) *replayGraph {
	return &replayGraph{after: make([][]int, n), indegree: make([]int, n), edges: make(map[[2]int]bool)}
}

// add records that stream from must land before stream to.
func (g *replayGraph) add(from, to int) {
	if from == to || g.edges[[2]int{from, to}] {
		return
	}
	g.edges[[2]int{from, to}] = true
	g.after[from] = append(g.after[from], to)
	g.indegree[to]++
}

// stableOrder returns the streams in an order satisfying every edge, always
// taking the ready stream that came first (Kahn's algorithm), so streams no
// edge moves keep their relative order. When the edges form a cycle it
// returns the streams it could place and the index of one it could not;
// blocked is -1 when every stream was placed.
func (g *replayGraph) stableOrder() (order []int, blocked int) {
	var ready []int
	for i, n := range g.indegree {
		if n == 0 {
			ready = append(ready, i)
		}
	}
	for len(ready) > 0 {
		next := ready[0]
		ready = ready[1:]
		order = append(order, next)
		for _, to := range g.after[next] {
			g.indegree[to]--
			if g.indegree[to] == 0 {
				at, _ := slices.BinarySearch(ready, to)
				ready = slices.Insert(ready, at, to)
			}
		}
	}
	for i, n := range g.indegree {
		if n > 0 {
			return order, i
		}
	}
	return order, -1
}

// uuidPattern matches a UUID in its canonical textual form, as event payloads
// carry them.
var uuidPattern = regexp.MustCompile(`[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}`)

// idsMentioned returns every id a stream's event payloads carry (see
// orderPersonMergesForReplay), not counting the stream's own.
func idsMentioned(group streamGroup) map[uuid.UUID]bool {
	ids := make(map[uuid.UUID]bool)
	for i := range group.events {
		for _, raw := range uuidPattern.FindAll(group.events[i].Data, -1) {
			id, err := uuid.ParseBytes(raw)
			if err == nil && id != group.streamID {
				ids[id] = true
			}
		}
	}
	return ids
}

// createsAggregate reports whether a replay group brings its entity into
// existence: its first event — the branch's first write to the stream — is a
// creation (PersonCreated, FamilyCreated, MediaCreated, …; an evidence
// conflict is created by EvidenceConflictDetected). A stream the branch only
// edits names an entity main already has, so nothing needs to land after it.
func createsAggregate(group streamGroup) bool {
	if len(group.events) == 0 {
		return false
	}
	first := group.events[0].EventType
	return strings.HasSuffix(first, "Created") || first == "EvidenceConflictDetected"
}

// joinPersonIDs renders person ids for an error message.
func joinPersonIDs(ids []uuid.UUID) string {
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = "person " + id.String()
	}
	return strings.Join(parts, ", ")
}

// validateMergedPersonsNotStale extends validatePlanNotStale to the persons a
// replayed PersonMerged merges away. Their streams are not replayed — the merge
// lands on the survivor's — but the replayed merge re-links main's rows for
// them and deletes them, and the conflict verdict covered main's changes to
// them. A mainline write to one after the verdict was computed was never
// reviewed, so the merge is refused as stale, with nothing written, exactly as
// for a replayed stream. As there, a missing pin is refused, never defaulted.
func (h *Handler) validateMergedPersonsNotStale(
	ctx context.Context,
	plan *query.MergePlan,
	groups []streamGroup,
	resolutions map[uuid.UUID]MergeResolution,
) error {
	checked := make(map[uuid.UUID]bool)
	for _, group := range groups {
		if resolutions[group.streamID] == ResolveMain {
			continue
		}
		merges, err := personMergesIn(group)
		if err != nil {
			return err
		}
		for _, m := range merges {
			if checked[m.mergedID] {
				continue
			}
			checked[m.mergedID] = true
			planned, pinned := plan.MainStreamVersions[m.mergedID]
			if !pinned {
				return fmt.Errorf(
					"%w: the merge plan for branch %s carries no pinned main version for person %s, "+
						"whom stream %s merges away",
					ErrMergePlanIncomplete, plan.Branch.ID, m.mergedID, group.streamID)
			}
			current, err := h.eventStore.GetStreamVersion(ctx, m.mergedID, domain.MainBranchID)
			if err != nil {
				return fmt.Errorf("getting main stream version for %s: %w", m.mergedID, err)
			}
			if current != planned {
				return fmt.Errorf(
					"%w: main is at version %d for person %s, whom the branch merges into %s, but the conflict "+
						"verdict was computed against version %d. Nothing has been written and branch %s is still "+
						"active — re-run GET /branches/{id}/compare to get a fresh verdict, then merge again",
					ErrMergePlanStale, current, m.mergedID, m.survivorID, planned, plan.Branch.ID)
			}
		}
	}
	return nil
}

// pendingPersonMerges maps the persons merged away by the streams a replay
// still has to append — replayed, but not already on main — to their
// survivors. A merge already on main has re-linked what it found then; a
// reference landing now is not re-linked by it, so it vouches for nothing.
func pendingPersonMerges(replayed map[uuid.UUID]streamGroup, landed map[uuid.UUID]bool) (map[uuid.UUID]uuid.UUID, error) {
	pending := make([]streamGroup, 0, len(replayed))
	for id, group := range replayed {
		if !landed[id] {
			pending = append(pending, group)
		}
	}
	// Map iteration order is random; a chain's final survivor does not depend
	// on it, but a stable input keeps any decoding error deterministic.
	sort.Slice(pending, func(i, j int) bool { return pending[i].streamID.String() < pending[j].streamID.String() })
	return branchPersonMerges(pending)
}

// personMergeSurvivor is personReferences' case for a PersonMerged: the
// survivor, the person the replayed event makes main write to. The merged
// person is not a reference — the merge removes them.
func personMergeSurvivor(evt repository.StoredEvent) ([]uuid.UUID, error) {
	var payload struct {
		SurvivorID uuid.UUID `json:"survivor_id"`
	}
	if err := json.Unmarshal(evt.Data, &payload); err != nil {
		return nil, fmt.Errorf("decoding person merge on stream %s: %w", evt.StreamID, err)
	}
	return []uuid.UUID{payload.SurvivorID}, nil
}

// collectPersonMergeBlockers reports every replayed PersonMerged that would
// land on a main where the two persons can no longer be merged the way the
// branch merged them, because of what main did to them after the fork. The
// merge command checked its guards against the BRANCH's state; main's changes
// since are cross-stream, so the per-stream conflict classifier never sees
// them:
//
//   - Main merged the merged person into someone else (a PersonMerged on that
//     other person's stream). Main holds one identity hypothesis for them and
//     the branch another; replaying the branch's would move main's copy of the
//     merged person's resolved fields onto the survivor while their families,
//     names and evidence stay with main's survivor. A merged person main
//     DELETED is not refused: both sides removed them, and the replay re-links
//     only what the branch itself added (see mergedPersonsMainChanged).
//   - The survivor and the merged person are children of different families
//     when the merge lands (main linked one of them after the fork). A person
//     has one child family, so the projection would drop the merged person's
//     parentage — the case MergePersons refuses as ErrChildFamilyConflict.
//
// Child families are tracked through the replay: main's read model is the
// starting point, and every replayed link, unlink, family delete and earlier
// person merge before the merge is applied in replay order, so a branch that
// unlinked one of them before merging is judged on the state the merge will
// actually meet. Each breach is a BlockerPersonMergeConflictsMain on the
// survivor's stream (#831), and its fix is the same as for any dangling
// reference: resolve the survivor's stream to "main", which skips the merge.
func (h *Handler) collectPersonMergeBlockers(
	ctx context.Context,
	groups []streamGroup,
	resolutions map[uuid.UUID]MergeResolution,
	created map[uuid.UUID]bool,
	list *blockerList,
) error {
	sim := childFamilySim{h: h, known: make(map[uuid.UUID]*uuid.UUID), deleted: make(map[uuid.UUID]bool)}
	for _, group := range groups {
		if resolutions[group.streamID] == ResolveMain {
			continue
		}
		for i := range group.events {
			evt := group.events[i]
			if evt.EventType == "PersonMerged" {
				if err := h.checkReplayedPersonMerge(ctx, group, evt, &sim, created, list); err != nil {
					return err
				}
				continue
			}
			if err := sim.apply(ctx, evt); err != nil {
				return err
			}
		}
	}
	return nil
}

// checkReplayedPersonMerge applies collectPersonMergeBlockers' rules to one
// replayed PersonMerged, then records its effect on child families.
func (h *Handler) checkReplayedPersonMerge(ctx context.Context, group streamGroup, evt repository.StoredEvent, sim *childFamilySim, created map[uuid.UUID]bool, list *blockerList) error {
	var payload struct {
		SurvivorID uuid.UUID `json:"survivor_id"`
		MergedID   uuid.UUID `json:"merged_id"`
	}
	if err := json.Unmarshal(evt.Data, &payload); err != nil {
		return fmt.Errorf("decoding person merge on stream %s: %w", evt.StreamID, err)
	}
	if !created[payload.MergedID] {
		mergedAway, err := h.mergedAwayOnMain(ctx, payload.MergedID)
		if err != nil {
			return err
		}
		if mergedAway {
			same, err := h.mainMergedInto(ctx, payload.MergedID, payload.SurvivorID)
			if err != nil {
				return err
			}
			if same {
				list.add(streamBlocker(group, BlockerPersonMergeConflictsMain, payload.MergedID, "person",
					"the branch merges person %s into %s, and main has since made the same merge. "+
						"Resolve stream %s to main to keep main's merge, which already holds that person's data",
					payload.MergedID, payload.SurvivorID, payload.SurvivorID))
			} else {
				list.add(streamBlocker(group, BlockerPersonMergeConflictsMain, payload.MergedID, "person",
					"the branch merges person %s into %s, but main has since merged person %s into another person. "+
						"The two merges disagree about who that person is; resolve stream %s to main to keep main's merge, "+
						"or undo main's merge and compare again",
					payload.MergedID, payload.SurvivorID, payload.MergedID, payload.SurvivorID))
			}
			// Main no longer has the merged person, so the child-family rule
			// below has nothing of main's to compare.
			sim.known[payload.MergedID] = nil
			return nil
		}
	}
	survivorFamily, err := sim.childFamily(ctx, payload.SurvivorID)
	if err != nil {
		return err
	}
	mergedFamily, err := sim.childFamily(ctx, payload.MergedID)
	if err != nil {
		return err
	}
	if survivorFamily != nil && mergedFamily != nil && *survivorFamily != *mergedFamily {
		blocker := streamBlocker(group, BlockerPersonMergeConflictsMain, payload.MergedID, "person",
			"the branch merges person %s into %s, but on main they are children of different families "+
				"(%s and %s) when the merge lands, and a person has one child family: the merge would drop %s's "+
				"parentage. Unlink one of them (on main or on the branch) and compare again, or resolve stream %s "+
				"to main to skip the merge: %s",
			payload.MergedID, payload.SurvivorID, *survivorFamily, *mergedFamily,
			payload.MergedID, payload.SurvivorID, ErrChildFamilyConflict)
		blocker.cause = ErrChildFamilyConflict
		list.add(blocker)
	}
	if survivorFamily == nil {
		sim.known[payload.SurvivorID] = mergedFamily
	}
	sim.known[payload.MergedID] = nil
	return nil
}

// mergedAwayOnMain reports whether main merged a person into another after
// having them: main's read model no longer has them, main's log has them, and
// their stream does not end in a delete (PersonMerged does not write to the
// merged person's stream; PersonDeleted does).
func (h *Handler) mergedAwayOnMain(ctx context.Context, personID uuid.UUID) (bool, error) {
	person, err := h.readStore.GetPerson(ctx, domain.MainBranchID, personID)
	if err != nil {
		return false, fmt.Errorf("checking person %s on main: %w", personID, err)
	}
	if person != nil {
		return false, nil
	}
	history, err := h.readMainStreams(ctx, []uuid.UUID{personID})
	if err != nil {
		return false, err
	}
	events := history[personID]
	if len(events) == 0 {
		return false, nil // main never had them; the branch created them
	}
	return events[len(events)-1].EventType != "PersonDeleted", nil
}

// mainMergedInto reports whether main's log merges a person into the given
// survivor: a PersonMerged naming them on the survivor's main stream, where
// main records a merge. It tells a merge both sides made alike from one where
// main chose a different survivor, so a refusal can say which it is.
func (h *Handler) mainMergedInto(ctx context.Context, mergedID, survivorID uuid.UUID) (bool, error) {
	history, err := h.readMainStreams(ctx, []uuid.UUID{survivorID})
	if err != nil {
		return false, err
	}
	for _, evt := range history[survivorID] {
		if evt.EventType != "PersonMerged" {
			continue
		}
		var payload struct {
			MergedID uuid.UUID `json:"merged_id"`
		}
		if err := json.Unmarshal(evt.Data, &payload); err != nil {
			return false, fmt.Errorf("decoding person merge on main stream %s: %w", survivorID, err)
		}
		if payload.MergedID == mergedID {
			return true, nil
		}
	}
	return false, nil
}

// childFamilySim tracks the child family of the persons a replay merges, as
// main's read model will have it at each point of the replay (see
// collectPersonMergeBlockers). A person not yet in known is read from main.
type childFamilySim struct {
	h     *Handler
	known map[uuid.UUID]*uuid.UUID

	// deleted holds the families the replay has deleted so far; a person main
	// files under one is no longer its child.
	deleted map[uuid.UUID]bool
}

// childFamily returns a person's child family at this point of the replay.
func (s *childFamilySim) childFamily(ctx context.Context, personID uuid.UUID) (*uuid.UUID, error) {
	if familyID, ok := s.known[personID]; ok {
		return familyID, nil
	}
	family, err := s.h.readStore.GetChildFamily(ctx, domain.MainBranchID, personID)
	if err != nil {
		return nil, fmt.Errorf("getting child family of %s on main: %w", personID, err)
	}
	var familyID *uuid.UUID
	if family != nil && !s.deleted[family.ID] {
		familyID = &family.ID
	}
	s.known[personID] = familyID
	return familyID, nil
}

// apply records one replayed event's effect on child families: a link sets
// its person's family, an unlink clears it, and a family delete clears every
// person filed under that family, now or when first read from main.
func (s *childFamilySim) apply(ctx context.Context, evt repository.StoredEvent) error {
	switch evt.EventType {
	case "ChildLinkedToFamily", "ChildUnlinkedFromFamily":
		var payload struct {
			FamilyID uuid.UUID `json:"family_id"`
			PersonID uuid.UUID `json:"person_id"`
		}
		if err := json.Unmarshal(evt.Data, &payload); err != nil {
			return fmt.Errorf("decoding %s on stream %s: %w", evt.EventType, evt.StreamID, err)
		}
		if evt.EventType == "ChildLinkedToFamily" {
			familyID := payload.FamilyID
			s.known[payload.PersonID] = &familyID
			return nil
		}
		current, err := s.childFamily(ctx, payload.PersonID)
		if err != nil {
			return err
		}
		if current != nil && *current == payload.FamilyID {
			s.known[payload.PersonID] = nil
		}
	case "FamilyDeleted":
		s.deleted[evt.StreamID] = true
		for personID, familyID := range s.known {
			if familyID != nil && *familyID == evt.StreamID {
				s.known[personID] = nil
			}
		}
	}
	return nil
}
