<script lang="ts">
	import { page } from '$app/stores';
	import { goto } from '$app/navigation';
	import { api, type Pedigree } from '$lib/api/client';
	import PedigreeChart, { type LayoutMode } from '$lib/components/PedigreeChart.svelte';
	import { createShortcutHandler } from '$lib/keyboard/useShortcuts.svelte';

	let pedigree: Pedigree | null = $state(null);
	let error: string | null = $state(null);
	let loading = $state(true);
	let generations = $state(4);
	let layout: LayoutMode = $state('compact');
	let chart: PedigreeChart;
	let selectedPersonId: string | null = $state(null);
	let announceMessage: string = $state('');

	// Screen reader announcement helper
	function announce(message: string) {
		announceMessage = '';
		// Small delay to ensure screen readers pick up the change
		setTimeout(() => {
			announceMessage = message;
		}, 50);
	}

	// Navigation handlers for keyboard shortcuts
	function navigateToFather() {
		if (!chart) return;
		const fatherId = chart.getFatherId();
		if (fatherId) {
			selectedPersonId = fatherId;
			const node = pedigree?.root;
			// Find the person's name for announcement
			const name = findPersonName(fatherId);
			announce(`Navigated to father: ${name || 'Unknown'}`);
		}
	}

	function navigateToMother() {
		if (!chart) return;
		const motherId = chart.getMotherId();
		if (motherId) {
			selectedPersonId = motherId;
			const name = findPersonName(motherId);
			announce(`Navigated to mother: ${name || 'Unknown'}`);
		}
	}

	function navigateToRoot() {
		if (!chart) return;
		const rootId = chart.getRootId();
		if (rootId) {
			selectedPersonId = rootId;
			const name = findPersonName(rootId);
			announce(`Navigated to root: ${name || 'Unknown'}`);
		}
	}

	function navigateToSpouse() {
		if (!chart) return;
		const spouseId = chart.getSpouseId();
		if (spouseId) {
			selectedPersonId = spouseId;
			const name = findPersonName(spouseId);
			announce(`Navigated to spouse: ${name || 'Unknown'}`);
		}
		// Note: spouse navigation not available in current pedigree data
	}

	function viewPersonDetail() {
		if (selectedPersonId) {
			goto(`/persons/${selectedPersonId}`);
		}
	}

	function handleZoomIn() {
		chart?.zoomIn();
	}

	function handleZoomOut() {
		chart?.zoomOut();
	}

	function handleResetView() {
		chart?.resetZoom();
		announce('View reset');
	}

	// Helper to find person name from pedigree data
	function findPersonName(personId: string): string | null {
		if (!pedigree?.root) return null;
		return searchForPerson(pedigree.root, personId);
	}

	function searchForPerson(node: import('$lib/api/client').PedigreeNode, personId: string): string | null {
		if (node.id === personId) {
			const given = node.given_name || '';
			const surname = node.surname || '';
			return `${given} ${surname}`.trim() || null;
		}
		if (node.father) {
			const result = searchForPerson(node.father, personId);
			if (result) return result;
		}
		if (node.mother) {
			const result = searchForPerson(node.mother, personId);
			if (result) return result;
		}
		return null;
	}

	// Set up keyboard shortcuts for pedigree context
	const { handleKeydown } = createShortcutHandler('pedigree', {
		'navigate-father': navigateToFather,
		'navigate-mother': navigateToMother,
		'navigate-root': navigateToRoot,
		'navigate-spouse': navigateToSpouse,
		'view-person-detail': viewPersonDetail,
		'zoom-in': handleZoomIn,
		'zoom-out': handleZoomOut,
		'reset-view': handleResetView
	});

	async function loadPedigree(personId: string, gens: number) {
		loading = true;
		error = null;
		try {
			pedigree = await api.getPedigree(personId, gens);
			// Initialize selection to the root person
			if (pedigree?.root?.id) {
				selectedPersonId = pedigree.root.id;
			}
		} catch (e) {
			error = (e as { message?: string }).message || 'Failed to load pedigree';
			pedigree = null;
			selectedPersonId = null;
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
		const personId = $page.params.id;
		if (personId) {
			loadPedigree(personId, generations);
		}
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

<svelte:window onkeydown={handleKeydown} />

<!-- Screen reader announcements for navigation -->
<div
	role="status"
	aria-live="polite"
	aria-atomic="true"
	class="sr-only"
>
	{announceMessage}
</div>

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
			<PedigreeChart bind:this={chart} data={pedigree.root} {layout} {selectedPersonId} onPersonClick={handlePersonClick} />
			<p class="hint">Click on any person to view their pedigree. Scroll to zoom, drag to pan. Use arrow keys to navigate, +/- to zoom, R to reset.</p>
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

	/* Screen reader only - visually hidden but accessible */
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
