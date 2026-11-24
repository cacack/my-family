<script lang="ts">
	/**
	 * Join two spellings of a place (ADR-009 §3, #1736). An editor picks the
	 * place whose records mean another place and the place they mean, sees the
	 * facts of both, and joins them; an alias listed under a picked place can be
	 * separated again. No record's text changes: the joined place's records keep
	 * their spelling and read the other place's facts.
	 *
	 * A viewer is not offered it. On a research branch it is withdrawn, since
	 * aliases are main-only and every branch reads them.
	 */
	import { onMount } from 'svelte';
	import { api, type Place, type PlaceAliasEntry } from '#lib/api/client.js';
	import { can } from '#lib/stores/access.svelte.js';
	import { activeBranch } from '#lib/stores/activeBranch.svelte.js';
	import { Button } from '#lib/components/ui/button/index.js';
	import PlaceCoordinates from '#lib/components/PlaceCoordinates.svelte';
	import LoadingState from '#lib/components/LoadingState.svelte';
	import ErrorState from '#lib/components/ErrorState.svelte';

	/** The most places one picker lists; typing narrows it. */
	const SHOWN = 50;

	const mayJoin = $derived(can('editor') && !activeBranch.id);

	let places: Place[] = $state([]);
	let loading = $state(true);
	let loadError: string | null = $state(null);
	let busy = $state(false);
	let error: string | null = $state(null);
	let notice: string | null = $state(null);

	const sides = $state([
		{
			key: 'absorbed',
			label: 'Place to join',
			hint: 'Its records will belong to the other place.',
			filter: '',
			id: ''
		},
		{
			key: 'target',
			label: 'Place it means',
			hint: 'Records of both spellings will belong here.',
			filter: '',
			id: ''
		}
	]);

	const picked = $derived(sides.map((side) => places.find((p) => p.id === side.id)));
	const absorbed = $derived(picked[0]);
	const target = $derived(picked[1]);

	/** The places a picker lists: those matching its filter, and the one it holds. */
	function matching(filter: string, selectedId: string): Place[] {
		const q = filter.trim().toLowerCase();
		const hits = places.filter(
			(p) => !q || p.name.toLowerCase().includes(q) || p.key.includes(q)
		);
		const shown = hits.slice(0, SHOWN);
		const selected = places.find((p) => p.id === selectedId);
		if (selected && !shown.includes(selected)) shown.unshift(selected);
		return shown;
	}

	/** Every place in use on the mainline, page by page. */
	async function load() {
		loading = true;
		loadError = null;
		try {
			const all: Place[] = [];
			for (;;) {
				const page = await api.listPlaces({ limit: 100, offset: all.length });
				all.push(...page.items);
				if (page.items.length === 0 || all.length >= page.total) break;
			}
			places = all;
			for (const side of sides) {
				if (!all.some((p) => p.id === side.id)) side.id = '';
			}
		} catch (e) {
			console.error('Failed to load places:', e);
			loadError = 'Failed to load places. Please try again.';
		} finally {
			loading = false;
		}
	}

	async function run(action: () => Promise<string>) {
		busy = true;
		error = null;
		notice = null;
		try {
			notice = await action();
			await load();
		} catch (e) {
			error = (e as { message?: string }).message || 'The change failed.';
		} finally {
			busy = false;
		}
	}

	function join() {
		const [from, to] = [absorbed, target];
		if (!from || !to) return;
		run(async () => {
			await api.aliasPlace(from.id, to.id, from.version);
			sides[0].id = '';
			return `Records of ${from.name} now belong to ${to.name}.`;
		});
	}

	function separate(alias: PlaceAliasEntry, from: Place) {
		run(async () => {
			const place = await api.getPlace(alias.id);
			await api.unaliasPlace(alias.id, place.version);
			return `${alias.name} is separate from ${from.name} again.`;
		});
	}

	onMount(() => {
		if (mayJoin) load();
	});
</script>

