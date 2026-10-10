<script lang="ts">
	import { tick } from 'svelte';
	import { api, formatPersonName, type MapLocation } from '$lib/api/client';
	import * as d3 from 'd3';
	import * as topojson from 'topojson-client';
	import type { Topology, GeometryCollection as TopoGeometryCollection } from 'topojson-specification';
	import { motionDuration, observeChartResize } from '$lib/utils/chart';
	import { getReducedMotion } from '$lib/stores/accessibilitySettings.svelte';
	import { loadWorldAtlas } from '$lib/utils/worldAtlas';
	import { clusterKind, clusterLabel, clusterPoints, type MapCluster } from '$lib/utils/mapClusters';
	import ErrorState from './ErrorState.svelte';

	// Markers closer than this on screen are drawn as one cluster
	const CLUSTER_RADIUS = 18;
	// Smallest tap target around a marker, in screen pixels
	const MIN_HIT_RADIUS = 12;
	const MARKER_FILL = { birth: '#3b82f6', death: '#6b7280', mixed: '#8b5cf6' };

	let container: HTMLDivElement;
	let tooltipEl: HTMLDivElement;
	let panelEl: HTMLDivElement | undefined = $state();
	let svg: d3.Selection<SVGSVGElement, unknown, null, undefined> | null = null;
	let zoom: d3.ZoomBehavior<SVGSVGElement, unknown> | null = null;
	// Loaded once on first draw and reused when the map is redrawn on resize
	let worldAtlas: Promise<Topology> | null = null;
	// Touch screens get a pinch hint instead of a scroll-wheel one
	const coarsePointer =
		typeof window !== 'undefined' && window.matchMedia?.('(pointer: coarse)').matches;
	let locations: MapLocation[] = $state([]);
	let loading = $state(true);
	let error = $state<string | null>(null);
	// The world outline failed to load; markers and the list still work without it
	let baseMapFailed = $state(false);
	// The locations of the marker the user picked (one cluster may hold several)
	let selected: MapLocation[] | null = $state(null);

	$effect(() => {
		loadData();
	});

	async function loadData() {
		try {
			loading = true;
			error = null;
			const result = await api.getMapLocations();
			locations = result.items;
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to load map data';
		} finally {
			loading = false;
		}
	}

	$effect(() => {
		if (!loading && container && locations.length > 0) {
			renderMap();
			// The height follows the width, so only width changes need a redraw
			return observeChartResize(container, renderMap);
		}
	});

	/** Retry from scratch: a failed world atlas load is cached, so drop it. */
	function retry() {
		worldAtlas = null;
		loadData();
	}

	function worldOutline(): Promise<Topology> {
		worldAtlas ??= loadWorldAtlas();
		return worldAtlas;
	}

	const locationKey = (l: MapLocation) => `${l.place}|${l.event_type}`;

	async function select(items: MapLocation[]) {
		const same =
			selected?.length === items.length &&
			selected.every((l, i) => locationKey(l) === locationKey(items[i]));
		selected = same ? null : items;
		if (selected) {
			await tick();
			const behavior = getReducedMotion() ? 'auto' : 'smooth';
			panelEl?.scrollIntoView?.({ block: 'nearest', behavior });
		}
	}

	function hideTooltip() {
		if (tooltipEl) tooltipEl.style.display = 'none';
	}

	function showTooltip(text: string) {
		if (!tooltipEl) return;
		tooltipEl.textContent = text;
		tooltipEl.style.display = 'block';
	}

	// Place the tooltip beside a point in the map, flipping it to stay inside the map
	function positionTooltip([x, y]: [number, number]) {
		if (!tooltipEl) return;
		// Measure at the left edge so the width isn't squeezed by the previous position
		tooltipEl.style.left = '0px';
		const { clientWidth: mapWidth, clientHeight: mapHeight } = container;
		const { offsetWidth: tipWidth, offsetHeight: tipHeight } = tooltipEl;
		const left = x + 12 + tipWidth > mapWidth ? x - 12 - tipWidth : x + 12;
		const top = y - 10 + tipHeight > mapHeight ? mapHeight - tipHeight : y - 10;
		tooltipEl.style.left = `${Math.max(0, left)}px`;
		tooltipEl.style.top = `${Math.max(0, top)}px`;
	}

	export function resetView() {
		if (svg && zoom) {
			svg.transition().duration(motionDuration(300)).call(zoom.transform, d3.zoomIdentity);
		}
	}

	function renderMap() {
		// Clear previous
		d3.select(container).select('svg').remove();
		hideTooltip();

		const width = container.clientWidth;
		// Tall enough to tap markers on a phone; capped on wide screens
		const height = Math.max(320, Math.min(width * 0.55, 600));

		svg = d3
			.select(container)
			.append('svg')
			.attr('width', width)
			.attr('height', height)
			.attr('viewBox', `0 0 ${width} ${height}`);
		const svgNode = svg.node()!;

		const g = svg.append('g');
		const baseLayer = g.append('g');
		const markerLayer = g.append('g');

		// Fill the height on narrow screens (the world is wider than the map then),
		// panning sideways to centre the family's locations without showing past the edge
		const projection = d3
			.geoNaturalEarth1()
			.scale(Math.max(width / 5.5, height / 2.8))
			.translate([width / 2, height / 2]);
		const halfWorld = projection([180, 0])![0] - width / 2;
		if (halfWorld > width / 2) {
			const dataX = projection([d3.mean(locations, (d) => d.longitude) ?? 0, 0])![0];
			const tx = Math.min(halfWorld, Math.max(width - halfWorld, width - dataX));
			projection.translate([tx, height / 2]);
		}

		const path = d3.geoPath().projection(projection);
		const projected = new Map(
			locations.map((d) => [d, projection([d.longitude, d.latitude]) ?? [0, 0]])
		);

		const radiusScale = d3
			.scaleSqrt()
			.domain([1, d3.max(locations, (d) => d.count) || 1])
			.range([7, 20])
			.clamp(true);
		const clusterRadius = (c: MapCluster<MapLocation>) =>
			radiusScale(d3.sum(c.items, (d) => d.count));

		// Largest first, so the biggest markers anchor their clusters
		const bySize = [...locations].sort((a, b) => b.count - a.count);
		let clusteredAt = 0;

		// Markers are re-clustered at each zoom level, so zooming in splits them apart
		function drawMarkers(k: number) {
			clusteredAt = k;
			hideTooltip();
			const clusters = clusterPoints(
				bySize,
				(d) => {
					const [x, y] = projected.get(d)!;
					return [x * k, y * k];
				},
				CLUSTER_RADIUS
			)
				.map((c) => ({ ...c, x: c.x / k, y: c.y / k }))
				// Tab through the markers west to east
				.sort((a, b) => a.x - b.x || a.y - b.y);

			markerLayer.selectAll('.marker').remove();
			const markers = markerLayer
				.selectAll<SVGGElement, MapCluster<MapLocation>>('.marker')
				.data(clusters)
				.enter()
				.append('g')
				.attr('class', 'marker')
				.attr('transform', (c) => `translate(${c.x},${c.y})`)
				.attr('tabindex', 0)
				.attr('role', 'button')
				.attr('aria-label', (c) => clusterLabel(c.items))
				.on('mouseenter', function (event: MouseEvent, c) {
					showTooltip(clusterLabel(c.items));
					positionTooltip(d3.pointer(event, container));
				})
				.on('mousemove', (event: MouseEvent) => positionTooltip(d3.pointer(event, container)))
				.on('mouseleave', hideTooltip)
				.on('focus', function (_event: FocusEvent, c) {
					showTooltip(clusterLabel(c.items));
					const box = this.getBoundingClientRect();
					const origin = container.getBoundingClientRect();
					positionTooltip([box.right - origin.left, box.top - origin.top + box.height / 2]);
				})
				.on('blur', hideTooltip)
				.on('click', (_event: MouseEvent, c) => select(c.items))
				.on('keydown', (event: KeyboardEvent, c) => {
					if (event.key === 'Enter' || event.key === ' ') {
						event.preventDefault();
						select(c.items);
					}
				});

			markers
				.append('circle')
				.attr('class', 'hit')
				.attr('r', (c) => Math.max(clusterRadius(c), MIN_HIT_RADIUS) / k)
				.attr('fill', 'transparent');
			markers
				.append('circle')
				.attr('class', 'dot')
				.attr('r', (c) => clusterRadius(c) / k)
				.attr('fill', (c) => MARKER_FILL[clusterKind(c.items)])
				.attr('fill-opacity', 0.75)
				.attr('stroke', '#fff')
				.attr('stroke-width', 1);
		}

		// Zoom behavior; markers and borders are counter-scaled to keep a constant on-screen size
		zoom = d3
			.zoom<SVGSVGElement, unknown>()
			.scaleExtent([1, 12])
			.on('start', hideTooltip)
			.on('zoom', (event) => {
				const k = event.transform.k;
				g.attr('transform', event.transform);
				markerLayer
					.selectAll<SVGCircleElement, MapCluster<MapLocation>>('.dot')
					.attr('r', (c) => clusterRadius(c) / k);
				markerLayer
					.selectAll<SVGCircleElement, MapCluster<MapLocation>>('.hit')
					.attr('r', (c) => Math.max(clusterRadius(c), MIN_HIT_RADIUS) / k);
				baseLayer.selectAll('.country').attr('stroke-width', 0.5 / k);
			})
			.on('end', (event) => {
				if (event.transform.k !== clusteredAt) drawMarkers(event.transform.k);
			});

		svg.call(zoom);

		// A tap or click away from a marker dismisses the tooltip (touch has no mouseleave)
		svg.on('click.tooltip', (event: MouseEvent) => {
			if (!(event.target as Element).closest('.marker')) hideTooltip();
		});

		drawMarkers(1);

		// The world outline is drawn underneath when it arrives; markers don't wait for it
		worldOutline()
			.then((world: Topology) => {
				const countries = topojson.feature(
					world,
					world.objects.countries as TopoGeometryCollection
				);
				const features = 'features' in countries ? countries.features : [countries];
				baseLayer
					.selectAll('.country')
					.data(features)
					.enter()
					.append('path')
					.attr('class', 'country')
					.attr('d', path as unknown as string)
					.attr('fill', '#e2e8f0')
					.attr('stroke', '#cbd5e1')
					.attr('stroke-width', 0.5 / d3.zoomTransform(svgNode).k);
				baseMapFailed = false;
			})
			.catch(() => {
				// Try again on the next redraw
				worldAtlas = null;
				baseMapFailed = true;
			});
	}
