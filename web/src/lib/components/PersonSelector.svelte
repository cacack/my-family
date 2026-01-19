<script lang="ts">
	import { api, type SearchResult, type Person, formatPersonName, formatLifespan } from '$lib/api/client';

	interface Props {
		label: string;
		selectedPerson?: Person | SearchResult | null;
		onSelect?: (person: SearchResult | null) => void;
		placeholder?: string;
		disabled?: boolean;
	}

	let { label, selectedPerson = null, onSelect, placeholder = 'Search for a person...', disabled = false }: Props = $props();

	let query = $state('');
	let results: SearchResult[] = $state([]);
	let loading = $state(false);
	let showDropdown = $state(false);
	let highlightedIndex = $state(-1);
	let debounceTimer: ReturnType<typeof setTimeout> | null = null;
	let inputRef: HTMLInputElement | undefined = $state();
	let dropdownRef: HTMLDivElement | undefined = $state();

	// Computed aria-activedescendant value
	let activeDescendant = $derived(
		highlightedIndex >= 0 ? `person-selector-result-${highlightedIndex}` : undefined
	);

	// Reset highlighted index when results change
	$effect(() => {
		results;
		highlightedIndex = -1;
	});

	// Scroll highlighted item into view
	$effect(() => {
		if (highlightedIndex >= 0 && dropdownRef) {
			const highlightedEl = dropdownRef.querySelector(`#person-selector-result-${highlightedIndex}`);
			highlightedEl?.scrollIntoView({ block: 'nearest' });
		}
	});

	async function search(searchQuery: string) {
		if (searchQuery.length < 2) {
			results = [];
			return;
		}

		loading = true;
		try {
			const response = await api.searchPersons({
				q: searchQuery,
				fuzzy: true,
				limit: 8
			});
			results = response.items;
		} catch {
			results = [];
		} finally {
			loading = false;
		}
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
		query = '';
		showDropdown = false;
		highlightedIndex = -1;
		results = [];
		onSelect?.(person);
	}

	function handleClear() {
		query = '';
		showDropdown = false;
		highlightedIndex = -1;
		results = [];
		onSelect?.(null);
		inputRef?.focus();
	}

	function handleFocus() {
		if (query.length >= 2 && results.length > 0) {
			showDropdown = true;
		}
	}

	function handleBlur() {
		// Delay hiding dropdown to allow click events to fire
		setTimeout(() => {
			showDropdown = false;
			highlightedIndex = -1;
		}, 200);
	}

	function handleKeydown(e: KeyboardEvent) {
		// Handle Escape
		if (e.key === 'Escape') {
			e.preventDefault();
			showDropdown = false;
			highlightedIndex = -1;
			return;
		}

		// Handle Tab
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
				if (highlightedIndex >= 0) {
					e.preventDefault();
					handleSelect(results[highlightedIndex]);
				}
				break;
		}
	}

	function getGenderBgClass(gender?: string): string {
		if (gender === 'male') return 'bg-blue-100 text-blue-600';
		if (gender === 'female') return 'bg-pink-100 text-pink-600';
		return 'bg-slate-100 text-slate-500';
	}
</script>

