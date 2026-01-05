<script lang="ts">
	import SurnameBrowser from '$lib/components/SurnameBrowser.svelte';
</script>

<svelte:head>
	<title>Browse by Surname | My Family</title>
</svelte:head>

<div class="browse-page">
	<header class="page-header">
		<h1>Browse by Surname</h1>
		<p class="description">Explore your family tree by surname. Click a letter to filter, then click a surname to see all people with that name.</p>
	</header>

	<SurnameBrowser />
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
