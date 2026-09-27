<script lang="ts">
	import CemeteryBrowser from '$lib/components/CemeteryBrowser.svelte';
</script>

<svelte:head>
	<title>Browse by Cemetery | My Family</title>
</svelte:head>

<div class="browse-page">
	<header class="page-header">
		<h1>Browse by Cemetery</h1>
		<p class="description">Explore burial and cremation places in your family tree.</p>
	</header>

	<CemeteryBrowser />
</div>

<style>
	.browse-page {
		max-width: 1200px;
		margin: 0 auto;
		padding: 1.5rem;
	}

	.page-header {
		margin-bottom: 2rem;
	}

	.page-header h1 {
		margin: 0 0 0.5rem;
		font-size: 1.5rem;
		color: #1e293b;
	}

	.description {
		margin: 0;
		color: #64748b;
		font-size: 0.9375rem;
	}
</style>