</script>

<div class="geographic-map">
	{#if loading}
		<div class="loading">Loading map data...</div>
	{:else if error}
		<ErrorState message={error} onRetry={retry} />
	{:else if locations.length === 0}
		<div class="empty">
			<p>No geographic data available.</p>
			<p class="hint">Import GEDCOM data with coordinates to see locations on the map.</p>
		</div>
	{:else}
		<div class="map-container" bind:this={container}>
			<div class="tooltip" bind:this={tooltipEl}></div>
		</div>

		{#if baseMapFailed}
			<p class="notice" role="status">
				The world outline could not be loaded, so locations are shown without it.
			</p>
		{/if}

		<div class="legend">
			<span class="legend-item">
				<span class="dot birth"></span> Birth
			</span>
			<span class="legend-item">
				<span class="dot death"></span> Death
			</span>
			<span class="legend-item">
				<span class="dot mixed"></span> Both
			</span>
			<span class="legend-hint">
				{coarsePointer ? 'Pinch to zoom, drag to pan' : 'Scroll to zoom, drag to pan'}
			</span>
			<button type="button" class="reset-btn" onclick={resetView}>Reset view</button>
		</div>

		{#if selected}
			<div class="detail-panel" bind:this={panelEl}>
				<div class="panel-header">
					<h2>{selected.length === 1 ? selected[0].place : `${selected.length} locations`}</h2>
					{#if selected.length === 1}
						{@render badge(selected[0])}
					{/if}
					<button
						type="button"
						class="close-btn"
						aria-label="Close details"
						onclick={() => (selected = null)}>&times;</button
					>
				</div>
				{#each selected as location (locationKey(location))}
					<section class="panel-location">
						{#if selected.length > 1}
							<h3>{location.place} {@render badge(location)}</h3>
						{/if}
						<p class="panel-count">
							{location.count}
							{location.count === 1 ? 'person' : 'persons'}
						</p>
						<ul class="person-list">
							{#each location.persons as person (person.id)}
								<li><a href="/persons/{person.id}">{formatPersonName(person)}</a></li>
							{/each}
						</ul>
					</section>
				{/each}
			</div>
		{/if}

		<details class="location-list" open={baseMapFailed}>
			<summary>All locations ({locations.length})</summary>
			<ul>
				{#each locations as location (locationKey(location))}
					<li>
						<button type="button" onclick={() => select([location])}>
							<span class="dot {location.event_type}"></span>
							<span class="location-place">{location.place}</span>
							<span class="location-meta">{location.event_type} · {location.count}</span>
						</button>
					</li>
				{/each}
			</ul>
		</details>
	{/if}
</div>

{#snippet badge(location: MapLocation)}
	<span
		class="event-badge"
		class:birth={location.event_type === 'birth'}
		class:death={location.event_type === 'death'}
	>
		{location.event_type}
	</span>
{/snippet}

<style>
	.geographic-map {
		position: relative;
	}

	.map-container {
		position: relative;
		width: 100%;
		border: 1px solid #e2e8f0;
		border-radius: 0.5rem;
		overflow: hidden;
		background: #f8fafc;
	}

	.map-container :global(svg) {
		display: block;
	}

	.map-container :global(.marker) {
		cursor: pointer;
		outline: none;
	}

	.map-container :global(.dot) {
		vector-effect: non-scaling-stroke;
	}

	.map-container :global(.marker:focus-visible .dot) {
		stroke: #1e293b;
		stroke-width: 3;
		fill-opacity: 1;
	}

	.notice {
		margin: 0.5rem 0 0;
		font-size: 0.8125rem;
		color: #92400e;
	}

	.tooltip {
		display: none;
		position: absolute;
		background: rgba(15, 23, 42, 0.9);
		color: #fff;
		padding: 0.5rem 0.75rem;
		border-radius: 0.375rem;
		font-size: 0.8125rem;
		pointer-events: none;
		z-index: 10;
		max-width: calc(100% - 1rem);
	}

	.legend {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 0.5rem 1.25rem;
		margin-top: 0.75rem;
		font-size: 0.8125rem;
		color: var(--color-text-muted);
	}

	.legend-item {
		display: flex;
		align-items: center;
		gap: 0.375rem;
	}

	.dot {
		width: 10px;
		height: 10px;
		border-radius: 50%;
		display: inline-block;
	}

	.dot.birth {
		background: #3b82f6;
	}

	.dot.death {
		background: #6b7280;
	}

	.dot.mixed {
		background: #8b5cf6;
	}

	.legend-hint {
		margin-left: auto;
		font-style: italic;
		font-size: 0.75rem;
	}

	.reset-btn {
		padding: 0.25rem 0.625rem;
		border: 1px solid #cbd5e1;
		border-radius: 6px;
		background: white;
		color: #475569;
		font-size: 0.75rem;
		white-space: nowrap;
		cursor: pointer;
	}

	.reset-btn:hover {
		background: #f1f5f9;
	}

	.detail-panel {
		margin-top: 1rem;
		padding: 1rem;
		border: 1px solid #e2e8f0;
		border-radius: 0.5rem;
		background: #fff;
	}

	.panel-header {
		display: flex;
		align-items: center;
		gap: 0.75rem;
	}

	.panel-header h2 {
		margin: 0;
		font-size: 1rem;
		font-weight: 600;
		color: #1e293b;
	}

	.event-badge {
		font-size: 0.6875rem;
		font-weight: 500;
		padding: 0.125rem 0.5rem;
		border-radius: 9999px;
		text-transform: uppercase;
		letter-spacing: 0.025em;
	}

	.event-badge.birth {
		background: #dbeafe;
		color: #1d4ed8;
	}

	.event-badge.death {
		background: #f1f5f9;
		color: #475569;
	}

	.close-btn {
		margin-left: auto;
		background: none;
		border: none;
		font-size: 1.25rem;
		cursor: pointer;
		color: var(--color-text-muted);
		padding: 0 0.25rem;
	}

	.close-btn:hover {
		color: #475569;
	}

	.panel-count {
		margin: 0.5rem 0;
		font-size: 0.875rem;
		color: var(--color-text-muted);
	}

	.person-list {
		list-style: none;
		padding: 0;
		margin: 0;
		max-height: 200px;
		overflow-y: auto;
	}

	.person-list li {
		padding: 0.375rem 0;
		border-bottom: 1px solid #f1f5f9;
	}

	.person-list li:last-child {
		border-bottom: none;
	}

	.person-list a {
		color: var(--color-link);
		text-decoration: none;
		font-size: 0.875rem;
	}

	.person-list a:hover {
		text-decoration: underline;
	}

	.panel-location + .panel-location {
		margin-top: 0.75rem;
		padding-top: 0.75rem;
		border-top: 1px solid #e2e8f0;
	}

	.panel-location h3 {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		margin: 0;
		font-size: 0.875rem;
		font-weight: 600;
		color: #1e293b;
	}

	.location-list {
		margin-top: 1rem;
		font-size: 0.875rem;
	}

	.location-list summary {
		cursor: pointer;
		color: #475569;
		font-weight: 500;
	}

	.location-list ul {
		list-style: none;
		padding: 0;
		margin: 0.5rem 0 0;
		max-height: 320px;
		overflow-y: auto;
	}

	.location-list button {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		width: 100%;
		min-height: 2.25rem;
		padding: 0.25rem 0.5rem;
		border: none;
		border-radius: 0.375rem;
		background: none;
		color: #1e293b;
		text-align: left;
		cursor: pointer;
	}

	.location-list button:hover {
		background: #f1f5f9;
	}

	.location-place {
		flex: 1;
		min-width: 0;
		overflow-wrap: anywhere;
	}

	.location-meta {
		color: var(--color-text-muted);
		font-size: 0.75rem;
		white-space: nowrap;
	}

	.loading,
	.empty {
		text-align: center;
		padding: 3rem 1rem;
		color: var(--color-text-muted);
	}

	.empty .hint {
		font-size: 0.875rem;
		margin-top: 0.5rem;
	}
</style>
