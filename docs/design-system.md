# Subcult design system

Subcult uses the original Subcults terminal CSS as its visual foundation: small
rounded corners, visible borders, monospace type, purple actions and cyan focus. On
October 3, 2026 the user asked for a display layer taken from the Subcult OS
illustration on subcult.tv: condensed Oswald capitals, the purple admit-one
ticket stub and numbered "01 / PLAN" labels. The user
selected this direction on September 30, 2026, replacing the September 29
mobile-derived monochrome direction. Reference: `subcults` revision `93a13af`,
`web/src/index.css`; the newer three-font styling is a separate revision.

Dark mode uses the original black/charcoal surfaces and neon status accents.
Light mode keeps the same structure and purple actions with pale neutral surfaces
and darker status text. Light, Dark and System remain available. Event artwork
continues to provide event identity.

## Foundations

`contracts/design/tokens.json` is the shared source. `color` defines the light
palette; `darkColor` defines its dark counterpart with the same semantic roles. Run
`node scripts/design-tokens.mjs` after editing it. The generator writes Tailwind
theme files for both clients and native TypeScript tokens. `make verify` checks
for drift and checks normal-text contrast for the admitted foreground/surface
pairs in both themes. Generated files stay committed so either app can build independently.

| Foundation | Rule |
| --- | --- |
| Surfaces | Light: muted lavender-grey canvas (`#eeebf3`) and off-white panels (`#f8f6fa`), not pure white. Dark: black canvas and charcoal panels. Immersive artwork and scanner surfaces stay black in both modes. QR codes keep a pure-white quiet zone. |
| Text | Foreground roles adapt to the theme. Inverse text follows the primary action; on-immersive text stays white for artwork and scanner surfaces. |
| Actions | Purple primary with white text in both modes, outlined secondary, quiet ghost. Use the outlined variant when a control needs a visible boundary. Every action needs a clear verb and a visible focus state. |
| Status | Green success, amber attention, red failure, cyan information. Always include words; color alone cannot describe a state. |
| Typography | Self-hosted Oswald 700 for `h1`–`h4` on web; `h1` and `h2` are uppercase by style. Self-hosted Space Mono 400/700 for body copy, labels, controls and the wordmark. Platform monospace (Courier on iOS) in native themed working-screen styles; native has no display face yet. Bold (700) is the heaviest weight. Body copy retains ordinary case and readable line height; buttons and button-styled links use uppercase labels. Exact native Space Mono loading remains a follow-up. |
| Labels | Page eyebrows, kickers and index labels use the `label` text role: cyan in dark mode, deep purple in light mode. Field labels inside cards (`publicFieldLabelClass`) and dense operator screens stay muted. |
| Spacing | 4, 8, 12, 16, 24, 32, 48 px. Use 16 px page gutters on small screens and 24–32 px on wider screens. |
| Corners | One 8 px radius on controls, cards, panels, badges, artwork frames and the ticket stub (`radius` tokens; Tailwind `rounded-*` sizes resolve to the same value). A zero-specificity base rule in `web/src/styles.css` rounds any element with a `border` utility or a surface/status/action fill, so an explicit `rounded-*` utility still wins. A stub attached to a panel drops its bottom corners (`rounded-b-none`) to meet the perforation. Native `terminalStyles()` gives every rounded frame the same radius; avatars keep their circle. |
| Depth | Borders separate surfaces; panels have no soft shadows. |
| Targets | 48 px minimum for primary controls; 56 px fields and door controls. Keep scanner, navigation, and compact icon targets independently reviewable on devices. |
| Motion | Functional state transitions; web respects reduced-motion preference. Do not animate information required to operate the door. |

Artwork must come from the event when available. Existing fallback imagery is
retained; the gallery uses one of the mobile app's existing fallback images.
Keep type on a solid area or a sufficiently dark scrim, and provide descriptive
alt text for meaningful web images. Do not use gradients or colored glass as a
replacement for event identity.

## Ticket stub and index labels

`TicketStub` in `web/src/ui/TicketStub.tsx` renders the purple slab: a
Space Mono kicker in the "Subcult OS / …" form, the heading element passed as
children, and a decorative barcode marked `aria-hidden`. The page keeps its
own heading level. `ticket-stub-title` sets the display size;
`ticket-perforation` draws the dashed tear line between a stub and the body
below it.

- Use the stub for a page title on public and entry screens (Discover, sign-in)
  and as the header of an issued ticket. Do not use it for status or for
  operator working screens.
- `tilt` rotates the stub by 3 degrees. Use it only for display titles. Ticket
  stubs, QR codes and anything staff scan or read stay upright.
