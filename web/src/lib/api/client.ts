/**
 * My Family Genealogy API Client
 * Types are generated from OpenAPI spec - see types.generated.ts
 * Run `npm run generate:types` after OpenAPI changes
 */

import type { components, operations } from './types.generated';

// Re-export Ahnentafel types from generated file (single source of truth)
export type AhnentafelResponse = components['schemas']['AhnentafelResponse'];
export type AhnentafelEntry = components['schemas']['AhnentafelEntry'];
export type AhnentafelSubject = components['schemas']['AhnentafelSubject'];

// Re-export Citation template types from generated file
export type CitationTemplate = components['schemas']['CitationTemplate'];
export type CitationTemplateField = components['schemas']['CitationTemplateField'];
export type CitationTemplateList = components['schemas']['CitationTemplateList'];
export type FormattedCitation = components['schemas']['FormattedCitation'];
export type CitationValidationIssue = components['schemas']['CitationValidationIssue'];

// Re-export Rollback types from generated file
export type RestorePoint = components['schemas']['RestorePoint'];
export type RestorePointsResponse = components['schemas']['RestorePointsResponse'];
export type RollbackRequest = components['schemas']['RollbackRequest'];
export type RollbackResponse = components['schemas']['RollbackResponse'];

// Re-export Quality/Validation types from generated file
export type ValidationIssue = components['schemas']['ValidationIssue'];
export type ValidationIssuesResponse = components['schemas']['ValidationIssuesResponse'];
export type QualityOverview = components['schemas']['QualityOverview'];

// Re-export Duplicate detection & merge types from generated file
export type DuplicatePair = components['schemas']['DuplicatePair'];
export type DuplicatesResponse = components['schemas']['DuplicatesResponse'];
export type MergePersonsRequest = components['schemas']['MergePersonsRequest'];
export type MergePersonsResponse = components['schemas']['MergePersonsResponse'];
export type MergeSummary = components['schemas']['MergeSummary'];
export type DismissDuplicateRequest = components['schemas']['DismissDuplicateRequest'];
export type DuplicatePairRef = components['schemas']['DuplicatePairRef'];
export type BatchMergeRequest = components['schemas']['BatchMergeRequest'];
export type BatchMergeResponse = components['schemas']['BatchMergeResponse'];
export type BatchMergeResult = components['schemas']['BatchMergeResult'];
export type BatchDismissRequest = components['schemas']['BatchDismissRequest'];
export type BatchDismissResponse = components['schemas']['BatchDismissResponse'];
export type BatchDismissResult = components['schemas']['BatchDismissResult'];

// Re-export Repository types from generated file (single source of truth)
export type Repository = components['schemas']['Repository'];
export type RepositoryDetail = components['schemas']['RepositoryDetail'];
export type RepositoryCreate = components['schemas']['RepositoryCreate'];
export type RepositoryUpdate = components['schemas']['RepositoryUpdate'];
export type RepositoryList = components['schemas']['RepositoryList'];
export type Address = components['schemas']['Address'];

// Re-export Research Branch types from generated file (single source of truth)
export type Branch = components['schemas']['Branch'];
export type BranchCreate = components['schemas']['BranchCreate'];
export type BranchUpdate = components['schemas']['BranchUpdate'];
/** The verdict a line of research reached (#835); independent of `status`. */
export type BranchOutcome = components['schemas']['BranchOutcome'];
export type BranchCloseOutcome = components['schemas']['BranchCloseOutcome'];
export type BranchCloseRequest = components['schemas']['BranchCloseRequest'];
export type BranchResearchArchive = components['schemas']['BranchResearchArchive'];
export type ArchivedResearchLog = components['schemas']['ArchivedResearchLog'];
export type ArchivedEvidenceAnalysis = components['schemas']['ArchivedEvidenceAnalysis'];
export type ArchivedProofSummary = components['schemas']['ArchivedProofSummary'];
export type PromoteResearchLogsResult = components['schemas']['PromoteResearchLogsResult'];
export type BranchSubject = components['schemas']['BranchSubject'];
export type BranchSubjectInput = components['schemas']['BranchSubjectInput'];
export type BranchProofSummaryRef = components['schemas']['BranchProofSummaryRef'];
export type BranchList = components['schemas']['BranchList'];
export type BranchDrift = components['schemas']['BranchDrift'];
export type BranchComparisonResult = components['schemas']['BranchComparisonResult'];
export type MergeConflict = components['schemas']['MergeConflict'];
/** A merged branch's record of what its merge decided (#832). */
export type MergeRecord = components['schemas']['MergeRecord'];
export type MergeRecordDecision = components['schemas']['MergeRecordDecision'];
export type MergeRecordExclusion = components['schemas']['MergeRecordExclusion'];
/** The merge a mainline change came with (#832). */
export type MergeOrigin = components['schemas']['MergeOrigin'];
/** One contested field of a conflict, valued at the fork and on each side (#828). */
export type MergeConflictField = components['schemas']['MergeConflictField'];
export type BranchMergeRequest = components['schemas']['BranchMergeRequest'];
export type BranchMergeResult = components['schemas']['BranchMergeResult'];
export type MergeResolutionEntry = components['schemas']['MergeResolutionEntry'];
export type BranchMergeConflictError = components['schemas']['BranchMergeConflictError'];
export type BranchMergeResumeRequest = components['schemas']['BranchMergeResumeRequest'];
export type BranchMergeResumeResult = components['schemas']['BranchMergeResumeResult'];
export type BranchMergeResumeError = components['schemas']['BranchMergeResumeError'];
/** One entity an interrupted merge has not replayed yet (#830). */
export type MergePendingEntity = components['schemas']['MergePendingEntity'];
export type MergeBlocker = components['schemas']['MergeBlocker'];
export type BranchMergePrecheckRequest = components['schemas']['BranchMergePrecheckRequest'];
export type BranchMergePrecheckResult = components['schemas']['BranchMergePrecheckResult'];
export type BranchEvidenceCoverage = components['schemas']['BranchEvidenceCoverage'];
export type BranchChangedFact = components['schemas']['BranchChangedFact'];
export type BranchHealth = components['schemas']['BranchHealth'];
export type BranchValidationIssue = components['schemas']['BranchValidationIssue'];
export type BranchQualityIssue = components['schemas']['BranchQualityIssue'];
export type BranchDuplicatePair = components['schemas']['BranchDuplicatePair'];
/**
 * The side that wins for one entity - `'branch' | 'main'`. Derived from the
 * generated entry rather than hand-written so it cannot drift from the spec's
 * enum, and so a review UI can type its selection state without restating it.
 */
export type MergeResolution = MergeResolutionEntry['resolution'];
/** The `ChangeEntry` carried by `BranchComparisonResult`. */
export type BranchChangeEntry = components['schemas']['ChangeEntry'];

// Re-export Research Snapshot types from generated file (single source of truth)
export type Snapshot = components['schemas']['Snapshot'];
export type SnapshotCreate = components['schemas']['SnapshotCreate'];
export type SnapshotList = components['schemas']['SnapshotList'];
export type SnapshotComparisonResult = components['schemas']['SnapshotComparisonResult'];
export type SnapshotCurrentComparisonResult =
	components['schemas']['SnapshotCurrentComparisonResult'];

const API_BASE = '/api/v1';

// ---------------------------------------------------------------------------
// Research branch scoping
// ---------------------------------------------------------------------------

/**
 * The active research branch id, or null for the mainline.
 *
 * It lives here rather than in `activeBranch.svelte.ts` so that `request()`
 * never has to import the store: the store imports this module, and the reverse
 * edge would be a circular import.
 */
let activeBranchId: string | null = null;

/** Point every branch-scoped request at `branchId`, or at the mainline when null. */
export function setClientBranch(branchId: string | null): void {
	activeBranchId = branchId;
}

/** The branch id currently scoping requests, or null for the mainline. */
export function getClientBranch(): string | null {
	return activeBranchId;
}

const UUID_SEGMENT = '[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}';

/**
 * One non-empty path segment of free text — a surname or a place name, already
 * percent-encoded by the caller. Unlike `{id}` segments these are not UUIDs, so
 * they cannot be pinned to a shape; excluding `/` is what keeps the pattern
 * honest, since an encoded name never contains a literal slash (`encodeURIComponent`
 * emits `%2F`) and a longer path must not collapse into a one-segment match.
 */
const TEXT_SEGMENT = '[^/]+';

/**
 * The operations that declare the `?branch=` (`branchScope`) parameter in
 * `internal/api/openapi.yaml`.
 *
 * The parameter is declared **per operation, not per path**, so this table
 * matches on method as well: two methods on one path need not agree.
 *
 * `{id}` segments are matched as UUIDs rather than as `[^/]+` so that the real
 * two-segment literal routes — `GET /persons/duplicates` and
 * `POST /persons/merge` — cannot be mistaken for `/persons/{id}`.
 *
 * The table covers the #669 vertical slice (persons, person names, families,
 * family children, pedigree), the browse and map aggregates that #676
 * sub-issue A (#756) fanned out over, the person/family facts of
 * sub-issue B (#757) — the cemetery index and the association endpoints — and
 * the evidence of sub-issue C (#758): sources (including search), citations
 * and notes, and the media of sub-issue D (#759): metadata CRUD, the person's
 * media list and upload, and the content and thumbnail reads. Person and
 * family history follow the branch (#824); source history stays mainline-only.
 * Restore points and rollback carry the scope only to be refused: rollback is
 * mainline-only for every entity (ADR-005). Search, the families list, the
 * group sheet, the Ahnentafel, descendancy and the relationship calculator
 * follow the branch too (#829), and so do research snapshots (#839): a
 * snapshot marks a position in one branch's view, so the list, create,
 * delete and both comparisons carry the scope. The quality overview behind
 * `/analytics` follows the branch too (#894). The
 * aggregates own no `branch_id` of their own — they read the overlay — so
 * scoping them is exactly this parameter and nothing else.
 *
 * Read models and operations still answering only from the mainline, and
 * therefore absent here on purpose: brick walls (not event-sourced — #761;
 * whether they become event-sourced is #802), the entities that stay main-only
 * by decision (submitters, repositories, LDS ordinances — ADR-005), GEDCOM
 * import and export, the JSON/CSV exports, the global and source history, and
 * the other quality, statistics and discovery checks computed over the mainline.
 * Pages showing them render `MainlineNotice.svelte` or withdraw the control. Grow this table one operation at a time as the spec
 * grows, and never by blanket-appending the parameter to every request.
 *
 * `client.test.ts` parses `openapi.yaml` and fails if the two disagree in
 * either direction, so adding `branchScope` to an operation without adding it
 * here is a red test rather than a silent mainline read.
 */
