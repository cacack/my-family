/**
 * Accessibility Settings Store
 *
 * Manages user accessibility preferences including font size, high contrast mode,
 * and reduced motion. Settings persist to localStorage and respect system preferences.
 */

const STORAGE_KEY = 'accessibility-settings';

export type FontSize = 'normal' | 'large' | 'larger';

interface AccessibilitySettings {
	fontSize: FontSize;
	highContrast: boolean;
	reducedMotion: boolean;
}

// Map font size to CSS class and scale value
const FONT_SIZE_CONFIG: Record<FontSize, { className: string; scale: number }> = {
	normal: { className: '', scale: 1 },
	large: { className: 'font-large', scale: 1.25 },
	larger: { className: 'font-larger', scale: 1.5 }
};

// Default settings factory
function getDefaultSettings(): AccessibilitySettings {
	// Check for system reduced motion preference
	const prefersReducedMotion =
		typeof window !== 'undefined' &&
		window.matchMedia('(prefers-reduced-motion: reduce)').matches;

	return {
		fontSize: 'normal',
		highContrast: false,
		reducedMotion: prefersReducedMotion
	};
}

// Load settings from localStorage
function loadSettings(): AccessibilitySettings {
	if (typeof window === 'undefined') {
		return getDefaultSettings();
	}

	try {
		const stored = localStorage.getItem(STORAGE_KEY);
		if (stored) {
			const parsed = JSON.parse(stored) as Partial<AccessibilitySettings>;
			const defaults = getDefaultSettings();

			// Merge with defaults and validate
			return {
				fontSize: isValidFontSize(parsed.fontSize) ? parsed.fontSize : defaults.fontSize,
				highContrast:
					typeof parsed.highContrast === 'boolean' ? parsed.highContrast : defaults.highContrast,
				reducedMotion:
					typeof parsed.reducedMotion === 'boolean' ? parsed.reducedMotion : defaults.reducedMotion
			};
		}
	} catch {
		// Invalid JSON, use defaults
	}

	return getDefaultSettings();
}

function isValidFontSize(value: unknown): value is FontSize {
	return value === 'normal' || value === 'large' || value === 'larger';
}

// Reactive state using Svelte 5 runes - using object to allow export
const accessibilityState = $state<AccessibilitySettings>({
	fontSize: 'normal',
	highContrast: false,
	reducedMotion: false
});

// Initialize from localStorage (runs once on module load in browser)
if (typeof window !== 'undefined') {
	const settings = loadSettings();
	accessibilityState.fontSize = settings.fontSize;
	accessibilityState.highContrast = settings.highContrast;
	accessibilityState.reducedMotion = settings.reducedMotion;
}

// Persist settings to localStorage when they change
$effect.root(() => {
	$effect(() => {
		if (typeof window === 'undefined') return;

		const settings: AccessibilitySettings = {
			fontSize: accessibilityState.fontSize,
			highContrast: accessibilityState.highContrast,
			reducedMotion: accessibilityState.reducedMotion
		};

		try {
			localStorage.setItem(STORAGE_KEY, JSON.stringify(settings));
		} catch {
			// Storage full or unavailable
		}
	});

	// Apply classes to document body when settings change
	$effect(() => {
		if (typeof document === 'undefined') return;

		const body = document.body;

		// Remove all font size classes
		body.classList.remove('font-large', 'font-larger');

		// Add current font size class
		const config = FONT_SIZE_CONFIG[accessibilityState.fontSize];
		if (config.className) {
			body.classList.add(config.className);
		}

		// Set CSS custom property for scale
		document.documentElement.style.setProperty('--font-size-scale', config.scale.toString());
	});

	$effect(() => {
		if (typeof document === 'undefined') return;

		const body = document.body;

		if (accessibilityState.highContrast) {
			body.classList.add('high-contrast');
		} else {
			body.classList.remove('high-contrast');
		}
	});

	$effect(() => {
		if (typeof document === 'undefined') return;

		const body = document.body;

		if (accessibilityState.reducedMotion) {
			body.classList.add('reduced-motion');
		} else {
			body.classList.remove('reduced-motion');
		}

		// Set CSS custom property for transition duration
		document.documentElement.style.setProperty(
			'--transition-duration',
			accessibilityState.reducedMotion ? '0s' : '0.15s'
		);
	});

	return () => {
		// Cleanup - no-op for this store
	};
});

// Setter functions
export function setFontSize(size: FontSize): void {
	accessibilityState.fontSize = size;
}

export function setHighContrast(enabled: boolean): void {
	accessibilityState.highContrast = enabled;
}

export function setReducedMotion(enabled: boolean): void {
	accessibilityState.reducedMotion = enabled;
}

export function toggleHighContrast(): void {
	accessibilityState.highContrast = !accessibilityState.highContrast;
}

export function toggleReducedMotion(): void {
	accessibilityState.reducedMotion = !accessibilityState.reducedMotion;
}

export function cycleFontSize(): void {
	const sizes: FontSize[] = ['normal', 'large', 'larger'];
	const currentIndex = sizes.indexOf(accessibilityState.fontSize);
	const nextIndex = (currentIndex + 1) % sizes.length;
	accessibilityState.fontSize = sizes[nextIndex];
}

export function resetToDefaults(): void {
	const defaults = getDefaultSettings();
	accessibilityState.fontSize = defaults.fontSize;
	accessibilityState.highContrast = defaults.highContrast;
	accessibilityState.reducedMotion = defaults.reducedMotion;
}

// Export getters for current state (these are reactive when used in Svelte components)
export function getFontSize(): FontSize {
	return accessibilityState.fontSize;
}

export function getHighContrast(): boolean {
	return accessibilityState.highContrast;
}

export function getReducedMotion(): boolean {
	return accessibilityState.reducedMotion;
}

// Export state object for direct access in Svelte components
export { accessibilityState };
