import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, within, fireEvent, waitFor } from '@testing-library/svelte';
import UnmappedPlaces from './UnmappedPlaces.svelte';
import * as apiModule from '#lib/api/client.js';
import { signInAs } from '#lib/test/roles.js';

vi.mock('#lib/api/client.js', async (importOriginal) => {
	const actual = await importOriginal<typeof apiModule>();
	return { ...actual, api: { listUnmappedPlaces: vi.fn(), updatePlace: vi.fn(), getPlace: vi.fn() } };
});

const { branchState } = vi.hoisted(() => ({ branchState: { id: null as string | null } }));
vi.mock('#lib/stores/activeBranch.svelte.js', () => ({ activeBranch: branchState }));

const boston: apiModule.UnmappedPlace = {
	id: 'b0570000-0000-5000-8000-000000000000',
	key: 'boston,ma',
	name: 'Boston',
	spelling: 'Boston, MA',
	count: 3,
	version: 4,
	suggestions: [
		{ latitude: 'N42.36', longitude: 'W71.06', count: 2 },
		{ latitude: 'N42.30', longitude: 'W71.00', count: 1 }
	]
};
const salem: apiModule.UnmappedPlace = {
	id: '5a1e0000-0000-5000-8000-000000000000',
	key: 'salem,ma',
	name: 'Salem, MA',
	spelling: 'Salem, MA',
	count: 1,
	version: 0,
	suggestions: []
};
const placeOf = (entry: apiModule.UnmappedPlace, extra: Partial<apiModule.Place> = {}): apiModule.Place => ({
	id: entry.id,
	key: entry.key,
	name: entry.name,
	count: entry.count,
	version: entry.version,
	...extra
});
const versionConflict = { status: 409, code: 'VERSION_CONFLICT', message: 'Version conflict' };