const BRANCH_SCOPED_OPERATIONS: ReadonlyArray<{
	readonly methods: readonly string[];
	readonly pattern: RegExp;
}> = [
	{ methods: ['GET', 'POST'], pattern: new RegExp('^/persons$') },
	{ methods: ['GET', 'PUT', 'DELETE'], pattern: new RegExp(`^/persons/${UUID_SEGMENT}$`) },
	{ methods: ['GET', 'POST'], pattern: new RegExp(`^/persons/${UUID_SEGMENT}/names$`) },
	{
		methods: ['PUT', 'DELETE'],
		pattern: new RegExp(`^/persons/${UUID_SEGMENT}/names/${UUID_SEGMENT}$`)
	},
	// GET /families (the list) follows the branch since #829.
	{ methods: ['GET', 'POST'], pattern: new RegExp('^/families$') },
	{ methods: ['GET', 'PUT', 'DELETE'], pattern: new RegExp(`^/families/${UUID_SEGMENT}$`) },
	{ methods: ['POST'], pattern: new RegExp(`^/families/${UUID_SEGMENT}/children$`) },
	{
		methods: ['DELETE'],
		pattern: new RegExp(`^/families/${UUID_SEGMENT}/children/${UUID_SEGMENT}$`)
	},
	{ methods: ['GET'], pattern: new RegExp(`^/pedigree/${UUID_SEGMENT}$`) },
	// Search and the kinship reads (#829): the header SearchBox, PersonSelector
	// and /search, the group sheet, the Ahnentafel (JSON and text), descendancy
	// and the relationship calculator.
	{ methods: ['GET'], pattern: new RegExp('^/search$') },
	{ methods: ['GET'], pattern: new RegExp(`^/families/${UUID_SEGMENT}/group-sheet$`) },
	{ methods: ['GET'], pattern: new RegExp(`^/ahnentafel/${UUID_SEGMENT}$`) },
	{ methods: ['GET'], pattern: new RegExp(`^/descendancy/${UUID_SEGMENT}$`) },
	{ methods: ['GET'], pattern: new RegExp(`^/relationship/${UUID_SEGMENT}/${UUID_SEGMENT}$`) },
	// The quality overview (#894): the analytics page's totals follow the branch.
	{ methods: ['GET'], pattern: new RegExp('^/quality/overview$') },
	// Person merge (#834): a merge made on a branch lands there only.
	{ methods: ['POST'], pattern: new RegExp('^/persons/merge$') },
	{ methods: ['POST'], pattern: new RegExp('^/persons/merge/batch$') },
	// Browse and map aggregates (#756), plus the cemetery index (#757).
	{ methods: ['GET'], pattern: new RegExp('^/browse/surnames$') },
	{ methods: ['GET'], pattern: new RegExp(`^/browse/surnames/${TEXT_SEGMENT}/persons$`) },
	{ methods: ['GET'], pattern: new RegExp('^/browse/places$') },
	{ methods: ['GET'], pattern: new RegExp(`^/browse/places/${TEXT_SEGMENT}/persons$`) },
	{ methods: ['GET'], pattern: new RegExp('^/browse/cemeteries$') },
	{ methods: ['GET'], pattern: new RegExp(`^/browse/cemeteries/${TEXT_SEGMENT}/persons$`) },
	{ methods: ['GET'], pattern: new RegExp('^/map/locations$') },
	// Person/family facts (#757).
	{ methods: ['GET', 'POST'], pattern: new RegExp('^/associations$') },
	{ methods: ['GET', 'PUT', 'DELETE'], pattern: new RegExp(`^/associations/${UUID_SEGMENT}$`) },
	{ methods: ['GET'], pattern: new RegExp(`^/persons/${UUID_SEGMENT}/associations$`) },
	// Evidence (#758): sources, citations and notes.
	{ methods: ['GET', 'POST'], pattern: new RegExp('^/sources$') },
	{ methods: ['GET'], pattern: new RegExp('^/sources/search$') },
	{ methods: ['GET', 'PUT', 'DELETE'], pattern: new RegExp(`^/sources/${UUID_SEGMENT}$`) },
	{ methods: ['GET'], pattern: new RegExp(`^/sources/${UUID_SEGMENT}/citations$`) },
	{ methods: ['POST'], pattern: new RegExp('^/citations$') },
	{ methods: ['GET', 'PUT', 'DELETE'], pattern: new RegExp(`^/citations/${UUID_SEGMENT}$`) },
	{ methods: ['GET'], pattern: new RegExp(`^/citations/${UUID_SEGMENT}/format$`) },
	{ methods: ['GET'], pattern: new RegExp(`^/persons/${UUID_SEGMENT}/citations$`) },
	{ methods: ['GET', 'POST'], pattern: new RegExp('^/notes$') },
	{ methods: ['GET', 'PUT', 'DELETE'], pattern: new RegExp(`^/notes/${UUID_SEGMENT}$`) },
	// Media metadata (#759). The file bytes are shared with the mainline, but the
	// content and thumbnail reads still take the scope: a branch-deleted item
	// must 404 there too.
	{ methods: ['GET', 'POST'], pattern: new RegExp(`^/persons/${UUID_SEGMENT}/media$`) },
	{ methods: ['GET', 'PUT', 'DELETE'], pattern: new RegExp(`^/media/${UUID_SEGMENT}$`) },
	{ methods: ['GET'], pattern: new RegExp(`^/media/${UUID_SEGMENT}/content$`) },
	{ methods: ['GET'], pattern: new RegExp(`^/media/${UUID_SEGMENT}/thumbnail$`) },
	// GPS artifacts (#760): evidence analyses, evidence conflicts, research logs
	// and proof summaries.
	{ methods: ['GET', 'POST'], pattern: new RegExp('^/evidence-analyses$') },
	{ methods: ['GET'], pattern: new RegExp('^/evidence-analyses/by-fact$') },
	{ methods: ['GET', 'PUT', 'DELETE'], pattern: new RegExp(`^/evidence-analyses/${UUID_SEGMENT}$`) },
	{ methods: ['GET'], pattern: new RegExp('^/evidence-conflicts$') },
	{ methods: ['GET'], pattern: new RegExp(`^/evidence-conflicts/${UUID_SEGMENT}$`) },
	{ methods: ['POST'], pattern: new RegExp(`^/evidence-conflicts/${UUID_SEGMENT}/resolve$`) },
	{ methods: ['GET'], pattern: new RegExp(`^/evidence-conflicts/by-subject/${UUID_SEGMENT}$`) },
	{ methods: ['GET', 'POST'], pattern: new RegExp('^/research-logs$') },
	{ methods: ['GET', 'PUT', 'DELETE'], pattern: new RegExp(`^/research-logs/${UUID_SEGMENT}$`) },
	{ methods: ['GET'], pattern: new RegExp(`^/research-logs/by-subject/${UUID_SEGMENT}$`) },
	{ methods: ['GET', 'POST'], pattern: new RegExp('^/proof-summaries$') },
	{ methods: ['GET'], pattern: new RegExp('^/proof-summaries/by-fact$') },
	{ methods: ['GET', 'PUT', 'DELETE'], pattern: new RegExp(`^/proof-summaries/${UUID_SEGMENT}$`) },
	// Person and family history (#823, #824): the branch's view of the stream,
	// each entry labelled with its `origin` (the branch's own, or inherited from
	// the mainline).
	{ methods: ['GET'], pattern: new RegExp(`^/persons/${UUID_SEGMENT}/history$`) },
	{ methods: ['GET'], pattern: new RegExp(`^/families/${UUID_SEGMENT}/history$`) },
	// Restore points and rollback declare the scope only so the server can
	// REFUSE it (409 `rollback_mainline_only`, #824): rollback stays mainline-only
	// (ADR-005). The pages hide these controls on a branch; forwarding the scope
	// makes any call that slips through fail instead of rewriting the mainline.
	{
		methods: ['GET'],
		pattern: new RegExp(
			`^/(persons|families|sources|citations)/${UUID_SEGMENT}/restore-points$`
		)
	},
	{
		methods: ['POST'],
		pattern: new RegExp(`^/(persons|families|sources|citations)/${UUID_SEGMENT}/rollback$`)
	},
	// Research snapshots (#839): a snapshot marks (branch, position), so every
	// snapshot operation answers for the active branch's snapshots only, and a
	// comparison reads the branch's view of the log.
	{ methods: ['GET', 'POST'], pattern: new RegExp('^/snapshots$') },
	{ methods: ['GET', 'DELETE'], pattern: new RegExp(`^/snapshots/${UUID_SEGMENT}$`) },
	{ methods: ['GET'], pattern: new RegExp(`^/snapshots/${UUID_SEGMENT}/compare-current$`) },
	{
		methods: ['GET'],
		pattern: new RegExp(`^/snapshots/${UUID_SEGMENT}/compare/${UUID_SEGMENT}$`)
	}
];

/**
 * True when `method path` is one of the operations that accepts `?branch=`.
 * Exported for unit testing — the allowlist is the safety property that keeps
 * the client from sending a scope parameter an endpoint would reject or ignore.
 */
export function isBranchScopedRequest(method: string, path: string): boolean {
	const pathname = path.split('?')[0];
	const verb = method.toUpperCase();
	return BRANCH_SCOPED_OPERATIONS.some(
		(op) => op.methods.includes(verb) && op.pattern.test(pathname)
	);
}

/**
 * Append `?branch=<id>` when a branch is active and the operation accepts it.
 * With no active branch the path is returned untouched — not one byte of a
 * mainline request changes.
 */
function withBranchScope(method: string, path: string): string {
	if (activeBranchId === null || !isBranchScopedRequest(method, path)) {
		return path;
	}
	// A caller that names a branch explicitly (e.g. the branch page reading the
	// branch it shows, whichever one is active) keeps its own scope.
	if (/[?&]branch=/.test(path)) {
		return path;
	}
	const separator = path.includes('?') ? '&' : '?';
	return `${path}${separator}branch=${encodeURIComponent(activeBranchId)}`;
}

type BranchWriteListener = (branchId: string) => void;
const branchWriteListeners = new Set<BranchWriteListener>();

/**
 * Subscribe to successful writes that landed on the active branch. The branch
 * banner uses it to re-read its "main moved" counts (#837): a write to a new
 * entity on the branch changes which of main's changes Compare will list.
 * Returns the unsubscribe function.
 */
export function onBranchWrite(listener: BranchWriteListener): () => void {
	branchWriteListeners.add(listener);
	return () => {
		branchWriteListeners.delete(listener);
	};
}

/**
 * Tell subscribers a write succeeded on `branchId`, if `method path` was a
 * branch-scoped write. Reads and mainline requests notify nobody. The id is the
 * scope the request was sent with, captured before it went out, so a scope
 * switch mid-flight cannot mislabel it.
 */
function notifyBranchWrite(branchId: string | null, method: string, path: string): void {
	if (branchId === null || method.toUpperCase() === 'GET' || !isBranchScopedRequest(method, path)) {
		return;
	}
	for (const listener of branchWriteListeners) {
		listener(branchId);
	}
}

// Types generated from the OpenAPI schemas (single source of truth). Several
// keep the names the client used before it adopted the generated types.
type Schemas = components['schemas'];

// Enumerations, mirrored from internal/domain/enums.go by the spec.
export type Gender = Schemas['Gender'];
export type RelationType = Schemas['RelationType'];
export type ChildRelationType = Schemas['ChildRelationType'];
export type SourceType = Schemas['SourceType'];
export type SourceQuality = Schemas['SourceQuality'];
export type InformantType = Schemas['InformantType'];
export type EvidenceType = Schemas['EvidenceType'];
export type MediaType = Schemas['MediaType'];
export type NameType = Schemas['NameType'];
export type ResearchStatus = Schemas['ResearchStatus'];
export type ConflictStatus = Schemas['ConflictStatus'];
export type ResearchOutcome = Schemas['ResearchOutcome'];
export type FactType = Schemas['FactType'];

export type GenDate = Schemas['GenDate'];

// Persons and families
export type Person = Schemas['Person'];
export type PersonCreate = Schemas['PersonCreate'];
export type PersonUpdate = Schemas['PersonUpdate'];
export type PersonSummary = Schemas['PersonSummary'];
export type PersonDetail = Schemas['PersonDetail'];
export type PersonList = Schemas['PersonList'];
export type FamilySummary = Schemas['FamilySummary'];
/** A GEDCOM 7.0 external identifier (EXID) with a server-resolved label and, for recognized systems, a URL. */
export type ExternalLink = Schemas['ExternalLink'];
export type Family = Schemas['Family'];
export type FamilyCreate = Schemas['FamilyCreate'];
export type FamilyUpdate = Schemas['FamilyUpdate'];
export type FamilyChild = Schemas['FamilyChild'];
export type FamilyDetail = Schemas['FamilyDetail'];
export type FamilyList = Schemas['FamilyList'];
export type AddChild = Schemas['AddChild'];

