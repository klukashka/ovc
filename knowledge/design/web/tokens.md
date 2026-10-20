# Design tokens

The values behind the warm, friendly visual language described in [the product overview](../../../docs/product.md#visual-language). Tokens are CSS custom properties consumed through the Tailwind theme; components never use raw values. Contrast ratios below were computed with the WCAG 2.2 relative-luminance formula on 2026-10-20.

## Typefaces

- **Nunito** (rounded, warm; includes Cyrillic and Cyrillic Extended) for all interface text, weights 400, 600, 700 and 800.
- **JetBrains Mono** (includes Cyrillic) for codes such as the tutor code and for code blocks in restricted Markdown.
- Both are self-hosted as WOFF2 files under the OVC origin ([ADR-0008](../../../docs/decisions/0008-hosting-and-data-residency.md)); no font CDN.

## Type scale

| Token | Size / line height | Weight | Use |
|---|---|---|---|
| `text-display` | 32 / 40 | 800 | Page greeting on Today and Home |
| `text-title` | 24 / 32 | 700 | Page titles |
| `text-heading` | 18 / 26 | 700 | Card and section headings |
| `text-body` | 16 / 24 | 400 | Body text, inputs (16 px avoids zoom on iOS) |
| `text-body-strong` | 16 / 24 | 600 | Emphasis, button labels |
| `text-small` | 14 / 20 | 400 | Secondary text, table cells |
| `text-caption` | 12 / 16 | 600 | Badges, timestamps |

## Spacing, radius, elevation

- **Spacing:** a 4 px base with the steps 4, 8, 12, 16, 24, 32, 48 and 64. Card padding is 16 on phones and 24 from 768 px.
- **Radius:** `radius-s` 8 (inputs, badges), `radius-m` 12 (buttons), `radius-l` 16 (cards, dialogs), `radius-full` (avatars, pills).
- **Elevation:**
  - `shadow-card`: `0 1px 2px rgba(42,33,28,.06), 0 2px 8px rgba(42,33,28,.06)`;
  - `shadow-raised`: `0 8px 24px rgba(42,33,28,.12)` for dialogs and menus.

  Dark theme uses borders instead of shadows.
- **Touch targets:** at least 44 × 44 px on every interactive element.
- **Layout widths:** content max 1200 px; reading width max 720 px; breakpoints 480, 768, 1024 and 1280 px.

## Colours

| Token | Light | Dark | Use |
|---|---|---|---|
| `bg` | `#FFF9F2` | `#1A1512` | Page background (warm cream / warm charcoal) |
| `surface` | `#FFFFFF` | `#241E19` | Cards, dialogs, inputs |
| `surface-sunken` | `#F6EEE3` | `#14100D` | Wells, sidebar, empty-state backgrounds |
| `border` | `#E6D8C6` | `#3A3029` | Decorative card dividers only |
| `border-strong` | `#8A7868` | `#857466` | Input and control boundaries (≥ 3:1) |
| `text` | `#2A211C` | `#F4ECE3` | Primary text |
| `text-muted` | `#6A5B50` | `#B8A899` | Secondary text |
| `accent` | `#B8482A` | `#F2875F` | Primary buttons, active navigation (terracotta) |
| `on-accent` | `#FFFFFF` | `#1A1512` | Text on `accent` |
| `accent-text` | `#9C3B21` | `#F5A07E` | Links and accent text |
| `accent-soft` | `#FBE4D8` | `#3D2419` | Selected rows, highlights |
| `focus` | `#2D62C8` | `#7FA7F2` | 2 px focus ring with 2 px offset |
| `success`, `success-soft` | `#2C7A4B`, `#E1F2E7` | `#5CC28A`, `#16301F` | Checked, accepted |
| `warning`, `warning-soft` | `#9A5B00`, `#FBEFD9` | `#E6A84A`, `#33260F` | Due soon, late, needs attention |
| `danger`, `danger-soft` | `#B42318`, `#FCE5E2` | `#F2766B`, `#3A1A17` | Errors, destructive actions, cancelled |
| `info`, `info-soft` | `#2D62C8`, `#E3ECFB` | `#7FA7F2`, `#18233A` | Submitted, informational |
| `live`, `live-soft` | `#C2185B`, `#FBE3EC` | `#F27BA8`, `#3A1726` | Lesson live now, recording |

Measured contrast:

| Pair | Light | Dark |
|---|---|---|
| `text` on `bg` / `surface` | 15.1 / 15.8 | 15.5 / 14.1 |
| `text-muted` on `surface` / `surface-sunken` | 6.5 / 5.7 | 7.1 / 8.2 |
| `on-accent` on `accent` | 5.3 | 7.3 |
| `accent-text` on `surface` / `accent-soft` | 6.9 / 5.6 | 8.0 / 7.0 |
| `success` on `surface` / `success-soft` | 5.3 / 4.5 | 7.5 / 6.5 |
| `warning` on `surface` / `warning-soft` | 5.4 / 4.8 | 7.9 / 7.1 |
| `danger` on `surface` / `danger-soft` | 6.6 / 5.5 | 6.0 / 5.7 |
| `info` on `surface` / `info-soft` | 5.7 / 4.8 | 6.8 / 6.5 |
| `live` on `surface` / `live-soft` | 5.9 / 4.8 | 6.4 / 6.1 |
| `focus` on `bg` | 5.4 | 7.5 |
| `border-strong` on `surface` / `bg` | 4.2 / 4.0 | 3.7 / 4.0 |

Every text pair meets WCAG AA for normal text (4.5:1); every control boundary and the focus ring meet 3:1. `border` (1.4:1 light, 1.3:1 dark) is used only where the boundary carries no meaning.

### Tutor colours

A relationship's colour labels a tutor's items in the student's views ([states](states.md#tutor-labels)). It is always shown together with the tutor's name or initials, never alone.

| Key | Light | Dark | On `surface` (light / dark) |
|---|---|---|---|
| `terracotta` | `#B8482A` | `#F2875F` | 5.3 / 6.6 |
| `green` | `#2C7A4B` | `#5CC28A` | 5.3 / 7.5 |
| `blue` | `#2D62C8` | `#7FA7F2` | 5.7 / 6.8 |
| `violet` | `#8E44AD` | `#C49AE0` | 5.9 / 7.1 |
| `amber` | `#9A5B00` | `#E6A84A` | 5.4 / 7.9 |
| `rose` | `#C2185B` | `#F27BA8` | 5.9 / 6.4 |
| `teal` | `#00796B` | `#4DC2B0` | 5.3 / 7.6 |
| `cocoa` | `#5D4037` | `#C9A28E` | 9.3 / 7.1 |

Group colours use the same palette.

## Illustration and iconography

- **Icons:** one outline icon set with rounded joins, at 20 and 24 px, using `currentColor`.
- **Empty-state illustrations:** flat, warm, two-tone (`accent-soft` plus `text-muted` line work), at most 200 px tall. They are decorative (`alt=""`) and always accompanied by a heading, one sentence and a primary action.

## Motion

- **Durations:** 120 ms (hover, press), 200 ms (menus, toasts), 280 ms (dialogs, page transitions); easing `cubic-bezier(.2,.8,.2,1)`.
- **The live indicator** pulses once every 2 seconds.
- **With `prefers-reduced-motion: reduce`:** all transitions become instant cross-fades and the pulse stops.

## Theme selection

The `theme` preference is `light`, `dark` or `system`; `system` follows `prefers-color-scheme`. The chosen theme is applied before first paint to avoid a flash.
