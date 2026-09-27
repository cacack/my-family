<script lang="ts">
	/**
	 * Compare two research snapshots: every mainline change recorded between
	 * their positions, oldest first.
	 *
	 * The two ids come from `?from=&to=` (see `snapshotCompareHref`). The server
	 * orders the pair itself - `older_first` says whether `snapshot1` is the
	 * older one - so the page always presents "older -> newer" whichever order
	 * the ids were given in.
	 *
	 * Every change is itemized: the server maps each event to an entity type
	 * (persons, families, sources and citations, but also names, life events,
	 * attributes, notes, media and research artifacts) or leaves it out of the
	 * change log altogether (snapshot markers, imports), so there is nothing
	 * left to count without listing (#827).
	 */
	import { page } from '$app/stores';
	import {
		api,
		type ApiError,
		type BranchChangeEntry,
		type Snapshot,
		type SnapshotComparisonResult
	} from '$lib/api/client';
	import DiffView from '$lib/components/DiffView.svelte';
	import MainlineNotice from '$lib/components/MainlineNotice.svelte';
	import { Badge } from '$lib/components/ui/badge';
	import { Label } from '$lib/components/ui/label';
	import { snapshotCompareHref } from '$lib/utils/snapshots';
	import {
		CHANGE_ENTITY_TYPES,
		ENTITY_TYPE_LABELS as ENTITY_LABELS,
		changeEntryLink
	} from '$lib/utils/changeEntries';

	type EntityType = BranchChangeEntry['entity_type'];
	type EntityFilter = EntityType | 'all';

	const fromId = $derived($page.url.searchParams.get('from') ?? '');
	const toId = $derived($page.url.searchParams.get('to') ?? '');

	let comparison = $state<SnapshotComparisonResult | null>(null);
	let loading = $state(true);
	let error: string | null = $state(null);
	let notFound = $state(false);
	let entityFilter = $state<EntityFilter>('all');
	let announcement = $state('');

	/** Why the requested pair cannot be compared at all, before asking the server. */
	const invalidReason = $derived(
		!fromId || !toId
			? 'Choose two snapshots to compare.'
			: fromId === toId
				? 'Choose two different snapshots - a snapshot compared with itself has no changes.'
				: null
	);

	const older: Snapshot | null = $derived(
		comparison ? (comparison.older_first ? comparison.snapshot1 : comparison.snapshot2) : null
	);
	const newer: Snapshot | null = $derived(
		comparison ? (comparison.older_first ? comparison.snapshot2 : comparison.snapshot1) : null
	);
	const itemized = $derived(comparison?.changes ?? []);
	const visible = $derived(
		entityFilter === 'all' ? itemized : itemized.filter((e) => e.entity_type === entityFilter)
	);
	const counts = $derived({
		created: itemized.filter((e) => e.action === 'created').length,
		updated: itemized.filter((e) => e.action === 'updated').length,
		deleted: itemized.filter((e) => e.action === 'deleted').length,
		merged: itemized.filter((e) => e.action === 'merged').length
	});
	/** Entity types actually present, so the filter never offers an empty choice. */
	const presentTypes = $derived(
		CHANGE_ENTITY_TYPES.filter((type) =>
			itemized.some((e) => e.entity_type === type)
		)
	);

	function plural(count: number, noun: string): string {
		return `${count} ${noun}${count === 1 ? '' : 's'}`;
	}

	const summary = $derived(
		itemized.length === 0
			? 'No changes between these snapshots.'
			: `${plural(itemized.length, 'change')}: ${counts.created} created, ${counts.updated} updated, ${counts.deleted} deleted${counts.merged > 0 ? `, ${counts.merged} merged` : ''}.`
	);

	function announce(message: string) {
		announcement = '';
		setTimeout(() => {
			announcement = message;
		}, 50);
	}

	function formatTimestamp(iso: string): string {
		return new Date(iso).toLocaleDateString('en-US', {
			month: 'short',
			day: 'numeric',
			year: 'numeric',
			hour: 'numeric',
			minute: '2-digit'
		});
	}

	function entityLink(entry: BranchChangeEntry): string | null {
		return changeEntryLink(entry);
	}

	// A soft navigation between two comparisons reuses this component, so a slow
	// first response could land after the second. A monotonic token orders them
	// (the same pattern as the branch comparison page).
	let comparisonRequest = 0;

	async function loadComparison(a: string, b: string) {
		const request = ++comparisonRequest;
		loading = true;
		error = null;
		notFound = false;
		entityFilter = 'all';
		try {
			const result = await api.compareSnapshots(a, b);
			if (request !== comparisonRequest) return;
			comparison = result;
		} catch (e) {
			if (request !== comparisonRequest) return;
			const apiError = e as ApiError;
			if (apiError.status === 404) {
				notFound = true;
			} else if (apiError.status === 400) {
				error = 'These snapshot links are malformed. Choose the snapshots again.';
			} else {
				error = apiError.message || 'Failed to compare snapshots';
			}
			comparison = null;
		} finally {
			if (request === comparisonRequest) {
				loading = false;
			}
		}
	}

	$effect(() => {
		const a = fromId;
		const b = toId;
		if (invalidReason) {
			// Invalidate any request still in flight for a previous pair.
			comparisonRequest++;
			comparison = null;
			loading = false;
			return;
		}
		loadComparison(a, b);
	});

	// Announce the outcome once per loaded comparison, not on every filter change.
	$effect(() => {
		if (comparison && !loading) {
			announce(`Comparison loaded. ${summary}`);
		}
	});
