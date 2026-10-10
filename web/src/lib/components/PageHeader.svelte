<script lang="ts">
	/**
	 * The heading block at the top of a page: an optional back link, the h1, a
	 * one-line description, any extra notes (`children`), and the page's
	 * actions. The actions wrap below the title when the row runs out of room,
	 * so no page needs its own breakpoint for them.
	 *
	 * Rich description content (links, several paragraphs, notes) goes in
	 * `children`, styled by the page. See docs/CONVENTIONS.md#ui-building-blocks.
	 */
	import type { Snippet } from 'svelte';

	let {
		title,
		description,
		backHref,
		backLabel,
		headingEl = $bindable(null),
		actions,
		children
	}: {
		title: string;
		description?: string;
		/** Where the back link goes; the link renders only with `backLabel`. */
		backHref?: string;
		backLabel?: string;
		/** The h1, for pages that move focus to it. */
		headingEl?: HTMLHeadingElement | null;
		actions?: Snippet;
		children?: Snippet;
	} = $props();
</script>

<header class="page-header">
	<div class="page-header-text">
		{#if backHref && backLabel}
			<a href={backHref} class="back-link">&larr; {backLabel}</a>
		{/if}
		<h1 bind:this={headingEl} tabindex="-1">{title}</h1>
		{#if description}
			<p class="description">{description}</p>
		{/if}
		{@render children?.()}
	</div>
	{#if actions}
		<div class="page-header-actions">
			{@render actions()}
		</div>
	{/if}
</header>

<style>
	.page-header {
		display: flex;
		flex-wrap: wrap;
		align-items: flex-start;
		justify-content: space-between;
		gap: 0.75rem 1rem;
		margin-bottom: 1.5rem;
	}

	.page-header-text {
		flex: 1 1 20rem;
		min-width: 0;
	}

	.back-link {
		display: inline-block;
		margin-bottom: 0.5rem;
		color: var(--color-text-muted);
		font-size: 0.875rem;
		text-decoration: none;
	}

	.back-link:hover {
		color: var(--color-link);
	}

	h1 {
		margin: 0;
		font-size: 1.5rem;
		color: var(--color-text);
		overflow-wrap: anywhere;
	}

	h1:focus {
		outline: none;
	}

	.description {
		margin: 0.25rem 0 0;
		max-width: 46rem;
		color: var(--color-text-muted);
		font-size: 0.875rem;
	}

	.page-header-actions {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 0.5rem;
	}
</style>
