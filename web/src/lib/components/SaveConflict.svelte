<script module lang="ts">
	export interface ConflictField {
		label: string;
		/** The value in the user's form, as display text. */
		mine: string;
		/** The value in the latest saved version, as display text. */
		latest: string;
	}
</script>

<script lang="ts">
	/**
	 * A save refused with a 409 because the record changed since the form was
	 * opened (#899). The user's edits stay in the form; this compares them with
	 * the latest saved version, field by field, and lets the user either save
	 * their edits over it or take the latest version instead.
	 *
	 * Distinct from `ConflictError`, which offers a blind retry for sub-resource
	 * writes the client already retried once: a whole-record edit is not safe to
	 * retry blindly, so this one shows what would be overwritten.
	 */
	import { Button } from '$lib/components/ui/button';

	interface Props {
		/** What was edited, for the message: "person", "family". */
		noun: string;
		fields: ConflictField[];
		onKeepMine: () => void;
		onUseLatest: () => void;
		busy?: boolean;
	}

	let { noun, fields, onKeepMine, onUseLatest, busy = false }: Props = $props();

	const differing = $derived(fields.filter((field) => field.mine !== field.latest));
</script>

<div class="save-conflict" role="alert">
	<p class="message">
		This {noun} was changed elsewhere while you were editing, so your changes were not saved. Your
		edits are still in the form.
	</p>
	{#if differing.length > 0}
		<div class="values-scroll">
			<table>
				<caption class="sr-only">Your edits compared with the latest saved version</caption>
				<thead>
					<tr>
						<th scope="col">Field</th>
						<th scope="col">Your edit</th>
						<th scope="col">Latest saved</th>
					</tr>
				</thead>
				<tbody>
					{#each differing as field (field.label)}
						<tr>
							<th scope="row">{field.label}</th>
							<td data-testid="mine">{field.mine || '—'}</td>
							<td data-testid="latest">{field.latest || '—'}</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	{:else}
		<p class="message">Your edits match the latest saved version.</p>
	{/if}
	<div class="actions">
		<Button onclick={onKeepMine} disabled={busy}>
			{busy ? 'Saving...' : 'Save my edits'}
		</Button>
		<Button variant="outline" onclick={onUseLatest} disabled={busy}>Use the latest version</Button>
	</div>
</div>

<style>
	.save-conflict {
		padding: 0.75rem 1rem;
		margin-bottom: 1rem;
		background: #fffbeb;
		border: 1px solid #fde68a;
		border-radius: 8px;
		font-size: 0.875rem;
	}

	.message {
		margin: 0 0 0.5rem;
		color: #92400e;
	}

	.values-scroll {
		overflow-x: auto;
	}

	table {
		width: 100%;
		border-collapse: collapse;
		font-size: 0.8125rem;
		background: #ffffff;
		border: 1px solid #fde68a;
	}

	th,
	td {
		padding: 0.375rem 0.5rem;
		text-align: left;
		vertical-align: top;
		border-bottom: 1px solid #fef3c7;
		overflow-wrap: anywhere;
		color: #1e293b;
	}

	thead th {
		font-size: 0.75rem;
		font-weight: 600;
		color: #64748b;
		background: #f8fafc;
	}

	.actions {
		display: flex;
		flex-wrap: wrap;
		gap: 0.5rem;
		margin-top: 0.75rem;
	}

	.sr-only {
		position: absolute;
		width: 1px;
		height: 1px;
		padding: 0;
		margin: -1px;
		overflow: hidden;
		clip: rect(0, 0, 0, 0);
		white-space: nowrap;
		border: 0;
	}

	:global(body.high-contrast) .save-conflict {
		border-color: #d97706;
		background: #fef3c7;
	}
</style>
