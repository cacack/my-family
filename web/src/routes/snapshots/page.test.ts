import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, waitFor, fireEvent } from '@testing-library/svelte';
import Page from './+page.svelte';
import type * as apiModule from '$lib/api/client';
import type { Snapshot } from '$lib/api/client';

// Hoisted so the module mocks below (which vitest lifts above the imports) can
// close over them.
const { listSnapshots, createSnapshot, deleteSnapshot, goto } = vi.hoisted(() => ({
	listSnapshots: vi.fn(),
	createSnapshot: vi.fn(),
	deleteSnapshot: vi.fn(),
	goto: vi.fn()
}));

vi.mock('$lib/api/client', async (importOriginal) => {
	const actual = await importOriginal<typeof apiModule>();
	return {
		...actual,
		api: {
			listSnapshots: () => listSnapshots(),
			createSnapshot: (data: apiModule.SnapshotCreate) => createSnapshot(data),
			deleteSnapshot: (id: string) => deleteSnapshot(id)
		}
	};
});

vi.mock('$app/navigation', () => ({ goto: (href: string) => goto(href) }));

// The notice reads the active branch; the mainline is the default here.
vi.mock('$lib/stores/activeBranch.svelte', () => ({
	activeBranch: { id: null, branch: null, revalidating: false, notice: null }
}));

// Newest first, as the API returns them.
const courthouse: Snapshot = {
	id: '33333333-3333-3333-3333-333333333333',
	name: 'After courthouse trip',
	description: 'Probate files from Franklin County',
	position: 120,
	created_at: '2026-03-01T15:00:00Z'
};

const dna: Snapshot = {
	id: '22222222-2222-2222-2222-222222222222',
	name: 'Post-DNA results',
	position: 80,
	created_at: '2026-02-01T12:00:00Z'
};

const baseline: Snapshot = {
	id: '11111111-1111-1111-1111-111111111111',
	name: 'Pre-DNA results',
	description: 'Research state before DNA test results arrived',
	position: 42,
	created_at: '2026-01-15T10:30:00Z'
};

