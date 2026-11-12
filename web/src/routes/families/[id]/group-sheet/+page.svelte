<script lang="ts">
	import { page } from '$app/state';
	import {
		api,
		formatPersonName,
		recordLoadError,
		type FamilyGroupSheet,
		type LoadFailure
	} from '#lib/api/client.js';
	import { Button } from '#lib/components/ui/button/index.js';
	import ActionBar from '#lib/components/ActionBar.svelte';
	import FamilyGroupSheetComponent from '#lib/components/FamilyGroupSheet.svelte';
	import ErrorState from '#lib/components/ErrorState.svelte';

	let data: FamilyGroupSheet | null = $state(null);
	let loading = $state(true);
	let error: LoadFailure | null = $state(null);

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

	// The family page's title (its h1), which the back link names.
	function familyName(): string {
		if (!data) return 'Family';
		const p1 = data.husband ? formatPersonName(data.husband) : 'Unknown';
		const p2 = data.wife ? formatPersonName(data.wife) : undefined;
		return p2 ? `${p1} & ${p2}` : p1;
	}

	$effect(() => {
		const id = page.params.id;
		if (id) {
			loadGroupSheet(id);
		}
	});
</script>

<svelte:head>
	<title>{getTitle()} | My Family</title>
</svelte:head>

<div class="group-sheet-page">
	<ActionBar back={{ href: `/families/${page.params.id}`, label: familyName() }}>
		{#snippet actions()}
			<!-- Nothing to print until the sheet has loaded (#1096). -->
			<Button onclick={handlePrint} disabled={!data}>Print</Button>
		{/snippet}
	</ActionBar>

	{#if loading}
		<div class="loading">Loading family group sheet...</div>
	{:else if error}
		<ErrorState {...error} onRetry={() => loadGroupSheet(page.params.id ?? '')} />
	{:else if data}
		<FamilyGroupSheetComponent {data} />
	{/if}
</div>

<style>
	.group-sheet-page {
		max-width: var(--page-width-detail);
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
