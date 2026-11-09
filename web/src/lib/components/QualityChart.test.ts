import { describe, it, expect, beforeAll, afterEach } from 'vitest';
import { render, cleanup } from '@testing-library/svelte';
import QualityChart from './QualityChart.svelte';
import { setFontSize } from '$lib/stores/accessibilitySettings.svelte';

beforeAll(() => {
	// jsdom lacks ResizeObserver.
	globalThis.ResizeObserver ??= class {
		observe() {}
		unobserve() {}
		disconnect() {}
	} as unknown as typeof ResizeObserver;
});

const longLabel = 'Missing death date (likely deceased)';
const data = [
	{ label: 'Missing birth date', value: 1200 },
	{ label: longLabel, value: 87 }
];

describe('QualityChart', () => {
	it('renders every label in full', () => {
		const { container } = render(QualityChart, { props: { data } });
		const labels = [...container.querySelectorAll('text.bar-label')].map((t) => t.textContent);
		expect(labels).toEqual(['Missing birth date', longLabel]);
	});

	it('gives each bar its full label and count as a tooltip', () => {
		const { container } = render(QualityChart, { props: { data } });
		const titles = [...container.querySelectorAll('.bar-group > title')].map(
			(t) => t.textContent
		);
		expect(titles).toEqual([
			`Missing birth date: ${(1200).toLocaleString()}`,
			`${longLabel}: 87`
		]);
	});

	it('truncates a label that does not fit, keeping the full text in the tooltip', () => {
		// Fake measurement: 10px per character on a 200px-wide chart.
		const proto = SVGElement.prototype as unknown as {
			getComputedTextLength?: () => number;
		};
		proto.getComputedTextLength = function (this: SVGElement) {
			return (this.textContent ?? '').length * 10;
		};
		const width = Object.getOwnPropertyDescriptor(HTMLElement.prototype, 'clientWidth');
		Object.defineProperty(HTMLElement.prototype, 'clientWidth', {
			configurable: true,
			get: () => 200
		});
		try {
			const { container } = render(QualityChart, { props: { data } });
			const label = container.querySelectorAll('text.bar-label')[1];
			expect(label.textContent!.endsWith('…')).toBe(true);
			expect(label.textContent!.length * 10).toBeLessThanOrEqual(200);
			expect(container.querySelectorAll('.bar-group > title')[1].textContent).toBe(
				`${longLabel}: 87`
			);
		} finally {
			delete proto.getComputedTextLength;
			if (width) Object.defineProperty(HTMLElement.prototype, 'clientWidth', width);
		}
	});

	it('renders nothing for empty data', () => {
		const { container } = render(QualityChart, { props: { data: [] } });
		expect(container.querySelector('svg')).toBeNull();
	});
	describe('font-size setting (#1103)', () => {
		afterEach(() => setFontSize('normal'));

		const measure = (container: Element) => ({
			label: parseFloat(container.querySelector('.bar-label')?.getAttribute('font-size') ?? ''),
			value: parseFloat(container.querySelector('.bar-value')?.getAttribute('font-size') ?? ''),
			height: Number(container.querySelector('svg')?.getAttribute('height'))
		});

		it('draws labels, values and rows bigger at Larger', () => {
			const normal = measure(render(QualityChart, { props: { data } }).container);
			cleanup();
			setFontSize('larger');
			const larger = measure(render(QualityChart, { props: { data } }).container);
			expect(larger.label).toBeGreaterThanOrEqual(normal.label * 1.25);
			expect(larger.value).toBeGreaterThanOrEqual(normal.value * 1.25);
			// Rows grow with the text, so labels never overlap the bar below
			expect(larger.height).toBeGreaterThanOrEqual(normal.height * 1.25);
		});

		it('redraws when the setting changes', async () => {
			const { container } = render(QualityChart, { props: { data } });
			const before = measure(container).label;
			setFontSize('large');
			await new Promise((resolve) => setTimeout(resolve, 0));
			expect(measure(container).label).toBeCloseTo(before * 1.25);
		});
	});
});
