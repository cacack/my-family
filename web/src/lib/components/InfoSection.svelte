<script lang="ts" module>
	export interface InfoField {
		label: string;
		/** Shown as `—` when empty. */
		value?: string | null;
		/** A status line under the value, e.g. how a typed date was saved. */
		note?: string;
	}
</script>

<script lang="ts">
	/**
	 * A read-only block on a detail page: a small uppercase `h2` label over a
	 * `dl` of `fields` (empty values show `—`) and/or any other content in
	 * `children`. See docs/CONVENTIONS.md#ui-building-blocks.
	 */
	import type { Snippet } from 'svelte';

	let {
		title,
		badge,
		actions,
		fields,
		hideEmpty = false,
		children
	}: {
		title: string;
		/** Rendered inside the heading, after the title (e.g. a count badge). */
		badge?: Snippet;
		/** Controls beside the heading (e.g. an "Add" button). */
		actions?: Snippet;
		fields?: InfoField[];
		/** Leave out fields with an empty value instead of showing `—`. */
		hideEmpty?: boolean;
		children?: Snippet;
	} = $props();

	let heading: HTMLHeadingElement | undefined = $state();

	const shownFields = $derived(hideEmpty ? (fields ?? []).filter((f) => f.value) : (fields ?? []));

	/** Moves focus to the heading, e.g. after the item that had focus is removed. */
	export function focusHeading() {
		heading?.focus();
	}
</script>

{#snippet headingEl()}
	<h2 bind:this={heading} tabindex="-1">
		{title}
		{@render badge?.()}
	</h2>
{/snippet}

<section class="info-section">
	{#if actions}
		<div class="section-header">
			{@render headingEl()}
			{@render actions()}
		</div>
	{:else}
		{@render headingEl()}
	{/if}
	{#if shownFields.length > 0}
		<dl>
			{#each shownFields as field (field.label)}
				<dt>{field.label}</dt>
				<dd>
					{field.value || '—'}
					{#if field.note}
						<span class="note" role="status">{field.note}</span>
					{/if}
				</dd>
			{/each}
		</dl>
	{/if}
	{@render children?.()}
</section>

<style>
	.info-section {
		margin-bottom: 1.5rem;
	}

	.info-section:last-child {
		margin-bottom: 0;
	}

	h2 {
		display: flex;
		align-items: center;
		margin: 0 0 0.75rem;
		font-size: 0.875rem;
		font-weight: 600;
		color: var(--color-text-muted);
		text-transform: uppercase;
		letter-spacing: 0.05em;
	}

	h2:focus {
		outline: none;
	}

	h2:focus-visible {
		outline: 2px solid var(--color-focus-ring);
		outline-offset: 2px;
	}

	.section-header {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		justify-content: space-between;
		gap: 1rem;
		margin-bottom: 0.75rem;
	}

	.section-header h2 {
		margin: 0;
	}

	dl {
		margin: 0;
		display: grid;
		grid-template-columns: auto minmax(0, 1fr);
		gap: 0.25rem 1rem;
	}

	dt {
		color: var(--color-text-muted);
		font-size: 0.8125rem;
	}

	dd {
		margin: 0;
		/* GEDCOM places are often comma-joined with no spaces to break on. */
		overflow-wrap: anywhere;
		color: var(--color-text);
		font-size: 0.875rem;
	}

	.note {
		display: block;
		margin-top: 0.125rem;
		color: var(--color-text-muted);
		font-size: 0.75rem;
	}
</style>
