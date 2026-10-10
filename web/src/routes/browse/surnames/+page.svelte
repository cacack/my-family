<script lang="ts">
	import SurnameBrowser from '$lib/components/SurnameBrowser.svelte';
	import PageHeader from '$lib/components/PageHeader.svelte';
</script>

<svelte:head>
	<title>Browse by Surname | My Family</title>
</svelte:head>

<div class="browse-page">
	<PageHeader
		title="Browse by Surname"
		description="Explore your family tree by surname. Click a letter to filter, then click a surname to see all people with that name."
	/>

	<SurnameBrowser />
</div>

<style>
	.browse-page {
		max-width: 1200px;
		margin: 0 auto;
		padding: 1.5rem;
	}
</style>
