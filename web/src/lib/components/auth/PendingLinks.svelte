<script lang="ts">
	/**
	 * The invite and password reset links still pending (#1430): who issued
	 * each and when it expires, never the link itself (the server keeps only a
	 * hash). An owner revokes one sent to the wrong person, and it then fails
	 * as an expired link does.
	 */
	import ActionError from '#lib/components/ActionError.svelte';
	import ErrorState from '#lib/components/ErrorState.svelte';
	import LoadingState from '#lib/components/LoadingState.svelte';
	import { Button } from '#lib/components/ui/button/index.js';
	import {
		api,
		type ApiError,
		type PendingInvite,
		type PendingPasswordReset
	} from '#lib/api/client.js';

	interface Props {
		/** Tells screen readers that a link was revoked. */
		announce?: (message: string) => void;
		/** Called with a link's ID once it is no longer pending: revoked here, or gone meanwhile. */
		onrevoke?: (id: string) => void;
	}

	let { announce, onrevoke }: Props = $props();

	/** A pending link as listed: what it is, who issued it, and how to revoke it. */
	interface Row {
		id: string;
		title: string;
		issuer: string;
		createdAt: string;
		expiresAt: string;
		revoke: () => Promise<void>;
	}

	let invites: Row[] = $state([]);
	let resets: Row[] = $state([]);
	let loading = $state(true);
	let loadError: string | null = $state(null);
	let busy: Record<string, boolean> = $state({});
	let rowErrors: Record<string, string> = $state({});

	function inviteRow(invite: PendingInvite): Row {
		return {
			id: invite.id,
			title: `${invite.role[0].toUpperCase()}${invite.role.slice(1)} invite`,
			issuer: invite.created_by_name ?? 'a removed user',
			createdAt: invite.created_at,
			expiresAt: invite.expires_at,
			revoke: () => api.revokeInvite(invite.id)
		};
	}

	function resetRow(reset: PendingPasswordReset): Row {
		return {
			id: reset.id,
			title: `Password reset for ${reset.user_name ?? 'a removed user'}`,
			issuer: reset.created_by ? (reset.created_by_name ?? 'a removed user') : 'the command line',
			createdAt: reset.created_at,
			expiresAt: reset.expires_at,
			revoke: () => api.revokePasswordReset(reset.id)
		};
	}

	/** Loads the pending links again, as after issuing one. */
	export async function reload() {
		try {
			const [pendingInvites, pendingResets] = await Promise.all([
				api.listInvites(),
				api.listPasswordResets()
			]);
			invites = pendingInvites.map(inviteRow);
			resets = pendingResets.map(resetRow);
			loadError = null;
		} catch (e) {
			const apiError = e as ApiError;
			loadError =
				apiError.status && apiError.status < 500
					? apiError.message
					: 'Failed to load the pending links. Please try again.';
		} finally {
			loading = false;
		}
	}

	$effect(() => {
		reload();
	});

	function retry() {
		loading = true;
		reload();
	}

	async function revoke(row: Row) {
		busy[row.id] = true;
		delete rowErrors[row.id];
		try {
			await row.revoke();
			invites = invites.filter((r) => r.id !== row.id);
			resets = resets.filter((r) => r.id !== row.id);
			onrevoke?.(row.id);
			announce?.(`${row.title} revoked`);
		} catch (e) {
			const apiError = e as ApiError;
			if (apiError.status === 404) {
				// Used, expired or revoked meanwhile: it is no longer pending.
				await reload();
				onrevoke?.(row.id);
				announce?.(`${row.title} was no longer pending`);
			} else {
				rowErrors[row.id] = apiError.message || 'Could not revoke the link. Try again.';
			}
		} finally {
			busy[row.id] = false;
		}
	}

	function when(iso: string): string {
		return new Date(iso).toLocaleString(undefined, { dateStyle: 'medium', timeStyle: 'short' });
	}
</script>

{#snippet list(label: string, rows: Row[], none: string)}
	<h3>{label}</h3>
	{#if rows.length === 0}
		<p class="none">{none}</p>
	{:else}
		<ul class="links" aria-label={label}>
			{#each rows as row (row.id)}
				<li class="link" data-testid="pending-link">
					<div class="link-head">
						<div class="link-text">
							<span class="title">{row.title}</span>
							<span class="meta">
								Created by {row.issuer}, {when(row.createdAt)}. Expires {when(row.expiresAt)}.
							</span>
						</div>
						<Button
							variant="outline"
							size="sm"
							disabled={busy[row.id]}
							onclick={() => revoke(row)}
							aria-label="Revoke {row.title.toLowerCase()} created {when(row.createdAt)}"
						>
							{busy[row.id] ? 'Revoking...' : 'Revoke'}
						</Button>
					</div>
					{#if rowErrors[row.id]}
						<ActionError message={rowErrors[row.id]} />
					{/if}
				</li>
			{/each}
		</ul>
	{/if}
{/snippet}

<section class="panel" aria-labelledby="pending-heading">
	<h2 id="pending-heading">Pending links</h2>
	{#if loading}
		<LoadingState label="Loading pending links..." />
	{:else if loadError}
		<ErrorState message={loadError} onRetry={retry} />
	{:else}
		{@render list('Invites', invites, 'No invite links are pending.')}
		{@render list('Password resets', resets, 'No password reset links are pending.')}
	{/if}
</section>

<style>
	.panel {
		display: flex;
		flex-direction: column;
		gap: 0.75rem;
		background: white;
		border: 1px solid #e2e8f0;
		border-radius: 8px;
		padding: 1rem;
		margin-bottom: 1.5rem;
	}

	:global(body.high-contrast) .panel {
		background: var(--color-bg);
		border-color: var(--a11y-color-border);
	}

	h2 {
		margin: 0;
		font-size: 0.8125rem;
		font-weight: 600;
		text-transform: uppercase;
		letter-spacing: 0.05em;
		color: var(--color-text-muted);
	}

	h3 {
		margin: 0;
		font-size: 0.875rem;
		font-weight: 600;
	}

	.none,
	.meta {
		margin: 0;
		font-size: 0.8125rem;
		color: var(--color-text-muted);
	}

	.links {
		list-style: none;
		margin: 0;
		padding: 0;
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
	}

	.link {
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
		padding-top: 0.5rem;
		border-top: 1px solid #e2e8f0;
	}

	.link:first-child {
		padding-top: 0;
		border-top: none;
	}

	.link-head {
		display: flex;
		align-items: flex-start;
		justify-content: space-between;
		gap: 0.75rem;
	}

	.link-text {
		display: flex;
		flex-direction: column;
		min-width: 0;
	}

	.title {
		font-weight: 600;
		overflow-wrap: anywhere;
	}

	.meta {
		overflow-wrap: anywhere;
	}
</style>
