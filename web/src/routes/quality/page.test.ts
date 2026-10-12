import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor, fireEvent } from '@testing-library/svelte';
import Page from './+page.svelte';
import * as apiModule from '$lib/api/client';
import { back, currentPath, historyLength, resetRouter } from '$lib/test/fakeRouter';

vi.mock('$app/stores', async () => (await import('$lib/test/fakeRouter')).appStores);
vi.mock('$app/navigation', async () => (await import('$lib/test/fakeRouter')).appNavigation);

// Mock the API module
vi.mock('$lib/api/client', async (importOriginal) => {
	const actual = await importOriginal<typeof apiModule>();
	return {
		...actual,
		api: {
			getValidationIssues: vi.fn(),
			getPersonsDuplicates: vi.fn(),
			dismissDuplicate: vi.fn(),
			batchDismissDuplicates: vi.fn()
		}
	};
});

const mockValidationResponse: apiModule.ValidationIssuesResponse = {
	issues: [
		{
			severity: 'error',
			code: 'missing_name',
			message: 'Person has no name',
			record_id: '11111111-1111-1111-1111-111111111111'
		},
		{
			severity: 'warning',
			code: 'missing_dates',
			message: 'Person has no birth or death date'
		}
	],
	total: 2,
	error_count: 1,
	warning_count: 2,
	info_count: 3
};

const mockDuplicatesResponse: apiModule.DuplicatesResponse = {
	duplicates: [
		{
			person1_id: 'p1-aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa',
			person1_name: 'Jane Doe',
			person2_id: 'p2-bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb',
			person2_name: 'Jane Doh',
			confidence: 0.87,
			match_reasons: ['Same surname', 'Birth year within 1', 'Same birth place']
		},
		{
			person1_id: 'p3-cccccccc-cccc-cccc-cccc-cccccccccccc',
			person1_name: 'John Smith',
			person2_id: 'p4-dddddddd-dddd-dddd-dddd-dddddddddddd',
			person2_name: 'Jon Smith',
			confidence: 0.72,
			match_reasons: ['Same surname']
		}
	],
	total: 2
};

const mockBatchDismissResponse: apiModule.BatchDismissResponse = {
	total: 1,
	successful: 1,
	failed: 0,
	results: [
		{
			person1_id: 'p1-aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa',
			person2_id: 'p2-bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb',
			success: true
		}
	]
};

