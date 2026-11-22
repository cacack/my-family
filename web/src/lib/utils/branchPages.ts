/**
 * Links to a research branch's own pages: its comparison (`/branches/<id>`)
 * and its research archive (`/branches/<id>/research`). A visitor reads the
 * mainline only (ADR-008 §4), so the server refuses them the API those pages
 * load: the branch routes in `signedInRoutes` (`internal/api/roles.go`). Every
 * link to a branch page goes through `branchPageHref`, so nobody is offered a
 * link that ends at a sign-in prompt. The branches list (`/branches`) is not a
 * branch page: a visitor may read it.
 */
import { isVisitor } from '#lib/stores/access.svelte.js';

/** Which of a branch's pages: the comparison or the research archive. */
export type BranchPage = 'compare' | 'research';

/**
 * The address of a branch's `page`, or null for a visitor, who cannot open
 * it: render the text without a link then. Reactive, like `isVisitor`.
 */
export function branchPageHref(id: string, page: BranchPage = 'compare'): string | null {
	if (isVisitor()) return null;
	const base = `/branches/${encodeURIComponent(id)}`;
	return page === 'research' ? `${base}/research` : base;
}
