<script lang="ts">
	import { goto } from '$app/navigation';
	import { api, type FamilyCreate } from '$lib/api/client';

	let saving = $state(false);
	let error: string | null = $state(null);

	// Form state
	let formData = $state<FamilyCreate>({
		relationship_type: 'unknown',
		marriage_date: '',
		marriage_place: ''
	});

	async function handleSubmit() {
		saving = true;
		error = null;
		try {
			const payload: FamilyCreate = {
				relationship_type: formData.relationship_type || undefined,
				marriage_date: formData.marriage_date || undefined,
				marriage_place: formData.marriage_place || undefined
			};
			const family = await api.createFamily(payload);
			goto(`/families/${family.id}`);
		} catch (e) {
			error = (e as { message?: string }).message || 'Failed to create family';
		} finally {
			saving = false;
		}
	}

	function handleCancel() {
		goto('/families');
	}
</script>

<svelte:head>
	<title>Add Family | My Family</title>
</svelte:head>

<div class="add-family-page">
	<header class="page-header">
		<a href="/families" class="back-link">&larr; Families</a>
		<h1>Add Family</h1>
	</header>

	{#if error}
		<div class="error">{error}</div>
	{/if}

	<form class="edit-form" onsubmit={(e) => { e.preventDefault(); handleSubmit(); }}>
		<div class="form-row">
			<label>
				Relationship Type
				<select bind:value={formData.relationship_type}>
					<option value="unknown">Unknown</option>
					<option value="marriage">Marriage</option>
					<option value="partnership">Partnership</option>
				</select>
			</label>
		</div>

		<div class="form-row">
			<label>
				Marriage Date
				<input type="text" bind:value={formData.marriage_date} placeholder="e.g., 1 JAN 1850 or ABT 1850" />
			</label>
			<label>
				Marriage Place
				<input type="text" bind:value={formData.marriage_place} placeholder="e.g., London, England" />
			</label>
		</div>

		<p class="helper-text">
			Partners can be added after creating the family by editing the family record.
		</p>

		<div class="form-actions">
			<button type="button" class="btn" onclick={handleCancel} disabled={saving}>Cancel</button>
			<button type="submit" class="btn btn-primary" disabled={saving}>
				{saving ? 'Creating...' : 'Create Family'}
			</button>
		</div>
	</form>
</div>

<style>
	.add-family-page {
		max-width: 800px;
		margin: 0 auto;
		padding: 1.5rem;
	}

	.page-header {
		display: flex;
		align-items: center;
		gap: 1rem;
		margin-bottom: 1.5rem;
	}

	.page-header h1 {
		margin: 0;
		font-size: 1.5rem;
		color: #1e293b;
	}

	.back-link {
		color: #64748b;
		text-decoration: none;
		font-size: 0.875rem;
	}

	.back-link:hover {
		color: #3b82f6;
	}

	.error {
		text-align: center;
		padding: 1rem;
		color: #dc2626;
		background: #fef2f2;
		border: 1px solid #fecaca;
		border-radius: 6px;
		margin-bottom: 1rem;
	}

	.btn {
		padding: 0.5rem 1rem;
		border: 1px solid #cbd5e1;
		border-radius: 6px;
		background: white;
		font-size: 0.875rem;
		cursor: pointer;
		text-decoration: none;
		color: #475569;
	}

	.btn:hover {
		background: #f1f5f9;
	}

	.btn:disabled {
		opacity: 0.6;
		cursor: not-allowed;
	}

	.btn-primary {
		background: #3b82f6;
		border-color: #3b82f6;
		color: white;
	}

	.btn-primary:hover {
		background: #2563eb;
	}

	/* Edit form styles */
	.edit-form {
		background: white;
		border-radius: 12px;
		border: 1px solid #e2e8f0;
		padding: 1.5rem;
	}

	.form-row {
		display: grid;
		grid-template-columns: repeat(2, 1fr);
		gap: 1rem;
		margin-bottom: 1rem;
	}

	.form-row:has(> :only-child) {
		grid-template-columns: 1fr;
	}

	label {
		display: flex;
		flex-direction: column;
		gap: 0.375rem;
		font-size: 0.875rem;
		color: #475569;
	}

	input,
	select {
		padding: 0.625rem 0.75rem;
		border: 1px solid #e2e8f0;
		border-radius: 6px;
		font-size: 0.875rem;
	}

	input:focus,
	select:focus {
		outline: none;
		border-color: #3b82f6;
		box-shadow: 0 0 0 3px rgba(59, 130, 246, 0.1);
	}

	.helper-text {
		font-size: 0.8125rem;
		color: #64748b;
		margin: 0.5rem 0 0;
	}

	.form-actions {
		display: flex;
		justify-content: flex-end;
		gap: 0.75rem;
		margin-top: 1.5rem;
		padding-top: 1rem;
		border-top: 1px solid #e2e8f0;
	}
</style>
