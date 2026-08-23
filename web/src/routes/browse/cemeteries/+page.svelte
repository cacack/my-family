<script lang="ts">
	import CemeteryBrowser from '$lib/components/CemeteryBrowser.svelte';
	import MainlineNotice from '$lib/components/MainlineNotice.svelte';
</script>

<svelte:head>
	<title>Browse by Cemetery | My Family</title>
</svelte:head>

<div class="browse-page">
	<MainlineNotice
		surface="The cemetery index"
		detail="It is built from life events, which carry no branch yet, while the person list for an individual cemetery does follow the branch. The two can disagree: a count here may include people your branch has deleted, so opening that cemetery can show fewer — or none."
	/>
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
