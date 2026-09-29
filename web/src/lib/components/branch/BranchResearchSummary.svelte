<script lang="ts">
	/**
	 * Read-only view of a branch's research record (#835): the question, the
	 * verdict, the persons/families it concerns and the proof summaries that
	 * argue it. Subjects link to their pages and proof summaries to theirs.
	 *
	 * Display names come from the single-branch reads (`getBranch`,
	 * `compareBranch`, `updateBranch`). A subject the server could not name no
	 * longer exists where the branch can see it; it is still listed, by type,
	 * so the record stays honest about what was linked.
	 */
	import type { Branch } from '$lib/api/client';
	import BranchOutcomeBadge from './BranchOutcomeBadge.svelte';
	import { proofSummaryHref, subjectHref, subjectLabel } from '$lib/utils/branchResearch';

	interface Props {
		branch: Branch;
		/**
		 * Shown under the links when they would open in a different scope than
		 * this branch's - the pages they lead to read the ACTIVE branch (or the
		 * mainline), so a branch-only subject is missing there.
		 */
		scopeNote?: string | null;
	}

	let { branch, scopeNote = null }: Props = $props();

	const subjects = $derived(branch.subjects ?? []);
	const proofIds = $derived(branch.proof_summary_ids ?? []);
	const proofRefs = $derived(new Map((branch.proof_summaries ?? []).map((ref) => [ref.id, ref])));
</script>

<section class="research" aria-labelledby="research-heading-{branch.id}" data-testid="branch-research">
	<div class="research-head">
		<h2 id="research-heading-{branch.id}">Research question</h2>
		<BranchOutcomeBadge outcome={branch.outcome} />
	</div>

	{#if branch.hypothesis}
		<p class="hypothesis" data-testid="branch-hypothesis">{branch.hypothesis}</p>
	{:else}
		<p class="empty">No research question recorded yet.</p>
	{/if}

	<dl class="research-meta">
		<div>
			<dt>Subjects</dt>
			<dd>
				{#if subjects.length === 0}
					<span class="empty">None linked</span>
				{:else}
					<ul class="chips">
						{#each subjects as subject (subject.type + subject.id)}
							<li>
								<span class="chip-type">{subject.type}</span>
								<a href={subjectHref(subject)} class:unresolved={!subject.name}>
									{subjectLabel(subject)}
								</a>
							</li>
						{/each}
					</ul>
				{/if}
			</dd>
		</div>
		<div>
			<dt>Proof summaries</dt>
			<dd>
				{#if proofIds.length === 0}
					<span class="empty">None linked</span>
				{:else}
					<ul class="proofs">
						{#each proofIds as id (id)}
							{@const ref = proofRefs.get(id)}
							<li>
								{#if ref}
									<a href={proofSummaryHref(id)}>{ref.conclusion}</a>
									<span class="fact-type">{ref.fact_type.replace(/_/g, ' ')}</span>
								{:else if branch.proof_summaries}
									<!-- The server resolved the list and this one was not in it. -->
									<span class="unresolved">Proof summary no longer available</span>
								{:else}
									<a href={proofSummaryHref(id)}>View proof summary</a>
								{/if}
							</li>
						{/each}
					</ul>
				{/if}
			</dd>
		</div>
	</dl>
	{#if scopeNote && (subjects.length > 0 || proofIds.length > 0)}
		<p class="scope-note" data-testid="branch-research-scope-note">{scopeNote}</p>
	{/if}
</section>

<style>
	.scope-note {
		margin: 0.5rem 0 0;
		font-size: 0.75rem;
		color: #64748b;
	}

	.research {
		margin-top: 0.75rem;
		padding: 0.75rem 1rem;
		background: #f8fafc;
		border: 1px solid #e2e8f0;
		border-radius: 8px;
	}

	.research-head {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		flex-wrap: wrap;
	}

	.research-head h2 {
		margin: 0;
		font-size: 0.8125rem;
		font-weight: 600;
		text-transform: uppercase;
		letter-spacing: 0.04em;
		color: #64748b;
	}

	.hypothesis {
		margin: 0.5rem 0 0;
		font-size: 0.9375rem;
		color: #1e293b;
		white-space: pre-line;
	}

	.empty {
		margin: 0.5rem 0 0;
		font-size: 0.8125rem;
		color: #94a3b8;
		font-style: italic;
	}

	.research-meta {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(14rem, 1fr));
		gap: 0.75rem;
		margin: 0.75rem 0 0;
	}

	.research-meta dt {
		font-size: 0.6875rem;
		text-transform: uppercase;
		letter-spacing: 0.04em;
		color: #94a3b8;
	}

	.research-meta dd {
		margin: 0.25rem 0 0;
		font-size: 0.8125rem;
		color: #475569;
	}

	.research-meta dd .empty {
		margin: 0;
	}

	.chips,
	.proofs {
		list-style: none;
		margin: 0;
		padding: 0;
		display: flex;
		flex-direction: column;
		gap: 0.25rem;
	}

	.chip-type,
	.fact-type {
		display: inline-block;
		margin-right: 0.375rem;
		font-size: 0.6875rem;
		text-transform: capitalize;
		color: #94a3b8;
	}

	.fact-type {
		margin: 0 0 0 0.375rem;
	}

	a {
		color: #2563eb;
		text-decoration: none;
	}

	a:hover {
		text-decoration: underline;
	}

	.unresolved {
		color: #94a3b8;
		font-style: italic;
	}
</style>
