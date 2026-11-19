<script lang="ts">
	/** The signed-in user's menu in the header: who, and sign out (ADR-007). */
	import * as DropdownMenu from '#lib/components/ui/dropdown-menu/index.js';
	import { auth, signOut } from '#lib/stores/auth.svelte.js';

	const name = $derived(auth.account?.display_name ?? '');
</script>

{#if auth.account}
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

	.account-btn svg {
		width: 1.25rem;
		height: 1.25rem;
	}
</style>
