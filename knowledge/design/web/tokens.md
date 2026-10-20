# Design tokens

The values behind the quiet visual language described in [the product overview](../../../docs/product.md#visual-language) and decided in [ADR-0010](../../../docs/decisions/0010-quiet-visual-language.md). Tokens are CSS custom properties consumed through the Tailwind theme; components never use raw values. Contrast ratios below were computed with the WCAG 2.2 relative-luminance formula on 2026-10-20. The [clickable prototype](prototype/README.md) shows the tokens in use.

## Principles

- **Typography carries the hierarchy**, not boxes: size, weight and the muted colour separate levels.
- **Hairline rules instead of cards.** Lists are rows divided by 1 px `border` lines and aligned in columns. A bordered `surface` panel is reserved for the element that needs to stand out (the next lesson, the join block, the first-run checklist, the review side panel, the block being edited in the builder) and for a set of equal choices (the two sign-up choices); a page has one such element or one such set.
- **No shadows on page content.** `shadow-raised` exists only for menus, popovers and dialogs that float above the page.
- **One accent per screen.** The filled `accent` button marks the single primary action of a view; everything else is a quiet button, a text button or a link.
- **Status is a word**, optionally preceded by a small dot in the state colour ([states](states.md)); no filled pills.

## Typefaces

- **Golos Text** (a humanist sans designed for Cyrillic first; Latin and Cyrillic) for all interface text, weights 400, 500 and 600.
- **Literata** (a book serif; Latin and Cyrillic) only for page titles and greetings, weight 400, and for quoted tutor feedback in italic.
- **JetBrains Mono** (Latin and Cyrillic) for codes such as the tutor code and for code blocks in restricted Markdown.
- All three are licensed under the SIL Open Font License and self-hosted as WOFF2 files under the OVC origin ([ADR-0008](../../../docs/decisions/0008-hosting-and-data-residency.md)); no font CDN.

## Type scale

| Token | Family | Size / line height | Weight | Use |
|---|---|---|---|---|
| `text-display` | Literata | 36 / 44 | 400 | Page titles and greetings on computers |
| `text-title` | Literata | 28 / 36 | 400 | Page titles on phones, dialog titles, the assignment title |
| `text-heading` | Golos Text | 18 / 26 | 500 | Join block state, large inline headings |
| `text-body` | Golos Text | 15 / 22 | 400 | Body text on computers |
| `text-body-phone` | Golos Text | 16 / 24 | 400 | Body text below 768 px and every input at any width (16 px avoids zoom on iOS) |
| `text-strong` | Golos Text | 15 / 22 | 500 | Names in rows, button labels, emphasis |
| `text-small` | Golos Text | 14 / 20 | 400 | Secondary lines, sidebar items, table cells |
| `text-label` | Golos Text | 13 / 18 | 600 | Section labels above lists, in `text-muted`, sentence case (never upper case) |
| `text-caption` | Golos Text | 12 / 16 | 500 | Counts in the sidebar, timestamps |

Times, dates and counts use tabular figures (`font-variant-numeric: tabular-nums`).

## Spacing, radius, elevation, layout

- **Spacing:** a 4 px base with the steps 4, 8, 12, 16, 24, 32, 40, 48 and 64. Sections on a page are 40 apart; list rows have 13 px vertical padding.
- **Radius:** `radius-s` 6 (sidebar items, small inputs, segmented controls), `radius-m` 8 (buttons, inputs), `radius-l` 10 (panels, dialogs), `radius-full` (avatars, state and tutor dots).
- **Elevation:** `shadow-raised` `0 8px 24px rgba(38,36,31,.12)` for menus, popovers and dialogs only. Dark theme uses a `border-strong` outline instead.
- **Targets:** from 1024 px with a fine pointer, buttons and inputs are at least 40 px tall, sidebar items 32 px tall and the sidebar's icon buttons 28 × 28 px; below 1024 px or with a coarse or absent pointer (`pointer: coarse` or `pointer: none`), every target is at least 44 × 44 px. Links inside running text are exempt, as WCAG 2.5.8 allows.
- **Layout widths:** the sidebar is 248 px; page content is centred with a maximum of 1120 px and side padding of 56 px; reading width is at most 720 px; the main column and a 300 px side column sit 64 px apart. Breakpoints are 768, 1024 and 1280 px.

## Colours

| Token | Light | Dark | Use |
|---|---|---|---|
| `bg` | `#F7F4EE` | `#1C1B18` | Page background (warm paper / warm graphite) |
| `surface` | `#FBF9F4` | `#23221E` | Highlighted panels, the review side panel, dialogs |
| `field` | `#FFFFFF` | `#2A2925` | Inputs and text areas |
| `sidebar` | `#F1EEE6` | `#201F1C` | Sidebar background |
| `hover` | `#E7E3D9` | `#2E2D28` | Hovered and current sidebar items, hovered rows, segmented control track |
| `border` | `#E2DCD0` | `#37352F` | Hairline rules between rows and around panels (decorative only) |
| `border-strong` | `#8F8676` | `#7D7667` | Input and control boundaries (≥ 3:1) |
| `text` | `#26241F` | `#ECE8DF` | Primary text |
| `text-muted` | `#6B655A` | `#ABA496` | Secondary text, section labels, sidebar group labels and counts |
| `icon` | `#736C60` | `#9C9587` | Outline icons; not for text |
| `accent` | `#5E6B3A` | `#A3B275` | The one primary button per view, current-tab underline, selected radio and checkbox (olive); never a state colour |
| `accent-hover` | `#525E32` | `#B2C083` | Hovered primary button |
| `on-accent` | `#FFFFFF` | `#1C1B18` | Text on `accent` |
| `accent-text` | `#4A5530` | `#B7C48C` | Links, accent text, and the state colour for states that invite the viewer to act (starting soon, needs checking, wants to join) |
| `focus` | `#5E6B3A` | `#A3B275` | 2 px focus ring with 2 px offset |
| `success` | `#2E6B4F` | `#86C3A4` | Checked, accepted (pine, distinct from the olive accent) |
| `warning` | `#8A5A12` | `#D9A75E` | Due soon, late, needs attention |
| `danger` | `#A3352A` | `#E58A7C` | Errors, destructive actions, wrong answers, cancelled |
| `info` | `#3F5E7A` | `#93B3D1` | Submitted, informational |
| `live` | `#A3352A` | `#E58A7C` | Lesson live now, recording (always with its word and the live dot) |

Measured contrast:

| Pair | Light | Dark |
|---|---|---|
| `text` on `bg` / `surface` / `sidebar` / `hover` / `field` | 14.1 / 14.7 / 13.4 / 12.1 / 15.5 | 14.1 / 13.0 / 13.5 / 11.3 / 11.9 |
| `text-muted` on `bg` / `surface` / `sidebar` / `hover` | 5.3 / 5.5 / 5.0 / 4.5 | 7.0 / 6.4 / 6.7 / 5.6 |
| `on-accent` on `accent` / `accent-hover` | 5.8 / 7.0 | 7.5 / 8.8 |
| `accent-text` on `bg` / `surface` / `sidebar` | 7.3 / 7.6 / 6.9 | 9.3 / 8.6 / 8.9 |
| `success` on `bg` / `surface` | 5.7 / 6.0 | 8.5 / 7.9 |
| `warning` on `bg` / `surface` | 5.4 / 5.6 | 7.9 / 7.3 |
| `danger` and `live` on `bg` / `surface` | 6.2 / 6.4 | 6.8 / 6.3 |
| `info` on `bg` / `surface` | 6.2 / 6.5 | 7.9 / 7.3 |
| `icon` on `bg` / `sidebar` / `hover` | 4.7 / 4.5 / 4.1 | 5.8 / 5.5 / 4.6 |
| `border-strong` on `bg` / `surface` / `field` | 3.3 / 3.4 / 3.6 | 3.8 / 3.5 / 3.2 |
| `focus` on `bg` / `surface` | 5.3 / 5.5 | 7.5 / 7.0 |

Every text pair meets WCAG AA for normal text (4.5:1); `icon`, `border-strong` on every background a control sits on (`bg`, `surface`, `field`) and the focus ring meet 3:1. Controls therefore never change their own fill to `hover`: a hovered quiet button fills with `surface`, and the segmented control's track is `hover` with its selected segment on `field` outlined by `border-strong` (3.6:1 against the segment's own `field`; against the track it is 2.8:1, so the pressed segment also changes its label to `text` and weight 500). The current sidebar item's `hover` fill is decorative; it is identified by weight 500 and `aria-current`. `border` (1.2:1 light, 1.4:1 dark) is used only where the boundary carries no meaning.

