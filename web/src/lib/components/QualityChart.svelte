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

		const margin = { top: 10, right: 60, bottom: 10, left: 140 };
		const width = container.clientWidth || 400;
		const barHeight = 28;
		const barGap = 8;
		const height = data.length * (barHeight + barGap) + margin.top + margin.bottom;

		const svg = d3
			.select(container)
			.append('svg')
			.attr('width', '100%')
			.attr('height', height)
			.attr('viewBox', `0 0 ${width} ${height}`);

		const chartWidth = width - margin.left - margin.right;

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
			.attr('transform', (_, i) => `translate(0, ${margin.top + i * (barHeight + barGap)})`);

		// Labels on left
		barGroups
			.append('text')
			.attr('x', margin.left - 10)
			.attr('y', barHeight / 2)
			.attr('text-anchor', 'end')
			.attr('dominant-baseline', 'middle')
			.attr('font-size', '13px')
			.attr('fill', '#475569')
			.text((d) => {
				const label = d.label;
				return label.length > 20 ? label.substring(0, 18) + '...' : label;
			});

		// Background bars
		barGroups
			.append('rect')
			.attr('x', margin.left)
			.attr('y', 0)
			.attr('width', chartWidth)
			.attr('height', barHeight)
			.attr('fill', '#f1f5f9')
			.attr('rx', 4);

		// Value bars
		barGroups
			.append('rect')
			.attr('x', margin.left)
			.attr('y', 0)
			.attr('width', (d) => xScale(d.value))
			.attr('height', barHeight)
			.attr('fill', (d, i) => d.color || defaultColors[i % defaultColors.length])
			.attr('rx', 4);

		// Value text on right
		barGroups
			.append('text')
			.attr('x', (d) => margin.left + xScale(d.value) + 8)
			.attr('y', barHeight / 2)
			.attr('dominant-baseline', 'middle')
			.attr('font-size', '13px')
			.attr('font-weight', '600')
			.attr('fill', '#1e293b')
			.text((d) => d.value.toLocaleString());
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
