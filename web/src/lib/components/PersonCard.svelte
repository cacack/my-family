<script lang="ts">
	import type { Person, PersonSummary } from '$lib/api/client';
	import { formatGenDate, formatPersonName, formatLifespan } from '$lib/api/client';
	import UncertaintyBadge from './UncertaintyBadge.svelte';
	import { Card, CardContent } from '$lib/components/ui/card';

	interface Props {
		person: Person | PersonSummary;
		variant?: 'default' | 'compact';
		href?: string;
		onclick?: () => void;
	}

	let { person, variant = 'default', href, onclick }: Props = $props();

	const fullName = formatPersonName(person);
	const lifespan = formatLifespan(person);
	const birthDate = person.birth_date ? formatGenDate(person.birth_date) : null;
	const deathDate =
		'death_date' in person && person.death_date ? formatGenDate(person.death_date) : null;

	// Get research_status if available (Person has it, PersonSummary doesn't)
	const researchStatus = 'research_status' in person ? person.research_status : undefined;

	const cardSize = variant === 'compact' ? 'sm' : 'default';
</script>

{#snippet cardInner()}
	<div class="avatar" class:compact={variant === 'compact'} data-gender={person.gender}>
		{#if person.gender === 'male'}
			<svg viewBox="0 0 24 24" fill="currentColor">
				<path
					d="M12 4a4 4 0 1 0 0 8 4 4 0 0 0 0-8zM6 8a6 6 0 1 1 12 0A6 6 0 0 1 6 8zm2 10a3 3 0 0 0-3 3 1 1 0 1 1-2 0 5 5 0 0 1 5-5h8a5 5 0 0 1 5 5 1 1 0 1 1-2 0 3 3 0 0 0-3-3H8z"
				/>
			</svg>
		{:else if person.gender === 'female'}
			<svg viewBox="0 0 24 24" fill="currentColor">
				<path
					d="M12 4a4 4 0 1 0 0 8 4 4 0 0 0 0-8zM6 8a6 6 0 1 1 12 0A6 6 0 0 1 6 8zm2 10a3 3 0 0 0-3 3 1 1 0 1 1-2 0 5 5 0 0 1 5-5h8a5 5 0 0 1 5 5 1 1 0 1 1-2 0 3 3 0 0 0-3-3H8z"
				/>
			</svg>
		{:else}
			<svg viewBox="0 0 24 24" fill="currentColor">
				<path
					d="M12 4a4 4 0 1 0 0 8 4 4 0 0 0 0-8zM6 8a6 6 0 1 1 12 0A6 6 0 0 1 6 8zm2 10a3 3 0 0 0-3 3 1 1 0 1 1-2 0 5 5 0 0 1 5-5h8a5 5 0 0 1 5 5 1 1 0 1 1-2 0 3 3 0 0 0-3-3H8z"
				/>
			</svg>
		{/if}
	</div>
	<div class="info">
		<div class="name-row">
			<h3 class="name" class:compact={variant === 'compact'}>{fullName}</h3>
			{#if researchStatus}
				<UncertaintyBadge status={researchStatus} size="small" />
			{/if}
		</div>
		{#if variant === 'default'}
			{#if birthDate}
				<p class="detail">b. {birthDate}</p>
			{/if}
			{#if deathDate}
				<p class="detail">d. {deathDate}</p>
			{/if}
		{:else}
			<p class="lifespan">{lifespan}</p>
		{/if}
	</div>
{/snippet}

<Card size={cardSize} class="p-0 hover:ring-foreground/20 hover:shadow-sm transition-all">
	<CardContent class="p-0">
		{#if href}
			<a {href} class="card-link" class:compact={variant === 'compact'}>
				{@render cardInner()}
			</a>
		{:else}
			<button type="button" class="card-link" class:compact={variant === 'compact'} {onclick}>
				{@render cardInner()}
			</button>
		{/if}
	</CardContent>
</Card>

<style>
	.card-link {
		display: flex;
		align-items: flex-start;
		gap: 0.75rem;
		padding: 1rem;
		text-decoration: none;
		color: inherit;
		cursor: pointer;
		width: 100%;
		text-align: left;
	}

	.card-link.compact {
		padding: 0.625rem 0.75rem;
		gap: 0.5rem;
	}

	.avatar {
		flex-shrink: 0;
		width: 2.5rem;
		height: 2.5rem;
		border-radius: 50%;
		display: flex;
		align-items: center;
		justify-content: center;
	}

	.avatar.compact {
		width: 2rem;
		height: 2rem;
	}

	.avatar :global(svg) {
		width: 1.25rem;
		height: 1.25rem;
	}

	.avatar.compact :global(svg) {
		width: 1rem;
		height: 1rem;
	}

	.avatar[data-gender='male'] {
		background: #dbeafe;
		color: #3b82f6;
	}

	.avatar[data-gender='female'] {
		background: #fce7f3;
		color: #ec4899;
	}

	.avatar[data-gender='unknown'],
	.avatar:not([data-gender]) {
		background: #f1f5f9;
		color: #64748b;
	}

	.info {
		flex: 1;
		min-width: 0;
	}

	.name-row {
		display: flex;
		align-items: center;
		gap: 0.5rem;
	}

	.name {
		margin: 0;
		font-size: 0.9375rem;
		font-weight: 600;
		color: #1e293b;
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}

	.name.compact {
		font-size: 0.875rem;
	}

	.detail {
		margin: 0.25rem 0 0;
		font-size: 0.8125rem;
		color: #64748b;
	}

	.lifespan {
		margin: 0.125rem 0 0;
		font-size: 0.75rem;
		color: #94a3b8;
	}
</style>
