import { describe, it, expect, vi, beforeEach } from 'vitest';
import { fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import RelationshipPage from './+page.svelte';
import * as apiModule from '$lib/api/client';
import { back, currentPath, historyLength, resetRouter } from '$lib/test/fakeRouter';

vi.mock('$lib/api/client', async (importOriginal) => {
	const actual = await importOriginal<typeof apiModule>();
	return {
		...actual,
		api: { getPerson: vi.fn(), getRelationship: vi.fn(), searchPersons: vi.fn() }
	};
});

vi.mock('$app/stores', async () => (await import('$lib/test/fakeRouter')).appStores);
vi.mock('$app/navigation', async () => (await import('$lib/test/fakeRouter')).appNavigation);

const people: Record<string, apiModule.Person> = {
	a: { id: 'a', given_name: 'Ada', surname: 'Lovelace', version: 1 } as apiModule.Person,
	b: { id: 'b', given_name: 'Byron', surname: 'Lovelace', version: 1 } as apiModule.Person
};

describe('Relationship page: the pair lives in the URL (#901)', () => {
	beforeEach(() => {
		vi.clearAllMocks();
		vi.mocked(apiModule.api.getPerson).mockImplementation(async (id: string) => {
			if (!people[id]) throw new Error('not found');
			return people[id] as Awaited<ReturnType<typeof apiModule.api.getPerson>>;
		});
		vi.mocked(apiModule.api.getRelationship).mockImplementation(async (idA, idB) => ({
			personA: people[idA],
			personB: people[idB],
			isRelated: false,
			paths: []
		}) as unknown as apiModule.RelationshipResult);
	});

	it('a shared link with both people shows the result straight away', async () => {
		resetRouter('/relationship?personA=a&personB=b');
		render(RelationshipPage);
		await waitFor(() => expect(apiModule.api.getRelationship).toHaveBeenCalledWith('a', 'b'));
	});

	it('only prefills when the link names one person', async () => {
		resetRouter('/relationship?personA=a');
		render(RelationshipPage);
		await waitFor(() => expect(apiModule.api.getPerson).toHaveBeenCalledWith('a'));
		await screen.findByRole('button', { name: /Calculate Relationship/ });
		expect(apiModule.api.getRelationship).not.toHaveBeenCalled();
	});

	it('pushes a new calculation, and Back shows the previous pair again', async () => {
		resetRouter('/relationship?personA=a&personB=b');
		render(RelationshipPage);
		await waitFor(() => expect(apiModule.api.getRelationship).toHaveBeenCalledTimes(1));

		await fireEvent.click(screen.getByRole('button', { name: 'Swap people' }));
		await fireEvent.click(screen.getByRole('button', { name: /Calculate Relationship/ }));
		await waitFor(() => expect(currentPath()).toBe('/relationship?personA=b&personB=a'));
		expect(historyLength()).toBe(2);
		await waitFor(() => expect(apiModule.api.getRelationship).toHaveBeenLastCalledWith('b', 'a'));
		// The calculator kept its own result: no reload for the pair it just wrote.
		expect(apiModule.api.getRelationship).toHaveBeenCalledTimes(2);

		back();
		await waitFor(() => expect(apiModule.api.getRelationship).toHaveBeenLastCalledWith('a', 'b'));
	});

	it('recalculating the pair already in the URL adds no history entry', async () => {
		resetRouter('/relationship?personA=a&personB=b');
		render(RelationshipPage);
		await waitFor(() => expect(apiModule.api.getRelationship).toHaveBeenCalledTimes(1));
		await fireEvent.click(screen.getByRole('button', { name: /Calculate Relationship/ }));
		await waitFor(() => expect(apiModule.api.getRelationship).toHaveBeenCalledTimes(2));
		expect(historyLength()).toBe(1);
	});
});
