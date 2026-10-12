import { describe, it, expect, vi, beforeEach } from 'vitest';
import { fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import CreatePersonStep from './CreatePersonStep.svelte';
import CompletionStep from './CompletionStep.svelte';
import type * as apiModule from '$lib/api/client';

const { createPerson } = vi.hoisted(() => ({ createPerson: vi.fn() }));

vi.mock('$lib/api/client', async (importOriginal) => {
	const actual = await importOriginal<typeof apiModule>();
	return { ...actual, api: { createPerson } };
});

async function createWithBirthDate(date: string) {
	await fireEvent.input(screen.getByLabelText('Given Name'), { target: { value: 'Ada' } });
	await fireEvent.input(screen.getByLabelText(/Birth Date/), { target: { value: date } });
	await fireEvent.click(screen.getByRole('button', { name: 'Create Person' }));
}

// A date the server can't read had no way out but typing parentheses (#996).
describe('Onboarding first person dates (#996)', () => {
	const onComplete = vi.fn();

	beforeEach(() => {
		vi.clearAllMocks();
	});

	it('offers to keep an unreadable birth date as text and creates the person with it as a phrase', async () => {
		createPerson.mockRejectedValueOnce({
			message: 'invalid input: birth_date: "spring 1850" is not a recognized date',
			details: { field: 'birth_date', unreadable: true }
		});
		createPerson.mockResolvedValueOnce({ id: 'p1', given_name: 'Ada', surname: '', birth_date: { raw: '(spring 1850)' } });
		render(CreatePersonStep, { onComplete, onBack: vi.fn() });
		await createWithBirthDate('spring 1850');

		await fireEvent.click(await screen.findByRole('button', { name: 'Keep as text' }));

		await waitFor(() => expect(createPerson).toHaveBeenCalledTimes(2));
		expect(createPerson.mock.calls[1][0].birth_date).toBe('(spring 1850)');
		await waitFor(() => expect(onComplete).toHaveBeenCalledWith(expect.objectContaining({ personId: 'p1', birthDateSavedAs: undefined })));
	});

	it('offers no Keep as text for a date that reads but does not exist', async () => {
		createPerson.mockRejectedValue({
			message: 'invalid input: birth_date: "31 FEB 1900" is not a real calendar date',
			details: { field: 'birth_date', unreadable: false }
		});
		render(CreatePersonStep, { onComplete, onBack: vi.fn() });
		await createWithBirthDate('31 FEB 1900');

		expect((await screen.findByRole('alert')).textContent).toContain('is not a real calendar date');
		expect(screen.queryByRole('button', { name: 'Keep as text' })).toBeNull();
	});

	it('passes on what a converted birth date was saved as', async () => {
		createPerson.mockResolvedValue({ id: 'p1', given_name: 'Ada', surname: '', birth_date: { raw: 'ABT 1850' } });
		render(CreatePersonStep, { onComplete, onBack: vi.fn() });
		await createWithBirthDate('about 1850');

		await waitFor(() =>
			expect(onComplete).toHaveBeenCalledWith({
				personId: 'p1',
				personName: 'Ada',
				birthDateSavedAs: { typed: 'about 1850', stored: 'ABT 1850' }
			})
		);
	});

	it('shows the converted birth date on the completion step', () => {
		render(CompletionStep, {
			completionData: { personId: 'p1', personName: 'Ada', birthDateSavedAs: { typed: 'about 1850', stored: 'ABT 1850' } },
			onFinish: vi.fn()
		});

		expect(screen.getByRole('status').textContent).toContain('Birth date saved as ABT 1850 from “about 1850”');
	});
});
