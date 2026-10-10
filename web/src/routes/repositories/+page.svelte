<script lang="ts">
	import MainlineNotice from '$lib/components/MainlineNotice.svelte';
	import PageHeader from '$lib/components/PageHeader.svelte';
	import { api, type Repository, type Address } from '$lib/api/client';
	import { Button } from '$lib/components/ui/button';
	import FormRow from '$lib/components/FormRow.svelte';

	let repositories: Repository[] = $state([]);
	let total = $state(0);
	let loading = $state(true);
	let loadError: string | null = $state(null);
	let currentPage = $state(1);
	let sort = $state<'name' | 'updated_at'>('name');
	let order = $state<'asc' | 'desc'>('asc');
	let showAddForm = $state(false);
	let saving = $state(false);
	let error: string | null = $state(null);
	const pageSize = 20;

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

	let newRepo = $state(emptyForm());

	async function loadRepositories() {
		loading = true;
		loadError = null;
		try {
			const result = await api.listRepositories({
				limit: pageSize,
				offset: (currentPage - 1) * pageSize,
				sort,
				order
			});
			repositories = result.repositories;
			total = result.total;
		} catch (e) {
			console.error('Failed to load repositories:', e);
			loadError = 'Failed to load repositories. Please try again.';
		} finally {
			loading = false;
		}
	}

	function handleSortChange(e: Event) {
		const select = e.target as HTMLSelectElement;
		sort = select.value as typeof sort;
		currentPage = 1;
		loadRepositories();
	}

	function handleOrderChange() {
		order = order === 'asc' ? 'desc' : 'asc';
		loadRepositories();
	}

	function prevPage() {
		if (currentPage > 1) {
			currentPage--;
			loadRepositories();
		}
	}

	function nextPage() {
		if (currentPage * pageSize < total) {
			currentPage++;
			loadRepositories();
		}
	}

	function openAddForm() {
		newRepo = emptyForm();
		showAddForm = true;
		error = null;
	}

	function cancelAdd() {
		showAddForm = false;
		error = null;
	}

	// Build an Address from form fields, returning undefined when no field is set.
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

	async function saveNewRepository() {
		if (!newRepo.name.trim()) {
			error = 'Name is required';
			return;
		}

		saving = true;
		error = null;
		try {
			await api.createRepository({
				name: newRepo.name.trim(),
				address: buildAddress(newRepo),
				notes: newRepo.notes.trim() || undefined,
				gedcom_xref: newRepo.gedcom_xref.trim() || undefined
			});
			showAddForm = false;
			loadRepositories();
		} catch (e) {
			error = (e as { message?: string }).message || 'Failed to create repository';
		} finally {
			saving = false;
		}
	}

	// Compose a one-line address summary for the table.
	function addressSummary(address?: Address): string {
		if (!address) return '';
		return [address.city, address.state, address.country].filter(Boolean).join(', ');
	}

	$effect(() => {
		loadRepositories();
	});

	const totalPages = $derived(Math.ceil(total / pageSize));
</script>

<svelte:head>
	<title>Repositories | My Family</title>
</svelte:head>

