import { describe, it, expect } from 'vitest';
import { clusterKind, clusterLabel, clusterPoints } from './mapClusters';

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
		const clusters = clusterPoints(boston, at, 0);
		expect(clusters).toHaveLength(1);
		expect(clusters[0]).toMatchObject({ x: 10, y: 10 });
		expect(clusters[0].items).toEqual(boston);
	});

	it('merges points within the radius and centres the cluster on their mean', () => {
		const clusters = clusterPoints(
			[loc('A', 'birth', 1, 0, 0), loc('B', 'birth', 1, 10, 0)],
			at,
			12
		);
		expect(clusters).toHaveLength(1);
		expect(clusters[0]).toMatchObject({ x: 5, y: 0 });
	});

	it('keeps points farther apart than the radius separate', () => {
		const clusters = clusterPoints(
			[loc('A', 'birth', 1, 0, 0), loc('B', 'birth', 1, 30, 0)],
			at,
			12
		);
		expect(clusters.map((c) => c.items.map((i) => i.place))).toEqual([['A'], ['B']]);
	});

	it('returns no clusters for no points', () => {
		expect(clusterPoints([], at, 10)).toEqual([]);
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
