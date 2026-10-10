import { describe, it, expect, vi, beforeEach } from 'vitest';
import { fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import FamilyPage from './+page.svelte';
import * as apiModule from '$lib/api/client';

const { branchState } = vi.hoisted(() => ({
	// The real store exposes a read-only view, so the active branch is injected.
	branchState: { id: null as string | null }
}));

vi.mock('$lib/stores/activeBranch.svelte', () => ({
	activeBranch: branchState
}));

// Mock the API module
vi.mock('$lib/api/client', async (importOriginal) => {
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
vi.mock('$app/stores', () => ({
	page: {
		subscribe: vi.fn((callback) => {
			callback({ params: { id: 'test-family-id' } });
			return () => {};
		})
	}
}));

// Mock navigation
vi.mock('$app/navigation', () => ({
	goto: vi.fn()
}));

const mockFamilyWithChildren: apiModule.FamilyDetail = {
	id: 'test-family-id',
	partner1_id: 'partner1-id',
	partner1_name: 'John Smith',
	partner2_id: 'partner2-id',
	partner2_name: 'Jane Smith',
	relationship_type: 'marriage',
	marriage_place: 'Chicago, IL',
	child_count: 2,
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
	partner1_name: 'John Smith',
	partner2_name: 'Jane Smith',
	child_count: 0,
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
			const partnerLinks = container.querySelectorAll('a.partner-card');
			expect(partnerLinks.length).toBe(2);
			expect(partnerLinks[0].getAttribute('href')).toBe('/persons/partner1-id');
			expect(partnerLinks[1].getAttribute('href')).toBe('/persons/partner2-id');
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
