<script lang="ts">
	import { page } from '$app/stores';
	import { goto } from '$app/navigation';
	import { api, type PersonDetail, type ChangeHistoryResponse, type Media, type ResearchStatus, formatGenDate, formatPersonName } from '$lib/api/client';
	import ChangeHistory from '$lib/components/ChangeHistory.svelte';
	import MediaGallery from '$lib/components/MediaGallery.svelte';
	import CitationSection from '$lib/components/CitationSection.svelte';
	import UncertaintyBadge from '$lib/components/UncertaintyBadge.svelte';
	import { createShortcutHandler } from '$lib/keyboard/useShortcuts.svelte';

	let person: PersonDetail | null = $state(null);
	let loading = $state(true);
	let error: string | null = $state(null);
	let editing = $state(false);
	let saving = $state(false);
	let historyExpanded = $state(false);
	let historyCount: number | null = $state(null);
	let mediaCount: number | null = $state(null);

	// Form state
	let formData = $state({
		given_name: '',
		surname: '',
		gender: '' as 'male' | 'female' | 'unknown' | '',
		birth_date: '',
		birth_place: '',
		death_date: '',
		death_place: '',
		notes: '',
		research_status: '' as ResearchStatus | ''
	});

	async function loadPerson(id: string) {
		loading = true;
		error = null;
		try {
			person = await api.getPerson(id);
			resetForm();
			// Fetch history count for badge
			const historyResponse = await api.getPersonHistory(id, { limit: 1, offset: 0 });
			historyCount = historyResponse.total;
			// Fetch media count for badge
			const mediaResponse = await api.listPersonMedia(id, { limit: 1, offset: 0 });
			mediaCount = mediaResponse.total;
		} catch (e) {
			error = (e as { message?: string }).message || 'Failed to load person';
			person = null;
		} finally {
			loading = false;
		}
	}

	function toggleHistory() {
		historyExpanded = !historyExpanded;
	}

	function handleMediaAdded(media: Media) {
		if (mediaCount !== null) {
			mediaCount++;
		}
	}

	function resetForm() {
		if (person) {
			formData = {
				given_name: person.given_name,
				surname: person.surname,
				gender: person.gender || '',
				birth_date: person.birth_date?.raw || '',
				birth_place: person.birth_place || '',
				death_date: person.death_date?.raw || '',
				death_place: person.death_place || '',
				notes: person.notes || '',
				research_status: person.research_status || ''
			};
		}
	}

	function startEdit() {
		resetForm();
		editing = true;
	}

	function cancelEdit() {
		resetForm();
		editing = false;
	}

	async function savePerson() {
		if (!person) return;
		saving = true;
		try {
			await api.updatePerson(person.id, {
				given_name: formData.given_name || undefined,
				surname: formData.surname || undefined,
				gender: (formData.gender || undefined) as 'male' | 'female' | 'unknown' | undefined,
				birth_date: formData.birth_date || undefined,
				birth_place: formData.birth_place || undefined,
				death_date: formData.death_date || undefined,
				death_place: formData.death_place || undefined,
				notes: formData.notes || undefined,
				research_status: (formData.research_status || undefined) as ResearchStatus | undefined,
				version: person.version
			});
			await loadPerson(person.id);
			editing = false;
		} catch (e) {
			error = (e as { message?: string }).message || 'Failed to save';
		} finally {
			saving = false;
		}
	}

	async function deletePerson() {
		if (!person) return;
		if (!confirm(`Delete ${formatPersonName(person)}? This cannot be undone.`)) return;

		try {
			await api.deletePerson(person.id);
			goto('/persons');
		} catch (e) {
			error = (e as { message?: string }).message || 'Failed to delete';
		}
	}

	$effect(() => {
		const id = $page.params.id;
		if (id) {
			loadPerson(id);
		}
	});

	// Keyboard shortcut handlers
	const { handleKeydown } = createShortcutHandler('person-detail', {
		'edit': () => {
			if (!editing && person && !loading) {
				startEdit();
			}
		},
		'save': () => {
			if (editing && !saving) {
				savePerson();
			}
		},
		'cancel': () => {
			if (editing) {
				cancelEdit();
			}
		}
	});
</script>

<svelte:head>
	<title>{person ? formatPersonName(person) : 'Person'} | My Family</title>
</svelte:head>

<svelte:window onkeydown={handleKeydown} />

