<script lang="ts">
	import type { ExportProgress } from '$lib/api/client';

	interface Props {
		progress: ExportProgress;
	}

	let { progress }: Props = $props();

	// Format phase name for display
	function formatPhase(phase: string): string {
		// Capitalize first letter and handle common phase names
		const phaseMap: Record<string, string> = {
			persons: 'Exporting people',
			families: 'Exporting families',
			sources: 'Exporting sources',
			citations: 'Exporting citations',
			events: 'Exporting events',
			notes: 'Exporting notes',
			header: 'Writing header',
			trailer: 'Finalizing',
			init: 'Initializing'
		};
		return phaseMap[phase.toLowerCase()] || phase.charAt(0).toUpperCase() + phase.slice(1);
	}
</script>

<div
	class="export-progress"
	role="progressbar"
	aria-valuenow={progress.percentage}
	aria-valuemin={0}
	aria-valuemax={100}
	aria-label="Export progress: {progress.percentage.toFixed(0)}% complete - {formatPhase(progress.phase)}"
>
	<div class="progress-header">
		<span class="phase-text">{formatPhase(progress.phase)}</span>
		<span class="percentage-text">{progress.percentage.toFixed(0)}%</span>
	</div>
	<div class="progress-bar-container">
		<div class="progress-bar-fill" style="width: {progress.percentage}%"></div>
	</div>
	{#if progress.total > 0}
		<div class="progress-detail">
			<span class="sr-only">Progress: </span>
			{progress.current} of {progress.total} records
		</div>
	{/if}
</div>

<style>
	.export-progress {
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

	.progress-detail {
		margin-top: 0.25rem;
		font-size: 0.75rem;
		color: #64748b;
		text-align: right;
		font-variant-numeric: tabular-nums;
	}

	/* Screen reader only */
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

	/* Responsive adjustments */
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
