<script lang="ts">
	import type { FamilyDetail } from '$lib/api/client';
	import { formatGenDate } from '$lib/api/client';
	import { Badge } from '$lib/components/ui/badge';
	import { Card, CardHeader, CardContent } from '$lib/components/ui/card';

	interface Props {
		family: FamilyDetail;
		href?: string;
		onclick?: () => void;
	}

	let { family, href, onclick }: Props = $props();

	const partner1Name = family.partner1_name || 'Unknown';
	const partner2Name = family.partner2_name || null;
	const marriageDate = family.marriage_date ? formatGenDate(family.marriage_date) : null;
	const childCount = family.child_count ?? family.children?.length ?? 0;
</script>

{#snippet cardInner()}
	<div class="partners">
		<span class="partner">{partner1Name}</span>
		{#if partner2Name}
			<span class="connector">&amp;</span>
			<span class="partner">{partner2Name}</span>
		{/if}
	</div>
	<div class="details">
		{#if family.relationship_type}
			<Badge variant="secondary" class="capitalize">{family.relationship_type}</Badge>
		{/if}
		{#if marriageDate}
			<span class="date">{marriageDate}</span>
		{/if}
		{#if childCount > 0}
			<span class="children">{childCount} {childCount === 1 ? 'child' : 'children'}</span>
		{/if}
	</div>
{/snippet}

<Card class="p-0 hover:ring-foreground/20 hover:shadow-sm transition-all">
	{#if href}
		<a {href} class="card-link">
			<CardHeader class="pb-0">
				{@render cardInner()}
			</CardHeader>
		</a>
	{:else}
		<button type="button" class="card-link" {onclick}>
			<CardHeader class="pb-0">
				{@render cardInner()}
			</CardHeader>
		</button>
	{/if}
</Card>

<style>
	.card-link {
		display: block;
		padding: 0;
		text-decoration: none;
		color: inherit;
		cursor: pointer;
		width: 100%;
		text-align: left;
	}

	.partners {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		flex-wrap: wrap;
	}

	.partner {
		font-size: 0.9375rem;
		font-weight: 600;
		color: #1e293b;
	}

	.connector {
		color: #94a3b8;
		font-weight: 400;
	}

	.details {
		display: flex;
		align-items: center;
		gap: 0.75rem;
		margin-top: 0.5rem;
	}

	.date {
		font-size: 0.8125rem;
		color: #64748b;
	}

	.children {
		font-size: 0.8125rem;
		color: #64748b;
	}
</style>
