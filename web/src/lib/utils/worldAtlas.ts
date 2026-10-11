import type { Topology } from 'topojson-specification';

/**
 * The world outline for the geographic map. It is bundled with the app, not
 * fetched from a CDN, so the map works offline and on a closed network (ADR-004);
 * the import keeps it in its own chunk, loaded only when a map is drawn.
 */
export async function loadWorldAtlas(): Promise<Topology> {
	const atlas = await import('world-atlas/countries-110m.json');
	return atlas.default as unknown as Topology;
}
