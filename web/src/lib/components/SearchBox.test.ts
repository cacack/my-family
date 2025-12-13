import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/svelte';
import SearchBox from './SearchBox.svelte';
import * as apiModule from '$lib/api/client';

// Mock the API module
vi.mock('$lib/api/client', async (importOriginal) => {
	const actual = await importOriginal<typeof apiModule>();
	return {
		...actual,
		api: {
			searchPersons: vi.fn()
		}
	};
});

const mockSearchResults = {
	items: [
		{
			id: '1',
			given_name: 'John',
			surname: 'Doe',
			birth_date: { year: 1950 },
			death_date: { year: 2020 }
		},
		{
			id: '2',
			given_name: 'Jane',
			surname: 'Doe',
			birth_date: { year: 1955 }
		}
	],
	total: 2
};

describe('SearchBox', () => {
	beforeEach(() => {
		vi.useFakeTimers();
		vi.clearAllMocks();
	});

	afterEach(() => {
		vi.useRealTimers();
	});

	it('renders the search input', () => {
		render(SearchBox);
		const input = screen.getByRole('textbox');
		expect(input).toBeDefined();
	});

	it('uses custom placeholder', () => {
		render(SearchBox, { props: { placeholder: 'Find ancestors...' } });
		const input = screen.getByPlaceholderText('Find ancestors...');
		expect(input).toBeDefined();
	});

	it('has search icon', () => {
		const { container } = render(SearchBox);
		const icon = container.querySelector('.search-icon');
		expect(icon).not.toBeNull();
	});

	it('has fuzzy toggle button', () => {
		const { container } = render(SearchBox);
		const fuzzyButton = container.querySelector('.fuzzy-toggle');
		expect(fuzzyButton).not.toBeNull();
		expect(fuzzyButton?.textContent).toBe('~');
	});

	it('toggles fuzzy mode when button is clicked', async () => {
		const { container } = render(SearchBox);
		const fuzzyButton = container.querySelector('.fuzzy-toggle') as HTMLButtonElement;

		expect(fuzzyButton.classList.contains('active')).toBe(false);

		await fireEvent.click(fuzzyButton);

		expect(fuzzyButton.classList.contains('active')).toBe(true);
	});

	it('debounces search input', async () => {
		vi.mocked(apiModule.api.searchPersons).mockResolvedValue(mockSearchResults);

		render(SearchBox);
		const input = screen.getByRole('textbox');

		// Type a search query
		await fireEvent.input(input, { target: { value: 'John' } });

		// API should not be called immediately
		expect(apiModule.api.searchPersons).not.toHaveBeenCalled();

		// Fast-forward debounce timer
		await vi.advanceTimersByTimeAsync(300);

		// Now API should be called
		expect(apiModule.api.searchPersons).toHaveBeenCalledWith({
			q: 'John',
			fuzzy: false,
			limit: 10
		});
	});

	it('does not search for queries shorter than 2 characters', async () => {
		vi.mocked(apiModule.api.searchPersons).mockResolvedValue(mockSearchResults);

		render(SearchBox);
		const input = screen.getByRole('textbox');

		await fireEvent.input(input, { target: { value: 'J' } });
		await vi.advanceTimersByTimeAsync(300);

		expect(apiModule.api.searchPersons).not.toHaveBeenCalled();
	});

	it('shows dropdown with results', async () => {
		vi.mocked(apiModule.api.searchPersons).mockResolvedValue(mockSearchResults);

		const { container } = render(SearchBox);
		const input = screen.getByRole('textbox');

		await fireEvent.input(input, { target: { value: 'Doe' } });
		await vi.advanceTimersByTimeAsync(300);

		await waitFor(() => {
			const dropdown = container.querySelector('.dropdown');
			expect(dropdown).not.toBeNull();
		});

		// Check result items
		const resultItems = container.querySelectorAll('.result-item');
		expect(resultItems.length).toBe(2);
	});

	it('shows no results message when empty', async () => {
		vi.mocked(apiModule.api.searchPersons).mockResolvedValue({ items: [], total: 0 });

		const { container } = render(SearchBox);
		const input = screen.getByRole('textbox');

		await fireEvent.input(input, { target: { value: 'xyz' } });
		await vi.advanceTimersByTimeAsync(300);

		await waitFor(() => {
			const noResults = container.querySelector('.no-results');
			expect(noResults).not.toBeNull();
			expect(noResults?.textContent).toBe('No results found');
		});
	});

	it('calls onSelect when result is clicked', async () => {
		vi.mocked(apiModule.api.searchPersons).mockResolvedValue(mockSearchResults);
		const selectHandler = vi.fn();

		const { container } = render(SearchBox, { props: { onSelect: selectHandler } });
		const input = screen.getByRole('textbox');

		await fireEvent.input(input, { target: { value: 'Doe' } });
		await vi.advanceTimersByTimeAsync(300);

		await waitFor(() => {
			const resultItems = container.querySelectorAll('.result-item');
			expect(resultItems.length).toBe(2);
		});

		const firstResult = container.querySelector('.result-item') as HTMLButtonElement;
		await fireEvent.click(firstResult);

		expect(selectHandler).toHaveBeenCalledWith(mockSearchResults.items[0]);
	});

	it('hides dropdown on escape key', async () => {
		vi.mocked(apiModule.api.searchPersons).mockResolvedValue(mockSearchResults);

		const { container } = render(SearchBox);
		const input = screen.getByRole('textbox');

		await fireEvent.input(input, { target: { value: 'Doe' } });
		await vi.advanceTimersByTimeAsync(300);

		await waitFor(() => {
			expect(container.querySelector('.dropdown')).not.toBeNull();
		});

		await fireEvent.keyDown(input, { key: 'Escape' });

		await waitFor(() => {
			expect(container.querySelector('.dropdown')).toBeNull();
		});
	});

	it('displays formatted names in results', async () => {
		vi.mocked(apiModule.api.searchPersons).mockResolvedValue(mockSearchResults);

		const { container } = render(SearchBox);
		const input = screen.getByRole('textbox');

		await fireEvent.input(input, { target: { value: 'Doe' } });
		await vi.advanceTimersByTimeAsync(300);

		await waitFor(() => {
			const names = container.querySelectorAll('.name');
			expect(names[0]?.textContent).toBe('John Doe');
			expect(names[1]?.textContent).toBe('Jane Doe');
		});
	});

	it('displays lifespan in results', async () => {
		vi.mocked(apiModule.api.searchPersons).mockResolvedValue(mockSearchResults);

		const { container } = render(SearchBox);
		const input = screen.getByRole('textbox');

		await fireEvent.input(input, { target: { value: 'Doe' } });
		await vi.advanceTimersByTimeAsync(300);

		await waitFor(() => {
			const lifespans = container.querySelectorAll('.lifespan');
			expect(lifespans[0]?.textContent).toBe('(1950–2020)');
			expect(lifespans[1]?.textContent).toBe('(b. 1955)');
		});
	});

	it('shows loading indicator during search', async () => {
		// Create a delayed response
		let resolveSearch: (value: typeof mockSearchResults) => void;
		const searchPromise = new Promise<typeof mockSearchResults>((resolve) => {
			resolveSearch = resolve;
		});
		vi.mocked(apiModule.api.searchPersons).mockReturnValue(searchPromise);

		const { container } = render(SearchBox);
		const input = screen.getByRole('textbox');

		await fireEvent.input(input, { target: { value: 'Doe' } });
		await vi.advanceTimersByTimeAsync(300);

		// Should show loading indicator
		await waitFor(() => {
			const loadingIndicator = container.querySelector('.loading-indicator');
			expect(loadingIndicator).not.toBeNull();
		});

		// Resolve the search
		resolveSearch!(mockSearchResults);

		// Loading indicator should disappear
		await waitFor(() => {
			const loadingIndicator = container.querySelector('.loading-indicator');
			expect(loadingIndicator).toBeNull();
		});
	});

	it('has correct aria attributes', () => {
		render(SearchBox);
		const input = screen.getByRole('textbox');

		expect(input.getAttribute('aria-label')).toBe('Search');
		expect(input.getAttribute('aria-haspopup')).toBe('listbox');
		expect(input.getAttribute('aria-expanded')).toBe('false');
	});

	it('searches with fuzzy mode when enabled', async () => {
		vi.mocked(apiModule.api.searchPersons).mockResolvedValue(mockSearchResults);

		const { container } = render(SearchBox);
		const fuzzyButton = container.querySelector('.fuzzy-toggle') as HTMLButtonElement;
		const input = screen.getByRole('textbox');

		// Enable fuzzy mode
		await fireEvent.click(fuzzyButton);

		// Type and search
		await fireEvent.input(input, { target: { value: 'Doe' } });
		await vi.advanceTimersByTimeAsync(300);

		expect(apiModule.api.searchPersons).toHaveBeenCalledWith({
			q: 'Doe',
			fuzzy: true,
			limit: 10
		});
	});
});
