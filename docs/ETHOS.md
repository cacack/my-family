# Project Ethos

The guiding philosophy, success factors, and strategic vision for my-family.

---

## Vision Statement

A premier self-hosted genealogy platform that combines **research rigor** with **engaging storytelling**, powered by a **git-inspired workflow** that treats family history research as the serious intellectual endeavor it deserves.

---

## Core Differentiators

### 1. Research Rigor (GPS-Compliant)
Align with the [Genealogical Proof Standard](https://bcgcertification.org/ethics-standards/) from the Board for Certification of Genealogists:
- **Reasonably exhaustive search** - Track what you've searched, including negative results
- **Complete citations** - Every fact tied to a source, Evidence Explained style
- **Analysis & correlation** - Separate evidence (what sources say) from conclusions (what you believe)
- **Conflict resolution** - Surface contradictions, require resolution
- **Written conclusions** - Proof summaries for non-obvious conclusions

### 2. Git-Inspired Workflow
Treat genealogy research like code - versioned, branched, collaborative:
- **Full audit trail** - Who changed what, when, and why
- **Research branches** - Explore hypotheses without polluting main tree
- **Merge with review** - Bring proven research into main tree with diff view
- **Rollback capability** - Mistakes are recoverable
- **Collaborative forks** - Others can propose changes via pull request workflow
- **Tags/snapshots** - Mark milestones ("Pre-DNA results", "After courthouse trip")

### 3. Bringing History to Life
Make ancestors feel like real people, not just names and dates:
- **Auto-generated narratives** - Prose stories from structured data
- **Historical context** - What was happening in the world during their lives
- **"A Day in Their Life"** - What was daily life like for their occupation/era?
- **Interactive timelines** - Zoomable, with historical events interwoven
- **Migration maps** - Animated paths showing family movements
- **Memory capture** - Oral histories, family stories attached to people

### 4. Personalization & Extensibility
Your genealogy tool, your way:
- **Theming engine** - Colors, fonts, layouts
- **Plugin architecture** - Community extensions
- **Custom fields** - Add your own data types
- **Customizable dashboards** - See what matters to you
- **Report templates** - Output in your preferred format

### 5. Fun & Engaging
Research should be enjoyable, not a chore:
- **Achievements & badges** - Celebrate milestones
- **Completeness scores** - Gamify filling in gaps
- **Brick wall tracker** - Track and celebrate breakthroughs
- **Discovery feed** - Suggestions to explore
- **Shareable spotlights** - Social media cards for sharing discoveries

---

## Success Factors

### Technical Foundation

| Factor | Why It Matters |
|--------|----------------|
| **Performance at scale** | Trees with 50,000+ people must stay fast |
| **Offline-first / PWA** | Researchers work in archives without internet |
| **Mobile experience** | Quick lookups at cemeteries, courthouses, reunions |
| **Easy self-hosting** | Docker one-liner, not a 20-step guide |
| **Automatic backups** | Data loss is catastrophic for genealogists |
| **Data integrity** | ACID transactions, referential integrity |
| **API-first architecture** | Everything accessible programmatically |

### User Experience & Adoption

| Factor | Why It Matters |
|--------|----------------|
| **Exceptional onboarding** | First 5 minutes determine retention |
| **Demo/sandbox mode** | Try before committing (no install required) |
| **Import from everywhere** | Not just GEDCOM - Ancestry, FamilySearch, Gramps |
| **Guided workflows** | Wizards for common tasks, not empty forms |
| **Keyboard shortcuts** | Power users need speed |
| **Accessibility (a11y)** | Older users, screen readers, motor impairments |
| **"Quick capture" mode** | Fast entry at courthouse, enrich later |

### Data & Interoperability

| Factor | Why It Matters |
|--------|----------------|
| **Lossless GEDCOM 7.0** | Modern standard with proper extensions |
| **Import from proprietary formats** | Ancestry exports lose DNA, photo tags, hints |
| **Bulk operations** | Mass edit, find/replace across tree |
| **Data validation tools** | Find inconsistencies, duplicates, errors |
| **Export flexibility** | JSON, CSV, GEDCOM, custom formats |
| **No vendor lock-in** | Your data is always yours |

### Research Integrity

| Factor | Why It Matters |
|--------|----------------|
| **Uncertain data markers** | Distinguish facts from speculation |
| **Multiple name handling** | Maiden names, aliases, spelling variants |
| **Flexible dates** | "about 1842", "between 1840-1845", "before 1850" |
| **Historical place context** | "Prussia" maps to modern Germany |
| **Relationship qualifiers** | Biological, adopted, step, foster |
| **Inclusive relationships** | Same-sex couples, modern family structures |

### AI/LLM Integration (Modern Differentiator)

| Capability | Description |
|------------|-------------|
| **Handwriting transcription** | OCR old documents, letters, records |
| **Record extraction** | Parse census images into structured data |
| **Translation assistance** | German church records, Swedish parish books |
| **Research suggestions** | "Based on this data, you might search for..." |
| **Story generation** | LLM-assisted narrative writing |
| **Conflict analysis** | Explain why sources might disagree |
| **Natural language search** | "Who lived in Ohio in 1850?" |

### Community & Ecosystem

| Factor | Why It Matters |
|--------|----------------|
| **Clear contribution guide** | How to help, what's needed |
| **Plugin SDK + documentation** | Enable community extensions |
| **Theme SDK + documentation** | Let designers contribute |
| **Public roadmap** | Transparency builds trust |
| **Discussion forum/Discord** | Community needs a home |
| **Showcase/testimonials** | Social proof matters |
| **Regular releases** | Activity signals project health |

### Adoption & Go-to-Market

| Factor | Why It Matters |
|--------|----------------|
| **Hosted demo instance** | Try without installing anything |
| **Comparison page** | "my-family vs Gramps vs TNG vs..." |
| **Migration guides** | Step-by-step from Ancestry, FamilySearch |
| **Video tutorials** | YouTube is how people learn |
| **Genealogy society outreach** | Built-in audience of serious researchers |
| **Browser extension** | Capture from Ancestry/FamilySearch |

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
