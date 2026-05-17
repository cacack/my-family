<script lang="ts">
	import { untrack } from 'svelte';
	import { page } from '$app/stores';
	import { goto } from '$app/navigation';
	import {
		api,
		type ResearchLogResponse,
		type ResearchLogCreateRequest
	} from '$lib/api/client';
	import { Button } from '$lib/components/ui/button';
	import { Badge } from '$lib/components/ui/badge';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { Textarea } from '$lib/components/ui/textarea';
	import { toRFC3339, outcomeBadgeProps } from '$lib/utils/evidence';
	import { nativeSelectClass } from '$lib/utils/forms';

	const outcomes = ['found', 'not_found', 'inconclusive'] as const;

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
		outcome: 'inconclusive' as 'found' | 'not_found' | 'inconclusive',
		notes: '',
		search_date: new Date().toISOString().split('T')[0]
	});

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
			resetForm();
		} catch (e) {
			error = (e as { message?: string }).message || 'Failed to load research log';
			log = null;
		} finally {
			loading = false;
		}
	}

	function resetForm() {
		if (log) {
			formData = {
				subject_id: log.subject_id,
				subject_type: log.subject_type,
				repository: log.repository,
				search_description: log.search_description,
				outcome: log.outcome,
				notes: log.notes || '',
				search_date: log.search_date.split('T')[0]
			};
		}
	}

	function startEdit() {
		resetForm();
		editing = true;
	}

	function cancelEdit() {
		if (isNew) {
			goto('/evidence');
			return;
		}
		resetForm();
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
			const status = (e as { status?: number }).status;
			if (status === 409) {
				error = 'Version conflict: someone else modified this record. Please reload and try again.';
			} else {
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
		const id = $page.params.id;
		// Track subjectId so navigating ?subjectId=A → ?subjectId=B re-prefills
		const subjectId = $page.url?.searchParams?.get('subjectId');
		if (id) {
			untrack(() => loadLog(id, subjectId ?? undefined));
		}
	});
</script>

<svelte:head>
	<title>{isNew ? 'New Research Log' : 'Research Log'} | My Family</title>
</svelte:head>

