<script lang="ts">
	/**
	 * Persistent indicator that the app is off the mainline.
	 *
	 * Modelled on DemoBanner: it sits above the whole layout and is impossible to
	 * miss, because every write made while it is showing lands on the branch
	 * rather than on the mainline.
	 *
	 * It also carries the fallback notice for a persisted branch that turned out
	 * to be gone or terminal — that message appears when NO branch is active,
	 * which is exactly what it is telling the user.
	 *
	 * Branches are live overlays, not frozen copies (ADR-005 §The model), so the
	 * banner also explains that and says how far the mainline has moved under
	 * the branch since it forked (#837) - a cheap count, not a compare.
	 */
	import { api, onBranchWrite, type BranchDrift } from '$lib/api/client';
	import {
		activeBranch,
		returnToMainline,
		dismissBranchNotice
	} from '$lib/stores/activeBranch.svelte';
	import { Button } from '$lib/components/ui/button';
	import BranchOutcomeBadge from '$lib/components/branch/BranchOutcomeBadge.svelte';
	import BranchDriftIndicator from './BranchDriftIndicator.svelte';
	import { subjectHref, subjectLabel } from '$lib/utils/branchResearch';

	/** How many subjects the banner names before summarising the rest. */
	const BANNER_SUBJECTS = 3;

	const subjects = $derived(activeBranch.branch?.subjects ?? []);
	const shownSubjects = $derived(subjects.slice(0, BANNER_SUBJECTS));
	const hiddenSubjects = $derived(subjects.length - shownSubjects.length);

	// Switching reloads the page, so this only has to survive until the reload
	// lands - it stops a second click from firing another one.
	let leaving = $state(false);

	let drift: BranchDrift | null = $state(null);
	/** The drift read failed; the banner says so rather than showing nothing. */
	let driftFailed = $state(false);
	/**
	 * Bumped whenever the counts may have gone stale: a write landed on this
	 * branch (it may have touched a new entity, moving main's changes to it into
	 * the on-branch count), or the tab regained focus (main may have moved in
	 * another tab). The banner is mounted once for the whole app, so without this
	 * it would keep the counts from the moment the branch was activated.
	 */
	let refreshTick = $state(0);
	/** The branch the shown counts belong to; a refresh of the same one keeps them on screen. */
	let driftFor: string | null = null;

	function requestRefresh() {
		refreshTick++;
	}

	$effect(() => {
		const id = activeBranch.id;
		if (!id) return;
		const unsubscribe = onBranchWrite((branchId) => {
			if (branchId === id) requestRefresh();
		});
		const onVisible = () => {
			if (document.visibilityState === 'visible') requestRefresh();
		};
		window.addEventListener('focus', requestRefresh);
		document.addEventListener('visibilitychange', onVisible);
		return () => {
			unsubscribe();
			window.removeEventListener('focus', requestRefresh);
			document.removeEventListener('visibilitychange', onVisible);
		};
	});

	// Re-read whenever the active branch changes or a refresh is requested. A
	// superseded read is dropped so a slow answer for an earlier request cannot
	// overwrite a newer one, or land on a different branch.
	$effect(() => {
		const id = activeBranch.id;
		void refreshTick;
		if (id !== driftFor) {
			drift = null;
			driftFailed = false;
			driftFor = id;
		}
		if (!id) return;

		let current = true;
		api
			.getBranchDrift(id)
			.then((result) => {
				if (current) {
					drift = result;
					driftFailed = false;
				}
			})
			.catch(() => {
				// A failed refresh keeps the last good counts rather than hiding them.
				if (current && drift === null) driftFailed = true;
			});
		return () => {
			current = false;
		};
	});

	function handleReturn() {
		leaving = true;
		returnToMainline();
	}
</script>

