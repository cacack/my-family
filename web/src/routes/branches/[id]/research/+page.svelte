<script lang="ts">
	/**
	 * A branch's research record, read-only (#836): the research logs -
	 * including the searches that found nothing - evidence analyses and proof
	 * summaries the branch recorded. The server rebuilds them from the branch's
	 * events, so this works for a closed branch whose isolated view is gone.
	 *
	 * A closed branch's research logs can be copied to the mainline from here,
	 * as the close dialog offers; copying again is safe.
	 */
	import { page } from '$app/stores';
	import {
		api,
		type ApiError,
		type Branch,
		type BranchResearchArchive,
		type PromoteResearchLogsResult
	} from '$lib/api/client';
	import BranchOutcomeBadge from '$lib/components/branch/BranchOutcomeBadge.svelte';
	import { promotionSummary } from '$lib/utils/branchResearch';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';

	const LOG_OUTCOME_LABELS: Record<string, string> = {
		found: 'Found',
		not_found: 'Not found',
		inconclusive: 'Inconclusive'
	};

	let branch: Branch | null = $state(null);
	let archive: BranchResearchArchive | null = $state(null);
	let loading = $state(true);
	let error: string | null = $state(null);
	let notFound = $state(false);

	let promoting = $state(false);
	let promotion: PromoteResearchLogsResult | null = $state(null);
	let promoteError: string | null = $state(null);

	const branchId = $derived($page.params.id ?? '');
	const closed = $derived.by(() => branch?.status === 'archived');
	const promotable = $derived.by(
		() => closed && (archive?.research_logs ?? []).some((entry) => entry.created_on_branch)
	);

	// Orders overlapping loads after a soft navigation between branches.
	let loadRequest = 0;

	async function load(id: string) {
		const request = ++loadRequest;
		loading = true;
		error = null;
		notFound = false;
		promotion = null;
		promoteError = null;
		try {
			const [b, a] = await Promise.all([api.getBranch(id), api.getBranchResearch(id)]);
			if (request !== loadRequest) return;
			branch = b;
			archive = a;
		} catch (e) {
			if (request !== loadRequest) return;
			const apiError = e as ApiError;
			if (apiError.status === 404) {
				notFound = true;
			} else {
				error = apiError.message || "Failed to load the branch's research";
			}
			branch = null;
			archive = null;
		} finally {
			if (request === loadRequest) loading = false;
		}
	}

	$effect(() => {
		const id = branchId;
		if (id) load(id);
	});

	async function handlePromote() {
		if (!branch || promoting) return;
		promoting = true;
		promoteError = null;
		try {
			promotion = await api.promoteBranchResearchLogs(branch.id);
		} catch (e) {
			promoteError = (e as ApiError).message || 'Failed to copy the research logs';
		} finally {
			promoting = false;
		}
	}

	function formatDate(iso: string | undefined): string {
		if (!iso) return '';
		return new Date(iso).toLocaleDateString('en-US', {
			month: 'short',
			day: 'numeric',
			year: 'numeric',
			timeZone: 'UTC'
		});
	}

	function factLabel(fact: string): string {
		return fact.replace(/_/g, ' ');
	}
</script>

<svelte:head>
	<title>{branch ? `${branch.name} research | Branches` : 'Branch research'} | My Family</title>
</svelte:head>

