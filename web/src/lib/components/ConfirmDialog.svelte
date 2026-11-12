<script lang="ts">
	/**
	 * The one confirmation for a delete, or any other step that loses data
	 * (see docs/CONVENTIONS.md, "Confirmation and feedback").
	 *
	 * `onconfirm` is awaited with the dialog open. Resolving closes it;
	 * throwing keeps it open and shows the error's message in it. A cancel
	 * returns focus to the trigger; a confirm, which may remove the trigger,
	 * hands it to `focusAfterConfirm` once the dialog has closed. A known
	 * `refusal` replaces the question: the dialog says why the step cannot be
	 * taken and offers only Close, so a refusal never follows the warning.
	 */
	import * as AlertDialog from '#lib/components/ui/alert-dialog/index.js';

	interface Props {
		open: boolean;
		/** The question, e.g. "Delete Ada Lovelace?". */
		title: string;
		/** What confirming does, e.g. "This cannot be undone." */
		description: string;
		confirmLabel?: string;
		/** The confirm button's label while `onconfirm` runs. */
		busyLabel?: string;
		cancelLabel?: string;
		/** Why the step cannot be taken; shown instead of the question. */
		refusal?: string | null;
		refusalTitle?: string;
		onconfirm: () => unknown;
		/** Moves focus after a confirm; the dialog's focus trap undoes it earlier. */
		focusAfterConfirm?: () => void;
	}

	let {
		open = $bindable(),
		title,
		description,
		confirmLabel = 'Delete',
		busyLabel = 'Deleting...',
		cancelLabel = 'Cancel',
		refusal = null,
		refusalTitle = 'This cannot be deleted yet',
		onconfirm,
		focusAfterConfirm
	}: Props = $props();

	let busy = $state(false);
	let error: string | null = $state(null);
	let confirmed = false;

	$effect(() => {
		if (open) error = null;
	});

	async function runConfirm(e: Event) {
		// Keep the dialog open until the step settles, so a failure shows in it.
		e.preventDefault();
		busy = true;
		error = null;
		try {
			await onconfirm();
			confirmed = true;
			open = false;
		} catch (err) {
			error = (err as { message?: string }).message || 'Something went wrong';
		} finally {
			busy = false;
		}
	}
</script>

<AlertDialog.Root
	{open}
	onOpenChange={(isOpen) => {
		if (!busy) open = isOpen;
	}}
>
	<AlertDialog.Content
		onCloseAutoFocus={(e) => {
			if (!confirmed) return;
			confirmed = false;
			e.preventDefault();
			// After the focus scope has let go, or it pulls focus back.
			setTimeout(() => focusAfterConfirm?.());
		}}
	>
		<AlertDialog.Header>
			<AlertDialog.Title>{refusal ? refusalTitle : title}</AlertDialog.Title>
			<AlertDialog.Description>{refusal ?? description}</AlertDialog.Description>
		</AlertDialog.Header>

		{#if error}
			<div class="dialog-error" role="alert">{error}</div>
		{/if}

		<AlertDialog.Footer>
			{#if refusal}
				<AlertDialog.Cancel>Close</AlertDialog.Cancel>
			{:else}
				<AlertDialog.Cancel disabled={busy}>{cancelLabel}</AlertDialog.Cancel>
				<AlertDialog.Action variant="destructive" disabled={busy} onclick={runConfirm}>
					{busy ? busyLabel : confirmLabel}
				</AlertDialog.Action>
			{/if}
		</AlertDialog.Footer>
	</AlertDialog.Content>
</AlertDialog.Root>

<style>
	.dialog-error {
		padding: 0.625rem 0.75rem;
		color: #dc2626;
		background: #fef2f2;
		border: 1px solid #fecaca;
		border-radius: 6px;
		font-size: 0.875rem;
	}
</style>
