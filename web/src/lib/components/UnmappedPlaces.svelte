<script lang="ts">
	/**
	 * The no-coordinates worklist (#1561, ADR-009 §9): each place with records
	 * that have no coordinates, most such records first, with the pairs other
	 * records there already carry. Each entry links to the people
	 * born or died at or under its spelling (GET /browse/places/{place}/persons),
	 * which is not the record set it counts: that list leaves out marriages, life
	 * events and attributes and resolves no alias, so the link says what it shows.
	 *
	 * An editor on the mainline can set the place's pair (#1815): apply a
	 * suggestion as stored (origin copied), or type one for an entry with none
	 * (origin manual), at the version the entry was read at. The entry then
	 * leaves the list, as every record there is mapped at the place's pair. A
	 * stale version opens the save-conflict flow. Place facts are main-only, so
	 * on a research branch the controls are withdrawn.
	 */
	import {
		api,
		isVersionConflict,
		type CoordinateSource,
		type Place,
		type UnmappedPlace
	} from '#lib/api/client.js';
	import { can } from '#lib/stores/access.svelte.js';
	import { activeBranch } from '#lib/stores/activeBranch.svelte.js';
	import { Button } from '#lib/components/ui/button/index.js';
	import { coordinatePairError, toGedcomCoordinate, type CoordinateError } from '#lib/utils/coordinates.js';
	import { SaveConflictFlow, type ConflictRow } from '#lib/utils/saveConflict.svelte.js';
	import ActionError from './ActionError.svelte';
	import CoordinateFields from './CoordinateFields.svelte';
	import SaveConflict from './SaveConflict.svelte';
	import ErrorState from './ErrorState.svelte';
	import LoadingState from './LoadingState.svelte';
	import EmptyState from './EmptyState.svelte';

	/** A pair in GEDCOM form, as the API stores it. */
	type Pair = { latitude: string; longitude: string };
	/** The place as the apply saves against: its version, own pair, and alias target if absorbed. */
	type PlaceBase = Pick<Place, 'version' | 'latitude' | 'longitude' | 'resolves_to'>;

	let entries: UnmappedPlace[] = $state([]);
	let loading = $state(true);
	let error: string | null = $state(null);

	const mayApply = $derived(can('editor') && !activeBranch.id);
	/** An editor on a branch, where the controls are withdrawn (place facts are main-only). */
	const withheld = $derived(can('editor') && !!activeBranch.id);

	/** The apply being saved, or refused for a stale version. */
	let pending: { entry: UnmappedPlace; base: PlaceBase; pair: Pair; source: CoordinateSource } | null =
		$state(null);
	let saving = $state(false);
	let applyError: { id: string; message: string } | null = $state(null);
	let notice: string | null = $state(null);
	/** A refusal for an entry that is no longer on the list as read. */
	let listNotice: string | null = $state(null);

	/** The entry whose pair is being typed, with the pair as typed. */
	let typing: { id: string; latitude: string; longitude: string; error: CoordinateError | null } | null =
		$state(null);

	const conflict = new SaveConflictFlow<PlaceBase, Pair>({
		base: () => pending?.base ?? null,
		formFrom: (saved) => ({ latitude: saved.latitude ?? '', longitude: saved.longitude ?? '' }),
		form: () => pending?.pair ?? { latitude: '', longitude: '' },
		adopt: (latest, pair) => {
			if (pending) pending = { ...pending, base: latest, pair };
		},
		save: () => save(),
		together: [['latitude', 'longitude']]
	});
	const conflictRows: ConflictRow<Pair>[] = [
		{ key: 'latitude', label: 'Latitude' },
		{ key: 'longitude', label: 'Longitude' }
	];

	async function load() {
		loading = true;
		error = null;
		try {
			entries = (await api.listUnmappedPlaces()).items;
		} catch (e) {
			console.error('Failed to load places without coordinates:', e);
			error = 'Failed to load places without coordinates. Please try again.';
		} finally {
			loading = false;
		}
	}

	$effect(() => {
		load();
	});

	function records(n: number): string {
		return `${n} ${n === 1 ? 'record' : 'records'}`;
	}

	function apply(entry: UnmappedPlace, pair: Pair, source: CoordinateSource) {
		conflict.clear();
		pending = { entry, base: { version: entry.version }, pair, source };
		save();
	}

	async function save() {
		if (!pending) return;
		const { entry, base, pair, source } = pending;
		saving = true;
		applyError = null;
		notice = null;
		listNotice = null;
		try {
			const saved = await api.updatePlace(entry.id, { version: base.version, ...pair, coordinates_origin: { source } });
			entries = entries.filter((e) => e.id !== entry.id);
			notice = `${saved.name} is now on the map at ${pair.latitude}, ${pair.longitude}.`;
			pending = null;
			if (typing?.id === entry.id) typing = null;
		} catch (e) {
			// Only a stale version is compared with the latest; any other refusal is shown as it came.
			// `pending` stays set while the conflict is open: `adopt` rebases it for "Save my edits".
			if (isVersionConflict(e)) {
				const latest = await api.getPlace(entry.id).catch(() => null);
				// Joined to another place since the list was read: its own pair is ignored while the
				// alias stands, so saving it would map nothing. Re-read the list, which shows the target.
				if (latest?.resolves_to) {
					pending = null;
					if (typing?.id === entry.id) typing = null;
					listNotice = `${entry.name} was joined to another place since this list was read, so its coordinates were not saved. The list now shows the place it was joined to.`;
					load();
					return;
				}
				if (latest && (await conflict.load(async () => latest))) return;
			}
			applyError = { id: entry.id, message: (e as { message?: string }).message || 'Saving the coordinates failed.' };
			pending = null;
		} finally {
			saving = false;
		}
	}

	/** Drop the apply for the place as saved now, and re-read the list. */
	function useLatest() {
		conflict.useLatest();
		pending = null;
		load();
	}

	function startTyping(entry: UnmappedPlace) {
		typing = { id: entry.id, latitude: '', longitude: '', error: null };
	}

	function submitTyped(entry: UnmappedPlace) {
		if (!typing) return;
		const pair = { latitude: typing.latitude, longitude: typing.longitude };
		typing.error = pair.latitude.trim() || pair.longitude.trim()
			? coordinatePairError(pair, entry.name)
			: { field: 'latitude', message: 'Enter a latitude and a longitude.' };
		if (typing.error) return;
		apply(
			entry,
			{
				latitude: toGedcomCoordinate(pair.latitude, 'latitude'),
				longitude: toGedcomCoordinate(pair.longitude, 'longitude')
			},
			'manual'
		);
	}
