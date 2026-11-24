<script lang="ts">
	import UnmappedPlaces from '#lib/components/UnmappedPlaces.svelte';
	import PageHeader from '#lib/components/PageHeader.svelte';
</script>

<svelte:head>
	<title>Places Without Coordinates | My Family</title>
</svelte:head>

<div class="unmapped-page">
	<PageHeader
		title="Places Without Coordinates"
		description="Places with records that have no latitude and longitude, so the map leaves them out, with the coordinates other records at the same place already carry."
		back={{ href: '/map', label: 'Family Map' }}
	/>

	<UnmappedPlaces />
</div>

<style>
	.unmapped-page {
		max-width: var(--page-width-list);
		margin: 0 auto;
		padding: 1.5rem;
	}
</style>
