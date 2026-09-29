import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/svelte';
import BranchResearchEditor from './BranchResearchEditor.svelte';
import type * as apiModule from '$lib/api/client';
import type { Branch, BranchSubject, BranchUpdate } from '$lib/api/client';

const { updateBranch, listProofSummaries, searchPersons, getPerson } = vi.hoisted(() => ({
	updateBranch: vi.fn(),
	listProofSummaries: vi.fn(),
	searchPersons: vi.fn(),
	getPerson: vi.fn()
}));

vi.mock('$lib/api/client', async (importOriginal) => {
	const actual = await importOriginal<typeof apiModule>();
	return {
		...actual,
		api: {
			updateBranch: (id: string, data: BranchUpdate) => updateBranch(id, data),
			listProofSummaries: (params: unknown) => listProofSummaries(params),
			searchPersons: (params: unknown) => searchPersons(params),
			getPerson: (id: string, options?: unknown) => getPerson(id, options)
		}
	};
});

const BRANCH_ID = '11111111-1111-1111-1111-111111111111';
const PERSON_ID = '99999999-9999-9999-9999-999999999999';
const FAMILY_ID = '88888888-8888-8888-8888-888888888888';
const PROOF_ID = '66666666-6666-6666-6666-666666666666';
const OTHER_PROOF_ID = '55555555-5555-5555-5555-555555555555';

const branch: Branch = {
	id: BRANCH_ID,
	name: 'Maternal Smith line',
	description: 'Census gap',
	base_position: 42,
	status: 'active',
	hypothesis: 'Was Mary the daughter of John?',
	outcome: 'open',
	subjects: [{ type: 'person', id: PERSON_ID, name: 'Mary Smith' }],
	proof_summary_ids: [PROOF_ID],
	proof_summaries: [{ id: PROOF_ID, fact_type: 'person_birth', conclusion: 'Born 1842' }],
	created_at: '2026-01-15T10:30:00Z'
};

function setup(overrides: Partial<Branch> = {}, candidates: BranchSubject[] = []) {
	const onsaved = vi.fn();
	const oncancel = vi.fn();
	render(BranchResearchEditor, { branch: { ...branch, ...overrides }, candidates, onsaved, oncancel });
	return { onsaved, oncancel };
}

