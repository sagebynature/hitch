# GitHub Pages Design System Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Refresh Hitch's GitHub Pages site with an MTPLX-inspired adapted design system while preserving Hitch content, docs usability, accessibility, and the existing static-site workflow.

**Architecture:** Keep the site static: `docs/index.html` and `docs/docs/latest/index.html` continue to share `docs/assets/site.css` and `docs/assets/site.js`. Implement the design system as CSS custom properties plus reusable component classes; use only small homepage markup changes if a component needs better semantic grouping. Browser QA is the verification source because this is a visual/static-site change.

**Tech Stack:** Static HTML, CSS custom properties, vanilla JavaScript theme toggle, browser visual verification.

---

## File structure

- Modify `docs/assets/site.css`: primary design-system implementation. Replace editorial serif tokens with MTPLX-inspired dark product tokens, system-sans typography, mono labels, compact nav, dark panels, docs styling, responsive rules, and reduced-motion transitions.
- Modify `docs/index.html`: only if needed for hero/nav semantic hooks. Preserve existing sections, links, and copy.
- Do not modify `docs/assets/site.js`: the theme toggle should keep working unchanged.
- Do not modify `docs/docs/latest/index.html`: it should benefit from shared CSS without generated-content edits.

---

### Task 1: Baseline current pages

**Files:**
- Read: `docs/index.html`
- Read: `docs/assets/site.css`
- Read: `docs/docs/latest/index.html`

- [ ] **Step 1: Inspect homepage structure**

Read the homepage sections and confirm the current class contract remains:

```text
docs/index.html must still expose:
- .site-header > .nav
- .brand and .brand-mark
- .nav-links
- .theme-toggle[data-theme-toggle]
- .hero
- .terminal-card, .terminal-top, .terminal-body
- .section, .section-grid, .section-intro
- .card-grid, .card, .pill-row, .pill
- .flow, .flow-step
- .split, .callout
- .docs-index, .doc-card
- .footer
```

Expected: all classes are present in `docs/index.html`.

- [ ] **Step 2: Inspect generated docs structure**

Read `docs/docs/latest/index.html` and confirm the docs classes remain:

```text
docs/docs/latest/index.html must still expose:
- .site-header > .nav
- .docs-shell
- .docs-sidebar
- .docs-content
- .eyebrow
- .lede
- .code-block
- .docs-index
- .doc-card
- tables and inline code inside .docs-content
```

Expected: all classes are present in `docs/docs/latest/index.html`.

- [ ] **Step 3: Start a local static server**

Run:

```bash
python3 -m http.server 8765 --directory docs
```

Expected: server prints a line containing `Serving HTTP on :: port 8765` or `Serving HTTP on 0.0.0.0 port 8765` and continues running.

- [ ] **Step 4: Capture baseline screenshots**

Use browser verification against:

```text
http://127.0.0.1:8765/
http://127.0.0.1:8765/docs/latest/
```

Expected: both pages load. Keep screenshots only for comparison during the current work session; do not commit screenshots.

---

### Task 2: Replace global design tokens and base elements

**Files:**
- Modify: `docs/assets/site.css`

- [ ] **Step 1: Replace the font import and root tokens**

At the top of `docs/assets/site.css`, replace the existing `@import`, `:root`, and `:root[data-theme='light']` blocks with:

```css
@import url('https://fonts.googleapis.com/css2?family=Inter:wght@400;500;600;700;800&family=JetBrains+Mono:wght@400;500;600;700&display=swap');

:root {
  color-scheme: dark;
  --bg-deep: #050507;
  --bg: #08090b;
  --bg-raised: #101114;
  --panel: rgb(255 255 255 / 0.045);
  --panel-strong: rgb(255 255 255 / 0.075);
  --text: #efefe9;
  --copy: rgb(239 239 233 / 0.66);
  --muted: rgb(239 239 233 / 0.38);
  --line: rgb(255 255 255 / 0.1);
  --line-strong: rgb(255 255 255 / 0.17);
  --accent: #9fb5ff;
  --accent-strong: #f5f5ef;
  --rust: #8ea0c7;
  --saffron: #d7d7ce;
  --ok: #9ee6be;
  --shadow: rgb(0 0 0 / 0.48);
  --sans: 'Inter', 'SF Pro Text', ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif;
  --display: 'Inter', 'SF Pro Display', ui-sans-serif, system-ui, sans-serif;
  --mono: 'JetBrains Mono', 'SF Mono', ui-monospace, Menlo, Consolas, monospace;
  --max: 1120px;
  --radius: 18px;
}

:root[data-theme='light'] {
  color-scheme: light;
  --bg-deep: #e9e9e3;
  --bg: #f8f8f4;
  --bg-raised: #ffffff;
  --panel: rgb(7 8 10 / 0.045);
  --panel-strong: rgb(7 8 10 / 0.075);
  --text: #090a0c;
  --copy: rgb(9 10 12 / 0.68);
  --muted: rgb(9 10 12 / 0.45);
  --line: rgb(7 8 10 / 0.11);
  --line-strong: rgb(7 8 10 / 0.2);
  --accent: #375fb8;
  --accent-strong: #0f172a;
  --rust: #52668f;
  --saffron: #4a4a43;
  --shadow: rgb(15 23 42 / 0.12);
}
```

