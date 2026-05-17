<script lang="ts">
	import { page } from '$app/stores';
	import { goto } from '$app/navigation';
	import {
		api,
		formatGenDate,
		formatPersonName,
		type GenDate,
		type MergePersonsRequest,
		type MergePersonsResponse,
		type PersonDetail
	} from '$lib/api/client';
	import { Button } from '$lib/components/ui/button';
	import {
		Card,
		CardContent,
		CardDescription,
		CardHeader,
		CardTitle
	} from '$lib/components/ui/card';
	import { RadioGroup, RadioGroupItem } from '$lib/components/ui/radio-group';
	import { Label } from '$lib/components/ui/label';

	const MERGEABLE_FIELDS = [
		'given_name',
		'surname',
		'gender',
		'birth_date',
		'birth_place',
		'death_date',
		'death_place',
		'notes',
		'research_status'
	] as const;
	type MergeableField = (typeof MERGEABLE_FIELDS)[number];
	type Side = 'survivor' | 'merged';

	const DATE_FIELDS: ReadonlySet<MergeableField> = new Set<MergeableField>([
		'birth_date',
		'death_date'
	]);

	// Pause after the success card lands before redirecting to the survivor page.
	// Long enough for the user to register the summary, short enough to feel snappy.
	const MERGE_REDIRECT_DELAY_MS = 1500;
	const ERROR_MESSAGE_MAX_LEN = 200;
	// Defense-in-depth: route params come from SvelteKit's matcher (which rejects
	// `/`), but we still constrain to a safe character set so a crafted ID can't
	// reshape the URL we hand to goto() or the API client. Permissive enough to
	// accept UUIDs, opaque IDs, and test fixtures like "p-survivor".
	const SAFE_ID_PATTERN = /^[A-Za-z0-9._-]+$/;

	let survivor = $state<PersonDetail | null>(null);
	let merged = $state<PersonDetail | null>(null);
	let loading = $state(true);
	let error = $state<string | null>(null);
	let submitting = $state(false);
	let result = $state<MergePersonsResponse | null>(null);
	let resolution = $state<Record<string, Side>>({});
	// Captured at load time so both the merge request payload and the post-merge
	// redirect bind to the trusted route params, not the IDs in the API response
	// body (which could be tampered with upstream).
	let routeSurvivorId = $state<string | null>(null);
	let routeMergedId = $state<string | null>(null);

	$effect(() => {
		const params = $page.params as Record<string, string | undefined>;
		const survivorId = params.survivorId;
		const mergedId = params.mergedId;
		if (survivorId && mergedId) {
			loadPersons(survivorId, mergedId);
		}
	});

	function truncateError(msg: unknown, fallback: string): string {
		const raw =
			typeof msg === 'string' && msg
				? msg
				: (msg as { message?: string } | null | undefined)?.message || fallback;
		return raw.length > ERROR_MESSAGE_MAX_LEN
			? `${raw.slice(0, ERROR_MESSAGE_MAX_LEN)}…`
			: raw;
	}

	async function loadPersons(survivorId: string, mergedId: string) {
		loading = true;
		error = null;
		survivor = null;
		merged = null;
		result = null;
		routeSurvivorId = null;
		routeMergedId = null;

		if (!SAFE_ID_PATTERN.test(survivorId) || !SAFE_ID_PATTERN.test(mergedId)) {
			error = 'Invalid person ID in URL.';
			loading = false;
			return;
		}

		if (survivorId === mergedId) {
			error = 'Cannot merge a person with themselves.';
			loading = false;
			return;
		}

		try {
			const [s, m] = await Promise.all([api.getPerson(survivorId), api.getPerson(mergedId)]);
			survivor = s;
			merged = m;
			routeSurvivorId = survivorId;
			routeMergedId = mergedId;
			resolution = buildInitialResolution(s, m);
		} catch (e) {
			error = truncateError(e, 'Failed to load one or both persons.');
			survivor = null;
			merged = null;
		} finally {
			loading = false;
		}
	}

	// All non-string non-primitive values on PersonDetail's mergeable fields are
	// GenDate-shaped (`birth_date`, `death_date`). If a future field adds another
	// object type, extend the type guard rather than the catch-all `return false`.
	function isGenDate(value: object): value is GenDate {
		return 'raw' in value;
	}

	function isEmpty(value: unknown): boolean {
		if (value === null || value === undefined) return true;
		if (typeof value === 'string') return value.trim() === '';
		if (typeof value === 'object') {
			if (!isGenDate(value)) return false;
			const raw = value.raw;
			if (raw === undefined || raw === null) return true;
			if (typeof raw === 'string' && raw.trim() === '') return true;
			return false;
		}
		return false;
	}

	function rawValue(person: PersonDetail | null, field: MergeableField): unknown {
		if (!person) return undefined;
		return (person as unknown as Record<string, unknown>)[field];
	}

	function buildInitialResolution(
		s: PersonDetail,
		m: PersonDetail
	): Record<string, Side> {
		const next: Record<string, Side> = {};
		for (const field of MERGEABLE_FIELDS) {
			const sVal = (s as unknown as Record<string, unknown>)[field];
			const mVal = (m as unknown as Record<string, unknown>)[field];
			if (isEmpty(sVal) && !isEmpty(mVal)) {
				next[field] = 'merged';
			} else {
				next[field] = 'survivor';
			}
		}
		return next;
	}

	function formatFieldName(field: string): string {
		return field
			.split('_')
			.map((w) => w.charAt(0).toUpperCase() + w.slice(1))
			.join(' ');
	}

	function displayValue(person: PersonDetail | null, field: MergeableField): string {
		const raw = rawValue(person, field);
		if (isEmpty(raw)) return '(empty)';
		if (DATE_FIELDS.has(field)) {
			return formatGenDate(raw as GenDate) || '(empty)';
		}
		return String(raw);
	}

	// Compare raw underlying values, not their formatted display strings — two
	// dates that format to the same case-folded text (e.g. "ABT 1850" vs "abt
	// 1850") can be semantically different and the user should be prompted to
	// pick a side.
	function fieldsAgree(field: MergeableField): boolean {
		if (!survivor || !merged) return true;
		const a = rawValue(survivor, field);
		const b = rawValue(merged, field);
		if (isEmpty(a) && isEmpty(b)) return true;
		if (typeof a === 'object' && a !== null && typeof b === 'object' && b !== null) {
			if (isGenDate(a) && isGenDate(b)) {
				return (a.raw ?? '') === (b.raw ?? '');
			}
			return JSON.stringify(a) === JSON.stringify(b);
		}
		return a === b;
	}

	async function handleMerge() {
		if (!survivor || !merged || !routeSurvivorId || !routeMergedId) return;
		const survivorTarget = routeSurvivorId;
		const mergedTarget = routeMergedId;
		submitting = true;
		error = null;
		try {
			const req: MergePersonsRequest = {
				// IDs come from the trusted route params, not the API response body
				// — defence-in-depth in case the response was tampered with.
				survivor_id: survivorTarget,
				merged_id: mergedTarget,
				survivor_version: survivor.version,
				merged_version: merged.version,
				field_resolution: { ...resolution }
			};
			result = await api.mergePersons(req);
			setTimeout(() => {
				goto(`/persons/${encodeURIComponent(survivorTarget)}`);
			}, MERGE_REDIRECT_DELAY_MS);
		} catch (e) {
			error = truncateError(e, 'Merge failed');
		} finally {
			submitting = false;
		}
	}

	function handleCancel() {
		goto('/quality');
	}
