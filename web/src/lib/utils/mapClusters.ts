import * as d3 from 'd3';

/**
 * Screen-space clustering and labels for the geographic map's markers.
 */

export interface MapCluster<T> {
	/** Centre of the cluster: the mean of its members' positions. */
	x: number;
	y: number;
	items: T[];
}

/**
 * Merges markers until no two drawn circles overlap: two clusters join whenever
 * their centres are no farther apart than the sum of their drawn radii, and the
 * merged centre is the mean of all its members' positions. A merge can move a
 * centre and grow its radius into a neighbour, so merging repeats until stable.
 * Co-located items always share a cluster. Pass items largest first so the big
 * markers anchor the clusters (and come first in each cluster's items).
 */
export function clusterPoints<T>(
	items: T[],
	position: (item: T) => [number, number],
	radius: (items: T[]) => number
): MapCluster<T>[] {
	const clusters = items.map((item) => {
		const [x, y] = position(item);
		return { x, y, items: [item], sumX: x, sumY: y, r: radius([item]) };
	});
	const overlap = (a: (typeof clusters)[number], b: (typeof clusters)[number]) =>
		Math.hypot(a.x - b.x, a.y - b.y) <= a.r + b.r;

	let merged = true;
	while (merged) {
		merged = false;
		for (let i = 0; i < clusters.length; i++) {
			const a = clusters[i];
			for (let j = i + 1; j < clusters.length; j++) {
				const b = clusters[j];
				if (!overlap(a, b)) continue;
				a.items.push(...b.items);
				a.sumX += b.sumX;
				a.sumY += b.sumY;
				a.x = a.sumX / a.items.length;
				a.y = a.sumY / a.items.length;
				a.r = radius(a.items);
				clusters.splice(j, 1);
				merged = true;
				// a moved and grew: check it against every later cluster again
				j = i;
			}
		}
	}
	return clusters.map(({ x, y, items }) => ({ x, y, items }));
}

/**
 * The drawn radius of a marker in screen pixels, by the people it holds: area
 * grows with the count, from 7px for one person up to 20px at `maxCount`.
 */
export function markerRadius(maxCount: number): (items: { count: number }[]) => number {
	const scale = d3
		.scaleSqrt()
		.domain([1, maxCount || 1])
		.range([7, 20])
		.clamp(true);
	return (items) => scale(d3.sum(items, (d) => d.count));
}

interface LabelledLocation {
	place: string;
	event_type: 'birth' | 'death';
	count: number;
}

function plural(n: number, word: string): string {
	return `${n} ${word}${n === 1 ? '' : 's'}`;
}

/**
 * The accessible name of a marker: its places, then its birth and death counts,
 * e.g. "Boston, MA; New York, NY: 2 births, 1 death".
 */
export function clusterLabel(locations: LabelledLocation[], maxPlaces = 3): string {
	const places = [...new Set(locations.map((l) => l.place))];
	const shown = places.slice(0, maxPlaces).join('; ');
	const more =
		places.length > maxPlaces ? ` and ${plural(places.length - maxPlaces, 'more place')}` : '';
	const births = locations.filter((l) => l.event_type === 'birth').reduce((n, l) => n + l.count, 0);
	const deaths = locations.filter((l) => l.event_type === 'death').reduce((n, l) => n + l.count, 0);
	const counts = [births && plural(births, 'birth'), deaths && plural(deaths, 'death')]
		.filter(Boolean)
		.join(', ');
	return `${shown}${more}: ${counts}`;
}

/** The marker colour key: all births, all deaths, or a mix of both. */
export function clusterKind(locations: LabelledLocation[]): 'birth' | 'death' | 'mixed' {
	const kinds = new Set(locations.map((l) => l.event_type));
	return kinds.size > 1 ? 'mixed' : (locations[0]?.event_type ?? 'birth');
}