Expected: the stylesheet no longer references `--serif-display` or `--serif-body` after later steps finish.

- [ ] **Step 2: Replace global body and decorative background rules**

Replace the existing `body`, `body::before`, and `body::after` rules with:

```css
body {
  min-height: 100vh;
  margin: 0;
  background:
    radial-gradient(1200px 800px at 80% -10%, rgb(255 255 255 / 0.055), transparent 60%),
    radial-gradient(900px 700px at -10% 30%, rgb(159 181 255 / 0.045), transparent 62%),
    linear-gradient(180deg, var(--bg), var(--bg-deep));
  color: var(--text);
  font: 400 16px/1.6 var(--sans);
  text-rendering: optimizeLegibility;
}
body::before {
  content: '';
  position: fixed;
  inset: 0;
  z-index: -1;
  background-image: linear-gradient(rgb(255 255 255 / 0.035) 1px, transparent 1px);
  background-size: 100% 1px;
  mask-image: linear-gradient(to bottom, rgb(0 0 0 / 0.72), transparent 68%);
  pointer-events: none;
}
body::after {
  content: '';
  position: fixed;
  inset: 0;
  z-index: -1;
  background-image: url("data:image/svg+xml,%3Csvg viewBox='0 0 160 160' xmlns='http://www.w3.org/2000/svg'%3E%3Cfilter id='n'%3E%3CfeTurbulence type='fractalNoise' baseFrequency='.85' numOctaves='4' stitchTiles='stitch'/%3E%3C/filter%3E%3Crect width='100%25' height='100%25' filter='url(%23n)' opacity='.22'/%3E%3C/svg%3E");
  opacity: 0.05;
  pointer-events: none;
}
```

Expected: the background becomes MTPLX-like: near-black, radial highlights, faint horizontal texture, no visible square grid.

- [ ] **Step 3: Replace base link, code, focus, and skip-link rules**

Set base interactions to:

```css
a { color: inherit; text-decoration-thickness: 0.08em; text-underline-offset: 0.18em; }
a:hover { color: var(--accent-strong); }
a:focus-visible,
button:focus-visible {
  outline: 2px solid color-mix(in srgb, var(--accent) 72%, white 28%);
  outline-offset: 3px;
}
code, kbd, pre { font-family: var(--mono); }
img, svg { max-width: 100%; }

.skip-link {
  position: absolute;
  left: 1rem;
  top: -4rem;
  z-index: 20;
  padding: .55rem .8rem;
  background: var(--bg-raised);
  border: 1px solid var(--line-strong);
  border-radius: 999px;
  color: var(--text);
  font: 600 .75rem/1 var(--mono);
}
.skip-link:focus { top: 1rem; }
```

Expected: keyboard focus is visible on nav links, buttons, cards, and skip link.

- [ ] **Step 4: Run a stylesheet sanity search**

Search `docs/assets/site.css` for:

```text
serif-display|serif-body|Fraunces|Source Serif
```

Expected: no matches remain after Task 3 also replaces heading rules.

---

### Task 3: Restyle header, hero, and homepage components

**Files:**
- Modify: `docs/assets/site.css`
- Optional Modify: `docs/index.html` only if CSS-only styling cannot express the design.

- [ ] **Step 1: Replace header/nav and brand styles**

In `docs/assets/site.css`, replace the current `.site-header`, `.nav`, `.brand`, `.brand-mark`, `.nav-links`, `.nav-links a`, `.theme-toggle`, and related hover/current rules with:

