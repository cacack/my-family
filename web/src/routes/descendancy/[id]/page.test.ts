import { describe, it, expect, vi, beforeEach } from 'vitest';
import { fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import DescendancyPage from './+page.svelte';
import * as apiModule from '$lib/api/client';
import { resetRouter } from '$lib/test/fakeRouter';

vi.mock('$lib/api/client', async (importOriginal) => {
	const actual = await importOriginal<typeof apiModule>();
	return { ...actual, api: { getDescendancy: vi.fn() } };
});

vi.mock('$app/stores', async () => (await import('$lib/test/fakeRouter')).appStores);
vi.mock('$app/navigation', async () => (await import('$lib/test/fakeRouter')).appNavigation);

// The root's name says which generations request produced the chart.
function descendancyFor(gens: number): apiModule.Descendancy {
	return {
		root: { id: 'p-1', given_name: `Gens${gens}`, surname: 'Root', generation: 0 },
		total_descendants: 0,
		max_generation: 0
	} as unknown as apiModule.Descendancy;
}

describe('Descendancy page: stale responses (#947)', () => {
	beforeEach(() => {
		vi.clearAllMocks();
	});

	it('ignores a load that resolves after a newer one', async () => {
		let resolveSlow: (p: apiModule.Descendancy) => void = () => {};
		vi.mocked(apiModule.api.getDescendancy).mockImplementation((_id, gens) =>
			gens === 5
				? new Promise<apiModule.Descendancy>((r) => (resolveSlow = r))
				: Promise.resolve(descendancyFor(gens ?? 0))
		);
		resetRouter('/descendancy/p-1?gens=3', { id: 'p-1' });
		const { container } = render(DescendancyPage);
		await waitFor(() => expect(container.textContent).toContain('Gens3'));

		const select = screen.getByLabelText(/Generations/);
		await fireEvent.change(select, { target: { value: '5' } });
		await fireEvent.change(select, { target: { value: '6' } });
		await waitFor(() => expect(container.textContent).toContain('Gens6'));

		resolveSlow(descendancyFor(5));
		await new Promise((r) => setTimeout(r, 0));
		expect(container.textContent).toContain('Gens6');
		expect(container.textContent).not.toContain('Gens5');
	});
});
