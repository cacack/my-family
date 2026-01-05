<objective>
Comprehensive testing of keyboard shortcuts and accessibility features for issues #25 and #63.

Verify all acceptance criteria are met through systematic manual testing and document any issues found.
</objective>

<context>
Issues: #25 (Keyboard shortcuts), #63 (Accessibility improvements)

Acceptance Criteria from #25:
- [ ] Common actions accessible via keyboard
- [ ] Help overlay shows available shortcuts
- [ ] Shortcuts do not conflict with browser defaults
- [ ] Works across all major pages

Acceptance Criteria from #63:
- [ ] Screen readers work correctly
- [ ] All features keyboard accessible
- [ ] High contrast mode available
- [ ] Font sizes adjustable
- [ ] WCAG 2.1 AA compliance
</context>

<requirements>
1. Create test checklist document:
   - Save to `./docs/accessibility-test-results.md`
   - Structured checklist with pass/fail for each item
   - Notes section for issues found

2. Keyboard shortcuts testing:
   - Test each global shortcut (g h, g p, g f, g s, /, ?)
   - Test pedigree shortcuts (arrows, +/-, r, Enter)
   - Test detail page shortcuts (e, s, Escape)
   - Test search navigation (arrows, Enter, Escape)
   - Verify no conflicts with browser (Ctrl+S, Ctrl+F, etc. still work)
   - Verify shortcuts disabled in input fields

3. Accessibility settings testing:
   - Font size changes apply across all pages
   - High contrast mode meets 4.5:1 ratio (use contrast checker)
   - Reduced motion disables animations
   - Settings persist after page reload
   - Settings persist after browser restart

4. Screen reader testing (VoiceOver on macOS):
   - Navigate home page with VoiceOver
   - Use search with VoiceOver (announces results)
   - Navigate pedigree chart (announces selected person)
   - Complete edit workflow on person page
   - Landmark navigation works (VO + U)

5. Keyboard-only navigation testing:
   - Tab through entire application without mouse
   - All interactive elements reachable
   - Focus visible at all times
   - Skip link works
   - Modal focus traps work

6. Report format:
   ```markdown
   # Accessibility Test Results
   Date: [date]
   Tester: Claude

   ## Keyboard Shortcuts (#25)
   | Test | Status | Notes |
   |------|--------|-------|
   | g h navigates home | ✅ | |
   | ... | | |

   ## Accessibility (#63)
   | Test | Status | Notes |
   |------|--------|-------|
   | High contrast 4.5:1 ratio | ✅ | Checked with DevTools |
   | ... | | |

   ## Issues Found
   1. [Issue description] - [Severity] - [Suggested fix]

   ## Summary
   - Total tests: X
   - Passed: X
   - Failed: X
   - Blocked: X
   ```
</requirements>

<implementation>
Testing approach:
1. Start fresh browser session
2. Clear localStorage to test defaults
3. Work through each test systematically
4. Document as you go
5. For failed tests, note exactly what happened vs expected

For contrast checking:
- Use browser DevTools accessibility panel
- Or Chrome extension: WAVE, axe DevTools
- Check: text on backgrounds, focus rings, buttons

For screen reader:
- macOS: Cmd+F5 to enable VoiceOver
- Navigate with VO+arrows, VO+U for rotor
- Listen for announcements, note any silent actions
</implementation>

<output>
Create file:
- `./docs/accessibility-test-results.md` - Complete test results

If issues found, create follow-up tasks:
- Update todo list with specific fixes needed
- Or create GitHub issues for significant problems
</output>

<verification>
Before completing:
- [ ] All tests executed and documented
- [ ] Test results saved to docs folder
- [ ] Any critical issues have follow-up action
- [ ] Summary indicates overall pass/fail status
</verification>

<success_criteria>
- All acceptance criteria from both issues verified
- Test results documented with evidence
- Any failures have clear reproduction steps
- Overall assessment: ready to ship or needs fixes
</success_criteria>
