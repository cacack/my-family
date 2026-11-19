<script lang="ts">
	/**
	 * The page a password reset link opens (`/reset-password#token=...`), from
	 * the owner or `myfamily user reset-password` (ADR-007). Setting the
	 * password signs the user out everywhere; they then sign in with it.
	 */
	import AuthScreen from './AuthScreen.svelte';
	import ActionError from '#lib/components/ActionError.svelte';
	import { Button } from '#lib/components/ui/button/index.js';
	import { Input } from '#lib/components/ui/input/index.js';
	import { Label } from '#lib/components/ui/label/index.js';
	import { api, type ApiError } from '#lib/api/client.js';
	import { loadAuth } from '#lib/stores/auth.svelte.js';

	const MIN_PASSWORD = 8;

	let { token }: { token: string } = $props();

	let password = $state('');
	let confirm = $state('');
	let submitting = $state(false);
	let done = $state(false);
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
			await api.resetPassword({ token, password });
			done = true;
			// The reset ended every session of the account, perhaps this one.
			await loadAuth();
		} catch (e) {
			const apiError = e as ApiError;
			error = apiError.message || 'Could not set the password. Try again.';
		} finally {
			submitting = false;
		}
	}
</script>

<AuthScreen title="Choose a new password" description="Set the password you will sign in with from now on.">
	{#if done}
		<div class="done" role="status">
			<p>Your password is set, and you are signed out everywhere.</p>
			<Button href="/">Sign in</Button>
		</div>
	{:else if !token}
		<p class="notice" role="note">This reset link is incomplete. Copy the whole link you were sent, or ask the owner for a new one.</p>
	{:else}
		<form class="auth-form" onsubmit={handleSubmit}>
			<div class="field">
				<Label for="reset-password">New password</Label>
				<Input id="reset-password" name="password" type="password" autocomplete="new-password" required minlength={MIN_PASSWORD} maxlength={256} bind:value={password} aria-describedby="reset-password-hint" />
				<p id="reset-password-hint" class="hint">At least {MIN_PASSWORD} characters.</p>
			</div>
			<div class="field">
				<Label for="reset-confirm">New password again</Label>
				<Input id="reset-confirm" name="confirm" type="password" autocomplete="new-password" required bind:value={confirm} aria-describedby={error ? 'reset-error' : undefined} />
			</div>
			{#if error}
				<ActionError id="reset-error" message={error} />
			{/if}
			<Button type="submit" disabled={submitting}>{submitting ? 'Saving...' : 'Set password'}</Button>
		</form>
	{/if}
</AuthScreen>

<style>
	.auth-form,
	.done {
		display: flex;
		flex-direction: column;
		gap: 1rem;
	}

	.done p {
		margin: 0;
	}

	.field {
		display: flex;
		flex-direction: column;
		gap: 0.375rem;
	}

	.hint,
	.notice {
		margin: 0;
		font-size: 0.8125rem;
		color: var(--color-text-muted);
	}

	.notice {
		font-size: 0.875rem;
	}
</style>
