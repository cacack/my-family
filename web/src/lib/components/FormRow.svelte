<script lang="ts">
	/**
	 * One row of form fields. Fields sit side by side while each can have at
	 * least `minColumnWidth`, and stack one per line below that, so no form
	 * needs its own breakpoint. The `min(…, 100%)` keeps a single field from
	 * overflowing a container narrower than `minColumnWidth`.
	 *
	 * The children's styling (labels, inputs) stays with the page that renders
	 * them; this only lays them out. See docs/CONVENTIONS.md#ui-building-blocks.
	 */
	import type { Snippet } from 'svelte';

	let {
		minColumnWidth = '14rem',
		children
	}: {
		/** A CSS length with a unit, e.g. `'10rem'`. */
		minColumnWidth?: string;
		children: Snippet;
	} = $props();
</script>

<div class="form-row" style:--form-row-min={minColumnWidth}>
	{@render children()}
</div>

<style>
	.form-row {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(min(var(--form-row-min), 100%), 1fr));
		gap: 1rem;
		margin-bottom: 1rem;
	}
</style>
