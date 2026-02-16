<script lang="ts">
	import {
		api,
		type SearchResult,
		type PlaceEntry,
		formatPersonName,
		formatGenDate,
		formatLifespan
	} from '$lib/api/client';

	// Form state
	let query = $state('');
	let fuzzy = $state(false);
	let soundex = $state(false);
	let birthYearFrom = $state('');
	let birthYearTo = $state('');
	let deathYearFrom = $state('');
	let deathYearTo = $state('');
	let birthPlace = $state('');
	let deathPlace = $state('');
	let sort = $state<'relevance' | 'name' | 'birth_date' | 'death_date'>('relevance');
	let order = $state<'asc' | 'desc'>('desc');

	// Results state
	let results: SearchResult[] = $state([]);
	let total = $state(0);
	let loading = $state(false);
	let searched = $state(false);
	let limit = $state(20);
	let error: string | null = $state(null);

	// Place autocomplete state
	let allPlaces: PlaceEntry[] = $state([]);
	let birthPlaceSuggestions: PlaceEntry[] = $state([]);
	let deathPlaceSuggestions: PlaceEntry[] = $state([]);
	let showBirthPlaceDropdown = $state(false);
	let showDeathPlaceDropdown = $state(false);
	let birthPlaceHighlight = $state(-1);
	let deathPlaceHighlight = $state(-1);

	// Derived state
	let hasAnyCriteria = $derived(
		query.trim().length > 0 ||
			birthYearFrom.trim().length > 0 ||
			birthYearTo.trim().length > 0 ||
			deathYearFrom.trim().length > 0 ||
			deathYearTo.trim().length > 0 ||
			birthPlace.trim().length > 0 ||
			deathPlace.trim().length > 0
	);

	// Load places on mount
	$effect(() => {
		loadPlaces();
	});

	// Debounced birth place filtering
	$effect(() => {
		const val = birthPlace;
		const timer = setTimeout(() => {
			birthPlaceSuggestions = filterPlaces(val);
		}, 300);
		return () => clearTimeout(timer);
	});

	// Debounced death place filtering
	$effect(() => {
		const val = deathPlace;
		const timer = setTimeout(() => {
			deathPlaceSuggestions = filterPlaces(val);
		}, 300);
		return () => clearTimeout(timer);
	});

	async function loadPlaces() {
		try {
			const result = await api.getPlaceHierarchy();
			allPlaces = [...result.items];
			// Load child places in parallel to avoid N+1 serial waterfall
			const childPromises = result.items
				.filter((place) => place.has_children)
				.map((place) =>
					api.getPlaceHierarchy(place.full_name || place.name).catch(() => ({ items: [] as PlaceEntry[] }))
				);
			const childResults = await Promise.all(childPromises);
			allPlaces = [...allPlaces, ...childResults.flatMap((r) => r.items)];
		} catch {
			// Silently fail — autocomplete is optional
		}
	}

	function filterPlaces(input: string): PlaceEntry[] {
		if (!input.trim()) return [];
		const lower = input.toLowerCase();
		return allPlaces
			.filter(
				(p) =>
					p.full_name.toLowerCase().includes(lower) || p.name.toLowerCase().includes(lower)
			)
			.slice(0, 10);
	}

	function selectBirthPlace(place: PlaceEntry) {
		birthPlace = place.full_name;
		showBirthPlaceDropdown = false;
		birthPlaceHighlight = -1;
		birthPlaceSuggestions = [];
	}

	function selectDeathPlace(place: PlaceEntry) {
		deathPlace = place.full_name;
		showDeathPlaceDropdown = false;
		deathPlaceHighlight = -1;
		deathPlaceSuggestions = [];
	}

	function handleBirthPlaceKeydown(e: KeyboardEvent) {
		if (e.key === 'Escape') {
			showBirthPlaceDropdown = false;
			birthPlaceHighlight = -1;
			return;
		}
		if (!showBirthPlaceDropdown || birthPlaceSuggestions.length === 0) return;
		if (e.key === 'ArrowDown') {
			e.preventDefault();
			birthPlaceHighlight = (birthPlaceHighlight + 1) % birthPlaceSuggestions.length;
		} else if (e.key === 'ArrowUp') {
			e.preventDefault();
			birthPlaceHighlight =
				birthPlaceHighlight <= 0 ? birthPlaceSuggestions.length - 1 : birthPlaceHighlight - 1;
		} else if (e.key === 'Enter' && birthPlaceHighlight >= 0) {
			e.preventDefault();
			selectBirthPlace(birthPlaceSuggestions[birthPlaceHighlight]);
		}
	}

	function handleDeathPlaceKeydown(e: KeyboardEvent) {
		if (e.key === 'Escape') {
			showDeathPlaceDropdown = false;
			deathPlaceHighlight = -1;
			return;
		}
		if (!showDeathPlaceDropdown || deathPlaceSuggestions.length === 0) return;
		if (e.key === 'ArrowDown') {
			e.preventDefault();
			deathPlaceHighlight = (deathPlaceHighlight + 1) % deathPlaceSuggestions.length;
		} else if (e.key === 'ArrowUp') {
			e.preventDefault();
			deathPlaceHighlight =
				deathPlaceHighlight <= 0 ? deathPlaceSuggestions.length - 1 : deathPlaceHighlight - 1;
		} else if (e.key === 'Enter' && deathPlaceHighlight >= 0) {
			e.preventDefault();
			selectDeathPlace(deathPlaceSuggestions[deathPlaceHighlight]);
		}
	}

	function buildSearchParams() {
		const params: Parameters<typeof api.searchPersons>[0] = {};

		if (query.trim()) params.q = query.trim();
		if (fuzzy) params.fuzzy = true;
		if (soundex) params.soundex = true;
		const isYear = (v: string) => /^\d{1,4}$/.test(v);
		if (birthYearFrom.trim() && isYear(birthYearFrom.trim()))
			params.birth_date_from = `${birthYearFrom.trim()}-01-01`;
		if (birthYearTo.trim() && isYear(birthYearTo.trim()))
			params.birth_date_to = `${birthYearTo.trim()}-12-31`;
		if (deathYearFrom.trim() && isYear(deathYearFrom.trim()))
			params.death_date_from = `${deathYearFrom.trim()}-01-01`;
		if (deathYearTo.trim() && isYear(deathYearTo.trim()))
			params.death_date_to = `${deathYearTo.trim()}-12-31`;
		if (birthPlace.trim()) params.birth_place = birthPlace.trim();
		if (deathPlace.trim()) params.death_place = deathPlace.trim();
		params.sort = sort;
		params.order = order;
		params.limit = limit;

		return params;
	}

	async function performSearch() {
		if (!hasAnyCriteria) return;

		loading = true;
		error = null;
		searched = true;
		try {
			const params = buildSearchParams();
			const result = await api.searchPersons(params);
			results = result.items;
			total = result.total;
		} catch (e) {
			const apiError = e as { message?: string };
			error = apiError.message || 'Search failed. Please try again.';
			results = [];
			total = 0;
		} finally {
			loading = false;
		}
	}

	function handleFormSubmit(e: Event) {
		e.preventDefault();
		limit = 20;
		performSearch();
	}

	function handleNameKeydown(e: KeyboardEvent) {
		if (e.key === 'Enter') {
			e.preventDefault();
			limit = 20;
			performSearch();
		}
	}

	function handleSortChange(e: Event) {
		const select = e.target as HTMLSelectElement;
		sort = select.value as typeof sort;
		if (searched) performSearch();
	}

	function handleOrderToggle() {
		order = order === 'asc' ? 'desc' : 'asc';
		if (searched) performSearch();
	}

	function clearAll() {
		query = '';
		fuzzy = false;
		soundex = false;
		birthYearFrom = '';
		birthYearTo = '';
		deathYearFrom = '';
		deathYearTo = '';
		birthPlace = '';
		deathPlace = '';
		sort = 'relevance';
		order = 'desc';
		results = [];
		total = 0;
		searched = false;
		limit = 20;
		error = null;
	}

	function loadMore() {
		limit += 20;
		performSearch();
	}

	function scoreColor(score: number | undefined): string {
		if (score === undefined) return 'score-low';
		if (score > 0.8) return 'score-high';
		if (score >= 0.5) return 'score-medium';
		return 'score-low';
	}

	function scoreLabel(score: number | undefined): string {
		if (score === undefined) return '—';
		return `${Math.round(score * 100)}%`;
	}
