import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/svelte';
import NameSection from './NameSection.svelte';
import * as apiModule from '$lib/api/client';

vi.mock('$lib/api/client', async (importOriginal) => {
	const actual = await importOriginal<typeof apiModule>();
	return {
		...actual,
		api: {
			getPersonNames: vi.fn(),
			addPersonName: vi.fn(),
			updatePersonName: vi.fn(),
			deletePersonName: vi.fn()
		}
	};
});

const PERSON_ID = 'p1';

function surnameless(): apiModule.PersonName {
	return {
		id: 'n1',
		person_id: PERSON_ID,
		given_name: 'Victoria',
		surname: '',
		name_type: 'birth',
		is_primary: true
	} as apiModule.PersonName;
}

// A name's surname is optional, as the API accepts (#911).
describe('NameSection', () => {
	beforeEach(() => {
		vi.clearAllMocks();
		vi.mocked(apiModule.api.getPersonNames).mockResolvedValue({ items: [surnameless()], total: 1 });
		vi.mocked(apiModule.api.addPersonName).mockResolvedValue(surnameless());
		vi.mocked(apiModule.api.updatePersonName).mockResolvedValue(surnameless());
	});

	it('shows a surname-less name without a trailing space', async () => {
		render(NameSection, { personId: PERSON_ID });

		// The matcher trims; the raw text must not carry an empty surname's space.
		expect((await screen.findByText('Victoria')).textContent).toBe('Victoria');
	});

	it('saves an edit to a name with no surname', async () => {
		render(NameSection, { personId: PERSON_ID });
		await fireEvent.click(await screen.findByRole('button', { name: 'Edit' }));

		const form = screen.getByLabelText(/^Given Name/).closest('form')!;
		expect(form.checkValidity()).toBe(true);
		await fireEvent.submit(form);

		await waitFor(() => expect(apiModule.api.updatePersonName).toHaveBeenCalled());
		expect(vi.mocked(apiModule.api.updatePersonName).mock.calls[0][2]).toMatchObject({
			given_name: 'Victoria',
			surname: ''
		});
	});

	it('adds a name with no surname', async () => {
		render(NameSection, { personId: PERSON_ID });
		await fireEvent.click(await screen.findByRole('button', { name: 'Add Name' }));

		await fireEvent.input(screen.getByLabelText(/^Given Name/), { target: { value: 'Alexandrina' } });
		await fireEvent.submit(screen.getByLabelText(/^Given Name/).closest('form')!);

		await waitFor(() => expect(apiModule.api.addPersonName).toHaveBeenCalled());
		expect(vi.mocked(apiModule.api.addPersonName).mock.calls[0][1]).toMatchObject({
			given_name: 'Alexandrina',
			surname: ''
		});
	});

	// An edit can rename the person (the primary name, #1093), so the page is told.
	it('reports a saved edit and a saved add to the page', async () => {
		const onNamesChanged = vi.fn();
		render(NameSection, { personId: PERSON_ID, onNamesChanged });

		await fireEvent.click(await screen.findByRole('button', { name: 'Edit' }));
		await fireEvent.submit(screen.getByLabelText(/^Given Name/).closest('form')!);
		await waitFor(() => expect(onNamesChanged).toHaveBeenCalledTimes(1));

		await fireEvent.click(screen.getByRole('button', { name: 'Add Name' }));
		await fireEvent.input(screen.getByLabelText(/^Given Name/), { target: { value: 'Alexandrina' } });
		await fireEvent.submit(screen.getByLabelText(/^Given Name/).closest('form')!);
		await waitFor(() => expect(onNamesChanged).toHaveBeenCalledTimes(2));
	});

	it('does not report a failed edit', async () => {
		const onNamesChanged = vi.fn();
		vi.mocked(apiModule.api.updatePersonName).mockRejectedValue({ message: 'boom' });
		render(NameSection, { personId: PERSON_ID, onNamesChanged });

		await fireEvent.click(await screen.findByRole('button', { name: 'Edit' }));
		await fireEvent.submit(screen.getByLabelText(/^Given Name/).closest('form')!);

		expect(await screen.findByRole('alert')).toHaveProperty('textContent', 'boom');
		expect(onNamesChanged).not.toHaveBeenCalled();
	});

	it('still requires a given name', async () => {
		render(NameSection, { personId: PERSON_ID });
		await fireEvent.click(await screen.findByRole('button', { name: 'Add Name' }));

		await fireEvent.submit(screen.getByLabelText(/^Given Name/).closest('form')!);

		expect(await screen.findByRole('alert')).toHaveProperty('textContent', 'Given name is required');
		expect(apiModule.api.addPersonName).not.toHaveBeenCalled();
	});

	// The server refuses to delete the primary (or only) name (#1106).
	describe('deleting a name', () => {
		const alias = {
			...surnameless(),
			id: 'n2',
			given_name: 'Drina',
			name_type: 'aka',
			is_primary: false
		} as apiModule.PersonName;

		it('offers no Delete on the primary name, and says how to free it', async () => {
			render(NameSection, { personId: PERSON_ID });

			await screen.findByText('Victoria');
			expect(screen.queryByRole('button', { name: 'Delete' })).toBeNull();
			expect(screen.getByText(/make another name primary first/)).toBeTruthy();
		});

		it('offers Delete on a non-primary name only', async () => {
			vi.mocked(apiModule.api.getPersonNames).mockResolvedValue({
				items: [surnameless(), alias],
				total: 2
			});
			render(NameSection, { personId: PERSON_ID });

			await screen.findByText('Drina');
			const deletes = screen.getAllByRole('button', { name: 'Delete' });
			expect(deletes).toHaveLength(1);
			expect(deletes[0].closest('li')!.textContent).toContain('Drina');
		});

		it('shows the server\'s refusal as it came, without a retry', async () => {
			vi.mocked(apiModule.api.getPersonNames).mockResolvedValue({
				items: [surnameless(), alias],
				total: 2
			});
			const message = 'The primary name cannot be deleted; make another name primary first';
			vi.mocked(apiModule.api.deletePersonName).mockRejectedValue({
				status: 409,
				code: 'CONFLICT',
				message
			});
			render(NameSection, { personId: PERSON_ID });

			await fireEvent.click(await screen.findByRole('button', { name: 'Delete' }));
			await fireEvent.click(screen.getByRole('button', { name: 'Yes, Delete' }));

			expect(await screen.findByRole('alert')).toHaveProperty('textContent', message);
			expect(screen.queryByRole('button', { name: 'Try Again' })).toBeNull();
		});

		it('offers a retry when the delete still hits a stale version', async () => {
			vi.mocked(apiModule.api.getPersonNames).mockResolvedValue({
				items: [surnameless(), alias],
				total: 2
			});
			vi.mocked(apiModule.api.deletePersonName).mockRejectedValue({
				status: 409,
				code: 'CONFLICT_RETRY_FAILED',
				message: 'This record was modified by another operation. Please try again.'
			});
			render(NameSection, { personId: PERSON_ID });

			await fireEvent.click(await screen.findByRole('button', { name: 'Delete' }));
			await fireEvent.click(screen.getByRole('button', { name: 'Yes, Delete' }));

			expect(await screen.findByRole('button', { name: 'Try Again' })).toBeTruthy();
		});
	});
});
