<script lang="ts">
	import { untrack } from 'svelte';
	import { page } from '$app/stores';
	import { api, type SurnameEntry, type LetterCount } from '$lib/api/client';
	import { Button } from '$lib/components/ui/button';
	import { readEnum, setQuery } from '$lib/utils/urlState';
	import ErrorState from './ErrorState.svelte';

	let letterCounts: LetterCount[] = $state([]);
	let allSurnames: SurnameEntry[] = $state([]);
	let letterSurnames: SurnameEntry[] = $state([]);
	let loading = $state(true);
	let loadingSurnames = $state(false);
	let error: string | null = $state(null);

	const ALPHABET = 'ABCDEFGHIJKLMNOPQRSTUVWXYZ'.split('');

	// The selected letter lives in the URL (see urlState.ts), so Back, reload
	// and a copied link keep it. It filters the list, so it replaces.
	const selectedLetter = $derived(readEnum($page.url.searchParams, 'letter', ALPHABET, '') || null);
	const surnames = $derived(selectedLetter ? letterSurnames : allSurnames);

	async function loadIndex() {
		loading = true;
		error = null;
		try {
			const result = await api.getSurnameIndex();
			letterCounts = result.letter_counts || [];
			allSurnames = result.items;
		} catch (e) {
			console.error('Failed to load surname index:', e);
			error = 'Failed to load surnames. Please try again.';
		} finally {
			loading = false;
		}
	}

	function selectLetter(letter: string) {
		// Selecting the active letter again clears the filter.
		setQuery($page.url, { letter: selectedLetter === letter ? null : letter });
	}

	// Back/Forward can start loads faster than they finish: only the latest one may land.
	let letterSeq = 0;

	async function loadLetter(letter: string) {
		const seq = ++letterSeq;
		loadingSurnames = true;
		error = null;
		try {
			const result = await api.getSurnameIndex(letter);
			if (seq !== letterSeq) return;
			letterSurnames = result.items;
		} catch (e) {
			if (seq !== letterSeq) return;
			console.error('Failed to load surnames for letter:', e);
			error = 'Failed to load surnames. Please try again.';
		} finally {
			if (seq === letterSeq) loadingSurnames = false;
		}
	}

	function getLetterCount(letter: string): number {
		const found = letterCounts.find((lc) => lc.letter === letter);
		return found?.count || 0;
	}

	function retry() {
		if (letterCounts.length === 0) loadIndex();
		if (selectedLetter) loadLetter(selectedLetter);
	}

	$effect(() => {
		untrack(loadIndex);
	});

	$effect(() => {
		const letter = selectedLetter;
		untrack(() => {
			if (letter) {
				loadLetter(letter);
			} else {
				// Back to the full list: drop any in-flight letter load and its
				// error, which belonged to the letter view.
				letterSeq++;
				loadingSurnames = false;
				if (letterCounts.length > 0) error = null;
			}
		});
	});
</script>

<div class="surname-browser">
	{#if loading}
		<div class="loading">Loading surnames...</div>
	{:else if error && letterCounts.length === 0}
		<ErrorState message={error} onRetry={retry} />
	{:else}
		<!-- A-Z Letter Navigation -->
		<div class="letter-nav">
			{#each ALPHABET as letter}
				{@const count = getLetterCount(letter)}
				<Button
					variant={selectedLetter === letter ? 'default' : 'outline'}
					size="sm"
					class="letter-btn relative"
					disabled={count === 0}
					onclick={() => selectLetter(letter)}
					title={count > 0 ? `${count} surname${count === 1 ? '' : 's'}` : 'No surnames'}
				>
					{letter}
					{#if count > 0}
						<span class="count-badge" class:count-badge-active={selectedLetter === letter}>{count}</span>
					{/if}
				</Button>
			{/each}
		</div>

		<!-- Surname List -->
		<div class="surname-list">
			{#if loadingSurnames}
				<div class="loading">Loading...</div>
			{:else if error}
				<ErrorState message={error} onRetry={retry} />
			{:else if surnames.length === 0}
				<div class="empty">No surnames found</div>
			{:else}
				<div class="surname-grid">
					{#each surnames as entry}
						<a href="/browse/surnames/{encodeURIComponent(entry.surname)}" class="surname-item" data-slot="card-link">
							<span class="surname-name">{entry.surname || '(Unknown)'}</span>
							<span class="surname-count">{entry.count}</span>
						</a>
					{/each}
				</div>
			{/if}
		</div>
	{/if}
</div>

<style>
	.surname-browser {
		max-width: 100%;
	}

	.loading,
	.empty {
		text-align: center;
		padding: 2rem;
		color: var(--color-text-muted);
	}

	.letter-nav {
		display: flex;
		flex-wrap: wrap;
		gap: 0.375rem;
		margin-bottom: 1.5rem;
		padding: 1rem;
		background: #f8fafc;
		border-radius: 8px;
	}

.count-badge {
		position: absolute;
		top: -4px;
		right: -4px;
		min-width: 16px;
		height: 16px;
		padding: 0 4px;
		background: var(--color-action);
		color: white;
		font-size: 0.625rem;
		font-weight: 600;
		border-radius: 8px;
		display: flex;
		align-items: center;
		justify-content: center;
	}

	.count-badge-active {
		background: white;
		color: var(--color-link);
	}

	.surname-grid {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(min(200px, 100%), 1fr));
		gap: 0.75rem;
	}

	.surname-item {
		display: flex;
		justify-content: space-between;
		align-items: center;
		gap: 0.5rem;
		padding: 0.75rem 1rem;
		background: white;
		border: 1px solid #e2e8f0;
		border-radius: 8px;
		text-decoration: none;
		color: inherit;
		transition: all 0.15s ease;
	}

	.surname-item:hover {
		background: #f8fafc;
		border-color: #3b82f6;
		transform: translateY(-1px);
		box-shadow: 0 2px 4px rgba(0, 0, 0, 0.05);
	}

	.surname-name {
		min-width: 0;
		overflow-wrap: anywhere;
		font-weight: 500;
		color: #1e293b;
	}

	.surname-count {
		flex-shrink: 0;
		display: flex;
		align-items: center;
		justify-content: center;
		min-width: 24px;
		height: 24px;
		padding: 0 8px;
		background: #f1f5f9;
		color: #475569;
		font-size: 0.75rem;
		font-weight: 600;
		border-radius: 12px;
	}

	/* Mobile responsive */
	@media (max-width: 640px) {
		.letter-nav {
			gap: 0.25rem;
			padding: 0.75rem;
		}

		.count-badge {
			display: none;
		}

		.surname-grid {
			grid-template-columns: 1fr;
		}
	}
</style>
