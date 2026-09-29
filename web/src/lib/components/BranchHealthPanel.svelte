<script lang="ts">
	/**
	 * The merge review's branch health (#838): the validation issues, quality
	 * issues and possible duplicates this branch INTRODUCES - what its view of
	 * the tree has that the mainline's does not - so a reviewer sees the
	 * branch's own problems rather than the tree's standing ones (those are on
	 * `/quality`). Errors and warnings are listed; notices and quality issues sit
	 * in expandable groups.
	 *
	 * Record links open as the ACTIVE branch sees the record, so while the
	 * reviewer is elsewhere the panel offers the switch: a person the branch
	 * created is not found on the mainline. A possible duplicate is not offered
	 * for merging here - person merge is not branch-scoped, so the merge page
	 * would merge the mainline's records, which the branch's edits made alike
	 * only on the branch; the pair is merged from the Quality page once the branch
	 * has merged.
	 *
	 * Informational only: nothing here holds the merge, and a failed check only
	 * says so. Loads its own data (`GET /branches/{id}/health`), keyed on
	 * `branchId`, with a request token so a late answer for another branch is
	 * dropped.
	 */
	import {
		api,
		type ApiError,
		type BranchHealth,
		type BranchValidationIssue
	} from '$lib/api/client';
	import SeverityBadge from '$lib/components/SeverityBadge.svelte';
	import { Button } from '$lib/components/ui/button';
	import {
		confidencePercent,
		healthSummary,
		issueRecordHref,
		subjectHref
	} from '$lib/utils/branchReview';

	interface Props {
		branchId: string;
		/** True when this branch is the active one, so record links show its view. */
		onBranch: boolean;
		/** Switches to this branch. */
		onswitch: () => void;
	}

	let { branchId, onBranch, onswitch }: Props = $props();

	let health: BranchHealth | null = $state(null);
	let loading = $state(true);
	let error: string | null = $state(null);
	let request = 0;

	async function load(id: string) {
		const token = ++request;
		loading = true;
		error = null;
		health = null;
		try {
			const result = await api.getBranchHealth(id);
			if (token !== request) return;
			health = result;
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

	// `$derived.by`: read inline, TypeScript narrows `health` to its `null`
	// initialiser.
	const issues = $derived.by(() => health?.validation_issues ?? []);
	const serious = $derived(issues.filter((i) => i.severity !== 'info'));
	const notices = $derived(issues.filter((i) => i.severity === 'info'));
	const introducesAnything = $derived.by(
		() =>
			issues.length > 0 ||
			(health?.quality_issues.length ?? 0) > 0 ||
			(health?.duplicates.length ?? 0) > 0
	);
</script>

{#snippet issueList(issues: BranchValidationIssue[])}
	<ul class="issue-list">
		{#each issues as issue (issue.key)}
			{@const href = issueRecordHref(issue)}
			<li class="issue">
				<div class="issue-head">
					<SeverityBadge severity={issue.severity} />
					<code class="code">{issue.code}</code>
					{#if href}
						<a {href} class="record">{issue.record_name || `Unnamed ${issue.record_type}`}</a>
					{/if}
				</div>
				<p class="message">{issue.message}</p>
			</li>
		{/each}
	</ul>
{/snippet}

<section class="health" aria-labelledby="branch-health-heading" data-testid="branch-health">
	<h2 id="branch-health-heading">Branch health</h2>
	{#if loading}
		<p class="status" role="status">Checking what this branch introduces...</p>
	{:else if error}
		<p class="status" role="note">
			Branch health could not be checked ({error}). It is informational and does not hold the merge.
		</p>
	{:else if health}
		<p class="summary" class:clean={!introducesAnything}>
			{healthSummary(health)}
			{#if health.resolved_count > 0}
				It also resolves {health.resolved_count}
				{health.resolved_count === 1 ? 'finding' : 'findings'} the mainline has.
			{/if}
		</p>

		{#if introducesAnything && !onBranch}
			<div class="switch-row">
				<span
					>Records open as the active branch sees them. Switch to this branch to open them as it
					has them, including the ones it created.</span
				>
				<Button variant="outline" size="sm" onclick={onswitch}>Switch to branch</Button>
			</div>
		{/if}

		{#if serious.length > 0}
			<h3>Validation issues</h3>
			{@render issueList(serious)}
		{/if}

		{#if health.duplicates.length > 0}
			<h3>Possible duplicates</h3>
			<p class="message duplicates-hint">
				Person merge does not work on a research branch. If these are the same person, merge them
				from the Quality page after this branch is merged.
			</p>
			<ul class="issue-list">
				{#each health.duplicates as pair (pair.key)}
					<li class="issue duplicate">
						<div class="issue-head">
							<a href={subjectHref('person', pair.person1_id)} class="record">{pair.person1_name}</a>
							<span class="muted">and</span>
							<a href={subjectHref('person', pair.person2_id)} class="record">{pair.person2_name}</a>
							<span class="muted">({confidencePercent(pair.confidence)} match)</span>
						</div>
						{#if pair.match_reasons.length > 0}
							<p class="message">{pair.match_reasons.join('; ')}</p>
						{/if}
					</li>
				{/each}
			</ul>
		{/if}

		{#if notices.length > 0}
			<details class="group">
				<summary>{notices.length} {notices.length === 1 ? 'notice' : 'notices'}</summary>
				{@render issueList(notices)}
			</details>
		{/if}

		{#if health.quality_issues.length > 0}
			<details class="group">
				<summary>
					{health.quality_issues.length} quality {health.quality_issues.length === 1
						? 'issue'
						: 'issues'}
				</summary>
				<ul class="issue-list">
					{#each health.quality_issues as issue (issue.key)}
						<li class="issue">
							<div class="issue-head">
								<a href={subjectHref('person', issue.person_id)} class="record"
									>{issue.person_name || 'Unnamed person'}</a
								>
								<span>{issue.issue}</span>
							</div>
						</li>
					{/each}
				</ul>
			</details>
		{/if}
	{/if}
</section>

<style>
	.health {
		margin-bottom: 2rem;
	}

	.health h2 {
		margin: 0 0 0.25rem;
		font-size: 1rem;
		color: #1e293b;
	}

	.health h3 {
		margin: 1rem 0 0.5rem;
		font-size: 0.875rem;
		color: #1e293b;
	}

	.status,
	.summary {
		margin: 0 0 0.75rem;
		font-size: 0.875rem;
		color: #475569;
		max-width: 52rem;
	}

	.summary.clean {
		padding: 0.75rem 1rem;
		background: #f0fdf4;
		border: 1px solid #bbf7d0;
		border-radius: 6px;
		color: #166534;
	}

	.issue-list {
		list-style: none;
		margin: 0;
		padding: 0;
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
	}

	.issue {
		padding: 0.625rem 0.75rem;
		background: white;
		border: 1px solid #e2e8f0;
		border-radius: 6px;
	}

	.issue-head {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		flex-wrap: wrap;
		min-width: 0;
		overflow-wrap: anywhere;
		font-size: 0.875rem;
		color: #1e293b;
	}

	.code {
		font-size: 0.75rem;
		background: #f1f5f9;
		padding: 0.125rem 0.375rem;
		border-radius: 4px;
		color: #475569;
	}

	.record {
		font-weight: 500;
		color: #1e293b;
		text-decoration: none;
	}

	.record:hover {
		color: #2563eb;
		text-decoration: underline;
	}

	.message {
		margin: 0.25rem 0 0;
		font-size: 0.8125rem;
		color: #475569;
		overflow-wrap: anywhere;
	}

	.muted {
		font-size: 0.8125rem;
		color: #64748b;
	}

	.duplicates-hint {
		margin: 0 0 0.5rem;
	}

	.switch-row {
		display: flex;
		align-items: center;
		gap: 0.75rem;
		flex-wrap: wrap;
		margin-bottom: 0.75rem;
		font-size: 0.8125rem;
		color: #475569;
	}

	.group {
		margin-top: 0.75rem;
	}

	.group summary {
		cursor: pointer;
		font-size: 0.875rem;
		color: #334155;
		margin-bottom: 0.5rem;
	}
</style>
