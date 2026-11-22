<script lang="ts">
	/**
	 * Where a mainline change came from (#832): the branch it was researched on
	 * and the merge note - the "why" of the promotion. Links to the merged
	 * branch, except for a visitor, who cannot open it (`branchPageHref`); the title says when the change was made on the branch and when
	 * the merge brought it over. Shared by entity history and the snapshot
	 * comparison, where it tells a merge's replayed changes apart from
	 * mainline edits in the same range (#833).
	 */
	import type { MergeOrigin } from '#lib/api/client.js';
	import { mergedFromLabel } from '#lib/utils/changeEntries.js';
	import { branchPageHref } from '#lib/utils/branchPages.js';

	let { origin }: { origin: MergeOrigin } = $props();

	const href = $derived(branchPageHref(origin.branch_id));

	function formatTimestamp(iso: string): string {
		return new Date(iso).toLocaleDateString('en-US', {
			month: 'short',
			day: 'numeric',
			year: 'numeric',
			hour: 'numeric',
			minute: '2-digit'
		});
	}
</script>

<svelte:element
	this={href ? 'a' : 'span'}
	{href}
	class="merge-chip"
	data-slot="chip"
	title="Made on the branch {formatTimestamp(origin.original_timestamp)}; merged {formatTimestamp(
		origin.merged_at
	)}"
	data-testid="merged-from"
>
	{mergedFromLabel(origin)}
</svelte:element>

<style>
	.merge-chip {
		display: inline-block;
		max-width: 100%;
		margin-top: 0.5rem;
		padding: 0.125rem 0.5rem;
		border: 1px solid #c4b5fd;
		border-radius: 9999px;
		background: #f5f3ff;
		font-size: 0.75rem;
		color: #5b21b6;
		text-decoration: none;
		overflow-wrap: anywhere;
	}

	a.merge-chip:hover {
		border-color: #7c3aed;
	}
</style>
