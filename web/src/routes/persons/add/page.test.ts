import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor, within } from '@testing-library/svelte';
import Page from './+page.svelte';
import * as apiModule from '#lib/api/client.js';
import { goto } from '$app/navigation';

vi.mock('#lib/api/client.js', async (importOriginal) => {
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

// The API needs a given name or a surname and accepts either empty
// (PersonCreate); the form enforces the same rule (#911, #1054).
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

	it('creates a person with a surname and no given name', async () => {
		render(Page);

		await fireEvent.input(screen.getByLabelText('Surname'), { target: { value: 'Tudor' } });
		expect(form().checkValidity()).toBe(true);
		await fireEvent.submit(form());

		await waitFor(() => expect(apiModule.api.createPerson).toHaveBeenCalled());
		expect(vi.mocked(apiModule.api.createPerson).mock.calls[0][0]).toMatchObject({
			given_name: '',
			surname: 'Tudor'
		});
	});

	it('asks for a given name or a surname when both are blank', async () => {
		render(Page);

		await fireEvent.submit(form());

		expect(await screen.findByText('Enter a given name or a surname')).toBeTruthy();
		expect(apiModule.api.createPerson).not.toHaveBeenCalled();
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

	// A rejected date was a banner with the server's raw prefix (#1112).
	it('shows a rejected date beside its field, tied to it and focused', async () => {
		createPerson.mockRejectedValue({
			code: 'VALIDATION_ERROR',
			message: 'invalid input: death_date: "31 FEB 1900" is not a real calendar date',
			details: { field: 'death_date', unreadable: false }
		});
		await createWithDate('Death Date', '31 FEB 1900');

		const input = screen.getByLabelText('Death Date');
		const message = await screen.findByRole('alert');
		expect(message.textContent?.trim()).toBe('"31 FEB 1900" is not a real calendar date');
		expect(input.closest('.date-field')?.contains(message)).toBe(true);
		expect(input.getAttribute('aria-invalid')).toBe('true');
		expect(input.getAttribute('aria-describedby')).toBe(message.id);
		expect(screen.getByLabelText('Birth Date').hasAttribute('aria-invalid')).toBe(false);
		await waitFor(() => expect(document.activeElement).toBe(input));
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

// A person entered by hand can be put on the map (#1033).
describe('Add Person form coordinates (#1033)', () => {
	const createPerson = vi.mocked(apiModule.api.createPerson);

	beforeEach(() => {
		vi.clearAllMocks();
		createPerson.mockResolvedValue({ id: 'p1', given_name: 'Ada', surname: '', version: 1 } as apiModule.Person);
	});

	function coordinates(place: string) {
		const group = within(screen.getByRole('group', { name: `${place} coordinates` }));
		return { latitude: group.getByLabelText('Latitude'), longitude: group.getByLabelText('Longitude') };
	}

	async function fill(place: string, latitude: string, longitude: string) {
		await fireEvent.input(screen.getByLabelText(place), { target: { value: 'Boston' } });
		await fireEvent.input(coordinates(place).latitude, { target: { value: latitude } });
		await fireEvent.input(coordinates(place).longitude, { target: { value: longitude } });
	}

	it('sends typed coordinates in GEDCOM form', async () => {
		render(Page);
		await fireEvent.input(screen.getByLabelText('Given Name'), { target: { value: 'Ada' } });
		await fill('Birth Place', '42.3601', '-71.0589');
		await fill('Death Place', '-33.8688', '151.2093');
		await fireEvent.click(screen.getByRole('button', { name: 'Create Person' }));

		await waitFor(() => expect(createPerson).toHaveBeenCalled());
		expect(createPerson.mock.calls[0][0]).toMatchObject({
			birth_place_latitude: 'N42.3601',
			birth_place_longitude: 'W71.0589',
			death_place_latitude: 'S33.8688',
			death_place_longitude: 'E151.2093'
		});
	});

	it('sends no coordinates when none are typed', async () => {
		render(Page);
		await fireEvent.input(screen.getByLabelText('Given Name'), { target: { value: 'Ada' } });
		await fireEvent.click(screen.getByRole('button', { name: 'Create Person' }));

		await waitFor(() => expect(createPerson).toHaveBeenCalled());
		expect(createPerson.mock.calls[0][0]).not.toHaveProperty('birth_place_latitude');
		expect(createPerson.mock.calls[0][0]).not.toHaveProperty('death_place_longitude');
	});

	it('refuses half a pair on the empty input without creating', async () => {
		render(Page);
		await fireEvent.input(screen.getByLabelText('Given Name'), { target: { value: 'Ada' } });
		await fill('Death Place', '', '151.2093');
		await fireEvent.click(screen.getByRole('button', { name: 'Create Person' }));

		const { latitude } = coordinates('Death Place');
		await waitFor(() => expect(latitude.getAttribute('aria-invalid')).toBe('true'));
		expect(document.getElementById(latitude.getAttribute('aria-describedby') ?? '')?.textContent?.trim()).toBe(
			'Enter a latitude too, or clear the longitude.'
		);
		await waitFor(() => expect(document.activeElement).toBe(latitude));
		expect(createPerson).not.toHaveBeenCalled();
	});
});