// Family group sheet
export type GroupSheetCitation = Schemas['GroupSheetCitation'];
export type GroupSheetEvent = Schemas['GroupSheetEvent'];
export type GroupSheetPerson = Schemas['GroupSheetPerson'];
export type GroupSheetChild = Schemas['GroupSheetChild'];
export type FamilyGroupSheet = Schemas['FamilyGroupSheet'];

// Charts
export type PedigreeNode = Schemas['PedigreeNode'];
export type Pedigree = Schemas['Pedigree'];
export type SpouseInfo = Schemas['SpouseInfo'];
export type DescendancyNode = Schemas['DescendancyNode'];
export type Descendancy = Schemas['Descendancy'];

// Search
export type SearchResult = Schemas['SearchResult'];
export type SearchResults = Schemas['SearchResults'];

// GEDCOM import and export
export type ImportWarning = Schemas['ImportWarning'];
export type ImportError = Schemas['ImportError'];
export type ImportResult = Schemas['ImportResult'];
export type ExportEstimate = Schemas['ExportEstimate'];
export type ExportPreview = Schemas['ExportPreview'];
export type DataLossItem = Schemas['DataLossItem'];
/** GEDCOM versions the export/preview endpoints accept. */
export type GedcomVersion = NonNullable<
	NonNullable<operations['exportGedcom']['parameters']['query']>['version']
>;

// Client-side progress reports for streamed imports and exports (not API schemas).
export interface ImportProgress {
	bytes_read: number;
	total_bytes: number; // -1 when unknown
	percent: number; // 0-100, or -1 when total is unknown
}

export interface ExportProgress {
	phase: string;
	current: number;
	total: number;
	percentage: number;
}

/** The API's error body, plus the HTTP status the client saw it with. */
export type ApiError = Schemas['Error'] & { status?: number };

// GPS evidence: analyses, conflicts, research logs and proof summaries
export type EvidenceAnalysisResponse = Schemas['EvidenceAnalysis'];
export type EvidenceAnalysisCreateRequest = Schemas['EvidenceAnalysisCreate'];
export type EvidenceAnalysisUpdateRequest = Schemas['EvidenceAnalysisUpdate'];
export type EvidenceAnalysisListResponse = Schemas['EvidenceAnalysisList'];
export type EvidenceConflictResponse = Schemas['EvidenceConflict'];
export type EvidenceConflictResolveRequest = Schemas['EvidenceConflictResolve'];
export type EvidenceConflictListResponse = Schemas['EvidenceConflictList'];
export type ResearchLogResponse = Schemas['ResearchLog'];
export type ResearchLogCreateRequest = Schemas['ResearchLogCreate'];
export type ResearchLogUpdateRequest = Schemas['ResearchLogUpdate'];
export type ResearchLogListResponse = Schemas['ResearchLogList'];
export type ProofSummaryResponse = Schemas['ProofSummary'];
export type ProofSummaryCreateRequest = Schemas['ProofSummaryCreate'];
export type ProofSummaryUpdateRequest = Schemas['ProofSummaryUpdate'];
export type ProofSummaryListResponse = Schemas['ProofSummaryList'];

// Sources and citations
export type Source = Schemas['Source'];
export type SourceDetail = Schemas['SourceDetail'];
export type SourceListResponse = Schemas['SourceList'];
export type CreateSourceRequest = Schemas['SourceCreate'];
export type UpdateSourceRequest = Schemas['SourceUpdate'];
export type SourceSearchResponse = Schemas['SourceSearchResults'];
export type Citation = Schemas['Citation'];
export type CitationListResponse = Schemas['CitationList'];
export type CreateCitationRequest = Schemas['CitationCreate'];
export type UpdateCitationRequest = Schemas['CitationUpdate'];

// Person names
export type PersonName = Schemas['PersonName'];
export type PersonNameCreate = Schemas['PersonNameCreate'];
export type PersonNameUpdate = Schemas['PersonNameUpdate'];
export type PersonNameList = Schemas['PersonNameList'];

// Media
export type Media = Schemas['Media'];
export type MediaListResponse = Schemas['MediaList'];
export type MediaUpdate = Schemas['MediaUpdate'];

// Relationship calculator
export type RelationshipPathNode = Schemas['RelationshipPathNode'];
export type RelationshipPath = Schemas['RelationshipPath'];
export type RelationshipResult = Schemas['RelationshipResult'];

// Browse indexes, map and brick walls
export type SurnameIndexResponse = Schemas['SurnameIndexResponse'];
export type SurnameEntry = Schemas['SurnameEntry'];
export type LetterCount = Schemas['LetterCount'];
export type PlaceIndexResponse = Schemas['PlaceIndexResponse'];
export type PlaceEntry = Schemas['PlaceEntry'];
export type CemeteryIndexResponse = Schemas['CemeteryIndexResponse'];
export type CemeteryEntry = Schemas['CemeteryEntry'];
export type MapLocationsResponse = Schemas['MapLocationsResponse'];
export type MapLocation = Schemas['MapLocation'];
export type BrickWallEntry = Schemas['BrickWallEntry'];
export type BrickWallsResponse = Schemas['BrickWallsResponse'];

// Discovery feed
export type DiscoverySuggestion = Schemas['DiscoverySuggestion'];
export type DiscoveryFeedResponse = Schemas['DiscoveryFeedResponse'];

// History
export type FieldChange = Schemas['FieldChange'];
export type ChangeEntry = Schemas['ChangeEntry'];
export type ChangeHistoryResponse = Schemas['ChangeHistoryResponse'];

/**
 * The genealogy API.
 *
 * ## Ambient branch scope
 *
 * Many of the methods below are **branch-scoped**: they answer from, and
 * write to, whichever research branch is currently active rather than the
 * mainline. That branch is module-level state, set by
 * `setClientBranch`/`getClientBranch` and driven in practice by `switchBranch`
 * in `$lib/stores/activeBranch.svelte`. Nothing about a call site shows it —
 * `api.getPerson(id)` returns branch or mainline data depending on state some
 * other file set, so a scoped method's result is only as well-defined as the
 * scope in force when it runs.
 *
 * `BRANCH_SCOPED_OPERATIONS` above is the authoritative list, mirrored from the
 * `branchScope` parameter in `internal/api/openapi.yaml` and pinned to it by a
 * test in `client.test.ts`. Every other method here is mainline-only whatever
 * branch is active — including the brick-wall setters, whose pages therefore
 * withdraw their controls while a branch is active. The
 * restore-point and rollback methods forward the scope only for the server to
 * refuse it (rollback is mainline-only, ADR-005), so their pages withdraw those
 * controls too. Individual scoped methods are marked below.
 */
class ApiClient {
	private async request<T>(
		method: string,
		path: string,
		body?: unknown,
		headers?: Record<string, string>
	): Promise<T> {
		const scope = activeBranchId;
		const url = `${API_BASE}${withBranchScope(method, path)}`;
		const options: RequestInit = {
			method,
			headers: {
				'Content-Type': 'application/json',
				...headers
			}
		};

		if (body !== undefined) {
			options.body = JSON.stringify(body);
		}

		const response = await fetch(url, options);

		if (!response.ok) {
			const error: ApiError = await response.json().catch(() => ({
				code: 'UNKNOWN_ERROR',
				message: response.statusText
			}));
			error.status = response.status;
			throw error;
		}

		notifyBranchWrite(scope, method, path);

		if (response.status === 204) {
			return undefined as T;
		}

		return response.json();
	}

	/**
	 * Execute a request with automatic retry on 409 Conflict.
	 * Person sub-resources share the Person aggregate's version counter,
	 * so conflicts between unrelated operations are common. A simple
	 * retry usually succeeds because the backend re-reads the current version.
	 */
	private async requestWithConflictRetry<T>(
		method: string,
		path: string,
		body?: unknown,
		headers?: Record<string, string>
	): Promise<T> {
		try {
			return await this.request<T>(method, path, body, headers);
		} catch (error) {
			const apiError = error as ApiError;
			if (apiError.status === 409) {
				await new Promise((resolve) => setTimeout(resolve, 100));
				try {
					return await this.request<T>(method, path, body, headers);
				} catch (retryError) {
					const retryApiError = retryError as ApiError;
					if (retryApiError.status === 409) {
						retryApiError.message =
							'This record was modified by another operation. Please try again.';
						retryApiError.code = 'CONFLICT_RETRY_FAILED';
					}
					throw retryApiError;
				}
			}
			throw error;
		}
	}

	// Person endpoints
	/** Branch-scoped: honors the active branch (see `BRANCH_SCOPED_OPERATIONS`). */
	async listPersons(params?: {
		limit?: number;
		offset?: number;
		sort?: 'surname' | 'given_name' | 'birth_date' | 'updated_at';
		order?: 'asc' | 'desc';
		research_status?: ResearchStatus | 'unset';
	}): Promise<PersonList> {
		const searchParams = new URLSearchParams();
		if (params?.limit) searchParams.set('limit', params.limit.toString());
		if (params?.offset) searchParams.set('offset', params.offset.toString());
		if (params?.sort) searchParams.set('sort', params.sort);
		if (params?.order) searchParams.set('order', params.order);
		if (params?.research_status) searchParams.set('research_status', params.research_status);

		const query = searchParams.toString();
		return this.request<PersonList>('GET', `/persons${query ? `?${query}` : ''}`);
	}

	/**
	 * Branch-scoped: honors the active branch (see `BRANCH_SCOPED_OPERATIONS`),
	 * unless `options.branch` names the branch to read instead - the branch
	 * research editor reads people as the branch it edits sees them, whichever
	 * branch is active.
	 */
	async getPerson(id: string, options?: { branch?: string }): Promise<PersonDetail> {
		const query = options?.branch ? `?branch=${encodeURIComponent(options.branch)}` : '';
		return this.request<PersonDetail>('GET', `/persons/${id}${query}`);
	}

	/** Branch-scoped: honors the active branch (see `BRANCH_SCOPED_OPERATIONS`). */
	async createPerson(data: PersonCreate): Promise<Person> {
		return this.request<Person>('POST', '/persons', data);
	}

	/** Branch-scoped: honors the active branch (see `BRANCH_SCOPED_OPERATIONS`). */
	async updatePerson(id: string, data: PersonUpdate): Promise<Person> {
		return this.request<Person>('PUT', `/persons/${id}`, data);
	}

	/** Branch-scoped: honors the active branch (see `BRANCH_SCOPED_OPERATIONS`). */
	async deletePerson(id: string): Promise<void> {
		return this.request<void>('DELETE', `/persons/${id}`);
	}

	// Family endpoints
	/** Branch-scoped: honors the active branch (see `BRANCH_SCOPED_OPERATIONS`). */
	async listFamilies(params?: { limit?: number; offset?: number }): Promise<FamilyList> {
		const searchParams = new URLSearchParams();
		if (params?.limit) searchParams.set('limit', params.limit.toString());
		if (params?.offset) searchParams.set('offset', params.offset.toString());

		const query = searchParams.toString();
		return this.request<FamilyList>('GET', `/families${query ? `?${query}` : ''}`);
	}

	/** Branch-scoped: honors the active branch (see `BRANCH_SCOPED_OPERATIONS`). */
	async getFamily(id: string): Promise<FamilyDetail> {
		return this.request<FamilyDetail>('GET', `/families/${id}`);
	}

