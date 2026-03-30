<script lang="ts">
	import {
		api,
		type CitationTemplate,
		type CitationTemplateList,
		type FormattedCitation,
		type CitationValidationIssue
	} from '$lib/api/client';
	import {
		Select,
		SelectContent,
		SelectGroup,
		SelectGroupHeading,
		SelectItem,
		SelectTrigger
	} from '$lib/components/ui/select';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { Textarea } from '$lib/components/ui/textarea';
	import {
		Tooltip,
		TooltipContent,
		TooltipTrigger
	} from '$lib/components/ui/tooltip';
	import { Separator } from '$lib/components/ui/separator';
	import { Card, CardContent, CardHeader, CardTitle } from '$lib/components/ui/card';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import Info from '@lucide/svelte/icons/info';
	import ChevronDown from '@lucide/svelte/icons/chevron-down';
	import X from '@lucide/svelte/icons/x';

	interface Props {
		templateId?: string;
		fields?: Record<string, string>;
		onchange?: (templateId: string | undefined, fields: Record<string, string>) => void;
	}

	let { templateId = $bindable(), fields: initialFields, onchange }: Props = $props();

	let templates: CitationTemplate[] = $state([]);
	let selectedTemplate: CitationTemplate | null = $state(null);
	let fieldValues: Record<string, string> = $state(initialFields ? { ...initialFields } : {});
	let loading = $state(true);
	let loadError: string | null = $state(null);

	// Preview state
	let preview: FormattedCitation | null = $state(null);
	let previewLoading = $state(false);
	let previewError: string | null = $state(null);
	let previewDebounceTimer: ReturnType<typeof setTimeout> | null = null;
	let previewRequestId = 0;

	// Mobile preview toggle
	let showMobilePreview = $state(false);

	// Group templates by category
	let templatesByCategory = $derived(() => {
		const groups: Record<string, CitationTemplate[]> = {};
		for (const t of templates) {
			if (!groups[t.category]) {
				groups[t.category] = [];
			}
			groups[t.category].push(t);
		}
		return groups;
	});

	// Select value (bits-ui uses string values)
	let selectValue = $state(templateId ?? '');

	function isTextareaField(key: string): boolean {
		const lower = key.toLowerCase();
		return lower.includes('comments') || lower.includes('notes') || lower.includes('description');
	}

	async function loadTemplates() {
		loading = true;
		loadError = null;
		try {
			const result: CitationTemplateList = await api.listCitationTemplates();
			templates = result.templates;
			// If we have an initial templateId, load that template
			if (templateId) {
				selectedTemplate = templates.find((t) => t.id === templateId) ?? null;
				selectValue = templateId;
			}
		} catch {
			loadError = 'Failed to load citation templates';
		} finally {
			loading = false;
		}
	}

	function handleTemplateSelect(value: string) {
		if (!value) {
			clearTemplate();
			return;
		}
		selectValue = value;
		selectedTemplate = templates.find((t) => t.id === value) ?? null;
		templateId = value;

		// Initialize field values for new template, preserving any that match
		const newFields: Record<string, string> = {};
		if (selectedTemplate) {
			for (const field of selectedTemplate.fields) {
				newFields[field.key] = fieldValues[field.key] ?? '';
			}
		}
		fieldValues = newFields;
		preview = null;
		onchange?.(value, fieldValues);
		schedulePreview();
	}

	function clearTemplate() {
		selectValue = '';
		selectedTemplate = null;
		templateId = undefined;
		fieldValues = {};
		preview = null;
		previewError = null;
		onchange?.(undefined, {});
	}

	function handleFieldChange(key: string, value: string) {
		fieldValues[key] = value;
		if (templateId) {
			onchange?.(templateId, { ...fieldValues });
		}
		schedulePreview();
	}

	function schedulePreview() {
		if (previewDebounceTimer) {
			clearTimeout(previewDebounceTimer);
		}
		const requestId = ++previewRequestId;
		previewDebounceTimer = setTimeout(() => {
			fetchPreview(requestId);
		}, 300);
	}

	async function fetchPreview(requestId: number) {
		if (!templateId || !selectedTemplate) return;

		previewLoading = true;
		previewError = null;
		try {
			const result = await api.previewCitationTemplate(templateId, { ...fieldValues });
			if (requestId !== previewRequestId) return;
			preview = result;
		} catch {
			if (requestId !== previewRequestId) return;
			previewError = 'Failed to load preview';
		} finally {
			if (requestId === previewRequestId) {
				previewLoading = false;
			}
		}
	}

	function getValidationIssuesForField(
		fieldKey: string
	): CitationValidationIssue[] {
		if (!preview?.validation_issues) return [];
		return preview.validation_issues.filter((i) => i.field === fieldKey);
	}

	$effect(() => {
		loadTemplates();
	});
