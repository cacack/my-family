<objective>
Audit and fix ARIA attributes across all components for issue #63.

This ensures screen readers work correctly throughout the application, fixing existing a11y-ignore comments and adding proper ARIA labels and live regions.
</objective>

<context>
Issues: #63 (Accessibility improvements)
Tech stack: Svelte 5, Tailwind CSS

Read CLAUDE.md for project conventions.

Components to audit (in order of complexity):
@web/src/lib/components/MediaLightbox.svelte (has a11y ignores)
@web/src/lib/components/MediaUpload.svelte (has a11y ignores)
@web/src/lib/components/MediaGallery.svelte
@web/src/lib/components/CitationSection.svelte
@web/src/lib/components/ChangeHistory.svelte
@web/src/lib/components/PlaceBrowser.svelte
@web/src/lib/components/PedigreeChart.svelte
@web/src/routes/import/+page.svelte (has a11y ignores)
</context>

<requirements>
1. Fix a11y-ignore comments by adding proper keyboard handlers:
   - Every click handler needs corresponding keydown handler
   - Pattern: `on:click={handler} on:keydown={(e) => e.key === 'Enter' && handler()}`
   - Or use `<button>` elements instead of `<div>` where appropriate

2. Add missing ARIA labels:
   - All interactive elements need accessible names
   - Icon-only buttons: `aria-label="Description"`
   - Images: `alt` text (meaningful or `alt=""` for decorative)
   - Form inputs: associated `<label>` or `aria-label`

3. Add aria-live regions for dynamic content:
   - Search results: `aria-live="polite"` to announce count
   - Form submission feedback: announce success/error
   - Loading states: announce when loading starts/completes
   - Pattern: `<div aria-live="polite" class="sr-only">{announcement}</div>`

4. Improve focus management:
   - When modals open, focus first interactive element
   - When modals close, return focus to trigger
   - After dynamic content loads, manage focus appropriately

5. Ensure proper heading hierarchy:
   - Each page has one `<h1>`
   - Headings don't skip levels (h1 → h2 → h3)
   - Section headings use appropriate levels
</requirements>

<implementation>
For click-to-keyboard conversion:
```svelte
<!-- Before -->
<div on:click={handleClick}>...</div>

<!-- After (option 1: add keyboard) -->
<div
  role="button"
  tabindex="0"
  on:click={handleClick}
  on:keydown={(e) => (e.key === 'Enter' || e.key === ' ') && handleClick()}
>...</div>

<!-- After (option 2: use button - preferred) -->
<button type="button" on:click={handleClick} class="appearance-none ...">...</button>
```

For live regions, create a store or component:
```typescript
// Simple announcement pattern
let announcement = $state('');
function announce(message: string) {
  announcement = '';
  // Small delay to ensure screen reader picks up change
  setTimeout(() => { announcement = message; }, 100);
}
```

Use sr-only class: `class="sr-only"` (Tailwind's screen-reader-only utility).
</implementation>

<output>
Modify files (audit each, fix issues found):
- `./web/src/lib/components/MediaLightbox.svelte`
- `./web/src/lib/components/MediaUpload.svelte`
- `./web/src/lib/components/MediaGallery.svelte`
- `./web/src/lib/components/CitationSection.svelte`
- `./web/src/lib/components/ChangeHistory.svelte`
- `./web/src/lib/components/PlaceBrowser.svelte`
- `./web/src/lib/components/PedigreeChart.svelte`
- `./web/src/routes/import/+page.svelte`

Optionally create:
- `./web/src/lib/components/Announcer.svelte` - Reusable live region component
</output>

<verification>
Before completing:
- [ ] No svelte-ignore a11y comments remain (all properly fixed)
- [ ] All interactive elements keyboard accessible
- [ ] All images have appropriate alt text
- [ ] Dynamic content changes announced to screen readers
- [ ] Tab through pages flows logically
- [ ] Test with browser accessibility inspector (no errors)
</verification>

<success_criteria>
- Zero a11y-ignore comments in codebase
- Screen reader can navigate all features
- Live regions announce important state changes
- Focus management correct for modals/dynamic content
- Heading hierarchy logical on all pages
</success_criteria>
