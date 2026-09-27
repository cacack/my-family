import { describe, it, expect } from 'vitest';
import {
	CHANGE_ENTITY_TYPES,
	changeEntryLink,
	entityTypeLabel,
	unnamedEntityLabel
} from './changeEntries';

const ID = '11111111-1111-1111-1111-111111111111';
const PARENT = '22222222-2222-2222-2222-222222222222';

describe('changeEntryLink', () => {
	it.each([
		['person', `/persons/${ID}`],
		['family', `/families/${ID}`],
		['source', `/sources/${ID}`],
		['repository', `/repositories/${ID}`],
		['evidence_analysis', `/evidence/analyses/${ID}`],
		['evidence_conflict', `/evidence/conflicts/${ID}`],
		['research_log', `/evidence/research-logs/${ID}`],
		['proof_summary', `/evidence/proof-summaries/${ID}`]
	] as const)('links a %s to its own page', (entityType, href) => {
		expect(
			changeEntryLink({
				entity_type: entityType,
				entity_id: ID,
				action: 'updated'
			})
		).toBe(href);
	});

	it('links a sub-record to the page that presents it, even once deleted', () => {
		const lifeEvent = {
			entity_type: 'life_event' as const,
			entity_id: ID,
			parent_entity_type: 'family',
			parent_entity_id: PARENT
		};
		expect(changeEntryLink({ ...lifeEvent, action: 'updated' })).toBe(`/families/${PARENT}`);
		expect(changeEntryLink({ ...lifeEvent, action: 'deleted' })).toBe(`/families/${PARENT}`);
	});

	it('does not link a deleted entity or one without any page', () => {
		expect(
			changeEntryLink({
				entity_type: 'person',
				entity_id: ID,
				action: 'deleted'
			})
		).toBeNull();
		expect(
			changeEntryLink({
				entity_type: 'note',
				entity_id: ID,
				action: 'created'
			})
		).toBeNull();
		expect(
			changeEntryLink({
				entity_type: 'submitter',
				entity_id: ID,
				action: 'updated'
			})
		).toBeNull();
	});
});

describe('entityTypeLabel', () => {
	it('labels every entity type', () => {
		expect(CHANGE_ENTITY_TYPES).toHaveLength(16);
		for (const type of CHANGE_ENTITY_TYPES) {
			expect(entityTypeLabel(type)).not.toBe(type);
		}
		expect(entityTypeLabel('lds_ordinance')).toBe('LDS ordinance');
	});

	it('falls back to a readable form of an unrecognised type', () => {
		expect(entityTypeLabel('some_thing')).toBe('Some thing');
		expect(entityTypeLabel('')).toBe('Entity');
	});
});

describe('unnamedEntityLabel', () => {
	it('names the entity type rather than saying "entity"', () => {
		expect(unnamedEntityLabel('evidence_analysis')).toBe('Unnamed evidence analysis');
		expect(unnamedEntityLabel('life_event')).toBe('Unnamed life event');
		expect(unnamedEntityLabel('lds_ordinance')).toBe('Unnamed LDS ordinance');
		expect(unnamedEntityLabel('')).toBe('Unnamed entity');
	});
});
