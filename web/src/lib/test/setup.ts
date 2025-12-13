import '@testing-library/svelte/vitest';
import { vi } from 'vitest';

// Mock ResizeObserver for D3/chart tests
global.ResizeObserver = vi.fn().mockImplementation(() => ({
	observe: vi.fn(),
	unobserve: vi.fn(),
	disconnect: vi.fn()
}));

// Mock SVG getBBox for D3 tests
if (typeof SVGElement !== 'undefined') {
	SVGElement.prototype.getBBox = vi.fn().mockReturnValue({
		x: 0,
		y: 0,
		width: 100,
		height: 100
	});
}