</script>

{#if loading}
	<LoadingState label="Loading places..." />
{:else if error}
	<ErrorState message={error} onRetry={load} />
{:else}
	{#if notice}
		<div class="notice" role="status">{notice} <a href="/map">Open the map</a></div>
	{/if}
	{#if listNotice}
		<ActionError message={listNotice} />
	{/if}
	{#if entries.length === 0}
		<EmptyState
			message="Every place has coordinates."
			description="No record names a place without a latitude and longitude."
		/>
	{:else}
		<p class="summary">{entries.length} {entries.length === 1 ? 'place' : 'places'} without coordinates</p>
		{#if withheld}
			<p class="withheld" role="note">
				Place coordinates are shared across all branches. Return to the mainline to set them.
			</p>
		{/if}
		<ul class="unmapped-list">
			{#each entries as entry (entry.id)}
				<li class="unmapped-item">
					<div class="unmapped-head">
						<span class="unmapped-name">{entry.name}</span>
						<span class="unmapped-count">{records(entry.count)} without coordinates</span>
					</div>
					<div class="unmapped-people">
						<a href="/browse/places/{encodeURIComponent(entry.spelling)}">People born or died in {entry.name}</a>
					</div>
					{#if entry.suggestions.length > 0}
						<div class="suggestions">
							<span class="suggestions-label">Other records here have:</span>
							<ul>
								{#each entry.suggestions as s (s.latitude + ' ' + s.longitude)}
									<li>
										<span><span class="pair">{s.latitude}, {s.longitude}</span> ({records(s.count)})</span>
										{#if mayApply}
											<Button
												variant="outline"
												size="sm"
												disabled={saving}
												aria-label="Use these coordinates, {s.latitude}, {s.longitude}, for {entry.name}"
												onclick={() => apply(entry, { latitude: s.latitude, longitude: s.longitude }, 'copied')}
											>
												Use these coordinates
											</Button>
										{/if}
									</li>
								{/each}
							</ul>
						</div>
					{:else}
						<p class="no-suggestions">No other record here has coordinates.</p>
						{#if mayApply}
							{#if typing?.id === entry.id}
								<form
									class="type-pair"
									novalidate
									onsubmit={(e) => {
										e.preventDefault();
										submitTyped(entry);
									}}
								>
									<CoordinateFields
										id="unmapped-{entry.id}"
										place={entry.name}
										bind:latitude={typing.latitude}
										bind:longitude={typing.longitude}
										error={typing.error}
									/>
									<div class="actions">
										<Button type="submit" size="sm" disabled={saving}>
											{saving && pending?.entry.id === entry.id ? 'Saving...' : 'Save coordinates'}
										</Button>
										<Button type="button" variant="outline" size="sm" disabled={saving} onclick={() => (typing = null)}>
											Cancel
										</Button>
									</div>
								</form>
							{:else}
								<div class="actions">
									<Button
										variant="outline"
										size="sm"
										aria-label="Enter coordinates for {entry.name}"
										onclick={() => startTyping(entry)}
									>
										Enter coordinates
									</Button>
								</div>
							{/if}
						{/if}
					{/if}
					{#if applyError?.id === entry.id}
						<ActionError message={applyError.message} />
					{/if}
					{#if conflict.latest && pending?.entry.id === entry.id}
						<div class="conflict">
							<SaveConflict
								noun="place"
								fields={conflict.fields(conflictRows)}
								onKeepMine={conflict.keepMine}
								onUseLatest={useLatest}
								busy={saving}
							/>
						</div>
					{/if}
				</li>
			{/each}
		</ul>
	{/if}
{/if}

<style>
	.summary {
		margin: 0 0 1rem;
		font-size: 0.875rem;
		color: var(--color-text-muted);
	}

	.unmapped-list {
		list-style: none;
		margin: 0;
		padding: 0;
		display: flex;
		flex-direction: column;
		gap: 0.75rem;
	}

	.unmapped-item {
		padding: 1rem 1.25rem;
		background: white;
		border: 1px solid var(--a11y-color-border);
		border-radius: 8px;
	}

	.unmapped-head {
		display: flex;
		flex-wrap: wrap;
		align-items: baseline;
		justify-content: space-between;
		gap: 0.25rem 1rem;
	}

	.unmapped-name {
		font-weight: 500;
		overflow-wrap: anywhere;
		min-width: 0;
	}

	.unmapped-people {
		margin-top: 0.25rem;
		font-size: 0.875rem;
	}

	.unmapped-people a {
		color: var(--color-link);
		overflow-wrap: anywhere;
	}

	.unmapped-count {
		font-size: 0.875rem;
		color: var(--color-text-muted);
	}

	.suggestions,
	.no-suggestions {
		margin: 0.5rem 0 0;
		font-size: 0.875rem;
		color: var(--color-text-muted);
	}

	.suggestions ul {
		margin: 0.25rem 0 0;
		padding-left: 1.25rem;
		display: flex;
		flex-direction: column;
		gap: 0.375rem;
	}

	.suggestions li {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 0.25rem 0.75rem;
	}

	.type-pair {
		margin-top: 0.5rem;
		max-width: 24rem;
	}

	.actions {
		display: flex;
		flex-wrap: wrap;
		gap: 0.5rem;
		margin-top: 0.5rem;
	}

	.conflict {
		margin-top: 0.75rem;
	}

	.notice {
		margin: 0 0 1rem;
		padding: 0.75rem;
		border-radius: 6px;
		font-size: 0.875rem;
		background: #f0fdf4;
		border: 1px solid #bbf7d0;
		color: var(--color-success);
	}

	.notice a {
		color: var(--color-link);
	}

	.withheld {
		margin: 0 0 1rem;
		font-size: 0.875rem;
		color: var(--color-text-muted);
	}

	.pair {
		font-family: ui-monospace, monospace;
		color: var(--color-text);
		overflow-wrap: anywhere;
	}
</style>
