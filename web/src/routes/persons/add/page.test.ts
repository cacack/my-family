import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/svelte';
import Page from './+page.svelte';
import * as apiModule from '$lib/api/client';
import { goto } from '$app/navigation';

vi.mock('$lib/api/client', async (importOriginal) => {
	const actual = await importOriginal<typeof apiModule>();
	return {
		...actual,
		api: {
			createPerson: vi.fn()
		}
	};
});

vi.mock('$app/navigation', () => ({
	goto: vi.fn()
}));

// The API requires a given name and accepts an empty surname (PersonCreate);
// the form enforces the same rule (#911).
describe('Add Person form', () => {
	beforeEach(() => {
		vi.clearAllMocks();
		vi.mocked(apiModule.api.createPerson).mockResolvedValue({
			id: 'p1',
			given_name: 'Madonna',
			surname: '',
			version: 1
		} as apiModule.Person);
	});

	function form(): HTMLFormElement {
		return screen.getByLabelText('Given Name').closest('form')!;
	}

	it('creates a person with no surname', async () => {
		render(Page);

		await fireEvent.input(screen.getByLabelText('Given Name'), { target: { value: 'Madonna' } });
		expect(form().checkValidity()).toBe(true);
		await fireEvent.submit(form());

		await waitFor(() => expect(apiModule.api.createPerson).toHaveBeenCalled());
		expect(vi.mocked(apiModule.api.createPerson).mock.calls[0][0]).toMatchObject({
			given_name: 'Madonna',
			surname: ''
		});
	});

	it('requires a given name', async () => {
		render(Page);

		await fireEvent.input(screen.getByLabelText('Surname'), { target: { value: 'Smith' } });

		expect(form().checkValidity()).toBe(false);
		expect((screen.getByLabelText('Given Name') as HTMLInputElement).validity.valueMissing).toBe(true);
	});
});

// A date the server can't read had no way out but typing parentheses (#996).
describe('Add Person form dates (#996)', () => {
	const createPerson = vi.mocked(apiModule.api.createPerson);

	beforeEach(() => {
		vi.clearAllMocks();
	});

	async function createWithDate(label: string, date: string) {
		render(Page);
		await fireEvent.input(screen.getByLabelText('Given Name'), { target: { value: 'Ada' } });
		await fireEvent.input(screen.getByLabelText(label), { target: { value: date } });
		await fireEvent.submit(screen.getByLabelText('Given Name').closest('form')!);
	}

	it('offers to keep an unreadable date as text and creates the person with it as a phrase', async () => {
		createPerson.mockRejectedValueOnce({
			code: 'VALIDATION_ERROR',
			message: 'invalid input: death_date: "spring 1900" is not a recognized date',
			details: { field: 'death_date', unreadable: true }
		});
		createPerson.mockResolvedValueOnce({
			id: 'p1',
			given_name: 'Ada',
			surname: '',
			death_date: { raw: '(spring 1900)' },
			version: 1
		} as apiModule.Person);
		await createWithDate('Death Date', 'spring 1900');

		await fireEvent.click(await screen.findByRole('button', { name: 'Keep as text' }));

		await waitFor(() => expect(createPerson).toHaveBeenCalledTimes(2));
		expect(createPerson.mock.calls[1][0].death_date).toBe('(spring 1900)');
		await waitFor(() => expect(goto).toHaveBeenCalledWith('/persons/p1', { state: { savedAs: {} } }));
	});

	it('offers no Keep as text for a date that reads but does not exist', async () => {
		createPerson.mockRejectedValue({
			code: 'VALIDATION_ERROR',
			message: 'invalid input: birth_date: "31 FEB 1900" is not a real calendar date',
			details: { field: 'birth_date', unreadable: false }
		});
		await createWithDate('Birth Date', '31 FEB 1900');

		expect((await screen.findByRole('alert')).textContent).toContain('is not a real calendar date');
		expect(screen.queryByRole('button', { name: 'Keep as text' })).toBeNull();
	});

	it('hands the person page what a converted date was saved as', async () => {
		createPerson.mockResolvedValue({
			id: 'p1',
			given_name: 'Ada',
			surname: '',
			birth_date: { raw: 'ABT 1850' },
			version: 1
		} as apiModule.Person);
		await createWithDate('Birth Date', 'about 1850');

		await waitFor(() =>
			expect(goto).toHaveBeenCalledWith('/persons/p1', {
				state: { savedAs: { birth_date: { typed: 'about 1850', stored: 'ABT 1850' } } }
			})
		);
	});
});
