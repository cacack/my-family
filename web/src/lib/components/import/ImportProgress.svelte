<script lang="ts">
	import type { ImportProgress } from '$lib/api/client';

	interface Props {
		progress: ImportProgress;
	}

	let { progress }: Props = $props();

	// Whether a meaningful percentage is available (total size known).
	let determinate = $derived(progress.percent >= 0 && progress.total_bytes > 0);

	// Format a byte count as a human-readable size.
	function formatBytes(bytes: number): string {
		if (bytes < 0) return 'unknown';
		if (bytes < 1024) return `${bytes} B`;
		const units = ['KB', 'MB', 'GB'];
		let value = bytes / 1024;
		let unit = 0;
		while (value >= 1024 && unit < units.length - 1) {
			value /= 1024;
			unit++;
		}
		return `${value.toFixed(1)} ${units[unit]}`;
	}
</script>

<div
	class="import-progress"
	role="progressbar"
	aria-valuenow={determinate ? progress.percent : undefined}
	aria-valuemin={0}
	aria-valuemax={100}
	aria-label={determinate
		? `Import progress: ${progress.percent}% complete`
		: 'Importing GEDCOM file'}
>
	<div class="progress-header">
		<span class="phase-text">Importing GEDCOM file</span>
		{#if determinate}
			<span class="percentage-text">{progress.percent}%</span>
		{/if}
	</div>
	<div class="progress-bar-container" class:indeterminate={!determinate}>
		{#if determinate}
			<div class="progress-bar-fill" style="width: {progress.percent}%"></div>
		{:else}
			<div class="progress-bar-fill indeterminate-fill"></div>
		{/if}
	</div>
	<div class="progress-detail">
		<span class="sr-only">Progress: </span>
		{formatBytes(progress.bytes_read)}{#if progress.total_bytes > 0}
			of {formatBytes(progress.total_bytes)}{/if} read
	</div>
</div>

<style>
	.import-progress {
		width: 100%;
	}

	.progress-header {
		display: flex;
		justify-content: space-between;
		align-items: center;
		margin-bottom: 0.375rem;
	}

	.phase-text {
		font-size: 0.875rem;
		font-weight: 500;
		color: #475569;
	}

	.percentage-text {
		font-size: 0.875rem;
		font-weight: 600;
		color: #1e293b;
		font-variant-numeric: tabular-nums;
	}

	.progress-bar-container {
		width: 100%;
		height: 0.5rem;
		background: #e2e8f0;
		border-radius: 9999px;
		overflow: hidden;
	}

	.progress-bar-fill {
		height: 100%;
		background: #3b82f6;
		border-radius: 9999px;
		transition: width 0.3s ease-out;
	}

	/* Indeterminate animation when the total size is unknown. */
	.indeterminate-fill {
		width: 40%;
		animation: indeterminate 1.2s ease-in-out infinite;
	}

	@keyframes indeterminate {
		0% {
			transform: translateX(-100%);
		}
		100% {
			transform: translateX(250%);
		}
	}

	.progress-detail {
		margin-top: 0.25rem;
		font-size: 0.75rem;
		color: #64748b;
		text-align: right;
		font-variant-numeric: tabular-nums;
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

	@media (max-width: 480px) {
		.progress-header {
			flex-direction: column;
			align-items: flex-start;
			gap: 0.125rem;
		}

		.percentage-text {
			font-size: 0.8125rem;
		}
	}
</style>
