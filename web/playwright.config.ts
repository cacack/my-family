/**
 * End-to-end suite: a browser against the real single binary.
 *
 * `make binary` embeds the built SPA into the server, so the UI and the API are
 * same-origin behind one port - one `webServer` entry, no proxy, no separate
 * Vite dev server, and no API mocking anywhere in the suite.
 *
 * This config deliberately does NOT build the binary. `make test-e2e` depends on
 * `make binary`; making the config build too would put a multi-minute frontend
 * build inside Playwright's `webServer` timeout and hide which step actually
 * failed.
 */
import { defineConfig, devices } from '@playwright/test';
import { existsSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { BASE_URL, E2E_PORT, OUTPUT_DIR } from './e2e/seed';

const BINARY = fileURLToPath(new URL('../myfamily', import.meta.url));

if (!existsSync(BINARY)) {
	throw new Error(
		`E2E: no binary at ${BINARY}. Run "make binary" first, or "make test-e2e" which does.`
	);
}

export default defineConfig({
	testDir: 'e2e',
	outputDir: OUTPUT_DIR,
	globalSetup: './e2e/global-setup.ts',

	/**
	 * One worker, no parallelism. The specs share one server whose store is in
	 * memory, and merging a branch is terminal - serial execution is what keeps
	 * a failure legible rather than a race to explain.
	 */
	workers: 1,
	fullyParallel: false,

	/**
	 * No retries, on purpose. This suite mutates server state irreversibly: once
	 * the merge smoke has merged its branch, a second attempt would fail on
	 * `branch_not_active` and report the retry's symptom instead of the original
	 * failure. A fresh run costs one boot and tells the truth.
	 */
	retries: 0,
	forbidOnly: !!process.env.CI,

	reporter: process.env.CI
		? [['list'], ['html', { open: 'never' }]]
		: [['list']],

	use: {
		baseURL: BASE_URL,
		trace: 'retain-on-failure',
		screenshot: 'only-on-failure',
		video: 'off'
	},

	projects: [{ name: 'chromium', use: { ...devices['Desktop Chrome'] } }],

	webServer: {
		command: `"${BINARY}" serve`,
		// The API answers before the SPA is ever requested, so this is the
		// earliest honest readiness signal.
		url: `${BASE_URL}/api/v1/branches`,
		env: { PORT: String(E2E_PORT) },
		// In CI a stray listener on this port is a bug, not a convenience.
		reuseExistingServer: !process.env.CI,
		// Echo logs one line per asset request; piping them buries the test
		// report. Failures still surface, on stderr.
		stdout: 'ignore',
		stderr: 'pipe',
		timeout: 30_000
	}
});
