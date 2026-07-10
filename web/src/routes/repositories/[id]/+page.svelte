<script lang="ts">
	import { page } from '$app/stores';
	import { goto } from '$app/navigation';
	import { api, type RepositoryDetail, type Address } from '$lib/api/client';
	import ExternalLinks from '$lib/components/ExternalLinks.svelte';
	import { Button } from '$lib/components/ui/button';

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

	async function loadRepository(id: string) {
		loading = true;
		error = null;
		try {
			repository = await api.getRepository(id);
			resetForm();
		} catch (e) {
			error = (e as { message?: string }).message || 'Failed to load repository';
			repository = null;
		} finally {
			loading = false;
		}
	}

	function resetForm() {
		if (repository) {
			const a = repository.address ?? {};
			formData = {
				name: repository.name,
				line1: a.line1 ?? '',
				line2: a.line2 ?? '',
				city: a.city ?? '',
				state: a.state ?? '',
				postal_code: a.postal_code ?? '',
				country: a.country ?? '',
				phone: a.phone ?? '',
				email: a.email ?? '',
				fax: a.fax ?? '',
				notes: repository.notes ?? '',
				gedcom_xref: repository.gedcom_xref ?? ''
			};
		}
	}

	function startEdit() {
		resetForm();
		editing = true;
		error = null;
	}

	function cancelEdit() {
		resetForm();
		editing = false;
		error = null;
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
		try {
			await api.updateRepository(repository.id, {
				name: formData.name.trim(),
				address: buildAddress(formData),
				notes: formData.notes.trim() || undefined,
				gedcom_xref: formData.gedcom_xref.trim() || undefined,
				version: repository.version
			});
			await loadRepository(repository.id);
			editing = false;
		} catch (e) {
			const err = e as { message?: string; status?: number };
			if (err.status === 409) {
				error = 'This repository was modified by someone else. Reload and try again.';
			} else {
				error = err.message || 'Failed to save';
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

<div class="repository-page">
	<header class="page-header">
		<a href="/repositories" class="back-link">&larr; Repositories</a>
		{#if repository && !editing}
			<div class="actions">
				<Button variant="outline" onclick={startEdit}>Edit</Button>
				<Button variant="destructive" onclick={deleteRepository} disabled={deleting}>
					{deleting ? 'Deleting...' : 'Delete'}
				</Button>
			</div>
		{/if}
	</header>

	{#if loading}
		<div class="loading">Loading...</div>
	{:else if error && !repository}
		<div class="error">{error}</div>
	{:else if repository}
		{#if editing}
			<form class="entity-form" onsubmit={(e) => { e.preventDefault(); saveRepository(); }}>
				{#if error}
					<div class="form-error">{error}</div>
				{/if}

				<label>
					Name <span class="required">*</span>
					<input type="text" bind:value={formData.name} required />
				</label>

				<fieldset>
					<legend>Address</legend>
					<div class="form-row">
						<label>
							Address Line 1
							<input type="text" bind:value={formData.line1} />
						</label>
						<label>
							Address Line 2
							<input type="text" bind:value={formData.line2} />
						</label>
					</div>
					<div class="form-row">
						<label>
							City
							<input type="text" bind:value={formData.city} />
						</label>
						<label>
							State/Province
							<input type="text" bind:value={formData.state} />
						</label>
					</div>
					<div class="form-row">
						<label>
							Postal Code
							<input type="text" bind:value={formData.postal_code} />
						</label>
						<label>
							Country
							<input type="text" bind:value={formData.country} />
						</label>
					</div>
					<div class="form-row">
						<label>
							Phone
							<input type="tel" bind:value={formData.phone} />
						</label>
						<label>
							Email
							<input type="email" bind:value={formData.email} />
						</label>
					</div>
					<div class="form-row">
						<label>
							Fax
							<input type="tel" bind:value={formData.fax} />
						</label>
					</div>
				</fieldset>

				<label>
					Notes
					<textarea bind:value={formData.notes} rows="4"></textarea>
				</label>

				<label>
					GEDCOM Xref
					<input type="text" bind:value={formData.gedcom_xref} placeholder="e.g., @R1@" />
				</label>

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
					<div class="info-section">
						<h2>Address</h2>
						<dl>
							{#if a?.line1}<dt>Line 1</dt><dd>{a.line1}</dd>{/if}
							{#if a?.line2}<dt>Line 2</dt><dd>{a.line2}</dd>{/if}
							{#if a?.city}<dt>City</dt><dd>{a.city}</dd>{/if}
							{#if a?.state}<dt>State/Province</dt><dd>{a.state}</dd>{/if}
							{#if a?.postal_code}<dt>Postal Code</dt><dd>{a.postal_code}</dd>{/if}
							{#if a?.country}<dt>Country</dt><dd>{a.country}</dd>{/if}
							{#if a?.phone}<dt>Phone</dt><dd>{a.phone}</dd>{/if}
							{#if a?.email}<dt>Email</dt><dd>{a.email}</dd>{/if}
							{#if a?.fax}<dt>Fax</dt><dd>{a.fax}</dd>{/if}
						</dl>
					</div>
				{/if}

				{#if repository.notes}
					<div class="info-section">
						<h2>Notes</h2>
						<p class="notes">{repository.notes}</p>
					</div>
				{/if}

				{#if repository.gedcom_xref}
					<div class="info-section">
						<h2>GEDCOM Xref</h2>
						<p class="mono">{repository.gedcom_xref}</p>
					</div>
				{/if}

				<!-- Guard here (in addition to ExternalLinks' own empty check) so the
				     "External links" heading is suppressed when there are none. -->
				{#if repository.external_ids && repository.external_ids.length > 0}
					<div class="info-section">
						<h2>External links</h2>
						<ExternalLinks externalIds={repository.external_ids} />
					</div>
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

	.actions {
		display: flex;
		gap: 0.5rem;
	}

	.loading,
	.error {
		text-align: center;
		padding: 3rem;
		color: #64748b;
	}

	.error {
		color: #dc2626;
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

	.info-section {
		margin-bottom: 1.5rem;
	}

	.info-section:last-child {
		margin-bottom: 0;
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

	fieldset {
		border: 1px solid #e2e8f0;
		border-radius: 8px;
		padding: 1rem;
		margin: 0;
		display: flex;
		flex-direction: column;
		gap: 1rem;
	}

	legend {
		font-size: 0.8125rem;
		font-weight: 600;
		color: #64748b;
		padding: 0 0.375rem;
		text-transform: uppercase;
		letter-spacing: 0.05em;
	}

	.form-row {
		display: grid;
		grid-template-columns: repeat(2, 1fr);
		gap: 1rem;
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
