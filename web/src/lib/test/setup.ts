import '@testing-library/svelte/vitest';
import { vi } from 'vitest';

// Mock ResizeObserver for D3/chart tests
// Using a class-based mock to avoid flaky "is not a constructor" errors
class MockResizeObserver {
	observe = vi.fn();
	unobserve = vi.fn();
	disconnect = vi.fn();
}
globalThis.ResizeObserver = MockResizeObserver;

// Mock SVG getBBox for D3 tests
if (typeof SVGElement !== 'undefined') {
	(SVGElement.prototype as SVGElement & { getBBox: () => DOMRect }).getBBox = vi.fn().mockReturnValue({
		x: 0,
		y: 0,
		width: 100,
		height: 100
	});
}

// Mock SVG transform.baseVal for D3 transition animations
// jsdom doesn't fully support SVG transforms, causing "Cannot read properties of undefined (reading 'baseVal')"
// when D3 tries to animate transform attributes
if (typeof SVGGraphicsElement !== 'undefined') {
	Object.defineProperty(SVGGraphicsElement.prototype, 'transform', {
		get() {
			return {
				baseVal: {
					numberOfItems: 0,
					consolidate: () => null,
					getItem: () => ({
						type: 1,
						matrix: { a: 1, b: 0, c: 0, d: 1, e: 0, f: 0 }
					})
				}
			};
		},
		configurable: true
	});
}
