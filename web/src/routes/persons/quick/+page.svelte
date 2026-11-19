<script lang="ts">
	import { can } from '#lib/stores/access.svelte.js';
	import RoleNotice from '#lib/components/auth/RoleNotice.svelte';
	import QuickCapture from '#lib/components/QuickCapture.svelte';
	import PageHeader from '#lib/components/PageHeader.svelte';
</script>

<svelte:head>
	<title>Quick Capture | My Family</title>
</svelte:head>

<div class="quick-capture-page">
	<PageHeader
		title="Quick Capture"
		description="Add people quickly with minimal info. Enrich details later."
		back={{ href: '/persons', label: 'People' }}
	/>

	{#if !can('editor')}
		<RoleNotice action="Adding people" required="editor" />
	{:else}
	<QuickCapture />
	{/if}
</div>

<style>
	.quick-capture-page {
		max-width: var(--page-width-detail);
		margin: 0 auto;
		padding: 1.5rem;
	}
</style>
