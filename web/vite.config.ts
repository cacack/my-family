import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig } from 'vitest/config';
import { svelteTesting } from '@testing-library/svelte/vite';
import tailwindcss from '@tailwindcss/vite';

export default defineConfig({
	plugins: [tailwindcss(), sveltekit(), svelteTesting()],
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
