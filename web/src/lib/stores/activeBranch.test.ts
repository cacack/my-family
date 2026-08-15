import { describe, it, expect, vi, beforeEach } from 'vitest';
import type * as apiModule from '$lib/api/client';

const getBranchMock = vi.fn();
const reloadMock = vi.fn();

vi.mock('$lib/api/client', async (importOriginal) => {
	const actual = await importOriginal<typeof apiModule>();
	return {
		...actual,
		api: {
			getBranch: (id: string) => getBranchMock(id)
		}
	};
});

const BRANCH_ID = '44444444-4444-4444-4444-444444444444';
const STORAGE_KEY = 'active-branch';
const NOTICE_KEY = 'active-branch-notice';

/** What sessionStorage held at the moment the reload fired. */
let noticeAtReload: string | null = null;

const activeBranchFixture: apiModule.Branch = {
	id: BRANCH_ID,
	name: 'Maternal Smith line',
	base_position: 42,
	status: 'active',
	created_at: '2026-01-15T10:30:00Z'
};

/**
 * The store hydrates at module load, so each case has to re-import it against a
 * freshly seeded localStorage.
 */
async function loadStore(persistedId: string | null, persistedNotice?: string) {
	vi.resetModules();
	localStorage.clear();
	sessionStorage.clear();
	if (persistedId !== null) {
		localStorage.setItem(STORAGE_KEY, persistedId);
	}
	if (persistedNotice !== undefined) {
		sessionStorage.setItem(NOTICE_KEY, persistedNotice);
	}
	const client = await import('$lib/api/client');
	const store = await import('./activeBranch.svelte');
	return { client, store };
}

