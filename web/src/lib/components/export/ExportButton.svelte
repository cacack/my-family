<script lang="ts">
	import { onMount } from 'svelte';
	import { api, type ExportEstimate, type ExportProgress } from '$lib/api/client';
	import ExportEstimateDisplay from './ExportEstimate.svelte';
	import ExportProgressBar from './ExportProgress.svelte';
	import ExportConfirmDialog from './ExportConfirmDialog.svelte';

	interface Props {
		/** Label for the export button */
		label?: string;
		/** Threshold in bytes for showing confirmation dialog (default: 10MB) */
		confirmThresholdBytes?: number;
		/** Threshold in records for showing progress bar (default: 1000) */
		progressThresholdRecords?: number;
		/** Whether to show estimation before export */
		showEstimate?: boolean;
		/** Called when export starts */
		onExportStart?: () => void;
		/** Called when export completes successfully */
		onExportComplete?: (data: string) => void;
		/** Called when export fails */
		onExportError?: (error: string) => void;
	}

	let {
		label = 'Export GEDCOM',
		confirmThresholdBytes = 10 * 1024 * 1024, // 10MB
		progressThresholdRecords = 1000,
		showEstimate = true,
		onExportStart,
		onExportComplete,
		onExportError
	}: Props = $props();

	let estimate: ExportEstimate | null = $state(null);
	let exporting = $state(false);
	let showConfirmDialog = $state(false);
	let progress: ExportProgress | null = $state(null);
	let error: string | null = $state(null);

	// Determine if we should show progress for this export
	let shouldShowProgress = $derived.by(() => {
		if (estimate === null) return false;
		return (
			estimate.total_records >= progressThresholdRecords ||
			estimate.estimated_bytes >= 1024 * 1024
		);
	});

	// Determine if we need confirmation
	let needsConfirmation = $derived.by(() => {
		if (estimate === null) return false;
		return estimate.estimated_bytes >= confirmThresholdBytes;
	});

	function handleEstimateLoaded(loadedEstimate: ExportEstimate) {
		estimate = loadedEstimate;
	}

	function handleExportClick() {
		if (needsConfirmation && !showConfirmDialog) {
			showConfirmDialog = true;
		} else {
			startExport();
		}
	}

	function handleConfirm() {
		showConfirmDialog = false;
		startExport();
	}

	function handleCancel() {
		showConfirmDialog = false;
	}

	async function startExport() {
		exporting = true;
		error = null;
		onExportStart?.();

		// Simulate progress for large exports
		if (shouldShowProgress && estimate) {
			simulateProgress(estimate);
		}

		try {
			const gedcom = await api.exportGedcom();

			// Complete the progress
			if (shouldShowProgress) {
				progress = {
					phase: 'complete',
					current: estimate?.total_records ?? 0,
					total: estimate?.total_records ?? 0,
					percentage: 100
				};
			}

			// Trigger download
			const blob = new Blob([gedcom], { type: 'text/plain' });
			const url = URL.createObjectURL(blob);
			const a = document.createElement('a');
			a.href = url;
			a.download = 'export.ged';
			a.click();
			URL.revokeObjectURL(url);

			onExportComplete?.(gedcom);

			// Reset progress after a brief delay
			setTimeout(() => {
				progress = null;
			}, 1500);
		} catch (e) {
			const errorMessage = (e as { message?: string }).message || 'Export failed';
			error = errorMessage;
			onExportError?.(errorMessage);
		} finally {
			exporting = false;
		}
	}

	function simulateProgress(est: ExportEstimate) {
		// Simulate progress phases based on record counts
		const phases = [
			{ name: 'header', records: 0, weight: 0.02 },
			{ name: 'persons', records: est.person_count, weight: 0.45 },
			{ name: 'families', records: est.family_count, weight: 0.25 },
			{ name: 'sources', records: est.source_count, weight: 0.15 },
			{ name: 'events', records: est.event_count, weight: 0.1 },
			{ name: 'trailer', records: 0, weight: 0.03 }
		];

		let currentPhaseIndex = 0;
		let currentProgress = 0;

		function updateProgress() {
			if (!exporting || currentPhaseIndex >= phases.length) {
				return;
			}

			const phase = phases[currentPhaseIndex];
			const phaseProgress = Math.min(1, currentProgress / (phase.weight * 100));

			if (phaseProgress >= 1) {
				currentPhaseIndex++;
				if (currentPhaseIndex < phases.length) {
					setTimeout(updateProgress, 100);
				}
				return;
			}

			const totalPercentage = phases.slice(0, currentPhaseIndex).reduce((sum, p) => sum + p.weight * 100, 0) + phaseProgress * phase.weight * 100;

			progress = {
				phase: phase.name,
				current: Math.floor(phaseProgress * phase.records),
				total: phase.records,
				percentage: Math.min(95, totalPercentage) // Cap at 95% until actual completion
			};

			currentProgress += 5 + Math.random() * 10;
			setTimeout(updateProgress, 150 + Math.random() * 100);
		}

		updateProgress();
	}
