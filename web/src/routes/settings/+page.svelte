<script lang="ts">
	/**
	 * The tree's privacy settings (owner only, ADR-008 §6): the living
	 * threshold, whether viewers see living people in full, and whether
	 * visitors may browse without signing in (ADR-007). The server keeps them
	 * beside the accounts, so they survive restarts. From here the owner also
	 * previews the tree as a viewer or a visitor sees it (#1413).
	 */
	import { goto } from '$app/navigation';
	import PageHeader from '#lib/components/PageHeader.svelte';
	import ActionError from '#lib/components/ActionError.svelte';
	import ErrorState from '#lib/components/ErrorState.svelte';
	import LoadingState from '#lib/components/LoadingState.svelte';
	import LiveRegion from '#lib/components/LiveRegion.svelte';
	import RoleNotice from '#lib/components/auth/RoleNotice.svelte';
	import { Button } from '#lib/components/ui/button/index.js';
	import { Input } from '#lib/components/ui/input/index.js';
	import { Label } from '#lib/components/ui/label/index.js';
	import { api, type ApiError, type PrivacySettings } from '#lib/api/client.js';

	type AnonymousAccess = PrivacySettings['anonymous_access'];
	import { can } from '#lib/stores/access.svelte.js';
	import { startPreview, type PreviewAs } from '#lib/stores/auth.svelte.js';

	// The range the server accepts (openapi.yaml, PrivacySettings).
	const MIN_YEARS = 50;
	const MAX_YEARS = 150;

	let liveRegion: LiveRegion | undefined = $state();

	let saved = $state<PrivacySettings | null>(null);
	let thresholdYears = $state(110);
	let viewersSeeLiving = $state(true);
	let anonymousAccess = $state<AnonymousAccess>('none');
	let loading = $state(true);
	let loadError: string | null = $state(null);
	let saving = $state(false);
	let saveError: string | null = $state(null);

	const owner = $derived(can('owner'));
	const changed = $derived(
		saved !== null &&
			(thresholdYears !== saved.living_threshold_years ||
				viewersSeeLiving !== saved.viewers_see_living ||
				anonymousAccess !== saved.anonymous_access)
	);

	function show(settings: PrivacySettings) {
		saved = settings;
		thresholdYears = settings.living_threshold_years;
		viewersSeeLiving = settings.viewers_see_living;
		anonymousAccess = settings.anonymous_access;
	}

	async function load() {
		loading = true;
		try {
			show(await api.getPrivacySettings());
			loadError = null;
		} catch (e) {
			const apiError = e as ApiError;
			loadError =
				apiError.status && apiError.status < 500
					? apiError.message
					: 'Failed to load the settings. Please try again.';
		} finally {
			loading = false;
		}
	}

	$effect(() => {
		if (owner) load();
	});

	/** Start the preview and look at the tree from its home page. */
	function preview(as: PreviewAs) {
		startPreview(as);
		goto('/');
	}

	async function save(event: SubmitEvent) {
		event.preventDefault();
		saving = true;
		saveError = null;
		try {
			show(
				await api.updatePrivacySettings({
					living_threshold_years: thresholdYears,
					viewers_see_living: viewersSeeLiving,
					anonymous_access: anonymousAccess
				})
			);
			liveRegion?.announce('Privacy settings saved');
		} catch (e) {
			saveError = (e as ApiError).message || 'Could not save the settings. Try again.';
		} finally {
			saving = false;
		}
	}
</script>

<svelte:head>
	<title>Settings | My Family</title>
</svelte:head>

<LiveRegion bind:this={liveRegion} />

