<script lang="ts" module>
	import type { Branch, PromoteResearchLogsResult } from '$lib/api/client';

	/** What a close produced, handed to `onclosed`. */
	export interface BranchCloseResult {
		branch: Branch;
		/** The promotion's result, when research logs were copied to the mainline. */
		promotion: PromoteResearchLogsResult | null;
		/** Set when the close succeeded but copying the research logs failed. */
		promotionError: string | null;
	}
</script>

<script lang="ts">
	/**
	 * Close a branch without merging it (#836): pick the outcome the research
	 * reached, say why, and optionally copy the branch's research logs to the
	 * mainline so its searches - above all the ones that found nothing - stay
	 * part of the mainline's record. The close is one request; the copy is a
	 * second one, made only once the close has succeeded, so a failed copy
	 * never leaves the branch half-closed (it can be retried from the branch's
	 * research page).
	 */
	import { api, type ApiError, type BranchCloseOutcome } from '$lib/api/client';
	import {
		CLOSE_OUTCOMES,
		CLOSE_OUTCOME_HINTS,
		CLOSE_REASON_MAX_LENGTH,
		OUTCOME_LABELS
	} from '$lib/utils/branchResearch';
	import { Button } from '$lib/components/ui/button';
	import { Label } from '$lib/components/ui/label';
	import { Textarea } from '$lib/components/ui/textarea';
	import * as Dialog from '$lib/components/ui/dialog';

	interface Props {
		/** The branch to close; the dialog is open while this is non-null. */
		branch: Branch | null;
		onclosed: (result: BranchCloseResult) => void;
		oncancel: () => void;
	}

	let { branch, onclosed, oncancel }: Props = $props();

	let outcome: BranchCloseOutcome | '' = $state('');
	let reason = $state('');
	let promote = $state(true);
	let closing = $state(false);
	let error: string | null = $state(null);

	// A fresh form for every branch the dialog is opened on.
	let openedFor: string | null = null;
	$effect(() => {
		const id = branch?.id ?? null;
		if (id !== openedFor) {
			openedFor = id;
			outcome = '';
			reason = '';
			promote = true;
			error = null;
			closing = false;
		}
	});

	async function handleSubmit(event: Event) {
		event.preventDefault();
		const target = branch;
		if (!target || !outcome || closing) return;

		closing = true;
		error = null;
		let closed: Branch;
		try {
			const trimmed = reason.trim();
			closed = await api.closeBranch(target.id, {
				outcome,
				...(trimmed ? { reason: trimmed } : {})
			});
		} catch (e) {
			const apiError = e as ApiError;
			error =
				apiError.status === 409
					? 'This branch is no longer active - it has already been merged or closed. Reload to see it.'
					: apiError.message || 'Failed to close the branch';
			closing = false;
			return;
		}

		let promotion: PromoteResearchLogsResult | null = null;
		let promotionError: string | null = null;
		if (promote) {
			try {
				promotion = await api.promoteBranchResearchLogs(target.id);
			} catch (e) {
				promotionError =
					(e as ApiError).message || 'The research logs could not be copied to the mainline';
			}
		}
		closing = false;
		onclosed({ branch: closed, promotion, promotionError });
	}
</script>

<Dialog.Root
	open={branch !== null}
	onOpenChange={(isOpen) => {
		if (!isOpen && !closing) oncancel();
	}}
>
	<Dialog.Content class="sm:max-w-lg">
		<Dialog.Header>
			<Dialog.Title>Close this branch?</Dialog.Title>
			<Dialog.Description>
				{branch?.name} will be closed without merging: its changes are not applied to the mainline
				and it accepts no further edits. Its research log, evidence analyses and proof summaries
				stay readable from the branch's research page.
			</Dialog.Description>
		</Dialog.Header>

		<form onsubmit={handleSubmit}>
			<div class="field">
				<Label for="close-outcome">Outcome</Label>
				<select id="close-outcome" class="native-select" bind:value={outcome} required>
					<option value="" disabled>Choose what the research concluded</option>
					{#each CLOSE_OUTCOMES as option (option)}
						<option value={option}>{OUTCOME_LABELS[option]}</option>
					{/each}
				</select>
				{#if outcome}
					<span class="field-hint left">{CLOSE_OUTCOME_HINTS[outcome]}</span>
				{/if}
			</div>

			<div class="field">
				<Label for="close-reason">Reason (optional)</Label>
				<Textarea
					id="close-reason"
					bind:value={reason}
					maxlength={CLOSE_REASON_MAX_LENGTH}
					rows={3}
					placeholder="The 1850 census places Mary in Ohio with a different father."
				/>
				<span class="field-hint">{reason.length}/{CLOSE_REASON_MAX_LENGTH}</span>
			</div>

			<label class="promote">
				<input type="checkbox" bind:checked={promote} />
				<span>
					Copy this branch's research logs to the mainline
					<span class="promote-hint">
						Searches about people and families that exist on the mainline are kept there, noting
						this branch and why it was closed.
					</span>
				</span>
			</label>

			{#if error}
				<div class="dialog-error" role="alert">{error}</div>
			{/if}

			<Dialog.Footer>
				<Button type="button" variant="secondary" disabled={closing} onclick={oncancel}>
					Cancel
				</Button>
				<Button type="submit" variant="destructive" disabled={closing || !outcome}>
					{closing ? 'Closing...' : 'Close branch'}
				</Button>
			</Dialog.Footer>
		</form>
	</Dialog.Content>
</Dialog.Root>

<style>
	.field {
		display: flex;
		flex-direction: column;
		gap: 0.375rem;
		margin-bottom: 1rem;
	}

	.field-hint {
		align-self: flex-end;
		font-size: 0.6875rem;
		color: #94a3b8;
	}

	.field-hint.left {
		align-self: flex-start;
		font-size: 0.75rem;
		color: #64748b;
	}

	.native-select {
		height: 2.25rem;
		padding: 0 0.5rem;
		border: 1px solid #cbd5e1;
		border-radius: 6px;
		background: white;
		font-size: 0.875rem;
	}

	.promote {
		display: flex;
		align-items: flex-start;
		gap: 0.5rem;
		margin-bottom: 1rem;
		font-size: 0.875rem;
		color: #1e293b;
	}

	.promote input {
		margin-top: 0.2rem;
	}

	.promote-hint {
		display: block;
		font-size: 0.75rem;
		color: #64748b;
	}

	.dialog-error {
		margin-bottom: 1rem;
		padding: 0.75rem;
		background: hsl(var(--destructive) / 0.1);
		border: 1px solid hsl(var(--destructive) / 0.3);
		border-radius: 6px;
		color: hsl(var(--destructive));
		font-size: 0.8125rem;
	}
</style>
