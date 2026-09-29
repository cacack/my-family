<script lang="ts">
	/**
	 * Edit a branch's description and research record (#835) through
	 * `PATCH /branches/{id}`.
	 *
	 * Only the fields the user actually changed are sent, and the server refuses
	 * an edit that raced another write to the branch (409 `branch_changed`), so
	 * a save never silently overwrites a concurrent change. What is
	 * editable follows the server's rule: an active branch takes every field; a
	 * merged one only its outcome (the verdict is often recorded once the merge
	 * has landed). Archived branches never reach this form.
	 *
	 * Subjects are picked by searching people. Search reads the mainline, so a
	 * picked person is then read as THIS branch sees them (an explicit
	 * `?branch=`, whichever branch is active): one the branch deleted is
	 * refused, and the families offered are the branch's. People and families
	 * that exist only on the branch cannot be found by search; the persons and
	 * families the branch changed are offered directly instead (`candidates`).
	 * Proof summaries are likewise picked from the ones this branch can see.
	 */
	import {
		api,
		formatPersonName,
		type ApiError,
		type Branch,
		type BranchOutcome,
		type BranchSubject,
		type BranchUpdate,
		type FamilySummary,
		type ProofSummaryResponse,
		type SearchResult
	} from '$lib/api/client';
	import PersonSelector from '$lib/components/PersonSelector.svelte';
	import { Button } from '$lib/components/ui/button';
	import { Label } from '$lib/components/ui/label';
	import { Textarea } from '$lib/components/ui/textarea';
	import {
		BRANCH_OUTCOMES,
		HYPOTHESIS_MAX_LENGTH,
		MAX_PROOF_SUMMARIES,
		MAX_SUBJECTS,
		OUTCOME_LABELS,
		branchOutcome,
		subjectLabel
	} from '$lib/utils/branchResearch';

	// Mirrors the maxLength on BranchUpdate.description in openapi.yaml.
	const DESCRIPTION_MAX_LENGTH = 500;

	interface Props {
		branch: Branch;
		/** Persons and families the branch changed, offered as subjects. */
		candidates?: BranchSubject[];
		onsaved: (branch: Branch) => void;
		oncancel: () => void;
	}

	let { branch, candidates = [], onsaved, oncancel }: Props = $props();

	// The form edits a snapshot of the branch taken when it opens; the parent
	// remounts it for a fresh edit.
	// svelte-ignore state_referenced_locally
	const initial = branch;
	const outcomeOnly = initial.status !== 'active';

	let description = $state(initial.description ?? '');
	let hypothesis = $state(initial.hypothesis ?? '');
	let outcome: BranchOutcome = $state(branchOutcome(initial.outcome));
	let subjects: BranchSubject[] = $state((initial.subjects ?? []).map((s) => ({ ...s })));
	let proofIds: string[] = $state([...(initial.proof_summary_ids ?? [])]);

	let saving = $state(false);
	let saveError: string | null = $state(null);

	/** Families of the last person picked, offered as subjects. */
	let pickedPerson: { id: string; name: string } | null = $state(null);
	let pickedFamilies: FamilySummary[] = $state([]);
	let subjectNotice: string | null = $state(null);

	const offeredCandidates = $derived(candidates.filter((c) => !hasSubject(c.type, c.id)));

	let availableProofs: ProofSummaryResponse[] = $state([]);
	let proofsLoading = $state(false);
	let proofsError: string | null = $state(null);

	const hypothesisValid = $derived(hypothesis.trim().length <= HYPOTHESIS_MAX_LENGTH);
	const descriptionValid = $derived(description.trim().length <= DESCRIPTION_MAX_LENGTH);

	/** Proof summaries to offer: the branch's own, plus any linked one the list lacks. */
	const proofOptions = $derived.by(() => {
		const options = availableProofs.map((p) => ({
			id: p.id,
			label: p.conclusion,
			factType: p.fact_type
		}));
		const known = new Set(options.map((o) => o.id));
		for (const ref of initial.proof_summaries ?? []) {
			if (!known.has(ref.id)) {
				options.push({ id: ref.id, label: ref.conclusion, factType: ref.fact_type });
				known.add(ref.id);
			}
		}
		for (const id of proofIds) {
			if (!known.has(id)) {
				options.push({ id, label: 'Proof summary no longer available', factType: '' });
				known.add(id);
			}
		}
		return options;
	});

	$effect(() => {
		if (outcomeOnly) return;
		loadProofSummaries();
	});

	async function loadProofSummaries() {
		proofsLoading = true;
		proofsError = null;
		try {
			const result = await api.listProofSummaries({ branch: initial.id, limit: 100 });
			availableProofs = result.summaries ?? [];
		} catch (e) {
			proofsError = (e as ApiError).message || 'Failed to load proof summaries';
		} finally {
			proofsLoading = false;
		}
	}

	function hasSubject(type: BranchSubject['type'], id: string): boolean {
		return subjects.some((s) => s.type === type && s.id === id);
	}

	function addSubject(subject: BranchSubject) {
		if (hasSubject(subject.type, subject.id) || subjects.length >= MAX_SUBJECTS) return;
		subjects = [...subjects, subject];
	}

	function removeSubject(subject: BranchSubject) {
		subjects = subjects.filter((s) => !(s.type === subject.type && s.id === subject.id));
	}

	async function handlePersonPicked(person: SearchResult | null) {
		if (!person) return;
		const name = formatPersonName(person);
		pickedPerson = { id: person.id, name };
		pickedFamilies = [];
		subjectNotice = null;
		try {
			// Read as the edited branch sees the person, not the active scope.
			const detail = await api.getPerson(person.id, { branch: initial.id });
			if (pickedPerson?.id !== person.id) return;
			addSubject({ type: 'person', id: person.id, name });
			const families = [...(detail.families_as_partner ?? [])];
			if (detail.family_as_child) families.push(detail.family_as_child);
			pickedFamilies = families;
		} catch (e) {
			if (pickedPerson?.id !== person.id) return;
			if ((e as ApiError)?.status === 404) {
				// Deleted on this branch: the server would refuse the save.
				subjectNotice = `${name} is not on this branch, so cannot be a subject of it.`;
				pickedPerson = null;
				return;
			}
			// Any other failure only costs the family shortcut; the server still
			// checks the person when the record is saved.
			addSubject({ type: 'person', id: person.id, name });
		}
	}

	function familyName(family: FamilySummary): string {
		const names = [family.partner1_name, family.partner2_name].filter(Boolean);
		return names.length > 0 ? names.join(' & ') : 'Unnamed family';
	}

	function toggleProof(id: string) {
		if (proofIds.includes(id)) {
			proofIds = proofIds.filter((p) => p !== id);
		} else if (proofIds.length < MAX_PROOF_SUMMARIES) {
			proofIds = [...proofIds, id];
		}
	}

	function sameSubjects(a: BranchSubject[], b: BranchSubject[]): boolean {
		return a.length === b.length && a.every((s, i) => s.type === b[i].type && s.id === b[i].id);
	}

	function sameIds(a: string[], b: string[]): boolean {
		return a.length === b.length && a.every((id, i) => id === b[i]);
	}

	/** Only what changed goes on the wire. */
	const patch: BranchUpdate = $derived.by(() => {
		const out: BranchUpdate = {};
		if (outcome !== branchOutcome(initial.outcome)) out.outcome = outcome;
		if (outcomeOnly) return out;
		if (description.trim() !== (initial.description ?? '')) out.description = description.trim();
		if (hypothesis.trim() !== (initial.hypothesis ?? '')) out.hypothesis = hypothesis.trim();
		if (!sameSubjects(subjects, initial.subjects ?? [])) {
			out.subjects = subjects.map(({ type, id }) => ({ type, id }));
		}
		if (!sameIds(proofIds, initial.proof_summary_ids ?? [])) out.proof_summary_ids = proofIds;
		return out;
	});

	const dirty = $derived(Object.keys(patch).length > 0);

	async function handleSubmit(event: Event) {
		event.preventDefault();
		if (saving || !dirty || !hypothesisValid || !descriptionValid) return;
		saving = true;
		saveError = null;
		try {
			const saved = await api.updateBranch(initial.id, patch);
			onsaved(saved);
		} catch (e) {
			const apiError = e as ApiError;
			if (apiError.code === 'branch_field_locked') {
				saveError = 'This branch has been merged, so only its outcome can still change.';
			} else if (apiError.code === 'branch_changed') {
				saveError =
					'This branch was changed elsewhere while you were editing. Nothing was saved - cancel and reopen the editor to see its current record.';
			} else if (apiError.code === 'branch_not_active') {
				saveError = 'This branch has been archived and accepts no further changes.';
			} else {
				saveError = apiError.message || 'Failed to save the research record';
			}
		} finally {
			saving = false;
		}
	}