</script>

<div class="template-form">
	<!-- Template Selector -->
	<div class="template-selector">
		<Label for="template-select">Citation Template</Label>
		{#if loading}
			<p class="text-sm text-muted-foreground">Loading templates...</p>
		{:else if loadError}
			<p class="text-sm text-destructive">{loadError}</p>
		{:else}
			<div class="flex items-center gap-2">
				<Select
					type="single"
					value={selectValue}
					onValueChange={handleTemplateSelect}
				>
					<SelectTrigger id="template-select" class="w-full">
						{#if selectedTemplate}
							{selectedTemplate.name}
						{:else}
							<span class="text-muted-foreground">Select a template...</span>
						{/if}
					</SelectTrigger>
					<SelectContent class="max-h-72">
						{#each Object.entries(templatesByCategory()) as [category, categoryTemplates]}
							<SelectGroup>
								<SelectGroupHeading>{category}</SelectGroupHeading>
								{#each categoryTemplates as template}
									<SelectItem value={template.id} label={template.name}>
										{template.name}
									</SelectItem>
								{/each}
							</SelectGroup>
						{/each}
					</SelectContent>
				</Select>
				{#if selectedTemplate}
					<Button
						variant="ghost"
						size="sm"
						onclick={clearTemplate}
						aria-label="Clear template selection"
					>
						<X class="size-4" />
					</Button>
				{/if}
			</div>
			{#if selectedTemplate?.description}
				<p class="mt-1 text-xs text-muted-foreground">{selectedTemplate.description}</p>
			{/if}
		{/if}
	</div>

	{#if selectedTemplate}
		<Separator class="my-4" />

		<div class="template-layout">
			<!-- Field Inputs -->
			<div class="template-fields">
				{#each selectedTemplate.fields as field (field.key)}
					{@const fieldId = `template-field-${field.key}`}
					{@const helpId = `template-help-${field.key}`}
					{@const issues = getValidationIssuesForField(field.key)}
					<div class="field-group">
						<div class="field-label-row">
							<Label for={fieldId}>
								{field.label}
								{#if field.required}
									<span class="text-destructive" aria-hidden="true">*</span>
								{/if}
							</Label>
							{#if field.help_text}
								<Tooltip>
									<TooltipTrigger>
										{#snippet child({ props })}
											<button
												{...props}
												type="button"
												class="inline-flex items-center text-muted-foreground hover:text-foreground"
												aria-label="Help for {field.label}"
											>
												<Info class="size-3.5" />
											</button>
										{/snippet}
									</TooltipTrigger>
									<TooltipContent>
										<p>{field.help_text}</p>
									</TooltipContent>
								</Tooltip>
							{/if}
						</div>
						{#if isTextareaField(field.key)}
							<Textarea
								id={fieldId}
								value={fieldValues[field.key] ?? ''}
								oninput={(e) => handleFieldChange(field.key, e.currentTarget.value)}
								aria-required={field.required ? 'true' : undefined}
								aria-describedby={field.help_text ? helpId : undefined}
								rows={3}
								placeholder={field.help_text ?? ''}
							/>
						{:else}
							<Input
								id={fieldId}
								value={fieldValues[field.key] ?? ''}
								oninput={(e) => handleFieldChange(field.key, e.currentTarget.value)}
								aria-required={field.required ? 'true' : undefined}
								aria-describedby={field.help_text ? helpId : undefined}
								placeholder={field.help_text ?? ''}
							/>
						{/if}
						{#if field.help_text}
							<span id={helpId} class="sr-only">{field.help_text}</span>
						{/if}
						{#each issues as issue}
							<p
								class="mt-1 text-xs"
								class:text-destructive={issue.level === 'error'}
								class:text-amber-600={issue.level === 'warning'}
							>
								{issue.message}
							</p>
						{/each}
					</div>
				{/each}
			</div>

			<!-- Preview Panel (desktop: side-by-side, mobile: collapsible) -->
			<div class="template-preview">
				<!-- Mobile toggle -->
				<button
					type="button"
					class="preview-toggle lg:hidden"
					onclick={() => (showMobilePreview = !showMobilePreview)}
					aria-expanded={showMobilePreview}
					aria-controls="preview-panel"
				>
					<span class="text-sm font-medium">Citation Preview</span>
					<span class="inline-flex transition-transform" class:rotate-180={showMobilePreview}>
						<ChevronDown class="size-4" />
					</span>
				</button>

				<div
					id="preview-panel"
					class="preview-content"
					class:hidden-mobile={!showMobilePreview}
					aria-live="polite"
				>
					<Card>
						<CardHeader class="pb-2">
							<CardTitle class="text-sm">Citation Preview</CardTitle>
						</CardHeader>
						<CardContent>
							{#if previewLoading}
								<p class="text-sm text-muted-foreground">Updating preview...</p>
							{:else if previewError}
								<p class="text-sm text-destructive">{previewError}</p>
							{:else if preview}
								<div class="space-y-3">
									<div>
										<p class="mb-1 text-xs font-medium text-muted-foreground uppercase tracking-wide">
											Full Citation
										</p>
										<p class="text-sm">{preview.full || 'Fill in fields to see preview'}</p>
									</div>
									<Separator />
									<div>
										<p class="mb-1 text-xs font-medium text-muted-foreground uppercase tracking-wide">
											Short Citation
										</p>
										<p class="text-sm">{preview.short || 'Fill in fields to see preview'}</p>
									</div>
									{#if preview.validation_issues && preview.validation_issues.length > 0}
										<Separator />
										<div>
											<p class="mb-1 text-xs font-medium text-muted-foreground uppercase tracking-wide">
												Issues
											</p>
											{#each preview.validation_issues as issue}
												<Badge
													variant={issue.level === 'error' ? 'destructive' : 'outline'}
													class="mr-1 mb-1"
												>
													{issue.field}: {issue.message}
												</Badge>
											{/each}
										</div>
									{/if}
								</div>
							{:else}
								<p class="text-sm text-muted-foreground">
									Fill in template fields to see a citation preview.
								</p>
							{/if}
						</CardContent>
					</Card>
				</div>
			</div>
		</div>
	{/if}
</div>

<style>
	.template-form {
		width: 100%;
	}

	.template-selector {
		display: flex;
		flex-direction: column;
		gap: 0.375rem;
	}

	.template-layout {
		display: flex;
		flex-direction: column;
		gap: 1.5rem;
	}

	@media (min-width: 1024px) {
		.template-layout {
			display: grid;
			grid-template-columns: 1fr 1fr;
			gap: 1.5rem;
		}
	}

	.template-fields {
		display: flex;
		flex-direction: column;
		gap: 1rem;
	}

	.field-group {
		display: flex;
		flex-direction: column;
		gap: 0.25rem;
	}

	.field-label-row {
		display: flex;
		align-items: center;
		gap: 0.375rem;
	}

	.preview-toggle {
		display: flex;
		align-items: center;
		justify-content: space-between;
		width: 100%;
		padding: 0.5rem 0;
		background: none;
		border: none;
		cursor: pointer;
	}

	.hidden-mobile {
		display: none;
	}

	@media (min-width: 1024px) {
		.preview-toggle {
			display: none;
		}

		.hidden-mobile {
			display: block;
		}

		.preview-content {
			position: sticky;
			top: 1rem;
		}
	}
</style>
