<script lang="ts">
	import { page } from '$app/stores';
	import { Button } from '$lib/components/ui/button';

	const notFound = $derived($page.status === 404);
</script>

<svelte:head>
	<title>{notFound ? 'Page not found' : 'Error'} | My Family</title>
</svelte:head>

<div class="error-page">
	<p class="status">{$page.status}</p>
	<h1>{notFound ? 'Page not found' : 'Something went wrong'}</h1>
	<p class="message">
		{#if notFound}
			There is no page at this address. It may have been moved, or the link may be wrong.
		{:else}
			{$page.error?.message || 'An unexpected error occurred.'}
		{/if}
	</p>
	<Button href="/">Go to the dashboard</Button>
</div>

<style>
	.error-page {
		max-width: 640px;
		margin: 0 auto;
		padding: 4rem 1.5rem;
		text-align: center;
	}

	.status {
		margin: 0;
		font-size: 3rem;
		font-weight: 700;
		color: #94a3b8;
	}

	h1 {
		margin: 0.5rem 0;
		font-size: 1.5rem;
		color: #1e293b;
	}

	.message {
		margin: 0 0 1.5rem;
		color: #64748b;
	}
</style>
