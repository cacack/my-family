<script lang="ts">
	/**
	 * Research snapshots: named markers ("tags") on the mainline's event history,
	 * e.g. "Pre-DNA results" or "After courthouse trip".
	 *
	 * A snapshot records nothing but a position, so creating or deleting one never
	 * touches research data. Two snapshots can be compared to see every mainline
	 * change recorded between them (`/snapshots/compare`).
	 */
	import { goto } from '$app/navigation';
	import { api, type ApiError, type Snapshot } from '$lib/api/client';
	import MainlineNotice from '$lib/components/MainlineNotice.svelte';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { Textarea } from '$lib/components/ui/textarea';
	import * as Dialog from '$lib/components/ui/dialog';
	import * as AlertDialog from '$lib/components/ui/alert-dialog';
	import { snapshotCompareHref } from '$lib/utils/snapshots';

	// Mirrors the maxLength on SnapshotCreate in openapi.yaml.
	const NAME_MAX_LENGTH = 100;
	const DESCRIPTION_MAX_LENGTH = 500;

	let snapshots: Snapshot[] = $state([]);
	let loading = $state(true);
	let error: string | null = $state(null);

	// Create dialog
	let createOpen = $state(false);
	let creating = $state(false);
	let createError: string | null = $state(null);
	let newName = $state('');
	let newDescription = $state('');

	// Delete dialog
	let deleteTarget: Snapshot | null = $state(null);
	let deleting = $state(false);
	let deleteError: string | null = $state(null);

	// Compare form: ids of the two snapshots to compare.
	let compareFrom = $state('');
	let compareTo = $state('');

	/** Polite screen-reader announcement for actions whose result is otherwise only visual. */
	let announcement = $state('');

	// Both halves measure the trimmed name, because the trimmed name is what
	// `handleCreate` sends: trailing whitespace must not cost a character.
	const trimmedName = $derived(newName.trim());
	const nameValid = $derived(trimmedName.length > 0 && trimmedName.length <= NAME_MAX_LENGTH);
	/** Only complain once the user has typed something; an untouched field is not an error. */
	const nameError = $derived(
		newName.length > 0 && trimmedName.length === 0 ? 'The name cannot be only spaces.' : null
	);
	const compareValid = $derived(
		compareFrom !== '' && compareTo !== '' && compareFrom !== compareTo
	);

	function announce(message: string) {
		// Clear first so a repeated message is still read out.
		announcement = '';
		setTimeout(() => {
			announcement = message;
		}, 50);
	}

	function formatTimestamp(iso: string): string {
		return new Date(iso).toLocaleDateString('en-US', {
			month: 'short',
			day: 'numeric',
			year: 'numeric',
			hour: 'numeric',
			minute: '2-digit'
		});
	}

	/** Names need not be unique, so the date tells two same-named snapshots apart. */
	function optionLabel(snapshot: Snapshot): string {
		return `${snapshot.name} (${formatTimestamp(snapshot.created_at)})`;
	}

	/**
	 * Keep the compare selection pointing at snapshots that still exist, and
	 * default it to the two most recent (older -> newer) when it does not.
	 */
	function syncCompareSelection() {
		const ids = new Set(snapshots.map((s) => s.id));
		if (!ids.has(compareFrom)) compareFrom = snapshots[1]?.id ?? '';
		if (!ids.has(compareTo)) compareTo = snapshots[0]?.id ?? '';
	}

	async function loadSnapshots() {
		loading = true;
		error = null;
		try {
			const result = await api.listSnapshots();
			snapshots = result.items;
		} catch (e) {
			error = (e as ApiError).message || 'Failed to load snapshots';
			snapshots = [];
		} finally {
			syncCompareSelection();
			loading = false;
		}
	}

	function openCreate() {
		newName = '';
		newDescription = '';
		createError = null;
		createOpen = true;
	}

	async function handleCreate(event: Event) {
		event.preventDefault();
		if (!nameValid || creating) return;

		creating = true;
		createError = null;
		try {
			const description = newDescription.trim();
			const created = await api.createSnapshot({
				name: trimmedName,
				// Omit rather than send "": an empty description is absence, not a value.
				...(description ? { description } : {})
			});
			createOpen = false;
			announce(`Snapshot ${created.name} created.`);
			await loadSnapshots();
		} catch (e) {
			createError = (e as ApiError).message || 'Failed to create snapshot';
		} finally {
			creating = false;
		}
	}

	function openDelete(snapshot: Snapshot) {
		deleteTarget = snapshot;
		deleteError = null;
	}

	async function handleDelete(event: Event) {
		// Without this, the AlertDialog closes before the request settles and the
		// error never gets a chance to render.
		event.preventDefault();
		const target = deleteTarget;
		if (!target || deleting) return;

		deleting = true;
		deleteError = null;
		try {
			await api.deleteSnapshot(target.id);
			deleteTarget = null;
			announce(`Snapshot ${target.name} deleted.`);
			await loadSnapshots();
		} catch (e) {
			const apiError = e as ApiError;
			if (apiError.status === 404) {
				// Already gone (another tab, another user): the outcome the user asked
				// for. Refresh rather than show an error they cannot act on.
				deleteTarget = null;
				announce(`Snapshot ${target.name} was already deleted.`);
				await loadSnapshots();
			} else {
				deleteError = apiError.message || 'Failed to delete snapshot';
			}
		} finally {
			deleting = false;
		}
	}

	function handleCompare(event: Event) {
		event.preventDefault();
		if (!compareValid) return;
		goto(snapshotCompareHref(compareFrom, compareTo));
	}

	$effect(() => {
		loadSnapshots();
	});
