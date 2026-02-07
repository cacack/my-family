<script lang="ts">
	import { api, type PersonCreate } from '$lib/api/client';
	import { goto } from '$app/navigation';
	import { parseName } from '$lib/utils/nameParse';

	let nameInput = $state('');
	let gender = $state<'male' | 'female' | 'unknown'>('unknown');
	let birthYear = $state('');
	let notes = $state('');
	let showNotes = $state(false);
	let sessionCount = $state(0);
	let error = $state<string | null>(null);
	let saving = $state(false);

	let parsed = $derived(parseName(nameInput));

	async function submit(mode: 'next' | 'view') {
		if (!parsed.givenName) {
			error = 'Please enter at least a given name.';
			return;
		}

		saving = true;
		error = null;

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

			if (mode === 'view') {
				goto(`/persons/${person.id}`);
			} else {
				// Reset form for next entry
				nameInput = '';
				gender = 'unknown';
				birthYear = '';
				notes = '';
				showNotes = false;
			}
		} catch (e) {
			error = (e as { message?: string }).message || 'Failed to create person';
		} finally {
			saving = false;
		}
	}
</script>

<div class="quick-capture">
	{#if sessionCount > 0}
		<div class="session-counter">
			{sessionCount} {sessionCount === 1 ? 'person' : 'people'} added this session
		</div>
	{/if}

	{#if error}
		<div class="error">{error}</div>
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
				<button
					type="button"
					class="gender-btn"
					class:active={gender === 'male'}
					onclick={() => (gender = 'male')}
				>
					Male
				</button>
				<button
					type="button"
					class="gender-btn"
					class:active={gender === 'female'}
					onclick={() => (gender = 'female')}
				>
					Female
				</button>
				<button
					type="button"
					class="gender-btn"
					class:active={gender === 'unknown'}
					onclick={() => (gender = 'unknown')}
				>
					Unknown
				</button>
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
			<button type="submit" class="btn btn-primary" disabled={saving}>
				{saving ? 'Adding...' : 'Add & Next'}
			</button>
			<button type="button" class="btn btn-secondary" disabled={saving} onclick={() => submit('view')}>
				Add & View
			</button>
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

	.error {
		background: #fef2f2;
		border: 1px solid #fecaca;
		color: #dc2626;
		padding: 0.75rem 1rem;
		border-radius: 6px;
		margin-bottom: 1rem;
		font-size: 0.875rem;
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
		color: #94a3b8;
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
		color: #64748b;
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
		color: #cbd5e1;
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

	.gender-buttons {
		display: grid;
		grid-template-columns: repeat(3, 1fr);
		gap: 0.5rem;
	}

	.gender-btn {
		padding: 0.75rem;
		min-height: 48px;
		border: 1px solid #e2e8f0;
		border-radius: 6px;
		background: white;
		font-size: 0.875rem;
		cursor: pointer;
		transition: all 0.15s;
		color: #475569;
	}

	.gender-btn:hover {
		background: #f1f5f9;
	}

	.gender-btn.active {
		background: #eff6ff;
		border-color: #3b82f6;
		color: #1d4ed8;
		font-weight: 600;
	}

	.toggle-notes {
		background: none;
		border: none;
		color: #64748b;
		font-size: 0.8125rem;
		cursor: pointer;
		padding: 0.25rem 0;
		text-align: left;
	}

	.toggle-notes:hover {
		color: #3b82f6;
	}

	.form-actions {
		display: flex;
		gap: 0.75rem;
		padding-top: 0.5rem;
	}

	.btn {
		padding: 0.75rem 1.25rem;
		min-height: 48px;
		border-radius: 6px;
		font-size: 0.875rem;
		font-weight: 500;
		cursor: pointer;
		transition: all 0.15s;
	}

	.btn:disabled {
		opacity: 0.6;
		cursor: not-allowed;
	}

	.btn-primary {
		flex: 1;
		background: #3b82f6;
		border: 1px solid #3b82f6;
		color: white;
	}

	.btn-primary:hover:not(:disabled) {
		background: #2563eb;
	}

	.btn-secondary {
		background: white;
		border: 1px solid #cbd5e1;
		color: #475569;
	}

	.btn-secondary:hover:not(:disabled) {
		background: #f1f5f9;
	}
</style>
