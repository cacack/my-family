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

	// Animation duration for collapse/expand transitions
	const ANIMATION_DURATION = 300;

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

	// Collapsed nodes state - tracks which node IDs have their ancestors collapsed
	// Using a plain Set since reactivity is managed through manual re-renders
	let collapsedNodes: Set<string> = new Set();

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

	// Convert pedigree data to D3 hierarchy format, respecting collapsed state
	function buildHierarchy(node: PedigreeNode): d3.HierarchyNode<PedigreeNode> {
		const hierarchyData = buildHierarchyData(node);
		return d3.hierarchy(hierarchyData as PedigreeNode);
	}

	function buildHierarchyData(node: PedigreeNode): PedigreeNode & { children?: PedigreeNode[] } {
		// If this node is collapsed, don't include its ancestors
		if (node.id && collapsedNodes.has(node.id)) {
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
			children: children.length > 0 ? children.map((c) => buildHierarchyData(c)) : undefined
		} as PedigreeNode & { children?: PedigreeNode[] };
	}

	// Store previous node positions for animations
	let previousNodePositions: Map<string, { x: number; y: number }> = new Map();

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

	function renderChartWithAnimation() {
		if (!container || !data) return;

		const width = container.clientWidth || 800;
		const height = container.clientHeight || 600;

		// Build new hierarchy with updated collapsed state
		const root = buildHierarchy(data);

		// Get layout config
		const config = LAYOUTS[layout];
		const cardWidth = config.cardWidth;
		const cardHeight = config.cardHeight;
		const horizontalSpacing = cardWidth + config.horizontalGap;
		const verticalSpacing = cardHeight + config.verticalGap;

		const treeLayout = d3
			.tree<PedigreeNode>()
			.nodeSize([horizontalSpacing, verticalSpacing])
			.separation((a, b) => (a.parent === b.parent ? 1 : config.cousinSeparation));

		const treeData = treeLayout(root);
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

		// Flip y coordinates
		const maxDepth = d3.max(nodes, (d) => d.depth) || 0;
		nodes.forEach((d) => {
			d.y = (maxDepth - d.depth) * verticalSpacing;
		});

		// Create sets for tracking which nodes exist
		const newNodeIds = new Set(nodes.map((d) => d.data.id).filter(Boolean));
		const previousNodeIds = new Set(previousNodePositions.keys());

		// Update links with animation
		const linkSelection = g.selectAll<SVGPathElement, d3.HierarchyLink<PedigreeNode>>('.link')
			.data(links, (d) => `${d.source.data.id}-${d.target.data.id}`);

		// Remove old links
		linkSelection.exit()
			.transition()
			.duration(ANIMATION_DURATION)
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
			.duration(ANIMATION_DURATION)
			.attr('stroke-opacity', 1);

		// Update existing links
		linkSelection.transition()
			.duration(ANIMATION_DURATION)
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
		const nodeSelection = g.selectAll<SVGGElement, d3.HierarchyPointNode<PedigreeNode>>('.node')
			.data(nodes, (d) => d.data.id || '');

		// Remove old nodes with fade out
		nodeSelection.exit()
			.transition()
			.duration(ANIMATION_DURATION)
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
			.duration(ANIMATION_DURATION)
			.attr('transform', (d) => `translate(${d.x},${d.y})`)
			.attr('opacity', 1);

		// Animate existing nodes to new positions
		nodeSelection.transition()
			.duration(ANIMATION_DURATION)
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

		// Given name (first line)
		nodeGroups
			.append('text')
			.attr('class', 'given-name')
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
			.attr('class', 'surname')
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
			.attr('class', 'dates')
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

	function renderCollapseToggles(
		nodeGroups: d3.Selection<SVGGElement, d3.HierarchyPointNode<PedigreeNode>, SVGGElement, unknown>,
		cardWidth: number,
		cardHeight: number
	) {
		// Filter to only nodes that have ancestors in the original data
		const nodesWithAncestors = nodeGroups.filter((d) => hasAncestors(d.data));

		// Create toggle group positioned below the card
		const toggleGroups = nodesWithAncestors
			.append('g')
			.attr('class', 'collapse-toggle')
			.attr('transform', `translate(0, ${cardHeight / 2 + 12})`)
			.style('cursor', 'pointer')
			.on('click', (event, d) => {
				event.stopPropagation(); // Prevent triggering person click
				if (d.data.id) {
					toggleCollapse(d.data.id);
				}
			});

		// Toggle button circle
		toggleGroups
			.append('circle')
			.attr('class', 'toggle-button')
			.attr('r', 9)
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
					.attr('font-size', '10px')
					.attr('font-weight', '500')
					.attr('fill', '#475569')
					.text(badgeText);
			});
	}

	function updateCollapseToggle(
		nodeGroup: d3.Selection<SVGGElement, d3.HierarchyPointNode<PedigreeNode>, null, undefined>,
		d: d3.HierarchyPointNode<PedigreeNode>,
		cardWidth: number,
		cardHeight: number
	) {
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
					.attr('font-size', '10px')
					.attr('font-weight', '500')
					.attr('fill', '#475569')
					.text(badgeText);
			}
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
		if (data && layout) {
			// Reset collapsed state when the root person changes (new person selected)
			if (previousDataId !== data.id) {
				previousDataId = data.id;
				// Use untracked assignment to avoid reactive loops
				collapsedNodes.clear();
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

<div
	class="pedigree-chart"
	bind:this={container}
	role="application"
	aria-label="Pedigree chart. Use arrow keys to navigate: Up for father, Left for mother, Down to return to root. Plus/minus to zoom, R to reset view. Click toggle buttons below nodes to collapse or expand ancestor branches."
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

	:global(.pedigree-chart .collapse-toggle:hover circle.toggle-button) {
		fill: #e2e8f0;
		stroke: #64748b;
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
