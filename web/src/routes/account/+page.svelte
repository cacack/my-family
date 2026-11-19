<script lang="ts">
	/**
	 * The signed-in user's own account (ADR-007): their display name, shown in
	 * history, and their password. A new password signs them out everywhere
	 * else; this browser stays signed in.
	 */
	import PageHeader from '#lib/components/PageHeader.svelte';
	import ActionError from '#lib/components/ActionError.svelte';
	import LiveRegion from '#lib/components/LiveRegion.svelte';
	import InfoSection from '#lib/components/InfoSection.svelte';
	import { Button } from '#lib/components/ui/button/index.js';
	import { Input } from '#lib/components/ui/input/index.js';
	import { Label } from '#lib/components/ui/label/index.js';
	import { auth, updateOwnAccount } from '#lib/stores/auth.svelte.js';
	import type { ApiError } from '#lib/api/client.js';

	const MIN_PASSWORD = 8;

	let liveRegion: LiveRegion | undefined = $state();

	let displayName = $state(auth.account?.display_name ?? '');
	let savingName = $state(false);
	let nameError = $state<string | null>(null);

	let currentPassword = $state('');
	let newPassword = $state('');
	let confirm = $state('');
	let savingPassword = $state(false);
	let passwordError = $state<string | null>(null);

	async function saveName(event: SubmitEvent) {
		event.preventDefault();
		nameError = null;
		savingName = true;
		try {
			await updateOwnAccount({ display_name: displayName.trim() });
			displayName = auth.account?.display_name ?? displayName;
			liveRegion?.announce('Display name saved');
		} catch (e) {
			nameError = (e as ApiError).message || 'Could not save the display name. Try again.';
		} finally {
			savingName = false;
		}
	}

	async function savePassword(event: SubmitEvent) {
		event.preventDefault();
		passwordError = null;
		if (newPassword !== confirm) {
			passwordError = 'The new passwords do not match.';
			return;
		}
		savingPassword = true;
		try {
			await updateOwnAccount({ current_password: currentPassword, new_password: newPassword });
			currentPassword = newPassword = confirm = '';
			liveRegion?.announce('Password changed. Your other sessions were signed out.');
		} catch (e) {
			passwordError = (e as ApiError).message || 'Could not change the password. Try again.';
		} finally {
			savingPassword = false;
		}
	}
</script>

<svelte:head>
	<title>Account | My Family</title>
</svelte:head>

<LiveRegion bind:this={liveRegion} />

<div class="account-page">
	<PageHeader title="Account" description="Your name in the tree's history, and the password you sign in with." />

	{#if !auth.account}
		<p class="notice" role="note">
			This server runs without sign-in (AUTH_MODE=open), so there are no accounts.
		</p>
	{:else}
		<InfoSection
			title="Signed in as"
			fields={[
				{ label: 'Username', value: auth.account.username },
				{ label: 'Role', value: auth.account.role }
			]}
		/>

		<section class="panel" aria-labelledby="name-heading">
			<h2 id="name-heading">Name</h2>
			<form class="panel-form" onsubmit={saveName}>
				<div class="field">
					<Label for="account-display-name">Display name</Label>
					<Input id="account-display-name" autocomplete="name" required maxlength={100} bind:value={displayName} aria-describedby={nameError ? 'account-name-error' : undefined} />
				</div>
				{#if nameError}
					<ActionError id="account-name-error" message={nameError} />
				{/if}
				<div>
					<Button type="submit" disabled={savingName || displayName.trim() === auth.account.display_name}>
						{savingName ? 'Saving...' : 'Save name'}
					</Button>
				</div>
			</form>
		</section>

		<section class="panel" aria-labelledby="password-heading">
			<h2 id="password-heading">Password</h2>
			<form class="panel-form" onsubmit={savePassword}>
				<div class="field">
					<Label for="account-current-password">Current password</Label>
					<Input id="account-current-password" type="password" autocomplete="current-password" required bind:value={currentPassword} />
				</div>
				<div class="field">
					<Label for="account-new-password">New password</Label>
					<Input id="account-new-password" type="password" autocomplete="new-password" required minlength={MIN_PASSWORD} maxlength={256} bind:value={newPassword} aria-describedby="account-new-password-hint" />
					<p id="account-new-password-hint" class="hint">
						At least {MIN_PASSWORD} characters. Changing it signs you out on every other device.
					</p>
				</div>
				<div class="field">
					<Label for="account-confirm">New password again</Label>
					<Input id="account-confirm" type="password" autocomplete="new-password" required bind:value={confirm} aria-describedby={passwordError ? 'account-password-error' : undefined} />
				</div>
				{#if passwordError}
					<ActionError id="account-password-error" message={passwordError} />
				{/if}
				<div>
					<Button type="submit" disabled={savingPassword}>
						{savingPassword ? 'Saving...' : 'Change password'}
					</Button>
				</div>
			</form>
		</section>
	{/if}
</div>

<style>
	.account-page {
		max-width: var(--page-width-detail);
		margin: 0 auto;
		padding: 1.5rem;
	}

	.panel {
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

	.panel h2 {
		margin: 0 0 0.75rem;
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
	.notice {
		margin: 0;
		font-size: 0.8125rem;
		color: var(--color-text-muted);
	}
</style>
