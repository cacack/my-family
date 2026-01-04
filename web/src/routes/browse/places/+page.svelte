<script lang="ts">
	import PlaceBrowser from '$lib/components/PlaceBrowser.svelte';
</script>

<svelte:head>
	<title>Browse by Place | My Family</title>
</svelte:head>

<div class="browse-page">
	<header class="page-header">
		<h1>Browse by Place</h1>
		<p class="description">Explore your family tree by location. Navigate through the hierarchy from countries down to cities, or click to view all people from a location.</p>
	</header>

	<PlaceBrowser />
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
