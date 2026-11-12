<script lang="ts">
	/**
	 * The one-off notice a navigation left in the page state, such as
	 * "Ada Lovelace was deleted." on the list a delete returns to (see
	 * docs/CONVENTIONS.md, "Confirmation and feedback").
	 */
	import { page } from '$app/state';
</script>

{#if page.state.notice}
	<p class="notice" role="status">{page.state.notice}</p>
{/if}

<style>
	.notice {
		margin: 0 0 1rem;
		padding: 0.75rem 1rem;
		background: #f0fdf4;
		border: 1px solid #bbf7d0;
		border-radius: 6px;
		color: #166534;
		font-size: 0.875rem;
	}
</style>
