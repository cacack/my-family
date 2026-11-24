<script lang="ts">
	import GeographicMap from '#lib/components/GeographicMap.svelte';
	import PageHeader from '#lib/components/PageHeader.svelte';
	import { Button } from '#lib/components/ui/button/index.js';
</script>

<svelte:head>
	<title>Family Map | My Family</title>
</svelte:head>

<div class="map-page">
	<PageHeader title="Family Map" description="Explore where your family members lived.">
		{#snippet actions()}
			<Button variant="outline" href="/places/unmapped">Places without coordinates</Button>
		{/snippet}
	</PageHeader>

	<GeographicMap />
</div>

<style>
	.map-page {
		max-width: var(--page-width-list);
		margin: 0 auto;
		padding: 1.5rem;
	}
</style>