</script>

<svelte:head>
	<title>Advanced Search | My Family</title>
</svelte:head>

<div class="search-page">
	<header class="page-header">
		<h1>Advanced Search</h1>
		<p class="description">Search people by name, dates, and places</p>
	</header>

	<form class="search-form" onsubmit={handleFormSubmit}>
		<!-- Name query -->
		<div class="form-section">
			<label class="field-label" for="search-query">Name</label>
			<input
				id="search-query"
				type="text"
				bind:value={query}
				onkeydown={handleNameKeydown}
				placeholder="Name (e.g., Smith, John Smith)"
				class="name-input"
			/>
		</div>

		<!-- Toggle buttons -->
		<div class="toggle-row">
			<button
				type="button"
				class="pill-toggle"
				class:active={fuzzy}
				onclick={() => (fuzzy = !fuzzy)}
				aria-pressed={fuzzy}
				title={fuzzy ? 'Fuzzy matching enabled' : 'Enable fuzzy matching'}
			>
				<span class="toggle-icon">~</span>
				Fuzzy
			</button>
			<button
				type="button"
				class="pill-toggle"
				class:active={soundex}
				onclick={() => (soundex = !soundex)}
				aria-pressed={soundex}
				title={soundex ? 'Phonetic matching enabled' : 'Enable phonetic matching'}
			>
				Phonetic
			</button>
		</div>

		<!-- Date ranges -->
		<div class="form-section">
			<div class="date-grid">
				<div class="date-group">
					<span class="field-label">Birth date</span>
					<div class="date-range">
						<label class="date-label">
							From
							<input
								type="text"
								bind:value={birthYearFrom}
								inputmode="numeric"
								pattern="[0-9]*"
								placeholder="YYYY"
								class="year-input"
								maxlength={4}
							/>
						</label>
						<label class="date-label">
							To
							<input
								type="text"
								bind:value={birthYearTo}
								inputmode="numeric"
								pattern="[0-9]*"
								placeholder="YYYY"
								class="year-input"
								maxlength={4}
							/>
						</label>
					</div>
				</div>
				<div class="date-group">
					<span class="field-label">Death date</span>
					<div class="date-range">
						<label class="date-label">
							From
							<input
								type="text"
								bind:value={deathYearFrom}
								inputmode="numeric"
								pattern="[0-9]*"
								placeholder="YYYY"
								class="year-input"
								maxlength={4}
							/>
						</label>
						<label class="date-label">
							To
							<input
								type="text"
								bind:value={deathYearTo}
								inputmode="numeric"
								pattern="[0-9]*"
								placeholder="YYYY"
								class="year-input"
								maxlength={4}
							/>
						</label>
					</div>
				</div>
			</div>
		</div>

		<!-- Place filters -->
		<div class="form-section">
			<div class="place-grid">
				<div class="place-field">
					<label class="field-label" for="birth-place-input">Birth place</label>
					<div class="place-wrapper">
						<input
							id="birth-place-input"
							type="text"
							bind:value={birthPlace}
							onfocus={() => {
								if (birthPlaceSuggestions.length > 0) showBirthPlaceDropdown = true;
							}}
							onblur={() => { showBirthPlaceDropdown = false; birthPlaceHighlight = -1; }}
							onkeydown={handleBirthPlaceKeydown}
							placeholder="e.g., Springfield, IL"
							role="combobox"
							aria-expanded={showBirthPlaceDropdown}
							aria-haspopup="listbox"
							aria-controls="birth-place-listbox"
							aria-autocomplete="list"
							aria-activedescendant={birthPlaceHighlight >= 0 ? `bp-option-${birthPlaceHighlight}` : undefined}
							autocomplete="off"
						/>
						{#if showBirthPlaceDropdown && birthPlaceSuggestions.length > 0}
							<!-- svelte-ignore a11y_interactive_supports_focus -->
						<div class="place-dropdown" role="listbox" id="birth-place-listbox" aria-label="Birth place suggestions" onmousedown={(e) => e.preventDefault()}>
								{#each birthPlaceSuggestions as place, i}
									<button
										type="button"
										id="bp-option-{i}"
										class="place-option"
										class:highlighted={i === birthPlaceHighlight}
										onclick={() => selectBirthPlace(place)}
										role="option"
										aria-selected={i === birthPlaceHighlight}
									>
										<span class="place-name">{place.full_name}</span>
										<span class="place-count">({place.count})</span>
									</button>
								{/each}
							</div>
						{/if}
					</div>
				</div>
				<div class="place-field">
					<label class="field-label" for="death-place-input">Death place</label>
					<div class="place-wrapper">
						<input
							id="death-place-input"
							type="text"
							bind:value={deathPlace}
							onfocus={() => {
								if (deathPlaceSuggestions.length > 0) showDeathPlaceDropdown = true;
							}}
							onblur={() => { showDeathPlaceDropdown = false; deathPlaceHighlight = -1; }}
							onkeydown={handleDeathPlaceKeydown}
							placeholder="e.g., Springfield, IL"
							role="combobox"
							aria-expanded={showDeathPlaceDropdown}
							aria-haspopup="listbox"
							aria-controls="death-place-listbox"
							aria-autocomplete="list"
							aria-activedescendant={deathPlaceHighlight >= 0 ? `dp-option-${deathPlaceHighlight}` : undefined}
							autocomplete="off"
						/>
						{#if showDeathPlaceDropdown && deathPlaceSuggestions.length > 0}
							<!-- svelte-ignore a11y_interactive_supports_focus -->
						<div class="place-dropdown" role="listbox" id="death-place-listbox" aria-label="Death place suggestions" onmousedown={(e) => e.preventDefault()}>
								{#each deathPlaceSuggestions as place, i}
									<button
										type="button"
										id="dp-option-{i}"
										class="place-option"
										class:highlighted={i === deathPlaceHighlight}
										onclick={() => selectDeathPlace(place)}
										role="option"
										aria-selected={i === deathPlaceHighlight}
									>
										<span class="place-name">{place.full_name}</span>
										<span class="place-count">({place.count})</span>
									</button>
								{/each}
							</div>
						{/if}
					</div>
				</div>
			</div>
		</div>

		<!-- Action buttons -->
		<div class="form-actions">
			<button type="submit" class="btn btn-primary" disabled={!hasAnyCriteria || loading}>
				{loading ? 'Searching...' : 'Search'}
			</button>
			<button type="button" class="btn btn-secondary" onclick={clearAll}>Clear All</button>
		</div>
	</form>

	<!-- Results section -->
	{#if searched}
		<div class="results-section">
			<!-- Sort controls and result count -->
			<div class="results-header">
				<div class="results-info">
					{#if loading}
						<span class="results-count" aria-live="polite">Searching...</span>
					{:else}
						<span class="results-count" aria-live="polite">Showing {results.length} of {total} results</span>
					{/if}
					{#if soundex}
						<span class="mode-badge">Phonetic</span>
					{/if}
				</div>
				<div class="sort-controls">
					<label class="sort-label">
						Sort by:
						<select value={sort} onchange={handleSortChange}>
							<option value="relevance">Relevance</option>
							<option value="name">Name</option>
							<option value="birth_date">Birth Date</option>
							<option value="death_date">Death Date</option>
						</select>
					</label>
					<button
						class="order-btn"
						onclick={handleOrderToggle}
						title="Toggle sort order ({order === 'asc' ? 'ascending' : 'descending'})"
					>
						{#if order === 'asc'}
							<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
								<path d="M12 5v14M5 12l7-7 7 7" />
							</svg>
						{:else}
							<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
								<path d="M12 19V5M5 12l7 7 7-7" />
							</svg>
						{/if}
					</button>
				</div>
			</div>

			{#if error}
				<div class="error-message" role="alert">{error}</div>
			{:else if !loading && results.length === 0}
				<div class="empty-state">No results found. Try adjusting your search criteria.</div>
			{:else if !loading}
				<!-- Desktop table -->
				<div class="results-table-wrapper">
					<table class="results-table">
						<thead>
							<tr>
								<th>Name</th>
								<th>Birth</th>
								<th>Death</th>
								<th>Score</th>
							</tr>
						</thead>
						<tbody>
							{#each results as person}
								<tr>
									<td>
										<a href="/persons/{person.id}" class="person-link">
											{formatPersonName(person)}
										</a>
									</td>
									<td class="date-cell">{formatGenDate(person.birth_date)}</td>
									<td class="date-cell">{formatGenDate(person.death_date)}</td>
									<td class="score-cell">
										<span class="score-badge {scoreColor(person.score)}">{scoreLabel(person.score)}</span>
									</td>
								</tr>
							{/each}
						</tbody>
					</table>
				</div>

				<!-- Mobile cards -->
				<div class="results-cards">
					{#each results as person}
						<a href="/persons/{person.id}" class="result-card">
							<div class="card-top">
								<span class="card-name">{formatPersonName(person)}</span>
								<span class="score-badge {scoreColor(person.score)}">{scoreLabel(person.score)}</span>
							</div>
							<span class="card-lifespan">{formatLifespan(person)}</span>
						</a>
					{/each}
				</div>

				<!-- Load more -->
				{#if results.length < total}
					<div class="load-more">
						<button class="btn btn-secondary" onclick={loadMore} disabled={loading}>
							Load more
						</button>
					</div>
				{/if}
			{/if}
		</div>
	{/if}
</div>

<style>
	.search-page {
		max-width: 1000px;
		margin: 0 auto;
		padding: 1.5rem;
	}

	.page-header {
		margin-bottom: 1.5rem;
	}

	.page-header h1 {
		margin: 0 0 0.5rem;
		font-size: 1.5rem;
		color: #1e293b;
	}

	.description {
		margin: 0;
		color: #64748b;
		font-size: 0.9375rem;
	}

	/* Search form */
	.search-form {
		background: white;
		border: 1px solid #e2e8f0;
		border-radius: 12px;
		padding: 1.5rem;
		margin-bottom: 2rem;
	}

	.form-section {
		margin-bottom: 1.25rem;
	}

	.field-label {
		display: block;
		font-size: 0.875rem;
		font-weight: 500;
		color: #475569;
		margin-bottom: 0.375rem;
	}

	.name-input {
		width: 100%;
		padding: 0.625rem 0.75rem;
		border: 1px solid #e2e8f0;
		border-radius: 6px;
		font-size: 0.875rem;
	}

	.name-input:focus {
		outline: none;
		border-color: #3b82f6;
		box-shadow: 0 0 0 3px rgba(59, 130, 246, 0.1);
	}

	/* Toggle buttons */
	.toggle-row {
		display: flex;
		gap: 0.5rem;
		margin-bottom: 1.25rem;
	}

	.pill-toggle {
		display: inline-flex;
		align-items: center;
		gap: 0.375rem;
		padding: 0.375rem 0.875rem;
		border: 1px solid #e2e8f0;
		border-radius: 9999px;
		background: white;
		color: #64748b;
		font-size: 0.8125rem;
		font-weight: 500;
		cursor: pointer;
		transition: all 0.15s;
	}

	.pill-toggle:hover {
		background: #f1f5f9;
		border-color: #cbd5e1;
	}

	.pill-toggle.active {
		background: #3b82f6;
		border-color: #3b82f6;
		color: white;
	}

	.toggle-icon {
		font-weight: 700;
		font-size: 0.9375rem;
	}

	/* Date ranges */
	.date-grid {
		display: grid;
		grid-template-columns: repeat(2, 1fr);
		gap: 1rem;
	}

	.date-group {
		display: flex;
		flex-direction: column;
	}

	.date-range {
		display: flex;
		align-items: center;
		gap: 0.75rem;
	}

	.date-label {
		display: flex;
		align-items: center;
		gap: 0.375rem;
		font-size: 0.8125rem;
		color: #64748b;
	}

	.year-input {
		width: 5.5rem;
		padding: 0.5rem 0.625rem;
		border: 1px solid #e2e8f0;
		border-radius: 6px;
		font-size: 0.875rem;
		text-align: center;
	}

	.year-input:focus {
		outline: none;
		border-color: #3b82f6;
		box-shadow: 0 0 0 3px rgba(59, 130, 246, 0.1);
	}

	/* Place filters */
	.place-grid {
		display: grid;
		grid-template-columns: repeat(2, 1fr);
		gap: 1rem;
	}

	.place-field {
		display: flex;
		flex-direction: column;
	}

	.place-wrapper {
		position: relative;
	}

	.place-wrapper input {
		width: 100%;
		padding: 0.625rem 0.75rem;
		border: 1px solid #e2e8f0;
		border-radius: 6px;
		font-size: 0.875rem;
	}

	.place-wrapper input:focus {
		outline: none;
		border-color: #3b82f6;
		box-shadow: 0 0 0 3px rgba(59, 130, 246, 0.1);
	}

	.place-dropdown {
		position: absolute;
		top: calc(100% + 4px);
		left: 0;
		right: 0;
		background: white;
		border: 1px solid #e2e8f0;
		border-radius: 8px;
		box-shadow: 0 4px 12px rgba(0, 0, 0, 0.1);
		z-index: 100;
		max-height: 200px;
		overflow-y: auto;
	}

	.place-option {
		display: flex;
		justify-content: space-between;
		align-items: center;
		width: 100%;
		padding: 0.5rem 0.75rem;
		border: none;
		background: none;
		cursor: pointer;
		text-align: left;
		font-size: 0.8125rem;
		transition: background 0.15s;
	}

	.place-option:hover,
	.place-option.highlighted {
		background: #f1f5f9;
	}

	.place-option.highlighted {
		outline: 2px solid #3b82f6;
		outline-offset: -2px;
	}

	.place-option:first-child {
		border-radius: 8px 8px 0 0;
	}

	.place-option:last-child {
		border-radius: 0 0 8px 8px;
	}

	.place-option:only-child {
		border-radius: 8px;
	}

	.place-name {
		color: #1e293b;
	}

	.place-count {
		color: #94a3b8;
		font-size: 0.75rem;
	}

	/* Action buttons */
	.form-actions {
		display: flex;
		gap: 0.75rem;
		margin-top: 1.5rem;
		padding-top: 1rem;
		border-top: 1px solid #e2e8f0;
	}

	.btn {
		padding: 0.5rem 1.25rem;
		border: 1px solid #cbd5e1;
		border-radius: 6px;
		background: white;
		font-size: 0.875rem;
		font-weight: 500;
		cursor: pointer;
		color: #475569;
		transition: all 0.15s;
	}

	.btn:hover:not(:disabled) {
		background: #f1f5f9;
	}

	.btn:disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}

	.btn-primary {
		background: #3b82f6;
		border-color: #3b82f6;
		color: white;
	}

	.btn-primary:hover:not(:disabled) {
		background: #2563eb;
	}

	.btn-secondary {
		background: white;
		border-color: #cbd5e1;
		color: #475569;
	}

	/* Results section */
	.results-section {
		background: white;
		border: 1px solid #e2e8f0;
		border-radius: 12px;
		padding: 1.5rem;
	}

	.results-header {
		display: flex;
		justify-content: space-between;
		align-items: center;
		margin-bottom: 1rem;
		flex-wrap: wrap;
		gap: 0.75rem;
	}

	.results-info {
		display: flex;
		align-items: center;
		gap: 0.5rem;
	}

	.results-count {
		font-size: 0.875rem;
		color: #64748b;
	}

	.mode-badge {
		display: inline-flex;
		align-items: center;
		padding: 0.125rem 0.5rem;
		border-radius: 9999px;
		font-size: 0.6875rem;
		font-weight: 600;
		text-transform: uppercase;
		letter-spacing: 0.025em;
		background: #eff6ff;
		color: #3b82f6;
	}

	.sort-controls {
		display: flex;
		align-items: center;
		gap: 0.5rem;
	}

	.sort-label {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		font-size: 0.875rem;
		color: #475569;
	}

	.sort-label select {
		padding: 0.375rem 0.75rem;
		border: 1px solid #cbd5e1;
		border-radius: 6px;
		background: white;
		font-size: 0.875rem;
	}

	.order-btn {
		display: flex;
		align-items: center;
		justify-content: center;
		width: 2.25rem;
		height: 2.25rem;
		border: 1px solid #cbd5e1;
		border-radius: 6px;
		background: white;
		cursor: pointer;
	}

	.order-btn:hover {
		background: #f1f5f9;
	}

	.order-btn svg {
		width: 1rem;
		height: 1rem;
		color: #64748b;
	}

	.error-message {
		padding: 0.75rem;
		background: #fef2f2;
		border: 1px solid #fecaca;
		border-radius: 6px;
		color: #dc2626;
		font-size: 0.875rem;
	}

	.empty-state {
		text-align: center;
		padding: 2rem;
		color: #64748b;
		font-size: 0.875rem;
	}

	/* Results table (desktop) */
	.results-table-wrapper {
		display: block;
		overflow-x: auto;
	}

	.results-table {
		width: 100%;
		border-collapse: collapse;
	}

	.results-table th {
		text-align: left;
		padding: 0.625rem 0.75rem;
		font-size: 0.75rem;
		font-weight: 600;
		color: #64748b;
		text-transform: uppercase;
		letter-spacing: 0.05em;
		border-bottom: 2px solid #e2e8f0;
	}

	.results-table td {
		padding: 0.75rem;
		font-size: 0.875rem;
		border-bottom: 1px solid #f1f5f9;
		color: #475569;
	}

	.results-table tr:hover {
		background: #fafafa;
	}

	.person-link {
		color: #1e293b;
		text-decoration: none;
		font-weight: 500;
	}

	.person-link:hover {
		color: #3b82f6;
	}

	.date-cell {
		white-space: nowrap;
	}

	.score-cell {
		white-space: nowrap;
	}

	.score-badge {
		display: inline-flex;
		align-items: center;
		padding: 0.125rem 0.5rem;
		border-radius: 9999px;
		font-size: 0.75rem;
		font-weight: 600;
	}

	.score-high {
		background: #dcfce7;
		color: #15803d;
	}

	.score-medium {
		background: #fef3c7;
		color: #b45309;
	}

	.score-low {
		background: #f1f5f9;
		color: #64748b;
	}

	/* Results cards (mobile) */
	.results-cards {
		display: none;
	}

	.result-card {
		display: block;
		padding: 0.875rem;
		border: 1px solid #e2e8f0;
		border-radius: 8px;
		text-decoration: none;
		transition: all 0.15s;
	}

	.result-card:hover {
		border-color: #cbd5e1;
		box-shadow: 0 2px 4px rgba(0, 0, 0, 0.05);
	}

	.card-top {
		display: flex;
		justify-content: space-between;
		align-items: center;
		margin-bottom: 0.25rem;
	}

	.card-name {
		font-weight: 500;
		color: #1e293b;
		font-size: 0.875rem;
	}

	.card-lifespan {
		font-size: 0.8125rem;
		color: #64748b;
	}

	/* Load more */
	.load-more {
		display: flex;
		justify-content: center;
		margin-top: 1.25rem;
		padding-top: 1rem;
		border-top: 1px solid #f1f5f9;
	}

	/* Responsive */
	@media (max-width: 640px) {
		.search-page {
			padding: 1rem;
		}

		.date-grid {
			grid-template-columns: 1fr;
		}

		.place-grid {
			grid-template-columns: 1fr;
		}

		.form-actions {
			flex-direction: column;
		}

		.form-actions .btn {
			width: 100%;
			text-align: center;
		}

		.results-table-wrapper {
			display: none;
		}

		.results-cards {
			display: flex;
			flex-direction: column;
			gap: 0.5rem;
		}

		.results-header {
			flex-direction: column;
			align-items: flex-start;
		}

		.sort-controls {
			width: 100%;
		}

		.place-dropdown {
			width: 100%;
		}
	}
</style>
