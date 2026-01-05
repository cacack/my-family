<script lang="ts">
	import {
		accessibilityState,
		setFontSize,
		setHighContrast,
		setReducedMotion,
		type FontSize
	} from '$lib/stores/accessibilitySettings.svelte';
	import {
		keyboardState,
		setShortcutsEnabled
	} from '$lib/stores/keyboardSettings.svelte';

	interface Props {
		open: boolean;
		onClose: () => void;
	}

	let { open = $bindable(), onClose }: Props = $props();

	let dialogRef: HTMLDivElement | undefined = $state();
	let closeButtonRef: HTMLButtonElement | undefined = $state();

	/**
	 * Font size options with labels
	 */
	const fontSizeOptions: { value: FontSize; label: string; description: string }[] = [
		{ value: 'normal', label: 'Normal', description: 'Default text size' },
		{ value: 'large', label: 'Large', description: '25% larger' },
		{ value: 'larger', label: 'Larger', description: '50% larger' }
	];

	/**
	 * Check if system prefers reduced motion
	 */
	function getSystemPrefersReducedMotion(): boolean {
		if (typeof window === 'undefined') return false;
		return window.matchMedia('(prefers-reduced-motion: reduce)').matches;
	}

	const systemPrefersReducedMotion = $derived(getSystemPrefersReducedMotion());

	/**
	 * Handle keyboard events
	 */
	function handleKeydown(e: KeyboardEvent) {
		if (!open) return;

		if (e.key === 'Escape') {
			e.preventDefault();
			onClose();
		} else if (e.key === 'Tab') {
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
			if (document.activeElement === firstElement) {
				e.preventDefault();
				lastElement.focus();
			}
		} else {
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
			requestAnimationFrame(() => {
				closeButtonRef?.focus();
			});
		}
	});
</script>

<svelte:window on:keydown={handleKeydown} />