describe('Snapshots page', () => {
	beforeEach(() => {
		vi.clearAllMocks();
		listSnapshots.mockResolvedValue({ items: [courthouse, dna, baseline], total: 3 });
		createSnapshot.mockResolvedValue(courthouse);
		deleteSnapshot.mockResolvedValue(undefined);
	});

	// bits-ui releases its body-scroll lock on a 24ms timer; drain past it so the
	// cleanup runs while the DOM still exists (see branches/page.test.ts).
	afterEach(async () => {
		await new Promise((resolve) => setTimeout(resolve, 30));
	});

	it('lists each snapshot with its name, description and created date', async () => {
		render(Page);

		expect(await screen.findByRole('heading', { name: 'After courthouse trip' })).toBeDefined();
		expect(screen.getByRole('heading', { name: 'Post-DNA results' })).toBeDefined();
		expect(screen.getByRole('heading', { name: 'Pre-DNA results' })).toBeDefined();
		expect(screen.getByText('Probate files from Franklin County')).toBeDefined();
		expect(screen.getByText('Research state before DNA test results arrived')).toBeDefined();

		const created = document.querySelector(`time[datetime="${baseline.created_at}"]`);
		expect(created?.textContent).toMatch(/Jan 15, 2026/);
		expect(screen.getByText('position 42')).toBeDefined();
	});

	it('shows an empty state and no compare form when there are no snapshots', async () => {
		listSnapshots.mockResolvedValue({ items: [], total: 0 });

		render(Page);

		await screen.findByText('No snapshots yet');
		expect(screen.queryByRole('button', { name: /^Compare$/ })).toBeNull();
		expect(screen.getByRole('button', { name: /new snapshot/i })).toBeDefined();
	});

	it('offers no compare form for a single snapshot', async () => {
		listSnapshots.mockResolvedValue({ items: [baseline], total: 1 });

		render(Page);

		await screen.findByRole('heading', { name: 'Pre-DNA results' });
		expect(screen.queryByLabelText('From')).toBeNull();
		expect(screen.queryByRole('link', { name: /compare .* with the previous/i })).toBeNull();
	});

	it('surfaces a load failure', async () => {
		listSnapshots.mockRejectedValue({ status: 500, code: 'internal', message: 'boom' });

		render(Page);

		const alert = await screen.findByRole('alert');
		expect(alert.textContent).toContain('boom');
	});

	describe('create', () => {
		async function openDialog() {
			render(Page);
			await screen.findByRole('heading', { name: 'Pre-DNA results' });
			await fireEvent.click(screen.getByRole('button', { name: /new snapshot/i }));
			return screen.findByLabelText('Name');
		}

		it('creates a snapshot with a trimmed name and description, then reloads and announces it', async () => {
			const nameInput = await openDialog();
			await fireEvent.input(nameInput, { target: { value: '  After courthouse trip  ' } });
			await fireEvent.input(screen.getByLabelText('Description (optional)'), {
				target: { value: ' Probate files ' }
			});
			await fireEvent.click(screen.getByRole('button', { name: /^Create snapshot$/ }));

			await waitFor(() => {
				expect(createSnapshot).toHaveBeenCalledWith({
					name: 'After courthouse trip',
					description: 'Probate files'
				});
			});
			await waitFor(() => expect(listSnapshots).toHaveBeenCalledTimes(2));
			await waitFor(() => {
				expect(screen.getByTestId('announcer').textContent).toContain(
					'Snapshot After courthouse trip created.'
				);
			});
		});

		it('omits an empty description rather than sending ""', async () => {
			const nameInput = await openDialog();
			await fireEvent.input(nameInput, { target: { value: 'Milestone' } });
			await fireEvent.click(screen.getByRole('button', { name: /^Create snapshot$/ }));

			await waitFor(() => {
				expect(createSnapshot).toHaveBeenCalledWith({ name: 'Milestone' });
			});
		});

		it('refuses a blank or whitespace-only name', async () => {
			const nameInput = await openDialog();
			const submit = screen.getByRole('button', { name: /^Create snapshot$/ }) as HTMLButtonElement;
			expect(submit.disabled).toBe(true);

			await fireEvent.input(nameInput, { target: { value: '   ' } });
			expect(submit.disabled).toBe(true);
			expect(screen.getByText('The name cannot be only spaces.')).toBeDefined();
			expect(nameInput.getAttribute('aria-invalid')).toBe('true');

			await fireEvent.submit(nameInput.closest('form')!);
			expect(createSnapshot).not.toHaveBeenCalled();
		});

		it('accepts a full-length name with trailing whitespace, since the name is trimmed', async () => {
			const nameInput = await openDialog();
			const name = 'a'.repeat(100);
			await fireEvent.input(nameInput, { target: { value: `${name} ` } });
			await fireEvent.click(screen.getByRole('button', { name: /^Create snapshot$/ }));

			await waitFor(() => {
				expect(createSnapshot).toHaveBeenCalledWith({ name });
			});
		});

		it('keeps the dialog open and shows a server validation error', async () => {
			createSnapshot.mockRejectedValue({
				status: 400,
				code: 'validation_error',
				message: 'snapshot name exceeds 100 characters'
			});

			const nameInput = await openDialog();
			await fireEvent.input(nameInput, { target: { value: 'Milestone' } });
			await fireEvent.click(screen.getByRole('button', { name: /^Create snapshot$/ }));

			expect(await screen.findByText('snapshot name exceeds 100 characters')).toBeDefined();
			expect(screen.getByLabelText('Name')).toBeDefined();
		});
	});

	describe('delete', () => {
		it('asks for confirmation before deleting, then reloads and announces it', async () => {
			render(Page);
			await screen.findByRole('heading', { name: 'Pre-DNA results' });

			await fireEvent.click(screen.getByRole('button', { name: 'Delete snapshot Pre-DNA results' }));
			expect(await screen.findByText('Delete this snapshot?')).toBeDefined();
			expect(deleteSnapshot).not.toHaveBeenCalled();

			await fireEvent.click(screen.getByRole('button', { name: /^Delete snapshot$/ }));

			await waitFor(() => expect(deleteSnapshot).toHaveBeenCalledWith(baseline.id));
			await waitFor(() => expect(listSnapshots).toHaveBeenCalledTimes(2));
			await waitFor(() => {
				expect(screen.getByTestId('announcer').textContent).toContain(
					'Snapshot Pre-DNA results deleted.'
				);
			});
		});

		it('does nothing when the confirmation is cancelled', async () => {
			render(Page);
			await screen.findByRole('heading', { name: 'Pre-DNA results' });

			await fireEvent.click(screen.getByRole('button', { name: 'Delete snapshot Pre-DNA results' }));
			await fireEvent.click(await screen.findByRole('button', { name: /^Cancel$/ }));

			await waitFor(() => expect(screen.queryByText('Delete this snapshot?')).toBeNull());
			expect(deleteSnapshot).not.toHaveBeenCalled();
		});

		it('keeps the dialog open and shows a failure', async () => {
			deleteSnapshot.mockRejectedValue({ status: 500, code: 'internal', message: 'disk full' });

			render(Page);
			await screen.findByRole('heading', { name: 'Pre-DNA results' });

			await fireEvent.click(screen.getByRole('button', { name: 'Delete snapshot Pre-DNA results' }));
			await fireEvent.click(await screen.findByRole('button', { name: /^Delete snapshot$/ }));

			expect(await screen.findByText('disk full')).toBeDefined();
			expect(screen.getByText('Delete this snapshot?')).toBeDefined();
		});

		it('treats a 404 as already deleted and refreshes', async () => {
			deleteSnapshot.mockRejectedValue({ status: 404, code: 'not_found', message: 'Snapshot not found' });

			render(Page);
			await screen.findByRole('heading', { name: 'Pre-DNA results' });

			await fireEvent.click(screen.getByRole('button', { name: 'Delete snapshot Pre-DNA results' }));
			await fireEvent.click(await screen.findByRole('button', { name: /^Delete snapshot$/ }));

			await waitFor(() => expect(listSnapshots).toHaveBeenCalledTimes(2));
			expect(screen.queryByText('Snapshot not found')).toBeNull();
		});
	});

	describe('compare', () => {
		it('defaults to the two most recent snapshots, older to newer', async () => {
			render(Page);
			await screen.findByRole('heading', { name: 'Pre-DNA results' });

			expect((screen.getByLabelText('From') as HTMLSelectElement).value).toBe(dna.id);
			expect((screen.getByLabelText('To') as HTMLSelectElement).value).toBe(courthouse.id);
		});

		it('navigates to the comparison for the chosen pair', async () => {
			render(Page);
			await screen.findByRole('heading', { name: 'Pre-DNA results' });

			await fireEvent.change(screen.getByLabelText('From'), { target: { value: baseline.id } });
			await fireEvent.click(screen.getByRole('button', { name: /^Compare$/ }));

			expect(goto).toHaveBeenCalledWith(
				`/snapshots/compare?from=${baseline.id}&to=${courthouse.id}`
			);
		});

		it('refuses to compare a snapshot with itself', async () => {
			render(Page);
			await screen.findByRole('heading', { name: 'Pre-DNA results' });

			await fireEvent.change(screen.getByLabelText('From'), { target: { value: courthouse.id } });

			const compare = screen.getByRole('button', { name: /^Compare$/ }) as HTMLButtonElement;
			expect(compare.disabled).toBe(true);
			expect(screen.getByText('Choose two different snapshots.')).toBeDefined();
			await fireEvent.submit(compare.closest('form')!);
			expect(goto).not.toHaveBeenCalled();
		});

		it('links each snapshot to a comparison with the one before it', async () => {
			render(Page);
			await screen.findByRole('heading', { name: 'Pre-DNA results' });

			const link = screen.getByRole('link', {
				name: 'Compare After courthouse trip with the previous snapshot, Post-DNA results'
			});
			expect(link.getAttribute('href')).toBe(
				`/snapshots/compare?from=${dna.id}&to=${courthouse.id}`
			);
			// The oldest has nothing before it.
			expect(
				screen.queryByRole('link', { name: /Compare Pre-DNA results with the previous/ })
			).toBeNull();
		});
	});
});
