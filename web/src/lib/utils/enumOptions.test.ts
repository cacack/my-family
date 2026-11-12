import { describe, it, expect } from 'vitest';
import { load } from 'js-yaml';
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

const { schemas } = (
	load(spec) as { components: { schemas: Record<string, { enum?: unknown[] }> } }
).components;

/** The values of the named enum schema in openapi.yaml. */
function specEnum(name: string): string[] {
	const values = schemas[name]?.enum;
	expect(values, `enum schema ${name}`).toBeDefined();
	return (values ?? []).map(String);
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
