<script lang="ts">
	import type { ExternalLink } from '$lib/api/client';
	import { Badge } from '$lib/components/ui/badge';

	interface Props {
		/** External identifiers with server-resolved label and (optional) URL. */
		externalIds?: ExternalLink[];
	}

	let { externalIds = [] }: Props = $props();
</script>

{#if externalIds.length > 0}
	<div class="external-links">
		{#each externalIds as link}
			{#if link.url}
				<!-- Linked badge shows only "View on <label>", so the tooltip surfaces
				     the raw identifier value; the unlinked badge below already shows
				     both label and value, so its tooltip surfaces the type URI instead. -->
				<Badge
					variant="outline"
					href={link.url}
					target="_blank"
					rel="noopener noreferrer"
					title={link.value}
				>
					View on {link.label}
				</Badge>
			{:else}
				<Badge variant="outline" title={link.type}>{link.label}: {link.value}</Badge>
			{/if}
		{/each}
	</div>
{/if}

<style>
	.external-links {
		display: flex;
		flex-wrap: wrap;
		gap: 0.5rem;
	}
</style>
