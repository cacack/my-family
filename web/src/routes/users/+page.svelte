<script lang="ts">
	/**
	 * Users (owner only, ADR-007): invite someone with a role, change a role,
	 * disable or re-enable an account, issue a password reset link, and revoke
	 * a pending link. No email is sent: an invite or reset link is shown once,
	 * to copy and pass on. The server keeps the tree's last active owner from
	 * being demoted or disabled, and says so here.
	 */
	import PageHeader from '#lib/components/PageHeader.svelte';
	import ActionError from '#lib/components/ActionError.svelte';
	import ErrorState from '#lib/components/ErrorState.svelte';
	import LoadingState from '#lib/components/LoadingState.svelte';
	import LiveRegion from '#lib/components/LiveRegion.svelte';
	import ConfirmDialog from '#lib/components/ConfirmDialog.svelte';
	import RoleNotice from '#lib/components/auth/RoleNotice.svelte';
	import AccountLinkBox from '#lib/components/auth/AccountLinkBox.svelte';
	import PendingLinks from '#lib/components/auth/PendingLinks.svelte';
	import { Badge } from '#lib/components/ui/badge/index.js';
	import { Button } from '#lib/components/ui/button/index.js';
	import { Label } from '#lib/components/ui/label/index.js';
	import { api, type AccountLink, type ApiError, type User } from '#lib/api/client.js';
	import { auth, can, type Role } from '#lib/stores/auth.svelte.js';
	import { inviteLink, resetLink } from '#lib/utils/accountLinks.js';

	const ROLES: { value: Role; label: string }[] = [
		{ value: 'viewer', label: 'Viewer: reads the tree' },
		{ value: 'editor', label: 'Editor: also changes it and exports' },
		{ value: 'owner', label: 'Owner: also manages users and imports' }
	];

	let liveRegion: LiveRegion | undefined = $state();
	let pendingLinks: PendingLinks | undefined = $state();

	let users: User[] = $state([]);
	let loading = $state(true);
	let loadError: string | null = $state(null);

	let inviteRole: Role = $state('viewer');
	let inviting = $state(false);
	let inviteError: string | null = $state(null);
	let invite: { link: AccountLink; role: Role } | null = $state(null);

	/** Per user: a refused change, and a reset link just issued. */
	let rowErrors: Record<string, string> = $state({});
	let resets: Record<string, AccountLink> = $state({});
	let busy: Record<string, boolean> = $state({});

	let disableOpen = $state(false);
	let disableTarget: User | null = $state(null);

	const managing = $derived(auth.account !== null && can('owner'));

	async function loadUsers() {
		loading = true;
		try {
			users = await api.listUsers();
			loadError = null;
		} catch (e) {
			const apiError = e as ApiError;
			loadError =
				apiError.status && apiError.status < 500
					? apiError.message
					: 'Failed to load users. Please try again.';
		} finally {
			loading = false;
		}
	}

	$effect(() => {
		if (managing) loadUsers();
	});

	function replace(updated: User) {
		users = users.map((u) => (u.id === updated.id ? updated : u));
	}

	async function createInvite(event: SubmitEvent) {
		event.preventDefault();
		inviting = true;
		inviteError = null;
		try {
			invite = { link: await api.createInvite(inviteRole), role: inviteRole };
			liveRegion?.announce(`Invite link for a new ${inviteRole} created`);
			pendingLinks?.reload();
		} catch (e) {
			inviteError = (e as ApiError).message || 'Could not create the invite. Try again.';
		} finally {
			inviting = false;
		}
	}

	async function update(user: User, change: Partial<Pick<User, 'role' | 'disabled'>>, done: string) {
		busy[user.id] = true;
		delete rowErrors[user.id];
		try {
			replace(await api.updateUser(user.id, change));
			liveRegion?.announce(done);
		} catch (e) {
			rowErrors[user.id] = (e as ApiError).message || 'Could not change this user. Try again.';
			throw e;
		} finally {
			busy[user.id] = false;
		}
	}

	async function changeRole(user: User, event: Event) {
		const select = event.currentTarget as HTMLSelectElement;
		const role = select.value as Role;
		if (role === user.role) return;
		try {
			await update(user, { role }, `${user.display_name} is now ${role === 'editor' ? 'an' : 'a'} ${role}`);
		} catch {
			// Refused (the last owner): show the role the server still holds.
			select.value = user.role;
		}
	}

	function openDisable(user: User) {
		disableTarget = user;
		disableOpen = true;
	}

	async function confirmDisable() {
		const user = disableTarget;
		if (!user) return;
		try {
			await update(user, { disabled: true }, `${user.display_name}'s account disabled`);
		} catch {
			// The refusal shows on the user's row.
		}
	}

	async function issueReset(user: User) {
		busy[user.id] = true;
		delete rowErrors[user.id];
		try {
			resets[user.id] = await api.createPasswordReset(user.id);
			liveRegion?.announce(`Password reset link for ${user.display_name} created`);
			pendingLinks?.reload();
		} catch (e) {
			rowErrors[user.id] = (e as ApiError).message || 'Could not create the reset link. Try again.';
		} finally {
			busy[user.id] = false;
		}
	}
