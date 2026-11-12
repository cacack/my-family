import { describe, it, expect, vi, beforeEach } from 'vitest';
import { fireEvent, render, screen, waitFor, within } from '@testing-library/svelte';
import { goto } from '$app/navigation';
import FamilyPage from './+page.svelte';
import SearchBox from '#lib/components/SearchBox.svelte';
import AccessibilityPanel from '#lib/components/AccessibilityPanel.svelte';
import * as apiModule from '#lib/api/client.js';

const { branchState, pageState } = vi.hoisted(() => ({
	// The real store exposes a read-only view, so the active branch is injected.
	branchState: { id: null as string | null },
	// The history state a navigation left, e.g. a create's converted date.
	pageState: {} as App.PageState
}));

vi.mock('#lib/stores/activeBranch.svelte.js', () => ({
	activeBranch: branchState
}));

// Mock the API module
vi.mock('#lib/api/client.js', async (importOriginal) => {
	const actual = await importOriginal<typeof apiModule>();
	return {
		...actual,
		api: {
			getFamily: vi.fn(),
			deleteFamily: vi.fn(),
			getFamilyHistory: vi.fn(),
			getFamilyRestorePoints: vi.fn(),
			updateFamily: vi.fn(),
			addChildToFamily: vi.fn(),
			removeChildFromFamily: vi.fn(),
			searchPersons: vi.fn()
		}
	};
});

// Mock the page store
vi.mock('$app/state', () => ({
	get page() {
		return { params: { id: 'test-family-id' }, state: pageState };
	}
}));

// Mock navigation
vi.mock('$app/navigation', () => ({
	goto: vi.fn()
}));

const mockFamilyWithChildren: apiModule.FamilyDetail = {
	id: 'test-family-id',
	partner1_id: 'partner1-id',
	partner2_id: 'partner2-id',
	relationship_type: 'marriage',
	marriage_place: 'Chicago, IL',
	version: 1,
	partner1: {
		id: 'partner1-id',
		given_name: 'John',
		surname: 'Smith'
	},
	partner2: {
		id: 'partner2-id',
		given_name: 'Jane',
		surname: 'Smith'
	},
	children: [
		{
			person_id: 'child1-id',
			relationship_type: 'biological',
			person: {
				id: 'child1-id',
				given_name: 'Alice',
				surname: 'Smith'
			}
		},
		{
			person_id: 'child2-id',
			relationship_type: 'adopted',
			person: {
				id: 'child2-id',
				given_name: 'Bob',
				surname: 'Smith'
			}
		}
	]
};

const mockFamilyNoChildren: apiModule.FamilyDetail = {
	id: 'test-family-id',
	partner1_id: 'partner1-id',
	partner2_id: 'partner2-id',
	partner1: { id: 'partner1-id', given_name: 'John', surname: 'Smith' },
	partner2: { id: 'partner2-id', given_name: 'Jane', surname: 'Smith' },
	version: 1,
	children: []
};

const mockEmptyHistory: apiModule.ChangeHistoryResponse = {
	items: [],
	total: 0,
	limit: 1,
	offset: 0,
	has_more: false
};

