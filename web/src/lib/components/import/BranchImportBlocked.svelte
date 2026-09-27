<script lang="ts">
	/**
	 * Replaces the GEDCOM upload controls while a research branch is active.
	 *
	 * GEDCOM import always writes the mainline — the command handler appends
	 * with a hardcoded main scope, and importing onto a branch is a stated
	 * non-goal (ADR-005). Uploading from beneath the branch banner would
	 * therefore rewrite the mainline while the UI claimed to be on the branch.
	 * A notice alone would not prevent that, so import is withdrawn outright
	 * (the same pattern as the merge page's `mergeBlockedByBranch`) and the user
	 * is offered the way back to the mainline. The API refuses an import that
	 * carries `?branch=` as a backstop (#825).
	 */
	import { activeBranch, returnToMainline } from '$lib/stores/activeBranch.svelte';
	import { Button } from '$lib/components/ui/button';

	const branchLabel = $derived(
		activeBranch.branch?.name ? `the research branch "${activeBranch.branch.name}"` : 'a research branch'
	);
</script>

<div
	role="note"
	class="rounded-md border border-amber-300 bg-amber-50 px-4 py-3 text-sm text-amber-900"
	data-testid="branch-import-blocked"
>
	<p class="font-semibold">Import is unavailable on a research branch</p>
	<p class="mt-1">
		You are working on {branchLabel}. GEDCOM import always writes to the mainline, so importing
		here would change the mainline rather than your branch. Switch to the mainline to import a
		file; you can start a new research branch from the imported tree afterwards.
	</p>
	<div class="mt-3">
		<Button variant="outline" size="sm" onclick={returnToMainline}>Switch to mainline</Button>
	</div>
</div>