<div class="settings-page">
	<PageHeader title="Settings" description="Who counts as living, what viewers may see of them, and whether visitors may look." />

	{#if !owner}
		<RoleNotice action="Changing the tree's settings" required="owner" />
	{:else if loading}
		<LoadingState label="Loading settings..." />
	{:else if loadError}
		<ErrorState message={loadError} onRetry={load} />
	{:else}
		<section class="panel" aria-labelledby="privacy-heading">
			<h2 id="privacy-heading">Privacy</h2>
			<form class="panel-form" onsubmit={save}>
				<div class="field">
					<Label for="living-threshold">Living threshold (years)</Label>
					<Input
						id="living-threshold"
						type="number"
						inputmode="numeric"
						required
						min={MIN_YEARS}
						max={MAX_YEARS}
						step={1}
						class="threshold-input"
						bind:value={thresholdYears}
						aria-describedby="living-threshold-hint"
					/>
					<p id="living-threshold-hint" class="hint">
						A person with no recorded death, burial or cremation counts as living unless they were born
						(or christened or baptized) more than this many years ago. Between {MIN_YEARS} and {MAX_YEARS}.
						An editor can override it for one person.
					</p>
				</div>
				<div class="field">
					<label class="check">
						<input type="checkbox" bind:checked={viewersSeeLiving} aria-describedby="viewers-see-living-hint" />
						Viewers see living people
					</label>
					<p id="viewers-see-living-hint" class="hint">
						When off, viewers see a living person as "Living", with no dates, places or other details, as
						visitors do. Editors and the owner always see everyone.
					</p>
				</div>
				<fieldset class="field visitors" aria-describedby="visitors-hint">
					<legend>Visitors who are not signed in</legend>
					<label class="check">
						<input type="radio" name="anonymous-access" value="none" bind:group={anonymousAccess} />
						Must sign in to see anything
					</label>
					<label class="check">
						<input type="radio" name="anonymous-access" value="viewer" bind:group={anonymousAccess} />
						May browse the tree as viewers
					</label>
					<div id="visitors-hint" class="hint">
						<p>
							Visitors will see people who have died, with their names, dates, places, families and sources, and
							the charts, map and search.
						</p>
						<p>
							Visitors will not see living people (each shows as "Living", with no name or details), records
							marked private, who made each change, or submitters' contact details. They cannot change, export or
							import anything.
						</p>
						<p><a href="#preview">Preview the tree as a visitor</a> to see exactly what they will see.</p>
					</div>
				</fieldset>
				{#if saveError}
					<ActionError message={saveError} />
				{/if}
				<div>
					<Button type="submit" disabled={saving || !changed}>
						{saving ? 'Saving...' : 'Save settings'}
					</Button>
				</div>
			</form>
		</section>
		<section id="preview" class="panel" aria-labelledby="preview-heading">
			<h2 id="preview-heading">Preview</h2>
			<p id="preview-hint" class="hint preview-intro">
				See the tree exactly as the server shows it to someone else, under the saved settings: the pages show
				only what they may see, and the changes they may not make are gone. A banner at the top ends the
				preview. You can preview visitors before letting them in.
			</p>
			<div class="preview-actions">
				<Button variant="outline" aria-describedby="preview-hint" onclick={() => preview('visitor')}>
					Preview as a visitor
				</Button>
				<Button variant="outline" aria-describedby="preview-hint" onclick={() => preview('viewer')}>
					Preview as a viewer
				</Button>
			</div>
		</section>
	{/if}
</div>

<style>
	.settings-page {
		max-width: var(--page-width-detail);
		margin: 0 auto;
		padding: 1.5rem;
	}

	.panel {
		background: white;
		border: 1px solid #e2e8f0;
		border-radius: 8px;
		padding: 1rem;
		margin-top: 1.5rem;
	}

	:global(body.high-contrast) .panel {
		background: var(--color-bg);
		border-color: var(--a11y-color-border);
	}

	.panel h2 {
		margin: 0 0 0.75rem;
		font-size: 0.8125rem;
		font-weight: 600;
		text-transform: uppercase;
		letter-spacing: 0.05em;
		color: var(--color-text-muted);
	}

	.panel-form {
		display: flex;
		flex-direction: column;
		gap: 1rem;
		max-width: 32rem;
	}

	.field {
		display: flex;
		flex-direction: column;
		gap: 0.375rem;
	}

	.field :global(.threshold-input) {
		max-width: 8rem;
	}

	.check {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		font-size: 0.875rem;
		font-weight: 500;
	}

	.check input {
		width: 1rem;
		height: 1rem;
	}

	.check input:focus-visible {
		outline: 2px solid var(--color-focus-ring);
		outline-offset: 2px;
	}

	.visitors {
		margin: 0;
		padding: 0;
		border: 0;
		min-width: 0;
	}

	.visitors legend {
		padding: 0;
		margin-bottom: 0.375rem;
		font-size: 0.875rem;
		font-weight: 500;
	}

	.preview-intro {
		max-width: 32rem;
	}

	.preview-actions {
		display: flex;
		flex-wrap: wrap;
		gap: 0.5rem;
		margin-top: 0.75rem;
	}

	.hint p {
		margin: 0 0 0.375rem;
	}

	.hint a {
		color: var(--color-link);
		text-decoration: underline;
	}

	.hint a:hover {
		color: var(--color-link-hover);
	}

	.hint {
		margin: 0;
		font-size: 0.8125rem;
		color: var(--color-text-muted);
	}
</style>
