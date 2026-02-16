import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/svelte';
import SearchPage from './+page.svelte';
import * as apiModule from '$lib/api/client';

// Mock the API module
vi.mock('$lib/api/client', async (importOriginal) => {
	const actual = await importOriginal<typeof apiModule>();
	return {
		...actual,
		api: {
			searchPersons: vi.fn(),
			getPlaceHierarchy: vi.fn()
		},
		formatPersonName: vi.fn((p: { given_name: string; surname: string }) => `${p.given_name} ${p.surname}`),
		formatGenDate: vi.fn((d?: { year?: number }) => d?.year?.toString() ?? ''),
		formatLifespan: vi.fn(() => '(1850-1920)')
	};
});

const mockSearchResults: apiModule.SearchResults = {
	items: [
		{
			id: 'person-1',
			given_name: 'John',
			surname: 'Smith',
			birth_date: { year: 1850 },
			death_date: { year: 1920 },
			score: 0.95
		},
		{
			id: 'person-2',
			given_name: 'Jane',
			surname: 'Doe',
			birth_date: { year: 1870 },
			death_date: { year: 1940 },
			score: 0.65
		},
		{
			id: 'person-3',
			given_name: 'Bob',
			surname: 'Jones',
			birth_date: { year: 1900 },
			score: 0.3
		}
	],
	total: 3
};

const mockPlaces: apiModule.PlaceIndexResponse = {
	items: [
		{ name: 'Springfield', full_name: 'Springfield, IL', count: 5, has_children: false },
		{ name: 'Springfield', full_name: 'Springfield, MO', count: 3, has_children: false },
		{ name: 'Chicago', full_name: 'Chicago, IL', count: 10, has_children: true }
	],
	total: 3
};

