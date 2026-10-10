<script lang="ts">
	import '../app.css';
	import favicon from '$lib/assets/favicon.svg';
	import logoMark from '$lib/assets/favicon.svg?raw';
	import { goto } from '$app/navigation';
	import { page } from '$app/stores';
	import SearchBox from '$lib/components/SearchBox.svelte';
	import KeyboardHelp from '$lib/components/KeyboardHelp.svelte';
	import AccessibilityPanel from '$lib/components/AccessibilityPanel.svelte';
	import DemoBanner from '$lib/components/DemoBanner.svelte';
	import BranchBanner from '$lib/components/BranchBanner.svelte';
	import BranchSwitcher from '$lib/components/BranchSwitcher.svelte';
	import MainNav from '$lib/components/MainNav.svelte';
	import TooltipProvider from '$lib/components/ui/tooltip/tooltip-provider.svelte';
	import { createShortcutHandler } from '$lib/keyboard/useShortcuts.svelte';
	import { loadAppConfig, getAppConfig } from '$lib/stores/appConfig.svelte';
	import { revalidateActiveBranch } from '$lib/stores/activeBranch.svelte';
	import type { SearchResult } from '$lib/api/client';

	let { children } = $props();

	const appConfig = getAppConfig();

	$effect(() => {
		loadAppConfig();
	});

	// Separate effect: revalidation reads the active branch id, so folding it in
	// above would re-run the config fetch whenever the branch changes.
	// A persisted branch that is gone or terminal must fall back to the mainline
	// before the user makes a write believing otherwise.
	$effect(() => {
		revalidateActiveBranch();
	});

	// Component refs
	let searchBoxRef: SearchBox | undefined = $state();

	// Panel states
	let helpOpen = $state(false);
	let accessibilityPanelOpen = $state(false);

	function handleSearchSelect(person: SearchResult) {
		goto(`/persons/${person.id}`);
	}

	// Global keyboard shortcuts
	const { handleKeydown } = createShortcutHandler('global', {
		'go-home': () => goto('/'),
		'go-people': () => goto('/persons'),
		'go-families': () => goto('/families'),
		'go-sources': () => goto('/sources'),
		'go-evidence': () => goto('/evidence'),
		'go-snapshots': () => goto('/snapshots'),
		'go-search': () => goto('/search'),
		'focus-search': () => searchBoxRef?.focus(),
		'show-help': () => {
			helpOpen = !helpOpen;
		},
		'close-modal': () => {
			if (helpOpen) {
				helpOpen = false;
			} else if (accessibilityPanelOpen) {
				accessibilityPanelOpen = false;
			}
		}
	});
</script>

<svelte:head>
	<link rel="icon" href={favicon} />
</svelte:head>

<svelte:window onkeydown={handleKeydown} />

<!-- Skip link for keyboard navigation -->
<a
	href="#main-content"
	class="sr-only focus:not-sr-only focus:absolute focus:top-4 focus:left-4 focus:z-50 focus:bg-white focus:px-4 focus:py-2 focus:rounded focus:shadow-lg focus:outline-2 focus:outline-blue-500"
>
	Skip to main content
</a>

