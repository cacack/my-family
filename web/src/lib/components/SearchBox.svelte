<script lang="ts">
	import { api, type SearchResult, formatPersonName, formatLifespan } from '$lib/api/client';

	interface Props {
		onSelect?: (person: SearchResult) => void;
		placeholder?: string;
	}

	let { onSelect, placeholder = 'Search people...' }: Props = $props();

	let query = $state('');
	let results: SearchResult[] = $state([]);
	let loading = $state(false);
	let showDropdown = $state(false);
	let fuzzy = $state(false);
	let debounceTimer: ReturnType<typeof setTimeout> | null = null;

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
		results = [];
		onSelect?.(person);
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
		}, 200);
	}

	function handleKeydown(e: KeyboardEvent) {
		if (e.key === 'Escape') {
			showDropdown = false;
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
		<svg class="search-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
			<circle cx="11" cy="11" r="8" />
			<path d="m21 21-4.35-4.35" />
		</svg>
		<input
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
		/>
		{#if loading}
			<span class="loading-indicator"></span>
		{/if}
		<button
			class="fuzzy-toggle"
			class:active={fuzzy}
			onclick={toggleFuzzy}
			title={fuzzy ? 'Fuzzy search enabled' : 'Enable fuzzy search'}
		>
			~
		</button>
	</div>

	{#if showDropdown && (results.length > 0 || (query.length >= 2 && !loading))}
		<div class="dropdown" role="listbox">
			{#if results.length === 0}
				<div class="no-results">No results found</div>
			{:else}
				{#each results as person}
					<button
						class="result-item"
						onclick={() => handleSelect(person)}
						role="option"
						aria-selected="false"
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

	.search-icon {
		position: absolute;
		left: 0.75rem;
		width: 1rem;
		height: 1rem;
		color: #94a3b8;
		pointer-events: none;
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
		color: #94a3b8;
		font-size: 0.875rem;
		font-weight: 600;
		cursor: pointer;
		transition: all 0.15s;
	}

	.fuzzy-toggle:hover {
		background: #f1f5f9;
	}

	.fuzzy-toggle.active {
		background: #3b82f6;
		border-color: #3b82f6;
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
		color: #94a3b8;
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

	.result-item:hover {
		background: #f1f5f9;
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
		color: #94a3b8;
	}
</style>
