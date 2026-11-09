<script lang="ts">
	import * as d3 from 'd3';
	import { onMount, untrack } from 'svelte';
	import { formatPersonName, formatYear, type PedigreeNode } from '#lib/api/client.js';
	import { describeContextKeys } from '#lib/keyboard/shortcuts.js';
	import { getFontScale } from '#lib/stores/accessibilitySettings.svelte.js';
	import {
		CARD_TEXT_INSET,
		cardsInView,
		centreOnX,
		firstViewDepth,
		fitText,
		fitTransform,
		motionDuration,
		nameLines,
		observeChartResize,
		offscreenX,
		scaleAbout,
		scaleBounds,
		touchScrollsPage,
		type ScaleExtent
	} from '#lib/utils/chart.js';
	import OffscreenIndicator from './OffscreenIndicator.svelte';

	export type LayoutMode = 'compact' | 'standard' | 'wide';

	interface LayoutConfig {
		cardWidth: number;
		cardHeight: number;
		horizontalGap: number;
		verticalGap: number;
		cousinSeparation: number; // Separation multiplier for nodes that don't share a parent
	}

	// cousinSeparation must be >= cardWidth/(cardWidth+horizontalGap) to avoid overlap
	// compact: 120/128 = 0.94, standard: 140/160 = 0.88, wide: 160/200 = 0.80
	// compact goes past 1 so cards of different families keep >= 8px apart (128 * 1.05 - 120 = 14.4)
	// verticalGap must be >= TOGGLE_OFFSET + TOGGLE_HIT_RADIUS so a toggle never covers the card below
	const LAYOUTS: Record<LayoutMode, LayoutConfig> = {
		compact: { cardWidth: 120, cardHeight: 65, horizontalGap: 8, verticalGap: 32, cousinSeparation: 1.05 },
		standard: { cardWidth: 140, cardHeight: 75, horizontalGap: 20, verticalGap: 40, cousinSeparation: 0.9 },
		wide: { cardWidth: 160, cardHeight: 80, horizontalGap: 40, verticalGap: 50, cousinSeparation: 0.85 }
	};

	// Animation duration for collapse/expand transitions
	const ANIMATION_DURATION = 300;

	const SCALE_EXTENT: ScaleExtent = [0.1, 4];
	// Collapse toggles hang below each card: centre offset from the card edge, the
	// drawn radius, and a larger invisible radius that keeps the touch target at
	// least 24px across at the smallest initial zoom (FIT_SCALE_RANGE)
	const TOGGLE_OFFSET = 16;
	const TOGGLE_RADIUS = 10;
	const TOGGLE_HIT_RADIUS = 14;
	const TOGGLE_EXTENT = TOGGLE_OFFSET + TOGGLE_HIT_RADIUS;

	interface Props {
		data: PedigreeNode;
		layout?: LayoutMode;
		selectedPersonId?: string | null;
		onPersonClick?: (personId: string) => void;
		onSelectionChange?: (personId: string | null) => void;
	}

	let { data, layout = 'standard', selectedPersonId = null, onPersonClick, onSelectionChange }: Props = $props();

	let container: HTMLDivElement;
	let svg: d3.Selection<SVGSVGElement, unknown, null, undefined>;
	let g: d3.Selection<SVGGElement, unknown, null, undefined>;
	// Links get a layer of their own below the cards, so a link drawn after a
	// collapse and expand can never cover a card (#1025)
	let linkLayer: d3.Selection<SVGGElement, unknown, null, undefined>;
	let nodeLayer: d3.Selection<SVGGElement, unknown, null, undefined>;
	// Chart units per layout unit: the Accessibility font-size scale (#1103)
	let contentScale = 1;
	let zoom: d3.ZoomBehavior<SVGSVGElement, unknown>;
	// Where the running animated zoom is heading, so quick +/- clicks chain
	let zoomTarget: d3.ZoomTransform | null = null;

	// Map of person ID to their node data for navigation
	let nodeMap: Map<string, d3.HierarchyPointNode<PedigreeNode>> = new Map();
	// Store the tree data for navigation
	let treeNodes: d3.HierarchyPointNode<PedigreeNode>[] = [];

	// Collapsed nodes state - tracks which node IDs have their ancestors collapsed
	// Using a plain Set since reactivity is managed through manual re-renders
	let collapsedNodes: Set<string> = new Set();
	// The collapse of the first view: on a narrow chart, the generations beyond
	// what fits with both parents readable are folded behind their +N badges
	// (#1098). Applied when a root is first drawn and on Reset; the user's own
	// expansions stand until then.
	let firstViewCollapsed: Set<string> = new Set();
	let showFirstView = true;

	// Parents lying off either side of the view (chart-unit x, nearest first)
	let offscreen = $state.raw<{ left: number[]; right: number[] }>({ left: [], right: [] });

	// Debounce timer for rapid collapse/expand operations
	let collapseDebounceTimer: ReturnType<typeof setTimeout> | null = null;

	interface TreeNode {
		data: PedigreeNode;
		x: number;
		y: number;
		parent: TreeNode | null;
		children: TreeNode[];
	}

	/**
	 * Count all ancestors recursively from the original data (before filtering for collapsed state)
	 */
	function countAncestors(node: PedigreeNode): number {
		let count = 0;
		if (node.father) {
			count += 1 + countAncestors(node.father);
		}
		if (node.mother) {
			count += 1 + countAncestors(node.mother);
		}
		return count;
	}

	/**
	 * Check if a node has ancestors (father or mother)
	 */
	function hasAncestors(node: PedigreeNode): boolean {
		return !!(node.father || node.mother);
	}

	/**
	 * Toggle collapse state for a node
	 */
	export function toggleCollapse(nodeId: string): void {
		// Clear any pending debounce timer
		if (collapseDebounceTimer) {
			clearTimeout(collapseDebounceTimer);
		}

		// Debounce rapid operations (50ms delay)
		collapseDebounceTimer = setTimeout(() => {
			if (collapsedNodes.has(nodeId)) {
				collapsedNodes.delete(nodeId);
			} else {
				collapsedNodes.add(nodeId);
			}
			renderChartWithAnimation();
		}, 50);
	}

	/**
	 * Check if a node is collapsed
	 */
	export function isCollapsed(nodeId: string): boolean {
		return collapsedNodes.has(nodeId);
	}

	/**
	 * Expand all collapsed branches
	 */
	export function expandAll(): void {
		collapsedNodes.clear();
		renderChartWithAnimation();
	}

	/**
	 * Collapse all branches that have ancestors
	 */
	export function collapseAll(): void {
		collapsedNodes.clear();
		function collectCollapsibleNodes(node: PedigreeNode) {
			if (node.id && hasAncestors(node)) {
				collapsedNodes.add(node.id);
			}
			if (node.father) collectCollapsibleNodes(node.father);
			if (node.mother) collectCollapsibleNodes(node.mother);
		}
		collectCollapsibleNodes(data);
		renderChartWithAnimation();
	}

	// Convert pedigree data to D3 hierarchy format, leaving out the ancestors of collapsed nodes
	function buildHierarchy(node: PedigreeNode, collapsed: Set<string>): d3.HierarchyNode<PedigreeNode> {
		const hierarchyData = buildHierarchyData(node, collapsed);
		return d3.hierarchy(hierarchyData as PedigreeNode);
	}

	function buildHierarchyData(node: PedigreeNode, collapsed: Set<string>): PedigreeNode & { children?: PedigreeNode[] } {
		// If this node is collapsed, don't include its ancestors
		if (node.id && collapsed.has(node.id)) {
			return {
				...node,
				children: undefined
			} as PedigreeNode & { children?: PedigreeNode[] };
		}

		// For pedigree charts, we show ancestors (parents above children)
		// We need to build the hierarchy with ancestors as "children" for the tree layout
		const children: PedigreeNode[] = [];
		if (node.father) children.push(node.father);
		if (node.mother) children.push(node.mother);

		return {
			...node,
			children: children.length > 0 ? children.map((c) => buildHierarchyData(c, collapsed)) : undefined
		} as PedigreeNode & { children?: PedigreeNode[] };
	}

	// Store previous node positions for animations
	let previousNodePositions: Map<string, { x: number; y: number }> = new Map();

	// Lay out the tree with `collapsed` folded, ancestors above (root at the bottom)
	function layoutTree(collapsed: Set<string>): d3.HierarchyPointNode<PedigreeNode> {
		const config = LAYOUTS[layout];
		const verticalSpacing = config.cardHeight + config.verticalGap;
		// Use nodeSize for consistent spacing regardless of tree size
		// Custom separation function to bring unrelated branches closer together
		const treeData = d3
			.tree<PedigreeNode>()
			.nodeSize([config.cardWidth + config.horizontalGap, verticalSpacing])
			.separation((a, b) => (a.parent === b.parent ? 1 : config.cousinSeparation))(buildHierarchy(data, collapsed));
		const maxDepth = treeData.height;
		treeData.each((d) => {
			d.y = (maxDepth - d.depth) * verticalSpacing;
		});
		return treeData;
	}

	// The nodes of the full tree `depth` generations up that have ancestors of their own
	function foldedAt(depth: number): Set<string> {
		return new Set(
			d3
				.hierarchy(data, (d) => [d.father, d.mother].filter((p): p is PedigreeNode => !!p))
				.descendants()
				.filter((d) => d.depth === depth && d.data.id && hasAncestors(d.data))
				.map((d) => d.data.id!)
		);
	}

	// The deepest collapse whose fitted first view shows both parents whole
	function computeFirstView(): Set<string> {
		const { cardWidth, cardHeight } = LAYOUTS[layout];
		const half = { width: (cardWidth / 2) * contentScale, height: (cardHeight / 2) * contentScale };
		const [width, height] = viewportSize();
		const fullDepth = buildHierarchy(data, new Set()).height;
		const depth = firstViewDepth(fullDepth, (depth) => {
			const nodes = layoutTree(foldedAt(depth)).descendants();
			const fit = fitToContent(nodes);
			const parents = nodes.filter((d) => d.depth === 1).map((d) => ({ x: d.x * contentScale, y: d.y * contentScale }));
			return fit !== null && cardsInView(parents, half, fit, width, height);
		});
		return foldedAt(depth);
	}

	function renderChart() {
		if (!container || !data) return;

		// Clear existing content
		d3.select(container).selectAll('*').remove();

		const [width, height] = viewportSize();
		zoomTarget = null;

		// Create SVG
		svg = d3
			.select(container)
			.append('svg')
			.attr('width', '100%')
			.attr('height', '100%')
			.attr('viewBox', `0 0 ${width} ${height}`);

		// Create group for zoomable content, drawn at the font-size scale
		g = svg.append('g');
		contentScale = getFontScale();
		const content = g.append('g').attr('class', 'chart-content').attr('transform', `scale(${contentScale})`);
		linkLayer = content.append('g').attr('class', 'links');
		nodeLayer = content.append('g').attr('class', 'nodes');

		// Setup zoom behavior
		zoom = d3
			.zoom<SVGSVGElement, unknown>()
			.scaleExtent(SCALE_EXTENT)
			.on('zoom', (event) => {
				g.attr('transform', event.transform);
				// A wheel or drag gesture replaces any pending button zoom
				if (event.sourceEvent) zoomTarget = null;
				updateOffscreen(event.transform);
			});

		svg.call(zoom);
		touchScrollsPage(svg.node()!);

		// Fold what the first view can't show, once per root; later redraws keep the user's state
		firstViewCollapsed = computeFirstView();
		if (showFirstView) {
			collapsedNodes = new Set(firstViewCollapsed);
			showFirstView = false;
		}

		const { cardWidth, cardHeight } = LAYOUTS[layout];
		const treeData = layoutTree(collapsedNodes);
		const nodes = treeData.descendants();
		const links = treeData.links();

		// Store nodes for navigation and build the nodeMap
		treeNodes = nodes;
		nodeMap = new Map();
		nodes.forEach((node) => {
			if (node.data.id) {
				nodeMap.set(node.data.id, node);
			}
		});

		// Draw links
		linkLayer.selectAll('.link')
			.data(links)
			.enter()
			.append('path')
			.attr('class', 'link')
			.attr('fill', 'none')
			.attr('stroke', '#94a3b8')
			.attr('stroke-width', 2)
			.attr(
				'd',
				d3
					.linkVertical<d3.HierarchyLink<PedigreeNode>, d3.HierarchyPointNode<PedigreeNode>>()
					.x((d) => d.x)
					.y((d) => d.y) as (
					d: d3.HierarchyLink<PedigreeNode>
				) => string | null
			);

		// Create node groups
		const nodeGroups = nodeLayer
			.selectAll('.node')
			.data(nodes)
			.enter()
			.append('g')
			.attr('class', 'node')
			.attr('transform', (d) => `translate(${d.x},${d.y})`)
			.style('cursor', 'pointer')
			.on('click', (_event, d) => {
				if (onPersonClick && d.data.id) {
					onPersonClick(d.data.id);
				}
			});

		// Render node cards
		renderNodeCards(nodeGroups, cardWidth, cardHeight);

		// Render collapse toggle buttons
		renderCollapseToggles(nodeGroups, cardWidth, cardHeight);

		// Store current positions for future animations
		nodes.forEach((d) => {
			if (d.data.id) {
				previousNodePositions.set(d.data.id, { x: d.x, y: d.y });
			}
		});

		// Initial zoom to fit
		const fit = fitToContent(treeNodes);
		if (fit) svg.call(zoom.transform, fit);
	}

	function viewportSize(): [number, number] {
		// Fractional size: a rounded viewBox would scale the chart slightly below 1:1
		const { width, height } = container.getBoundingClientRect();
		return [width || 800, height || 600];
	}

	// Keep one SVG unit per pixel when only the height changes, preserving zoom and pan
	function updateViewBox() {
		if (!svg) return;
		const [width, height] = viewportSize();
		svg.attr('viewBox', `0 0 ${width} ${height}`);
	}

	// The transform that fits the laid-out `nodes`, keeping the root in view
	function fitToContent(nodes: d3.HierarchyPointNode<PedigreeNode>[]): d3.ZoomTransform | null {
		if (nodes.length === 0) return null;
		const { cardWidth, cardHeight } = LAYOUTS[layout];
		const [minX, maxX] = d3.extent(nodes, (d) => d.x) as [number, number];
		const [minY, maxY] = d3.extent(nodes, (d) => d.y) as [number, number];
		const bounds = {
			x: minX - cardWidth / 2,
			y: minY - cardHeight / 2,
			width: maxX - minX + cardWidth,
			height: maxY - minY + cardHeight + TOGGLE_EXTENT
		};
		const [width, height] = viewportSize();
		const root = { x: nodes[0].x * contentScale, y: nodes[0].y * contentScale };
		return fitTransform(scaleBounds(bounds, contentScale), width, height, SCALE_EXTENT, root);
	}

	function updateOffscreen(t: d3.ZoomTransform) {
		if (!container) return;
		const parents = treeNodes.filter((d) => d.depth === 1).map((d) => d.x * contentScale);
		const [width] = viewportSize();
		offscreen = offscreenX(parents, (LAYOUTS[layout].cardWidth / 2) * contentScale, t, width);
	}

	function panToOffscreen(side: 'left' | 'right') {
		const x = offscreen[side][0];
		if (x === undefined || !svg) return;
		const [width] = viewportSize();
		animateZoomTo(centreOnX(zoomTarget ?? d3.zoomTransform(svg.node()!), x, width));
	}

	function animateZoomTo(target: d3.ZoomTransform) {
		zoomTarget = target;
		svg
			.transition()
			.duration(motionDuration(300))
			.call(zoom.transform, target)
			.on('end', () => {
				zoomTarget = null;
			});
	}

	function renderChartWithAnimation() {
		if (!container || !data) return;

		// Lay out the tree with the updated collapsed state
		const { cardWidth, cardHeight } = LAYOUTS[layout];
		const treeData = layoutTree(collapsedNodes);
		const nodes = treeData.descendants();
		const links = treeData.links();

		// Update nodeMap
		treeNodes = nodes;
		nodeMap = new Map();
		nodes.forEach((node) => {
			if (node.data.id) {
				nodeMap.set(node.data.id, node);
			}
		});

		// Create sets for tracking which nodes exist
		const newNodeIds = new Set(nodes.map((d) => d.data.id).filter(Boolean));
		const previousNodeIds = new Set(previousNodePositions.keys());

		// Update links with animation
		const linkSelection = linkLayer.selectAll<SVGPathElement, d3.HierarchyLink<PedigreeNode>>('.link')
			.data(links, (d) => `${d.source.data.id}-${d.target.data.id}`);

		// Remove old links
		linkSelection.exit()
			.transition()
			.duration(motionDuration(ANIMATION_DURATION))
			.attr('stroke-opacity', 0)
			.remove();

		// Add new links
		const newLinks = linkSelection.enter()
			.append('path')
			.attr('class', 'link')
			.attr('fill', 'none')
			.attr('stroke', '#94a3b8')
			.attr('stroke-width', 2)
			.attr('stroke-opacity', 0)
			.attr(
				'd',
				d3
					.linkVertical<d3.HierarchyLink<PedigreeNode>, d3.HierarchyPointNode<PedigreeNode>>()
					.x((d) => d.x)
					.y((d) => d.y) as (
					d: d3.HierarchyLink<PedigreeNode>
				) => string | null
			);

		// Animate new links appearing
		newLinks.transition()
			.duration(motionDuration(ANIMATION_DURATION))
			.attr('stroke-opacity', 1);

		// Update existing links
		linkSelection.transition()
			.duration(motionDuration(ANIMATION_DURATION))
			.attr(
				'd',
				d3
					.linkVertical<d3.HierarchyLink<PedigreeNode>, d3.HierarchyPointNode<PedigreeNode>>()
					.x((d) => d.x)
					.y((d) => d.y) as (
					d: d3.HierarchyLink<PedigreeNode>
				) => string | null
			);

		// Update nodes with animation
		const nodeSelection = nodeLayer.selectAll<SVGGElement, d3.HierarchyPointNode<PedigreeNode>>('.node')
			.data(nodes, (d) => d.data.id || '');

		// Remove old nodes with fade out
		nodeSelection.exit()
			.transition()
			.duration(motionDuration(ANIMATION_DURATION))
			.attr('opacity', 0)
			.remove();

		// Add new nodes
		const newNodes = nodeSelection.enter()
			.append('g')
			.attr('class', 'node')
			.attr('transform', (d) => {
				// Start from parent position if exists, otherwise from own position
				const parentId = d.parent?.data.id;
				if (parentId && previousNodePositions.has(parentId)) {
					const parentPos = previousNodePositions.get(parentId)!;
					return `translate(${parentPos.x},${parentPos.y})`;
				}
				return `translate(${d.x},${d.y})`;
			})
			.attr('opacity', 0)
			.style('cursor', 'pointer')
			.on('click', (_event, d) => {
				if (onPersonClick && d.data.id) {
					onPersonClick(d.data.id);
				}
			});

		// Render cards on new nodes
		renderNodeCards(newNodes, cardWidth, cardHeight);
		renderCollapseToggles(newNodes, cardWidth, cardHeight);

		// Animate new nodes appearing and moving to position
		newNodes.transition()
			.duration(motionDuration(ANIMATION_DURATION))
			.attr('transform', (d) => `translate(${d.x},${d.y})`)
			.attr('opacity', 1);

		// Animate existing nodes to new positions
		nodeSelection.transition()
			.duration(motionDuration(ANIMATION_DURATION))
			.attr('transform', (d) => `translate(${d.x},${d.y})`);

		// Update collapse toggle state on existing nodes
		nodeSelection.each(function(d) {
			const nodeGroup = d3.select(this) as d3.Selection<SVGGElement, d3.HierarchyPointNode<PedigreeNode>, null, undefined>;
			updateCollapseToggle(nodeGroup, d, cardWidth, cardHeight);
		});

		// Store new positions for future animations
		previousNodePositions = new Map();
		nodes.forEach((d) => {
			if (d.data.id) {
				previousNodePositions.set(d.data.id, { x: d.x, y: d.y });
			}
		});

		// Re-fit so the root stays in view as the tree grows or shrinks
		const fit = fitToContent(treeNodes);
		if (fit) animateZoomTo(fit);
	}

	function renderNodeCards(
		nodeGroups: d3.Selection<SVGGElement, d3.HierarchyPointNode<PedigreeNode>, SVGGElement, unknown>,
		cardWidth: number,
		cardHeight: number
	) {
		// Node card background
		nodeGroups
			.append('rect')
			.attr('class', 'node-card')
			.attr('x', -cardWidth / 2)
			.attr('y', -cardHeight / 2)
			.attr('width', cardWidth)
			.attr('height', cardHeight)
			.attr('rx', 8)
			.attr('fill', (d) => {
				if (d.data.gender === 'male') return '#dbeafe';
				if (d.data.gender === 'female') return '#fce7f3';
				return '#f1f5f9';
			})
			.attr('stroke', (d) => {
				if (d.data.id === selectedPersonId) return '#f59e0b'; // Amber for selected
				if (d.data.gender === 'male') return '#3b82f6';
				if (d.data.gender === 'female') return '#ec4899';
				return '#64748b';
			})
			.attr('stroke-width', (d) => d.data.id === selectedPersonId ? 3 : 2);

		// Add selection ring for better visibility
		nodeGroups
			.filter((d) => d.data.id === selectedPersonId)
			.insert('rect', '.node-card')
			.attr('class', 'selection-ring')
			.attr('x', -cardWidth / 2 - 4)
			.attr('y', -cardHeight / 2 - 4)
			.attr('width', cardWidth + 8)
			.attr('height', cardHeight + 8)
			.attr('rx', 12)
			.attr('fill', 'none')
			.attr('stroke', '#f59e0b')
			.attr('stroke-width', 2)
			.attr('stroke-dasharray', '4,2');

		// Full name on hover, since the lines below may be truncated
		nodeGroups
			.append('title')
			.text((d) => formatPersonName(d.data));

		// Given name (first line)
		nodeGroups
			.append('text')
			.attr('class', 'given-name')
			.attr('y', -14)
			.attr('text-anchor', 'middle')
			.attr('font-size', '13px')
			.attr('font-weight', '600')
			.attr('fill', '#1e293b')
			.text((d) => nameLines(d.data)[0])
			.each(function () {
				fitText(this, cardWidth - 2 * CARD_TEXT_INSET);
			});

		// Surname (second line)
		nodeGroups
			.append('text')
			.attr('class', 'surname')
			.attr('y', 2)
			.attr('text-anchor', 'middle')
			.attr('font-size', '13px')
			.attr('font-weight', '500')
			.attr('fill', '#475569')
			.text((d) => nameLines(d.data)[1])
			.each(function () {
				fitText(this, cardWidth - 2 * CARD_TEXT_INSET);
			});

		// Birth-death dates (third line)
		nodeGroups
			.append('text')
			.attr('class', 'dates')
			.attr('y', 18)
			.attr('text-anchor', 'middle')
			.attr('font-size', '11px')
			.attr('fill', '#64748b')
			.text((d) => {
				const birth = d.data.birth_date?.year;
				const death = d.data.death_date?.year;
				if (!birth && !death) return '';
				if (birth && !death) return `b. ${formatYear(birth)}`;
				if (!birth && death) return `d. ${formatYear(death)}`;
				return `${formatYear(birth!)} - ${formatYear(death!)}`;
			});
	}

	function renderCollapseToggles(
		nodeGroups: d3.Selection<SVGGElement, d3.HierarchyPointNode<PedigreeNode>, SVGGElement, unknown>,
		cardWidth: number,
		cardHeight: number
	) {
		// Filter to only nodes that have ancestors in the original data
		const nodesWithAncestors = nodeGroups.filter((d) => hasAncestors(d.data));

		// Create toggle group positioned below the card: a keyboard-operable
		// button whose label and state follow the collapse (#1100)
		const toggleGroups = nodesWithAncestors
			.append('g')
			.attr('class', 'collapse-toggle')
			.attr('transform', `translate(0, ${cardHeight / 2 + TOGGLE_OFFSET})`)
			.attr('role', 'button')
			.attr('tabindex', 0)
			.style('cursor', 'pointer')
			.on('click', (event, d) => {
				event.stopPropagation(); // Prevent triggering person click
				if (d.data.id) {
					toggleCollapse(d.data.id);
				}
			})
			.on('keydown', (event: KeyboardEvent, d) => {
				if (event.key !== 'Enter' && event.key !== ' ') return;
				// Keep the key from scrolling the page or reaching the chart shortcuts
				event.preventDefault();
				event.stopPropagation();
				if (d.data.id) {
					toggleCollapse(d.data.id);
				}
			});
		labelCollapseToggle(toggleGroups);

		// Invisible touch target, larger than the drawn button
		toggleGroups
			.append('circle')
			.attr('class', 'toggle-hit-area')
			.attr('r', TOGGLE_HIT_RADIUS)
			.attr('fill', 'transparent');

		// Focus ring, shown only while the toggle has keyboard focus
		toggleGroups
			.append('circle')
			.attr('class', 'toggle-focus-ring')
			.attr('r', TOGGLE_HIT_RADIUS)
			.attr('fill', 'none');

		// Toggle button circle
		toggleGroups
			.append('circle')
			.attr('class', 'toggle-button')
			.attr('r', TOGGLE_RADIUS)
			.attr('fill', '#f8fafc')
			.attr('stroke', '#94a3b8')
			.attr('stroke-width', 1.5);

		// Toggle button text (+ or -)
		toggleGroups
			.append('text')
			.attr('class', 'toggle-text')
			.attr('text-anchor', 'middle')
			.attr('dominant-baseline', 'central')
			.attr('font-size', '14px')
			.attr('font-weight', '600')
			.attr('fill', '#64748b')
			.attr('pointer-events', 'none')
			.text((d) => d.data.id && collapsedNodes.has(d.data.id) ? '+' : '-');

		// Ancestor count badge (only for collapsed nodes)
		const collapsedToggles = toggleGroups.filter((d) => Boolean(d.data.id && collapsedNodes.has(d.data.id)));

		collapsedToggles
			.append('g')
			.attr('class', 'ancestor-badge')
			.attr('transform', 'translate(16, 0)')
			.each(function(d) {
				const badgeGroup = d3.select(this);
				const ancestorCount = countAncestors(d.data);
				const badgeText = `+${ancestorCount}`;
				const textWidth = badgeText.length * 7 + 8;

				// Badge background
				badgeGroup
					.append('rect')
					.attr('x', 0)
					.attr('y', -8)
					.attr('width', textWidth)
					.attr('height', 16)
					.attr('rx', 8)
					.attr('fill', '#e2e8f0')
					.attr('stroke', '#94a3b8')
					.attr('stroke-width', 1);

				// Badge text
				badgeGroup
					.append('text')
					.attr('x', textWidth / 2)
					.attr('y', 0)
					.attr('text-anchor', 'middle')
					.attr('dominant-baseline', 'central')
					.attr('font-size', '11px')
					.attr('font-weight', '500')
					.attr('fill', '#475569')
					.text(badgeText);
			});
	}

	function labelCollapseToggle(
		toggles: d3.Selection<SVGGElement, d3.HierarchyPointNode<PedigreeNode>, d3.BaseType, unknown>
	) {
		const collapsed = (d: d3.HierarchyPointNode<PedigreeNode>) => Boolean(d.data.id && collapsedNodes.has(d.data.id));
		toggles
			.attr('aria-expanded', (d) => String(!collapsed(d)))
			.attr('aria-label', (d) => `${collapsed(d) ? 'Expand' : 'Collapse'} ancestors of ${formatPersonName(d.data)}`);
	}

	function updateCollapseToggle(
		nodeGroup: d3.Selection<SVGGElement, d3.HierarchyPointNode<PedigreeNode>, null, undefined>,
		d: d3.HierarchyPointNode<PedigreeNode>,
		cardWidth: number,
		cardHeight: number
	) {
		labelCollapseToggle(nodeGroup.select<SVGGElement>('.collapse-toggle'));

		// Update toggle text
		nodeGroup.select('.toggle-text')
			.text(d.data.id && collapsedNodes.has(d.data.id) ? '+' : '-');

		// Remove existing badge
		nodeGroup.select('.ancestor-badge').remove();

		// Add badge if collapsed
		if (d.data.id && collapsedNodes.has(d.data.id)) {
			const toggleGroup = nodeGroup.select('.collapse-toggle');
			if (!toggleGroup.empty()) {
				const badgeGroup = toggleGroup
					.append('g')
					.attr('class', 'ancestor-badge')
					.attr('transform', 'translate(16, 0)');

				const ancestorCount = countAncestors(d.data);
				const badgeText = `+${ancestorCount}`;
				const textWidth = badgeText.length * 7 + 8;

				// Badge background
				badgeGroup
					.append('rect')
					.attr('x', 0)
					.attr('y', -8)
					.attr('width', textWidth)
					.attr('height', 16)
					.attr('rx', 8)
					.attr('fill', '#e2e8f0')
					.attr('stroke', '#94a3b8')
					.attr('stroke-width', 1);

				// Badge text
				badgeGroup
					.append('text')
					.attr('x', textWidth / 2)
					.attr('y', 0)
					.attr('text-anchor', 'middle')
					.attr('dominant-baseline', 'central')
					.attr('font-size', '11px')
					.attr('font-weight', '500')
					.attr('fill', '#475569')
					.text(badgeText);
			}
		}
	}

	// Zoom control functions
	// Each step starts from the previous step's target, so rapid clicks add up
	function zoomBy(factor: number) {
		if (!svg || !zoom) return;
		const [width, height] = viewportSize();
		const from = zoomTarget ?? d3.zoomTransform(svg.node()!);
		animateZoomTo(scaleAbout(from, factor, [width / 2, height / 2], SCALE_EXTENT));
	}

	export function zoomIn() {
		zoomBy(1.2);
	}

	export function zoomOut() {
		zoomBy(0.8);
	}

	// Back to the first view: its collapse and its fit
	export function resetZoom() {
		if (!svg || !zoom) return;
		collapsedNodes = new Set(firstViewCollapsed);
		renderChartWithAnimation();
	}

	// Navigation helper functions
	/**
	 * Get the father of the currently selected person
	 */
	export function getFatherId(): string | null {
		if (!selectedPersonId) return null;
		const node = nodeMap.get(selectedPersonId);
		if (!node) return null;
		// In pedigree, father is the first child (left in the tree structure)
		const pedigreeData = node.data;
		return pedigreeData.father?.id || null;
	}

	/**
	 * Get the mother of the currently selected person
	 */
	export function getMotherId(): string | null {
		if (!selectedPersonId) return null;
		const node = nodeMap.get(selectedPersonId);
		if (!node) return null;
		const pedigreeData = node.data;
		return pedigreeData.mother?.id || null;
	}

	/**
	 * Get the root person ID (starting person of the pedigree)
	 */
	export function getRootId(): string | null {
		return data?.id || null;
	}

	/**
	 * Check if a person exists in the current pedigree view
	 */
	export function hasNode(personId: string): boolean {
		return nodeMap.has(personId);
	}

	/**
	 * Update selection highlighting without full re-render
	 */
	function updateSelectionHighlight() {
		if (!g) return;

		const config = LAYOUTS[layout];
		const cardWidth = config.cardWidth;
		const cardHeight = config.cardHeight;

		// Update existing node cards
		g.selectAll<SVGRectElement, d3.HierarchyPointNode<PedigreeNode>>('.node-card')
			.attr('stroke', (d) => {
				if (d.data.id === selectedPersonId) return '#f59e0b';
				if (d.data.gender === 'male') return '#3b82f6';
				if (d.data.gender === 'female') return '#ec4899';
				return '#64748b';
			})
			.attr('stroke-width', (d) => d.data.id === selectedPersonId ? 3 : 2);

		// Remove old selection rings
		g.selectAll('.selection-ring').remove();

		// Add new selection ring
		const selectedNode = g.selectAll<SVGGElement, d3.HierarchyPointNode<PedigreeNode>>('.node')
			.filter((d) => d.data.id === selectedPersonId);

		selectedNode
			.insert('rect', '.node-card')
			.attr('class', 'selection-ring')
			.attr('x', -cardWidth / 2 - 4)
			.attr('y', -cardHeight / 2 - 4)
			.attr('width', cardWidth + 8)
			.attr('height', cardHeight + 8)
			.attr('rx', 12)
			.attr('fill', 'none')
			.attr('stroke', '#f59e0b')
			.attr('stroke-width', 2)
			.attr('stroke-dasharray', '4,2');
	}

	onMount(() => {
		renderChart();

		// Re-render when the width changes; a height-only change keeps the view
		const stopObserving = observeChartResize(container, renderChart, updateViewBox);

		return () => {
			stopObserving();
			// Clear any pending debounce timer
			if (collapseDebounceTimer) {
				clearTimeout(collapseDebounceTimer);
			}
		};
	});

	// Track the previous data ID to detect when person changes
	let previousDataId: string | undefined = undefined;

	// Re-render when data or layout changes
	$effect(() => {
		// Reading the scale here redraws the chart when the font-size setting changes
		if (data && layout && getFontScale()) {
			// Reset collapsed state when the root person changes (new person selected)
			// Use untrack to prevent reactive loops when reading/writing previousDataId
			const prevId = untrack(() => previousDataId);
			if (prevId !== data.id) {
				untrack(() => {
					previousDataId = data.id;
				});
				showFirstView = true;
				previousNodePositions.clear();
			}
			renderChart();
		}
	});

	// Update selection highlighting when selectedPersonId changes (without full re-render)
	$effect(() => {
		if (selectedPersonId !== undefined && g) {
			updateSelectionHighlight();
		}
	});
