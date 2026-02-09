<script lang="ts">
	import '../app.css';
	import favicon from '$lib/assets/favicon.svg';
	import { goto } from '$app/navigation';
	import { page } from '$app/stores';
	import SearchBox from '$lib/components/SearchBox.svelte';
	import KeyboardHelp from '$lib/components/KeyboardHelp.svelte';
	import AccessibilityPanel from '$lib/components/AccessibilityPanel.svelte';
	import DemoBanner from '$lib/components/DemoBanner.svelte';
	import { createShortcutHandler } from '$lib/keyboard/useShortcuts.svelte';
	import { loadAppConfig, getAppConfig } from '$lib/stores/appConfig.svelte';
	import type { SearchResult } from '$lib/api/client';

	let { children } = $props();

	const appConfig = getAppConfig();

	$effect(() => {
		loadAppConfig();
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

<div class="app-layout">
	<header class="app-header" role="banner">
		<a href="/" class="logo">My Family</a>
		<nav class="nav" role="navigation" aria-label="Main navigation">
			<a href="/persons" class:active={$page.url.pathname.startsWith('/persons')}>People</a>
			<a href="/families" class:active={$page.url.pathname.startsWith('/families')}>Families</a>
			<div class="nav-dropdown">
				<button class="nav-dropdown-toggle" class:active={$page.url.pathname.startsWith('/browse')}>
					Browse
					<svg class="dropdown-arrow" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
						<polyline points="6 9 12 15 18 9" />
					</svg>
				</button>
				<div class="nav-dropdown-menu">
					<a href="/browse/surnames">By Surname</a>
					<a href="/browse/places">By Place</a>
					<a href="/browse/cemeteries">By Cemetery</a>
				</div>
			</div>
			<a href="/sources" class:active={$page.url.pathname.startsWith('/sources')}>Sources</a>
			<a href="/history" class:active={$page.url.pathname.startsWith('/history')}>History</a>
			<a href="/map" class:active={$page.url.pathname.startsWith('/map')}>Map</a>
			<a href="/analytics" class:active={$page.url.pathname.startsWith('/analytics')}>Analytics</a>
			<a href="/relationship" class:active={$page.url.pathname.startsWith('/relationship')}>Relationship</a>
			<a href="/import" class:active={$page.url.pathname === '/import'}>Import</a>
		</nav>
		<div class="header-controls">
			<div class="search-wrapper">
				<SearchBox bind:this={searchBoxRef} onSelect={handleSearchSelect} placeholder="Search people..." />
			</div>
			<button
				class="accessibility-btn"
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
	<main id="main-content" class="app-main" role="main">
		{@render children()}
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
		--color-border: #666;
		--color-focus-ring: #ffff00;
		background: var(--color-bg);
		color: var(--color-text);
	}

	:global(body.font-large) {
		font-size: 125%;
	}

	:global(body.font-larger) {
		font-size: 150%;
	}

	:global(body.reduced-motion *),
	:global(body.reduced-motion *::before),
	:global(body.reduced-motion *::after) {
		animation-duration: 0.01ms !important;
		animation-iteration-count: 1 !important;
		transition-duration: 0.01ms !important;
	}

	/* Skip link styles */
	:global(.sr-only) {
		position: absolute;
		width: 1px;
		height: 1px;
		padding: 0;
		margin: -1px;
		overflow: hidden;
		clip: rect(0, 0, 0, 0);
		white-space: nowrap;
		border-width: 0;
	}

	:global(.focus\:not-sr-only:focus) {
		position: absolute;
		width: auto;
		height: auto;
		padding: 0;
		margin: 0;
		overflow: visible;
		clip: auto;
		white-space: normal;
	}

	.app-layout {
		min-height: 100vh;
		display: flex;
		flex-direction: column;
	}

	.app-header {
		display: flex;
		align-items: center;
		gap: 2rem;
		padding: 0.75rem 1.5rem;
		background: white;
		border-bottom: 1px solid #e2e8f0;
	}

	:global(body.high-contrast) .app-header {
		background: var(--color-bg-secondary);
		border-bottom-color: var(--color-border);
	}

	.logo {
		font-size: 1.25rem;
		font-weight: 700;
		color: #1e293b;
		text-decoration: none;
	}

	:global(body.high-contrast) .logo {
		color: var(--color-text);
	}

	.nav {
		display: flex;
		gap: 0.25rem;
	}

	.nav a {
		padding: 0.5rem 1rem;
		border-radius: 6px;
		color: #64748b;
		text-decoration: none;
		font-size: 0.875rem;
		font-weight: 500;
		transition: all 0.15s;
	}

	:global(body.high-contrast) .nav a {
		color: var(--color-text-muted);
	}

	.nav a:hover {
		background: #f1f5f9;
		color: #1e293b;
	}

	:global(body.high-contrast) .nav a:hover {
		background: var(--color-border);
		color: var(--color-text);
	}

	.nav a.active {
		background: #eff6ff;
		color: #3b82f6;
	}

	:global(body.high-contrast) .nav a.active {
		background: var(--color-focus-ring);
		color: #000;
	}

	.nav a:focus {
		outline: 2px solid #3b82f6;
		outline-offset: 2px;
	}

	:global(body.high-contrast) .nav a:focus {
		outline-color: var(--color-focus-ring);
	}

	.nav-dropdown {
		position: relative;
	}

	.nav-dropdown-toggle {
		display: flex;
		align-items: center;
		gap: 0.25rem;
		padding: 0.5rem 1rem;
		border: none;
		border-radius: 6px;
		background: transparent;
		color: #64748b;
		font-size: 0.875rem;
		font-weight: 500;
		cursor: pointer;
		transition: all 0.15s;
	}

	:global(body.high-contrast) .nav-dropdown-toggle {
		color: var(--color-text-muted);
	}

	.nav-dropdown-toggle:hover {
		background: #f1f5f9;
		color: #1e293b;
	}

	:global(body.high-contrast) .nav-dropdown-toggle:hover {
		background: var(--color-border);
		color: var(--color-text);
	}

	.nav-dropdown-toggle.active {
		background: #eff6ff;
		color: #3b82f6;
	}

	:global(body.high-contrast) .nav-dropdown-toggle.active {
		background: var(--color-focus-ring);
		color: #000;
	}

	.nav-dropdown-toggle:focus {
		outline: 2px solid #3b82f6;
		outline-offset: 2px;
	}

	:global(body.high-contrast) .nav-dropdown-toggle:focus {
		outline-color: var(--color-focus-ring);
	}

	.dropdown-arrow {
		width: 1rem;
		height: 1rem;
	}

	.nav-dropdown-menu {
		position: absolute;
		top: 100%;
		left: 0;
		margin-top: 0.25rem;
		padding: 0.5rem 0;
		background: white;
		border: 1px solid #e2e8f0;
		border-radius: 8px;
		box-shadow: 0 4px 6px -1px rgba(0, 0, 0, 0.1);
		min-width: 140px;
		opacity: 0;
		visibility: hidden;
		transform: translateY(-4px);
		transition: all 0.15s ease;
		z-index: 100;
	}

	:global(body.high-contrast) .nav-dropdown-menu {
		background: var(--color-bg-secondary);
		border-color: var(--color-border);
	}

	.nav-dropdown:hover .nav-dropdown-menu {
		opacity: 1;
		visibility: visible;
		transform: translateY(0);
	}

	.nav-dropdown-menu a {
		display: block;
		padding: 0.5rem 1rem;
		color: #475569;
		text-decoration: none;
		font-size: 0.875rem;
		transition: background 0.15s;
	}

	:global(body.high-contrast) .nav-dropdown-menu a {
		color: var(--color-text-muted);
	}

	.nav-dropdown-menu a:hover {
		background: #f1f5f9;
		color: #1e293b;
	}

	:global(body.high-contrast) .nav-dropdown-menu a:hover {
		background: var(--color-border);
		color: var(--color-text);
	}

	.header-controls {
		display: flex;
		align-items: center;
		gap: 0.75rem;
		margin-left: auto;
	}

	.search-wrapper {
		/* Contained in header-controls now */
	}

	.accessibility-btn {
		display: flex;
		align-items: center;
		justify-content: center;
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

	:global(body.high-contrast) .accessibility-btn {
		background: var(--color-bg-secondary);
		border-color: var(--color-border);
		color: var(--color-text-muted);
	}

	.accessibility-btn:hover {
		background: #f1f5f9;
		color: #1e293b;
		border-color: #cbd5e1;
	}

	:global(body.high-contrast) .accessibility-btn:hover {
		background: var(--color-border);
		color: var(--color-text);
	}

	.accessibility-btn:focus {
		outline: 2px solid #3b82f6;
		outline-offset: 2px;
	}

	:global(body.high-contrast) .accessibility-btn:focus {
		outline-color: var(--color-focus-ring);
	}

	.accessibility-btn svg {
		width: 1.25rem;
		height: 1.25rem;
	}

	.app-main {
		flex: 1;
		overflow: auto;
	}

	:global(body.high-contrast) .app-main {
		background: var(--color-bg);
	}
</style>
