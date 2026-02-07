/**
 * Onboarding Settings Store
 *
 * Tracks whether the user has completed (or skipped) the onboarding wizard.
 * Settings persist to localStorage so the wizard only appears once.
 */

const STORAGE_KEY = 'onboarding-settings';

interface OnboardingSettings {
	completed: boolean;
}

// Default settings factory
function getDefaultSettings(): OnboardingSettings {
	return {
		completed: false
	};
}

// Load settings from localStorage
function loadSettings(): OnboardingSettings {
	if (typeof window === 'undefined') {
		return getDefaultSettings();
	}

	try {
		const stored = localStorage.getItem(STORAGE_KEY);
		if (stored) {
			const parsed = JSON.parse(stored) as Partial<OnboardingSettings>;
			const defaults = getDefaultSettings();

			return {
				completed:
					typeof parsed.completed === 'boolean' ? parsed.completed : defaults.completed
			};
		}
	} catch {
		// Invalid JSON, use defaults
	}

	return getDefaultSettings();
}

// Reactive state using Svelte 5 runes - using object to allow export
const onboardingState = $state<OnboardingSettings>({
	completed: false
});

// Initialize from localStorage (runs once on module load in browser)
if (typeof window !== 'undefined') {
	const settings = loadSettings();
	onboardingState.completed = settings.completed;
}

// Persist settings to localStorage when they change
$effect.root(() => {
	$effect(() => {
		if (typeof window === 'undefined') return;

		const settings: OnboardingSettings = {
			completed: onboardingState.completed
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
export function setOnboardingCompleted(value: boolean): void {
	onboardingState.completed = value;
}

export function resetOnboarding(): void {
	const defaults = getDefaultSettings();
	onboardingState.completed = defaults.completed;
}

// Export getters for current state (these are reactive when used in Svelte components)
export function getOnboardingCompleted(): boolean {
	return onboardingState.completed;
}

// Export state object for direct access in Svelte components
export { onboardingState };
