<script lang="ts" module>
	import type { FamilyUpdate, PersonSummary } from '$lib/api/client';

	type PartnerFields = Pick<
		FamilyUpdate,
		'partner1_id' | 'partner2_id' | 'clear_partner1' | 'clear_partner2'
	>;

	/**
	 * The partner part of a family update: only what changed. A partner picked
	 * sends its id, a partner removed sends `clear_partnerN`, and an untouched
	 * slot sends nothing, so an edit of the marriage date alone never rewrites
	 * the partners.
	 */
	export function partnerChanges(
		current: { partner1_id?: string; partner2_id?: string },
		partner1: PersonSummary | null,
		partner2: PersonSummary | null
	): PartnerFields {
		const changes: PartnerFields = {};
		const next1 = partner1?.id;
		const next2 = partner2?.id;
		if (next1 !== current.partner1_id) {
			if (next1) changes.partner1_id = next1;
			else changes.clear_partner1 = true;
		}
		if (next2 !== current.partner2_id) {
			if (next2) changes.partner2_id = next2;
			else changes.clear_partner2 = true;
		}
		return changes;
	}
</script>

<script lang="ts">
	import PersonSelector from './PersonSelector.svelte';

	interface Props {
		partner1?: PersonSummary | null;
		partner2?: PersonSummary | null;
		/** People who can be neither partner, such as the family's children. */
		excludeIds?: readonly string[];
		disabled?: boolean;
	}

	let {
		partner1 = $bindable(null),
		partner2 = $bindable(null),
		excludeIds = [],
		disabled = false
	}: Props = $props();

	// A person cannot partner themselves, so each picker hides the other's pick.
	const exclude1 = $derived([...excludeIds, ...(partner2 ? [partner2.id] : [])]);
	const exclude2 = $derived([...excludeIds, ...(partner1 ? [partner1.id] : [])]);
</script>

<fieldset class="partner-pickers" {disabled}>
	<legend>Partners</legend>
	<p class="hint">Search by name. Both are optional, and either can be changed or removed later.</p>
	<div class="pickers">
		<PersonSelector
			label="Partner 1"
			selectedPerson={partner1}
			onSelect={(person) => (partner1 = person)}
			excludeIds={exclude1}
			{disabled}
		/>
		<PersonSelector
			label="Partner 2"
			selectedPerson={partner2}
			onSelect={(person) => (partner2 = person)}
			excludeIds={exclude2}
			{disabled}
		/>
	</div>
</fieldset>

<style>
	.partner-pickers {
		border: none;
		margin: 0 0 1rem;
		padding: 0;
		min-width: 0;
	}

	legend {
		font-size: 0.875rem;
		font-weight: 600;
		color: #1e293b;
		padding: 0;
		margin-bottom: 0.25rem;
	}

	.hint {
		font-size: 0.8125rem;
		color: #64748b;
		margin: 0 0 0.75rem;
	}

	.pickers {
		display: grid;
		grid-template-columns: repeat(2, minmax(0, 1fr));
		gap: 1rem;
	}

	@media (max-width: 640px) {
		.pickers {
			grid-template-columns: 1fr;
		}
	}
</style>
