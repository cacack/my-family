<script lang="ts">
	import PlaceJoin from '#lib/components/PlaceJoin.svelte';
	import PageHeader from '#lib/components/PageHeader.svelte';
</script>

<svelte:head>
	<title>Join Places | My Family</title>
</svelte:head>

<div class="join-page">
	<PageHeader
		title="Join Places"
		description="Say that two spellings name one place, such as Köln and Cologne. Every record keeps the spelling it was given; both are read as one place."
	/>

	<PlaceJoin />
</div>

<style>
	.join-page {
		max-width: var(--page-width-list);
		margin: 0 auto;
		padding: 1.5rem;
	}
</style>