describe('Family Detail Page', () => {
	beforeEach(() => {
		vi.clearAllMocks();
		branchState.id = null;
		// Default mock for history - returns empty
		vi.mocked(apiModule.api.getFamilyHistory).mockResolvedValue(mockEmptyHistory);
	});

	it('renders loading state initially', () => {
		vi.mocked(apiModule.api.getFamily).mockReturnValue(new Promise(() => {}));

		render(FamilyPage);
		expect(screen.getByText('Loading...')).toBeDefined();
	});

	it('renders family with partners', async () => {
		vi.mocked(apiModule.api.getFamily).mockResolvedValue(mockFamilyWithChildren);

		render(FamilyPage);

		await waitFor(() => {
			expect(screen.getByText('John Smith & Jane Smith')).toBeDefined();
		});
	});

	it('renders marriage badge', async () => {
		vi.mocked(apiModule.api.getFamily).mockResolvedValue(mockFamilyWithChildren);

		render(FamilyPage);

		await waitFor(() => {
			expect(screen.getByText('marriage')).toBeDefined();
		});
	});

	it('renders children list with names', async () => {
		vi.mocked(apiModule.api.getFamily).mockResolvedValue(mockFamilyWithChildren);

		render(FamilyPage);

		await waitFor(() => {
			expect(screen.getByText('Children (2)')).toBeDefined();
			expect(screen.getByText('Alice Smith')).toBeDefined();
			expect(screen.getByText('Bob Smith')).toBeDefined();
		});
	});

	it('shows adopted badge for adopted children', async () => {
		vi.mocked(apiModule.api.getFamily).mockResolvedValue(mockFamilyWithChildren);

		render(FamilyPage);

		await waitFor(() => {
			expect(screen.getByText('(adopted)')).toBeDefined();
		});
	});

	it('does not show biological badge for biological children', async () => {
		vi.mocked(apiModule.api.getFamily).mockResolvedValue(mockFamilyWithChildren);

		const { container } = render(FamilyPage);

		await waitFor(() => {
			expect(screen.getByText('Alice Smith')).toBeDefined();
		});

		// Biological children should not have a type badge
		const childTypes = container.querySelectorAll('.child-type');
		expect(childTypes.length).toBe(1); // Only adopted child has badge
	});

	it('renders empty children message when no children', async () => {
		vi.mocked(apiModule.api.getFamily).mockResolvedValue(mockFamilyNoChildren);

		render(FamilyPage);

		await waitFor(() => {
			expect(screen.getByText('No children recorded')).toBeDefined();
		});
	});

	it('renders marriage place', async () => {
		vi.mocked(apiModule.api.getFamily).mockResolvedValue(mockFamilyWithChildren);

		render(FamilyPage);

		await waitFor(() => {
			expect(screen.getByText('Chicago, IL')).toBeDefined();
		});
	});

	it('renders partner links to person pages', async () => {
		vi.mocked(apiModule.api.getFamily).mockResolvedValue(mockFamilyWithChildren);

		const { container } = render(FamilyPage);

		await waitFor(() => {
			const partnerLinks = container.querySelectorAll('.partners-grid a');
			expect(partnerLinks.length).toBe(2);
			expect(partnerLinks[0].getAttribute('href')).toBe('/persons/partner1-id');
			expect(partnerLinks[1].getAttribute('href')).toBe('/persons/partner2-id');
		});
	});

	it('styles partner cards by gender like PersonCard', async () => {
		vi.mocked(apiModule.api.getFamily).mockResolvedValue({
			...mockFamilyWithChildren,
			partner1: { ...mockFamilyWithChildren.partner1!, gender: 'male' },
			partner2: { ...mockFamilyWithChildren.partner2!, gender: 'female' }
		});

		const { container } = render(FamilyPage);

		await waitFor(() => {
			const avatars = container.querySelectorAll('.partners-grid .avatar');
			expect(avatars.length).toBe(2);
			expect(avatars[0].getAttribute('data-gender')).toBe('male');
			expect(avatars[1].getAttribute('data-gender')).toBe('female');
		});
	});

	it('renders child links to person pages', async () => {
		vi.mocked(apiModule.api.getFamily).mockResolvedValue(mockFamilyWithChildren);

		const { container } = render(FamilyPage);

		await waitFor(() => {
			const childLinks = container.querySelectorAll('.children-list a');
			expect(childLinks.length).toBe(2);
			expect(childLinks[0].getAttribute('href')).toBe('/persons/child1-id');
			expect(childLinks[1].getAttribute('href')).toBe('/persons/child2-id');
		});
	});

	it('renders error state on API failure', async () => {
		vi.mocked(apiModule.api.getFamily).mockRejectedValue({ message: 'Family not found' });

		render(FamilyPage);

		await waitFor(() => {
			expect(screen.getByText('Family not found')).toBeDefined();
		});
	});

	it('renders back link to families list', async () => {
		vi.mocked(apiModule.api.getFamily).mockResolvedValue(mockFamilyWithChildren);

		const { container } = render(FamilyPage);

		await waitFor(() => {
			const backLink = container.querySelector('.back-link');
			expect(backLink).not.toBeNull();
			expect(backLink?.getAttribute('href')).toBe('/families');
		});
	});

	// #823: a family created on a branch used to blank the page, because the
	// history count ran in the same try as the family and 404'd on the mainline.
	it('still renders the family when the history count fails', async () => {
		branchState.id = 'branch-id';
		vi.spyOn(console, 'warn').mockImplementation(() => {});
		vi.mocked(apiModule.api.getFamily).mockResolvedValue(mockFamilyWithChildren);
		vi.mocked(apiModule.api.getFamilyHistory).mockRejectedValue({ message: 'Family not found' });

		render(FamilyPage);

		await waitFor(() => {
			expect(screen.getByText('John Smith & Jane Smith')).toBeDefined();
		});
		await waitFor(() => expect(apiModule.api.getFamilyHistory).toHaveBeenCalled());
		expect(screen.queryByText('Family not found')).toBeNull();
		expect(screen.getByRole('heading', { name: /History/ }).textContent?.trim()).toBe('History');
	});

	it('shows the history count on the badge', async () => {
		vi.mocked(apiModule.api.getFamily).mockResolvedValue(mockFamilyWithChildren);
		vi.mocked(apiModule.api.getFamilyHistory).mockResolvedValue({ ...mockEmptyHistory, total: 3 });

		render(FamilyPage);

		await waitFor(() => {
			expect(screen.getByRole('heading', { name: /History/ }).textContent).toContain('3');
		});
	});

	it('offers the Restore tab on the mainline', async () => {
		vi.mocked(apiModule.api.getFamily).mockResolvedValue(mockFamilyWithChildren);

		render(FamilyPage);
		await fireEvent.click(await screen.findByRole('button', { name: /History/ }));

		expect(screen.getByRole('button', { name: 'Restore' })).toBeDefined();
	});

	// The selected tab was shown only by a class (#1061).
	it('exposes the selected history tab with aria-pressed', async () => {
		vi.mocked(apiModule.api.getFamily).mockResolvedValue(mockFamilyWithChildren);

		render(FamilyPage);
		await fireEvent.click(await screen.findByRole('button', { name: /History/ }));

		const changeLog = screen.getByRole('button', { name: 'Change Log', pressed: true });
		const restore = screen.getByRole('button', { name: 'Restore', pressed: false });
		await fireEvent.click(restore);
		expect(restore.getAttribute('aria-pressed')).toBe('true');
		expect(changeLog.getAttribute('aria-pressed')).toBe('false');
		expect(changeLog.classList.contains('active')).toBe(false);
	});

	// #824: rollback is mainline-only, so on a branch the Restore tab and the
	// rollback dialog are withdrawn; the change log (the branch's view) stays.
	it('withdraws Restore and rollback on a branch', async () => {
		branchState.id = 'branch-id';
		vi.mocked(apiModule.api.getFamily).mockResolvedValue(mockFamilyWithChildren);

		render(FamilyPage);
		await fireEvent.click(await screen.findByRole('button', { name: /History/ }));

		expect(screen.getByText(/Restore points and rollback work on the mainline only/)).toBeDefined();
		expect(screen.queryByRole('button', { name: 'Restore' })).toBeNull();
		await waitFor(() =>
			expect(apiModule.api.getFamilyHistory).toHaveBeenCalledWith('test-family-id', {
				limit: 20,
				offset: 0
			})
		);
		expect(apiModule.api.getFamilyRestorePoints).not.toHaveBeenCalled();
	});
});