<div class="mx-auto max-w-3xl p-6">
	<header class="mb-6 flex items-center justify-between">
		<a href="/evidence" class="text-sm text-slate-500 no-underline hover:text-blue-500">
			&larr; Evidence
		</a>
		{#if log && !editing}
			<div class="flex gap-2">
				<Button variant="outline" onclick={startEdit}>Edit</Button>
				<Button variant="destructive" onclick={deleteLog} disabled={deleting}>
					{deleting ? 'Deleting...' : 'Delete'}
				</Button>
			</div>
		{/if}
	</header>

	{#if loading}
		<div class="p-12 text-center text-slate-500">Loading...</div>
	{:else if error && !log && !isNew}
		<div class="p-12 text-center text-red-600">
			<p class="m-0 mb-4">{error}</p>
			<Button variant="outline" onclick={() => loadLog($page.params.id!)}>Retry</Button>
		</div>
	{:else if editing}
		<form
			class="rounded-xl border border-slate-200 bg-white p-6"
			onsubmit={(e) => {
				e.preventDefault();
				saveLog();
			}}
		>
			<h1 class="m-0 mb-6 text-xl text-slate-800">
				{isNew ? 'New Research Log' : 'Edit Research Log'}
			</h1>

			{#if error}
				<div
					class="mb-4 rounded-md border border-red-200 bg-red-50 p-3 text-sm text-red-600"
					role="alert"
				>
					{error}
				</div>
			{/if}

			<div class="mb-4 grid grid-cols-1 gap-4 sm:grid-cols-2">
				<div class="flex flex-col gap-1.5">
					<Label for="subject-id" class="text-sm text-slate-600">
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
					<Label for="subject-type" class="text-sm text-slate-600">Subject Type</Label>
					<select
						id="subject-type"
						bind:value={formData.subject_type}
						class={nativeSelectClass}
					>
						<option value="person">Person</option>
						<option value="family">Family</option>
					</select>
				</div>
			</div>

			<div class="mb-4 grid grid-cols-1 gap-4 sm:grid-cols-2">
				<div class="flex flex-col gap-1.5">
					<Label for="repository" class="text-sm text-slate-600">
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
					<Label for="search-date" class="text-sm text-slate-600">
						Search Date <span class="text-red-600">*</span>
					</Label>
					<Input id="search-date" type="date" bind:value={formData.search_date} required />
				</div>
			</div>

			<div class="mb-4 flex flex-col gap-1.5">
				<Label for="search-description" class="text-sm text-slate-600">
					Search Description <span class="text-red-600">*</span>
				</Label>
				<Textarea
					id="search-description"
					bind:value={formData.search_description}
					rows={3}
					required
				/>
			</div>

			<div class="mb-4 grid grid-cols-1 gap-4 sm:grid-cols-2">
				<div class="flex flex-col gap-1.5">
					<Label for="outcome" class="text-sm text-slate-600">Outcome</Label>
					<select id="outcome" bind:value={formData.outcome} class={nativeSelectClass}>
						{#each outcomes as o}
							<option value={o}>{formatOutcome(o)}</option>
						{/each}
					</select>
				</div>
			</div>

			<div class="mb-4 flex flex-col gap-1.5">
				<Label for="notes" class="text-sm text-slate-600">Notes</Label>
				<Textarea id="notes" bind:value={formData.notes} rows={3} />
			</div>

			<div class="mt-6 flex justify-end gap-3 border-t border-slate-200 pt-4">
				<Button variant="outline" onclick={cancelEdit} disabled={saving}>Cancel</Button>
				<Button type="submit" disabled={saving}>
					{saving ? 'Saving...' : isNew ? 'Create Research Log' : 'Save Changes'}
				</Button>
			</div>
		</form>
	{:else if log}
		{@const outcomeBadge = outcomeBadgeProps(log.outcome)}
		<div class="rounded-xl border border-slate-200 bg-white p-6">
			<div class="mb-6 border-b border-slate-200 pb-4">
				<h1 class="m-0 mb-2 text-2xl text-slate-800">Research Log</h1>
				<div class="flex items-center gap-2">
					<Badge variant={outcomeBadge.variant} class={outcomeBadge.class}>
						{outcomeBadge.label}
					</Badge>
				</div>
			</div>

			<div
				class="mb-6 grid grid-cols-[repeat(auto-fit,minmax(250px,1fr))] gap-6"
			>
				<div class="mb-6">
					<h2
						class="m-0 mb-3 text-sm font-semibold uppercase tracking-wider text-slate-500"
					>
						Details
					</h2>
					<dl class="m-0 grid grid-cols-[auto_1fr] gap-x-4 gap-y-1">
						<dt class="text-[0.8125rem] text-slate-400">Subject</dt>
						<dd class="m-0 text-sm text-slate-800">
							<a
								href="/{log.subject_type === 'family' ? 'families' : 'persons'}/{log.subject_id}"
								class="text-blue-500 no-underline hover:underline"
							>
								{log.subject_id}
							</a>
						</dd>
						<dt class="text-[0.8125rem] text-slate-400">Subject Type</dt>
						<dd class="m-0 text-sm text-slate-800">
							{log.subject_type.charAt(0).toUpperCase() + log.subject_type.slice(1)}
						</dd>
						<dt class="text-[0.8125rem] text-slate-400">Repository</dt>
						<dd class="m-0 text-sm text-slate-800">{log.repository}</dd>
						<dt class="text-[0.8125rem] text-slate-400">Search Date</dt>
						<dd class="m-0 text-sm text-slate-800">
							{new Date(log.search_date).toLocaleDateString()}
						</dd>
						<dt class="text-[0.8125rem] text-slate-400">Outcome</dt>
						<dd class="m-0 text-sm text-slate-800">{formatOutcome(log.outcome)}</dd>
					</dl>
				</div>
			</div>

			<div class="mb-6">
				<h2 class="m-0 mb-3 text-sm font-semibold uppercase tracking-wider text-slate-500">
					Search Description
				</h2>
				<p class="m-0 whitespace-pre-wrap text-sm leading-relaxed text-slate-600">
					{log.search_description}
				</p>
			</div>

			{#if log.notes}
				<div class="mb-6">
					<h2 class="m-0 mb-3 text-sm font-semibold uppercase tracking-wider text-slate-500">
						Notes
					</h2>
					<p class="m-0 whitespace-pre-wrap text-sm leading-relaxed text-slate-600">
						{log.notes}
					</p>
				</div>
			{/if}

			<div
				class="mt-6 flex flex-wrap gap-6 border-t border-slate-200 pt-4 text-xs text-slate-400"
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
