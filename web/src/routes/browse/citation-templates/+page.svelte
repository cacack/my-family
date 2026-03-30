<script lang="ts">
	import { api, type CitationTemplate } from '$lib/api/client';
	import * as Card from '$lib/components/ui/card';
	import * as Select from '$lib/components/ui/select';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import { Separator } from '$lib/components/ui/separator';

	let templates: CitationTemplate[] = $state([]);
	let loading = $state(true);
	let error: string | null = $state(null);
	let activeCategory = $state('all');
	let sourceTypeFilter = $state('');
	let expandedIds: Set<string> = $state(new Set());

	// Derive categories from loaded templates
	let categories = $derived(
		Array.from(new Set(templates.map((t) => t.category))).sort()
	);

	// Derive all unique source types for the filter
	let allSourceTypes = $derived(
		Array.from(new Set(templates.flatMap((t) => t.source_types))).sort()
	);

	// Filter templates by active category and source type
	let filteredTemplates = $derived.by(() => {
		let result = templates;
		if (activeCategory !== 'all') {
			result = result.filter((t) => t.category === activeCategory);
		}
		if (sourceTypeFilter) {
			result = result.filter((t) => t.source_types.includes(sourceTypeFilter));
		}
		return result;
	});

	function toggleExpanded(id: string) {
		const next = new Set(expandedIds);
		if (next.has(id)) {
			next.delete(id);
		} else {
			next.add(id);
		}
		expandedIds = next;
	}

	function fieldSummary(template: CitationTemplate): string {
		const total = template.fields.length;
		const required = template.fields.filter((f) => f.required).length;
		return `${total} field${total !== 1 ? 's' : ''}, ${required} required`;
	}

	async function loadTemplates() {
		loading = true;
		error = null;
		try {
			const result = await api.listCitationTemplates();
			templates = result.templates;
		} catch (e) {
			console.error('Failed to load citation templates:', e);
			error = 'Failed to load citation templates. Please try again.';
		} finally {
			loading = false;
		}
	}

	$effect(() => {
		loadTemplates();
	});
</script>

<svelte:head>
	<title>Citation Templates | My Family</title>
</svelte:head>