<div class="repositories-page">
	<PageHeader title="Repositories">
		{#snippet actions()}
			<div class="controls">
				<label>
					Sort by:
					<select value={sort} onchange={handleSortChange}>
						<option value="name">Name</option>
						<option value="updated_at">Last Updated</option>
					</select>
				</label>
				<button
					class="order-btn"
					onclick={handleOrderChange}
					title="Toggle sort order"
					aria-label="Sort order: {order === 'asc' ? 'ascending' : 'descending'}"
				>
					{#if order === 'asc'}
						<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true">
							<path d="M12 5v14M5 12l7-7 7 7" />
						</svg>
					{:else}
						<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true">
							<path d="M12 19V5M5 12l7 7 7-7" />
						</svg>
					{/if}
				</button>
				<Button onclick={openAddForm}>Add Repository</Button>
			</div>
		{/snippet}
	</PageHeader>

	<MainlineNotice
		message="Repositories are shared across all branches. Creating, editing or deleting a repository here changes the mainline, and every research branch sees the change."
	/>

	{#if showAddForm}
		<div class="add-form-container">
			<form class="entity-form" onsubmit={(e) => { e.preventDefault(); saveNewRepository(); }}>
				<h2>Add New Repository</h2>

				{#if error}
					<div class="form-error">{error}</div>
				{/if}

				<label>
					Name <span class="required">*</span>
					<input type="text" bind:value={newRepo.name} required />
				</label>

				<fieldset>
					<legend>Address</legend>
					<FormRow>
						<label>
							Address Line 1
							<input type="text" bind:value={newRepo.line1} />
						</label>
						<label>
							Address Line 2
							<input type="text" bind:value={newRepo.line2} />
						</label>
					</FormRow>
					<FormRow>
						<label>
							City
							<input type="text" bind:value={newRepo.city} />
						</label>
						<label>
							State/Province
							<input type="text" bind:value={newRepo.state} />
						</label>
					</FormRow>
					<FormRow>
						<label>
							Postal Code
							<input type="text" bind:value={newRepo.postal_code} />
						</label>
						<label>
							Country
							<input type="text" bind:value={newRepo.country} />
						</label>
					</FormRow>
					<FormRow>
						<label>
							Phone
							<input type="tel" bind:value={newRepo.phone} />
						</label>
						<label>
							Email
							<input type="email" bind:value={newRepo.email} />
						</label>
					</FormRow>
					<FormRow>
						<label>
							Fax
							<input type="tel" bind:value={newRepo.fax} />
						</label>
					</FormRow>
				</fieldset>

				<label>
					Notes
					<textarea bind:value={newRepo.notes} rows="3"></textarea>
				</label>

				<label>
					GEDCOM Xref
					<input type="text" bind:value={newRepo.gedcom_xref} placeholder="e.g., @R1@" />
				</label>

				<div class="form-actions">
					<Button variant="outline" onclick={cancelAdd} disabled={saving}>Cancel</Button>
					<Button type="submit" disabled={saving}>
						{saving ? 'Saving...' : 'Create Repository'}
					</Button>
				</div>
			</form>
		</div>
	{/if}

	{#if loading}
		<div class="loading">Loading...</div>
	{:else if loadError}
		<div class="load-error" role="alert">
			<p>{loadError}</p>
			<Button variant="outline" onclick={loadRepositories}>Retry</Button>
		</div>
	{:else if repositories.length === 0}
		<div class="empty">
			<p>No repositories found.</p>
			<Button onclick={openAddForm}>Add Repository</Button>
		</div>
	{:else}
		<table class="repositories-table">
			<thead>
				<tr>
					<th scope="col">Name</th>
					<th scope="col">Location</th>
				</tr>
			</thead>
			<tbody>
				{#each repositories as repo}
					<tr>
						<td><a href="/repositories/{repo.id}" class="repo-link">{repo.name}</a></td>
						<td class="location">{addressSummary(repo.address) || '—'}</td>
					</tr>
				{/each}
			</tbody>
		</table>

		{#if totalPages > 1}
			<div class="pagination">
				<button onclick={prevPage} disabled={currentPage === 1}>Previous</button>
				<span>Page {currentPage} of {totalPages}</span>
				<button onclick={nextPage} disabled={currentPage >= totalPages}>Next</button>
			</div>
		{/if}
	{/if}
</div>

<style>
	.repositories-page {
		max-width: 1000px;
		margin: 0 auto;
		padding: 1.5rem;
	}

	.controls {
		display: flex;
		align-items: center;
		gap: 0.75rem;
		flex-wrap: wrap;
	}

	.controls label {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		font-size: 0.875rem;
		color: #475569;
	}

	.controls select {
		padding: 0.375rem 0.75rem;
		border: 1px solid #cbd5e1;
		border-radius: 6px;
		background: white;
		font-size: 0.875rem;
	}

	.order-btn {
		display: flex;
		align-items: center;
		justify-content: center;
		width: 2.25rem;
		height: 2.25rem;
		border: 1px solid #cbd5e1;
		border-radius: 6px;
		background: white;
		cursor: pointer;
	}

	.order-btn:hover {
		background: #f1f5f9;
	}

	.order-btn svg {
		width: 1rem;
		height: 1rem;
		color: #64748b;
	}

	.add-form-container {
		margin-bottom: 1.5rem;
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

	.entity-form h2 {
		margin: 0;
		font-size: 1.125rem;
		color: #1e293b;
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
		color: #64748b;
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

	.loading,
	.empty {
		text-align: center;
		padding: 3rem;
		color: #64748b;
	}

	.empty p {
		margin: 0 0 1rem;
	}

	.load-error {
		text-align: center;
		padding: 3rem;
		color: #dc2626;
	}

	.load-error p {
		margin: 0 0 1rem;
	}

	.repositories-table {
		width: 100%;
		border-collapse: collapse;
		background: white;
		border: 1px solid #e2e8f0;
		border-radius: 12px;
		overflow: hidden;
	}

	.repositories-table th {
		text-align: left;
		padding: 0.75rem 1rem;
		font-size: 0.75rem;
		font-weight: 600;
		color: #64748b;
		text-transform: uppercase;
		letter-spacing: 0.05em;
		background: #f8fafc;
		border-bottom: 1px solid #e2e8f0;
	}

	.repositories-table td {
		padding: 0.75rem 1rem;
		font-size: 0.875rem;
		color: #1e293b;
		border-bottom: 1px solid #f1f5f9;
	}

	.repositories-table tr:last-child td {
		border-bottom: none;
	}

	.repo-link {
		color: #3b82f6;
		text-decoration: none;
		font-weight: 500;
	}

	.repo-link:hover {
		text-decoration: underline;
	}

	.location {
		color: #64748b;
	}

	.pagination {
		display: flex;
		justify-content: center;
		align-items: center;
		gap: 1rem;
		margin-top: 2rem;
		padding-top: 1rem;
		border-top: 1px solid #e2e8f0;
	}

	.pagination button {
		padding: 0.5rem 1rem;
		border: 1px solid #cbd5e1;
		border-radius: 6px;
		background: white;
		font-size: 0.875rem;
		cursor: pointer;
	}

	.pagination button:hover:not(:disabled) {
		background: #f1f5f9;
	}

	.pagination button:disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}

	.pagination span {
		font-size: 0.875rem;
		color: #64748b;
	}
</style>
