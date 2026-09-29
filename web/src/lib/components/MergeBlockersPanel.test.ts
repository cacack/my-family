import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/svelte';
import MergeBlockersPanel from './MergeBlockersPanel.svelte';
import type { MergeBlocker } from '$lib/api/client';

const leaveOut: MergeBlocker = {
	stream_id: '11111111-1111-1111-1111-111111111111',
	entity_type: 'citation',
	entity_name: '1880 Census (Birth)',
	referenced_id: '22222222-2222-2222-2222-222222222222',
	referenced_type: 'source',
	referenced_name: '1880 Census',
	kind: 'missing_source',
	suggested_resolution: 'leave_out',
	message: 'raw'
};

const include: MergeBlocker = {
	stream_id: '33333333-3333-3333-3333-333333333333',
	entity_type: 'family',
	entity_name: 'Pat Newcomer',
	referenced_id: '44444444-4444-4444-4444-444444444444',
	referenced_type: 'person',
	referenced_name: 'Pat Newcomer',
	kind: 'missing_person',
	suggested_resolution: 'include_referenced',
	message: 'raw'
};

describe('MergeBlockersPanel', () => {
	it('lists every blocker by name, each with its one-click fix', async () => {
		const onfix = vi.fn();
		render(MergeBlockersPanel, { props: { blockers: [leaveOut, include], onfix } });

		expect(screen.getByRole('heading', { name: '2 merge blockers' })).toBeDefined();
		expect(
			screen.getByText('Citation "1880 Census (Birth)" cites source "1880 Census", which will not exist on the mainline.')
		).toBeDefined();
		expect(
			screen.getByText('Family "Pat Newcomer" names person "Pat Newcomer", who will not exist on the mainline.')
		).toBeDefined();

		await fireEvent.click(screen.getByRole('button', { name: 'Also leave out 1880 Census (Birth)' }));
		expect(onfix).toHaveBeenLastCalledWith(leaveOut);
		await fireEvent.click(screen.getByRole('button', { name: 'Include Pat Newcomer' }));
		expect(onfix).toHaveBeenLastCalledWith(include);
	});

	it('says one blocker in the singular and shows a re-check in flight', () => {
		render(MergeBlockersPanel, { props: { blockers: [leaveOut], checking: true, onfix: vi.fn() } });
		expect(screen.getByRole('heading', { name: '1 merge blocker' })).toBeDefined();
		expect(screen.getByRole('status').textContent).toContain('Re-checking');
	});

	it('disables the fixes while a merge is in flight', () => {
		render(MergeBlockersPanel, { props: { blockers: [leaveOut], disabled: true, onfix: vi.fn() } });
		expect(
			(screen.getByRole('button', { name: 'Also leave out 1880 Census (Birth)' }) as HTMLButtonElement)
				.disabled
		).toBe(true);
	});

	it('renders nothing without blockers, and a note when the check failed', () => {
		const { container, unmount } = render(MergeBlockersPanel, {
			props: { blockers: [], onfix: vi.fn() }
		});
		expect(container.textContent?.trim()).toBe('');
		unmount();

		render(MergeBlockersPanel, { props: { blockers: [], error: 'offline', onfix: vi.fn() } });
		expect(screen.getByRole('note').textContent).toContain('could not be checked (offline)');
	});
});
