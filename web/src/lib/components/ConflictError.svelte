<script lang="ts">
	interface Props {
		message?: string;
		onRetry: () => void;
		retrying?: boolean;
	}

	let { message, onRetry, retrying = false }: Props = $props();
</script>

<div class="conflict-error" role="alert">
	<div class="conflict-icon" aria-hidden="true">
		<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" width="20" height="20">
			<path d="M10.29 3.86L1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z" />
			<line x1="12" y1="9" x2="12" y2="13" />
			<line x1="12" y1="17" x2="12.01" y2="17" />
		</svg>
	</div>
	<div class="conflict-content">
		<p class="conflict-message">
			{message || 'This record was updated by another operation. Your changes were not lost.'}
		</p>
		<button class="btn-retry" onclick={onRetry} disabled={retrying}>
			{retrying ? 'Retrying...' : 'Try Again'}
		</button>
	</div>
</div>

<style>
	.conflict-error {
		display: flex;
		gap: 0.75rem;
		padding: 0.75rem;
		background: #fffbeb;
		border: 1px solid #fde68a;
		border-radius: 6px;
		margin-bottom: 1rem;
	}

	.conflict-icon {
		flex-shrink: 0;
		color: #d97706;
	}

	.conflict-content {
		flex: 1;
	}

	.conflict-message {
		margin: 0 0 0.5rem;
		font-size: 0.875rem;
		color: #92400e;
	}

	.btn-retry {
		padding: 0.375rem 0.75rem;
		font-size: 0.8125rem;
		border: 1px solid #fbbf24;
		border-radius: 6px;
		background: white;
		color: #92400e;
		cursor: pointer;
	}

	.btn-retry:hover {
		background: #fffbeb;
	}

	.btn-retry:disabled {
		opacity: 0.6;
		cursor: not-allowed;
	}

	:global(body.high-contrast) .conflict-error {
		border-color: #d97706;
		background: #fef3c7;
	}
</style>
