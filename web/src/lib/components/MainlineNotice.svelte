<script lang="ts">
	/**
	 * Inline notice for surfaces that are NOT branch-scoped, shown only while a
	 * research branch is active.
	 *
	 * The `?branch=` parameter is declared on the #669 vertical slice — persons,
	 * person names, families, family children and pedigree —
	 * on the browse and map aggregates that read that slice's overlay (#676
	 * sub-issue A, #756), on the person/family facts of sub-issue B (#757):
	 * the cemetery index and the association endpoints, on the evidence of
	 * sub-issue C (#758): sources, citations and notes, on the media of
	 * sub-issue D (#759): metadata, person media lists, content and thumbnails
	 * (the file bytes themselves are shared with the mainline), and on the GPS
	 * artifacts of sub-issue E (#760): evidence analyses, evidence conflicts,
	 * research logs and proof summaries, and on search, the families list, the
	 * group sheet, the Ahnentafel, descendancy and the relationship calculator
	 * (#829). The remaining surfaces — aggregates
	 * computed over the mainline, history, and the entities that stay main-only
	 * by decision (ADR-005) — still answer from the mainline. Rendering one of
	 * them unlabelled beneath the branch banner would be the UI quietly lying
	 * about what the user is looking at.
	 *
	 * ## Where it is placed, and where it deliberately is not
	 *
	 * Roughly twenty surfaces read mainline-only data while a branch is active.
	 * Labelling all of them would be noise, so this is a chosen subset: the
	 * surfaces whose content is most easily mistaken for branch content.
	 *
	 * Placed:
	 * - `/` (dashboard) — the research suggestions (discovery feed) come from
	 *   the mainline while the people and family counts and the recent people
	 *   and families follow the branch; the notice says which is which
	 * - `/quality` — validation issues and duplicate pairs
	 * - `/history` — the global change feed
	 * - `/browse/brick-walls` — brick walls are not event-sourced (#761)
	 * - `/repositories` (list and detail, including its edit form) —
	 *   repositories are main-only by decision (ADR-005), so creating, editing
	 *   or deleting one writes the mainline and every branch sees it
	 * - `/import`, export section — JSON, CSV and GEDCOM exports cover the
	 *   mainline
	 *
	 * GEDCOM import is not labelled but withdrawn: it always writes the mainline,
	 * so `/import` and the onboarding wizard's import step replace their upload
	 * controls with `BranchImportBlocked` while a branch is active (the same
	 * pattern as the merge page's `mergeBlockedByBranch`), and the API refuses an
	 * import that carries `?branch=`. The onboarding wizard itself is suppressed
	 * on a branch, so an empty branch view never offers to "start" a tree.
	 *
	 * Deliberately not placed, because these surfaces now follow the branch:
	 * `/browse/surnames` (index and per-surname list), `/browse/places` (index
	 * and per-place list), `/browse/cemeteries` (index and per-cemetery list,
	 * since #757), `/map`, `/sources` (list and detail, since #758) and
	 * `/evidence` with its research-log and proof-summary pages (since #760), and
	 * `/snapshots` with `/snapshots/compare` (since #839: a snapshot marks a
	 * position in one branch's view, and its comparison reads that view). The
	 * media gallery on person detail pages follows the branch too (since #759),
	 * and so does the evidence panel (since #760). Since #829 so do `/search`
	 * and every other search surface (the header SearchBox and PersonSelector),
	 * `/families` (the list), `/analytics`, the family group sheet,
	 * `/ahnentafel/{id}`, `/descendancy/{id}` and `/relationship`.
	 *
	 * Known gaps still open: none among the per-person and per-family reads.
	 *
	 * The history panels on person and family detail pages follow the branch
	 * (#824), labelling each entry as the branch's own or inherited from the
	 * mainline. Restore points and rollback are mainline-only (ADR-005), so those
	 * pages withdraw them on a branch rather than labelling them.
	 */
	import { activeBranch } from '$lib/stores/activeBranch.svelte';

	interface Props {
		/** What this page shows, e.g. "Sources". Used in the sentence. */
		surface?: string;
		/**
		 * Replaces the default explanation, for a surface that needs to say more
		 * precisely which of its parts are mainline.
		 */
		detail?: string;
		/**
		 * Replaces the whole sentence, for surfaces that are only partly mainline
		 * (the dashboard, `/analytics`) or that are shared rather than merely
		 * unscoped (repositories, exports). `surface` and `detail` are ignored.
		 */
		message?: string;
	}

	let {
		surface = 'This page',
		message,
		detail = 'Branch scoping currently covers people, families, pedigrees, descendancy, relationships, search, sources, citations, notes, media and the browse and map views.'
	}: Props = $props();
</script>

{#if activeBranch.id}
	<div class="mainline-notice" role="note">
		<svg
			class="notice-icon"
			viewBox="0 0 24 24"
			fill="none"
			stroke="currentColor"
			stroke-width="2"
			aria-hidden="true"
		>
			<circle cx="12" cy="12" r="10" />
			<line x1="12" y1="16" x2="12" y2="12" />
			<line x1="12" y1="8" x2="12.01" y2="8" />
		</svg>
		<span>
			{#if message}
				{message}
			{:else}
				{surface} always shows mainline data, even while a research branch is active. {detail}
			{/if}
		</span>
	</div>
{/if}

<style>
	.mainline-notice {
		display: flex;
		align-items: flex-start;
		gap: 0.5rem;
		margin-bottom: 1rem;
		padding: 0.625rem 0.875rem;
		background: #f1f5f9;
		border: 1px solid #cbd5e1;
		border-radius: 6px;
		font-size: 0.8125rem;
		color: #475569;
	}

	:global(body.high-contrast) .mainline-notice {
		background: #1a1a1a;
		border-color: #666;
		color: #ccc;
	}

	.notice-icon {
		width: 1rem;
		height: 1rem;
		flex-shrink: 0;
		margin-top: 0.125rem;
	}
</style>
