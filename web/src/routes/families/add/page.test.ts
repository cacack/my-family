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
		await waitFor(() => expect(goto).toHaveBeenCalledWith('/families/new-fam', { state: { savedAs: {} } }));
	});

	it('requires at least one partner before it can be created', async () => {
		render(AddFamilyPage);
		expect(screen.queryByText(/Both are optional/)).toBeNull();
		expect(screen.getByTestId('partner-required')).toBeDefined();
		const submit = screen.getByRole('button', { name: 'Create Family' }) as HTMLButtonElement;
		expect(submit.disabled).toBe(true);
		await fireEvent.submit(submit.closest('form')!);
		expect(createFamily).not.toHaveBeenCalled();

		await pick('Partner 2', 'Jones', /Ann Jones/);
		expect(screen.queryByTestId('partner-required')).toBeNull();
		expect(submit.disabled).toBe(false);
		await fireEvent.click(submit);
		await waitFor(() => expect(createFamily).toHaveBeenCalled());
		const payload = createFamily.mock.calls[0][0];
		expect(payload.partner1_id).toBeUndefined();
		expect(payload.partner2_id).toBe('ann-id');
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
		await waitFor(() => expect(goto).toHaveBeenCalledWith('/families/new-fam', { state: { savedAs: {} } }));
	});

	it('says the family exists when only the child link fails', async () => {
		pageState.search = '?child=mary-id';
		addChildToFamily.mockRejectedValue({ message: 'Circular ancestry detected' });
		render(AddFamilyPage);
		await screen.findByTestId('child-note');
		await pick('Partner 1', 'Smith', /John Smith/);

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
		getPerson.mockResolvedValue(john);
		await pick('Partner 1', 'Smith', /John Smith/);
		await fireEvent.click(screen.getByRole('button', { name: 'Create Family' }));
		await waitFor(() => expect(addChildToFamily).toHaveBeenCalledWith('new-fam', { person_id: 'mary-id' }));
	});
});

// A date the server can't read had no way out but typing parentheses (#996).
describe('Add Family page dates (#996)', () => {
	beforeEach(() => {
		vi.clearAllMocks();
		pageState.search = '?partner1=john-id';
		getPerson.mockResolvedValue(john);
	});

	async function createWithDate(date: string) {
		render(AddFamilyPage);
		await screen.findByRole('button', { name: 'Clear Partner 1: John Smith' });
		await fireEvent.input(screen.getByLabelText('Marriage Date'), { target: { value: date } });
		await fireEvent.click(screen.getByRole('button', { name: 'Create Family' }));
	}

	it('offers to keep an unreadable date as text and creates the family with it as a phrase', async () => {
		createFamily.mockRejectedValueOnce({
			code: 'VALIDATION_ERROR',
			message: 'invalid input: marriage_date: "spring 1875" is not a recognized date',
			details: { field: 'marriage_date', unreadable: true }
		});
		createFamily.mockResolvedValueOnce({ id: 'new-fam', version: 1, marriage_date: { raw: '(spring 1875)' } });
		await createWithDate('spring 1875');

		await fireEvent.click(await screen.findByRole('button', { name: 'Keep as text' }));

		await waitFor(() => expect(createFamily).toHaveBeenCalledTimes(2));
		expect(createFamily.mock.calls[1][0].marriage_date).toBe('(spring 1875)');
		await waitFor(() => expect(goto).toHaveBeenCalledWith('/families/new-fam', { state: { savedAs: {} } }));
	});

	it('offers no Keep as text for a date that reads but does not exist', async () => {
		createFamily.mockRejectedValue({
			code: 'VALIDATION_ERROR',
			message: 'invalid input: marriage_date: "31 FEB 1900" is not a real calendar date',
			details: { field: 'marriage_date', unreadable: false }
		});
		await createWithDate('31 FEB 1900');

		expect((await screen.findByRole('alert')).textContent).toContain('is not a real calendar date');
		expect(screen.queryByRole('button', { name: 'Keep as text' })).toBeNull();
	});

	it('hands the family page what a converted date was saved as', async () => {
		createFamily.mockResolvedValue({ id: 'new-fam', version: 1, marriage_date: { raw: 'ABT 1875' } });
		await createWithDate('circa 1875');

		await waitFor(() =>
			expect(goto).toHaveBeenCalledWith('/families/new-fam', {
				state: { savedAs: { marriage_date: { typed: 'circa 1875', stored: 'ABT 1875' } } }
			})
		);
	});
});
