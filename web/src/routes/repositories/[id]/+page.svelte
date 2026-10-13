<script lang="ts">
	import MainlineNotice from '$lib/components/MainlineNotice.svelte';
	import { page } from '$app/stores';
	import { goto } from '$app/navigation';
	import { api, isVersionConflict, recordLoadError, type RepositoryDetail, type Address } from '$lib/api/client';
	import ExternalLinks from '$lib/components/ExternalLinks.svelte';
	import InfoSection from '$lib/components/InfoSection.svelte';
	import { Button } from '$lib/components/ui/button';
	import ActionBar from '$lib/components/ActionBar.svelte';
	import FormRow from '$lib/components/FormRow.svelte';
	import ErrorState from '$lib/components/ErrorState.svelte';
	import ActionError from '$lib/components/ActionError.svelte';
	import SaveConflict from '$lib/components/SaveConflict.svelte';
	import { SaveConflictFlow, type ConflictRow } from '$lib/utils/saveConflict.svelte';

	let repository: RepositoryDetail | null = $state(null);
	let loading = $state(true);
	let error: string | null = $state(null);
	let editing = $state(false);
	let saving = $state(false);
	let deleting = $state(false);

	const emptyForm = () => ({
		name: '',
		line1: '',
		line2: '',
		city: '',
		state: '',
		postal_code: '',
		country: '',
		phone: '',
		email: '',
		fax: '',
		notes: '',
		gedcom_xref: ''
	});

	let formData = $state(emptyForm());

	const saveConflict = new SaveConflictFlow<RepositoryDetail, typeof formData>({
		base: () => repository,
		formFrom,
		form: () => formData,
		adopt: (latest, form) => {
			repository = latest;
			formData = form;
		},
		save: () => saveRepository()
	});

	// The edit form's fields, in its order, for comparing with the latest version.
	const conflictRows: ConflictRow<typeof formData>[] = [
		{ key: 'name', label: 'Name' },
		{ key: 'line1', label: 'Address Line 1' },
		{ key: 'line2', label: 'Address Line 2' },
		{ key: 'city', label: 'City' },
		{ key: 'state', label: 'State/Province' },
		{ key: 'postal_code', label: 'Postal Code' },
		{ key: 'country', label: 'Country' },
		{ key: 'phone', label: 'Phone' },
		{ key: 'email', label: 'Email' },
		{ key: 'fax', label: 'Fax' },
		{ key: 'notes', label: 'Notes' },
		{ key: 'gedcom_xref', label: 'GEDCOM Xref' }
	];

	async function loadRepository(id: string) {
		loading = true;
		error = null;
		try {
			repository = await api.getRepository(id);
			resetForm();
		} catch (e) {
			error = recordLoadError(e, 'repository');
			repository = null;
		} finally {
			loading = false;
		}
	}

	/** The edit form's values for a repository as saved. */
	function formFrom(saved: RepositoryDetail): ReturnType<typeof emptyForm> {
		const a = saved.address ?? {};
		return {
			name: saved.name,
			line1: a.line1 ?? '',
			line2: a.line2 ?? '',
			city: a.city ?? '',
			state: a.state ?? '',
			postal_code: a.postal_code ?? '',
			country: a.country ?? '',
			phone: a.phone ?? '',
			email: a.email ?? '',
			fax: a.fax ?? '',
			notes: saved.notes ?? '',
			gedcom_xref: saved.gedcom_xref ?? ''
		};
	}

	function resetForm() {
		if (repository) {
			formData = formFrom(repository);
		}
	}

	function startEdit() {
		resetForm();
		editing = true;
		error = null;
		saveConflict.clear();
	}

	function cancelEdit() {
		resetForm();
		editing = false;
		error = null;
		saveConflict.clear();
	}

	function buildAddress(f: ReturnType<typeof emptyForm>): Address | undefined {
		const address: Address = {};
		if (f.line1.trim()) address.line1 = f.line1.trim();
		if (f.line2.trim()) address.line2 = f.line2.trim();
		if (f.city.trim()) address.city = f.city.trim();
		if (f.state.trim()) address.state = f.state.trim();
		if (f.postal_code.trim()) address.postal_code = f.postal_code.trim();
		if (f.country.trim()) address.country = f.country.trim();
		if (f.phone.trim()) address.phone = f.phone.trim();
		if (f.email.trim()) address.email = f.email.trim();
		if (f.fax.trim()) address.fax = f.fax.trim();
		return Object.keys(address).length > 0 ? address : undefined;
	}

	async function saveRepository() {
		if (!repository) return;
		if (!formData.name.trim()) {
			error = 'Name is required';
			return;
		}

		saving = true;
		error = null;
		saveConflict.clear();
		try {
			await api.updateRepository(repository.id, {
				name: formData.name.trim(),
				// An empty address or string clears the field; an omitted one is
				// left unchanged.
				address: buildAddress(formData) ?? (repository.address ? {} : undefined),
				notes: formData.notes.trim(),
				gedcom_xref: formData.gedcom_xref.trim(),
				version: repository.version
			});
			await loadRepository(repository.id);
			editing = false;
		} catch (e) {
			// Only a stale version is compared with the latest (#940); other 409s
			// are refusals, and a repository that can't be re-read shows the refusal.
			const id = repository.id;
			if (!(isVersionConflict(e) && (await saveConflict.load(() => api.getRepository(id))))) {
				error = (e as { message?: string }).message || 'Failed to save';
			}
		} finally {
			saving = false;
		}
	}

	async function deleteRepository() {
		if (!repository) return;
		if (!confirm(`Delete "${repository.name}"? This cannot be undone.`)) return;

		deleting = true;
		error = null;
		try {
			await api.deleteRepository(repository.id, repository.version);
			goto('/repositories');
		} catch (e) {
			const err = e as { message?: string; status?: number };
			if (err.status === 409) {
				error = 'This repository was modified by someone else. Reload and try again.';
			} else if (err.status === 404) {
				error = 'This repository no longer exists.';
			} else {
				error = err.message || 'Failed to delete';
			}
			deleting = false;
		}
	}

	// Whether the repository has any address field populated.
	function hasAddress(a?: Address): boolean {
		return !!a && Object.values(a).some((v) => v != null && v !== '');
	}

	$effect(() => {
		const id = $page.params.id;
		if (id) {
			loadRepository(id);
		}
	});
