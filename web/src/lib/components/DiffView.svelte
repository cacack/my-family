<script lang="ts">
	import type { FieldChange } from '$lib/api/client';

	interface Props {
		changes: Record<string, FieldChange>;
	}

	let { changes }: Props = $props();

	function formatValue(value: unknown): string {
		if (value === null || value === undefined) {
			return '(empty)';
		}
		if (typeof value === 'object') {
			// Handle GenDate objects
			if ('raw' in (value as Record<string, unknown>)) {
				return String((value as Record<string, unknown>).raw) || '(empty)';
			}
			return JSON.stringify(value);
		}
		return String(value);
	}

	function formatFieldName(field: string): string {
		// Convert snake_case to Title Case
		return field
			.split('_')
			.map((word) => word.charAt(0).toUpperCase() + word.slice(1))
			.join(' ');
	}

	function isDateField(field: string): boolean {
		return field.includes('date') || field.includes('Date');
	}
</script>

<div class="diff-view">
	{#each Object.entries(changes) as [field, change]}
		<div class="diff-row">
			<span class="field-name">{formatFieldName(field)}</span>
			<div class="diff-values">
				{#if change.old_value !== undefined && change.old_value !== null}
					<span class="old-value">{formatValue(change.old_value)}</span>
				{:else}
					<span class="empty-value">(empty)</span>
				{/if}
				<span class="arrow">→</span>
				{#if change.new_value !== undefined && change.new_value !== null}
					<span class="new-value">{formatValue(change.new_value)}</span>
				{:else}
					<span class="empty-value">(empty)</span>
				{/if}
			</div>
		</div>
	{/each}
</div>

<style>
	.diff-view {
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
	}

	.diff-row {
		display: flex;
		flex-direction: column;
		gap: 0.25rem;
		padding: 0.5rem;
		background: #f8fafc;
		border-radius: 4px;
	}

	.field-name {
		font-size: 0.75rem;
		font-weight: 600;
		color: #64748b;
		text-transform: uppercase;
		letter-spacing: 0.025em;
	}

	.diff-values {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		flex-wrap: wrap;
	}

	.old-value {
		background: #fecaca;
		color: #991b1b;
		padding: 0.125rem 0.5rem;
		border-radius: 4px;
		font-size: 0.875rem;
		text-decoration: line-through;
	}

	.new-value {
		background: #bbf7d0;
		color: #166534;
		padding: 0.125rem 0.5rem;
		border-radius: 4px;
		font-size: 0.875rem;
	}

	.empty-value {
		color: #94a3b8;
		font-size: 0.875rem;
		font-style: italic;
	}

	.arrow {
		color: #94a3b8;
		font-size: 0.875rem;
	}
</style>
