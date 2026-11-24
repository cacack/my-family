<script lang="ts">
	/**
	 * A place's coordinate pair and where it came from (#1563), shown under
	 * the place in a form. Renders nothing when the place has no pair.
	 */
	import type { CoordinateOrigin } from '#lib/api/client.js';
	import { coordinateOriginText } from '#lib/utils/coordinateOrigin.js';

	let {
		id,
		latitude,
		longitude,
		origin
	}: {
		/** So the place input can name this as its description. */
		id: string;
		latitude?: string | null;
		longitude?: string | null;
		origin?: CoordinateOrigin;
	} = $props();
</script>

{#if latitude && longitude}
	<p class="place-coordinates" {id} data-testid="place-coordinates">
		<span class="pair">{latitude}, {longitude}</span>
		{#if origin}
			<span aria-hidden="true">·</span>
			{coordinateOriginText(origin)}
		{/if}
	</p>
{/if}

<style>
	.place-coordinates {
		margin: 0.25rem 0 0;
		font-size: 0.8125rem;
		color: var(--color-text-muted);
	}

	.pair {
		font-variant-numeric: tabular-nums;
	}
</style>
