import { describe, it, expect } from 'vitest';
import * as d3 from 'd3';
import { fitTransform, scaleAbout, truncateLabel, wrapLabel } from './chart';

describe('fitTransform', () => {
	const extent: [number, number] = [0.1, 4];

	it('fits and centres content that needs no clamping', () => {
		const t = fitTransform({ x: 0, y: 0, width: 1000, height: 500 }, 1000, 500, extent);
		expect(t.k).toBeCloseTo(0.85);
		expect(t.apply([500, 250])).toEqual([500, 250]);
	});

	it('clamps a tiny tree to the maximum zoom', () => {
		const t = fitTransform({ x: -70, y: -40, width: 140, height: 80 }, 1280, 700, extent);
		expect(t.k).toBe(4);
	});

	it('clamps a huge tree to the minimum zoom and centres the overflowing axis on the focus', () => {
		const focus = { x: 9000, y: 0 };
		const t = fitTransform({ x: 0, y: 0, width: 30000, height: 300 }, 1000, 600, extent, focus);
		expect(t.k).toBe(0.1);
		const [fx, fy] = t.apply([focus.x, focus.y]);
		expect(fx).toBeCloseTo(500);
		// The height fits, so the vertical axis stays centred on the bounds
		expect(t.apply([0, 150])[1]).toBeCloseTo(300);
		expect(fy).toBeCloseTo(285);
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

describe('wrapLabel', () => {
	it('splits a name over two lines', () => {
		expect(wrapLabel('Louise Margaret of_Prussia', 18)).toEqual(['Louise Margaret', 'of_Prussia']);
	});

	it('keeps a short name on one line', () => {
		expect(wrapLabel('Marie Alexandrovna', 18)).toEqual(['Marie Alexandrovna', '']);
	});
});
