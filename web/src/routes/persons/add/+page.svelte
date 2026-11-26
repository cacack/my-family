<script lang="ts">
	import { can } from '#lib/stores/access.svelte.js';
	import RoleNotice from '#lib/components/auth/RoleNotice.svelte';
	import { goto } from '$app/navigation';
	import { api, missingNameError, type PersonCreate } from '#lib/api/client.js';
	import { Button } from '#lib/components/ui/button/index.js';
	import PageHeader from '#lib/components/PageHeader.svelte';
	import FormRow from '#lib/components/FormRow.svelte';
	import ActionError from '#lib/components/ActionError.svelte';
	import CoordinateFields from '#lib/components/CoordinateFields.svelte';
	import { GENDER_OPTIONS } from '#lib/utils/enumOptions.js';
	import {
		asDatePhrase,
		dateErrorAttrs,
		dateErrorId,
		dateFieldError,
		focusFirstInvalid,
		savedAsNotes,
		type DateFieldError
	} from '#lib/utils/dateInput.js';
	import { coordinateChange, coordinatePairError, type CoordinateError, type CoordinateInput } from '#lib/utils/coordinates.js';
	import { pickedOrigin, type CoordinatePick } from '#lib/utils/geocode.js';

	const DATE_FIELDS = ['birth_date', 'death_date'] as const;
	type DateField = (typeof DATE_FIELDS)[number];
	const PLACE_FIELDS = ['birth_place', 'death_place'] as const;
	type PlaceField = (typeof PLACE_FIELDS)[number];

	let saving = $state(false);
	let error: string | null = $state(null);
	// The date field a create was refused for, shown beside it (#1112); an
	// unreadable one is offered to keep as text (#996).
	let dateError: DateFieldError<DateField> | null = $state(null);
	let form: HTMLFormElement | null = $state(null);
	// Each place's pair in signed decimal degrees, and the input a create was
	// stopped for (#1033).
	let coordinates: Record<PlaceField, CoordinateInput> = $state({
		birth_place: { latitude: '', longitude: '' },
		death_place: { latitude: '', longitude: '' }
	});
	let coordinateErrors: Partial<Record<PlaceField, CoordinateError>> = $state({});
	// The geocoder candidate picked into each place's inputs (#1607).
	let picks: Record<PlaceField, CoordinatePick | null> = $state({ birth_place: null, death_place: null });

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
		dateError = null;
		error = missingNameError(formData);
		if (error) return;
		coordinateErrors = {};
		for (const field of PLACE_FIELDS) {
			const coordinateError = coordinatePairError(coordinates[field], formData[field] ?? '');
			if (coordinateError) coordinateErrors[field] = coordinateError;
		}
		if (Object.keys(coordinateErrors).length) {
			focusFirstInvalid(form);
			return;
		}
		saving = true;
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
			// A new record has no stored pair, so a change is always a set pair.
			const birth = coordinateChange(coordinates.birth_place);
			if (birth?.latitude && birth.longitude) {
				payload.birth_place_latitude = birth.latitude;
				payload.birth_place_longitude = birth.longitude;
				payload.birth_place_coordinates_origin = pickedOrigin(coordinates.birth_place, picks.birth_place, formData.birth_place ?? '');
			}
			const death = coordinateChange(coordinates.death_place);
			if (death?.latitude && death.longitude) {
				payload.death_place_latitude = death.latitude;
				payload.death_place_longitude = death.longitude;
				payload.death_place_coordinates_origin = pickedOrigin(coordinates.death_place, picks.death_place, formData.death_place ?? '');
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
			dateError = dateFieldError(e, DATE_FIELDS);
			if (dateError) focusFirstInvalid(form);
			else error = (e as { message?: string }).message || 'Failed to create person';
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

	{#if !can('editor')}
		<RoleNotice action="Adding people" required="editor" />
	{:else}

	<div class="quick-capture-hint">
		Need to add many people quickly? Try <a href="/persons/quick">Quick Capture mode</a>
	</div>

	{#if error}
		<ActionError message={error} />
	{/if}

	{#snippet dateMessage(field: DateField)}
		{#if dateError?.field === field}
			<ActionError
				id={dateErrorId(field)}
				message={dateError.message}
				action={dateError.unreadable
					? { label: 'Keep as text', onclick: () => keepDateAsText(field), disabled: saving }
					: undefined}
			/>
		{/if}
	{/snippet}

	<form class="edit-form" bind:this={form} onsubmit={(e) => { e.preventDefault(); createPerson(); }}>
		<FormRow>
			<label>
				Given Name
				<input type="text" bind:value={formData.given_name} />
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
			<div class="date-field">
				<label>
					Birth Date
					<input type="text" bind:value={formData.birth_date} {...dateErrorAttrs(dateError, 'birth_date')} placeholder="e.g., 1 JAN 1850 or about 1850" />
				</label>
				{@render dateMessage('birth_date')}
			</div>
			<div class="place-field">
				<label>
					Birth Place
					<input type="text" bind:value={formData.birth_place} />
				</label>
				<CoordinateFields
					id="birth-place"
					place="Birth Place"
					bind:latitude={coordinates.birth_place.latitude}
					bind:longitude={coordinates.birth_place.longitude}
					error={coordinateErrors.birth_place}
					lookup={formData.birth_place ?? ''}
					bind:picked={picks.birth_place}
				/>
			</div>
		</FormRow>

		<FormRow>
			<div class="date-field">
				<label>
					Death Date
					<input type="text" bind:value={formData.death_date} {...dateErrorAttrs(dateError, 'death_date')} placeholder="e.g., 15 MAR 1920 or before 1920" />
				</label>
				{@render dateMessage('death_date')}
			</div>
			<div class="place-field">
				<label>
					Death Place
					<input type="text" bind:value={formData.death_place} />
				</label>
				<CoordinateFields
					id="death-place"
					place="Death Place"
					bind:latitude={coordinates.death_place.latitude}
					bind:longitude={coordinates.death_place.longitude}
					error={coordinateErrors.death_place}
					lookup={formData.death_place ?? ''}
					bind:picked={picks.death_place}
				/>
			</div>
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
	{/if}
</div>

<style>
	.person-page {
		max-width: var(--page-width-detail);
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

	/* A date input's label with any refusal of it underneath (#1112), and a
	   place input's with its coordinates (#1033). */
	.date-field,
	.place-field {
		display: flex;
		flex-direction: column;
		gap: 0.375rem;
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