</script>

<div class="chart-frame">
	<div
		class="pedigree-chart"
		bind:this={container}
		role="application"
		aria-label="Pedigree chart. Keys: {describeContextKeys('pedigree')}. Tab to the toggle buttons below nodes to collapse or expand ancestor branches."
		tabindex="0"
	></div>
	<OffscreenIndicator
		left={offscreen.left.length}
		right={offscreen.right.length}
		noun={['parent', 'parents']}
		onPan={panToOffscreen}
		focusFallback={container}
	/>
</div>

<style>
	.chart-frame {
		position: relative;
		width: 100%;
		height: 100%;
	}

	.pedigree-chart {
		width: 100%;
		height: 100%;
		min-height: 240px;
		/* Size from the page, never from the drawn SVG, or the chart could grow but not shrink */
		contain: size;
		background: #fafafa;
		border-radius: 8px;
		overflow: hidden;
	}

	.pedigree-chart:focus {
		outline: 2px solid var(--color-focus-ring);
		outline-offset: 2px;
	}

	:global(.pedigree-chart .node:hover rect.node-card) {
		filter: brightness(0.95);
	}

	:global(.pedigree-chart .collapse-toggle:hover circle.toggle-button) {
		fill: #e2e8f0;
		stroke: #64748b;
	}

	:global(.pedigree-chart .collapse-toggle:focus) {
		outline: none;
	}

	:global(.pedigree-chart .collapse-toggle:focus-visible circle.toggle-focus-ring) {
		stroke: var(--color-focus-ring);
		stroke-width: 2.5px;
	}

	/* High contrast mode support for selection indicator */
	@media (forced-colors: active) {
		:global(.pedigree-chart .selection-ring) {
			stroke: Highlight !important;
			stroke-width: 3px !important;
		}

		:global(.pedigree-chart .node-card[stroke-width="3"]) {
			stroke: Highlight !important;
		}
	}
</style>
