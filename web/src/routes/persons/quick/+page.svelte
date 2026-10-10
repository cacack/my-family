<script lang="ts">
	import QuickCapture from '$lib/components/QuickCapture.svelte';
	import PageHeader from '$lib/components/PageHeader.svelte';
</script>

<svelte:head>
	<title>Quick Capture | My Family</title>
</svelte:head>

<div class="quick-capture-page">
	<PageHeader
		title="Quick Capture"
		description="Add people quickly with minimal info. Enrich details later."
		backHref="/persons"
		backLabel="People"
	/>
	<QuickCapture />
</div>

<style>
	.quick-capture-page {
		max-width: 800px;
		margin: 0 auto;
		padding: 1.5rem;
	}
</style>
