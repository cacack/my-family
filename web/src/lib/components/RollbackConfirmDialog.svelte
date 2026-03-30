<script lang="ts">
	import { api, type RollbackResponse } from '$lib/api/client';
	import { Button } from '$lib/components/ui/button';
	import * as AlertDialog from '$lib/components/ui/alert-dialog';

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
		open = $bindable(),
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

	async function performRollback(e: Event) {
		// Prevent AlertDialog from closing automatically
		e.preventDefault();
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
</script>

<AlertDialog.Root bind:open onOpenChange={(isOpen) => { if (!isOpen && !rolling) onCancel(); }}>
	<AlertDialog.Content>
		<AlertDialog.Header>
			<AlertDialog.Title>Confirm Rollback</AlertDialog.Title>
			<AlertDialog.Description>
				You are about to rollback <strong>{entityName}</strong> from
				version {currentVersion} to version {targetVersion}.
			</AlertDialog.Description>
		</AlertDialog.Header>

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

		<AlertDialog.Footer>
			<AlertDialog.Cancel disabled={rolling}>Cancel</AlertDialog.Cancel>
			<AlertDialog.Action variant="warning" disabled={rolling} onclick={performRollback}>
				{rolling ? 'Rolling back...' : 'Confirm Rollback'}
			</AlertDialog.Action>
		</AlertDialog.Footer>
	</AlertDialog.Content>
</AlertDialog.Root>

<style>
	.detail-card {
		background: hsl(var(--muted));
		border: 1px solid hsl(var(--border));
		border-radius: 8px;
		padding: 0.75rem 1rem;
	}

	.detail-row {
		display: flex;
		justify-content: space-between;
		padding: 0.375rem 0;
	}

	.detail-row + .detail-row {
		border-top: 1px solid hsl(var(--border));
	}

	.detail-label {
		font-size: 0.8125rem;
		color: hsl(var(--muted-foreground));
	}

	.detail-value {
		font-size: 0.8125rem;
		color: hsl(var(--foreground));
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
		color: hsl(var(--muted-foreground));
		line-height: 1.5;
		text-align: center;
	}

	.error {
		padding: 0.75rem;
		background: hsl(var(--destructive) / 0.1);
		border: 1px solid hsl(var(--destructive) / 0.3);
		border-radius: 6px;
		color: hsl(var(--destructive));
		font-size: 0.8125rem;
	}
</style>
