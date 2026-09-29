/**
 * The `to` value that compares a snapshot with the current state of the tree
 * (the log head) rather than with a second snapshot (#839). It is a page-level
 * token only: the API has a separate endpoint for it
 * (`/snapshots/{id}/compare-current`), so it never reaches a snapshot path.
 */
export const CURRENT_STATE = 'current';

/**
 * The page that compares two research snapshots, or a snapshot with the
 * current state when `toId` is `CURRENT_STATE`. Ids travel in the query
 * string, encoded, so the route stays one page however the ids are shaped.
 */
export function snapshotCompareHref(fromId: string, toId: string): string {
	const params = new URLSearchParams({ from: fromId, to: toId });
	return `/snapshots/compare?${params.toString()}`;
}

/** The page that compares a snapshot with the current state ("compare to now"). */
export function snapshotCompareToNowHref(fromId: string): string {
	return snapshotCompareHref(fromId, CURRENT_STATE);
}

/**
 * The page that compares a snapshot with the log up to position `until`
 * (#833): a fixed range that stays the same however far the log moves on.
 * A merge's pre-merge snapshot up to the last change the merge replayed is
 * exactly what the merge changed.
 */
export function snapshotCompareRangeHref(fromId: string, until: number): string {
	const params = new URLSearchParams({ from: fromId, to: CURRENT_STATE, until: String(until) });
	return `/snapshots/compare?${params.toString()}`;
}

/**
 * Reads the `until` query value of the compare page: a non-negative integer
 * log position, `null` when absent, or `undefined` when present but malformed.
 */
export function parseComparisonEnd(raw: string | null): number | null | undefined {
	if (raw === null) return null;
	if (!/^\d+$/.test(raw)) return undefined;
	const value = Number(raw);
	return Number.isSafeInteger(value) ? value : undefined;
}
