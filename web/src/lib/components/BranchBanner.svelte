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
	 */
	import {
		activeBranch,
		returnToMainline,
		dismissBranchNotice
	} from '$lib/stores/activeBranch.svelte';

	// Switching reloads the page, so this only has to survive until the reload
	// lands - it stops a second click from firing another one.
	let leaving = $state(false);

	function handleReturn() {
		leaving = true;
		returnToMainline();
	}
</script>

{#if activeBranch.notice}
	<div class="branch-notice" role="alert">
		<span class="notice-text">{activeBranch.notice}</span>
		<button class="notice-dismiss" onclick={dismissBranchNotice}>Dismiss</button>
	</div>
{/if}

{#if activeBranch.id}
	<div class="branch-banner" role="status">
		<span class="branch-label">Research Branch</span>
		<span class="branch-text">
			{#if activeBranch.branch}
				Working on <strong>{activeBranch.branch.name}</strong>. Changes to people, families and
				pedigrees are isolated to this branch.
			{:else}
				Working on a research branch. Changes to people, families and pedigrees are isolated to
				this branch.
			{/if}
			{#if activeBranch.unconfirmed}
				<span class="branch-unconfirmed">
					Couldn't confirm this branch's status with the server, so it is still in use.
				</span>
			{/if}
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

	.branch-unconfirmed {
		display: block;
		font-style: italic;
		opacity: 0.85;
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

	.notice-dismiss {
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

	:global(body.high-contrast) .notice-dismiss {
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
