<script lang="ts">
	/**
	 * "Look up" beside a place name (#1607): asks the server's geocoder
	 * (#1562) for candidate locations and lists them with their coordinates,
	 * on a small map, with the provider's attribution, which its data license
	 * requires wherever candidates are shown. Looking up writes nothing; the
	 * caller decides what a pick does. Closing the list picks nothing.
	 *
	 * Renders nothing unless the server has the geocoder enabled (`GET
	 * /config`) and the user may edit, so with it off no trace of it appears.
	 */
	import { api, type GeocodeCandidate } from '#lib/api/client.js';
	import { can } from '#lib/stores/access.svelte.js';
	import { getAppConfig } from '#lib/stores/appConfig.svelte.js';
	import { Button } from '#lib/components/ui/button/index.js';
	import { candidateInput, geocodeErrorMessage } from '#lib/utils/geocode.js';
	import ActionError from './ActionError.svelte';
	import GeocodeCandidateMap from './GeocodeCandidateMap.svelte';

	let {
		id,
		place,
		label,
		onPick,
		busy = false
	}: {
		/** Prefix of the ids inside, e.g. `'birth-place'`. */
		id: string;
		/** The place name to look up. */
		place: string;
		/** What the place is, for the controls' names: "Birth Place", "Boston". */
		label: string;
		/** Called with the candidate picked; the list closes first. */
		onPick: (candidate: GeocodeCandidate) => void;
		/** Disables picking, while the caller saves an earlier pick. */
		busy?: boolean;
	} = $props();

	const shown = $derived(getAppConfig().geocoder_enabled && can('editor'));
	const name = $derived(place.trim().split(/\s+/).join(' '));

	type Lookup =
		| { state: 'loading'; name: string }
		| { state: 'found'; name: string; candidates: GeocodeCandidate[] }
		| { state: 'failed'; name: string; message: string };
	let lookup = $state<Lookup | null>(null);
	// Answers to an earlier lookup that arrive after a later one are dropped.
	let sent = 0;
	let button: HTMLElement | null = $state(null);
	let title: HTMLElement | null = $state(null);

	async function look() {
		if (!name) return;
		const asked = name;
		const n = ++sent;
		lookup = { state: 'loading', name: asked };
		try {
			const { items } = await api.geocodePlace(asked);
			if (n !== sent) return;
			lookup = { state: 'found', name: asked, candidates: items };
		} catch (e) {
			if (n !== sent) return;
			lookup = { state: 'failed', name: asked, message: geocodeErrorMessage(e) };
		}
		title?.focus();
	}

	function close() {
		sent++;
		lookup = null;
		button?.focus();
	}

	function pick(candidate: GeocodeCandidate) {
		lookup = null;
		button?.focus();
		onPick(candidate);
	}

	function onKeydown(event: KeyboardEvent) {
		if (event.key === 'Escape' && lookup && !event.defaultPrevented) {
			event.preventDefault();
			close();
		}
	}

	const attribution = $derived(
		lookup?.state === 'found' ? [...new Set(lookup.candidates.map((c) => c.attribution))] : []
	);
</script>

{#if shown}
	<!-- svelte-ignore a11y_no_static_element_interactions -->
	<div class="geocode-lookup" onkeydown={onKeydown}>
		<Button
			bind:ref={button}
			type="button"
			variant="outline"
			size="sm"
			disabled={!name}
			aria-label="Look up coordinates for {label}"
			aria-expanded={!!lookup && lookup.state !== 'loading'}
			aria-controls="{id}-geocode"
			onclick={look}
		>
			{lookup?.state === 'loading' ? 'Looking up...' : 'Look up'}
		</Button>
		<div id="{id}-geocode">
			{#if lookup && lookup.state !== 'loading'}
				<section class="candidates" aria-labelledby="{id}-geocode-heading">
					<div class="candidates-head">
						<p class="candidates-title" id="{id}-geocode-heading" tabindex="-1" bind:this={title}>
							{lookup.state === 'failed'
								? `Lookup of “${lookup.name}” failed`
								: lookup.candidates.length
									? `Locations matching “${lookup.name}”`
									: `No locations matching “${lookup.name}”`}
						</p>
						<Button type="button" variant="ghost" size="sm" onclick={close}>Close</Button>
					</div>
					{#if lookup.state === 'failed'}
						<ActionError message={lookup.message} />
					{:else if lookup.candidates.length === 0}
						<p class="empty" role="status">
							The geocoder found no location for this name. Try a shorter or more common form of it, or
							enter the coordinates by hand.
						</p>
					{:else}
						<GeocodeCandidateMap candidates={lookup.candidates} />
						<ol class="candidate-list">
							{#each lookup.candidates as c, i (i)}
								{@const pair = candidateInput(c)}
								<li>
									<span class="candidate-n" aria-hidden="true">{i + 1}</span>
									<span class="candidate-text">
										<span class="candidate-name">{c.display_name}</span>
										<span class="pair">{pair.latitude}, {pair.longitude}</span>
									</span>
									<Button
										type="button"
										variant="outline"
										size="sm"
										disabled={busy}
										aria-label="Use this, {c.display_name}, {pair.latitude}, {pair.longitude}, for {label}"
										onclick={() => pick(c)}
									>
										Use this
									</Button>
								</li>
							{/each}
						</ol>
						<p class="attribution" data-testid="geocode-attribution">
							{#each attribution as text (text)}<span>{text}</span>{/each}
						</p>
					{/if}
				</section>
			{/if}
		</div>
	</div>
{/if}

<style>
	.geocode-lookup {
		margin-top: 0.5rem;
		min-width: 0;
	}

	.candidates {
		margin-top: 0.5rem;
		padding: 0.75rem;
		border: 1px solid var(--a11y-color-border);
		border-radius: 6px;
		background: white;
	}

	.candidates-head {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 0.5rem;
		margin-bottom: 0.5rem;
	}

	.candidates-title {
		margin: 0;
		font-size: 0.875rem;
		font-weight: 500;
		overflow-wrap: anywhere;
		min-width: 0;
	}

	.candidates-title:focus {
		outline: none;
	}

	.candidate-list {
		list-style: none;
		margin: 0.5rem 0 0;
		padding: 0;
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
	}

	.candidate-list li {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		font-size: 0.8125rem;
	}

	.candidate-n {
		flex: none;
		display: inline-flex;
		align-items: center;
		justify-content: center;
		width: 1.25rem;
		height: 1.25rem;
		border-radius: 999px;
		background: #3b82f6;
		color: white;
		font-size: 0.6875rem;
		font-weight: 600;
	}

	.candidate-text {
		display: flex;
		flex-direction: column;
		flex: 1;
		min-width: 0;
	}

	.candidate-name {
		overflow-wrap: anywhere;
	}

	.pair {
		font-variant-numeric: tabular-nums;
		color: var(--color-text-muted);
	}

	.empty,
	.attribution {
		margin: 0.5rem 0 0;
		font-size: 0.8125rem;
		color: var(--color-text-muted);
	}

	.attribution span + span::before {
		content: ' · ';
	}
</style>
