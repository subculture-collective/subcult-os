# Subcult design system

Subcult uses the original Subcults terminal CSS as its visual foundation: sharp
corners, visible borders, monospace type, purple actions and cyan focus. The user
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
| Surfaces | Light: pale lavender-neutral canvas and white panels. Dark: black canvas and charcoal panels. Immersive artwork and scanner surfaces stay black in both modes. |
| Text | Foreground roles adapt to the theme. Inverse text follows the primary action; on-immersive text stays white for artwork and scanner surfaces. |
| Actions | Purple primary with white text in both modes, outlined secondary, quiet ghost. Use the outlined variant when a control needs a visible boundary. Every action needs a clear verb and a visible focus state. |
| Status | Green success, amber attention, red failure, cyan information. Always include words; color alone cannot describe a state. |
| Typography | Self-hosted Space Mono 400/700 on web. Platform monospace (Courier on iOS) in native themed working-screen styles. Bold (700) is the heaviest weight. Body copy retains ordinary case and readable line height; buttons and button-styled links use uppercase labels. Exact native Space Mono loading remains a follow-up. |
| Spacing | 4, 8, 12, 16, 24, 32, 48 px. Use 16 px page gutters on small screens and 24–32 px on wider screens. |
| Corners | Square controls, cards, panels, badges and artwork frames. Native avatars retain their existing circular shape. |
| Depth | Borders separate surfaces; panels have no soft shadows. |
| Targets | 48 px minimum for primary controls; 56 px fields and door controls. Keep scanner, navigation, and compact icon targets independently reviewable on devices. |
| Motion | Functional state transitions; web respects reduced-motion preference. Do not animate information required to operate the door. |

Artwork must come from the event when available. Existing fallback imagery is
retained; the gallery uses one of the mobile app's existing fallback images.
Keep type on a solid area or a sufficiently dark scrim, and provide descriptive
alt text for meaningful web images. Do not use gradients or colored glass as a
replacement for event identity.

## Brand assets

The Subcult OS pack in the private `subcult-studio` repository
(`branding/2026-10-01-packs/brands/subcult-os`, direction "The terminal ledger")
owns the marks. Its colors and type restate this repository's dark tokens, so
`contracts/design/tokens.json` remains the source for interface color.

`python3 scripts/brand-assets.py --studio-root <subcult-studio checkout>` exports
the web selection and writes `docs/brand-provenance.json` with the Studio
revision, each source hash, the derivation and the exported hash. Exporting
needs Pillow and `rsvg-convert`. `make check-contracts` runs the script with
`--check`, which compares the committed files with that record and needs
neither Studio nor the render tools.

| File | Source in the pack | Use |
| --- | --- | --- |
| `web/public/brand/mark.svg` | `logos/mark-primary-outlined.svg`, viewBox cropped to the frame | SVG favicon and the mark in the `Brand` lockup |
| `web/public/favicon-32.png` | render of the cropped mark | Favicon for browsers without SVG icon support |
| `web/public/apple-touch-icon.png` | `social/avatar.png` at 180 px | Home-screen icon |
| `web/public/og-image.png` | `social/banner-x-bluesky.png` centered on a 1200×630 black canvas | Link preview image |

`Brand` in `web/src/ui/Brand.tsx` sets the mark beside the wordmark "Subcult OS"
in live Space Mono bold, so the wordmark follows the theme and stays selectable
text. The pack uses the same dark-panel mark on light and dark backgrounds.
Follow the pack guide: keep a quarter of the mark height clear, never show the
mark below 32 px, and do not stretch, recolor or redraw it. Authentication,
invitation, identity, consent and operator home headers use the lockup. Public
event, ticket and door screens keep event identity first and carry no lockup.

The link preview image is the same for every URL because the web client renders
routes in the browser. Crawlers need an absolute address, so the web build
writes `PUBLIC_WEB_URL` into `og:image` and falls back to
`https://os.subcult.tv` when it is unset. The mobile app still ships the Expo placeholder icons;
the pack has no approved app-icon export yet.

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
radius becomes square except styles named `avatar`, and text styles get the
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
from the existing Subcults `@fontsource/space-mono` 5.2.9 package.
`web/src/assets/fonts/provenance.json` records source filenames and SHA-256 hashes. They load locally
without a runtime font-provider request. Other scripts fall back to the installed
monospace font. Native themed StyleSheet factories apply monospace typography and
square corners centrally; isolated inline styles and native font loading still
need device review. Existing interaction and data contracts remain unchanged.

The token generator computes contrast from actual color values. The historical
Subcults design document contains inaccurate contrast figures and is not used as
contrast evidence. Purple actions use white text in both themes; cyan focus and
visible borders distinguish secondary actions and fields.
