<script lang="ts">
	/**
	 * What each side of a merge conflict says, per contested field (#828): the
	 * value at the fork, the branch's and the mainline's, side by side.
	 *
	 * A real `<table>` with row and column headers, so a screen reader announces
	 * "Surname, This branch: Lovelace" rather than three unlabelled values. The
	 * values arrive as display text from the server, ids already resolved to
	 * names, so nothing here formats or looks anything up.
	 */
	import type { MergeConflict } from '$lib/api/client';

	interface Props {
		conflict: MergeConflict;
	}

	let { conflict }: Props = $props();

	const values = $derived(conflict.field_values ?? []);

	/** A null value in words: the deleting side's is "Deleted", any other "Not set". */
	function missing(side: 'branch' | 'main' | 'base'): string {
		return side !== 'base' && conflict.deleted_by === side ? 'Deleted' : 'Not set';
	}
</script>

{#if values.length > 0}
	<div class="values-scroll">
		<table class="conflict-values">
			<caption class="sr-only">
				What each side says about {conflict.entity_name || 'this entity'}
			</caption>
			<thead>
				<tr>
					<th scope="col">Field</th>
					<th scope="col">At the fork</th>
					<th scope="col">This branch</th>
					<th scope="col">Mainline</th>
				</tr>
			</thead>
			<tbody>
				{#each values as value (value.field)}
					<tr>
						<th scope="row">{value.label}</th>
						<td class="base">
							{#if value.base_value != null}
								{value.base_value}
							{:else}
								<span class="missing">{missing('base')}</span>
							{/if}
						</td>
						<td class="branch" data-testid="branch-value">
							{#if value.branch_value != null}
								{value.branch_value}
							{:else}
								<span class="missing">{missing('branch')}</span>
							{/if}
						</td>
						<td class="main" data-testid="main-value">
							{#if value.main_value != null}
								{value.main_value}
							{:else}
								<span class="missing">{missing('main')}</span>
							{/if}
						</td>
					</tr>
				{/each}
			</tbody>
		</table>
	</div>
{/if}

<style>
	.values-scroll {
		margin-top: 0.5rem;
		overflow-x: auto;
	}

	.conflict-values {
		width: 100%;
		border-collapse: collapse;
		font-size: 0.8125rem;
		background: #ffffff;
		border: 1px solid #fecaca;
		border-radius: 4px;
	}

	.conflict-values th,
	.conflict-values td {
		padding: 0.375rem 0.5rem;
		text-align: left;
		vertical-align: top;
		border-bottom: 1px solid #fee2e2;
		overflow-wrap: anywhere;
	}

	.conflict-values thead th {
		font-size: 0.75rem;
		font-weight: 600;
		color: #64748b;
		background: #f8fafc;
	}

	.conflict-values tbody th {
		font-weight: 600;
		color: #334155;
	}

	.conflict-values td.base {
		color: #64748b;
	}

	.conflict-values td.branch {
		color: #1e3a8a;
	}

	.conflict-values td.main {
		color: #14532d;
	}

	.missing {
		font-style: italic;
		color: #94a3b8;
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
</style>
