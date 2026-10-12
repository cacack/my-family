<script lang="ts">
	import { page } from '$app/stores';
	import { api, recordLoadError, type FamilyGroupSheet } from '$lib/api/client';
	import { Button } from '$lib/components/ui/button';
	import ActionBar from '$lib/components/ActionBar.svelte';
	import FamilyGroupSheetComponent from '$lib/components/FamilyGroupSheet.svelte';
	import ErrorState from '$lib/components/ErrorState.svelte';

	let data: FamilyGroupSheet | null = $state(null);
	let loading = $state(true);
	let error: string | null = $state(null);

	async function loadGroupSheet(id: string) {
		loading = true;
		error = null;
		try {
			data = await api.getFamilyGroupSheet(id);
		} catch (e) {
			error = recordLoadError(e, 'family');
			data = null;
		} finally {
			loading = false;
		}
	}

	function handlePrint() {
		window.print();
	}

	function getTitle(): string {
		if (!data) return 'Family Group Sheet';
		const parts: string[] = [];
		if (data.husband) {
			parts.push(`${data.husband.given_name} ${data.husband.surname}`);
		}
		if (data.wife) {
			parts.push(`${data.wife.given_name} ${data.wife.surname}`);
		}
		return parts.length > 0 ? `${parts.join(' & ')} - Group Sheet` : 'Family Group Sheet';
	}

	$effect(() => {
		const id = $page.params.id;
		if (id) {
			loadGroupSheet(id);
		}
	});
</script>

<svelte:head>
	<title>{getTitle()} | My Family</title>
</svelte:head>

<div class="group-sheet-page">
	<ActionBar back={{ href: `/families/${$page.params.id}`, label: 'Back to Family' }}>
		{#snippet actions()}
			<Button onclick={handlePrint}>Print</Button>
		{/snippet}
	</ActionBar>

	{#if loading}
		<div class="loading">Loading family group sheet...</div>
	{:else if error}
		<ErrorState message={error} onRetry={() => loadGroupSheet($page.params.id ?? '')} />
	{:else if data}
		<FamilyGroupSheetComponent {data} />
	{/if}
</div>

<style>
	.group-sheet-page {
		max-width: 900px;
		margin: 0 auto;
		padding: 1.5rem;
	}

	.loading {
		text-align: center;
		padding: 3rem;
		color: var(--color-text-muted);
	}

	@media print {
		.group-sheet-page {
			padding: 0;
			max-width: none;
		}

	}
</style>
