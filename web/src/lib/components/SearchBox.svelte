<script lang="ts">
	import { api, type SearchResult, formatPersonName, formatLifespan } from '#lib/api/client.js';

	interface Props {
		onSelect?: (person: SearchResult) => void;
		placeholder?: string;
	}

	let { onSelect, placeholder = 'Search people...' }: Props = $props();

	let query = $state('');
	let results: SearchResult[] = $state([]);
	let loading = $state(false);
	let showDropdown = $state(false);
	// The dropdown renders only with results or a settled "no results" message;
	// Escape is consumed only when it closes this visible list (#1139).
	let dropdownVisible = $derived(
		showDropdown && (results.length > 0 || (query.length >= 2 && !loading))
	);
	let fuzzy = $state(false);
	let highlightedIndex = $state(-1);
	let debounceTimer: ReturnType<typeof setTimeout> | null = null;
	let blurTimer: ReturnType<typeof setTimeout> | null = null;
	let inputRef: HTMLInputElement | undefined = $state();
	let dropdownRef: HTMLDivElement | undefined = $state();

	// Computed aria-activedescendant value
	let activeDescendant = $derived(
		highlightedIndex >= 0 ? `search-result-${highlightedIndex}` : undefined
	);

	// Reset highlighted index when results change
	$effect(() => {
		// Track results array to reset highlight on change
		results;
		highlightedIndex = -1;
	});

	// Scroll highlighted item into view
	$effect(() => {
		if (highlightedIndex >= 0 && dropdownRef) {
			const highlightedEl = dropdownRef.querySelector(`#search-result-${highlightedIndex}`);
			highlightedEl?.scrollIntoView({ block: 'nearest' });
		}
	});

	/**
	 * Focus the search input programmatically.
	 * Used by global keyboard shortcut.
	 */
	export function focus() {
		inputRef?.focus();
	}

	/**
	 * Alias for focus() - alternative name for layout integration.
	 */
	export function focusInput() {
		inputRef?.focus();
	}

	async function search(searchQuery: string) {
		if (searchQuery.length < 2) {
			results = [];
			return;
		}

		loading = true;
		try {
			const response = await api.searchPersons({
				q: searchQuery,
				fuzzy,
				limit: 10
			});
			results = response.items;
		} catch {
			results = [];
		} finally {
			loading = false;
		}
	}

	/**
	 * Search for the current text at once (no debounce) and show the results.
	 * The magnifier and Enter use this so a filled box can be re-run (#1022).
	 */
	function searchNow() {
		if (debounceTimer) {
			clearTimeout(debounceTimer);
			debounceTimer = null;
		}
		if (query.length < 2) return;
		showDropdown = true;
		search(query);
	}

	function handleSearchClick() {
		searchNow();
		inputRef?.focus();
	}

	function handleInput(e: Event) {
		const input = e.target as HTMLInputElement;
		query = input.value;
		showDropdown = true;

		// Debounce search
		if (debounceTimer) {
			clearTimeout(debounceTimer);
		}
		debounceTimer = setTimeout(() => {
			search(query);
		}, 300);
	}

	function handleSelect(person: SearchResult) {
		query = formatPersonName(person);
		showDropdown = false;
		highlightedIndex = -1;
		results = [];
		onSelect?.(person);
	}

	function handleFocus() {
		// Coming back (e.g. via the magnifier) keeps the dropdown open
		if (blurTimer) {
			clearTimeout(blurTimer);
			blurTimer = null;
		}
		// Refocusing a filled box reopens its results, fetching them if none are held
		if (query.length >= 2) {
			showDropdown = true;
			if (results.length === 0 && !loading) {
				search(query);
			}
		}
	}

	function handleBlur() {
		// Delay hiding dropdown to allow click events to fire
		blurTimer = setTimeout(() => {
			blurTimer = null;
			showDropdown = false;
			highlightedIndex = -1;
		}, 200);
	}

	function handleKeydown(e: KeyboardEvent) {
		// Escape closes an open dropdown and is consumed; with nothing open it is
		// left for page shortcuts (e.g. cancelling an edit) (#1139).
		if (e.key === 'Escape') {
			if (dropdownVisible) {
				e.preventDefault();
				showDropdown = false;
				highlightedIndex = -1;
			}
			return;
		}

		// Enter with nothing highlighted runs the search and opens the results
		if (e.key === 'Enter' && highlightedIndex < 0) {
			e.preventDefault();
			searchNow();
			return;
		}

		// Handle Tab - close dropdown but allow default behavior
		if (e.key === 'Tab') {
			showDropdown = false;
			highlightedIndex = -1;
			return;
		}

		// Only handle navigation keys when dropdown is open with results
		if (!showDropdown || results.length === 0) {
			return;
		}

		switch (e.key) {
			case 'ArrowDown':
				e.preventDefault();
				highlightedIndex = (highlightedIndex + 1) % results.length;
				break;
			case 'ArrowUp':
				e.preventDefault();
				highlightedIndex = highlightedIndex <= 0 ? results.length - 1 : highlightedIndex - 1;
				break;
			case 'Enter':
				e.preventDefault();
				handleSelect(results[highlightedIndex]);
				break;
		}
	}

	function toggleFuzzy() {
		fuzzy = !fuzzy;
		if (query.length >= 2) {
			search(query);
		}
	}
</script>

