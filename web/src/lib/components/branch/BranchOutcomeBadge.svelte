<script lang="ts">
	/**
	 * The verdict a branch's research reached (#835). Colour is a hint only:
	 * the word is always shown, prefixed so a screen reader hears what it is.
	 */
	import type { BranchOutcome } from '$lib/api/client';
	import { OUTCOME_LABELS, branchOutcome } from '$lib/utils/branchResearch';

	interface Props {
		outcome: BranchOutcome | undefined | null;
	}

	let { outcome }: Props = $props();

	const value = $derived(branchOutcome(outcome));
</script>

<span class="outcome outcome-{value}" data-testid="branch-outcome" data-outcome={value}>
	<span class="sr-only">Outcome:</span>
	{OUTCOME_LABELS[value]}
</span>

<style>
	.outcome {
		display: inline-flex;
		align-items: center;
		height: 1.25rem;
		padding: 0 0.5rem;
		border-radius: 9999px;
		border: 1px solid transparent;
		font-size: 0.75rem;
		font-weight: 500;
		white-space: nowrap;
	}

	.outcome-open {
		background: #f1f5f9;
		border-color: #cbd5e1;
		color: #334155;
	}

	.outcome-proved {
		background: #dcfce7;
		border-color: #86efac;
		color: #166534;
	}

	.outcome-disproved {
		background: #fee2e2;
		border-color: #fca5a5;
		color: #991b1b;
	}

	.outcome-inconclusive {
		background: #fef3c7;
		border-color: #fcd34d;
		color: #92400e;
	}

	.outcome-superseded {
		background: #e0e7ff;
		border-color: #a5b4fc;
		color: #3730a3;
	}

	.sr-only {
		position: absolute;
		width: 1px;
		height: 1px;
		padding: 0;
		margin: -1px;
		overflow: hidden;
		clip: rect(0, 0, 0, 0);
		white-space: nowrap;
		border: 0;
	}
</style>
