<script lang="ts">
	import { page } from '$app/stores';
	import { goto } from '$app/navigation';
	import { api, type Descendancy, type DescendancyNode } from '$lib/api/client';
	import DescendancyChart, { type LayoutMode } from '$lib/components/DescendancyChart.svelte';
	import { createShortcutHandler } from '$lib/keyboard/useShortcuts.svelte';

	let descendancy: Descendancy | null = $state(null);
	let error: string | null = $state(null);
	let loading = $state(true);
	let generations = $state(4);
	let layout: LayoutMode = $state('compact');
	let chart: DescendancyChart;
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
	function navigateToFirstChild() {
		if (!chart) return;
		const childId = chart.getFirstChildId();
		if (childId) {
			selectedPersonId = childId;
			const name = findPersonName(childId);
			announce(`Navigated to child: ${name || 'Unknown'}`);
		}
	}

	function navigateToParent() {
		if (!chart) return;
		const parentId = chart.getParentId();
		if (parentId) {
			selectedPersonId = parentId;
			const name = findPersonName(parentId);
			announce(`Navigated to parent: ${name || 'Unknown'}`);
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

	function navigateToNextSibling() {
		if (!chart) return;
		const siblingId = chart.getNextSiblingId();
		if (siblingId) {
			selectedPersonId = siblingId;
			const name = findPersonName(siblingId);
			announce(`Navigated to next sibling: ${name || 'Unknown'}`);
		}
	}

	function navigateToPrevSibling() {
		if (!chart) return;
		const siblingId = chart.getPrevSiblingId();
		if (siblingId) {
			selectedPersonId = siblingId;
			const name = findPersonName(siblingId);
			announce(`Navigated to previous sibling: ${name || 'Unknown'}`);
		}
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

	// Helper to find person name from descendancy data
	function findPersonName(personId: string): string | null {
		if (!descendancy?.root) return null;
		return searchForPerson(descendancy.root, personId);
	}

	function searchForPerson(node: DescendancyNode, personId: string): string | null {
		if (node.id === personId) {
			const given = node.given_name || '';
			const surname = node.surname || '';
			return `${given} ${surname}`.trim() || null;
		}
		// Search in spouses
		if (node.spouses) {
			for (const spouse of node.spouses) {
				if (spouse.id === personId) {
					const given = spouse.given_name || '';
					const surname = spouse.surname || '';
					return `${given} ${surname}`.trim() || null;
				}
			}
		}
		// Search in children
		if (node.children) {
			for (const child of node.children) {
				const result = searchForPerson(child, personId);
				if (result) return result;
			}
		}
		return null;
	}

	// Set up keyboard shortcuts for descendancy context
	const { handleKeydown } = createShortcutHandler('descendancy', {
		'navigate-first-child': navigateToFirstChild,
		'navigate-parent': navigateToParent,
		'navigate-root': navigateToRoot,
		'navigate-next-sibling': navigateToNextSibling,
		'navigate-prev-sibling': navigateToPrevSibling,
		'view-person-detail': viewPersonDetail,
		'zoom-in': handleZoomIn,
		'zoom-out': handleZoomOut,
		'reset-view': handleResetView
	});

	async function loadDescendancy(personId: string, gens: number) {
		loading = true;
		error = null;
		try {
			descendancy = await api.getDescendancy(personId, gens);
			// Initialize selection to the root person
			if (descendancy?.root?.id) {
				selectedPersonId = descendancy.root.id;
			}
		} catch (e) {
			error = (e as { message?: string }).message || 'Failed to load descendancy';
			descendancy = null;
			selectedPersonId = null;
		} finally {
			loading = false;
		}
	}

	function handlePersonClick(personId: string) {
		goto(`/descendancy/${personId}`);
	}

	function handleGenerationsChange(e: Event) {
		const select = e.target as HTMLSelectElement;
		generations = parseInt(select.value, 10);
		const personId = $page.params.id;
		if (personId) {
			loadDescendancy(personId, generations);
		}
	}

	$effect(() => {
		const personId = $page.params.id;
		if (personId) {
			loadDescendancy(personId, generations);
		}
	});
</script>

<svelte:head>
	<title>Descendancy Chart | My Family</title>
</svelte:head>

<svelte:window onkeydown={handleKeydown} />

<!-- Screen reader announcements for navigation -->
<div role="status" aria-live="polite" aria-atomic="true" class="sr-only">
	{announceMessage}
</div>

<div class="descendancy-page">
	<header class="page-header">
		<div class="header-left">
			<a href="/" class="back-link">&larr; Back</a>
			<h1>Descendancy Chart</h1>
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
					<option value={7}>7</option>
					<option value={8}>8</option>
					<option value={9}>9</option>
					<option value={10}>10</option>
				</select>
			</label>
			<div class="layout-toggle">
				<button
					class:active={layout === 'compact'}
					onclick={() => (layout = 'compact')}
					title="Compact layout"
				>
					Compact
				</button>
				<button
					class:active={layout === 'standard'}
					onclick={() => (layout = 'standard')}
					title="Standard layout"
				>
					Standard
				</button>
				<button
					class:active={layout === 'wide'}
					onclick={() => (layout = 'wide')}
					title="Wide layout"
				>
					Wide
				</button>
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
			<div class="loading">Loading descendancy...</div>
		{:else if error}
			<div class="error">{error}</div>
		{:else if descendancy}
			<DescendancyChart
				bind:this={chart}
				data={descendancy.root}
				{layout}
				{selectedPersonId}
				onPersonClick={handlePersonClick}
			/>
			<div class="chart-info">
				<span class="stat">
					{descendancy.total_descendants} descendant{descendancy.total_descendants !== 1
						? 's'
						: ''}
				</span>
				<span class="stat">{descendancy.max_generation} generation{descendancy.max_generation !== 1 ? 's' : ''}</span>
			</div>
			<p class="hint">
				Click on any person to view their descendants. Scroll to zoom, drag to pan. Use arrow keys
				to navigate, +/- to zoom, R to reset.
			</p>
		{:else}
			<div class="empty">No descendancy data available.</div>
		{/if}
	</main>
</div>

<style>
	.descendancy-page {
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

	.chart-info {
		display: flex;
		justify-content: center;
		gap: 2rem;
		margin-top: 0.5rem;
	}

	.stat {
		font-size: 0.875rem;
		color: #64748b;
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
