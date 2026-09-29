<script lang="ts">
	/**
	 * The merge dialog's "Snapshot before merging" option (#833), on by
	 * default. When checked, the merge request asks the server to mark the
	 * mainline with a snapshot named "Before merging <branch>" just before the
	 * merge claims the branch, so the merged branch can later link to exactly
	 * what the merge changed.
	 */
	import { Checkbox } from '$lib/components/ui/checkbox';

	interface Props {
		checked: boolean;
		branchName: string;
		disabled?: boolean;
	}

	let { checked = $bindable(true), branchName, disabled = false }: Props = $props();
</script>

<div class="snapshot-option">
	<Checkbox
		id="merge-snapshot-before"
		bind:checked
		{disabled}
		aria-describedby="merge-snapshot-before-hint"
	/>
	<div class="snapshot-text">
		<label for="merge-snapshot-before" class="snapshot-label">Snapshot before merging</label>
		<p id="merge-snapshot-before-hint" class="snapshot-hint">
			Saves the mainline as it stands now as "Before merging {branchName}", so you can see
			afterwards exactly what this merge changed.
		</p>
	</div>
</div>

<style>
	.snapshot-option {
		display: flex;
		align-items: flex-start;
		gap: 0.625rem;
		padding: 0.625rem 0.75rem;
		border: 1px solid #e2e8f0;
		border-radius: 6px;
	}

	.snapshot-option :global([data-slot='checkbox']) {
		margin-top: 0.1875rem;
	}

	.snapshot-text {
		display: flex;
		flex-direction: column;
		gap: 0.125rem;
		min-width: 0;
	}

	.snapshot-label {
		font-size: 0.875rem;
		font-weight: 600;
		color: #1e293b;
		cursor: pointer;
	}

	.snapshot-hint {
		margin: 0;
		font-size: 0.8125rem;
		color: #64748b;
		overflow-wrap: anywhere;
	}
</style>
