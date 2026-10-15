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
			updatePersonName: vi.fn()
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
});
