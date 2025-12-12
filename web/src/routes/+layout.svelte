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

	.search-wrapper {
		margin-left: auto;
	}

	.app-main {
		flex: 1;
		overflow: auto;
	}
</style>
