<script lang="ts">
	import PlaceBrowser from '$lib/components/PlaceBrowser.svelte';
	import PageHeader from '$lib/components/PageHeader.svelte';
</script>

<svelte:head>
	<title>Browse by Place | My Family</title>
</svelte:head>

<div class="browse-page">
	<PageHeader
		title="Browse by Place"
		description="Explore your family tree by location. Navigate through the hierarchy from countries down to cities, or click to view all people from a location."
	/>

	<PlaceBrowser />
</div>

<style>
	.browse-page {
		max-width: 1200px;
		margin: 0 auto;
		padding: 1.5rem;
	}
</style>
