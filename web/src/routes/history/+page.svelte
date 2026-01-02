<script lang="ts">
	import ChangeHistory from '$lib/components/ChangeHistory.svelte';
</script>

<svelte:head>
	<title>Change History | My Family</title>
</svelte:head>

<div class="history-page">
	<header class="page-header">
		<h1>Change History</h1>
		<p class="subtitle">Complete audit trail of all changes to your family tree</p>
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