{#if activeBranch.notice}
	<div class="branch-notice" role="alert">
		<span class="notice-text">{activeBranch.notice}</span>
		{#if activeBranch.noticeHref}
			<Button variant="outline" size="sm" href={activeBranch.noticeHref} onclick={dismissBranchNotice}>
				Finish merge
			</Button>
		{/if}
		<Button variant="outline" size="sm" onclick={dismissBranchNotice}>Dismiss</Button>
	</div>
{/if}

{#if activeBranch.id}
	<div class="branch-banner" role="status">
		<div class="branch-text">
			<!-- The label leads the first line rather than sitting in its own column,
			     which on a phone squeezed everything else into a sliver (#1095). -->
			<span class="branch-label">Research Branch</span>
			{#if activeBranch.branch}
				Working on <strong>{activeBranch.branch.name}</strong>. Edits stay on this branch.
			{:else}
				Working on a research branch. Edits stay on this branch.
			{/if}
			{#if activeBranch.branch}
				<span class="branch-research" data-testid="banner-research">
					<BranchOutcomeBadge outcome={activeBranch.branch.outcome} />
					{#if activeBranch.branch.hypothesis}
						<span class="branch-hypothesis" title={activeBranch.branch.hypothesis}>
							{activeBranch.branch.hypothesis}
						</span>
					{/if}
					{#if shownSubjects.length > 0}
						<span class="branch-subjects" data-testid="banner-subjects">
							<span class="subjects-label">About</span>
							{#each shownSubjects as subject, i (subject.type + subject.id)}
								<a href={subjectHref(subject)}>{subjectLabel(subject)}</a>{#if i < shownSubjects.length - 1},{/if}
							{/each}
							{#if hiddenSubjects > 0}
								<a href="/branches/{activeBranch.id}">+{hiddenSubjects} more</a>
							{/if}
						</span>
					{/if}
				</span>
			{/if}
			{#if activeBranch.unconfirmed}
				<span class="branch-unconfirmed">
					Couldn't confirm this branch's status with the server, so it is still in use.
				</span>
			{/if}
			{#if drift}
				<span class="branch-drift">
					<BranchDriftIndicator {drift} />
				</span>
			{:else if driftFailed}
				<span class="branch-drift">Couldn't check how far the mainline has moved.</span>
			{/if}
			<details class="branch-help">
				<summary>How branches work</summary>
				<p>
					Changes to people, families, sources, media and evidence are isolated to this branch;
					pages that stay on the mainline say so.
				</p>
				<p>
					A branch is a live view over the mainline, not a frozen copy. Records you haven't edited
					here always show the mainline's current data, so later mainline corrections appear on
					this branch too - there is nothing to rebase. Records you edit here are isolated: the
					mainline can't overwrite your version, and any mainline changes to them are listed in
					Compare for you to review before merging.
				</p>
			</details>
		</div>
		<div class="branch-actions">
			<Button variant="outline" size="sm" href="/branches/{activeBranch.id}">Compare</Button>
			<Button variant="outline" size="sm" onclick={handleReturn} disabled={leaving}>
				{leaving ? 'Returning...' : 'Return to Mainline'}
			</Button>
		</div>
	</div>
{/if}

<style>
	.branch-banner,
	.branch-notice {
		display: flex;
		align-items: center;
		gap: 0.75rem;
		padding: 0.5rem 1.5rem;
		font-size: 0.875rem;
		flex-wrap: wrap;
	}

	/* Tighter leading keeps a full research record within ~160px on a phone (#1095). */
	.branch-banner {
		line-height: 1.25;
		background: #ede9fe;
		border-bottom: 1px solid #7c3aed;
		color: #5b21b6;
	}

	.branch-label {
		font-weight: 700;
		white-space: nowrap;
		margin-right: 0.5rem;
	}

	.branch-text {
		flex: 1;
		min-width: 12rem;
	}

	/* Wraps so the subjects drop below a long question instead of squeezing
	   it to a few characters; the question grows to its own width, keeps at
	   least 8rem, and truncates with the full text in its title. */
	.branch-research {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		column-gap: 0.5rem;
		row-gap: 0.125rem;
		margin-top: 0.25rem;
		min-width: 0;
	}

	.branch-hypothesis {
		flex: 1 1 8rem;
		max-width: max-content;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		font-style: italic;
		min-width: 0;
	}

	.branch-subjects {
		display: inline-flex;
		flex-wrap: wrap;
		gap: 0.25rem;
		white-space: nowrap;
	}

	.branch-subjects a {
		color: inherit;
		text-decoration: underline;
	}

	.subjects-label {
		font-weight: 600;
	}

	.branch-unconfirmed {
		display: block;
		font-style: italic;
		opacity: 0.85;
	}

	.branch-drift {
		display: block;
		margin-top: 0.125rem;
	}

	.branch-help {
		margin-top: 0.125rem;
		font-size: 0.8125rem;
	}

	.branch-help summary {
		cursor: pointer;
		width: fit-content;
		text-decoration: underline;
		text-underline-offset: 2px;
	}

	.branch-help summary:focus-visible {
		outline: 2px solid #7c3aed;
		outline-offset: 2px;
	}

	.branch-help p {
		margin: 0.25rem 0 0;
		max-width: 48rem;
	}

	.branch-actions {
		display: flex;
		flex-wrap: wrap;
		gap: 0.5rem;
	}

	.branch-notice {
		background: #fef3c7;
		border-bottom: 1px solid #f59e0b;
		color: #92400e;
	}

	.notice-text {
		flex: 1;
		min-width: 12rem;
	}
</style>