</script>

<svelte:head>
	<title>{repository ? repository.name : 'Repository'} | My Family</title>
</svelte:head>

{#snippet repositoryActions()}
	<Button variant="outline" onclick={startEdit}>Edit</Button>
	<Button variant="destructive" onclick={deleteRepository} disabled={deleting}>
		{deleting ? 'Deleting...' : 'Delete'}
	</Button>
{/snippet}

<div class="repository-page">
	<ActionBar back={{ href: '/repositories', label: 'Repositories' }} actions={repository && !editing ? repositoryActions : undefined} />

	<MainlineNotice
		message="Repositories are shared across all branches. Creating, editing or deleting a repository here changes the mainline, and every research branch sees the change."
	/>

	{#if loading}
		<div class="loading">Loading...</div>
	{:else if error && !repository}
		<ErrorState message={error} onRetry={() => loadRepository($page.params.id ?? '')} />
	{:else if repository}
		<!-- In the form, the error shows in it; out of it, a failed delete shows here. -->
		{#if error && !editing}
			<ActionError message={error} />
		{/if}
		{#if editing}
			<form class="entity-form" onsubmit={(e) => { e.preventDefault(); saveRepository(); }}>
				{#if error}
					<div class="form-error">{error}</div>
				{/if}

				<label>
					<span>Name <span class="required">*</span></span>
					<input type="text" bind:value={formData.name} required />
				</label>

				<fieldset>
					<legend>Address</legend>
					<FormRow>
						<label>
							Address Line 1
							<input type="text" bind:value={formData.line1} />
						</label>
						<label>
							Address Line 2
							<input type="text" bind:value={formData.line2} />
						</label>
					</FormRow>
					<FormRow>
						<label>
							City
							<input type="text" bind:value={formData.city} />
						</label>
						<label>
							State/Province
							<input type="text" bind:value={formData.state} />
						</label>
					</FormRow>
					<FormRow>
						<label>
							Postal Code
							<input type="text" bind:value={formData.postal_code} />
						</label>
						<label>
							Country
							<input type="text" bind:value={formData.country} />
						</label>
					</FormRow>
					<FormRow>
						<label>
							Phone
							<input type="tel" bind:value={formData.phone} />
						</label>
						<label>
							Email
							<input type="email" bind:value={formData.email} />
						</label>
					</FormRow>
					<FormRow>
						<label>
							Fax
							<input type="tel" bind:value={formData.fax} />
						</label>
					</FormRow>
				</fieldset>

				<label>
					Notes
					<textarea bind:value={formData.notes} rows="4"></textarea>
				</label>

				<label>
					GEDCOM Xref
					<input type="text" bind:value={formData.gedcom_xref} placeholder="e.g., @R1@" />
				</label>

				<!-- Beside the Save button, where the user is looking when it fails. -->
				{#if saveConflict.latest}
					<SaveConflict
						noun="repository"
						fields={saveConflict.fields(conflictRows)}
						onKeepMine={saveConflict.keepMine}
						onUseLatest={saveConflict.useLatest}
						busy={saving}
					/>
				{/if}

				<div class="form-actions">
					<Button variant="outline" onclick={cancelEdit} disabled={saving}>Cancel</Button>
					<Button type="submit" disabled={saving}>
						{saving ? 'Saving...' : 'Save Changes'}
					</Button>
				</div>
			</form>
		{:else}
			<div class="repository-detail">
				<div class="repository-header">
					<h1>{repository.name}</h1>
				</div>

				{#if hasAddress(repository.address)}
					{@const a = repository.address}
					<InfoSection
						title="Address"
						hideEmpty
						fields={[
							{ label: 'Line 1', value: a?.line1 },
							{ label: 'Line 2', value: a?.line2 },
							{ label: 'City', value: a?.city },
							{ label: 'State/Province', value: a?.state },
							{ label: 'Postal Code', value: a?.postal_code },
							{ label: 'Country', value: a?.country },
							{ label: 'Phone', value: a?.phone },
							{ label: 'Email', value: a?.email },
							{ label: 'Fax', value: a?.fax }
						]}
					/>
				{/if}

				{#if repository.notes}
					<InfoSection title="Notes">
						<p class="notes">{repository.notes}</p>
					</InfoSection>
				{/if}

				{#if repository.gedcom_xref}
					<InfoSection title="GEDCOM Xref">
						<p class="mono">{repository.gedcom_xref}</p>
					</InfoSection>
				{/if}

				<!-- Guard here (in addition to ExternalLinks' own empty check) so the
				     "External links" heading is suppressed when there are none. -->
				{#if repository.external_ids && repository.external_ids.length > 0}
					<InfoSection title="External links">
						<ExternalLinks externalIds={repository.external_ids} />
					</InfoSection>
				{/if}
			</div>
		{/if}
	{/if}
</div>

<style>
	.repository-page {
		max-width: 800px;
		margin: 0 auto;
		padding: 1.5rem;
	}

	.loading {
		text-align: center;
		padding: 3rem;
		color: var(--color-text-muted);
	}

	.repository-detail {
		background: white;
		border-radius: 12px;
		border: 1px solid #e2e8f0;
		padding: 1.5rem;
	}

	.repository-header {
		margin-bottom: 1.5rem;
		padding-bottom: 1.5rem;
		border-bottom: 1px solid #e2e8f0;
	}

	.repository-header h1 {
		margin: 0;
		font-size: 1.5rem;
		color: #1e293b;
	}

	.notes {
		margin: 0;
		color: #475569;
		font-size: 0.875rem;
		white-space: pre-wrap;
	}

	.mono {
		margin: 0;
		font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
		font-size: 0.875rem;
		color: #475569;
	}

	.entity-form {
		background: white;
		border-radius: 12px;
		border: 1px solid #e2e8f0;
		padding: 1.5rem;
		display: flex;
		flex-direction: column;
		gap: 1rem;
	}

	.form-error {
		padding: 0.75rem;
		background: #fef2f2;
		border: 1px solid #fecaca;
		border-radius: 6px;
		color: #dc2626;
		font-size: 0.875rem;
	}

	/* Each FormRow spaces itself 1rem below, so the bottom padding is left to it. */
	fieldset {
		border: 1px solid #e2e8f0;
		border-radius: 8px;
		padding: 1rem 1rem 0;
		margin: 0;
	}

	legend {
		font-size: 0.8125rem;
		font-weight: 600;
		color: var(--color-text-muted);
		padding: 0 0.375rem;
		text-transform: uppercase;
		letter-spacing: 0.05em;
	}

	.entity-form label {
		display: flex;
		flex-direction: column;
		gap: 0.375rem;
		font-size: 0.875rem;
		color: #475569;
	}

	.required {
		color: #dc2626;
	}

	.entity-form input,
	.entity-form textarea {
		padding: 0.625rem 0.75rem;
		border: 1px solid #e2e8f0;
		border-radius: 6px;
		font-size: 0.875rem;
	}

	.entity-form input:focus,
	.entity-form textarea:focus {
		outline: none;
		border-color: #3b82f6;
		box-shadow: 0 0 0 3px rgba(59, 130, 246, 0.1);
	}

	.entity-form textarea {
		resize: vertical;
	}

	.form-actions {
		display: flex;
		justify-content: flex-end;
		gap: 0.75rem;
		margin-top: 0.5rem;
		padding-top: 1rem;
		border-top: 1px solid #e2e8f0;
	}
</style>
