<script lang="ts">
	import {
		DEFAULT_SHORTCUTS,
		type Shortcut,
		type ShortcutContext
	} from '$lib/keyboard/shortcuts';

	interface Props {
		open: boolean;
		onClose: () => void;
	}

	let { open = $bindable(), onClose }: Props = $props();

	let dialogRef: HTMLDivElement | undefined = $state();
	let closeButtonRef: HTMLButtonElement | undefined = $state();

	/**
	 * Context display names for section headings
	 */
	const contextLabels: Record<ShortcutContext, string> = {
		global: 'Global',
		pedigree: 'Pedigree View',
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

	/**
	 * Handle keyboard events
	 */
	function handleKeydown(e: KeyboardEvent) {
		if (!open) return;

		if (e.key === 'Escape') {
			e.preventDefault();
			onClose();
		} else if (e.key === 'Tab') {
			// Focus trap: keep focus within the dialog
			handleFocusTrap(e);
		}
	}

	/**
	 * Trap focus within the modal
	 */
	function handleFocusTrap(e: KeyboardEvent) {
		if (!dialogRef) return;

		const focusableElements = dialogRef.querySelectorAll<HTMLElement>(
			'button, [href], input, select, textarea, [tabindex]:not([tabindex="-1"])'
		);

		if (focusableElements.length === 0) return;

		const firstElement = focusableElements[0];
		const lastElement = focusableElements[focusableElements.length - 1];

		if (e.shiftKey) {
			// Shift + Tab: going backwards
			if (document.activeElement === firstElement) {
				e.preventDefault();
				lastElement.focus();
			}
		} else {
			// Tab: going forwards
			if (document.activeElement === lastElement) {
				e.preventDefault();
				firstElement.focus();
			}
		}
	}

	/**
	 * Handle backdrop click
	 */
	function handleBackdropClick(e: MouseEvent) {
		if (e.target === e.currentTarget) {
			onClose();
		}
	}

	/**
	 * Focus the close button when modal opens
	 */
	$effect(() => {
		if (open && closeButtonRef) {
			// Use requestAnimationFrame to ensure the element is rendered
			requestAnimationFrame(() => {
				closeButtonRef?.focus();
			});
		}
	});
</script>

<svelte:window on:keydown={handleKeydown} />

{#if open}
	<div
		class="keyboard-help-overlay"
		onclick={handleBackdropClick}
		onkeydown={(e) => e.key === 'Escape' && onClose()}
		role="presentation"
	>
		<div
			bind:this={dialogRef}
			role="dialog"
			aria-modal="true"
			aria-labelledby="keyboard-help-title"
			class="keyboard-help-dialog"
		>
			<header class="dialog-header">
				<h2 id="keyboard-help-title">Keyboard Shortcuts</h2>
				<button
					bind:this={closeButtonRef}
					class="close-btn"
					onclick={onClose}
					aria-label="Close keyboard shortcuts help"
				>
					<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
						<line x1="18" y1="6" x2="6" y2="18" />
						<line x1="6" y1="6" x2="18" y2="18" />
					</svg>
				</button>
			</header>

			<div class="dialog-content">
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

			<footer class="dialog-footer">
				<p class="hint">Press <kbd class="key">?</kbd> to toggle this help</p>
			</footer>
		</div>
	</div>
{/if}

<style>
	.keyboard-help-overlay {
		position: fixed;
		inset: 0;
		background: rgba(0, 0, 0, 0.6);
		z-index: 1000;
		display: flex;
		align-items: center;
		justify-content: center;
		padding: 1rem;
	}

	.keyboard-help-dialog {
		background: white;
		border-radius: 12px;
		box-shadow:
			0 20px 25px -5px rgba(0, 0, 0, 0.1),
			0 10px 10px -5px rgba(0, 0, 0, 0.04);
		max-width: 600px;
		width: 100%;
		max-height: 80vh;
		display: flex;
		flex-direction: column;
		overflow: hidden;
	}

	.dialog-header {
		display: flex;
		align-items: center;
		justify-content: space-between;
		padding: 1.25rem 1.5rem;
		border-bottom: 1px solid #e5e7eb;
	}

	.dialog-header h2 {
		margin: 0;
		font-size: 1.25rem;
		font-weight: 600;
		color: #111827;
	}

	.close-btn {
		width: 2rem;
		height: 2rem;
		border: none;
		background: transparent;
		border-radius: 6px;
		cursor: pointer;
		display: flex;
		align-items: center;
		justify-content: center;
		color: #6b7280;
		transition: all 0.15s;
	}

	.close-btn:hover {
		background: #f3f4f6;
		color: #111827;
	}

	.close-btn:focus {
		outline: 2px solid #3b82f6;
		outline-offset: 2px;
	}

	.close-btn svg {
		width: 1.25rem;
		height: 1.25rem;
	}

	.dialog-content {
		flex: 1;
		overflow-y: auto;
		padding: 1rem 1.5rem;
	}

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
		color: #6b7280;
		margin: 0 0 0.75rem;
		padding-bottom: 0.5rem;
		border-bottom: 1px solid #f3f4f6;
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
		background: #f9fafb;
		border: 1px solid #d1d5db;
		border-radius: 6px;
		font-family: ui-monospace, SFMono-Regular, 'SF Mono', Menlo, Consolas, monospace;
		font-size: 0.8125rem;
		font-weight: 500;
		color: #374151;
		box-shadow: 0 1px 2px rgba(0, 0, 0, 0.05);
	}

	.key-separator {
		font-size: 0.75rem;
		color: #9ca3af;
		margin: 0 0.25rem;
	}

	.shortcut-description {
		color: #4b5563;
		font-size: 0.875rem;
		text-align: right;
	}

	.dialog-footer {
		padding: 1rem 1.5rem;
		border-top: 1px solid #e5e7eb;
		background: #f9fafb;
	}

	.hint {
		margin: 0;
		font-size: 0.8125rem;
		color: #6b7280;
		text-align: center;
	}

	.hint .key {
		display: inline-flex;
		height: 1.5rem;
		min-width: 1.5rem;
		padding: 0 0.375rem;
		font-size: 0.75rem;
		vertical-align: middle;
	}

	/* Responsive styles for mobile */
	@media (max-width: 640px) {
		.keyboard-help-overlay {
			padding: 0.5rem;
		}

		.keyboard-help-dialog {
			max-height: 90vh;
			border-radius: 8px;
		}

		.dialog-header {
			padding: 1rem;
		}

		.dialog-header h2 {
			font-size: 1.125rem;
		}

		.dialog-content {
			padding: 0.75rem 1rem;
		}

		.shortcut-item {
			flex-direction: column;
			align-items: flex-start;
			gap: 0.25rem;
			padding: 0.625rem 0;
		}

		.shortcut-description {
			text-align: left;
			color: #6b7280;
			font-size: 0.8125rem;
		}

		.dialog-footer {
			padding: 0.75rem 1rem;
		}
	}

	/* Dark mode support (if needed in future) */
	@media (prefers-color-scheme: dark) {
		/* Dark mode styles can be added here */
	}
</style>
