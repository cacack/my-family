import { describe, it, expect, beforeAll } from 'vitest';
import { render } from '@testing-library/svelte';
import QualityChart from './QualityChart.svelte';

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
});
