<script lang="ts">
	import type { ExportEstimate } from '$lib/api/client';
	import { Button } from '$lib/components/ui/button';
	import * as Dialog from '$lib/components/ui/dialog';

	interface Props {
		open: boolean;
		estimate: ExportEstimate;
		onConfirm: () => void;
		onCancel: () => void;
	}

	let { open = $bindable(), estimate, onConfirm, onCancel }: Props = $props();

	function formatBytes(bytes: number): string {
		if (bytes < 1024) return `${bytes} B`;
		if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
		return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
	}
</script>

<Dialog.Root bind:open onOpenChange={(isOpen) => { if (!isOpen) onCancel(); }}>
	<Dialog.Content showCloseButton={false} class="sm:max-w-md">
		<Dialog.Header>
			<Dialog.Title class="flex items-center gap-3">
				<svg
					class="h-6 w-6 shrink-0 text-amber-500"
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
				Large Export Warning
			</Dialog.Title>
			<Dialog.Description>
				This export contains <strong>{estimate.total_records.toLocaleString()}</strong> records
				(approximately <strong>{formatBytes(estimate.estimated_bytes)}</strong>). The export may
				take some time to complete.
			</Dialog.Description>
		</Dialog.Header>

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

		<Dialog.Footer>
			<Button variant="secondary" onclick={onCancel}>Cancel</Button>
			<Button variant="default" onclick={onConfirm}>Proceed with Export</Button>
		</Dialog.Footer>
	</Dialog.Content>
</Dialog.Root>

<style>
	.record-summary {
		background: hsl(var(--muted));
		border: 1px solid hsl(var(--border));
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
		border-bottom: 1px solid hsl(var(--border));
		padding-bottom: 0.375rem;
		margin-bottom: 0.375rem;
	}

	.record-label {
		color: hsl(var(--muted-foreground));
	}

	.record-value {
		font-weight: 500;
		color: hsl(var(--foreground));
		font-variant-numeric: tabular-nums;
	}
</style>
