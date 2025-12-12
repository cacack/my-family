<script lang="ts">
	import type { FamilyDetail } from '$lib/api/client';
	import { formatGenDate } from '$lib/api/client';

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

{#if href}
	<a {href} class="family-card">
		<div class="partners">
			<span class="partner">{partner1Name}</span>
			{#if partner2Name}
				<span class="connector">&amp;</span>
				<span class="partner">{partner2Name}</span>
			{/if}
		</div>
		<div class="details">
			{#if family.relationship_type}
				<span class="badge">{family.relationship_type}</span>
			{/if}
			{#if marriageDate}
				<span class="date">{marriageDate}</span>
			{/if}
			{#if childCount > 0}
				<span class="children">{childCount} {childCount === 1 ? 'child' : 'children'}</span>
			{/if}
		</div>
	</a>
{:else}
	<button class="family-card" {onclick}>
		<div class="partners">
			<span class="partner">{partner1Name}</span>
			{#if partner2Name}
				<span class="connector">&amp;</span>
				<span class="partner">{partner2Name}</span>
			{/if}
		</div>
		<div class="details">
			{#if family.relationship_type}
				<span class="badge">{family.relationship_type}</span>
			{/if}
			{#if marriageDate}
				<span class="date">{marriageDate}</span>
			{/if}
			{#if childCount > 0}
				<span class="children">{childCount} {childCount === 1 ? 'child' : 'children'}</span>
			{/if}
		</div>
	</button>
{/if}

<style>
	.family-card {
		display: block;
		padding: 1rem;
		background: white;
		border: 1px solid #e2e8f0;
		border-radius: 8px;
		text-decoration: none;
		color: inherit;
		cursor: pointer;
		transition: all 0.15s;
		width: 100%;
		text-align: left;
	}

	.family-card:hover {
		border-color: #cbd5e1;
		box-shadow: 0 2px 8px rgba(0, 0, 0, 0.05);
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

	.badge {
		display: inline-block;
		padding: 0.125rem 0.5rem;
		background: #f1f5f9;
		border-radius: 4px;
		font-size: 0.75rem;
		color: #475569;
		text-transform: capitalize;
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
