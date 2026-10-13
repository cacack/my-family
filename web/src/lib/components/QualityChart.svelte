<script lang="ts">
	import * as d3 from 'd3';
	import { onMount } from 'svelte';

	interface ChartData {
		label: string;
		value: number;
		color?: string;
	}

	interface Props {
		data: ChartData[];
	}

	let { data }: Props = $props();

	let container: HTMLDivElement;

	const defaultColors = ['#3b82f6', '#ef4444', '#eab308', '#22c55e', '#8b5cf6', '#ec4899'];

	function renderChart() {
		if (!container || !data || data.length === 0) return;

		// Clear existing content
		d3.select(container).selectAll('*').remove();

		// Each label sits on its own line above its bar so it gets the full chart
		// width instead of a fixed left margin that cut long labels off (#922).
		const margin = { top: 4, right: 60, bottom: 4, left: 0 };
		const width = container.clientWidth || 400;
		const labelHeight = 18;
		const barHeight = 20;
		const rowGap = 12;
		const rowHeight = labelHeight + barHeight + rowGap;
		const height = data.length * rowHeight - rowGap + margin.top + margin.bottom;

		const svg = d3
			.select(container)
			.append('svg')
			.attr('width', '100%')
			.attr('height', height)
			.attr('viewBox', `0 0 ${width} ${height}`);

		const chartWidth = Math.max(width - margin.left - margin.right, 0);

		// Find max value for scale
		const maxValue = d3.max(data, (d) => d.value) || 1;

		// Create scale
		const xScale = d3.scaleLinear().domain([0, maxValue]).range([0, chartWidth]);

		// Create groups for each bar
		const barGroups = svg
			.selectAll('.bar-group')
			.data(data)
			.enter()
			.append('g')
			.attr('class', 'bar-group')
			.attr('transform', (_, i) => `translate(0, ${margin.top + i * rowHeight})`);

		// Full label and count as the tooltip and accessible name, even if the
		// visible label has to be truncated on a very narrow chart.
		barGroups.append('title').text((d) => `${d.label}: ${d.value.toLocaleString()}`);

		// Label above the bar
		barGroups
			.append('text')
			.attr('class', 'bar-label')
			.attr('x', margin.left)
			.attr('y', labelHeight / 2)
			.attr('dominant-baseline', 'middle')
			.attr('font-size', '13px')
			.attr('fill', '#475569')
			.text((d) => d.label)
			.each(function () {
				fitText(this, width - margin.left);
			});

		const barY = labelHeight;

		// Background bars
		barGroups
			.append('rect')
			.attr('x', margin.left)
			.attr('y', barY)
			.attr('width', chartWidth)
			.attr('height', barHeight)
			.attr('fill', '#f1f5f9')
			.attr('rx', 4);

		// Value bars
		barGroups
			.append('rect')
			.attr('x', margin.left)
			.attr('y', barY)
			.attr('width', (d) => xScale(d.value))
			.attr('height', barHeight)
			.attr('fill', (d, i) => d.color || defaultColors[i % defaultColors.length])
			.attr('rx', 4);

		// Value text on right
		barGroups
			.append('text')
			.attr('x', (d) => margin.left + xScale(d.value) + 8)
			.attr('y', barY + barHeight / 2)
			.attr('dominant-baseline', 'middle')
			.attr('font-size', '13px')
			.attr('font-weight', '600')
			.attr('fill', '#1e293b')
			.text((d) => d.value.toLocaleString());
	}

	// Shorten a text node with an ellipsis until it fits maxWidth. A no-op where
	// text can't be measured (e.g. jsdom), leaving the full label.
	function fitText(node: SVGTextElement, maxWidth: number) {
		if (typeof node.getComputedTextLength !== 'function') return;
		const full = node.textContent ?? '';
		if (node.getComputedTextLength() <= maxWidth) return;
		let end = full.length;
		while (end > 0) {
			end--;
			node.textContent = full.slice(0, end).trimEnd() + '…';
			if (node.getComputedTextLength() <= maxWidth) return;
		}
	}

	onMount(() => {
		renderChart();

		const resizeObserver = new ResizeObserver(() => {
			renderChart();
		});
		resizeObserver.observe(container);

		return () => {
			resizeObserver.disconnect();
		};
	});

	$effect(() => {
		if (data) {
			renderChart();
		}
	});
</script>

<div class="quality-chart" bind:this={container}></div>

<style>
	.quality-chart {
		width: 100%;
		min-height: 100px;
	}
</style>
