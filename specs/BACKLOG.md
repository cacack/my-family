# Feature Backlog

Features under consideration for my-family, a premier self-hosted genealogy platform.

See [ETHOS.md](./ETHOS.md) for the guiding philosophy and strategic vision.

## Vision

**Core Differentiators:**
- **Research Rigor** - GPS-compliant workflow with proper sources, citations, and evidence analysis
- **Git-Inspired Workflow** - Version control, branching hypotheses, collaborative merging
- **Bringing History to Life** - Stories, timelines, historical context that make ancestors feel real
- **Personalization** - Theming, plugins, customizable experience
- **Fun & Engaging** - Gamification, achievements, discovery that makes research enjoyable

## Priority Tiers

- **MVP-Next**: High value, builds on existing MVP foundation
- **Near-term**: Important features after core is solid
- **Future**: Nice-to-have, lower priority or higher complexity

---

## MVP-Next

### Technical Foundation
- [ ] Performance optimization for large trees (50,000+ people)
- [ ] Docker deployment (one-liner install)
- [ ] Automatic backups with restore capability
- [ ] API-first architecture (REST/GraphQL)
- [ ] Data integrity guarantees (ACID, referential integrity)

### Testing & Quality
- [ ] Playwright E2E tests (full browser testing of critical paths)
- [ ] OpenAPI contract tests (validate API matches spec)
- [ ] Property-based testing / fuzzing for GEDCOM parser
- [ ] Coverage gates in CI (reject PRs that decrease coverage)
- [ ] Dependabot for automated dependency updates
- [ ] Improve command package coverage (currently 52%)
- [ ] Test GEDCOM edge cases (divorced/remarried, multiple marriages, unknown gender)
- [ ] Concurrent modification / optimistic locking tests

### User Experience Foundation
- [ ] Onboarding wizard (guided first-time setup)
- [ ] Demo/sandbox mode (try without data commitment)
- [ ] Keyboard shortcuts for power users
- [ ] Responsive design (tablet/mobile friendly)

### Charts & Visualization
- [ ] Pedigree chart (unlimited generations, collapsible)
- [ ] Descendancy chart (standard format with boxes/lines)
- [ ] Family group sheet view
- [ ] Timeline chart (compare lifespans of multiple people)
- [ ] Interactive timeline with zoom/pan

### Search & Navigation
- [ ] Advanced search (name, date ranges, places)
- [ ] Soundex/metaphone matching for name variants
- [ ] Surname browser with counts
- [ ] Places list with hierarchy drill-down

### Media Management
- [ ] Photo upload and attachment to individuals
- [ ] Document upload (certificates, records)
- [ ] Thumbnail generation
- [ ] Media gallery view

### Sources & Citations (GPS Foundation)
- [ ] Source repository (centralized source management)
- [ ] Citation templates (Evidence Explained style)
- [ ] Attach sources to facts/events
- [ ] Source quality indicators (original/derivative, primary/secondary)

### Research Integrity (Foundation)
- [ ] Uncertain data markers (distinguish facts from speculation)
- [ ] Flexible date formats ("about 1842", "between 1840-1845", "before 1850")
- [ ] Multiple name handling (maiden, aliases, spelling variants)
- [ ] Relationship qualifiers (biological, adopted, step, foster)

### Data Quality
- [ ] "What's New" page (recent changes/additions)
- [ ] Basic statistics page (counts, date ranges, top surnames)
- [ ] Completeness scores per person (how complete is this record?)

### Git-Inspired Workflow (Foundation)
- [ ] Change history / audit trail (who changed what, when)
- [ ] Rollback capability (undo changes, restore previous states)

### Data Import/Export
- [ ] GEDCOM 7.0 support (lossless import/export)
- [ ] Ancestry GEDCOM import (handle Ancestry-specific extensions)
- [ ] FamilySearch GEDCOM import
- [ ] Gramps XML import
- [ ] Export to JSON/CSV

---

## Near-term

### Technical Foundation (Advanced)
- [ ] Offline-first / PWA support
- [ ] Mobile app or optimized mobile web
- [ ] "Quick capture" mode (fast entry, enrich later)
- [ ] Bulk operations (mass edit, find/replace across tree)
- [ ] Data validation/cleanup tools (find inconsistencies, duplicates)

### Charts & Visualization
- [ ] Relationship calculator (how are two people related?)
- [ ] Ahnentafel report
- [ ] Descendancy chart (register/narrative format)
- [ ] Fan chart view
- [ ] Migration maps (animated paths showing family movements)

