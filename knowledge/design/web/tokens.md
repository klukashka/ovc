# Design tokens

The values behind the quiet visual language described in [the product overview](../../../docs/product.md#visual-language) and decided in [ADR-0010](../../../docs/decisions/0010-quiet-visual-language.md). Tokens are CSS custom properties consumed through the Tailwind theme, defined in `frontend/src/styles.css` with Tailwind's default palette, type scale, radii, shadows and breakpoints cleared; components never use raw values. Contrast ratios below were computed with the WCAG 2.2 relative-luminance formula on 2026-10-20. The [clickable prototype](prototype/README.md) shows the tokens in use.

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
| `text-display` | Literata | 36 / 44 | 400 | The Today and Home titles and greetings on computers (design slice step L, pending owner review) |
| `text-title` | Literata | 28 / 36 | 400 | Page titles at every width, except Today and Home on computers; dialog titles, the assignment title (design slice step L, pending owner review) |
| `text-heading` | Golos Text | 18 / 26 | 500 | Join block state, large inline headings |
| `text-body` | Golos Text | 15 / 22 | 400 | Body text on computers |
| `text-body-phone` | Golos Text | 16 / 24 | 400 | Body text below 768 px and every input at any width (16 px avoids zoom on iOS) |
| `text-strong` | Golos Text | 15 / 22 | 500 | Names in rows, button labels, emphasis |
| `text-small` | Golos Text | 14 / 20 | 400 | Secondary lines, sidebar items, table cells |
| `text-label` | Golos Text | 13 / 18 | 600 | Section labels above lists, in `text-muted`, sentence case (never upper case); at weight 500, counts in the sidebar and timestamps (interface rework stage, change U2, pending owner review) |
| `text-caption` | Golos Text | 12 / 16 | 500 | Only the phone frame: the bottom bar's labels and count badges and the More sheet's avatar and counts, whose phone design is deferred (owner's decision of 2026-11-23 on change U2, pending owner review) |

Times, dates and counts use tabular figures (`font-variant-numeric: tabular-nums`).

## Spacing, radius, elevation, layout

- **Spacing:** a 4 px base (`--spacing: 4px` in the Tailwind theme, independent of the root font size) with the steps 4, 8, 12, 16, 24, 32, 40, 48 and 64. Sections on a page are 40 apart; list rows have 13 px vertical padding.
- **Radius:** `radius-s` 6 (sidebar items, small inputs, segmented controls), `radius-m` 8 (buttons, inputs), `radius-l` 10 (panels, dialogs), `radius-full` (avatars, state and tutor dots). In the Tailwind theme they are `--radius-sm`, `--radius-md`, `--radius-lg` and `--radius-full` (utilities `rounded-sm`, `rounded-md`, `rounded-lg`, `rounded-full`), because `rounded-s` is Tailwind's start-side utility.
- **Elevation:** `shadow-raised` `0 8px 24px rgba(38,36,31,.12)` for menus, popovers and dialogs only. Dark theme uses a `border-strong` outline instead.
- **Targets:** from 1024 px with a fine pointer, buttons and inputs are at least 40 px tall, sidebar items 32 px tall and the sidebar's icon buttons 28 × 28 px; below 1024 px or with a coarse or absent pointer (`pointer: coarse` or `pointer: none`), every target is at least 44 × 44 px. Links inside running text are exempt, as WCAG 2.5.8 allows.
- **Layout widths:** the sidebar is 248 px; page content is centred with a maximum of 1120 px and side padding of 56 px; reading width is at most 720 px, and running text inside it at most 68 `ch` (`--container-running-text`, utility `max-w-running-text`; interface rework stage, change U2, pending owner review); the main column and a 300 px side column (`--container-aside`) sit 64 px apart. Breakpoints are 768, 1024 and 1280 px.
- **Page widths** (design slice step L, pending owner review): signed-in pages use the full page content width by default (`Page` with `width="page"`); public pages, the token pages and short forms keep the reading width (`width="reading"`). Until their own design slice step, the student card and group page (step D5) also keep the reading width; Today and Home take the page width from step D2.
- **Side column** (owner's decision of 2026-11-08, pending owner review): from 1280 px the main column and the side column sit side by side; below 1280 px the side column follows the main column in one column, because at 1024 px a side column would leave the main column only 300 px. The side column comes after the main column in the document, so reading and tab order match the stacked order.

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
| `scrim` | `rgba(38,36,31,.32)` | `rgba(0,0,0,.5)` | The dimmed page behind a modal overlay (decorative) |
| `mine` | `#ECEEDF` | `#2B3023` | The bubble of the caller's own messages in Messages; others' bubbles use `surface` (interface rework stage, specified 2026-11-23 by change U1b from the prototype the owner approved, pending owner review; built by M1 on 2026-11-24) |

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
| `text` / `text-muted` / `accent-text` on `mine` (computed 2026-11-23 with the WCAG 2 formula) | 13.2 / 4.9 / 6.8 | 11.1 / 5.5 / 7.3 |

Every text pair meets WCAG AA for normal text (4.5:1); `icon`, `border-strong` on every background a control sits on (`bg`, `surface`, `field`) and the focus ring meet 3:1. Controls therefore never change their own fill to `hover`: a hovered quiet button fills with `surface`, and the segmented control's track is `hover` with its selected segment on `field` outlined by `border-strong` (3.6:1 against the segment's own `field`; against the track it is 2.8:1, so the pressed segment also changes its label to `text` and weight 500). The current sidebar item's `hover` fill is decorative; it is identified by weight 500 and `aria-current`. `border` (1.2:1 light, 1.4:1 dark) is used only where the boundary carries no meaning.

### Tutor colours

A relationship's colour labels a tutor's items in the student's views ([states](states.md#tutor-labels)) as an 8 px dot. It is always shown together with the tutor's name or initials, never alone. The palette is muted and earthy so it sits beside the olive accent without competing with it; `slate` is its most vivid blue, which keeps it apart from `info` (design slice step C, pending owner review).

| Key | Word | Light | Dark | On `bg` / `surface` (light) | On `bg` / `surface` (dark) |
|---|---|---|---|---|---|
| `clay` | brown | `#6E3D22` | `#C29582` | 8.1 / 8.5 | 6.5 / 6.0 |
| `slate` | blue | `#13759D` | `#5E9AC8` | 4.7 / 4.9 | 5.7 / 5.3 |
| `moss` | green | `#2B5037` | `#848F7E` | 8.3 / 8.7 | 5.1 / 4.7 |
| `plum` | purple | `#7A5E81` | `#B183AE` | 5.1 / 5.3 | 5.5 / 5.1 |
| `ochre` | mustard | `#7F6B37` | `#CFB87C` | 4.7 / 4.9 | 8.9 / 8.2 |
| `teal` | teal | `#2F7475` | `#54AEA4` | 4.9 / 5.1 | 6.5 / 6.0 |
| `rust` | pink | `#9D5264` | `#F5ABBD` | 5.0 / 5.2 | 9.4 / 8.7 |
| `ink` | grey | `#50555E` | `#BFC0CD` | 6.8 / 7.1 | 9.5 / 8.8 |

Measured on 2026-11-08 with CIE76 (sRGB to CIE Lab, D65): the closest palette pair is `plum`–`ink` at 21.7 in the light theme and `clay`–`ochre` at 24.4 in the dark theme; the closest value to a state colour (`info`, `success`, `warning`, `danger`, `live`, `accent`) is `moss`–`success` at 12.4 and `ink`–`info` at 14.9. The thresholds are at least 20 between palette values and at least 12 to a state colour in both themes, and `frontend/scripts/palette.test.ts` computes all three measures from `styles.css` (design slice step C, pending owner review). The keys keep their stored names; the words people hear and see in tooltips follow the colours.

Group colours use the same palette.

## Components

- **Sidebar item:** 16 px outline icon in `icon`, label in `text-small`, optional count in `text-caption` `text-muted` aligned right; `radius-s`; `hover` background on hover and for the current page, where the label turns `text` and weight 500. Unavailable items are never shown.
- **Bottom bar** (slice 10 change G0, pending owner review; phones below 768 px, [navigation](navigation.md#frames)): 56 px plus the bottom safe area, `sidebar` background, a 1 px `border` rule above; equal-width items, each a 20 px outline icon in `icon` above a label in `text-caption` `text-muted`, at least 44 × 44 px; the current item's icon and label in `text`, the label at weight 600, on a 32 × 24 px `hover` pill behind the icon; a count is a badge at the icon's top right in `text-caption` `text` on `surface` with a 1 px `border-strong` outline and `radius-full`.
- **Sheet** (slice 10 change G0, pending owner review): a modal dialog anchored to the bottom edge, `surface` background, `radius-l` on its top corners only, `shadow-raised` (a `border-strong` outline in the dark theme), at most 80 % of the window's height, over the same dimmed page as the sidebar overlay; rows at least 48 px high with a 20 px icon and a label in `text-body-phone`, separated by 1 px `border` hairlines.
- **Row:** a grid of columns on one baseline, 1 px `border` above, the last row also closed by a rule below. A whole-row link highlights with `surface` on hover.
- **Page header** (design slice step L, pending owner review): one line with the title on the left and, on the right, the page's main action and a menu for its rare actions; it wraps below the title when the line is too narrow.
- **Table** (design slice step L, pending owner review): lists of people and things are semantic tables with column headers in `text-label` `text-muted`, rows separated by 1 px `border` hairlines with no striping, the name cell as the row header holding the row's link when it has one, and row actions in the last cell, so a row never nests buttons inside a link. A row highlights with `surface` on hover. Long words wrap inside their cell, except a name that is the row's link in Students and Groups, which is cut with an ellipsis on one line. Below 768 px secondary columns are hidden and their content is shown under the name in the first cell, so the table never scrolls sideways and keeps its table semantics.
- **Settings row** (design slice step L2, pending owner review): a 1 px `border` rule above, the label in `text-strong` with an optional `text-small` `text-muted` description in a 240 px column (`--container-settings-label`), the value or control beside it from 768 px and below it under 768 px.
- **Composer** (design slice step D3, pending owner review): a field on `field` with a `border-strong` outline that starts at one line and grows to eight, an icon button for attaching files with its accessible name, and the primary button for Send; it sits under a 1 px `border` rule at the bottom of the conversation.
- **Swatch picker** (design slice step C, pending owner review): a group colour is chosen from eight round 32 px swatches with no visible names, each a native radio covering a 44 px target (40 px from 1024 px with a fine pointer); the radio's accessible name and the tooltip are the plain colour word; the chosen swatch has a 2 px `text` ring with a 2 px offset and a check mark in `bg`, so the selection never depends on hue; the focus ring is the shared `focus` ring outside it; arrow keys move the choice as native radios do.
- **Filter row** (design slice step L, pending owner review): a list's tabs, search and filters sit in one row above its table from 1024 px, and wrap below.
- **Primary button:** `accent` fill, `on-accent` label, `radius-m`, 40 px tall (48 px for the join button).
- **Quiet button:** transparent with a 1 px `border-strong` outline and `text` label; hover fills with `surface`.
- **Text button:** `text-muted` label with no box; `text` on hover.
- **Tabs** switch between panels of one view (the homework status tabs, the issue page's Submissions and Analytics): text labels in `text-muted`; the current tab is `text`, weight 500, with a 2 px `accent` underline on the shared rule. They follow the ARIA tabs pattern: `tablist`, `tab` with `aria-selected` and `aria-controls`, `tabpanel`, a roving `tabindex` and the arrow, Home and End keys. When a tab changes the address, the tabs are links with `aria-current="page"` instead.
- **Segmented control** filters a view (the student's tutor filter): a labelled `group` of buttons with `aria-pressed` on a `hover` track; the pressed segment sits on `field` with a `border-strong` outline and `text` label, the others are `text-muted`.
- **Panel:** `surface` with a 1 px `border` and `radius-l`, padding 20–24 px.
- **News dot:** a 7 px `accent-text` dot on an icon button that hides counts (the hidden sidebar's "Show sidebar" button); never the only signal, the accessible name says it too.
- **Inputs:** `field` background, 1 px `border-strong`, `radius-m`, `text-body-phone` size at every width. A large title input (the assignment title) keeps at least a bottom `border-strong` rule.

## Shared interface rules

The interface rework stage's shared rules ([plan](../../plans/ui-rework.md), D4 and D5, from the UI guidelines research's gaps G5, G9 to G13, G17, G19 to G21 and G23), written once here and built once in `frontend/src/components/ui/` (interface rework stage, specified 2026-11-23 by change U1b from the prototype the owner approved, pending owner review; built by change U2 in `Form.tsx` (`FormActions`), `FormProblems.tsx` (the error summary and the field problems), `Tabs.tsx` (`RouteTabs`, `SegmentedControl`) and `formRoute.ts` (long forms as pages), pending owner review):

- **Dialog and form buttons:** the primary or danger action comes first, on the left, then a quiet Cancel. A danger button uses `danger` for its label and outline (a filled `danger` button only for the final confirmation of a deletion) and names the consequence in its label ("Delete the group", "Remove from the group"), never colour alone or "OK". The filled `danger` style is `dangerFilledButtonClass` in `Form.tsx` (change P1b, pending owner review).
- **Error summary:** a form with more than three fields shows, on a refused submit, a summary above the form in the `danger` notice style: a heading ("Check these fields") and a list of links, one per problem, each with the same wording as its inline message and moving focus to its field; the summary takes focus when it appears. Forms of three fields or fewer show inline messages only.
- **Inline errors:** an inline message clears as soon as its field becomes valid, on input, not on the next submit; a server refusal stays until the field changes.
- **Optional fields and hints:** an optional field's label ends with "(optional)"; a hint stands below the label and above the field, linked by `aria-describedby`; a placeholder is never the only label.
- **Temporarily unavailable buttons** stay focusable with `aria-disabled="true"` and say when they become available, for example "Send again in 42 s", updated at most once a second and announced only when they become available. The only such button, the confirmation resend of "Check your email", keeps its separate line "You can send it again in {seconds} s." until change L1, which changes the sign-up pages, because U2 changes no phone test beyond the approved replacements (owner's decision of 2026-11-23 on change U2, pending owner review). A button that stays unavailable until the user acts, rather than for a time, says why instead of when: the drop zone of a full `file` question on the student's work page keeps `aria-disabled="true"` with the hint "You have added the maximum of {count} files" (change H2, pending owner review).
- **Failures and results:** a failed action shows its error inline in the page, near the action, as a notice; toasts report results only, never a failure.
- **Action notices** (change P1a, pending owner review): the result of a reversible action is a notice with a `border` hairline, `radius-md`, the result text as a status and one text button ("Undo", "Add back") on its right, wrapping under the text when narrow; the button takes focus when the notice appears, and an inline refusal stands under the row in `danger` text ([pages](pages.md#shared-behavior)).
- **Reading width:** running text (descriptions, instructions, feedback, legal text) is capped at 68 `ch`; tables, the calendar and Messages are not. U2 applies it to the restricted Markdown renderer (instructions, feedback, descriptions) and the lesson description (pending owner review).
- **Type scale:** `text-caption` merges into `text-label`: counts and timestamps use `text-label` at weight 500, section labels keep weight 600; `text-small` and `text-body` stay two sizes on computers as the prototype shows them, and the phone sizes do not change, because the phone design is deferred: the phone frame keeps `text-caption` (owner's decision of 2026-11-23 on change U2, pending owner review).
- **Tabs and filters:** one pattern per concept: ARIA tabs (`tablist`, arrow keys) for panels inside one page, links styled as tabs for sections that are routes (the Homework tabs, Settings sections), a segmented control for a filter of up to four options and a select beyond four. The back link keeps its order below the page header, as the owner decided on 2026-11-22. U2 builds the route tabs (Homework, Students and the student card's sections) and the segmented control (the student's tutor filter, Schedule's Week and Month, the review feedback's Write and Preview); in-page ARIA tabs are built with the first page that needs them (owner's decision of 2026-11-23 on change U2, pending owner review).
- **Long forms are pages:** New lesson and Issue are routes at every width (`/schedule/new`, `/homework/issue`, [navigation](navigation.md#tutor-addresses)), so resizing never switches between a dialog and a page; arriving moves focus to the page heading, a browser Back leaves them, and leaving returns focus to the control that opened the page when the page the user came from still holds it. Back, Cancel and the back link go one step back in the browser history when the page was opened from a control of OVC, and otherwise replace the page with its fallback (`/schedule`, `/homework/library`); the opening control takes focus once the page it came from renders it again, unless the user moved focus meanwhile or a result notice takes it (pending owner review).

## Illustration and iconography

- **Icons:** one outline icon set with rounded joins and 1.3 px strokes at 16 px (20 px on phones, that is below 768 px, in the frame and on every page, from slice 10 change G1; the stroke stays 1.3 px at both sizes), using `currentColor`. No emoji or typographic symbols stand in for icons.
- **Empty states:** a short heading in `text-heading`, one sentence and a primary action. Illustrations, if added later, are single-colour line drawings in `icon`, at most 160 px tall, decorative (`alt=""`).

## Motion

- **Durations:** 120 ms (hover, press), 200 ms (menus, toasts, sidebar collapse), 280 ms (dialogs); easing `cubic-bezier(.2,.8,.2,1)`.
- **The live dot** pulses once every 2 seconds.
- **With `prefers-reduced-motion: reduce`:** transitions are removed, changes appear at once, and the pulse stops.

## Theme selection

The `theme` preference is `light`, `dark` or `system`; `system` follows `prefers-color-scheme`. The chosen theme is applied before first paint to avoid a flash.
