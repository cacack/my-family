<script lang="ts">
	import { api, type ApiError, type Branch, type BranchOutcome } from '$lib/api/client';
	import BranchOutcomeBadge from '$lib/components/branch/BranchOutcomeBadge.svelte';
	import CloseBranchDialog, {
		type BranchCloseResult
	} from '$lib/components/branch/CloseBranchDialog.svelte';
	import {
		BRANCH_OUTCOMES,
		HYPOTHESIS_MAX_LENGTH,
		OUTCOME_LABELS,
		branchOutcome,
		promotionSummary
	} from '$lib/utils/branchResearch';
	import { activeBranch, switchBranch } from '$lib/stores/activeBranch.svelte';
	import { Button } from '$lib/components/ui/button';
	import { Badge } from '$lib/components/ui/badge';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { Textarea } from '$lib/components/ui/textarea';
	import * as Dialog from '$lib/components/ui/dialog';
	import { branchMergeSummary, isIncompleteMerge } from '$lib/utils/mergeState';

	// Mirrors the maxLength on BranchCreate in openapi.yaml.
	const NAME_MAX_LENGTH = 100;
	const DESCRIPTION_MAX_LENGTH = 500;

	let branches: Branch[] = $state([]);
	let loading = $state(true);
	let error: string | null = $state(null);
	/** Set when the server has no branch registry configured (503). */
	let unavailable = $state(false);

	// Create dialog
	let createOpen = $state(false);
	let creating = $state(false);
	let createError: string | null = $state(null);
	let newName = $state('');
	let newDescription = $state('');
	let newHypothesis = $state('');
	let newOutcome: BranchOutcome = $state('open');

	// Close dialog (#836)
	let closeTarget: Branch | null = $state(null);
	/** What the last close did, shown above the list until the next action. */
	let closeNotice: { branchId: string; text: string; warning: string | null } | null =
		$state(null);

	// Filters: lifecycle status and outcome. "all" shows every branch.
	type StatusFilter = 'all' | Branch['status'];
	const STATUS_FILTERS: { value: StatusFilter; label: string }[] = [
		{ value: 'all', label: 'All' },
		{ value: 'active', label: 'Active' },
		{ value: 'merged', label: 'Merged' },
		{ value: 'archived', label: 'Closed' }
	];
	let statusFilter: StatusFilter = $state('all');
	let outcomeFilter: BranchOutcome | 'all' = $state('all');

	const visibleBranches = $derived(
		branches.filter(
			(b) =>
				(statusFilter === 'all' || b.status === statusFilter) &&
				(outcomeFilter === 'all' || branchOutcome(b.outcome) === outcomeFilter)
		)
	);
	const activeBranches = $derived(visibleBranches.filter((b) => b.status === 'active'));
	const mergedBranches = $derived(visibleBranches.filter((b) => b.status === 'merged'));
	const closedBranches = $derived(visibleBranches.filter((b) => b.status === 'archived'));
	// Both halves measure the trimmed name, because the trimmed name is what
	// `handleCreate` actually sends: trailing whitespace must not cost the user
	// a significant character.
	const nameValid = $derived(
		newName.trim().length > 0 && newName.trim().length <= NAME_MAX_LENGTH
	);

	function formatTimestamp(iso: string): string {
		return new Date(iso).toLocaleDateString('en-US', {
			month: 'short',
			day: 'numeric',
			year: 'numeric',
			hour: 'numeric',
			minute: '2-digit'
		});
	}

	async function loadBranches() {
		loading = true;
		error = null;
		unavailable = false;
		try {
			const result = await api.listBranches();
			branches = result.items;
		} catch (e) {
			const apiError = e as ApiError;
			if (apiError.status === 503) {
				unavailable = true;
			} else {
				error = apiError.message || 'Failed to load branches';
			}
			branches = [];
		} finally {
			loading = false;
		}
	}

	function openCreate() {
		newName = '';
		newDescription = '';
		newHypothesis = '';
		newOutcome = 'open';
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
			const hypothesis = newHypothesis.trim();
			await api.createBranch({
				name: newName.trim(),
				// Omit rather than send "": an empty description is absence, not a value.
				...(description ? { description } : {}),
				...(hypothesis ? { hypothesis } : {}),
				// `open` is the server's default, so only a different verdict is sent.
				...(newOutcome !== 'open' ? { outcome: newOutcome } : {})
			});
			createOpen = false;
			await loadBranches();
		} catch (e) {
			createError = (e as ApiError).message || 'Failed to create branch';
		} finally {
			creating = false;
		}
	}

	function openClose(branch: Branch) {
		closeNotice = null;
		closeTarget = branch;
	}

	async function handleClosed(result: BranchCloseResult) {
		const closed = result.branch;
		closeTarget = null;
		// Closing the branch we are standing on would leave every scoped read
		// pointing at purged overlay rows. This reloads the page.
		if (activeBranch.id === closed.id) {
			switchBranch(null);
		}
		const outcome = OUTCOME_LABELS[branchOutcome(closed.outcome)].toLowerCase();
		const copied = result.promotion ? ` ${promotionSummary(result.promotion)}` : '';
		closeNotice = {
			branchId: closed.id,
			text: `Closed "${closed.name}" as ${outcome}.${copied}`,
			warning: result.promotionError
		};
		await loadBranches();
	}

	/** The status as a person reads it: an archived branch was closed. */
	function statusLabel(status: Branch['status']): string {
		return status === 'archived' ? 'closed' : status;
	}

	function statusVariant(status: Branch['status']): 'secondary' | 'outline' | undefined {
		return status === 'active' ? undefined : status === 'merged' ? 'secondary' : 'outline';
	}

	$effect(() => {
		loadBranches();
	});
