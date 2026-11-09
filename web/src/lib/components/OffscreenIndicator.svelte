<script lang="ts">
	/**
	 * Edge buttons over a chart saying how many first-generation relatives lie
	 * off each side of the view; a click pans to the nearest one (#1098).
	 */
	interface Props {
		left: number;
		right: number;
		/** Singular and plural noun, e.g. ['child', 'children']. */
		noun: [string, string];
		onPan: (side: 'left' | 'right') => void;
	}

	let { left, right, noun, onPan }: Props = $props();

	const label = (n: number) => `${n} ${n === 1 ? noun[0] : noun[1]}`;
</script>

{#if left > 0}
	<button
		type="button"
		class="offscreen-indicator left"
		aria-label="Show {label(left)} off-screen to the left"
		onclick={() => onPan('left')}>‹ {label(left)}</button
	>
{/if}
{#if right > 0}
	<button
		type="button"
		class="offscreen-indicator right"
		aria-label="Show {label(right)} off-screen to the right"
		onclick={() => onPan('right')}>{label(right)} ›</button
	>
{/if}

<style>
	.offscreen-indicator {
		position: absolute;
		top: 0.5rem;
		padding: 0.25rem 0.625rem;
		font-size: 0.8125rem;
		font-weight: 600;
		color: var(--color-text);
		background: var(--color-bg);
		border: 1px solid var(--color-border);
		border-radius: 9999px;
		box-shadow: 0 1px 3px rgb(0 0 0 / 0.15);
		cursor: pointer;
	}

	.offscreen-indicator:hover {
		background: var(--color-bg-secondary);
	}

	.offscreen-indicator:focus-visible {
		outline: 2px solid var(--color-focus-ring);
		outline-offset: 2px;
	}

	.left {
		left: 0.5rem;
	}

	.right {
		right: 0.5rem;
	}
</style>
