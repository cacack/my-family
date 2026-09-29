<script lang="ts">
	/**
	 * "Main moved underneath" indicator for one research branch (#837).
	 *
	 * Branches are live overlays (ADR-005): every record the branch has not
	 * edited shows the mainline's current data, so the mainline keeps moving
	 * under an open branch. This says how much, from the cheap `drift` counts -
	 * no full compare - and links to the part of the compare page that lists
	 * the mainline changes to entities this branch touched.
	 *
	 * It inherits its text colour so it sits on the banner and on a branch card
	 * alike.
	 */
	import type { BranchDrift } from '$lib/api/client';

	interface Props {
		drift: BranchDrift;
	}

	let { drift }: Props = $props();

	const moved = $derived(drift.main_change_count > 0);
	const touched = $derived(drift.main_change_count_on_branch_entities);

	/**
	 * `has_more` means the server stopped counting at its cap. The on-branch
	 * count can never exceed the total, so the total is always a lower bound
	 * then, and the on-branch count is one too only when it reached the total.
	 */
	const total = $derived(
		`${drift.main_change_count.toLocaleString('en-US')}${drift.has_more ? '+' : ''}`
	);
	const onBranch = $derived(
		`${touched.toLocaleString('en-US')}${drift.has_more && touched === drift.main_change_count ? '+' : ''}`
	);
</script>

<span class="drift" class:drift-touched={touched > 0} data-testid="branch-drift">
	{#if moved}
		<span class="drift-text">
			Mainline changed {total}
			{drift.main_change_count === 1 && !drift.has_more ? 'time' : 'times'} since you branched,
			<strong>{onBranch}</strong> on entities this branch touched.
		</span>
		<a class="drift-link" href="/branches/{drift.branch_id}#main-changes">
			{touched > 0 ? 'Review mainline changes' : 'Open compare'}
		</a>
	{:else}
		<span class="drift-text">Mainline unchanged since you branched.</span>
	{/if}
</span>

<style>
	.drift {
		display: inline-flex;
		align-items: baseline;
		gap: 0.5rem;
		flex-wrap: wrap;
		font-size: 0.8125rem;
		color: inherit;
	}

	.drift-touched .drift-text strong {
		font-weight: 700;
	}

	.drift-link {
		color: inherit;
		font-weight: 500;
		text-decoration: underline;
		text-underline-offset: 2px;
		white-space: nowrap;
	}

	.drift-link:focus-visible {
		outline: 2px solid currentColor;
		outline-offset: 2px;
	}
</style>
