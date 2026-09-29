<script lang="ts">
	/**
	 * Inline notice for surfaces that are NOT branch-scoped, shown only while a
	 * research branch is active.
	 *
	 * `?branch=` covers the genealogy data and the research on it (ADR-005):
	 * persons, names, families and their children, pedigree, the browse and map
	 * aggregates (#756), the cemetery index and associations (#757), sources,
	 * citations and notes (#758), media (#759), the GPS artifacts (#760), search,
	 * the families list, the group sheet, the Ahnentafel, descendancy and the
	 * relationship calculator (#829), person and family history (#824),
	 * snapshots (#839) and person merges (#834). `BRANCH_SCOPED_OPERATIONS` in
	 * `$lib/api/client.ts` is the exact list. The remaining surfaces — checks and
	 * suggestions computed over the mainline, the global history, brick walls,
	 * and the entities that stay main-only by decision (ADR-005) — still answer
	 * from the mainline. Rendering one of them unlabelled beneath the branch
	 * banner would be the UI quietly lying about what the user is looking at.
	 *
	 * ## Where it is placed
	 *
	 * Every page that reads mainline-only data while a branch is active renders
	 * this notice or withdraws the mainline-only controls:
	 * - `/` (dashboard) — the research suggestions (discovery feed) come from
	 *   the mainline while the people and family counts and the recent people
	 *   and families follow the branch; the notice says which is which
	 * - `/quality` — validation issues and duplicate pairs (a branch's own findings,
	 *   the ones it introduces over the mainline, are in its merge review's branch
	 *   health section, #838). A pair's "Merge" link opens the merge page, which
	 *   follows the branch (#834): it shows the two persons as the branch sees
	 *   them and merges them on the branch only
	 * - `/history` — the global change feed
	 * - `/browse/brick-walls` — brick walls are not event-sourced (#761; whether
	 *   they become event-sourced is #802)
	 * - `/repositories` (list and detail, including its edit form) —
	 *   repositories are main-only by decision (ADR-005), so creating, editing
	 *   or deleting one writes the mainline and every branch sees it
	 * - `/import`, export section — JSON, CSV and GEDCOM exports cover the
	 *   mainline
	 *
	 * Withdrawn rather than labelled: GEDCOM import always writes the mainline,
	 * so `/import` and the onboarding wizard's import step replace their upload
	 * controls with `BranchImportBlocked` while a branch is active, and the API
	 * refuses an import that carries `?branch=`. The onboarding wizard itself is
	 * suppressed on a branch, so an empty branch view never offers to "start" a
	 * tree. The person page withdraws its brick-wall controls with a note, and
	 * the person and family pages withdraw their Restore tab and rollback
	 * dialog, since rollback is mainline-only (#824).
	 *
	 * Not placed, because these surfaces follow the branch: `/browse/surnames`,
	 * `/browse/places` and `/browse/cemeteries` (indexes and per-entry lists),
	 * `/map`, `/sources`, `/evidence` with its analysis, conflict, research-log
	 * and proof-summary pages, `/snapshots` with `/snapshots/compare`, `/search`
	 * and every other search surface (the header SearchBox and PersonSelector),
	 * `/families`, `/analytics`, the family group sheet, `/pedigree/{id}`,
	 * `/ahnentafel/{id}`, `/descendancy/{id}`, `/relationship`, the person
	 * merge page, and the media gallery, evidence panel and history panels on
	 * person and family pages.
	 *
	 * Known gap: the brick-wall flag shown on a person page is part of the
	 * person's row, which a branch copies the first time it writes that person.
	 * For a person the branch has edited it therefore shows the flag as it stood
	 * then, not a later mainline change (#802).
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
		detail = 'A branch covers people, families, sources, citations, notes, media, associations, evidence and research logs, snapshots, search, charts and reports, and the browse and map views.'
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