</script>

<div class="export-button-container">
	{#if showEstimate && !exporting}
		<ExportEstimateDisplay onEstimateLoaded={handleEstimateLoaded} />
	{/if}

	{#if exporting && progress}
		<div class="progress-container" aria-live="polite">
			<ExportProgressBar {progress} />
		</div>
	{:else if exporting}
		<div class="exporting-message" aria-live="polite">
			<span class="spinner" aria-hidden="true"></span>
			<span>Preparing export...</span>
		</div>
	{/if}

	{#if error}
		<p class="error-message" role="alert">{error}</p>
	{/if}

	<button
		type="button"
		class="btn btn-export"
		onclick={handleExportClick}
		disabled={exporting}
		aria-busy={exporting}
	>
		{#if exporting}
			<span class="spinner" aria-hidden="true"></span>
			Exporting...
		{:else}
			<svg
				class="icon"
				viewBox="0 0 24 24"
				fill="none"
				stroke="currentColor"
				stroke-width="2"
				aria-hidden="true"
			>
				<path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4" />
				<polyline points="7 10 12 15 17 10" />
				<line x1="12" y1="15" x2="12" y2="3" />
			</svg>
			{label}
		{/if}
	</button>
</div>

{#if showConfirmDialog && estimate}
	<ExportConfirmDialog {estimate} onConfirm={handleConfirm} onCancel={handleCancel} />
{/if}

<style>
	.export-button-container {
		display: flex;
		flex-direction: column;
		gap: 0.75rem;
	}

	.progress-container {
		padding: 0.75rem;
		background: #f8fafc;
		border: 1px solid #e2e8f0;
		border-radius: 8px;
	}

	.exporting-message {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		padding: 0.75rem;
		background: #f8fafc;
		border: 1px solid #e2e8f0;
		border-radius: 8px;
		font-size: 0.875rem;
		color: #475569;
	}

	.spinner {
		width: 1rem;
		height: 1rem;
		border: 2px solid #e2e8f0;
		border-top-color: #3b82f6;
		border-radius: 50%;
		animation: spin 0.6s linear infinite;
		flex-shrink: 0;
	}

	@keyframes spin {
		to {
			transform: rotate(360deg);
		}
	}

	.error-message {
		margin: 0;
		padding: 0.75rem;
		background: #fef2f2;
		border: 1px solid #fecaca;
		border-radius: 8px;
		color: #dc2626;
		font-size: 0.875rem;
	}

	.btn {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		gap: 0.5rem;
		padding: 0.625rem 1.25rem;
		border-radius: 6px;
		font-size: 0.875rem;
		font-weight: 500;
		cursor: pointer;
		transition: all 0.15s;
	}

	.btn-export {
		background: white;
		border: 1px solid #cbd5e1;
		color: #475569;
	}

	.btn-export:hover:not(:disabled) {
		background: #f1f5f9;
		border-color: #94a3b8;
	}

	.btn-export:focus {
		outline: none;
		box-shadow: 0 0 0 3px rgba(59, 130, 246, 0.2);
	}

	.btn-export:disabled {
		opacity: 0.7;
		cursor: not-allowed;
	}

	.btn-export .spinner {
		border-color: rgba(71, 85, 105, 0.3);
		border-top-color: #475569;
	}

	.icon {
		width: 1.125rem;
		height: 1.125rem;
	}

	/* Responsive adjustments */
	@media (max-width: 480px) {
		.btn-export {
			width: 100%;
		}
	}
</style>
