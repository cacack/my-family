# Brand Guide

**Status:** In development (2026-03)

## Brand Essence

Modern, privacy-first genealogy that treats family history as both a rigorous archive and a living story.

**Tagline:** Your roots. Your data.

**Brand Attributes:** Trustworthy, Warm, Modern, Empowering, Rich

## Naming

- **Display name:** My Family (used in logo, UI, docs, marketing)
- **Technical name:** my-family (used in repo name, CLI, URLs, package names)

## Logo

### Concept

An abstract tree composed of connected nodes and branching lines -- a family tree that simultaneously reads as a version-control branch graph. The dual meaning reflects the product's core identity: genealogy software built on a git-inspired event-sourced architecture.

### Mark Description

- A single trunk rises from a root base and branches outward/upward
- Branches terminate in solid circular nodes (representing people/commits)
- Lines connecting nodes have clean, slightly organic curves -- not rigidly geometric, but structured
- The overall silhouette reads as a tree canopy, but the internal structure reads as a directed graph
- Single color (forest green #1F4D3A), no gradients
- Works at small sizes (favicon 16x16) due to bold nodes and clean lines

### Logo Asset

The canonical logo mark SVG is at [`docs/branding/logo-mark-traced.svg`](branding/logo-mark-traced.svg). Traced from AI-generated reference using Adobe's vector converter, with cracks fixed (unified fill color + thin matching stroke).

### Logo Lockups

Three approved lockup arrangements (all using the same mark):

1. **Stacked (primary):** Mark centered above "My Family" text, tagline below. Best for splash screens, about pages, documentation covers.
2. **Horizontal:** Mark to the left of "My Family" text, tagline below text. Best for app headers, navigation bars, GitHub README.
3. **App icon:** Mark centered inside a rounded-square container (forest green background, cream/white mark). Best for favicons, app icons, social preview cards.

**Rejected:** Integrating the mark into the hyphen of "my-family" (bottom-left variant from exploration) -- loses readability and the mark's standalone impact.

### Typography in Logo

- "My Family" set in a modern serif, charcoal #1F2933
- "Your roots. Your data." set in a clean sans-serif, lighter weight, smaller size

## Color Palette

### Primary Colors

| Role       | Name     | Hex       | Usage                                    |
|------------|----------|-----------|------------------------------------------|
| Primary    | Forest   | `#1F4D3A` | Logo, primary buttons, nav backgrounds   |
| Accent     | Amber    | `#D97706` | CTAs, highlights, active states, links    |
| Background | Cream    | `#F6F1E7` | Page backgrounds, cards                  |
| Text       | Charcoal | `#1F2933` | Body text, headings                      |

### Secondary Colors (under exploration)

Muted tones for supporting UI elements, status indicators, and extended palette:

- Sage green (muted) -- success states, secondary backgrounds
- Slate blue/grey -- neutral UI, borders, disabled states
- Burgundy (muted) -- error states, alerts, possible accent alternative

### Guidance

- Forest + Amber carry the brand boldly; use them as the recognizable pair
- Cream + Charcoal form the quiet base layer
- Secondary colors should be muted enough to never compete with Forest or Amber
- Dark mode: invert Cream to a dark charcoal/near-black; Forest and Amber remain as accent colors

## Typography

### Direction

Serif headings + sans-serif body -- a mix that signals "heritage with modern usability."

| Role      | Style           | Candidates (free, Google Fonts)                     |
|-----------|-----------------|-----------------------------------------------------|
| Headlines | Modern serif    | Playfair Display, Libre Baskerville, Source Serif Pro |
| Body / UI | Clean sans-serif| Inter                                                |

- Headlines in serif convey warmth, storytelling, and trust
- Body in Inter ensures readability at all sizes, strong UI performance
- Monospace (for code/technical contexts): JetBrains Mono or Fira Code

## Brand Voice

| Attribute     | Do                                        | Don't                                      |
|---------------|-------------------------------------------|--------------------------------------------|
| Trustworthy   | Precise language, cite sources            | Vague claims, marketing fluff              |
| Warm          | "Your family's story" not "user data"     | Cold technical jargon in user-facing copy  |
| Empowering    | Emphasize ownership, control, self-hosted | Apologize for requiring technical setup    |
| Modern        | Clean, direct, contemporary tone          | Archaic or overly formal language          |

## Reproduction Prompt

Use this prompt as a seed to reproduce or extend the brand in AI image generators:

> Logo for "My Family" genealogy software: an abstract tree made of connected nodes and branching lines. The trunk rises from a single root and branches upward, with branches terminating in solid circular nodes. The overall silhouette reads as a tree, but the internal structure resembles a version-control branch diagram (git graph). Clean lines, slightly organic curves, not rigidly geometric. Single color, forest green #1F4D3A. Modern, minimalist, works at small sizes. The mark should read as both "family tree" and "data graph" simultaneously.

## Design Exploration History

Brand direction developed 2026-03 using AI image generation (ChatGPT native image gen). The process explored three initial directions:

1. **"Living Tree" (Board 1)** -- Organic & warm, forest/amber/cream palette. Strong color foundation.
2. **"Thread & Archive" (Board 2)** -- Minimal & technical, slate/navy palette. Logo concept (branching graph) was strong but palette felt too cold.
3. **"Hearthstone" (Board 3)** -- Rich & narrative, burgundy/gold palette. Book/scroll motifs, old-study warmth.

**Convergence:** Board 1 palette + Board 2 logo concept (tree-as-branch-graph) became the winning combination. Tagline refined from generic "Genealogy - Connected" to "Your roots. Your data." which captures both heritage and data-ownership positioning.
