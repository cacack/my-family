<script lang="ts">
	import { page } from '$app/stores';
	import { goto } from '$app/navigation';
	import { api, recordLoadError, type Descendancy, type DescendancyNode } from '$lib/api/client';
	import DescendancyChart, { type LayoutMode } from '$lib/components/DescendancyChart.svelte';
	import { CHART_DEFAULTS, readChartOptions, setQuery } from '$lib/utils/urlState';
	import { createShortcutHandler } from '$lib/keyboard/useShortcuts.svelte';
	import ErrorState from '$lib/components/ErrorState.svelte';
	import PageHeader from '$lib/components/PageHeader.svelte';

	let descendancy: Descendancy | null = $state(null);
	let error: string | null = $state(null);
	let loading = $state(true);
	// Generations and layout live in the URL (see urlState.ts), so reload, Back
	// and a copied link keep them; both refine the view, so they replace.
	const personId = $derived($page.params.id);
	const chartOptions = $derived(readChartOptions($page.url.searchParams));
	const generations = $derived(chartOptions.generations);
	const layout: LayoutMode = $derived(chartOptions.layout);
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
					return spouse.name || null;
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

	// Re-rooting, a generations change or Back can start loads faster than they
	// finish: only the latest one may land.
	let loadSeq = 0;

	async function loadDescendancy(personId: string, gens: number) {
		const seq = ++loadSeq;
		loading = true;
		error = null;
		try {
			const result = await api.getDescendancy(personId, gens);
			if (seq !== loadSeq) return;
			descendancy = result;
			// Initialize selection to the root person
			if (descendancy?.root?.id) {
				selectedPersonId = descendancy.root.id;
			}
		} catch (e) {
			if (seq !== loadSeq) return;
			error = recordLoadError(e, 'person');
			descendancy = null;
			selectedPersonId = null;
		} finally {
			if (seq === loadSeq) loading = false;
		}
	}

	// Re-rooting the chart keeps the current options.
	function handlePersonClick(id: string) {
		goto(`/descendancy/${id}${$page.url.search}`);
	}

	function setChartOption(updates: { gens?: number; layout?: LayoutMode }) {
		setQuery($page.url, updates, { defaults: CHART_DEFAULTS });
	}

	function handleGenerationsChange(e: Event) {
		setChartOption({ gens: parseInt((e.target as HTMLSelectElement).value, 10) });
	}

	$effect(() => {
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
	<div class="chart-container">
		<!-- Inside the container on purpose: PageHeader has no padding, so the container's padding is its gutter. -->
		<PageHeader title="Descendancy Chart" back={{ href: '/', label: 'Back' }}>
			{#snippet actions()}
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
							aria-pressed={layout === 'compact'}
							onclick={() => setChartOption({ layout: 'compact' })}
							title="Compact layout"
						>
							Compact
						</button>
						<button
							aria-pressed={layout === 'standard'}
							onclick={() => setChartOption({ layout: 'standard' })}
							title="Standard layout"
						>
							Standard
						</button>
						<button
							aria-pressed={layout === 'wide'}
							onclick={() => setChartOption({ layout: 'wide' })}
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
			{/snippet}
		</PageHeader>
		{#if loading}
			<div class="loading">Loading descendancy...</div>
		{:else if error}
			<ErrorState message={error} onRetry={() => loadDescendancy($page.params.id ?? '', generations)} />
		{:else if descendancy}
			<div class="chart-area">
				<DescendancyChart
					bind:this={chart}
					data={descendancy.root}
					{layout}
					{selectedPersonId}
					onPersonClick={handlePersonClick}
				/>
			</div>
			<div class="chart-info">
				<span class="stat">
					{descendancy.total_descendants} descendant{descendancy.total_descendants !== 1
						? 's'
						: ''}
				</span>
				<span class="stat">{descendancy.max_generation} generation{descendancy.max_generation !== 1 ? 's' : ''}</span>
			</div>
			<p class="hint">
				Click on any person to view their descendants. Scroll to zoom, drag to pan (two fingers on a touchscreen). Use arrow keys
				to navigate, +/- to zoom, R to reset.
			</p>
		{:else}
			<div class="empty">No descendancy data available.</div>
		{/if}
	</div>
</div>

<style>
	/* Fills the viewport under the app header (see +layout.svelte); on a short
	   screen the page keeps its minimum height and scrolls instead. */
	.descendancy-page {
		display: flex;
		flex-direction: column;
		flex: 1;
		background: #f8fafc;
	}

	.controls {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 0.75rem 1.5rem;
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
		color: var(--color-text-muted);
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

	.layout-toggle button[aria-pressed='true'] {
		background: var(--color-action);
		border-color: var(--color-action);
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
		display: flex;
		flex-direction: column;
		flex: 1;
		padding: 1rem;
	}

	.chart-area {
		flex: 1;
	}

	.loading,
	.empty {
		display: flex;
		align-items: center;
		justify-content: center;
		height: 100%;
		color: var(--color-text-muted);
		font-size: 1rem;
	}

	.chart-info {
		display: flex;
		justify-content: center;
		gap: 2rem;
		margin-top: 0.5rem;
	}

	.stat {
		font-size: 0.875rem;
		color: var(--color-text-muted);
	}

	.hint {
		text-align: center;
		color: var(--color-text-muted);
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
