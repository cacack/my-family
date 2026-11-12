<script lang="ts">
	/**
	 * The page-level state for a list or view with nothing to show (#1115): a
	 * message, an optional description and optional actions, in one look. Every
	 * list page shows its empty state with this.
	 */
	import type { Snippet } from 'svelte';

	interface Props {
		message: string;
		description?: string;
		/** Buttons or links that get the user started (Add, Import). */
		actions?: Snippet;
	}

	let { message, description, actions }: Props = $props();
</script>

<div class="empty-state" data-testid="empty-state">
	<p class="message">{message}</p>
	{#if description}
		<p class="description">{description}</p>
	{/if}
	{#if actions}
		<div class="actions">{@render actions()}</div>
	{/if}
</div>

<style>
	.empty-state {
		text-align: center;
		padding: 3rem 1rem;
		font-size: 0.9375rem;
		color: var(--color-text-muted);
		overflow-wrap: anywhere;
	}

	.message {
		margin: 0;
	}

	.description {
		margin: 0.5rem 0 0;
		font-size: 0.8125rem;
	}

	.actions {
		display: flex;
		flex-wrap: wrap;
		justify-content: center;
		gap: 0.5rem;
		margin-top: 1rem;
	}
</style>
