<script lang="ts">
	/**
	 * Announcer component for screen reader live region announcements.
	 * Use the announce() function to make announcements that screen readers will read.
	 *
	 * Usage:
	 *   import { announce } from '$lib/components/Announcer.svelte';
	 *   announce('5 results found');
	 */

	let announcement = $state('');
	let assertiveAnnouncement = $state('');

	/**
	 * Announce a message to screen readers using polite priority.
	 * The announcement will be read when the user is idle.
	 */
	export function announce(message: string) {
		// Clear first to ensure screen reader picks up the change even if same message
		announcement = '';
		setTimeout(() => {
			announcement = message;
		}, 100);
	}

	/**
	 * Announce a message to screen readers using assertive priority.
	 * The announcement will interrupt the user immediately.
	 * Use sparingly - only for critical information like errors.
	 */
	export function announceAssertive(message: string) {
		assertiveAnnouncement = '';
		setTimeout(() => {
			assertiveAnnouncement = message;
		}, 100);
	}

	/**
	 * Clear all pending announcements.
	 */
	export function clearAnnouncements() {
		announcement = '';
		assertiveAnnouncement = '';
	}
</script>

<!-- Polite announcements - read when user is idle -->
<div
	role="status"
	aria-live="polite"
	aria-atomic="true"
	class="sr-only"
>
	{announcement}
</div>

<!-- Assertive announcements - interrupt immediately -->
<div
	role="alert"
	aria-live="assertive"
	aria-atomic="true"
	class="sr-only"
>
	{assertiveAnnouncement}
</div>

<style>
	.sr-only {
		position: absolute;
		width: 1px;
		height: 1px;
		padding: 0;
		margin: -1px;
		overflow: hidden;
		clip: rect(0, 0, 0, 0);
		white-space: nowrap;
		border: 0;
	}
</style>
