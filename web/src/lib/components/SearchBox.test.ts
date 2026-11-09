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
		const input = screen.getByRole('combobox');
		expect(input).toBeDefined();
	});

	it('uses custom placeholder', () => {
		render(SearchBox, { props: { placeholder: 'Find ancestors...' } });
		const input = screen.getByPlaceholderText('Find ancestors...');
		expect(input).toBeDefined();
	});

	// The magnifier used to be a decorative icon that did nothing (#1022).
	it('re-runs the search when the Search button is clicked', async () => {
		vi.mocked(apiModule.api.searchPersons).mockResolvedValue(mockSearchResults);
		const { container } = render(SearchBox);
		const input = screen.getByRole('combobox');
		await fireEvent.input(input, { target: { value: 'Doe' } });
		await vi.advanceTimersByTimeAsync(300);
		await fireEvent.keyDown(input, { key: 'Escape' });
		expect(container.querySelector('.dropdown')).toBeNull();

		await fireEvent.click(screen.getByRole('button', { name: 'Search' }));

		expect(apiModule.api.searchPersons).toHaveBeenCalledTimes(2);
		await waitFor(() => expect(container.querySelectorAll('.result-item').length).toBe(2));
	});

	it('does not search from the Search button with under 2 characters', async () => {
		render(SearchBox);
		await fireEvent.input(screen.getByRole('combobox'), { target: { value: 'D' } });

		await fireEvent.click(screen.getByRole('button', { name: 'Search' }));

		expect(apiModule.api.searchPersons).not.toHaveBeenCalled();
	});

	it('reopens the results when a filled box is focused again', async () => {
		vi.mocked(apiModule.api.searchPersons).mockResolvedValue(mockSearchResults);
		const { container } = render(SearchBox);
		const input = screen.getByRole('combobox');
		await fireEvent.input(input, { target: { value: 'Doe' } });
		await vi.advanceTimersByTimeAsync(300);
		await waitFor(() => expect(container.querySelector('.result-item')).not.toBeNull());
		// Picking a result clears the held results but keeps the text.
		await fireEvent.click(container.querySelector('.result-item') as HTMLButtonElement);
		await fireEvent.blur(input);
		await vi.advanceTimersByTimeAsync(200);
		expect(container.querySelector('.dropdown')).toBeNull();

		await fireEvent.focus(input);

		expect(apiModule.api.searchPersons).toHaveBeenLastCalledWith({ q: 'John Doe', fuzzy: false, limit: 10 });
		await waitFor(() => expect(container.querySelectorAll('.result-item').length).toBe(2));
	});

	it('runs the search on Enter when no result is highlighted', async () => {
		vi.mocked(apiModule.api.searchPersons).mockResolvedValue(mockSearchResults);
		const { container } = render(SearchBox);
		const input = screen.getByRole('combobox');
		await fireEvent.input(input, { target: { value: 'Doe' } });

		const enter = new KeyboardEvent('keydown', { key: 'Enter', cancelable: true, bubbles: true });
		input.dispatchEvent(enter);

		// At once, without waiting out the debounce
		expect(apiModule.api.searchPersons).toHaveBeenCalledWith({ q: 'Doe', fuzzy: false, limit: 10 });
		expect(enter.defaultPrevented).toBe(true);
		await waitFor(() => expect(container.querySelectorAll('.result-item').length).toBe(2));
		await vi.advanceTimersByTimeAsync(300);
		expect(apiModule.api.searchPersons).toHaveBeenCalledTimes(1);
	});

	it('selects the highlighted result on Enter', async () => {
		// jsdom has no scrollIntoView; the highlight scrolls into view
		Element.prototype.scrollIntoView = vi.fn();
		vi.mocked(apiModule.api.searchPersons).mockResolvedValue(mockSearchResults);
		const onSelect = vi.fn();
		const { container } = render(SearchBox, { props: { onSelect } });
		const input = screen.getByRole('combobox');
		await fireEvent.input(input, { target: { value: 'Doe' } });
		await vi.advanceTimersByTimeAsync(300);
		await waitFor(() => expect(container.querySelector('.result-item')).not.toBeNull());

		await fireEvent.keyDown(input, { key: 'ArrowDown' });
		await fireEvent.keyDown(input, { key: 'Enter' });

		expect(onSelect).toHaveBeenCalledWith(mockSearchResults.items[0]);
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

	it('names the fuzzy toggle by purpose and exposes its state', async () => {
		render(SearchBox);
		const fuzzyButton = screen.getByRole('button', { name: 'Fuzzy search' });

		expect(fuzzyButton.getAttribute('aria-pressed')).toBe('false');

		await fireEvent.click(fuzzyButton);

		expect(fuzzyButton.getAttribute('aria-pressed')).toBe('true');
	});

	it('debounces search input', async () => {
		vi.mocked(apiModule.api.searchPersons).mockResolvedValue(mockSearchResults);

		render(SearchBox);
		const input = screen.getByRole('combobox');

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
		const input = screen.getByRole('combobox');

		await fireEvent.input(input, { target: { value: 'J' } });
		await vi.advanceTimersByTimeAsync(300);

		expect(apiModule.api.searchPersons).not.toHaveBeenCalled();
	});

	it('shows dropdown with results', async () => {
		vi.mocked(apiModule.api.searchPersons).mockResolvedValue(mockSearchResults);

		const { container } = render(SearchBox);
		const input = screen.getByRole('combobox');

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
		const input = screen.getByRole('combobox');

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
		const input = screen.getByRole('combobox');

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
		const input = screen.getByRole('combobox');

		await fireEvent.input(input, { target: { value: 'Doe' } });
		await vi.advanceTimersByTimeAsync(300);

		await waitFor(() => {
			expect(container.querySelector('.dropdown')).not.toBeNull();
		});

		const escape = new KeyboardEvent('keydown', { key: 'Escape', cancelable: true, bubbles: true });
		input.dispatchEvent(escape);

		// Consumed, so page shortcuts (cancel edit) leave it alone (#1139)
		expect(escape.defaultPrevented).toBe(true);
		await waitFor(() => {
			expect(container.querySelector('.dropdown')).toBeNull();
		});
	});

	it('leaves Escape for page shortcuts when no dropdown is open', () => {
		render(SearchBox);
		const escape = new KeyboardEvent('keydown', { key: 'Escape', cancelable: true, bubbles: true });
		screen.getByRole('combobox').dispatchEvent(escape);

		expect(escape.defaultPrevented).toBe(false);
	});

	it('displays formatted names in results', async () => {
		vi.mocked(apiModule.api.searchPersons).mockResolvedValue(mockSearchResults);

		const { container } = render(SearchBox);
		const input = screen.getByRole('combobox');

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
		const input = screen.getByRole('combobox');

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
		const input = screen.getByRole('combobox');

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
		const input = screen.getByRole('combobox');

		expect(input.getAttribute('aria-label')).toBe('Search');
		expect(input.getAttribute('aria-haspopup')).toBe('listbox');
		expect(input.getAttribute('aria-expanded')).toBe('false');
		expect(input.getAttribute('aria-autocomplete')).toBe('list');
	});

	it('searches with fuzzy mode when enabled', async () => {
		vi.mocked(apiModule.api.searchPersons).mockResolvedValue(mockSearchResults);

		const { container } = render(SearchBox);
		const fuzzyButton = container.querySelector('.fuzzy-toggle') as HTMLButtonElement;
		const input = screen.getByRole('combobox');

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