	/** Branch-scoped: honors the active branch (see `BRANCH_SCOPED_OPERATIONS`). */
	async createFamily(data: FamilyCreate): Promise<Family> {
		return this.request<Family>('POST', '/families', data);
	}

	/** Branch-scoped: honors the active branch (see `BRANCH_SCOPED_OPERATIONS`). */
	async updateFamily(id: string, data: FamilyUpdate): Promise<Family> {
		return this.request<Family>('PUT', `/families/${id}`, data);
	}

	/** Branch-scoped: honors the active branch (see `BRANCH_SCOPED_OPERATIONS`). */
	async deleteFamily(id: string): Promise<void> {
		return this.request<void>('DELETE', `/families/${id}`);
	}

	/** Branch-scoped: honors the active branch (see `BRANCH_SCOPED_OPERATIONS`). */
	async addChildToFamily(familyId: string, data: AddChild): Promise<FamilyChild> {
		return this.request<FamilyChild>('POST', `/families/${familyId}/children`, data);
	}

	/** Branch-scoped: honors the active branch (see `BRANCH_SCOPED_OPERATIONS`). */
	async removeChildFromFamily(familyId: string, personId: string): Promise<void> {
		return this.request<void>('DELETE', `/families/${familyId}/children/${personId}`);
	}

	/** Branch-scoped: honors the active branch (see `BRANCH_SCOPED_OPERATIONS`). */
	async getFamilyGroupSheet(id: string): Promise<FamilyGroupSheet> {
		return this.request<FamilyGroupSheet>('GET', `/families/${id}/group-sheet`);
	}

	// Pedigree endpoint
	/** Branch-scoped: honors the active branch (see `BRANCH_SCOPED_OPERATIONS`). */
	async getPedigree(personId: string, generations?: number): Promise<Pedigree> {
		const params = generations ? `?generations=${generations}` : '';
		return this.request<Pedigree>('GET', `/pedigree/${personId}${params}`);
	}

	// Ahnentafel endpoint
	/** Branch-scoped: honors the active branch (see `BRANCH_SCOPED_OPERATIONS`). */
	async getAhnentafel(personId: string, generations?: number): Promise<AhnentafelResponse> {
		const params = generations ? `?generations=${generations}` : '';
		return this.request<AhnentafelResponse>('GET', `/ahnentafel/${personId}${params}`);
	}

	/**
	 * Branch-scoped: honors the active branch (see `BRANCH_SCOPED_OPERATIONS`).
	 * Fetched directly because the body is text, so the scope is added here.
	 */
	async getAhnentafelText(personId: string, generations?: number): Promise<string> {
		const params = new URLSearchParams();
		params.set('format', 'text');
		if (generations) params.set('generations', generations.toString());

		const response = await fetch(
			`${API_BASE}${withBranchScope('GET', `/ahnentafel/${personId}?${params.toString()}`)}`
		);

		if (!response.ok) {
			const error: ApiError = await response.json().catch(() => ({
				code: 'UNKNOWN_ERROR',
				message: response.statusText
			}));
			error.status = response.status;
			throw error;
		}

		return response.text();
	}

	// Descendancy endpoint
	/** Branch-scoped: honors the active branch (see `BRANCH_SCOPED_OPERATIONS`). */
	async getDescendancy(personId: string, generations?: number): Promise<Descendancy> {
		const params = generations ? `?generations=${generations}` : '';
		return this.request<Descendancy>('GET', `/descendancy/${personId}${params}`);
	}

	// Search endpoint
	/** Branch-scoped: honors the active branch (see `BRANCH_SCOPED_OPERATIONS`). */
	async searchPersons(params: {
		q?: string;
		fuzzy?: boolean;
		soundex?: boolean;
		birth_date_from?: string;
		birth_date_to?: string;
		death_date_from?: string;
		death_date_to?: string;
		birth_place?: string;
		death_place?: string;
		sort?: 'relevance' | 'name' | 'birth_date' | 'death_date';
		order?: 'asc' | 'desc';
		limit?: number;
	}): Promise<SearchResults> {
		const searchParams = new URLSearchParams();
		if (params.q) searchParams.set('q', params.q);
		if (params.fuzzy) searchParams.set('fuzzy', 'true');
		if (params.soundex) searchParams.set('soundex', 'true');
		if (params.birth_date_from) searchParams.set('birth_date_from', params.birth_date_from);
		if (params.birth_date_to) searchParams.set('birth_date_to', params.birth_date_to);
		if (params.death_date_from) searchParams.set('death_date_from', params.death_date_from);
		if (params.death_date_to) searchParams.set('death_date_to', params.death_date_to);
		if (params.birth_place) searchParams.set('birth_place', params.birth_place);
		if (params.death_place) searchParams.set('death_place', params.death_place);
		if (params.sort) searchParams.set('sort', params.sort);
		if (params.order) searchParams.set('order', params.order);
		if (params.limit) searchParams.set('limit', params.limit.toString());

		return this.request<SearchResults>('GET', `/search?${searchParams.toString()}`);
	}

	// GEDCOM endpoints
	async importGedcom(file: File): Promise<ImportResult> {
		const formData = new FormData();
		formData.append('file', file);

		const response = await fetch(`${API_BASE}/gedcom/import`, {
			method: 'POST',
			body: formData
		});

		if (!response.ok) {
			const error: ApiError = await response.json().catch(() => ({
				code: 'UNKNOWN_ERROR',
				message: response.statusText
			}));
			error.status = response.status;
			throw error;
		}

		return response.json();
	}

	/**
	 * Import a GEDCOM file while receiving real-time parse progress via Server-Sent
	 * Events. onProgress is invoked with each progress update; the resolved value is
	 * the final import result. Falls back cleanly if the browser/stream provides no
	 * progress events (onProgress simply won't be called).
	 */
	async importGedcomStream(
		file: File,
		onProgress?: (progress: ImportProgress) => void
	): Promise<ImportResult> {
		const formData = new FormData();
		formData.append('file', file);

		const response = await fetch(`${API_BASE}/gedcom/import/stream`, {
			method: 'POST',
			body: formData
		});

		if (!response.ok || !response.body) {
			const error: ApiError = await response.json().catch(() => ({
				code: 'UNKNOWN_ERROR',
				message: response.statusText
			}));
			error.status = response.status;
			throw error;
		}

		const reader = response.body.getReader();
		const decoder = new TextDecoder();
		let buffer = '';
		let result: ImportResult | null = null;
		let streamError: string | null = null;

		// Parse the SSE stream: events are separated by a blank line and consist of
		// `event: <name>` and `data: <json>` lines.
		const handleEvent = (raw: string) => {
			let eventName = 'message';
			const dataLines: string[] = [];
			for (const line of raw.split('\n')) {
				if (line.startsWith('event:')) eventName = line.slice(6).trim();
				else if (line.startsWith('data:')) dataLines.push(line.slice(5).trim());
			}
			if (dataLines.length === 0) return;
			const data = JSON.parse(dataLines.join('\n'));
			if (eventName === 'progress') {
				onProgress?.(data as ImportProgress);
			} else if (eventName === 'result') {
				result = data as ImportResult;
			} else if (eventName === 'error') {
				streamError = (data as { message?: string }).message || 'Import failed';
			}
		};

		for (;;) {
			const { done, value } = await reader.read();
			if (done) break;
			buffer += decoder.decode(value, { stream: true });
			let sep: number;
			// Process complete events (delimited by a blank line).
			while ((sep = buffer.indexOf('\n\n')) !== -1) {
				const rawEvent = buffer.slice(0, sep);
				buffer = buffer.slice(sep + 2);
				if (rawEvent.trim()) handleEvent(rawEvent);
			}
		}
		// Flush any trailing event without a terminating blank line.
		if (buffer.trim()) handleEvent(buffer);

		if (streamError !== null) {
			throw { code: 'IMPORT_ERROR', message: streamError } as ApiError;
		}
		if (result === null) {
			throw { code: 'IMPORT_ERROR', message: 'Import stream ended without a result' } as ApiError;
		}
		return result;
	}

	async exportGedcom(version?: GedcomVersion): Promise<string> {
		const query = version ? `?version=${encodeURIComponent(version)}` : '';
		const response = await fetch(`${API_BASE}/gedcom/export${query}`);

		if (!response.ok) {
			const error: ApiError = await response.json().catch(() => ({
				code: 'UNKNOWN_ERROR',
				message: response.statusText
			}));
			error.status = response.status;
			throw error;
		}

		return response.text();
	}

	/**
	 * Preview a GEDCOM export at the given version, reporting any data loss,
	 * without producing a file. Backed by a full server-side export build, so
	 * call it when the resolved version selection changes — not on every
	 * keystroke or render. Pass an AbortSignal to cancel a superseded request.
	 */
	async previewGedcomExport(version?: GedcomVersion, signal?: AbortSignal): Promise<ExportPreview> {
		const query = version ? `?version=${encodeURIComponent(version)}` : '';
		const response = await fetch(`${API_BASE}/gedcom/export/preview${query}`, { signal });

		if (!response.ok) {
			const error: ApiError = await response.json().catch(() => ({
				code: 'UNKNOWN_ERROR',
				message: response.statusText
			}));
			error.status = response.status;
			throw error;
		}

		return response.json();
	}

	async exportTree(): Promise<string> {
		const response = await fetch(`${API_BASE}/export/tree`);

		if (!response.ok) {
			const error: ApiError = await response.json().catch(() => ({
				code: 'UNKNOWN_ERROR',
				message: response.statusText
			}));
			error.status = response.status;
			throw error;
		}

		return response.text();
	}

	async exportPersons(format: 'json' | 'csv', fields?: string[]): Promise<string> {
		const params = new URLSearchParams({ format });
		if (fields?.length) params.set('fields', fields.join(','));

		const response = await fetch(`${API_BASE}/export/persons?${params}`);

		if (!response.ok) {
			const error: ApiError = await response.json().catch(() => ({
				code: 'UNKNOWN_ERROR',
				message: response.statusText
			}));
			error.status = response.status;
			throw error;
		}

		return response.text();
	}

	async exportFamilies(format: 'json' | 'csv', fields?: string[]): Promise<string> {
		const params = new URLSearchParams({ format });
		if (fields?.length) params.set('fields', fields.join(','));

		const response = await fetch(`${API_BASE}/export/families?${params}`);

		if (!response.ok) {
			const error: ApiError = await response.json().catch(() => ({
				code: 'UNKNOWN_ERROR',
				message: response.statusText
			}));
			error.status = response.status;
			throw error;
		}

		return response.text();
	}

	async exportSources(format: 'json' | 'csv', fields?: string[]): Promise<string> {
		const params = new URLSearchParams({ format });
		if (fields?.length) params.set('fields', fields.join(','));

		const response = await fetch(`${API_BASE}/export/sources?${params}`);

		if (!response.ok) {
			const error: ApiError = await response.json().catch(() => ({
				code: 'UNKNOWN_ERROR',
				message: response.statusText
			}));
			error.status = response.status;
			throw error;
		}

		return response.text();
	}

	async exportCitations(format: 'json' | 'csv', fields?: string[]): Promise<string> {
		const params = new URLSearchParams({ format });
		if (fields?.length) params.set('fields', fields.join(','));

		const response = await fetch(`${API_BASE}/export/citations?${params}`);

		if (!response.ok) {
			const error: ApiError = await response.json().catch(() => ({
				code: 'UNKNOWN_ERROR',
				message: response.statusText
			}));
			error.status = response.status;
			throw error;
		}

		return response.text();
	}

