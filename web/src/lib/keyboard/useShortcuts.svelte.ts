/**
 * Keyboard Shortcuts Composable Hook
 *
 * Provides keyboard shortcut handling with sequence detection for Svelte 5 components.
 * Supports vim-style key sequences (e.g., `g h`) with configurable timeout.
 */

import { keyboardState } from '$lib/stores/keyboardSettings.svelte';
import {
	getShortcutsForContext,
	sequenceMatches,
	isPartialMatch,
	type ShortcutContext,
	type Shortcut
} from './shortcuts';

/** Timeout for key sequences in milliseconds */
const SEQUENCE_TIMEOUT_MS = 1000;

/**
 * Check if the currently focused element is an input-like element
 * where keyboard shortcuts should be ignored.
 */
function isInputElement(element: Element | null): boolean {
	if (!element) return false;

	const tagName = element.tagName.toLowerCase();
	if (tagName === 'input' || tagName === 'textarea' || tagName === 'select') {
		return true;
	}

	// Check for contenteditable
	if (element.getAttribute('contenteditable') === 'true') {
		return true;
	}

	return false;
}

/**
 * Controls whose native Enter/arrow-key behaviour a shortcut must not cancel,
 * including focusable scroll regions (arrow keys scroll them).
 */
const INTERACTIVE_SELECTOR =
	'a[href], button, summary, [role="button"], [role="link"], [role="menuitem"], [role="tab"], [role="region"][tabindex]';

/**
 * Check if the focused element is a link, button or similar control that
 * handles named keys (Enter, arrows) itself.
 */
function isInteractiveElement(element: Element | null): boolean {
	return element?.matches(INTERACTIVE_SELECTOR) ?? false;
}

/**
 * Normalize a keyboard event key to the form shortcuts are registered in:
 * printable characters are lower-cased (so Shift+R still means `r`), while
 * named keys such as `ArrowUp`, `Enter` and `Escape` keep their KeyboardEvent
 * spelling.
 */
function normalizeKey(event: KeyboardEvent): string {
	return event.key.length === 1 ? event.key.toLowerCase() : event.key;
}

/**
 * A shortcut handler. Returning `false` means it had nothing to do (say, no
 * dialog was open to close): the key then falls through to the next matching
 * action and is left for other listeners, rather than consumed.
 */
export type ShortcutHandler = () => unknown;

/**
 * Creates a keyboard shortcut handler for a specific context.
 *
 * Only shortcuts this instance has a handler for are matched, so a page's
 * instance and the layout's global one can share the window without one
 * consuming keys meant for the other. Events another listener has already
 * consumed (`defaultPrevented`, e.g. Escape closing a dropdown) are ignored.
 *
 * @param context - The shortcut context (e.g., 'global', 'pedigree')
 * @param handlers - Map of action names to handler functions
 * @returns Object with keydown handler and cleanup function
 *
 * @example
 * ```svelte
 * <script lang="ts">
 *   import { useShortcuts } from '$lib/keyboard';
 *   import { goto } from '$app/navigation';
 *
 *   const shortcuts = useShortcuts('global', {
 *     'go-home': () => goto('/'),
 *     'go-people': () => goto('/persons'),
 *     'focus-search': () => document.querySelector<HTMLInputElement>('#search')?.focus()
 *   });
 * </script>
 *
 * <svelte:window on:keydown={shortcuts.handleKeydown} />
 * ```
 */
