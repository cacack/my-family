<script lang="ts">
	import CemeteryBrowser from '$lib/components/CemeteryBrowser.svelte';
	import PageHeader from '$lib/components/PageHeader.svelte';
</script>

<svelte:head>
	<title>Browse by Cemetery | My Family</title>
</svelte:head>

<div class="browse-page">
	<PageHeader title="Browse by Cemetery" description="Explore burial and cremation places in your family tree." />

	<CemeteryBrowser />
</div>

<style>
	.browse-page {
		max-width: 1200px;
		margin: 0 auto;
		padding: 1.5rem;
	}
</style>
