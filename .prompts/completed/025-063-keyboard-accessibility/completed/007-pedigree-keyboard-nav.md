<objective>
Add keyboard navigation to the pedigree chart for issue #25.

Power users should be able to navigate the family tree using arrow keys, zoom with +/-, and reset view with 'r', making the D3-based visualization fully keyboard accessible.
</objective>

<context>
Issues: #25 (Keyboard shortcuts)
Tech stack: Svelte 5 with runes, D3.js for visualization
Depends on: Step 2 (keyboard system)

Read CLAUDE.md for project conventions.

@web/src/lib/components/PedigreeChart.svelte (D3-based chart component)
@web/src/routes/pedigree/[id]/+page.svelte (page that hosts the chart)
@web/src/lib/keyboard/useShortcuts.svelte.ts
</context>

<requirements>
1. Add pedigree-specific shortcuts to registry (if not already):
   - `ArrowUp`: Navigate to father (if exists)
   - `ArrowDown`: Navigate to currently displayed person (root)
   - `ArrowLeft`: Navigate to mother (if exists)
   - `ArrowRight`: Navigate to first spouse/family (if exists)
   - `+` or `=`: Zoom in
   - `-`: Zoom out
   - `r`: Reset view (center and reset zoom)
   - `Enter`: View selected person's detail page

2. Track selected person in chart:
   - `selectedPersonId: string | null` state
   - Visual indicator on selected node (highlight ring, different color)
   - Start with root person selected

3. Implement navigation logic:
   - Arrow keys traverse the tree structure
   - Moving to non-existent relative does nothing (no wrap)
   - Selection updates D3 visualization to highlight node

4. Zoom controls:
   - `+`/`-` adjust D3 zoom transform
   - Small increments (e.g., 0.2 scale per keypress)
   - Respect min/max zoom bounds

5. Page-level integration:
   - Use `useShortcuts('pedigree', handlers)` in the page component
   - Pass selected state and handlers to chart component
   - Or integrate shortcuts directly in chart with `<svelte:window>`

6. Accessibility:
   - Announce navigation changes to screen readers
   - `aria-label` on chart container describing keyboard controls
   - Visual focus indicator visible in high contrast mode
</requirements>

<implementation>
For D3 zoom integration:
```typescript
// Assuming zoom behavior is stored
function zoomIn() {
  svg.transition().call(zoom.scaleBy, 1.2);
}
function zoomOut() {
  svg.transition().call(zoom.scaleBy, 0.8);
}
function resetView() {
  svg.transition().call(zoom.transform, d3.zoomIdentity);
}
```

For tree navigation, you'll need access to the tree data structure to find parent/child relationships from the selected node.

Visual selection indicator:
```typescript
nodes.classed('selected', d => d.data.id === selectedPersonId);
// CSS: .selected { stroke: var(--color-focus-ring); stroke-width: 3px; }
```
</implementation>

<output>
Modify files:
- `./web/src/lib/keyboard/shortcuts.ts` - Add pedigree context shortcuts
- `./web/src/lib/components/PedigreeChart.svelte` - Add selection, highlight, zoom controls
- `./web/src/routes/pedigree/[id]/+page.svelte` - Integrate keyboard hook
</output>

<verification>
Before completing:
- [ ] Arrow keys navigate tree (up=father, left=mother)
- [ ] Selected person highlighted visually
- [ ] +/- zoom in/out smoothly
- [ ] 'r' resets view to center
- [ ] Enter navigates to person detail page
- [ ] Navigation works in high contrast mode
- [ ] No shortcuts fire when typing in other inputs
</verification>

<success_criteria>
- Complete keyboard-only pedigree exploration possible
- Visual feedback clear for selected person
- Zoom controls smooth and bounded
- Integrates with existing mouse/touch interactions
</success_criteria>
