import { describe, it, expect, vi, beforeEach } from 'vitest';
import * as apiModule from '#lib/api/client.js';
import { SubjectNames, subjectHref } from './subjectNames.svelte.js';

vi.mock('#lib/api/client.js', async (importOriginal) => {
	const actual = await importOriginal<typeof apiModule>();
	return { ...actual, api: { getPerson: vi.fn(), getFamily: vi.fn() } };
});

describe('SubjectNames', () => {
	beforeEach(() => {
		vi.clearAllMocks();
	});

	it('reads a person and a family by name, each once', async () => {
		vi.mocked(apiModule.api.getPerson).mockResolvedValue({
			id: 'p-1',
			given_name: 'Ada',
			surname: 'Lovelace'
		} as apiModule.PersonDetail);
		vi.mocked(apiModule.api.getFamily).mockResolvedValue({
			id: 'f-1',
			partner1: { id: 'p-1', given_name: 'Ada', surname: 'Lovelace' },
			partner2: { id: 'p-2', given_name: 'William', surname: 'King' }
		} as apiModule.FamilyDetail);

		const names = new SubjectNames();
		const ada = { type: 'person', id: 'p-1' } as const;
		const family = { type: 'family', id: 'f-1' } as const;
		expect(names.label(ada)).toBe('…');
		await names.load([ada, family, ada]);
		await names.load([ada]);

		expect(names.label(ada)).toBe('Ada Lovelace');
		expect(names.name(family)).toBe('Ada Lovelace & William King');
		expect(apiModule.api.getPerson).toHaveBeenCalledTimes(1);
		expect(apiModule.api.getFamily).toHaveBeenCalledTimes(1);
	});

	it('labels a subject it cannot read as unknown', async () => {
		vi.mocked(apiModule.api.getPerson).mockRejectedValue(new Error('not found'));
		const names = new SubjectNames();
		const gone = { type: 'person', id: 'p-gone' } as const;
		await names.load([gone]);
		expect(names.name(gone)).toBeUndefined();
		expect(names.label(gone)).toBe('Unknown person');
	});

	it('links a subject to its page', () => {
		expect(subjectHref({ type: 'person', id: 'p-1' })).toBe('/persons/p-1');
		expect(subjectHref({ type: 'family', id: 'f-1' })).toBe('/families/f-1');
	});
});