describe('activeBranch store', () => {
	beforeEach(() => {
		vi.clearAllMocks();
		noticeAtReload = null;
		reloadMock.mockImplementation(() => {
			noticeAtReload = sessionStorage.getItem(NOTICE_KEY);
		});
		// Switching reloads the page; jsdom has no navigation, so stand in for it.
		Object.defineProperty(window, 'location', {
			configurable: true,
			value: { ...window.location, reload: reloadMock }
		});
	});

	it('scopes the API client synchronously at load, before revalidation runs', async () => {
		getBranchMock.mockResolvedValue(activeBranchFixture);
		const { client, store } = await loadStore(BRANCH_ID);

		// Asserted before awaiting revalidation: a page that fetches in this
		// window must already be scoped or it silently reads the mainline.
		expect(client.getClientBranch()).toBe(BRANCH_ID);
		expect(store.activeBranch.id).toBe(BRANCH_ID);
	});

	it('keeps a persisted branch that is still active', async () => {
		getBranchMock.mockResolvedValue(activeBranchFixture);
		const { client, store } = await loadStore(BRANCH_ID);

		await store.revalidateActiveBranch();

		expect(getBranchMock).toHaveBeenCalledWith(BRANCH_ID);
		expect(store.activeBranch.id).toBe(BRANCH_ID);
		expect(store.activeBranch.branch?.name).toBe('Maternal Smith line');
		expect(store.activeBranch.notice).toBeNull();
		expect(client.getClientBranch()).toBe(BRANCH_ID);
	});

	it('falls back to mainline with a notice when the persisted branch is gone', async () => {
		getBranchMock.mockRejectedValue({ status: 404, code: 'not_found', message: 'not found' });
		const { client, store } = await loadStore(BRANCH_ID);

		await store.revalidateActiveBranch();

		expect(store.activeBranch.id).toBeNull();
		expect(store.activeBranch.branch).toBeNull();
		expect(store.activeBranch.notice).toContain('no longer available');
		expect(client.getClientBranch()).toBeNull();
		expect(localStorage.getItem(STORAGE_KEY)).toBeNull();
	});

	it('reloads after dropping a stale branch, with the notice already persisted', async () => {
		getBranchMock.mockRejectedValue({ status: 404, code: 'not_found', message: 'not found' });
		const { store } = await loadStore(BRANCH_ID);

		await store.revalidateActiveBranch();

		// Without the reload the user is left on a page of 404s; without the
		// persisted notice the reload is a silent scope change.
		expect(reloadMock).toHaveBeenCalled();
		expect(noticeAtReload).toContain('no longer available');
		expect(store.activeBranch.notice).toContain('no longer available');
	});

	it('does not reload when the branch id could not be cleared from storage', async () => {
		getBranchMock.mockRejectedValue({ status: 404, code: 'not_found', message: 'not found' });
		const { store } = await loadStore(BRANCH_ID);
		const removeItem = vi
			.spyOn(Storage.prototype, 'removeItem')
			.mockImplementation(() => {
				throw new Error('storage unavailable');
			});

		try {
			await store.revalidateActiveBranch();
		} finally {
			removeItem.mockRestore();
		}

		// Reloading would re-hydrate the same dead branch and drop it again.
		expect(reloadMock).not.toHaveBeenCalled();
		expect(store.activeBranch.id).toBeNull();
		expect(store.activeBranch.notice).toContain('no longer available');
	});

	// The mirror of the test above, on the switch path. If the new id cannot be
	// stored, a reload re-hydrates the OLD id and silently reverts the switch —
	// the user clicks a branch and lands back on the one they left.
	it('does not reload when the chosen branch id could not be persisted', async () => {
		const { store } = await loadStore(null);
		const setItem = vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
			throw new Error('storage unavailable');
		});

		try {
			store.switchBranch({
				id: BRANCH_ID,
				name: 'Maternal Smith line',
				base_position: 2,
				status: 'active',
				created_at: '2026-08-09T00:00:00Z'
			});
		} finally {
			setItem.mockRestore();
		}

		expect(reloadMock).not.toHaveBeenCalled();
		// The in-memory scope still stands, so later requests hit the branch.
		expect(store.activeBranch.id).toBe(BRANCH_ID);
	});

	it('shows a persisted drop notice on the next load, then clears it', async () => {
		const { store } = await loadStore(null, 'Your branch went away.');

		expect(store.activeBranch.notice).toBe('Your branch went away.');
		// Consumed on read, so a later reload does not resurrect it.
		expect(sessionStorage.getItem(NOTICE_KEY)).toBeNull();
	});

	it('keeps the branch when revalidation fails for a reason other than 404', async () => {
		getBranchMock.mockRejectedValue({
			status: 503,
			code: 'unavailable',
			message: 'branch registry not configured'
		});
		const { client, store } = await loadStore(BRANCH_ID);

		await store.revalidateActiveBranch();

		// A server that cannot answer says nothing about whether the branch exists.
		expect(store.activeBranch.id).toBe(BRANCH_ID);
		expect(client.getClientBranch()).toBe(BRANCH_ID);
		expect(localStorage.getItem(STORAGE_KEY)).toBe(BRANCH_ID);
		expect(store.activeBranch.notice).toBeNull();
		expect(store.activeBranch.unconfirmed).toBe(true);
		expect(reloadMock).not.toHaveBeenCalled();
	});

	it('lets a later attempt retry after an inconclusive failure', async () => {
		getBranchMock.mockRejectedValueOnce({ status: 500, code: 'internal', message: 'boom' });
		const { store } = await loadStore(BRANCH_ID);

		await store.revalidateActiveBranch();
		expect(store.activeBranch.unconfirmed).toBe(true);

		getBranchMock.mockResolvedValue(activeBranchFixture);
		await store.revalidateActiveBranch();

		expect(getBranchMock).toHaveBeenCalledTimes(2);
		expect(store.activeBranch.unconfirmed).toBe(false);
		expect(store.activeBranch.branch?.name).toBe('Maternal Smith line');
	});

	it('falls back to mainline when the persisted branch is terminal', async () => {
		getBranchMock.mockResolvedValue({ ...activeBranchFixture, status: 'archived' });
		const { client, store } = await loadStore(BRANCH_ID);

		await store.revalidateActiveBranch();

		expect(store.activeBranch.id).toBeNull();
		expect(store.activeBranch.notice).toContain('archived');
		expect(client.getClientBranch()).toBeNull();
		expect(localStorage.getItem(STORAGE_KEY)).toBeNull();
	});

	it('does nothing on the mainline', async () => {
		const { store } = await loadStore(null);

		await store.revalidateActiveBranch();

		expect(getBranchMock).not.toHaveBeenCalled();
		expect(store.activeBranch.id).toBeNull();
		expect(store.activeBranch.notice).toBeNull();
	});

	it('switching scopes the client, persists, and reloads so nothing stale survives', async () => {
		const { client, store } = await loadStore(null);

		store.switchBranch(activeBranchFixture);

		expect(store.activeBranch.id).toBe(BRANCH_ID);
		expect(client.getClientBranch()).toBe(BRANCH_ID);
		// Persisted before the reload, so the new scope survives it.
		expect(localStorage.getItem(STORAGE_KEY)).toBe(BRANCH_ID);
		expect(reloadMock).toHaveBeenCalled();
	});

	it('returning to mainline clears the scope and the persisted id', async () => {
		getBranchMock.mockResolvedValue(activeBranchFixture);
		const { client, store } = await loadStore(BRANCH_ID);

		store.returnToMainline();

		expect(store.activeBranch.id).toBeNull();
		expect(client.getClientBranch()).toBeNull();
		expect(localStorage.getItem(STORAGE_KEY)).toBeNull();
		expect(reloadMock).toHaveBeenCalled();
	});

	it('does not revalidate a branch the user just chose', async () => {
		const { store } = await loadStore(null);

		store.switchBranch(activeBranchFixture);
		await store.revalidateActiveBranch();

		expect(getBranchMock).not.toHaveBeenCalled();
	});

	it('dismisses the stale-branch notice', async () => {
		getBranchMock.mockRejectedValue({ status: 404, code: 'not_found', message: 'not found' });
		const { store } = await loadStore(BRANCH_ID);

		await store.revalidateActiveBranch();
		expect(store.activeBranch.notice).not.toBeNull();

		store.dismissBranchNotice();
		expect(store.activeBranch.notice).toBeNull();
	});

	it('exposes a read-only view, so scope cannot be changed behind the client', async () => {
		const { client, store } = await loadStore(null);

		const view = store.activeBranch as unknown as Record<string, unknown>;
		expect(() => {
			view.id = BRANCH_ID;
		}).toThrow();

		expect(store.activeBranch.id).toBeNull();
		expect(client.getClientBranch()).toBeNull();
	});
});
