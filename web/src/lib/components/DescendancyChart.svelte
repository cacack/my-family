<script lang="ts">
	import * as d3 from 'd3';
	import { onMount } from 'svelte';
	import { formatGenDate, type DescendancyNode } from '$lib/api/client';
	import {
		fitTransform,
		motionDuration,
		observeChartResize,
		scaleAbout,
		truncateLabel,
		wrapLabel,
		type ScaleExtent
	} from '$lib/utils/chart';

	export type LayoutMode = 'compact' | 'standard' | 'wide';

	interface LayoutConfig {
		cardWidth: number;
		cardHeight: number;
		horizontalGap: number;
		verticalGap: number;
		siblingSeparation: number; // Separation multiplier for siblings
		spouseGap: number; // Gap between person and spouse cards
		spouseCardScale: number; // Scale factor for spouse cards (slightly smaller)
	}

	const LAYOUTS: Record<LayoutMode, LayoutConfig> = {
		compact: {
			cardWidth: 120,
			cardHeight: 65,
			horizontalGap: 8,
			verticalGap: 25,
			siblingSeparation: 1.0,
			spouseGap: 8,
			spouseCardScale: 0.9
		},
		standard: {
			cardWidth: 140,
			cardHeight: 75,
			horizontalGap: 20,
			verticalGap: 40,
			siblingSeparation: 1.0,
			spouseGap: 12,
			spouseCardScale: 0.9
		},
		wide: {
			cardWidth: 160,
			cardHeight: 80,
			horizontalGap: 40,
			verticalGap: 50,
			siblingSeparation: 1.0,
			spouseGap: 16,
			spouseCardScale: 0.9
		}
	};

	const SCALE_EXTENT: ScaleExtent = [0.1, 4];

	interface Props {
		data: DescendancyNode;
		layout?: LayoutMode;
		selectedPersonId?: string | null;
		onPersonClick?: (personId: string) => void;
		onSelectionChange?: (personId: string | null) => void;
	}

	let {
		data,
		layout = 'standard',
		selectedPersonId = null,
		onPersonClick,
		onSelectionChange
	}: Props = $props();

	let container: HTMLDivElement;
	let svg: d3.Selection<SVGSVGElement, unknown, null, undefined>;
	let g: d3.Selection<SVGGElement, unknown, null, undefined>;
	let zoom: d3.ZoomBehavior<SVGSVGElement, unknown>;
	// Where the running animated zoom is heading, so quick +/- clicks chain
	let zoomTarget: d3.ZoomTransform | null = null;

	// Map of person ID to their node data for navigation
	let nodeMap: Map<string, d3.HierarchyPointNode<DescendancyNode>> = new Map();
	// Store the tree data for navigation
	let treeNodes: d3.HierarchyPointNode<DescendancyNode>[] = [];

	// Convert descendancy data to D3 hierarchy format
	function buildHierarchy(node: DescendancyNode): d3.HierarchyNode<DescendancyNode> {
		return d3.hierarchy(node, (d) => d.children);
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

		// Create group for zoomable content
		g = svg.append('g');

		// Setup zoom behavior
		zoom = d3
			.zoom<SVGSVGElement, unknown>()
			.scaleExtent(SCALE_EXTENT)
			.on('zoom', (event) => {
				g.attr('transform', event.transform);
				// A wheel or drag gesture replaces any pending button zoom
				if (event.sourceEvent) zoomTarget = null;
			});

		svg.call(zoom);

		// Build hierarchy and create tree layout
		const root = buildHierarchy(data);

		// Get layout config
		const config = LAYOUTS[layout];
		const cardWidth = config.cardWidth;
		const cardHeight = config.cardHeight;
		// Account for spouse cards in horizontal spacing
		const spouseWidth = config.spouseCardScale * cardWidth + config.spouseGap;
		const horizontalSpacing = cardWidth + spouseWidth + config.horizontalGap;
		const verticalSpacing = cardHeight + config.verticalGap;

		// Use nodeSize for consistent spacing regardless of tree size
		const treeLayout = d3
			.tree<DescendancyNode>()
			.nodeSize([horizontalSpacing, verticalSpacing])
			.separation((a, b) => (a.parent === b.parent ? config.siblingSeparation : 1.2));

		const treeData = treeLayout(root);
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

		// Draw links (vertical lines from parent to children)
		g.selectAll('.link')
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
					.linkVertical<d3.HierarchyLink<DescendancyNode>, d3.HierarchyPointNode<DescendancyNode>>()
					.x((d) => d.x)
					.y((d) => d.y) as (d: d3.HierarchyLink<DescendancyNode>) => string | null
			);

		// Create node groups
		const nodeGroups = g
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

		// Render main person card
		renderPersonCard(nodeGroups, cardWidth, cardHeight);

		// Render spouse cards for each node
		nodes.forEach((node) => {
			if (node.data.spouses && node.data.spouses.length > 0) {
				renderSpouseCards(node, config);
			}
		});

		// Initial zoom to fit
		const fit = fitToContent();
		if (fit) svg.call(zoom.transform, fit);
	}

	function viewportSize(): [number, number] {
		return [container.clientWidth || 800, container.clientHeight || 600];
	}

	// Keep one SVG unit per pixel when only the height changes, preserving zoom and pan
	function updateViewBox() {
		if (!svg) return;
		const [width, height] = viewportSize();
		svg.attr('viewBox', `0 0 ${width} ${height}`);
	}

	// The transform that fits the drawn tree, keeping the root in view
	function fitToContent(): d3.ZoomTransform | null {
		const bounds = g?.node()?.getBBox();
		if (!bounds || treeNodes.length === 0) return null;
		const [width, height] = viewportSize();
		return fitTransform(bounds, width, height, SCALE_EXTENT, treeNodes[0]);
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

	function renderPersonCard(
		nodeGroups: d3.Selection<
			SVGGElement,
			d3.HierarchyPointNode<DescendancyNode>,
			SVGGElement,
			unknown
		>,
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
			.attr('stroke-width', (d) => (d.data.id === selectedPersonId ? 3 : 2));

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
			.text((d) => [d.data.given_name, d.data.surname].filter(Boolean).join(' ') || 'Unknown');

		// Given name (first line)
		nodeGroups
			.append('text')
			.attr('y', -14)
			.attr('text-anchor', 'middle')
			.attr('font-size', '13px')
			.attr('font-weight', '600')
			.attr('fill', '#1e293b')
			.text((d) => {
				return truncateLabel(d.data.given_name || '?', 16);
			});

		// Surname (second line)
		nodeGroups
			.append('text')
			.attr('y', 2)
			.attr('text-anchor', 'middle')
			.attr('font-size', '13px')
			.attr('font-weight', '500')
			.attr('fill', '#475569')
			.text((d) => {
				return truncateLabel(d.data.surname || '?', 16);
			});

		// Birth-death dates (third line)
		nodeGroups
			.append('text')
			.attr('y', 18)
			.attr('text-anchor', 'middle')
			.attr('font-size', '11px')
			.attr('fill', '#64748b')
			.text((d) => {
				const birth = d.data.birth_date?.year;
				const death = d.data.death_date?.year;
				if (!birth && !death) return '';
				if (birth && !death) return `b. ${birth}`;
				if (!birth && death) return `d. ${death}`;
				return `${birth} - ${death}`;
			});
	}

	function renderSpouseCards(
		node: d3.HierarchyPointNode<DescendancyNode>,
		config: LayoutConfig
	) {
		const spouses = node.data.spouses || [];
		const cardWidth = config.cardWidth;
		const cardHeight = config.cardHeight;
		const spouseCardWidth = cardWidth * config.spouseCardScale;
		const spouseCardHeight = cardHeight * config.spouseCardScale;

		spouses.forEach((spouse, index) => {
			// Position spouse to the right of the person
			const spouseX = node.x + cardWidth / 2 + config.spouseGap + spouseCardWidth / 2;
			const spouseY = node.y + index * (spouseCardHeight + 4);

			// Create spouse group
			const spouseGroup = g
				.append('g')
				.attr('class', 'spouse-group')
				.attr('transform', `translate(${spouseX},${spouseY})`)
				.style('cursor', 'pointer')
				.on('click', () => {
					if (onPersonClick && spouse.id) {
						onPersonClick(spouse.id);
					}
				});

			// Draw horizontal connector line from person to spouse
			g.append('line')
				.attr('class', 'spouse-link')
				.attr('x1', node.x + cardWidth / 2)
				.attr('y1', node.y)
				.attr('x2', spouseX - spouseCardWidth / 2)
				.attr('y2', spouseY)
				.attr('stroke', '#94a3b8')
				.attr('stroke-width', 2)
				.attr('stroke-dasharray', '4,2');

			// Spouse card background (neutral: the API gives no gender for spouses)
			spouseGroup
				.append('rect')
				.attr('class', 'spouse-card')
				.attr('x', -spouseCardWidth / 2)
				.attr('y', -spouseCardHeight / 2)
				.attr('width', spouseCardWidth)
				.attr('height', spouseCardHeight)
				.attr('rx', 6)
				.attr('fill', '#f1f5f9')
				.attr('stroke', spouse.id === selectedPersonId ? '#f59e0b' : '#64748b')
				.attr('stroke-width', spouse.id === selectedPersonId ? 3 : 1.5)
				.attr('opacity', 0.95);

			// Selection ring for spouse if selected
			if (spouse.id === selectedPersonId) {
				spouseGroup
					.insert('rect', '.spouse-card')
					.attr('class', 'selection-ring')
					.attr('x', -spouseCardWidth / 2 - 3)
					.attr('y', -spouseCardHeight / 2 - 3)
					.attr('width', spouseCardWidth + 6)
					.attr('height', spouseCardHeight + 6)
					.attr('rx', 9)
					.attr('fill', 'none')
					.attr('stroke', '#f59e0b')
					.attr('stroke-width', 2)
					.attr('stroke-dasharray', '4,2');
			}

			// Full name on hover, since the lines below may be truncated
			spouseGroup.append('title').text(spouse.name || 'Unknown');

			// Spouse name, wrapped over two lines
			const [nameLine1, nameLine2] = wrapLabel(spouse.name || '?', 18);
			spouseGroup
				.append('text')
				.attr('y', -10)
				.attr('text-anchor', 'middle')
				.attr('font-size', '11px')
				.attr('font-weight', '600')
				.attr('fill', '#1e293b')
				.text(nameLine1);

			spouseGroup
				.append('text')
				.attr('y', 3)
				.attr('text-anchor', 'middle')
				.attr('font-size', '11px')
				.attr('font-weight', '600')
				.attr('fill', '#1e293b')
				.text(nameLine2);

			// Marriage date
			spouseGroup
				.append('text')
				.attr('y', 16)
				.attr('text-anchor', 'middle')
				.attr('font-size', '9px')
				.attr('fill', '#64748b')
				.text(spouse.marriage_date
						? `m. ${spouse.marriage_date.year ?? formatGenDate(spouse.marriage_date)}`
						: '');
		});
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

	export function resetZoom() {
		if (!svg || !zoom) return;
		const fit = fitToContent();
		if (fit) animateZoomTo(fit);
	}

	// Navigation helper functions
	/**
	 * Get the first child of the currently selected person
	 */
	export function getFirstChildId(): string | null {
		if (!selectedPersonId) return null;
		const node = nodeMap.get(selectedPersonId);
		if (!node) return null;
		const children = node.data.children;
		return children && children.length > 0 ? children[0].id : null;
	}

	/**
	 * Get the parent of the currently selected person (in the descendancy tree)
	 */
	export function getParentId(): string | null {
		if (!selectedPersonId) return null;
		const node = nodeMap.get(selectedPersonId);
		if (!node || !node.parent) return null;
		return node.parent.data.id || null;
	}

	/**
	 * Get the root person ID (starting person of the descendancy)
	 */
	export function getRootId(): string | null {
		return data?.id || null;
	}

	/**
	 * Get the next sibling ID
	 */
	export function getNextSiblingId(): string | null {
		if (!selectedPersonId) return null;
		const node = nodeMap.get(selectedPersonId);
		if (!node || !node.parent) return null;
		const siblings = node.parent.children || [];
		const currentIndex = siblings.findIndex((s) => s.data.id === selectedPersonId);
		if (currentIndex >= 0 && currentIndex < siblings.length - 1) {
			return siblings[currentIndex + 1].data.id || null;
		}
		return null;
	}

	/**
	 * Get the previous sibling ID
	 */
	export function getPrevSiblingId(): string | null {
		if (!selectedPersonId) return null;
		const node = nodeMap.get(selectedPersonId);
		if (!node || !node.parent) return null;
		const siblings = node.parent.children || [];
		const currentIndex = siblings.findIndex((s) => s.data.id === selectedPersonId);
		if (currentIndex > 0) {
			return siblings[currentIndex - 1].data.id || null;
		}
		return null;
	}

	/**
	 * Get the first spouse ID of the selected person
	 */
	export function getSpouseId(): string | null {
		if (!selectedPersonId) return null;
		const node = nodeMap.get(selectedPersonId);
		if (!node) return null;
		const spouses = node.data.spouses;
		return spouses && spouses.length > 0 ? spouses[0].id : null;
	}

	/**
	 * Check if a person exists in the current descendancy view
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
		g.selectAll<SVGRectElement, d3.HierarchyPointNode<DescendancyNode>>('.node-card')
			.attr('stroke', (d) => {
				if (d.data.id === selectedPersonId) return '#f59e0b';
				if (d.data.gender === 'male') return '#3b82f6';
				if (d.data.gender === 'female') return '#ec4899';
				return '#64748b';
			})
			.attr('stroke-width', (d) => (d.data.id === selectedPersonId ? 3 : 2));

		// Remove old selection rings
		g.selectAll('.selection-ring').remove();

		// Add new selection ring for main nodes
		const selectedNode = g
			.selectAll<SVGGElement, d3.HierarchyPointNode<DescendancyNode>>('.node')
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
		return observeChartResize(container, renderChart, updateViewBox);
	});

	// Re-render when data or layout changes
	$effect(() => {
		if (data && layout) {
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

<div
	class="descendancy-chart"
	bind:this={container}
	role="application"
	aria-label="Descendancy chart. Use arrow keys to navigate: Down for first child, Up to return to parent, Left/Right to cycle siblings. Plus/minus to zoom, R to reset view."
	tabindex="0"
></div>

<style>
	.descendancy-chart {
		width: 100%;
		height: 100%;
		min-height: 240px;
		background: #fafafa;
		border-radius: 8px;
		overflow: hidden;
	}

	.descendancy-chart:focus {
		outline: 2px solid #3b82f6;
		outline-offset: 2px;
	}

	:global(.descendancy-chart .node:hover rect.node-card) {
		filter: brightness(0.95);
	}

	:global(.descendancy-chart .spouse-group:hover rect.spouse-card) {
		filter: brightness(0.95);
	}

	/* High contrast mode support for selection indicator */
	@media (forced-colors: active) {
		:global(.descendancy-chart .selection-ring) {
			stroke: Highlight !important;
			stroke-width: 3px !important;
		}

		:global(.descendancy-chart .node-card[stroke-width="3"]) {
			stroke: Highlight !important;
		}
	}
</style>
