import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, within, fireEvent, waitFor } from '@testing-library/svelte';
import UnmappedPlaces from './UnmappedPlaces.svelte';
import * as apiModule from '#lib/api/client.js';
import { signInAs } from '#lib/test/roles.js';
import { bostonCandidate, setGeocoderEnabled } from '#lib/test/geocode.js';

vi.mock('#lib/api/client.js', async (importOriginal) => {
	const actual = await importOriginal<typeof apiModule>();
	return { ...actual, api: { listUnmappedPlaces: vi.fn(), updatePlace: vi.fn(), getPlace: vi.fn(), geocodePlace: vi.fn() } };
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

// A geocoder candidate is saved on the place, as geocoded (#1607, ADR-009).
describe('UnmappedPlaces geocoder lookup (#1607)', () => {
	const lookUp = (name: string) => screen.findByRole('button', { name: `Look up coordinates for ${name}` });

	beforeEach(() => {
		vi.clearAllMocks();
		branchState.id = null;
		signInAs('editor');
		setGeocoderEnabled(true);
		vi.mocked(apiModule.api.listUnmappedPlaces).mockResolvedValue({ items: [boston, salem], total: 2 });
		vi.mocked(apiModule.api.updatePlace).mockImplementation(async (id) =>
			placeOf(id === boston.id ? boston : salem, { version: 5 })
		);
		vi.mocked(apiModule.api.geocodePlace).mockResolvedValue({
			items: [bostonCandidate, { ...bostonCandidate, display_name: 'Boston, Lincolnshire', latitude: 52.97, longitude: -0.02, query: 'Boston, MA' }],
			total: 2
		});
	});

	it('offers no lookup when the geocoder is disabled', async () => {
		setGeocoderEnabled(false);
		render(UnmappedPlaces);
		await screen.findByText('Boston');
		expect(screen.queryByRole('button', { name: /^Look up/ })).toBeNull();
	});

	it('offers no lookup on a research branch, where place facts cannot be set', async () => {
		branchState.id = 'branch-1';
		render(UnmappedPlaces);
		await screen.findByText('Boston');
		expect(screen.queryByRole('button', { name: /^Look up/ })).toBeNull();
	});

	it('looks up every entry, with or without suggestions, by its name', async () => {
		render(UnmappedPlaces);
		await lookUp('Boston');
		expect(screen.getByRole('button', { name: 'Look up coordinates for Salem, MA' })).toBeTruthy();
		await fireEvent.click(await lookUp('Salem, MA'));
		expect(apiModule.api.geocodePlace).toHaveBeenCalledWith('Salem, MA');
		expect(apiModule.api.updatePlace).not.toHaveBeenCalled();
	});

	it.each([
		['the whole name, as geocoded', 0, 'N42.3554334', 'W71.060511', { source: 'geocoded', provider: 'Nominatim', precision: 'full' }],
		['text the name does not hold, as entered by hand', 1, 'N52.97', 'W0.02', { source: 'manual' }]
	] as const)('saves a candidate that matched %s', async (_n, index, latitude, longitude, origin) => {
		render(UnmappedPlaces);
		await fireEvent.click(await lookUp('Boston'));
		const uses = await screen.findAllByRole('button', { name: /^Use this, .* for Boston$/ });
		await fireEvent.click(uses[index]);

		await waitFor(() =>
			expect(apiModule.api.updatePlace).toHaveBeenCalledWith(boston.id, {
				version: 4,
				latitude,
				longitude,
				coordinates_origin: origin
			})
		);
		await waitFor(() => expect(screen.queryByText('3 records without coordinates')).toBeNull());
	});

	it('sends no write when the list is closed without a pick', async () => {
		render(UnmappedPlaces);
		await fireEvent.click(await lookUp('Boston'));
		await screen.findByRole('region', { name: 'Locations matching “Boston”' });
		expect(screen.getByTestId('geocode-attribution').textContent).toBe(bostonCandidate.attribution);
		await fireEvent.click(screen.getByRole('button', { name: 'Close' }));
		expect(screen.queryByRole('region')).toBeNull();
		expect(apiModule.api.updatePlace).not.toHaveBeenCalled();
	});

	it('says when the geocoder failed, and keeps the other controls usable', async () => {
		vi.mocked(apiModule.api.geocodePlace).mockRejectedValue({ status: 502, code: 'geocoder_error', message: 'x' });
		render(UnmappedPlaces);
		await fireEvent.click(await lookUp('Salem, MA'));
		expect((await screen.findByRole('alert')).textContent).toContain('The geocoder failed to answer.');
		await fireEvent.click(screen.getByRole('button', { name: 'Enter coordinates for Salem, MA' }));
		expect(screen.getByLabelText('Latitude')).toBeTruthy();
	});
});

// "Look up all" queues each entry's lookup, one request at a time (#1609).
describe('UnmappedPlaces look up all (#1609)', () => {
	const plymouth: apiModule.UnmappedPlace = {
		...salem,
		id: '91e00000-0000-5000-8000-000000000000',
		key: 'plymouth,ma',
		name: 'Plymouth',
		spelling: 'Plymouth'
	};
	const lookUpAll = () => screen.findByRole('button', { name: 'Look up all places' });
	// The run's own status line; a save adds a status of its own.
	const status = () =>
		screen.getAllByRole('status').find((el) => el.classList.contains('run-status'))?.textContent?.trim();

	/** Lookups held open until answered, with the most ever in flight at once. */
	let held: { name: string; answer: () => void }[];
	let inFlight: number;
	let maxInFlight: number;
	/** Answer the oldest held lookup with a candidate named after it. */
	async function answerNext() {
		await waitFor(() => expect(held.length).toBeGreaterThan(0));
		held.shift()!.answer();
	}

	beforeEach(() => {
		vi.clearAllMocks();
		branchState.id = null;
		signInAs('editor');
		setGeocoderEnabled(true);
		held = [];
		inFlight = 0;
		maxInFlight = 0;
		vi.mocked(apiModule.api.listUnmappedPlaces).mockResolvedValue({ items: [boston, salem, plymouth], total: 3 });
		vi.mocked(apiModule.api.geocodePlace).mockImplementation(
			(name) =>
				new Promise((resolve) => {
					inFlight++;
					maxInFlight = Math.max(maxInFlight, inFlight);
					held.push({
						name,
						answer: () => {
							inFlight--;
							resolve({ items: [{ ...bostonCandidate, display_name: `${name}, found`, query: name }], total: 1 });
						}
					});
				})
		);
	});

	it.each([
		['a viewer', () => signInAs('viewer')],
		['anyone, with the geocoder disabled', () => setGeocoderEnabled(false)],
		['an editor on a research branch', () => (branchState.id = 'branch-1')]
	])('is not offered to %s', async (_who, arrange) => {
		arrange();
		render(UnmappedPlaces);
		await screen.findByText('Boston');
		expect(screen.queryByRole('button', { name: 'Look up all places' })).toBeNull();
	});

	it('looks up each place once, one request at a time, and shows each one’s candidates without saving', async () => {
		render(UnmappedPlaces);
		await fireEvent.click(await lookUpAll());
		expect(status()).toBe('Looking up places: 0 of 3 done.');
		expect(screen.getByRole('progressbar', { name: 'Places looked up' }).getAttribute('max')).toBe('3');

		await answerNext();
		await screen.findByText('Boston, found');
		await waitFor(() => expect(status()).toBe('Looking up places: 1 of 3 done.'));
		await answerNext();
		await screen.findByText('Salem, MA, found');
		await answerNext();
		await screen.findByText('Plymouth, found');

		await waitFor(() => expect(status()).toBe("Looked up 3 of 3 places. Review each place's locations below."));
		expect(vi.mocked(apiModule.api.geocodePlace).mock.calls.map(([name]) => name)).toEqual([
			'Boston',
			'Salem, MA',
			'Plymouth'
		]);
		expect(maxInFlight).toBe(1);
		expect(apiModule.api.updatePlace).not.toHaveBeenCalled();
		// The focus stays where the editor left it, not on each answer as it comes.
		expect(document.activeElement?.closest('.candidates')).toBeNull();
	});

	it('saves a candidate only when the editor picks one, while the run goes on', async () => {
		vi.mocked(apiModule.api.updatePlace).mockResolvedValue(placeOf(boston, { version: 5 }));
		render(UnmappedPlaces);
		await fireEvent.click(await lookUpAll());
		await answerNext();
		await fireEvent.click(await screen.findByRole('button', { name: /^Use this, Boston, found, .* for Boston$/ }));
		await waitFor(() => expect(apiModule.api.updatePlace).toHaveBeenCalledTimes(1));
		expect(vi.mocked(apiModule.api.updatePlace).mock.calls[0][0]).toBe(boston.id);
		await answerNext();
		await answerNext();
		await waitFor(() => expect(status()).toMatch(/^Looked up 3 of 3 places/));
		expect(apiModule.api.updatePlace).toHaveBeenCalledTimes(1);
	});

	it('stops when cancelled, after the lookup in flight', async () => {
		render(UnmappedPlaces);
		await fireEvent.click(await lookUpAll());
		await answerNext();
		await waitFor(() => expect(apiModule.api.geocodePlace).toHaveBeenCalledTimes(2));
		await fireEvent.click(screen.getByRole('button', { name: 'Stop looking up' }));
		expect(status()).toBe('Stopped after 1 of 3 places.');
		await answerNext();
		await screen.findByText('Salem, MA, found');
		await new Promise((r) => setTimeout(r, 0));
		expect(apiModule.api.geocodePlace).toHaveBeenCalledTimes(2);
		expect(status()).toBe('Stopped after 1 of 3 places.');
		expect(screen.getByRole('button', { name: 'Look up all places' })).toBeTruthy();
	});

	it('stops when the page is left', async () => {
		const { unmount } = render(UnmappedPlaces);
		await fireEvent.click(await lookUpAll());
		await waitFor(() => expect(apiModule.api.geocodePlace).toHaveBeenCalledTimes(1));
		unmount();
		held.shift()!.answer();
		await new Promise((r) => setTimeout(r, 0));
		expect(apiModule.api.geocodePlace).toHaveBeenCalledTimes(1);
	});
});