- `IndexList` and `indexLabel()` produce numbered labels such as "01 / Listings".
  Use them for ordered stages and page sections; they are not decoration on
  unordered content.

## Brand assets

The Subcult OS pack owns the marks and promotional artwork. Studio sources moved
into the private `subcult-tv` repository on October 2, 2026; the pack is
`studio/branding/library/brands/subcult-os` (direction "The terminal ledger").
Its colors and type restate this repository's dark tokens and the display
layer above, so `contracts/design/tokens.json` remains the source for interface
color. Its banner, square post, story and link preview use the tilted ticket
stub, cyan index labels and Oswald capitals; its marks are unchanged.

The pack's `product/` folder holds each file at its final size.
`python3 scripts/brand-assets.py --studio-root <subcult-tv checkout>` copies
the selection unchanged and writes `docs/brand-provenance.json` with the Studio
revision and each file's hash. `make check-contracts` runs the script with
`--check`, which compares the committed files with that record and does not
need Studio. Change an asset in Studio and re-export; do not crop, resize or
recolor it here.

| File | Pack source (`product/`) | Use |
| --- | --- | --- |
| `web/public/brand/mark.svg` | `mark-tight-outlined.svg` | SVG favicon and the mark in the `Brand` lockup |
| `web/public/favicon-32.png` | `favicon-32.png` | Favicon for browsers without SVG icon support |
| `web/public/apple-touch-icon.png` | `apple-touch-icon.png` | Home-screen icon |
| `web/public/og-image.png` | `og.png` | Link preview image |
| `mobile/assets/icon.png` | `app-icon.png` | iOS and store icon |
| `mobile/assets/android-icon-foreground.png` | `android-adaptive-foreground.png` | Android adaptive icon over the `#000000` background set in `mobile/app.json` |
| `mobile/assets/android-icon-monochrome.png` | `android-adaptive-monochrome.png` | Android themed icon |
| `mobile/assets/splash-icon.png` | `splash-icon.png` | Splash mark; `mobile/app.json` configures no splash screen yet |
| `mobile/assets/favicon.png` | `favicon-48.png` | Expo web favicon |

`Brand` in `web/src/ui/Brand.tsx` sets the mark beside the wordmark "Subcult OS"
in live Space Mono bold, so the wordmark follows the theme and stays selectable
text. The pack uses the same dark-panel mark on light and dark backgrounds.
Follow the pack guide: keep a quarter of the mark height clear, never show the
mark below 32 px, and do not stretch, recolor or redraw it. Authentication,
invitation, identity, consent and operator home headers use the lockup. Discover
is the signed-out front page, so it carries the lockup and one line saying what
Subcult OS is (`discoveryProductLine`, from the Studio claims register entry
C-OS). Public event, ticket and door screens keep event identity first and carry
no lockup.

The link preview image is the same for every URL because the web client renders
routes in the browser. Crawlers need an absolute address, so the web build
writes `PUBLIC_WEB_URL` into `og:image` and falls back to
`https://os.subcult.tv` when it is unset. The mobile icons have not been
reviewed on a device or simulator.

## Components and adoption

Web foundations live in `web/src/styles.css` and `web/src/ui/`. `Button` exposes
primary, secondary, ghost, and danger variants, defaults to `type="button"`, and disables
busy actions. `Notice` announces errors as alerts and other feedback as status.
Buttons and links styled as actions use the same `btn-primary`, `btn-secondary`, `btn-ghost`, and `btn-danger` utilities; destructive actions use `btn-danger`, not the purple primary. Counter grids in side columns size by container with `repeat(auto-fit, minmax(min(<size>, 100%), 1fr))`; without the `min()` clamp a grid column reports its full repeated width as its minimum and pushes the page wider than a phone screen. Give grid sidebars `min-w-0`. Uppercase labels use `tracking-[0.2em]`; buttons keep the shared 0.05em. Status colors mark states, not decoration: ordinary panels use `border-stroke-subtle` and muted labels. Text links are underlined. Use a native label with the shared `field` class for inputs; entered text stays regular weight inside bold labels. The existing
`publicUi` exports remain the common styling contract for public pages, tickets,
participant journeys, and the door.

Operator screens use semantic theme classes, including
workspace, event editing, identity, invitations, imports, lifecycle notices,
finance, and archive approval. Existing workflow handlers and confirmation steps
remain intact. Authentication uses the shared button and notice components.

