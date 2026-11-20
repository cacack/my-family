<script lang="ts">
	/**
	 * Takes the place of a change-log page's content (history, snapshots, a
	 * comparison) for a visitor who reached it by a link or a typed URL. The
	 * server refuses visitors the change log, whose records name who made each
	 * change (ADR-008 §4), and the nav hides it from them, so this says why and
	 * offers to sign in instead of showing an error (#1469). The page shows it
	 * without asking the server.
	 */
	import EmptyState from '#lib/components/EmptyState.svelte';
	import { Button } from '#lib/components/ui/button/index.js';
	import { showSignIn } from '#lib/stores/auth.svelte.js';

	interface Props {
		/** What signing in shows, e.g. "the change history". */
		subject: string;
	}

	let { subject }: Props = $props();
</script>

<EmptyState
	message="Sign in to see {subject}"
	description="Change records name who made each change, so only signed-in users can see them."
>
	{#snippet actions()}
		<Button onclick={() => showSignIn()}>Sign in</Button>
	{/snippet}
</EmptyState>