</script>

<form class="editor" onsubmit={handleSubmit} aria-label="Edit research record" data-testid="branch-research-editor">
	{#if outcomeOnly}
		<p class="locked-note">
			This branch is {initial.status}. Its question, subjects and evidence stay as they were
			merged; you can still record its outcome.
		</p>
	{/if}

	<div class="field">
		<Label for="branch-outcome">Outcome</Label>
		<select id="branch-outcome" class="native-select" bind:value={outcome} disabled={saving}>
			{#each BRANCH_OUTCOMES as option (option)}
				<option value={option}>{OUTCOME_LABELS[option]}</option>
			{/each}
		</select>
	</div>

	{#if !outcomeOnly}
		<div class="field">
			<Label for="branch-hypothesis">Research question</Label>
			<Textarea
				id="branch-hypothesis"
				bind:value={hypothesis}
				maxlength={HYPOTHESIS_MAX_LENGTH}
				rows={3}
				placeholder="Was Mary Smith (b. 1842) the daughter of John Smith of Albany?"
				disabled={saving}
			/>
			<span class="field-hint">{hypothesis.length}/{HYPOTHESIS_MAX_LENGTH}</span>
		</div>

		<div class="field">
			<Label for="branch-edit-description">Description</Label>
			<Textarea
				id="branch-edit-description"
				bind:value={description}
				maxlength={DESCRIPTION_MAX_LENGTH}
				rows={2}
				disabled={saving}
			/>
			<span class="field-hint">{description.length}/{DESCRIPTION_MAX_LENGTH}</span>
		</div>

		<fieldset class="field">
			<legend>Subjects</legend>
			{#if subjects.length === 0}
				<p class="hint">No subjects yet. Search for a person to add them.</p>
			{:else}
				<ul class="subject-list">
					{#each subjects as subject (subject.type + subject.id)}
						<li>
							<span class="chip-type">{subject.type}</span>
							<span class="subject-name">{subjectLabel(subject)}</span>
							<button
								type="button"
								class="remove"
								onclick={() => removeSubject(subject)}
								aria-label="Remove {subject.type} {subjectLabel(subject)}"
								disabled={saving}
							>
								Remove
							</button>
						</li>
					{/each}
				</ul>
			{/if}
			{#if subjects.length < MAX_SUBJECTS && offeredCandidates.length > 0}
				<div class="family-offers" data-testid="branch-subject-candidates">
					<span class="hint">Changed on this branch:</span>
					{#each offeredCandidates as candidate (candidate.type + candidate.id)}
						<Button
							type="button"
							variant="outline"
							size="sm"
							disabled={saving}
							onclick={() => addSubject({ ...candidate })}
						>
							Add {candidate.type}: {subjectLabel(candidate)}
						</Button>
					{/each}
				</div>
			{/if}
			{#if subjects.length < MAX_SUBJECTS}
				<PersonSelector
					label="Add a person"
					onSelect={handlePersonPicked}
					disabled={saving}
					placeholder="Search for a person..."
				/>
				<p class="hint">
					Search covers the main tree. People and families created on this branch are offered
					under "Changed on this branch".
				</p>
			{/if}
			{#if subjectNotice}
				<p class="error" role="alert">{subjectNotice}</p>
			{/if}
			{#if pickedPerson && pickedFamilies.length > 0}
				<div class="family-offers">
					<span class="hint">Families of {pickedPerson.name}:</span>
					{#each pickedFamilies as family (family.id)}
						<Button
							type="button"
							variant="outline"
							size="sm"
							disabled={saving || hasSubject('family', family.id)}
							onclick={() => addSubject({ type: 'family', id: family.id, name: familyName(family) })}
						>
							{hasSubject('family', family.id) ? 'Added' : 'Add family'}: {familyName(family)}
						</Button>
					{/each}
				</div>
			{/if}
		</fieldset>

		<fieldset class="field">
			<legend>Proof summaries</legend>
			{#if proofsLoading}
				<p class="hint" role="status">Loading proof summaries...</p>
			{:else if proofsError}
				<p class="error" role="alert">{proofsError}</p>
			{:else if proofOptions.length === 0}
				<p class="hint">No proof summaries on this branch yet.</p>
			{:else}
				<ul class="proof-options">
					{#each proofOptions as option (option.id)}
						<li>
							<label>
								<input
									type="checkbox"
									checked={proofIds.includes(option.id)}
									onchange={() => toggleProof(option.id)}
									disabled={saving ||
										(!proofIds.includes(option.id) && proofIds.length >= MAX_PROOF_SUMMARIES)}
								/>
								<span>{option.label}</span>
								{#if option.factType}
									<span class="chip-type">{option.factType.replace(/_/g, ' ')}</span>
								{/if}
							</label>
						</li>
					{/each}
				</ul>
			{/if}
		</fieldset>
	{/if}

	{#if saveError}
		<div class="error" role="alert">{saveError}</div>
	{/if}

	<div class="actions">
		<Button type="button" variant="secondary" onclick={oncancel} disabled={saving}>Cancel</Button>
		<Button type="submit" disabled={saving || !dirty || !hypothesisValid || !descriptionValid}>
			{saving ? 'Saving...' : 'Save research record'}
		</Button>
	</div>
</form>

<style>
	.editor {
		margin-top: 0.75rem;
		padding: 1rem;
		background: white;
		border: 1px solid #e2e8f0;
		border-radius: 8px;
		max-width: 46rem;
	}

	.locked-note {
		margin: 0 0 0.75rem;
		font-size: 0.8125rem;
		color: #64748b;
	}

	.field {
		display: flex;
		flex-direction: column;
		gap: 0.375rem;
		margin: 0 0 1rem;
		padding: 0;
		border: 0;
		min-width: 0;
	}

	legend {
		margin-bottom: 0.375rem;
		font-size: 0.875rem;
		font-weight: 500;
	}

	.field-hint {
		align-self: flex-end;
		font-size: 0.6875rem;
		color: #94a3b8;
	}

	.native-select {
		max-width: 16rem;
		height: 2.25rem;
		padding: 0 0.5rem;
		border: 1px solid #cbd5e1;
		border-radius: 6px;
		background: white;
		font-size: 0.875rem;
	}

	.hint {
		margin: 0;
		font-size: 0.8125rem;
		color: #94a3b8;
	}

	.subject-list,
	.proof-options {
		list-style: none;
		margin: 0 0 0.5rem;
		padding: 0;
		display: flex;
		flex-direction: column;
		gap: 0.375rem;
	}

	.subject-list li {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		font-size: 0.875rem;
	}

	.proof-options label {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		font-size: 0.875rem;
		cursor: pointer;
	}

	.chip-type {
		font-size: 0.6875rem;
		text-transform: capitalize;
		color: #94a3b8;
	}

	.remove {
		margin-left: auto;
		padding: 0.125rem 0.5rem;
		border: 1px solid #e2e8f0;
		border-radius: 4px;
		background: white;
		font-size: 0.75rem;
		color: #64748b;
		cursor: pointer;
	}

	.remove:hover:not(:disabled) {
		border-color: #fca5a5;
		color: #b91c1c;
	}

	.family-offers {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 0.375rem;
		margin-top: 0.5rem;
	}

	.error {
		margin-bottom: 0.75rem;
		padding: 0.625rem 0.75rem;
		background: hsl(var(--destructive) / 0.1);
		border: 1px solid hsl(var(--destructive) / 0.3);
		border-radius: 6px;
		color: hsl(var(--destructive));
		font-size: 0.8125rem;
	}

	.actions {
		display: flex;
		justify-content: flex-end;
		gap: 0.5rem;
	}
</style>
