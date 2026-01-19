<script lang="ts">
	import { page } from '$app/stores';
	import { onMount } from 'svelte';
	import { api, type Person } from '$lib/api/client';
	import RelationshipCalculator from '$lib/components/RelationshipCalculator.svelte';

	let initialPersonA: Person | null = $state(null);
	let initialPersonB: Person | null = $state(null);
	let loading = $state(true);

	onMount(async () => {
		// Check for query parameters to pre-populate the selectors
		const params = $page.url.searchParams;
		const personIdA = params.get('personA');
		const personIdB = params.get('personB');

		const loadPromises: Promise<void>[] = [];

		if (personIdA) {
			loadPromises.push(
				api.getPerson(personIdA)
					.then((person) => {
						initialPersonA = person;
					})
					.catch(() => {
						// Person not found, ignore
					})
			);
		}

		if (personIdB) {
			loadPromises.push(
				api.getPerson(personIdB)
					.then((person) => {
						initialPersonB = person;
					})
					.catch(() => {
						// Person not found, ignore
					})
			);
		}

		await Promise.all(loadPromises);
		loading = false;
	});
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

	<main class="page-content">
		{#if loading}
			<div class="loading-container">
				<div class="loading-spinner"></div>
				<span>Loading...</span>
			</div>
		{:else}
			<RelationshipCalculator {initialPersonA} {initialPersonB} />
		{/if}
	</main>
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