describe('Advanced Search Page', () => {
	beforeEach(() => {
		vi.useFakeTimers();
		vi.clearAllMocks();
		// Default: places load returns empty so autocomplete doesn't interfere
		vi.mocked(apiModule.api.getPlaceHierarchy).mockResolvedValue({ items: [], total: 0 });
		vi.mocked(apiModule.api.searchPersons).mockResolvedValue(mockSearchResults);
	});

	afterEach(() => {
		vi.useRealTimers();
	});

	// ─── Rendering ───

	describe('Rendering', () => {
		it('renders the page with title "Advanced Search"', async () => {
			render(SearchPage);
			await vi.advanceTimersByTimeAsync(0);
			expect(screen.getByText('Advanced Search')).toBeDefined();
		});

		it('renders the subtitle description', async () => {
			render(SearchPage);
			await vi.advanceTimersByTimeAsync(0);
			expect(screen.getByText('Search people by name, dates, and places')).toBeDefined();
		});

		it('renders name input field', async () => {
			render(SearchPage);
			await vi.advanceTimersByTimeAsync(0);
			const nameInput = screen.getByPlaceholderText('Name (e.g., Smith, John Smith)');
			expect(nameInput).toBeDefined();
		});

		it('renders birth and death year inputs', async () => {
			render(SearchPage);
			await vi.advanceTimersByTimeAsync(0);
			const yearInputs = screen.getAllByPlaceholderText('YYYY');
			expect(yearInputs.length).toBe(4); // birth from/to, death from/to
		});

		it('renders birth and death place inputs', async () => {
			render(SearchPage);
			await vi.advanceTimersByTimeAsync(0);
			const placeInputs = screen.getAllByPlaceholderText('e.g., Springfield, IL');
			expect(placeInputs.length).toBe(2);
		});

		it('renders Fuzzy toggle in inactive state', async () => {
			render(SearchPage);
			await vi.advanceTimersByTimeAsync(0);
			const fuzzyBtn = screen.getByText('Fuzzy').closest('button');
			expect(fuzzyBtn).toBeDefined();
			expect(fuzzyBtn!.getAttribute('aria-pressed')).toBe('false');
			expect(fuzzyBtn!.classList.contains('active')).toBe(false);
		});

		it('renders Phonetic (Soundex) toggle in inactive state', async () => {
			render(SearchPage);
			await vi.advanceTimersByTimeAsync(0);
			const phoneticBtn = screen.getByText('Phonetic').closest('button');
			expect(phoneticBtn).toBeDefined();
			expect(phoneticBtn!.getAttribute('aria-pressed')).toBe('false');
			expect(phoneticBtn!.classList.contains('active')).toBe(false);
		});

		it('has Search button initially disabled', async () => {
			render(SearchPage);
			await vi.advanceTimersByTimeAsync(0);
			const searchBtn = screen.getByText('Search');
			expect(searchBtn.closest('button')!.hasAttribute('disabled')).toBe(true);
		});

		it('renders Clear All button', async () => {
			render(SearchPage);
			await vi.advanceTimersByTimeAsync(0);
			expect(screen.getByText('Clear All')).toBeDefined();
		});
	});

	// ─── Form Interactions ───

	describe('Form Interactions', () => {
		it('typing in name input enables Search button', async () => {
			render(SearchPage);
			await vi.advanceTimersByTimeAsync(0);
			const nameInput = screen.getByPlaceholderText('Name (e.g., Smith, John Smith)');
			const searchBtn = screen.getByText('Search').closest('button')!;

			expect(searchBtn.hasAttribute('disabled')).toBe(true);
			await fireEvent.input(nameInput, { target: { value: 'Smith' } });
			expect(searchBtn.hasAttribute('disabled')).toBe(false);
		});

		it('entering a birth year enables Search button', async () => {
			render(SearchPage);
			await vi.advanceTimersByTimeAsync(0);
			const yearInputs = screen.getAllByPlaceholderText('YYYY');
			const searchBtn = screen.getByText('Search').closest('button')!;

			expect(searchBtn.hasAttribute('disabled')).toBe(true);
			await fireEvent.input(yearInputs[0], { target: { value: '1850' } });
			expect(searchBtn.hasAttribute('disabled')).toBe(false);
		});

		it('entering a place enables Search button', async () => {
			render(SearchPage);
			await vi.advanceTimersByTimeAsync(0);
			const placeInputs = screen.getAllByPlaceholderText('e.g., Springfield, IL');
			const searchBtn = screen.getByText('Search').closest('button')!;

			expect(searchBtn.hasAttribute('disabled')).toBe(true);
			await fireEvent.input(placeInputs[0], { target: { value: 'Chicago' } });
			expect(searchBtn.hasAttribute('disabled')).toBe(false);
		});

		it('clicking Fuzzy toggle activates it', async () => {
			render(SearchPage);
			await vi.advanceTimersByTimeAsync(0);
			const fuzzyBtn = screen.getByText('Fuzzy').closest('button')!;

			expect(fuzzyBtn.getAttribute('aria-pressed')).toBe('false');
			await fireEvent.click(fuzzyBtn);
			expect(fuzzyBtn.getAttribute('aria-pressed')).toBe('true');
			expect(fuzzyBtn.classList.contains('active')).toBe(true);
		});

		it('clicking Phonetic toggle activates it', async () => {
			render(SearchPage);
			await vi.advanceTimersByTimeAsync(0);
			const phoneticBtn = screen.getByText('Phonetic').closest('button')!;

			expect(phoneticBtn.getAttribute('aria-pressed')).toBe('false');
			await fireEvent.click(phoneticBtn);
			expect(phoneticBtn.getAttribute('aria-pressed')).toBe('true');
			expect(phoneticBtn.classList.contains('active')).toBe(true);
		});

		it('Clear All resets all form fields to defaults', async () => {
			render(SearchPage);
			await vi.advanceTimersByTimeAsync(0);

			// Fill in various fields
			const nameInput = screen.getByPlaceholderText('Name (e.g., Smith, John Smith)') as HTMLInputElement;
			await fireEvent.input(nameInput, { target: { value: 'Smith' } });

			const yearInputs = screen.getAllByPlaceholderText('YYYY') as HTMLInputElement[];
			await fireEvent.input(yearInputs[0], { target: { value: '1850' } });

			const fuzzyBtn = screen.getByText('Fuzzy').closest('button')!;
			await fireEvent.click(fuzzyBtn);

			// Verify fields are filled
			expect(nameInput.value).toBe('Smith');
			expect(yearInputs[0].value).toBe('1850');
			expect(fuzzyBtn.getAttribute('aria-pressed')).toBe('true');

			// Click Clear All
			const clearBtn = screen.getByText('Clear All');
			await fireEvent.click(clearBtn);

			// Verify fields are reset
			expect(nameInput.value).toBe('');
			expect(yearInputs[0].value).toBe('');
			expect(fuzzyBtn.getAttribute('aria-pressed')).toBe('false');
		});

		it('Clear All also clears results', async () => {
			render(SearchPage);
			await vi.advanceTimersByTimeAsync(0);

			// Perform a search first
			const nameInput = screen.getByPlaceholderText('Name (e.g., Smith, John Smith)');
			await fireEvent.input(nameInput, { target: { value: 'Smith' } });

			const searchBtn = screen.getByText('Search').closest('button')!;
			await fireEvent.click(searchBtn);
			await vi.advanceTimersByTimeAsync(0);

			await waitFor(() => {
				expect(screen.getByText(/Showing \d+ of \d+ results/)).toBeDefined();
			});

			// Click Clear All
			await fireEvent.click(screen.getByText('Clear All'));

			// Results should be gone
			expect(screen.queryByText(/Showing \d+ of \d+ results/)).toBeNull();
		});
	});

	// ─── Search Execution ───

	describe('Search Execution', () => {
		it('clicking Search calls api.searchPersons with correct params', async () => {
			render(SearchPage);
			await vi.advanceTimersByTimeAsync(0);

			const nameInput = screen.getByPlaceholderText('Name (e.g., Smith, John Smith)');
			await fireEvent.input(nameInput, { target: { value: 'Smith' } });

			const searchBtn = screen.getByText('Search').closest('button')!;
			await fireEvent.click(searchBtn);
			await vi.advanceTimersByTimeAsync(0);

			expect(apiModule.api.searchPersons).toHaveBeenCalledWith(
				expect.objectContaining({ q: 'Smith' })
			);
		});

		it('name query is passed as q parameter', async () => {
			render(SearchPage);
			await vi.advanceTimersByTimeAsync(0);

			const nameInput = screen.getByPlaceholderText('Name (e.g., Smith, John Smith)');
			await fireEvent.input(nameInput, { target: { value: 'John Doe' } });

			await fireEvent.click(screen.getByText('Search').closest('button')!);
			await vi.advanceTimersByTimeAsync(0);

			const callArgs = vi.mocked(apiModule.api.searchPersons).mock.calls[0][0];
			expect(callArgs.q).toBe('John Doe');
		});

		it('fuzzy toggle is passed as boolean param', async () => {
			render(SearchPage);
			await vi.advanceTimersByTimeAsync(0);

			const nameInput = screen.getByPlaceholderText('Name (e.g., Smith, John Smith)');
			await fireEvent.input(nameInput, { target: { value: 'Smith' } });

			const fuzzyBtn = screen.getByText('Fuzzy').closest('button')!;
			await fireEvent.click(fuzzyBtn);

			await fireEvent.click(screen.getByText('Search').closest('button')!);
			await vi.advanceTimersByTimeAsync(0);

			const callArgs = vi.mocked(apiModule.api.searchPersons).mock.calls[0][0];
			expect(callArgs.fuzzy).toBe(true);
		});

		it('soundex toggle is passed as boolean param', async () => {
			render(SearchPage);
			await vi.advanceTimersByTimeAsync(0);

			const nameInput = screen.getByPlaceholderText('Name (e.g., Smith, John Smith)');
			await fireEvent.input(nameInput, { target: { value: 'Smith' } });

			const phoneticBtn = screen.getByText('Phonetic').closest('button')!;
			await fireEvent.click(phoneticBtn);

			await fireEvent.click(screen.getByText('Search').closest('button')!);
			await vi.advanceTimersByTimeAsync(0);

			const callArgs = vi.mocked(apiModule.api.searchPersons).mock.calls[0][0];
			expect(callArgs.soundex).toBe(true);
		});

		it('year inputs converted to date strings (1850 to "1850-01-01" for from, "1850-12-31" for to)', async () => {
			render(SearchPage);
			await vi.advanceTimersByTimeAsync(0);

			const yearInputs = screen.getAllByPlaceholderText('YYYY');
			// Birth From
			await fireEvent.input(yearInputs[0], { target: { value: '1850' } });
			// Birth To
			await fireEvent.input(yearInputs[1], { target: { value: '1900' } });

			await fireEvent.click(screen.getByText('Search').closest('button')!);
			await vi.advanceTimersByTimeAsync(0);

			const callArgs = vi.mocked(apiModule.api.searchPersons).mock.calls[0][0];
			expect(callArgs.birth_date_from).toBe('1850-01-01');
			expect(callArgs.birth_date_to).toBe('1900-12-31');
		});

		it('place values passed as birth_place/death_place params', async () => {
			render(SearchPage);
			await vi.advanceTimersByTimeAsync(0);

			const placeInputs = screen.getAllByPlaceholderText('e.g., Springfield, IL');
			await fireEvent.input(placeInputs[0], { target: { value: 'Chicago, IL' } });
			await fireEvent.input(placeInputs[1], { target: { value: 'New York, NY' } });

			await fireEvent.click(screen.getByText('Search').closest('button')!);
			await vi.advanceTimersByTimeAsync(0);

			const callArgs = vi.mocked(apiModule.api.searchPersons).mock.calls[0][0];
			expect(callArgs.birth_place).toBe('Chicago, IL');
			expect(callArgs.death_place).toBe('New York, NY');
		});

		it('empty filter fields are omitted from API call', async () => {
			render(SearchPage);
			await vi.advanceTimersByTimeAsync(0);

			// Only fill name, leave everything else empty
			const nameInput = screen.getByPlaceholderText('Name (e.g., Smith, John Smith)');
			await fireEvent.input(nameInput, { target: { value: 'Smith' } });

			await fireEvent.click(screen.getByText('Search').closest('button')!);
			await vi.advanceTimersByTimeAsync(0);

			const callArgs = vi.mocked(apiModule.api.searchPersons).mock.calls[0][0];
			expect(callArgs.q).toBe('Smith');
			expect(callArgs.birth_date_from).toBeUndefined();
			expect(callArgs.birth_date_to).toBeUndefined();
			expect(callArgs.death_date_from).toBeUndefined();
			expect(callArgs.death_date_to).toBeUndefined();
			expect(callArgs.birth_place).toBeUndefined();
			expect(callArgs.death_place).toBeUndefined();
		});

		it('shows loading state during search', async () => {
			// Create a never-resolving promise to keep loading visible
			let resolveSearch!: (value: apiModule.SearchResults) => void;
			const searchPromise = new Promise<apiModule.SearchResults>((resolve) => {
				resolveSearch = resolve;
			});
			vi.mocked(apiModule.api.searchPersons).mockReturnValue(searchPromise);

			const { container } = render(SearchPage);
			await vi.advanceTimersByTimeAsync(0);

			const nameInput = screen.getByPlaceholderText('Name (e.g., Smith, John Smith)');
			await fireEvent.input(nameInput, { target: { value: 'Smith' } });

			await fireEvent.click(screen.getByText('Search').closest('button')!);
			await vi.advanceTimersByTimeAsync(0);

			// The submit button text changes to "Searching..."
			const submitBtn = container.querySelector('button[type="submit"]');
			expect(submitBtn?.textContent?.trim()).toBe('Searching...');

			// Resolve and verify loading is removed
			resolveSearch(mockSearchResults);
			await vi.advanceTimersByTimeAsync(0);

			await waitFor(() => {
				expect(submitBtn?.textContent?.trim()).toBe('Search');
			});
		});

		it('results displayed after search completes', async () => {
			render(SearchPage);
			await vi.advanceTimersByTimeAsync(0);

			const nameInput = screen.getByPlaceholderText('Name (e.g., Smith, John Smith)');
			await fireEvent.input(nameInput, { target: { value: 'Smith' } });

			await fireEvent.click(screen.getByText('Search').closest('button')!);
			await vi.advanceTimersByTimeAsync(0);

			await waitFor(() => {
				expect(screen.getByText('Showing 3 of 3 results')).toBeDefined();
			});
		});
	});

	// ─── Results Display ───

	describe('Results Display', () => {
		async function performSearch() {
			render(SearchPage);
			await vi.advanceTimersByTimeAsync(0);

			const nameInput = screen.getByPlaceholderText('Name (e.g., Smith, John Smith)');
			await fireEvent.input(nameInput, { target: { value: 'Smith' } });

			await fireEvent.click(screen.getByText('Search').closest('button')!);
			await vi.advanceTimersByTimeAsync(0);

			await waitFor(() => {
				expect(screen.getByText('Showing 3 of 3 results')).toBeDefined();
			});
		}

		it('results show person names via formatPersonName', async () => {
			await performSearch();

			expect(apiModule.formatPersonName).toHaveBeenCalled();
			// The mock returns "given_name surname" - names appear in both table and mobile cards
			expect(screen.getAllByText('John Smith').length).toBeGreaterThanOrEqual(1);
			expect(screen.getAllByText('Jane Doe').length).toBeGreaterThanOrEqual(1);
			expect(screen.getAllByText('Bob Jones').length).toBeGreaterThanOrEqual(1);
		});

		it('person names are links to /persons/{id}', async () => {
			const { container } = render(SearchPage);
			await vi.advanceTimersByTimeAsync(0);

			const nameInput = screen.getByPlaceholderText('Name (e.g., Smith, John Smith)');
			await fireEvent.input(nameInput, { target: { value: 'Smith' } });
			await fireEvent.click(screen.getByText('Search').closest('button')!);
			await vi.advanceTimersByTimeAsync(0);

			await waitFor(() => {
				const personLinks = container.querySelectorAll('a.person-link');
				expect(personLinks.length).toBe(3);
				expect(personLinks[0].getAttribute('href')).toBe('/persons/person-1');
				expect(personLinks[1].getAttribute('href')).toBe('/persons/person-2');
				expect(personLinks[2].getAttribute('href')).toBe('/persons/person-3');
			});
		});

		it('score indicator uses correct color (green > 0.8)', async () => {
			const { container } = render(SearchPage);
			await vi.advanceTimersByTimeAsync(0);

			const nameInput = screen.getByPlaceholderText('Name (e.g., Smith, John Smith)');
			await fireEvent.input(nameInput, { target: { value: 'Smith' } });
			await fireEvent.click(screen.getByText('Search').closest('button')!);
			await vi.advanceTimersByTimeAsync(0);

			await waitFor(() => {
				const scoreBadges = container.querySelectorAll('.results-table .score-badge');
				// person-1 score=0.95 -> score-high
				expect(scoreBadges[0].classList.contains('score-high')).toBe(true);
				expect(scoreBadges[0].textContent).toBe('95%');
			});
		});

		it('score indicator uses correct color (amber 0.5-0.8)', async () => {
			const { container } = render(SearchPage);
			await vi.advanceTimersByTimeAsync(0);

			const nameInput = screen.getByPlaceholderText('Name (e.g., Smith, John Smith)');
			await fireEvent.input(nameInput, { target: { value: 'Smith' } });
			await fireEvent.click(screen.getByText('Search').closest('button')!);
			await vi.advanceTimersByTimeAsync(0);

			await waitFor(() => {
				const scoreBadges = container.querySelectorAll('.results-table .score-badge');
				// person-2 score=0.65 -> score-medium
				expect(scoreBadges[1].classList.contains('score-medium')).toBe(true);
				expect(scoreBadges[1].textContent).toBe('65%');
			});
		});

		it('score indicator uses correct color (gray < 0.5)', async () => {
			const { container } = render(SearchPage);
			await vi.advanceTimersByTimeAsync(0);

			const nameInput = screen.getByPlaceholderText('Name (e.g., Smith, John Smith)');
			await fireEvent.input(nameInput, { target: { value: 'Smith' } });
			await fireEvent.click(screen.getByText('Search').closest('button')!);
			await vi.advanceTimersByTimeAsync(0);

			await waitFor(() => {
				const scoreBadges = container.querySelectorAll('.results-table .score-badge');
				// person-3 score=0.3 -> score-low
				expect(scoreBadges[2].classList.contains('score-low')).toBe(true);
				expect(scoreBadges[2].textContent).toBe('30%');
			});
		});

		it('total count displayed ("Showing X of Y results")', async () => {
			await performSearch();
			expect(screen.getByText('Showing 3 of 3 results')).toBeDefined();
		});

		it('empty results show appropriate message', async () => {
			vi.mocked(apiModule.api.searchPersons).mockResolvedValue({ items: [], total: 0 });

			render(SearchPage);
			await vi.advanceTimersByTimeAsync(0);

			const nameInput = screen.getByPlaceholderText('Name (e.g., Smith, John Smith)');
			await fireEvent.input(nameInput, { target: { value: 'zzzzz' } });
			await fireEvent.click(screen.getByText('Search').closest('button')!);
			await vi.advanceTimersByTimeAsync(0);

			await waitFor(() => {
				expect(screen.getByText('No results found. Try adjusting your search criteria.')).toBeDefined();
			});
		});
	});

	// ─── Sorting ───

	describe('Sorting', () => {
		async function performSearchAndReturn() {
			const { container } = render(SearchPage);
			await vi.advanceTimersByTimeAsync(0);

			const nameInput = screen.getByPlaceholderText('Name (e.g., Smith, John Smith)');
			await fireEvent.input(nameInput, { target: { value: 'Smith' } });
			await fireEvent.click(screen.getByText('Search').closest('button')!);
			await vi.advanceTimersByTimeAsync(0);

			await waitFor(() => {
				expect(screen.getByText('Showing 3 of 3 results')).toBeDefined();
			});

			return container;
		}

		it('sort dropdown defaults to "Relevance"', async () => {
			const container = await performSearchAndReturn();
			const sortSelect = container.querySelector('.sort-controls select') as HTMLSelectElement;
			expect(sortSelect.value).toBe('relevance');
		});

		it('changing sort field triggers new search with updated sort param', async () => {
			const container = await performSearchAndReturn();

			vi.clearAllMocks();
			vi.mocked(apiModule.api.searchPersons).mockResolvedValue(mockSearchResults);

			const sortSelect = container.querySelector('.sort-controls select') as HTMLSelectElement;
			await fireEvent.change(sortSelect, { target: { value: 'name' } });
			await vi.advanceTimersByTimeAsync(0);

			expect(apiModule.api.searchPersons).toHaveBeenCalledWith(
				expect.objectContaining({ sort: 'name' })
			);
		});

		it('toggling order (asc/desc) triggers new search', async () => {
			const container = await performSearchAndReturn();

			vi.clearAllMocks();
			vi.mocked(apiModule.api.searchPersons).mockResolvedValue(mockSearchResults);

			const orderBtn = container.querySelector('.order-btn') as HTMLButtonElement;
			await fireEvent.click(orderBtn);
			await vi.advanceTimersByTimeAsync(0);

			expect(apiModule.api.searchPersons).toHaveBeenCalledWith(
				expect.objectContaining({ order: 'asc' })
			);
		});
	});

	// ─── Place Autocomplete ───

	describe('Place Autocomplete', () => {
		// Place autocomplete uses debounce timers + Svelte $effect.
		// Use real timers here to avoid conflicts between waitFor and fake timers.
		beforeEach(() => {
			vi.useRealTimers();
			vi.mocked(apiModule.api.getPlaceHierarchy).mockImplementation(async (parent?: string) => {
				if (parent) {
					// Child call — return empty (no further sub-places)
					return { items: [], total: 0 };
				}
				return mockPlaces;
			});
		});

		afterEach(() => {
			vi.useFakeTimers();
		});

		async function openBirthPlaceDropdown(container: HTMLElement) {
			const birthPlaceInput = document.getElementById('birth-place-input') as HTMLInputElement;

			// Type to trigger the debounced filter
			await fireEvent.input(birthPlaceInput, { target: { value: 'Spring' } });

			// Wait for debounce (300ms) to populate suggestions
			await new Promise(r => setTimeout(r, 500));

			// Re-focus to open dropdown (blur is now synchronous, no setTimeout race)
			await fireEvent.blur(birthPlaceInput);
			await fireEvent.focus(birthPlaceInput);

			await waitFor(() => {
				expect(container.querySelector('#birth-place-listbox')).not.toBeNull();
			}, { timeout: 2000 });

			return birthPlaceInput;
		}

		it('typing in place input shows dropdown with suggestions', async () => {
			const { container } = render(SearchPage);

			// Wait for places to load from API
			await waitFor(() => {
				expect(apiModule.api.getPlaceHierarchy).toHaveBeenCalled();
			});

			await openBirthPlaceDropdown(container);

			const dropdown = container.querySelector('#birth-place-listbox');
			expect(dropdown).not.toBeNull();
		});

		it('suggestions show place name and count', async () => {
			const { container } = render(SearchPage);

			await waitFor(() => {
				expect(apiModule.api.getPlaceHierarchy).toHaveBeenCalled();
			});

			await openBirthPlaceDropdown(container);

			const options = container.querySelectorAll('#birth-place-listbox .place-option');
			expect(options.length).toBe(2); // Springfield, IL and Springfield, MO
			expect(options[0].querySelector('.place-name')?.textContent).toBe('Springfield, IL');
			expect(options[0].querySelector('.place-count')?.textContent).toBe('(5)');
		});

		it('clicking a suggestion fills the input and closes dropdown', async () => {
			const { container } = render(SearchPage);

			await waitFor(() => {
				expect(apiModule.api.getPlaceHierarchy).toHaveBeenCalled();
			});

			const birthPlaceInput = await openBirthPlaceDropdown(container);

			// Click first suggestion
			const firstOption = container.querySelector('#birth-place-listbox .place-option') as HTMLButtonElement;
			await fireEvent.click(firstOption);

			await waitFor(() => {
				expect(birthPlaceInput.value).toBe('Springfield, IL');
				expect(container.querySelector('#birth-place-listbox')).toBeNull();
			});
		});

		it('Escape key closes dropdown', async () => {
			const { container } = render(SearchPage);

			await waitFor(() => {
				expect(apiModule.api.getPlaceHierarchy).toHaveBeenCalled();
			});

			const birthPlaceInput = await openBirthPlaceDropdown(container);

			await fireEvent.keyDown(birthPlaceInput, { key: 'Escape' });

			await waitFor(() => {
				expect(container.querySelector('#birth-place-listbox')).toBeNull();
			});
		});
	});

	// ─── Accessibility ───

	describe('Accessibility', () => {
		it('place inputs have role="combobox" and aria-expanded', async () => {
			render(SearchPage);
			await vi.advanceTimersByTimeAsync(0);

			const birthPlaceInput = document.getElementById('birth-place-input');
			expect(birthPlaceInput?.getAttribute('role')).toBe('combobox');
			expect(birthPlaceInput?.getAttribute('aria-expanded')).toBe('false');

			const deathPlaceInput = document.getElementById('death-place-input');
			expect(deathPlaceInput?.getAttribute('role')).toBe('combobox');
			expect(deathPlaceInput?.getAttribute('aria-expanded')).toBe('false');
		});

		it('toggle buttons have accessible pressed states', async () => {
			render(SearchPage);
			await vi.advanceTimersByTimeAsync(0);

			const fuzzyBtn = screen.getByText('Fuzzy').closest('button')!;
			expect(fuzzyBtn.getAttribute('aria-pressed')).toBe('false');

			const phoneticBtn = screen.getByText('Phonetic').closest('button')!;
			expect(phoneticBtn.getAttribute('aria-pressed')).toBe('false');
		});

		it('toggle buttons have title attributes', async () => {
			render(SearchPage);
			await vi.advanceTimersByTimeAsync(0);

			const fuzzyBtn = screen.getByText('Fuzzy').closest('button')!;
			expect(fuzzyBtn.getAttribute('title')).toBe('Enable fuzzy matching');

			const phoneticBtn = screen.getByText('Phonetic').closest('button')!;
			expect(phoneticBtn.getAttribute('title')).toBe('Enable phonetic matching');
		});

		it('results count has aria-live for search state', async () => {
			const { container } = render(SearchPage);
			await vi.advanceTimersByTimeAsync(0);

			// Perform a search so results section appears
			const nameInput = screen.getByPlaceholderText('Name (e.g., Smith, John Smith)');
			await fireEvent.input(nameInput, { target: { value: 'Smith' } });

			// Use a slow promise to check aria-live on loading state
			let resolveSearch!: (value: apiModule.SearchResults) => void;
			const searchPromise = new Promise<apiModule.SearchResults>((resolve) => {
				resolveSearch = resolve;
			});
			vi.mocked(apiModule.api.searchPersons).mockReturnValue(searchPromise);

			await fireEvent.click(screen.getByText('Search').closest('button')!);
			await vi.advanceTimersByTimeAsync(0);

			const liveRegion = container.querySelector('[aria-live="polite"]');
			expect(liveRegion).not.toBeNull();

			resolveSearch(mockSearchResults);
			await vi.advanceTimersByTimeAsync(0);
		});
	});

	// ─── Error Handling ───

	describe('Error Handling', () => {
		it('API error shows error message to user', async () => {
			vi.mocked(apiModule.api.searchPersons).mockRejectedValue({ message: 'Server error' });

			render(SearchPage);
			await vi.advanceTimersByTimeAsync(0);

			const nameInput = screen.getByPlaceholderText('Name (e.g., Smith, John Smith)');
			await fireEvent.input(nameInput, { target: { value: 'Smith' } });
			await fireEvent.click(screen.getByText('Search').closest('button')!);
			await vi.advanceTimersByTimeAsync(0);

			await waitFor(() => {
				expect(screen.getByText('Server error')).toBeDefined();
			});
		});

		it('API error with no message shows default message', async () => {
			vi.mocked(apiModule.api.searchPersons).mockRejectedValue({});

			render(SearchPage);
			await vi.advanceTimersByTimeAsync(0);

			const nameInput = screen.getByPlaceholderText('Name (e.g., Smith, John Smith)');
			await fireEvent.input(nameInput, { target: { value: 'Smith' } });
			await fireEvent.click(screen.getByText('Search').closest('button')!);
			await vi.advanceTimersByTimeAsync(0);

			await waitFor(() => {
				expect(screen.getByText('Search failed. Please try again.')).toBeDefined();
			});
		});

		it('error message has role="alert"', async () => {
			vi.mocked(apiModule.api.searchPersons).mockRejectedValue({ message: 'Server error' });

			const { container } = render(SearchPage);
			await vi.advanceTimersByTimeAsync(0);

			const nameInput = screen.getByPlaceholderText('Name (e.g., Smith, John Smith)');
			await fireEvent.input(nameInput, { target: { value: 'Smith' } });
			await fireEvent.click(screen.getByText('Search').closest('button')!);
			await vi.advanceTimersByTimeAsync(0);

			await waitFor(() => {
				const errorEl = container.querySelector('[role="alert"]');
				expect(errorEl).not.toBeNull();
				expect(errorEl?.textContent).toBe('Server error');
			});
		});
	});
});
