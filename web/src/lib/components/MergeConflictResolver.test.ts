import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/svelte';
import MergeConflictResolver from './MergeConflictResolver.svelte';
import type { MergeConflict, MergeResolution } from '$lib/api/client';

const EDIT_EDIT: MergeConflict = {
	stream_id: '11111111-1111-1111-1111-111111111111',
	entity_type: 'person',
	entity_name: 'Ada Lovelace',
	kind: 'edit_edit',
	detail: 'Both sides changed surname to different values',
	fields: ['surname'],
	supported_resolutions: ['branch', 'main']
};

const CREATE_CREATE: MergeConflict = {
	stream_id: '22222222-2222-2222-2222-222222222222',
	entity_type: 'source',
	entity_name: '',
	kind: 'create_create',
	detail: 'Both sides created a source carrying xref @S12@',
	supported_resolutions: ['main']
};

const DELETE_EDIT: MergeConflict = {
	stream_id: '33333333-3333-3333-3333-333333333333',
	entity_type: 'family',
	entity_name: 'Lovelace / Byron',
	kind: 'delete_edit',
	detail: 'The mainline deleted this family while the branch went on changing it',
	supported_resolutions: ['main']
};

/** Radios rendered for one conflict, in DOM order. */
function radiosFor(container: HTMLElement, conflict: MergeConflict): HTMLElement[] {
	return Array.from(
		container.querySelectorAll<HTMLElement>(
			`[id^="conflict-${conflict.stream_id}-resolution-"][role="radio"]`
		)
	);
}

function renderResolver(
	conflicts: MergeConflict[],
	options: {
		resolutions?: Map<string, MergeResolution>;
		disabled?: boolean;
		onresolve?: (streamId: string, resolution: MergeResolution) => void;
	} = {}
) {
	const onresolve = options.onresolve ?? vi.fn();
	const result = render(MergeConflictResolver, {
		props: {
			conflicts,
			resolutions: options.resolutions ?? new Map<string, MergeResolution>(),
			onresolve,
			disabled: options.disabled ?? false
		}
	});
	return { ...result, onresolve };
}

describe('MergeConflictResolver', () => {
	beforeEach(() => {
		vi.clearAllMocks();
	});

	it('renders nothing when there are no conflicts', () => {
		const { container } = renderResolver([]);
		expect(container.querySelector('.conflict-list')).toBeNull();
	});

	it('offers both resolutions when the conflict supports both', () => {
		const { container } = renderResolver([EDIT_EDIT]);

		const radios = radiosFor(container, EDIT_EDIT);
		expect(radios.map((r) => r.dataset.value)).toEqual(['branch', 'main']);
		// Not labelled with the bare enum values.
		expect(screen.getByText("Take the branch's version")).toBeDefined();
		expect(screen.getByText("Keep the mainline's version")).toBeDefined();
	});

	// The rule that keeps the server from returning 400 invalid_resolution.
	it('offers only `main` for a create_create, and says why', () => {
		const { container } = renderResolver([CREATE_CREATE]);

		const radios = radiosFor(container, CREATE_CREATE);
		expect(radios).toHaveLength(1);
		expect(radios[0].dataset.value).toBe('main');
		expect(screen.queryByText("Take the branch's version")).toBeNull();
		expect(screen.getByText(/two different records/)).toBeDefined();
	});

	it('offers only `main` for a mainline-deleted delete_edit, and says why', () => {
		const { container } = renderResolver([DELETE_EDIT]);

		const radios = radiosFor(container, DELETE_EDIT);
		expect(radios).toHaveLength(1);
		expect(radios[0].dataset.value).toBe('main');
		expect(screen.getByText(/cannot bring a deleted entity back/)).toBeDefined();
	});

	it('calls onresolve with the conflict stream id and the chosen value', async () => {
		const { container, onresolve } = renderResolver([EDIT_EDIT, CREATE_CREATE]);

		const [, mainRadio] = radiosFor(container, EDIT_EDIT);
		await fireEvent.click(mainRadio);

		expect(onresolve).toHaveBeenCalledTimes(1);
		expect(onresolve).toHaveBeenCalledWith(EDIT_EDIT.stream_id, 'main');
	});

	it('renders an already-decided conflict as checked from the resolutions prop', () => {
		const { container } = renderResolver([EDIT_EDIT], {
			resolutions: new Map([[EDIT_EDIT.stream_id, 'branch' as MergeResolution]])
		});

		const [branchRadio, mainRadio] = radiosFor(container, EDIT_EDIT);
		expect(branchRadio.getAttribute('aria-checked')).toBe('true');
		expect(mainRadio.getAttribute('aria-checked')).toBe('false');
	});

	it('marks only the undecided conflicts', () => {
		renderResolver([EDIT_EDIT, CREATE_CREATE], {
			resolutions: new Map([[EDIT_EDIT.stream_id, 'branch' as MergeResolution]])
		});

		// Text, not colour alone - one badge for the one conflict still open.
		expect(screen.getAllByText('Needs a decision')).toHaveLength(1);
	});

	it('labels each radio group by its entity heading', () => {
		const { container } = renderResolver([EDIT_EDIT]);

		const group = container.querySelector('[role="radiogroup"]');
		const headingId = group?.getAttribute('aria-labelledby');
		expect(headingId).toBe(`conflict-${EDIT_EDIT.stream_id}-entity`);
		expect(container.querySelector(`#${headingId}`)?.textContent).toContain('Ada Lovelace');
	});

	it('disables every control while a merge is in flight', () => {
		const { container } = renderResolver([EDIT_EDIT, CREATE_CREATE], { disabled: true });

		const radios = [...radiosFor(container, EDIT_EDIT), ...radiosFor(container, CREATE_CREATE)];
		expect(radios).toHaveLength(3);
		for (const radio of radios) {
			expect(radio.hasAttribute('disabled')).toBe(true);
		}
	});

	it('falls back to a placeholder when the entity name is empty', () => {
		renderResolver([CREATE_CREATE]);

		expect(screen.getByText('Unnamed entity')).toBeDefined();
		expect(screen.queryByText('Ada Lovelace')).toBeNull();
	});

	it('lists the contested fields for an edit_edit only', () => {
		renderResolver([EDIT_EDIT, CREATE_CREATE]);

		expect(screen.getByText('Contested fields: surname')).toBeDefined();
		expect(screen.getAllByText(/Contested fields:/)).toHaveLength(1);
	});

	it('moves between options with the arrow keys', async () => {
		const { container } = renderResolver([EDIT_EDIT]);

		const [branchRadio, mainRadio] = radiosFor(container, EDIT_EDIT);
		branchRadio.focus();
		expect(document.activeElement).toBe(branchRadio);

		await fireEvent.keyDown(branchRadio, { key: 'ArrowDown' });
		expect(document.activeElement).toBe(mainRadio);

		await fireEvent.keyDown(mainRadio, { key: 'ArrowUp' });
		expect(document.activeElement).toBe(branchRadio);
	});
});
