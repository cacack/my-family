<script lang="ts">
	/**
	 * The page an invite link opens (`/join#token=...`): choose a username and
	 * password, and the account gets the role the owner chose (ADR-007). There
	 * is no other way to create an account here.
	 */
	import { goto } from '$app/navigation';
	import AuthScreen from './AuthScreen.svelte';
	import ActionError from '#lib/components/ActionError.svelte';
	import { Button } from '#lib/components/ui/button/index.js';
	import { Input } from '#lib/components/ui/input/index.js';
	import { Label } from '#lib/components/ui/label/index.js';
	import { acceptInvite, auth } from '#lib/stores/auth.svelte.js';
	import type { ApiError } from '#lib/api/client.js';

	const MIN_PASSWORD = 8;

	let { token }: { token: string } = $props();

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
			await acceptInvite({ token, username, display_name: displayName.trim() || username, password });
			await goto('/');
		} catch (e) {
			const apiError = e as ApiError;
			error = apiError.message || 'Could not create the account. Try again.';
		} finally {
			submitting = false;
		}
	}
</script>

<AuthScreen title="Join My Family" description="You were invited to this family tree. Choose how you will sign in.">
	{#if auth.phase === 'open'}
		<p class="notice" role="note">This server runs without sign-in, so there is nothing to join. <a href="/">Open the tree</a>.</p>
	{:else if !token}
		<p class="notice" role="note">This invite link is incomplete. Copy the whole link the owner sent you, or ask them for a new one.</p>
	{:else}
		<form class="auth-form" onsubmit={handleSubmit}>
			<div class="field">
				<Label for="join-username">Username</Label>
				<Input id="join-username" name="username" autocomplete="username" autocapitalize="none" required maxlength={64} bind:value={username} />
			</div>
			<div class="field">
				<Label for="join-display-name">Display name</Label>
				<Input id="join-display-name" name="display_name" autocomplete="name" maxlength={100} bind:value={displayName} aria-describedby="join-display-name-hint" />
				<p id="join-display-name-hint" class="hint">Shown in the history of your changes. Defaults to the username.</p>
			</div>
			<div class="field">
				<Label for="join-password">Password</Label>
				<Input id="join-password" name="password" type="password" autocomplete="new-password" required minlength={MIN_PASSWORD} maxlength={256} bind:value={password} aria-describedby="join-password-hint" />
				<p id="join-password-hint" class="hint">At least {MIN_PASSWORD} characters.</p>
			</div>
			<div class="field">
				<Label for="join-confirm">Password again</Label>
				<Input id="join-confirm" name="confirm" type="password" autocomplete="new-password" required bind:value={confirm} aria-describedby={error ? 'join-error' : undefined} />
			</div>
			{#if error}
				<ActionError id="join-error" message={error} />
			{/if}
			<Button type="submit" disabled={submitting}>{submitting ? 'Creating...' : 'Create account and sign in'}</Button>
		</form>
	{/if}
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
