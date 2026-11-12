<script lang="ts">
	import { untrack } from 'svelte';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import {
		api,
		isVersionConflict,
		recordLoadError,
		type ResearchLogResponse,
		type ResearchLogCreateRequest,
		type ResearchOutcome
	} from '#lib/api/client.js';
	import { Button } from '#lib/components/ui/button/index.js';
	import { Badge } from '#lib/components/ui/badge/index.js';
	import { Input } from '#lib/components/ui/input/index.js';
	import { Label } from '#lib/components/ui/label/index.js';
	import { Textarea } from '#lib/components/ui/textarea/index.js';
	import { toRFC3339, outcomeBadgeProps } from '#lib/utils/evidence.js';
	import { SubjectNames, subjectHref, type Subject } from '#lib/utils/subjectNames.svelte.js';
	import { nativeSelectClass } from '#lib/utils/forms.js';
	import ErrorState from '#lib/components/ErrorState.svelte';
	import ActionError from '#lib/components/ActionError.svelte';
	import ActionBar from '#lib/components/ActionBar.svelte';
	import PageHeader from '#lib/components/PageHeader.svelte';
	import InfoSection from '#lib/components/InfoSection.svelte';
	import FormRow from '#lib/components/FormRow.svelte';
	import SaveConflict from '#lib/components/SaveConflict.svelte';
	import { SaveConflictFlow, type ConflictRow } from '#lib/utils/saveConflict.svelte.js';

	const outcomes: readonly ResearchOutcome[] = ['found', 'not_found', 'inconclusive'];

	let log: ResearchLogResponse | null = $state(null);
	let loading = $state(true);
	let error: string | null = $state(null);
	let editing = $state(false);
	let saving = $state(false);
	let deleting = $state(false);
	let isNew = $state(false);

	let formData = $state({
		subject_id: '',
		subject_type: 'person',
		repository: '',
		search_description: '',
		outcome: 'inconclusive' as ResearchOutcome,
		notes: '',
		search_date: new Date().toISOString().split('T')[0]
	});

	const saveConflict = new SaveConflictFlow<ResearchLogResponse, typeof formData>({
		base: () => log,
		formFrom,
		form: () => formData,
		adopt: (latest, form) => {
			log = latest;
			formData = form;
		},
		save: () => saveLog()
	});

	// The edit form's fields, in its order, for comparing with the latest version.
	const conflictRows: ConflictRow<typeof formData>[] = [
		{ key: 'subject_id', label: 'Subject ID' },
		{ key: 'subject_type', label: 'Subject Type' },
		{ key: 'repository', label: 'Repository' },
		{ key: 'search_date', label: 'Search Date' },
		{ key: 'search_description', label: 'Search Description' },
		{ key: 'outcome', label: 'Outcome', format: (value) => formatOutcome(String(value)) },
		{ key: 'notes', label: 'Notes' }
	];

	// The log stores its subject's id; the page shows the subject's name.
	const subjects = new SubjectNames();
	function subjectOf(saved: ResearchLogResponse): Subject {
		return { type: saved.subject_type === 'family' ? 'family' : 'person', id: saved.subject_id };
	}

	function formatOutcome(outcome: string): string {
		return outcome.replace(/_/g, ' ').replace(/\b\w/g, (c) => c.toUpperCase());
	}

	async function loadLog(id: string, urlSubjectId?: string) {
		if (id === 'new') {
			log = null;
			error = null;
			formData = {
				subject_id: urlSubjectId ?? '',
				subject_type: 'person',
				repository: '',
				search_description: '',
				outcome: 'inconclusive' as const,
				notes: '',
				search_date: new Date().toISOString().split('T')[0]
			};
			isNew = true;
			editing = true;
			loading = false;
			return;
		}
		isNew = false;
		editing = false;
		loading = true;
		error = null;
		try {
			log = await api.getResearchLog(id);
			subjects.load([subjectOf(log)]);
			resetForm();
		} catch (e) {
			error = recordLoadError(e, 'research log');
			log = null;
		} finally {
			loading = false;
		}
	}

	/** The edit form's values for a research log as saved. */
	function formFrom(saved: ResearchLogResponse): typeof formData {
		return {
			subject_id: saved.subject_id,
			subject_type: saved.subject_type,
			repository: saved.repository,
			search_description: saved.search_description,
			outcome: saved.outcome,
			notes: saved.notes || '',
			search_date: saved.search_date.split('T')[0]
		};
	}

	function resetForm() {
		if (log) {
			formData = formFrom(log);
		}
	}

	function startEdit() {
		resetForm();
		saveConflict.clear();
		editing = true;
	}

	function cancelEdit() {
		if (isNew) {
			goto('/evidence');
			return;
		}
		resetForm();
		saveConflict.clear();
		editing = false;
	}

	async function saveLog() {
		const errors: string[] = [];
		if (!formData.subject_id.trim()) errors.push('Subject ID is required');
		if (!formData.repository.trim()) errors.push('Repository is required');
		if (!formData.search_description.trim()) errors.push('Search description is required');
		if (!formData.search_date) errors.push('Search date is required');
		if (errors.length > 0) {
			error = errors.join('. ');
			return;
		}

		saving = true;
		error = null;
		saveConflict.clear();
		try {
			if (isNew) {
				const data: ResearchLogCreateRequest = {
					subject_id: formData.subject_id.trim(),
					subject_type: formData.subject_type.trim(),
					repository: formData.repository.trim(),
					search_description: formData.search_description.trim(),
					outcome: formData.outcome,
					notes: formData.notes.trim() || undefined,
					search_date: toRFC3339(formData.search_date)
				};
				const created = await api.createResearchLog(data);
				goto(`/evidence/research-logs/${created.id}`);
			} else if (log) {
				await api.updateResearchLog(log.id, {
					subject_id: formData.subject_id.trim(),
					subject_type: formData.subject_type.trim(),
					repository: formData.repository.trim(),
					search_description: formData.search_description.trim(),
					outcome: formData.outcome,
					notes: formData.notes.trim() || undefined,
					search_date: toRFC3339(formData.search_date),
					version: log.version
				});
				await loadLog(log.id);
				editing = false;
			}
		} catch (e) {
			// Only a stale version is compared with the latest (#940); other 409s
			// are refusals, and a research log that can't be re-read shows the refusal.
			const id = log?.id;
			if (!(id && isVersionConflict(e) && (await saveConflict.load(() => api.getResearchLog(id))))) {
				error = (e as { message?: string }).message || 'Failed to save';
			}
		} finally {
			saving = false;
		}
	}

	async function deleteLog() {
		if (!log) return;
		if (!confirm('Delete this research log? This cannot be undone.')) return;

		deleting = true;
		error = null;
		try {
			await api.deleteResearchLog(log.id, log.version);
			await goto('/evidence');
		} catch (e) {
			error = (e as { message?: string }).message || 'Failed to delete';
		} finally {
			deleting = false;
		}
	}

	$effect(() => {
		const id = page.params.id;
		// Track subjectId so navigating ?subjectId=A → ?subjectId=B re-prefills
		const subjectId = page.url?.searchParams?.get('subjectId');
		if (id) {
			untrack(() => loadLog(id, subjectId ?? undefined));
		}
	});