</script>

<svelte:head>
	<title>
		{older && newer ? `${older.name} to ${newer.name} | Snapshots` : 'Snapshot Comparison'} | My Family
	</title>
</svelte:head>

<div class="sr-only" role="status" aria-live="polite" aria-atomic="true" data-testid="announcer">
	{announcement}
</div>

{#snippet snapshotCard(label: string, snapshot: Snapshot)}
	<div class="endpoint">
		<span class="endpoint-label">{label}</span>
		<span class="endpoint-name">{snapshot.name}</span>
		{#if snapshot.description}
			<span class="endpoint-description">{snapshot.description}</span>
		{/if}
		<span class="endpoint-meta">
			<time datetime={snapshot.created_at}>{formatTimestamp(snapshot.created_at)}</time>
			&middot; position {snapshot.position}
		</span>
	</div>
{/snippet}

<div class="compare-page">
	<a href="/snapshots" class="back-link">&larr; All snapshots</a>

	<MainlineNotice
		surface="Snapshot comparison"
		detail="Snapshot comparisons list mainline changes only. Branch edits appear once the branch is merged."
	/>

	<h1>Snapshot comparison</h1>

	{#if invalidReason}
		<div class="state empty">
			<p>{invalidReason}</p>
			<p><a href="/snapshots">Back to snapshots</a></p>
		</div>
	{:else if loading}
		<div class="state" role="status" aria-live="polite">Loading comparison...</div>
	{:else if notFound}
		<div class="state empty">
			<h2>Snapshot not found</h2>
			<p>One of these snapshots may have been deleted. <a href="/snapshots">Choose again</a>.</p>
		</div>
	{:else if error}
		<div class="state error" role="alert">{error}</div>
	{:else if comparison && older && newer}
		<div class="endpoints">
			{@render snapshotCard('From (older)', older)}
			<span class="endpoint-arrow" aria-hidden="true">&rarr;</span>
			{@render snapshotCard('To (newer)', newer)}
		</div>
		{#if !comparison.older_first}
			<p class="note">
				Listed oldest first, so {older.name} is shown as the starting point.
				<a href={snapshotCompareHref(older.id, newer.id)}>Link to this order</a>
			</p>
		{/if}

		<p class="summary">{summary}</p>
		{#if comparison.has_more}
			<p class="truncated" role="note">
				This range holds more changes than can be compared at once, so only the earliest ones are
				shown. Compare snapshots that are closer together to see the rest.
			</p>
		{/if}

		{#if itemized.length > 0}
			{#if presentTypes.length > 1}
				<div class="filter">
					<Label for="entity-filter">Show</Label>
					<select id="entity-filter" class="native-select" bind:value={entityFilter}>
						<option value="all">All changes ({itemized.length})</option>
						{#each presentTypes as type (type)}
							<option value={type}>
								{ENTITY_LABELS[type]} changes ({itemized.filter((e) => e.entity_type === type)
									.length})
							</option>
						{/each}
					</select>
				</div>
			{/if}

			<ol class="change-list" aria-label="Changes, oldest first">
				{#each visible as entry (entry.id)}
					{@const link = entityLink(entry)}
					<li class="change-entry">
						<div class="change-head">
							<time class="change-time" datetime={entry.timestamp}>
								{formatTimestamp(entry.timestamp)}
							</time>
							<Badge
								variant={entry.action === 'deleted' ? 'destructive' : 'secondary'}
								class="capitalize"
							>
								{entry.action}
							</Badge>
						</div>
						<div class="change-body">
							<span class="entity-type">{ENTITY_LABELS[entry.entity_type]}</span>
							{#if link}
								<a href={link} class="entity-name">{entry.entity_name || 'Unnamed'}</a>
							{:else}
								<span class="entity-name" class:deleted={entry.action === 'deleted'}>{entry.entity_name || 'Unnamed'}</span>
							{/if}
						</div>
						{#if entry.changes && Object.keys(entry.changes).length > 0}
							<div class="change-diff">
								<DiffView changes={entry.changes} />
							</div>
						{/if}
					</li>
				{/each}
			</ol>
		{/if}
	{/if}
</div>

<style>
	.compare-page {
		max-width: 1000px;
		margin: 0 auto;
		padding: 1.5rem;
	}

	.back-link {
		display: inline-block;
		margin-bottom: 1rem;
		font-size: 0.875rem;
		color: #64748b;
		text-decoration: none;
	}

	.back-link:hover {
		color: #3b82f6;
	}

	h1 {
		margin: 0 0 1rem;
		font-size: 1.5rem;
		color: #1e293b;
	}

	.endpoints {
		display: flex;
		align-items: stretch;
		gap: 0.75rem;
		flex-wrap: wrap;
		margin-bottom: 1rem;
	}

	.endpoint {
		flex: 1 1 14rem;
		min-width: 0;
		display: flex;
		flex-direction: column;
		gap: 0.25rem;
		background: white;
		border: 1px solid #e2e8f0;
		border-radius: 8px;
		padding: 0.75rem 1rem;
	}

	.endpoint-label {
		font-size: 0.6875rem;
		text-transform: uppercase;
		letter-spacing: 0.04em;
		color: #64748b;
	}

	.endpoint-name {
		font-weight: 600;
		color: #1e293b;
		overflow-wrap: anywhere;
	}

	.endpoint-description {
		font-size: 0.8125rem;
		color: #475569;
		overflow-wrap: anywhere;
	}

	.endpoint-meta {
		font-size: 0.75rem;
		color: #64748b;
	}

	.endpoint-arrow {
		align-self: center;
		color: #94a3b8;
		font-size: 1.25rem;
	}

	.summary {
		margin: 0 0 0.5rem;
		font-weight: 600;
		color: #1e293b;
	}

	.note {
		margin: 0 0 0.75rem;
		font-size: 0.8125rem;
		color: #64748b;
	}

	.truncated {
		margin: 0 0 1rem;
		padding: 0.75rem;
		background: #fffbeb;
		border: 1px solid #fcd34d;
		border-radius: 6px;
		font-size: 0.8125rem;
		color: #92400e;
	}

	.filter {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		margin: 1rem 0;
	}

	.native-select {
		height: 2.25rem;
		padding: 0 0.5rem;
		border: 1px solid #cbd5e1;
		border-radius: 6px;
		background: white;
		font-size: 0.875rem;
		color: #1e293b;
	}

	.native-select:focus-visible {
		outline: 2px solid #3b82f6;
		outline-offset: 1px;
	}

	.change-list {
		list-style: none;
		margin: 0;
		padding: 0;
		display: flex;
		flex-direction: column;
		gap: 0.75rem;
	}

	.change-entry {
		background: white;
		border: 1px solid #e2e8f0;
		border-radius: 8px;
		padding: 0.75rem 1rem;
	}

	.change-head {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		flex-wrap: wrap;
	}

	.change-time {
		font-size: 0.75rem;
		color: #64748b;
	}

	.change-body {
		display: flex;
		align-items: baseline;
		gap: 0.5rem;
		margin-top: 0.375rem;
	}

	.entity-type {
		font-size: 0.6875rem;
		text-transform: uppercase;
		letter-spacing: 0.04em;
		color: #64748b;
	}

	.entity-name {
		font-weight: 600;
		color: #1e293b;
		text-decoration: none;
		overflow-wrap: anywhere;
	}

	a.entity-name:hover {
		color: #3b82f6;
		text-decoration: underline;
	}

	.entity-name.deleted {
		color: #64748b;
		text-decoration: line-through;
	}

	.change-diff {
		margin-top: 0.5rem;
	}

	.state {
		padding: 2rem;
		text-align: center;
		color: #64748b;
	}

	.state.error {
		color: #dc2626;
	}

	.state.empty {
		background: white;
		border: 1px dashed #cbd5e1;
		border-radius: 8px;
	}

	.state.empty h2 {
		margin: 0 0 0.375rem;
		font-size: 1rem;
		color: #1e293b;
	}

	.state.empty p {
		margin: 0.25rem 0 0;
		font-size: 0.875rem;
	}
</style>
