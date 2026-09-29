import { describe, it, expect } from 'vitest';
import type { MergeBlocker } from '$lib/api/client';
import {
	blockedEntityIds,
	blockerFix,
	blockerFixLabel,
	describeBlocker
} from './mergeBlockers';

function blocker(overrides: Partial<MergeBlocker> = {}): MergeBlocker {
	return {
		stream_id: 'stream',
		entity_type: 'person',
		entity_name: 'Ada Lovelace',
		referenced_id: 'ref',
		referenced_type: 'evidence_analysis',
		referenced_name: 'Birth: born 1815',
		kind: 'missing_person',
		suggested_resolution: 'leave_out',
		message: 'raw',
		...overrides
	};
}

describe('describeBlocker', () => {
	const cases: Array<[MergeBlocker['kind'], string]> = [
		['missing_person', 'names evidence analysis "Birth: born 1815", who will not exist on the mainline.'],
		['missing_source', 'cites evidence analysis "Birth: born 1815", which will not exist on the mainline.'],
		['source_delete_orphans_citation', `Deleting Person "Ada Lovelace" would also delete the mainline's`],
		['missing_media_owner', 'is attached to evidence analysis'],
		['owner_delete_orphans_media', 'which this branch never saw.'],
		['missing_gps_artifact', 'Person "Ada Lovelace" edits research the mainline no longer has.'],
		['missing_gps_subject', 'is about evidence analysis "Birth: born 1815"'],
		['subject_delete_orphans_gps', 'added or changed after the fork.'],
		[
			'person_merge_conflicts_main',
			'Merging evidence analysis "Birth: born 1815" into Person "Ada Lovelace" no longer fits the mainline'
		]
	];
	it.each(cases)('%s says what breaks, by name', (kind, fragment) => {
		expect(describeBlocker(blocker({ kind }))).toContain(fragment);
	});

	it('has a fallback for a kind this client does not know', () => {
		const unknown = blocker({ kind: 'future_kind' as MergeBlocker['kind'] });
		expect(describeBlocker(unknown)).toContain('references evidence analysis');
	});

	it('names unnamed entities by type', () => {
		const text = describeBlocker(blocker({ entity_name: '', referenced_name: '' }));
		expect(text).toBe('Person "Unnamed person" names evidence analysis "Unnamed evidence analysis", who will not exist on the mainline.');
	});
});

describe('blocker fixes', () => {
	it('leaves the blocking entity out', () => {
		const b = blocker();
		expect(blockerFixLabel(b)).toBe('Also leave out Ada Lovelace');
		expect(blockerFix(b)).toEqual({ streamId: 'stream', resolution: 'main' });
	});

	it('includes the referenced entity', () => {
		const b = blocker({ suggested_resolution: 'include_referenced' });
		expect(blockerFixLabel(b)).toBe('Include Birth: born 1815');
		expect(blockerFix(b)).toEqual({ streamId: 'ref', resolution: 'branch' });
	});

	it('collects every involved entity for highlighting', () => {
		expect([...blockedEntityIds([blocker(), blocker({ stream_id: 'other' })])].sort()).toEqual([
			'other',
			'ref',
			'stream'
		]);
	});
});