### Tutor colours

A relationship's colour labels a tutor's items in the student's views ([states](states.md#tutor-labels)) as an 8 px dot. It is always shown together with the tutor's name or initials, never alone. The palette is muted and earthy so it sits beside the olive accent without competing with it.

| Key | Light | Dark | On `bg` / `surface` (light) | On `bg` / `surface` (dark) |
|---|---|---|---|---|
| `clay` | `#9A5B3C` | `#D49A7B` | 4.9 / 5.1 | 7.1 / 6.6 |
| `slate` | `#3F5E7A` | `#8FB0CE` | 6.2 / 6.5 | 7.6 / 7.0 |
| `moss` | `#4F6B4A` | `#9DBB95` | 5.4 / 5.6 | 8.2 / 7.6 |
| `plum` | `#7A4A6B` | `#C99BBB` | 6.3 / 6.6 | 7.3 / 6.7 |
| `ochre` | `#86621A` | `#D3B064` | 5.1 / 5.3 | 8.3 / 7.7 |
| `teal` | `#2F6B6B` | `#7FBDBA` | 5.6 / 5.8 | 8.1 / 7.5 |
| `rust` | `#A0442E` | `#E3907B` | 5.7 / 5.9 | 7.0 / 6.5 |
| `ink` | `#4A4A5E` | `#AEAEC4` | 7.9 / 8.2 | 7.9 / 7.3 |

