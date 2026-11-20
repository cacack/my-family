<script lang="ts">
	/**
	 * Says, above the whole layout, that the owner is previewing the tree as
	 * a viewer or a visitor (#1413), and ends the preview. While it shows,
	 * every page is what the API returns to that principal, and the controls
	 * they may not use are gone.
	 */
	import { Button } from '#lib/components/ui/button/index.js';
	import { previewing, stopPreview } from '#lib/stores/auth.svelte.js';

	const as = $derived(previewing());
</script>

{#if as}
	<div class="preview-banner" role="status">
		<span class="preview-text">
			<span class="preview-label">Preview</span>
			{#if as === 'viewer'}
				You see the tree as a signed-in viewer does.
			{:else}
				You see the tree as a visitor who is not signed in does.
			{/if}
		</span>
		<Button variant="outline" size="sm" onclick={stopPreview}>Stop preview</Button>
	</div>
{/if}

<style>
	.preview-banner {
		display: flex;
		align-items: center;
		flex-wrap: wrap;
		gap: 0.5rem 0.75rem;
		padding: 0.5rem 1.5rem;
		background: #e0f2fe;
		border-bottom: 1px solid #0284c7;
		color: #075985;
		font-size: 0.875rem;
		line-height: 1.25;
	}

	.preview-text {
		flex: 1;
		min-width: 12rem;
	}

	.preview-label {
		font-weight: 700;
		margin-right: 0.5rem;
	}

	@media (max-width: 639px) {
		.preview-banner {
			padding-inline: 1rem;
		}
	}

	:global(body.high-contrast) .preview-banner {
		background: var(--color-bg);
		border-bottom-color: var(--a11y-color-border);
		color: var(--color-text);
	}
</style>
