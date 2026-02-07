# Project Ethos

The guiding philosophy, success factors, and strategic vision for my-family.

---

## Vision Statement

A premier self-hosted genealogy platform that combines **research rigor** with **engaging storytelling**, powered by a **git-inspired workflow** that treats family history research as the serious intellectual endeavor it deserves.

---

## Phase Guide

Features and capabilities are organized into phases to focus effort. **Phase 1 is the current priority** - resist the temptation to jump ahead.

| Phase | Focus | Principle |
|-------|-------|-----------|
| **Phase 1 (Now)** | Core data, import/export, basic UI, self-hosting | Nail the Basics First |
| **Phase 2 (Near-term)** | Research workflows, collaboration, richer visualizations | Dogfood Relentlessly |
| **Phase 3 (Future)** | AI features, plugins, community ecosystem | Start Small, Ship Often |

---

## Core Differentiators

### 1. Research Rigor (GPS-Compliant)
Align with the [Genealogical Proof Standard](https://bcgcertification.org/ethics-standards/) from the Board for Certification of Genealogists:
- **Complete citations** - Every fact tied to a source, Evidence Explained style `Phase 1`
- **Uncertain data markers** - Distinguish facts from speculation `Phase 1`
- **Reasonably exhaustive search** - Track what you've searched, including negative results `Phase 2`
- **Analysis & correlation** - Separate evidence (what sources say) from conclusions (what you believe) `Phase 2`
- **Conflict resolution** - Surface contradictions, require resolution `Phase 2`
- **Written conclusions** - Proof summaries for non-obvious conclusions `Phase 3`

### 2. Git-Inspired Workflow
Treat genealogy research like code - versioned, branched, collaborative:
- **Full audit trail** - Who changed what, when, and why `Phase 1`
- **Tags/snapshots** - Mark milestones ("Pre-DNA results", "After courthouse trip") `Phase 1`
- **Rollback capability** - Mistakes are recoverable `Phase 1`
- **Research branches** - Explore hypotheses without polluting main tree `Phase 2`
- **Merge with review** - Bring proven research into main tree with diff view `Phase 2`
- **Collaborative forks** - Others can propose changes via pull request workflow `Phase 3`

### 3. Bringing History to Life
Make ancestors feel like real people, not just names and dates:
- **Interactive timelines** - Zoomable, with historical events interwoven `Phase 2`
- **Migration maps** - Animated paths showing family movements `Phase 2`
- **Memory capture** - Oral histories, family stories attached to people `Phase 2`
- **Auto-generated narratives** - Prose stories from structured data `Phase 3`
- **Historical context** - What was happening in the world during their lives `Phase 3`
- **"A Day in Their Life"** - What was daily life like for their occupation/era? `Phase 3`

### 4. Personalization & Extensibility
Your genealogy tool, your way:
- **Report templates** - Output in your preferred format `Phase 2`
- **Custom fields** - Add your own data types `Phase 2`
- **Theming engine** - Colors, fonts, layouts `Phase 3`
- **Plugin architecture** - Community extensions `Phase 3`
- **Customizable dashboards** - See what matters to you `Phase 3`

### 5. Fun & Engaging
Research should be enjoyable, not a chore:
- **Completeness scores** - Gamify filling in gaps `Phase 1`
- **Brick wall tracker** - Track and celebrate breakthroughs `Phase 2`
- **Discovery feed** - Suggestions to explore `Phase 3`
- **Achievements & badges** - Celebrate milestones `Phase 3`
- **Shareable spotlights** - Social media cards for sharing discoveries `Phase 3`

---

## Success Factors

### Technical Foundation

| Factor | Why It Matters | Phase |
|--------|----------------|-------|
| **Data integrity** | ACID transactions, referential integrity | 1 |
| **Easy self-hosting** | Docker one-liner, not a 20-step guide | 1 |
| **API-first architecture** | Everything accessible programmatically | 1 |
| **Performance at scale** | Trees with 50,000+ people must stay fast | 2 |
| **Mobile experience** | Quick lookups at cemeteries, courthouses, reunions | 2 |
| **Automatic backups** | Data loss is catastrophic for genealogists | 2 |
| **Offline-first / PWA** | Researchers work in archives without internet | 3 |

### User Experience & Adoption

| Factor | Why It Matters | Phase |
|--------|----------------|-------|
| **Keyboard shortcuts** | Power users need speed | 1 |
| **Accessibility (a11y)** | Older users, screen readers, motor impairments | 1 |
| **Import from everywhere** | Not just GEDCOM - Ancestry, FamilySearch, Gramps | 1-2 |
| **Exceptional onboarding** | First 5 minutes determine retention | 2 |
| **Demo/sandbox mode** | Try before committing (no install required) | 2 |
| **Guided workflows** | Wizards for common tasks, not empty forms | 2 |
| **"Quick capture" mode** | Fast entry at courthouse, enrich later | 3 |

### Data & Interoperability

| Factor | Why It Matters | Phase |
|--------|----------------|-------|
| **No vendor lock-in** | Your data is always yours | 1 |
| **Export flexibility** | JSON, CSV, GEDCOM, custom formats | 1 |
| **Data validation tools** | Find inconsistencies, duplicates, errors | 1 |
| **Lossless GEDCOM 7.0** | Modern standard with proper extensions | 2 |
| **Bulk operations** | Mass edit, find/replace across tree | 2 |
| **Import from proprietary formats** | Ancestry exports lose DNA, photo tags, hints | 3 |

### Research Integrity

| Factor | Why It Matters | Phase |
|--------|----------------|-------|
| **Flexible dates** | "about 1842", "between 1840-1845", "before 1850" | 1 |
| **Multiple name handling** | Maiden names, aliases, spelling variants | 1 |
| **Relationship qualifiers** | Biological, adopted, step, foster | 1 |
| **Inclusive relationships** | Same-sex couples, modern family structures | 1 |
| **Uncertain data markers** | Distinguish facts from speculation | 1 |
| **Historical place context** | "Prussia" maps to modern Germany | 2 |

### AI/LLM Integration (Modern Differentiator) `Phase 3`

| Capability | Description |
|------------|-------------|
| **Handwriting transcription** | OCR old documents, letters, records |
| **Record extraction** | Parse census images into structured data |
| **Translation assistance** | German church records, Swedish parish books |
| **Research suggestions** | "Based on this data, you might search for..." |
| **Story generation** | LLM-assisted narrative writing |
| **Conflict analysis** | Explain why sources might disagree |
| **Natural language search** | "Who lived in Ohio in 1850?" |

### Community & Ecosystem `Phase 2-3`

| Factor | Why It Matters | Phase |
|--------|----------------|-------|
| **Clear contribution guide** | How to help, what's needed | 1 |
| **Public roadmap** | Transparency builds trust | 1 |
| **Regular releases** | Activity signals project health | 1 |
| **Discussion forum/Discord** | Community needs a home | 2 |
| **Plugin SDK + documentation** | Enable community extensions | 3 |
| **Theme SDK + documentation** | Let designers contribute | 3 |
| **Showcase/testimonials** | Social proof matters | 3 |

### Adoption & Go-to-Market `Phase 2-3`

| Factor | Why It Matters | Phase |
|--------|----------------|-------|
| **Hosted demo instance** | Try without installing anything | 2 |
| **Migration guides** | Step-by-step from Ancestry, FamilySearch | 2 |
| **Comparison page** | "my-family vs Gramps vs TNG vs..." | 2 |
| **Video tutorials** | YouTube is how people learn | 2 |
| **Genealogy society outreach** | Built-in audience of serious researchers | 3 |
| **Browser extension** | Capture from Ancestry/FamilySearch | 3 |

---

## Strategic Principles

### 1. Nail the Basics First
Fast, reliable, easy to install, great import/export. No fancy features matter if the core is broken.

### 2. Dogfood Relentlessly
Use it for real research. Feel the pain points. The best features come from actual use.

### 3. Start Small, Ship Often
One polished feature beats ten half-done ones. Iterate based on real feedback.

### 4. Document as You Go
Good documentation is a feature. Lack of docs kills adoption.

### 5. Build in Public
Blog posts, progress updates, transparent roadmap. Community forms around openness.

### 6. Find Real Users Early
Five serious genealogists providing feedback beats a thousand GitHub stars.

### 7. Respect the Data
Genealogy data is irreplaceable. Never lose it, never lock it in, never corrupt it.

### 8. Honor the Craft
Genealogy is a scholarly discipline. Build tools worthy of serious researchers.

---

## Inspirations

- **TNG** - Comprehensive feature set, proven in the community
- **Ancestry.com** - Hints, ThruLines, engagement features
- **Git/GitHub** - Version control, branching, collaboration model
- **Evidence Explained** - Citation standards and methodology
- **Genealogical Proof Standard** - Research rigor framework

---

## Anti-Patterns to Avoid

- **Vendor lock-in** - Data must always be exportable
- **Feature bloat** - Do fewer things well
- **Complexity for its own sake** - Simple by default, powerful when needed
- **Ignoring standards** - GEDCOM compliance matters
- **Desktop-only thinking** - Mobile and offline are first-class
- **Developer-centric UX** - Build for genealogists, not programmers

---

## References

- [Genealogical Proof Standard - BCG](https://bcgcertification.org/ethics-standards/)
- [Evidence Explained - Elizabeth Shown Mills](https://www.evidenceexplained.com/)
- [GEDCOM 7.0 Specification](https://gedcom.io/)
- [TNG Features](https://www.tngsitebuilding.com/features.php)
- [Sustainable Open Source - Aaron Stannard](https://aaronstannard.com/sustainable-open-source-software/)

---

## Related

- [Architecture Decision Records](./adr/) - Decisions guided by this ethos
- [CONTRIBUTING.md](../CONTRIBUTING.md) - How to contribute
