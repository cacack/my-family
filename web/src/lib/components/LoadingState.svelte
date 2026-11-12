<script lang="ts">
	/**
	 * The page-level state for content that is still loading (#1115). Every
	 * route shows its load in progress with this, so the text, spacing and
	 * screen-reader announcement are the same everywhere.
	 *
	 * It is for a page's (or a page tab's) content. A busy button keeps its own
	 * label ("Saving...", "Load more").
	 */
	interface Props {
		/** What is loading, e.g. "Loading people...". */
		label?: string;
	}

	let { label = 'Loading...' }: Props = $props();
</script>

<div class="loading-state" role="status" aria-live="polite">{label}</div>

<style>
	.loading-state {
		text-align: center;
		padding: 3rem 1rem;
		font-size: 0.9375rem;
		color: var(--color-text-muted);
		overflow-wrap: anywhere;
	}
</style>
