<script lang="ts">
	/**
	 * A polite live region for an outcome that is otherwise only visual, such
	 * as a save (see docs/CONVENTIONS.md, "Confirmation and feedback").
	 * Bind to it and call `announce()`.
	 */
	let message = $state('');

	export function announce(text: string) {
		// Cleared first, so the same message twice is read twice.
		message = '';
		setTimeout(() => {
			message = text;
		}, 50);
	}
</script>

<div class="sr-only" role="status" aria-live="polite" aria-atomic="true" data-testid="announcer">
	{message}
</div>
