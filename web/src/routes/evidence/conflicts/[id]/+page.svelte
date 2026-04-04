<script lang="ts">
	import { page } from '$app/stores';
	import { goto } from '$app/navigation';
	import {
		api,
		type EvidenceConflictResponse,
		type EvidenceAnalysisResponse
	} from '$lib/api/client';
	import { Button } from '$lib/components/ui/button';
	import { Badge } from '$lib/components/ui/badge';
	import UncertaintyBadge from '$lib/components/UncertaintyBadge.svelte';

	let conflict: EvidenceConflictResponse | null = $state(null);
	let linkedAnalyses: EvidenceAnalysisResponse[] = $state([]);
	let loading = $state(true);
	let error: string | null = $state(null);
	let resolving = $state(false);
	let resolutionText = $state('');

	function formatFactType(type: string): string {
		return type.replace(/_/g, ' ').replace(/\b\w/g, (c) => c.toUpperCase());
	}

	async function loadConflict(id: string) {
		loading = true;
		error = null;
		linkedAnalyses = [];
		try {
			conflict = await api.getEvidenceConflict(id);
			// Fetch linked analyses in parallel
			if (conflict.analysis_ids && conflict.analysis_ids.length > 0) {
				const results = await Promise.allSettled(
					conflict.analysis_ids.map((aid) => api.getEvidenceAnalysis(aid))
				);
				linkedAnalyses = results
					.filter((r): r is PromiseFulfilledResult<EvidenceAnalysisResponse> => r.status === 'fulfilled')
					.map((r) => r.value);
			}
		} catch (e) {
			error = (e as { message?: string }).message || 'Failed to load conflict';
			conflict = null;
		} finally {
			loading = false;
		}
	}

	async function resolveConflict() {
		if (!conflict) return;
		if (!resolutionText.trim()) {
			error = 'Resolution text is required';
			return;
		}

		resolving = true;
		error = null;
		try {
			await api.resolveEvidenceConflict(conflict.id, {
				resolution: resolutionText.trim(),
				version: conflict.version
			});
			await loadConflict(conflict.id);
			resolutionText = '';
		} catch (e) {
			const msg = (e as { message?: string }).message || 'Failed to resolve conflict';
			if (msg.includes('conflict') || msg.includes('version')) {
				error = 'Version conflict: someone else modified this record. Please reload and try again.';
			} else {
				error = msg;
			}
		} finally {
			resolving = false;
		}
	}

	$effect(() => {
		const id = $page.params.id;
		if (id) {
			loadConflict(id);
		}
	});
</script>

<svelte:head>
	<title>Evidence Conflict | My Family</title>
</svelte:head>

