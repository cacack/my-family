import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, within } from '@testing-library/svelte';
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
		onresolveall?: (decisions: Array<[string, MergeResolution]>) => void;
		rationales?: Map<string, string>;
		onrationale?: (streamId: string, rationale: string) => void;
	} = {}
) {
	const onresolve = options.onresolve ?? vi.fn();
	const result = render(MergeConflictResolver, {
		props: {
			conflicts,
			resolutions: options.resolutions ?? new Map<string, MergeResolution>(),
			onresolve,
			onresolveall: options.onresolveall,
			rationales: options.rationales,
			onrationale: options.onrationale,
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

		// The type, not "entity" (#828).
		expect(screen.getByText('Unnamed source')).toBeDefined();
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

	describe('what each side says (#828)', () => {
		const VALUED: MergeConflict = {
			...EDIT_EDIT,
			field_values: [
				{
					field: 'surname',
					label: 'Surname',
					base_value: 'Byron',
					branch_value: 'Lovelace',
					main_value: 'King'
				},
				{
					field: 'children[44444444-4444-4444-4444-444444444444]',
					label: 'Child: Byron King',
					base_value: null,
					branch_value: 'Linked as a child',
					main_value: 'Not linked'
				}
			]
		};

		it('renders the fork, branch and mainline values side by side under readable labels', () => {
			renderResolver([VALUED]);

			const table = screen.getByRole('table');
			const headers = within(table)
				.getAllByRole('columnheader')
				.map((th) => th.textContent?.trim());
			expect(headers).toEqual(['Field', 'At the fork', 'This branch', 'Mainline']);

			const surname = within(table).getByRole('rowheader', { name: 'Surname' }).closest('tr')!;
			expect(
				Array.from(surname.querySelectorAll('td')).map((td) => td.textContent?.trim())
			).toEqual(['Byron', 'Lovelace', 'King']);

			// A structural field is named by what it refers to, never by its raw key.
			const child = within(table).getByRole('rowheader', { name: 'Child: Byron King' }).closest('tr')!;
			expect(
				Array.from(child.querySelectorAll('td')).map((td) => td.textContent?.trim())
			).toEqual(['Not set', 'Linked as a child', 'Not linked']);
			expect(screen.queryByText(/children\[/)).toBeNull();
			// The values replace the bare field list.
			expect(screen.queryByText(/Contested fields:/)).toBeNull();
		});

		it("says 'Deleted' for the side that deleted the entity", () => {
			renderResolver([
				{
					...DELETE_EDIT,
					deleted_by: 'main',
					field_values: [
						{
							field: 'marriage_place',
							label: 'Marriage place',
							base_value: 'Old Chapel',
							branch_value: 'Branch Chapel',
							main_value: null
						}
					]
				}
			]);

			const row = screen.getByRole('rowheader', { name: 'Marriage place' }).closest('tr')!;
			expect(
				Array.from(row.querySelectorAll('td')).map((td) => td.textContent?.trim())
			).toEqual(['Old Chapel', 'Branch Chapel', 'Deleted']);
		});

		it("says 'Unknown', not 'Not set', for a fork value the server could not read", () => {
			renderResolver([
				{
					...EDIT_EDIT,
					field_values: [
						{
							field: 'surname',
							label: 'Surname',
							base_value: null,
							base_unknown: true,
							branch_value: 'Lovelace',
							main_value: 'King'
						}
					]
				}
			]);

			const row = screen.getByRole('rowheader', { name: 'Surname' }).closest('tr')!;
			expect(
				Array.from(row.querySelectorAll('td')).map((td) => td.textContent?.trim())
			).toEqual(['Unknown', 'Lovelace', 'King']);
		});
	});

	describe('bulk resolution', () => {
		const SOURCE_EDIT: MergeConflict = {
			...EDIT_EDIT,
			stream_id: '55555555-5555-5555-5555-555555555555',
			entity_type: 'source',
			entity_name: 'Parish register'
		};

		it('is not offered for a single conflict', () => {
			renderResolver([EDIT_EDIT]);
			expect(screen.queryByRole('group', { name: 'Decide several at once' })).toBeNull();
		});

		it("takes the branch's version for every conflict that accepts it, and says which it skipped", async () => {
			const onresolveall = vi.fn();
			renderResolver([EDIT_EDIT, CREATE_CREATE, DELETE_EDIT], { onresolveall });

			const group = screen.getByRole('group', { name: 'Decide several at once' });
			// The live region is rendered before its first message, so that
			// message is announced.
			expect(within(group).getByRole('status').textContent).toBe('');
			await fireEvent.click(
				within(group).getByRole('button', { name: "Take the branch's version for all" })
			);

			// Only the edit_edit accepts "branch"; the two main-only conflicts are
			// left alone rather than flipped to the other side.
			expect(onresolveall).toHaveBeenCalledWith([[EDIT_EDIT.stream_id, 'branch']]);
			expect(within(group).getByRole('status').textContent).toBe(
				"Chose the branch's version for 1 conflict. 2 conflicts cannot take the branch's version and were left as they were."
			);
		});

		it("keeps the mainline's version for all", async () => {
			const onresolveall = vi.fn();
			renderResolver([EDIT_EDIT, CREATE_CREATE], { onresolveall });

			await fireEvent.click(screen.getByRole('button', { name: "Keep the mainline's version for all" }));

			expect(onresolveall).toHaveBeenCalledWith([
				[EDIT_EDIT.stream_id, 'main'],
				[CREATE_CREATE.stream_id, 'main']
			]);
			expect(screen.getByRole('status').textContent).toBe(
				"Chose the mainline's version for 2 conflicts."
			);
		});

		it('decides one entity type at a time', async () => {
			const onresolveall = vi.fn();
			renderResolver([EDIT_EDIT, SOURCE_EDIT, CREATE_CREATE], { onresolveall });

			await fireEvent.click(
				screen.getByRole('button', { name: "Take the branch's version for every source conflict" })
			);
			expect(onresolveall).toHaveBeenLastCalledWith([[SOURCE_EDIT.stream_id, 'branch']]);

			await fireEvent.click(
				screen.getByRole('button', { name: "Keep the mainline's version for every person conflict" })
			);
			expect(onresolveall).toHaveBeenLastCalledWith([[EDIT_EDIT.stream_id, 'main']]);
		});

		it('disables a side no targeted conflict accepts', () => {
			renderResolver([CREATE_CREATE, DELETE_EDIT]);

			const takeAll = screen.getByRole('button', {
				name: "Take the branch's version for all"
			}) as HTMLButtonElement;
			expect(takeAll.disabled).toBe(true);
		});

		it('falls back to one onresolve per conflict without onresolveall', async () => {
			const { onresolve } = renderResolver([EDIT_EDIT, SOURCE_EDIT]);

			await fireEvent.click(screen.getByRole('button', { name: "Take the branch's version for all" }));

			expect(onresolve).toHaveBeenCalledTimes(2);
			expect(onresolve).toHaveBeenCalledWith(EDIT_EDIT.stream_id, 'branch');
			expect(onresolve).toHaveBeenCalledWith(SOURCE_EDIT.stream_id, 'branch');
		});

		it('is disabled while a merge is in flight', () => {
			renderResolver([EDIT_EDIT, SOURCE_EDIT], { disabled: true });

			for (const button of within(
				screen.getByRole('group', { name: 'Decide several at once' })
			).getAllByRole('button')) {
				expect((button as HTMLButtonElement).disabled).toBe(true);
			}
		});

		it('is reachable from the keyboard as ordinary buttons', () => {
			renderResolver([EDIT_EDIT, SOURCE_EDIT]);

			const button = screen.getByRole('button', { name: "Keep the mainline's version for all" });
			button.focus();
			expect(document.activeElement).toBe(button);
		});
	});

	describe('rationale', () => {
		it('is hidden without an onrationale handler', () => {
			renderResolver([EDIT_EDIT]);
			expect(screen.queryByLabelText(/Why this side/)).toBeNull();
		});

		it('reports what is typed, labelled for its conflict', async () => {
			const onrationale = vi.fn();
			renderResolver([EDIT_EDIT], {
				onrationale,
				rationales: new Map([[EDIT_EDIT.stream_id, 'Census']])
			});

			const field = screen.getByLabelText(/Why this side/) as HTMLTextAreaElement;
			expect(field.value).toBe('Census');
			expect(field.maxLength).toBe(1000);

			await fireEvent.input(field, { target: { value: 'Census 1881' } });
			expect(onrationale).toHaveBeenCalledWith(EDIT_EDIT.stream_id, 'Census 1881');
		});
	});
});
