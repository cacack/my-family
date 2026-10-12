<script lang="ts">
	import { goto } from '$app/navigation';
	import { api, type PersonCreate } from '$lib/api/client';
	import { Button } from '$lib/components/ui/button';
	import PageHeader from '$lib/components/PageHeader.svelte';
	import FormRow from '$lib/components/FormRow.svelte';
	import ActionError from '$lib/components/ActionError.svelte';
	import { GENDER_OPTIONS } from '$lib/utils/enumOptions';
	import { asDatePhrase, savedAsNotes, unreadableDateField } from '$lib/utils/dateInput';

	const DATE_FIELDS = ['birth_date', 'death_date'] as const;
	type DateField = (typeof DATE_FIELDS)[number];

	let saving = $state(false);
	let error: string | null = $state(null);
	// The date field a create was refused for because its text isn't a date,
	// which the error offers to keep as text (#996).
	let unreadableDate: DateField | null = $state(null);

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
		unreadableDate = null;
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
			// The new person's page says which dates were stored in another form (#996).
			const savedAs = savedAsNotes(
				{ birth_date: formData.birth_date ?? '', death_date: formData.death_date ?? '' },
				{ birth_date: person.birth_date?.raw ?? '', death_date: person.death_date?.raw ?? '' }
			);
			goto(`/persons/${person.id}`, { state: { savedAs } });
		} catch (e) {
			error = (e as { message?: string }).message || 'Failed to create person';
			unreadableDate = unreadableDateField(e, DATE_FIELDS);
		} finally {
			saving = false;
		}
	}

	/** Create with an unreadable date as a date phrase, so it is kept as text (#996). */
	function keepDateAsText(field: DateField) {
		formData[field] = asDatePhrase(formData[field] ?? '');
		createPerson();
	}

	function cancel() {
		goto('/persons');
	}
</script>

<svelte:head>
	<title>Add Person | My Family</title>
</svelte:head>

<div class="person-page">
	<PageHeader title="Add Person" back={{ href: '/persons', label: 'People' }} />

	<div class="quick-capture-hint">
		Need to add many people quickly? Try <a href="/persons/quick">Quick Capture mode</a>
	</div>

	{#if error}
		{@const dateField = unreadableDate}
		<ActionError
			message={error}
			action={dateField
				? { label: 'Keep as text', onclick: () => keepDateAsText(dateField), disabled: saving }
				: undefined}
		/>
	{/if}

	<form class="edit-form" onsubmit={(e) => { e.preventDefault(); createPerson(); }}>
		<FormRow>
			<label>
				Given Name
				<input type="text" bind:value={formData.given_name} required />
			</label>
			<label>
				Surname
				<input type="text" bind:value={formData.surname} />
			</label>
		</FormRow>

		<FormRow>
			<label>
				Gender
				<select bind:value={formData.gender}>
					<option value={undefined}>Unknown</option>
					{#each GENDER_OPTIONS.filter((o) => o.value !== 'unknown') as option (option.value)}
						<option value={option.value}>{option.label}</option>
					{/each}
				</select>
			</label>
		</FormRow>

		<FormRow>
			<label>
				Birth Date
				<input type="text" bind:value={formData.birth_date} placeholder="e.g., 1 JAN 1850 or about 1850" />
			</label>
			<label>
				Birth Place
				<input type="text" bind:value={formData.birth_place} />
			</label>
		</FormRow>

		<FormRow>
			<label>
				Death Date
				<input type="text" bind:value={formData.death_date} placeholder="e.g., 15 MAR 1920 or before 1920" />
			</label>
			<label>
				Death Place
				<input type="text" bind:value={formData.death_place} />
			</label>
		</FormRow>

		<label>
			Notes
			<textarea bind:value={formData.notes} rows="4"></textarea>
		</label>

		<div class="form-actions">
			<Button variant="outline" onclick={cancel} disabled={saving}>Cancel</Button>
			<Button type="submit" disabled={saving}>
				{saving ? 'Creating...' : 'Create Person'}
			</Button>
		</div>
	</form>
</div>

<style>
	.person-page {
		max-width: 800px;
		margin: 0 auto;
		padding: 1.5rem;
	}

	/* Edit form styles */
	.edit-form {
		background: white;
		border-radius: 12px;
		border: 1px solid #e2e8f0;
		padding: 1.5rem;
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
		color: var(--color-text-muted);
		margin-bottom: 1rem;
		padding: 0.5rem 0.75rem;
		background: #f8fafc;
		border-radius: 6px;
	}

	.quick-capture-hint a {
		color: var(--color-link);
		text-decoration: underline;
	}
</style>