describe('Family Detail Page: partners and children (#826)', () => {
	const api = apiModule.api;

	beforeEach(() => {
		vi.clearAllMocks();
		branchState.id = null;
		vi.mocked(api.getFamilyHistory).mockResolvedValue(mockEmptyHistory);
		vi.mocked(api.getFamily).mockResolvedValue(mockFamilyWithChildren);
		vi.mocked(api.updateFamily).mockResolvedValue({ id: 'test-family-id', version: 2 });
		vi.mocked(api.removeChildFromFamily).mockResolvedValue(undefined);
		vi.mocked(api.searchPersons).mockResolvedValue({
			items: [
				{ id: 'partner3-id', given_name: 'Carl', surname: 'Jones' },
				{ id: 'child1-id', given_name: 'Alice', surname: 'Smith' }
			],
			total: 2
		});
	});

	async function keepEditing() {
		const dialog = await screen.findByRole('alertdialog');
		expect(dialog.textContent).toContain('Discard your unsaved changes?');
		await fireEvent.click(within(dialog).getByRole('button', { name: 'Keep editing' }));
		await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull());
	}

	async function discard() {
		const dialog = await screen.findByRole('alertdialog');
		await fireEvent.click(within(dialog).getByRole('button', { name: 'Discard' }));
		await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull());
	}

	async function openEdit() {
		render(FamilyPage);
		await screen.findByText('John Smith & Jane Smith');
		await fireEvent.click(screen.getByRole('button', { name: 'Edit' }));
	}

	it('moves focus into the form on Edit and back to Edit on Cancel', async () => {
		await openEdit();
		await waitFor(() =>
			expect(document.activeElement).toBe(screen.getByRole('button', { name: 'Clear Partner 1: John Smith' }))
		);

		await fireEvent.click(screen.getByRole('button', { name: 'Cancel' }));
		await waitFor(() => expect(document.activeElement).toBe(screen.getByRole('button', { name: 'Edit' })));
	});

	// Escape in a field used to drop every edit unasked (#1097).
	it('asks before Escape discards edits, and keeps them when declined', async () => {
		await openEdit();

		const date = screen.getByLabelText('Marriage Date') as HTMLInputElement;
		date.focus();
		await fireEvent.input(date, { target: { value: '1875' } });
		await fireEvent.keyDown(date, { key: 'Escape' });

		await keepEditing();
		expect((screen.getByLabelText('Marriage Date') as HTMLInputElement).value).toBe('1875');
	});

	it('asks before Escape discards a changed partner', async () => {
		await openEdit();

		await fireEvent.click(screen.getByRole('button', { name: 'Clear Partner 1: John Smith' }));
		const date = screen.getByLabelText('Marriage Date');
		date.focus();
		await fireEvent.keyDown(date, { key: 'Escape' });

		await keepEditing();
		expect(screen.getByLabelText('Marriage Date')).toBeDefined();
	});

	it('leaves edit mode on Escape without asking when nothing changed', async () => {
		await openEdit();

		const date = screen.getByLabelText('Marriage Date');
		date.focus();
		await fireEvent.keyDown(date, { key: 'Escape' });

		expect(screen.queryByRole('alertdialog')).toBeNull();
		expect(screen.queryByLabelText('Marriage Date')).toBeNull();
		await waitFor(() => expect(document.activeElement).toBe(screen.getByRole('button', { name: 'Edit' })));
	});

	// Escape on a form button (the form focuses Clear Partner 1 first) did nothing (#1139).
	it('leaves edit mode on Escape from a form button, after the discard prompt', async () => {
		await openEdit();

		await fireEvent.input(screen.getByLabelText('Marriage Date'), { target: { value: '1875' } });
		const clear = screen.getByRole('button', { name: 'Clear Partner 1: John Smith' });
		clear.focus();
		await fireEvent.keyDown(clear, { key: 'Escape' });

		await discard();
		expect(screen.queryByLabelText('Marriage Date')).toBeNull();
	});

	// An overlay's Escape also reached the page's cancel (#1139).
	it('keeps edit mode when Escape closes the search dropdown', async () => {
		vi.mocked(apiModule.api.searchPersons).mockResolvedValue({ items: [], total: 0 });
		await openEdit();
		await fireEvent.input(screen.getByLabelText('Marriage Date'), { target: { value: '1875' } });
		render(SearchBox);
		const search = screen.getByRole('combobox', { name: 'Search' });
		search.focus();
		await fireEvent.input(search, { target: { value: 'Smith' } });

		await fireEvent.keyDown(search, { key: 'Escape' });

		expect(search.getAttribute('aria-expanded')).toBe('false');
		expect(screen.queryByRole('alertdialog')).toBeNull();
		expect(screen.getByLabelText('Marriage Date')).toBeDefined();
	});

	it('keeps edit mode when Escape closes the accessibility panel', async () => {
		await openEdit();
		await fireEvent.input(screen.getByLabelText('Marriage Date'), { target: { value: '1875' } });
		const onClose = vi.fn();
		render(AccessibilityPanel, { open: true, onClose });
		const closeButton = screen.getByRole('button', { name: 'Close accessibility settings' });
		closeButton.focus();

		await fireEvent.keyDown(closeButton, { key: 'Escape' });

		expect(onClose).toHaveBeenCalled();
		expect(screen.queryByRole('alertdialog')).toBeNull();
		expect(screen.getByLabelText('Marriage Date')).toBeDefined();
	});

	// PersonSelector consumed every Escape, so a closed partner search swallowed cancel (#1139).
	it('leaves edit mode on Escape from a closed partner search, after the discard prompt', async () => {
		await openEdit();
		await fireEvent.click(screen.getByRole('button', { name: 'Clear Partner 1: John Smith' }));
		const partner = screen.getByRole('combobox', { name: 'Partner 1' });
		partner.focus();

		await fireEvent.keyDown(partner, { key: 'Escape' });

		await discard();
		expect(screen.queryByLabelText('Marriage Date')).toBeNull();
	});

	it('keeps edit mode when Escape closes the partner search results', async () => {
		await openEdit();
		await fireEvent.click(screen.getByRole('button', { name: 'Clear Partner 1: John Smith' }));
		const partner = screen.getByRole('combobox', { name: 'Partner 1' });
		partner.focus();
		await fireEvent.input(partner, { target: { value: 'Jones' } });
		await screen.findByRole('option', { name: /Carl Jones/ }, { timeout: 1500 });

		await fireEvent.keyDown(partner, { key: 'Escape' });

		await waitFor(() => expect(screen.queryByRole('listbox')).toBeNull());
		expect(screen.queryByRole('alertdialog')).toBeNull();
		expect(screen.getByLabelText('Marriage Date')).toBeDefined();
	});

	it('shows the current partners in the edit form', async () => {
		await openEdit();
		expect(screen.getByRole('button', { name: 'Clear Partner 1: John Smith' })).toBeDefined();
		expect(screen.getByRole('button', { name: 'Clear Partner 2: Jane Smith' })).toBeDefined();
	});

	it('sends no partner fields when the partners are untouched', async () => {
		await openEdit();
		await fireEvent.click(screen.getByRole('button', { name: 'Save Changes' }));

		await waitFor(() => expect(api.updateFamily).toHaveBeenCalled());
		const body = vi.mocked(api.updateFamily).mock.calls[0][1];
		expect(body).not.toHaveProperty('partner1_id');
		expect(body).not.toHaveProperty('partner2_id');
		expect(body).not.toHaveProperty('clear_partner1');
		expect(body).not.toHaveProperty('clear_partner2');
	});

	it('sends a cleared marriage place as an empty string, and no unchanged type', async () => {
		await openEdit();
		await fireEvent.input(screen.getByLabelText('Marriage Place'), { target: { value: '' } });
		await fireEvent.click(screen.getByRole('button', { name: 'Save Changes' }));

		await waitFor(() => expect(api.updateFamily).toHaveBeenCalled());
		const body = vi.mocked(api.updateFamily).mock.calls[0][1];
		expect(body.marriage_place).toBe('');
		expect(body.relationship_type).toBeUndefined();
	});

	it('clears a removed partner and sets a newly picked one', async () => {
		await openEdit();
		await fireEvent.click(screen.getByRole('button', { name: 'Clear Partner 2: Jane Smith' }));
		await fireEvent.click(screen.getByRole('button', { name: 'Clear Partner 1: John Smith' }));

		const input = screen.getByRole('combobox', { name: 'Partner 1' });
		await fireEvent.input(input, { target: { value: 'Jones' } });
		await fireEvent.click(await screen.findByRole('option', { name: /Carl Jones/ }, { timeout: 1500 }));
		// A child of the family is never offered as its partner.
		expect(screen.queryByRole('option', { name: /Alice Smith/ })).toBeNull();

		await fireEvent.click(screen.getByRole('button', { name: 'Save Changes' }));
		await waitFor(() =>
			expect(api.updateFamily).toHaveBeenCalledWith(
				'test-family-id',
				expect.objectContaining({ partner1_id: 'partner3-id', clear_partner2: true, version: 1 })
			)
		);
	});

	it('announces a save in the live region', async () => {
		await openEdit();
		await fireEvent.click(screen.getByRole('button', { name: 'Save Changes' }));
		await waitFor(() => expect(screen.getByTestId('announcer').textContent).toContain('Family saved'));
	});

	it('will not save a family with both partners cleared', async () => {
		await openEdit();
		expect(screen.queryByTestId('partner-required')).toBeNull();
		await fireEvent.click(screen.getByRole('button', { name: 'Clear Partner 1: John Smith' }));
		await fireEvent.click(screen.getByRole('button', { name: 'Clear Partner 2: Jane Smith' }));

		expect(screen.getByTestId('partner-required')).toBeDefined();
		const save = screen.getByRole('button', { name: 'Save Changes' }) as HTMLButtonElement;
		expect(save.disabled).toBe(true);
		await fireEvent.submit(save.closest('form')!);
		expect(api.updateFamily).not.toHaveBeenCalled();
	});

	it('keeps the form open and shows a refused save in it', async () => {
		vi.mocked(api.updateFamily).mockRejectedValue({ message: 'partner1 and partner2 must be different people' });
		await openEdit();
		await fireEvent.click(screen.getByRole('button', { name: 'Save Changes' }));

		expect((await screen.findByRole('alert')).textContent).toContain('must be different people');
		expect(screen.getByRole('button', { name: 'Save Changes' })).toBeDefined();
	});

	// An unreadable date is a dead end without a way to keep it as text (#978).
	it('offers to keep an unreadable marriage date as text and resubmits it as a phrase', async () => {
		vi.mocked(api.updateFamily).mockRejectedValueOnce({
			code: 'VALIDATION_ERROR',
			message: 'invalid input: marriage_date: "spring 1875" is not a recognized date',
			details: { field: 'marriage_date', unreadable: true }
		});
		await openEdit();
		await fireEvent.input(screen.getByLabelText('Marriage Date'), { target: { value: 'spring 1875' } });
		await fireEvent.click(screen.getByRole('button', { name: 'Save Changes' }));
		await fireEvent.click(await screen.findByRole('button', { name: 'Keep as text' }));

		await waitFor(() => expect(api.updateFamily).toHaveBeenCalledTimes(2));
		expect(vi.mocked(api.updateFamily).mock.calls[1][1].marriage_date).toBe('(spring 1875)');
	});

	// Family edit showed a rejected date below the form with the server's raw prefix (#1112).
	it('shows a rejected date beside its field, tied to it and focused', async () => {
		vi.mocked(api.updateFamily).mockRejectedValue({
			code: 'VALIDATION_ERROR',
			message: 'invalid input: marriage_date: "31 FEB 1900" is not a real calendar date',
			details: { field: 'marriage_date', unreadable: false }
		});
		await openEdit();
		await fireEvent.click(screen.getByRole('button', { name: 'Save Changes' }));

		const input = screen.getByLabelText('Marriage Date');
		const message = await screen.findByRole('alert');
		expect(message.textContent?.trim()).toBe('"31 FEB 1900" is not a real calendar date');
		expect(input.closest('.date-field')?.contains(message)).toBe(true);
		expect(input.getAttribute('aria-invalid')).toBe('true');
		expect(input.getAttribute('aria-describedby')).toBe(message.id);
		await waitFor(() => expect(document.activeElement).toBe(input));
	});

	it('offers no Keep as text for a refusal that is not an unreadable date', async () => {
		vi.mocked(api.updateFamily).mockRejectedValue({
			code: 'VALIDATION_ERROR',
			message: 'invalid input: marriage_date: "31 FEB 1900" is not a real calendar date',
			details: { field: 'marriage_date', unreadable: false }
		});
		await openEdit();
		await fireEvent.click(screen.getByRole('button', { name: 'Save Changes' }));

		expect((await screen.findByRole('alert')).textContent).toContain('is not a real calendar date');
		expect(screen.queryByRole('button', { name: 'Keep as text' })).toBeNull();
	});

	it('says what a converted marriage date was saved as', async () => {
		vi.mocked(api.getFamily)
			.mockResolvedValueOnce(mockFamilyWithChildren)
			.mockResolvedValue({ ...mockFamilyWithChildren, marriage_date: { raw: 'ABT 1875' }, version: 2 });
		await openEdit();
		await fireEvent.input(screen.getByLabelText('Marriage Date'), { target: { value: 'circa 1875' } });
		await fireEvent.click(screen.getByRole('button', { name: 'Save Changes' }));

		expect(await screen.findByText('Saved as ABT 1875 from “circa 1875”')).toBeDefined();
	});

	// A create that converted the date navigates here with what it stored (#996).
	it('says what a create stored a converted marriage date as', async () => {
		vi.mocked(api.getFamily).mockResolvedValue({ ...mockFamilyWithChildren, marriage_date: { raw: 'ABT 1875' } });
		pageState.savedAs = { marriage_date: { typed: 'circa 1875', stored: 'ABT 1875' } };
		try {
			render(FamilyPage);
			expect(await screen.findByText('Saved as ABT 1875 from “circa 1875”')).toBeDefined();
		} finally {
			delete pageState.savedAs;
		}
	});

	// Back/Forward replays that state after the date may have been edited (#996).
	it('drops a create note the stored marriage date no longer matches', async () => {
		vi.mocked(api.getFamily).mockResolvedValue({ ...mockFamilyWithChildren, marriage_date: { raw: '1876' } });
		pageState.savedAs = { marriage_date: { typed: 'circa 1875', stored: 'ABT 1875' } };
		try {
			render(FamilyPage);
			await screen.findByText('1876');
			expect(screen.queryByText(/Saved as/)).toBeNull();
		} finally {
			delete pageState.savedAs;
		}
	});

	it('offers Add child, which opens the dialog', async () => {
		render(FamilyPage);
		await fireEvent.click(await screen.findByRole('button', { name: 'Add child' }));
		expect(await screen.findByRole('dialog')).toBeDefined();
		expect(screen.getByText(/as a child of John Smith & Jane Smith/)).toBeDefined();
	});

	it('offers Add child when the family has no children yet', async () => {
		vi.mocked(api.getFamily).mockResolvedValue(mockFamilyNoChildren);
		render(FamilyPage);
		expect(await screen.findByRole('button', { name: 'Add child' })).toBeDefined();
	});

	it('removes a child only after confirmation, then re-reads the family', async () => {
		render(FamilyPage);
		await fireEvent.click(await screen.findByRole('button', { name: 'Remove Bob Smith from this family' }));

		const dialog = await screen.findByRole('alertdialog');
		expect(dialog.textContent).toContain('Bob Smith will no longer be recorded as a child');
		expect(api.removeChildFromFamily).not.toHaveBeenCalled();

		vi.mocked(api.getFamily).mockResolvedValue({
			...mockFamilyWithChildren,
			children: [mockFamilyWithChildren.children![0]]
		});
		await fireEvent.click(screen.getByRole('button', { name: 'Remove child' }));

		await waitFor(() => expect(api.removeChildFromFamily).toHaveBeenCalledWith('test-family-id', 'child2-id'));
		await waitFor(() => expect(screen.queryByText('Bob Smith')).toBeNull());
		await waitFor(() =>
			expect(screen.getByTestId('announcer').textContent).toContain('Bob Smith removed from this family')
		);
	});

	it('cancelling the confirmation removes nothing', async () => {
		render(FamilyPage);
		await fireEvent.click(await screen.findByRole('button', { name: 'Remove Alice Smith from this family' }));
		await screen.findByRole('alertdialog');
		await fireEvent.click(screen.getByRole('button', { name: 'Cancel' }));

		await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull());
		expect(api.removeChildFromFamily).not.toHaveBeenCalled();
	});

	it('shows a refused removal in the confirmation', async () => {
		vi.mocked(api.removeChildFromFamily).mockRejectedValue({ message: 'Branch is not active' });
		render(FamilyPage);
		await fireEvent.click(await screen.findByRole('button', { name: 'Remove Alice Smith from this family' }));
		await fireEvent.click(await screen.findByRole('button', { name: 'Remove child' }));

		expect((await screen.findByRole('alert')).textContent).toContain('Branch is not active');
		expect(screen.getByRole('alertdialog')).toBeDefined();
	});

	it('offers the child controls on a branch too', async () => {
		branchState.id = 'branch-1';
		render(FamilyPage);
		expect(await screen.findByRole('button', { name: 'Add child' })).toBeDefined();
		expect(screen.getByRole('button', { name: 'Remove Alice Smith from this family' })).toBeDefined();
	});
});

