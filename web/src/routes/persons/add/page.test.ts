import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/svelte';
import Page from './+page.svelte';
import * as apiModule from '$lib/api/client';

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
