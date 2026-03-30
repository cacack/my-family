<script lang="ts">
	import {
		DEFAULT_SHORTCUTS,
		type Shortcut,
		type ShortcutContext
	} from '$lib/keyboard/shortcuts';
	import * as Dialog from '$lib/components/ui/dialog';

	interface Props {
		open: boolean;
		onClose: () => void;
	}

	let { open = $bindable(), onClose }: Props = $props();

	/**
	 * Context display names for section headings
	 */
	const contextLabels: Record<ShortcutContext, string> = {
		global: 'Global',
		pedigree: 'Pedigree View',
		descendancy: 'Descendancy View',
		'person-detail': 'Person Detail',
		'family-detail': 'Family Detail',
		search: 'Search'
	};

	/**
	 * Order in which contexts should be displayed
	 */
	const contextOrder: ShortcutContext[] = [
		'global',
		'pedigree',
		'descendancy',
		'person-detail',
		'family-detail',
		'search'
	];

	/**
	 * Group shortcuts by context
	 */
	function getShortcutsByContext(): Map<ShortcutContext, Shortcut[]> {
		const groups = new Map<ShortcutContext, Shortcut[]>();

		for (const context of contextOrder) {
			const shortcuts = DEFAULT_SHORTCUTS.filter((s) => s.context === context);
			if (shortcuts.length > 0) {
				groups.set(context, shortcuts);
			}
		}

		return groups;
	}

	const shortcutGroups = $derived(getShortcutsByContext());
</script>

<Dialog.Root bind:open onOpenChange={(isOpen) => { if (!isOpen) onClose(); }}>
	<Dialog.Content class="sm:max-w-[600px] max-h-[80vh] flex flex-col overflow-hidden">
		<Dialog.Header>
			<Dialog.Title>Keyboard Shortcuts</Dialog.Title>
		</Dialog.Header>

		<div class="flex-1 overflow-y-auto">
			{#each contextOrder as context}
				{@const shortcuts = shortcutGroups.get(context)}
				{#if shortcuts && shortcuts.length > 0}
					<section class="shortcut-group">
						<h3 class="group-title">{contextLabels[context]}</h3>
						<ul class="shortcut-list">
							{#each shortcuts as shortcut}
								<li class="shortcut-item">
									<span class="shortcut-keys">
										{#each shortcut.keys as key, i}
											<kbd class="key">{key}</kbd>
											{#if i < shortcut.keys.length - 1}
												<span class="key-separator">then</span>
											{/if}
										{/each}
									</span>
									<span class="shortcut-description">{shortcut.description}</span>
								</li>
							{/each}
						</ul>
					</section>
				{/if}
			{/each}
		</div>

		<Dialog.Footer>
			<p class="hint">Press <kbd class="key">?</kbd> to toggle this help</p>
		</Dialog.Footer>
	</Dialog.Content>
</Dialog.Root>

<style>
	.shortcut-group {
		margin-bottom: 1.5rem;
	}

	.shortcut-group:last-child {
		margin-bottom: 0;
	}

	.group-title {
		font-size: 0.75rem;
		font-weight: 600;
		text-transform: uppercase;
		letter-spacing: 0.05em;
		color: var(--muted-foreground);
		margin: 0 0 0.75rem;
		padding-bottom: 0.5rem;
		border-bottom: 1px solid var(--border);
	}

	.shortcut-list {
		list-style: none;
		margin: 0;
		padding: 0;
	}

	.shortcut-item {
		display: flex;
		align-items: center;
		justify-content: space-between;
		padding: 0.5rem 0;
		gap: 1rem;
	}

	.shortcut-keys {
		display: flex;
		align-items: center;
		gap: 0.25rem;
		flex-shrink: 0;
	}

	.key {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		min-width: 1.75rem;
		height: 1.75rem;
		padding: 0 0.5rem;
		background: var(--muted);
		border: 1px solid var(--border);
		border-radius: 6px;
		font-family: ui-monospace, SFMono-Regular, 'SF Mono', Menlo, Consolas, monospace;
		font-size: 0.8125rem;
		font-weight: 500;
		color: var(--foreground);
		box-shadow: 0 1px 2px rgba(0, 0, 0, 0.05);
	}

	.key-separator {
		font-size: 0.75rem;
		color: var(--muted-foreground);
		margin: 0 0.25rem;
	}

	.shortcut-description {
		color: var(--muted-foreground);
		font-size: 0.875rem;
		text-align: right;
	}

	.hint {
		margin: 0;
		font-size: 0.8125rem;
		color: var(--muted-foreground);
		text-align: center;
		width: 100%;
	}

	.hint .key {
		display: inline-flex;
		height: 1.5rem;
		min-width: 1.5rem;
		padding: 0 0.375rem;
		font-size: 0.75rem;
		vertical-align: middle;
	}

	@media (max-width: 640px) {
		.shortcut-item {
			flex-direction: column;
			align-items: flex-start;
			gap: 0.25rem;
			padding: 0.625rem 0;
		}

		.shortcut-description {
			text-align: left;
			font-size: 0.8125rem;
		}
	}
</style>