{#if open}
	<div
		class="accessibility-overlay"
		onclick={handleBackdropClick}
		onkeydown={(e) => e.key === 'Escape' && onClose()}
		role="presentation"
	>
		<div
			bind:this={dialogRef}
			role="dialog"
			aria-modal="true"
			aria-labelledby="accessibility-panel-title"
			class="accessibility-dialog"
		>
			<header class="dialog-header">
				<h2 id="accessibility-panel-title">Accessibility Settings</h2>
				<button
					bind:this={closeButtonRef}
					class="close-btn"
					onclick={onClose}
					aria-label="Close accessibility settings"
				>
					<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
						<line x1="18" y1="6" x2="6" y2="18" />
						<line x1="6" y1="6" x2="18" y2="18" />
					</svg>
				</button>
			</header>

			<div class="dialog-content">
				<!-- Font Size Section -->
				<fieldset class="settings-group">
					<legend class="group-title">
						<svg class="group-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
							<polyline points="4,7 4,4 20,4 20,7" />
							<line x1="9" y1="20" x2="15" y2="20" />
							<line x1="12" y1="4" x2="12" y2="20" />
						</svg>
						Font Size
					</legend>
					<p class="group-description">Adjust the text size throughout the application.</p>
					<div class="font-size-buttons" role="group" aria-label="Font size selection">
						{#each fontSizeOptions as option}
							<button
								type="button"
								class="font-size-btn"
								class:selected={accessibilityState.fontSize === option.value}
								onclick={() => setFontSize(option.value)}
								aria-pressed={accessibilityState.fontSize === option.value}
							>
								<span class="font-size-label">{option.label}</span>
								<span class="font-size-description">{option.description}</span>
							</button>
						{/each}
					</div>
				</fieldset>

				<!-- High Contrast Section -->
				<fieldset class="settings-group">
					<legend class="group-title">
						<svg class="group-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
							<circle cx="12" cy="12" r="10" />
							<path d="M12 2a10 10 0 0 1 0 20z" fill="currentColor" />
						</svg>
						High Contrast
					</legend>
					<p class="group-description">Increase color contrast for better visibility.</p>
					<label class="toggle-control">
						<input
							type="checkbox"
							checked={accessibilityState.highContrast}
							onchange={(e) => setHighContrast(e.currentTarget.checked)}
							class="toggle-input"
						/>
						<span class="toggle-switch" aria-hidden="true"></span>
						<span class="toggle-label">
							{accessibilityState.highContrast ? 'Enabled' : 'Disabled'}
						</span>
					</label>
				</fieldset>

				<!-- Reduced Motion Section -->
				<fieldset class="settings-group">
					<legend class="group-title">
						<svg class="group-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
							<rect x="2" y="6" width="20" height="12" rx="2" />
							<line x1="6" y1="12" x2="6" y2="12" stroke-linecap="round" />
							<line x1="10" y1="12" x2="10" y2="12" stroke-linecap="round" />
							<line x1="14" y1="12" x2="14" y2="12" stroke-linecap="round" />
							<line x1="18" y1="12" x2="18" y2="12" stroke-linecap="round" />
						</svg>
						Reduced Motion
					</legend>
					<p class="group-description">
						Minimize animations and transitions.
						{#if systemPrefersReducedMotion}
							<span class="system-preference-note">Your system prefers reduced motion.</span>
						{/if}
					</p>
					<label class="toggle-control">
						<input
							type="checkbox"
							checked={accessibilityState.reducedMotion}
							onchange={(e) => setReducedMotion(e.currentTarget.checked)}
							class="toggle-input"
						/>
						<span class="toggle-switch" aria-hidden="true"></span>
						<span class="toggle-label">
							{#if accessibilityState.reducedMotion}
								Enabled
								{#if systemPrefersReducedMotion}
									<span class="following-system">(following system)</span>
								{:else}
									<span class="overridden">(overridden)</span>
								{/if}
							{:else}
								Disabled
								{#if systemPrefersReducedMotion}
									<span class="overridden">(overriding system)</span>
								{/if}
							{/if}
						</span>
					</label>
				</fieldset>

				<!-- Keyboard Shortcuts Section -->
				<fieldset class="settings-group">
					<legend class="group-title">
						<svg class="group-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
							<rect x="2" y="6" width="20" height="12" rx="2" />
							<line x1="6" y1="10" x2="6" y2="10" stroke-linecap="round" />
							<line x1="10" y1="10" x2="14" y2="10" stroke-linecap="round" />
							<line x1="18" y1="10" x2="18" y2="10" stroke-linecap="round" />
							<line x1="8" y1="14" x2="16" y2="14" stroke-linecap="round" />
						</svg>
						Keyboard Shortcuts
					</legend>
					<p class="group-description">Enable or disable keyboard navigation shortcuts.</p>
					<label class="toggle-control">
						<input
							type="checkbox"
							checked={keyboardState.shortcutsEnabled}
							onchange={(e) => setShortcutsEnabled(e.currentTarget.checked)}
							class="toggle-input"
						/>
						<span class="toggle-switch" aria-hidden="true"></span>
						<span class="toggle-label">
							{keyboardState.shortcutsEnabled ? 'Enabled' : 'Disabled'}
						</span>
					</label>
				</fieldset>
			</div>

			<footer class="dialog-footer">
				<p class="footer-note">Changes are saved automatically.</p>
			</footer>
		</div>
	</div>
{/if}

<style>
	.accessibility-overlay {
		position: fixed;
		inset: 0;
		background: rgba(0, 0, 0, 0.6);
		z-index: 1000;
		display: flex;
		align-items: center;
		justify-content: center;
		padding: 1rem;
	}

	.accessibility-dialog {
		background: var(--color-bg, white);
		color: var(--color-text, #111827);
		border-radius: 12px;
		box-shadow:
			0 20px 25px -5px rgba(0, 0, 0, 0.1),
			0 10px 10px -5px rgba(0, 0, 0, 0.04);
		max-width: 480px;
		width: 100%;
		max-height: 85vh;
		display: flex;
		flex-direction: column;
		overflow: hidden;
	}

	.dialog-header {
		display: flex;
		align-items: center;
		justify-content: space-between;
		padding: 1.25rem 1.5rem;
		border-bottom: 1px solid var(--color-border, #e5e7eb);
	}

	.dialog-header h2 {
		margin: 0;
		font-size: 1.25rem;
		font-weight: 600;
		color: var(--color-text, #111827);
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
		color: var(--color-text-muted, #6b7280);
		transition: all var(--transition-duration, 0.15s);
	}

	.close-btn:hover {
		background: var(--color-bg-secondary, #f3f4f6);
		color: var(--color-text, #111827);
	}

	.close-btn:focus {
		outline: 2px solid var(--color-focus-ring, #3b82f6);
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

	.settings-group {
		border: none;
		padding: 0;
		margin: 0 0 1.5rem;
	}

	.settings-group:last-child {
		margin-bottom: 0;
	}

	.group-title {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		font-size: 0.9375rem;
		font-weight: 600;
		color: var(--color-text, #111827);
		margin-bottom: 0.25rem;
		padding: 0;
	}

	.group-icon {
		width: 1.125rem;
		height: 1.125rem;
		flex-shrink: 0;
	}

	.group-description {
		font-size: 0.8125rem;
		color: var(--color-text-muted, #6b7280);
		margin: 0 0 0.75rem;
		line-height: 1.4;
	}

	.system-preference-note {
		display: block;
		margin-top: 0.25rem;
		font-style: italic;
	}

	/* Font Size Buttons */
	.font-size-buttons {
		display: flex;
		gap: 0.5rem;
	}

	.font-size-btn {
		flex: 1;
		padding: 0.75rem 0.5rem;
		background: var(--color-bg-secondary, #f9fafb);
		border: 2px solid var(--color-border, #e5e7eb);
		border-radius: 8px;
		cursor: pointer;
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: 0.25rem;
		transition: all var(--transition-duration, 0.15s);
	}

	.font-size-btn:hover {
		border-color: var(--color-text-muted, #9ca3af);
	}

	.font-size-btn:focus {
		outline: 2px solid var(--color-focus-ring, #3b82f6);
		outline-offset: 2px;
	}

	.font-size-btn.selected {
		background: var(--color-focus-ring, #3b82f6);
		border-color: var(--color-focus-ring, #3b82f6);
		color: white;
	}

	.font-size-label {
		font-size: 0.875rem;
		font-weight: 600;
	}

	.font-size-description {
		font-size: 0.6875rem;
		opacity: 0.75;
	}

	/* Toggle Switch */
	.toggle-control {
		display: flex;
		align-items: center;
		gap: 0.75rem;
		cursor: pointer;
	}

	.toggle-input {
		position: absolute;
		opacity: 0;
		width: 0;
		height: 0;
	}

	.toggle-switch {
		position: relative;
		width: 44px;
		height: 24px;
		background: var(--color-border, #d1d5db);
		border-radius: 12px;
		flex-shrink: 0;
		transition: background var(--transition-duration, 0.15s);
	}

	.toggle-switch::after {
		content: '';
		position: absolute;
		top: 2px;
		left: 2px;
		width: 20px;
		height: 20px;
		background: white;
		border-radius: 50%;
		box-shadow: 0 1px 3px rgba(0, 0, 0, 0.2);
		transition: transform var(--transition-duration, 0.15s);
	}

	.toggle-input:checked + .toggle-switch {
		background: var(--color-focus-ring, #3b82f6);
	}

	.toggle-input:checked + .toggle-switch::after {
		transform: translateX(20px);
	}

	.toggle-input:focus + .toggle-switch {
		outline: 2px solid var(--color-focus-ring, #3b82f6);
		outline-offset: 2px;
	}

	.toggle-label {
		font-size: 0.875rem;
		color: var(--color-text, #374151);
	}

	.following-system,
	.overridden {
		font-size: 0.75rem;
		color: var(--color-text-muted, #6b7280);
		margin-left: 0.25rem;
	}

	/* Footer */
	.dialog-footer {
		padding: 0.75rem 1.5rem;
		border-top: 1px solid var(--color-border, #e5e7eb);
		background: var(--color-bg-secondary, #f9fafb);
	}

	.footer-note {
		margin: 0;
		font-size: 0.8125rem;
		color: var(--color-text-muted, #6b7280);
		text-align: center;
	}

	/* Responsive styles for mobile */
	@media (max-width: 640px) {
		.accessibility-overlay {
			padding: 0.5rem;
		}

		.accessibility-dialog {
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

		.font-size-buttons {
			flex-direction: column;
		}

		.font-size-btn {
			flex-direction: row;
			justify-content: space-between;
			padding: 0.75rem 1rem;
		}

		.dialog-footer {
			padding: 0.75rem 1rem;
		}
	}

	/* High contrast mode adjustments for this panel */
	:global(body.high-contrast) .accessibility-dialog {
		border: 2px solid var(--color-border);
	}

	:global(body.high-contrast) .toggle-switch {
		border: 2px solid var(--color-border);
	}

	:global(body.high-contrast) .font-size-btn.selected {
		background: var(--color-text);
		color: var(--color-bg);
	}
</style>