<div class="person-selector">
	<span class="selector-label" id="person-selector-label-{label.replace(/\s+/g, '-').toLowerCase()}">{label}</span>

	{#if selectedPerson}
		<!-- Selected person display -->
		<div class="selected-person" data-gender={selectedPerson.gender}>
			<div class="person-avatar {getGenderBgClass(selectedPerson.gender)}">
				<svg viewBox="0 0 24 24" fill="currentColor">
					<path d="M12 4a4 4 0 1 0 0 8 4 4 0 0 0 0-8zM6 8a6 6 0 1 1 12 0A6 6 0 0 1 6 8zm2 10a3 3 0 0 0-3 3 1 1 0 1 1-2 0 5 5 0 0 1 5-5h8a5 5 0 0 1 5 5 1 1 0 1 1-2 0 3 3 0 0 0-3-3H8z" />
				</svg>
			</div>
			<div class="person-info">
				<span class="person-name">{formatPersonName(selectedPerson)}</span>
				<span class="person-lifespan">{formatLifespan(selectedPerson)}</span>
			</div>
			<button
				type="button"
				class="clear-btn"
				onclick={handleClear}
				aria-label="Clear selection"
				{disabled}
			>
				<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
					<path d="M18 6L6 18M6 6l12 12" />
				</svg>
			</button>
		</div>
	{:else}
		<!-- Search input -->
		<div class="input-wrapper">
			<svg class="search-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
				<circle cx="11" cy="11" r="8" />
				<path d="m21 21-4.35-4.35" />
			</svg>
			<input
				bind:this={inputRef}
				type="text"
				value={query}
				oninput={handleInput}
				onfocus={handleFocus}
				onblur={handleBlur}
				onkeydown={handleKeydown}
				{placeholder}
				{disabled}
				aria-label={label}
				aria-expanded={showDropdown}
				aria-haspopup="listbox"
				aria-controls="person-selector-listbox"
				aria-autocomplete="list"
				aria-activedescendant={activeDescendant}
				role="combobox"
			/>
			{#if loading}
				<span class="loading-indicator"></span>
			{/if}
		</div>

		<!-- Screen reader announcement -->
		<div class="sr-only" aria-live="polite" aria-atomic="true">
			{#if results.length > 0}
				{results.length} result{results.length === 1 ? '' : 's'} found
			{:else if query.length >= 2 && !loading && showDropdown}
				No results found
			{/if}
		</div>

		<!-- Dropdown results -->
		{#if showDropdown && (results.length > 0 || (query.length >= 2 && !loading))}
			<div
				bind:this={dropdownRef}
				class="dropdown"
				role="listbox"
				id="person-selector-listbox"
				aria-label="Search results"
			>
				{#if results.length === 0}
					<div class="no-results">No people found</div>
				{:else}
					{#each results as person, index}
						<button
							id="person-selector-result-{index}"
							type="button"
							class="result-item"
							class:highlighted={index === highlightedIndex}
							onclick={() => handleSelect(person)}
							role="option"
							aria-selected={index === highlightedIndex}
						>
							<div class="result-avatar {getGenderBgClass(person.gender)}">
								<svg viewBox="0 0 24 24" fill="currentColor">
									<path d="M12 4a4 4 0 1 0 0 8 4 4 0 0 0 0-8zM6 8a6 6 0 1 1 12 0A6 6 0 0 1 6 8zm2 10a3 3 0 0 0-3 3 1 1 0 1 1-2 0 5 5 0 0 1 5-5h8a5 5 0 0 1 5 5 1 1 0 1 1-2 0 3 3 0 0 0-3-3H8z" />
								</svg>
							</div>
							<div class="result-info">
								<span class="result-name">{formatPersonName(person)}</span>
								<span class="result-lifespan">{formatLifespan(person)}</span>
							</div>
						</button>
					{/each}
				{/if}
			</div>
		{/if}
	{/if}
</div>

<style>
	.person-selector {
		position: relative;
		width: 100%;
	}

	.selector-label {
		display: block;
		margin-bottom: 0.5rem;
		font-size: 0.875rem;
		font-weight: 600;
		color: #374151;
		cursor: default;
	}

	/* Selected person display */
	.selected-person {
		display: flex;
		align-items: center;
		gap: 0.75rem;
		padding: 0.75rem;
		background: white;
		border: 2px solid #3b82f6;
		border-radius: 8px;
	}

	.person-avatar {
		flex-shrink: 0;
		width: 2.5rem;
		height: 2.5rem;
		border-radius: 50%;
		display: flex;
		align-items: center;
		justify-content: center;
	}

	.person-avatar svg {
		width: 1.25rem;
		height: 1.25rem;
	}

	.person-info {
		flex: 1;
		min-width: 0;
		display: flex;
		flex-direction: column;
		gap: 0.125rem;
	}

	.person-name {
		font-size: 0.9375rem;
		font-weight: 600;
		color: #1e293b;
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}

	.person-lifespan {
		font-size: 0.8125rem;
		color: #64748b;
	}

	.clear-btn {
		flex-shrink: 0;
		padding: 0.375rem;
		border: none;
		border-radius: 6px;
		background: transparent;
		color: #94a3b8;
		cursor: pointer;
		transition: all 0.15s;
	}

	.clear-btn:hover:not(:disabled) {
		background: #f1f5f9;
		color: #64748b;
	}

	.clear-btn:disabled {
		cursor: not-allowed;
		opacity: 0.5;
	}

	.clear-btn svg {
		width: 1.25rem;
		height: 1.25rem;
	}

	/* Search input */
	.input-wrapper {
		position: relative;
		display: flex;
		align-items: center;
	}

	.search-icon {
		position: absolute;
		left: 0.75rem;
		width: 1.125rem;
		height: 1.125rem;
		color: #94a3b8;
		pointer-events: none;
	}

	input {
		width: 100%;
		padding: 0.75rem 2.5rem 0.75rem 2.5rem;
		border: 1px solid #d1d5db;
		border-radius: 8px;
		font-size: 0.9375rem;
		background: white;
		transition: border-color 0.15s, box-shadow 0.15s;
	}

	input:focus {
		outline: none;
		border-color: #3b82f6;
		box-shadow: 0 0 0 3px rgba(59, 130, 246, 0.1);
	}

	input:disabled {
		background: #f9fafb;
		cursor: not-allowed;
	}

	.loading-indicator {
		position: absolute;
		right: 0.75rem;
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

	/* Screen reader only */
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

	/* Dropdown */
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
		max-height: 280px;
		overflow-y: auto;
	}

	.no-results {
		padding: 1rem;
		color: #94a3b8;
		font-size: 0.875rem;
		text-align: center;
	}

	.result-item {
		display: flex;
		align-items: center;
		gap: 0.75rem;
		width: 100%;
		padding: 0.75rem 1rem;
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
		outline: 2px solid #3b82f6;
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

	.result-avatar {
		flex-shrink: 0;
		width: 2rem;
		height: 2rem;
		border-radius: 50%;
		display: flex;
		align-items: center;
		justify-content: center;
	}

	.result-avatar svg {
		width: 1rem;
		height: 1rem;
	}

	.result-info {
		flex: 1;
		min-width: 0;
		display: flex;
		flex-direction: column;
		gap: 0.125rem;
	}

	.result-name {
		font-size: 0.875rem;
		color: #1e293b;
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}

	.result-lifespan {
		font-size: 0.75rem;
		color: #94a3b8;
	}

	/* Tailwind-like utility classes for avatar colors */
	.bg-blue-100 {
		background-color: #dbeafe;
	}
	.text-blue-600 {
		color: #2563eb;
	}
	.bg-pink-100 {
		background-color: #fce7f3;
	}
	.text-pink-600 {
		color: #db2777;
	}
	.bg-slate-100 {
		background-color: #f1f5f9;
	}
	.text-slate-500 {
		color: #64748b;
	}
</style>
