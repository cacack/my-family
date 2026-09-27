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
import { existsSync, mkdtempSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { BASE_URL, E2E_PORT, OUTPUT_DIR } from './e2e/seed';

const BINARY = fileURLToPath(new URL('../myfamily', import.meta.url));

if (!existsSync(BINARY)) {
	throw new Error(
		`E2E: no binary at ${BINARY}. Run "make binary" first, or "make test-e2e" which does.`
	);
}

/**
 * `serve` persists to SQLite by default, so the suite points it at a database
 * file of its own - fresh every run - rather than at `./myfamily.db`. Global
 * setup relies on starting from an empty store, and a developer's own data must
 * never be seeded into.
 *
 * This module is evaluated by the runner and again by every worker. Only the
 * runner creates the directory; it publishes the path through the environment,
 * which the workers inherit, so there is exactly one database per run. The
 * runner removes it on exit.
 */
const DB_DIR_ENV = 'MYFAMILY_E2E_DB_DIR';
if (!process.env[DB_DIR_ENV]) {
	const dir = mkdtempSync(join(tmpdir(), 'myfamily-e2e-'));
	process.env[DB_DIR_ENV] = dir;
	process.once('exit', () => rmSync(dir, { recursive: true, force: true }));
}
const SQLITE_PATH = join(process.env[DB_DIR_ENV] as string, 'myfamily.db');

export default defineConfig({
	testDir: 'e2e',
	outputDir: OUTPUT_DIR,
	globalSetup: './e2e/global-setup.ts',

	/**
	 * One worker, no parallelism. The specs share one server and one store, and
	 * merging a branch is terminal - serial execution is what keeps
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
		// SQLITE_PATH as well as PORT: see SQLITE_PATH above. DATABASE_URL and
		// DEMO_MODE are pinned empty so an exported value in the developer's shell
		// cannot redirect the suite to a real database or the demo tree.
		env: { PORT: String(E2E_PORT), SQLITE_PATH, DATABASE_URL: '', DEMO_MODE: '' },
		// In CI a stray listener on this port is a bug, not a convenience.
		reuseExistingServer: !process.env.CI,
		// Echo logs one line per asset request; piping them buries the test
		// report. Failures still surface, on stderr.
		stdout: 'ignore',
		stderr: 'pipe',
		timeout: 30_000
	}
});
