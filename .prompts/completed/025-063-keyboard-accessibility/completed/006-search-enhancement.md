<objective>
Enhance the SearchBox component with full keyboard navigation for issues #25 and #63.

Users should be able to navigate search results with arrow keys, select with Enter, and close with Escape - standard accessible combobox behavior.
</objective>

<context>
Issues: #25 (Keyboard shortcuts), #63 (Accessibility improvements)
Tech stack: Svelte 5 with runes, Tailwind CSS
Depends on: Step 5 (layout integration provides `/` shortcut to focus)

Read CLAUDE.md for project conventions.

@web/src/lib/components/SearchBox.svelte (existing component to enhance)
</context>

<requirements>
1. Add keyboard navigation to dropdown:
   - `ArrowDown`: Move highlight to next result (wrap to first)
   - `ArrowUp`: Move highlight to previous result (wrap to last)
   - `Enter`: Select highlighted result (navigate to it)
   - `Escape`: Close dropdown, clear highlight
   - `Tab`: Close dropdown, move to next focusable element

2. Track highlighted index:
   - `highlightedIndex: number` state (-1 = none)
   - Reset to -1 when dropdown closes or results change
   - Visual indicator on highlighted item (background color)

3. Improve ARIA compliance:
   - Input: `aria-activedescendant` pointing to highlighted option ID
   - Input: `aria-autocomplete="list"`
   - Each result: `id="search-result-{index}"`
   - Each result: `aria-selected={index === highlightedIndex}`
   - Announce result count to screen readers via `aria-live` region

4. Export focus function:
   - `export function focusInput() { inputEl?.focus(); }`
   - Used by layout's `/` shortcut

5. Scroll highlighted item into view:
   - When navigating with arrows, ensure highlighted item is visible
   - Use `element.scrollIntoView({ block: 'nearest' })`
</requirements>

<implementation>
Update keydown handler:
```typescript
function handleKeydown(e: KeyboardEvent) {
  if (!showDropdown || results.length === 0) {
    if (e.key === 'Escape') {
      showDropdown = false;
    }
    return;
  }

  switch (e.key) {
    case 'ArrowDown':
      e.preventDefault();
      highlightedIndex = (highlightedIndex + 1) % results.length;
      break;
    case 'ArrowUp':
      e.preventDefault();
      highlightedIndex = highlightedIndex <= 0 ? results.length - 1 : highlightedIndex - 1;
      break;
    case 'Enter':
      if (highlightedIndex >= 0) {
        e.preventDefault();
        selectResult(results[highlightedIndex]);
      }
      break;
    case 'Escape':
      e.preventDefault();
      showDropdown = false;
      highlightedIndex = -1;
      break;
  }
}
```

Use $effect to scroll highlighted into view and update aria-activedescendant.
</implementation>

<output>
Modify file:
- `./web/src/lib/components/SearchBox.svelte` - Add keyboard navigation and ARIA
</output>

<verification>
Before completing:
- [ ] Arrow keys navigate through results
- [ ] Highlighted item has visible indicator
- [ ] Enter selects and navigates
- [ ] Escape closes dropdown
- [ ] Focus function exported and callable
- [ ] Screen reader announces result count
- [ ] aria-activedescendant updates correctly
- [ ] Long result lists scroll to keep highlight visible
</verification>

<success_criteria>
- Full keyboard-only search workflow possible
- Matches standard combobox accessibility patterns
- Visual and programmatic highlight in sync
- No regression to existing mouse-based functionality
</success_criteria>