describe('Family Detail Page: failed actions (#899)', () => {
	const api = apiModule.api;

	beforeEach(() => {
		vi.clearAllMocks();
		branchState.id = null;
		vi.mocked(api.getFamilyHistory).mockResolvedValue(mockEmptyHistory);
		vi.mocked(api.getFamily).mockResolvedValue(mockFamilyWithChildren);
	});

	async function editMarriagePlace(value: string) {
		render(FamilyPage);
		await fireEvent.click(await screen.findByRole('button', { name: 'Edit' }));
		await fireEvent.input(screen.getByLabelText('Marriage Place'), { target: { value } });
		await fireEvent.click(screen.getByRole('button', { name: 'Save Changes' }));
	}

	it('compares the edits with the latest version on a 409, keeping the form', async () => {
		vi.mocked(api.updateFamily).mockRejectedValueOnce({ status: 409, code: 'VERSION_CONFLICT', message: 'Conflict' });
		vi.mocked(api.getFamily)
			.mockResolvedValueOnce(mockFamilyWithChildren)
			.mockResolvedValueOnce({ ...mockFamilyWithChildren, marriage_place: 'Springfield, IL', version: 2 });
		await editMarriagePlace('Evanston, IL');

		const row = (await screen.findByRole('rowheader', { name: 'Marriage Place' })).closest('tr')!;
		expect(row.textContent).toContain('Evanston, IL');
		expect(row.textContent).toContain('Springfield, IL');
		expect(screen.queryByRole('rowheader', { name: 'Partner 1' })).toBeNull();
		expect((screen.getByLabelText('Marriage Place') as HTMLInputElement).value).toBe('Evanston, IL');
	});

	it('saves the edits over the latest version when the user keeps them', async () => {
		vi.mocked(api.updateFamily)
			.mockRejectedValueOnce({ status: 409, code: 'VERSION_CONFLICT', message: 'Conflict' })
			.mockResolvedValueOnce({ id: 'test-family-id', version: 3 });
		vi.mocked(api.getFamily)
			.mockResolvedValueOnce(mockFamilyWithChildren)
			.mockResolvedValue({ ...mockFamilyWithChildren, marriage_place: 'Springfield, IL', version: 2 });
		await editMarriagePlace('Evanston, IL');

		await fireEvent.click(await screen.findByRole('button', { name: 'Save my edits' }));

		await waitFor(() => expect(api.updateFamily).toHaveBeenCalledTimes(2));
		expect(vi.mocked(api.updateFamily).mock.calls[1][1]).toMatchObject({
			marriage_place: 'Evanston, IL',
			version: 2
		});
	});

	it("keeps another writer's changes to the fields the user did not edit", async () => {
		vi.mocked(api.updateFamily)
			.mockRejectedValueOnce({ status: 409, code: 'VERSION_CONFLICT', message: 'Conflict' })
			.mockResolvedValueOnce({ id: 'test-family-id', version: 3 });
		const theirs = {
			...mockFamilyWithChildren,
			marriage_date: { raw: '2 FEB 1902' },
			partner2_id: 'partner3-id',
			partner2: { id: 'partner3-id', given_name: 'Mary', surname: 'Jones' },
			version: 2
		};
		vi.mocked(api.getFamily).mockResolvedValueOnce(mockFamilyWithChildren).mockResolvedValue(theirs);
		await editMarriagePlace('Evanston, IL');

		// Only the edited field is compared.
		expect(await screen.findByRole('rowheader', { name: 'Marriage Place' })).toBeDefined();
		expect(screen.queryByRole('rowheader', { name: 'Marriage Date' })).toBeNull();
		expect(screen.queryByRole('rowheader', { name: 'Partner 2' })).toBeNull();

		await fireEvent.click(screen.getByRole('button', { name: 'Save my edits' }));

		await waitFor(() => expect(api.updateFamily).toHaveBeenCalledTimes(2));
		const sent = vi.mocked(api.updateFamily).mock.calls[1][1];
		expect(sent).toMatchObject({ marriage_place: 'Evanston, IL', marriage_date: '2 FEB 1902', version: 2 });
		expect(sent).not.toHaveProperty('partner2_id');
		expect(sent).not.toHaveProperty('clear_partner2');
	});

	it('shows a 409 that is not a change elsewhere, such as circular ancestry, as it came', async () => {
		vi.mocked(api.updateFamily).mockRejectedValueOnce({
			status: 409,
			code: 'CONFLICT',
			message: 'Circular ancestry detected - this would create an impossible family tree'
		});
		await editMarriagePlace('Evanston, IL');

		expect((await screen.findByRole('alert')).textContent).toContain('Circular ancestry detected');
		// The code says it is a refusal, so the family is not re-read (#940).
		expect(api.getFamily).toHaveBeenCalledTimes(1);
		expect(screen.queryByText(/changed elsewhere/)).toBeNull();
		expect(screen.queryByRole('button', { name: 'Save my edits' })).toBeNull();
		expect((screen.getByLabelText('Marriage Place') as HTMLInputElement).value).toBe('Evanston, IL');
	});

	// The refusal used to arrive only after "This cannot be undone." (#1113).
	it('says a family with children cannot be deleted, instead of offering to', async () => {
		render(FamilyPage);
		await fireEvent.click(await screen.findByRole('button', { name: 'Delete' }));

		const dialog = await screen.findByRole('alertdialog');
		expect(dialog.textContent).toContain('This family still has children');
		expect(dialog.textContent).not.toContain('This cannot be undone');
		expect(within(dialog).queryByRole('button', { name: 'Delete' })).toBeNull();
		await fireEvent.click(within(dialog).getByRole('button', { name: 'Close' }));
		await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull());
		expect(api.deleteFamily).not.toHaveBeenCalled();
	});

	it('deletes a childless family once confirmed, then says so on the families list', async () => {
		vi.mocked(api.getFamily).mockResolvedValue(mockFamilyNoChildren);
		vi.mocked(api.deleteFamily).mockResolvedValue(undefined);
		render(FamilyPage);
		await fireEvent.click(await screen.findByRole('button', { name: 'Delete' }));

		const dialog = await screen.findByRole('alertdialog');
		expect(dialog.textContent).toContain('This cannot be undone');
		expect(api.deleteFamily).not.toHaveBeenCalled();
		await fireEvent.click(within(dialog).getByRole('button', { name: 'Delete' }));

		await waitFor(() =>
			expect(goto).toHaveBeenCalledWith('/families', {
				state: { notice: `The ${apiModule.formatFamilyName(mockFamilyNoChildren)} family was deleted.` }
			})
		);
	});

	// A refusal the loaded family did not foresee, e.g. a child added elsewhere.
	it('keeps the family, and the dialog open, when a delete is refused', async () => {
		vi.mocked(api.getFamily).mockResolvedValue(mockFamilyNoChildren);
		vi.mocked(api.deleteFamily).mockRejectedValueOnce({
			status: 409,
			message: 'Family has children and cannot be deleted'
		});
		render(FamilyPage);
		await fireEvent.click(await screen.findByRole('button', { name: 'Delete' }));
		const dialog = await screen.findByRole('alertdialog');
		await fireEvent.click(within(dialog).getByRole('button', { name: 'Delete' }));

		expect((await within(dialog).findByRole('alert')).textContent).toContain('has children');
		expect(goto).not.toHaveBeenCalled();
	});

	it('says a missing family is not found and offers Retry', async () => {
		vi.mocked(api.getFamily).mockRejectedValueOnce({ status: 404, message: 'Resource not found' });
		render(FamilyPage);

		expect((await screen.findByRole('alert')).textContent).toContain('This family could not be found');
		await fireEvent.click(screen.getByRole('button', { name: 'Retry' }));
		expect(await screen.findByText('John Smith & Jane Smith')).toBeDefined();
	});
});
