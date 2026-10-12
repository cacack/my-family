<script lang="ts">
	import { untrack } from 'svelte';
	import { page } from '$app/stores';
	import { api, type Person } from '$lib/api/client';
	import RelationshipCalculator from '$lib/components/RelationshipCalculator.svelte';
	import ActionBar from '$lib/components/ActionBar.svelte';
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
	const pairKey = (idA: string | null, idB: string | null) => `${idA ?? ''}|${idB ?? ''}`;

	$effect(() => {
		const pair = pairKey(personIdA, personIdB);
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
		shownPair = pairKey(idA, idB); // So the effect does not reload the pair just calculated
		setQuery($page.url, { personA: idA, personB: idB }, { push: true });
	}
</script>

<svelte:head>
	<title>Relationship Calculator | My Family</title>
	<meta name="description" content="Calculate the relationship between two people in your family tree" />
</svelte:head>

<div class="relationship-page">
	<div class="page-content">
		<ActionBar back={{ href: '/', label: 'Back' }} />
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
		color: var(--color-text-muted);
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
