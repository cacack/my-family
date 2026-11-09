import { describe, it, expect } from 'vitest';
import * as d3 from 'd3';
import { clusterKind, clusterLabel, clusterPoints, markerRadius } from './mapClusters';

type Loc = {
	place: string;
	event_type: 'birth' | 'death';
	count: number;
	x: number;
	y: number;
};

const loc = (
	place: string,
	event_type: 'birth' | 'death',
	count: number,
	x: number,
	y: number
): Loc => ({
	place,
	event_type,
	count,
	x,
	y
});
const at = (l: Loc): [number, number] => [l.x, l.y];

describe('clusterPoints', () => {
	it('puts co-located points in one cluster', () => {
		const boston = [loc('Boston', 'birth', 2, 10, 10), loc('Boston', 'death', 1, 10, 10)];
		const clusters = clusterPoints(boston, at, () => 0);
		expect(clusters).toHaveLength(1);
		expect(clusters[0]).toMatchObject({ x: 10, y: 10 });
		expect(clusters[0].items).toEqual(boston);
	});

	it('merges points within the radius and centres the cluster on their mean', () => {
		const clusters = clusterPoints(
			[loc('A', 'birth', 1, 0, 0), loc('B', 'birth', 1, 10, 0)],
			at,
			() => 6
		);
		expect(clusters).toHaveLength(1);
		expect(clusters[0]).toMatchObject({ x: 5, y: 0 });
	});

	it('keeps points farther apart than the radius separate', () => {
		const clusters = clusterPoints(
			[loc('A', 'birth', 1, 0, 0), loc('B', 'birth', 1, 30, 0)],
			at,
			() => 6
		);
		expect(clusters.map((c) => c.items.map((i) => i.place))).toEqual([['A'], ['B']]);
	});

	it('returns no clusters for no points', () => {
		expect(clusterPoints([], at, () => 5)).toEqual([]);
	});

	it('merges clusters that a moved centre brings into overlap', () => {
		// C joins A, moving its centre to x=6, which then overlaps B at x=13
		const clusters = clusterPoints(
			[loc('A', 'birth', 1, 0, 0), loc('B', 'birth', 1, 13, 0), loc('C', 'birth', 1, 12, 0)],
			at,
			() => 6
		);
		expect(clusters.map((c) => c.items.map((i) => i.place))).toEqual([['A', 'C', 'B']]);
	});

	it('leaves no two drawn markers overlapping for nearby European cities', () => {
		const cities: [string, number, number, number][] = [
			['London', -0.13, 51.51, 40],
			['Edinburgh', -3.19, 55.95, 12],
			['Dublin', -6.26, 53.35, 9],
			['Paris', 2.35, 48.86, 30],
			['Lyon', 4.84, 45.76, 6],
			['Brussels', 4.35, 50.85, 4],
			['Amsterdam', 4.9, 52.37, 8],
			['Berlin', 13.4, 52.52, 25],
			['Hamburg', 9.99, 53.55, 7],
			['Munich', 11.58, 48.14, 5],
			['Vienna', 16.37, 48.21, 3],
			['Rome', 12.5, 41.9, 18],
			['Madrid', -3.7, 40.42, 14],
			['Barcelona', 2.17, 41.39, 2]
		];
		const items = cities.map(([place, lon, lat, count]) => ({
			...loc(place, 'birth', count, lon, lat),
			longitude: lon,
			latitude: lat
		}));
		const radius = markerRadius(d3.max(items, (d) => d.count)!);
		for (const width of [375, 768, 1280]) {
			const projection = d3
				.geoNaturalEarth1()
				.scale(width / 5.5)
				.translate([width / 2, 200]);
			const clusters = clusterPoints(
				[...items].sort((a, b) => b.count - a.count),
				(d) => projection([d.longitude, d.latitude])!,
				radius
			);
			if (width === 1280) expect(clusters.length).toBeGreaterThan(1);
			for (const [i, a] of clusters.entries()) {
				for (const b of clusters.slice(i + 1)) {
					const distance = Math.hypot(a.x - b.x, a.y - b.y);
					expect(distance).toBeGreaterThanOrEqual(radius(a.items) + radius(b.items));
				}
			}
		}
	});
});

describe('markerRadius', () => {
	it('grows with the people at a marker, from 7px up to a 20px cap', () => {
		const radius = markerRadius(100);
		expect(radius([{ count: 1 }])).toBe(7);
		expect(radius([{ count: 60 }, { count: 40 }])).toBe(20);
		expect(radius([{ count: 500 }])).toBe(20);
		expect(radius([{ count: 25 }])).toBeGreaterThan(7);
	});
});

describe('clusterLabel', () => {
	it('names a single place and its counts', () => {
		expect(clusterLabel([loc('Boston, MA', 'birth', 1, 0, 0)])).toBe('Boston, MA: 1 birth');
	});

	it('lists each place once and totals births and deaths', () => {
		const label = clusterLabel([
			loc('Boston, MA', 'birth', 2, 0, 0),
			loc('Boston, MA', 'death', 1, 0, 0),
			loc('New York, NY', 'death', 2, 0, 0)
		]);
		expect(label).toBe('Boston, MA; New York, NY: 2 births, 3 deaths');
	});

	it('summarises places beyond the limit', () => {
		const label = clusterLabel(
			['A', 'B', 'C', 'D', 'E'].map((p) => loc(p, 'birth', 1, 0, 0)),
			3
		);
		expect(label).toBe('A; B; C and 2 more places: 5 births');
	});
});

describe('clusterKind', () => {
	it('is the event type when all agree, else mixed', () => {
		expect(clusterKind([loc('A', 'death', 1, 0, 0)])).toBe('death');
		expect(clusterKind([loc('A', 'birth', 1, 0, 0), loc('A', 'birth', 2, 0, 0)])).toBe('birth');
		expect(clusterKind([loc('A', 'birth', 1, 0, 0), loc('A', 'death', 1, 0, 0)])).toBe('mixed');
	});
});
