# Research Feature Prompt

Research a feature before implementation to understand prior art, standards, and best approaches.

---

## Context

Feature: $ARGUMENTS

## Instructions

1. **Understand the Feature**
   - Read the feature description from `specs/BACKLOG.md`
   - Identify the core problem being solved
   - Note which core differentiators apply (from `specs/ETHOS.md`)

2. **Research Prior Art**
   - How do TNG, Ancestry, Gramps, and FamilySearch handle this?
   - What patterns work well? What frustrates users?
   - Search for genealogy community discussions about this feature

3. **Review Standards**
   - Check GEDCOM 7.0 spec for relevant structures
   - Check Genealogical Proof Standard if research-related
   - Check Evidence Explained if citation-related

4. **Technical Research**
   - What libraries/tools exist that could help?
   - What are the performance implications?
   - What are the security considerations?

5. **Document Findings**
   - Create `specs/NNN-feature-name/research.md` using template
   - Summarize key insights and recommendations
   - List open questions that need resolution

## Output

Provide a summary of findings with:
- Recommended approach based on research
- Key considerations for implementation
- Any red flags or risks identified
- Open questions for the user to decide
