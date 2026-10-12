import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/svelte';
import SourcesPage from './+page.svelte';
import * as apiModule from '$lib/api/client';
import { back, currentPath, historyLength, resetRouter } from '$lib/test/fakeRouter';

vi.mock('$app/stores', async () => (await import('$lib/test/fakeRouter')).appStores);
vi.mock('$app/navigation', async () => (await import('$lib/test/fakeRouter')).appNavigation);

vi.mock('$lib/api/client', async (importOriginal) => {
	const actual = await importOriginal<typeof apiModule>();
	return {
		...actual,
		api: {
			listSources: vi.fn(),
			createSource: vi.fn()
		}
	};
});

// domain.SourceType: the API rejects every other value.
const API_SOURCE_TYPES = [
	'book', 'archive', 'webpage', 'census', 'vital_record', 'church_record',
	'newspaper', 'photograph', 'interview', 'correspondence', 'other'
];

describe('Sources page', () => {
	beforeEach(() => {
		vi.clearAllMocks();
		resetRouter('/sources');
		vi.mocked(apiModule.api.listSources).mockResolvedValue({ sources: [], total: 0, limit: 20, offset: 0 });
		vi.mocked(apiModule.api.createSource).mockResolvedValue({
			id: 's-1',
			source_type: 'other',
			title: 'Parish register',
			version: 1
		} as apiModule.Source);
	});

	it('offers exactly the source types the API accepts and creates with a valid default', async () => {
		render(SourcesPage);
		await screen.findByText('No sources found.');
		await fireEvent.click(screen.getAllByRole('button', { name: 'Add Source' })[0]);

		const select = screen.getByLabelText('Source Type') as HTMLSelectElement;
		expect(Array.from(select.options).map((o) => o.value).sort()).toEqual([...API_SOURCE_TYPES].sort());

		await fireEvent.input(screen.getByLabelText(/Title/), { target: { value: 'Parish register' } });
		await fireEvent.click(screen.getByRole('button', { name: 'Create Source' }));

		await waitFor(() => expect(apiModule.api.createSource).toHaveBeenCalled());
		const body = vi.mocked(apiModule.api.createSource).mock.calls[0][0];
		expect(API_SOURCE_TYPES).toContain(body.source_type);
	});

	it('names the search box and says the current sort direction', async () => {
		render(SourcesPage);
		expect(screen.getByRole('textbox', { name: 'Search sources' })).toBeTruthy();
		await fireEvent.click(screen.getByRole('button', { name: 'Sort order: ascending' }));
		await screen.findByRole('button', { name: 'Sort order: descending' });
	});

	it('shows a load failure as an error with Retry, not as an empty list', async () => {
		vi.mocked(apiModule.api.listSources).mockRejectedValueOnce(new Error('boom'));
		const spy = vi.spyOn(console, 'error').mockImplementation(() => {});
		render(SourcesPage);

		const alert = await screen.findByRole('alert');
		expect(alert.textContent).toContain('Failed to load sources');
		expect(screen.queryByText('No sources found.')).toBeNull();

		await fireEvent.click(screen.getByRole('button', { name: 'Retry' }));
		await screen.findByText('No sources found.');
		spy.mockRestore();
	});

	const source = (i: number) => ({ id: `s-${i}`, source_type: 'book', title: `Source ${i}`, version: 1 }) as apiModule.Source;
	const manySources = { sources: [source(1)], total: 45, limit: 20, offset: 0 };

	it('restores search, sort and page from the URL (#946)', async () => {
		vi.mocked(apiModule.api.listSources).mockResolvedValue(manySources);
		resetRouter('/sources?q=parish&sort=author&order=desc&page=2');
		render(SourcesPage);
		await waitFor(() =>
			expect(apiModule.api.listSources).toHaveBeenCalledWith({
				limit: 20,
				offset: 20,
				sort: 'author',
				order: 'desc',
				q: 'parish'
			})
		);
		expect((screen.getByRole('textbox', { name: 'Search sources' }) as HTMLInputElement).value).toBe('parish');
		await screen.findByText('Page 2 of 3');
	});

	it('pushes a page change, so Back returns to the previous page (#946)', async () => {
		vi.mocked(apiModule.api.listSources).mockResolvedValue(manySources);
		render(SourcesPage);
		await screen.findByText('Page 1 of 3');

		await fireEvent.click(screen.getByRole('button', { name: 'Next' }));
		await waitFor(() => expect(currentPath()).toBe('/sources?page=2'));
		expect(historyLength()).toBe(2);
		await screen.findByText('Page 2 of 3');

		back();
		await screen.findByText('Page 1 of 3');
	});

	it('replaces the URL with a search after a pause and starts at page 1 (#946)', async () => {
		resetRouter('/sources?page=3');
		render(SourcesPage);
		const box = screen.getByRole('textbox', { name: 'Search sources' }) as HTMLInputElement;
		await fireEvent.input(box, { target: { value: 'census' } });
		await waitFor(() => expect(currentPath()).toBe('/sources?q=census'));
		expect(historyLength()).toBe(1);
		expect(box.value).toBe('census');
		await waitFor(() =>
			expect(apiModule.api.listSources).toHaveBeenLastCalledWith(
				expect.objectContaining({ q: 'census', offset: 0 })
			)
		);

		await fireEvent.click(await screen.findByRole('button', { name: 'Clear Search' }));
		await waitFor(() => expect(currentPath()).toBe('/sources'));
		expect(box.value).toBe('');
	});

	it('drops a pending search when the page is left (#946)', async () => {
		vi.useFakeTimers();
		try {
			const { unmount } = render(SourcesPage);
			const box = screen.getByRole('textbox', { name: 'Search sources' });
			await fireEvent.input(box, { target: { value: 'census' } });
			unmount();
			resetRouter('/people');
			await vi.advanceTimersByTimeAsync(500);
			expect(currentPath()).toBe('/people');
		} finally {
			vi.useRealTimers();
		}
	});
});