```css
.site-header {
  position: sticky;
  top: 0;
  z-index: 10;
  backdrop-filter: blur(20px);
  background: color-mix(in srgb, var(--bg-deep) 82%, transparent);
  border-bottom: 1px solid rgb(255 255 255 / 0.055);
}
.nav {
  width: min(100% - 40px, var(--max));
  min-height: 64px;
  margin: 0 auto;
  display: flex;
  align-items: center;
  gap: 1rem;
}
.brand {
  display: inline-flex;
  align-items: center;
  gap: .68rem;
  color: var(--text);
  font: 700 .82rem/1 var(--sans);
  letter-spacing: .04em;
  text-decoration: none;
  text-transform: uppercase;
}
.brand-mark {
  width: 1.72rem;
  height: 1.72rem;
  display: grid;
  place-items: center;
  border: 1px solid var(--line-strong);
  border-radius: 7px;
  background: linear-gradient(180deg, rgb(255 255 255 / 0.14), rgb(255 255 255 / 0.035));
  box-shadow: inset 0 1px 0 rgb(255 255 255 / 0.12), 0 12px 28px var(--shadow);
  font: 700 .72rem/1 var(--mono);
}
.nav-links {
  margin-left: auto;
  display: flex;
  align-items: center;
  gap: .45rem;
  color: var(--muted);
  font: 500 .78rem/1 var(--sans);
}
.nav-links a,
.theme-toggle {
  min-height: 2.1rem;
  display: inline-flex;
  align-items: center;
  border: 1px solid transparent;
  border-radius: 999px;
  color: var(--copy);
  padding: .48rem .72rem;
  background: transparent;
  text-decoration: none;
}
.nav-links a[aria-current='page'],
.nav-links a:hover,
.theme-toggle:hover {
  border-color: var(--line);
  background: rgb(255 255 255 / 0.045);
  color: var(--text);
}
.theme-toggle { cursor: pointer; font: inherit; }
```

Expected: nav resembles MTPLX: compact, dark, low-border, pill interactions.

- [ ] **Step 2: Replace layout, labels, headings, copy, and button styles**

Replace `main`, `.hero`, `.eyebrow`, `.label`, heading, `.lede`, `.hero-actions`, `.actions`, and `.button` rules with:

```css
main { width: min(100% - 40px, var(--max)); margin: 0 auto; }
.hero {
  min-height: calc(100vh - 64px);
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(320px, .92fr);
  align-items: center;
  gap: clamp(2.5rem, 6vw, 6rem);
  padding: clamp(4.5rem, 9vw, 8rem) 0 clamp(5rem, 10vw, 9rem);
}
.eyebrow, .label {
  display: block;
  margin: 0 0 1.1rem;
  color: var(--muted);
  font: 500 .68rem/1.4 var(--mono);
  letter-spacing: .24em;
  text-transform: uppercase;
}
.eyebrow::before,
.label::before {
  content: '•';
  margin-right: .55rem;
  color: var(--text);
}
h1, h2, h3 {
  font-family: var(--display);
  font-weight: 650;
  line-height: .94;
  letter-spacing: -.065em;
}
h1 { max-width: 10ch; margin: 0; font-size: clamp(4.25rem, 12vw, 9.5rem); }
h2 { margin: 0 0 1rem; font-size: clamp(2.8rem, 6.2vw, 5.4rem); }
h3 { margin: 0 0 .7rem; font-size: clamp(1.35rem, 1.8vw, 1.8rem); letter-spacing: -.045em; }
.lede { max-width: 42rem; color: var(--copy); font-size: clamp(1.08rem, 1.65vw, 1.28rem); line-height: 1.52; }
.hero-actions, .actions { display: flex; flex-wrap: wrap; gap: .7rem; margin-top: 2rem; }
.button {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-height: 2.75rem;
  padding: .72rem 1rem;
  border: 1px solid var(--line);
  border-radius: 12px;
  background: rgb(255 255 255 / 0.045);
  color: var(--text);
  box-shadow: 0 18px 42px var(--shadow), inset 0 1px 0 rgb(255 255 255 / 0.08);
  font: 600 .82rem/1 var(--sans);
  text-decoration: none;
}
.button.primary {
  border-color: transparent;
  background: linear-gradient(180deg, #ffffff, #bdbdb6);
  color: #0a0a0c;
}
.button:hover { transform: translateY(-1px); border-color: var(--line-strong); color: var(--text); }
.button.primary:hover { color: #0a0a0c; }
```

