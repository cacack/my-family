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
	/** Right edge of the card before this one (the person's, or the previous spouse's), where its connector starts. */
	linkFromX: number;
	/** Midpoint of the connector into this card, where this couple's children hang from. */
	dropX: number;
}

/** A line from a couple (or a lone parent) down to one of their children. */
export interface ChildLink {
	parent: d3.HierarchyPointNode<DescendancyNode>;
	child: d3.HierarchyPointNode<DescendancyNode>;
	/** The parent's spouse in the child's family, when the API names one. */
	spouse?: SpouseInfo;
	/** The midpoint of the couple's connector, or the parent's card centre. */
	source: { x: number; y: number };
	/** The child's card centre. */
	target: { x: number; y: number };
	/**
	 * The line as an elbow: straight down from `source` to a bus in the gap
	 * below the parent's row, along it, then down to `target`. Each family of a
	 * parent gets its own bus height, so the drop never runs under a card.
	 */
	points: { x: number; y: number }[];
}

export interface DescendancyLayout {
	/** Tree nodes; `x`/`y` is the centre of the person's own card. */
	nodes: d3.HierarchyPointNode<DescendancyNode>[];
	links: ChildLink[];
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
		(node.data.spouses ?? []).map((spouse, index) => {
			const linkFromX = node.x + cardWidth / 2 + index * (spouseGap + spouseWidth);
			return {
				node,
				spouse,
				index,
				linkFromX,
				dropX: linkFromX + spouseGap / 2,
				x: linkFromX + spouseGap + spouseWidth / 2,
				y: node.y,
				width: spouseWidth,
				height: spouseHeight
			};
		})
	);

	// Each child hangs from the connector between its parent and the spouse of
	// its family, so the child's other parent is the card the line leaves from
	// (#1099); a child of a family with no named spouse hangs from the parent.
	const spouseCardOf = new Map<string, SpouseCard>();
	for (const card of spouses) spouseCardOf.set(`${card.node.data.id}/${card.spouse.family_id}`, card);
	const treeLinks = tree.links().map(({ source: parent, target: child }) => ({
		parent,
		child,
		card: child.data.family_id ? spouseCardOf.get(`${parent.data.id}/${child.data.family_id}`) : undefined
	}));

	// One bus height per family of each parent, in spouse order, lone-parent last
	const laneKey = (card?: SpouseCard) => card?.index ?? Number.POSITIVE_INFINITY;
	const lanesOf = new Map<d3.HierarchyPointNode<DescendancyNode>, number[]>();
	for (const { parent, card } of treeLinks) {
		const lanes = lanesOf.get(parent) ?? [];
		if (!lanes.includes(laneKey(card))) lanes.push(laneKey(card));
		lanesOf.set(parent, lanes);
	}
	for (const lanes of lanesOf.values()) lanes.sort((a, b) => a - b);

	const links: ChildLink[] = treeLinks.map(({ parent, child, card }) => {
		const lanes = lanesOf.get(parent)!;
		const lane = lanes.indexOf(laneKey(card));
		const busY = parent.y + cardHeight / 2 + (config.verticalGap * (lane + 1)) / (lanes.length + 1);
		const source = { x: card ? card.dropX : parent.x, y: parent.y };
		const target = { x: child.x, y: child.y };
		const points = [source, { x: source.x, y: busY }, { x: target.x, y: busY }, target];
		return { parent, child, spouse: card?.spouse, source, target, points };
	});

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
		links,
		spouses,
		cards,
		bounds: { x: left, y: top, width: right - left, height: bottom - top }
	};
}
