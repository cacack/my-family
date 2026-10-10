import { describe, it, expect, vi, beforeEach } from 'vitest';
import { fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import AddChildDialog, { CHILD_RELATIONSHIPS } from './AddChildDialog.svelte';
import type * as apiModule from '$lib/api/client';
import spec from '../../../../internal/api/openapi.yaml?raw';

const { searchPersons, addChildToFamily } = vi.hoisted(() => ({
	searchPersons: vi.fn(),
	addChildToFamily: vi.fn()
}));

vi.mock('$lib/api/client', async (importOriginal) => {
	const actual = await importOriginal<typeof apiModule>();
	return { ...actual, api: { searchPersons, addChildToFamily } };
});

const mary = { id: 'mary-id', given_name: 'Mary', surname: 'Smith' };
const john = { id: 'john-id', given_name: 'John', surname: 'Smith' };

function renderDialog(overrides: Record<string, unknown> = {}) {
	const onAdded = vi.fn();
	render(AddChildDialog, {
		open: true,
		familyId: 'fam-id',
		familyName: 'John Smith & Ann Jones',
		excludeIds: ['john-id'],
		onAdded,
		...overrides
	});
	return { onAdded };
}

async function pickMary() {
	const input = await screen.findByRole('combobox', { name: 'Child' });
	await fireEvent.input(input, { target: { value: 'Smith' } });
	await fireEvent.click(await screen.findByRole('option', { name: /Mary Smith/ }, { timeout: 1500 }));
}

describe('AddChildDialog', () => {
	beforeEach(() => {
		vi.clearAllMocks();
		searchPersons.mockResolvedValue({ items: [mary, john], total: 2 });
		addChildToFamily.mockResolvedValue({ person_id: 'mary-id', relationship_type: 'adopted' });
	});

	it('offers exactly the relationship types the API accepts', () => {
		const block = spec.slice(spec.indexOf('    ChildRelationType:'));
		const enumLine = block.match(/enum: \[([^\]]+)\]/);
		expect(enumLine).not.toBeNull();
		const values = enumLine![1].split(',').map((v) => v.trim());
		expect(CHILD_RELATIONSHIPS.map((option) => option.value)).toEqual(values);
	});

	it('names the family and keeps Add disabled until a person is picked', async () => {
		renderDialog();
		expect(await screen.findByText(/as a child of John Smith & Ann Jones/)).toBeDefined();
		expect(screen.getByRole('button', { name: 'Add child' }).hasAttribute('disabled')).toBe(true);
	});

	it('hides excluded people (the partners and current children) from the search', async () => {
		renderDialog();
		const input = await screen.findByRole('combobox', { name: 'Child' });
		await fireEvent.input(input, { target: { value: 'Smith' } });
		await screen.findByRole('option', { name: /Mary Smith/ }, { timeout: 1500 });
		expect(screen.queryByRole('option', { name: /John Smith/ })).toBeNull();
	});

	it('links the picked person with the chosen relationship type', async () => {
		const { onAdded } = renderDialog();
		await pickMary();
		await fireEvent.change(screen.getByLabelText('Relationship to the parents'), {
			target: { value: 'adopted' }
		});
		await fireEvent.click(screen.getByRole('button', { name: 'Add Mary Smith' }));

		await waitFor(() =>
			expect(addChildToFamily).toHaveBeenCalledWith('fam-id', {
				person_id: 'mary-id',
				relationship_type: 'adopted'
			})
		);
		await waitFor(() => expect(onAdded).toHaveBeenCalledWith(expect.objectContaining({ person_id: 'mary-id' }), mary));
	});

	it('defaults to a birth (biological) link', async () => {
		renderDialog();
		await pickMary();
		await fireEvent.click(screen.getByRole('button', { name: 'Add Mary Smith' }));
		await waitFor(() =>
			expect(addChildToFamily).toHaveBeenCalledWith('fam-id', {
				person_id: 'mary-id',
				relationship_type: 'biological'
			})
		);
	});

	it('shows a refusal in the dialog and does not report success', async () => {
		addChildToFamily.mockRejectedValue({ message: 'Circular ancestry detected' });
		const { onAdded } = renderDialog();
		await pickMary();
		await fireEvent.click(screen.getByRole('button', { name: 'Add Mary Smith' }));

		expect((await screen.findByRole('alert')).textContent).toContain('Circular ancestry detected');
		expect(onAdded).not.toHaveBeenCalled();
		expect(screen.getByRole('dialog')).toBeDefined();
	});
});
