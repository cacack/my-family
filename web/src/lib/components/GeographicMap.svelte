<script lang="ts">
	import { api, type MapLocation } from '$lib/api/client';
	import * as d3 from 'd3';
	import * as topojson from 'topojson-client';
	import type { Topology, GeometryCollection as TopoGeometryCollection } from 'topojson-specification';

	let container: HTMLDivElement;
	let tooltipEl: HTMLDivElement;
	let locations: MapLocation[] = $state([]);
	let loading = $state(true);
	let error = $state<string | null>(null);
	let selectedLocation: MapLocation | null = $state(null);

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
		}
	});

	function renderMap() {
		// Clear previous
		d3.select(container).select('svg').remove();

		const width = container.clientWidth;
		const height = Math.min(width * 0.55, 600);

		const svg = d3
			.select(container)
			.append('svg')
			.attr('width', width)
			.attr('height', height)
			.attr('viewBox', `0 0 ${width} ${height}`);

		const g = svg.append('g');

		const projection = d3
			.geoNaturalEarth1()
			.scale(width / 5.5)
			.translate([width / 2, height / 2]);

		const path = d3.geoPath().projection(projection);

		// Zoom behavior
		const zoom = d3
			.zoom<SVGSVGElement, unknown>()
			.scaleExtent([1, 12])
			.on('zoom', (event) => {
				g.attr('transform', event.transform);
			});

		svg.call(zoom);

		// Load world data
		fetch('https://cdn.jsdelivr.net/npm/world-atlas@2/countries-110m.json')
			.then((res) => res.json())
			.then((world: Topology) => {
				const countries = topojson.feature(
					world,
					world.objects.countries as TopoGeometryCollection
				);

				// Draw countries
				const features = 'features' in countries ? countries.features : [countries];
				g.selectAll('.country')
					.data(features)
					.enter()
					.append('path')
					.attr('class', 'country')
					.attr('d', path as unknown as string)
					.attr('fill', '#e2e8f0')
					.attr('stroke', '#cbd5e1')
					.attr('stroke-width', 0.5);

				// Draw location circles
				const radiusScale = d3
					.scaleSqrt()
					.domain([1, d3.max(locations, (d) => d.count) || 1])
					.range([4, 18]);

				g.selectAll('.location')
					.data(locations)
					.enter()
					.append('circle')
					.attr('class', 'location')
					.attr('cx', (d) => {
						const coords = projection([d.longitude, d.latitude]);
						return coords ? coords[0] : 0;
					})
					.attr('cy', (d) => {
						const coords = projection([d.longitude, d.latitude]);
						return coords ? coords[1] : 0;
					})
					.attr('r', (d) => radiusScale(d.count))
					.attr('fill', (d) => (d.event_type === 'birth' ? '#3b82f6' : '#6b7280'))
					.attr('fill-opacity', 0.7)
					.attr('stroke', '#fff')
					.attr('stroke-width', 1)
					.attr('cursor', 'pointer')
					.on('mouseenter', (event: MouseEvent, d: MapLocation) => {
						if (!tooltipEl) return;
						tooltipEl.style.display = 'block';
						tooltipEl.style.left = `${event.offsetX + 12}px`;
						tooltipEl.style.top = `${event.offsetY - 10}px`;
						tooltipEl.replaceChildren();
						const title = document.createElement('strong');
						title.textContent = d.place;
						const meta = document.createTextNode(
							`${d.count} ${d.count === 1 ? 'person' : 'persons'} (${d.event_type})`
						);
						tooltipEl.append(title, document.createElement('br'), meta);
					})
					.on('mousemove', (event: MouseEvent) => {
						if (!tooltipEl) return;
						tooltipEl.style.left = `${event.offsetX + 12}px`;
						tooltipEl.style.top = `${event.offsetY - 10}px`;
					})
					.on('mouseleave', () => {
						if (tooltipEl) tooltipEl.style.display = 'none';
					})
					.on('click', (_event: MouseEvent, d: MapLocation) => {
						selectedLocation = selectedLocation?.place === d.place && selectedLocation?.event_type === d.event_type ? null : d;
					});
			});
	}
</script>

<div class="geographic-map">
	{#if loading}
		<div class="loading">Loading map data...</div>
	{:else if error}
		<div class="error">{error}</div>
	{:else if locations.length === 0}
		<div class="empty">
			<p>No geographic data available.</p>
			<p class="hint">Import GEDCOM data with coordinates to see locations on the map.</p>
		</div>
	{:else}
		<div class="map-container" bind:this={container}>
			<div class="tooltip" bind:this={tooltipEl}></div>
		</div>

		<div class="legend">
			<span class="legend-item">
				<span class="dot birth"></span> Birth
			</span>
			<span class="legend-item">
				<span class="dot death"></span> Death
			</span>
			<span class="legend-hint">Scroll to zoom, drag to pan</span>
		</div>

		{#if selectedLocation}
			<div class="detail-panel">
				<div class="panel-header">
					<h3>{selectedLocation.place}</h3>
					<span class="event-badge" class:birth={selectedLocation.event_type === 'birth'} class:death={selectedLocation.event_type === 'death'}>
						{selectedLocation.event_type}
					</span>
					<button class="close-btn" onclick={() => (selectedLocation = null)}>&times;</button>
				</div>
				<p class="panel-count">{selectedLocation.count} {selectedLocation.count === 1 ? 'person' : 'persons'}</p>
				<ul class="person-list">
					{#each selectedLocation.person_ids as id}
						<li><a href="/persons/{id}">View person</a></li>
					{/each}
				</ul>
			</div>
		{/if}
	{/if}
</div>

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
		white-space: nowrap;
	}

	.legend {
		display: flex;
		align-items: center;
		gap: 1.25rem;
		margin-top: 0.75rem;
		font-size: 0.8125rem;
		color: #64748b;
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

	.legend-hint {
		margin-left: auto;
		font-style: italic;
		font-size: 0.75rem;
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

	.panel-header h3 {
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
		color: #94a3b8;
		padding: 0 0.25rem;
	}

	.close-btn:hover {
		color: #475569;
	}

	.panel-count {
		margin: 0.5rem 0;
		font-size: 0.875rem;
		color: #64748b;
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
		color: #3b82f6;
		text-decoration: none;
		font-size: 0.875rem;
	}

	.person-list a:hover {
		text-decoration: underline;
	}

	.loading,
	.error,
	.empty {
		text-align: center;
		padding: 3rem 1rem;
		color: #64748b;
	}

	.error {
		color: #dc2626;
	}

	.empty .hint {
		font-size: 0.875rem;
		margin-top: 0.5rem;
	}
</style>