{#if !can('editor')}
	<p class="withheld">Only editors can join or separate places.</p>
{:else if activeBranch.id}
	<p class="withheld" role="note">
		Joined places are shared across all branches. Return to the mainline to join or separate
		places.
	</p>
{:else if loading && places.length === 0}
	<LoadingState label="Loading places..." />
{:else if loadError}
	<ErrorState message={loadError} onRetry={load} />
{:else}
	<div class="place-join">
		{#if error}
			<div class="form-error" role="alert">{error}</div>
		{/if}
		{#if notice}
			<div class="form-notice" role="status">{notice}</div>
		{/if}

		<div class="sides">
			{#each sides as side, i (side.key)}
				{@const place = picked[i]}
				<section class="side" aria-labelledby="{side.key}-heading">
					<h2 id="{side.key}-heading">{side.label}</h2>
					<p class="hint">{side.hint}</p>
					<label class="field">
						<span>Search places</span>
						<input type="search" bind:value={side.filter} autocomplete="off" />
					</label>
					<label class="field">
						<span>{side.label}</span>
						<select size="6" bind:value={side.id} data-testid="{side.key}-select">
							{#each matching(side.filter, side.id) as option (option.id)}
								<option value={option.id}>{option.name} ({option.count})</option>
							{/each}
						</select>
					</label>

					{#if place}
						<dl class="facts" data-testid="{side.key}-facts">
							<dt>Name</dt>
							<dd>{place.name}</dd>
							<dt>Key</dt>
							<dd class="key">{place.key}</dd>
							<dt>Uses</dt>
							<dd>{place.count}</dd>
							<dt>Coordinates</dt>
							<dd>
								{#if place.latitude && place.longitude}
									<PlaceCoordinates
										id="{side.key}-coordinates"
										latitude={place.latitude}
										longitude={place.longitude}
										origin={place.coordinates_origin}
									/>
								{:else}
									None
								{/if}
							</dd>
						</dl>
						{#if place.aliases?.length}
							<h3>Joined spellings</h3>
							<ul class="aliases">
								{#each place.aliases as alias (alias.id)}
									<li>
										<span>{alias.name}</span>
										<Button
											variant="outline"
											size="sm"
											disabled={busy}
											onclick={() => separate(alias, place)}
										>
											Separate
										</Button>
									</li>
								{/each}
							</ul>
						{/if}
					{/if}
				</section>
			{/each}
		</div>

		{#if absorbed && target && absorbed.id === target.id}
			<p class="hint">Pick two different places.</p>
		{/if}
		{#if absorbed && target && absorbed.id !== target.id}
			<p class="summary">
				Every record of <strong>{absorbed.name}</strong> will belong to
				<strong>{target.name}</strong> and read its name and coordinates. Each record keeps
				the spelling it was given. {absorbed.name}'s own facts are kept and ignored while they
				are joined; copy any you need across first.
			</p>
		{/if}
		<div class="actions">
			<Button
				disabled={busy || !absorbed || !target || absorbed.id === target.id}
				onclick={join}
			>
				{busy ? 'Saving...' : 'Join places'}
			</Button>
		</div>
	</div>
{/if}

<style>
	.place-join {
		display: flex;
		flex-direction: column;
		gap: 1rem;
	}

	.sides {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(min(100%, 18rem), 1fr));
		gap: 1rem;
	}

	.side {
		min-width: 0;
		padding: 1rem;
		border: 1px solid var(--border);
		border-radius: 8px;
		background: var(--card);
	}

	h2 {
		margin: 0;
		font-size: 1.0625rem;
	}

	h3 {
		margin: 1rem 0 0.5rem;
		font-size: 0.9375rem;
	}

	.hint,
	.withheld {
		margin: 0.25rem 0 0.75rem;
		font-size: 0.875rem;
		color: var(--color-text-muted);
	}

	.field {
		display: flex;
		flex-direction: column;
		gap: 0.25rem;
		margin-bottom: 0.75rem;
		font-size: 0.875rem;
	}

	.field input,
	.field select {
		width: 100%;
		padding: 0.375rem 0.5rem;
		border: 1px solid var(--input);
		border-radius: 6px;
		background: var(--background);
		color: var(--foreground);
		font-size: 0.875rem;
	}

	.facts {
		display: grid;
		grid-template-columns: max-content minmax(0, 1fr);
		gap: 0.25rem 0.75rem;
		margin: 0;
		font-size: 0.875rem;
	}

	.facts dt {
		color: var(--color-text-muted);
	}

	.facts dd {
		margin: 0;
		overflow-wrap: anywhere;
	}

	.facts dd :global(.place-coordinates) {
		margin: 0;
	}

	.key {
		font-family: ui-monospace, monospace;
	}

	.aliases {
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
		margin: 0;
		padding: 0;
		list-style: none;
		font-size: 0.875rem;
	}

	.aliases li {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 0.5rem;
		overflow-wrap: anywhere;
	}

	.summary {
		margin: 0;
		font-size: 0.875rem;
	}

	.actions {
		display: flex;
		justify-content: flex-end;
	}

	.form-error,
	.form-notice {
		padding: 0.75rem;
		border-radius: 6px;
		font-size: 0.875rem;
	}

	.form-error {
		background: #fef2f2;
		border: 1px solid #fecaca;
		color: #b91c1c;
	}

	.form-notice {
		background: #f0fdf4;
		border: 1px solid #bbf7d0;
		color: var(--color-success);
	}
</style>
