import { describe, it, expect, vi, beforeEach } from 'vitest';
import { fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import PersonSelector from './PersonSelector.svelte';
import PartnerPickers from './PartnerPickers.svelte';
import type * as apiModule from '$lib/api/client';

const { searchPersons } = vi.hoisted(() => ({ searchPersons: vi.fn() }));

vi.mock('$lib/api/client', async (importOriginal) => {
	const actual = await importOriginal<typeof apiModule>();
	return { ...actual, api: { searchPersons } };
});

const ann = { id: 'ann-id', given_name: 'Ann', surname: 'Smith', gender: 'female' as const };
const john = { id: 'john-id', given_name: 'John', surname: 'Smith', gender: 'male' as const };

async function typeQuery(input: HTMLElement, value: string) {
	await fireEvent.input(input, { target: { value } });
	// The search is debounced by 300ms.
	await waitFor(() => expect(searchPersons).toHaveBeenCalled(), { timeout: 1500 });
}

describe('PersonSelector', () => {
	beforeEach(() => {
		vi.clearAllMocks();
		searchPersons.mockResolvedValue({ items: [ann, john], total: 2 });
	});

	it('searches, lists the results and reports the pick', async () => {
		const onSelect = vi.fn();
		render(PersonSelector, { label: 'Partner 1', onSelect });

		await typeQuery(screen.getByRole('combobox', { name: 'Partner 1' }), 'Smith');
		expect(searchPersons).toHaveBeenCalledWith({ q: 'Smith', fuzzy: true, limit: 8 });

		const option = await screen.findByRole('option', { name: /John Smith/ });
		await fireEvent.click(option);
		expect(onSelect).toHaveBeenCalledWith(john);
	});

	it('hides excluded people from the results', async () => {
		render(PersonSelector, { label: 'Child', excludeIds: ['ann-id'] });

		await typeQuery(screen.getByRole('combobox', { name: 'Child' }), 'Smith');
		await screen.findByRole('option', { name: /John Smith/ });
		expect(screen.queryByRole('option', { name: /Ann Smith/ })).toBeNull();
	});

	it('picks with the keyboard and points aria-activedescendant at the highlighted option', async () => {
		const onSelect = vi.fn();
		render(PersonSelector, { label: 'Partner 2', onSelect });
		const input = screen.getByRole('combobox', { name: 'Partner 2' });

		await typeQuery(input, 'Smith');
		const options = await screen.findAllByRole('option');
		await fireEvent.keyDown(input, { key: 'ArrowDown' });
		expect(input.getAttribute('aria-activedescendant')).toBe(options[0].id);
		await fireEvent.keyDown(input, { key: 'Enter' });
		expect(onSelect).toHaveBeenCalledWith(ann);
	});

	it('keeps Escape from reaching an enclosing dialog while its list is open', async () => {
		const outer = vi.fn();
		document.addEventListener('keydown', outer);
		try {
			render(PersonSelector, { label: 'Child' });
			const input = screen.getByRole('combobox', { name: 'Child' });
			await typeQuery(input, 'Smith');
			await screen.findAllByRole('option');

			await fireEvent.keyDown(input, { key: 'Escape' });
			expect(outer).not.toHaveBeenCalled();
			expect(screen.queryByRole('listbox')).toBeNull();

			// With the list closed, Escape goes on to close the dialog as usual.
			await fireEvent.keyDown(input, { key: 'Escape' });
			expect(outer).toHaveBeenCalledTimes(1);
		} finally {
			document.removeEventListener('keydown', outer);
		}
	});

	it('names the clear button after the slot and the person in it', async () => {
		const onSelect = vi.fn();
		render(PersonSelector, { label: 'Partner 1', selectedPerson: john, onSelect });

		await fireEvent.click(screen.getByRole('button', { name: 'Clear Partner 1: John Smith' }));
		expect(onSelect).toHaveBeenCalledWith(null);
	});
});

describe('PartnerPickers', () => {
	beforeEach(() => {
		vi.clearAllMocks();
		searchPersons.mockResolvedValue({ items: [ann, john], total: 2 });
	});

	it('gives the two pickers distinct listbox ids', async () => {
		render(PartnerPickers, {});
		const [first, second] = screen.getAllByRole('combobox');
		expect(first.getAttribute('aria-controls')).not.toBe(second.getAttribute('aria-controls'));
	});

	it('hides partner 1 from the partner 2 results', async () => {
		render(PartnerPickers, { partner1: john });

		await typeQuery(screen.getByRole('combobox', { name: 'Partner 2' }), 'Smith');
		await screen.findByRole('option', { name: /Ann Smith/ });
		expect(screen.queryByRole('option', { name: /John Smith/ })).toBeNull();
	});

	it('hides the excluded ids from both pickers', async () => {
		render(PartnerPickers, { excludeIds: ['ann-id'] });

		await typeQuery(screen.getByRole('combobox', { name: 'Partner 1' }), 'Smith');
		await screen.findByRole('option', { name: /John Smith/ });
		expect(screen.queryByRole('option', { name: /Ann Smith/ })).toBeNull();
	});
});