export function useShortcuts(
	context: ShortcutContext,
	handlers: Record<string, ShortcutHandler>
): {
	handleKeydown: (event: KeyboardEvent) => void;
	cleanup: () => void;
	getPendingKeys: () => string[];
} {
	// State for sequence detection
	let pendingKeys: string[] = [];
	let lastKeyTime = 0;
	let timeoutId: ReturnType<typeof setTimeout> | null = null;

	// Shortcuts for this context that this instance can act on
	const shortcuts = getShortcutsForContext(context).filter((shortcut) => shortcut.action in handlers);

	/**
	 * Run the first of these actions whose handler does not decline.
	 * Returns whether one handled the key.
	 */
	function runFirst(actions: string[]): boolean {
		return actions.some((action) => {
			const handler = handlers[action];
			return handler !== undefined && handler() !== false;
		});
	}

	/**
	 * Clear pending key sequence
	 */
	function clearPendingKeys(): void {
		pendingKeys = [];
		if (timeoutId !== null) {
			clearTimeout(timeoutId);
			timeoutId = null;
		}
	}

	/**
	 * Handle a keydown event
	 */
	function handleKeydown(event: KeyboardEvent): void {
		// Skip if shortcuts are disabled globally, or another listener (a
		// dropdown or dialog) already consumed the key
		if (!keyboardState.shortcutsEnabled || event.defaultPrevented) {
			return;
		}

		// Skip if focus is in an input element
		if (isInputElement(document.activeElement)) {
			// Exception: Escape should always work to blur/close/cancel
			if (event.key === 'Escape') {
				// Try close-modal first (for modals), then cancel (for edit forms)
				if (runFirst(['close-modal', 'cancel'])) {
					event.preventDefault();
				}
			}
			return;
		}

		// Skip modifier key combinations (let browser handle them)
		if (event.ctrlKey || event.metaKey || event.altKey) {
			return;
		}

		const key = normalizeKey(event);

		// Leave named keys (Enter, arrows) to a focused link or button so its
		// native activation and scrolling still work; Escape stays global.
		if (key.length > 1 && key !== 'Escape' && isInteractiveElement(document.activeElement)) {
			clearPendingKeys();
			return;
		}
		const now = Date.now();

		// Check if we should continue a sequence or start fresh
		if (now - lastKeyTime > SEQUENCE_TIMEOUT_MS) {
			clearPendingKeys();
		}

		// Add key to pending sequence
		pendingKeys = [...pendingKeys, key];
		lastKeyTime = now;

		// Clear any existing timeout
		if (timeoutId !== null) {
			clearTimeout(timeoutId);
		}

		// Check for exact matches (Escape matches both close-modal and cancel)
		const matched = shortcuts.filter((shortcut) => sequenceMatches(pendingKeys, shortcut.keys));

		if (matched.length > 0) {
			clearPendingKeys();
			if (runFirst(matched.map((shortcut) => shortcut.action))) {
				event.preventDefault();
			}
			return;
		}

		// Check if this could be the start of a sequence
		if (isPartialMatch(pendingKeys, shortcuts)) {
			// Prevent default for single keys that are part of sequences
			// (e.g., 'g' should not type 'g' while waiting for next key)
			event.preventDefault();

			// Set timeout to clear pending keys
			timeoutId = setTimeout(() => {
				clearPendingKeys();
			}, SEQUENCE_TIMEOUT_MS);
			return;
		}

		// No match and not a partial match - clear and ignore
		clearPendingKeys();
	}

	/**
	 * Cleanup function to clear timeouts
	 */
	function cleanup(): void {
		clearPendingKeys();
	}

	/**
	 * Get current pending keys (useful for debugging/UI feedback)
	 */
	function getPendingKeys(): string[] {
		return [...pendingKeys];
	}

	return {
		handleKeydown,
		cleanup,
		getPendingKeys
	};
}

/**
 * Svelte 5 runes-based shortcut hook with automatic cleanup.
 *
 * Use this in a component's <script> block to automatically set up
 * keyboard shortcut handling with proper lifecycle management.
 *
 * @param context - The shortcut context
 * @param handlers - Map of action names to handler functions
 *
 * @example
 * ```svelte
 * <script lang="ts">
 *   import { createShortcutHandler } from '$lib/keyboard';
 *   import { goto } from '$app/navigation';
 *
 *   const { handleKeydown } = createShortcutHandler('global', {
 *     'go-home': () => goto('/'),
 *     'go-people': () => goto('/persons')
 *   });
 * </script>
 *
 * <svelte:window on:keydown={handleKeydown} />
 * ```
 */
export function createShortcutHandler(
	context: ShortcutContext,
	handlers: Record<string, ShortcutHandler>
): {
	handleKeydown: (event: KeyboardEvent) => void;
} {
	const shortcuts = useShortcuts(context, handlers);

	// Use $effect for automatic cleanup when component unmounts
	$effect(() => {
		return () => {
			shortcuts.cleanup();
		};
	});

	return {
		handleKeydown: shortcuts.handleKeydown
	};
}
