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
