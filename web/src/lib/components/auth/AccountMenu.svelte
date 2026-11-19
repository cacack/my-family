<script lang="ts">
	/**
	 * The signed-in user's menu in the header (ADR-007): who, their account
	 * page, the users and settings pages for an owner, and sign out. A
	 * visitor gets a sign-in button in its place. Open mode has no account
	 * and everyone is the owner, so it gets a link to the settings page.
	 */
	import { goto } from '$app/navigation';
	import * as DropdownMenu from '#lib/components/ui/dropdown-menu/index.js';
	import { auth, can, showSignIn, signOut } from '#lib/stores/auth.svelte.js';

	const name = $derived(auth.account?.display_name ?? '');
</script>

{#if auth.phase === 'visitor'}
	<button type="button" class="sign-in-btn" onclick={() => showSignIn()}>Sign in</button>
{:else if auth.phase === 'open'}
	<a href="/settings" class="account-btn" data-slot="action-link" aria-label="Settings" title="Settings">
		<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true">
			<circle cx="12" cy="12" r="3" />
			<path
				d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 1 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 1 1-4 0v-.09a1.65 1.65 0 0 0-1.08-1.51 1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 1 1-2.83-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 1 1 0-4h.09a1.65 1.65 0 0 0 1.51-1.08 1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 1 1 2.83-2.83l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 1 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 1 1 2.83 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 1 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z"
			/>
		</svg>
	</a>
{:else if auth.account}
	<DropdownMenu.Root>
		<DropdownMenu.Trigger>
			{#snippet child({ props })}
				<button {...props} class="account-btn" aria-label="Account: {name}" title={name}>
					<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true">
						<circle cx="12" cy="8" r="4" />
						<path d="M4 21c0-4 4-6 8-6s8 2 8 6" />
					</svg>
				</button>
			{/snippet}
		</DropdownMenu.Trigger>
		<DropdownMenu.Content class="w-56" align="end">
			<DropdownMenu.Label>
				<span class="block truncate">{name}</span>
				<span class="block truncate text-xs font-normal text-muted-foreground">{auth.account.username}</span>
			</DropdownMenu.Label>
			<DropdownMenu.Separator />
			<DropdownMenu.Item onSelect={() => goto('/account')}>Account</DropdownMenu.Item>
			{#if can('owner')}
				<DropdownMenu.Item onSelect={() => goto('/users')}>Users</DropdownMenu.Item>
				<DropdownMenu.Item onSelect={() => goto('/settings')}>Settings</DropdownMenu.Item>
			{/if}
			<DropdownMenu.Separator />
			<DropdownMenu.Item onSelect={() => signOut()}>Sign out</DropdownMenu.Item>
		</DropdownMenu.Content>
	</DropdownMenu.Root>
{/if}

<style>
	.account-btn {
		display: flex;
		align-items: center;
		justify-content: center;
		flex-shrink: 0;
		width: 2.25rem;
		height: 2.25rem;
		padding: 0;
		border: 1px solid #e2e8f0;
		border-radius: 6px;
		background: white;
		color: #64748b;
		cursor: pointer;
	}

	.account-btn:hover {
		background: #f1f5f9;
		color: #1e293b;
	}

	.account-btn:focus-visible {
		outline: 2px solid var(--color-focus-ring);
		outline-offset: 2px;
	}

	:global(body.high-contrast) .account-btn {
		background: var(--color-bg);
		border-color: var(--a11y-color-border);
		color: var(--color-text);
	}

	.sign-in-btn {
		flex-shrink: 0;
		height: 2.25rem;
		padding: 0 0.75rem;
		border: 1px solid #e2e8f0;
		border-radius: 6px;
		background: white;
		color: #1e293b;
		font-size: 0.875rem;
		font-weight: 500;
		white-space: nowrap;
		cursor: pointer;
	}

	.sign-in-btn:hover {
		background: #f1f5f9;
	}

	.sign-in-btn:focus-visible {
		outline: 2px solid var(--color-focus-ring);
		outline-offset: 2px;
	}

	:global(body.high-contrast) .sign-in-btn {
		background: var(--color-bg);
		border-color: var(--a11y-color-border);
		color: var(--color-text);
	}

	.account-btn svg {
		width: 1.25rem;
		height: 1.25rem;
	}
</style>
