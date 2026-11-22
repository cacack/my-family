/**
 * An accounts-mode server beside the suite's own, for the specs that need
 * someone signed in (the suite runs in open mode, playwright.config.ts): the
 * same binary on E2E_ACCOUNTS_PORT, with a fresh SQLite file and an owner made
 * by `myfamily user create`, signed in on `owner`. The specs run one at a
 * time, so they share the port; `stop` waits for the server to exit.
 */
import { expect, request as apiRequest, type APIRequestContext } from '@playwright/test';
import { spawn, execFileSync } from 'node:child_process';
import { randomBytes } from 'node:crypto';
import { mkdtempSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { BINARY, E2E_ACCOUNTS_PORT } from './seed';

export interface AccountsServer {
	baseURL: string;
	/** Signed in as the owner. */
	owner: APIRequestContext;
	stop(): Promise<void>;
}

export async function startAccountsServer(): Promise<AccountsServer> {
	const baseURL = `http://127.0.0.1:${E2E_ACCOUNTS_PORT}`;
	const username = 'e2e-owner';
	// Made at runtime: no password literal lives in the repository.
	const password = randomBytes(18).toString('base64url');

	// 0 while nothing listens. A stray listener would be seeded and checked instead.
	const status = () => fetch(`${baseURL}/api/v1/auth/status`).then((r) => r.status, () => 0);
	if ((await status()) !== 0) throw new Error(`E2E: something already listens on ${baseURL}`);

	const dir = mkdtempSync(join(tmpdir(), 'myfamily-e2e-accounts-'));
	const env = {
		...process.env,
		PORT: String(E2E_ACCOUNTS_PORT),
		SQLITE_PATH: join(dir, 'myfamily.db'),
		DATABASE_URL: '',
		DEMO_MODE: '',
		AUTH_MODE: 'accounts'
	};
	execFileSync(BINARY, ['user', 'create', '--username', username], {
		env,
		input: `${password}\n`,
		stdio: ['pipe', 'ignore', 'pipe']
	});
	// Echo's per-request lines go to stdout; startup failures surface on stderr.
	const server = spawn(BINARY, ['serve'], { env, stdio: ['ignore', 'ignore', 'inherit'] });
	const exited = new Promise<void>((resolve) => server.once('exit', () => resolve()));

	const shutDown = async () => {
		server.kill();
		await exited;
		rmSync(dir, { recursive: true, force: true });
	};

	try {
		await expect.poll(status, { timeout: 30_000 }).toBe(200);
		const owner = await apiRequest.newContext({ baseURL });
		const signIn = await owner.post('/api/v1/auth/sign-in', { data: { username, password } });
		expect(signIn.status()).toBe(200);
		return {
			baseURL,
			owner,
			async stop() {
				await owner.dispose();
				await shutDown();
			}
		};
	} catch (e) {
		await shutDown();
		throw e;
	}
}
