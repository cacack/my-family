<script lang="ts">
	import GeographicMap from '$lib/components/GeographicMap.svelte';
</script>

<svelte:head>
	<title>Geographic Map | My Family</title>
</svelte:head>

<div class="map-page">
	<header class="page-header">
		<h1>Family Map</h1>
		<p class="description">Explore where your family members lived.</p>
	</header>

	<GeographicMap />
</div>

<style>
	.map-page {
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
