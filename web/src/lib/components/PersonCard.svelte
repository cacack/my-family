<script lang="ts">
	import type { Person, PersonSummary } from '$lib/api/client';
	import { formatGenDate, formatPersonName, formatLifespan } from '$lib/api/client';

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
</script>

{#if href}
	<a {href} class="person-card" class:compact={variant === 'compact'} data-gender={person.gender}>
		<div class="avatar">
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
			<h3 class="name">{fullName}</h3>
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
	</a>
{:else}
	<button
		class="person-card"
		class:compact={variant === 'compact'}
		data-gender={person.gender}
		{onclick}
	>
		<div class="avatar">
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
			<h3 class="name">{fullName}</h3>
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
	</button>
{/if}

<style>
	.person-card {
		display: flex;
		align-items: flex-start;
		gap: 0.75rem;
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

	.person-card:hover {
		border-color: #cbd5e1;
		box-shadow: 0 2px 8px rgba(0, 0, 0, 0.05);
	}

	.person-card.compact {
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

	.compact .avatar {
		width: 2rem;
		height: 2rem;
	}

	.avatar svg {
		width: 1.25rem;
		height: 1.25rem;
	}

	.compact .avatar svg {
		width: 1rem;
		height: 1rem;
	}

	[data-gender='male'] .avatar {
		background: #dbeafe;
		color: #3b82f6;
	}

	[data-gender='female'] .avatar {
		background: #fce7f3;
		color: #ec4899;
	}

	[data-gender='unknown'] .avatar,
	.person-card:not([data-gender]) .avatar {
		background: #f1f5f9;
		color: #64748b;
	}

	.info {
		flex: 1;
		min-width: 0;
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

	.compact .name {
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
