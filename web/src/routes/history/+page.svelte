<script lang="ts">
	import ChangeHistory from '#lib/components/ChangeHistory.svelte';
	import PageHeader from '#lib/components/PageHeader.svelte';
	import MainlineNotice from '#lib/components/MainlineNotice.svelte';
	import SignInRequired from '#lib/components/auth/SignInRequired.svelte';
	import { Button } from '#lib/components/ui/button/index.js';
	import { isVisitor } from '#lib/stores/access.svelte.js';
</script>

<svelte:head>
	<title>Change History | My Family</title>
</svelte:head>

<div class="history-page">
	<MainlineNotice
		surface="Change History"
		detail="Changes made on a research branch do not appear here until the branch is merged."
	/>
	<PageHeader
		title="Change History"
		description="Complete audit trail of all changes to your family tree"
	>
		{#snippet actions()}
			<Button variant="outline" size="sm" href="/snapshots">Research snapshots</Button>
		{/snippet}
	</PageHeader>

	{#if isVisitor()}
		<SignInRequired subject="the change history" />
	{:else}
		<ChangeHistory />
	{/if}
</div>

<style>
	.history-page {
		max-width: var(--page-width-list);
		margin: 0 auto;
		padding: 1.5rem;
	}
</style>
