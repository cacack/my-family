import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import BranchResearchSummary from './BranchResearchSummary.svelte';
import BranchOutcomeBadge from './BranchOutcomeBadge.svelte';
import type { Branch } from '$lib/api/client';

const PERSON_ID = '99999999-9999-9999-9999-999999999999';
const FAMILY_ID = '88888888-8888-8888-8888-888888888888';
const GONE_ID = '77777777-7777-7777-7777-777777777777';
const PROOF_ID = '66666666-6666-6666-6666-666666666666';
const LOST_PROOF_ID = '55555555-5555-5555-5555-555555555555';

const base: Branch = {
	id: '11111111-1111-1111-1111-111111111111',
	name: 'Maternal Smith line',
	base_position: 42,
	status: 'active',
	outcome: 'open',
	subjects: [],
	proof_summary_ids: [],
	created_at: '2026-01-15T10:30:00Z'
};

describe('BranchResearchSummary', () => {
	it('shows the question, the verdict, and links every subject and proof summary', () => {
		render(BranchResearchSummary, {
			branch: {
				...base,
				hypothesis: 'Was Mary the daughter of John?',
				outcome: 'proved',
				subjects: [
					{ type: 'person', id: PERSON_ID, name: 'Mary Smith' },
					{ type: 'family', id: FAMILY_ID, name: 'John Smith & Ann Doe' }
				],
				proof_summary_ids: [PROOF_ID],
				proof_summaries: [{ id: PROOF_ID, fact_type: 'person_birth', conclusion: 'Born 1842' }]
			}
		});

		expect(screen.getByTestId('branch-hypothesis').textContent).toBe('Was Mary the daughter of John?');
		expect(screen.getByTestId('branch-outcome').textContent).toContain('Proved');
		expect(screen.getByRole('link', { name: 'Mary Smith' }).getAttribute('href')).toBe(`/persons/${PERSON_ID}`);
		expect(screen.getByRole('link', { name: 'John Smith & Ann Doe' }).getAttribute('href')).toBe(
			`/families/${FAMILY_ID}`
		);
		expect(screen.getByRole('link', { name: 'Born 1842' }).getAttribute('href')).toBe(
			`/evidence/proof-summaries/${PROOF_ID}`
		);
		expect(screen.getByText('person birth')).toBeDefined();
	});

	it('says what is missing rather than hiding it', () => {
		render(BranchResearchSummary, {
			branch: {
				...base,
				subjects: [{ type: 'person', id: GONE_ID }],
				proof_summary_ids: [LOST_PROOF_ID],
				proof_summaries: []
			}
		});

		expect(screen.getByText('No research question recorded yet.')).toBeDefined();
		expect(screen.getByRole('link', { name: 'Unnamed person' }).getAttribute('href')).toBe(`/persons/${GONE_ID}`);
		expect(screen.getByText('Proof summary no longer available')).toBeDefined();
	});

	it('links proof summaries by id when the record was not resolved (list reads)', () => {
		render(BranchResearchSummary, { branch: { ...base, proof_summary_ids: [PROOF_ID] } });
		expect(screen.getByRole('link', { name: 'View proof summary' }).getAttribute('href')).toBe(
			`/evidence/proof-summaries/${PROOF_ID}`
		);
		expect(screen.getAllByText('None linked')).toHaveLength(1);
	});
});

describe('BranchOutcomeBadge', () => {
	it.each([
		['open', 'Open'],
		['proved', 'Proved'],
		['disproved', 'Disproved'],
		['inconclusive', 'Inconclusive'],
		['superseded', 'Superseded']
	] as const)('labels %s as %s', (outcome, label) => {
		render(BranchOutcomeBadge, { outcome });
		const badge = screen.getByTestId('branch-outcome');
		expect(badge.textContent).toContain(label);
		expect(badge.getAttribute('data-outcome')).toBe(outcome);
	});

	it('notes when its links open in another scope, but only when there are links', () => {
		const note = 'These links open in the mainline. Switch to this branch to see them as it has them.';
		const { unmount } = render(BranchResearchSummary, {
			branch: { ...base, subjects: [{ type: 'person', id: PERSON_ID, name: 'Mary Smith' }] },
			scopeNote: note
		});
		expect(screen.getByTestId('branch-research-scope-note').textContent).toBe(note);
		unmount();

		render(BranchResearchSummary, { branch: base, scopeNote: note });
		expect(screen.queryByTestId('branch-research-scope-note')).toBeNull();
	});

	it('reads a missing outcome as open', () => {
		render(BranchOutcomeBadge, { outcome: undefined });
		expect(screen.getByTestId('branch-outcome').textContent).toContain('Open');
	});
});