<div class="detail-page">
	<header class="page-header">
		<a href="/evidence" class="back-link">&larr; Evidence</a>
	</header>

	{#if loading}
		<div class="loading">Loading...</div>
	{:else if error && !conflict}
		<div class="error">
			<p>{error}</p>
			<Button variant="outline" onclick={() => loadConflict($page.params.id!)}>Retry</Button>
		</div>
	{:else if conflict}
		<div class="detail-card">
			<div class="detail-header">
				<h1>Evidence Conflict</h1>
				<div class="header-badges">
					<Badge variant="secondary">{formatFactType(conflict.fact_type)}</Badge>
					{#if conflict.status === 'open'}
						<Badge variant="destructive">Open</Badge>
					{:else}
						<Badge class="bg-green-50 text-green-700 border-green-200">Resolved</Badge>
					{/if}
				</div>
			</div>

			{#if error}
				<div class="form-error">{error}</div>
			{/if}

			<div class="info-grid">
				<div class="info-section">
					<h2>Details</h2>
					<dl>
						<dt>Subject</dt>
						<dd><a href="/persons/{conflict.subject_id}">{conflict.subject_id}</a></dd>
						<dt>Fact Type</dt>
						<dd>{formatFactType(conflict.fact_type)}</dd>
						<dt>Status</dt>
						<dd class="status-{conflict.status}">{conflict.status.charAt(0).toUpperCase() + conflict.status.slice(1)}</dd>
					</dl>
				</div>
			</div>

			<div class="info-section">
				<h2>Description</h2>
				<p class="text-content">{conflict.description}</p>
			</div>

			{#if conflict.resolution}
				<div class="info-section resolution-section">
					<h2>Resolution</h2>
					<p class="text-content">{conflict.resolution}</p>
				</div>
			{/if}

			{#if linkedAnalyses.length > 0}
				<div class="info-section">
					<h2>Linked Analyses ({linkedAnalyses.length})</h2>
					<div class="analysis-cards">
						{#each linkedAnalyses as la}
							<a href="/evidence/analyses/{la.id}" class="analysis-card">
								<div class="analysis-card-header">
									<span class="analysis-fact-type">{formatFactType(la.fact_type)}</span>
									{#if la.research_status}
										<UncertaintyBadge status={la.research_status} showLabel size="small" />
									{/if}
								</div>
								<p class="analysis-conclusion">{la.conclusion}</p>
								{#if la.citation_ids && la.citation_ids.length > 0}
									<span class="analysis-meta">{la.citation_ids.length} citations</span>
								{/if}
							</a>
						{/each}
					</div>
				</div>
			{:else if conflict.analysis_ids && conflict.analysis_ids.length > 0}
				<div class="info-section">
					<h2>Linked Analysis IDs ({conflict.analysis_ids.length})</h2>
					<ul class="linked-ids">
						{#each conflict.analysis_ids as aid}
							<li><a href="/evidence/analyses/{aid}"><code>{aid}</code></a></li>
						{/each}
					</ul>
				</div>
			{/if}

			{#if conflict.status === 'open'}
				<div class="resolve-section">
					<h2>Resolve Conflict</h2>
					<p class="resolve-hint">Provide a resolution explaining how this conflict was addressed.</p>
					<label>
						Resolution <span class="required">*</span>
						<textarea bind:value={resolutionText} rows="4" placeholder="Describe how this conflict was resolved..." aria-label="Resolution text"></textarea>
					</label>
					<div class="resolve-actions">
						<Button onclick={resolveConflict} disabled={resolving}>
							{resolving ? 'Resolving...' : 'Resolve Conflict'}
						</Button>
					</div>
				</div>
			{/if}

			<div class="meta-footer">
				{#if conflict.created_at}
					<span>Created: {new Date(conflict.created_at).toLocaleDateString()}</span>
				{/if}
				{#if conflict.updated_at}
					<span>Updated: {new Date(conflict.updated_at).toLocaleDateString()}</span>
				{/if}
				<span>Version: {conflict.version}</span>
			</div>
		</div>
	{/if}
</div>

<style>
	.detail-page {
		max-width: 800px;
		margin: 0 auto;
		padding: 1.5rem;
	}

	.page-header {
		display: flex;
		justify-content: space-between;
		align-items: center;
		margin-bottom: 1.5rem;
	}

	.back-link {
		color: #64748b;
		text-decoration: none;
		font-size: 0.875rem;
	}

	.back-link:hover {
		color: #3b82f6;
	}

	.loading {
		text-align: center;
		padding: 3rem;
		color: #64748b;
	}

	.error {
		text-align: center;
		padding: 3rem;
		color: #dc2626;
	}

	.error p {
		margin: 0 0 1rem;
	}

	.detail-card {
		background: white;
		border-radius: 12px;
		border: 1px solid #e2e8f0;
		padding: 1.5rem;
	}

	.detail-header {
		margin-bottom: 1.5rem;
		padding-bottom: 1rem;
		border-bottom: 1px solid #e2e8f0;
	}

	.detail-header h1 {
		margin: 0 0 0.5rem;
		font-size: 1.5rem;
		color: #1e293b;
	}

	.header-badges {
		display: flex;
		gap: 0.5rem;
		align-items: center;
	}

	.form-error {
		padding: 0.75rem;
		background: #fef2f2;
		border: 1px solid #fecaca;
		border-radius: 6px;
		color: #dc2626;
		font-size: 0.875rem;
		margin-bottom: 1rem;
	}

	.info-grid {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(250px, 1fr));
		gap: 1.5rem;
		margin-bottom: 1.5rem;
	}

	.info-section {
		margin-bottom: 1.5rem;
	}

	.info-section h2 {
		margin: 0 0 0.75rem;
		font-size: 0.875rem;
		font-weight: 600;
		color: #64748b;
		text-transform: uppercase;
		letter-spacing: 0.05em;
	}

	.info-section dl {
		margin: 0;
		display: grid;
		grid-template-columns: auto 1fr;
		gap: 0.25rem 1rem;
	}

	.info-section dt {
		color: #94a3b8;
		font-size: 0.8125rem;
	}

	.info-section dd {
		margin: 0;
		color: #1e293b;
		font-size: 0.875rem;
	}

	.info-section dd a {
		color: #3b82f6;
		text-decoration: none;
	}

	.info-section dd a:hover {
		text-decoration: underline;
	}

	.status-open {
		color: #dc2626;
		font-weight: 500;
	}

	.status-resolved {
		color: #16a34a;
		font-weight: 500;
	}

	.text-content {
		margin: 0;
		color: #475569;
		font-size: 0.875rem;
		white-space: pre-wrap;
		line-height: 1.6;
	}

	.resolution-section {
		background: #f0fdf4;
		border: 1px solid #bbf7d0;
		border-radius: 8px;
		padding: 1rem;
	}

	.linked-ids {
		list-style: none;
		padding: 0;
		margin: 0;
		display: flex;
		flex-direction: column;
		gap: 0.375rem;
	}

	.linked-ids a {
		color: #3b82f6;
		text-decoration: none;
	}

	.linked-ids a:hover {
		text-decoration: underline;
	}

	.linked-ids code {
		font-size: 0.8125rem;
		background: #f1f5f9;
		padding: 0.25rem 0.5rem;
		border-radius: 4px;
	}

	.analysis-cards {
		display: flex;
		flex-direction: column;
		gap: 0.75rem;
	}

	.analysis-card {
		display: block;
		border: 1px solid #e2e8f0;
		border-radius: 8px;
		padding: 1rem;
		text-decoration: none;
		color: inherit;
		transition: border-color 0.15s;
	}

	.analysis-card:hover {
		border-color: #3b82f6;
	}

	.analysis-card-header {
		display: flex;
		justify-content: space-between;
		align-items: center;
		margin-bottom: 0.5rem;
	}

	.analysis-fact-type {
		font-weight: 600;
		font-size: 0.875rem;
		color: #1e293b;
	}

	.analysis-conclusion {
		margin: 0;
		font-size: 0.8125rem;
		color: #475569;
		line-height: 1.4;
	}

	.analysis-meta {
		display: inline-block;
		margin-top: 0.5rem;
		font-size: 0.75rem;
		color: #94a3b8;
	}

	.resolve-section {
		margin-top: 1.5rem;
		padding-top: 1.5rem;
		border-top: 2px solid #fecaca;
	}

	.resolve-section h2 {
		margin: 0 0 0.5rem;
		font-size: 1rem;
		font-weight: 600;
		color: #1e293b;
	}

	.resolve-hint {
		margin: 0 0 1rem;
		font-size: 0.8125rem;
		color: #64748b;
	}

	.resolve-section label {
		display: flex;
		flex-direction: column;
		gap: 0.375rem;
		font-size: 0.875rem;
		color: #475569;
	}

	.required {
		color: #dc2626;
	}

	.resolve-section textarea {
		padding: 0.625rem 0.75rem;
		border: 1px solid #e2e8f0;
		border-radius: 6px;
		font-size: 0.875rem;
		resize: vertical;
	}

	.resolve-section textarea:focus {
		outline: none;
		border-color: #3b82f6;
		box-shadow: 0 0 0 3px rgba(59, 130, 246, 0.1);
	}

	.resolve-actions {
		display: flex;
		justify-content: flex-end;
		margin-top: 1rem;
	}

	.meta-footer {
		margin-top: 1.5rem;
		padding-top: 1rem;
		border-top: 1px solid #e2e8f0;
		display: flex;
		gap: 1.5rem;
		flex-wrap: wrap;
		font-size: 0.75rem;
		color: #94a3b8;
	}
</style>