Expected: homepage switches from editorial serif to MTPLX-like developer-tool typography.

- [ ] **Step 3: Replace terminal card styles**

Replace `.terminal-card`, `.terminal-card::before`, `.terminal-top`, `.terminal-body`, `.prompt`, `.comment`, and `.ok` rules with:

```css
.terminal-card {
  position: relative;
  border: 1px solid var(--line);
  border-radius: 20px;
  background:
    linear-gradient(180deg, rgb(255 255 255 / 0.085), rgb(255 255 255 / 0.025)),
    var(--bg-raised);
  box-shadow: inset 0 1px 0 rgb(255 255 255 / 0.08), 0 34px 90px var(--shadow);
  overflow: hidden;
}
.terminal-card::before {
  content: '';
  position: absolute;
  inset: 0;
  background: radial-gradient(480px 220px at 50% 0%, rgb(255 255 255 / 0.09), transparent 70%);
  pointer-events: none;
}
.terminal-top {
  position: relative;
  display: flex;
  justify-content: space-between;
  gap: 1rem;
  padding: .88rem 1rem;
  border-bottom: 1px solid var(--line);
  color: var(--muted);
  font: 500 .68rem/1 var(--mono);
  letter-spacing: .16em;
  text-transform: uppercase;
}
.terminal-body {
  position: relative;
  margin: 0;
  padding: 1.2rem;
  overflow-x: auto;
  color: rgb(220 220 214 / 0.92);
  font-size: .82rem;
  line-height: 1.75;
}
.prompt { color: #f1f1eb; }
.comment { color: var(--muted); }
.ok { color: var(--ok); }
```

Expected: the hero panel reads as a Hitch-specific terminal/protocol panel, not an MTPLX speed metric clone.

- [ ] **Step 4: Replace sections, cards, pills, flow, callouts, docs index, and footer styles**

Replace homepage component rules from `.section` through `.doc-card small`, plus `.footer`, with:

```css
.section { padding: clamp(4.5rem, 9vw, 8rem) 0; border-top: 1px solid rgb(255 255 255 / 0.06); }
.section-grid { display: grid; grid-template-columns: minmax(230px, .48fr) minmax(0, 1fr); gap: clamp(2rem, 5vw, 5rem); }
.section-intro p { color: var(--copy); font-size: 1rem; line-height: 1.62; }
.card-grid { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: .9rem; }
.card, .callout, .doc-card {
  border: 1px solid var(--line);
  border-radius: var(--radius);
  background: linear-gradient(180deg, rgb(255 255 255 / 0.055), rgb(255 255 255 / 0.025));
  box-shadow: inset 0 1px 0 rgb(255 255 255 / 0.06), 0 20px 55px var(--shadow);
}
.card { padding: 1.1rem; }
.card p, .doc-card p, .callout p { color: var(--copy); font-size: .95rem; line-height: 1.55; }
.card code, .pill code { color: var(--accent-strong); }
.pill-row { display: flex; flex-wrap: wrap; gap: .45rem; margin-top: 1rem; }
.pill {
  border: 1px solid var(--line);
  border-radius: 999px;
  padding: .32rem .5rem;
  background: rgb(255 255 255 / 0.035);
  color: var(--copy);
  font: 500 .68rem/1 var(--mono);
}
.flow {
  display: grid;
  grid-template-columns: repeat(5, minmax(0, 1fr));
  gap: .65rem;
  margin-top: 1.25rem;
}
.flow-step {
  min-height: 8rem;
  padding: .9rem;
  border: 1px solid var(--line);
  border-radius: 16px;
  background: rgb(255 255 255 / 0.035);
}
.flow-step span { color: var(--muted); font: 600 .68rem/1 var(--mono); letter-spacing: .12em; }
.flow-step strong { display: block; margin: .5rem 0; font: 650 1.05rem/1 var(--display); letter-spacing: -.04em; }
.flow-step p { margin: 0; color: var(--muted); font-size: .82rem; line-height: 1.4; }
.split { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: .9rem; }
.callout { padding: clamp(1.2rem, 2vw, 1.8rem); }
.callout.accent { border-color: color-mix(in srgb, var(--accent) 32%, var(--line)); background: linear-gradient(180deg, color-mix(in srgb, var(--accent) 10%, transparent), rgb(255 255 255 / 0.025)); }
.code-block {
  margin: 1rem 0 0;
  padding: 1rem;
  overflow-x: auto;
  border: 1px solid var(--line);
  border-radius: 16px;
  background: rgb(0 0 0 / 0.32);
  color: rgb(220 220 214 / 0.92);
  font-size: .82rem;
  line-height: 1.7;
}
.docs-index { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: .9rem; }
.doc-card { display: block; padding: 1.15rem; text-decoration: none; }
.doc-card:hover { border-color: var(--line-strong); color: var(--text); transform: translateY(-1px); }
.doc-card small { display: block; margin-bottom: .55rem; color: var(--muted); font: 600 .66rem/1 var(--mono); letter-spacing: .18em; text-transform: uppercase; }
.footer {
  width: min(100% - 40px, var(--max));
  margin: 0 auto;
  padding: 3rem 0 4rem;
  border-top: 1px solid rgb(255 255 255 / 0.06);
  color: var(--muted);
  font: 500 .76rem/1.5 var(--mono);
}
.footer a { color: var(--copy); }
```