	async exportEvents(format: 'json' | 'csv', fields?: string[]): Promise<string> {
		const params = new URLSearchParams({ format });
		if (fields?.length) params.set('fields', fields.join(','));

		const response = await fetch(`${API_BASE}/export/events?${params}`);

		if (!response.ok) {
			const error: ApiError = await response.json().catch(() => ({
				code: 'UNKNOWN_ERROR',
				message: response.statusText
			}));
			error.status = response.status;
			throw error;
		}

		return response.text();
	}

	async exportAttributes(format: 'json' | 'csv', fields?: string[]): Promise<string> {
		const params = new URLSearchParams({ format });
		if (fields?.length) params.set('fields', fields.join(','));

		const response = await fetch(`${API_BASE}/export/attributes?${params}`);

		if (!response.ok) {
			const error: ApiError = await response.json().catch(() => ({
				code: 'UNKNOWN_ERROR',
				message: response.statusText
			}));
			error.status = response.status;
			throw error;
		}

		return response.text();
	}

	async getExportEstimate(): Promise<ExportEstimate> {
		return this.request<ExportEstimate>('GET', '/export/estimate');
	}

	// Source endpoints
	async listSources(params?: {
		limit?: number;
		offset?: number;
		sort?: string;
		order?: 'asc' | 'desc';
		q?: string;
	}): Promise<SourceListResponse> {
		const searchParams = new URLSearchParams();
		if (params?.limit) searchParams.set('limit', params.limit.toString());
		if (params?.offset) searchParams.set('offset', params.offset.toString());
		if (params?.sort) searchParams.set('sort', params.sort);
		if (params?.order) searchParams.set('order', params.order);
		if (params?.q) searchParams.set('q', params.q);

		const query = searchParams.toString();
		return this.request<SourceListResponse>('GET', `/sources${query ? `?${query}` : ''}`);
	}

	async getSource(id: string): Promise<SourceDetail> {
		return this.request<SourceDetail>('GET', `/sources/${id}`);
	}

	async createSource(data: CreateSourceRequest): Promise<Source> {
		return this.request<Source>('POST', '/sources', data);
	}

	async updateSource(id: string, data: UpdateSourceRequest): Promise<Source> {
		return this.request<Source>('PUT', `/sources/${id}`, data);
	}

	async deleteSource(id: string, version: number): Promise<void> {
		return this.request<void>('DELETE', `/sources/${id}?version=${version}`);
	}

	// Repository endpoints
	async listRepositories(params?: {
		limit?: number;
		offset?: number;
		sort?: 'name' | 'updated_at';
		order?: 'asc' | 'desc';
	}): Promise<RepositoryList> {
		const searchParams = new URLSearchParams();
		if (params?.limit) searchParams.set('limit', params.limit.toString());
		if (params?.offset) searchParams.set('offset', params.offset.toString());
		if (params?.sort) searchParams.set('sort', params.sort);
		if (params?.order) searchParams.set('order', params.order);

		const query = searchParams.toString();
		return this.request<RepositoryList>('GET', `/repositories${query ? `?${query}` : ''}`);
	}

	async getRepository(id: string): Promise<RepositoryDetail> {
		return this.request<RepositoryDetail>('GET', `/repositories/${id}`);
	}

	async createRepository(data: RepositoryCreate): Promise<Repository> {
		return this.request<Repository>('POST', '/repositories', data);
	}

	async updateRepository(id: string, data: RepositoryUpdate): Promise<Repository> {
		return this.request<Repository>('PUT', `/repositories/${id}`, data);
	}

	async deleteRepository(id: string, version: number): Promise<void> {
		return this.request<void>('DELETE', `/repositories/${id}?version=${version}`);
	}

	async searchSources(q: string, limit?: number): Promise<SourceSearchResponse> {
		const searchParams = new URLSearchParams();
		searchParams.set('q', q);
		if (limit) searchParams.set('limit', limit.toString());

		return this.request<SourceSearchResponse>('GET', `/sources/search?${searchParams.toString()}`);
	}

	// Citation endpoints
	async getPersonCitations(personId: string): Promise<CitationListResponse> {
		return this.request<CitationListResponse>('GET', `/persons/${personId}/citations`);
	}

	async createCitation(data: CreateCitationRequest): Promise<Citation> {
		return this.request<Citation>('POST', '/citations', data);
	}

	async updateCitation(id: string, data: UpdateCitationRequest): Promise<Citation> {
		return this.request<Citation>('PUT', `/citations/${id}`, data);
	}

	async deleteCitation(id: string, version: number): Promise<void> {
		return this.request<void>('DELETE', `/citations/${id}?version=${version}`);
	}

	// Citation template endpoints
	async listCitationTemplates(sourceType?: SourceType): Promise<CitationTemplateList> {
		const params = sourceType ? `?source_type=${encodeURIComponent(sourceType)}` : '';
		return this.request<CitationTemplateList>('GET', `/citation-templates${params}`);
	}

	async getCitationTemplate(id: string): Promise<CitationTemplate> {
		return this.request<CitationTemplate>('GET', `/citation-templates/${encodeURIComponent(id)}`);
	}

	async formatCitation(id: string): Promise<FormattedCitation> {
		return this.request<FormattedCitation>('GET', `/citations/${id}/format`);
	}

	async previewCitationTemplate(
		templateId: string,
		fields: Record<string, string>
	): Promise<FormattedCitation> {
		return this.request<FormattedCitation>(
			'POST',
			`/citation-templates/${encodeURIComponent(templateId)}/preview`,
			{ fields }
		);
	}

	// PersonName endpoints
	/** Branch-scoped: honors the active branch (see `BRANCH_SCOPED_OPERATIONS`). */
	async getPersonNames(personId: string): Promise<PersonNameList> {
		return this.request<PersonNameList>('GET', `/persons/${personId}/names`);
	}

	/** Branch-scoped: honors the active branch (see `BRANCH_SCOPED_OPERATIONS`). */
	async addPersonName(personId: string, data: PersonNameCreate): Promise<PersonName> {
		return this.requestWithConflictRetry<PersonName>('POST', `/persons/${personId}/names`, data);
	}

	/** Branch-scoped: honors the active branch (see `BRANCH_SCOPED_OPERATIONS`). */
	async updatePersonName(personId: string, nameId: string, data: PersonNameUpdate): Promise<PersonName> {
		return this.requestWithConflictRetry<PersonName>('PUT', `/persons/${personId}/names/${nameId}`, data);
	}

	/** Branch-scoped: honors the active branch (see `BRANCH_SCOPED_OPERATIONS`). */
	async deletePersonName(personId: string, nameId: string): Promise<void> {
		return this.requestWithConflictRetry<void>('DELETE', `/persons/${personId}/names/${nameId}`);
	}

	// Media endpoints
	async listPersonMedia(
		personId: string,
		params?: { limit?: number; offset?: number }
	): Promise<MediaListResponse> {
		const searchParams = new URLSearchParams();
		if (params?.limit) searchParams.set('limit', params.limit.toString());
		if (params?.offset) searchParams.set('offset', params.offset.toString());

		const query = searchParams.toString();
		return this.request<MediaListResponse>('GET', `/persons/${personId}/media${query ? `?${query}` : ''}`);
	}

	async uploadPersonMedia(
		personId: string,
		file: File,
		title: string,
		description?: string,
		mediaType?: MediaType
	): Promise<Media> {
		const formData = new FormData();
		formData.append('file', file);
		formData.append('title', title);
		if (description) formData.append('description', description);
		if (mediaType) formData.append('media_type', mediaType);

		// Raw fetch (multipart), so the branch scope is applied here rather than by
		// request().
		const scope = activeBranchId;
		const mediaPath = `/persons/${personId}/media`;
		const response = await fetch(`${API_BASE}${withBranchScope('POST', mediaPath)}`, {
			method: 'POST',
			body: formData
		});

		if (!response.ok) {
			const error: ApiError = await response.json().catch(() => ({
				code: 'UNKNOWN_ERROR',
				message: response.statusText
			}));
			error.status = response.status;
			throw error;
		}

		notifyBranchWrite(scope, 'POST', mediaPath);
		return response.json();
	}

	async getMedia(id: string): Promise<Media> {
		return this.request<Media>('GET', `/media/${id}`);
	}

	async updateMedia(id: string, data: MediaUpdate): Promise<Media> {
		return this.request<Media>('PUT', `/media/${id}`, data);
	}

	async deleteMedia(id: string, version: number): Promise<void> {
		return this.request<void>('DELETE', `/media/${id}?version=${version}`);
	}

	// URL builders for <img src> and download links, which bypass request(): they
	// carry the active branch scope themselves (#759).
	getMediaContentUrl(id: string): string {
		return `${API_BASE}${withBranchScope('GET', `/media/${id}/content`)}`;
	}

	getMediaThumbnailUrl(id: string): string {
		return `${API_BASE}${withBranchScope('GET', `/media/${id}/thumbnail`)}`;
	}

	// History endpoints
	async getGlobalHistory(params?: {
		entity_type?: string;
		from?: string;
		to?: string;
		limit?: number;
		offset?: number;
	}): Promise<ChangeHistoryResponse> {
		const searchParams = new URLSearchParams();
		if (params?.entity_type) searchParams.set('entity_type', params.entity_type);
		if (params?.from) searchParams.set('from', params.from);
		if (params?.to) searchParams.set('to', params.to);
		if (params?.limit) searchParams.set('limit', params.limit.toString());
		if (params?.offset) searchParams.set('offset', params.offset.toString());

		const query = searchParams.toString();
		return this.request<ChangeHistoryResponse>('GET', `/history${query ? `?${query}` : ''}`);
	}

	/** Branch-scoped: honors the active branch (see `BRANCH_SCOPED_OPERATIONS`). */
	async getPersonHistory(
		personId: string,
		params?: { limit?: number; offset?: number }
	): Promise<ChangeHistoryResponse> {
		const searchParams = new URLSearchParams();
		if (params?.limit) searchParams.set('limit', params.limit.toString());
		if (params?.offset) searchParams.set('offset', params.offset.toString());

		const query = searchParams.toString();
		return this.request<ChangeHistoryResponse>(
			'GET',
			`/persons/${personId}/history${query ? `?${query}` : ''}`
		);
	}

	/** Branch-scoped: honors the active branch (see `BRANCH_SCOPED_OPERATIONS`). */
	async getFamilyHistory(
		familyId: string,
		params?: { limit?: number; offset?: number }
	): Promise<ChangeHistoryResponse> {
		const searchParams = new URLSearchParams();
		if (params?.limit) searchParams.set('limit', params.limit.toString());
		if (params?.offset) searchParams.set('offset', params.offset.toString());

		const query = searchParams.toString();
		return this.request<ChangeHistoryResponse>(
			'GET',
			`/families/${familyId}/history${query ? `?${query}` : ''}`
		);
	}

	async getSourceHistory(
		sourceId: string,
		params?: { limit?: number; offset?: number }
	): Promise<ChangeHistoryResponse> {
		const searchParams = new URLSearchParams();
		if (params?.limit) searchParams.set('limit', params.limit.toString());
		if (params?.offset) searchParams.set('offset', params.offset.toString());

		const query = searchParams.toString();
		return this.request<ChangeHistoryResponse>(
			'GET',
			`/sources/${sourceId}/history${query ? `?${query}` : ''}`
		);
	}

	// Browse endpoints
	async getSurnameIndex(letter?: string): Promise<SurnameIndexResponse> {
		const params = letter ? `?letter=${encodeURIComponent(letter)}` : '';
		return this.request<SurnameIndexResponse>('GET', `/browse/surnames${params}`);
	}

