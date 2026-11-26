<script lang="ts">
	/**
	 * A small, still map of geocoder candidates (#1607), each marked with its
	 * number in the list beside it, so "Boston, England" is told from "Boston,
	 * Massachusetts" at a glance. The list is the accessible form; the map is
	 * a picture of it. Drawn in a fixed view box that scales with its width,
	 * with the world outline the family map uses (`GeographicMap`).
	 */
	import * as d3 from 'd3';
	import * as topojson from 'topojson-client';
	import type { GeometryCollection as TopoGeometryCollection } from 'topojson-specification';
	import type { GeocodeCandidate } from '#lib/api/client.js';
	import { fitMapProjection } from '#lib/utils/mapProjection.js';
	import { loadWorldAtlas } from '#lib/utils/worldAtlas.js';

	let { candidates }: { candidates: GeocodeCandidate[] } = $props();

	const WIDTH = 320;
	const HEIGHT = 160;
	const MARGIN = 14;

	const projection = $derived(
		fitMapProjection(
			WIDTH,
			HEIGHT,
			candidates.map((c) => [c.longitude, c.latitude]),
			MARGIN
		)
	);
	const markers = $derived(
		candidates.map((c, i) => {
			const [x, y] = projection([c.longitude, c.latitude]) ?? [0, 0];
			return { n: i + 1, x, y };
		})
	);

	// The outline arrives after the markers; without it they are drawn alone.
	let countries: string[] = $state([]);
	$effect(() => {
		const path = d3.geoPath().projection(projection);
		let current = true;
		loadWorldAtlas()
			.then((world) => {
				const shapes = topojson.feature(world, world.objects.countries as TopoGeometryCollection);
				const features = 'features' in shapes ? shapes.features : [shapes];
				if (current) countries = features.map((f) => path(f) ?? '');
			})
			.catch(() => {
				if (current) countries = [];
			});
		return () => {
			current = false;
		};
	});
</script>

<svg
	class="candidate-map"
	viewBox="0 0 {WIDTH} {HEIGHT}"
	role="img"
	aria-label="Map of the {candidates.length === 1 ? 'candidate' : `${candidates.length} candidates`}"
	data-testid="geocode-candidate-map"
>
	<g aria-hidden="true">
		{#each countries as d, i (i)}
			<path class="country" {d} />
		{/each}
		{#each markers as m (m.n)}
			<g class="marker" transform="translate({m.x},{m.y})">
				<circle r="7" />
				<text dy="0.35em">{m.n}</text>
			</g>
		{/each}
	</g>
</svg>

<style>
	.candidate-map {
		display: block;
		width: 100%;
		max-width: 24rem;
		height: auto;
		background: #f8fafc;
		border: 1px solid var(--a11y-color-border);
		border-radius: 6px;
	}

	.country {
		fill: #e2e8f0;
		stroke: #cbd5e1;
		stroke-width: 0.5;
	}

	.marker circle {
		fill: #3b82f6;
		stroke: #fff;
		stroke-width: 1;
	}

	.marker text {
		fill: #fff;
		font-size: 9px;
		font-weight: 600;
		text-anchor: middle;
	}
</style>
