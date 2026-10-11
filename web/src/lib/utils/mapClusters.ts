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
 * Greedy clustering: each item joins the first cluster whose centre lies within
 * `radius` of it, else starts a new one. Co-located items always share a cluster.
 * Pass items largest first so the big markers anchor the clusters.
 */
export function clusterPoints<T>(
	items: T[],
	position: (item: T) => [number, number],
	radius: number
): MapCluster<T>[] {
	const clusters: MapCluster<T>[] = [];
	for (const item of items) {
		const [x, y] = position(item);
		const near = clusters.find((c) => Math.hypot(c.x - x, c.y - y) <= radius);
		if (near) {
			const n = near.items.length;
			near.x = (near.x * n + x) / (n + 1);
			near.y = (near.y * n + y) / (n + 1);
			near.items.push(item);
		} else {
			clusters.push({ x, y, items: [item] });
		}
	}
	return clusters;
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
