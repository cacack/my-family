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
		/** Takes focus when the pressed button disappears and no other is shown. */
		focusFallback?: HTMLElement;
	}

	let { left, right, noun, onPan, focusFallback }: Props = $props();

	let leftButton = $state<HTMLButtonElement>();
	let rightButton = $state<HTMLButtonElement>();
	/** The side last pressed, until its count reaches zero. */
	let pressed: 'left' | 'right' | null = null;

	const label = (n: number) => `${n} ${n === 1 ? noun[0] : noun[1]}`;

	function pan(side: 'left' | 'right') {
		pressed = side;
		onPan(side);
	}

	// Panning can empty the pressed side and unmount its button, dropping focus
	// to <body>; hand it to the other side or the fallback instead (WCAG 2.4.3).
	$effect(() => {
		const counts = { left, right };
		if (!pressed || counts[pressed] > 0) return;
		const emptied = pressed;
		pressed = null;
		const active = document.activeElement;
		if (active && active !== document.body) return;
		const other = emptied === 'left' ? rightButton : leftButton;
		(other ?? focusFallback)?.focus();
	});
</script>

{#if left > 0}
	<button
		type="button"
		class="offscreen-indicator left"
		aria-label="Show {label(left)} off-screen to the left"
		bind:this={leftButton}
		onclick={() => pan('left')}>‹ {label(left)}</button
	>
{/if}
{#if right > 0}
	<button
		type="button"
		class="offscreen-indicator right"
		aria-label="Show {label(right)} off-screen to the right"
		bind:this={rightButton}
		onclick={() => pan('right')}>{label(right)} ›</button
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
