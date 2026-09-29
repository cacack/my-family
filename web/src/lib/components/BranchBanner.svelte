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
			<a class="notice-action" href={activeBranch.noticeHref} onclick={dismissBranchNotice}>
				Finish merge
			</a>
		{/if}
		<button class="notice-dismiss" onclick={dismissBranchNotice}>Dismiss</button>
	</div>
{/if}

{#if activeBranch.id}
	<div class="branch-banner" role="status">
		<span class="branch-label">Research Branch</span>
		<span class="branch-text">
			{#if activeBranch.branch}
				Working on <strong>{activeBranch.branch.name}</strong>. Changes to people, families,
				sources, media and evidence are isolated to this branch; pages that stay on the mainline say
				so.
			{:else}
				Working on a research branch. Changes to people, families, sources, media and evidence are
				isolated to this branch; pages that stay on the mainline say so.
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
					A branch is a live view over the mainline, not a frozen copy. Records you haven't edited
					here always show the mainline's current data, so later mainline corrections appear on
					this branch too - there is nothing to rebase. Records you edit here are isolated: the
					mainline can't overwrite your version, and any mainline changes to them are listed in
					Compare for you to review before merging.
				</p>
			</details>
		</span>
		<a class="branch-compare" href="/branches/{activeBranch.id}">Compare</a>
		<button class="branch-exit" onclick={handleReturn} disabled={leaving}>
			{leaving ? 'Returning...' : 'Return to Mainline'}
		</button>
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

	.branch-banner {
		background: #ede9fe;
		border-bottom: 1px solid #7c3aed;
		color: #5b21b6;
	}

	:global(body.high-contrast) .branch-banner {
		background: #2e1065;
		border-bottom-color: #a78bfa;
		color: #ede9fe;
	}

	.branch-label {
		font-weight: 700;
		white-space: nowrap;
	}

	.branch-text {
		flex: 1;
		min-width: 12rem;
	}

	.branch-research {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		margin-top: 0.25rem;
		min-width: 0;
	}

	.branch-hypothesis {
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

	.branch-compare,
	.branch-exit {
		padding: 0.25rem 0.75rem;
		border: 1px solid #7c3aed;
		border-radius: 4px;
		background: white;
		color: #5b21b6;
		font-size: 0.8125rem;
		font-weight: 500;
		text-decoration: none;
		cursor: pointer;
		white-space: nowrap;
		transition: background 0.15s;
	}

	:global(body.high-contrast) .branch-compare,
	:global(body.high-contrast) .branch-exit {
		background: #1e1b4b;
		border-color: #a78bfa;
		color: #ede9fe;
	}

	.branch-compare:hover,
	.branch-exit:hover:not(:disabled) {
		background: #ede9fe;
	}

	.branch-compare:focus-visible,
	.branch-exit:focus-visible {
		outline: 2px solid #7c3aed;
		outline-offset: 2px;
	}

	.branch-exit:disabled {
		opacity: 0.6;
		cursor: not-allowed;
	}

	.branch-notice {
		background: #fef3c7;
		border-bottom: 1px solid #f59e0b;
		color: #92400e;
	}

	:global(body.high-contrast) .branch-notice {
		background: #78350f;
		border-bottom-color: #f59e0b;
		color: #fef3c7;
	}

	.notice-text {
		flex: 1;
		min-width: 12rem;
	}

	.notice-dismiss,
	.notice-action {
		padding: 0.25rem 0.75rem;
		border: 1px solid #d97706;
		border-radius: 4px;
		background: white;
		color: #92400e;
		font-size: 0.8125rem;
		font-weight: 500;
		cursor: pointer;
		white-space: nowrap;
	}

	.notice-action {
		text-decoration: none;
		font-weight: 600;
	}

	:global(body.high-contrast) .notice-dismiss,
	:global(body.high-contrast) .notice-action {
		background: #451a03;
		border-color: #f59e0b;
		color: #fef3c7;
	}

	.notice-dismiss:hover {
		background: #fef3c7;
	}

	.notice-dismiss:focus-visible {
		outline: 2px solid #d97706;
		outline-offset: 2px;
	}
</style>