Group colours use the same palette.

## Components

- **Sidebar item:** 16 px outline icon in `icon`, label in `text-small`, optional count in `text-caption` `text-muted` aligned right; `radius-s`; `hover` background on hover and for the current page, where the label turns `text` and weight 500. Unavailable items are never shown.
- **Row:** a grid of columns on one baseline, 1 px `border` above, the last row also closed by a rule below. A whole-row link highlights with `surface` on hover.
- **Primary button:** `accent` fill, `on-accent` label, `radius-m`, 40 px tall (48 px for the join button).
- **Quiet button:** transparent with a 1 px `border-strong` outline and `text` label; hover fills with `surface`.
- **Text button:** `text-muted` label with no box; `text` on hover.
- **Tabs** switch between panels of one view (the homework status tabs, the issue page's Submissions and Analytics): text labels in `text-muted`; the current tab is `text`, weight 500, with a 2 px `accent` underline on the shared rule. They follow the ARIA tabs pattern: `tablist`, `tab` with `aria-selected` and `aria-controls`, `tabpanel`, a roving `tabindex` and the arrow, Home and End keys. When a tab changes the address, the tabs are links with `aria-current="page"` instead.
- **Segmented control** filters a view (the student's tutor filter): a labelled `group` of buttons with `aria-pressed` on a `hover` track; the pressed segment sits on `field` with a `border-strong` outline and `text` label, the others are `text-muted`.
- **Panel:** `surface` with a 1 px `border` and `radius-l`, padding 20–24 px.
- **News dot:** a 7 px `accent-text` dot on an icon button that hides counts (the hidden sidebar's "Show sidebar" button); never the only signal, the accessible name says it too.
- **Inputs:** `field` background, 1 px `border-strong`, `radius-m`, `text-body-phone` size at every width. A large title input (the assignment title) keeps at least a bottom `border-strong` rule.

## Illustration and iconography

- **Icons:** one outline icon set with rounded joins and 1.3 px strokes at 16 px (20 px on phones), using `currentColor`. No emoji or typographic symbols stand in for icons.
- **Empty states:** a short heading in `text-heading`, one sentence and a primary action. Illustrations, if added later, are single-colour line drawings in `icon`, at most 160 px tall, decorative (`alt=""`).

## Motion

- **Durations:** 120 ms (hover, press), 200 ms (menus, toasts, sidebar collapse), 280 ms (dialogs); easing `cubic-bezier(.2,.8,.2,1)`.
- **The live dot** pulses once every 2 seconds.
- **With `prefers-reduced-motion: reduce`:** transitions are removed, changes appear at once, and the pulse stops.

## Theme selection

The `theme` preference is `light`, `dark` or `system`; `system` follows `prefers-color-scheme`. The chosen theme is applied before first paint to avoid a flash.
