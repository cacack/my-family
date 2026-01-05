<objective>
Create the accessibility settings store and CSS custom properties foundation for issues #25 and #63.

This establishes the core infrastructure for font size controls, high contrast mode, and reduced motion preferences that will be used throughout the application.
</objective>

<context>
Issues: #25 (Keyboard shortcuts), #63 (Accessibility improvements)
Tech stack: Svelte 5 with runes ($state, $derived, $effect), SvelteKit, Tailwind CSS
Project: Self-hosted genealogy software

Read CLAUDE.md for project conventions.

@web/src/app.css
@web/src/lib/components/SearchBox.svelte (example of existing Svelte 5 patterns)
</context>

<requirements>
1. Create accessibility settings store at `web/src/lib/stores/accessibilitySettings.svelte.ts`:
   - `fontSize`: 'normal' | 'large' | 'larger' (maps to 100%, 125%, 150%)
   - `highContrast`: boolean (enables high contrast color scheme)
   - `reducedMotion`: boolean (respects user preference, can be overridden)
   - Persist all settings to localStorage with key 'accessibility-settings'
   - Initialize from localStorage on load, fallback to system preferences
   - Use Svelte 5 runes pattern ($state for reactive state)

2. Add CSS custom properties to `web/src/app.css`:
   - `--font-size-base`: Base font size (1rem default)
   - `--font-size-scale`: Scale multiplier (1, 1.25, 1.5)
   - `--color-text`: Text color (adjusts for contrast)
   - `--color-bg`: Background color
   - `--color-border`: Border color
   - `--color-focus-ring`: Focus indicator color
   - `--color-link`: Link color
   - `--transition-duration`: For reduced motion (0s when reduced)

3. Add `.high-contrast` class styles that override colors for WCAG AA compliance (4.5:1 contrast ratio minimum)

4. Add `.font-large` and `.font-larger` classes that scale text appropriately
</requirements>

<implementation>
Follow existing Svelte 5 patterns in the codebase:
- Use `$state()` for reactive store values
- Export functions to update settings
- Use `$effect()` for localStorage persistence and applying classes to document.body

CSS custom properties should be defined in :root and overridden by classes on body element.

For reduced motion, check `window.matchMedia('(prefers-reduced-motion: reduce)')` as default.
</implementation>

<output>
Create/modify files:
- `./web/src/lib/stores/accessibilitySettings.svelte.ts` - New store file
- `./web/src/app.css` - Add CSS custom properties and utility classes
</output>

<verification>
Before completing:
- [ ] Store compiles without TypeScript errors
- [ ] Settings persist across page reloads (test in browser console)
- [ ] High contrast class applies correct color overrides
- [ ] Font size classes scale text appropriately
- [ ] Reduced motion respected from system preferences
</verification>

<success_criteria>
- Store exports: fontSize, highContrast, reducedMotion state and setter functions
- CSS variables are defined and cascade correctly
- localStorage persistence works
- No breaking changes to existing styles
</success_criteria>
