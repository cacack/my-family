import { describe, it, expect } from 'vitest';
import type { DescendancyNode } from '$lib/api/client';
import { layoutDescendancy, type CardBox, type DescendancyLayoutConfig } from './descendancyLayout';

const CONFIGS: Record<string, DescendancyLayoutConfig> = {
	compact: { cardWidth: 120, cardHeight: 65, horizontalGap: 8, verticalGap: 25, spouseGap: 8, spouseCardScale: 0.9 },
	wide: { cardWidth: 160, cardHeight: 80, horizontalGap: 40, verticalGap: 50, spouseGap: 16, spouseCardScale: 0.9 }
};

let nextId = 0;
function person(spouseCount: number, children: DescendancyNode[] = []): DescendancyNode {
	const id = String(++nextId);
	return {
		id,
		given_name: `P${id}`,
		spouses: Array.from({ length: spouseCount }, (_, i) => ({ id: `${id}-s${i}`, name: `Spouse ${i}` })),
		children
	};
}

function overlaps(a: CardBox, b: CardBox): boolean {
	return (
		Math.abs(a.x - b.x) < (a.width + b.width) / 2 && Math.abs(a.y - b.y) < (a.height + b.height) / 2
	);
}

function overlappingPairs(cards: CardBox[]): [CardBox, CardBox][] {
	const pairs: [CardBox, CardBox][] = [];
	for (let i = 0; i < cards.length; i++) {
		for (let j = i + 1; j < cards.length; j++) {
			if (overlaps(cards[i], cards[j])) pairs.push([cards[i], cards[j]]);
		}
	}
	return pairs;
}

describe('layoutDescendancy', () => {
	for (const [name, config] of Object.entries(CONFIGS)) {
		for (let spouses = 1; spouses <= 4; spouses++) {
			it(`never overlaps cards when people have ${spouses} spouse(s) (${name})`, () => {
				// Ten people over three generations; every sibling and cousin has the same spouse count
				const tree = person(spouses, [
					person(spouses, [person(spouses), person(spouses), person(spouses)]),
					person(spouses, [person(spouses)]),
					person(spouses, [person(spouses), person(spouses)])
				]);
				const layout = layoutDescendancy(tree, config);
				expect(layout.cards).toHaveLength(10 * (1 + spouses));
				expect(overlappingPairs(layout.cards)).toEqual([]);
			});
		}

		it(`never overlaps cards with mixed spouse counts (${name})`, () => {
			const tree = person(2, [
				person(4, [person(0), person(3)]),
				person(0, [person(1), person(4), person(0)]),
				person(1, [person(2)]),
				person(3)
			]);
			expect(overlappingPairs(layoutDescendancy(tree, config).cards)).toEqual([]);
		});
	}

	it('puts spouses in the person\'s row, to the right and in order', () => {
		const config = CONFIGS.compact;
		const layout = layoutDescendancy(person(3), config);
		const [root] = layout.nodes;
		expect(layout.spouses.map((s) => s.y)).toEqual([root.y, root.y, root.y]);
		const xs = layout.spouses.map((s) => s.x);
		expect(xs[0]).toBeGreaterThan(root.x);
		expect(xs[1]).toBeGreaterThan(xs[0]);
		expect(xs[2]).toBeGreaterThan(xs[1]);
	});

	it('starts each spouse connector at the right edge of the card before it', () => {
		const config = CONFIGS.wide;
		const layout = layoutDescendancy(person(3), config);
		const [root] = layout.nodes;
		const rightEdges = [root.x + config.cardWidth / 2, ...layout.spouses.map((s) => s.x + s.width / 2)];
		layout.spouses.forEach((s, i) => {
			expect(s.linkFromX).toBeCloseTo(rightEdges[i]);
			expect(s.x - s.width / 2 - s.linkFromX).toBeCloseTo(config.spouseGap);
		});
	});

	it('bounds every card', () => {
		const layout = layoutDescendancy(person(2, [person(0), person(1)]), CONFIGS.wide);
		const { x, y, width, height } = layout.bounds;
		for (const c of layout.cards) {
			expect(c.x - c.width / 2).toBeGreaterThanOrEqual(x);
			expect(c.x + c.width / 2).toBeLessThanOrEqual(x + width);
			expect(c.y - c.height / 2).toBeGreaterThanOrEqual(y);
			expect(c.y + c.height / 2).toBeLessThanOrEqual(y + height);
		}
	});
});
