<script lang="ts">
	/**
	 * The merge review's evidence-coverage warning (#838): the facts and
	 * relationships this branch changed that have no evidence analysis or proof
	 * summary written on the branch, each linking to the fact's page and to "add
	 * analysis". Soft and non-blocking - proof summaries are a later roadmap
	 * phase - so it never holds the merge, and a failed check only says so.
	 *
	 * An analysis added from here must land on THIS branch, so the "add
	 * analysis" links are offered only while it is the active branch; otherwise
	 * the panel offers the switch instead.
	 *
	 * Loads its own data (`GET /branches/{id}/evidence-coverage`), keyed on
	 * `branchId`, with a request token so a late answer for another branch is
	 * dropped.
	 */
	import { api, type ApiError, type BranchEvidenceCoverage } from '$lib/api/client';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import {
		addAnalysisHref,
		changedFactLabel,
		coverageHeading,
		subjectHref
	} from '$lib/utils/branchReview';

	interface Props {
		branchId: string;
		/** True when this branch is the active one, so new analyses land on it. */
		onBranch: boolean;
		/** Switches to this branch. */
		onswitch: () => void;
	}

	let { branchId, onBranch, onswitch }: Props = $props();

	let coverage: BranchEvidenceCoverage | null = $state(null);
	let loading = $state(true);
	let error: string | null = $state(null);
	let request = 0;

	async function load(id: string) {
		const token = ++request;
		loading = true;
		error = null;
		coverage = null;
		try {
			const result = await api.getBranchEvidenceCoverage(id);
			if (token !== request) return;
			coverage = result;
		} catch (e) {
			if (token !== request) return;
			error = (e as ApiError)?.message || 'the check failed';
		} finally {
			if (token === request) loading = false;
		}
	}

	$effect(() => {
		if (branchId) load(branchId);
	});

	// `$derived.by`: read inline, TypeScript narrows `coverage` to its `null`
	// initialiser.
	const uncovered = $derived.by(() => coverage?.uncovered ?? []);
</script>

{#if loading}
	<p class="coverage-status" role="status">Checking evidence coverage...</p>
{:else if error}
	<p class="coverage-status" role="note">
		Evidence coverage could not be checked ({error}). It is a reminder, not a merge requirement.
	</p>
{:else if coverage && coverage.changed_fact_count > 0}
	{#if uncovered.length === 0}
		<p class="coverage-clean" data-testid="evidence-coverage">
			Every fact and relationship this branch changed has an evidence analysis or proof summary on
			the branch.
		</p>
	{:else}
		<section
			class="coverage"
			aria-labelledby="evidence-coverage-heading"
			data-testid="evidence-coverage"
		>
			<h2 id="evidence-coverage-heading">{coverageHeading(uncovered.length)}</h2>
			<p class="hint">
				Findings are merged back with review: an analysis or proof summary on the branch says why
				each change is right. This is a reminder, not a merge requirement.
				{#if coverage.has_more}
					The branch is larger than the check reads, so this list may be incomplete.
				{/if}
			</p>
			{#if !onBranch}
				<div class="switch-row">
					<span>Switch to this branch to add analyses on it.</span>
					<Button variant="outline" size="sm" onclick={onswitch}>Switch to branch</Button>
				</div>
			{/if}
			<ul class="fact-list">
				{#each uncovered as fact (`${fact.kind}:${fact.fact_type ?? ''}:${fact.subject_id}`)}
					<li class="fact">
						<div class="fact-text">
							<Badge variant="outline">{changedFactLabel(fact)}</Badge>
							{#if fact.kind === 'deletion'}
								<!-- Deleted on the branch: there is no page for it there. -->
								<span class="subject">{fact.subject_name || `Unnamed ${fact.subject_type}`}</span>
							{:else}
								<a href={subjectHref(fact.subject_type, fact.subject_id)} class="subject"
									>{fact.subject_name || `Unnamed ${fact.subject_type}`}</a
								>
							{/if}
							<span class="count"
								>{fact.change_count} change{fact.change_count === 1 ? '' : 's'}</span
							>
						</div>
						{#if onBranch}
							<a class="add-link" href={addAnalysisHref(fact)}
								>Add analysis<span class="sr-only">
									for {changedFactLabel(fact)} of {fact.subject_name || fact.subject_type}</span
								></a
							>
						{/if}
					</li>
				{/each}
			</ul>
		</section>
	{/if}
{/if}

<style>
	.coverage {
		margin-bottom: 2rem;
		padding: 0.875rem 1rem;
		background: #fefce8;
		border: 1px solid #fde047;
		border-radius: 8px;
	}

	.coverage h2 {
		margin: 0;
		font-size: 1rem;
		color: #713f12;
	}

	.hint {
		margin: 0.25rem 0 0.75rem;
		font-size: 0.8125rem;
		color: #854d0e;
		max-width: 52rem;
	}

	.switch-row {
		display: flex;
		align-items: center;
		gap: 0.75rem;
		flex-wrap: wrap;
		margin-bottom: 0.75rem;
		font-size: 0.8125rem;
		color: #713f12;
	}

	.fact-list {
		list-style: none;
		margin: 0;
		padding: 0;
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
	}

	.fact {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 0.75rem;
		flex-wrap: wrap;
		padding: 0.5rem 0.75rem;
		background: white;
		border: 1px solid #fef08a;
		border-radius: 6px;
	}

	.fact-text {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		flex-wrap: wrap;
		min-width: 0;
		overflow-wrap: anywhere;
		font-size: 0.875rem;
	}

	.subject {
		font-weight: 500;
		color: #1e293b;
		text-decoration: none;
	}

	.subject:hover,
	.add-link:hover {
		color: #2563eb;
		text-decoration: underline;
	}

	.count {
		font-size: 0.75rem;
		color: #64748b;
	}

	.add-link {
		font-size: 0.8125rem;
		color: #1d4ed8;
		text-decoration: none;
	}

	.coverage-status,
	.coverage-clean {
		margin: 0 0 1.5rem;
		padding: 0.625rem 0.875rem;
		border-radius: 6px;
		font-size: 0.8125rem;
	}

	.coverage-status {
		background: #f8fafc;
		border: 1px solid #e2e8f0;
		color: #475569;
	}

	.coverage-clean {
		background: #f0fdf4;
		border: 1px solid #bbf7d0;
		color: #166534;
	}
</style>
