<script lang="ts">
	interface Props {
		message: string;
		changes?: Record<string, unknown>;
		onDismiss: () => void;
	}

	let { message, changes, onDismiss }: Props = $props();

	$effect(() => {
		const timer = setTimeout(onDismiss, 5000);
		return () => clearTimeout(timer);
	});

	function formatFieldName(field: string): string {
		return field
			.split('_')
			.map((word: string) => word.charAt(0).toUpperCase() + word.slice(1))
			.join(' ');
	}
</script>

<div class="success-banner" role="status" aria-live="polite">
	<div class="banner-content">
		<div class="banner-icon">
			<svg viewBox="0 0 24 24" fill="currentColor" width="20" height="20">
				<path d="M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm-2 15l-5-5 1.41-1.41L10 14.17l7.59-7.59L19 8l-9 9z" />
			</svg>
		</div>
		<div class="banner-text">
			<p class="banner-message">{message}</p>
			{#if changes && Object.keys(changes).length > 0}
				<div class="changes-list">
					<span class="changes-label">Restored fields:</span>
					{#each Object.keys(changes) as field}
						<span class="change-tag">{formatFieldName(field)}</span>
					{/each}
				</div>
			{/if}
		</div>
		<button class="dismiss-btn" onclick={onDismiss} aria-label="Dismiss">
			<svg viewBox="0 0 24 24" fill="currentColor" width="16" height="16">
				<path d="M19 6.41L17.59 5 12 10.59 6.41 5 5 6.41 10.59 12 5 17.59 6.41 19 12 13.41 17.59 19 19 17.59 13.41 12z" />
			</svg>
		</button>
	</div>
</div>

<style>
	.success-banner {
		background: #f0fdf4;
		border: 1px solid #86efac;
		border-radius: 8px;
		padding: 0.75rem 1rem;
		margin-bottom: 1rem;
		animation: slideDown 0.3s ease-out;
	}

	@keyframes slideDown {
		from {
			opacity: 0;
			transform: translateY(-0.5rem);
		}
		to {
			opacity: 1;
			transform: translateY(0);
		}
	}

	.banner-content {
		display: flex;
		align-items: flex-start;
		gap: 0.75rem;
	}

	.banner-icon {
		color: #22c55e;
		flex-shrink: 0;
		margin-top: 0.125rem;
	}

	.banner-text {
		flex: 1;
		min-width: 0;
	}

	.banner-message {
		margin: 0;
		font-size: 0.875rem;
		font-weight: 500;
		color: #166534;
	}

	.changes-list {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 0.375rem;
		margin-top: 0.5rem;
	}

	.changes-label {
		font-size: 0.75rem;
		color: #4ade80;
	}

	.change-tag {
		display: inline-block;
		padding: 0.0625rem 0.375rem;
		background: #dcfce7;
		border-radius: 4px;
		font-size: 0.6875rem;
		color: #166534;
	}

	.dismiss-btn {
		flex-shrink: 0;
		padding: 0.25rem;
		border: none;
		background: none;
		color: #86efac;
		cursor: pointer;
		border-radius: 4px;
	}

	.dismiss-btn:hover {
		background: #dcfce7;
		color: #22c55e;
	}
</style>
