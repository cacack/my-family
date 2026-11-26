/**
 * App Config Store
 *
 * Fetches application configuration from the backend (e.g., demo mode status,
 * and whether the server's geocoder is enabled, #1562).
 * Loaded once on app startup via the root layout.
 */

interface AppConfig {
	demo_mode: boolean;
	/** Whether `GET /geocode` may be called; the lookup controls are hidden when not (#1607). */
	geocoder_enabled: boolean;
}

let config = $state<AppConfig>({
	demo_mode: false,
	geocoder_enabled: false
});

export async function loadAppConfig(): Promise<void> {
	try {
		const res = await fetch('/api/v1/config');
		if (res.ok) {
			const data = await res.json();
			config.demo_mode = data.demo_mode ?? false;
			config.geocoder_enabled = data.geocoder_enabled ?? false;
		}
	} catch {
		// Silently fail - defaults are safe
	}
}

export async function resetDemo(): Promise<boolean> {
	try {
		const res = await fetch('/api/v1/demo/reset', { method: 'POST' });
		return res.ok;
	} catch {
		return false;
	}
}

export function getAppConfig(): AppConfig {
	return config;
}
