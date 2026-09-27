import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import ChangeHistory from './ChangeHistory.svelte';
import * as apiModule from '$lib/api/client';

vi.mock('$lib/api/client', async (importOriginal) => {
	const actual = await importOriginal<typeof apiModule>();
	return {
		...actual,
		api: {
			getPersonHistory: vi.fn(),
			getFamilyHistory: vi.fn(),
			getSourceHistory: vi.fn(),
			getGlobalHistory: vi.fn()
		}
	};
});

const PERSON_ID = '11111111-1111-1111-1111-111111111111';

function entry(overrides: Partial<apiModule.ChangeEntry>): apiModule.ChangeEntry {
	return {
		id: crypto.randomUUID(),
		timestamp: '2026-01-15T10:30:00Z',
		entity_type: 'person',
		entity_id: PERSON_ID,
		entity_name: 'Ada Lovelace',
		action: 'updated',
		...overrides
	};
}

describe('ChangeHistory origin labels (#824)', () => {
	beforeEach(() => {
		vi.clearAllMocks();
	});

	it('labels branch-scoped entries as the branch’s own or inherited from the mainline', async () => {
		vi.mocked(apiModule.api.getPersonHistory).mockResolvedValue({
			items: [entry({ action: 'created', origin: 'main' }), entry({ origin: 'branch' })],
			total: 2,
			limit: 20,
			offset: 0,
			has_more: false
		});

		render(ChangeHistory, { entityType: 'person', entityId: PERSON_ID });

		expect(await screen.findByText('Mainline')).toBeDefined();
		expect(screen.getByText('This branch')).toBeDefined();
	});

	it('shows no origin label on mainline history', async () => {
		vi.mocked(apiModule.api.getFamilyHistory).mockResolvedValue({
			items: [entry({ entity_type: 'family', action: 'created' })],
			total: 1,
			limit: 20,
			offset: 0,
			has_more: false
		});

		render(ChangeHistory, { entityType: 'family', entityId: PERSON_ID });

		expect(await screen.findByText('created')).toBeDefined();
		expect(screen.queryByText('Mainline')).toBeNull();
		expect(screen.queryByText('This branch')).toBeNull();
	});
});
