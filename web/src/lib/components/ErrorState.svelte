<script lang="ts">
	/**
	 * The page-level state for content that failed to load (#899): the reason
	 * and a Retry. Every list and detail page shows its load failure with this,
	 * so a failed load always offers a way back without a browser reload.
	 *
	 * It is for loads only. A failed save or delete must not replace the page:
	 * that is shown inline, beside the content and the user's input.
	 */
	import { Button } from '$lib/components/ui/button';

	interface Props {
		message: string;
		onRetry: () => void;
	}

	let { message, onRetry }: Props = $props();
</script>

<div class="error-state" role="alert">
	<p>{message}</p>
	<Button variant="outline" onclick={onRetry}>Retry</Button>
</div>

<style>
	.error-state {
		text-align: center;
		padding: 3rem 1rem;
		overflow-wrap: anywhere;
	}

	.error-state p {
		margin: 0 0 1rem;
		color: #dc2626;
	}
</style>
