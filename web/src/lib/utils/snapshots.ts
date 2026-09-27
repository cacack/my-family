/**
 * The page that compares two research snapshots. Ids travel in the query
 * string, encoded, so the route stays one page however the ids are shaped.
 */
export function snapshotCompareHref(fromId: string, toId: string): string {
	const params = new URLSearchParams({ from: fromId, to: toId });
	return `/snapshots/compare?${params.toString()}`;
}
