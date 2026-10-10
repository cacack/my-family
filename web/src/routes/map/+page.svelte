<script lang="ts">
	import GeographicMap from '$lib/components/GeographicMap.svelte';
	import PageHeader from '$lib/components/PageHeader.svelte';
</script>

<svelte:head>
	<title>Family Map | My Family</title>
</svelte:head>

<div class="map-page">
	<PageHeader title="Family Map" description="Explore where your family members lived." />

	<GeographicMap />
</div>

<style>
	.map-page {
		max-width: 1200px;
		margin: 0 auto;
		padding: 1.5rem;
	}
</style>
