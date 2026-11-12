<script lang="ts">
	import { untrack } from 'svelte';
	import { page } from '$app/state';
	import {
		api,
		isVersionConflict,
		recordLoadError,
		type LoadFailure,
		type EvidenceConflictResponse,
		type EvidenceAnalysisResponse
	} from '#lib/api/client.js';
	import { Button } from '#lib/components/ui/button/index.js';
	import { Badge } from '#lib/components/ui/badge/index.js';
	import { Label } from '#lib/components/ui/label/index.js';
	import { Textarea } from '#lib/components/ui/textarea/index.js';
	import UncertaintyBadge from '#lib/components/UncertaintyBadge.svelte';
	import {
		formatFactType,
		subjectTypeOf,
		conflictBadgeProps,
		readableConflictDescription
	} from '#lib/utils/evidence.js';
	import { SubjectNames, subjectHref } from '#lib/utils/subjectNames.svelte.js';
	import ErrorState from '#lib/components/ErrorState.svelte';
	import ActionError from '#lib/components/ActionError.svelte';
	import PageHeader from '#lib/components/PageHeader.svelte';
	import InfoSection from '#lib/components/InfoSection.svelte';
	import SaveConflict from '#lib/components/SaveConflict.svelte';
	import { SaveConflictFlow, type ConflictRow } from '#lib/utils/saveConflict.svelte.js';

	let conflict: EvidenceConflictResponse | null = $state(null);
	let linkedAnalyses: EvidenceAnalysisResponse[] = $state([]);
	let loading = $state(true);
	let error: string | null = $state(null);
	let loadError: LoadFailure | null = $state(null);
	let resolving = $state(false);
	let resolutionText = $state('');

	// The resolution form, for a resolve refused because the conflict changed.
	type ResolutionForm = { resolution: string };
	const saveConflict = new SaveConflictFlow<EvidenceConflictResponse, ResolutionForm>({
		base: () => conflict,
		formFrom: (saved) => ({ resolution: saved.resolution ?? '' }),
		form: () => ({ resolution: resolutionText }),
		adopt: (latest, form) => {
			conflict = latest;
			resolutionText = form.resolution;
		},
		save: () => resolveConflict()
	});
	const conflictRows: ConflictRow<ResolutionForm>[] = [{ key: 'resolution', label: 'Resolution' }];

	// The conflict stores its subject's id; the page shows the subject's name.
	const subjects = new SubjectNames();

	async function loadConflict(id: string) {
		loading = true;
		error = null;
		loadError = null;
		linkedAnalyses = [];
		try {
			conflict = await api.getEvidenceConflict(id);
			subjects.load([{ type: subjectTypeOf(conflict.fact_type), id: conflict.subject_id }]);
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
			loadError = recordLoadError(e, 'conflict');
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
		saveConflict.clear();
		try {
			await api.resolveEvidenceConflict(conflict.id, {
				resolution: resolutionText.trim(),
				version: conflict.version
			});
			await loadConflict(conflict.id);
			resolutionText = '';
		} catch (e) {
			// Only a stale version is compared with the latest (#940); other 409s
			// are refusals, and a conflict that can't be re-read shows the refusal.
			const id = conflict.id;
			if (!(isVersionConflict(e) && (await saveConflict.load(() => api.getEvidenceConflict(id))))) {
				error = (e as { message?: string }).message || 'Failed to resolve conflict';
			} else if (saveConflict.latest?.status !== 'open') {
				// Resolved elsewhere: show that resolution rather than offer to overwrite it.
				saveConflict.useLatest();
				error = 'Someone else resolved this conflict while you were editing. Their resolution is shown below.';
			}
		} finally {
			resolving = false;
		}
	}

	$effect(() => {
		const id = page.params.id;
		if (id) {
			untrack(() => loadConflict(id));
		}
	});
</script>

<svelte:head>
	<title>{conflict ? `${formatFactType(conflict.fact_type)} Conflict` : 'Evidence Conflict'} | My Family</title>
</svelte:head>

<div class="mx-auto max-w-3xl p-6">
	{#if loading}
		<PageHeader title="Evidence Conflict" back={{ href: '/evidence', label: 'Evidence' }} />
		<div class="p-12 text-center text-muted-foreground">Loading...</div>
	{:else if loadError}
		<PageHeader title={loadError.title} back={{ href: '/evidence', label: 'Evidence' }} />
		<ErrorState
			message={loadError.message}
			notFound={loadError.notFound}
			onRetry={() => loadConflict(page.params.id!)}
		/>
	{:else if conflict}
		{@const statusBadge = conflictBadgeProps(conflict.status)}
		{@const subject = { type: subjectTypeOf(conflict.fact_type), id: conflict.subject_id } as const}
		{#snippet conflictBadges()}
			<Badge variant="secondary">{formatFactType(conflict!.fact_type)}</Badge>
			<Badge variant={statusBadge.variant} class={statusBadge.class}>
				{statusBadge.label}
			</Badge>
		{/snippet}
		<PageHeader
			title="Evidence Conflict"
			back={{ href: '/evidence', label: 'Evidence' }}
			badges={conflictBadges}
		/>
		<div class="rounded-xl border border-slate-200 bg-white p-6">
			{#if error}
				<ActionError message={error} />
			{/if}

			<InfoSection
				title="Details"
				fields={[
					{ label: 'Subject', value: subjects.label(subject), href: subjectHref(subject) },
					{ label: 'Fact Type', value: formatFactType(conflict.fact_type) },
					{ label: 'Status', value: statusBadge.label }
				]}
			/>

			<InfoSection title="Description">
				<p class="m-0 whitespace-pre-wrap text-sm leading-relaxed text-foreground">
					{readableConflictDescription(
						conflict.description,
						conflict.fact_type,
						conflict.subject_id,
						subjects.name(subject)
					)}
				</p>
			</InfoSection>

			{#if conflict.resolution}
				<div class="mb-6 rounded-lg border border-green-200 bg-green-50 p-4">
					<InfoSection title="Resolution">
						<p class="m-0 whitespace-pre-wrap text-sm leading-relaxed text-foreground">
							{conflict.resolution}
						</p>
					</InfoSection>
				</div>
			{/if}

			{#if linkedAnalyses.length > 0}
				<InfoSection title={`Linked Analyses (${linkedAnalyses.length})`}>
					<div class="flex flex-col gap-3">
						{#each linkedAnalyses as la}
							<a
								href="/evidence/analyses/{la.id}"
								class="block rounded-lg border border-slate-200 p-4 text-inherit no-underline transition-colors hover:border-blue-500"
								data-slot="card-link"
							>
								<div class="mb-2 flex items-center justify-between">
									<span class="text-sm font-semibold text-foreground">
										{formatFactType(la.fact_type)}
									</span>
									{#if la.research_status}
										<UncertaintyBadge status={la.research_status} showLabel size="small" />
									{/if}
								</div>
								<p class="m-0 text-[0.8125rem] leading-snug text-muted-foreground">{la.conclusion}</p>
								{#if la.citation_ids && la.citation_ids.length > 0}
									<span class="mt-2 inline-block text-xs text-muted-foreground">
										{la.citation_ids.length} citations
									</span>
								{/if}
							</a>
						{/each}
					</div>
				</InfoSection>
			{:else if conflict.analysis_ids && conflict.analysis_ids.length > 0}
				<InfoSection title={`Linked Analysis IDs (${conflict.analysis_ids.length})`}>
					<ul class="m-0 flex list-none flex-col gap-1.5 p-0">
						{#each conflict.analysis_ids as aid}
							<li>
								<a href="/evidence/analyses/{aid}" class="text-link no-underline hover:underline">
									<code class="rounded bg-slate-100 px-2 py-1 text-[0.8125rem]">{aid}</code>
								</a>
							</li>
						{/each}
					</ul>
				</InfoSection>
			{/if}

			{#if conflict.status === 'open'}
				<div class="mt-6 border-t-2 border-red-200 pt-6">
					<h2 class="m-0 mb-2 text-base font-semibold text-foreground">Resolve Conflict</h2>
					<p class="m-0 mb-4 text-[0.8125rem] text-muted-foreground">
						Provide a resolution explaining how this conflict was addressed.
					</p>
					<div class="flex flex-col gap-1.5">
						<Label for="resolution" class="text-sm text-muted-foreground">
							Resolution <span class="text-red-600">*</span>
						</Label>
						<Textarea
							id="resolution"
							bind:value={resolutionText}
							rows={4}
							placeholder="Describe how this conflict was resolved..."
							aria-label="Resolution text"
						/>
					</div>
					<!-- Beside the Resolve button, where the user is looking when it fails. -->
					{#if saveConflict.latest}
						<div class="mt-4">
							<SaveConflict
								noun="conflict"
								fields={saveConflict.fields(conflictRows)}
								onKeepMine={saveConflict.keepMine}
								onUseLatest={saveConflict.useLatest}
								busy={resolving}
							/>
						</div>
					{/if}
					<div class="mt-4 flex justify-end">
						<Button onclick={resolveConflict} disabled={resolving}>
							{resolving ? 'Resolving...' : 'Resolve Conflict'}
						</Button>
					</div>
				</div>
			{/if}

			<div
				class="mt-6 flex flex-wrap gap-6 border-t border-slate-200 pt-4 text-xs text-muted-foreground"
			>
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
