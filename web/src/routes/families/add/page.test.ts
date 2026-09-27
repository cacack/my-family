import { describe, it, expect, vi, beforeEach } from 'vitest';
import { fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import AddFamilyPage from './+page.svelte';
import type * as apiModule from '$lib/api/client';
import { goto } from '$app/navigation';

const { createFamily, addChildToFamily, getPerson, searchPersons, pageState } = vi.hoisted(() => ({
	createFamily: vi.fn(),
	addChildToFamily: vi.fn(),
	getPerson: vi.fn(),
	searchPersons: vi.fn(),
	pageState: { search: '' }
}));

vi.mock('$lib/api/client', async (importOriginal) => {
	const actual = await importOriginal<typeof apiModule>();
	return { ...actual, api: { createFamily, addChildToFamily, getPerson, searchPersons } };
});

vi.mock('$app/stores', () => ({
	page: {
		subscribe: (callback: (value: { url: URL; params: Record<string, string> }) => void) => {
			callback({ url: new URL(`http://localhost/families/add${pageState.search}`), params: {} });
			return () => {};
		}
	}
}));

vi.mock('$app/navigation', () => ({ goto: vi.fn() }));

const john = { id: 'john-id', given_name: 'John', surname: 'Smith', version: 1 };
const ann = { id: 'ann-id', given_name: 'Ann', surname: 'Jones', version: 1 };
const mary = { id: 'mary-id', given_name: 'Mary', surname: 'Smith', version: 1 };

async function pick(label: string, query: string, name: RegExp) {
	const input = screen.getByRole('combobox', { name: label });
	await fireEvent.input(input, { target: { value: query } });
	await fireEvent.click(await screen.findByRole('option', { name }, { timeout: 1500 }));
}

describe('Add Family page (#826)', () => {
	beforeEach(() => {
		vi.clearAllMocks();
		pageState.search = '';
		createFamily.mockResolvedValue({ id: 'new-fam', version: 1 });
		addChildToFamily.mockResolvedValue({ person_id: 'mary-id', relationship_type: 'biological' });
		searchPersons.mockImplementation(async ({ q }: { q: string }) => ({
			items: q === 'Jones' ? [ann] : [john, mary],
			total: 1
		}));
		getPerson.mockImplementation(async (id: string) => ({ 'john-id': john, 'mary-id': mary })[id]);
	});

	it('no longer claims partners can only be added afterwards', () => {
		render(AddFamilyPage);
		expect(screen.queryByText(/Partners can be added after creating the family/)).toBeNull();
		expect(screen.getByRole('group', { name: 'Partners' })).toBeDefined();
	});

	it('creates the family with the picked partners', async () => {
		render(AddFamilyPage);
		await pick('Partner 1', 'Smith', /John Smith/);
		await pick('Partner 2', 'Jones', /Ann Jones/);
		await fireEvent.change(screen.getByLabelText('Relationship Type'), { target: { value: 'marriage' } });
		await fireEvent.click(screen.getByRole('button', { name: 'Create Family' }));

		await waitFor(() =>
			expect(createFamily).toHaveBeenCalledWith(
				expect.objectContaining({ partner1_id: 'john-id', partner2_id: 'ann-id', relationship_type: 'marriage' })
			)
		);
		expect(addChildToFamily).not.toHaveBeenCalled();
		await waitFor(() => expect(goto).toHaveBeenCalledWith('/families/new-fam'));
	});

	it('creates a family with no partners when none are picked', async () => {
		render(AddFamilyPage);
		await fireEvent.click(screen.getByRole('button', { name: 'Create Family' }));
		await waitFor(() => expect(createFamily).toHaveBeenCalled());
		const payload = createFamily.mock.calls[0][0];
		expect(payload.partner1_id).toBeUndefined();
		expect(payload.partner2_id).toBeUndefined();
	});

	it('prefills partner 1 from ?partner1= (the person page "Add family" shortcut)', async () => {
		pageState.search = '?partner1=john-id';
		render(AddFamilyPage);

		expect(await screen.findByRole('button', { name: 'Clear Partner 1: John Smith' })).toBeDefined();
		await fireEvent.click(screen.getByRole('button', { name: 'Create Family' }));
		await waitFor(() => expect(createFamily).toHaveBeenCalledWith(expect.objectContaining({ partner1_id: 'john-id' })));
	});

	it('links ?child= as the new family\'s child (the "Add parents" shortcut)', async () => {
		pageState.search = '?child=mary-id';
		render(AddFamilyPage);

		expect((await screen.findByTestId('child-note')).textContent).toContain('Mary Smith will be added as a child');
		await pick('Partner 1', 'Smith', /John Smith/);
		// The child is never offered as one of the parents.
		const input = screen.getByRole('combobox', { name: 'Partner 2' });
		await fireEvent.input(input, { target: { value: 'Smith' } });
		await screen.findByText('No people found', {}, { timeout: 1500 });

		await fireEvent.click(screen.getByRole('button', { name: 'Create Family' }));
		await waitFor(() => expect(addChildToFamily).toHaveBeenCalledWith('new-fam', { person_id: 'mary-id' }));
		await waitFor(() => expect(goto).toHaveBeenCalledWith('/families/new-fam'));
	});

	it('says the family exists when only the child link fails', async () => {
		pageState.search = '?child=mary-id';
		addChildToFamily.mockRejectedValue({ message: 'Circular ancestry detected' });
		render(AddFamilyPage);
		await screen.findByTestId('child-note');

		await fireEvent.click(screen.getByRole('button', { name: 'Create Family' }));

		const alert = await screen.findByRole('alert');
		expect(alert.textContent).toContain('The family was created, but Mary Smith could not be added');
		expect(screen.getByRole('link', { name: 'Open the family' }).getAttribute('href')).toBe('/families/new-fam');
		expect(goto).not.toHaveBeenCalled();
		// A second submit would create a duplicate family.
		expect(screen.getByRole('button', { name: 'Create Family' }).hasAttribute('disabled')).toBe(true);
	});

	it('reports a prefill person that cannot be loaded, and will not create a childless family', async () => {
		pageState.search = '?child=missing';
		getPerson.mockRejectedValue({ message: 'Person not found' });
		render(AddFamilyPage);
		expect((await screen.findByRole('alert')).textContent).toContain('Person not found');

		const submit = screen.getByRole('button', { name: 'Create Family' });
		expect(submit.hasAttribute('disabled')).toBe(true);
		await fireEvent.submit(submit.closest('form')!);
		expect(createFamily).not.toHaveBeenCalled();
	});

	it('cannot be submitted while the ?child= prefill is still loading', async () => {
		pageState.search = '?child=mary-id';
		let release: (value: typeof mary) => void = () => {};
		getPerson.mockImplementation(() => new Promise((resolve) => (release = resolve)));
		render(AddFamilyPage);

		const submit = await screen.findByRole('button', { name: 'Loading...' });
		expect(submit.hasAttribute('disabled')).toBe(true);
		await fireEvent.submit(submit.closest('form')!);
		expect(createFamily).not.toHaveBeenCalled();

		release(mary);
		await screen.findByTestId('child-note');
		await fireEvent.click(screen.getByRole('button', { name: 'Create Family' }));
		await waitFor(() => expect(addChildToFamily).toHaveBeenCalledWith('new-fam', { person_id: 'mary-id' }));
	});
});
