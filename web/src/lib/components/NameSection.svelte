<script lang="ts">
	import { api, isConflictError, type PersonName, type NameType } from '$lib/api/client';
	import ConflictError from './ConflictError.svelte';

	interface Props {
		personId: string;
	}

	let { personId }: Props = $props();

	let names: PersonName[] = $state([]);
	let loading = $state(true);
	let error: string | null = $state(null);

	// Add name form state
	let showAddForm = $state(false);
	let saving = $state(false);
	let deleteConfirm: string | null = $state(null);
	let editingId: string | null = $state(null);

	// Conflict retry state
	let conflictError = $state(false);
	let retryAction: (() => Promise<void>) | null = $state(null);
	let retrying = $state(false);

	// Form state for add/edit
	let formData = $state({
		given_name: '',
		surname: '',
		name_prefix: '',
		name_suffix: '',
		surname_prefix: '',
		nickname: '',
		name_type: 'birth' as NameType,
		is_primary: false
	});

	async function loadNames() {
		loading = true;
		error = null;
		try {
			const result = await api.getPersonNames(personId);
			names = result.items;
		} catch (e) {
			error = (e as { message?: string }).message || 'Failed to load names';
		} finally {
			loading = false;
		}
	}

	function resetForm() {
		formData = {
			given_name: '',
			surname: '',
			name_prefix: '',
			name_suffix: '',
			surname_prefix: '',
			nickname: '',
			name_type: 'birth',
			is_primary: false
		};
	}

	async function handleRetry() {
		if (!retryAction) return;
		retrying = true;
		conflictError = false;
		error = null;
		try {
			await retryAction();
		} catch (e) {
			if (isConflictError(e)) {
				conflictError = true;
			} else {
				error = (e as { message?: string }).message || 'Operation failed';
			}
		} finally {
			retrying = false;
		}
	}

	function openAddForm() {
		resetForm();
		editingId = null;
		showAddForm = true;
		error = null;
	}

	function cancelAdd() {
		showAddForm = false;
		error = null;
	}

	function startEdit(name: PersonName) {
		editingId = name.id;
		formData = {
			given_name: name.given_name,
			surname: name.surname,
			name_prefix: name.name_prefix || '',
			name_suffix: name.name_suffix || '',
			surname_prefix: name.surname_prefix || '',
			nickname: name.nickname || '',
			name_type: name.name_type,
			is_primary: name.is_primary
		};
		showAddForm = false;
		error = null;
	}

	function cancelEdit() {
		editingId = null;
		error = null;
	}

	async function saveNewName() {
		if (!formData.given_name.trim() || !formData.surname.trim()) {
			error = 'Given name and surname are required';
			return;
		}

		saving = true;
		error = null;
		try {
			await api.addPersonName(personId, {
				given_name: formData.given_name.trim(),
				surname: formData.surname.trim(),
				name_prefix: formData.name_prefix.trim() || undefined,
				name_suffix: formData.name_suffix.trim() || undefined,
				surname_prefix: formData.surname_prefix.trim() || undefined,
				nickname: formData.nickname.trim() || undefined,
				name_type: formData.name_type,
				is_primary: formData.is_primary
			});
			showAddForm = false;
			conflictError = false;
			loadNames();
		} catch (e) {
			if (isConflictError(e)) {
				conflictError = true;
				retryAction = () => saveNewName();
			} else {
				error = (e as { message?: string }).message || 'Failed to add name';
			}
		} finally {
			saving = false;
		}
	}

	async function saveEdit() {
		if (!editingId) return;
		if (!formData.given_name.trim() || !formData.surname.trim()) {
			error = 'Given name and surname are required';
			return;
		}

		saving = true;
		error = null;
		try {
			await api.updatePersonName(personId, editingId, {
				given_name: formData.given_name.trim(),
				surname: formData.surname.trim(),
				name_prefix: formData.name_prefix.trim(),
				name_suffix: formData.name_suffix.trim(),
				surname_prefix: formData.surname_prefix.trim(),
				nickname: formData.nickname.trim(),
				name_type: formData.name_type,
				is_primary: formData.is_primary
			});
			editingId = null;
			conflictError = false;
			loadNames();
		} catch (e) {
			if (isConflictError(e)) {
				conflictError = true;
				retryAction = () => saveEdit();
			} else {
				error = (e as { message?: string }).message || 'Failed to update name';
			}
		} finally {
			saving = false;
		}
	}

	async function deleteName(name: PersonName) {
		saving = true;
		error = null;
		try {
			await api.deletePersonName(personId, name.id);
			deleteConfirm = null;
			conflictError = false;
			loadNames();
		} catch (e) {
			if (isConflictError(e)) {
				conflictError = true;
				retryAction = () => deleteName(name);
			} else {
				error = (e as { message?: string }).message || 'Failed to delete name';
			}
		} finally {
			saving = false;
		}
	}

	function formatDisplayName(name: PersonName): string {
		const parts: string[] = [];
		if (name.name_prefix) parts.push(name.name_prefix);
		parts.push(name.given_name);
		if (name.nickname) parts.push(`"${name.nickname}"`);
		if (name.surname_prefix) parts.push(name.surname_prefix);
		parts.push(name.surname);
		if (name.name_suffix) parts.push(name.name_suffix);
		return parts.join(' ');
	}

	function nameTypeBadgeInfo(type: NameType): { label: string; bg: string; color: string } {
		switch (type) {
			case 'birth':
				return { label: 'Birth', bg: '#dbeafe', color: '#3b82f6' };
			case 'married':
				return { label: 'Married', bg: '#fce7f3', color: '#ec4899' };
			case 'aka':
				return { label: 'AKA', bg: '#f3e8ff', color: '#a855f7' };
			case 'immigrant':
				return { label: 'Immigrant', bg: '#dcfce7', color: '#16a34a' };
			case 'religious':
				return { label: 'Religious', bg: '#fef9c3', color: '#d97706' };
			case 'professional':
				return { label: 'Professional', bg: '#f1f5f9', color: '#64748b' };
			default:
				return { label: type, bg: '#f1f5f9', color: '#64748b' };
		}
	}

	$effect(() => {
		if (personId) {
			showAddForm = false;
			editingId = null;
			deleteConfirm = null;
			resetForm();
			loadNames();
		}
	});
