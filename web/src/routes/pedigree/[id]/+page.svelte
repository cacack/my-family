<script lang="ts">
	import { page } from '$app/stores';
	import { goto } from '$app/navigation';
	import { api, type Pedigree } from '$lib/api/client';
	import PedigreeChart, { type LayoutMode } from '$lib/components/PedigreeChart.svelte';

	let pedigree: Pedigree | null = $state(null);
	let error: string | null = $state(null);
	let loading = $state(true);
	let generations = $state(4);
	let layout: LayoutMode = $state('compact');
	let chart: PedigreeChart;

	async function loadPedigree(personId: string, gens: number) {
		loading = true;
		error = null;
		try {
			pedigree = await api.getPedigree(personId, gens);
		} catch (e) {
			error = (e as { message?: string }).message || 'Failed to load pedigree';
			pedigree = null;
		} finally {
			loading = false;
		}
	}

	function handlePersonClick(personId: string) {
		goto(`/pedigree/${personId}`);
	}

	function handleGenerationsChange(e: Event) {
		const select = e.target as HTMLSelectElement;
		generations = parseInt(select.value, 10);
		loadPedigree($page.params.id, generations);
	}

	$effect(() => {
		const personId = $page.params.id;
		if (personId) {
			loadPedigree(personId, generations);
		}
	});
</script>

<svelte:head>
	<title>Pedigree Chart | My Family</title>
</svelte:head>

<div class="pedigree-page">
	<header class="page-header">
		<div class="header-left">
			<a href="/" class="back-link">&larr; Back</a>
			<h1>Pedigree Chart</h1>
		</div>
		<div class="controls">
			<label>
				Generations:
				<select value={generations} onchange={handleGenerationsChange}>
					<option value={2}>2</option>
					<option value={3}>3</option>
					<option value={4}>4</option>
					<option value={5}>5</option>
					<option value={6}>6</option>
				</select>
			</label>
			<div class="layout-toggle">
				<button
					class:active={layout === 'compact'}
					onclick={() => (layout = 'compact')}
					title="Compact layout">Compact</button>
				<button
					class:active={layout === 'standard'}
					onclick={() => (layout = 'standard')}
					title="Standard layout">Standard</button>
				<button
					class:active={layout === 'wide'}
					onclick={() => (layout = 'wide')}
					title="Wide layout">Wide</button>
			</div>
			<div class="zoom-controls">
				<button onclick={() => chart?.zoomIn()} title="Zoom In">+</button>
				<button onclick={() => chart?.zoomOut()} title="Zoom Out">-</button>
				<button onclick={() => chart?.resetZoom()} title="Reset Zoom">Reset</button>
			</div>
		</div>
	</header>

	<main class="chart-container">
		{#if loading}
			<div class="loading">Loading pedigree...</div>
		{:else if error}
			<div class="error">{error}</div>
		{:else if pedigree}
			<PedigreeChart bind:this={chart} data={pedigree.root} {layout} onPersonClick={handlePersonClick} />
			<p class="hint">Click on any person to view their pedigree. Scroll to zoom, drag to pan.</p>
		{:else}
			<div class="empty">No pedigree data available.</div>
		{/if}
	</main>
</div>

<style>
	.pedigree-page {
		display: flex;
		flex-direction: column;
		height: 100vh;
		background: #f8fafc;
	}

	.page-header {
		display: flex;
		justify-content: space-between;
		align-items: center;
		padding: 1rem 1.5rem;
		background: white;
		border-bottom: 1px solid #e2e8f0;
	}

	.header-left {
		display: flex;
		align-items: center;
		gap: 1rem;
	}

	.back-link {
		color: #64748b;
		text-decoration: none;
		font-size: 0.875rem;
	}

	.back-link:hover {
		color: #3b82f6;
	}

	h1 {
		margin: 0;
		font-size: 1.5rem;
		color: #1e293b;
	}

	.controls {
		display: flex;
		align-items: center;
		gap: 1.5rem;
	}

	.controls label {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		font-size: 0.875rem;
		color: #475569;
	}

	.controls select {
		padding: 0.375rem 0.75rem;
		border: 1px solid #cbd5e1;
		border-radius: 6px;
		background: white;
		font-size: 0.875rem;
	}

	.layout-toggle {
		display: flex;
		gap: 0;
	}

	.layout-toggle button {
		padding: 0.375rem 0.625rem;
		border: 1px solid #cbd5e1;
		background: white;
		cursor: pointer;
		font-size: 0.8125rem;
		color: #64748b;
	}

	.layout-toggle button:first-child {
		border-radius: 6px 0 0 6px;
	}

	.layout-toggle button:last-child {
		border-radius: 0 6px 6px 0;
	}

	.layout-toggle button:not(:first-child) {
		border-left: none;
	}

	.layout-toggle button:hover {
		background: #f1f5f9;
	}

	.layout-toggle button.active {
		background: #3b82f6;
		border-color: #3b82f6;
		color: white;
	}

	.zoom-controls {
		display: flex;
		gap: 0.25rem;
	}

	.zoom-controls button {
		padding: 0.375rem 0.75rem;
		border: 1px solid #cbd5e1;
		background: white;
		border-radius: 6px;
		cursor: pointer;
		font-size: 0.875rem;
	}

	.zoom-controls button:hover {
		background: #f1f5f9;
	}

	.chart-container {
		flex: 1;
		padding: 1rem;
		overflow: hidden;
	}

	.loading,
	.error,
	.empty {
		display: flex;
		align-items: center;
		justify-content: center;
		height: 100%;
		color: #64748b;
		font-size: 1rem;
	}

	.error {
		color: #dc2626;
	}

	.hint {
		text-align: center;
		color: #94a3b8;
		font-size: 0.75rem;
		margin-top: 0.5rem;
	}
</style>
