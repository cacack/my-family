<script lang="ts">
	import { page } from '$app/stores';
	import { goto } from '$app/navigation';
	import { api, recordLoadError, type Pedigree } from '$lib/api/client';
	import PedigreeChart, { type LayoutMode } from '$lib/components/PedigreeChart.svelte';
	import { CHART_DEFAULTS, readChartOptions, setQuery } from '$lib/utils/urlState';
	import { createShortcutHandler } from '$lib/keyboard/useShortcuts.svelte';
	import ErrorState from '$lib/components/ErrorState.svelte';
	import PageHeader from '$lib/components/PageHeader.svelte';

	let pedigree: Pedigree | null = $state(null);
	let error: string | null = $state(null);
	let loading = $state(true);
	// Generations and layout live in the URL (see urlState.ts), so reload, Back
	// and a copied link keep them; both refine the view, so they replace.
	const personId = $derived($page.params.id);
	const chartOptions = $derived(readChartOptions($page.url.searchParams));
	const generations = $derived(chartOptions.generations);
	const layout: LayoutMode = $derived(chartOptions.layout);
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

	// Re-rooting, a generations change or Back can start loads faster than they
	// finish: only the latest one may land.
	let loadSeq = 0;

	async function loadPedigree(personId: string, gens: number) {
		const seq = ++loadSeq;
		loading = true;
		error = null;
		try {
			const result = await api.getPedigree(personId, gens);
			if (seq !== loadSeq) return;
			pedigree = result;
			// Initialize selection to the root person
			if (pedigree?.root?.id) {
				selectedPersonId = pedigree.root.id;
			}
		} catch (e) {
			if (seq !== loadSeq) return;
			error = recordLoadError(e, 'person');
			pedigree = null;
			selectedPersonId = null;
		} finally {
			if (seq === loadSeq) loading = false;
		}
	}

	// Re-rooting the chart keeps the current options.
	function handlePersonClick(id: string) {
		goto(`/pedigree/${id}${$page.url.search}`);
	}

	function setChartOption(updates: { gens?: number; layout?: LayoutMode }) {
		setQuery($page.url, updates, { defaults: CHART_DEFAULTS });
	}

	function handleGenerationsChange(e: Event) {
		setChartOption({ gens: parseInt((e.target as HTMLSelectElement).value, 10) });
	}

	$effect(() => {
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
	<div class="chart-container">
		<!-- Inside the container on purpose: PageHeader has no padding, so the container's padding is its gutter. -->
		<PageHeader title="Pedigree Chart" back={{ href: '/', label: 'Back' }}>
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
							title="Compact layout">Compact</button>
						<button
							aria-pressed={layout === 'standard'}
							onclick={() => setChartOption({ layout: 'standard' })}
							title="Standard layout">Standard</button>
						<button
							aria-pressed={layout === 'wide'}
							onclick={() => setChartOption({ layout: 'wide' })}
							title="Wide layout">Wide</button>
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
			<div class="loading">Loading pedigree...</div>
		{:else if error}
			<ErrorState message={error} onRetry={() => loadPedigree($page.params.id ?? '', generations)} />
		{:else if pedigree}
			<div class="chart-area">
				<PedigreeChart bind:this={chart} data={pedigree.root} {layout} {selectedPersonId} onPersonClick={handlePersonClick} />
			</div>
			<p class="hint">Click on any person to view their pedigree. Scroll to zoom, drag to pan (two fingers on a touchscreen). Use arrow keys to navigate, +/- to zoom, R to reset.</p>
		{:else}
			<div class="empty">No pedigree data available.</div>
		{/if}
	</div>
</div>

<style>
	/* Fills the viewport under the app header (see +layout.svelte); on a short
	   screen the page keeps its minimum height and scrolls instead. */
	.pedigree-page {
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