describe('UnmappedPlaces', () => {
	beforeEach(() => {
		vi.clearAllMocks();
		branchState.id = null;
		signInAs('editor');
		vi.mocked(apiModule.api.listUnmappedPlaces).mockResolvedValue({ items: [boston, salem], total: 2 });
		vi.mocked(apiModule.api.updatePlace).mockImplementation(async (id) =>
			placeOf(id === boston.id ? boston : salem, { version: 5 })
		);
	});

	it('lists each place in order with its count and suggestions', async () => {
		render(UnmappedPlaces);
		await screen.findByText('Boston');
		const places = Array.from(document.querySelectorAll<HTMLElement>('.unmapped-item'));
		expect(places.map((li) => li.querySelector('.unmapped-name')?.textContent)).toEqual(['Boston', 'Salem, MA']);

		const first = within(places[0]);
		expect(first.getByText('3 records without coordinates')).toBeTruthy();
		expect(first.getByText('N42.36, W71.06').parentElement?.textContent).toBe('N42.36, W71.06 (2 records)');
		expect(first.getByText('N42.30, W71.00').parentElement?.textContent).toBe('N42.30, W71.00 (1 record)');

		const second = within(places[1]);
		expect(second.getByText('1 record without coordinates')).toBeTruthy();
		expect(second.getByText('No other record here has coordinates.')).toBeTruthy();
		expect(screen.getByText('2 places without coordinates')).toBeTruthy();
	});

	it('links each entry to the people born or died there by a recorded spelling, not its display name', async () => {
		render(UnmappedPlaces);
		const boston = await screen.findByRole('link', { name: 'People born or died in Boston' });
		expect(boston.getAttribute('href')).toBe('/browse/places/Boston%2C%20MA');
		expect(screen.getByRole('link', { name: 'People born or died in Salem, MA' }).getAttribute('href')).toBe(
			'/browse/places/Salem%2C%20MA'
		);
	});

	it('shows the empty state when every place has coordinates', async () => {
		vi.mocked(apiModule.api.listUnmappedPlaces).mockResolvedValue({ items: [], total: 0 });
		render(UnmappedPlaces);
		expect((await screen.findByTestId('empty-state')).textContent).toContain('Every place has coordinates.');
		expect(screen.queryByRole('link')).toBeNull();
	});

	it('shows a load error with Retry', async () => {
		vi.mocked(apiModule.api.listUnmappedPlaces).mockRejectedValueOnce(new Error('boom'));
		vi.spyOn(console, 'error').mockImplementation(() => {});
		render(UnmappedPlaces);
		const alert = await screen.findByRole('alert');
		expect(alert.textContent).toContain('Failed to load places without coordinates. Please try again.');
		expect(alert.textContent).not.toContain('boom');
		await fireEvent.click(within(alert).getByRole('button', { name: 'Retry' }));
		await screen.findByText('Boston');
	});

	it('applies a suggestion as stored, copied, at the version the entry was read at', async () => {
		render(UnmappedPlaces);
		await fireEvent.click(await screen.findByRole('button', { name: 'Use these coordinates, N42.30, W71.00, for Boston' }));
		await waitFor(() =>
			expect(apiModule.api.updatePlace).toHaveBeenCalledWith(boston.id, {
				version: 4,
				latitude: 'N42.30',
				longitude: 'W71.00',
				coordinates_origin: { source: 'copied' }
			})
		);
		const notice = await screen.findByRole('status');
		expect(notice.textContent).toContain('Boston is now on the map at N42.30, W71.00.');
		expect(within(notice).getByRole('link', { name: 'Open the map' }).getAttribute('href')).toBe('/map');
		expect(screen.queryByText('Boston')).toBeNull();
		expect(screen.getByText('1 place without coordinates')).toBeTruthy();
	});

	it('offers no apply or entry control to a viewer', async () => {
		signInAs('viewer');
		render(UnmappedPlaces);
		await screen.findByText('Boston');
		expect(screen.queryByRole('button')).toBeNull();
		expect(screen.queryByRole('note')).toBeNull();
	});

	it('withdraws the controls on a research branch, as place facts are main-only', async () => {
		branchState.id = 'b1';
		render(UnmappedPlaces);
		await screen.findByText('Boston');
		expect(screen.queryByRole('button')).toBeNull();
		expect(screen.getByRole('note').textContent).toContain('Return to the mainline');
	});

	it('shows the save-conflict flow on a stale version and saves only when asked', async () => {
		vi.mocked(apiModule.api.updatePlace).mockRejectedValueOnce(versionConflict);
		vi.mocked(apiModule.api.getPlace).mockResolvedValue(placeOf(boston, { version: 7, name: 'Boston, Massachusetts' }));
		render(UnmappedPlaces);
		await fireEvent.click(await screen.findByRole('button', { name: 'Use these coordinates, N42.36, W71.06, for Boston' }));

		const alert = await screen.findByRole('alert');
		expect(alert.textContent).toContain('This place was changed elsewhere');
		expect(apiModule.api.getPlace).toHaveBeenCalledWith(boston.id);
		expect(apiModule.api.updatePlace).toHaveBeenCalledTimes(1);
		expect(screen.getByText('Boston')).toBeTruthy();
		expect(screen.queryByRole('status')).toBeNull();
		expect(within(alert).getAllByTestId('mine').map((td) => td.textContent)).toEqual(['N42.36', 'W71.06']);

		await fireEvent.click(within(alert).getByRole('button', { name: 'Save my edits' }));
		await waitFor(() => expect(apiModule.api.updatePlace).toHaveBeenCalledTimes(2));
		expect(apiModule.api.updatePlace).toHaveBeenLastCalledWith(boston.id, {
			version: 7,
			latitude: 'N42.36',
			longitude: 'W71.06',
			coordinates_origin: { source: 'copied' }
		});
		await waitFor(() => expect(screen.queryByText('Boston')).toBeNull());
		expect(screen.queryByRole('alert')).toBeNull();
	});

	it('drops the apply for the latest version and re-reads the list', async () => {
		vi.mocked(apiModule.api.updatePlace).mockRejectedValueOnce(versionConflict);
		vi.mocked(apiModule.api.getPlace).mockResolvedValue(
			placeOf(boston, { version: 7, latitude: 'N42.3', longitude: 'W71.1', coordinates_origin: { source: 'manual' } })
		);
		render(UnmappedPlaces);
		await fireEvent.click(await screen.findByRole('button', { name: 'Use these coordinates, N42.36, W71.06, for Boston' }));
		const alert = await screen.findByRole('alert');
		expect(within(alert).getAllByTestId('latest').map((td) => td.textContent)).toEqual(['N42.3', 'W71.1']);

		vi.mocked(apiModule.api.listUnmappedPlaces).mockResolvedValue({ items: [salem], total: 1 });
		await fireEvent.click(within(alert).getByRole('button', { name: 'Use the latest version' }));
		await waitFor(() => expect(screen.queryByText('Boston')).toBeNull());
		expect(apiModule.api.listUnmappedPlaces).toHaveBeenCalledTimes(2);
		expect(apiModule.api.updatePlace).toHaveBeenCalledTimes(1);
	});

	it('does not save onto a place joined to another since the list was read', async () => {
		vi.mocked(apiModule.api.updatePlace).mockRejectedValueOnce(versionConflict);
		vi.mocked(apiModule.api.getPlace).mockResolvedValue(
			placeOf(boston, { version: 5, resolves_to: salem.id })
		);
		render(UnmappedPlaces);
		await fireEvent.click(await screen.findByRole('button', { name: 'Use these coordinates, N42.36, W71.06, for Boston' }));

		vi.mocked(apiModule.api.listUnmappedPlaces).mockResolvedValue({ items: [salem], total: 1 });
		const alert = await screen.findByRole('alert');
		expect(alert.textContent).toContain('Boston was joined to another place since this list was read');
		expect(screen.queryByRole('button', { name: 'Save my edits' })).toBeNull();
		await waitFor(() => expect(apiModule.api.listUnmappedPlaces).toHaveBeenCalledTimes(2));
		expect(apiModule.api.updatePlace).toHaveBeenCalledTimes(1);
		expect(screen.queryByRole('status')).toBeNull();
	});

	it('shows any other refusal as it came and keeps the entry', async () => {
		vi.mocked(apiModule.api.updatePlace).mockRejectedValueOnce({ status: 400, message: 'latitude out of range' });
		render(UnmappedPlaces);
		await fireEvent.click(await screen.findByRole('button', { name: 'Use these coordinates, N42.36, W71.06, for Boston' }));
		expect((await screen.findByRole('alert')).textContent).toContain('latitude out of range');
		expect(apiModule.api.getPlace).not.toHaveBeenCalled();
		expect(screen.getByText('Boston')).toBeTruthy();
	});

	it('takes a typed pair for an entry with no suggestions, validated as the person form does', async () => {
		render(UnmappedPlaces);
		expect(screen.queryByRole('button', { name: 'Enter coordinates for Boston' })).toBeNull();
		await fireEvent.click(await screen.findByRole('button', { name: 'Enter coordinates for Salem, MA' }));
		const latitude = screen.getByLabelText('Latitude');
		const longitude = screen.getByLabelText('Longitude');
		const save = screen.getByRole('button', { name: 'Save coordinates' });

		await fireEvent.click(save);
		expect((await screen.findByRole('alert')).textContent).toContain('Enter a latitude and a longitude.');

		await fireEvent.input(latitude, { target: { value: '95' } });
		await fireEvent.input(longitude, { target: { value: '-70.9' } });
		await fireEvent.click(save);
		expect((await screen.findByRole('alert')).textContent).toContain('Latitude must be between -90 and 90.');
		expect(latitude.getAttribute('aria-invalid')).toBe('true');

		await fireEvent.input(latitude, { target: { value: '42.52' } });
		await fireEvent.input(longitude, { target: { value: '' } });
		await fireEvent.click(save);
		expect((await screen.findByRole('alert')).textContent).toContain('Enter a longitude too');
		expect(apiModule.api.updatePlace).not.toHaveBeenCalled();

		await fireEvent.input(longitude, { target: { value: '-70.9' } });
		await fireEvent.click(save);
		await waitFor(() =>
			expect(apiModule.api.updatePlace).toHaveBeenCalledWith(salem.id, {
				version: 0,
				latitude: 'N42.52',
				longitude: 'W70.9',
				coordinates_origin: { source: 'manual' }
			})
		);
		await waitFor(() => expect(screen.queryByText('Salem, MA')).toBeNull());
	});

	it('cancels typing without writing', async () => {
		render(UnmappedPlaces);
		await fireEvent.click(await screen.findByRole('button', { name: 'Enter coordinates for Salem, MA' }));
		await fireEvent.click(screen.getByRole('button', { name: 'Cancel' }));
		expect(screen.queryByLabelText('Latitude')).toBeNull();
		expect(apiModule.api.updatePlace).not.toHaveBeenCalled();
	});
});