{#snippet provenance(createdOnBranch: boolean)}
	<Badge variant="outline">{createdOnBranch ? 'Recorded on this branch' : 'Mainline entry, edited'}</Badge>
{/snippet}

<div class="research-page">
	<a href={branchId ? `/branches/${branchId}` : '/branches'} class="back-link">&larr; Branch</a>

	{#if loading}
		<div class="state" role="status" aria-live="polite">Loading research...</div>
	{:else if notFound}
		<div class="state empty">
			<h2>Branch not found</h2>
			<p>It may not exist, or the branch registry is not configured on this server.</p>
		</div>
	{:else if error}
		<div class="state error" role="alert">{error}</div>
	{:else if branch && archive}
		<header class="page-header">
			<div class="title-row">
				<h1>{branch.name}</h1>
				<Badge variant="secondary" class="capitalize">
					{branch.status === 'archived' ? 'closed' : branch.status}
				</Badge>
				<BranchOutcomeBadge outcome={branch.outcome} />
			</div>
			<p class="lede">
				The research this branch recorded, as it left it. Read-only: rebuilt from the branch's
				history{closed ? ', since a closed branch has no view of its own' : ''}.
			</p>
			{#if branch.hypothesis}
				<p class="record"><span class="label">Question</span> {branch.hypothesis}</p>
			{/if}
			{#if branch.closed_at}
				<p class="record" data-testid="closed-on">
					<span class="label">Closed</span>
					{formatDate(branch.closed_at)}
				</p>
			{/if}
			{#if branch.close_reason}
				<p class="record" data-testid="research-close-reason">
					<span class="label">Why it was closed</span>
					{branch.close_reason}
				</p>
			{/if}
		</header>

		{#if archive.truncated}
			<div class="note" role="note">
				This branch has more history than can be read at once, so some research may be missing
				below.
			</div>
		{/if}

		<section aria-labelledby="logs-heading">
			<div class="section-head">
				<h2 id="logs-heading">Research log</h2>
				{#if promotable}
					<Button size="sm" variant="outline" disabled={promoting} onclick={handlePromote}>
						{promoting ? 'Copying...' : 'Copy research logs to the mainline'}
					</Button>
				{/if}
			</div>
			{#if promotion}
				<p class="notice" role="status">{promotionSummary(promotion)}</p>
			{/if}
			{#if promoteError}
				<p class="state error" role="alert">{promoteError}</p>
			{/if}
			{#if archive.research_logs.length === 0}
				<p class="empty-line">This branch recorded no searches.</p>
			{:else}
				<ol class="entries">
					{#each archive.research_logs as entry (entry.log.id)}
						<li class="entry" data-testid="archived-research-log">
							<div class="entry-head">
								<span class="entry-title">{entry.log.search_description}</span>
								<span
									class="log-outcome log-outcome-{entry.log.outcome}"
									data-testid="log-outcome"
								>
									{LOG_OUTCOME_LABELS[entry.log.outcome] ?? entry.log.outcome}
								</span>
								{@render provenance(entry.created_on_branch)}
							</div>
							<dl class="entry-meta">
								<div>
									<dt>Repository</dt>
									<dd>{entry.log.repository}</dd>
								</div>
								<div>
									<dt>Searched</dt>
									<dd>{formatDate(entry.log.search_date)}</dd>
								</div>
								<div>
									<dt>About</dt>
									<dd>{entry.subject_name || `Unnamed ${entry.log.subject_type}`}</dd>
								</div>
							</dl>
							{#if entry.log.notes}
								<p class="entry-notes">{entry.log.notes}</p>
							{/if}
						</li>
					{/each}
				</ol>
			{/if}
		</section>

		<section aria-labelledby="analyses-heading">
			<h2 id="analyses-heading">Evidence analyses</h2>
			{#if archive.evidence_analyses.length === 0}
				<p class="empty-line">This branch recorded no evidence analyses.</p>
			{:else}
				<ol class="entries">
					{#each archive.evidence_analyses as entry (entry.analysis.id)}
						<li class="entry" data-testid="archived-analysis">
							<div class="entry-head">
								<span class="entry-title">{entry.analysis.conclusion}</span>
								{@render provenance(entry.created_on_branch)}
							</div>
							<dl class="entry-meta">
								<div>
									<dt>Fact</dt>
									<dd class="capitalize">{factLabel(entry.analysis.fact_type)}</dd>
								</div>
								<div>
									<dt>About</dt>
									<dd>{entry.subject_name || 'Unnamed subject'}</dd>
								</div>
								{#if entry.analysis.research_status}
									<div>
										<dt>Status</dt>
										<dd class="capitalize">{entry.analysis.research_status}</dd>
									</div>
								{/if}
							</dl>
							{#if entry.analysis.notes}
								<p class="entry-notes">{entry.analysis.notes}</p>
							{/if}
						</li>
					{/each}
				</ol>
			{/if}
		</section>

		<section aria-labelledby="proofs-heading">
			<h2 id="proofs-heading">Proof summaries</h2>
			{#if archive.proof_summaries.length === 0}
				<p class="empty-line">This branch recorded no proof summaries.</p>
			{:else}
				<ol class="entries">
					{#each archive.proof_summaries as entry (entry.summary.id)}
						<li class="entry" data-testid="archived-proof-summary">
							<div class="entry-head">
								<span class="entry-title">{entry.summary.conclusion}</span>
								{@render provenance(entry.created_on_branch)}
							</div>
							<dl class="entry-meta">
								<div>
									<dt>Fact</dt>
									<dd class="capitalize">{factLabel(entry.summary.fact_type)}</dd>
								</div>
								<div>
									<dt>About</dt>
									<dd>{entry.subject_name || 'Unnamed subject'}</dd>
								</div>
							</dl>
							<p class="entry-notes">{entry.summary.argument}</p>
						</li>
					{/each}
				</ol>
			{/if}
		</section>

		{#if archive.deleted_count > 0}
			<p class="empty-line">
				The branch also deleted {archive.deleted_count} research
				{archive.deleted_count === 1 ? 'entry' : 'entries'}, not shown.
			</p>
		{/if}
	{/if}
</div>

<style>
	.research-page {
		max-width: 1000px;
		margin: 0 auto;
		padding: 1.5rem;
	}

	.back-link {
		display: inline-block;
		margin-bottom: 1rem;
		font-size: 0.875rem;
		color: #64748b;
		text-decoration: none;
	}

	.back-link:hover {
		color: #3b82f6;
	}

	.page-header {
		margin-bottom: 1.5rem;
	}

	.title-row {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		flex-wrap: wrap;
	}

	h1 {
		margin: 0;
		font-size: 1.5rem;
		color: #1e293b;
	}

	.lede {
		margin: 0.375rem 0 0.75rem;
		font-size: 0.875rem;
		color: #64748b;
	}

	.record {
		margin: 0.25rem 0 0;
		font-size: 0.875rem;
		color: #1e293b;
	}

	.label {
		margin-right: 0.375rem;
		font-size: 0.6875rem;
		font-weight: 600;
		text-transform: uppercase;
		letter-spacing: 0.04em;
		color: #94a3b8;
	}

	section {
		margin-bottom: 2rem;
	}

	.section-head {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 0.75rem;
		flex-wrap: wrap;
	}

	h2 {
		margin: 0 0 0.75rem;
		font-size: 0.8125rem;
		font-weight: 600;
		text-transform: uppercase;
		letter-spacing: 0.05em;
		color: #64748b;
	}

	.entries {
		list-style: none;
		margin: 0;
		padding: 0;
		display: flex;
		flex-direction: column;
		gap: 0.75rem;
	}

	.entry {
		background: white;
		border: 1px solid #e2e8f0;
		border-radius: 8px;
		padding: 0.875rem 1rem;
	}

	.entry-head {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		flex-wrap: wrap;
	}

	.entry-title {
		font-weight: 600;
		color: #1e293b;
	}

	.entry-meta {
		display: flex;
		gap: 1.5rem;
		flex-wrap: wrap;
		margin: 0.625rem 0 0;
	}

	.entry-meta div {
		display: flex;
		flex-direction: column;
		gap: 0.125rem;
	}

	.entry-meta dt {
		font-size: 0.6875rem;
		text-transform: uppercase;
		letter-spacing: 0.04em;
		color: #94a3b8;
	}

	.entry-meta dd {
		margin: 0;
		font-size: 0.8125rem;
		color: #475569;
	}

	.entry-notes {
		margin: 0.625rem 0 0;
		font-size: 0.8125rem;
		color: #475569;
		white-space: pre-line;
	}

	.log-outcome {
		display: inline-flex;
		align-items: center;
		height: 1.25rem;
		padding: 0 0.5rem;
		border-radius: 9999px;
		font-size: 0.75rem;
		font-weight: 500;
		background: #f1f5f9;
		color: #334155;
	}

	.log-outcome-not_found {
		background: #fee2e2;
		color: #991b1b;
	}

	.log-outcome-found {
		background: #dcfce7;
		color: #166534;
	}

	.log-outcome-inconclusive {
		background: #fef3c7;
		color: #92400e;
	}

	.capitalize {
		text-transform: capitalize;
	}

	.note,
	.notice {
		margin: 0 0 1rem;
		padding: 0.75rem 1rem;
		border-radius: 8px;
		font-size: 0.875rem;
	}

	.note {
		background: #fffbeb;
		border: 1px solid #fde68a;
		color: #92400e;
	}

	.notice {
		background: #f0fdf4;
		border: 1px solid #bbf7d0;
		color: #166534;
	}

	.empty-line {
		margin: 0;
		font-size: 0.875rem;
		color: #64748b;
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
		color: #1e293b;
		text-transform: none;
		letter-spacing: normal;
		font-size: 1rem;
	}
</style>
