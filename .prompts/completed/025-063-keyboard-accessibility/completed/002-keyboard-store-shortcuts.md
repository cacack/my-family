<objective>
Create the keyboard shortcut system with store, registry, and composable hook for issue #25.

This provides the foundation for all keyboard shortcuts across the application, supporting vim-style sequences (e.g., `g h` for "go home") to avoid conflicts with browser defaults.
</objective>

<context>
Issues: #25 (Keyboard shortcuts), #63 (Accessibility improvements)
Tech stack: Svelte 5 with runes, SvelteKit
Depends on: Step 1 (accessibility store) for reduced motion awareness

Read CLAUDE.md for project conventions.

@web/src/lib/stores/accessibilitySettings.svelte.ts (created in step 1)
@web/src/lib/components/MediaLightbox.svelte (example of existing keyboard handling with svelte:window)
@web/src/routes/+layout.svelte (where global shortcuts will be integrated)
</context>

<requirements>
1. Create keyboard settings store at `web/src/lib/stores/keyboardSettings.svelte.ts`:
   - `shortcutsEnabled`: boolean (global toggle, default true)
   - Persist to localStorage with key 'keyboard-settings'

2. Create shortcut registry at `web/src/lib/keyboard/shortcuts.ts`:
   - Define shortcut type: `{ keys: string[], action: string, description: string, context: string }`
   - Default shortcuts (vim-style sequences to avoid browser conflicts):
     - `g h` - Go to home (/)
     - `g p` - Go to people (/persons)
     - `g f` - Go to families (/families)
     - `g s` - Go to sources (/sources)
     - `/` - Focus search (single key, common convention)
     - `?` - Show help overlay
     - `Escape` - Close modal/cancel action
   - Context support: 'global', 'pedigree', 'person-detail', 'family-detail', 'search'
   - Export `getShortcutsForContext(context: string)` function

3. Create composable hook at `web/src/lib/keyboard/useShortcuts.svelte.ts`:
   - `useShortcuts(context: string, handlers: Record<string, () => void>)`
   - Handles sequence detection (tracks pending keys with 1s timeout)
   - Ignores shortcuts when focus is in input/textarea/contenteditable
   - Respects `shortcutsEnabled` from store
   - Returns cleanup function for component unmount
   - Uses `$effect()` for lifecycle management
</requirements>

<implementation>
Sequence detection approach:
1. Track `pendingKeys: string[]` and `lastKeyTime: number`
2. On keydown, if within 1000ms of last key, append to pendingKeys
3. Check if pendingKeys matches any registered shortcut
4. Clear pendingKeys on match or timeout

Avoid browser default conflicts:
- Don't use Ctrl/Cmd combinations (browser reserved)
- Don't use F1-F12 (system reserved)
- Single keys only when in non-input context
- Sequences like `g h` are safe as they're not browser shortcuts

Use `<svelte:window>` pattern from MediaLightbox for global listeners.
</implementation>

<output>
Create files:
- `./web/src/lib/stores/keyboardSettings.svelte.ts` - Keyboard settings store
- `./web/src/lib/keyboard/shortcuts.ts` - Shortcut registry and types
- `./web/src/lib/keyboard/useShortcuts.svelte.ts` - Composable hook
- `./web/src/lib/keyboard/index.ts` - Barrel export
</output>

<verification>
Before completing:
- [ ] All files compile without TypeScript errors
- [ ] Shortcut sequences work (test `g h` in isolation)
- [ ] Single key shortcuts work (`/`, `?`, `Escape`)
- [ ] Shortcuts ignored when typing in input fields
- [ ] Settings persist to localStorage
</verification>

<success_criteria>
- Clean API: `useShortcuts('global', { 'go-home': () => goto('/') })`
- Sequence detection works reliably with 1s timeout
- No interference with normal typing
- TypeScript types exported for shortcut definitions
</success_criteria>
