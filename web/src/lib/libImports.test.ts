/**
 * `#lib` subpath imports replace the $lib alias Kit 3 removed (#1197).
 *
 * `package.json` `imports` is the one place `#lib` is defined; Vite and
 * TypeScript resolve it natively. shadcn-svelte writes new components with
 * the `components.json` aliases, so each must sit under `#lib` and name a real
 * path, or `shadcn-svelte add` emits imports that do not build.
 */
import { describe, expect, it } from 'vitest';
import pkg from '../../package.json';
import components from '../../components.json';

const sources = import.meta.glob('/src/**/*.{ts,js,svelte}', {
	query: '?raw',
	import: 'default',
	eager: true
}) as Record<string, string>;

// A path exists if it is a source file (any extension) or a directory holding one.
// Uses the glob keys rather than node:fs, which web/ has no type definitions for.
const exists = (path: string) =>
	Object.keys(sources).some(
		(file) => file.startsWith(`${path}/`) || file.replace(/\.[^./]+$/, '') === path
	);

describe('#lib subpath imports', () => {
	it('maps #lib and #lib/* to src/lib', () => {
		expect(pkg.imports).toEqual({ '#lib': './src/lib', '#lib/*': './src/lib/*' });
	});

	it.each(Object.entries(components.aliases))(
		'components.json alias %s resolves under #lib',
		(name, alias) => {
			expect(alias).toMatch(/^#lib(\/|$)/);
			const target = alias.replace(/^#lib/, '/src/lib');
			// `hooks` is created on first use; the rest exist today.
			if (name !== 'hooks') expect(exists(target)).toBe(true);
		}
	);

	it('leaves no $lib specifier in src', () => {
		const offenders = Object.entries(sources)
			.filter(([, source]) => /['"`]\$lib[/'"`]/.test(source))
			.map(([file]) => file);
		expect(offenders).toEqual([]);
	});
});
