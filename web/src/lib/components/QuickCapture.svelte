<script lang="ts">
	import { api, type PersonCreate, type Gender } from '$lib/api/client';
	import { Button } from '$lib/components/ui/button';
	import { goto } from '$app/navigation';
	import { parseName } from '$lib/utils/nameParse';
	import ActionError from '$lib/components/ActionError.svelte';
	import { asDatePhrase, savedAsNotes, unreadableDateField, type SavedAs } from '$lib/utils/dateInput';

	let nameInput = $state('');
	let gender = $state<Gender>('unknown');
	let birthYear = $state('');
	let notes = $state('');
	let showNotes = $state(false);
	let sessionCount = $state(0);
	let error = $state<string | null>(null);
	let saving = $state(false);
	// True when an add was refused because the birth year isn't a date, which
	// the error offers to keep as text (#996).
	let unreadableDate = $state(false);
	// The last added person's birth year, when it was stored in another form
	// than typed (#996).
	let savedAs: SavedAs | undefined = $state();
	// How the last add was submitted, so keeping its date as text repeats it.
	let lastMode: 'next' | 'view' = 'next';

	let parsed = $derived(parseName(nameInput));

	async function submit(mode: 'next' | 'view') {
		if (!parsed.givenName) {
			error = 'Please enter at least a given name.';
			unreadableDate = false;
			return;
		}

		lastMode = mode;
		saving = true;
		error = null;
		unreadableDate = false;
		savedAs = undefined;

		try {
			const payload: PersonCreate = {
				given_name: parsed.givenName,
				surname: parsed.surname,
				gender: gender,
				research_status: 'possible'
			};

			if (notes) {
				payload.notes = `[Quick capture] ${notes}`;
			} else {
				payload.notes = '[Quick capture]';
			}

			if (birthYear) {
				payload.birth_date = birthYear;
			}

			const person = await api.createPerson(payload);
			sessionCount++;
			const dateNotes = savedAsNotes({ birth_date: birthYear }, { birth_date: person.birth_date?.raw ?? '' });

			if (mode === 'view') {
				goto(`/persons/${person.id}`, { state: { savedAs: dateNotes } });
			} else {
				savedAs = dateNotes.birth_date;
				// Reset form for next entry
				nameInput = '';
				gender = 'unknown';
				birthYear = '';
				notes = '';
				showNotes = false;
			}
		} catch (e) {
			error = (e as { message?: string }).message || 'Failed to create person';
			unreadableDate = unreadableDateField(e, ['birth_date']) !== null;
		} finally {
			saving = false;
		}
	}

	/** Add with an unreadable birth year as a date phrase, so it is kept as text (#996). */
	function keepDateAsText() {
		birthYear = asDatePhrase(birthYear);
		submit(lastMode);
	}
</script>

