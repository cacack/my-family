/**
 * Keyboard Shortcut Registry
 *
 * Defines all keyboard shortcuts and their metadata. Shortcuts use vim-style
 * sequences (e.g., `g h` for "go home") to avoid conflicts with browser defaults.
 */

/**
 * Represents a keyboard shortcut definition
 */
export interface Shortcut {
	/** Key sequence to trigger the shortcut (e.g., ['g', 'h'] for "g h") */
	keys: string[];
	/** Unique action identifier used for handler lookup */
	action: string;
	/** Human-readable description for help overlay */
	description: string;
	/** Context where this shortcut is active */
	context: ShortcutContext;
}

/**
 * Available shortcut contexts
 */
export type ShortcutContext =
	| 'global'
	| 'pedigree'
	| 'person-detail'
	| 'family-detail'
	| 'search';

/**
 * Default keyboard shortcuts
 *
 * Design decisions:
 * - Vim-style sequences (g + key) avoid conflicts with browser shortcuts
 * - Single keys (/, ?, Escape) follow common conventions
 * - No Ctrl/Cmd combinations (reserved by browsers)
 * - No F1-F12 keys (reserved by system)
 */
export const DEFAULT_SHORTCUTS: Shortcut[] = [
	// Navigation shortcuts (global)
	{
		keys: ['g', 'h'],
		action: 'go-home',
		description: 'Go to home',
		context: 'global'
	},
	{
		keys: ['g', 'p'],
		action: 'go-people',
		description: 'Go to people list',
		context: 'global'
	},
	{
		keys: ['g', 'f'],
		action: 'go-families',
		description: 'Go to families list',
		context: 'global'
	},
	{
		keys: ['g', 's'],
		action: 'go-sources',
		description: 'Go to sources',
		context: 'global'
	},

	// Search shortcuts (global)
	{
		keys: ['/'],
		action: 'focus-search',
		description: 'Focus search box',
		context: 'global'
	},

	// Help shortcuts (global)
	{
		keys: ['?'],
		action: 'show-help',
		description: 'Show keyboard shortcuts help',
		context: 'global'
	},

	// Modal/cancel shortcuts (global)
	{
		keys: ['Escape'],
		action: 'close-modal',
		description: 'Close modal or cancel action',
		context: 'global'
	},

	// Pedigree chart navigation shortcuts
	{
		keys: ['ArrowUp'],
		action: 'navigate-father',
		description: 'Navigate to father',
		context: 'pedigree'
	},
	{
		keys: ['ArrowDown'],
		action: 'navigate-root',
		description: 'Navigate to root person',
		context: 'pedigree'
	},
	{
		keys: ['ArrowLeft'],
		action: 'navigate-mother',
		description: 'Navigate to mother',
		context: 'pedigree'
	},
	{
		keys: ['ArrowRight'],
		action: 'navigate-spouse',
		description: 'Navigate to first spouse/family',
		context: 'pedigree'
	},
	{
		keys: ['Enter'],
		action: 'view-person-detail',
		description: 'View selected person details',
		context: 'pedigree'
	},

	// Pedigree chart zoom shortcuts
	{
		keys: ['+'],
		action: 'zoom-in',
		description: 'Zoom in',
		context: 'pedigree'
	},
	{
		keys: ['='],
		action: 'zoom-in',
		description: 'Zoom in',
		context: 'pedigree'
	},
	{
		keys: ['-'],
		action: 'zoom-out',
		description: 'Zoom out',
		context: 'pedigree'
	},
	{
		keys: ['r'],
		action: 'reset-view',
		description: 'Reset view to center',
		context: 'pedigree'
	},

	// Person detail page shortcuts
	{
		keys: ['e'],
		action: 'edit',
		description: 'Enter edit mode',
		context: 'person-detail'
	},
	{
		keys: ['s'],
		action: 'save',
		description: 'Save changes',
		context: 'person-detail'
	},
	{
		keys: ['Escape'],
		action: 'cancel',
		description: 'Cancel edit / exit edit mode',
		context: 'person-detail'
	},

	// Family detail page shortcuts
	{
		keys: ['e'],
		action: 'edit',
		description: 'Enter edit mode',
		context: 'family-detail'
	},
	{
		keys: ['s'],
		action: 'save',
		description: 'Save changes',
		context: 'family-detail'
	},
	{
		keys: ['Escape'],
		action: 'cancel',
		description: 'Cancel edit / exit edit mode',
		context: 'family-detail'
	}
];

/**
 * Get all shortcuts for a specific context
 *
 * @param context - The context to filter by
 * @returns Array of shortcuts active in the given context
 */
export function getShortcutsForContext(context: ShortcutContext): Shortcut[] {
	return DEFAULT_SHORTCUTS.filter(
		(shortcut) => shortcut.context === context || shortcut.context === 'global'
	);
}

/**
 * Get all global shortcuts
 *
 * @returns Array of global shortcuts
 */
export function getGlobalShortcuts(): Shortcut[] {
	return DEFAULT_SHORTCUTS.filter((shortcut) => shortcut.context === 'global');
}

/**
 * Format a key sequence for display (e.g., ['g', 'h'] -> "g h")
 *
 * @param keys - Array of key names
 * @returns Formatted string for display
 */
export function formatKeySequence(keys: string[]): string {
	return keys.join(' ');
}

/**
 * Check if two key sequences match
 *
 * @param sequence - The sequence to check
 * @param shortcutKeys - The shortcut's key sequence
 * @returns True if sequences match
 */
export function sequenceMatches(sequence: string[], shortcutKeys: string[]): boolean {
	if (sequence.length !== shortcutKeys.length) {
		return false;
	}
	return sequence.every((key, index) => key === shortcutKeys[index]);
}

/**
 * Check if a sequence could be the start of a shortcut
 *
 * @param sequence - The partial sequence to check
 * @param shortcuts - Array of shortcuts to check against
 * @returns True if sequence could lead to a valid shortcut
 */
export function isPartialMatch(sequence: string[], shortcuts: Shortcut[]): boolean {
	if (sequence.length === 0) {
		return false;
	}
	return shortcuts.some((shortcut) => {
		if (sequence.length >= shortcut.keys.length) {
			return false;
		}
		return sequence.every((key, index) => key === shortcut.keys[index]);
	});
}