	async getPersonsBySurname(
		surname: string,
		params?: { limit?: number; offset?: number }
	): Promise<PersonList> {
		const searchParams = new URLSearchParams();
		if (params?.limit) searchParams.set('limit', params.limit.toString());
		if (params?.offset) searchParams.set('offset', params.offset.toString());

		const query = searchParams.toString();
		return this.request<PersonList>(
			'GET',
			`/browse/surnames/${encodeURIComponent(surname)}/persons${query ? `?${query}` : ''}`
		);
	}

	async getPlaceHierarchy(parent?: string): Promise<PlaceIndexResponse> {
		const params = parent ? `?parent=${encodeURIComponent(parent)}` : '';
		return this.request<PlaceIndexResponse>('GET', `/browse/places${params}`);
	}

	async getPersonsByPlace(
		place: string,
		params?: { limit?: number; offset?: number }
	): Promise<PersonList> {
		const searchParams = new URLSearchParams();
		if (params?.limit) searchParams.set('limit', params.limit.toString());
		if (params?.offset) searchParams.set('offset', params.offset.toString());

		const query = searchParams.toString();
		return this.request<PersonList>(
			'GET',
			`/browse/places/${encodeURIComponent(place)}/persons${query ? `?${query}` : ''}`
		);
	}

	async getCemeteryIndex(): Promise<CemeteryIndexResponse> {
		return this.request<CemeteryIndexResponse>('GET', '/browse/cemeteries');
	}

	async getPersonsByCemetery(
		place: string,
		params?: { limit?: number; offset?: number }
	): Promise<PersonList> {
		const searchParams = new URLSearchParams();
		if (params?.limit) searchParams.set('limit', params.limit.toString());
		if (params?.offset) searchParams.set('offset', params.offset.toString());

		const query = searchParams.toString();
		return this.request<PersonList>(
			'GET',
			`/browse/cemeteries/${encodeURIComponent(place)}/persons${query ? `?${query}` : ''}`
		);
	}

	// Map endpoints
	async getMapLocations(): Promise<MapLocationsResponse> {
		return this.request<MapLocationsResponse>('GET', '/map/locations');
	}

	// Brick wall endpoints
	async getBrickWalls(includeResolved?: boolean): Promise<BrickWallsResponse> {
		const params = includeResolved ? '?include_resolved=true' : '';
		return this.request<BrickWallsResponse>('GET', `/browse/brick-walls${params}`);
	}

	async setPersonBrickWall(personId: string, note: string): Promise<void> {
		return this.request<void>('PUT', `/persons/${encodeURIComponent(personId)}/brick-wall`, {
			note
		});
	}

	async resolvePersonBrickWall(personId: string): Promise<void> {
		return this.request<void>('DELETE', `/persons/${encodeURIComponent(personId)}/brick-wall`);
	}

	// Relationship endpoint
	/** Branch-scoped: honors the active branch (see `BRANCH_SCOPED_OPERATIONS`). */
	async getRelationship(personId1: string, personId2: string): Promise<RelationshipResult> {
		return this.request<RelationshipResult>(
			'GET',
			`/relationship/${encodeURIComponent(personId1)}/${encodeURIComponent(personId2)}`
		);
	}

	// Rollback endpoints
	/** Mainline-only: on an active branch the server refuses it (409 `rollback_mainline_only`). */
	async getPersonRestorePoints(
		personId: string,
		params?: { limit?: number; offset?: number }
	): Promise<RestorePointsResponse> {
		const searchParams = new URLSearchParams();
		if (params?.limit) searchParams.set('limit', params.limit.toString());
		if (params?.offset) searchParams.set('offset', params.offset.toString());

		const query = searchParams.toString();
		return this.request<RestorePointsResponse>(
			'GET',
			`/persons/${personId}/restore-points${query ? `?${query}` : ''}`
		);
	}

	/** Mainline-only: on an active branch the server refuses it (409 `rollback_mainline_only`). */
	async rollbackPerson(personId: string, targetVersion: number): Promise<RollbackResponse> {
		return this.request<RollbackResponse>('POST', `/persons/${personId}/rollback`, {
			target_version: targetVersion
		});
	}

	/** Mainline-only: on an active branch the server refuses it (409 `rollback_mainline_only`). */
	async getFamilyRestorePoints(
		familyId: string,
		params?: { limit?: number; offset?: number }
	): Promise<RestorePointsResponse> {
		const searchParams = new URLSearchParams();
		if (params?.limit) searchParams.set('limit', params.limit.toString());
		if (params?.offset) searchParams.set('offset', params.offset.toString());

		const query = searchParams.toString();
		return this.request<RestorePointsResponse>(
			'GET',
			`/families/${familyId}/restore-points${query ? `?${query}` : ''}`
		);
	}

	/** Mainline-only: on an active branch the server refuses it (409 `rollback_mainline_only`). */
	async rollbackFamily(familyId: string, targetVersion: number): Promise<RollbackResponse> {
		return this.request<RollbackResponse>('POST', `/families/${familyId}/rollback`, {
			target_version: targetVersion
		});
	}

	/** Mainline-only: on an active branch the server refuses it (409 `rollback_mainline_only`). */
	async getSourceRestorePoints(
		sourceId: string,
		params?: { limit?: number; offset?: number }
	): Promise<RestorePointsResponse> {
		const searchParams = new URLSearchParams();
		if (params?.limit) searchParams.set('limit', params.limit.toString());
		if (params?.offset) searchParams.set('offset', params.offset.toString());

		const query = searchParams.toString();
		return this.request<RestorePointsResponse>(
			'GET',
			`/sources/${sourceId}/restore-points${query ? `?${query}` : ''}`
		);
	}

	/** Mainline-only: on an active branch the server refuses it (409 `rollback_mainline_only`). */
	async rollbackSource(sourceId: string, targetVersion: number): Promise<RollbackResponse> {
		return this.request<RollbackResponse>('POST', `/sources/${sourceId}/rollback`, {
			target_version: targetVersion
		});
	}

	/** Mainline-only: on an active branch the server refuses it (409 `rollback_mainline_only`). */
	async getCitationRestorePoints(
		citationId: string,
		params?: { limit?: number; offset?: number }
	): Promise<RestorePointsResponse> {
		const searchParams = new URLSearchParams();
		if (params?.limit) searchParams.set('limit', params.limit.toString());
		if (params?.offset) searchParams.set('offset', params.offset.toString());

		const query = searchParams.toString();
		return this.request<RestorePointsResponse>(
			'GET',
			`/citations/${citationId}/restore-points${query ? `?${query}` : ''}`
		);
	}

	/** Mainline-only: on an active branch the server refuses it (409 `rollback_mainline_only`). */
	async rollbackCitation(citationId: string, targetVersion: number): Promise<RollbackResponse> {
		return this.request<RollbackResponse>('POST', `/citations/${citationId}/rollback`, {
			target_version: targetVersion
		});
	}

	// Discovery feed endpoint
	async getDiscoveryFeed(limit?: number): Promise<DiscoveryFeedResponse> {
		const params = limit != null ? `?limit=${limit}` : '';
		return this.request<DiscoveryFeedResponse>('GET', `/analytics/discovery${params}`);
	}

	// Evidence Analysis endpoints
	async listEvidenceAnalyses(params?: {
		limit?: number;
		offset?: number;
		sort?: string;
		order?: 'asc' | 'desc';
	}): Promise<EvidenceAnalysisListResponse> {
		const searchParams = new URLSearchParams();
		if (params?.limit) searchParams.set('limit', params.limit.toString());
		if (params?.offset) searchParams.set('offset', params.offset.toString());
		if (params?.sort) searchParams.set('sort', params.sort);
		if (params?.order) searchParams.set('order', params.order);

		const query = searchParams.toString();
		return this.request<EvidenceAnalysisListResponse>(
			'GET',
			`/evidence-analyses${query ? `?${query}` : ''}`
		);
	}

	async createEvidenceAnalysis(
		data: EvidenceAnalysisCreateRequest
	): Promise<EvidenceAnalysisResponse> {
		return this.request<EvidenceAnalysisResponse>('POST', '/evidence-analyses', data);
	}

	async getEvidenceAnalysis(id: string): Promise<EvidenceAnalysisResponse> {
		return this.request<EvidenceAnalysisResponse>('GET', `/evidence-analyses/${id}`);
	}

	async updateEvidenceAnalysis(
		id: string,
		data: EvidenceAnalysisUpdateRequest
	): Promise<EvidenceAnalysisResponse> {
		return this.request<EvidenceAnalysisResponse>('PUT', `/evidence-analyses/${id}`, data);
	}

	async deleteEvidenceAnalysis(id: string, version: number): Promise<void> {
		return this.request<void>('DELETE', `/evidence-analyses/${id}?version=${version}`);
	}

	async getAnalysesByFact(
		factType: FactType,
		subjectId: string
	): Promise<EvidenceAnalysisResponse[]> {
		const params = new URLSearchParams({ factType, subjectId });
		return this.request<EvidenceAnalysisResponse[]>(
			'GET',
			`/evidence-analyses/by-fact?${params.toString()}`
		);
	}

	// Evidence Conflict endpoints
	async listEvidenceConflicts(params?: {
		limit?: number;
		offset?: number;
		status?: ConflictStatus;
	}): Promise<EvidenceConflictListResponse> {
		const searchParams = new URLSearchParams();
		if (params?.limit) searchParams.set('limit', params.limit.toString());
		if (params?.offset) searchParams.set('offset', params.offset.toString());
		if (params?.status) searchParams.set('status', params.status);

		const query = searchParams.toString();
		return this.request<EvidenceConflictListResponse>(
			'GET',
			`/evidence-conflicts${query ? `?${query}` : ''}`
		);
	}

	async getEvidenceConflict(id: string): Promise<EvidenceConflictResponse> {
		return this.request<EvidenceConflictResponse>('GET', `/evidence-conflicts/${id}`);
	}

	async resolveEvidenceConflict(
		id: string,
		data: EvidenceConflictResolveRequest
	): Promise<EvidenceConflictResponse> {
		return this.request<EvidenceConflictResponse>(
			'POST',
			`/evidence-conflicts/${id}/resolve`,
			data
		);
	}

	async getConflictsBySubject(subjectId: string): Promise<EvidenceConflictResponse[]> {
		return this.request<EvidenceConflictResponse[]>(
			'GET',
			`/evidence-conflicts/by-subject/${subjectId}`
		);
	}

	// Research Log endpoints
	async listResearchLogs(params?: {
		limit?: number;
		offset?: number;
		sort?: string;
		order?: 'asc' | 'desc';
	}): Promise<ResearchLogListResponse> {
		const searchParams = new URLSearchParams();
		if (params?.limit) searchParams.set('limit', params.limit.toString());
		if (params?.offset) searchParams.set('offset', params.offset.toString());
		if (params?.sort) searchParams.set('sort', params.sort);
		if (params?.order) searchParams.set('order', params.order);

		const query = searchParams.toString();
		return this.request<ResearchLogListResponse>(
			'GET',
			`/research-logs${query ? `?${query}` : ''}`
		);
	}

	async createResearchLog(data: ResearchLogCreateRequest): Promise<ResearchLogResponse> {
		return this.request<ResearchLogResponse>('POST', '/research-logs', data);
	}

	async getResearchLog(id: string): Promise<ResearchLogResponse> {
		return this.request<ResearchLogResponse>('GET', `/research-logs/${id}`);
	}

	async updateResearchLog(
		id: string,
		data: ResearchLogUpdateRequest
	): Promise<ResearchLogResponse> {
		return this.request<ResearchLogResponse>('PUT', `/research-logs/${id}`, data);
	}

	async deleteResearchLog(id: string, version: number): Promise<void> {
		return this.request<void>('DELETE', `/research-logs/${id}?version=${version}`);
	}

