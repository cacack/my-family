import { describe, it, expect } from 'vitest';
import {
	CURRENT_STATE,
	parseComparisonEnd,
	snapshotCompareHref,
	snapshotCompareRangeHref,
	snapshotCompareToNowHref
} from './snapshots';

describe('snapshot comparison links', () => {
	it('builds the pair, to-now and fixed-range links', () => {
		expect(snapshotCompareHref('a', 'b')).toBe('/snapshots/compare?from=a&to=b');
		expect(snapshotCompareToNowHref('a')).toBe(`/snapshots/compare?from=a&to=${CURRENT_STATE}`);
		expect(snapshotCompareRangeHref('a', 131)).toBe(
			`/snapshots/compare?from=a&to=${CURRENT_STATE}&until=131`
		);
	});

	it('reads an end position, telling absent from malformed', () => {
		expect(parseComparisonEnd(null)).toBeNull();
		expect(parseComparisonEnd('0')).toBe(0);
		expect(parseComparisonEnd('131')).toBe(131);
		expect(parseComparisonEnd('')).toBeUndefined();
		expect(parseComparisonEnd('-1')).toBeUndefined();
		expect(parseComparisonEnd('1.5')).toBeUndefined();
		expect(parseComparisonEnd('soon')).toBeUndefined();
		expect(parseComparisonEnd('99999999999999999999')).toBeUndefined();
	});
});
