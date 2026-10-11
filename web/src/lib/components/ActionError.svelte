<script lang="ts">
	/**
	 * A failed save, delete or other action, shown inline beside the content it
	 * concerns (#899). Unlike `ErrorState` it never replaces the page, so the
	 * content and any unsaved input stay where they were.
	 */
	interface Props {
		message: string;
	}

	let { message }: Props = $props();
</script>

<div class="action-error" role="alert">{message}</div>

<style>
	.action-error {
		padding: 0.75rem 1rem;
		margin-bottom: 1rem;
		background: #fef2f2;
		border: 1px solid #fecaca;
		border-radius: 8px;
		color: #dc2626;
		font-size: 0.875rem;
		overflow-wrap: anywhere;
	}
</style>
