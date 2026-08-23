import '@testing-library/svelte/vitest';
import { cleanup } from '@testing-library/svelte';
import { afterEach, vi } from 'vitest';

// Drain bits-ui's deferred body-scroll-lock cleanup before vitest tears jsdom down.
//
// Unmounting a scroll-locking bits-ui component (dropdown, dialog, popover) does not
// restore document.body immediately — it schedules the reset on a timer, 24ms by
// default (`actualDelay = delay === null ? 24 : delay` in
// node_modules/bits-ui/dist/internal/body-scroll-lock.svelte.js). If the last test in
// a file unmounts one and the environment is torn down inside that window, the
// callback dereferences a `document` that no longer exists. Vitest reports it as an
// unhandled error and exits non-zero even though every test passed — which is exactly
// how it presents: "31 passed / 446 passed / 1 error".
//
// Whether the teardown wins the race depends on machine speed, so this reproduces on
// CI while passing locally. Waiting out the timer here makes it deterministic.
//
// cleanup() is called explicitly first because afterEach hooks run in reverse
// registration order: the auto-cleanup registered by the import above would otherwise
// run *after* this hook, scheduling the timer we are trying to drain. cleanup() is
// idempotent, so the later auto-cleanup is a no-op.
const BITS_UI_SCROLL_LOCK_DELAY_MS = 24;

afterEach(async () => {
	cleanup();
	if (typeof document === 'undefined' || document.body.style.length === 0) {
		// Nothing locked the body, so there is no deferred reset pending.
		return;
	}
	await new Promise((resolve) => setTimeout(resolve, BITS_UI_SCROLL_LOCK_DELAY_MS + 8));
});

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
