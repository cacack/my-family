<script lang="ts">
	import * as DropdownMenu from '$lib/components/ui/dropdown-menu';
	import { Button } from '$lib/components/ui/button';
	import {
		mainNav,
		isGroup,
		isEntryActive,
		isLinkActive,
		type NavLink
	} from '$lib/navigation';

	interface Props {
		/** The current page's path; decides which section is highlighted. */
		pathname: string;
	}

	let { pathname }: Props = $props();

	const triggerClass = 'gap-1 px-3 py-2 text-sm font-medium';
	const activeClass = 'bg-[#eff6ff] text-[#1d4ed8]';
	const idleClass = 'text-[#64748b] hover:bg-[#f1f5f9] hover:text-[#1e293b]';
</script>

<!-- `child` on Triggers and Items for the reasons given in BranchSwitcher.svelte:
     no nested buttons, and Enter / whole-row clicks actually navigate. -->
{#snippet chevron()}
	<svg class="size-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true">
		<polyline points="6 9 12 15 18 9" />
	</svg>
{/snippet}

{#snippet menuLink(link: NavLink)}
	{@const active = isLinkActive(link, pathname)}
	<DropdownMenu.Item>
		{#snippet child({ props })}
			<a
				href={link.href}
				{...props}
				class="{props.class} {active ? 'font-semibold text-[#1d4ed8]' : ''}"
				aria-current={active ? 'page' : undefined}>{link.label}</a
			>
		{/snippet}
	</DropdownMenu.Item>
{/snippet}

<nav class="main-nav" aria-label="Main navigation">
	<!-- Wide screens: top-level links and one menu per group, on one row. -->
	<div class="nav-wide">
		{#each mainNav as entry (entry.label)}
			{@const active = isEntryActive(entry, pathname)}
			{#if isGroup(entry)}
				<DropdownMenu.Root>
					<DropdownMenu.Trigger>
						{#snippet child({ props })}
							<Button {...props} variant="ghost" class="{triggerClass} {active ? activeClass : idleClass}">
								{entry.label}
								{@render chevron()}
							</Button>
						{/snippet}
					</DropdownMenu.Trigger>
					<DropdownMenu.Content class="w-52">
						{#each entry.links as link (link.href)}
							{@render menuLink(link)}
						{/each}
					</DropdownMenu.Content>
				</DropdownMenu.Root>
			{:else}
				<a
					href={entry.href}
					class="nav-link {active ? activeClass : idleClass}"
					aria-current={active ? 'page' : undefined}>{entry.label}</a
				>
			{/if}
		{/each}
	</div>

	<!-- Narrow screens: the same entries in one sectioned menu, so every
	     destination is two taps away and nothing scrolls sideways. -->
	<div class="nav-narrow">
		<DropdownMenu.Root>
			<DropdownMenu.Trigger>
				{#snippet child({ props })}
					<Button {...props} variant="ghost" class="{triggerClass} {idleClass}">
						<svg class="size-5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true">
							<line x1="4" y1="6" x2="20" y2="6" />
							<line x1="4" y1="12" x2="20" y2="12" />
							<line x1="4" y1="18" x2="20" y2="18" />
						</svg>
						Menu
					</Button>
				{/snippet}
			</DropdownMenu.Trigger>
			<DropdownMenu.Content class="max-h-(--bits-dropdown-menu-content-available-height) w-64">
				{#each mainNav as entry, i (entry.label)}
					<!-- Separators fence each group off, so a plain link after one
					     does not read as part of it. -->
					{#if i > 0 && (isGroup(entry) || isGroup(mainNav[i - 1]))}
						<DropdownMenu.Separator />
					{/if}
					{#if isGroup(entry)}
						<DropdownMenu.Group>
							<DropdownMenu.GroupHeading class="text-xs font-medium text-[#64748b]">{entry.label}</DropdownMenu.GroupHeading>
							{#each entry.links as link (link.href)}
								{@render menuLink(link)}
							{/each}
						</DropdownMenu.Group>
					{:else}
						{@render menuLink(entry)}
					{/if}
				{/each}
			</DropdownMenu.Content>
		</DropdownMenu.Root>
	</div>
</nav>

<style>
	.nav-wide {
		display: none;
		gap: 0.25rem;
		align-items: center;
	}

	.nav-narrow {
		display: flex;
	}

	@media (min-width: 1024px) {
		.nav-wide {
			display: flex;
		}

		.nav-narrow {
			display: none;
		}
	}

	.nav-link {
		padding: 0.5rem 0.75rem;
		border-radius: 6px;
		text-decoration: none;
		font-size: 0.875rem;
		font-weight: 500;
		white-space: nowrap;
		transition: all 0.15s;
	}

	.nav-link:focus-visible {
		outline: 2px solid #3b82f6;
		outline-offset: 2px;
	}

	:global(body.high-contrast) .nav-link {
		color: var(--color-text-muted);
	}

	:global(body.high-contrast) .nav-link:hover {
		background: var(--a11y-color-border);
		color: var(--color-text);
	}

	:global(body.high-contrast) .nav-link[aria-current='page'] {
		background: var(--color-focus-ring);
		color: #000;
	}

	:global(body.high-contrast) .nav-link:focus-visible {
		outline-color: var(--color-focus-ring);
	}
</style>
