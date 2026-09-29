<script lang="ts" module>
	import type { AddChild } from '$lib/api/client';

	export type ChildRelationship = NonNullable<AddChild['relationship_type']>;

	/** The `AddChild.relationship_type` enum in openapi.yaml, with its labels. */
	export const CHILD_RELATIONSHIPS: ReadonlyArray<{ value: ChildRelationship; label: string }> = [
		{ value: 'biological', label: 'Birth (biological)' },
		{ value: 'adopted', label: 'Adopted' },
		{ value: 'foster', label: 'Foster' }
	];
</script>

<script lang="ts">
	import { api, formatPersonName, type FamilyChild, type PersonSummary } from '$lib/api/client';
	import { Button } from '$lib/components/ui/button';
	import * as Dialog from '$lib/components/ui/dialog';
	import PersonSelector from './PersonSelector.svelte';

	interface Props {
		open: boolean;
		familyId: string;
		/** How the family is named in the dialog, e.g. "John Smith & Ann Jones". */
		familyName: string;
		/** People who cannot be this family's child: the partners and current children. */
		excludeIds?: readonly string[];
		onAdded: (child: FamilyChild, person: PersonSummary) => void;
	}

	let { open = $bindable(), familyId, familyName, excludeIds = [], onAdded }: Props = $props();

	let person: PersonSummary | null = $state(null);
	let relationship: ChildRelationship = $state('biological');
	let saving = $state(false);
	let error: string | null = $state(null);

	// Every opening starts from a clean form.
	$effect(() => {
		if (open) {
			person = null;
			relationship = 'biological';
			error = null;
		}
	});

	async function handleSubmit(e: Event) {
		e.preventDefault();
		if (!person || saving) return;
		saving = true;
		error = null;
		try {
			const child = await api.addChildToFamily(familyId, {
				person_id: person.id,
				relationship_type: relationship
			});
			const added = person;
			open = false;
			onAdded(child, added);
		} catch (err) {
			error = (err as { message?: string }).message || 'Failed to add the child';
		} finally {
			saving = false;
		}
	}
</script>

<Dialog.Root bind:open>
	<Dialog.Content class="sm:max-w-lg">
		<Dialog.Header>
			<Dialog.Title>Add child</Dialog.Title>
			<Dialog.Description>
				Link an existing person as a child of {familyName}.
			</Dialog.Description>
		</Dialog.Header>

		<form onsubmit={handleSubmit} class="add-child-form">
			<PersonSelector
				label="Child"
				selectedPerson={person}
				onSelect={(picked) => (person = picked)}
				{excludeIds}
				disabled={saving}
			/>

			<label class="field">
				Relationship to the parents
				<select bind:value={relationship} disabled={saving}>
					{#each CHILD_RELATIONSHIPS as option (option.value)}
						<option value={option.value}>{option.label}</option>
					{/each}
				</select>
			</label>

			{#if error}
				<div class="dialog-error" role="alert">{error}</div>
			{/if}

			<Dialog.Footer>
				<Button type="button" variant="secondary" disabled={saving} onclick={() => (open = false)}>
					Cancel
				</Button>
				<Button type="submit" disabled={saving || !person}>
					{saving ? 'Adding...' : person ? `Add ${formatPersonName(person)}` : 'Add child'}
				</Button>
			</Dialog.Footer>
		</form>
	</Dialog.Content>
</Dialog.Root>

<style>
	.add-child-form {
		display: flex;
		flex-direction: column;
		gap: 1rem;
	}

	.field {
		display: flex;
		flex-direction: column;
		gap: 0.375rem;
		font-size: 0.875rem;
		font-weight: 600;
		color: #374151;
	}

	.field select {
		padding: 0.625rem 0.75rem;
		border: 1px solid #d1d5db;
		border-radius: 6px;
		font-size: 0.875rem;
		font-weight: 400;
		background: white;
	}

	.field select:focus {
		outline: none;
		border-color: #3b82f6;
		box-shadow: 0 0 0 3px rgba(59, 130, 246, 0.1);
	}

	.dialog-error {
		padding: 0.625rem 0.75rem;
		color: #dc2626;
		background: #fef2f2;
		border: 1px solid #fecaca;
		border-radius: 6px;
		font-size: 0.875rem;
	}
</style>