	async getResearchLogsBySubject(subjectId: string): Promise<ResearchLogResponse[]> {
		return this.request<ResearchLogResponse[]>(
			'GET',
			`/research-logs/by-subject/${subjectId}`
		);
	}

	// Proof Summary endpoints
	async listProofSummaries(params?: {
		limit?: number;
		offset?: number;
		sort?: string;
		order?: 'asc' | 'desc';
		/**
		 * Read this branch's view instead of the active scope - the branch page
		 * lists the proof summaries of the branch it shows, which need not be
		 * the active one.
		 */
		branch?: string;
	}): Promise<ProofSummaryListResponse> {
		const searchParams = new URLSearchParams();
		if (params?.branch) searchParams.set('branch', params.branch);
		if (params?.limit) searchParams.set('limit', params.limit.toString());
		if (params?.offset) searchParams.set('offset', params.offset.toString());
		if (params?.sort) searchParams.set('sort', params.sort);
		if (params?.order) searchParams.set('order', params.order);

		const query = searchParams.toString();
		return this.request<ProofSummaryListResponse>(
			'GET',
			`/proof-summaries${query ? `?${query}` : ''}`
		);
	}

	async createProofSummary(data: ProofSummaryCreateRequest): Promise<ProofSummaryResponse> {
		return this.request<ProofSummaryResponse>('POST', '/proof-summaries', data);
	}

	async getProofSummary(id: string): Promise<ProofSummaryResponse> {
		return this.request<ProofSummaryResponse>('GET', `/proof-summaries/${id}`);
	}

	async updateProofSummary(
		id: string,
		data: ProofSummaryUpdateRequest
	): Promise<ProofSummaryResponse> {
		return this.request<ProofSummaryResponse>('PUT', `/proof-summaries/${id}`, data);
	}

	async deleteProofSummary(id: string, version: number): Promise<void> {
		return this.request<void>('DELETE', `/proof-summaries/${id}?version=${version}`);
	}

	async getProofSummaryByFact(
		factType: FactType,
		subjectId: string
	): Promise<ProofSummaryResponse[]> {
		const params = new URLSearchParams({ factType, subjectId });
		return this.request<ProofSummaryResponse[]>(
			'GET',
			`/proof-summaries/by-fact?${params.toString()}`
		);
	}

	// Quality / validation endpoints
	async getQualityOverview(): Promise<QualityOverview> {
		return this.request<QualityOverview>('GET', '/quality/overview');
	}

	async getValidationIssues(params?: {
		severity?: 'error' | 'warning' | 'info';
		limit?: number;
		offset?: number;
	}): Promise<ValidationIssuesResponse> {
		const searchParams = new URLSearchParams();
		if (params?.severity) searchParams.set('severity', params.severity);
		if (params?.limit != null) searchParams.set('limit', params.limit.toString());
		if (params?.offset != null) searchParams.set('offset', params.offset.toString());

		const query = searchParams.toString();
		return this.request<ValidationIssuesResponse>(
			'GET',
			`/quality/validation${query ? `?${query}` : ''}`
		);
	}

	// Duplicate detection endpoints
	async getPersonsDuplicates(params?: {
		limit?: number;
		offset?: number;
	}): Promise<DuplicatesResponse> {
		const searchParams = new URLSearchParams();
		if (params?.limit != null) searchParams.set('limit', params.limit.toString());
		if (params?.offset != null) searchParams.set('offset', params.offset.toString());

		const query = searchParams.toString();
		return this.request<DuplicatesResponse>(
			'GET',
			`/persons/duplicates${query ? `?${query}` : ''}`
		);
	}

	async dismissDuplicate(
		person1Id: string,
		person2Id: string,
		req?: DismissDuplicateRequest
	): Promise<void> {
		return this.request<void>(
			'POST',
			`/persons/duplicates/${encodeURIComponent(person1Id)}/${encodeURIComponent(person2Id)}/dismiss`,
			req
		);
	}

	// Merge endpoints
	/** Branch-scoped: honors the active branch (see `BRANCH_SCOPED_OPERATIONS`). */
	async mergePersons(req: MergePersonsRequest): Promise<MergePersonsResponse> {
		return this.requestWithConflictRetry<MergePersonsResponse>(
			'POST',
			'/persons/merge',
			req
		);
	}

	/** Branch-scoped: honors the active branch (see `BRANCH_SCOPED_OPERATIONS`). */
	async batchMergePersons(req: BatchMergeRequest): Promise<BatchMergeResponse> {
		return this.request<BatchMergeResponse>('POST', '/persons/merge/batch', req);
	}

	async batchDismissDuplicates(req: BatchDismissRequest): Promise<BatchDismissResponse> {
		return this.request<BatchDismissResponse>(
			'POST',
			'/persons/duplicates/dismiss/batch',
			req
		);
	}

	// Research snapshot endpoints. A snapshot is a named marker ("tag") of an
	// event-store position in one branch's view, so these follow the active
	// branch (#839, see the allowlist above): the list, create and delete act on
	// the active branch's snapshots, and comparisons read its view of the log.
	async listSnapshots(): Promise<SnapshotList> {
		return this.request<SnapshotList>('GET', '/snapshots');
	}

	async createSnapshot(data: SnapshotCreate): Promise<Snapshot> {
		return this.request<Snapshot>('POST', '/snapshots', data);
	}

	async deleteSnapshot(id: string): Promise<void> {
		return this.request<void>('DELETE', `/snapshots/${encodeURIComponent(id)}`);
	}

	/**
	 * The changes recorded between two snapshots in the active branch's view,
	 * oldest first, whichever order they are passed in. Snapshots from
	 * different branches are refused (409 `snapshot_branch_mismatch`).
	 */
	async compareSnapshots(id1: string, id2: string): Promise<SnapshotComparisonResult> {
		return this.request<SnapshotComparisonResult>(
			'GET',
			`/snapshots/${encodeURIComponent(id1)}/compare/${encodeURIComponent(id2)}`
		);
	}

	/**
	 * The changes recorded since a snapshot, up to the current state, in the
	 * active branch's view. `until` stops the comparison at that log position
	 * instead (#833) - a merge's pre-merge snapshot up to the last change the
	 * merge replayed is exactly what the merge changed.
	 */
	async compareSnapshotToCurrent(
		id: string,
		until?: number
	): Promise<SnapshotCurrentComparisonResult> {
		const query = until === undefined ? '' : `?until=${encodeURIComponent(String(until))}`;
		return this.request<SnapshotCurrentComparisonResult>(
			'GET',
			`/snapshots/${encodeURIComponent(id)}/compare-current${query}`
		);
	}

	// Research branch endpoints. These manage the branches themselves, so they
	// are never branch-scoped — `/branches*` is absent from the allowlist above.
	// All of them answer 503 when the branch registry is not configured.
	/**
	 * With `includeDrift`, every active branch carries `drift`: how far the
	 * mainline has moved under it since the fork, counted server-side for the
	 * whole list in one query.
	 */
	async listBranches(options: { includeDrift?: boolean } = {}): Promise<BranchList> {
		const query = options.includeDrift ? '?include_drift=true' : '';
		return this.request<BranchList>('GET', `/branches${query}`);
	}

	async createBranch(data: BranchCreate): Promise<Branch> {
		return this.request<Branch>('POST', '/branches', data);
	}

	async getBranch(id: string): Promise<Branch> {
		return this.request<Branch>('GET', `/branches/${encodeURIComponent(id)}`);
	}

	/**
	 * Delete a branch: its events are retained, its overlay rows are purged and
	 * its status becomes `archived`. Answers 409 if the branch is not active.
	 */
	/**
	 * Partial edit of a branch's description and research record (#835). An
	 * active branch takes every field; a merged one only `outcome` (409
	 * `branch_field_locked` otherwise); an archived one nothing (409
	 * `branch_not_active`). Added subjects and proof summaries must exist on
	 * the branch (400 `invalid_reference`).
	 */
	async updateBranch(id: string, data: BranchUpdate): Promise<Branch> {
		return this.request<Branch>('PATCH', `/branches/${encodeURIComponent(id)}`, data);
	}

	/**
	 * Close a branch without merging, recording its outcome and reason (#836).
	 * Prefer this over deleteBranch, which records no outcome or reason (an open branch reads as abandoned).
	 */
	async closeBranch(id: string, req: BranchCloseRequest): Promise<Branch> {
		return this.request<Branch>('POST', `/branches/${encodeURIComponent(id)}/close`, req);
	}

	/** A branch's research, rebuilt from its events; works once it is closed. */
	async getBranchResearch(id: string): Promise<BranchResearchArchive> {
		return this.request<BranchResearchArchive>(
			'GET',
			`/branches/${encodeURIComponent(id)}/research`
		);
	}

	/** Copy a closed branch's research logs to the mainline; omit ids for all. */
	async promoteBranchResearchLogs(
		id: string,
		logIds?: string[]
	): Promise<PromoteResearchLogsResult> {
		return this.request<PromoteResearchLogsResult>(
			'POST',
			`/branches/${encodeURIComponent(id)}/research-logs/promote`,
			logIds && logIds.length > 0 ? { log_ids: logIds } : {}
		);
	}

	async deleteBranch(id: string): Promise<void> {
		return this.request<void>('DELETE', `/branches/${encodeURIComponent(id)}`);
	}

	/**
	 * How far the mainline has moved under a branch since it forked - a cheap
	 * count, not a diff. `compareBranch()` is the full picture.
	 */
	async getBranchDrift(id: string): Promise<BranchDrift> {
		return this.request<BranchDrift>('GET', `/branches/${encodeURIComponent(id)}/drift`);
	}

	async compareBranch(id: string): Promise<BranchComparisonResult> {
		return this.request<BranchComparisonResult>(
			'GET',
			`/branches/${encodeURIComponent(id)}/compare`
		);
	}

	/**
	 * Merge a branch into the mainline. The body is optional per the spec, so an
	 * omitted one is sent as `{}` — a conflict-free branch, no note, no
	 * resolutions. Every conflict `compareBranch()` reported must appear in
	 * `resolutions` or the merge is refused.
	 *
	 * Deliberately plain `request()`, **never** `requestWithConflictRetry()`.
	 * That helper exists for optimistic-locking clashes on the person aggregate,
	 * where a blind retry usually succeeds. Every 409 here is a considered
	 * refusal instead: `merge_conflicts` needs a human decision;
	 * `branch_not_active` / `merge_already_claimed` mean someone else already
	 * merged and can never succeed on retry; `branch_too_large` /
	 * `main_too_far_ahead` are documented as not retryable; and
	 * `merge_plan_stale` is retryable only after re-presenting the verdict to the
	 * user — a silent retry would merge over a mainline write they never saw.
	 *
	 * Narrow what this throws with `isBranchMergeRefusal()`.
	 */
	async mergeBranch(id: string, req: BranchMergeRequest = {}): Promise<BranchMergeResult> {
		return this.request<BranchMergeResult>(
			'POST',
			`/branches/${encodeURIComponent(id)}/merge`,
			req
		);
	}

	/**
	 * The merge blockers the proposed resolutions would be refused with
	 * (`409 merge_dangling_reference`), without merging (#831). Writes nothing,
	 * so the review can re-check after every decision. Refusals share the merge's
	 * codes (`branch_not_active`, `merge_empty`, ...).
	 */
	async precheckBranchMerge(
		id: string,
		req: BranchMergePrecheckRequest = {}
	): Promise<BranchMergePrecheckResult> {
		return this.request<BranchMergePrecheckResult>(
			'POST',
			`/branches/${encodeURIComponent(id)}/merge/precheck`,
			req
		);
	}

