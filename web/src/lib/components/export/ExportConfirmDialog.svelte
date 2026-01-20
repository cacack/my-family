<script lang="ts">
	import { onMount } from 'svelte';
	import type { ExportEstimate } from '$lib/api/client';

	interface Props {
		estimate: ExportEstimate;
		onConfirm: () => void;
		onCancel: () => void;
	}

	let { estimate, onConfirm, onCancel }: Props = $props();
	let dialogRef: HTMLDivElement | undefined = $state();
	let confirmButtonRef: HTMLButtonElement | undefined = $state();

	function formatBytes(bytes: number): string {
		if (bytes < 1024) return `${bytes} B`;
		if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
		return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
	}

	function handleKeydown(e: KeyboardEvent) {
		if (e.key === 'Escape') {
			e.preventDefault();
			onCancel();
		}
	}

	function handleBackdropClick(e: MouseEvent) {
		if (e.target === e.currentTarget) {
			onCancel();
		}
	}

	onMount(() => {
		// Focus the confirm button when dialog opens
		confirmButtonRef?.focus();

		// Trap focus within dialog
		const focusableElements = dialogRef?.querySelectorAll<HTMLElement>(
			'button, [href], input, select, textarea, [tabindex]:not([tabindex="-1"])'
		);
		const firstElement = focusableElements?.[0];
		const lastElement = focusableElements?.[focusableElements.length - 1];

		function handleTabTrap(e: KeyboardEvent) {
			if (e.key !== 'Tab') return;

			if (e.shiftKey) {
				if (document.activeElement === firstElement) {
					e.preventDefault();
					lastElement?.focus();
				}
			} else {
				if (document.activeElement === lastElement) {
					e.preventDefault();
					firstElement?.focus();
				}
			}
		}

		document.addEventListener('keydown', handleTabTrap);
		return () => document.removeEventListener('keydown', handleTabTrap);
	});
</script>

<svelte:window onkeydown={handleKeydown} />

<!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
<div
	class="dialog-backdrop"
	role="dialog"
	aria-modal="true"
	aria-labelledby="dialog-title"
	aria-describedby="dialog-description"
	tabindex="-1"
	onclick={handleBackdropClick}
	onkeydown={handleKeydown}
>
	<div class="dialog-content" bind:this={dialogRef}>
		<div class="dialog-header">
			<svg
				class="warning-icon"
				viewBox="0 0 24 24"
				fill="none"
				stroke="currentColor"
				stroke-width="2"
				aria-hidden="true"
			>
				<path
					d="M10.29 3.86L1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z"
				/>
				<line x1="12" y1="9" x2="12" y2="13" />
				<line x1="12" y1="17" x2="12.01" y2="17" />
			</svg>
			<h3 id="dialog-title" class="dialog-title">Large Export Warning</h3>
		</div>

		<div id="dialog-description" class="dialog-body">
			<p class="dialog-message">
				This export contains <strong>{estimate.total_records.toLocaleString()}</strong> records
				(approximately <strong>{formatBytes(estimate.estimated_bytes)}</strong>). The export may
				take some time to complete.
			</p>

			<div class="record-summary">
				<div class="record-row">
					<span class="record-label">People</span>
					<span class="record-value">{estimate.person_count.toLocaleString()}</span>
				</div>
				<div class="record-row">
					<span class="record-label">Families</span>
					<span class="record-value">{estimate.family_count.toLocaleString()}</span>
				</div>
				{#if estimate.source_count > 0}
					<div class="record-row">
						<span class="record-label">Sources</span>
						<span class="record-value">{estimate.source_count.toLocaleString()}</span>
					</div>
				{/if}
				{#if estimate.event_count > 0}
					<div class="record-row">
						<span class="record-label">Events</span>
						<span class="record-value">{estimate.event_count.toLocaleString()}</span>
					</div>
				{/if}
			</div>
		</div>

		<div class="dialog-actions">
			<button type="button" class="btn btn-secondary" onclick={onCancel}> Cancel </button>
			<button type="button" class="btn btn-primary" bind:this={confirmButtonRef} onclick={onConfirm}>
				Proceed with Export
			</button>
		</div>
	</div>
</div>

<style>
	.dialog-backdrop {
		position: fixed;
		inset: 0;
		background: rgba(0, 0, 0, 0.5);
		display: flex;
		align-items: center;
		justify-content: center;
		z-index: 1000;
		padding: 1rem;
	}

	.dialog-content {
		background: white;
		border-radius: 12px;
		box-shadow:
			0 20px 25px -5px rgba(0, 0, 0, 0.1),
			0 10px 10px -5px rgba(0, 0, 0, 0.04);
		max-width: 28rem;
		width: 100%;
		max-height: 90vh;
		overflow-y: auto;
	}

	.dialog-header {
		display: flex;
		align-items: center;
		gap: 0.75rem;
		padding: 1.25rem 1.5rem 0;
	}

	.warning-icon {
		width: 1.5rem;
		height: 1.5rem;
		color: #f59e0b;
		flex-shrink: 0;
	}

	.dialog-title {
		margin: 0;
		font-size: 1.125rem;
		font-weight: 600;
		color: #1e293b;
	}

	.dialog-body {
		padding: 1rem 1.5rem;
	}

	.dialog-message {
		margin: 0 0 1rem;
		color: #475569;
		font-size: 0.9375rem;
		line-height: 1.5;
	}

	.dialog-message strong {
		color: #1e293b;
		font-weight: 600;
	}

	.record-summary {
		background: #f8fafc;
		border: 1px solid #e2e8f0;
		border-radius: 8px;
		padding: 0.75rem 1rem;
	}

	.record-row {
		display: flex;
		justify-content: space-between;
		padding: 0.25rem 0;
		font-size: 0.875rem;
	}

	.record-row:not(:last-child) {
		border-bottom: 1px solid #e2e8f0;
		padding-bottom: 0.375rem;
		margin-bottom: 0.375rem;
	}

	.record-label {
		color: #64748b;
	}

	.record-value {
		font-weight: 500;
		color: #1e293b;
		font-variant-numeric: tabular-nums;
	}

	.dialog-actions {
		display: flex;
		justify-content: flex-end;
		gap: 0.75rem;
		padding: 1rem 1.5rem 1.25rem;
		border-top: 1px solid #e2e8f0;
	}

	.btn {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		padding: 0.625rem 1.25rem;
		border-radius: 6px;
		font-size: 0.875rem;
		font-weight: 500;
		cursor: pointer;
		transition: all 0.15s;
	}

	.btn-secondary {
		background: white;
		border: 1px solid #cbd5e1;
		color: #475569;
	}

	.btn-secondary:hover {
		background: #f1f5f9;
		border-color: #94a3b8;
	}

	.btn-secondary:focus {
		outline: none;
		box-shadow: 0 0 0 3px rgba(100, 116, 139, 0.2);
	}

	.btn-primary {
		background: #3b82f6;
		border: 1px solid #3b82f6;
		color: white;
	}

	.btn-primary:hover {
		background: #2563eb;
		border-color: #2563eb;
	}

	.btn-primary:focus {
		outline: none;
		box-shadow: 0 0 0 3px rgba(59, 130, 246, 0.3);
	}

	/* Responsive adjustments */
	@media (max-width: 480px) {
		.dialog-backdrop {
			padding: 0.5rem;
		}

		.dialog-actions {
			flex-direction: column-reverse;
		}

		.btn {
			width: 100%;
		}
	}
</style>
