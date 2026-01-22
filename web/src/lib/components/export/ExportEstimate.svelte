<script lang="ts">
	import { onMount } from 'svelte';
	import { api, type ExportEstimate } from '$lib/api/client';

	interface Props {
		onEstimateLoaded?: (estimate: ExportEstimate) => void;
	}

	let { onEstimateLoaded }: Props = $props();

	let estimate: ExportEstimate | null = $state(null);
	let loading = $state(true);
	let error: string | null = $state(null);

	onMount(async () => {
		await loadEstimate();
	});

	async function loadEstimate() {
		loading = true;
		error = null;
		try {
			estimate = await api.getExportEstimate();
			onEstimateLoaded?.(estimate);
		} catch {
			error = 'Unable to estimate export size';
		} finally {
			loading = false;
		}
	}

	function formatBytes(bytes: number): string {
		if (bytes < 1024) return `${bytes} B`;
		if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
		return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
	}

	// Expose estimate for parent components
	export function getEstimate(): ExportEstimate | null {
		return estimate;
	}

	export function refresh() {
		loadEstimate();
	}
</script>

<div class="export-estimate" aria-live="polite">
	{#if loading}
		<div class="estimate-loading">
			<span class="spinner" aria-hidden="true"></span>
			<span class="text-gray-500">Calculating export size...</span>
		</div>
	{:else if error}
		<span class="estimate-error text-gray-400">{error}</span>
	{:else if estimate}
		<div class="estimate-info">
			<div class="estimate-size">
				<svg
					class="icon"
					viewBox="0 0 24 24"
					fill="none"
					stroke="currentColor"
					stroke-width="2"
					aria-hidden="true"
				>
					<path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z" />
					<polyline points="14 2 14 8 20 8" />
				</svg>
				<span class="size-text">
					Estimated size: <strong>{formatBytes(estimate.estimated_bytes)}</strong>
				</span>
			</div>
			<div class="estimate-records">
				<span class="record-count">{estimate.total_records} total records</span>
				<span class="record-breakdown">
					({estimate.person_count} people, {estimate.family_count} families)
				</span>
			</div>
			{#if estimate.is_large_export}
				<div class="large-export-warning" role="alert">
					<svg
						class="warning-icon"
						viewBox="0 0 24 24"
						fill="none"
						stroke="currentColor"
						stroke-width="2"
						aria-hidden="true"
					>
						<path
							d="M10.29 3.86L1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z"
						/>
						<line x1="12" y1="9" x2="12" y2="13" />
						<line x1="12" y1="17" x2="12.01" y2="17" />
					</svg>
					<span>Large export - may take a moment</span>
				</div>
			{/if}
		</div>
	{/if}
</div>

<style>
	.export-estimate {
		font-size: 0.875rem;
	}

	.estimate-loading {
		display: flex;
		align-items: center;
		gap: 0.5rem;
	}

	.spinner {
		width: 1rem;
		height: 1rem;
		border: 2px solid #e2e8f0;
		border-top-color: #3b82f6;
		border-radius: 50%;
		animation: spin 0.6s linear infinite;
	}

	@keyframes spin {
		to {
			transform: rotate(360deg);
		}
	}

	.estimate-error {
		font-style: italic;
	}

	.estimate-info {
		display: flex;
		flex-direction: column;
		gap: 0.375rem;
	}

	.estimate-size {
		display: flex;
		align-items: center;
		gap: 0.375rem;
		color: #475569;
	}

	.icon {
		width: 1rem;
		height: 1rem;
		color: #64748b;
		flex-shrink: 0;
	}

	.size-text {
		color: #475569;
	}

	.size-text strong {
		color: #1e293b;
		font-weight: 600;
	}

	.estimate-records {
		color: #64748b;
		font-size: 0.8125rem;
		margin-left: 1.375rem;
	}

	.record-count {
		font-weight: 500;
	}

	.record-breakdown {
		color: #94a3b8;
	}

	.large-export-warning {
		display: flex;
		align-items: center;
		gap: 0.375rem;
		margin-top: 0.25rem;
		padding: 0.5rem 0.75rem;
		background: #fefce8;
		border: 1px solid #fde047;
		border-radius: 6px;
		color: #a16207;
		font-size: 0.8125rem;
	}

	.warning-icon {
		width: 1rem;
		height: 1rem;
		color: #ca8a04;
		flex-shrink: 0;
	}

	/* Responsive adjustments */
	@media (max-width: 480px) {
		.estimate-records {
			margin-left: 0;
		}

		.record-breakdown {
			display: block;
			margin-top: 0.125rem;
		}
	}
</style>
