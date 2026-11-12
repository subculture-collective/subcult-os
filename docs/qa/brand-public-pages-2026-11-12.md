# Public pages brand and accessibility audit, 2026-11-12

Scope: the signed-out pages `/discover` and `/login` (and `/signup`) on
https://os.subcult.tv, compared with the Subcult OS pack in `subcult-tv`
(`studio/brands/subcult-os/`: `GUIDE.md`, `tokens.json`), `docs/design-system.md`
and `contracts/design/tokens.json`. Copy was checked against
`studio/content/VOICE.md` and claims register entry C-OS.

Trigger: a Studio brand review the same day, made from a signed-out 1440 px
light-scheme capture, reported Discover as pale lilac, without the pack's cyan,
without the boxed "OS" mark, without a line saying what Subcult OS is, and "two
thirds empty".

## How it was checked

- Rendered pages, not source alone. Captures of `/discover` and `/login` at 1440
  and 390 px, light and dark, from a local build before and after each change
  and from the live site after deploy. They are kept on Kvant under
  `~/.local/state/brand-site-review-20261112/subcult-os-fix/` (`before/`,
  `after/`, `live/`, `a11y/`).
- Contrast: a Playwright script walked every visible text element and form
  control on both pages in all four theme and width combinations. It composited
  backgrounds up the ancestor chain and applied WCAG 2.2 thresholds (4.5:1 text,
  3:1 large text, 3:1 control boundaries and focus indicators). States not
  rendered when signed out (warning, danger and info notices; disabled buttons)
  were checked from the tokens.
- Keyboard: Tab and Shift+Tab through both pages in all four combinations,
  recording each stop, its focus ring, its contrast and whether it was clipped.
- Each reported accessibility defect was re-measured by two separate checks,
  each told to try to refute it. All defects listed below survived both.

Not covered: Discover with published events (the live list was empty, so event
cards and artwork were not measured), screen-reader output in a real assistive
technology, and pages behind sign-in.

## Brand findings

| # | Reported | Result | Owner and action |
|---|---|---|---|
| B1 | Page is pale lilac | Not a defect. The light theme is documented here and in the pack's `GUIDE.md`. | None |
| B2 | No cyan | Not a defect. Cyan fails contrast on the light canvas, so light uses deep purple labels (`#5b21b6`, 7.62:1). Dark uses cyan (16.75:1). | None |
| B3 | No boxed "OS" mark | Defect on Discover; sign-in already had it. | Fixed in subcult-os #126 |
| B4 | Nothing says what Subcult OS is | Defect. | Fixed in #126 with `discoveryProductLine`, from C-OS |
| B5 | "Two thirds empty" | Capture artefact: the page was 1087 px tall in a taller frame. The empty states did say too little. | Empty-state copy rewritten in #126 and here |
| B6 | Pack tokens hold no light values | Specification gap. `tokens.json` is dark only; the light theme is described only in `GUIDE.md` prose, which is what misled the review. | subcult-tv [#63](https://github.com/subculture-collective/subcult-tv/issues/63) |

## Accessibility findings

| # | Page | Finding | Rule | Result |
|---|---|---|---|---|
| A1 | `/login`, `/signup` | The Sign in / Create account switch showed the current mode only by fill colour, and neither link exposed it. In dark the two fills differ by 2.89:1. | 1.3.1, 4.1.2 (A) | Fixed: `aria-current="page"` on the current link, an underline as a non-colour cue, and the pair wrapped in `<nav aria-label="Account">` |
| A2 | `/login`, `/signup` | Page title was the generic "Subcult OS". | 2.4.2 (A) | Fixed: "Sign in \| Subcult OS" and "Create account \| Subcult OS" |
| A3 | `/discover` at 390 px | When keyboard focus scrolled the Search button or search field into view, the browser left it flush with the viewport edge and cut off one edge of the focus ring. | Advisory; 2.4.11 still passes | Fixed: `scroll-margin-block: 0.5rem` on focusable elements |
| A4 | Both | The Appearance control, the first tab stop, sat outside every landmark. | Advisory | Fixed: wrapper is a region named "Display settings" |
| A5 | Both | The lockup is not a link. Its name comes from the text and the mark has `alt=""`, which is correct. | Advisory | Left as is |
| A6 | Both, dark | Hovered primary buttons (`#5b21b6`) are 2.34:1 against black. The button text stays at 8.98:1 and identifies the control, so 1.4.11 is met. | Advisory | Left as is |

A1 also removed a dead `hover:border-stroke-strong` class from the inactive link;
it had no border width, so it never showed.

## What matched

- No text or control-boundary contrast failure on either page, in either theme,
  at either width. The lowest text pair is white on `action.primary` `#7c3aed`
  at 5.70:1. Placeholders are 6.18:1 (light) and 8.39:1 (dark). Input and button
  borders are 3.85:1 or better.
- The focus ring (2 px at 3 px offset) appears on every tab stop: `#006b73` at
  5.32:1 in light and `#00ffff` at 16.75:1 in dark. Tab order follows the
  visual order and there are no keyboard traps.
- The lockup uses the pack's exported mark and the live Space Mono wordmark.
  Display type is Oswald and body type is Space Mono, as in the pack.

## Web and mobile copy

`docs/qa/discovery-ui-parity-2026-06.md` requires the web and mobile Discover
empty states to match. #126 changed only the web string, and the new string
promised a ticket count that mobile slides do not show. Both clients now read:
"Nothing on the board. Events run on Subcult OS are listed here. When a host
publishes one, it shows with its date and place."

The product line stays web-only. The parity rule covers loading, empty, error
and button copy, and the mobile screen has no header to hold it without a
layout change.
