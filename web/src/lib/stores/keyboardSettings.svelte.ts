/**
 * Keyboard Settings Store
 *
 * Manages keyboard shortcut preferences. Settings persist to localStorage.
 */

const STORAGE_KEY = 'keyboard-settings';

interface KeyboardSettings {
	shortcutsEnabled: boolean;
}

// Default settings factory
function getDefaultSettings(): KeyboardSettings {
	return {
		shortcutsEnabled: true
	};
}

// Load settings from localStorage
function loadSettings(): KeyboardSettings {
	if (typeof window === 'undefined') {
		return getDefaultSettings();
	}

	try {
		const stored = localStorage.getItem(STORAGE_KEY);
		if (stored) {
			const parsed = JSON.parse(stored) as Partial<KeyboardSettings>;
			const defaults = getDefaultSettings();

			return {
				shortcutsEnabled:
					typeof parsed.shortcutsEnabled === 'boolean'
						? parsed.shortcutsEnabled
						: defaults.shortcutsEnabled
			};
		}
	} catch {
		// Invalid JSON, use defaults
	}

	return getDefaultSettings();
}

// Reactive state using Svelte 5 runes - using object to allow export
const keyboardState = $state({
	shortcutsEnabled: true
});

// Initialize from localStorage (runs once on module load in browser)
if (typeof window !== 'undefined') {
	const settings = loadSettings();
	keyboardState.shortcutsEnabled = settings.shortcutsEnabled;
}

// Persist settings to localStorage when they change
$effect.root(() => {
	$effect(() => {
		if (typeof window === 'undefined') return;

		const settings: KeyboardSettings = {
			shortcutsEnabled: keyboardState.shortcutsEnabled
		};

		try {
			localStorage.setItem(STORAGE_KEY, JSON.stringify(settings));
		} catch {
			// Storage full or unavailable
		}
	});

	return () => {
		// Cleanup - no-op for this store
	};
});

// Setter functions
export function setShortcutsEnabled(enabled: boolean): void {
	keyboardState.shortcutsEnabled = enabled;
}

export function toggleShortcuts(): void {
	keyboardState.shortcutsEnabled = !keyboardState.shortcutsEnabled;
}

// Export getters for current state (these are reactive when used in Svelte components)
export function getShortcutsEnabled(): boolean {
	return keyboardState.shortcutsEnabled;
}

// Export state object for direct access in Svelte components
export { keyboardState };
