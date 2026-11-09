/**
 * Fakes SVG text measurement, which jsdom lacks: a capital letter is 0.75em of
 * the element's `font-size` attribute (16px when unset) and any other
 * character 0.6em, so a name's width depends on its glyphs, not just its
 * length. Returns a function that removes the fake.
 */
export function mockTextLength(): () => void {
	const proto = SVGElement.prototype as unknown as { getComputedTextLength?: () => number };
	proto.getComputedTextLength = function (this: SVGElement) {
		const size = parseFloat(this.getAttribute('font-size') ?? '') || 16;
		const ems = [...(this.textContent ?? '')].reduce((w, c) => w + (/[A-Z]/.test(c) ? 0.75 : 0.6), 0);
		return ems * size;
	};
	return () => {
		delete proto.getComputedTextLength;
	};
}