describe('Quality landing page', () => {
	beforeEach(() => {
		vi.clearAllMocks();
		resetRouter('/quality');
		vi.mocked(apiModule.api.getValidationIssues).mockResolvedValue(mockValidationResponse);
		vi.mocked(apiModule.api.getPersonsDuplicates).mockResolvedValue(mockDuplicatesResponse);
		vi.mocked(apiModule.api.dismissDuplicate).mockResolvedValue(undefined);
		vi.mocked(apiModule.api.batchDismissDuplicates).mockResolvedValue(mockBatchDismissResponse);
	});

	it('renders both tab triggers', async () => {
		render(Page);
		expect(screen.getByRole('tab', { name: /validation issues/i })).toBeDefined();
		expect(screen.getByRole('tab', { name: /duplicates/i })).toBeDefined();
	});

	it('loads validation issues with no severity filter on mount', async () => {
		render(Page);
		await waitFor(() => {
			expect(apiModule.api.getValidationIssues).toHaveBeenCalledWith({
				severity: undefined,
				limit: 20,
				offset: 0
			});
		});
	});

	it('refetches with the selected severity when a filter pill is clicked', async () => {
		render(Page);

		// Initial unfiltered load establishes the count baseline.
		await waitFor(() => {
			expect(apiModule.api.getValidationIssues).toHaveBeenCalledWith({
				severity: undefined,
				limit: 20,
				offset: 0
			});
		});

		const errorsPill = await screen.findByRole('button', { name: /^Errors \(/ });
		await fireEvent.click(errorsPill);

		await waitFor(() => {
			expect(apiModule.api.getValidationIssues).toHaveBeenCalledWith({
				severity: 'error',
				limit: 20,
				offset: 0
			});
		});

		const warningsPill = await screen.findByRole('button', { name: /^Warnings \(/ });
		await fireEvent.click(warningsPill);

		await waitFor(() => {
			expect(apiModule.api.getValidationIssues).toHaveBeenCalledWith({
				severity: 'warning',
				limit: 20,
				offset: 0
			});
		});

		const infoPill = await screen.findByRole('button', { name: /^Info \(/ });
		await fireEvent.click(infoPill);

		await waitFor(() => {
			expect(apiModule.api.getValidationIssues).toHaveBeenCalledWith({
				severity: 'info',
				limit: 20,
				offset: 0
			});
		});
	});

	it('sends a batch dismiss request when a duplicate is selected and "Dismiss selected" is clicked', async () => {
		render(Page);

		// Switch to duplicates tab.
		const duplicatesTab = screen.getByRole('tab', { name: /duplicates/i });
		await fireEvent.click(duplicatesTab);

		await waitFor(() => {
			expect(apiModule.api.getPersonsDuplicates).toHaveBeenCalled();
		});

		// Wait for the duplicates list to render.
		await screen.findByText('Jane Doe');

		// Click the first per-row checkbox (skip the page-wide select-all).
		const rowCheckbox = await screen.findByRole('checkbox', {
			name: /Select duplicate pair Jane Doe and Jane Doh/i
		});
		await fireEvent.click(rowCheckbox);

		// The bulk-action toolbar appears.
		const dismissButton = await screen.findByRole('button', { name: /^Dismiss selected$/ });
		await fireEvent.click(dismissButton);

		await waitFor(() => {
			expect(apiModule.api.batchDismissDuplicates).toHaveBeenCalledWith({
				dismissals: [
					{
						person1_id: 'p1-aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa',
						person2_id: 'p2-bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb'
					}
				]
			});
		});
	});

	const manyIssues: apiModule.ValidationIssuesResponse = { ...mockValidationResponse, total: 45 };

	it('restores the tab, page and severity filter from the URL (#946)', async () => {
		vi.mocked(apiModule.api.getValidationIssues).mockResolvedValue(manyIssues);
		resetRouter('/quality?severity=warning&page=2');
		render(Page);
		await waitFor(() =>
			expect(apiModule.api.getValidationIssues).toHaveBeenCalledWith({
				severity: 'warning',
				limit: 20,
				offset: 20
			})
		);
		await screen.findByText('Page 2 of 3');
		expect(screen.getByRole('button', { name: /^Warnings/ }).getAttribute('aria-pressed')).toBe('true');
	});

	it('pushes page and tab changes, so Back steps through them (#946)', async () => {
		vi.mocked(apiModule.api.getValidationIssues).mockResolvedValue(manyIssues);
		render(Page);
		await screen.findByText('Page 1 of 3');

		await fireEvent.click(screen.getByRole('button', { name: 'Next' }));
		await waitFor(() => expect(currentPath()).toBe('/quality?page=2'));
		await screen.findByText('Page 2 of 3');

		await fireEvent.click(screen.getByRole('tab', { name: /duplicates/i }));
		await waitFor(() => expect(currentPath()).toBe('/quality?tab=duplicates'));
		expect(historyLength()).toBe(3);
		await screen.findByText('Jane Doe');

		back();
		await screen.findByText('Page 2 of 3');
		expect(screen.getByRole('tab', { name: /validation/i }).getAttribute('aria-selected')).toBe('true');
	});

	it('replaces the URL on a severity change and starts at page 1 (#946)', async () => {
		resetRouter('/quality?page=2');
		render(Page);
		await fireEvent.click(await screen.findByRole('button', { name: /^Errors/ }));
		await waitFor(() => expect(currentPath()).toBe('/quality?severity=error'));
		expect(historyLength()).toBe(1);

		await fireEvent.click(screen.getByRole('button', { name: /^All/ }));
		await waitFor(() => expect(currentPath()).toBe('/quality'));
	});

	it('moves a page past the end to the last page, replacing the URL (#946)', async () => {
		resetRouter('/quality?tab=duplicates&page=9');
		render(Page);
		await waitFor(() => expect(currentPath()).toBe('/quality?tab=duplicates'));
		expect(historyLength()).toBe(1);
		await waitFor(() =>
			expect(apiModule.api.getPersonsDuplicates).toHaveBeenLastCalledWith({ limit: 20, offset: 0 })
		);
		await screen.findByText('Jane Doe');
	});
});