### Media Management
- [ ] Image tagging (mark individuals in photos)
- [ ] Media cropping per GEDCOM 7.0
- [ ] Video/audio support
- [ ] Media albums/collections
- [ ] Photo timeline (visual progression through photos)
- [ ] Memory/story capture (oral histories attached to people)

### Research Rigor (GPS Advanced)
- [ ] Evidence vs conclusion separation (what sources say vs what you believe)
- [ ] Conflict tracking (flag contradictory evidence, require resolution)
- [ ] Research logs (document searches, including negative results)
- [ ] Proof summaries (attach proof arguments for non-obvious conclusions)

### Research Integrity (Advanced)
- [ ] Historical place context ("Prussia" → modern Germany mapping)
- [ ] Same-sex relationship support (modern families, historical contexts)
- [ ] Custom relationship types

### Git-Inspired Workflow (Advanced)
- [ ] Research branches (create branches for unproven hypotheses)
- [ ] Merge with review (merge branches with diff view when evidence supports)
- [ ] Tags/snapshots (milestone markers: "Pre-DNA results", "After courthouse trip")
- [ ] Conflict resolution UI (visual merge when evidence contradicts)

### Bringing History to Life
- [ ] Auto-generated stories (narrative prose from facts)
- [ ] Historical context cards ("During Mary's childhood, the Civil War began...")
- [ ] "A Day in Their Life" (what was daily life like for their occupation/era?)
- [ ] Anniversary notifications ("Today is the 150th anniversary of...")

### AI Integration (Foundation)
- [ ] Handwriting transcription / OCR for old documents
- [ ] Translation assistance (German church records, Swedish parish books)
- [ ] Research suggestions ("Based on this data, you might search for...")
- [ ] Natural language search ("who lived in Ohio in 1850?")

### Privacy & Access Control
- [ ] Living person data protection
- [ ] User registration with admin approval
- [ ] Configurable access levels (public, registered, admin)
- [ ] Branch-based permissions (per family line)

### Data Management
- [ ] Multiple family trees (separate databases)
- [ ] GEDCOM export for visitors
- [ ] PDF export (pedigree, descendancy, individual pages)
- [ ] Printer-friendly page versions

### Search & Discovery
- [ ] Heat map (geographic distribution)
- [ ] Cemetery/burial place browser
- [ ] "Brick Wall" tracker (track and celebrate breaking through blocks)
- [ ] Discovery feed ("You might be interested in..." suggestions)

### Personalization (Foundation)
- [ ] Theming engine (custom colors, fonts, layouts)
- [ ] Dashboard widgets (customizable home dashboard)
- [ ] Custom fields (add your own data fields per person/family)

### Accessibility
- [ ] Screen reader support (ARIA labels, semantic HTML)
- [ ] Keyboard navigation throughout
- [ ] High contrast mode
- [ ] Font size controls

---

## Future

### AI Integration (Advanced)
- [ ] Record extraction (parse census images into structured data)
- [ ] LLM-assisted story generation
- [ ] Conflict analysis ("These sources disagree - here's why...")
- [ ] Photo colorization/enhancement
- [ ] Smart record matching suggestions

### Smart Discovery (Ancestry-Inspired)
- [ ] Hints system (suggest record matches based on tree data)
- [ ] Record search integration (census, vitals, military records)
- [ ] Shared ancestor detection (find common ancestors with other users)
- [ ] ThruLines-style visualization (how DNA matches connect)

### DNA Integration
- [ ] DNA test result tracking
- [ ] Ethnicity estimate display
- [ ] DNA match management
- [ ] Descent tracker visualization

### Advanced Research Tools
- [ ] Custom event types for obscure GEDCOM tags
- [ ] Merge duplicate individuals tool
- [ ] Place name merge utility
- [ ] Custom report generator
- [ ] Custom queries for advanced users
- [ ] Book/narrative generation

### Collaboration (Advanced)
- [ ] Forks for collaboration (others fork your tree, propose changes)
- [ ] Pull request workflow (review and accept proposed changes)
- [ ] Visitor suggestions/feedback per person
- [ ] Real-time collaboration

### Personalization (Advanced)
- [ ] Theme marketplace (share/download community themes)
- [ ] Plugin architecture (extend functionality via plugins)
- [ ] Plugin SDK + documentation
- [ ] Theme SDK + documentation
- [ ] Report templates (customize output formats)
- [ ] Shareable spotlights (generate cards for social media)

