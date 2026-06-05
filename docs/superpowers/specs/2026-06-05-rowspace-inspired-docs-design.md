# Rowspace-Inspired Hitch Docs Design

Date: 2026-06-05
Status: Approved for implementation planning

## Goal

Apply the content-presentation direction of Rowspace to Hitch documentation without copying Rowspace's brand. Hitch should feel editorial, spacious, and scroll-led on the landing page while keeping the reference documentation practical and scan-friendly.

## Scope

### In scope

- Refresh `docs/index.html` with a stronger Rowspace-inspired landing-page structure.
- Refresh shared styling in `docs/assets/site.css`.
- Make light-touch improvements to `docs/docs/latest/index.html` using the same visual language.
- Preserve the existing theme toggle, navigation links, content links, accessibility basics, and responsive behavior.

### Out of scope

- New product copy beyond what is needed to reorganize existing Hitch messaging.
- Heavy scroll animations, scroll snapping, or JS-driven scrollytelling.
- Reference-doc content rewrites unrelated to presentation.
- New generated imagery or external asset dependencies.

## Design Direction

Use Rowspace as a presentation reference: high-contrast editorial surfaces, oversized direct headlines, large vertical spacing, thin rules, sticky navigation, asymmetric split layouts, and clear narrative progression through the page.

Do not copy Rowspace's exact identity. Hitch keeps its own technical voice: agent lifecycle events, normalized envelopes, REST dispatch, handler decisions, audit, and replay.

## Landing Page Architecture

`docs/index.html` should become the primary expressive page.

### Hero

- Keep a sticky top nav.
- Use an oversized headline with a short, forceful product claim.
- Use a split composition: left side for headline and short narrative; right side for an abstract Hitch event/protocol visual built from existing HTML/CSS, such as a layered native payload, normalized envelope, REST route, and handler decision stack.
- Add a full-width bottom CTA bar or strong CTA row that visually anchors the first viewport.

### Vertical Content Flow

Replace dense card grids with fewer, larger sections:

1. Problem: every harness emits different lifecycle events and response contracts.
2. Hitch in action: show the event path from harness payload to normalized envelope to handler decision.
3. Use cases: solo developer guardrails and team-wide lifecycle routing.
4. Quickstart: compact command panel with primary docs links.
5. References: restrained doc cards for protocol, handler development, event mapping, HTTP API, harness contracts, and schemas.

### Visual System

- Airier spacing and stronger section rhythm.
- Thin horizontal rules and section dividers.
- White/light editorial surfaces in light mode; restrained dark editorial surfaces in dark mode.
- Fewer small pills and cards; more large panels and split layouts.
- Monospace only where it supports protocol/code content.

## Latest Docs Page Architecture

`docs/docs/latest/index.html` should receive the same polish at lower intensity.

- Keep the existing two-column docs shell and sticky sidebar.
- Improve vertical rhythm, sidebar affordance, code blocks, tables, and section boundaries.
- Avoid viewport-sized sections and heavy narrative layouts because the page is a reference document.
- Keep all existing anchors stable.

## Interaction and Accessibility

- Theme toggle behavior remains unchanged.
- Navigation links remain keyboard accessible.
- Use semantic headings and sections.
- Maintain readable contrast in light and dark themes.
- Respect reduced-motion users. If transitions are used, keep them cosmetic and nonessential.
- Mobile layout collapses to one column with readable spacing and no horizontal overflow.

## Implementation Notes

- Prefer CSS-only changes plus direct HTML restructuring.
- Keep `docs/assets/site.js` focused on theme handling unless a necessary interaction appears during implementation.
- Do not introduce framework dependencies.
- Do not add external images or video.
- Use existing docs content and commands where possible.

## Verification

- Open `docs/index.html` locally and inspect desktop and mobile widths.
- Open `docs/docs/latest/index.html` locally and verify anchors and sidebar remain usable.
- Check dark and light themes.
- Run a static docs smoke check with available project commands if present.
