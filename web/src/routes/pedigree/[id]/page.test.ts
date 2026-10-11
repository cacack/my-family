import { describe, it, expect, vi, beforeEach } from 'vitest';
import { fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import PedigreePage from './+page.svelte';
import * as apiModule from '$lib/api/client';
import { resetRouter } from '$lib/test/fakeRouter';

vi.mock('$lib/api/client', async (importOriginal) => {
	const actual = await importOriginal<typeof apiModule>();
	return { ...actual, api: { getPedigree: vi.fn() } };
});

vi.mock('$app/stores', async () => (await import('$lib/test/fakeRouter')).appStores);
vi.mock('$app/navigation', async () => (await import('$lib/test/fakeRouter')).appNavigation);

// The root's name says which generations request produced the chart.
function pedigreeFor(gens: number): apiModule.Pedigree {
	return {
		root: { id: 'p-1', given_name: `Gens${gens}`, surname: 'Root', generation: 0 },
		total_ancestors: 0,
		max_generation: 0
	} as unknown as apiModule.Pedigree;
}

describe('Pedigree page: stale responses (#947)', () => {
	beforeEach(() => {
		vi.clearAllMocks();
	});

	it('ignores a load that resolves after a newer one', async () => {
		let resolveSlow: (p: apiModule.Pedigree) => void = () => {};
		vi.mocked(apiModule.api.getPedigree).mockImplementation((_id, gens) =>
			gens === 5
				? new Promise<apiModule.Pedigree>((r) => (resolveSlow = r))
				: Promise.resolve(pedigreeFor(gens ?? 0))
		);
		resetRouter('/pedigree/p-1?gens=3', { id: 'p-1' });
		const { container } = render(PedigreePage);
		await waitFor(() => expect(container.textContent).toContain('Gens3'));

		const select = screen.getByLabelText(/Generations/);
		await fireEvent.change(select, { target: { value: '5' } });
		await fireEvent.change(select, { target: { value: '6' } });
		await waitFor(() => expect(container.textContent).toContain('Gens6'));

		resolveSlow(pedigreeFor(5));
		await new Promise((r) => setTimeout(r, 0));
		expect(container.textContent).toContain('Gens6');
		expect(container.textContent).not.toContain('Gens5');
	});
});
