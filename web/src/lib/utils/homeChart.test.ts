import { describe, it, expect, vi, beforeEach } from 'vitest';
import { isRedirect } from '@sveltejs/kit';
import * as apiModule from '#lib/api/client.js';
import { loadHomeChart } from './homeChart.js';

vi.mock('#lib/api/client.js', async (importOriginal) => {
	const actual = await importOriginal<typeof apiModule>();
	return { ...actual, api: { getHomePerson: vi.fn() } };
});

describe('loadHomeChart', () => {
	beforeEach(() => vi.clearAllMocks());

	it.each(['pedigree', 'descendancy', 'ahnentafel'] as const)(
		'starts the %s from the home person',
		async (chart) => {
			vi.mocked(apiModule.api.getHomePerson).mockResolvedValue({
				person: { id: 'me-1' } as apiModule.Person
			});
			const thrown = await loadHomeChart(chart).catch((e: unknown) => e);
			expect(isRedirect(thrown)).toBe(true);
			expect(thrown).toMatchObject({ status: 307, location: `/${chart}/me-1` });
		}
	);

	it('renders the route when no home person is set', async () => {
		vi.mocked(apiModule.api.getHomePerson).mockResolvedValue({});
		await expect(loadHomeChart('pedigree')).resolves.toEqual({});
	});

	it('renders the route when the setting cannot be read', async () => {
		vi.mocked(apiModule.api.getHomePerson).mockRejectedValue(new Error('offline'));
		await expect(loadHomeChart('pedigree')).resolves.toEqual({});
	});
});
