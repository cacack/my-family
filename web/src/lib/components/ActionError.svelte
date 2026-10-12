<script lang="ts">
	import { Button } from '$lib/components/ui/button';

	/**
	 * A failed save, delete or other action, shown inline beside the content it
	 * concerns (#899). Unlike `ErrorState` it never replaces the page, so the
	 * content and any unsaved input stay where they were. An optional `action`
	 * offers a one-click way out, such as keeping an unreadable date as text.
	 */
	interface Props {
		message: string;
		action?: { label: string; onclick: () => void; disabled?: boolean };
	}

	let { message, action }: Props = $props();
</script>

<div class="action-error" role="alert">
	{message}
	{#if action}
		<div class="action-error-action">
			<Button variant="outline" size="sm" onclick={action.onclick} disabled={action.disabled}>{action.label}</Button>
		</div>
	{/if}
</div>

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

	.action-error-action {
		margin-top: 0.5rem;
	}
</style>