Native foundations live in `mobile/src/theme/` and `mobile/src/global.css`.
Screen components read `useThemeTokens()` and `useThemedStyles()` so common
colors and corner sizes update when appearance changes. `useThemedStyles()`
applies `terminalStyles()` from `mobile/src/theme/terminal.ts`: every corner
radius becomes the shared 8 px token except styles named `avatar`, and text styles get the
platform monospace font capped at bold. Placeholder and icon colors come from
tokens; Uniwind
components use the generated theme. `PrimaryButton` exposes disabled and busy
states and button accessibility semantics. `Pill` supports all feedback tones.
The bottom navigation exposes its selected tab state and uses readable muted
labels. Immersive discovery and scanner screens keep their existing dark artwork
treatment. Screen-specific dimensions and image overlays remain local where
they serve a particular layout.

For new work, use semantic roles rather than adding another literal palette.
Extract a component when interaction or structure is repeated; do not build a
second router, form framework, or token package around these foundations.

## Words

Working screens (door, scanner, staffing, close-out) use short literal labels.
Door staff read them one-handed in poor light, so a label names the thing or the
action and nothing else. Errors say what happened and what to do next.

Headings, empty states and onboarding may sound like a run sheet: clipped,
role-first, dry. Example: an empty event list reads "Nothing on the sheet. New
event starts a draft."

- Sentence case in body text and headings. Buttons are uppercased by style, not
  in the source string.
- Public pages write "event" and "host" in lower case. "Door" and "End of
  Night" keep their capitals where they name the tool or the action.
- Say "archive" or "record", not "memory" (`CONTEXT.md` lists Memory under
  Avoid for Archive).
- Example times such as "19:00 doors" belong only where the screen labels them
  as examples. A fallback for a missing value says the value is missing
  ("Time to be announced"); it never invents one.
- Do not promise paid ticketing, payouts, app-store availability or a
  production service. Free reservations are the documented path.

The shared SUBCULT voice guide lives in the private `subcult-tv` repository
(`studio/content/VOICE.md`, section "Subcult OS").

## Appearance

Both clients default to the system setting and offer Light, Dark, and System.
The web Appearance selector is at the top of the page. Native preferences are
in Settings → Appearance. Each client saves its local preference; it is not an
account setting and does not sync across devices. Storage failures leave the
current session's controls usable.

Web reads its saved setting before rendering, responds to system changes in
System mode, and syncs saved preference changes between tabs. Native uses the
installed Uniwind theme API for both utility classes and reactive StyleSheet
factories. Fixed white scrims and inverse text on artwork stay independent of
the working-surface palette.

Base link and font resets belong in Tailwind's base layer. An unlayered
`a { color: inherit }` overrides text-color utilities and can make a light button
inside a dark artwork panel unreadable. Verify actual computed colors as well
as token contrast. Mode changes switch text and backgrounds together; controls
only transition opacity, transforms, and shadows.

## Review and verification

Start the isolated preview with `bash scripts/dev-env.sh start`. Open
`/design-system` on its printed URL to review event treatment, type, controls,
disabled/busy states, fields, badges, and feedback. The gallery is a development
route and does not appear in product navigation. Its Save/Reset controls affect
only local gallery state.

Run `bash scripts/dev-env.sh verify` for the repository checks and disposable DB
gate. `make check-contracts` includes token drift and contrast checks. An Expo
export checks native bundling; it does not establish visual quality or touch
behavior on a device.

Before release, review the gallery and real discovery, event, authentication,
workspace, editor, ticket, and door journeys at narrow and wide web widths.
Review discovery, event details, forms, navigation, and scanner on iOS and
Android, including keyboard focus where supported and increased text size.
Build and unit-test results are separate from browser and device evidence.

## Font provenance and remaining qualification

Web Latin Space Mono 400/700 WOFF2 files and the SIL Open Font License are vendored
from the existing Subcults `@fontsource/space-mono` 5.2.9 package. Oswald 700 is
a Latin WOFF2 subset of the Studio pack copy of Google Fonts Oswald
v57, with its OFL as `Oswald-LICENSE`.
`web/src/assets/fonts/provenance.json` records source filenames and SHA-256 hashes. They load locally
without a runtime font-provider request. Other scripts fall back to the installed
monospace font. Native themed StyleSheet factories apply monospace typography and
the shared corner radius centrally; isolated inline styles and native font loading still
need device review. Existing interaction and data contracts remain unchanged.

The token generator computes contrast from actual color values. The historical
Subcults design document contains inaccurate contrast figures and is not used as
contrast evidence. Purple actions use white text in both themes; cyan focus and
visible borders distinguish secondary actions and fields.
