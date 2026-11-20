import { describe, it, expect, vi, beforeEach } from 'vitest';
import * as apiModule from '#lib/api/client.js';
import type { Person } from '#lib/api/client.js';
import { homePerson, loadHomePerson, resetHomePerson, setHomePerson, clearHomePerson } from './homePerson.svelte.js';

vi.mock('#lib/api/client.js', async (importOriginal) => {
	const actual = await importOriginal<typeof apiModule>();
	return { ...actual, api: { getHomePerson: vi.fn(), setHomePerson: vi.fn(), clearHomePerson: vi.fn() } };
});

const api = vi.mocked(apiModule.api);

function person(id: string): Person {
	return { id, given_name: id, surname: '', version: 1 } as Person;
}

/** A promise and the function that settles it, to answer requests out of order. */
function deferred<T>(): { promise: Promise<T>; resolve: (v: T) => void } {
	let resolve!: (v: T) => void;
	const promise = new Promise<T>((r) => (resolve = r));
	return { promise, resolve };
}

describe('home person store', () => {
	beforeEach(() => {
		vi.clearAllMocks();
		resetHomePerson();
	});

	it('loads the home person and marks the store loaded', async () => {
		api.getHomePerson.mockResolvedValue({ person: person('ada') });
		await loadHomePerson();
		expect(homePerson.person?.id).toBe('ada');
		expect(homePerson.loaded).toBe(true);
	});

	it('leaves it unset when the load fails', async () => {
		api.getHomePerson.mockRejectedValue({ status: 500 });
		await loadHomePerson();
		expect(homePerson.person).toBeNull();
		expect(homePerson.loaded).toBe(true);
	});

	it('keeps the newer of two overlapping loads when the older answers last', async () => {
		const first = deferred<{ person?: Person }>();
		const second = deferred<{ person?: Person }>();
		api.getHomePerson.mockReturnValueOnce(first.promise).mockReturnValueOnce(second.promise);
		const older = loadHomePerson();
		const newer = loadHomePerson();
		second.resolve({ person: person('new') });
		await newer;
		first.resolve({ person: person('old') });
		await older;
		expect(homePerson.person?.id).toBe('new');
	});

	it('does not let a load that started earlier undo a choice made since', async () => {
		const load = deferred<{ person?: Person }>();
		api.getHomePerson.mockReturnValueOnce(load.promise);
		api.setHomePerson.mockResolvedValue({ person: person('chosen') });
		const loading = loadHomePerson();
		await setHomePerson('chosen');
		load.resolve({});
		await loading;
		expect(homePerson.person?.id).toBe('chosen');
	});

	it('forgets the last viewer’s choice and drops their load still in flight', async () => {
		const load = deferred<{ person?: Person }>();
		api.getHomePerson.mockReturnValueOnce(load.promise);
		api.clearHomePerson.mockResolvedValue(undefined);
		api.setHomePerson.mockResolvedValue({ person: person('mine') });
		await setHomePerson('mine');
		const loading = loadHomePerson();
		resetHomePerson();
		expect(homePerson.person).toBeNull();
		expect(homePerson.loaded).toBe(false);
		load.resolve({ person: person('mine') });
		await loading;
		expect(homePerson.person).toBeNull();
		expect(homePerson.loaded).toBe(false);
	});

	it('clears the home person', async () => {
		api.setHomePerson.mockResolvedValue({ person: person('ada') });
		api.clearHomePerson.mockResolvedValue(undefined);
		await setHomePerson('ada');
		await clearHomePerson();
		expect(homePerson.person).toBeNull();
		expect(homePerson.loaded).toBe(true);
	});
});
