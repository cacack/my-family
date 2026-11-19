<script lang="ts">
	/**
	 * Takes the place of a page or section the signed-in role cannot use
	 * (ADR-007), for someone who reached it by a link or a typed URL: the
	 * controls are hidden everywhere else. It names the role the action needs
	 * and the user's own, so they know whom to ask.
	 */
	import { currentRole, type Role } from '#lib/stores/access.svelte.js';

	interface Props {
		/** What the user cannot do, as the start of a sentence, e.g. "Importing GEDCOM". */
		action: string;
		/** The least role that may do it. */
		required: Role;
	}

	let { action, required }: Props = $props();

	const needs = $derived(required === 'owner' ? 'the owner role' : `the ${required} role or above`);
</script>

<div
	role="note"
	class="rounded-md border border-slate-300 bg-slate-50 px-4 py-3 text-sm text-slate-800"
	data-testid="role-notice"
>
	<p class="font-semibold">{action} needs {needs}</p>
	<p class="mt-1">
		You are signed in as {currentRole() === 'editor' ? 'an' : 'a'} {currentRole() ?? 'visitor'}. Ask the
		tree's owner if you need to do this.
	</p>
</div>
