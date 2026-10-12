<script lang="ts">
	/**
	 * The bar at the top of a page whose heading lives in its body (an entity
	 * detail page, a printable report): a back link on the left and the page's
	 * actions on the right, wrapping below it when the row runs out of room.
	 * It never prints. Pages with their own heading use PageHeader instead.
	 * See docs/CONVENTIONS.md#ui-building-blocks.
	 */
	import type { Snippet } from 'svelte';

	let {
		back,
		actions
	}: {
		/** The back link, e.g. `{ href: '/persons', label: 'People' }`. */
		back: { href: string; label: string };
		actions?: Snippet;
	} = $props();
</script>

<div class="action-bar">
	<a href={back.href} class="back-link">&larr; {back.label}</a>
	{#if actions}
		<div class="action-bar-actions">
			{@render actions()}
		</div>
	{/if}
</div>

<style>
	.action-bar {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		justify-content: space-between;
		gap: 0.75rem 1rem;
		margin-bottom: 1.5rem;
	}

	/* 24px tall, the minimum touch target (WCAG 2.5.8). */
	.back-link {
		display: inline-flex;
		align-items: center;
		min-height: 24px;
		color: var(--color-text-muted);
		font-size: 0.875rem;
		text-decoration: none;
	}

	.back-link:hover {
		color: var(--color-link);
	}

	.action-bar-actions {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 0.5rem;
	}

	@media print {
		.action-bar {
			display: none;
		}
	}
</style>
