<objective>
Add keyboard shortcuts to person and family detail pages for issue #25.

Power users should be able to quickly edit, save, and cancel on detail pages without reaching for the mouse.
</objective>

<context>
Issues: #25 (Keyboard shortcuts)
Tech stack: Svelte 5 with runes, SvelteKit
Depends on: Step 2 (keyboard system)

Read CLAUDE.md for project conventions.

@web/src/routes/persons/[id]/+page.svelte (person detail page)
@web/src/routes/families/[id]/+page.svelte (family detail page)
@web/src/lib/keyboard/useShortcuts.svelte.ts
@web/src/lib/keyboard/shortcuts.ts
</context>

<requirements>
1. Add detail page shortcuts to registry:
   - `e`: Enter edit mode (when not editing)
   - `s`: Save changes (when editing) - same as Ctrl+S convention
   - `Escape`: Cancel edit / exit edit mode
   - Context: 'person-detail' and 'family-detail'

2. Person detail page (`/persons/[id]`):
   - Integrate `useShortcuts('person-detail', handlers)`
   - `e` triggers edit mode (same as clicking Edit button)
   - `s` triggers save (same as clicking Save button)
   - `Escape` triggers cancel (same as clicking Cancel)
   - Only active shortcuts should work (e.g., 's' only when editing)

3. Family detail page (`/families/[id]`):
   - Same pattern as person detail
   - Use 'family-detail' context

4. Visual feedback:
   - Brief toast or visual indicator when action triggered by keyboard
   - Or rely on existing UI feedback (button states, form changes)

5. Safety:
   - 's' should not save if form is invalid
   - 'Escape' should prompt if unsaved changes (if not already implemented)
   - Shortcuts disabled when focus is in text input (to allow typing 'e', 's')
</requirements>

<implementation>
In page component:
```typescript
import { useShortcuts } from '$lib/keyboard/useShortcuts.svelte';

let editing = $state(false);
let formData = $state({...});

// Conditional handlers based on edit state
$effect(() => {
  const handlers = editing ? {
    'save': handleSave,
    'cancel': () => { editing = false; }
  } : {
    'edit': () => { editing = true; }
  };

  return useShortcuts('person-detail', handlers);
});
```

Alternative: Always register all handlers but check state inside:
```typescript
const handlers = {
  'edit': () => { if (!editing) editing = true; },
  'save': () => { if (editing) handleSave(); },
  'cancel': () => { if (editing) editing = false; }
};
```

The hook should handle focus-in-input detection, but double-check this works for text areas in the forms.
</implementation>

<output>
Modify files:
- `./web/src/lib/keyboard/shortcuts.ts` - Add person-detail and family-detail shortcuts
- `./web/src/routes/persons/[id]/+page.svelte` - Add keyboard hook and handlers
- `./web/src/routes/families/[id]/+page.svelte` - Add keyboard hook and handlers
</output>

<verification>
Before completing:
- [ ] 'e' enters edit mode on person page
- [ ] 's' saves when editing
- [ ] Escape cancels edit
- [ ] Shortcuts don't fire when typing in form fields
- [ ] Same functionality works on family page
- [ ] Invalid form prevents save via 's'
- [ ] Help overlay shows these shortcuts in correct context
</verification>

<success_criteria>
- Edit/save/cancel workflow fully keyboard accessible
- No accidental triggers when typing
- Both person and family pages have consistent shortcuts
- Shortcuts documented in help overlay
</success_criteria>