<div class="browse-page">
	<header class="page-header">
		<h1>Citation Templates</h1>
		<p class="description">
			Evidence Explained citation templates for documenting genealogical sources with precision.
		</p>
	</header>

	{#if loading}
		<div class="loading" role="status" aria-live="polite">Loading citation templates...</div>
	{:else if error}
		<div class="error" role="alert">
			<p>{error}</p>
			<Button variant="outline" onclick={loadTemplates}>Retry</Button>
		</div>
	{:else if templates.length === 0}
		<div class="empty">No citation templates available.</div>
	{:else}
		<!-- Source type filter -->
		{#if allSourceTypes.length > 0}
			<div class="filters">
				<label for="source-type-filter" class="filter-label">Filter by source type:</label>
				<Select.Root
					type="single"
					value={sourceTypeFilter || undefined}
					onValueChange={(v) => { sourceTypeFilter = v ?? ''; }}
				>
					<Select.Trigger id="source-type-filter" class="source-type-trigger">
						{sourceTypeFilter || 'All source types'}
					</Select.Trigger>
					<Select.Content>
						<Select.Item value="">All source types</Select.Item>
						<Select.Separator />
						{#each allSourceTypes as st}
							<Select.Item value={st}>{st}</Select.Item>
						{/each}
					</Select.Content>
				</Select.Root>
			</div>
		{/if}

		<Separator class="my-4" />

		<!-- Category filter buttons -->
		<div class="category-filters" role="group" aria-label="Filter by category">
			<Button
				variant={activeCategory === 'all' ? 'default' : 'outline'}
				size="sm"
				onclick={() => { activeCategory = 'all'; }}
			>
				All ({templates.length})
			</Button>
			{#each categories as cat}
				<Button
					variant={activeCategory === cat ? 'default' : 'outline'}
					size="sm"
					onclick={() => { activeCategory = cat; }}
				>
					{cat} ({templates.filter((t) => t.category === cat).length})
				</Button>
			{/each}
		</div>

		<!-- Template grid -->
		<div class="templates-content" role="region" aria-live="polite">
			{#if filteredTemplates.length === 0}
				<div class="empty">No templates match the current filters.</div>
			{:else}
				<div class="template-grid">
					{#each filteredTemplates as template (template.id)}
						{@const isExpanded = expandedIds.has(template.id)}
						{@const requiredFields = template.fields.filter((f) => f.required)}
						<Card.Root class="template-card">
							<Card.Header>
								<Card.Title class="template-title">{template.name}</Card.Title>
								{#if template.description}
									<Card.Description>{template.description}</Card.Description>
								{/if}
							</Card.Header>
							<Card.Content>
								<div class="template-meta">
									<span class="field-count">{fieldSummary(template)}</span>
								</div>
								<div class="source-types">
									{#each template.source_types as st}
										<Badge variant="secondary">{st}</Badge>
									{/each}
								</div>
								{#if requiredFields.length > 0}
									<div class="required-fields">
										<span class="required-label">Required:</span>
										{#each requiredFields as field, i}
											<span class="required-field">{field.label}{i < requiredFields.length - 1 ? ',' : ''}</span>
										{/each}
									</div>
								{/if}
							</Card.Content>
							<Card.Footer class="template-footer">
								<Button
									variant="ghost"
									size="sm"
									onclick={() => toggleExpanded(template.id)}
									aria-expanded={isExpanded}
									aria-controls="fields-{template.id}"
								>
									{isExpanded ? 'Hide fields' : 'Show all fields'}
								</Button>
							</Card.Footer>
							{#if isExpanded}
								<div id="fields-{template.id}" class="field-details">
									<Separator />
									<div class="field-list">
										{#each template.fields as field}
											<div class="field-item">
												<div class="field-header">
													<span class="field-name">{field.label}</span>
													{#if field.required}
														<Badge variant="destructive" class="field-badge">Required</Badge>
													{:else}
														<Badge variant="outline" class="field-badge">Optional</Badge>
													{/if}
												</div>
												{#if field.help_text}
													<p class="field-help">{field.help_text}</p>
												{/if}
											</div>
										{/each}
									</div>
								</div>
							{/if}
						</Card.Root>
					{/each}
				</div>
			{/if}
		</div>
	{/if}
</div>

<style>
	.browse-page {
		max-width: 1200px;
		margin: 0 auto;
		padding: 1.5rem;
	}

	.page-header {
		margin-bottom: 1.5rem;
	}

	.page-header h1 {
		margin: 0 0 0.5rem;
		font-size: 1.5rem;
		color: #1e293b;
	}

	.description {
		margin: 0;
		color: #64748b;
		font-size: 0.9375rem;
	}

	.loading,
	.empty {
		text-align: center;
		padding: 2rem;
		color: #64748b;
	}

	.error {
		text-align: center;
		padding: 2rem;
		color: #dc2626;
	}

	.filters {
		display: flex;
		align-items: center;
		gap: 0.75rem;
	}

	.filter-label {
		font-size: 0.875rem;
		font-weight: 500;
		color: #475569;
		white-space: nowrap;
	}

	:global(.source-type-trigger) {
		min-width: 200px;
	}

	.category-filters {
		display: flex;
		flex-wrap: wrap;
		gap: 0.5rem;
		margin-bottom: 1rem;
	}

	.templates-content {
		margin-top: 1rem;
	}

	.template-grid {
		display: grid;
		grid-template-columns: repeat(3, 1fr);
		gap: 1rem;
	}

	.template-meta {
		margin-bottom: 0.75rem;
	}

	.field-count {
		font-size: 0.8125rem;
		color: #64748b;
	}

	.source-types {
		display: flex;
		flex-wrap: wrap;
		gap: 0.375rem;
		margin-bottom: 0.75rem;
	}

	.required-fields {
		display: flex;
		flex-wrap: wrap;
		gap: 0.25rem;
		font-size: 0.8125rem;
		color: #475569;
	}

	.required-label {
		font-weight: 600;
	}

	.required-field {
		color: #64748b;
	}

	:global(.template-footer) {
		padding-top: 0 !important;
	}

	.field-details {
		padding: 0 1.5rem 1.5rem;
	}

	.field-list {
		display: flex;
		flex-direction: column;
		gap: 0.75rem;
		margin-top: 1rem;
	}

	.field-item {
		display: flex;
		flex-direction: column;
		gap: 0.25rem;
	}

	.field-header {
		display: flex;
		align-items: center;
		gap: 0.5rem;
	}

	.field-name {
		font-size: 0.875rem;
		font-weight: 500;
		color: #1e293b;
	}

	:global(.field-badge) {
		font-size: 0.6875rem !important;
	}

	.field-help {
		margin: 0;
		font-size: 0.8125rem;
		color: #64748b;
		line-height: 1.4;
	}

	/* Responsive */
	@media (max-width: 1024px) {
		.template-grid {
			grid-template-columns: repeat(2, 1fr);
		}
	}

	@media (max-width: 640px) {
		.template-grid {
			grid-template-columns: 1fr;
		}

		.filters {
			flex-direction: column;
			align-items: stretch;
		}
	}
</style>