{#if appConfig.demo_mode}
	<DemoBanner />
{/if}

<BranchBanner />

<div class="app-layout">
	<header class="app-header">
		<a href="/" class="logo">
			<span class="logo-mark">{@html logoMark}</span>
			<span class="logo-text">My Family</span>
		</a>
		<MainNav pathname={$page.url.pathname} />
		<div class="header-controls">
			<BranchSwitcher />
			<div class="search-wrapper">
				<SearchBox bind:this={searchBoxRef} onSelect={handleSearchSelect} placeholder="Search people..." />
			</div>
			<!-- The search box finds people by name; this is the one way into the
			     full search, so it is not also a nav link. -->
			<a
				href="/search"
				class="icon-btn"
				class:active={$page.url.pathname === '/search'}
				aria-current={$page.url.pathname === '/search' ? 'page' : undefined}
				aria-label="Advanced search"
				title="Advanced search"
			>
				<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true">
					<path d="M4 6h3M11 6h9M4 12h9M17 12h3M4 18h2M10 18h10" />
					<circle cx="9" cy="6" r="2" />
					<circle cx="15" cy="12" r="2" />
					<circle cx="8" cy="18" r="2" />
				</svg>
			</a>
			<button
				class="icon-btn"
				onclick={() => accessibilityPanelOpen = true}
				aria-label="Accessibility settings"
				title="Accessibility settings"
			>
				<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
					<circle cx="12" cy="12" r="10" />
					<circle cx="12" cy="8" r="2" />
					<path d="M12 10v6" />
					<path d="M8 14l4-2 4 2" />
					<path d="M9 18l3-4 3 4" />
				</svg>
			</button>
		</div>
	</header>
	<main id="main-content" class="app-main" tabindex="-1">
		<TooltipProvider>
			{@render children()}
		</TooltipProvider>
	</main>
</div>

<!-- Modals/Overlays -->
<KeyboardHelp bind:open={helpOpen} onClose={() => helpOpen = false} />
<AccessibilityPanel bind:open={accessibilityPanelOpen} onClose={() => accessibilityPanelOpen = false} />

<style>
	:global(*, *::before, *::after) {
		box-sizing: border-box;
	}

	:global(body) {
		margin: 0;
		font-family:
			-apple-system,
			BlinkMacSystemFont,
			'Segoe UI',
			Roboto,
			Oxygen,
			Ubuntu,
			sans-serif;
		background: #f8fafc;
		color: #1e293b;
	}

	/* Accessibility class styles */
	:global(body.high-contrast) {
		--color-bg: #000;
		--color-bg-secondary: #1a1a1a;
		--color-text: #fff;
		--color-text-muted: #ccc;
		--a11y-color-border: #666;
		--color-focus-ring: #ffff00;
		background: var(--color-bg);
		color: var(--color-text);
	}

	:global(body.reduced-motion *),
	:global(body.reduced-motion *::before),
	:global(body.reduced-motion *::after) {
		animation-duration: 0.01ms !important;
		animation-iteration-count: 1 !important;
		transition-duration: 0.01ms !important;
	}

	.app-layout {
		min-height: 100vh;
		display: flex;
		flex-direction: column;
	}

	.app-header {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 0.5rem 1rem;
		padding: 0.75rem 1.5rem;
		background: white;
		border-bottom: 1px solid #e2e8f0;
	}

	:global(body.high-contrast) .app-header {
		background: var(--color-bg-secondary);
		border-bottom-color: var(--a11y-color-border);
	}

	.logo {
		font-size: 1.25rem;
		font-weight: 700;
		color: #1F2933;
		text-decoration: none;
		display: flex;
		align-items: center;
		gap: 0.5rem;
		white-space: nowrap;
	}

	.logo-mark {
		display: flex;
		width: 28px;
		height: 28px;
	}

	.logo-mark :global(svg) {
		width: 100%;
		height: 100%;
	}

	/* Between the width where the grouped nav appears and the width where the
	   wordmark fits beside it, the mark alone keeps the header on one row. */
	@media (min-width: 1024px) and (max-width: 1279px) {
		.logo-text {
			position: absolute;
			width: 1px;
			height: 1px;
			overflow: hidden;
			clip: rect(0, 0, 0, 0);
			white-space: nowrap;
		}
	}

	:global(body.high-contrast) .logo {
		color: var(--color-text);
	}

	.header-controls {
		display: flex;
		align-items: center;
		gap: 0.75rem;
		margin-left: auto;
		/* On narrow screens the controls wrap below the logo and menu. */
		flex-wrap: wrap;
	}

	/* On a phone the search box takes a full row of its own under the other
	   controls, rather than squeezing beside them. */
	@media (max-width: 639px) {
		.header-controls {
			flex: 1 0 100%;
		}

		.search-wrapper {
			order: 1;
			flex: 1 0 100%;
		}
	}

	/* From 1024px the header is one row: the search box gives up width rather
	   than pushing the controls onto a second line. */
	@media (min-width: 1024px) {
		.app-header {
			flex-wrap: nowrap;
		}

		.header-controls {
			flex: 1 1 auto;
			flex-wrap: nowrap;
			justify-content: flex-end;
			min-width: 0;
		}

		.search-wrapper {
			flex: 0 1 15rem;
			min-width: 6rem;
		}
	}

	.icon-btn {
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
		transition: all 0.15s;
	}

	:global(body.high-contrast) .icon-btn {
		background: var(--color-bg-secondary);
		border-color: var(--a11y-color-border);
		color: var(--color-text-muted);
	}

	.icon-btn:hover {
		background: #f1f5f9;
		color: #1e293b;
		border-color: #cbd5e1;
	}

	:global(body.high-contrast) .icon-btn:hover {
		background: var(--a11y-color-border);
		color: var(--color-text);
	}

	.icon-btn:focus {
		outline: 2px solid #3b82f6;
		outline-offset: 2px;
	}

	.icon-btn.active {
		background: #eff6ff;
		color: #1d4ed8;
		border-color: #bfdbfe;
	}

	:global(body.high-contrast) .icon-btn:focus {
		outline-color: var(--color-focus-ring);
	}

	.icon-btn svg {
		width: 1.25rem;
		height: 1.25rem;
	}

	.app-main {
		flex: 1;
		overflow: auto;
	}

	/* Focused only as the skip link's target; children keep their own outlines. */
	.app-main:focus {
		outline: none;
	}

	:global(body.high-contrast) .app-main {
		background: var(--color-bg);
	}
</style>