<div class="person-page">
	<header class="page-header">
		<a href="/persons" class="back-link">&larr; People</a>
		{#if person && !editing}
			<div class="actions">
				<a href="/pedigree/{person.id}" class="btn">View Pedigree</a>
				<button class="btn" onclick={startEdit}>Edit</button>
				<button class="btn btn-danger" onclick={deletePerson}>Delete</button>
			</div>
		{/if}
	</header>

	{#if loading}
		<div class="loading">Loading...</div>
	{:else if error}
		<div class="error">{error}</div>
	{:else if person}
		{#if editing}
			<form class="edit-form" onsubmit={(e) => { e.preventDefault(); savePerson(); }}>
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
							<option value="">Unknown</option>
							<option value="male">Male</option>
							<option value="female">Female</option>
						</select>
					</label>
					<label>
						Research Status
						<select bind:value={formData.research_status}>
							<option value="">Not assessed</option>
							<option value="certain">Certain - Confirmed with strong evidence</option>
							<option value="probable">Probable - Good supporting evidence</option>
							<option value="possible">Possible - Limited evidence</option>
							<option value="unknown">Unknown - Not yet assessed</option>
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
					<button type="button" class="btn" onclick={cancelEdit} disabled={saving}>Cancel</button>
					<button type="submit" class="btn btn-primary" disabled={saving}>
						{saving ? 'Saving...' : 'Save Changes'}
					</button>
				</div>
			</form>
		{:else}
			<div class="person-detail">
				<div class="person-header" data-gender={person.gender}>
					<div class="avatar">
						<svg viewBox="0 0 24 24" fill="currentColor">
							<path d="M12 4a4 4 0 1 0 0 8 4 4 0 0 0 0-8zM6 8a6 6 0 1 1 12 0A6 6 0 0 1 6 8zm2 10a3 3 0 0 0-3 3 1 1 0 1 1-2 0 5 5 0 0 1 5-5h8a5 5 0 0 1 5 5 1 1 0 1 1-2 0 3 3 0 0 0-3-3H8z" />
						</svg>
					</div>
					<div class="person-title">
						<div class="name-row">
							<h1>{formatPersonName(person)}</h1>
							{#if person.research_status}
								<UncertaintyBadge status={person.research_status} showLabel={true} />
							{/if}
						</div>
						{#if person.gender}
							<span class="gender-badge">{person.gender}</span>
						{/if}
					</div>
				</div>

				<div class="info-grid">
					<div class="info-section">
						<h2>Birth</h2>
						<dl>
							<dt>Date</dt>
							<dd>{formatGenDate(person.birth_date) || '—'}</dd>
							<dt>Place</dt>
							<dd>{person.birth_place || '—'}</dd>
						</dl>
					</div>

					<div class="info-section">
						<h2>Death</h2>
						<dl>
							<dt>Date</dt>
							<dd>{formatGenDate(person.death_date) || '—'}</dd>
							<dt>Place</dt>
							<dd>{person.death_place || '—'}</dd>
						</dl>
					</div>
				</div>

				{#if person.notes}
					<div class="info-section">
						<h2>Notes</h2>
						<p class="notes">{person.notes}</p>
					</div>
				{/if}

				{#if person.families_as_partner && person.families_as_partner.length > 0}
					<div class="info-section">
						<h2>Families</h2>
						<ul class="family-list">
							{#each person.families_as_partner as family}
								<li>
									<a href="/families/{family.id}">
										{family.partner1_name || 'Unknown'}
										{#if family.partner2_name} &amp; {family.partner2_name}{/if}
										{#if family.relationship_type}
											<span class="badge">{family.relationship_type}</span>
										{/if}
									</a>
								</li>
							{/each}
						</ul>
					</div>
				{/if}

				{#if person.family_as_child}
					<div class="info-section">
						<h2>Parents</h2>
						<a href="/families/{person.family_as_child.id}" class="parent-link">
							{person.family_as_child.partner1_name || 'Unknown'}
							{#if person.family_as_child.partner2_name} &amp; {person.family_as_child.partner2_name}{/if}
						</a>
					</div>
				{/if}

				<div class="info-section media-section">
					<h2>
						Media
						{#if mediaCount !== null && mediaCount > 0}
							<span class="count-badge">{mediaCount}</span>
						{/if}
					</h2>
					<MediaGallery personId={person.id} onMediaAdded={handleMediaAdded} />
				</div>

				<CitationSection personId={person.id} />

				<div class="history-section">
					<button class="history-header" onclick={toggleHistory}>
						<h2>
							History
							{#if historyCount !== null}
								<span class="count-badge">{historyCount}</span>
							{/if}
						</h2>
						<span class="expand-icon">{historyExpanded ? '−' : '+'}</span>
					</button>
					{#if historyExpanded}
						<div class="history-content">
							<ChangeHistory entityType="person" entityId={person.id} />
						</div>
					{/if}
				</div>
			</div>
		{/if}
	{/if}
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

	.loading,
	.error {
		text-align: center;
		padding: 3rem;
		color: #64748b;
	}

	.error {
		color: #dc2626;
	}

	.person-detail {
		background: white;
		border-radius: 12px;
		border: 1px solid #e2e8f0;
		padding: 1.5rem;
	}

	.person-header {
		display: flex;
		align-items: center;
		gap: 1rem;
		margin-bottom: 1.5rem;
		padding-bottom: 1.5rem;
		border-bottom: 1px solid #e2e8f0;
	}

	.avatar {
		width: 4rem;
		height: 4rem;
		border-radius: 50%;
		display: flex;
		align-items: center;
		justify-content: center;
	}

	.avatar svg {
		width: 2rem;
		height: 2rem;
	}

	[data-gender="male"] .avatar {
		background: #dbeafe;
		color: #3b82f6;
	}

	[data-gender="female"] .avatar {
		background: #fce7f3;
		color: #ec4899;
	}

	[data-gender="unknown"] .avatar,
	.person-header:not([data-gender]) .avatar {
		background: #f1f5f9;
		color: #64748b;
	}

	.name-row {
		display: flex;
		align-items: center;
		gap: 0.75rem;
		flex-wrap: wrap;
	}

	.person-title h1 {
		margin: 0;
		font-size: 1.5rem;
		color: #1e293b;
	}

	.gender-badge {
		display: inline-block;
		padding: 0.125rem 0.5rem;
		background: #f1f5f9;
		border-radius: 4px;
		font-size: 0.75rem;
		color: #64748b;
		text-transform: capitalize;
		margin-top: 0.25rem;
	}

	.info-grid {
		display: grid;
		grid-template-columns: repeat(2, 1fr);
		gap: 1.5rem;
		margin-bottom: 1.5rem;
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

	.family-list {
		list-style: none;
		padding: 0;
		margin: 0;
	}

	.family-list li {
		padding: 0.5rem 0;
	}

	.family-list a {
		color: #1e293b;
		text-decoration: none;
	}

	.family-list a:hover {
		color: #3b82f6;
	}

	.parent-link {
		color: #1e293b;
		text-decoration: none;
	}

	.parent-link:hover {
		color: #3b82f6;
	}

	.badge {
		display: inline-block;
		padding: 0.125rem 0.375rem;
		background: #f1f5f9;
		border-radius: 4px;
		font-size: 0.6875rem;
		color: #64748b;
		margin-left: 0.5rem;
		text-transform: capitalize;
	}

	.count-badge {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		min-width: 1.25rem;
		height: 1.25rem;
		padding: 0 0.375rem;
		background: #3b82f6;
		border-radius: 9999px;
		font-size: 0.6875rem;
		font-weight: 600;
		color: white;
		margin-left: 0.5rem;
		vertical-align: middle;
	}

	.media-section {
		margin-top: 1.5rem;
		padding-top: 1.5rem;
		border-top: 1px solid #e2e8f0;
	}

	.media-section h2 {
		display: flex;
		align-items: center;
	}

	/* History section styles */
	.history-section {
		margin-top: 1.5rem;
		padding-top: 1.5rem;
		border-top: 1px solid #e2e8f0;
	}

	.history-header {
		display: flex;
		align-items: center;
		justify-content: space-between;
		width: 100%;
		padding: 0;
		border: none;
		background: none;
		cursor: pointer;
		text-align: left;
	}

	.history-header h2 {
		display: flex;
		align-items: center;
		margin: 0;
		font-size: 0.875rem;
		font-weight: 600;
		color: #64748b;
		text-transform: uppercase;
		letter-spacing: 0.05em;
	}

	.expand-icon {
		font-size: 1.25rem;
		font-weight: 600;
		color: #64748b;
	}

	.history-content {
		margin-top: 1rem;
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
</style>
