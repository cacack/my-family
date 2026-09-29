<script lang="ts">
	/**
	 * Flags a merged branch whose merge did not finish (#830) - its replay onto
	 * the mainline stopped partway - and offers to finish it. Until then the
	 * mainline holds only part of the branch's research, which is exactly what
	 * this has to make impossible to miss.
	 */
	import type { MergePendingEntity } from '$lib/api/client';
	import { Button } from '$lib/components/ui/button';
	import { incompleteMergeSummary, UNREADABLE_MERGE_SUMMARY } from '$lib/utils/mergeState';

	interface Props {
		pending: MergePendingEntity[];
		/** The server could not read how far the merge got (`merge_state: unknown`). */
		unreadable?: boolean;
		/** Omit to show the flag without the action. */
		onfinish?: () => void;
		disabled?: boolean;
	}

	let { pending, unreadable = false, onfinish, disabled = false }: Props = $props();
</script>

<div class="incomplete-merge" role="alert" data-testid="incomplete-merge">
	<div class="text">
		<p class="title">This merge did not finish</p>
		<p class="detail">
			{#if unreadable}
				The branch is marked merged, but its merge may not have finished. {UNREADABLE_MERGE_SUMMARY}
			{:else}
				The branch is marked merged, but its replay onto the mainline stopped partway.
				{incompleteMergeSummary(pending)}
			{/if}
		</p>
	</div>
	{#if onfinish}
		<Button onclick={onfinish} {disabled}>Finish merge</Button>
	{/if}
</div>

<style>
	.incomplete-merge {
		display: flex;
		align-items: center;
		gap: 1rem;
		flex-wrap: wrap;
		margin-bottom: 1rem;
		padding: 0.75rem 1rem;
		background: #fff7ed;
		border: 1px solid #fb923c;
		border-radius: 6px;
		color: #7c2d12;
	}

	:global(body.high-contrast) .incomplete-merge {
		background: #431407;
		border-color: #fdba74;
		color: #ffedd5;
	}

	.text {
		flex: 1;
		min-width: 14rem;
	}

	.title {
		margin: 0;
		font-weight: 600;
		font-size: 0.9375rem;
	}

	.detail {
		margin: 0.25rem 0 0;
		font-size: 0.8125rem;
	}
</style>
