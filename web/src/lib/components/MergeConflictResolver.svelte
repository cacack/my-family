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
	 * Each conflict shows what each side says (`ConflictValues`: the fork, the
	 * branch and the mainline, side by side), and can carry an optional
	 * rationale - why that side won - which the merge records (#828). With two
	 * or more conflicts, bulk controls decide them all, or all of one entity
	 * type, at once; a bulk choice only ever sets a side a conflict accepts.
	 *
	 * Controlled on purpose: it renders `resolutions` and calls `onresolve`, it
	 * does not own the decisions. The page owns that map because it has to
	 * survive a `409 merge_conflicts` re-render and be serialized into the merge
	 * request. The same goes for `rationales`.
	 */
	import type { MergeConflict, MergeResolution } from '$lib/api/client';
	import ConflictValues from '$lib/components/ConflictValues.svelte';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import { Label } from '$lib/components/ui/label';
	import { RadioGroup, RadioGroupItem } from '$lib/components/ui/radio-group';
	import { Textarea } from '$lib/components/ui/textarea';
	import { entityTypeLabel, unnamedEntityLabel } from '$lib/utils/changeEntries';

	/** The server's limit on one rationale, in characters. */
	const RATIONALE_MAX_LENGTH = 1000;

	interface Props {
		conflicts: MergeConflict[];
		/** Current decisions, keyed by `stream_id`. */
		resolutions: Map<string, MergeResolution>;
		onresolve: (streamId: string, resolution: MergeResolution) => void;
		/**
		 * Several decisions at once, from a bulk control. Optional: without it
		 * the bulk controls fall back to one `onresolve` call per conflict.
		 */
		onresolveall?: (decisions: Array<[string, MergeResolution]>) => void;
		/** Optional reasoning per decision, keyed by `stream_id`. */
		rationales?: Map<string, string>;
		/** Omit to hide the rationale fields. */
		onrationale?: (streamId: string, rationale: string) => void;
		/** True while a merge request is in flight. */
		disabled?: boolean;
	}

	let {
		conflicts,
		resolutions,
		onresolve,
		onresolveall,
		rationales = new Map(),
		onrationale,
		disabled = false
	}: Props = $props();

	/** Entity types among the conflicts, in first-seen order, with their counts. */
	const conflictTypes = $derived.by(() => {
		const counts = new Map<string, number>();
		for (const conflict of conflicts) {
			counts.set(conflict.entity_type, (counts.get(conflict.entity_type) ?? 0) + 1);
		}
		return [...counts].map(([entityType, count]) => ({ entityType, count }));
	});

	/** The outcome of the last bulk action, announced politely. */
	let bulkStatus = $state('');

	function targetsOf(entityType: string | null): MergeConflict[] {
		return entityType === null
			? conflicts
			: conflicts.filter((conflict) => conflict.entity_type === entityType);
	}

	function acceptsAny(entityType: string | null, resolution: MergeResolution): boolean {
		return targetsOf(entityType).some((c) => c.supported_resolutions.includes(resolution));
	}

	function plural(n: number, word: string): string {
		return `${n} ${word}${n === 1 ? '' : 's'}`;
	}

	/**
	 * Decide every targeted conflict for one side. A conflict that does not
	 * accept that side is left exactly as it was - never flipped to the other -
	 * and the status line says how many were skipped and why.
	 */
	function decideAll(resolution: MergeResolution, entityType: string | null) {
		const targets = targetsOf(entityType);
		const applicable = targets.filter((c) => c.supported_resolutions.includes(resolution));
		const decisions: Array<[string, MergeResolution]> = applicable.map((c) => [
			c.stream_id,
			resolution
		]);
		if (onresolveall) {
			onresolveall(decisions);
		} else {
			for (const [streamId, side] of decisions) onresolve(streamId, side);
		}

		const side = resolution === 'branch' ? "the branch's version" : "the mainline's version";
		let status = `Chose ${side} for ${plural(applicable.length, 'conflict')}.`;
		const skipped = targets.length - applicable.length;
		if (skipped > 0) {
			status += ` ${plural(skipped, 'conflict')} cannot take ${side} and ${skipped === 1 ? 'was' : 'were'} left as ${skipped === 1 ? 'it was' : 'they were'}.`;
		}
		bulkStatus = status;
	}

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
	const rationaleId = (streamId: string) => `conflict-${streamId}-rationale`;
	const optionId = (streamId: string, resolution: string) =>
		`conflict-${streamId}-resolution-${resolution}`;
</script>

{#if conflicts.length > 1}
	<div class="bulk" role="group" aria-labelledby="bulk-heading">
		<p class="bulk-heading" id="bulk-heading">Decide several at once</p>
		<div class="bulk-row">
			<span class="bulk-scope">All {plural(conflicts.length, 'conflict')}</span>
			<Button
				variant="outline"
				size="sm"
				disabled={disabled || !acceptsAny(null, 'branch')}
				onclick={() => decideAll('branch', null)}
			>
				Take the branch's version for all
			</Button>
			<Button
				variant="outline"
				size="sm"
				disabled={disabled || !acceptsAny(null, 'main')}
				onclick={() => decideAll('main', null)}
			>
				Keep the mainline's version for all
			</Button>
		</div>
		{#if conflictTypes.length > 1}
			{#each conflictTypes as { entityType, count } (entityType)}
				{@const typeLabel = entityTypeLabel(entityType)}
				<div class="bulk-row">
					<span class="bulk-scope">{typeLabel} ({count})</span>
					<Button
						variant="ghost"
						size="sm"
						disabled={disabled || !acceptsAny(entityType, 'branch')}
						aria-label="Take branch for every {typeLabel.toLowerCase()} conflict"
						onclick={() => decideAll('branch', entityType)}
					>
						Take branch
					</Button>
					<Button
						variant="ghost"
						size="sm"
						disabled={disabled || !acceptsAny(entityType, 'main')}
						aria-label="Keep mainline for every {typeLabel.toLowerCase()} conflict"
						onclick={() => decideAll('main', entityType)}
					>
						Keep mainline
					</Button>
				</div>
			{/each}
		{/if}
		<p class="bulk-status" role="status" aria-live="polite">{bulkStatus}</p>
	</div>
{/if}

{#if conflicts.length > 0}
	<ul class="conflict-list">
		{#each conflicts as conflict (conflict.stream_id)}
			{@const decided = resolutions.get(conflict.stream_id)}
			{@const options = conflict.supported_resolutions}
			<li class="conflict" class:undecided={!decided}>
				<div class="conflict-head">
					<h3 class="conflict-title" id={headingId(conflict.stream_id)}>
						<span class="entity-type">{entityTypeLabel(conflict.entity_type)}</span>
						<span class="conflict-name"
							>{conflict.entity_name || unnamedEntityLabel(conflict.entity_type)}</span
						>
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

				{#if conflict.field_values && conflict.field_values.length > 0}
					<ConflictValues {conflict} />
				{:else if conflict.fields && conflict.fields.length > 0}
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

				{#if onrationale}
					<div class="rationale">
						<Label for={rationaleId(conflict.stream_id)} class="rationale-label">
							Why this side? <span class="optional">(optional - recorded with the merge)</span>
						</Label>
						<Textarea
							id={rationaleId(conflict.stream_id)}
							value={rationales.get(conflict.stream_id) ?? ''}
							oninput={(event) =>
								onrationale(conflict.stream_id, (event.currentTarget as HTMLTextAreaElement).value)}
							maxlength={RATIONALE_MAX_LENGTH}
							rows={2}
							{disabled}
							placeholder="The evidence you weighed, e.g. the 1881 census gives her birthplace as..."
						/>
					</div>
				{/if}
			</li>
		{/each}
	</ul>
{/if}

<style>
	.bulk {
		margin-bottom: 0.75rem;
		padding: 0.625rem 0.875rem;
		background: #f8fafc;
		border: 1px solid #e2e8f0;
		border-radius: 6px;
	}

	.bulk-heading {
		margin: 0 0 0.375rem;
		font-size: 0.8125rem;
		font-weight: 600;
		color: #334155;
	}

	.bulk-row {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		flex-wrap: wrap;
		margin-top: 0.25rem;
	}

	.bulk-scope {
		min-width: 9rem;
		font-size: 0.8125rem;
		color: #475569;
	}

	.bulk-status {
		margin: 0.375rem 0 0;
		font-size: 0.8125rem;
		color: #334155;
	}

	/* Empty, the live region stays rendered (so the first announcement is
	   heard) but takes no space. */
	.bulk-status:empty {
		margin: 0;
	}

	.rationale {
		margin-top: 0.625rem;
		display: flex;
		flex-direction: column;
		gap: 0.25rem;
	}

	.optional {
		font-weight: 400;
		color: #64748b;
	}

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
