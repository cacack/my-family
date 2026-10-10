<script lang="ts">
	import { page } from '$app/stores';
	import { goto } from '$app/navigation';
	import {
		api,
		formatPersonName,
		type FamilyCreate,
		type PersonSummary
	} from '$lib/api/client';
	import { Button } from '$lib/components/ui/button';
	import PageHeader from '$lib/components/PageHeader.svelte';
	import FormRow from '$lib/components/FormRow.svelte';
	import PartnerPickers from '$lib/components/PartnerPickers.svelte';

	let saving = $state(false);
	let error: string | null = $state(null);

	// Form state
	let formData = $state<FamilyCreate>({
		relationship_type: 'unknown',
		marriage_date: '',
		marriage_place: ''
	});
	let partner1: PersonSummary | null = $state(null);
	let partner2: PersonSummary | null = $state(null);
	// Set when the family was created but linking the prefilled child failed.
	let createdFamilyId: string | null = $state(null);

	/**
	 * The person-page shortcuts open this form prefilled: `?partner1=<id>`
	 * ("Add family") puts the person in the first partner slot, and
	 * `?child=<id>` ("Add parents") links them as the new family's child.
	 */
	let child: PersonSummary | null = $state(null);
	let prefillError: string | null = $state(null);
	let prefilledFor = '';
	// True while the prefilled people are loading. Submitting then would create
	// the family without the child the "Add parents" shortcut was opened for.
	let prefilling = $state(false);
	// The `?child=` id asked for; while it is set but not loaded the form
	// cannot be submitted (a failed load must not create a childless family).
	let requestedChildId: string | null = $state(null);
	let prefillToken = 0;
	const childPending = $derived.by(() => !!requestedChildId && child?.id !== requestedChildId);
	const formLocked = $derived(saving || !!createdFamilyId || prefilling);
	// A family needs at least one partner; the server refuses one without.
	const hasPartner = $derived(!!partner1 || !!partner2);

	async function prefill(partner1Id: string | null, childId: string | null) {
		const token = ++prefillToken;
		prefillError = null;
		prefilling = true;
		try {
			const [p1, c] = await Promise.all([
				partner1Id ? api.getPerson(partner1Id) : Promise.resolve(null),
				childId ? api.getPerson(childId) : Promise.resolve(null)
			]);
			if (token !== prefillToken) return;
			if (p1) partner1 = p1;
			child = c ?? null;
			if (childId && !c) prefillError = 'The person to add as a child could not be found.';
		} catch (e) {
			if (token !== prefillToken) return;
			prefillError = (e as { message?: string }).message || 'Failed to load the person';
		} finally {
			if (token === prefillToken) prefilling = false;
		}
	}

	$effect(() => {
		const params = $page.url?.searchParams;
		const partner1Id = params?.get('partner1') ?? null;
		const childId = params?.get('child') ?? null;
		const key = `${partner1Id}|${childId}`;
		if (key === prefilledFor) return;
		prefilledFor = key;
		requestedChildId = childId;
		if (!childId) child = null;
		if (partner1Id || childId) prefill(partner1Id, childId);
	});

	async function handleSubmit() {
		if (formLocked || childPending || !hasPartner) return;
		saving = true;
		error = null;
		try {
			const payload: FamilyCreate = {
				partner1_id: partner1?.id,
				partner2_id: partner2?.id,
				relationship_type: formData.relationship_type || undefined,
				marriage_date: formData.marriage_date || undefined,
				marriage_place: formData.marriage_place || undefined
			};
			const family = await api.createFamily(payload);
			if (child) {
				try {
					await api.addChildToFamily(family.id, { person_id: child.id });
				} catch (e) {
					// The family exists; say so, rather than implying nothing was saved.
					error = `The family was created, but ${formatPersonName(child)} could not be added as its child: ${
						(e as { message?: string }).message || 'unknown error'
					}. Open the family to try again.`;
					createdFamilyId = family.id;
					return;
				}
			}
			goto(`/families/${family.id}`);
		} catch (e) {
			error = (e as { message?: string }).message || 'Failed to create family';
		} finally {
			saving = false;
		}
	}

	function handleCancel() {
		goto(child ? `/persons/${child.id}` : '/families');
	}
</script>

<svelte:head>
	<title>Add Family | My Family</title>
</svelte:head>

<div class="add-family-page">
	<PageHeader title="Add Family" back={{ href: '/families', label: 'Families' }} />

	{#if error}
		<div class="error" role="alert">
			{error}
			{#if createdFamilyId}
				<a href="/families/{createdFamilyId}">Open the family</a>
			{/if}
		</div>
	{/if}
	{#if prefillError}
		<div class="error" role="alert">
			{prefillError}
			{#if childPending}
				The family cannot be created until the child loads; reload the page to try again.
			{/if}
		</div>
	{/if}

	<form class="edit-form" onsubmit={(e) => { e.preventDefault(); handleSubmit(); }}>
		{#if child}
			<p class="child-note" data-testid="child-note">
				{formatPersonName(child)} will be added as a child of this family.
			</p>
		{/if}

		<PartnerPickers
			bind:partner1
			bind:partner2
			excludeIds={child ? [child.id] : []}
			disabled={formLocked}
		/>

		<FormRow>
			<label>
				Relationship Type
				<select bind:value={formData.relationship_type}>
					<option value="unknown">Unknown</option>
					<option value="marriage">Marriage</option>
					<option value="partnership">Partnership</option>
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
				<input type="text" bind:value={formData.marriage_place} placeholder="e.g., London, England" />
			</label>
		</FormRow>

		{#if !requestedChildId}
			<p class="helper-text">
				Children are added from the family's page once it is created.
			</p>
		{/if}

		<div class="form-actions">
			<Button type="button" variant="outline" onclick={handleCancel} disabled={saving}>Cancel</Button>
			<Button type="submit" disabled={formLocked || childPending || !hasPartner}>
				{saving ? 'Creating...' : prefilling ? 'Loading...' : 'Create Family'}
			</Button>
		</div>
	</form>
</div>

<style>
	.add-family-page {
		max-width: 800px;
		margin: 0 auto;
		padding: 1.5rem;
	}

	.error {
		text-align: center;
		padding: 1rem;
		color: #dc2626;
		background: #fef2f2;
		border: 1px solid #fecaca;
		border-radius: 6px;
		margin-bottom: 1rem;
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

	.child-note {
		margin: 0 0 1rem;
		padding: 0.625rem 0.75rem;
		font-size: 0.875rem;
		color: #1e3a8a;
		background: #eff6ff;
		border: 1px solid #bfdbfe;
		border-radius: 6px;
	}

	.error a {
		margin-left: 0.25rem;
		color: inherit;
		font-weight: 600;
	}

	.helper-text {
		font-size: 0.8125rem;
		color: #64748b;
		margin: 0.5rem 0 0;
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
