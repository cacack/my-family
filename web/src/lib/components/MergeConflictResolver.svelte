<script lang="ts">
	/**
	 * Interactive picker for the conflicts a branch comparison reported - one
	 * decision per contested entity.
	 *
	 * `POST /branches/{id}/merge` refuses the whole merge unless every conflict
	 * carries a resolution, and rejects a resolution outside the conflict's own
	 * `supported_resolutions` with `400 invalid_resolution`. So this component
	 * renders *only* the values the server would accept: two conflict shapes
	 * (a `delete_edit` where the mainline is the deleter, and every
	 * `create_create`) accept only `main`, and for those a lone radio is
	 * meaningless without the reason - hence `soleOptionReason()`.
	 *
	 * Controlled on purpose: it renders `resolutions` and calls `onresolve`, it
	 * does not own the decisions. The page owns that map because it has to
	 * survive a `409 merge_conflicts` re-render and be serialized into the merge
	 * request.
	 */
	import type { MergeConflict, MergeResolution } from '$lib/api/client';
	import { Badge } from '$lib/components/ui/badge';
	import { Label } from '$lib/components/ui/label';
	import { RadioGroup, RadioGroupItem } from '$lib/components/ui/radio-group';

	interface Props {
		conflicts: MergeConflict[];
		/** Current decisions, keyed by `stream_id`. */
		resolutions: Map<string, MergeResolution>;
		onresolve: (streamId: string, resolution: MergeResolution) => void;
		/** True while a merge request is in flight. */
		disabled?: boolean;
	}

	let { conflicts, resolutions, onresolve, disabled = false }: Props = $props();

	function conflictLabel(kind: MergeConflict['kind']): string {
		switch (kind) {
			case 'edit_edit':
				return 'Both sides edited';
			case 'delete_edit':
				return 'Deleted on one side';
			case 'create_create':
				return 'Created on both sides';
			default:
				return kind;
		}
	}

	/** Never the bare enum value - "branch" and "main" say nothing about the outcome. */
	function optionTitle(resolution: MergeResolution): string {
		return resolution === 'branch' ? "Take the branch's version" : "Keep the mainline's version";
	}

	function optionHelp(resolution: MergeResolution): string {
		return resolution === 'branch'
			? "The branch's changes are replayed onto the mainline, replacing what is there."
			: "The mainline's version stands. This branch's changes to this entity are dropped.";
	}

	/** Paraphrases the `supported_resolutions` schema note for a single-option conflict. */
	function soleOptionReason(conflict: MergeConflict): string {
		switch (conflict.kind) {
			case 'delete_edit':
				return "The mainline deleted this entity. Replaying the branch's edits cannot bring a deleted entity back, so taking the branch's version is not offered - it would report success while the entity stayed deleted.";
			case 'create_create':
				return "Both sides created this entity independently, so they are two different records. Taking the branch's copy would promote it and leave the mainline's beside it - the duplicate this conflict exists to prevent.";
			default:
				return 'Only one resolution would actually produce the outcome it names for this conflict.';
		}
	}

	const headingId = (streamId: string) => `conflict-${streamId}-entity`;
	const optionId = (streamId: string, resolution: string) =>
		`conflict-${streamId}-resolution-${resolution}`;
</script>

{#if conflicts.length > 0}
	<ul class="conflict-list">
		{#each conflicts as conflict (conflict.stream_id)}
			{@const decided = resolutions.get(conflict.stream_id)}
			{@const options = conflict.supported_resolutions}
			<li class="conflict" class:undecided={!decided}>
				<div class="conflict-head">
					<h3 class="conflict-title" id={headingId(conflict.stream_id)}>
						<span class="entity-type">{conflict.entity_type}</span>
						<span class="conflict-name">{conflict.entity_name || 'Unnamed entity'}</span>
					</h3>
					<Badge variant="destructive">{conflictLabel(conflict.kind)}</Badge>
					{#if !decided}
						<!-- Text, not just the border colour, so the state does not depend on sight. -->
						<Badge variant="outline" class="border-amber-500 text-amber-700">
							Needs a decision
						</Badge>
					{/if}
				</div>

				<p class="conflict-detail">{conflict.detail}</p>

				{#if conflict.fields && conflict.fields.length > 0}
					<p class="conflict-fields">Contested fields: {conflict.fields.join(', ')}</p>
				{/if}

				{#if options.length === 1}
					<p class="sole-option">{soleOptionReason(conflict)}</p>
				{/if}

				<RadioGroup
					value={decided ?? ''}
					onValueChange={(value) => onresolve(conflict.stream_id, value as MergeResolution)}
					{disabled}
					aria-labelledby={headingId(conflict.stream_id)}
					class="mt-3 gap-2"
				>
					{#each options as option (option)}
						<div class="option">
							<RadioGroupItem
								value={option}
								id={optionId(conflict.stream_id, option)}
								class="mt-1"
							/>
							<Label
								for={optionId(conflict.stream_id, option)}
								class="cursor-pointer flex-col items-start gap-0.5 font-normal"
							>
								<span class="option-title">{optionTitle(option)}</span>
								<span class="option-help">{optionHelp(option)}</span>
							</Label>
						</div>
					{/each}
				</RadioGroup>
			</li>
		{/each}
	</ul>
{/if}

<style>
	.conflict-list {
		list-style: none;
		margin: 0;
		padding: 0;
		display: flex;
		flex-direction: column;
		gap: 0.75rem;
	}

	.conflict {
		padding: 0.875rem 1rem;
		background: #fef2f2;
		border: 1px solid #fecaca;
		border-radius: 6px;
	}

	.conflict.undecided {
		border-left: 4px solid #f59e0b;
	}

	.conflict-head {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		flex-wrap: wrap;
	}

	.conflict-title {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		flex-wrap: wrap;
		margin: 0;
		font-size: 0.9375rem;
	}

	.entity-type {
		font-size: 0.75rem;
		font-weight: 400;
		color: #94a3b8;
		text-transform: capitalize;
		padding: 0.125rem 0.375rem;
		background: #f1f5f9;
		border-radius: 4px;
	}

	.conflict-name {
		font-weight: 600;
		color: #1e293b;
		overflow-wrap: anywhere;
	}

	.conflict-detail {
		margin: 0.375rem 0 0;
		font-size: 0.875rem;
		color: #7f1d1d;
	}

	.conflict-fields {
		margin: 0.25rem 0 0;
		font-size: 0.8125rem;
		color: #b91c1c;
	}

	.sole-option {
		margin: 0.625rem 0 0;
		padding: 0.5rem 0.75rem;
		background: #fffbeb;
		border: 1px solid #fde68a;
		border-radius: 4px;
		font-size: 0.8125rem;
		color: #92400e;
	}

	.option {
		display: flex;
		align-items: flex-start;
		gap: 0.5rem;
	}

	.option-title {
		font-size: 0.875rem;
		font-weight: 500;
		color: #1e293b;
	}

	.option-help {
		font-size: 0.8125rem;
		color: #64748b;
		overflow-wrap: anywhere;
	}
</style>
