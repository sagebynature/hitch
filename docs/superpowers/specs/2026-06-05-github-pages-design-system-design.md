# GitHub Pages design system refresh

## Goal

Update Hitch's GitHub Pages site to use MTPLX as a design reference while keeping Hitch's own technical identity, content hierarchy, and documentation usability.

## Approved direction

Use **B — Adapted Hitch system**.

The site should borrow MTPLX's high-contrast black stage, compact navigation, system-sans typography, mono micro-labels, subtle radial lighting, pill actions, and glassy technical panels. It should not clone MTPLX's logo treatment, metallic speed metric, or product-specific visuals. Hitch's motifs should come from its domain: universal hook protocol, harness adapters, JSON envelopes, handler dispatch, and audit records.

## Scope

In scope:

- Refresh the static GitHub Pages design system shared by the homepage and generated docs pages.
- Update `docs/index.html` only where markup changes are needed to support the new visual system or better semantic grouping.
- Update `docs/assets/site.css` as the primary implementation surface.
- Preserve existing theme toggle behavior in `docs/assets/site.js`.
- Preserve homepage section content and links unless a tiny label adjustment is required for visual fit.
- Ensure docs pages under `docs/latest/` remain readable with the refreshed shared stylesheet.

Out of scope:

- Rewriting product copy, docs content, schemas, Go code, installer code, or release scripts.
- Adding a build system, JavaScript framework, image assets, analytics, or remote runtime dependency beyond CSS font imports already used by the site.
- Creating a pixel-perfect MTPLX clone.

## Visual system

### Color

Use a dark-first palette:

- Near-black page background with faint radial highlights.
- Off-white primary text.
- Muted warm-gray secondary text.
- Low-opacity white borders for panels, cards, nav pills, tables, and code blocks.
- One restrained Hitch accent for protocol/adapter details. The accent should be cooler and quieter than the current blue/rust gradient so the page feels closer to MTPLX while still distinguishing Hitch.

Light theme should continue to exist because the current site exposes a theme toggle. It can be a functional inverse using warm off-white surfaces and dark text, but the primary design target is dark mode.

### Typography

Move the public site away from the current editorial serif look and toward MTPLX's technical product feel:

- System sans for body and display text.
- Monospace for nav microcopy, labels, code, pills, counters, and flow indexes.
- Large, tight display headings with negative letter spacing.
- Compact nav text and button labels.

The result should feel like a developer tool landing page, not a magazine layout.

### Layout and rhythm

Keep the existing page architecture:

1. Sticky global nav.
2. Hero.
3. Why Hitch.
4. Contract flow.
5. Quick start.
6. Docs index.
7. Footer.

Restyle the rhythm:

- More black space and quieter section dividers.
- Compact max-width container similar to MTPLX.
- Hero as a two-column technical landing section: left copy/actions, right terminal/protocol panel.
- Cards and flow blocks as dark, low-border panels with subtle internal hierarchy.
- Mobile layouts collapse to one column without horizontal overflow.

## Component requirements

### Header/nav

- Keep the skip link.
- Keep the Hitch brand and current links: Home, Docs, GitHub, theme toggle.
- Style the nav like MTPLX: compact, centered container, translucent dark bar, subtle border, small text, pill GitHub/theme controls.
- Preserve `aria-current` on the active page.

### Hero

- Keep the current core message: “One handler. Every harness.”
- Replace the current editorial hero feel with a dark product-stage composition.
- Use an MTPLX-inspired right-side technical panel, but make the panel Hitch-specific: install command, serve command, managed harness install, protocol/audit hints, or JSON envelope motif.
- Primary action remains docs; secondary action remains source.

### Cards and callouts

- Use low-contrast dark panels with rounded corners smaller than the current large editorial radius.
- Use mono labels/pills for event names, decisions, harnesses, and handler outputs.
- Hover states should be subtle: border brightening and small translate only when reduced motion allows.

### Contract flow

- Keep the five-step flow.
- Present each step as a compact technical block with mono index and short body copy.
- On mobile, stack steps vertically.

### Quick start/code

- Code blocks should match the MTPLX terminal style: dark panel, subtle top/header treatment if useful, monospace text, muted comments, no heavy gradients.
- Preserve command content.

### Docs pages

The same stylesheet must keep generated docs usable:

- Sidebar remains sticky on desktop and static on mobile.
- Content width stays readable.
- Tables scroll horizontally when needed.
- Inline code wraps safely.
- Code blocks and tables adopt the refreshed dark-panel treatment.

## Accessibility and resilience

- Maintain semantic HTML landmarks and heading order.
- Preserve skip-link behavior.
- Preserve keyboard focus visibility for links, buttons, and theme toggle.
- Preserve color contrast for primary text, secondary text, code, and buttons.
- Respect `prefers-reduced-motion` for transitions.
- Avoid relying on background images or decorative effects for comprehension.
- Avoid adding JavaScript for layout or animation.

## Implementation constraints

- Prefer editing existing files over creating new runtime assets.
- Do not introduce a CSS preprocessor, bundler, framework, or generated asset pipeline.
- Keep styles maintainable through clear custom properties and reusable component classes.
- Remove obsolete visual tokens or styles that no longer serve the new system.
- Do not modify generated docs content unless required to keep pages working.

## Verification plan

After implementation:

1. Serve or open the static docs site locally.
2. Inspect `docs/index.html` at desktop width.
3. Inspect `docs/index.html` at mobile width.
4. Inspect `docs/latest/` at desktop width.
5. Inspect `docs/latest/` at mobile width.
6. Confirm theme toggle still switches dark, light, and system modes.
7. Confirm primary links navigate to docs and GitHub.
8. Confirm no visible horizontal overflow.
9. Confirm code blocks, tables, cards, nav, and footer remain readable.

## Acceptance criteria

- Homepage visually follows the approved MTPLX-inspired adapted direction.
- Hitch remains visually distinct from MTPLX through domain-specific technical panels and copy hierarchy.
- Existing homepage sections and calls to action remain available.
- Docs pages using `docs/assets/site.css` remain readable and navigable.
- The site works without a new build step.
- Browser verification confirms desktop and mobile layouts are usable.
