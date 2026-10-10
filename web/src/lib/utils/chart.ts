/**
 * Shared helpers for the D3 tree charts and the map: zoom fitting, zoom steps,
 * resize handling, motion preferences and label truncation.
 */
import * as d3 from 'd3';
import { getReducedMotion } from '$lib/stores/accessibilitySettings.svelte';

export type ScaleExtent = [number, number];

interface Bounds {
	x: number;
	y: number;
	width: number;
	height: number;
}

/** Transition length in ms, or 0 when the user has asked for reduced motion. */
export function motionDuration(ms: number): number {
	return getReducedMotion() ? 0 : ms;
}

function clamp(value: number, [min, max]: ScaleExtent): number {
	return Math.min(max, Math.max(min, value));
}

/**
 * The transform that fits `bounds` into a `width` x `height` viewport (with a
 * margin), clamped to the zoom extent. When the clamped content still overflows
 * an axis, that axis is centred on `focus` (the root person) instead of the
 * bounding box, so the root stays visible on very large trees.
 */
export function fitTransform(
	bounds: Bounds,
	width: number,
	height: number,
	extent: ScaleExtent,
	focus?: { x: number; y: number }
): d3.ZoomTransform {
	const fit = 0.85 / Math.max(bounds.width / width, bounds.height / height);
	const scale = clamp(fit, extent);
	let cx = bounds.x + bounds.width / 2;
	let cy = bounds.y + bounds.height / 2;
	if (focus && bounds.width * scale > width) cx = focus.x;
	if (focus && bounds.height * scale > height) cy = focus.y;
	return d3.zoomIdentity.translate(width / 2 - scale * cx, height / 2 - scale * cy).scale(scale);
}

/** `from` scaled by `factor` about the viewport point `center`, clamped to the extent. */
export function scaleAbout(
	from: d3.ZoomTransform,
	factor: number,
	center: [number, number],
	extent: ScaleExtent
): d3.ZoomTransform {
	const k = clamp(from.k * factor, extent);
	const [px, py] = from.invert(center);
	return d3.zoomIdentity.translate(center[0] - px * k, center[1] - py * k).scale(k);
}

/**
 * Debounced resize handling for charts. `onWidthChange` runs only when the width
 * changes by more than a pixel; height-only changes (such as the mobile URL bar
 * showing or hiding) call `onHeightChange`, so the user's zoom and pan survive.
 * Returns a cleanup function.
 */
export function observeChartResize(
	el: HTMLElement,
	onWidthChange: () => void,
	onHeightChange: () => void = () => {},
	delay = 150
): () => void {
	let lastWidth = el.clientWidth;
	let lastHeight = el.clientHeight;
	let timer: ReturnType<typeof setTimeout> | null = null;
	const observer = new ResizeObserver(() => {
		if (timer) clearTimeout(timer);
		timer = setTimeout(() => {
			const width = el.clientWidth;
			const height = el.clientHeight;
			if (Math.abs(width - lastWidth) > 1) {
				onWidthChange();
			} else if (height !== lastHeight) {
				onHeightChange();
			}
			lastWidth = width;
			lastHeight = height;
		}, delay);
	});
	observer.observe(el);
	return () => {
		if (timer) clearTimeout(timer);
		observer.disconnect();
	};
}

/**
 * Shortens `text` to at most `max` characters, preferring a word boundary and
 * never leaving whitespace before the ellipsis.
 */
export function truncateLabel(text: string, max: number): string {
	if (text.length <= max) return text;
	const cut = text.slice(0, max - 1);
	const space = cut.lastIndexOf(' ');
	const head = space >= max / 2 ? cut.slice(0, space) : cut;
	return `${head.trimEnd()}…`;
}

/** Splits `text` over two lines of at most `max` characters, truncating the rest. */
export function wrapLabel(text: string, max: number): [string, string] {
	const words = text.trim().split(/\s+/).filter(Boolean);
	let first = '';
	while (words.length > 0 && `${first} ${words[0]}`.trim().length <= max) {
		first = `${first} ${words.shift()}`.trim();
	}
	if (!first && words.length > 0) first = truncateLabel(words.shift()!, max);
	return [first, truncateLabel(words.join(' '), max)];
}
