<script lang="ts">
	/**
	 * The frame of the pages shown before the app: setup and sign-in (ADR-007).
	 * A centred card under the wordmark, with no navigation, since nothing else
	 * can be reached yet.
	 */
	import type { Snippet } from 'svelte';
	import logoMark from '#lib/assets/favicon.svg?raw';
	import * as Card from '#lib/components/ui/card/index.js';

	let {
		title,
		description,
		children
	}: { title: string; description: string; children: Snippet } = $props();
</script>

<svelte:head>
	<title>{title} | My Family</title>
</svelte:head>

<main id="main-content" class="auth-screen" tabindex="-1">
	<div class="auth-brand">
		<span class="auth-logo" aria-hidden="true">{@html logoMark}</span>
		<span>My Family</span>
	</div>
	<Card.Root class="auth-card">
		<Card.Header>
			<Card.Title><h1 class="auth-title">{title}</h1></Card.Title>
			<Card.Description>{description}</Card.Description>
		</Card.Header>
		<Card.Content>
			{@render children()}
		</Card.Content>
	</Card.Root>
</main>

<style>
	.auth-screen {
		min-height: 100dvh;
		display: flex;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		gap: 1.5rem;
		padding: 2rem 1rem;
	}

	.auth-screen:focus {
		outline: none;
	}

	.auth-brand {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		font-size: 1.25rem;
		font-weight: 700;
		color: var(--color-text);
	}

	.auth-logo {
		display: flex;
		width: 32px;
		height: 32px;
	}

	.auth-logo :global(svg) {
		width: 100%;
		height: 100%;
	}

	.auth-screen :global(.auth-card) {
		width: 100%;
		max-width: 26rem;
	}

	.auth-title {
		margin: 0;
		font-size: 1.25rem;
	}
</style>
