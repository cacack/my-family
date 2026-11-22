/**
 * An accounts-mode server beside the suite's own, for the specs that need
 * sign-in (the suite runs in open mode, playwright.config.ts): the same binary
 * on E2E_ACCOUNTS_PORT, with a fresh SQLite file. `startAccountsServer` makes an
 * owner by `myfamily user create` and signs it in on `owner`;
 * `startFreshAccountsServer` makes none, so the server answers its setup page
 * and logs the setup token. The specs run one at a time, so they share the
 * port; `stop` waits for the server to exit.
 */
import { expect, request as apiRequest, type APIRequestContext } from '@playwright/test';
import { spawn, execFileSync } from 'node:child_process';
import { randomBytes } from 'node:crypto';
import { mkdtempSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { ACCOUNTS_BASE_URL, BINARY, E2E_ACCOUNTS_PORT } from './seed';

export interface AccountsServer {
	baseURL: string;
	/** The owner's sign-in. */
	username: string;
	password: string;
	/** Signed in as the owner. */
	owner: APIRequestContext;
	stop(): Promise<void>;
}

export interface FreshAccountsServer {
	baseURL: string;
	/** The one-time token the server logged at startup. */
	setupToken: string;
	stop(): Promise<void>;
}

/** Made at runtime: no password literal lives in the repository. */
export function newPassword(): string {
	return randomBytes(18).toString('base64url');
}

// The server's startup line while it has no owner (cmd/myfamily/user.go).
const SETUP_TOKEN = /enter the setup token (\S+),/;

interface Launched {
	baseURL: string;
	/** Everything the server wrote to stderr so far. */
	log(): string;
	stop(): Promise<void>;
}

async function launch(beforeServe?: (env: NodeJS.ProcessEnv) => void): Promise<Launched> {
	const baseURL = ACCOUNTS_BASE_URL;

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
	beforeServe?.(env);
	// Echo's per-request lines go to stdout; startup failures surface on stderr,
	// which is passed through and kept for the setup token.
	const server = spawn(BINARY, ['serve'], { env, stdio: ['ignore', 'ignore', 'pipe'] });
	let log = '';
	server.stderr.on('data', (chunk: Buffer) => {
		log += chunk;
		process.stderr.write(chunk);
	});
	const exited = new Promise<void>((resolve) => server.once('exit', () => resolve()));

	const stop = async () => {
		server.kill();
		await exited;
		rmSync(dir, { recursive: true, force: true });
	};

	try {
		await expect.poll(status, { timeout: 30_000 }).toBe(200);
	} catch (e) {
		await stop();
		throw e;
	}
	return { baseURL, log: () => log, stop };
}

export async function startAccountsServer(): Promise<AccountsServer> {
	const username = 'e2e-owner';
	const password = newPassword();
	const { baseURL, stop } = await launch((env) => {
		execFileSync(BINARY, ['user', 'create', '--username', username], {
			env,
			input: `${password}\n`,
			stdio: ['pipe', 'ignore', 'pipe']
		});
	});

	try {
		const owner = await apiRequest.newContext({ baseURL });
		const signIn = await owner.post('/api/v1/auth/sign-in', { data: { username, password } });
		expect(signIn.status()).toBe(200);
		return {
			baseURL,
			username,
			password,
			owner,
			async stop() {
				await owner.dispose();
				await stop();
			}
		};
	} catch (e) {
		await stop();
		throw e;
	}
}

export async function startFreshAccountsServer(): Promise<FreshAccountsServer> {
	const { baseURL, log, stop } = await launch();
	// The token is logged before the server listens, so it is in by now.
	const setupToken = log().match(SETUP_TOKEN)?.[1];
	if (!setupToken) {
		await stop();
		throw new Error('E2E: the fresh accounts server logged no setup token');
	}
	return { baseURL, setupToken, stop };
}
