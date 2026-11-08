import adapter from '@sveltejs/adapter-static';
import { sveltekit } from '@sveltejs/kit/vite';
import { vitePreprocess } from '@sveltejs/vite-plugin-svelte';
import { defineConfig } from 'vitest/config';
import { svelteTesting } from '@testing-library/svelte/vite';
import tailwindcss from '@tailwindcss/vite';

export default defineConfig({
	plugins: [
		tailwindcss(),
		sveltekit({
			preprocess: vitePreprocess(),
			adapter: adapter({
				pages: 'build',
				assets: 'build',
				fallback: 'index.html',
				precompress: false,
				strict: true
			}),
			// Kit 3 dropped the built-in `$lib` alias in favour of `#lib` subpath
			// imports. This keeps the existing `$lib` imports (and shadcn-svelte's
			// components.json aliases) working until they are migrated.
			alias: { $lib: 'src/lib' },
			paths: {
				base: ''
			}
		}),
		svelteTesting()
	],
	server: {
		fs: {
			// `web/` is its own npm package, so Vite's file allowlist stops at it.
			// The branch-scope drift test imports `internal/api/openapi.yaml?raw`
			// to check the client's allowlist against the spec, so that one
			// sibling directory is opened up — and nothing else.
			allow: ['.', '../internal/api']
		}
	},
	test: {
		include: ['src/**/*.{test,spec}.{js,ts}'],
		environment: 'jsdom',
		globals: true,
		// Vitest blanks CSS by default, `?raw` imports included; the high-contrast
		// test reads app.css as text.
		css: { include: [/\/src\/app\.css/] },
		setupFiles: ['./src/lib/test/setup.ts']
	}
});