</script>

<svelte:head>
	<title>Research Snapshots | My Family</title>
</svelte:head>

<div class="sr-only" role="status" aria-live="polite" aria-atomic="true" data-testid="announcer">
	{announcement}
</div>

<div class="snapshots-page">
	<MainlineNotice
		surface="Snapshots"
		detail="A snapshot marks a point in the mainline's history, and comparisons list mainline changes only. Branch edits appear once the branch is merged."
	/>

	<header class="page-header">
		<div>
			<h1>Research Snapshots</h1>
			<p class="description">
				Mark milestones in your research, like "Pre-DNA results" or "After courthouse trip", then
				compare two of them to see everything that changed in between. A snapshot is only a
				marker: creating or deleting one never changes your data.
			</p>
		</div>
		<Button onclick={openCreate}>New snapshot</Button>
	</header>

	{#if loading}
		<div class="state" role="status" aria-live="polite">Loading snapshots...</div>
	{:else if error}
		<div class="state error" role="alert">{error}</div>
	{:else if snapshots.length === 0}
		<div class="state empty">
			<h2>No snapshots yet</h2>
			<p>Create one to mark where your research stands today.</p>
		</div>
	{:else}
		{#if snapshots.length >= 2}
			<section class="compare-panel" aria-labelledby="compare-heading">
				<h2 id="compare-heading">Compare two snapshots</h2>
				<form class="compare-form" onsubmit={handleCompare}>
					<div class="compare-field">
						<Label for="compare-from">From</Label>
						<select id="compare-from" class="native-select" bind:value={compareFrom}>
							{#each snapshots as snapshot (snapshot.id)}
								<option value={snapshot.id}>{optionLabel(snapshot)}</option>
							{/each}
						</select>
					</div>
					<div class="compare-field">
						<Label for="compare-to">To</Label>
						<select id="compare-to" class="native-select" bind:value={compareTo}>
							{#each snapshots as snapshot (snapshot.id)}
								<option value={snapshot.id}>{optionLabel(snapshot)}</option>
							{/each}
						</select>
					</div>
					<Button type="submit" disabled={!compareValid} aria-describedby="compare-hint">
						Compare
					</Button>
				</form>
				<p id="compare-hint" class="compare-hint">
					{#if compareFrom !== '' && compareFrom === compareTo}
						Choose two different snapshots.
					{:else}
						Changes are listed oldest first, whichever order you pick.
					{/if}
				</p>
			</section>
		{/if}

		<ol class="snapshot-list" aria-label="Snapshots, newest first">
			{#each snapshots as snapshot, index (snapshot.id)}
				{@const previous = snapshots[index + 1]}
				<li class="snapshot-card">
					<div class="snapshot-head">
						<h2 class="snapshot-name">{snapshot.name}</h2>
						<div class="snapshot-actions">
							{#if previous}
								<Button
									variant="outline"
									size="sm"
									href={snapshotCompareHref(previous.id, snapshot.id)}
									aria-label="Compare {snapshot.name} with the previous snapshot, {previous.name}"
								>
									Compare with previous
								</Button>
							{/if}
							<Button
								variant="destructive"
								size="sm"
								onclick={() => openDelete(snapshot)}
								aria-label="Delete snapshot {snapshot.name}"
							>
								Delete
							</Button>
						</div>
					</div>

					{#if snapshot.description}
						<p class="snapshot-description">{snapshot.description}</p>
					{/if}

					<dl class="snapshot-meta">
						<div>
							<dt>Created</dt>
							<dd><time datetime={snapshot.created_at}>{formatTimestamp(snapshot.created_at)}</time></dd>
						</div>
						<div>
							<dt>Marks</dt>
							<dd>position {snapshot.position}</dd>
						</div>
					</dl>
				</li>
			{/each}
		</ol>
	{/if}
</div>

<Dialog.Root bind:open={createOpen}>
	<Dialog.Content class="sm:max-w-md">
		<Dialog.Header>
			<Dialog.Title>New snapshot</Dialog.Title>
			<Dialog.Description>
				Marks the current point in your research history. You can compare it with another snapshot
				later to see what changed.
			</Dialog.Description>
		</Dialog.Header>

		<form onsubmit={handleCreate}>
			<div class="field">
				<Label for="snapshot-name">Name</Label>
				<Input
					id="snapshot-name"
					bind:value={newName}
					maxlength={NAME_MAX_LENGTH}
					placeholder="Pre-DNA results"
					required
					aria-invalid={nameError ? 'true' : undefined}
					aria-describedby={nameError ? 'snapshot-name-error' : undefined}
				/>
				<div class="field-foot">
					{#if nameError}
						<span id="snapshot-name-error" class="field-error">{nameError}</span>
					{/if}
					<span class="field-hint">{newName.length}/{NAME_MAX_LENGTH}</span>
				</div>
			</div>

			<div class="field">
				<Label for="snapshot-description">Description (optional)</Label>
				<Textarea
					id="snapshot-description"
					bind:value={newDescription}
					maxlength={DESCRIPTION_MAX_LENGTH}
					rows={3}
					placeholder="Research state before DNA test results arrived"
				/>
				<span class="field-hint">{newDescription.length}/{DESCRIPTION_MAX_LENGTH}</span>
			</div>

			{#if createError}
				<div class="dialog-error" role="alert">{createError}</div>
			{/if}

			<Dialog.Footer>
				<Button
					type="button"
					variant="secondary"
					disabled={creating}
					onclick={() => (createOpen = false)}
				>
					Cancel
				</Button>
				<Button type="submit" disabled={creating || !nameValid}>
					{creating ? 'Creating...' : 'Create snapshot'}
				</Button>
			</Dialog.Footer>
		</form>
	</Dialog.Content>
</Dialog.Root>

<AlertDialog.Root
	open={deleteTarget !== null}
	onOpenChange={(isOpen) => {
		if (!isOpen && !deleting) deleteTarget = null;
	}}
>
	<AlertDialog.Content>
		<AlertDialog.Header>
			<AlertDialog.Title>Delete this snapshot?</AlertDialog.Title>
			<AlertDialog.Description>
				The snapshot {deleteTarget?.name} will be removed, so it can no longer be compared against.
				Only the marker is deleted: your research data and its change history are untouched.
			</AlertDialog.Description>
		</AlertDialog.Header>

		{#if deleteError}
			<div class="dialog-error" role="alert">{deleteError}</div>
		{/if}

		<AlertDialog.Footer>
			<AlertDialog.Cancel disabled={deleting}>Cancel</AlertDialog.Cancel>
			<AlertDialog.Action variant="destructive" disabled={deleting} onclick={handleDelete}>
				{deleting ? 'Deleting...' : 'Delete snapshot'}
			</AlertDialog.Action>
		</AlertDialog.Footer>
	</AlertDialog.Content>
</AlertDialog.Root>

<style>
	.snapshots-page {
		max-width: 1000px;
		margin: 0 auto;
		padding: 1.5rem;
	}

	.page-header {
		display: flex;
		align-items: flex-start;
		justify-content: space-between;
		gap: 1rem;
		flex-wrap: wrap;
		margin-bottom: 1.5rem;
	}

	.page-header h1 {
		margin: 0;
		font-size: 1.5rem;
		color: #1e293b;
	}

	.description {
		margin: 0.25rem 0 0;
		font-size: 0.875rem;
		color: #64748b;
		max-width: 46rem;
	}

	.compare-panel {
		background: white;
		border: 1px solid #e2e8f0;
		border-radius: 8px;
		padding: 1rem;
		margin-bottom: 1.5rem;
	}

	.compare-panel h2 {
		margin: 0 0 0.75rem;
		font-size: 0.8125rem;
		font-weight: 600;
		text-transform: uppercase;
		letter-spacing: 0.05em;
		color: #64748b;
	}

	.compare-form {
		display: flex;
		align-items: flex-end;
		gap: 0.75rem;
		flex-wrap: wrap;
	}

	.compare-field {
		display: flex;
		flex-direction: column;
		gap: 0.375rem;
		flex: 1 1 12rem;
		min-width: 0;
	}

	.native-select {
		height: 2.25rem;
		padding: 0 0.5rem;
		border: 1px solid #cbd5e1;
		border-radius: 6px;
		background: white;
		font-size: 0.875rem;
		color: #1e293b;
		max-width: 100%;
	}

	.native-select:focus-visible {
		outline: 2px solid #3b82f6;
		outline-offset: 1px;
	}

	.compare-hint {
		margin: 0.5rem 0 0;
		font-size: 0.75rem;
		color: #64748b;
	}

	.snapshot-list {
		list-style: none;
		margin: 0;
		padding: 0;
	}

	.snapshot-card {
		background: white;
		border: 1px solid #e2e8f0;
		border-radius: 8px;
		padding: 1rem;
	}

	.snapshot-card + .snapshot-card {
		margin-top: 0.75rem;
	}

	.snapshot-head {
		display: flex;
		align-items: flex-start;
		justify-content: space-between;
		gap: 0.75rem;
		flex-wrap: wrap;
	}

	.snapshot-name {
		margin: 0;
		font-size: 1rem;
		font-weight: 600;
		color: #1e293b;
		overflow-wrap: anywhere;
	}

	.snapshot-actions {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		flex-wrap: wrap;
	}

	.snapshot-description {
		margin: 0.5rem 0 0;
		font-size: 0.875rem;
		color: #475569;
		overflow-wrap: anywhere;
	}

	.snapshot-meta {
		display: flex;
		gap: 1.5rem;
		flex-wrap: wrap;
		margin: 0.75rem 0 0;
	}

	.snapshot-meta div {
		display: flex;
		flex-direction: column;
		gap: 0.125rem;
	}

	.snapshot-meta dt {
		font-size: 0.6875rem;
		text-transform: uppercase;
		letter-spacing: 0.04em;
		color: #64748b;
	}

	.snapshot-meta dd {
		margin: 0;
		font-size: 0.8125rem;
		color: #475569;
	}

	.state {
		padding: 2rem;
		text-align: center;
		color: #64748b;
	}

	.state.error {
		color: #dc2626;
	}

	.state.empty {
		background: white;
		border: 1px dashed #cbd5e1;
		border-radius: 8px;
	}

	.state.empty h2 {
		margin: 0 0 0.375rem;
		font-size: 1rem;
		color: #1e293b;
	}

	.state.empty p {
		margin: 0;
		font-size: 0.875rem;
	}

	.field {
		display: flex;
		flex-direction: column;
		gap: 0.375rem;
		margin-bottom: 1rem;
	}

	.field-foot {
		display: flex;
		justify-content: space-between;
		gap: 0.5rem;
	}

	.field-error {
		font-size: 0.75rem;
		color: #dc2626;
	}

	.field-hint {
		margin-left: auto;
		font-size: 0.6875rem;
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
