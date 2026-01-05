<objective>
Create the accessibility settings panel component for issue #63.

This panel allows users to adjust font size, toggle high contrast mode, and manage other accessibility preferences. It should be easily accessible from the main layout.
</objective>

<context>
Issues: #63 (Accessibility improvements)
Tech stack: Svelte 5 with runes, Tailwind CSS
Depends on: Step 1 (accessibility store)

Read CLAUDE.md for project conventions.

@web/src/lib/stores/accessibilitySettings.svelte.ts (from step 1)
@web/src/lib/components/KeyboardHelp.svelte (similar modal pattern from step 3)
</context>

<requirements>
1. Create `web/src/lib/components/AccessibilityPanel.svelte`:
   - Slide-out panel or modal (triggered from layout)
   - Props: `open: boolean`, `onClose: () => void`

2. Controls to include:
   - **Font Size**: Three buttons (Normal, Large, Larger) or a slider
     - Show current selection clearly
     - Live preview as user changes
   - **High Contrast**: Toggle switch
     - Show on/off state clearly
   - **Reduced Motion**: Toggle switch
     - Default follows system preference
     - Label indicates when following system vs overridden
   - **Keyboard Shortcuts**: Toggle switch (from keyboard settings store)
     - Enable/disable all keyboard shortcuts

3. Accessibility requirements:
   - All controls keyboard accessible
   - Focus trap when open
   - Escape closes panel
   - `role="dialog"`, `aria-labelledby`
   - Labels associated with controls via `for`/`id`
   - Changes apply immediately (no save button needed)

4. Visual design:
   - Clean, spacious layout
   - Clear section groupings
   - Icons for visual clarity (optional)
   - High contrast mode should apply to the panel itself
</requirements>

<implementation>
Connect to stores:
```typescript
import { fontSize, highContrast, reducedMotion, setFontSize, setHighContrast, setReducedMotion } from '$lib/stores/accessibilitySettings.svelte';
import { shortcutsEnabled, setShortcutsEnabled } from '$lib/stores/keyboardSettings.svelte';
```

Use semantic form elements:
- `<fieldset>` and `<legend>` for grouped options
- `<button>` for font size selection (not radio, for better UX)
- Toggle switches as styled checkboxes with proper labels

Panel position: Consider right-side slide-out or centered modal based on existing patterns.
</implementation>

<output>
Create file:
- `./web/src/lib/components/AccessibilityPanel.svelte` - Settings panel component
</output>

<verification>
Before completing:
- [ ] All settings connect to stores correctly
- [ ] Changes persist (check localStorage after changing)
- [ ] High contrast mode applies to the panel itself
- [ ] All controls are keyboard accessible
- [ ] Focus trap works
- [ ] Screen reader announces control labels and states
</verification>

<success_criteria>
- All four settings (font, contrast, motion, shortcuts) controllable
- Live preview of changes
- Settings persist across sessions
- Fully keyboard navigable
- Works with screen readers
</success_criteria>
