<script lang="ts">
	/**
	 * The merged branch's link to what its merge changed (#833), from the merge
	 * record's pre-merge snapshot.
	 *
	 * With `replayed_through_position` the link is an exact range: from the
	 * snapshot to the last change the merge replayed, so later mainline work
	 * never shows up in it (for a merge that replayed nothing, the range is
	 * empty). Without it (the scan for the replayed changes was cut short
	 * before it found them all) it compares the snapshot to now, and says so -
	 * that view also holds whatever the mainline did after the merge.
	 *
	 * The snapshot is a mainline snapshot, and snapshot comparisons follow the
	 * active branch, so standing on a branch the comparison cannot open; the
	 * link says to return to the mainline first rather than lead to a refusal.
	 */
	import type { MergeRecord } from '$lib/api/client';
	import { snapshotCompareRangeHref, snapshotCompareToNowHref } from '$lib/utils/snapshots';

	interface Props {
		record: MergeRecord;
		/** True when a research branch is active, so mainline snapshots cannot be compared. */
		onBranch?: boolean;
	}

	let { record, onBranch = false }: Props = $props();

	const link = $derived.by(() => {
		const snapshotId = record.pre_merge_snapshot_id;
		if (!snapshotId) return null;
		if (record.replayed_through_position !== undefined) {
			return {
				href: snapshotCompareRangeHref(snapshotId, record.replayed_through_position),
				label: 'See exactly what this merge changed',
				hint: 'Compares the snapshot taken just before the merge with the mainline right after it.'
			};
		}
		return {
			href: snapshotCompareToNowHref(snapshotId),
			label: 'See everything changed since before the merge',
			hint: 'Compares the snapshot taken just before the merge with the mainline now, so it also lists changes made after the merge.'
		};
	});
</script>

<div class="merge-effect" data-testid="merge-effect">
	{#if link}
		<a href={link.href} class="effect-link" data-testid="merge-effect-link">{link.label}</a>
		<p class="effect-hint">{link.hint}</p>
		{#if onBranch}
			<p class="effect-hint" role="note">
				The snapshot is on the mainline: return to the mainline to open the comparison.
			</p>
		{/if}
	{:else}
		<p class="effect-hint">No snapshot was taken before this merge, so there is no view of its effect alone.</p>
	{/if}
</div>

<style>
	.merge-effect {
		margin: 0.75rem 0 1rem;
	}

	.effect-link {
		font-weight: 600;
		color: #2563eb;
	}

	.effect-link:hover {
		text-decoration: underline;
	}

	.effect-hint {
		margin: 0.25rem 0 0;
		font-size: 0.8125rem;
		color: #64748b;
	}
</style>
