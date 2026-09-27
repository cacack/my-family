<script lang="ts">
	import ChangeHistory from '$lib/components/ChangeHistory.svelte';
	import MainlineNotice from '$lib/components/MainlineNotice.svelte';
	import { Button } from '$lib/components/ui/button';
</script>

<svelte:head>
	<title>Change History | My Family</title>
</svelte:head>

<div class="history-page">
	<MainlineNotice
		surface="Change History"
		detail="Changes made on a research branch do not appear here until the branch is merged."
	/>
	<header class="page-header">
		<div>
			<h1>Change History</h1>
			<p class="subtitle">Complete audit trail of all changes to your family tree</p>
		</div>
		<Button variant="outline" size="sm" href="/snapshots">Research snapshots</Button>
	</header>

	<ChangeHistory />
</div>

<style>
	.history-page {
		max-width: 900px;
		margin: 0 auto;
		padding: 1.5rem;
	}

	.page-header {
		display: flex;
		align-items: flex-start;
		justify-content: space-between;
		gap: 1rem;
		flex-wrap: wrap;
		margin-bottom: 1.5rem;
	}

	.page-header h1 {
		margin: 0 0 0.5rem;
		font-size: 1.5rem;
		color: #1e293b;
	}

	.subtitle {
		margin: 0;
		font-size: 0.875rem;
		color: #64748b;
	}
</style>