<div class="quick-capture">
	{#if sessionCount > 0}
		<div class="session-counter">
			{sessionCount} {sessionCount === 1 ? 'person' : 'people'} added this session
		</div>
	{/if}

	{#if savedAs}
		<p class="saved-as" role="status">Birth year saved as {savedAs.stored} from “{savedAs.typed}”</p>
	{/if}

	{#if error}
		<ActionError
			message={error}
			action={unreadableDate
				? { label: 'Keep as text', onclick: keepDateAsText, disabled: saving }
				: undefined}
		/>
	{/if}

	<form onsubmit={(e) => { e.preventDefault(); submit('next'); }}>
		<label class="field">
			Full Name
			<input
				type="text"
				bind:value={nameInput}
				placeholder="e.g., John Smith or Madonna"
				autocomplete="off"
				required
			/>
		</label>

		{#if nameInput.trim()}
			<div class="name-preview">
				<span class="preview-label">Given:</span> {parsed.givenName}
				{#if parsed.surname}
					<span class="preview-sep">|</span>
					<span class="preview-label">Surname:</span> {parsed.surname}
				{/if}
			</div>
		{/if}

		<fieldset class="gender-group">
			<legend>Gender</legend>
			<div class="gender-buttons">
				<Button
					type="button"
					variant={gender === 'male' ? 'default' : 'outline'}
					size="sm"
					class="min-h-12 flex-1"
					onclick={() => (gender = 'male')}
				>
					Male
				</Button>
				<Button
					type="button"
					variant={gender === 'female' ? 'default' : 'outline'}
					size="sm"
					class="min-h-12 flex-1"
					onclick={() => (gender = 'female')}
				>
					Female
				</Button>
				<Button
					type="button"
					variant={gender === 'unknown' ? 'default' : 'outline'}
					size="sm"
					class="min-h-12 flex-1"
					onclick={() => (gender = 'unknown')}
				>
					Unknown
				</Button>
			</div>
		</fieldset>

		<label class="field">
			Birth Year <span class="optional">(optional)</span>
			<input
				type="text"
				bind:value={birthYear}
				placeholder="e.g., 1850"
				inputmode="numeric"
			/>
		</label>

		{#if showNotes}
			<label class="field">
				Notes <span class="optional">(optional)</span>
				<textarea bind:value={notes} rows="2" placeholder="Any quick notes..."></textarea>
			</label>
		{:else}
			<button type="button" class="toggle-notes" onclick={() => (showNotes = true)}>
				+ Add notes
			</button>
		{/if}

		<div class="form-actions">
			<Button type="submit" disabled={saving} class="flex-1">
				{saving ? 'Adding...' : 'Add & Next'}
			</Button>
			<Button variant="secondary" disabled={saving} onclick={() => submit('view')}>
				Add & View
			</Button>
		</div>
	</form>
</div>

<style>
	.quick-capture {
		max-width: 480px;
		margin: 0 auto;
	}

	.session-counter {
		background: #f0fdf4;
		border: 1px solid #bbf7d0;
		color: #166534;
		padding: 0.5rem 1rem;
		border-radius: 6px;
		font-size: 0.875rem;
		text-align: center;
		margin-bottom: 1rem;
	}

	.saved-as {
		margin: 0 0 1rem;
		color: #64748b;
		font-size: 0.8125rem;
		text-align: center;
		overflow-wrap: anywhere;
	}

	form {
		background: white;
		border-radius: 12px;
		border: 1px solid #e2e8f0;
		padding: 1.5rem;
		display: flex;
		flex-direction: column;
		gap: 1rem;
	}

	.field {
		display: flex;
		flex-direction: column;
		gap: 0.375rem;
		font-size: 0.875rem;
		color: #475569;
	}

	.optional {
		color: var(--color-text-muted);
		font-weight: normal;
	}

	input,
	textarea {
		padding: 0.75rem;
		border: 1px solid #e2e8f0;
		border-radius: 6px;
		font-size: 1rem;
		min-height: 48px;
		box-sizing: border-box;
	}

	input:focus,
	textarea:focus {
		outline: none;
		border-color: #3b82f6;
		box-shadow: 0 0 0 3px rgba(59, 130, 246, 0.1);
	}

	textarea {
		resize: vertical;
	}

	.name-preview {
		font-size: 0.8125rem;
		color: var(--color-text-muted);
		padding: 0.375rem 0.75rem;
		background: #f8fafc;
		border-radius: 4px;
		margin-top: -0.5rem;
	}

	.preview-label {
		font-weight: 600;
		color: #475569;
	}

	.preview-sep {
		margin: 0 0.5rem;
		color: var(--color-text-muted);
	}

	.gender-group {
		border: none;
		padding: 0;
		margin: 0;
	}

	.gender-group legend {
		font-size: 0.875rem;
		color: #475569;
		margin-bottom: 0.375rem;
	}

	/* Equal widths while they fit; wrap rather than overflow at large font sizes. */
	.gender-buttons {
		display: flex;
		flex-wrap: wrap;
		gap: 0.5rem;
	}

.toggle-notes {
		background: none;
		border: none;
		color: var(--color-text-muted);
		font-size: 0.8125rem;
		cursor: pointer;
		padding: 0.25rem 0;
		text-align: left;
	}

	.toggle-notes:hover {
		color: var(--color-link);
	}

	.form-actions {
		display: flex;
		flex-wrap: wrap;
		gap: 0.75rem;
		padding-top: 0.5rem;
	}

</style>