</script>

<div class="name-section">
	<div class="section-header">
		<h2>Names <span class="count-badge">{names.length}</span></h2>
		{#if !showAddForm}
			<button class="btn btn-small" onclick={openAddForm}>Add Name</button>
		{/if}
	</div>

	{#if conflictError}
		<ConflictError onRetry={handleRetry} {retrying} />
	{:else if error}
		<div class="section-error" role="alert">{error}</div>
	{/if}

	{#if showAddForm}
		<form class="add-form" onsubmit={(e) => { e.preventDefault(); saveNewName(); }}>
			<div class="form-row">
				<label>
					Given Name <span class="required">*</span>
					<input type="text" bind:value={formData.given_name} placeholder="e.g., John" required />
				</label>
				<label>
					Surname <span class="required">*</span>
					<input type="text" bind:value={formData.surname} placeholder="e.g., Smith" required />
				</label>
			</div>

			<div class="form-row">
				<label>
					Name Prefix
					<input type="text" bind:value={formData.name_prefix} placeholder="e.g., Dr." />
				</label>
				<label>
					Name Suffix
					<input type="text" bind:value={formData.name_suffix} placeholder="e.g., Jr." />
				</label>
			</div>

			<div class="form-row">
				<label>
					Surname Prefix
					<input type="text" bind:value={formData.surname_prefix} placeholder="e.g., von" />
				</label>
				<label>
					Nickname
					<input type="text" bind:value={formData.nickname} placeholder="e.g., Bill" />
				</label>
			</div>

			<div class="form-row">
				<label>
					Name Type <span class="required">*</span>
					<select bind:value={formData.name_type} required>
						<option value="birth">Birth</option>
						<option value="married">Married</option>
						<option value="aka">AKA</option>
						<option value="immigrant">Immigrant</option>
						<option value="religious">Religious</option>
						<option value="professional">Professional</option>
					</select>
				</label>
			</div>

			<div class="checkbox-row">
				<label class="checkbox-label">
					<input type="checkbox" bind:checked={formData.is_primary} />
					Primary name
				</label>
				<span class="checkbox-note">Setting as primary will demote the current primary name</span>
			</div>

			<div class="form-actions">
				<button type="button" class="btn" onclick={cancelAdd} disabled={saving}>Cancel</button>
				<button type="submit" class="btn btn-primary" disabled={saving}>
					{saving ? 'Saving...' : 'Add Name'}
				</button>
			</div>
		</form>
	{/if}

	{#if loading}
		<div class="loading-state" role="status" aria-live="polite">Loading names...</div>
	{:else if names.length === 0 && !showAddForm}
		<div class="empty-state">
			<p>No name variants yet.</p>
			<button class="btn btn-small" onclick={openAddForm}>Add the first name</button>
		</div>
	{:else if names.length > 0}
		<ul class="name-list">
			{#each names as name}
				<li class="name-item">
					{#if editingId === name.id}
						<form class="add-form edit-form" onsubmit={(e) => { e.preventDefault(); saveEdit(); }}>
							<div class="form-row">
								<label>
									Given Name <span class="required">*</span>
									<input type="text" bind:value={formData.given_name} required />
								</label>
								<label>
									Surname <span class="required">*</span>
									<input type="text" bind:value={formData.surname} required />
								</label>
							</div>

							<div class="form-row">
								<label>
									Name Prefix
									<input type="text" bind:value={formData.name_prefix} />
								</label>
								<label>
									Name Suffix
									<input type="text" bind:value={formData.name_suffix} />
								</label>
							</div>

							<div class="form-row">
								<label>
									Surname Prefix
									<input type="text" bind:value={formData.surname_prefix} />
								</label>
								<label>
									Nickname
									<input type="text" bind:value={formData.nickname} />
								</label>
							</div>

							<div class="form-row">
								<label>
									Name Type <span class="required">*</span>
									<select bind:value={formData.name_type} required>
										<option value="birth">Birth</option>
										<option value="married">Married</option>
										<option value="aka">AKA</option>
										<option value="immigrant">Immigrant</option>
										<option value="religious">Religious</option>
										<option value="professional">Professional</option>
									</select>
								</label>
							</div>

							<div class="checkbox-row">
								<label class="checkbox-label">
									<input type="checkbox" bind:checked={formData.is_primary} />
									Primary name
								</label>
								<span class="checkbox-note">Setting as primary will demote the current primary name</span>
							</div>

							<div class="form-actions">
								<button type="button" class="btn" onclick={cancelEdit} disabled={saving}>Cancel</button>
								<button type="submit" class="btn btn-primary" disabled={saving}>
									{saving ? 'Saving...' : 'Save'}
								</button>
							</div>
						</form>
					{:else}
						{@const badgeInfo = nameTypeBadgeInfo(name.name_type)}
						<div class="name-header">
							<span class="name-display">{formatDisplayName(name)}</span>
							<span
								class="name-type-badge"
								style="background: {badgeInfo.bg}; color: {badgeInfo.color};"
							>
								{badgeInfo.label}
							</span>
							{#if name.is_primary}
								<span class="primary-badge">Primary</span>
							{/if}
						</div>

						<div class="name-actions">
							{#if deleteConfirm === name.id}
								<span class="delete-confirm">Delete this name?</span>
								<button class="btn btn-small btn-danger" onclick={() => deleteName(name)} disabled={saving}>Yes, Delete</button>
								<button class="btn btn-small" onclick={() => deleteConfirm = null}>Cancel</button>
							{:else}
								<button class="btn btn-small btn-text" onclick={() => startEdit(name)}>Edit</button>
								<button class="btn btn-small btn-text" onclick={() => deleteConfirm = name.id}>Delete</button>
							{/if}
						</div>
					{/if}
				</li>
			{/each}
		</ul>
	{/if}
</div>

<style>
	.name-section {
		margin-top: 1.5rem;
	}

	.section-header {
		display: flex;
		justify-content: space-between;
		align-items: center;
		margin-bottom: 1rem;
	}

	.section-header h2 {
		margin: 0;
		font-size: 0.875rem;
		font-weight: 600;
		color: #64748b;
		text-transform: uppercase;
		letter-spacing: 0.05em;
		display: flex;
		align-items: center;
		gap: 0.5rem;
	}

	.count-badge {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		min-width: 1.25rem;
		height: 1.25rem;
		padding: 0 0.375rem;
		background: #dbeafe;
		border-radius: 9999px;
		font-size: 0.6875rem;
		font-weight: 600;
		color: #3b82f6;
		text-transform: none;
		letter-spacing: 0;
	}

	.btn {
		padding: 0.5rem 1rem;
		border: 1px solid #cbd5e1;
		border-radius: 6px;
		background: white;
		font-size: 0.875rem;
		cursor: pointer;
		color: #475569;
	}

	.btn:hover {
		background: #f1f5f9;
	}

	.btn-small {
		padding: 0.375rem 0.75rem;
		font-size: 0.8125rem;
	}

	.btn-primary {
		background: #3b82f6;
		border-color: #3b82f6;
		color: white;
	}

	.btn-primary:hover {
		background: #2563eb;
	}

	.btn-danger {
		color: #dc2626;
		border-color: #fecaca;
	}

	.btn-danger:hover {
		background: #fef2f2;
	}

	.btn-text {
		background: transparent;
		border-color: transparent;
		color: #64748b;
	}

	.btn-text:hover {
		color: #dc2626;
		background: transparent;
	}

	.section-error {
		padding: 0.75rem;
		background: #fef2f2;
		border: 1px solid #fecaca;
		border-radius: 6px;
		color: #dc2626;
		font-size: 0.875rem;
		margin-bottom: 1rem;
	}

	.add-form {
		background: #f8fafc;
		border: 1px solid #e2e8f0;
		border-radius: 8px;
		padding: 1rem;
		margin-bottom: 1rem;
	}

	.edit-form {
		margin-bottom: 0;
	}

	.form-row {
		display: grid;
		grid-template-columns: repeat(2, 1fr);
		gap: 1rem;
		margin-bottom: 1rem;
	}

	.add-form label {
		display: flex;
		flex-direction: column;
		gap: 0.375rem;
		font-size: 0.875rem;
		color: #475569;
	}

	.required {
		color: #dc2626;
	}

	.add-form input[type="text"],
	.add-form select {
		padding: 0.5rem 0.75rem;
		border: 1px solid #e2e8f0;
		border-radius: 6px;
		font-size: 0.875rem;
		background: white;
	}

	.add-form input[type="text"]:focus,
	.add-form select:focus {
		outline: none;
		border-color: #3b82f6;
		box-shadow: 0 0 0 3px rgba(59, 130, 246, 0.1);
	}

	.checkbox-row {
		display: flex;
		align-items: center;
		gap: 0.75rem;
		margin-bottom: 1rem;
	}

	.checkbox-label {
		display: flex;
		align-items: center;
		gap: 0.375rem;
		font-size: 0.875rem;
		color: #475569;
		cursor: pointer;
	}

	.checkbox-note {
		font-size: 0.75rem;
		color: #94a3b8;
	}

	.form-actions {
		display: flex;
		justify-content: flex-end;
		gap: 0.75rem;
		margin-top: 1rem;
		padding-top: 1rem;
		border-top: 1px solid #e2e8f0;
	}

	.loading-state,
	.empty-state {
		text-align: center;
		padding: 2rem;
		color: #64748b;
		font-size: 0.875rem;
	}

	.empty-state p {
		margin: 0 0 1rem;
	}

	.name-list {
		list-style: none;
		padding: 0;
		margin: 0;
	}

	.name-item {
		padding: 1rem;
		border: 1px solid #e2e8f0;
		border-radius: 8px;
		margin-bottom: 0.75rem;
	}

	.name-item:last-child {
		margin-bottom: 0;
	}

	.name-header {
		display: flex;
		align-items: center;
		gap: 0.75rem;
		flex-wrap: wrap;
	}

	.name-display {
		font-size: 0.9375rem;
		font-weight: 500;
		color: #1e293b;
	}

	.name-type-badge {
		display: inline-block;
		padding: 0.125rem 0.5rem;
		border-radius: 4px;
		font-size: 0.6875rem;
		text-transform: uppercase;
		font-weight: 500;
	}

	.primary-badge {
		display: inline-block;
		padding: 0.125rem 0.5rem;
		background: #dcfce7;
		color: #166534;
		border-radius: 4px;
		font-size: 0.6875rem;
		text-transform: uppercase;
		font-weight: 500;
	}

	.name-actions {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		margin-top: 0.75rem;
		padding-top: 0.75rem;
		border-top: 1px solid #f1f5f9;
	}

	.delete-confirm {
		font-size: 0.8125rem;
		color: #dc2626;
	}

	@media (max-width: 640px) {
		.form-row {
			grid-template-columns: 1fr;
		}
	}
</style>
