import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/svelte';
import PlaceJoin from './PlaceJoin.svelte';
import * as apiModule from '#lib/api/client.js';
import { signInAs } from '#lib/test/roles.js';

vi.mock('#lib/api/client.js', async (importOriginal) => {
	const actual = await importOriginal<typeof apiModule>();
	return {
		...actual,
		api: {
			listPlaces: vi.fn(),
			getPlace: vi.fn(),
			aliasPlace: vi.fn(),
			unaliasPlace: vi.fn()
		}
	};
});

const { branchState } = vi.hoisted(() => ({ branchState: { id: null as string | null } }));
vi.mock('#lib/stores/activeBranch.svelte.js', () => ({ activeBranch: branchState }));

const koln: apiModule.Place = {
	id: '00000000-0000-0000-0000-00000000000a',
	key: 'köln,germany',
	name: 'Köln, Germany',
	count: 2,
	version: 1,
	latitude: 'N1',
	longitude: 'E1'
};
const cologne: apiModule.Place = {
	id: '00000000-0000-0000-0000-00000000000b',
	key: 'cologne,germany',
	name: 'Cologne',
	count: 3,
	version: 4,
	latitude: 'N50.94',
	longitude: 'E6.96',
	coordinates_origin: { source: 'manual' },
	aliases: [{ id: '00000000-0000-0000-0000-00000000000c', key: 'coeln,germany', name: 'Coeln, Germany' }]
};

describe('PlaceJoin', () => {
	beforeEach(() => {
		vi.clearAllMocks();
		branchState.id = null;
		vi.mocked(apiModule.api.listPlaces).mockResolvedValue({ items: [cologne, koln], total: 2 });
		vi.mocked(apiModule.api.aliasPlace).mockResolvedValue({ ...koln, version: 2, resolves_to: cologne.id });
		vi.mocked(apiModule.api.getPlace).mockResolvedValue({
			id: '00000000-0000-0000-0000-00000000000c',
			key: 'coeln,germany',
			name: 'Coeln, Germany',
			count: 1,
			version: 5,
			resolves_to: cologne.id
		});
		vi.mocked(apiModule.api.unaliasPlace).mockResolvedValue({ ...koln, version: 6 });
	});

	it('is not offered to a viewer', async () => {
		signInAs('viewer');
		render(PlaceJoin);
		expect(screen.getByText('Only editors can join or separate places.')).toBeTruthy();
		expect(screen.queryByRole('button', { name: 'Join places' })).toBeNull();
		expect(apiModule.api.listPlaces).not.toHaveBeenCalled();
	});

	it('is withdrawn on a research branch, where aliases are shared from the mainline', () => {
		branchState.id = 'b-1';
		render(PlaceJoin);
		expect(screen.getByRole('note').textContent).toContain('shared across all branches');
		expect(screen.queryByRole('button', { name: 'Join places' })).toBeNull();
		expect(apiModule.api.listPlaces).not.toHaveBeenCalled();
	});

	it('shows the facts of both places and joins them', async () => {
		signInAs('editor');
		render(PlaceJoin);
		const absorbed = (await screen.findByTestId('absorbed-select')) as HTMLSelectElement;
		const join = screen.getByRole('button', { name: 'Join places' }) as HTMLButtonElement;
		expect(join.disabled).toBe(true);

		await fireEvent.change(absorbed, { target: { value: koln.id } });
		await fireEvent.change(screen.getByTestId('target-select'), { target: { value: cologne.id } });

		const absorbedFacts = screen.getByTestId('absorbed-facts').textContent ?? '';
		expect(absorbedFacts).toContain('köln,germany');
		expect(absorbedFacts).toContain('N1, E1');
		const targetFacts = screen.getByTestId('target-facts').textContent ?? '';
		expect(targetFacts).toContain('cologne,germany');
		expect(targetFacts).toContain('N50.94, E6.96');
		expect(screen.getByText(/keeps\s+the spelling it was given/)).toBeTruthy();

		expect(join.disabled).toBe(false);
		await fireEvent.click(join);
		await waitFor(() =>
			expect(apiModule.api.aliasPlace).toHaveBeenCalledWith(koln.id, cologne.id, koln.version)
		);
		expect(await screen.findByText('Records of Köln, Germany now belong to Cologne.')).toBeTruthy();
		expect(apiModule.api.listPlaces).toHaveBeenCalledTimes(2);
	});

	it('will not join a place to itself', async () => {
		render(PlaceJoin);
		await fireEvent.change(await screen.findByTestId('absorbed-select'), { target: { value: koln.id } });
		await fireEvent.change(screen.getByTestId('target-select'), { target: { value: koln.id } });
		expect((screen.getByRole('button', { name: 'Join places' }) as HTMLButtonElement).disabled).toBe(true);
		expect(screen.getByText('Pick two different places.')).toBeTruthy();
	});

	it('separates a joined spelling at its current version', async () => {
		render(PlaceJoin);
		await fireEvent.change(await screen.findByTestId('target-select'), { target: { value: cologne.id } });
		expect(screen.getByText('Coeln, Germany')).toBeTruthy();
		await fireEvent.click(screen.getByRole('button', { name: 'Separate' }));
		await waitFor(() =>
			expect(apiModule.api.unaliasPlace).toHaveBeenCalledWith('00000000-0000-0000-0000-00000000000c', 5)
		);
		expect(await screen.findByText('Coeln, Germany is separate from Cologne again.')).toBeTruthy();
	});

	it('shows why a join failed', async () => {
		vi.mocked(apiModule.api.aliasPlace).mockRejectedValue({ message: 'Köln is itself an alias of another place' });
		render(PlaceJoin);
		await fireEvent.change(await screen.findByTestId('absorbed-select'), { target: { value: koln.id } });
		await fireEvent.change(screen.getByTestId('target-select'), { target: { value: cologne.id } });
		await fireEvent.click(screen.getByRole('button', { name: 'Join places' }));
		expect((await screen.findByRole('alert')).textContent).toContain('itself an alias');
	});

	it('narrows a picker by name or key', async () => {
		render(PlaceJoin);
		const absorbed = (await screen.findByTestId('absorbed-select')) as HTMLSelectElement;
		expect(absorbed.options).toHaveLength(2);
		await fireEvent.input(screen.getAllByLabelText('Search places')[0], { target: { value: 'köln' } });
		expect(Array.from(absorbed.options).map((o) => o.textContent)).toEqual(['Köln, Germany (2)']);
	});
});
