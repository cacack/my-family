<script lang="ts">
	import { api, type PersonCreate } from '$lib/api/client';

	interface Props {
		onComplete: (data: { personId: string; personName: string }) => void;
		onBack: () => void;
	}

	let { onComplete, onBack }: Props = $props();

	let saving = $state(false);
	let error: string | null = $state(null);

	let formData = $state<PersonCreate>({
		given_name: '',
		surname: '',
		gender: '',
		birth_date: '',
		birth_place: ''
	});

	async function createPerson() {
		saving = true;
		error = null;
		try {
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

			const person = await api.createPerson(payload);
			onComplete({
				personId: person.id,
				personName: `${person.given_name} ${person.surname}`
			});
		} catch (e) {
			error = (e as { message?: string }).message || 'Failed to create person';
		} finally {
			saving = false;
		}
	}
</script>

<div class="create-step">
	<button class="back-btn" onclick={onBack}>&larr; Back</button>

	<h2>Create Your First Person</h2>
	<p class="description">Start building your family tree by adding the first person. You can always edit and add more details later.</p>

	{#if error}
		<div class="error" role="alert">{error}</div>
	{/if}

	<form class="edit-form" onsubmit={(e) => { e.preventDefault(); createPerson(); }}>
		<div class="form-row">
			<label>
				Given Name
				<input type="text" bind:value={formData.given_name} required placeholder="e.g., John" />
			</label>
			<label>
				Surname
				<input type="text" bind:value={formData.surname} required placeholder="e.g., Smith" />
			</label>
		</div>

		<div class="form-row">
			<label>
				Gender
				<select bind:value={formData.gender}>
					<option value="">Unknown</option>
					<option value="male">Male</option>
					<option value="female">Female</option>
				</select>
			</label>
		</div>

		<div class="form-row">
			<label>
				Birth Date <span class="optional">(optional)</span>
				<input type="text" bind:value={formData.birth_date} placeholder="e.g., 1 JAN 1850 or ABT 1850" />
			</label>
			<label>
				Birth Place <span class="optional">(optional)</span>
				<input type="text" bind:value={formData.birth_place} placeholder="e.g., London, England" />
			</label>
		</div>

		<div class="form-actions">
			<button type="submit" class="btn btn-primary" disabled={saving}>
				{saving ? 'Creating...' : 'Create Person'}
			</button>
		</div>
	</form>
</div>

<style>
	.create-step {
		max-width: 540px;
		margin: 0 auto;
	}

	.back-btn {
		background: none;
		border: none;
		padding: 0;
		font-size: 0.875rem;
		color: #64748b;
		cursor: pointer;
		margin-bottom: 1rem;
		font-family: inherit;
	}

	.back-btn:hover {
		color: #3b82f6;
	}

	h2 {
		margin: 0 0 0.5rem;
		font-size: 1.5rem;
		color: #1e293b;
	}

	.description {
		margin: 0 0 1.5rem;
		color: #64748b;
		font-size: 0.875rem;
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

	@media (max-width: 480px) {
		.form-row {
			grid-template-columns: 1fr;
		}
	}

	label {
		display: flex;
		flex-direction: column;
		gap: 0.375rem;
		font-size: 0.875rem;
		color: #475569;
	}

	.optional {
		color: #94a3b8;
		font-weight: 400;
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

	.form-actions {
		display: flex;
		justify-content: flex-end;
		gap: 0.75rem;
		margin-top: 1.5rem;
		padding-top: 1rem;
		border-top: 1px solid #e2e8f0;
	}

	.btn {
		display: inline-flex;
		align-items: center;
		gap: 0.5rem;
		padding: 0.625rem 1.25rem;
		border: 1px solid #cbd5e1;
		border-radius: 6px;
		background: white;
		font-size: 0.875rem;
		font-weight: 500;
		color: #475569;
		cursor: pointer;
		transition: all 0.15s;
		font-family: inherit;
	}

	.btn:hover {
		background: #f1f5f9;
	}

	.btn-primary {
		background: #3b82f6;
		border-color: #3b82f6;
		color: white;
	}

	.btn-primary:hover {
		background: #2563eb;
	}

	.btn:disabled {
		opacity: 0.7;
		cursor: not-allowed;
	}

	/* High contrast mode */
	:global(body.high-contrast) h2 {
		color: var(--color-text);
	}

	:global(body.high-contrast) .description {
		color: var(--color-text-muted);
	}

	:global(body.high-contrast) .edit-form {
		background: var(--color-bg-secondary);
		border-color: var(--color-border);
	}

	:global(body.high-contrast) label {
		color: var(--color-text-muted);
	}

	:global(body.high-contrast) input,
	:global(body.high-contrast) select {
		background: var(--color-bg);
		border-color: var(--color-border);
		color: var(--color-text);
	}

	:global(body.high-contrast) input:focus,
	:global(body.high-contrast) select:focus {
		border-color: var(--color-focus-ring);
		box-shadow: 0 0 0 3px rgba(255, 255, 0, 0.3);
	}

	:global(body.high-contrast) .back-btn {
		color: var(--color-text-muted);
	}

	:global(body.high-contrast) .back-btn:hover {
		color: var(--color-focus-ring);
	}
</style>
