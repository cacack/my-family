import { describe, it, expect } from 'vitest';
import * as d3 from 'd3';
import {
	FIT_SCALE_RANGE,
	MIN_CARD_FONT_PX,
	centreOnX,
	fitTransform,
	nameLines,
	offscreenX,
	scaleBounds,
	scaleAbout,
	touchScrollsPage,
	truncateLabel,
	wrapLabel
} from './chart';

describe('fitTransform', () => {
	const extent: [number, number] = [0.1, 4];
	const [minFit] = FIT_SCALE_RANGE;

	it('fits and centres content that needs no clamping', () => {
		const t = fitTransform({ x: 0, y: 0, width: 850, height: 100 }, 950, 600, extent);
		expect(t.k).toBeCloseTo(0.95);
		const [cx, cy] = t.apply([425, 50]);
		expect(cx).toBeCloseTo(475);
		expect(cy).toBeCloseTo(300);
	});

	it('never zooms a tiny tree above 1x', () => {
		const t = fitTransform({ x: -70, y: -40, width: 140, height: 80 }, 1280, 700, extent);
		expect(t.k).toBe(1);
		expect(t.apply([0, 0])).toEqual([640, 350]);
	});

	it('keeps card text at least 10px on a huge tree', () => {
		const t = fitTransform({ x: 0, y: 0, width: 30000, height: 3000 }, 375, 400, extent);
		expect(t.k).toBe(minFit);
		expect(MIN_CARD_FONT_PX * t.k).toBeGreaterThanOrEqual(10);
	});

	it('centres an overflowing axis on the focus', () => {
		const focus = { x: 9000, y: 0 };
		const t = fitTransform({ x: 0, y: 0, width: 30000, height: 300 }, 1000, 600, extent, focus);
		const [fx, fy] = t.apply([focus.x, focus.y]);
		expect(fx).toBeCloseTo(500);
		// The height fits, so the vertical axis stays centred on the bounds
		expect(t.apply([0, 150])[1]).toBeCloseTo(300);
		expect(fy).toBeCloseTo(300 - 150 * minFit);
	});

	it('keeps the content edge at the viewport edge when the focus is near it', () => {
		// A descendancy root at the top of a tall, wide tree
		const focus = { x: 5000, y: 40 };
		const t = fitTransform({ x: 0, y: 0, width: 10000, height: 4000 }, 375, 600, extent, focus);
		expect(t.apply([focus.x, focus.y])[0]).toBeCloseTo(187.5);
		// The top edge sits just inside the viewport instead of the root sitting mid-screen
		expect(t.apply([0, 0])[1]).toBeCloseTo(16);
	});
});

describe('offscreenX', () => {
	it('lists cards wholly off each side, nearest first', () => {
		const t = d3.zoomIdentity.translate(500, 0);
		// Viewport 0..1000 shows chart x -500..500; cards are 100 wide
		const xs = [-2000, -560, -540, 0, 540, 560, 3000];
		expect(offscreenX(xs, 50, t, 1000)).toEqual({ left: [-560, -2000], right: [560, 3000] });
	});

	it('honours the zoom scale', () => {
		const t = d3.zoomIdentity.translate(500, 0).scale(0.5);
		expect(offscreenX([-1000, 1000, 1200], 50, t, 1000)).toEqual({ left: [], right: [1200] });
	});
});

describe('centreOnX', () => {
	it('pans a point to mid-viewport, keeping the scale and vertical offset', () => {
		const t = centreOnX(d3.zoomIdentity.translate(10, 40).scale(2), 300, 1000);
		expect(t.apply([300, 0])).toEqual([500, 40]);
		expect(t.k).toBe(2);
	});
});

describe('scaleBounds', () => {
	it('scales position and size', () => {
		expect(scaleBounds({ x: -10, y: 4, width: 100, height: 50 }, 1.5)).toEqual({ x: -15, y: 6, width: 150, height: 75 });
	});
});

describe('touchScrollsPage', () => {
	function touchMove(target: Element, touches: number): boolean {
		const event = new Event('touchmove', { bubbles: true });
		Object.defineProperty(event, 'touches', { value: { length: touches } });
		let reached = false;
		target.addEventListener('touchmove', () => (reached = true));
		target.dispatchEvent(event);
		return reached;
	}

	it('lets the browser scroll vertically and hides one-finger moves from zoom', () => {
		const svg = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
		touchScrollsPage(svg);
		expect(svg.style.touchAction).toBe('pan-y');
		expect(touchMove(svg, 1)).toBe(false);
	});

	it('passes two-finger moves through to zoom', () => {
		const svg = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
		touchScrollsPage(svg);
		expect(touchMove(svg, 2)).toBe(true);
	});
});

describe('scaleAbout', () => {
	it('chains steps and keeps the centre fixed', () => {
		let t = d3.zoomIdentity.translate(10, 20).scale(1);
		const centre: [number, number] = [400, 300];
		const before = t.invert(centre);
		for (let i = 0; i < 3; i++) t = scaleAbout(t, 1.2, centre, [0.1, 4]);
		expect(t.k).toBeCloseTo(1.728);
		const after = t.invert(centre);
		expect(after[0]).toBeCloseTo(before[0]);
		expect(after[1]).toBeCloseTo(before[1]);
	});

	it('clamps to the extent', () => {
		expect(scaleAbout(d3.zoomIdentity.scale(3.9), 1.2, [0, 0], [0.1, 4]).k).toBe(4);
	});
});

describe('truncateLabel', () => {
	it('leaves short text alone', () => {
		expect(truncateLabel('Victoria', 16)).toBe('Victoria');
	});

	it('cuts at a word boundary without a space before the ellipsis', () => {
		expect(truncateLabel('Beatrice Mary Victoria', 16)).toBe('Beatrice Mary…');
	});

	it('cuts a long single word', () => {
		expect(truncateLabel('Alexander-Christopher', 16)).toBe('Alexander-Chris…');
	});
});

describe('nameLines', () => {
	it.each([
		[{ given_name: 'Jane', surname: 'Smith' }, ['Jane', 'Smith']],
		[{ given_name: 'Jane', surname: '' }, ['Jane', '']],
		[{ given_name: '', surname: 'Smith' }, ['', 'Smith']],
		[{ given_name: ' ', surname: undefined }, ['Unknown', '']],
		[{}, ['Unknown', '']]
	])('%o -> %o', (person, lines) => {
		expect(nameLines(person)).toEqual(lines);
	});
});

describe('wrapLabel', () => {
	it('splits a name over two lines', () => {
		expect(wrapLabel('Louise Margaret of_Prussia', 18)).toEqual(['Louise Margaret', 'of_Prussia']);
	});

	it('keeps a short name on one line', () => {
		expect(wrapLabel('Marie Alexandrovna', 18)).toEqual(['Marie Alexandrovna', '']);
	});
});
