/**
 * App Config Store
 *
 * Fetches application configuration from the backend (e.g., demo mode status).
 * Loaded once on app startup via the root layout.
 */

interface AppConfig {
	demo_mode: boolean;
}

let config = $state<AppConfig>({
	demo_mode: false
});

export async function loadAppConfig(): Promise<void> {
	try {
		const res = await fetch('/api/v1/config');
		if (res.ok) {
			const data = await res.json();
			config.demo_mode = data.demo_mode ?? false;
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