### Gamification & Engagement
- [ ] Research achievements/badges (milestones, DNA match, etc.)
- [ ] Progress tracking (research goals and streaks)
- [ ] Animated family tree exploration

### Integration
- [ ] WordPress embedding
- [ ] External tree linking (FamilySearch, Ancestry hints)
- [ ] Browser extension (capture from Ancestry/FamilySearch)
- [ ] API for third-party integrations

### Localization
- [ ] Multi-language support
- [ ] Dynamic language switching
- [ ] Customizable text/labels

### Ecosystem & Community
- [ ] Hosted demo instance (public try-before-install)
- [ ] Comparison page (my-family vs alternatives)
- [ ] Migration guides (step-by-step from other platforms)
- [ ] Video tutorials
- [ ] Contribution guide (how to help)
- [ ] Public roadmap
- [ ] Discussion forum / Discord community

---

## Completed

*Move items here as they're implemented.*

- [x] GEDCOM import (001-genealogy-mvp)
- [x] Basic individual view (001-genealogy-mvp)
- [x] Basic family view (001-genealogy-mvp)

---

## Promoting a Feature to Implementation

When ready to implement a backlog item, follow this pipeline:

### 1. Create Feature Branch & Spec Folder

```bash
# Pick next feature number (e.g., 002)
git checkout main
git checkout -b 002-feature-name
mkdir -p specs/002-feature-name
cp -r specs/TEMPLATE-feature/* specs/002-feature-name/
```

### 2. Research Phase (Optional but Recommended)

```bash
# Use meta-prompt to research before specifying
/create-prompt research-feature "002-feature-name"
```

- Investigate prior art (TNG, Ancestry, Gramps)
- Review relevant standards (GEDCOM, GPS)
- Document findings in `specs/002-feature-name/research.md`

### 3. Specify Requirements

```bash
/speckit.specify
```

- Define user stories with acceptance criteria
- Identify alignment with core differentiators
- Document non-functional requirements
- Output: `specs/002-feature-name/spec.md`

### 4. Clarify Ambiguities

```bash
/speckit.clarify
```

- Identify underspecified areas
- Resolve open questions
- Update spec with answers

### 5. Plan Implementation

```bash
/speckit.plan
```

- Choose technical approach
- Design data model changes
- Plan API and frontend changes
- Output: `specs/002-feature-name/plan.md`

### 6. Generate Tasks

```bash
/speckit.tasks
```

- Break plan into actionable tasks
- Define verification criteria
- Output: `specs/002-feature-name/tasks.md`

### 7. Implement

```bash
/speckit.implement
```

Execute tasks with optional meta-prompts for quality:

```bash
# For GPS-related features
/create-prompt implement-with-gps "002-feature-name"

# For features needing versioning
/create-prompt implement-git-workflow "002-feature-name"

# For UI features
/create-prompt review-accessibility "ComponentName"

# For any feature
/create-prompt write-tests "package/function"
```

### 8. Validate & Ship

```bash
# Analyze for consistency
/speckit.analyze

# Run tests
go test ./...

# Create PR
gh pr create
```

### Available Meta-Prompts

| Prompt | Purpose |
|--------|---------|
| `research-feature` | Research before implementing |
| `implement-with-gps` | Add source/citation/evidence support |
| `implement-git-workflow` | Add versioning/audit trail |
| `review-accessibility` | Check a11y compliance |
| `write-tests` | Generate tests following patterns |
| `bring-to-life` | Enhance engagement/storytelling |

---

## Notes

- Features may move between tiers as priorities evolve
- Each feature should become a formal spec before implementation
- Consider dependencies when planning (e.g., sources before evidence/conclusion separation)
- GPS = Genealogical Proof Standard (BCG best practices)
- Git-workflow features are a key differentiator - prioritize accordingly

---

## Related

- [ETHOS.md](./ETHOS.md) - Strategic principles and success factors
- [CONVENTIONS.md](./CONVENTIONS.md) - Code standards for implementation
- [TEMPLATE-feature/](./TEMPLATE-feature/) - Template for feature specs
- [decisions/](./decisions/) - Architectural decisions
- [../CONTRIBUTING.md](../CONTRIBUTING.md) - Developer workflow guide
- [../.claude/prompts/](../.claude/prompts/) - Quality meta-prompts