</script>

<svelte:head>
	<title>Merge Persons | My Family</title>
</svelte:head>

<div class="mx-auto max-w-5xl space-y-6 p-6">
	<header class="space-y-1">
		<a
			href="/quality"
			class="text-sm text-muted-foreground hover:text-foreground hover:underline"
		>
			&larr; Back to Quality
		</a>
		<h1 class="text-2xl font-semibold text-foreground">Merge Persons</h1>
		<p class="text-sm text-muted-foreground">
			Choose which value to keep for each field. The merged record will be deleted.
		</p>
	</header>

	{#if !result}
		<Card>
			<CardHeader>
				<CardTitle>Suggested merge</CardTitle>
				<CardDescription>Review fields below before merging.</CardDescription>
			</CardHeader>
		</Card>
	{/if}

	{#if error}
		<div
			role="alert"
			class="rounded-md border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700"
		>
			{error}
		</div>
	{/if}

	{#if loading}
		<div class="py-12 text-center text-sm text-muted-foreground">Loading persons&hellip;</div>
	{:else if result}
		<Card class="border-green-300 bg-green-50">
			<CardHeader>
				<CardTitle class="text-green-900">Merged successfully</CardTitle>
				<CardDescription class="text-green-800">
					{result.merge_summary.merged_person_name} was merged into the survivor.
				</CardDescription>
			</CardHeader>
			<CardContent class="space-y-2 text-sm text-green-900">
				<ul class="space-y-1">
					<li>
						Fields updated:
						<span class="font-medium">
							{result.merge_summary.fields_updated.length > 0
								? result.merge_summary.fields_updated.join(', ')
								: '(none)'}
						</span>
					</li>
					<li>
						Citations transferred:
						<span class="font-medium">{result.merge_summary.citations_transferred}</span>
					</li>
					<li>
						Names transferred:
						<span class="font-medium">{result.merge_summary.names_transferred}</span>
					</li>
					<li>
						Events transferred:
						<span class="font-medium">{result.merge_summary.events_transferred}</span>
					</li>
					<li>
						Media transferred:
						<span class="font-medium">{result.merge_summary.media_transferred}</span>
					</li>
					<li>
						Families updated:
						<span class="font-medium">{result.merge_summary.families_updated}</span>
					</li>
				</ul>
				{#if survivor && routeSurvivorId}
					<p class="pt-2">
						Redirecting to {formatPersonName(survivor)}&hellip;
						<a
							class="ml-1 underline hover:no-underline"
							href={`/persons/${encodeURIComponent(routeSurvivorId)}`}
						>
							Go now
						</a>
					</p>
				{/if}
			</CardContent>
		</Card>
	{:else if survivor && merged}
		<section class="grid grid-cols-1 gap-6 md:grid-cols-2">
			{@render personPanel(survivor, 'Survivor (keeps existing ID)')}
			{@render personPanel(merged, 'Will be merged & deleted')}
		</section>

		<Card>
			<CardHeader>
				<CardTitle>Field resolution</CardTitle>
				<CardDescription>
					Pick which side wins for each field. Rows where the two sides differ are
					highlighted.
				</CardDescription>
			</CardHeader>
			<CardContent class="space-y-3">
				{#each MERGEABLE_FIELDS as field (field)}
					{@const agree = fieldsAgree(field)}
					{@const survivorText = displayValue(survivor, field)}
					{@const mergedText = displayValue(merged, field)}
					<div
						class="rounded-md border p-3 {agree
							? 'border-slate-200 bg-slate-50'
							: 'border-amber-200 bg-amber-50'}"
					>
						<div class="mb-2 flex items-center justify-between gap-2">
							<span class="text-sm font-semibold text-foreground">
								{formatFieldName(field)}
							</span>
							{#if agree}
								<span class="text-xs text-muted-foreground">
									Both: {survivorText}
								</span>
							{:else}
								<span class="text-xs font-medium text-amber-700">Values differ</span>
							{/if}
						</div>
						<RadioGroup
							bind:value={resolution[field]}
							class="grid gap-2 sm:grid-cols-2"
						>
							<div class="flex items-start gap-2">
								<RadioGroupItem
									value="survivor"
									id={`field-${field}-survivor`}
								/>
								<Label
									for={`field-${field}-survivor`}
									class="cursor-pointer text-sm font-normal text-foreground"
								>
									<span class="block text-xs uppercase tracking-wide text-muted-foreground">
										Keep survivor
									</span>
									<span class="block break-words">{survivorText}</span>
								</Label>
							</div>
							<div class="flex items-start gap-2">
								<RadioGroupItem
									value="merged"
									id={`field-${field}-merged`}
								/>
								<Label
									for={`field-${field}-merged`}
									class="cursor-pointer text-sm font-normal text-foreground"
								>
									<span class="block text-xs uppercase tracking-wide text-muted-foreground">
										Keep merged
									</span>
									<span class="block break-words">{mergedText}</span>
								</Label>
							</div>
						</RadioGroup>
					</div>
				{/each}
			</CardContent>
		</Card>

		<div class="flex items-center justify-end gap-3">
			<Button variant="outline" onclick={handleCancel} disabled={submitting}>Cancel</Button>
			<Button
				onclick={handleMerge}
				disabled={loading || submitting || !survivor || !merged}
			>
				{submitting ? 'Merging…' : 'Merge persons'}
			</Button>
		</div>
	{/if}
</div>

{#snippet personPanel(person: PersonDetail, title: string)}
	<Card>
		<CardHeader>
			<CardTitle>{title}</CardTitle>
			<CardDescription>
				<span class="block text-base font-medium text-foreground">
					{formatPersonName(person)}
				</span>
				<span
					class="mt-1 inline-block rounded bg-slate-100 px-1.5 py-0.5 font-mono text-[0.7rem] text-slate-600"
				>
					{person.id}
				</span>
			</CardDescription>
		</CardHeader>
		<CardContent>
			<dl class="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1 text-sm">
				{#each MERGEABLE_FIELDS as field (field)}
					<dt class="text-xs uppercase tracking-wide text-muted-foreground">
						{formatFieldName(field)}
					</dt>
					<dd class="text-foreground break-words">{displayValue(person, field)}</dd>
				{/each}
			</dl>
		</CardContent>
	</Card>
{/snippet}
