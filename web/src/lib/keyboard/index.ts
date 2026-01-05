/**
 * Keyboard Shortcuts Module
 *
 * Provides vim-style keyboard shortcuts for the application.
 *
 * @example
 * ```svelte
 * <script lang="ts">
 *   import { createShortcutHandler, getShortcutsForContext } from '$lib/keyboard';
 *   import { goto } from '$app/navigation';
 *
 *   const { handleKeydown } = createShortcutHandler('global', {
 *     'go-home': () => goto('/'),
 *     'go-people': () => goto('/persons'),
 *     'focus-search': () => document.querySelector<HTMLInputElement>('#search')?.focus()
 *   });
 *
 *   // For displaying shortcuts in help overlay
 *   const shortcuts = getShortcutsForContext('global');
 * </script>
 *
 * <svelte:window on:keydown={handleKeydown} />
 * ```
 */

// Re-export types
export type { Shortcut, ShortcutContext } from './shortcuts';

// Re-export shortcut registry functions
export {
	DEFAULT_SHORTCUTS,
	getShortcutsForContext,
	getGlobalShortcuts,
	formatKeySequence,
	sequenceMatches,
	isPartialMatch
} from './shortcuts';

// Re-export hooks
export { useShortcuts, createShortcutHandler } from './useShortcuts.svelte';
