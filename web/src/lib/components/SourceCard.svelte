<script lang="ts">
	import type { Source } from '$lib/api/client';

	interface Props {
		source: Source;
		href?: string;
		onclick?: () => void;
	}

	let { source, href, onclick }: Props = $props();

	function formatSourceType(type: string): string {
		return type.replace(/_/g, ' ').replace(/\b\w/g, (c) => c.toUpperCase());
	}
</script>

{#if href}
	<a {href} class="source-card">
		<div class="source-icon">
			<svg viewBox="0 0 24 24" fill="currentColor">
				<path d="M19 2H6c-1.206 0-3 .799-3 3v14c0 2.201 1.794 3 3 3h15v-2H6.012C5.55 19.988 5 19.806 5 19s.55-.988 1.012-1H21V4c0-1.103-.897-2-2-2zm0 14H5V5c0-.806.55-.988 1-1h13v12z" />
			</svg>
		</div>
		<div class="info">
			<h3 class="title">{source.title}</h3>
			<div class="meta">
				{#if source.author}
					<span class="author">{source.author}</span>
				{/if}
				<span class="type-badge">{formatSourceType(source.source_type)}</span>
				{#if source.citation_count > 0}
					<span class="citation-count">{source.citation_count} {source.citation_count === 1 ? 'citation' : 'citations'}</span>
				{/if}
			</div>
		</div>
	</a>
{:else}
	<button class="source-card" {onclick}>
		<div class="source-icon">
			<svg viewBox="0 0 24 24" fill="currentColor">
				<path d="M19 2H6c-1.206 0-3 .799-3 3v14c0 2.201 1.794 3 3 3h15v-2H6.012C5.55 19.988 5 19.806 5 19s.55-.988 1.012-1H21V4c0-1.103-.897-2-2-2zm0 14H5V5c0-.806.55-.988 1-1h13v12z" />
			</svg>
		</div>
		<div class="info">
			<h3 class="title">{source.title}</h3>
			<div class="meta">
				{#if source.author}
					<span class="author">{source.author}</span>
				{/if}
				<span class="type-badge">{formatSourceType(source.source_type)}</span>
				{#if source.citation_count > 0}
					<span class="citation-count">{source.citation_count} {source.citation_count === 1 ? 'citation' : 'citations'}</span>
				{/if}
			</div>
		</div>
	</button>
{/if}

<style>
	.source-card {
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

	.source-card:hover {
		border-color: #cbd5e1;
		box-shadow: 0 2px 8px rgba(0, 0, 0, 0.05);
	}

	.source-icon {
		flex-shrink: 0;
		width: 2.5rem;
		height: 2.5rem;
		border-radius: 8px;
		display: flex;
		align-items: center;
		justify-content: center;
		background: #f1f5f9;
		color: #64748b;
	}

	.source-icon svg {
		width: 1.25rem;
		height: 1.25rem;
	}

	.info {
		flex: 1;
		min-width: 0;
	}

	.title {
		margin: 0;
		font-size: 0.9375rem;
		font-weight: 600;
		color: #1e293b;
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}

	.meta {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		margin-top: 0.375rem;
		flex-wrap: wrap;
	}

	.author {
		font-size: 0.8125rem;
		color: #64748b;
	}

	.type-badge {
		display: inline-block;
		padding: 0.125rem 0.5rem;
		background: #f1f5f9;
		border-radius: 4px;
		font-size: 0.75rem;
		color: #475569;
	}

	.citation-count {
		font-size: 0.8125rem;
		color: #94a3b8;
	}
</style>