	/**
	 * The facts and relationships the branch changed without an evidence
	 * analysis or proof summary on the branch (#838). A soft warning for the
	 * merge review, never a gate.
	 */
	async getBranchEvidenceCoverage(id: string): Promise<BranchEvidenceCoverage> {
		return this.request<BranchEvidenceCoverage>(
			'GET',
			`/branches/${encodeURIComponent(id)}/evidence-coverage`
		);
	}

	/**
	 * The validation issues, quality issues and duplicate pairs the branch
	 * introduces over the mainline (#838). Only an active branch answers; a
	 * merged or archived one is a 404.
	 */
	async getBranchHealth(id: string): Promise<BranchHealth> {
		return this.request<BranchHealth>('GET', `/branches/${encodeURIComponent(id)}/health`);
	}

	/**
	 * Finish a merge that answered `500 merge_partially_applied` (#685). Only
	 * the branch events not already on the mainline are replayed, and resuming
	 * a completed merge is a no-op (`replayed_event_count: 0`), so unlike
	 * `mergeBranch()` a retry after a 500 is safe.
	 *
	 * A `409 merge_resume_needs_resolution` carries `pending`: the entities
	 * that need a decision, named, with why and which sides each accepts.
	 * Resolve each via `req.resolutions`. Narrow what this throws with
	 * `isBranchMergeResumeRefusal()`.
	 */
	async resumeBranchMerge(
		id: string,
		req: BranchMergeResumeRequest = {}
	): Promise<BranchMergeResumeResult> {
		return this.request<BranchMergeResumeResult>(
			'POST',
			`/branches/${encodeURIComponent(id)}/merge/resume`,
			req
		);
	}
}

export const api = new ApiClient();

/** Check if an error is a version conflict (409). For endpoints using requestWithConflictRetry, auto-retry has already been attempted. */
export function isConflictError(error: unknown): boolean {
	const apiError = error as ApiError;
	return apiError?.status === 409 || apiError?.code === 'CONFLICT_RETRY_FAILED';
}

/**
 * Why a record fetched by its id failed to load, in words (#899). A 404, and a
 * 400 (the only 400 a fetch by id can give is a malformed id, which no record
 * can have), read as "not found", never as the server's parameter-binding
 * detail ("Invalid format for parameter id: error unmarshaling ...").
 */
export function recordLoadError(error: unknown, noun: string): string {
	const apiError = error as ApiError;
	if (apiError?.status === 404 || apiError?.status === 400) {
		return `This ${noun} could not be found. It may have been deleted, or the link may be wrong.`;
	}
	return apiError?.message || `Failed to load the ${noun}.`;
}

/**
 * Every `code` `POST /branches/{id}/merge` refuses with. The first seven are the
 * 409 enum of the generated `BranchMergeConflictError`. The last three are not:
 * `merge_partially_applied` is the 500, and `invalid_resolution` /
 * `validation_error` are the two 400s, all of which carry the generic
 * `BadRequest`/`Error` schema. They share the refusal *shape* but live outside
 * the generated enum, so they are hand-maintained here and must be kept in step
 * with `MergeBranch400`/`500` in `internal/api/branch_handlers.go` by hand.
 *
 * The 400s are genuinely reachable, not defensive: compare can offer a
 * resolution the merge's own re-detection no longer supports (an entity
 * reclassified `edit_edit` -> `delete_edit` by a mainline delete), and
 * `invalid_resolution` is decided before the staleness check, so it is the
 * verdict the user sees. Both are recoverable by re-comparing.
 *
 * An allowlist rather than "any 4xx/5xx from this call": an optimistic-locking
 * 409 bubbling up from an unrelated endpoint must not be mistaken for a merge
 * verdict the UI claims it can explain.
 */
const BRANCH_MERGE_REFUSAL_CODES = [
	'merge_conflicts',
	'branch_not_active',
	'merge_already_claimed',
	'branch_too_large',
	'main_too_far_ahead',
	'merge_plan_stale',
	'merge_dangling_reference',
	'merge_empty',
	'merge_partially_applied',
	'invalid_resolution',
	'validation_error'
] as const satisfies readonly (
	| BranchMergeConflictError['code']
	| 'merge_partially_applied'
	| 'invalid_resolution'
	| 'validation_error'
)[];

export type BranchMergeRefusalCode = (typeof BRANCH_MERGE_REFUSAL_CODES)[number];

/**
 * Compile-time completeness. `satisfies` above only proves no *invented* code
 * crept in; this proves the other direction — if the spec grows a 409 code the
 * list is missing, `Exclude` is non-empty and this fails to type-check, instead
 * of the guard silently answering false for it. Types only, no runtime cost.
 */
type AssertNever<T extends never> = T;
type _BranchMergeRefusalCodesAreComplete = AssertNever<
	Exclude<BranchMergeConflictError['code'], BranchMergeRefusalCode>
>;

/**
 * A refusal thrown by `mergeBranch()`, plus the HTTP `status` that `request()`
 * stamps onto every error body it rethrows.
 */
export type BranchMergeRefusal = Omit<BranchMergeConflictError, 'code'> & {
	code: BranchMergeRefusalCode;
	status?: number;
};

/**
 * Narrow a thrown value to a merge refusal, so a caller can render distinct copy
 * per code. `conflicts` is deliberately not required: the schema marks it
 * optional and only `merge_conflicts` populates it.
 */
export function isBranchMergeRefusal(error: unknown): error is BranchMergeRefusal {
	if (typeof error !== 'object' || error === null) return false;
	const candidate = error as { code?: unknown; message?: unknown };
	return (
		typeof candidate.code === 'string' &&
		(BRANCH_MERGE_REFUSAL_CODES as readonly string[]).includes(candidate.code) &&
		typeof candidate.message === 'string'
	);
}

/**
 * Every `code` `POST /branches/{id}/merge/resume` refuses with: the 409 enum of
 * the generated `BranchMergeResumeError`, plus the 500 `merge_partially_applied`
 * and the two 400s, which carry the generic error shape. An allowlist for the
 * same reason as `BRANCH_MERGE_REFUSAL_CODES`.
 */
const BRANCH_MERGE_RESUME_REFUSAL_CODES = [
	'merge_not_claimed',
	'merge_resume_needs_resolution',
	'merge_dangling_reference',
	'branch_too_large',
	'merge_resume_concurrent',
	'merge_partially_applied',
	'invalid_resolution',
	'validation_error'
] as const satisfies readonly (
	| BranchMergeResumeError['code']
	| 'merge_partially_applied'
	| 'invalid_resolution'
	| 'validation_error'
)[];

export type BranchMergeResumeRefusalCode = (typeof BRANCH_MERGE_RESUME_REFUSAL_CODES)[number];

type _BranchMergeResumeRefusalCodesAreComplete = AssertNever<
	Exclude<BranchMergeResumeError['code'], BranchMergeResumeRefusalCode>
>;

/** A refusal thrown by `resumeBranchMerge()`, with the HTTP `status`. */
export type BranchMergeResumeRefusal = Omit<BranchMergeResumeError, 'code'> & {
	code: BranchMergeResumeRefusalCode;
	status?: number;
};

/** Narrow a thrown value to a resume refusal. */
export function isBranchMergeResumeRefusal(error: unknown): error is BranchMergeResumeRefusal {
	if (typeof error !== 'object' || error === null) return false;
	const candidate = error as { code?: unknown; message?: unknown };
	return (
		typeof candidate.code === 'string' &&
		(BRANCH_MERGE_RESUME_REFUSAL_CODES as readonly string[]).includes(candidate.code) &&
		typeof candidate.message === 'string'
	);
}

// Utility functions for formatting
/**
 * Human-readable label for a GEDCOM calendar escape token, or '' for the
 * default Gregorian calendar (or an unknown token).
 */
export function calendarLabel(calendar?: string): string {
	switch (calendar) {
		case 'DJULIAN':
			return 'Julian';
		case 'DHEBREW':
			return 'Hebrew';
		case 'DFRENCH R':
			return 'French Republican';
		default:
			return '';
	}
}

/**
 * Remove a GEDCOM calendar escape sequence (e.g. "@#DJULIAN@") from a string,
 * returning the cleaned string and the escape token that was stripped (without
 * the "@#"/"@" delimiters), if any.
 */
function stripCalendarEscape(raw: string): { text: string; token?: string } {
	const match = raw.match(/@#(D[^@]*)@/);
	return {
		text: raw.replace(/@#D[^@]*@\s*/g, '').trim(),
		token: match?.[1]
	};
}

export function formatGenDate(date?: GenDate): string {
	if (!date) return '';

	if (date.raw) {
		// Show the date as originally recorded, without the raw escape sequence,
		// annotated with the calendar system when it is not Gregorian. Derive the
		// label from date.calendar, falling back to the escape token in the raw
		// string so a stripped escape never disappears without annotation.
		const stripped = stripCalendarEscape(date.raw);
		const rawLabel = calendarLabel(date.calendar) || calendarLabel(stripped.token);
		return stripped.text + (rawLabel ? ` (${rawLabel})` : '');
	}

	const label = calendarLabel(date.calendar);
	const suffix = label ? ` (${label})` : '';

	const parts: string[] = [];

	if (date.qualifier && date.qualifier !== 'exact') {
		parts.push(date.qualifier.toUpperCase());
	}

	if (date.day) parts.push(date.day.toString());

	if (date.month) {
		const months = [
			'JAN',
			'FEB',
			'MAR',
			'APR',
			'MAY',
			'JUN',
			'JUL',
			'AUG',
			'SEP',
			'OCT',
			'NOV',
			'DEC'
		];
		parts.push(months[date.month - 1]);
	}

	if (date.year) parts.push(date.year.toString());

	if (date.qualifier === 'bet' && date.year2) {
		parts.push('AND');
		if (date.day2) parts.push(date.day2.toString());
		if (date.month2) {
			const months = [
				'JAN',
				'FEB',
				'MAR',
				'APR',
				'MAY',
				'JUN',
				'JUL',
				'AUG',
				'SEP',
				'OCT',
				'NOV',
				'DEC'
			];
			parts.push(months[date.month2 - 1]);
		}
		parts.push(date.year2.toString());
	}

	if (date.qualifier === 'int' && date.interpreted_from) {
		parts.push(`(${date.interpreted_from})`);
	}

	return parts.join(' ') + suffix;
}

/**
 * Format a person's display name from given_name + surname.
 *
 * Handles missing/empty fields without producing dangling spaces. Returns
 * 'Unknown' when no usable name parts are available — matches the historical
 * fallback used by FamilyCard before the surname field was populated by the
 * backend (issue #483).
 *
 * Examples:
 *   formatPersonName({ given_name: 'Jane', surname: 'Smith' }) -> 'Jane Smith'
 *   formatPersonName({ given_name: 'Jane', surname: '' })      -> 'Jane'
 *   formatPersonName({ given_name: '', surname: 'Smith' })     -> 'Smith'
 *   formatPersonName({})                                       -> 'Unknown'
 *   formatPersonName(null)                                     -> 'Unknown'
 */
export function formatPersonName(
	person: { given_name?: string; surname?: string } | null | undefined
): string {
	if (!person) return 'Unknown';
	const parts = [person.given_name, person.surname]
		.map((s) => (s ?? '').trim())
		.filter((s) => s.length > 0);
	return parts.length > 0 ? parts.join(' ') : 'Unknown';
}

export function formatLifespan(person: { birth_date?: GenDate; death_date?: GenDate }): string {
	const birth = person.birth_date?.year;
	const death = person.death_date?.year;

	if (!birth && !death) return '';
	if (birth && !death) return `(b. ${birth})`;
	if (!birth && death) return `(d. ${death})`;
	return `(${birth}–${death})`;
}