</script>

<svelte:head>
	<title>Research Branches | My Family</title>
</svelte:head>

{#snippet branchCard(branch: Branch)}
	<article class="branch-card">
		<div class="branch-head">
			<div class="branch-title">
				<a href="/branches/{branch.id}" class="branch-name">{branch.name}</a>
				<Badge variant={statusVariant(branch.status)} class="capitalize">
					{statusLabel(branch.status)}
				</Badge>
				<BranchOutcomeBadge outcome={branch.outcome} />
				{#if activeBranch.id === branch.id}
					<Badge class="bg-violet-100 text-violet-800">Current</Badge>
				{/if}
				{#if isIncompleteMerge(branch)}
					<Badge variant="outline" class="border-orange-500 text-orange-800">Merge unfinished</Badge>
				{/if}
			</div>
			<div class="branch-actions">
				{#if branch.status === 'active'}
					{#if activeBranch.id === branch.id}
						<Button variant="outline" size="sm" onclick={() => switchBranch(null)}>
							Return to mainline
						</Button>
					{:else}
						<Button variant="outline" size="sm" onclick={() => switchBranch(branch)}>
							Switch to branch
						</Button>
					{/if}
					<Button variant="destructive" size="sm" onclick={() => openClose(branch)}>
						Close
					</Button>
				{/if}
				{#if branch.status === 'archived'}
					<Button variant="outline" size="sm" href="/branches/{branch.id}/research">
						Research
					</Button>
				{/if}
				{#if isIncompleteMerge(branch)}
					<Button size="sm" href="/branches/{branch.id}">Finish merge</Button>
				{/if}
				<Button variant="ghost" size="sm" href="/branches/{branch.id}">Compare</Button>
			</div>
		</div>

		{#if branch.hypothesis}
			<p class="branch-hypothesis" data-testid="card-hypothesis">
				<span class="hypothesis-label">Question</span>
				{branch.hypothesis}
			</p>
		{/if}

		{#if branch.description}
			<p class="branch-description">{branch.description}</p>
		{/if}

		<dl class="branch-meta">
			<div>
				<dt>Forked at</dt>
				<dd>position {branch.base_position}</dd>
			</div>
			<div>
				<dt>Created</dt>
				<dd>{formatTimestamp(branch.created_at)}</dd>
			</div>
			{#if (branch.subjects ?? []).length > 0}
				<div>
					<dt>Subjects</dt>
					<dd>{branch.subjects.length}</dd>
				</div>
			{/if}
			{#if branch.merged_at}
				<div>
					<dt>Merged</dt>
					<dd>{formatTimestamp(branch.merged_at)}</dd>
				</div>
			{/if}
			{#if branch.closed_at}
				<div>
					<dt>Closed</dt>
					<dd>{formatTimestamp(branch.closed_at)}</dd>
				</div>
			{/if}
		</dl>

		{#if isIncompleteMerge(branch)}
			<p class="merge-unfinished" role="note">
				Its merge did not finish. {branchMergeSummary(branch)}
			</p>
		{/if}

		{#if branch.close_reason}
			<p class="merge-note" data-testid="close-reason">
				<span class="merge-note-label">Why it was closed</span>
				{branch.close_reason}
			</p>
		{/if}

		{#if branch.merge_note}
			<p class="merge-note"><span class="merge-note-label">Merge note</span> {branch.merge_note}</p>
		{/if}
	</article>
{/snippet}

<div class="branches-page">
	<header class="page-header">
		<div>
			<h1>Research Branches</h1>
			<p class="description">
				Explore a line of research in isolation, then compare it against the mainline. Branch
				scoping covers people, families and pedigrees.
			</p>
		</div>
		{#if !unavailable}
			<Button onclick={openCreate}>New branch</Button>
		{/if}
	</header>

	{#if loading}
		<div class="state" role="status" aria-live="polite">Loading branches...</div>
	{:else if unavailable}
		<div class="state empty">
			<h2>Branches are not configured</h2>
			<p>
				The branch registry is not configured on this server, so research branches are
				unavailable.
			</p>
		</div>
	{:else if error}
		<div class="state error" role="alert">{error}</div>
	{:else if branches.length === 0}
		<div class="state empty">
			<h2>No research branches yet</h2>
			<p>Create one to explore a speculative line without touching the mainline.</p>
		</div>
	{:else}
		{#if closeNotice}
			<div class="notice" role="status">
				<p>
					{closeNotice.text}
					<a href="/branches/{closeNotice.branchId}/research">View its research</a>
				</p>
				{#if closeNotice.warning}
					<p class="notice-warning">
						{closeNotice.warning} You can copy them from the branch's research page.
					</p>
				{/if}
			</div>
		{/if}

		<div class="filters" role="group" aria-label="Filter branches">
			<div class="status-filter" role="radiogroup" aria-label="Status">
				{#each STATUS_FILTERS as option (option.value)}
					<Button
						variant={statusFilter === option.value ? 'default' : 'outline'}
						size="sm"
						role="radio"
						aria-checked={statusFilter === option.value}
						onclick={() => (statusFilter = option.value)}
					>
						{option.label}
					</Button>
				{/each}
			</div>
			<div class="outcome-filter">
				<span aria-hidden="true">Outcome</span>
				<select class="native-select" bind:value={outcomeFilter} aria-label="Filter by outcome">
					<option value="all">Any outcome</option>
					{#each BRANCH_OUTCOMES as option (option)}
						<option value={option}>{OUTCOME_LABELS[option]}</option>
					{/each}
				</select>
			</div>
		</div>

		{#if visibleBranches.length === 0}
			<div class="state empty">
				<p>No branches match these filters.</p>
			</div>
		{/if}
		{#if activeBranches.length > 0}
			<section class="branch-group">
				<h2>Active</h2>
				{#each activeBranches as branch (branch.id)}
					{@render branchCard(branch)}
				{/each}
			</section>
		{/if}
		{#if mergedBranches.length > 0}
			<section class="branch-group">
				<h2>Merged</h2>
				{#each mergedBranches as branch (branch.id)}
					{@render branchCard(branch)}
				{/each}
			</section>
		{/if}
		{#if closedBranches.length > 0}
			<section class="branch-group">
				<h2>Closed</h2>
				{#each closedBranches as branch (branch.id)}
					{@render branchCard(branch)}
				{/each}
			</section>
		{/if}
	{/if}
</div>

<Dialog.Root bind:open={createOpen}>
	<Dialog.Content class="sm:max-w-md">
		<Dialog.Header>
			<Dialog.Title>New research branch</Dialog.Title>
			<Dialog.Description>
				The branch forks from the current mainline position. Changes made on it stay isolated
				until it is merged.
			</Dialog.Description>
		</Dialog.Header>

		<form onsubmit={handleCreate}>
			<div class="field">
				<Label for="branch-name">Name</Label>
				<Input
					id="branch-name"
					bind:value={newName}
					maxlength={NAME_MAX_LENGTH}
					placeholder="Maternal Smith line"
					required
				/>
				<span class="field-hint">{newName.length}/{NAME_MAX_LENGTH}</span>
			</div>

			<div class="field">
				<Label for="branch-hypothesis">Research question (optional)</Label>
				<Textarea
					id="branch-hypothesis"
					bind:value={newHypothesis}
					maxlength={HYPOTHESIS_MAX_LENGTH}
					rows={3}
					placeholder="Was Mary Smith (b. 1842) the daughter of John Smith of Albany?"
				/>
				<span class="field-hint">{newHypothesis.length}/{HYPOTHESIS_MAX_LENGTH}</span>
			</div>

			<div class="field">
				<Label for="branch-outcome">Outcome</Label>
				<select id="branch-outcome" class="native-select" bind:value={newOutcome}>
					{#each BRANCH_OUTCOMES as option (option)}
						<option value={option}>{OUTCOME_LABELS[option]}</option>
					{/each}
				</select>
			</div>

			<div class="field">
				<Label for="branch-description">Description (optional)</Label>
				<Textarea
					id="branch-description"
					bind:value={newDescription}
					maxlength={DESCRIPTION_MAX_LENGTH}
					rows={3}
					placeholder="What this branch explores"
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
					{creating ? 'Creating...' : 'Create branch'}
				</Button>
			</Dialog.Footer>
		</form>
	</Dialog.Content>
</Dialog.Root>

<CloseBranchDialog
	branch={closeTarget}
	onclosed={handleClosed}
	oncancel={() => (closeTarget = null)}
/>

<style>
	.merge-unfinished {
		margin: 0.5rem 0 0;
		padding: 0.5rem 0.75rem;
		background: #fff7ed;
		border: 1px solid #fb923c;
		border-radius: 6px;
		color: #7c2d12;
		font-size: 0.8125rem;
	}

	.branches-page {
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

	.branch-group {
		margin-bottom: 2rem;
	}

	.branch-group h2 {
		margin: 0 0 0.75rem;
		font-size: 0.8125rem;
		font-weight: 600;
		text-transform: uppercase;
		letter-spacing: 0.05em;
		color: #64748b;
	}

	.branch-card {
		background: white;
		border: 1px solid #e2e8f0;
		border-radius: 8px;
		padding: 1rem;
	}

	.branch-card + .branch-card {
		margin-top: 0.75rem;
	}

	.branch-head {
		display: flex;
		align-items: flex-start;
		justify-content: space-between;
		gap: 0.75rem;
		flex-wrap: wrap;
	}

	.branch-title {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		flex-wrap: wrap;
	}

	.branch-name {
		font-size: 1rem;
		font-weight: 600;
		color: #1e293b;
		text-decoration: none;
	}

	.branch-name:hover {
		color: #3b82f6;
	}

	.branch-actions {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		flex-wrap: wrap;
	}

	.branch-description {
		margin: 0.5rem 0 0;
		font-size: 0.875rem;
		color: #475569;
	}

	.branch-hypothesis {
		margin: 0.5rem 0 0;
		font-size: 0.875rem;
		color: #1e293b;
	}

	.hypothesis-label {
		margin-right: 0.375rem;
		font-size: 0.6875rem;
		font-weight: 600;
		text-transform: uppercase;
		letter-spacing: 0.04em;
		color: #94a3b8;
	}

	.native-select {
		height: 2.25rem;
		padding: 0 0.5rem;
		border: 1px solid #cbd5e1;
		border-radius: 6px;
		background: white;
		font-size: 0.875rem;
	}

	.branch-meta {
		display: flex;
		gap: 1.5rem;
		flex-wrap: wrap;
		margin: 0.75rem 0 0;
	}

	.branch-meta div {
		display: flex;
		flex-direction: column;
		gap: 0.125rem;
	}

	.branch-meta dt {
		font-size: 0.6875rem;
		text-transform: uppercase;
		letter-spacing: 0.04em;
		color: #94a3b8;
	}

	.branch-meta dd {
		margin: 0;
		font-size: 0.8125rem;
		color: #475569;
	}

	.merge-note {
		margin: 0.75rem 0 0;
		padding-top: 0.75rem;
		border-top: 1px solid #f1f5f9;
		font-size: 0.8125rem;
		color: #475569;
	}

	.merge-note-label {
		font-weight: 600;
		color: #64748b;
	}

	.filters {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 0.75rem;
		flex-wrap: wrap;
		margin-bottom: 1.25rem;
	}

	.status-filter {
		display: flex;
		gap: 0.375rem;
		flex-wrap: wrap;
	}

	.outcome-filter {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		font-size: 0.8125rem;
		color: #64748b;
	}

	.notice {
		margin-bottom: 1rem;
		padding: 0.75rem 1rem;
		background: #f0fdf4;
		border: 1px solid #bbf7d0;
		border-radius: 8px;
		font-size: 0.875rem;
		color: #166534;
	}

	.notice p {
		margin: 0;
	}

	.notice a {
		margin-left: 0.25rem;
		font-weight: 500;
		text-decoration: underline;
	}

	.notice-warning {
		margin-top: 0.375rem !important;
		color: #92400e;
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

	.field-hint {
		align-self: flex-end;
		font-size: 0.6875rem;
		color: #94a3b8;
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
