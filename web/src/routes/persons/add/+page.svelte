<script lang="ts">
	import { goto } from '$app/navigation';
	import { api, type PersonCreate } from '$lib/api/client';

	let saving = $state(false);
	let error: string | null = $state(null);

	// Form state
	let formData = $state<PersonCreate>({
		given_name: '',
		surname: '',
		gender: undefined,
		birth_date: '',
		birth_place: '',
		death_date: '',
		death_place: '',
		notes: ''
	});

	async function createPerson() {
		saving = true;
		error = null;
		try {
			// Build the create payload, only including non-empty optional fields
			const payload: PersonCreate = {
				given_name: formData.given_name,
				surname: formData.surname
			};

			if (formData.gender) {
				payload.gender = formData.gender;
			}
			if (formData.birth_date) {
				payload.birth_date = formData.birth_date;
			}
			if (formData.birth_place) {
				payload.birth_place = formData.birth_place;
			}
			if (formData.death_date) {
				payload.death_date = formData.death_date;
			}
			if (formData.death_place) {
				payload.death_place = formData.death_place;
			}
			if (formData.notes) {
				payload.notes = formData.notes;
			}

			const person = await api.createPerson(payload);
			goto(`/persons/${person.id}`);
		} catch (e) {
			error = (e as { message?: string }).message || 'Failed to create person';
		} finally {
			saving = false;
		}
	}

	function cancel() {
		goto('/persons');
	}
</script>

<svelte:head>
	<title>Add Person | My Family</title>
</svelte:head>

<div class="person-page">
	<header class="page-header">
		<a href="/persons" class="back-link">&larr; People</a>
		<h1>Add Person</h1>
	</header>

	<div class="quick-capture-hint">
		Need to add many people quickly? Try <a href="/persons/quick">Quick Capture mode</a>
	</div>

	{#if error}
		<div class="error">{error}</div>
	{/if}

	<form class="edit-form" onsubmit={(e) => { e.preventDefault(); createPerson(); }}>
		<div class="form-row">
			<label>
				Given Name
				<input type="text" bind:value={formData.given_name} required />
			</label>
			<label>
				Surname
				<input type="text" bind:value={formData.surname} required />
			</label>
		</div>

		<div class="form-row">
			<label>
				Gender
				<select bind:value={formData.gender}>
					<option value={undefined}>Unknown</option>
					<option value="male">Male</option>
					<option value="female">Female</option>
				</select>
			</label>
		</div>

		<div class="form-row">
			<label>
				Birth Date
				<input type="text" bind:value={formData.birth_date} placeholder="e.g., 1 JAN 1850 or ABT 1850" />
			</label>
			<label>
				Birth Place
				<input type="text" bind:value={formData.birth_place} />
			</label>
		</div>

		<div class="form-row">
			<label>
				Death Date
				<input type="text" bind:value={formData.death_date} placeholder="e.g., 15 MAR 1920" />
			</label>
			<label>
				Death Place
				<input type="text" bind:value={formData.death_place} />
			</label>
		</div>

		<label>
			Notes
			<textarea bind:value={formData.notes} rows="4"></textarea>
		</label>

		<div class="form-actions">
			<button type="button" class="btn" onclick={cancel} disabled={saving}>Cancel</button>
			<button type="submit" class="btn btn-primary" disabled={saving}>
				{saving ? 'Creating...' : 'Create Person'}
			</button>
		</div>
	</form>
</div>

<style>
	.person-page {
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

	.btn-primary:disabled {
		background: #93c5fd;
		border-color: #93c5fd;
	}

	.error {
		background: #fef2f2;
		border: 1px solid #fecaca;
		color: #dc2626;
		padding: 0.75rem 1rem;
		border-radius: 6px;
		margin-bottom: 1rem;
		font-size: 0.875rem;
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

	label {
		display: flex;
		flex-direction: column;
		gap: 0.375rem;
		font-size: 0.875rem;
		color: #475569;
	}

	input,
	select,
	textarea {
		padding: 0.625rem 0.75rem;
		border: 1px solid #e2e8f0;
		border-radius: 6px;
		font-size: 0.875rem;
	}

	input:focus,
	select:focus,
	textarea:focus {
		outline: none;
		border-color: #3b82f6;
		box-shadow: 0 0 0 3px rgba(59, 130, 246, 0.1);
	}

	textarea {
		resize: vertical;
	}

	.form-actions {
		display: flex;
		justify-content: flex-end;
		gap: 0.75rem;
		margin-top: 1.5rem;
		padding-top: 1rem;
		border-top: 1px solid #e2e8f0;
	}

	.quick-capture-hint {
		font-size: 0.8125rem;
		color: #64748b;
		margin-bottom: 1rem;
		padding: 0.5rem 0.75rem;
		background: #f8fafc;
		border-radius: 6px;
	}

	.quick-capture-hint a {
		color: #3b82f6;
		text-decoration: none;
	}

	.quick-capture-hint a:hover {
		text-decoration: underline;
	}
</style>
