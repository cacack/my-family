<script lang="ts">
	import QuickCapture from '$lib/components/QuickCapture.svelte';
</script>

<svelte:head>
	<title>Quick Capture | My Family</title>
</svelte:head>

<div class="quick-capture-page">
	<header class="page-header">
		<a href="/persons" class="back-link">&larr; People</a>
		<h1>Quick Capture</h1>
	</header>
	<p class="subtitle">Add people quickly with minimal info. Enrich details later.</p>
	<QuickCapture />
</div>

<style>
	.quick-capture-page {
		max-width: 800px;
		margin: 0 auto;
		padding: 1.5rem;
	}

	.page-header {
		display: flex;
		align-items: center;
		gap: 1rem;
		margin-bottom: 0.5rem;
	}

	.page-header h1 {
		margin: 0;
		font-size: 1.5rem;
		color: #1e293b;
	}

	.back-link {
		color: #64748b;
		text-decoration: none;
		font-size: 0.875rem;
	}

	.back-link:hover {
		color: #3b82f6;
	}

	.subtitle {
		color: #64748b;
		font-size: 0.875rem;
		margin: 0 0 1.5rem;
	}
</style>
