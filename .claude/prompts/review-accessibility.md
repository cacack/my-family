# Review for Accessibility

Review a component or page for accessibility compliance.

---

## Context

Component/Page: $ARGUMENTS

## Instructions

Review the specified component or page for accessibility issues:

### 1. Semantic HTML

- [ ] Proper heading hierarchy (h1 → h2 → h3)
- [ ] Lists use `<ul>`, `<ol>`, `<dl>` appropriately
- [ ] Tables have proper `<thead>`, `<th>`, scope attributes
- [ ] Forms use `<label>` elements linked to inputs
- [ ] Buttons are `<button>`, not styled divs
- [ ] Links are `<a>` with meaningful href

### 2. ARIA Labels

- [ ] Interactive elements have accessible names
- [ ] Icons have `aria-label` or `aria-hidden="true"`
- [ ] Dynamic content has `aria-live` regions
- [ ] Modals have proper `role="dialog"` and focus management
- [ ] Complex widgets have appropriate ARIA roles

### 3. Keyboard Navigation

- [ ] All interactive elements are focusable
- [ ] Focus order is logical (matches visual order)
- [ ] Focus is visible (outline or equivalent)
- [ ] Escape closes modals/dropdowns
- [ ] No keyboard traps
- [ ] Skip links for main content

### 4. Color & Contrast

- [ ] Text meets WCAG AA contrast (4.5:1 normal, 3:1 large)
- [ ] Information not conveyed by color alone
- [ ] Focus indicators have sufficient contrast
- [ ] Works in high contrast mode

### 5. Screen Reader Testing

- [ ] Content makes sense when read linearly
- [ ] Images have meaningful alt text (or empty for decorative)
- [ ] Form errors are announced
- [ ] State changes are announced

### 6. Responsive & Zoom

- [ ] Works at 200% zoom
- [ ] Touch targets are at least 44x44px
- [ ] No horizontal scrolling at mobile widths

## Common Genealogy-Specific Considerations

- Family trees need text alternatives or structured descriptions
- Charts should be navigable by keyboard
- Date/place pickers must be accessible
- Photo tagging needs screen reader support

## Output

Provide:
1. List of issues found with severity (Critical/Major/Minor)
2. Specific code fixes for each issue
3. Testing recommendations
