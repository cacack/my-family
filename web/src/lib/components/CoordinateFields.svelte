<script lang="ts">
	/**
	 * Latitude and longitude inputs under a place in a form (#1033), in signed
	 * decimal degrees. The page converts and validates the pair with
	 * `#lib/utils/coordinates`; this shows it, any refusal of one half, and
	 * where the stored pair came from (#1563) while it is left as stored. The
	 * pair is the record's own: while it has none, the place's own pair (#1736)
	 * is shown as a hint, so typing one isn't mistaken for filling a gap.
	 */
	import type { CoordinateOrigin } from '#lib/api/client.js';
	import ActionError from '#lib/components/ActionError.svelte';
	import { coordinateOriginText } from '#lib/utils/coordinateOrigin.js';
	import {
		coordinatesChanged,
		toDecimalDegrees,
		type CoordinateAxis,
		type CoordinateError,
		type GedcomCoordinates
	} from '#lib/utils/coordinates.js';

	let {
		id,
		place,
		latitude = $bindable(''),
		longitude = $bindable(''),
		error = null,
		saved,
		placePair
	}: {
		/** Prefix of the ids of the refusal and hint text, e.g. `'birth-place'`. */
		id: string;
		/** The place input's label, which names the group: "Birth Place". */
		place: string;
		latitude?: string;
		longitude?: string;
		error?: CoordinateError | null;
		/** The stored pair and its origin, on an edit form. */
		saved?: GedcomCoordinates & { origin?: CoordinateOrigin };
		/** The place's own pair, which the map uses while the record has none. */
		placePair?: GedcomCoordinates;
	} = $props();

	const originId = $derived(`${id}-coordinates-origin`);
	const origin = $derived(
		saved?.origin && saved.latitude && saved.longitude && !coordinatesChanged({ latitude, longitude }, saved)
			? coordinateOriginText(saved.origin)
			: null
	);

	// The place's pair, while the record has none stored or typed.
	const placeHint = $derived(
		!origin && placePair?.latitude && placePair.longitude && !saved?.latitude && !saved?.longitude && !latitude && !longitude
			? `The place has coordinates, ${toDecimalDegrees(placePair.latitude, 'latitude')}, ${toDecimalDegrees(placePair.longitude, 'longitude')}. A pair typed here is for this record only.`
			: null
	);

	const errorId = (field: CoordinateAxis) => `${id}-${field}-error`;

	/** An input's ARIA attributes: its refusal, else the origin or place hint. */
	function describe(field: CoordinateAxis) {
		if (error?.field === field) return { 'aria-invalid': 'true' as const, 'aria-describedby': errorId(field) };
		return origin || placeHint ? { 'aria-describedby': originId } : {};
	}
</script>

<fieldset class="coordinate-fields">
	<legend class="sr-only">{place} coordinates</legend>
	<div class="inputs">
		<div class="field">
			<label>
				Latitude
				<input type="text" inputmode="decimal" autocomplete="off" bind:value={latitude} placeholder="e.g., 42.3601" {...describe('latitude')} />
			</label>
			{#if error?.field === 'latitude'}
				<ActionError id={errorId('latitude')} message={error.message} />
			{/if}
		</div>
		<div class="field">
			<label>
				Longitude
				<input type="text" inputmode="decimal" autocomplete="off" bind:value={longitude} placeholder="e.g., -71.0589" {...describe('longitude')} />
			</label>
			{#if error?.field === 'longitude'}
				<ActionError id={errorId('longitude')} message={error.message} />
			{/if}
		</div>
	</div>
	{#if origin}
		<p class="origin" id={originId} data-testid="coordinates-origin">{origin}</p>
	{:else if placeHint}
		<p class="origin" id={originId} data-testid="coordinates-place-hint">{placeHint}</p>
	{/if}
</fieldset>

<style>
	.coordinate-fields {
		min-width: 0;
		margin: 0;
		padding: 0;
		border: 0;
	}

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

	.inputs {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(min(7rem, 100%), 1fr));
		gap: 0.75rem;
	}

	.field {
		display: flex;
		flex-direction: column;
		gap: 0.375rem;
		min-width: 0;
	}

	label {
		display: flex;
		flex-direction: column;
		gap: 0.375rem;
		font-size: 0.8125rem;
		color: #475569;
	}

	:global(body.high-contrast) label {
		color: var(--color-text);
	}

	input {
		min-width: 0;
		padding: 0.625rem 0.75rem;
		border: 1px solid #e2e8f0;
		border-radius: 6px;
		font-size: 0.875rem;
		font-variant-numeric: tabular-nums;
	}

	input:focus {
		outline: none;
		border-color: #3b82f6;
		box-shadow: 0 0 0 3px rgba(59, 130, 246, 0.1);
	}

	.origin {
		margin: 0.25rem 0 0;
		font-size: 0.8125rem;
		color: var(--color-text-muted);
	}
</style>