Expected: cards and flow blocks become low-noise technical panels and retain all current content.

---

### Task 4: Restyle docs pages and responsive behavior

**Files:**
- Modify: `docs/assets/site.css`

- [ ] **Step 1: Replace docs shell/sidebar/content styles**

Replace `.docs-shell`, `.docs-sidebar`, `.docs-sidebar a`, `.docs-content`, `.docs-content h1`, `.docs-content h2`, `.docs-content h3`, `.docs-content p`, `.docs-content li`, `.docs-content table`, `.docs-content th`, `.docs-content td`, and `.docs-content :not(pre) > code` rules with:

```css
.docs-shell {
  display: grid;
  grid-template-columns: 250px minmax(0, 1fr);
  gap: clamp(2rem, 4vw, 4rem);
  padding: clamp(2rem, 5vw, 5rem) 0;
}
.docs-sidebar {
  position: sticky;
  top: 88px;
  height: calc(100vh - 112px);
  align-self: start;
  overflow: auto;
  padding: .85rem;
  border: 1px solid var(--line);
  border-radius: var(--radius);
  background: linear-gradient(180deg, rgb(255 255 255 / 0.055), rgb(255 255 255 / 0.025));
}
.docs-sidebar a {
  display: block;
  padding: .46rem .55rem;
  border-radius: 10px;
  color: var(--copy);
  font: 500 .74rem/1.25 var(--sans);
  text-decoration: none;
}
.docs-sidebar a:hover { background: rgb(255 255 255 / 0.055); color: var(--text); }
.docs-content { max-width: 820px; }
.docs-content h1 { max-width: none; font-size: clamp(3.1rem, 8vw, 6.4rem); }
.docs-content h2 { margin-top: 4rem; font-size: clamp(2.25rem, 4.4vw, 4.1rem); }
.docs-content h3 { margin-top: 2rem; }
.docs-content p, .docs-content li { color: var(--copy); }
.docs-content li + li { margin-top: .35rem; }
.docs-content table {
  display: block;
  width: 100%;
  overflow-x: auto;
  border-collapse: collapse;
  margin: 1rem 0;
  border: 1px solid var(--line);
  border-radius: 14px;
  font-size: .92rem;
}
.docs-content th, .docs-content td { padding: .78rem; border-bottom: 1px solid var(--line); text-align: left; vertical-align: top; }
.docs-content th { background: rgb(255 255 255 / 0.05); color: var(--text); font: 600 .72rem/1.35 var(--mono); letter-spacing: .1em; text-transform: uppercase; }
.docs-content tr:last-child td { border-bottom: 0; }
.docs-content :not(pre) > code {
  border: 1px solid var(--line);
  border-radius: 6px;
  padding: .08rem .28rem;
  background: rgb(255 255 255 / 0.04);
  color: var(--accent-strong);
  overflow-wrap: anywhere;
}
```

Expected: generated docs remain readable with refreshed table, code, and sidebar styling.

- [ ] **Step 2: Replace responsive and reduced-motion rules**

Replace the existing `@media (max-width: 920px)`, `@media (max-width: 560px)`, and reduced-motion block with:

```css
@media (max-width: 920px) {
  body { font-size: 16px; }
  .nav { flex-wrap: wrap; padding: .75rem 0; }
  .nav-links { width: 100%; margin-left: 0; overflow-x: auto; padding-bottom: .25rem; }
  .hero, .section-grid, .docs-shell { grid-template-columns: 1fr; }
  .hero { min-height: auto; }
  .card-grid, .docs-index, .split { grid-template-columns: 1fr; }
  .flow { grid-template-columns: 1fr; }
  .docs-sidebar { position: static; height: auto; }
}

@media (max-width: 560px) {
  main, .nav, .footer { width: min(100% - 28px, var(--max)); }
  h1 { font-size: clamp(3.2rem, 21vw, 5rem); }
  .button { width: 100%; }
  .hero-actions, .actions { width: 100%; }
  .card, .callout, .doc-card { border-radius: 15px; }
}

@media (prefers-reduced-motion: no-preference) {
  .card, .doc-card, .button { transition: transform .18s ease, border-color .18s ease, background .18s ease; }
}
```

Expected: mobile pages are single-column and do not horizontally overflow.

- [ ] **Step 3: Search for obsolete font variables**

Search `docs/assets/site.css` for:

```text
--serif-display|--serif-body|Fraunces|Source Serif
```

Expected: no matches.

- [ ] **Step 4: Search for obsolete square-grid styling**

Search `docs/assets/site.css` for:

```text
120px 120px|24px 24px
```

Expected: no matches.

---

### Task 5: Browser verification and final commit

**Files:**
- Verify: `docs/index.html`
- Verify: `docs/docs/latest/index.html`
- Verify: `docs/assets/site.css`

- [ ] **Step 1: Start local static server if not already running**

Run:

```bash
python3 -m http.server 8765 --directory docs
```

Expected: server stays running on port `8765`. If the port is already in use by the previous server from this plan, reuse the running server.

- [ ] **Step 2: Verify homepage desktop**

Open:

```text
http://127.0.0.1:8765/
```

Set viewport to `1440x1000`.

Expected:

```text
- Header is compact and sticky.
- Hero is two columns.
- Right terminal panel is visible and readable.
- Buttons are visible and clickable.
- Cards, flow blocks, quickstart, docs index, and footer use the refreshed dark system.
- No horizontal scrollbar is visible.
```

- [ ] **Step 3: Verify homepage mobile**

Open:

```text
http://127.0.0.1:8765/
```

Set viewport to `390x844`.

Expected:

```text
- Header wraps without clipping.
- Hero stacks into one column.
- Buttons fill the available width.
- Terminal/code blocks scroll internally instead of causing page overflow.
- No horizontal scrollbar is visible.
```

- [ ] **Step 4: Verify docs desktop**

Open:

```text
http://127.0.0.1:8765/docs/latest/
```

Set viewport to `1440x1000`.

Expected:

```text
- Sidebar is sticky and readable.
- Documentation content is readable.
- Tables and code blocks use the refreshed panel treatment.
- Inline code wraps safely.
- No horizontal scrollbar is visible on the page.
```

- [ ] **Step 5: Verify docs mobile**

Open:

```text
http://127.0.0.1:8765/docs/latest/
```

Set viewport to `390x844`.

Expected:

```text
- Sidebar becomes static above content.
- Tables scroll within their own bounds.
- Code blocks scroll within their own bounds.
- Page content is readable without zooming.
- No horizontal scrollbar is visible on the page.
```

- [ ] **Step 6: Verify theme toggle**

On the homepage, click the theme toggle three times.

Expected:

```text
- Button text cycles through Light, Dark, and System.
- document.documentElement.dataset.themeMode changes with the button text.
- document.documentElement.dataset.theme changes to a valid light or dark value.
- Page remains readable in light and dark themes.
```

- [ ] **Step 7: Verify primary links**

On the homepage, activate these links:

```text
- Read the docs
- View source
- Docs nav link
- GitHub nav link
```

Expected:

```text
- Docs links resolve to /docs/latest/ under the local server.
- GitHub links point to https://github.com/sagebynature/hitch.
```

- [ ] **Step 8: Commit implementation**

Run:

```bash
git add docs/index.html docs/assets/site.css
git commit -m "style: refresh GitHub Pages design system"
```

Expected: commit succeeds. If `docs/index.html` was not modified, omit it from `git add` and commit only `docs/assets/site.css`.
