<objective>
Integrate the keyboard shortcuts and accessibility features into the main layout for issues #25 and #63.

This is the critical integration step that wires up global shortcuts, adds skip links, landmark regions, and the accessibility panel trigger to the application shell.
</objective>

<context>
Issues: #25 (Keyboard shortcuts), #63 (Accessibility improvements)
Tech stack: Svelte 5 with runes, SvelteKit, Tailwind CSS
Depends on: Steps 1-4 (all foundation components)

Read CLAUDE.md for project conventions.

@web/src/routes/+layout.svelte (main layout to modify)
@web/src/lib/stores/accessibilitySettings.svelte.ts
@web/src/lib/stores/keyboardSettings.svelte.ts
@web/src/lib/keyboard/useShortcuts.svelte.ts
@web/src/lib/components/KeyboardHelp.svelte
@web/src/lib/components/AccessibilityPanel.svelte
</context>

<requirements>
1. Apply accessibility settings to document:
   - Add reactive classes to `<body>` based on store values
   - `.high-contrast` when highContrast is true
   - `.font-large` or `.font-larger` based on fontSize
   - `.reduced-motion` when reducedMotion is true

2. Add skip link (first focusable element):
   - "Skip to main content" link
   - Visually hidden until focused (sr-only + focus:not-sr-only)
   - Links to `#main-content` anchor

3. Add landmark regions:
   - `role="banner"` on header
   - `role="navigation"` on nav (with aria-label)
   - `role="main"` on main content area
   - Add `id="main-content"` to main element

4. Add global keyboard shortcuts:
   - Use `useShortcuts('global', handlers)` hook
   - Implement handlers for: go-home, go-people, go-families, go-sources, focus-search, show-help
   - Use SvelteKit's `goto()` for navigation

5. Add UI triggers:
   - Accessibility icon button in header (opens AccessibilityPanel)
   - Position near other header controls
   - `aria-label="Accessibility settings"`

6. Include components:
   - `<KeyboardHelp bind:open={helpOpen} onClose={() => helpOpen = false} />`
   - `<AccessibilityPanel bind:open={panelOpen} onClose={() => panelOpen = false} />`
</requirements>

<implementation>
Use `$effect()` to apply classes to document.body:
```typescript
$effect(() => {
  document.body.classList.toggle('high-contrast', highContrast);
  document.body.classList.toggle('font-large', fontSize === 'large');
  document.body.classList.toggle('font-larger', fontSize === 'larger');
  document.body.classList.toggle('reduced-motion', reducedMotion);
});
```

For skip link, use Tailwind's sr-only pattern:
```html
<a href="#main-content" class="sr-only focus:not-sr-only focus:absolute focus:top-4 focus:left-4 focus:z-50 focus:bg-white focus:px-4 focus:py-2 focus:rounded">
  Skip to main content
</a>
```

For search focus, you'll need a ref or global function. Consider adding `focusSearch` export to SearchBox.
</implementation>

<output>
Modify file:
- `./web/src/routes/+layout.svelte` - Add all integrations

May need to modify:
- `./web/src/lib/components/SearchBox.svelte` - Add focusSearch export if needed
</output>

<verification>
Before completing:
- [ ] Skip link appears on Tab from page load
- [ ] Skip link jumps to main content
- [ ] Landmark regions present (check with accessibility inspector)
- [ ] `g h`, `g p`, `g f`, `g s` navigate to correct pages
- [ ] `/` focuses search input
- [ ] `?` opens help overlay
- [ ] Accessibility button opens panel
- [ ] Body classes apply based on settings
- [ ] No console errors
</verification>

<success_criteria>
- All global shortcuts work from any page
- Skip link functional and properly hidden/shown
- Landmark regions improve screen reader navigation
- Accessibility panel accessible from header
- Settings visually apply to the page
</success_criteria>
