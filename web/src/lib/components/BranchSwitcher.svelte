<script lang="ts">
	/**
	 * Header control for choosing the research branch reads and writes are
	 * scoped to.
	 *
	 * The branch list is fetched lazily when the menu opens, so an installation
	 * that never uses branches pays nothing for this control — and a server with
	 * no branch registry configured (503) says so instead of failing silently.
	 *
	 * Only `active` branches are switch targets: `merged` and `archived` are
	 * terminal and their overlay rows are purged, so scoping to one would 404
	 * every person and family page.
	 */
	import { api, type ApiError, type Branch } from '$lib/api/client';
	import { activeBranch, switchBranch } from '$lib/stores/activeBranch.svelte';
	import * as DropdownMenu from '$lib/components/ui/dropdown-menu';
	import { Button } from '$lib/components/ui/button';

	let open = $state(false);
	let branches: Branch[] = $state([]);
	let loading = $state(false);
	let error: string | null = $state(null);

	const currentLabel = $derived(
		activeBranch.id === null ? 'Mainline' : (activeBranch.branch?.name ?? 'Research branch')
	);

	// Every open fires a fetch, so a slow one can still be in flight when the
	// next opens. Only the newest response may touch the menu's state.
	let loadGeneration = 0;

	async function loadBranches() {
		const generation = ++loadGeneration;
		loading = true;
		error = null;
		try {
			const result = await api.listBranches();
			if (generation !== loadGeneration) return;
			branches = result.items.filter((b) => b.status === 'active');
		} catch (e) {
			if (generation !== loadGeneration) return;
			const apiError = e as ApiError;
			error =
				apiError.status === 503
					? 'Branches are not configured on this server.'
					: apiError.message || 'Failed to load branches';
			branches = [];
		} finally {
			if (generation === loadGeneration) {
				loading = false;
			}
		}
	}

	function handleOpenChange(isOpen: boolean) {
		open = isOpen;
		if (isOpen) {
			loadBranches();
		}
	}

	function handleSelect(branch: Branch | null) {
		switchBranch(branch);
	}
</script>

<DropdownMenu.Root bind:open onOpenChange={handleOpenChange}>
	<DropdownMenu.Trigger>
		<!-- `child` so the Button *is* the trigger: the default would nest a
		     button inside bits-ui's own, and the aria-label would land on the
		     inner one rather than the control that takes focus. -->
		{#snippet child({ props })}
			<Button
				{...props}
				variant="outline"
				size="sm"
				class="max-w-[12rem] gap-1.5"
				aria-label="Switch research branch. Currently on {currentLabel}"
			>
				<svg
					class="size-4 shrink-0"
					viewBox="0 0 24 24"
					fill="none"
					stroke="currentColor"
					stroke-width="2"
					aria-hidden="true"
				>
					<line x1="6" y1="3" x2="6" y2="15" />
					<circle cx="18" cy="6" r="3" />
					<circle cx="6" cy="18" r="3" />
					<path d="M18 9a9 9 0 0 1-9 9" />
				</svg>
				<span class="truncate">{currentLabel}</span>
			</Button>
		{/snippet}
	</DropdownMenu.Trigger>
	<DropdownMenu.Content class="w-64" align="end">
		<DropdownMenu.Group>
			<DropdownMenu.GroupHeading>Research branch</DropdownMenu.GroupHeading>
			<DropdownMenu.Item onSelect={() => handleSelect(null)}>
				<span class="mark" aria-hidden="true">{activeBranch.id === null ? '✓' : ''}</span>
				<span class="truncate">Mainline</span>
			</DropdownMenu.Item>

			{#if loading}
				<DropdownMenu.Item disabled>Loading branches...</DropdownMenu.Item>
			{:else if error}
				<DropdownMenu.Item disabled>{error}</DropdownMenu.Item>
			{:else if branches.length === 0}
				<DropdownMenu.Item disabled>No active branches</DropdownMenu.Item>
			{:else}
				{#each branches as branch (branch.id)}
					<DropdownMenu.Item onSelect={() => handleSelect(branch)}>
						<span class="mark" aria-hidden="true">{activeBranch.id === branch.id ? '✓' : ''}</span>
						<span class="truncate">{branch.name}</span>
					</DropdownMenu.Item>
				{/each}
			{/if}
		</DropdownMenu.Group>

		<DropdownMenu.Separator />
		<!--
			The anchor must come through the `child` snippet, not be nested inside a
			default-rendered Item. bits-ui 2 removed the `href` prop; without `child`
			the Item renders its own element around the link, so it swallows the
			activation keypress and Enter never navigates - the entry works with a
			mouse and is dead to the keyboard.
		-->
		<DropdownMenu.Item>
			{#snippet child({ props })}
				<a href="/branches" {...props}>Manage branches</a>
			{/snippet}
		</DropdownMenu.Item>
	</DropdownMenu.Content>
</DropdownMenu.Root>

<style>
	.mark {
		display: inline-block;
		width: 1rem;
		flex-shrink: 0;
	}

	.truncate {
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
</style>
