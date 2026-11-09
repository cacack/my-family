/**
 * Shared helpers for the D3 tree charts and the map: zoom fitting, zoom steps,
 * resize handling, motion preferences and label truncation.
 */
import * as d3 from 'd3';
import { formatPersonName } from '#lib/api/client.js';
import { getReducedMotion } from '#lib/stores/accessibilitySettings.svelte.js';

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

/** Text on screen should be at least this big, in CSS pixels. */
const MIN_TEXT_PX = 10;
/** The smallest font drawn on a chart, in chart units (the chart component tests check this). */
export const MIN_CARD_FONT_PX = 11;
/**
 * The initial fit stays within this range: never above 1x (a lone person would
 * otherwise open as a giant card), and never so small that card text drops
 * below MIN_TEXT_PX. A tree too big to fit at the minimum overflows instead.
 */
export const FIT_SCALE_RANGE: ScaleExtent = [MIN_TEXT_PX / MIN_CARD_FONT_PX, 1];
/** Gap kept between an overflowing tree's edge and the viewport edge. */
const FIT_EDGE_MARGIN = 16;

/**
 * Translation along one axis: centre the content when it fits, otherwise centre
 * on `focus` but never leave a gap past the content's near edge.
 */
function fitAxis(start: number, size: number, view: number, scale: number, focus?: number): number {
	if (focus === undefined || size * scale <= view) return view / 2 - scale * (start + size / 2);
	const centred = view / 2 - scale * focus;
	const min = view - FIT_EDGE_MARGIN - scale * (start + size);
	const max = FIT_EDGE_MARGIN - scale * start;
	return Math.min(max, Math.max(min, centred));
}

/**
 * The transform that fits `bounds` into a `width` x `height` viewport (with a
 * margin), clamped to FIT_SCALE_RANGE and the zoom extent. When the content
 * still overflows an axis, that axis is centred on `focus` (the root person)
 * and pushed back to the content's edge, so the root and its nearest relatives
 * open readable on any tree size.
 */
export function fitTransform(
	bounds: Bounds,
	width: number,
	height: number,
	extent: ScaleExtent,
	focus?: { x: number; y: number }
): d3.ZoomTransform {
	const fit = 0.85 / Math.max(bounds.width / width, bounds.height / height);
	const scale = clamp(clamp(fit, FIT_SCALE_RANGE), extent);
	const x = fitAxis(bounds.x, bounds.width, width, scale, focus?.x);
	const y = fitAxis(bounds.y, bounds.height, height, scale, focus?.y);
	return d3.zoomIdentity.translate(x, y).scale(scale);
}

/** Bounds and points in layout units, scaled to chart units (see `getFontScale`). */
export function scaleBounds(bounds: Bounds, k: number): Bounds {
	return { x: bounds.x * k, y: bounds.y * k, width: bounds.width * k, height: bounds.height * k };
}

/**
 * The cards (centre x and half-width, in chart units) lying wholly left or
 * right of a `width`-wide viewport under `t`, nearest first on each side. The
 * charts use it to point at first-generation relatives a first view can't
 * reach at a readable zoom (#1098).
 */
export function offscreenX(
	xs: number[],
	halfWidth: number,
	t: d3.ZoomTransform,
	width: number
): { left: number[]; right: number[] } {
	const left = xs.filter((x) => t.applyX(x + halfWidth) < 0).sort((a, b) => b - a);
	const right = xs.filter((x) => t.applyX(x - halfWidth) > width).sort((a, b) => a - b);
	return { left, right };
}

/** `t` panned horizontally so chart point `x` sits mid-viewport. */
export function centreOnX(t: d3.ZoomTransform, x: number, width: number): d3.ZoomTransform {
	return d3.zoomIdentity.translate(width / 2 - t.k * x, t.y).scale(t.k);
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
 * Leaves one-finger touch gestures to the browser so the page can scroll past
 * the chart; d3-zoom then pans and pinches only with two fingers. Call right
 * after applying the zoom behaviour, which sets `touch-action: none`.
 */
export function touchScrollsPage(svg: SVGSVGElement): void {
	svg.style.touchAction = 'pan-y';
	// Capture listeners on the target run before d3-zoom's, so single-finger
	// moves never reach it and the browser scrolls instead
	svg.addEventListener(
		'touchmove',
		(event) => {
			if (event.touches.length < 2) event.stopImmediatePropagation();
		},
		{ capture: true, passive: true }
	);
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

/** The drawn width of a string, in the chart units of the text it will sit in. */
export type MeasureText = (text: string) => number;

/** Room kept between card text and each side edge of its card, in chart units. */
export const CARD_TEXT_INSET = 6;

/**
 * `text` shortened with `truncateLabel` until it measures at most `maxWidth`,
 * so a long unbroken name never spills out of its card (#1213).
 */
export function fitLabel(text: string, maxWidth: number, measure: MeasureText): string {
	if (measure(text) <= maxWidth) return text;
	for (let max = text.length - 1; max > 1; max--) {
		const label = truncateLabel(text, max);
		if (measure(label) <= maxWidth) return label;
	}
	return '…';
}

/** Splits `text` over two lines no wider than `maxWidth`, truncating the rest. */
export function wrapLabel(text: string, maxWidth: number, measure: MeasureText): [string, string] {
	const words = text.trim().split(/\s+/).filter(Boolean);
	let first = '';
	while (words.length > 0 && measure(`${first} ${words[0]}`.trim()) <= maxWidth) {
		first = `${first} ${words.shift()}`.trim();
	}
	if (!first && words.length > 0) first = fitLabel(words.shift()!, maxWidth, measure);
	return [first, fitLabel(words.join(' '), maxWidth, measure)];
}

/**
 * Measures strings as drawn in `el` (its font, size and weight), in its own
 * chart units, so the zoom and the font-size scale don't change the result.
 * Each call replaces the element's text. Measures 0 where there is no layout
 * (jsdom), so nothing is cut.
 */
export function measureWith(el: SVGTextElement): MeasureText {
	return (text) => {
		el.textContent = text;
		return typeof el.getComputedTextLength === 'function' ? el.getComputedTextLength() : 0;
	};
}

/** Shortens the text of `el` with an ellipsis until it is at most `maxWidth` wide. */
export function fitText(el: SVGTextElement, maxWidth: number): void {
	el.textContent = fitLabel(el.textContent ?? '', maxWidth, measureWith(el));
}

/**
 * The given-name and surname lines of a chart card. A missing part is left
 * blank; a person with no name at all reads "Unknown" on the first line, as
 * `formatPersonName` does elsewhere (#932).
 */
export function nameLines(person: { given_name?: string; surname?: string }): [string, string] {
	const given = (person.given_name ?? '').trim();
	const surname = (person.surname ?? '').trim();
	if (!given && !surname) return [formatPersonName(person), ''];
	return [given, surname];
}
