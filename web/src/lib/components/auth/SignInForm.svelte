<script lang="ts">
	/** The sign-in page: username and password (ADR-007). */
	import AuthScreen from './AuthScreen.svelte';
	import ActionError from '#lib/components/ActionError.svelte';
	import { Button } from '#lib/components/ui/button/index.js';
	import { Input } from '#lib/components/ui/input/index.js';
	import { Label } from '#lib/components/ui/label/index.js';
	import { auth, browseAsVisitor, signIn } from '#lib/stores/auth.svelte.js';
	import type { ApiError } from '#lib/api/client.js';

	let username = $state('');
	let password = $state('');
	let submitting = $state(false);
	let error = $state<string | null>(null);

	async function handleSubmit(event: SubmitEvent) {
		event.preventDefault();
		submitting = true;
		error = null;
		try {
			await signIn(username, password);
		} catch (e) {
			const apiError = e as ApiError;
			error = apiError.message || 'Could not sign in. Try again.';
			password = '';
		} finally {
			submitting = false;
		}
	}
</script>

<AuthScreen title="Sign in" description="Sign in to see and edit the family tree.">
	<form class="auth-form" onsubmit={handleSubmit}>
		<div class="field">
			<Label for="signin-username">Username</Label>
			<Input id="signin-username" name="username" autocomplete="username" autocapitalize="none" required bind:value={username} />
		</div>
		<div class="field">
			<Label for="signin-password">Password</Label>
			<Input
				id="signin-password"
				name="password"
				type="password"
				autocomplete="current-password"
				required
				bind:value={password}
				aria-describedby={error ? 'signin-error' : undefined}
			/>
		</div>
		{#if error}
			<ActionError id="signin-error" message={error} />
		{/if}
		<Button type="submit" disabled={submitting}>{submitting ? 'Signing in...' : 'Sign in'}</Button>
		{#if auth.visitorsWelcome}
			<Button type="button" variant="ghost" onclick={() => browseAsVisitor()}>Browse without signing in</Button>
		{/if}
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
</style>
