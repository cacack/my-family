<script lang="ts">
	/**
	 * The setup page: create the first owner (ADR-007). It needs the one-time
	 * token the server writes to its log, so only whoever runs the server can
	 * claim it.
	 */
	import AuthScreen from './AuthScreen.svelte';
	import ActionError from '#lib/components/ActionError.svelte';
	import { Button } from '#lib/components/ui/button/index.js';
	import { Input } from '#lib/components/ui/input/index.js';
	import { Label } from '#lib/components/ui/label/index.js';
	import { completeSetup } from '#lib/stores/auth.svelte.js';
	import type { ApiError } from '#lib/api/client.js';

	const MIN_PASSWORD = 8;

	let token = $state('');
	let username = $state('');
	let displayName = $state('');
	let password = $state('');
	let confirm = $state('');
	let submitting = $state(false);
	let error = $state<string | null>(null);

	async function handleSubmit(event: SubmitEvent) {
		event.preventDefault();
		error = null;
		if (password !== confirm) {
			error = 'The passwords do not match.';
			return;
		}
		submitting = true;
		try {
			await completeSetup({
				token: token.trim(),
				username,
				display_name: displayName.trim() || username,
				password
			});
		} catch (e) {
			const apiError = e as ApiError;
			error = apiError.message || 'Could not create the owner. Try again.';
		} finally {
			submitting = false;
		}
	}
</script>

<AuthScreen
	title="Set up My Family"
	description="Create the owner account. The owner manages the tree and, later, who else may use it."
>
	<form class="auth-form" onsubmit={handleSubmit}>
		<div class="field">
			<Label for="setup-token">Setup token</Label>
			<Input id="setup-token" name="token" autocomplete="off" autocapitalize="none" spellcheck="false" required bind:value={token} aria-describedby="setup-token-hint" />
			<p id="setup-token-hint" class="hint">
				The server wrote it to its log when it started ("Setup required: ..."). You can also run
				<code>myfamily user create</code> on the server instead.
			</p>
		</div>
		<div class="field">
			<Label for="setup-username">Username</Label>
			<Input id="setup-username" name="username" autocomplete="username" autocapitalize="none" required maxlength={64} bind:value={username} />
		</div>
		<div class="field">
			<Label for="setup-display-name">Display name</Label>
			<Input id="setup-display-name" name="display_name" autocomplete="name" maxlength={100} bind:value={displayName} aria-describedby="setup-display-name-hint" />
			<p id="setup-display-name-hint" class="hint">Shown in the history of your changes. Defaults to the username.</p>
		</div>
		<div class="field">
			<Label for="setup-password">Password</Label>
			<Input id="setup-password" name="password" type="password" autocomplete="new-password" required minlength={MIN_PASSWORD} maxlength={256} bind:value={password} aria-describedby="setup-password-hint" />
			<p id="setup-password-hint" class="hint">At least {MIN_PASSWORD} characters.</p>
		</div>
		<div class="field">
			<Label for="setup-confirm">Password again</Label>
			<Input id="setup-confirm" name="confirm" type="password" autocomplete="new-password" required bind:value={confirm} aria-describedby={error ? 'setup-error' : undefined} />
		</div>
		{#if error}
			<ActionError id="setup-error" message={error} />
		{/if}
		<Button type="submit" disabled={submitting}>{submitting ? 'Creating...' : 'Create owner and sign in'}</Button>
	</form>
</AuthScreen>

<style>
	.auth-form {
		display: flex;
		flex-direction: column;
		gap: 1rem;
	}

	.field {
		display: flex;
		flex-direction: column;
		gap: 0.375rem;
	}

	.hint {
		margin: 0;
		font-size: 0.8125rem;
		color: var(--color-text-muted);
	}

	.hint code {
		font-size: 0.75rem;
	}
</style>
