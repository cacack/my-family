<script lang="ts">
	import { page } from '$app/stores';
	import { goto } from '$app/navigation';
	import { api, type PersonDetail, type ChangeHistoryResponse, type Media, type ResearchStatus, type RollbackResponse, formatGenDate, formatPersonName } from '$lib/api/client';
	import ChangeHistory from '$lib/components/ChangeHistory.svelte';
	import RestorePointBrowser from '$lib/components/RestorePointBrowser.svelte';
	import RollbackConfirmDialog from '$lib/components/RollbackConfirmDialog.svelte';
	import RollbackSuccessBanner from '$lib/components/RollbackSuccessBanner.svelte';
	import MediaGallery from '$lib/components/MediaGallery.svelte';
	import CitationSection from '$lib/components/CitationSection.svelte';
	import EvidencePanel from '$lib/components/EvidencePanel.svelte';
	import NameSection from '$lib/components/NameSection.svelte';
	import UncertaintyBadge from '$lib/components/UncertaintyBadge.svelte';
	import { createShortcutHandler } from '$lib/keyboard/useShortcuts.svelte';
	import { Button } from '$lib/components/ui/button';
	import { Badge } from '$lib/components/ui/badge';

	let person: PersonDetail | null = $state(null);
	let loading = $state(true);
	let error: string | null = $state(null);
	let editing = $state(false);
	let saving = $state(false);
	let historyExpanded = $state(false);
	let historyTab: 'history' | 'restore' = $state('history');
	let historyCount: number | null = $state(null);
	let mediaCount: number | null = $state(null);

	// Brick wall state
	let showBrickWallForm = $state(false);
	let brickWallNote = $state('');
	let brickWallSaving = $state(false);
	let brickWallCelebrating = $state(false);
	let brickWallToast = $state('');

	// Rollback state
	let rollbackDialog = $state({ open: false, targetVersion: 0, targetSummary: '' });
	let rollbackSuccess: { show: boolean; message: string; changes?: Record<string, unknown> } = $state({ show: false, message: '' });

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

	function handleSelectVersion(version: number, summary: string) {
		rollbackDialog = { open: true, targetVersion: version, targetSummary: summary };
	}

	async function handleRollbackConfirm(response: RollbackResponse) {
		rollbackDialog = { open: false, targetVersion: 0, targetSummary: '' };
		if (person) {
			await loadPerson(person.id);
		}
		historyTab = 'history';
		rollbackSuccess = {
			show: true,
			message: response.message || 'Successfully restored to version ' + response.new_version,
			changes: response.changes
		};
	}

	function handleRollbackCancel() {
		rollbackDialog = { open: false, targetVersion: 0, targetSummary: '' };
	}

	function dismissRollbackSuccess() {
		rollbackSuccess = { show: false, message: '' };
	}

	function handleMediaAdded(media: Media) {
		if (mediaCount !== null) {
			mediaCount++;
		}
	}

	async function markBrickWall() {
		if (!person || !brickWallNote.trim()) return;
		brickWallSaving = true;
		try {
			await api.setPersonBrickWall(person.id, brickWallNote.trim());
			await loadPerson(person.id);
			showBrickWallForm = false;
			brickWallNote = '';
		} catch (e) {
			error = (e as { message?: string }).message || 'Failed to mark brick wall';
		} finally {
			brickWallSaving = false;
		}
	}

	async function resolveBrickWall() {
		if (!person) return;
		brickWallSaving = true;
		try {
			await api.resolvePersonBrickWall(person.id);
			brickWallCelebrating = true;
			brickWallToast = 'Brick wall broken! Great research breakthrough!';
			await loadPerson(person.id);
			setTimeout(() => {
				brickWallCelebrating = false;
				brickWallToast = '';
			}, 3000);
		} catch (e) {
			error = (e as { message?: string }).message || 'Failed to resolve brick wall';
		} finally {
			brickWallSaving = false;
		}
	}

	function cancelBrickWallForm() {
		showBrickWallForm = false;
		brickWallNote = '';
	}

	function formatBrickWallDate(dateStr: string): string {
		return new Date(dateStr).toLocaleDateString();
	}

	function formatBrickWallDuration(dateStr: string): string {
		const date = new Date(dateStr);
		const now = new Date();
		const diffMs = now.getTime() - date.getTime();
		const diffDays = Math.floor(diffMs / (1000 * 60 * 60 * 24));
		if (diffDays === 0) return 'today';
		if (diffDays === 1) return '1 day ago';
		if (diffDays < 30) return `${diffDays} days ago`;
		const diffMonths = Math.floor(diffDays / 30);
		if (diffMonths === 1) return '1 month ago';
		if (diffMonths < 12) return `${diffMonths} months ago`;
		const diffYears = Math.floor(diffMonths / 12);
		if (diffYears === 1) return '1 year ago';
		return `${diffYears} years ago`;
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
				<Button variant="outline" href="/pedigree/{person.id}">Pedigree</Button>
				<Button variant="outline" href="/ahnentafel/{person.id}">Ahnentafel</Button>
				<Button variant="outline" onclick={startEdit}>Edit</Button>
				<Button variant="destructive" onclick={deletePerson}>Delete</Button>
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
					<Button variant="outline" onclick={cancelEdit} disabled={saving}>Cancel</Button>
					<Button type="submit" disabled={saving}>
						{saving ? 'Saving...' : 'Save Changes'}
					</Button>
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

				<!-- Brick Wall Section -->
				<div class="brick-wall-section" class:celebrating={brickWallCelebrating}>
					{#if brickWallToast}
						<div class="brick-wall-toast" role="status" aria-live="polite">
							{brickWallToast}
						</div>
					{/if}

					{#if person.brick_wall_note && !person.brick_wall_resolved_at}
						<!-- Active brick wall -->
						<div class="brick-wall-indicator active">
							<div class="brick-wall-header">
								<Badge variant="destructive" class="gap-1 uppercase text-[0.6875rem] tracking-wide font-semibold">
									<svg viewBox="0 0 24 24" fill="currentColor" class="size-3">
										<path d="M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm1 15h-2v-2h2v2zm0-4h-2V7h2v6z"/>
									</svg>
									Brick Wall
								</Badge>
								{#if person.brick_wall_since}
									<span class="brick-wall-since">Since {formatBrickWallDuration(person.brick_wall_since)}</span>
								{/if}
							</div>
							<p class="brick-wall-note">{person.brick_wall_note}</p>
							<Button
								variant="warning"
								onclick={resolveBrickWall}
								disabled={brickWallSaving}
							>
								{brickWallSaving ? 'Resolving...' : 'Resolve Brick Wall'}
							</Button>
						</div>
					{:else if person.brick_wall_resolved_at}
						<!-- Resolved brick wall -->
						<div class="brick-wall-indicator resolved">
							<div class="brick-wall-header">
								<Badge variant="outline" class="gap-1 border-green-200 bg-green-50 text-green-700 uppercase text-[0.6875rem] tracking-wide font-semibold dark:border-green-800 dark:bg-green-950 dark:text-green-400">
									<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" class="size-3">
										<polyline points="20 6 9 17 4 12" />
									</svg>
									Resolved
								</Badge>
								<span class="brick-wall-since">Resolved {formatBrickWallDate(person.brick_wall_resolved_at)}</span>
							</div>
							{#if person.brick_wall_note}
								<p class="brick-wall-note">{person.brick_wall_note}</p>
							{/if}
						</div>
					{:else}
						<!-- No brick wall — show Mark button -->
						{#if showBrickWallForm}
							<div class="brick-wall-form">
								<label class="brick-wall-form-label">
									Brick wall note
									<textarea
										bind:value={brickWallNote}
										placeholder="Describe the research obstacle..."
										rows="3"
									></textarea>
								</label>
								<div class="brick-wall-form-actions">
									<Button variant="outline" onclick={cancelBrickWallForm} disabled={brickWallSaving}>Cancel</Button>
									<Button
										variant="warning"
										onclick={markBrickWall}
										disabled={brickWallSaving || !brickWallNote.trim()}
									>
										{brickWallSaving ? 'Saving...' : 'Mark as Brick Wall'}
									</Button>
								</div>
							</div>
						{:else}
							<Button variant="ghost" onclick={() => showBrickWallForm = true}>
								Mark as Brick Wall
							</Button>
						{/if}
					{/if}
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
											<Badge variant="secondary" class="capitalize">{family.relationship_type}</Badge>
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
							<Badge variant="outline" class="ml-2">{mediaCount}</Badge>
						{/if}
					</h2>
					<MediaGallery personId={person.id} onMediaAdded={handleMediaAdded} />
				</div>

				<CitationSection personId={person.id} />

				<EvidencePanel subjectId={person.id} />

				<NameSection personId={person.id} />

				{#if rollbackSuccess.show}
					<RollbackSuccessBanner
						message={rollbackSuccess.message}
						changes={rollbackSuccess.changes}
						onDismiss={dismissRollbackSuccess}
					/>
				{/if}

				<div class="history-section">
					<button class="history-header" onclick={toggleHistory}>
						<h2>
							History
							{#if historyCount !== null}
								<Badge variant="outline" class="ml-2">{historyCount}</Badge>
							{/if}
						</h2>
						<span class="expand-icon">{historyExpanded ? '−' : '+'}</span>
					</button>
					{#if historyExpanded}
						<div class="history-tabs">
							<button
								class="tab-btn"
								class:active={historyTab === 'history'}
								onclick={() => historyTab = 'history'}
							>
								Change Log
							</button>
							<button
								class="tab-btn"
								class:active={historyTab === 'restore'}
								onclick={() => historyTab = 'restore'}
							>
								Restore
							</button>
						</div>
						<div class="history-content">
							{#if historyTab === 'history'}
								<ChangeHistory entityType="person" entityId={person.id} />
							{:else}
								<RestorePointBrowser
									entityType="person"
									entityId={person.id}
									currentVersion={person.version}
									onSelectVersion={handleSelectVersion}
								/>
							{/if}
						</div>
					{/if}
				</div>

				<RollbackConfirmDialog
					open={rollbackDialog.open}
					entityType="person"
					entityId={person.id}
					entityName={formatPersonName(person)}
					currentVersion={person.version}
					targetVersion={rollbackDialog.targetVersion}
					targetSummary={rollbackDialog.targetSummary}
					onConfirm={handleRollbackConfirm}
					onCancel={handleRollbackCancel}
				/>
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

	/* History tab styles */
	.history-tabs {
		display: flex;
		gap: 0;
		margin-top: 0.75rem;
		margin-bottom: 0.75rem;
		border-bottom: 2px solid #e2e8f0;
	}

	.tab-btn {
		padding: 0.5rem 1rem;
		border: none;
		background: none;
		font-size: 0.8125rem;
		font-weight: 500;
		color: #64748b;
		cursor: pointer;
		border-bottom: 2px solid transparent;
		margin-bottom: -2px;
		transition: color 0.15s, border-color 0.15s;
	}

	.tab-btn:hover {
		color: #475569;
	}

	.tab-btn.active {
		color: #3b82f6;
		border-bottom-color: #3b82f6;
	}

	/* Brick Wall styles */
	.brick-wall-section {
		margin-top: 1.5rem;
		padding-top: 1.5rem;
		border-top: 1px solid #e2e8f0;
	}

	.brick-wall-toast {
		padding: 0.75rem 1rem;
		margin-bottom: 1rem;
		background: #dcfce7;
		border: 1px solid #86efac;
		border-radius: 8px;
		color: #15803d;
		font-size: 0.875rem;
		font-weight: 500;
		text-align: center;
	}

	.brick-wall-indicator {
		padding: 1rem;
		border-radius: 8px;
		border: 1px solid #e2e8f0;
	}

	.brick-wall-indicator.active {
		border-left: 4px solid #f59e0b;
		background: #fffbeb;
	}

	.brick-wall-indicator.resolved {
		border-left: 4px solid #22c55e;
		background: #f0fdf4;
	}

	.brick-wall-header {
		display: flex;
		align-items: center;
		gap: 0.75rem;
		margin-bottom: 0.5rem;
	}

	.brick-wall-since {
		font-size: 0.75rem;
		color: #94a3b8;
	}

	.brick-wall-note {
		margin: 0 0 0.75rem;
		font-size: 0.8125rem;
		color: #475569;
		line-height: 1.5;
	}

	.brick-wall-form {
		padding: 1rem;
		background: #f8fafc;
		border-radius: 8px;
		border: 1px solid #e2e8f0;
	}

	.brick-wall-form-label {
		display: flex;
		flex-direction: column;
		gap: 0.375rem;
		font-size: 0.875rem;
		color: #475569;
	}

	.brick-wall-form-actions {
		display: flex;
		justify-content: flex-end;
		gap: 0.5rem;
		margin-top: 0.75rem;
	}

	/* Celebration animation */
	@keyframes celebrate {
		0% { transform: scale(1); }
		50% { transform: scale(1.05); box-shadow: 0 0 20px rgba(34, 197, 94, 0.5); }
		100% { transform: scale(1); }
	}

	.celebrating {
		animation: celebrate 0.6s ease-out;
	}
</style>
