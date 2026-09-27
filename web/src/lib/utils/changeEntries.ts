/**
 * Shared rendering of change-log entries (`ChangeEntry`) for every surface
 * that lists them: the global and entity history, branch compare / merge
 * review, and snapshot compare (#739, #827).
 *
 * The server maps every event type to one of the entity types below or leaves
 * it out (docs/HISTORY-EVENT-TYPES.md), so these tables are exhaustive: the
 * `Record` over the generated enum makes a new server-side type a type error
 * here until it has a label.
 */
import type { components } from '$lib/api/types.generated';

type Entry = components['schemas']['ChangeEntry'];
export type ChangeEntityType = Entry['entity_type'];

/** Human-readable label per entity type, in the order filters offer them. */
export const ENTITY_TYPE_LABELS: Record<ChangeEntityType, string> = {
	person: 'Person',
	family: 'Family',
	source: 'Source',
	citation: 'Citation',
	life_event: 'Life event',
	attribute: 'Attribute',
	association: 'Association',
	note: 'Note',
	media: 'Media',
	repository: 'Repository',
	submitter: 'Submitter',
	lds_ordinance: 'LDS ordinance',
	evidence_analysis: 'Evidence analysis',
	evidence_conflict: 'Evidence conflict',
	research_log: 'Research log',
	proof_summary: 'Proof summary'
};

/** Every entity type, in label order. */
export const CHANGE_ENTITY_TYPES = Object.keys(ENTITY_TYPE_LABELS) as ChangeEntityType[];

/**
 * The label for an entity type. Also accepts the free-form strings merge
 * conflicts carry, falling back to a readable form of the raw value.
 */
export function entityTypeLabel(entityType: string): string {
	const known = ENTITY_TYPE_LABELS[entityType as ChangeEntityType];
	if (known) return known;
	const words = entityType.replace(/_/g, ' ').trim();
	return words ? words.charAt(0).toUpperCase() + words.slice(1) : 'Entity';
}

/**
 * What to show for an entity whose display name could not be resolved - its
 * type, never a bare "Unnamed entity" (#828).
 */
export function unnamedEntityLabel(entityType: string): string {
	const label = entityTypeLabel(entityType);
	// Keep an acronym ("LDS ordinance") as it is; lower-case an ordinary word.
	const acronym = /^[A-Z]{2,}\b/.test(label);
	return `Unnamed ${acronym ? label : label.charAt(0).toLowerCase() + label.slice(1)}`;
}

/** The page of an entity that has one of its own. */
function pageOf(entityType: string, id: string): string | null {
	switch (entityType) {
		case 'person':
			return `/persons/${id}`;
		case 'family':
			return `/families/${id}`;
		case 'source':
			return `/sources/${id}`;
		case 'repository':
			return `/repositories/${id}`;
		case 'evidence_analysis':
			return `/evidence/analyses/${id}`;
		case 'evidence_conflict':
			return `/evidence/conflicts/${id}`;
		case 'research_log':
			return `/evidence/research-logs/${id}`;
		case 'proof_summary':
			return `/evidence/proof-summaries/${id}`;
		default:
			return null;
	}
}

type LinkableEntry = Pick<Entry, 'entity_type' | 'entity_id' | 'action'> & {
	parent_entity_type?: string;
	parent_entity_id?: string;
};

/**
 * Where an entry links: the entity's own page, or for a record without one (a
 * life event, citation, media item, ...) the page that presents it. A deleted
 * entity has no page left, but a deleted sub-record's parent usually still
 * does. Null when there is nowhere to go (a note, a submitter).
 */
export function changeEntryLink(entry: LinkableEntry): string | null {
	if (entry.action !== 'deleted') {
		const own = pageOf(entry.entity_type, entry.entity_id);
		if (own) return own;
	}
	if (entry.parent_entity_type && entry.parent_entity_id) {
		return pageOf(entry.parent_entity_type, entry.parent_entity_id);
	}
	return null;
}
