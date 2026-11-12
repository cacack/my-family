<script lang="ts">
	/**
	 * The page-level state for content that failed to load (#899, #1096). Every
	 * list and detail page shows its load failure with this, in one of two forms
	 * (see docs/CONVENTIONS.md):
	 *
	 * - a load error (a 5xx, a network failure) offers Retry, so a failed load
	 *   always has a way back without a browser reload;
	 * - a not-found state (`notFound`) offers none, since no retry can make a
	 *   missing record exist.
	 *
	 * A page that renders no heading of its own in this state passes `title`,
	 * shown as its h1; `recordLoadError` returns all three as a `LoadFailure`.
	 *
	 * It is for loads only. A failed save or delete must not replace the page:
	 * that is shown inline, beside the content and the user's input.
	 */
	import { Button } from '#lib/components/ui/button/index.js';

	interface Props {
		title?: string;
		message: string;
		notFound?: boolean;
		onRetry: () => void;
	}

	let { title, message, notFound = false, onRetry }: Props = $props();
</script>

<div class="error-state" class:not-found={notFound} role="alert">
	{#if title}
		<h1>{title}</h1>
	{/if}
	<p>{message}</p>
	{#if !notFound}
		<Button variant="outline" onclick={onRetry}>Retry</Button>
	{/if}
</div>

<style>
	.error-state {
		text-align: center;
		padding: 3rem 1rem;
		overflow-wrap: anywhere;
	}

	.error-state h1 {
		margin: 0 0 0.5rem;
		font-size: 1.25rem;
		color: var(--color-text);
	}

	.error-state p {
		margin: 0 0 1rem;
		color: #dc2626;
	}

	.error-state.not-found p {
		color: var(--color-text-muted);
	}
</style>
