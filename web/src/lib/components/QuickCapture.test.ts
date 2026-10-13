import { describe, it, expect, vi, beforeEach } from 'vitest';
import { fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import QuickCapture from './QuickCapture.svelte';
import type * as apiModule from '$lib/api/client';
import { goto } from '$app/navigation';

const { createPerson } = vi.hoisted(() => ({ createPerson: vi.fn() }));

vi.mock('$lib/api/client', async (importOriginal) => {
	const actual = await importOriginal<typeof apiModule>();
	return { ...actual, api: { createPerson } };
});

vi.mock('$app/navigation', () => ({ goto: vi.fn() }));

const created = (birthDate?: string) => ({
	id: 'p1',
	given_name: 'Ada',
	surname: 'Lovelace',
	version: 1,
	...(birthDate ? { birth_date: { raw: birthDate } } : {})
});

async function addWithBirthDate(year: string, button = 'Add & Next') {
	await fireEvent.input(screen.getByLabelText('Full Name'), { target: { value: 'Ada Lovelace' } });
	await fireEvent.input(screen.getByLabelText(/Birth Date/), { target: { value: year } });
	await fireEvent.click(screen.getByRole('button', { name: button }));
}

// A date the server can't read had no way out but typing parentheses (#996).
describe('QuickCapture dates (#996)', () => {
	beforeEach(() => {
		vi.clearAllMocks();
	});

	it('offers to keep an unreadable birth date as text and adds the person with it as a phrase', async () => {
		createPerson.mockRejectedValueOnce({
			code: 'VALIDATION_ERROR',
			message: 'invalid input: birth_date: "late 1850s" is not a recognized date',
			details: { field: 'birth_date', unreadable: true }
		});
		createPerson.mockResolvedValueOnce(created('(late 1850s)'));
		render(QuickCapture);
		await addWithBirthDate('late 1850s');

		await fireEvent.click(await screen.findByRole('button', { name: 'Keep as text' }));

		await waitFor(() => expect(createPerson).toHaveBeenCalledTimes(2));
		expect(createPerson.mock.calls[1][0].birth_date).toBe('(late 1850s)');
		expect(await screen.findByText('1 person added this session')).toBeDefined();
	});

	it('keeps an unreadable date as text with the same action it was added with', async () => {
		createPerson.mockRejectedValueOnce({
			message: 'invalid input: birth_date: "late 1850s" is not a recognized date',
			details: { field: 'birth_date', unreadable: true }
		});
		createPerson.mockResolvedValueOnce(created('(late 1850s)'));
		render(QuickCapture);
		await addWithBirthDate('late 1850s', 'Add & View');

		await fireEvent.click(await screen.findByRole('button', { name: 'Keep as text' }));

		await waitFor(() => expect(goto).toHaveBeenCalledWith('/persons/p1', { state: { savedAs: {} } }));
	});

	it('offers no Keep as text for a date that reads but does not exist', async () => {
		createPerson.mockRejectedValue({
			message: 'invalid input: birth_date: "31 FEB 1900" is not a real calendar date',
			details: { field: 'birth_date', unreadable: false }
		});
		render(QuickCapture);
		await addWithBirthDate('31 FEB 1900');

		expect((await screen.findByRole('alert')).textContent).toContain('is not a real calendar date');
		expect(screen.queryByRole('button', { name: 'Keep as text' })).toBeNull();
	});

	it('says what a converted birth date was saved as', async () => {
		createPerson.mockResolvedValue(created('ABT 1850'));
		render(QuickCapture);
		await addWithBirthDate('about 1850');

		expect((await screen.findByRole('status')).textContent).toContain(
			'Birth date saved as ABT 1850 from “about 1850”'
		);
	});

	it('hands the person page what a converted birth date was saved as', async () => {
		createPerson.mockResolvedValue(created('ABT 1850'));
		render(QuickCapture);
		await addWithBirthDate('about 1850', 'Add & View');

		await waitFor(() =>
			expect(goto).toHaveBeenCalledWith('/persons/p1', {
				state: { savedAs: { birth_date: { typed: 'about 1850', stored: 'ABT 1850' } } }
			})
		);
	});

	it('asks for a birth date in English, not a year on a number keypad (#1007)', () => {
		render(QuickCapture);
		const input = screen.getByLabelText(/Birth Date/);

		expect(input.getAttribute('placeholder')).toBe('e.g., 1850 or about 1850');
		expect(input.hasAttribute('inputmode')).toBe(false);
	});

	it('says nothing when the birth date was stored as typed', async () => {
		createPerson.mockResolvedValue(created('1850'));
		render(QuickCapture);
		await addWithBirthDate('1850');

		await screen.findByText('1 person added this session');
		expect(screen.queryByRole('status')).toBeNull();
	});
});
