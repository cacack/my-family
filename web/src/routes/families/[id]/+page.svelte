<script lang="ts">
	import { page } from '$app/stores';
	import { goto } from '$app/navigation';
	import { tick } from 'svelte';
	import {
		api,
		type FamilyChild,
		type FamilyDetail,
		type PersonSummary,
		type RollbackResponse,
		formatGenDate,
		formatPersonName,
		type RelationType
	} from '$lib/api/client';
	import AddChildDialog from '$lib/components/AddChildDialog.svelte';
	import PartnerPickers, { partnerChanges } from '$lib/components/PartnerPickers.svelte';
	import * as AlertDialog from '$lib/components/ui/alert-dialog';
	import ChangeHistory from '$lib/components/ChangeHistory.svelte';
	import ExternalLinks from '$lib/components/ExternalLinks.svelte';
	import RestorePointBrowser from '$lib/components/RestorePointBrowser.svelte';
	import RollbackConfirmDialog from '$lib/components/RollbackConfirmDialog.svelte';
	import RollbackSuccessBanner from '$lib/components/RollbackSuccessBanner.svelte';
	import { createShortcutHandler } from '$lib/keyboard/useShortcuts.svelte';
	import { Button } from '$lib/components/ui/button';
	import FormRow from '$lib/components/FormRow.svelte';
	import { Badge } from '$lib/components/ui/badge';
	import { activeBranch } from '$lib/stores/activeBranch.svelte';
	import { ROLLBACK_MAINLINE_ONLY } from '$lib/utils/rollbackScope';
	import { RELATION_TYPE_OPTIONS } from '$lib/utils/enumOptions';

	let family: FamilyDetail | null = $state(null);
	let loading = $state(true);
	let error: string | null = $state(null);
	let editing = $state(false);
	let saving = $state(false);
	let saveError: string | null = $state(null);
	let historyExpanded = $state(false);
	let historyTab: 'history' | 'restore' = $state('history');
	let historyCount: number | null = $state(null);

	// Rollback is mainline-only (ADR-005, #824); see ROLLBACK_MAINLINE_ONLY.
	const rollbackOnMainlineOnly = $derived(activeBranch.id !== null);

	// Rollback state
	let rollbackDialog = $state({ open: false, targetVersion: 0, targetSummary: '' });
	let rollbackSuccess: { show: boolean; message: string; changes?: Record<string, unknown> } = $state({ show: false, message: '' });

	// Children: the Add child dialog and the Remove child confirmation.
	let addChildOpen = $state(false);
	let removeTarget: FamilyChild | null = $state(null);
	let removing = $state(false);
	let removeError: string | null = $state(null);
	let childrenHeading: HTMLHeadingElement | undefined = $state();

	// Polite live region for the outcome of child and partner changes.
	let announcement = $state('');
	function announce(message: string) {
		announcement = '';
		setTimeout(() => {
			announcement = message;
		}, 50);
	}

	// The partner pickers of the edit form.
	let partner1: PersonSummary | null = $state(null);
	let partner2: PersonSummary | null = $state(null);
	// A family must keep at least one partner; the server refuses clearing both.
	const hasPartner = $derived(!!partner1 || !!partner2);

	/** The family's partners and children: none of them can be added as a child. */
	function linkedIds(current: FamilyDetail | null): string[] {
		if (!current) return [];
		return [
			...(current.partner1_id ? [current.partner1_id] : []),
			...(current.partner2_id ? [current.partner2_id] : []),
			...childIdsOf(current)
		];
	}

	function childIdsOf(current: FamilyDetail | null): string[] {
		return (current?.children ?? []).map((child) => child.person_id);
	}

	const linkedPersonIds = $derived(linkedIds(family));
	const childIds = $derived(childIdsOf(family));

	// Form state
	let formData = $state({
		relationship_type: 'unknown' as RelationType,
		marriage_date: '',
		marriage_place: ''
	});

	async function loadFamily(id: string) {
		loading = true;
		error = null;
		try {
			family = await api.getFamily(id);
			resetForm();
		} catch (e) {
			error = (e as { message?: string }).message || 'Failed to load family';
			family = null;
		} finally {
			loading = false;
		}
		if (family) {
			await loadHistoryCount(id);
		}
	}

	/**
	 * Re-read the family after a change made on this page, without the loading
	 * state: the page stays mounted, so focus and open dialogs are not lost.
	 */
	async function refreshFamily(id: string) {
		family = await api.getFamily(id);
		resetForm();
		await loadHistoryCount(id);
	}

	// Guards loadHistoryCount against a slower, older request overwriting a newer one.
	let historyCountRequest = 0;

	/**
	 * The History badge count, loaded independently of the family (#823): if it
	 * fails the badge is dropped but the page still renders. The History panel
	 * reports its own load error when opened.
	 */
	async function loadHistoryCount(id: string) {
		const request = ++historyCountRequest;
		try {
			const response = await api.getFamilyHistory(id, { limit: 1, offset: 0 });
			if (request === historyCountRequest) historyCount = response.total;
		} catch (e) {
			if (request === historyCountRequest) historyCount = null;
			console.warn('Failed to load the family history count', e);
		}
	}

	function toggleHistory() {
		historyExpanded = !historyExpanded;
	}

	function handleSelectVersion(version: number, summary: string) {
		if (rollbackOnMainlineOnly) return;
		rollbackDialog = { open: true, targetVersion: version, targetSummary: summary };
	}

	async function handleRollbackConfirm(response: RollbackResponse) {
		rollbackDialog = { open: false, targetVersion: 0, targetSummary: '' };
		if (family) {
			await loadFamily(family.id);
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

	/** A partner as the picker shows it; a partner without a summary keeps its id. */
	function partnerSummary(
		id: string | undefined,
		summary: PersonSummary | undefined
	): PersonSummary | null {
		if (!id) return null;
		if (summary) return summary;
		return { id, given_name: '', surname: '' };
	}

	function resetForm() {
		if (family) {
			partner1 = partnerSummary(family.partner1_id, family.partner1);
			partner2 = partnerSummary(family.partner2_id, family.partner2);
			formData = {
				relationship_type: family.relationship_type || 'unknown',
				marriage_date: family.marriage_date?.raw || '',
				marriage_place: family.marriage_place || ''
			};
		}
	}

	// The Edit button and the form replace each other, so focus is moved
	// explicitly or it falls to the page body.
	let editButton: HTMLElement | null = $state(null);
	let editForm: HTMLFormElement | null = $state(null);

	async function focusFirstField() {
		await tick();
		editForm?.querySelector<HTMLElement>('input, select, textarea, button')?.focus();
	}

	async function focusEditButton() {
		await tick();
		editButton?.focus();
	}

	function startEdit() {
		resetForm();
		saveError = null;
		editing = true;
		focusFirstField();
	}

	function cancelEdit() {
		resetForm();
		editing = false;
		focusEditButton();
	}

	async function saveFamily() {
		if (!family || !hasPartner) return;
		saving = true;
		saveError = null;
		try {
			await api.updateFamily(family.id, {
				// Sent only when changed, so saving a family with no type
				// doesn't set one.
				relationship_type:
					formData.relationship_type !== (family.relationship_type || 'unknown')
						? formData.relationship_type
						: undefined,
				// An empty string clears the field; an omitted one is left unchanged.
				marriage_date: formData.marriage_date,
				marriage_place: formData.marriage_place,
				...partnerChanges(family, partner1, partner2),
				version: family.version
			});
			await refreshFamily(family.id);
			editing = false;
			focusEditButton();
			announce('Family saved');
		} catch (e) {
			saveError = (e as { message?: string }).message || 'Failed to save';
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

	function childName(child: FamilyChild): string {
		return child.person ? formatPersonName(child.person) : 'Unknown';
	}

	async function handleChildAdded(_child: FamilyChild, person: PersonSummary) {
		if (!family) return;
		try {
			await refreshFamily(family.id);
		} catch (e) {
			error = (e as { message?: string }).message || 'Failed to load family';
			return;
		}
		announce(`${formatPersonName(person)} added as a child`);
	}

	function openRemoveChild(child: FamilyChild) {
		removeError = null;
		removeTarget = child;
	}

	async function confirmRemoveChild(e: Event) {
		// Keep the dialog open until the request settles, so a failure shows in it.
		e.preventDefault();
		if (!family || !removeTarget) return;
		const target = removeTarget;
		removing = true;
		removeError = null;
		try {
			await api.removeChildFromFamily(family.id, target.person_id);
			removeTarget = null;
			await refreshFamily(family.id);
			announce(`${childName(target)} removed from this family`);
			// The row and its button are gone; land focus on the section instead.
			await tick();
			childrenHeading?.focus();
		} catch (err) {
			removeError = (err as { message?: string }).message || 'Failed to remove the child';
		} finally {
			removing = false;
		}
	}

	function getPartnerDisplay(): string {
		if (!family) return '';
		const p1 = family.partner1 ? formatPersonName(family.partner1) : 'Unknown';
		const p2 = family.partner2 ? formatPersonName(family.partner2) : undefined;
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

<div class="sr-only" role="status" aria-live="polite" aria-atomic="true" data-testid="announcer">
	{announcement}
</div>

<div class="family-page">
	<header class="page-header">
		<a href="/families" class="back-link">&larr; Families</a>
		{#if family && !editing}
			<div class="actions">
				<Button variant="outline" href="/families/{family.id}/group-sheet">Group Sheet</Button>
				<Button variant="outline" onclick={startEdit} bind:ref={editButton}>Edit</Button>
				<Button variant="destructive" onclick={deleteFamily}>Delete</Button>
			</div>
		{/if}
	</header>

	{#if loading}
		<div class="loading">Loading...</div>
	{:else if error}
		<div class="error">{error}</div>
	{:else if family}
		{#if editing}
			<form class="edit-form" bind:this={editForm} onsubmit={(e) => { e.preventDefault(); saveFamily(); }}>
				<h2 class="edit-title">{getPartnerDisplay()}</h2>

				<PartnerPickers bind:partner1 bind:partner2 excludeIds={childIds} disabled={saving} />

				{#if saveError}
					<div class="dialog-error" role="alert">{saveError}</div>
				{/if}

				<FormRow>
					<label>
						Relationship Type
						<select bind:value={formData.relationship_type}>
							{#each RELATION_TYPE_OPTIONS as option (option.value)}
								<option value={option.value}>{option.label}</option>
							{/each}
						</select>
					</label>
				</FormRow>

				<FormRow>
					<label>
						Marriage Date
						<input type="text" bind:value={formData.marriage_date} placeholder="e.g., 1 JAN 1850 or ABT 1850" />
					</label>
					<label>
						Marriage Place
						<input type="text" bind:value={formData.marriage_place} />
					</label>
				</FormRow>

				<div class="form-actions">
					<Button variant="outline" onclick={cancelEdit} disabled={saving}>Cancel</Button>
					<Button type="submit" disabled={saving || !hasPartner}>
						{saving ? 'Saving...' : 'Save Changes'}
					</Button>
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
						{/if}

						{#if family.partner2}
							<a href="/persons/{family.partner2.id}" class="partner-card">
								<div class="partner-name">{formatPersonName(family.partner2)}</div>
							</a>
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

				<div class="info-section">
					<div class="section-header">
						<h2 bind:this={childrenHeading} tabindex="-1">
							Children{#if family.children && family.children.length > 0}&nbsp;({family.children.length}){/if}
						</h2>
						<Button variant="outline" size="sm" onclick={() => (addChildOpen = true)}>Add child</Button>
					</div>
					{#if family.children && family.children.length > 0}
						<ul class="children-list">
							{#each family.children as child (child.person_id)}
								<li>
									<a href="/persons/{child.person?.id || child.person_id}">
										{childName(child)}
									</a>
									{#if child.relationship_type && child.relationship_type !== 'biological'}
										<span class="child-type">({child.relationship_type})</span>
									{/if}
									<button
										type="button"
										class="remove-child"
										aria-label="Remove {childName(child)} from this family"
										onclick={() => openRemoveChild(child)}
									>
										Remove
									</button>
								</li>
							{/each}
						</ul>
					{:else}
						<p class="empty-message">No children recorded</p>
					{/if}
				</div>

				<!-- Guard here (in addition to ExternalLinks' own empty check) so the
				     "External links" heading is suppressed when there are none. -->
				{#if family.external_ids && family.external_ids.length > 0}
					<div class="info-section">
						<h2>External links</h2>
						<ExternalLinks externalIds={family.external_ids} />
					</div>
				{/if}

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
						{#if rollbackOnMainlineOnly}
							<p class="rollback-mainline-only" role="note">{ROLLBACK_MAINLINE_ONLY}</p>
						{:else}
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
						{/if}
						<div class="history-content">
							{#if historyTab === 'history' || rollbackOnMainlineOnly}
								<ChangeHistory entityType="family" entityId={family.id} />
							{:else}
								<RestorePointBrowser
									entityType="family"
									entityId={family.id}
									currentVersion={family.version}
									onSelectVersion={handleSelectVersion}
								/>
							{/if}
						</div>
					{/if}
				</div>

				{#if !rollbackOnMainlineOnly}
					<RollbackConfirmDialog
						open={rollbackDialog.open}
						entityType="family"
						entityId={family.id}
						entityName={getPartnerDisplay()}
						currentVersion={family.version}
						targetVersion={rollbackDialog.targetVersion}
						targetSummary={rollbackDialog.targetSummary}
						onConfirm={handleRollbackConfirm}
						onCancel={handleRollbackCancel}
					/>
				{/if}

				<AddChildDialog
					bind:open={addChildOpen}
					familyId={family.id}
					familyName={getPartnerDisplay()}
					excludeIds={linkedPersonIds}
					onAdded={handleChildAdded}
				/>

				<AlertDialog.Root
					open={removeTarget !== null}
					onOpenChange={(isOpen) => {
						if (!isOpen && !removing) removeTarget = null;
					}}
				>
					<AlertDialog.Content>
						<AlertDialog.Header>
							<AlertDialog.Title>Remove this child from the family?</AlertDialog.Title>
							<AlertDialog.Description>
								{removeTarget ? childName(removeTarget) : ''} will no longer be recorded as a child of
								{getPartnerDisplay()}. The person is kept; only the link is removed, and it stays in
								the family's history.
							</AlertDialog.Description>
						</AlertDialog.Header>

						{#if removeError}
							<div class="dialog-error" role="alert">{removeError}</div>
						{/if}

						<AlertDialog.Footer>
							<AlertDialog.Cancel disabled={removing}>Cancel</AlertDialog.Cancel>
							<AlertDialog.Action variant="destructive" disabled={removing} onclick={confirmRemoveChild}>
								{removing ? 'Removing...' : 'Remove child'}
							</AlertDialog.Action>
						</AlertDialog.Footer>
					</AlertDialog.Content>
				</AlertDialog.Root>
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

	.rollback-mainline-only {
		margin: 0 0 0.75rem;
		font-size: 0.8125rem;
		font-style: italic;
		color: #64748b;
		line-height: 1.5;
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
		color: #475569;
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

	.section-header {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 1rem;
		margin-bottom: 0.75rem;
	}

	.info-section .section-header h2 {
		margin: 0;
	}

	.section-header h2:focus {
		outline: none;
	}

	.section-header h2:focus-visible {
		outline: 2px solid #3b82f6;
		outline-offset: 2px;
	}

	.remove-child {
		float: right;
		padding: 0.125rem 0.5rem;
		border: 1px solid transparent;
		border-radius: 6px;
		background: none;
		color: #b91c1c;
		font-size: 0.8125rem;
		cursor: pointer;
	}

	.remove-child:hover {
		background: #fef2f2;
		border-color: #fecaca;
	}

	.remove-child:focus-visible {
		outline: 2px solid #3b82f6;
		outline-offset: 1px;
	}

	.dialog-error {
		padding: 0.625rem 0.75rem;
		color: #dc2626;
		background: #fef2f2;
		border: 1px solid #fecaca;
		border-radius: 6px;
		font-size: 0.875rem;
	}

	.sr-only {
		position: absolute;
		width: 1px;
		height: 1px;
		padding: 0;
		margin: -1px;
		overflow: hidden;
		clip: rect(0, 0, 0, 0);
		white-space: nowrap;
		border: 0;
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
</style>
