<script lang="ts">
	import { api, type RollbackResponse } from '$lib/api/client';

	interface Props {
		open: boolean;
		entityType: 'person' | 'family' | 'source' | 'citation';
		entityId: string;
		entityName: string;
		currentVersion: number;
		targetVersion: number;
		targetSummary: string;
		onConfirm: (response: RollbackResponse) => void;
		onCancel: () => void;
	}

	let {
		open,
		entityType,
		entityId,
		entityName,
		currentVersion,
		targetVersion,
		targetSummary,
		onConfirm,
		onCancel
	}: Props = $props();

	let rolling = $state(false);
	let error: string | null = $state(null);

	$effect(() => {
		if (open) {
			error = null;
		}
	});

	async function performRollback() {
		rolling = true;
		error = null;
		try {
			let response: RollbackResponse;
			switch (entityType) {
				case 'person':
					response = await api.rollbackPerson(entityId, targetVersion);
					break;
				case 'family':
					response = await api.rollbackFamily(entityId, targetVersion);
					break;
				case 'source':
					response = await api.rollbackSource(entityId, targetVersion);
					break;
				case 'citation':
					response = await api.rollbackCitation(entityId, targetVersion);
					break;
				default:
					throw new Error('Unknown entity type');
			}
			onConfirm(response);
		} catch (e) {
			error = (e as { message?: string }).message || 'Rollback failed';
		} finally {
			rolling = false;
		}
	}

	function handleBackdropClick(event: MouseEvent) {
		if (event.target === event.currentTarget && !rolling) {
			onCancel();
		}
	}

	function handleKeydown(event: KeyboardEvent) {
		if (event.key === 'Escape' && !rolling) {
			onCancel();
		}
	}
</script>

{#if open}
	<div class="backdrop" onclick={handleBackdropClick} onkeydown={handleKeydown} role="dialog" aria-modal="true" aria-label="Confirm rollback" tabindex="-1">
		<div class="dialog">
			<div class="dialog-header">
				<h3>Confirm Rollback</h3>
			</div>

			<div class="dialog-body">
				<div class="warning-icon">
					<svg viewBox="0 0 24 24" fill="currentColor" width="24" height="24">
						<path d="M1 21h22L12 2 1 21zm12-3h-2v-2h2v2zm0-4h-2v-4h2v4z" />
					</svg>
				</div>

				<p class="warning-text">
					You are about to rollback <strong>{entityName}</strong> from
					version {currentVersion} to version {targetVersion}.
				</p>

				<div class="detail-card">
					<div class="detail-row">
						<span class="detail-label">Entity</span>
						<span class="detail-value">{entityName}</span>
					</div>
					<div class="detail-row">
						<span class="detail-label">Type</span>
						<span class="detail-value capitalize">{entityType}</span>
					</div>
					<div class="detail-row">
						<span class="detail-label">Current Version</span>
						<span class="detail-value">v{currentVersion}</span>
					</div>
					<div class="detail-row">
						<span class="detail-label">Restore To</span>
						<span class="detail-value">v{targetVersion}</span>
					</div>
					<div class="detail-row">
						<span class="detail-label">Target State</span>
						<span class="detail-value">{targetSummary}</span>
					</div>
				</div>

				<p class="info-text">
					This will create a new version with the data from version {targetVersion}.
					The current data will remain in the history and can be restored later.
				</p>

				{#if error}
					<div class="error" role="alert">{error}</div>
				{/if}
			</div>

			<div class="dialog-footer">
				<button class="btn" onclick={onCancel} disabled={rolling}>
					Cancel
				</button>
				<button class="btn btn-warning" onclick={performRollback} disabled={rolling}>
					{rolling ? 'Rolling back...' : 'Confirm Rollback'}
				</button>
			</div>
		</div>
	</div>
{/if}

<style>
	.backdrop {
		position: fixed;
		inset: 0;
		background: rgba(0, 0, 0, 0.5);
		display: flex;
		align-items: center;
		justify-content: center;
		z-index: 1000;
		padding: 1rem;
	}

	.dialog {
		background: white;
		border-radius: 12px;
		box-shadow: 0 20px 25px -5px rgba(0, 0, 0, 0.1), 0 8px 10px -6px rgba(0, 0, 0, 0.1);
		width: 100%;
		max-width: 480px;
		overflow: hidden;
	}

	.dialog-header {
		padding: 1.25rem 1.5rem;
		border-bottom: 1px solid #e2e8f0;
	}

	.dialog-header h3 {
		margin: 0;
		font-size: 1.125rem;
		color: #1e293b;
	}

	.dialog-body {
		padding: 1.5rem;
	}

	.warning-icon {
		display: flex;
		align-items: center;
		justify-content: center;
		width: 3rem;
		height: 3rem;
		border-radius: 50%;
		background: #fef3c7;
		color: #f59e0b;
		margin: 0 auto 1rem;
	}

	.warning-icon svg {
		width: 1.5rem;
		height: 1.5rem;
	}

	.warning-text {
		text-align: center;
		margin: 0 0 1rem;
		font-size: 0.875rem;
		color: #475569;
		line-height: 1.5;
	}

	.detail-card {
		background: #f8fafc;
		border: 1px solid #e2e8f0;
		border-radius: 8px;
		padding: 0.75rem 1rem;
		margin-bottom: 1rem;
	}

	.detail-row {
		display: flex;
		justify-content: space-between;
		padding: 0.375rem 0;
	}

	.detail-row + .detail-row {
		border-top: 1px solid #e2e8f0;
	}

	.detail-label {
		font-size: 0.8125rem;
		color: #94a3b8;
	}

	.detail-value {
		font-size: 0.8125rem;
		color: #1e293b;
		font-weight: 500;
		text-align: right;
		max-width: 60%;
	}

	.capitalize {
		text-transform: capitalize;
	}

	.info-text {
		margin: 0;
		font-size: 0.75rem;
		color: #64748b;
		line-height: 1.5;
		text-align: center;
	}

	.error {
		margin-top: 1rem;
		padding: 0.75rem;
		background: #fef2f2;
		border: 1px solid #fecaca;
		border-radius: 6px;
		color: #dc2626;
		font-size: 0.8125rem;
	}

	.dialog-footer {
		display: flex;
		justify-content: flex-end;
		gap: 0.75rem;
		padding: 1rem 1.5rem;
		border-top: 1px solid #e2e8f0;
		background: #f8fafc;
	}

	.btn {
		padding: 0.5rem 1rem;
		border: 1px solid #cbd5e1;
		border-radius: 6px;
		background: white;
		font-size: 0.875rem;
		cursor: pointer;
		color: #475569;
	}

	.btn:hover:not(:disabled) {
		background: #f1f5f9;
	}

	.btn:disabled {
		opacity: 0.6;
		cursor: not-allowed;
	}

	.btn-warning {
		background: #f59e0b;
		border-color: #f59e0b;
		color: white;
	}

	.btn-warning:hover:not(:disabled) {
		background: #d97706;
	}
</style>
