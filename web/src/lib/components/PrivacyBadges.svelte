<script lang="ts">
	/**
	 * Marks a protected person or record (ADR-008): "Living" when the living
	 * rule counts the person as living, "Private" when the record is marked
	 * private. Each badge is an image with its own accessible name, so a
	 * screen reader announces it with the name beside it. Renders nothing for
	 * a record that is neither.
	 */
	import Lock from '@lucide/svelte/icons/lock';
	import Shield from '@lucide/svelte/icons/shield';
	import { Badge } from '#lib/components/ui/badge/index.js';

	interface Props {
		living?: boolean;
		private?: boolean;
		/** What the record is, for the private badge's name: "person", "family". */
		noun?: string;
	}

	let { living = false, private: isPrivate = false, noun = 'person' }: Props = $props();
</script>

{#if living || isPrivate}
	<span class="privacy-badges">
		{#if living}
			<Badge
				variant="outline"
				role="img"
				aria-label="Living person"
				title="Counts as living under the living rule"
				class="privacy-badge"
				data-testid="living-badge"
			>
				<Shield aria-hidden="true" />
				<span aria-hidden="true">Living</span>
			</Badge>
		{/if}
		{#if isPrivate}
			<Badge
				variant="outline"
				role="img"
				aria-label="Private {noun}"
				title="Marked private"
				class="privacy-badge"
				data-testid="private-badge"
			>
				<Lock aria-hidden="true" />
				<span aria-hidden="true">Private</span>
			</Badge>
		{/if}
	</span>
{/if}

<style>
	.privacy-badges {
		display: inline-flex;
		flex-wrap: wrap;
		gap: 0.25rem;
		vertical-align: middle;
	}

	.privacy-badges :global(.privacy-badge) {
		border-color: #94a3b8;
		color: #334155;
		background: #f8fafc;
	}

	:global(body.high-contrast) .privacy-badges :global(.privacy-badge) {
		background: var(--color-bg);
		border-color: var(--a11y-color-border);
		color: var(--color-text);
	}
</style>
