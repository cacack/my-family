<script lang="ts">
	import * as d3 from 'd3';
	import { onMount } from 'svelte';
	import type { PedigreeNode } from '$lib/api/client';

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
	const LAYOUTS: Record<LayoutMode, LayoutConfig> = {
		compact: { cardWidth: 120, cardHeight: 65, horizontalGap: 8, verticalGap: 25, cousinSeparation: 0.95 },
		standard: { cardWidth: 140, cardHeight: 75, horizontalGap: 20, verticalGap: 40, cousinSeparation: 0.9 },
		wide: { cardWidth: 160, cardHeight: 80, horizontalGap: 40, verticalGap: 50, cousinSeparation: 0.85 }
	};

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
	let zoom: d3.ZoomBehavior<SVGSVGElement, unknown>;

	// Map of person ID to their node data for navigation
	let nodeMap: Map<string, d3.HierarchyPointNode<PedigreeNode>> = new Map();
	// Store the tree data for navigation
	let treeNodes: d3.HierarchyPointNode<PedigreeNode>[] = [];

	interface TreeNode {
		data: PedigreeNode;
		x: number;
		y: number;
		parent: TreeNode | null;
		children: TreeNode[];
	}

	// Convert pedigree data to D3 hierarchy format
	function buildHierarchy(node: PedigreeNode): d3.HierarchyNode<PedigreeNode> {
		// For pedigree charts, we show ancestors (parents above children)
		// We need to build the hierarchy with ancestors as "children" for the tree layout
		const children: PedigreeNode[] = [];
		if (node.father) children.push(node.father);
		if (node.mother) children.push(node.mother);

		const hierarchyData = {
			...node,
			children: children.length > 0 ? children.map((c) => buildHierarchyData(c)) : undefined
		};

		return d3.hierarchy(hierarchyData as PedigreeNode);
	}

	function buildHierarchyData(node: PedigreeNode): PedigreeNode & { children?: PedigreeNode[] } {
		const children: PedigreeNode[] = [];
		if (node.father) children.push(node.father);
		if (node.mother) children.push(node.mother);

		return {
			...node,
			children: children.length > 0 ? children.map((c) => buildHierarchyData(c)) : undefined
		} as PedigreeNode & { children?: PedigreeNode[] };
	}

	function renderChart() {
		if (!container || !data) return;

		// Clear existing content
		d3.select(container).selectAll('*').remove();

		const width = container.clientWidth || 800;
		const height = container.clientHeight || 600;

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
			.scaleExtent([0.1, 4])
			.on('zoom', (event) => {
				g.attr('transform', event.transform);
			});

		svg.call(zoom);

		// Build hierarchy and create tree layout
		const root = buildHierarchy(data);

		// Get layout config
		const config = LAYOUTS[layout];
		const cardWidth = config.cardWidth;
		const cardHeight = config.cardHeight;
		const horizontalSpacing = cardWidth + config.horizontalGap;
		const verticalSpacing = cardHeight + config.verticalGap;

		// Use nodeSize for consistent spacing regardless of tree size
		// Custom separation function to bring unrelated branches closer together
		const treeLayout = d3
			.tree<PedigreeNode>()
			.nodeSize([horizontalSpacing, verticalSpacing])
			.separation((a, b) => (a.parent === b.parent ? 1 : config.cousinSeparation));

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

		// Flip y coordinates so ancestors are above (root at bottom)
		const maxDepth = d3.max(nodes, (d) => d.depth) || 0;
		nodes.forEach((d) => {
			d.y = (maxDepth - d.depth) * verticalSpacing;
		});

		// Draw links
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
					.linkVertical<d3.HierarchyLink<PedigreeNode>, d3.HierarchyPointNode<PedigreeNode>>()
					.x((d) => d.x)
					.y((d) => d.y) as (
					d: d3.HierarchyLink<PedigreeNode>
				) => string | null
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

		// Given name (first line)
		nodeGroups
			.append('text')
			.attr('y', -14)
			.attr('text-anchor', 'middle')
			.attr('font-size', '13px')
			.attr('font-weight', '600')
			.attr('fill', '#1e293b')
			.text((d) => {
				const given = d.data.given_name || '?';
				return given.length > 16 ? given.substring(0, 14) + '...' : given;
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
				const surname = d.data.surname || '?';
				return surname.length > 16 ? surname.substring(0, 14) + '...' : surname;
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

		// Initial zoom to fit
		const bounds = g.node()?.getBBox();
		if (bounds) {
			const dx = bounds.width;
			const dy = bounds.height;
			const x = bounds.x + dx / 2;
			const y = bounds.y + dy / 2;
			const scale = 0.85 / Math.max(dx / width, dy / height);
			const translate = [width / 2 - scale * x, height / 2 - scale * y];

			svg.call(
				zoom.transform,
				d3.zoomIdentity.translate(translate[0], translate[1]).scale(scale)
			);
		}
	}

	// Zoom control functions
	export function zoomIn() {
		if (svg && zoom) {
			svg.transition().duration(300).call(zoom.scaleBy, 1.2);
		}
	}

	export function zoomOut() {
		if (svg && zoom) {
			svg.transition().duration(300).call(zoom.scaleBy, 0.8);
		}
	}

	export function resetZoom() {
		if (svg && zoom) {
			svg.transition().duration(300).call(zoom.transform, d3.zoomIdentity);
			// Re-fit after reset
			setTimeout(renderChart, 350);
		}
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
	 * Get the first spouse/family ID if available
	 * Note: In pedigree chart data, spouses are not directly available
	 * This would need additional data from the API
	 */
	export function getSpouseId(): string | null {
		// Spouse navigation would require additional data - returning null for now
		// Could be extended when the API provides spouse information
		return null;
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

		// Re-render on window resize
		const resizeObserver = new ResizeObserver(() => {
			renderChart();
		});
		resizeObserver.observe(container);

		return () => {
			resizeObserver.disconnect();
		};
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
	class="pedigree-chart"
	bind:this={container}
	role="application"
	aria-label="Pedigree chart. Use arrow keys to navigate: Up for father, Left for mother, Down to return to root. Plus/minus to zoom, R to reset view."
	tabindex="0"
></div>

<style>
	.pedigree-chart {
		width: 100%;
		height: 100%;
		min-height: 400px;
		background: #fafafa;
		border-radius: 8px;
		overflow: hidden;
	}

	.pedigree-chart:focus {
		outline: 2px solid #3b82f6;
		outline-offset: 2px;
	}

	:global(.pedigree-chart .node:hover rect.node-card) {
		filter: brightness(0.95);
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
