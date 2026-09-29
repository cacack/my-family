<script lang="ts">
	/**
	 * The merge blockers the review's current decisions would run into (#831):
	 * every cross-entity reference the merge would break, by name, each with the
	 * one-click fix the server suggests. The page re-checks after every change
	 * (`POST /branches/{id}/merge/precheck`), so fixing one refreshes the list -
	 * a fix can surface another blocker (leaving out a person can strand a
	 * family that links them), which is why the list is never edited locally.
	 *
	 * Controlled: it renders `blockers` and calls `onfix`; the page owns the
	 * decisions a fix changes.
	 */
	import type { MergeBlocker } from '$lib/api/client';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import { blockerFixLabel, describeBlocker } from '$lib/utils/mergeBlockers';

	interface Props {
		blockers: MergeBlocker[];
		/** True while a re-check is in flight. */
		checking?: boolean;
		/** Set when the last check failed; the merge still checks for itself. */
		error?: string | null;
		onfix: (blocker: MergeBlocker) => void;
		/** True while a merge request is in flight. */
		disabled?: boolean;
	}

	let { blockers, checking = false, error = null, onfix, disabled = false }: Props = $props();

	const heading = $derived(
		blockers.length === 1 ? '1 merge blocker' : `${blockers.length} merge blockers`
	);
</script>

{#if blockers.length > 0}
	<section class="blockers" aria-labelledby="merge-blockers-heading" data-testid="merge-blockers">
		<div class="blockers-head">
			<h2 id="merge-blockers-heading">{heading}</h2>
			{#if checking}
				<span class="checking" role="status">Re-checking...</span>
			{/if}
		</div>
		<p class="hint">
			Decisions are made per entity, but entities reference each other. With the decisions below,
			merging would break these references, so the merge is held until each is fixed.
		</p>
		<ul class="blocker-list">
			{#each blockers as blocker (`${blocker.kind}:${blocker.stream_id}:${blocker.referenced_id}`)}
				<li class="blocker">
					<div class="blocker-text">
						<Badge variant="destructive">Merge blocker</Badge>
						<span>{describeBlocker(blocker)}</span>
					</div>
					<Button
						variant="outline"
						size="sm"
						{disabled}
						onclick={() => onfix(blocker)}
					>
						{blockerFixLabel(blocker)}
					</Button>
				</li>
			{/each}
		</ul>
	</section>
{:else if error}
	<p class="check-error" role="note">
		The merge blockers could not be checked ({error}). Merging still checks them, and refuses if
		any remain.
	</p>
{/if}

<style>
	.blockers {
		margin-bottom: 2rem;
		padding: 0.875rem 1rem;
		background: #fff7ed;
		border: 1px solid #fdba74;
		border-radius: 8px;
	}

	.blockers-head {
		display: flex;
		align-items: center;
		gap: 0.75rem;
		flex-wrap: wrap;
	}

	.blockers-head h2 {
		margin: 0;
		font-size: 1rem;
		color: #7c2d12;
	}

	.checking {
		font-size: 0.8125rem;
		color: #9a3412;
	}

	.hint {
		margin: 0.25rem 0 0.75rem;
		font-size: 0.8125rem;
		color: #9a3412;
		max-width: 52rem;
	}

	.blocker-list {
		list-style: none;
		margin: 0;
		padding: 0;
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
	}

	.blocker {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 0.75rem;
		flex-wrap: wrap;
		padding: 0.625rem 0.75rem;
		background: white;
		border: 1px solid #fed7aa;
		border-radius: 6px;
	}

	.blocker-text {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		flex-wrap: wrap;
		font-size: 0.875rem;
		color: #1e293b;
		min-width: 0;
		overflow-wrap: anywhere;
	}

	.check-error {
		margin: 0 0 1.5rem;
		padding: 0.625rem 0.875rem;
		background: #f8fafc;
		border: 1px solid #e2e8f0;
		border-radius: 6px;
		font-size: 0.8125rem;
		color: #475569;
	}
</style>
