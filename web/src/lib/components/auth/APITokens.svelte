<script lang="ts">
	/**
	 * The signed-in user's API tokens (#1416): a script sends one as
	 * `Authorization: Bearer <token>` to act as this user, with their role. A
	 * new token is shown once, as the server keeps only its hash; each listed
	 * token shows its name and when it was created and last used, and can be
	 * revoked.
	 */
	import AccountLinkBox from './AccountLinkBox.svelte';
	import ActionError from '#lib/components/ActionError.svelte';
	import ErrorState from '#lib/components/ErrorState.svelte';
	import LoadingState from '#lib/components/LoadingState.svelte';
	import { Button } from '#lib/components/ui/button/index.js';
	import { Input } from '#lib/components/ui/input/index.js';
	import { Label } from '#lib/components/ui/label/index.js';
	import { api, type APIToken, type ApiError, type CreatedAPIToken } from '#lib/api/client.js';

	interface Props {
		/** Tells screen readers that a token was created or revoked. */
		announce?: (message: string) => void;
	}

	let { announce }: Props = $props();

	let tokens: APIToken[] = $state([]);
	let loading = $state(true);
	let loadError: string | null = $state(null);

	let name = $state('');
	let creating = $state(false);
	let createError: string | null = $state(null);
	let created: CreatedAPIToken | null = $state(null);

	let busy: Record<string, boolean> = $state({});
	let rowErrors: Record<string, string> = $state({});

	async function reload() {
		try {
			tokens = await api.listAPITokens();
			loadError = null;
		} catch (e) {
			const apiError = e as ApiError;
			loadError =
				apiError.status && apiError.status < 500
					? apiError.message
					: 'Failed to load your API tokens. Please try again.';
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

	async function create(event: SubmitEvent) {
		event.preventDefault();
		createError = null;
		creating = true;
		try {
			created = await api.createAPIToken(name.trim());
			name = '';
			await reload();
			announce?.(`API token ${created.name} created. Copy it now: it is not shown again.`);
		} catch (e) {
			createError = (e as ApiError).message || 'Could not create the token. Try again.';
		} finally {
			creating = false;
		}
	}

	async function revoke(token: APIToken) {
		busy[token.id] = true;
		delete rowErrors[token.id];
		try {
			await api.revokeAPIToken(token.id);
			announce?.(`API token ${token.name} revoked`);
		} catch (e) {
			const apiError = e as ApiError;
			if (apiError.status !== 404) {
				rowErrors[token.id] = apiError.message || 'Could not revoke the token. Try again.';
				return;
			}
			// Revoked meanwhile, from another browser: it is gone either way.
			announce?.(`API token ${token.name} was already revoked`);
		} finally {
			busy[token.id] = false;
		}
		tokens = tokens.filter((t) => t.id !== token.id);
		if (created?.id === token.id) created = null;
	}

	function when(iso: string): string {
		return new Date(iso).toLocaleString(undefined, { dateStyle: 'medium', timeStyle: 'short' });
	}
</script>

<section class="panel" aria-labelledby="tokens-heading">
	<h2 id="tokens-heading">API tokens</h2>
	<p class="hint">
		A script sends a token as <code>Authorization: Bearer &lt;token&gt;</code> to use the API as
		you, with your role. Revoke a token you no longer use, or one that may have leaked.
	</p>

	<form class="panel-form" onsubmit={create}>
		<div class="field">
			<Label for="api-token-name">Token name</Label>
			<Input
				id="api-token-name"
				required
				maxlength={100}
				placeholder="e.g. nightly backup"
				bind:value={name}
				aria-describedby={createError ? 'api-token-error' : undefined}
			/>
		</div>
		{#if createError}
			<ActionError id="api-token-error" message={createError} />
		{/if}
		<div>
			<Button type="submit" disabled={creating || name.trim() === ''}>
				{creating ? 'Creating...' : 'Create token'}
			</Button>
		</div>
	</form>

	{#if created}
		<AccountLinkBox
			id="api-token-secret"
			label="New token: {created.name}"
			url={created.token}
			hint="Copy it now and keep it somewhere safe: it is not shown again."
		/>
	{/if}

	{#if loading}
		<LoadingState label="Loading API tokens..." />
	{:else if loadError}
		<ErrorState message={loadError} onRetry={retry} />
	{:else if tokens.length === 0}
		<p class="none">You have no API tokens.</p>
	{:else}
		<ul class="tokens" aria-label="Your API tokens">
			{#each tokens as token (token.id)}
				<li class="token" data-testid="api-token">
					<div class="token-head">
						<div class="token-text">
							<span class="title">{token.name}</span>
							<span class="meta">
								Created {when(token.created_at)}.
								{token.last_used_at ? `Last used ${when(token.last_used_at)}.` : 'Never used.'}
							</span>
						</div>
						<Button
							variant="outline"
							size="sm"
							disabled={busy[token.id]}
							onclick={() => revoke(token)}
							aria-label="Revoke API token {token.name}"
						>
							{busy[token.id] ? 'Revoking...' : 'Revoke'}
						</Button>
					</div>
					{#if rowErrors[token.id]}
						<ActionError message={rowErrors[token.id]} />
					{/if}
				</li>
			{/each}
		</ul>
	{/if}
</section>

<style>
	.panel {
		display: flex;
		flex-direction: column;
		gap: 1rem;
		background: white;
		border: 1px solid #e2e8f0;
		border-radius: 8px;
		padding: 1rem;
		margin-top: 1.5rem;
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

	.panel-form {
		display: flex;
		flex-direction: column;
		gap: 1rem;
		max-width: 26rem;
	}

	.field {
		display: flex;
		flex-direction: column;
		gap: 0.375rem;
	}

	.hint,
	.none,
	.meta {
		margin: 0;
		font-size: 0.8125rem;
		color: var(--color-text-muted);
	}

	.hint code {
		overflow-wrap: anywhere;
	}

	.tokens {
		list-style: none;
		margin: 0;
		padding: 0;
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
	}

	.token {
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
		padding-top: 0.5rem;
		border-top: 1px solid #e2e8f0;
	}

	.token:first-child {
		padding-top: 0;
		border-top: none;
	}

	.token-head {
		display: flex;
		align-items: flex-start;
		justify-content: space-between;
		gap: 0.75rem;
	}

	.token-text {
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
