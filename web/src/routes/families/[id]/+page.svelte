<script lang="ts">
	import { page } from '$app/stores';
	import { goto } from '$app/navigation';
	import { api, type FamilyDetail, formatGenDate, formatPersonName } from '$lib/api/client';
	import ChangeHistory from '$lib/components/ChangeHistory.svelte';
	import { createShortcutHandler } from '$lib/keyboard/useShortcuts.svelte';

	let family: FamilyDetail | null = $state(null);
	let loading = $state(true);
	let error: string | null = $state(null);
	let editing = $state(false);
	let saving = $state(false);
	let historyExpanded = $state(false);
	let historyCount: number | null = $state(null);

	// Form state
	let formData = $state({
		relationship_type: '' as 'marriage' | 'partnership' | 'unknown' | '',
		marriage_date: '',
		marriage_place: ''
	});

	async function loadFamily(id: string) {
		loading = true;
		error = null;
		try {
			family = await api.getFamily(id);
			resetForm();
			// Fetch history count for badge
			const historyResponse = await api.getFamilyHistory(id, { limit: 1, offset: 0 });
			historyCount = historyResponse.total;
		} catch (e) {
			error = (e as { message?: string }).message || 'Failed to load family';
			family = null;
		} finally {
			loading = false;
		}
	}

	function toggleHistory() {
		historyExpanded = !historyExpanded;
	}

	function resetForm() {
		if (family) {
			formData = {
				relationship_type: family.relationship_type || '',
				marriage_date: family.marriage_date?.raw || '',
				marriage_place: family.marriage_place || ''
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

	async function saveFamily() {
		if (!family) return;
		saving = true;
		try {
			await api.updateFamily(family.id, {
				relationship_type: (formData.relationship_type || undefined) as
					| 'marriage'
					| 'partnership'
					| 'unknown'
					| undefined,
				marriage_date: formData.marriage_date || undefined,
				marriage_place: formData.marriage_place || undefined,
				version: family.version
			});
			await loadFamily(family.id);
			editing = false;
		} catch (e) {
			error = (e as { message?: string }).message || 'Failed to save';
		} finally {
			saving = false;
		}
	}

	async function deleteFamily() {
		if (!family) return;
		if (!confirm('Delete this family? This cannot be undone.')) return;

		try {
			await api.deleteFamily(family.id);
			goto('/families');
		} catch (e) {
			error = (e as { message?: string }).message || 'Failed to delete';
		}
	}

	function getPartnerDisplay(): string {
		if (!family) return '';
		const p1 = family.partner1 ? formatPersonName(family.partner1) : family.partner1_name || 'Unknown';
		const p2 = family.partner2 ? formatPersonName(family.partner2) : family.partner2_name;
		return p2 ? `${p1} & ${p2}` : p1;
	}

	$effect(() => {
		const id = $page.params.id;
		if (id) {
			loadFamily(id);
		}
	});

	// Keyboard shortcut handlers
	const { handleKeydown } = createShortcutHandler('family-detail', {
		'edit': () => {
			if (!editing && family && !loading) {
				startEdit();
			}
		},
		'save': () => {
			if (editing && !saving) {
				saveFamily();
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
	<title>{family ? getPartnerDisplay() : 'Family'} | My Family</title>
</svelte:head>

<svelte:window onkeydown={handleKeydown} />

<div class="family-page">
	<header class="page-header">
		<a href="/families" class="back-link">&larr; Families</a>
		{#if family && !editing}
			<div class="actions">
				<a href="/families/{family.id}/group-sheet" class="btn">Group Sheet</a>
				<button class="btn" onclick={startEdit}>Edit</button>
				<button class="btn btn-danger" onclick={deleteFamily}>Delete</button>
			</div>
		{/if}
	</header>

	{#if loading}
		<div class="loading">Loading...</div>
	{:else if error}
		<div class="error">{error}</div>
	{:else if family}
		{#if editing}
			<form class="edit-form" onsubmit={(e) => { e.preventDefault(); saveFamily(); }}>
				<h2 class="edit-title">{getPartnerDisplay()}</h2>

				<div class="form-row">
					<label>
						Relationship Type
						<select bind:value={formData.relationship_type}>
							<option value="">Unknown</option>
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
						<input type="text" bind:value={formData.marriage_place} />
					</label>
				</div>

				<div class="form-actions">
					<button type="button" class="btn" onclick={cancelEdit} disabled={saving}>Cancel</button>
					<button type="submit" class="btn btn-primary" disabled={saving}>
						{saving ? 'Saving...' : 'Save Changes'}
					</button>
				</div>
			</form>
		{:else}
			<div class="family-detail">
				<div class="family-header">
					<h1>{getPartnerDisplay()}</h1>
					{#if family.relationship_type}
						<span class="relationship-badge">{family.relationship_type}</span>
					{/if}
				</div>

				<div class="partners-section">
					<h2>Partners</h2>
					<div class="partners-grid">
						{#if family.partner1}
							<a href="/persons/{family.partner1.id}" class="partner-card">
								<div class="partner-name">{formatPersonName(family.partner1)}</div>
							</a>
						{:else if family.partner1_name}
							<div class="partner-card">
								<div class="partner-name">{family.partner1_name}</div>
							</div>
						{/if}

						{#if family.partner2}
							<a href="/persons/{family.partner2.id}" class="partner-card">
								<div class="partner-name">{formatPersonName(family.partner2)}</div>
							</a>
						{:else if family.partner2_name}
							<div class="partner-card">
								<div class="partner-name">{family.partner2_name}</div>
							</div>
						{/if}
					</div>
				</div>

				{#if family.marriage_date || family.marriage_place}
					<div class="info-section">
						<h2>Marriage</h2>
						<dl>
							{#if family.marriage_date}
								<dt>Date</dt>
								<dd>{formatGenDate(family.marriage_date)}</dd>
							{/if}
							{#if family.marriage_place}
								<dt>Place</dt>
								<dd>{family.marriage_place}</dd>
							{/if}
						</dl>
					</div>
				{/if}

				{#if family.children && family.children.length > 0}
					<div class="info-section">
						<h2>Children ({family.children.length})</h2>
						<ul class="children-list">
							{#each family.children as child}
								<li>
									<a href="/persons/{child.id}">
										{child.name}
									</a>
									{#if child.relationship_type && child.relationship_type !== 'biological'}
										<span class="child-type">({child.relationship_type})</span>
									{/if}
								</li>
							{/each}
						</ul>
					</div>
				{:else}
					<div class="info-section">
						<h2>Children</h2>
						<p class="empty-message">No children recorded</p>
					</div>
				{/if}

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
							<ChangeHistory entityType="family" entityId={family.id} />
						</div>
					{/if}
				</div>
			</div>
		{/if}
	{/if}
</div>

<style>
	.family-page {
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

	.family-detail {
		background: white;
		border-radius: 12px;
		border: 1px solid #e2e8f0;
		padding: 1.5rem;
	}

	.family-header {
		margin-bottom: 1.5rem;
		padding-bottom: 1.5rem;
		border-bottom: 1px solid #e2e8f0;
	}

	.family-header h1 {
		margin: 0 0 0.5rem;
		font-size: 1.5rem;
		color: #1e293b;
	}

	.relationship-badge {
		display: inline-block;
		padding: 0.25rem 0.75rem;
		background: #f1f5f9;
		border-radius: 4px;
		font-size: 0.875rem;
		color: #64748b;
		text-transform: capitalize;
	}

	.partners-section {
		margin-bottom: 1.5rem;
	}

	.partners-section h2 {
		margin: 0 0 0.75rem;
		font-size: 0.875rem;
		font-weight: 600;
		color: #64748b;
		text-transform: uppercase;
		letter-spacing: 0.05em;
	}

	.partners-grid {
		display: grid;
		grid-template-columns: repeat(2, 1fr);
		gap: 1rem;
	}

	.partner-card {
		display: block;
		padding: 1rem;
		background: #f8fafc;
		border-radius: 8px;
		text-decoration: none;
		border: 1px solid #e2e8f0;
		transition: border-color 0.2s;
	}

	a.partner-card:hover {
		border-color: #3b82f6;
	}

	a.partner-card:hover .partner-name {
		color: #3b82f6;
	}

	.partner-name {
		font-weight: 500;
		color: #1e293b;
		transition: color 0.2s;
	}

	.info-section {
		margin-bottom: 1.5rem;
	}

	.info-section:last-child {
		margin-bottom: 0;
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

	.children-list {
		list-style: none;
		padding: 0;
		margin: 0;
	}

	.children-list li {
		padding: 0.5rem 0;
		border-bottom: 1px solid #f1f5f9;
	}

	.children-list li:last-child {
		border-bottom: none;
	}

	.children-list a {
		color: #1e293b;
		text-decoration: none;
	}

	.children-list a:hover {
		color: #3b82f6;
	}

	.child-type {
		color: #94a3b8;
		font-size: 0.75rem;
		margin-left: 0.5rem;
	}

	.empty-message {
		margin: 0;
		color: #94a3b8;
		font-size: 0.875rem;
		font-style: italic;
	}

	/* Edit form styles */
	.edit-form {
		background: white;
		border-radius: 12px;
		border: 1px solid #e2e8f0;
		padding: 1.5rem;
	}

	.edit-title {
		margin: 0 0 1.5rem;
		font-size: 1.25rem;
		color: #1e293b;
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

	.btn-primary {
		background: #3b82f6;
		border-color: #3b82f6;
		color: white;
	}

	.btn-primary:hover {
		background: #2563eb;
	}

	/* History section styles */
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
</style>
