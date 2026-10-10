<script lang="ts">
	import { untrack } from 'svelte';
	import { page } from '$app/stores';
	import { api, type Person } from '$lib/api/client';
	import RelationshipCalculator from '$lib/components/RelationshipCalculator.svelte';
	import { setQuery } from '$lib/utils/urlState';

	// The calculated pair lives in the URL (see urlState.ts), so a result can be
	// shared, reloaded and reached again with Back.
	const personIdA = $derived($page.url.searchParams.get('personA'));
	const personIdB = $derived($page.url.searchParams.get('personB'));

	let initialPersonA: Person | null = $state(null);
	let initialPersonB: Person | null = $state(null);
	let loading = $state(true);
	// The pair the calculator is showing, and a key that remounts it for a new one.
	let shownPair: string | null = null;
	let mountKey = $state(0);

	$effect(() => {
		const pair = `${personIdA ?? ''}|${personIdB ?? ''}`;
		untrack(() => {
			if (pair !== shownPair) loadPair(pair, personIdA, personIdB);
		});
	});

	async function loadPair(pair: string, idA: string | null, idB: string | null) {
		shownPair = pair;
		loading = true;
		const fetchPerson = (id: string | null) =>
			id ? api.getPerson(id).catch(() => null) : Promise.resolve(null); // Unknown ids are ignored
		const [a, b] = await Promise.all([fetchPerson(idA), fetchPerson(idB)]);
		if (pair !== shownPair) return; // Superseded by a later navigation
		initialPersonA = a;
		initialPersonB = b;
		mountKey++;
		loading = false;
	}

	// Asking for a relationship is navigation-level, like submitting a search: push.
	function handleCalculate(idA: string, idB: string) {
		if (idA === personIdA && idB === personIdB) return; // Already the URL's pair
		shownPair = `${idA}|${idB}`;
		setQuery($page.url, { personA: idA, personB: idB }, { push: true });
	}
</script>

<svelte:head>
	<title>Relationship Calculator | My Family</title>
	<meta name="description" content="Calculate the relationship between two people in your family tree" />
</svelte:head>

<div class="relationship-page">
	<header class="page-header">
		<a href="/" class="back-link">
			<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
				<path d="M19 12H5m0 0l7 7m-7-7l7-7" />
			</svg>
			Back
		</a>
	</header>

	<div class="page-content">
		{#if loading}
			<div class="loading-container">
				<div class="loading-spinner"></div>
				<span>Loading...</span>
			</div>
		{:else}
			{#key mountKey}
				<RelationshipCalculator {initialPersonA} {initialPersonB} onCalculate={handleCalculate} />
			{/key}
		{/if}
	</div>
</div>

<style>
	.relationship-page {
		min-height: 100vh;
		background: #f8fafc;
	}

	.page-header {
		padding: 1rem 1.5rem;
		background: white;
		border-bottom: 1px solid #e2e8f0;
	}

	.back-link {
		display: inline-flex;
		align-items: center;
		gap: 0.375rem;
		color: #64748b;
		text-decoration: none;
		font-size: 0.875rem;
		font-weight: 500;
		transition: color 0.15s;
	}

	.back-link:hover {
		color: #3b82f6;
	}

	.back-link svg {
		width: 1rem;
		height: 1rem;
	}

	.page-content {
		padding: 2rem 1rem;
	}

	@media (max-width: 640px) {
		.page-content {
			padding: 1rem;
		}
	}

	.loading-container {
		display: flex;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		gap: 1rem;
		padding: 4rem 2rem;
		color: #64748b;
	}

	.loading-spinner {
		width: 2rem;
		height: 2rem;
		border: 3px solid #e2e8f0;
		border-top-color: #3b82f6;
		border-radius: 50%;
		animation: spin 0.6s linear infinite;
	}

	@keyframes spin {
		to {
			transform: rotate(360deg);
		}
	}
</style>
