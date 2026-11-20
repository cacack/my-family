<script lang="ts">
	/**
	 * The privacy part of a record's edit form (ADR-008): mark the record
	 * private and, for a person, override the living rule. `living` is the
	 * rule's current verdict, shown so the editor knows what "decided by the
	 * living rule" means for this person.
	 */
	import type { LivingOverride } from '#lib/api/client.js';
	import { LIVING_OVERRIDE_OPTIONS } from '#lib/utils/enumOptions.js';

	interface Props {
		/** What the record is, for the labels: "person", "family". */
		noun: string;
		private: boolean;
		/** Set for a person only; the living status control is shown with it. */
		livingOverride?: LivingOverride;
		living?: boolean;
	}

	let {
		noun,
		private: isPrivate = $bindable(),
		livingOverride = $bindable(),
		living
	}: Props = $props();

	const id = $props.id();
</script>

<fieldset class="privacy-fields">
	<legend>Privacy</legend>
	<div class="field">
		<label class="check">
			<input type="checkbox" bind:checked={isPrivate} aria-describedby="{id}-private-hint" />
			Private {noun}
		</label>
		<p id="{id}-private-hint" class="hint">
			Exported to GEDCOM as a restriction notice. Viewers and visitors do not see a private {noun};
			editors and the owner do.
		</p>
	</div>
	{#if livingOverride !== undefined}
		<div class="field">
			<label for="{id}-living">Living status</label>
			<select id="{id}-living" bind:value={livingOverride} aria-describedby="{id}-living-hint">
				{#each LIVING_OVERRIDE_OPTIONS as option (option.value)}
					<option value={option.value}>{option.label}</option>
				{/each}
			</select>
			<p id="{id}-living-hint" class="hint">
				{#if living !== undefined}
					The rule currently counts this {noun} as {living ? 'living' : 'deceased'}.
				{/if}
				Visitors see a living person as "Living", with no details, and so do viewers when the tree's
				privacy settings say so.
			</p>
		</div>
	{/if}
</fieldset>

<style>
	.privacy-fields {
		display: flex;
		flex-direction: column;
		gap: 0.75rem;
		min-width: 0;
		margin: 0;
		padding: 0.75rem 1rem 1rem;
		border: 1px solid #e2e8f0;
		border-radius: 8px;
	}

	:global(body.high-contrast) .privacy-fields {
		border-color: var(--a11y-color-border);
	}

	legend {
		padding: 0 0.25rem;
		font-size: 0.875rem;
		font-weight: 600;
	}

	.field {
		display: flex;
		flex-direction: column;
		gap: 0.375rem;
		min-width: 0;
	}

	.check {
		display: flex;
		align-items: center;
		gap: 0.5rem;
	}

	.check input {
		width: 1rem;
		height: 1rem;
		flex-shrink: 0;
	}

	label {
		font-size: 0.875rem;
		color: #475569;
	}

	select {
		max-width: 100%;
		padding: 0.625rem 0.75rem;
		border: 1px solid #e2e8f0;
		border-radius: 6px;
		background: white;
		font-size: 0.875rem;
	}

	select:focus-visible,
	.check input:focus-visible {
		outline: 2px solid var(--color-focus-ring);
		outline-offset: 2px;
	}

	:global(body.high-contrast) label {
		color: var(--color-text);
	}

	.hint {
		margin: 0;
		font-size: 0.8125rem;
		color: var(--color-text-muted);
	}
</style>
