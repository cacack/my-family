import { describe, it, expect } from 'vitest';
import type { Branch, MergePendingEntity } from '$lib/api/client';
import {
	branchMergeSummary,
	decisionsNeeded,
	UNREADABLE_MERGE_SUMMARY,
	incompleteMergeSummary,
	isIncompleteMerge,
	isUnreadableMerge,
	pendingAsResolvable,
	pendingEntities,
	pendingReasonDetail,
	pendingReasonLabel,
	pendingSoleOptionReason
} from './mergeState';

const REASONS: MergePendingEntity['reason'][] = [
	'ready',
	'main_changed',
	'main_removed',
	'no_plan',
	'breaks_reference',
	'needs_repair'
];

function entity(reason: MergePendingEntity['reason'], name = 'Ada Lovelace'): MergePendingEntity {
	return {
		stream_id: `stream-${reason}`,
		entity_type: 'person',
		entity_name: name,
		reason,
		needs_resolution: reason !== 'ready' && reason !== 'needs_repair',
		supported_resolutions:
			reason === 'ready' || reason === 'needs_repair' ? [] : reason === 'main_removed' || reason === 'breaks_reference' ? ['main'] : ['branch', 'main']
	};
}

describe('mergeState', () => {
	const merged: Branch = {
		id: 'b',
		name: 'Line',
		base_position: 1,
		status: 'merged',
		created_at: '2026-01-01T00:00:00Z'
	};

	it('recognises only a merged branch whose merge is incomplete', () => {
		expect(isIncompleteMerge({ ...merged, merge_state: 'incomplete' })).toBe(true);
		expect(isIncompleteMerge({ ...merged, merge_state: 'complete' })).toBe(false);
		expect(isIncompleteMerge(merged)).toBe(false);
		expect(isIncompleteMerge({ ...merged, status: 'active', merge_state: 'incomplete' })).toBe(false);
		expect(isIncompleteMerge(null)).toBe(false);
		// A state the server could not read is flagged too, so Finish merge can say what is wrong.
		expect(isIncompleteMerge({ ...merged, merge_state: 'unknown' })).toBe(true);
		expect(isUnreadableMerge({ ...merged, merge_state: 'unknown' })).toBe(true);
		expect(isUnreadableMerge({ ...merged, merge_state: 'incomplete' })).toBe(false);
	});

	it('lists the pending entities, or none', () => {
		expect(pendingEntities(undefined)).toEqual([]);
		expect(pendingEntities({ merge_pending: [entity('ready')] })).toHaveLength(1);
	});

	it('sums up what is left in words', () => {
		expect(incompleteMergeSummary([entity('ready')])).toBe(
			'1 entity has not reached the mainline yet. Finishing the merge takes care of it.'
		);
		expect(incompleteMergeSummary([entity('ready'), entity('ready')])).toMatch(/takes care of them\.$/);
		// A replayed change whose data did not follow is not "not reached the mainline".
		expect(incompleteMergeSummary([entity('needs_repair')])).toBe(
			"1 entity reached the mainline's history but not its data. Finishing the merge takes care of it."
		);
		expect(incompleteMergeSummary([entity('ready'), entity('needs_repair'), entity('main_changed')])).toBe(
			"2 entities have not reached the mainline yet, and 1 entity reached the mainline's history but not its data, and one needs your decision first."
		);
		expect(incompleteMergeSummary([])).toMatch(/checks the mainline/);
		expect(incompleteMergeSummary([entity('ready'), entity('main_changed')])).toBe(
			'2 entities have not reached the mainline yet, and one needs your decision first.'
		);
		expect(incompleteMergeSummary([entity('no_plan'), entity('main_changed')])).toMatch(/2 need your decision/);
		expect(decisionsNeeded([entity('ready'), entity('no_plan')])).toBe(1);
	});

	it('sums up a branch, including one whose state could not be read', () => {
		expect(branchMergeSummary({ ...merged, merge_state: 'unknown' })).toBe(UNREADABLE_MERGE_SUMMARY);
		expect(branchMergeSummary({ ...merged, merge_state: 'incomplete', merge_pending: [entity('ready')] })).toMatch(
			/1 entity has not reached/
		);
	});

	it('names every reason, and never by its raw value', () => {
		for (const reason of REASONS) {
			expect(pendingReasonLabel(reason)).not.toBe(reason);
			expect(pendingReasonDetail(reason).length).toBeGreaterThan(20);
		}
		expect(pendingSoleOptionReason('main_removed')).toMatch(/cannot bring a deleted entity back/);
		expect(pendingSoleOptionReason('breaks_reference')).toMatch(/refused/);
		expect(pendingSoleOptionReason('no_plan')).toMatch(/Only one resolution/);
	});

	it('shapes a pending entity for the resolver', () => {
		const resolvable = pendingAsResolvable(entity('main_removed'));
		expect(resolvable.stream_id).toBe('stream-main_removed');
		expect(resolvable.kind).toBeUndefined();
		expect(resolvable.supported_resolutions).toEqual(['main']);
		expect(resolvable.detail).toBe(pendingReasonDetail('main_removed'));
	});
});
