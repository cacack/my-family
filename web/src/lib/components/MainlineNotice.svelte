<script lang="ts">
	/**
	 * Inline notice for surfaces that are NOT branch-scoped, shown only while a
	 * research branch is active.
	 *
	 * The `?branch=` parameter is declared on the #669 vertical slice only —
	 * persons, person names, families (detail, not list), family children and
	 * pedigree. Every other read model still answers from the mainline. Rendering
	 * one of them unlabelled beneath the branch banner would be the UI quietly
	 * lying about what the user is looking at.
	 *
	 * #676 fans the branch overlay out; this notice retires with it.
	 *
	 * ## Where it is placed, and where it deliberately is not
	 *
	 * Roughly twenty surfaces read mainline-only data while a branch is active.
	 * Labelling all of them would be noise, so this is a chosen subset: the
	 * surfaces whose content is most easily mistaken for branch content.
	 *
	 * Placed:
	 * - `/analytics` — quality scores computed over mainline persons
	 * - `/evidence` — evidence analyses, conflicts, research logs, proofs
	 * - `/families` (list only; family *detail* is branch-scoped)
	 * - `/quality` — validation issues and duplicate pairs
	 * - `/search` — advanced search
	 * - `/sources` — sources and citations
	 * - `/history` — the global change feed
	 * - `/ahnentafel/{id}` — the ancestor report
	 *
	 * Known gap, to close with #676 rather than by sprinkling more notices:
	 * `/` (dashboard and discovery feed), `/browse/*`, `/descendancy/{id}`,
	 * `/map`, `/relationship`, `/repositories`, `/import`, and the panels on
	 * person and family detail pages that are not themselves scoped — change
	 * history, restore points, media, citations and evidence. All of them
	 * answer from the mainline today; none of them says so.
	 */
	import { activeBranch } from '$lib/stores/activeBranch.svelte';

	interface Props {
		/** What this page shows, e.g. "Sources". Used in the sentence. */
		surface: string;
		/**
		 * Replaces the default explanation. The families *list* needs its own,
		 * because family detail pages are branch-scoped while the list is not.
		 */
		detail?: string;
	}

	let {
		surface,
		detail = 'Branch scoping currently covers people, families and pedigrees only.'
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
			{surface} always shows mainline data, even while a research branch is active. {detail}
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
