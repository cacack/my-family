import { describe, it, expect } from 'vitest';
import { fitMapProjection, mapHeight } from './mapProjection';

const MARGIN = 24;
// Spread right around the globe, with Suva just west of the antimeridian
const WORLD: [number, number][] = [
	[-149.9, 61.22], // Anchorage
	[-71.06, 42.36], // Boston
	[-0.13, 51.51], // London
	[13.4, 52.52], // Berlin
	[12.5, 41.9], // Rome
	[-43.17, -22.91], // Rio de Janeiro
	[28.05, -26.2], // Johannesburg
	[72.88, 19.08], // Mumbai
	[151.21, -33.87], // Sydney
	[178.44, -18.14] // Suva
];
const EUROPE: [number, number][] = [
	[-0.13, 51.51],
	[13.4, 52.52],
	[12.5, 41.9]
];

describe('fitMapProjection', () => {
	for (const width of [375, 768, 1280]) {
		it(`keeps every point inside the margin at ${width}px`, () => {
			const height = mapHeight(width);
			const projection = fitMapProjection(width, height, WORLD, MARGIN);
			for (const point of WORLD) {
				const [x, y] = projection(point)!;
				expect(x).toBeGreaterThanOrEqual(MARGIN - 1e-6);
				expect(x).toBeLessThanOrEqual(width - MARGIN + 1e-6);
				expect(y).toBeGreaterThanOrEqual(MARGIN - 1e-6);
				expect(y).toBeLessThanOrEqual(height - MARGIN + 1e-6);
			}
		});
	}

	it('fills a narrow map and pans to the points without showing past the world edge', () => {
		const [width, height] = [375, mapHeight(375)];
		const projection = fitMapProjection(width, height, EUROPE, MARGIN);
		expect(projection.scale()).toBeCloseTo(height / 2.8);
		// The world is wider than the map, so its edges stay outside
		expect(projection([-180, 0])![0]).toBeLessThanOrEqual(0);
		expect(projection([180, 0])![0]).toBeGreaterThanOrEqual(width);
		for (const point of EUROPE) {
			const [x] = projection(point)!;
			expect(x).toBeGreaterThan(MARGIN);
			expect(x).toBeLessThan(width - MARGIN);
		}
	});

	it('centres a single location at the filling scale', () => {
		const projection = fitMapProjection(1280, mapHeight(1280), [[0, 0]], MARGIN);
		expect(projection.scale()).toBeCloseTo(1280 / 5.5);
		expect(projection([0, 0])![0]).toBeCloseTo(640);
	});

	it('keeps the filling scale for a map too small for its margins', () => {
		const projection = fitMapProjection(0, 320, WORLD, MARGIN);
		expect(projection.scale()).toBeCloseTo(320 / 2.8);
	});
});
