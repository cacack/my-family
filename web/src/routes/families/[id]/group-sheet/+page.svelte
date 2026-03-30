<script lang="ts">
	import { page } from '$app/stores';
	import { api, type FamilyGroupSheet } from '$lib/api/client';
	import { Button } from '$lib/components/ui/button';
	import FamilyGroupSheetComponent from '$lib/components/FamilyGroupSheet.svelte';

	let data: FamilyGroupSheet | null = $state(null);
	let loading = $state(true);
	let error: string | null = $state(null);

	async function loadGroupSheet(id: string) {
		loading = true;
		error = null;
		try {
			data = await api.getFamilyGroupSheet(id);
		} catch (e) {
			error = (e as { message?: string }).message || 'Failed to load family group sheet';
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
	<header class="page-header no-print">
		<a href="/families/{$page.params.id}" class="back-link">&larr; Back to Family</a>
		<div class="actions">
			<Button onclick={handlePrint}>Print</Button>
		</div>
	</header>

	{#if loading}
		<div class="loading">Loading family group sheet...</div>
	{:else if error}
		<div class="error">{error}</div>
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

	.page-header {
		display: flex;
		justify-content: space-between;
		align-items: center;
		margin-bottom: 1.5rem;
	}

	.back-link {
		color: #64748b;
		text-decoration: none;
		font-size: 0.875rem;
	}

	.back-link:hover {
		color: #3b82f6;
	}

	.actions {
		display: flex;
		gap: 0.5rem;
	}

	.loading,
	.error {
		text-align: center;
		padding: 3rem;
		color: #64748b;
	}

	.error {
		color: #dc2626;
	}

	@media print {
		.group-sheet-page {
			padding: 0;
			max-width: none;
		}

		.no-print {
			display: none !important;
		}
	}
</style>
