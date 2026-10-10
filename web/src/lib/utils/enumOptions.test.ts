import { describe, it, expect } from 'vitest';
import spec from '../../../../internal/api/openapi.yaml?raw';
import {
	EVIDENCE_TYPE_OPTIONS,
	GENDER_OPTIONS,
	INFORMANT_TYPE_OPTIONS,
	NAME_TYPE_OPTIONS,
	RELATION_TYPE_OPTIONS,
	RESEARCH_STATUS_OPTIONS,
	SOURCE_QUALITY_OPTIONS,
	UPLOAD_MEDIA_TYPE_OPTIONS
} from './enumOptions';
import { FACT_TYPES } from './evidence';
import { SOURCE_TYPES } from './sourceTypes';

const unquote = (v: string) => v.replace(/#.*$/, '').trim().replace(/^(['"])(.*)\1$/, '$2');

/**
 * The values of the named enum schema in openapi.yaml, inline or block list.
 * A light line scan, not a YAML parser (none is a dependency): it fails loudly
 * on a shape it cannot read rather than returning an empty list.
 */
function specEnum(name: string): string[] {
	const start = spec.indexOf(`\n    ${name}:\n`);
	expect(start, `schema ${name}`).toBeGreaterThan(-1);
	const block = spec.slice(start + 1).split(/\n(?=    \S)/)[0];
	const inline = block.match(/enum:\s*\[([^\]]+)\]/);
	const found = inline
		? inline[1].split(',').map(unquote)
		: [...block.matchAll(/^ {8}- (.+)$/gm)].map((m) => unquote(m[1]));
	const parsed = found.filter(Boolean);
	expect(parsed.length, `could not read the enum values of ${name}`).toBeGreaterThan(0);
	return parsed;
}

const values = (options: readonly { value: string }[]) => options.map((o) => o.value);

// The types make an invalid option a svelte-check error; these make a missing
// one a test failure, so every value the API accepts is offered.
describe('enum option lists', () => {
	it.each([
		['Gender', values(GENDER_OPTIONS)],
		['RelationType', values(RELATION_TYPE_OPTIONS)],
		['NameType', values(NAME_TYPE_OPTIONS)],
		['ResearchStatus', values(RESEARCH_STATUS_OPTIONS)],
		['SourceQuality', values(SOURCE_QUALITY_OPTIONS)],
		['InformantType', values(INFORMANT_TYPE_OPTIONS)],
		['EvidenceType', values(EVIDENCE_TYPE_OPTIONS)],
		['SourceType', values(SOURCE_TYPES)],
		['FactType', [...FACT_TYPES]]
	])('offers every %s value', (name, offered) => {
		expect([...offered].sort()).toEqual(specEnum(name).sort());
	});

	it('offers only media types the API accepts', () => {
		const accepted = specEnum('MediaType');
		for (const value of values(UPLOAD_MEDIA_TYPE_OPTIONS)) {
			expect(accepted).toContain(value);
		}
	});
});