</script>

<svelte:head>
	<title>Users | My Family</title>
</svelte:head>

<LiveRegion bind:this={liveRegion} />

<div class="users-page">
	<PageHeader title="Users" description="Who can use this tree, and what each of them may do.">
		{#snippet actions()}
			{#if managing}<Button variant="outline" href="/users/audit">Audit log</Button>{/if}
		{/snippet}
	</PageHeader>

	{#if auth.phase === 'open'}
		<p class="notice" role="note">
			This server runs without sign-in (AUTH_MODE=open), so there are no users to manage.
		</p>
	{:else if !auth.account || !managing}
		<RoleNotice action="Managing users" required="owner" />
	{:else}
		<section class="panel" aria-labelledby="invite-heading">
			<h2 id="invite-heading">Invite someone</h2>
			<form class="invite-form" onsubmit={createInvite}>
				<div class="field">
					<Label for="invite-role">Role</Label>
					<select id="invite-role" class="native-select" bind:value={inviteRole}>
						{#each ROLES as role (role.value)}
							<option value={role.value}>{role.label}</option>
						{/each}
					</select>
				</div>
				<Button type="submit" disabled={inviting}>{inviting ? 'Creating...' : 'Create invite link'}</Button>
			</form>
			{#if inviteError}
				<ActionError message={inviteError} />
			{/if}
			{#if invite}
				<AccountLinkBox
					id="invite-link"
					label="Invite link ({invite.role})"
					url={inviteLink(invite.link.token)}
					expiresAt={invite.link.expires_at}
				/>
			{/if}
		</section>

		{#if loading}
			<LoadingState label="Loading users..." />
		{:else if loadError}
			<ErrorState message={loadError} onRetry={loadUsers} />
		{:else}
			<ul class="user-list" aria-label="Users">
				{#each users as user (user.id)}
					{@const self = user.id === auth.account.id}
					<li class="user-card" data-testid="user-row">
						<div class="user-head">
							<div class="user-name">
								<span class="display-name">{user.display_name}</span>
								<span class="username">{user.username}{self ? ' (you)' : ''}</span>
							</div>
							{#if user.disabled}<Badge variant="destructive">Disabled</Badge>{/if}
						</div>
						<div class="user-actions">
							<div class="field role-field">
								<Label for="role-{user.id}">Role</Label>
								<select
									id="role-{user.id}"
									class="native-select"
									value={user.role}
									disabled={busy[user.id]}
									onchange={(e) => changeRole(user, e)}
								>
									{#each ROLES as role (role.value)}
										<option value={role.value}>{role.value}</option>
									{/each}
								</select>
							</div>
							<div class="buttons">
								{#if user.disabled}
									<Button
										variant="outline"
										size="sm"
										disabled={busy[user.id]}
										onclick={() => update(user, { disabled: false }, `${user.display_name}'s account re-enabled`).catch(() => {})}
										aria-label="Re-enable {user.display_name}"
									>
										Re-enable
									</Button>
								{:else}
									<Button
										variant="destructive"
										size="sm"
										disabled={busy[user.id]}
										onclick={() => openDisable(user)}
										aria-label="Disable {user.display_name}"
									>
										Disable
									</Button>
								{/if}
								<Button
									variant="outline"
									size="sm"
									disabled={busy[user.id]}
									onclick={() => issueReset(user)}
									aria-label="Reset password for {user.display_name}"
								>
									Reset password
								</Button>
							</div>
						</div>
						{#if rowErrors[user.id]}
							<ActionError message={rowErrors[user.id]} />
						{/if}
						{#if resets[user.id]}
							<AccountLinkBox
								id="reset-link-{user.id}"
								label="Password reset link for {user.display_name}"
								url={resetLink(resets[user.id].token)}
								expiresAt={resets[user.id].expires_at}
							/>
						{/if}
					</li>
				{/each}
			</ul>
		{/if}

		<PendingLinks bind:this={pendingLinks} announce={(message) => liveRegion?.announce(message)} />
	{/if}
</div>

<ConfirmDialog
	bind:open={disableOpen}
	title="Disable {disableTarget?.display_name ?? 'this user'}?"
	description="They are signed out at once and cannot sign in until you re-enable the account. Their changes stay in the tree's history."
	confirmLabel="Disable"
	busyLabel="Disabling..."
	onconfirm={confirmDisable}
/>

<style>
	.users-page {
		max-width: var(--page-width-list);
		margin: 0 auto;
		padding: 1.5rem;
	}

	.panel,
	.user-card {
		display: flex;
		flex-direction: column;
		gap: 0.75rem;
		background: white;
		border: 1px solid #e2e8f0;
		border-radius: 8px;
		padding: 1rem;
	}

	:global(body.high-contrast) .panel,
	:global(body.high-contrast) .user-card {
		background: var(--color-bg);
		border-color: var(--a11y-color-border);
	}

	.panel {
		margin-bottom: 1.5rem;
	}

	.panel h2 {
		margin: 0;
		font-size: 0.8125rem;
		font-weight: 600;
		text-transform: uppercase;
		letter-spacing: 0.05em;
		color: var(--color-text-muted);
	}

	.invite-form,
	.user-actions {
		display: flex;
		align-items: flex-end;
		gap: 0.75rem;
		flex-wrap: wrap;
	}

	.field {
		display: flex;
		flex-direction: column;
		gap: 0.375rem;
		min-width: 0;
	}

	.native-select {
		height: 2.25rem;
		padding: 0 0.5rem;
		border: 1px solid #cbd5e1;
		border-radius: 6px;
		background: white;
		font-size: 0.875rem;
		color: #1e293b;
		max-width: 100%;
	}

	.native-select:focus-visible {
		outline: 2px solid var(--color-focus-ring);
		outline-offset: 1px;
	}

	.user-list {
		list-style: none;
		margin: 0 0 1.5rem;
		padding: 0;
		display: flex;
		flex-direction: column;
		gap: 0.75rem;
	}

	.user-head {
		display: flex;
		align-items: flex-start;
		justify-content: space-between;
		gap: 0.5rem;
	}

	.user-name {
		display: flex;
		flex-direction: column;
		min-width: 0;
	}

	.display-name {
		font-weight: 600;
		overflow-wrap: anywhere;
	}

	.username {
		font-size: 0.8125rem;
		color: var(--color-text-muted);
		overflow-wrap: anywhere;
	}

	.buttons {
		display: flex;
		gap: 0.5rem;
		flex-wrap: wrap;
	}

	.notice {
		margin: 0;
		font-size: 0.875rem;
		color: var(--color-text-muted);
	}
</style>
