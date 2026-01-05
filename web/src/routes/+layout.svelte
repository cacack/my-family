<script lang="ts">
	import '../app.css';
	import favicon from '$lib/assets/favicon.svg';
	import { goto } from '$app/navigation';
	import { page } from '$app/stores';
	import SearchBox from '$lib/components/SearchBox.svelte';
	import type { SearchResult } from '$lib/api/client';

	let { children } = $props();

	function handleSearchSelect(person: SearchResult) {
		goto(`/persons/${person.id}`);
	}
</script>

<svelte:head>
	<link rel="icon" href={favicon} />
</svelte:head>

<div class="app-layout">
	<header class="app-header">
		<a href="/" class="logo">My Family</a>
		<nav class="nav">
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
				</div>
			</div>
			<a href="/sources" class:active={$page.url.pathname.startsWith('/sources')}>Sources</a>
			<a href="/history" class:active={$page.url.pathname.startsWith('/history')}>History</a>
			<a href="/analytics" class:active={$page.url.pathname.startsWith('/analytics')}>Analytics</a>
			<a href="/import" class:active={$page.url.pathname === '/import'}>Import</a>
		</nav>
		<div class="search-wrapper">
			<SearchBox onSelect={handleSearchSelect} placeholder="Search people..." />
		</div>
	</header>
	<main class="app-main">
		{@render children()}
	</main>
</div>

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

	.logo {
		font-size: 1.25rem;
		font-weight: 700;
		color: #1e293b;
		text-decoration: none;
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

	.nav a:hover {
		background: #f1f5f9;
		color: #1e293b;
	}

	.nav a.active {
		background: #eff6ff;
		color: #3b82f6;
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

	.nav-dropdown-toggle:hover {
		background: #f1f5f9;
		color: #1e293b;
	}

	.nav-dropdown-toggle.active {
		background: #eff6ff;
		color: #3b82f6;
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

	.nav-dropdown-menu a:hover {
		background: #f1f5f9;
		color: #1e293b;
	}

	.search-wrapper {
		margin-left: auto;
	}

	.app-main {
		flex: 1;
		overflow: auto;
	}
</style>