<div class="search-box">
	<div class="input-wrapper">
		<button type="button" class="search-button" onclick={handleSearchClick} aria-label="Search" title="Search">
			<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true">
				<circle cx="11" cy="11" r="8" />
				<path d="m21 21-4.35-4.35" />
			</svg>
		</button>
		<input
			bind:this={inputRef}
			type="text"
			value={query}
			oninput={handleInput}
			onfocus={handleFocus}
			onblur={handleBlur}
			onkeydown={handleKeydown}
			{placeholder}
			aria-label="Search"
			aria-expanded={showDropdown}
			aria-haspopup="listbox"
			aria-controls="search-listbox"
			aria-autocomplete="list"
			aria-activedescendant={activeDescendant}
			role="combobox"
		/>
		{#if loading}
			<span class="loading-indicator"></span>
		{/if}
		<button
			class="fuzzy-toggle"
			class:active={fuzzy}
			onclick={toggleFuzzy}
			title={fuzzy ? 'Fuzzy search enabled' : 'Enable fuzzy search'}
			aria-label="Fuzzy search"
			aria-pressed={fuzzy}
		>
			~
		</button>
	</div>

	<!-- Screen reader announcement for result count -->
	<div class="sr-only" aria-live="polite" aria-atomic="true">
		{#if results.length > 0}
			{results.length} result{results.length === 1 ? '' : 's'} found
		{:else if query.length >= 2 && !loading && showDropdown}
			No results found
		{/if}
	</div>

	{#if dropdownVisible}
		<div
			bind:this={dropdownRef}
			class="dropdown"
			role="listbox"
			id="search-listbox"
			aria-label="Search results"
		>
			{#if results.length === 0}
				<div class="no-results">No results found</div>
			{:else}
				{#each results as person, index}
					<button
						id="search-result-{index}"
						class="result-item"
						class:highlighted={index === highlightedIndex}
						onclick={() => handleSelect(person)}
						role="option"
						aria-selected={index === highlightedIndex}
					>
						<span class="name">{formatPersonName(person)}</span>
						<span class="lifespan">{formatLifespan(person)}</span>
					</button>
				{/each}
			{/if}
		</div>
	{/if}
</div>

<style>
	/* Screen reader only - visually hidden but accessible */
	.sr-only {
		position: absolute;
		width: 1px;
		height: 1px;
		padding: 0;
		margin: -1px;
		overflow: hidden;
		clip: rect(0, 0, 0, 0);
		white-space: nowrap;
		border: 0;
	}

	.search-box {
		position: relative;
		width: 100%;
		max-width: 400px;
	}

	.input-wrapper {
		position: relative;
		display: flex;
		align-items: center;
	}

	.search-button {
		position: absolute;
		left: 0.375rem;
		display: flex;
		align-items: center;
		justify-content: center;
		width: 1.75rem;
		height: 1.75rem;
		padding: 0;
		border: none;
		border-radius: 4px;
		background: none;
		color: var(--color-text-muted);
		cursor: pointer;
	}

	.search-button:hover {
		background: #f1f5f9;
	}

	.search-button svg {
		width: 1rem;
		height: 1rem;
	}

	input {
		width: 100%;
		padding: 0.625rem 2.5rem 0.625rem 2.5rem;
		border: 1px solid #e2e8f0;
		border-radius: 8px;
		font-size: 0.875rem;
		background: white;
		transition: border-color 0.15s, box-shadow 0.15s;
	}

	input:focus {
		outline: none;
		border-color: #3b82f6;
		box-shadow: 0 0 0 3px rgba(59, 130, 246, 0.1);
	}

	.loading-indicator {
		position: absolute;
		right: 2.5rem;
		width: 1rem;
		height: 1rem;
		border: 2px solid #e2e8f0;
		border-top-color: #3b82f6;
		border-radius: 50%;
		animation: spin 0.6s linear infinite;
	}

	@keyframes spin {
		to {
			transform: rotate(360deg);
		}
	}

	.fuzzy-toggle {
		position: absolute;
		right: 0.5rem;
		padding: 0.25rem 0.5rem;
		border: 1px solid #e2e8f0;
		border-radius: 4px;
		background: white;
		color: var(--color-text-muted);
		font-size: 0.875rem;
		font-weight: 600;
		cursor: pointer;
		transition: all 0.15s;
	}

	.fuzzy-toggle:hover {
		background: #f1f5f9;
	}

	.fuzzy-toggle.active {
		background: var(--color-action);
		border-color: var(--color-action);
		color: white;
	}

	.dropdown {
		position: absolute;
		top: calc(100% + 4px);
		left: 0;
		right: 0;
		background: white;
		border: 1px solid #e2e8f0;
		border-radius: 8px;
		box-shadow: 0 4px 12px rgba(0, 0, 0, 0.1);
		z-index: 100;
		max-height: 300px;
		overflow-y: auto;
	}

	.no-results {
		padding: 0.75rem 1rem;
		color: var(--color-text-muted);
		font-size: 0.875rem;
		text-align: center;
	}

	.result-item {
		display: flex;
		justify-content: space-between;
		align-items: center;
		width: 100%;
		padding: 0.625rem 1rem;
		border: none;
		background: none;
		cursor: pointer;
		text-align: left;
		transition: background 0.15s;
	}

	.result-item:hover,
	.result-item.highlighted {
		background: #f1f5f9;
	}

	.result-item.highlighted {
		outline: 2px solid var(--color-focus-ring);
		outline-offset: -2px;
	}

	.result-item:first-child {
		border-radius: 8px 8px 0 0;
	}

	.result-item:last-child {
		border-radius: 0 0 8px 8px;
	}

	.result-item:only-child {
		border-radius: 8px;
	}

	.name {
		font-size: 0.875rem;
		color: #1e293b;
	}

	.lifespan {
		font-size: 0.75rem;
		color: var(--color-text-muted);
	}
</style>
