<script lang="ts">
	/**
	 * The auth audit log (owner only, ADR-007): sign-ins, failed sign-ins,
	 * invites, role changes, disables and password resets, newest first. The
	 * log is append-only; this page only reads it, a page at a time, paging by
	 * cursor (#1577).
	 */
	import PageHeader from '#lib/components/PageHeader.svelte';
	import ActionError from '#lib/components/ActionError.svelte';
	import EmptyState from '#lib/components/EmptyState.svelte';
	import ErrorState from '#lib/components/ErrorState.svelte';
	import LoadingState from '#lib/components/LoadingState.svelte';
	import RoleNotice from '#lib/components/auth/RoleNotice.svelte';
	import { Button } from '#lib/components/ui/button/index.js';
	import { api, type ApiError, type AuthAuditEntry } from '#lib/api/client.js';
	import { auth, can } from '#lib/stores/auth.svelte.js';
	import { AUTH_ACTION_LABELS, auditActor } from '#lib/utils/authAudit.js';

	const PAGE_SIZE = 50;

	let entries: AuthAuditEntry[] = $state([]);
	// The cursor each page shown so far was read with, newest page first; the
	// last is the page on screen. Going back to a newer page rereads its cursor.
	let cursors: (string | undefined)[] = $state([undefined]);
	let nextCursor: string | undefined = $state(undefined);
	let loading = $state(true);
	let loadError: string | null = $state(null);
	let paging = $state(false);
	let pageError: string | null = $state(null);

	const managing = $derived(auth.account !== null && can('owner'));
	const pageNumber = $derived(cursors.length);

	async function load() {
		loading = true;
		try {
			const page = await api.listAuthAudit({ limit: PAGE_SIZE });
			entries = page.items;
			cursors = [undefined];
			nextCursor = page.next_cursor;
			loadError = null;
		} catch (e) {
			const apiError = e as ApiError;
			loadError =
				apiError.status && apiError.status < 500
					? apiError.message
					: 'Failed to load the audit log. Please try again.';
		} finally {
			loading = false;
		}
	}

	/** Shows the page read with cursors[cursors.length - 1] of the given stack. */
	async function show(stack: (string | undefined)[]) {
		paging = true;
		pageError = null;
		try {
			const page = await api.listAuthAudit({ limit: PAGE_SIZE, cursor: stack[stack.length - 1] });
			entries = page.items;
			cursors = stack;
			nextCursor = page.next_cursor;
		} catch (e) {
			pageError = (e as ApiError).message || 'Could not load that page. Try again.';
		} finally {
			paging = false;
		}
	}

	const older = () => show([...cursors, nextCursor]);
	const newer = () => show(cursors.slice(0, -1));

	$effect(() => {
		if (managing) load();
	});

	function when(iso: string): string {
		return new Date(iso).toLocaleString(undefined, { dateStyle: 'medium', timeStyle: 'medium' });
	}
</script>

<svelte:head>
	<title>Audit Log | My Family</title>
</svelte:head>

<div class="audit-page">
	<PageHeader
		title="Audit Log"
		description="Sign-ins and every change to who can use the tree, newest first. Entries are never changed or removed."
		back={{ href: '/users', label: 'Users' }}
	/>

	{#if auth.phase === 'open'}
		<p class="notice" role="note">
			This server runs without sign-in (AUTH_MODE=open), so nothing is recorded here.
		</p>
	{:else if !managing}
		<RoleNotice action="Reading the audit log" required="owner" />
	{:else if loading}
		<LoadingState label="Loading the audit log..." />
	{:else if loadError}
		<ErrorState message={loadError} onRetry={load} />
	{:else if entries.length === 0}
		<EmptyState message="Nothing recorded yet" />
	{:else}
		<ol class="entries" aria-label="Audit log, newest first">
			{#each entries as entry, i (i)}
				<li class="entry" class:failed={entry.action === 'sign_in_failed'} data-testid="audit-entry">
					<div class="entry-head">
						<span class="action">{AUTH_ACTION_LABELS[entry.action]}</span>
						<time datetime={entry.at}>{when(entry.at)}</time>
					</div>
					<dl class="entry-fields">
						<div><dt>By</dt><dd>{auditActor(entry)}</dd></div>
						{#if entry.username}<div><dt>Account</dt><dd>{entry.username}</dd></div>{/if}
						{#if entry.detail}<div><dt>Detail</dt><dd>{entry.detail}</dd></div>{/if}
						{#if entry.client_ip}<div><dt>From</dt><dd>{entry.client_ip}</dd></div>{/if}
					</dl>
				</li>
			{/each}
		</ol>
		{#if pageNumber > 1 || nextCursor !== undefined}
			<nav class="pager" aria-label="Audit log pages">
				<Button variant="outline" disabled={paging || pageNumber === 1} onclick={newer}>Newer</Button>
				<p class="count">Page {pageNumber}</p>
				<Button variant="outline" disabled={paging || nextCursor === undefined} onclick={older}>Older</Button>
				{#if pageError}<ActionError message={pageError} />{/if}
			</nav>
		{/if}
	{/if}
</div>

<style>
	.audit-page {
		max-width: var(--page-width-list);
		margin: 0 auto;
		padding: 1.5rem;
	}

	.entries {
		list-style: none;
		margin: 0;
		padding: 0;
		background: white;
		border: 1px solid #e2e8f0;
		border-radius: 8px;
	}

	:global(body.high-contrast) .entries {
		background: var(--color-bg);
		border-color: var(--a11y-color-border);
	}

	.entry {
		padding: 0.75rem 1rem;
		border-top: 1px solid #e2e8f0;
	}

	.entry:first-child {
		border-top: none;
	}

	.entry-head {
		display: flex;
		flex-wrap: wrap;
		justify-content: space-between;
		gap: 0.25rem 1rem;
	}

	.action {
		font-weight: 600;
	}

	.failed .action {
		color: var(--color-destructive);
	}

	time {
		font-size: 0.8125rem;
		color: var(--color-text-muted);
	}

	.entry-fields {
		display: flex;
		flex-wrap: wrap;
		gap: 0.25rem 1.25rem;
		margin: 0.375rem 0 0;
		font-size: 0.8125rem;
	}

	.entry-fields div {
		display: flex;
		gap: 0.375rem;
		min-width: 0;
	}

	.entry-fields dt {
		flex-shrink: 0;
		white-space: nowrap;
		color: var(--color-text-muted);
	}

	.entry-fields dd {
		margin: 0;
		min-width: 0;
		overflow-wrap: anywhere;
	}

	.pager {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 0.75rem;
		margin-top: 1rem;
	}

	.count,
	.notice {
		margin: 0;
		font-size: 0.875rem;
		color: var(--color-text-muted);
	}
</style>
