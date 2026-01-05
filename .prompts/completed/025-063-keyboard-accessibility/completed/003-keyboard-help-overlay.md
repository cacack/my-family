<objective>
Create the keyboard shortcut help overlay component for issue #25.

When users press `?`, this modal displays all available keyboard shortcuts grouped by context, helping users discover and learn the shortcuts.
</objective>

<context>
Issues: #25 (Keyboard shortcuts)
Tech stack: Svelte 5 with runes, Tailwind CSS
Depends on: Step 2 (keyboard system)

Read CLAUDE.md for project conventions.

@web/src/lib/keyboard/shortcuts.ts (shortcut registry from step 2)
@web/src/lib/components/MediaLightbox.svelte (example of modal with focus trap and escape handling)
</context>

<requirements>
1. Create `web/src/lib/components/KeyboardHelp.svelte`:
   - Modal overlay triggered by `?` key (handled externally)
   - Props: `open: boolean`, `onClose: () => void`
   - Display shortcuts grouped by context (Global, Pedigree, Person Detail, etc.)
   - Show key combination and description for each shortcut
   - Visual key styling (kbd-like appearance)

2. Accessibility requirements:
   - Focus trap: Tab cycles within modal only
   - First focusable element (close button) receives focus on open
   - Escape key closes modal
   - `aria-modal="true"`, `role="dialog"`, `aria-labelledby` for title
   - Backdrop click closes modal

3. Visual design:
   - Semi-transparent dark backdrop
   - Centered card with max-width
   - Section headings for each context group
   - Key combinations styled as keyboard keys (rounded, bordered, monospace)
   - Responsive: scrollable on small screens
</requirements>

<implementation>
Use existing MediaLightbox patterns for:
- `<svelte:window on:keydown>` for escape handling
- Backdrop with click handler
- Focus management with `$effect()`

Group shortcuts using `getShortcutsForContext()` or iterate all contexts from registry.

Style key combinations with Tailwind:
```
<kbd class="px-2 py-1 bg-gray-100 border rounded text-sm font-mono">g</kbd>
<span class="mx-1">then</span>
<kbd class="px-2 py-1 bg-gray-100 border rounded text-sm font-mono">h</kbd>
```
</implementation>

<output>
Create file:
- `./web/src/lib/components/KeyboardHelp.svelte` - Help overlay component
</output>

<verification>
Before completing:
- [ ] Component renders all shortcuts from registry
- [ ] Shortcuts grouped by context with clear headings
- [ ] Focus trap works (Tab stays within modal)
- [ ] Escape closes modal
- [ ] Backdrop click closes modal
- [ ] Accessible: screen reader announces as dialog
- [ ] Responsive on mobile (scrolls if needed)
</verification>

<success_criteria>
- All registered shortcuts displayed with descriptions
- Proper ARIA attributes for accessibility
- Clean, readable visual design
- Works on mobile and desktop
</success_criteria>
