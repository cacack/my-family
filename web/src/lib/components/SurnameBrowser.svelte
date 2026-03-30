<script lang="ts">
	import { api, type SurnameEntry, type LetterCount } from '$lib/api/client';
	import { Button } from '$lib/components/ui/button';

	let letterCounts: LetterCount[] = $state([]);
	let surnames: SurnameEntry[] = $state([]);
	let selectedLetter: string | null = $state(null);
	let loading = $state(true);
	let loadingSurnames = $state(false);

	const ALPHABET = 'ABCDEFGHIJKLMNOPQRSTUVWXYZ'.split('');

	async function loadIndex() {
		loading = true;
		try {
			const result = await api.getSurnameIndex();
			letterCounts = result.letter_counts || [];
			surnames = result.items;
		} catch (e) {
			console.error('Failed to load surname index:', e);
		} finally {
			loading = false;
		}
	}

	async function selectLetter(letter: string) {
		if (selectedLetter === letter) {
			// Deselect - show all surnames
			selectedLetter = null;
			loadIndex();
			return;
		}

		selectedLetter = letter;
		loadingSurnames = true;
		try {
			const result = await api.getSurnameIndex(letter);
			surnames = result.items;
		} catch (e) {
			console.error('Failed to load surnames for letter:', e);
		} finally {
			loadingSurnames = false;
		}
	}

	function getLetterCount(letter: string): number {
		const found = letterCounts.find((lc) => lc.letter === letter);
		return found?.count || 0;
	}

	$effect(() => {
		loadIndex();
	});
</script>

<div class="surname-browser">
	{#if loading}
		<div class="loading">Loading surnames...</div>
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
			{:else if surnames.length === 0}
				<div class="empty">No surnames found</div>
			{:else}
				<div class="surname-grid">
					{#each surnames as entry}
						<a href="/browse/surnames/{encodeURIComponent(entry.surname)}" class="surname-item">
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
		color: #64748b;
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
		background: #3b82f6;
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
		color: #3b82f6;
	}

	.surname-grid {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(200px, 1fr));
		gap: 0.75rem;
	}

	.surname-item {
		display: flex;
		justify-content: space-between;
		align-items: center;
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
		font-weight: 500;
		color: #1e293b;
	}

	.surname-count {
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
