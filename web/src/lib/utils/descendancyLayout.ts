/**
 * Geometry for the descendancy chart: where each person card and spouse card
 * goes. A person and their spouses sit side by side in one row, and the tree
 * reserves the whole row's width, so no two cards overlap however many spouses
 * a person has.
 */
import * as d3 from 'd3';
import type { DescendancyNode, SpouseInfo } from '$lib/api/client';

export interface DescendancyLayoutConfig {
	cardWidth: number;
	cardHeight: number;
	horizontalGap: number; // Gap between neighbouring rows of cards
	verticalGap: number;
	spouseGap: number; // Gap between a person and each spouse card
	spouseCardScale: number; // Spouse cards are drawn slightly smaller
}

/** A card's centre and size, in chart units. */
export interface CardBox {
	x: number;
	y: number;
	width: number;
	height: number;
}

export interface SpouseCard extends CardBox {
	node: d3.HierarchyPointNode<DescendancyNode>;
	spouse: SpouseInfo;
	index: number;
}

export interface DescendancyLayout {
	/** Tree nodes; `x`/`y` is the centre of the person's own card. */
	nodes: d3.HierarchyPointNode<DescendancyNode>[];
	links: d3.HierarchyPointLink<DescendancyNode>[];
	spouses: SpouseCard[];
	/** Every card, person and spouse alike. */
	cards: CardBox[];
	bounds: { x: number; y: number; width: number; height: number };
}

/** Width of a person's card plus the spouse cards beside it. */
function rowWidth(spouseCount: number, config: DescendancyLayoutConfig): number {
	return config.cardWidth + spouseCount * (config.spouseGap + config.cardWidth * config.spouseCardScale);
}

function spouseCount(node: d3.HierarchyNode<DescendancyNode>): number {
	return node.data.spouses?.length ?? 0;
}

export function layoutDescendancy(
	data: DescendancyNode,
	config: DescendancyLayoutConfig
): DescendancyLayout {
	const { cardWidth, cardHeight, horizontalGap, spouseGap } = config;
	const spouseWidth = cardWidth * config.spouseCardScale;
	const spouseHeight = cardHeight * config.spouseCardScale;

	// One horizontal unit per pixel, so separation can be measured in pixels:
	// half of each row plus a gap, wider between cousins than between siblings
	const root = d3.hierarchy(data, (d) => d.children);
	const tree = d3
		.tree<DescendancyNode>()
		.nodeSize([1, cardHeight + config.verticalGap])
		.separation(
			(a, b) =>
				(rowWidth(spouseCount(a), config) + rowWidth(spouseCount(b), config)) / 2 +
				(a.parent === b.parent ? horizontalGap : 2 * horizontalGap)
		)(root);

	const nodes = tree.descendants();
	// The tree places the centre of each row; move x to the person's own card
	for (const node of nodes) {
		node.x += (cardWidth - rowWidth(spouseCount(node), config)) / 2;
	}

	const spouses: SpouseCard[] = nodes.flatMap((node) =>
		(node.data.spouses ?? []).map((spouse, index) => ({
			node,
			spouse,
			index,
			x: node.x + cardWidth / 2 + spouseGap + spouseWidth / 2 + index * (spouseWidth + spouseGap),
			y: node.y,
			width: spouseWidth,
			height: spouseHeight
		}))
	);

	const cards: CardBox[] = [
		...nodes.map((node) => ({ x: node.x, y: node.y, width: cardWidth, height: cardHeight })),
		...spouses
	];
	const left = d3.min(cards, (c) => c.x - c.width / 2) ?? 0;
	const right = d3.max(cards, (c) => c.x + c.width / 2) ?? 0;
	const top = d3.min(cards, (c) => c.y - c.height / 2) ?? 0;
	const bottom = d3.max(cards, (c) => c.y + c.height / 2) ?? 0;

	return {
		nodes,
		links: tree.links(),
		spouses,
		cards,
		bounds: { x: left, y: top, width: right - left, height: bottom - top }
	};
}