describe('BranchResearchEditor', () => {
	beforeEach(() => {
		vi.clearAllMocks();
		listProofSummaries.mockResolvedValue({
			summaries: [
				{ id: PROOF_ID, fact_type: 'person_birth', subject_id: PERSON_ID, conclusion: 'Born 1842', argument: 'a', version: 1 },
				{ id: OTHER_PROOF_ID, fact_type: 'person_death', subject_id: PERSON_ID, conclusion: 'Died 1900', argument: 'b', version: 1 }
			],
			total: 2
		});
	});

	it("lists the branch's own proof summaries, read with an explicit branch scope", async () => {
		setup();
		await waitFor(() => expect(screen.getByText('Died 1900')).toBeDefined());
		expect(listProofSummaries).toHaveBeenCalledWith({ branch: BRANCH_ID, limit: 100 });
	});

	it('saves nothing until something changes, then sends only what changed', async () => {
		updateBranch.mockResolvedValue({ ...branch, outcome: 'proved' });
		const { onsaved } = setup();

		const save = screen.getByRole('button', { name: 'Save research record' });
		expect((save as HTMLButtonElement).disabled).toBe(true);

		await fireEvent.change(screen.getByLabelText('Outcome'), { target: { value: 'proved' } });
		expect((save as HTMLButtonElement).disabled).toBe(false);
		await fireEvent.click(save);

		await waitFor(() => expect(onsaved).toHaveBeenCalled());
		expect(updateBranch).toHaveBeenCalledWith(BRANCH_ID, { outcome: 'proved' });
	});

	it('edits the question, removes a subject and links a proof summary', async () => {
		updateBranch.mockResolvedValue(branch);
		setup();
		await waitFor(() => expect(screen.getByText('Died 1900')).toBeDefined());

		await fireEvent.input(screen.getByLabelText('Research question'), {
			target: { value: '  Was Mary the daughter of John Smith?  ' }
		});
		await fireEvent.click(screen.getByRole('button', { name: 'Remove person Mary Smith' }));
		await fireEvent.click(screen.getByRole('checkbox', { name: /Died 1900/ }));
		await fireEvent.click(screen.getByRole('button', { name: 'Save research record' }));

		await waitFor(() => expect(updateBranch).toHaveBeenCalled());
		expect(updateBranch).toHaveBeenCalledWith(BRANCH_ID, {
			hypothesis: 'Was Mary the daughter of John Smith?',
			subjects: [],
			proof_summary_ids: [PROOF_ID, OTHER_PROOF_ID]
		});
	});

	it("adds a searched person and offers that person's families", async () => {
		updateBranch.mockResolvedValue(branch);
		const NEW_ID = '44444444-4444-4444-4444-444444444444';
		searchPersons.mockResolvedValue({
			items: [{ id: NEW_ID, given_name: 'John', surname: 'Smith', score: 1 }],
			total: 1
		});
		getPerson.mockResolvedValue({
			id: NEW_ID,
			given_name: 'John',
			surname: 'Smith',
			version: 1,
			families_as_partner: [{ id: FAMILY_ID, partner1_name: 'John Smith', partner2_name: 'Ann Doe' }]
		});
		setup({ subjects: [] });

		await fireEvent.input(screen.getByLabelText('Add a person'), { target: { value: 'John' } });
		const option = await screen.findByText('John Smith', {}, { timeout: 2000 });
		await fireEvent.click(option);

		const addFamily = await screen.findByRole('button', { name: /Add family: John Smith & Ann Doe/ });
		// Read as the edited branch sees the person, not the active scope.
		expect(getPerson).toHaveBeenCalledWith(NEW_ID, { branch: BRANCH_ID });
		await fireEvent.click(addFamily);
		await fireEvent.click(screen.getByRole('button', { name: 'Save research record' }));

		await waitFor(() => expect(updateBranch).toHaveBeenCalled());
		expect(updateBranch).toHaveBeenCalledWith(BRANCH_ID, {
			subjects: [
				{ type: 'person', id: NEW_ID },
				{ type: 'family', id: FAMILY_ID }
			]
		});
	});

	it('refuses a searched person the edited branch does not have', async () => {
		const GONE_ID = '33333333-3333-3333-3333-333333333333';
		searchPersons.mockResolvedValue({
			items: [{ id: GONE_ID, given_name: 'Gone', surname: 'Person', score: 1 }],
			total: 1
		});
		getPerson.mockRejectedValue({ status: 404, code: 'not_found', message: 'Person not found' });
		setup({ subjects: [] });

		await fireEvent.input(screen.getByLabelText('Add a person'), { target: { value: 'Gone' } });
		await fireEvent.click(await screen.findByText('Gone Person', {}, { timeout: 2000 }));

		expect(await screen.findByText(/Gone Person is not on this branch/)).toBeDefined();
		expect(screen.getByText('No subjects yet. Search for a person to add them.')).toBeDefined();
		expect((screen.getByRole('button', { name: 'Save research record' }) as HTMLButtonElement).disabled).toBe(true);
	});

	it('still adds a searched person when only the family lookup fails', async () => {
		updateBranch.mockResolvedValue(branch);
		const NEW_ID = '44444444-4444-4444-4444-444444444444';
		searchPersons.mockResolvedValue({
			items: [{ id: NEW_ID, given_name: 'John', surname: 'Smith', score: 1 }],
			total: 1
		});
		getPerson.mockRejectedValue({ status: 500, message: 'boom' });
		setup({ subjects: [] });

		await fireEvent.input(screen.getByLabelText('Add a person'), { target: { value: 'John' } });
		await fireEvent.click(await screen.findByText('John Smith', {}, { timeout: 2000 }));
		await waitFor(() =>
			expect(screen.getByRole('button', { name: 'Remove person John Smith' })).toBeDefined()
		);
	});

	it('offers the persons and families the branch changed as subjects', async () => {
		updateBranch.mockResolvedValue(branch);
		const BRANCH_ONLY = '22222222-2222-2222-2222-222222222222';
		setup({}, [
			{ type: 'person', id: PERSON_ID, name: 'Mary Smith' },
			{ type: 'person', id: BRANCH_ONLY, name: 'Branch Only' },
			{ type: 'family', id: FAMILY_ID, name: 'Branch Family' }
		]);

		// Mary is already a subject, so she is not offered again.
		expect(screen.queryByRole('button', { name: /Add person: Mary Smith/ })).toBeNull();
		await fireEvent.click(screen.getByRole('button', { name: 'Add person: Branch Only' }));
		expect(screen.queryByRole('button', { name: 'Add person: Branch Only' })).toBeNull();
		await fireEvent.click(screen.getByRole('button', { name: 'Add family: Branch Family' }));
		await fireEvent.click(screen.getByRole('button', { name: 'Save research record' }));

		await waitFor(() => expect(updateBranch).toHaveBeenCalled());
		expect(updateBranch).toHaveBeenCalledWith(BRANCH_ID, {
			subjects: [
				{ type: 'person', id: PERSON_ID },
				{ type: 'person', id: BRANCH_ONLY },
				{ type: 'family', id: FAMILY_ID }
			]
		});
	});

	it('explains a save that raced another change to the branch', async () => {
		updateBranch.mockRejectedValue({ status: 409, code: 'branch_changed', message: 'conflict' });
		const { onsaved } = setup();

		await fireEvent.change(screen.getByLabelText('Outcome'), { target: { value: 'proved' } });
		await fireEvent.click(screen.getByRole('button', { name: 'Save research record' }));

		expect(await screen.findByText(/changed elsewhere while you were editing/)).toBeDefined();
		expect(onsaved).not.toHaveBeenCalled();
	});

	it('offers only the outcome on a merged branch', async () => {
		updateBranch.mockResolvedValue({ ...branch, status: 'merged', outcome: 'proved' });
		setup({ status: 'merged' });

		expect(screen.queryByLabelText('Research question')).toBeNull();
		expect(screen.queryByText('Subjects')).toBeNull();
		expect(screen.getByText(/you can still record its outcome/)).toBeDefined();
		expect(listProofSummaries).not.toHaveBeenCalled();

		await fireEvent.change(screen.getByLabelText('Outcome'), { target: { value: 'proved' } });
		await fireEvent.click(screen.getByRole('button', { name: 'Save research record' }));
		await waitFor(() => expect(updateBranch).toHaveBeenCalledWith(BRANCH_ID, { outcome: 'proved' }));
	});

	it('explains a refusal instead of closing', async () => {
		updateBranch.mockRejectedValue({ status: 409, code: 'branch_field_locked', message: 'locked' });
		const { onsaved } = setup();

		await fireEvent.change(screen.getByLabelText('Outcome'), { target: { value: 'disproved' } });
		await fireEvent.click(screen.getByRole('button', { name: 'Save research record' }));

		expect(await screen.findByText(/only its outcome can still change/)).toBeDefined();
		expect(onsaved).not.toHaveBeenCalled();
	});

	it('shows the server message for other failures, and cancels on request', async () => {
		updateBranch.mockRejectedValue({ status: 400, code: 'invalid_reference', message: 'branch subject not found' });
		const { oncancel } = setup();

		await fireEvent.change(screen.getByLabelText('Outcome'), { target: { value: 'superseded' } });
		await fireEvent.click(screen.getByRole('button', { name: 'Save research record' }));
		expect(await screen.findByText('branch subject not found')).toBeDefined();

		await fireEvent.click(screen.getByRole('button', { name: 'Cancel' }));
		expect(oncancel).toHaveBeenCalled();
	});

	it('reports an archived-branch refusal and a proof-summary load failure', async () => {
		listProofSummaries.mockRejectedValue({ status: 500, message: 'boom' });
		updateBranch.mockRejectedValue({ status: 409, code: 'branch_not_active', message: 'x' });
		setup();

		expect(await screen.findByText('boom')).toBeDefined();
		await fireEvent.change(screen.getByLabelText('Outcome'), { target: { value: 'proved' } });
		await fireEvent.click(screen.getByRole('button', { name: 'Save research record' }));
		expect(await screen.findByText(/archived and accepts no further changes/)).toBeDefined();
	});
});
