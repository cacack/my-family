import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/svelte';
import DashboardPage from './+page.svelte';
import * as apiModule from '$lib/api/client';

vi.mock('$lib/api/client', async (importOriginal) => {
	const actual = await importOriginal<typeof apiModule>();
	return {
		...actual,
		api: {
			listPersons: vi.fn(),
			listFamilies: vi.fn(),
			getDiscoveryFeed: vi.fn()
		}
	};
});

const { branchState, returnToMainline, onboarding } = vi.hoisted(() => ({
	branchState: { id: null as string | null, branch: null as { name: string } | null },
	returnToMainline: vi.fn(),
	onboarding: { completed: false }
}));

vi.mock('$lib/stores/activeBranch.svelte', () => ({
	activeBranch: branchState,
	returnToMainline
}));

vi.mock('$lib/stores/onboardingSettings.svelte', () => ({
	onboardingState: onboarding,
	setOnboardingCompleted: vi.fn()
}));

function emptyLists() {
	vi.mocked(apiModule.api.listPersons).mockResolvedValue({
		items: [],
		total: 0
	} as unknown as apiModule.PersonList);
	vi.mocked(apiModule.api.listFamilies).mockResolvedValue({
		items: [],
		total: 0
	} as unknown as Awaited<ReturnType<typeof apiModule.api.listFamilies>>);
}

describe('Dashboard', () => {
	beforeEach(() => {
		vi.clearAllMocks();
		branchState.id = null;
		branchState.branch = null;
		onboarding.completed = false;
		emptyLists();
		vi.mocked(apiModule.api.getDiscoveryFeed).mockResolvedValue({
			items: [],
			total: 0
		} as unknown as apiModule.DiscoveryFeedResponse);
	});

	it('shows the onboarding wizard for an empty mainline', async () => {
		render(DashboardPage);
		await waitFor(() => expect(screen.getByText('Welcome to My Family')).toBeTruthy());
	});

	it('never shows the onboarding wizard on a research branch, even with no people in view (#825)', async () => {
		branchState.id = 'b-1';
		render(DashboardPage);
		await waitFor(() => expect(screen.getByText('Recent People')).toBeTruthy());
		expect(screen.queryByText('Welcome to My Family')).toBeNull();
	});

	it('does not show the onboarding wizard on a branch when loading fails', async () => {
		branchState.id = 'b-1';
		vi.mocked(apiModule.api.listPersons).mockRejectedValue(new Error('boom'));
		const spy = vi.spyOn(console, 'error').mockImplementation(() => {});
		render(DashboardPage);
		await waitFor(() => expect(screen.getByText('Recent People')).toBeTruthy());
		expect(screen.queryByText('Welcome to My Family')).toBeNull();
		spy.mockRestore();
	});

	it('says which dashboard figures are mainline while a branch is active', async () => {
		branchState.id = 'b-1';
		render(DashboardPage);
		await waitFor(() => expect(screen.getByText('Recent People')).toBeTruthy());
		const note = screen.getByRole('note');
		expect(note.textContent).toContain('recent families and research suggestions still come from the mainline');
	});

	it('shows no mainline notice on the mainline', async () => {
		onboarding.completed = true;
		render(DashboardPage);
		await waitFor(() => expect(screen.getByText('Recent People')).toBeTruthy());
		expect(screen.queryByRole('note')).toBeNull();
	});
});
