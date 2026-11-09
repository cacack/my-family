# web

The my-family frontend: Svelte 5 + SvelteKit, built as a static site with `@sveltejs/adapter-static`.
The adapter is configured in `vite.config.ts` (there is no `svelte.config.js`).

## Developing

```sh
npm ci
npm run dev        # dev server with hot reload; or `make dev-frontend` from the repo root
npm run check      # svelte-check
npm test -- --run  # vitest
```

## Building

```sh
npm run build      # writes the static site to web/build
```

`web/build` is not served by itself. The Go binary embeds the frontend from `internal/web/dist`,
which `make embed-assets` fills by copying `web/build` there. To build the full binary, run
`make binary` from the repo root; see [Building from Source](../README.md#building-from-source).