</script>

<svelte:head>
	<title>{isNew ? 'New Research Log' : log ? `${log.repository} | Research Log` : 'Research Log'} | My Family</title>
</svelte:head>

{#snippet recordActions()}
	<Button variant="outline" onclick={startEdit}>Edit</Button>
	<Button variant="destructive" onclick={deleteLog} disabled={deleting}>
		{deleting ? 'Deleting...' : 'Delete'}
	</Button>
{/snippet}

<div class="mx-auto max-w-3xl p-6">
	<ActionBar
		back={{ href: '/evidence', label: 'Evidence' }}
		actions={log && !editing ? recordActions : undefined}
	/>

	{#if loading}
		<div class="p-12 text-center text-muted-foreground">Loading...</div>
	{:else if error && !log && !isNew}
		<ErrorState message={error} onRetry={() => loadLog(page.params.id!)} />
	{:else if editing}
		<form
			class="rounded-xl border border-slate-200 bg-white p-6"
			onsubmit={(e) => {
				e.preventDefault();
				saveLog();
			}}
		>
			<PageHeader title={isNew ? 'New Research Log' : 'Edit Research Log'} />

			{#if error}
				<ActionError message={error} />
			{/if}

			<FormRow>
				<div class="flex flex-col gap-1.5">
					<Label for="subject-id" class="text-sm text-muted-foreground">
						Subject ID <span class="text-red-600">*</span>
					</Label>
					<Input
						id="subject-id"
						type="text"
						bind:value={formData.subject_id}
						required
						placeholder="Person or family UUID"
					/>
				</div>
				<div class="flex flex-col gap-1.5">
					<Label for="subject-type" class="text-sm text-muted-foreground">Subject Type</Label>
					<select
						id="subject-type"
						bind:value={formData.subject_type}
						class={nativeSelectClass}
					>
						<option value="person">Person</option>
						<option value="family">Family</option>
					</select>
				</div>
			</FormRow>

			<FormRow>
				<div class="flex flex-col gap-1.5">
					<Label for="repository" class="text-sm text-muted-foreground">
						Repository <span class="text-red-600">*</span>
					</Label>
					<Input
						id="repository"
						type="text"
						bind:value={formData.repository}
						required
						placeholder="e.g., National Archives"
					/>
				</div>
				<div class="flex flex-col gap-1.5">
					<Label for="search-date" class="text-sm text-muted-foreground">
						Search Date <span class="text-red-600">*</span>
					</Label>
					<Input id="search-date" type="date" bind:value={formData.search_date} required />
				</div>
			</FormRow>

			<div class="mb-4 flex flex-col gap-1.5">
				<Label for="search-description" class="text-sm text-muted-foreground">
					Search Description <span class="text-red-600">*</span>
				</Label>
				<Textarea
					id="search-description"
					bind:value={formData.search_description}
					rows={3}
					required
				/>
			</div>

			<FormRow>
				<div class="flex flex-col gap-1.5">
					<Label for="outcome" class="text-sm text-muted-foreground">Outcome</Label>
					<select id="outcome" bind:value={formData.outcome} class={nativeSelectClass}>
						{#each outcomes as o}
							<option value={o}>{formatOutcome(o)}</option>
						{/each}
					</select>
				</div>
			</FormRow>

			<div class="mb-4 flex flex-col gap-1.5">
				<Label for="notes" class="text-sm text-muted-foreground">Notes</Label>
				<Textarea id="notes" bind:value={formData.notes} rows={3} />
			</div>

			<!-- Beside the Save button, where the user is looking when it fails. -->
			{#if saveConflict.latest}
				<SaveConflict
					noun="research log"
					fields={saveConflict.fields(conflictRows)}
					onKeepMine={saveConflict.keepMine}
					onUseLatest={saveConflict.useLatest}
					busy={saving}
				/>
			{/if}

			<div class="mt-6 flex justify-end gap-3 border-t border-slate-200 pt-4">
				<Button variant="outline" onclick={cancelEdit} disabled={saving}>Cancel</Button>
				<Button type="submit" disabled={saving}>
					{saving ? 'Saving...' : isNew ? 'Create Research Log' : 'Save Changes'}
				</Button>
			</div>
		</form>
	{:else if log}
		{@const outcomeBadge = outcomeBadgeProps(log.outcome)}
		{@const subject = subjectOf(log)}
		<div class="rounded-xl border border-slate-200 bg-white p-6">
			{#snippet logBadges()}
				<Badge variant={outcomeBadge.variant} class={outcomeBadge.class}>
					{outcomeBadge.label}
				</Badge>
			{/snippet}
			<PageHeader title="Research Log" badges={logBadges} />

			{#if error}
				<ActionError message={error} />
			{/if}

			<InfoSection
				title="Details"
				fields={[
					{ label: 'Subject', value: subjects.label(subject), href: subjectHref(subject) },
					{
						label: 'Subject Type',
						value: log.subject_type.charAt(0).toUpperCase() + log.subject_type.slice(1)
					},
					{ label: 'Repository', value: log.repository },
					{ label: 'Search Date', value: new Date(log.search_date).toLocaleDateString() },
					{ label: 'Outcome', value: formatOutcome(log.outcome) }
				]}
			/>

			<InfoSection title="Search Description">
				<p class="m-0 whitespace-pre-wrap text-sm leading-relaxed text-foreground">
					{log.search_description}
				</p>
			</InfoSection>

			{#if log.notes}
				<InfoSection title="Notes">
					<p class="m-0 whitespace-pre-wrap text-sm leading-relaxed text-foreground">
						{log.notes}
					</p>
				</InfoSection>
			{/if}

			<div
				class="mt-6 flex flex-wrap gap-6 border-t border-slate-200 pt-4 text-xs text-muted-foreground"
			>
				{#if log.created_at}
					<span>Created: {new Date(log.created_at).toLocaleDateString()}</span>
				{/if}
				{#if log.updated_at}
					<span>Updated: {new Date(log.updated_at).toLocaleDateString()}</span>
				{/if}
				<span>Version: {log.version}</span>
			</div>
		</div>
	{/if}
</div>
