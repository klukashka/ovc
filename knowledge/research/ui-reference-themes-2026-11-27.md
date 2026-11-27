# Themes: reference study, checked 2026-11-27

Status: agent research, checked 2026-11-27 from public first-party sources; decides nothing. Product behavior is volatile; recheck before relying on it. Supplies the reference study that the AGENTS.md UI work rule requires for R0 (`feat/redesign-themes`) of the umbrella redesign plan `knowledge/plans/redesign.md` (revision 3 at commit `8b3a8b0`, approved by the owner on 2026-11-27 as a sequencing plan). "FP" marks a first-party source, "TP" a third-party one; numbers refer to the source list at the end.

## Scope and OVC baseline

The subject is the colour theme of every screen: the light and dark palettes, the system option, the page background, tinted surfaces (selected rows, the outgoing message bubble, the live-lesson panel), and the contrast of text, status labels and control boundaries on each of those surfaces. The design questions are how a product organises colour into semantic roles, how it derives tinted surfaces, how it keeps secondary text and status colours legible on them, and how it proves contrast.

OVC today, per [tokens: Colours](../design/web/tokens.md#colours) and `frontend/src/styles.css`:

- Semantic tokens per theme (`bg`, `surface`, `field`, `sidebar`, `hover`, `border`, `border-strong`, `text`, `text-muted`, `icon`, `accent`, `accent-hover`, `on-accent`, `accent-text`, `focus`, the four status colours, `live`, `scrim`, `mine`), in a light block and a `[data-theme='dark']` block. Components never use raw values.
- A measured-contrast table (computed 2026-10-20, `mine` 2026-11-23): every text pair at least 4.5:1, `icon`, `border-strong` and the focus ring at least 3:1 on every background a control sits on; `border` is decorative only (1.2:1 light). Hovered controls never fill with `hover`, because `border-strong` would drop below 3:1 there.
- `mine` (`#ECEEDF` / `#2B3023`) is the caller's own message bubble; others' bubbles use `surface`.
- [Theme selection](../design/web/tokens.md#theme-selection): `light`, `dark` or `system`, applied before first paint. The background is already one flat colour (plan section 4, Themes row); `styles.css` holds no gradient or background image.
- `frontend/scripts/palette.test.ts` reads `styles.css` and tests only the eight tutor colours: CIE76 distance at least 20 between palette values and 12 to state colours, and 4.5:1 on `bg` and `surface`. It has no test of the semantic text, surface or boundary pairs.

Owner's decisions that bind R0 (plan section 3): item 2 chooses "Warm paper" for light; item 3 chooses "Warm paper · night, ink" for dark and gives the pair precedence over colours drawn in individual mockups; item 4 requires one flat page background with no glow, grid or gradient; item 5 asks for outgoing bubbles "with an accent tint". The palette table after item 18 fixes ten roles per theme; the derived samples are `tint` `#E9EBE4` / `#302E24`, `bubble` `#DDE0D5` / `#333727`, `livepanel` `#ECE9DE` / `#24251D` and `strong` `#9C978B` / `#817965`. Plan section 7, R0, maps `bubble` to `mine`, forbids lowering thresholds, names three failing light pairs and the first usage fixes, and sends any remaining failure to the owner as an alternative value.

Mockups (`knowledge/design/web/prototype/redesign/` at `8b3a8b0`): `palettes.html` shows six palettes on the same Today screen and three background treatments (a soft corner glow, a dot grid, a sidebar-to-background gradient), of which item 4 keeps none. `theme-pairs.html` shows the light reference beside three dark candidates on Today, a message pair, the lobby and an action menu; each board states its own minimum over six pairs (light 5.21:1, chosen dark 6.56:1).

### Ratios recomputed for this study

Computed on 2026-11-27 with the WCAG 2.2 relative-luminance formula (the same formula as `palette.test.ts`), unrounded comparison:

| Pair | Ratio | Threshold | Result |
|---|---|---|---|
| muted `#6A6458` on bubble `#DDE0D5` | 4.389:1 | 4.5:1 text | fails |
| live `#B9442F` on sidebar `#EDE8DC` | 4.370:1 | 4.5:1 text | fails |
| strong `#9C978B` on white | 2.911:1 | 3:1 boundary | fails |
| text `#24221D` on bubble | 11.878:1 | 4.5:1 | passes |
| live on `bg` `#F5F1E8` / on white | 4.740 / 5.342:1 | 4.5:1 | passes |
| live on `livepanel` `#ECE9DE` / on `tint` `#E9EBE4` / on bubble | 4.395 / 4.444 / 3.994:1 | 4.5:1 | fail (not listed in the plan) |
| muted on `sidebar` / `tint` / `livepanel` | 4.803 / 4.884 / 4.831:1 | 4.5:1 | pass |
| current `border-strong` `#8F8676` on white / new `bg` | 3.596 / 3.190:1 | 3:1 | pass |
| current `border-strong` on new `sidebar` / `tint` | 2.941 / 2.991:1 | 3:1 | fail |
| strong `#9C978B` on new `bg` | 2.583:1 | 3:1 | fails |
| dark: muted `#ADA593` on bubble `#333727` | 4.999:1 | 4.5:1 | passes |
| dark: live `#E8907F` on sidebar `#1F1C17` | 7.056:1 | 4.5:1 | passes |
| dark: strong `#817965` on surface `#24211B` / `bg` | 3.712 / 4.061:1 | 3:1 | pass |

The three plan values are confirmed to two decimals (4.39, 4.37, 2.91). Three further light pairs fail if the live colour is used as text on a tint, and the existing `border-strong` falls just below 3:1 on the new sidebar and tint.

## How the compared products solve it

### Telegram

- Each of the 8 chat themes introduced in 2021 "comes with a day and night version" and follows the app's night mode settings; the themes use gradient message bubbles, animated backgrounds and patterns [8].
- The theme editor has Main Color, Background and My Messages tabs, so the outgoing bubble is styled separately from the base theme, and gradients can be added to messages and background [9].
- Night mode on Android has a switch at the top of the menu, and on iOS the theme can sync with the system dark mode [9].
- The theme API defines five base themes (classic, day, night, tinted, arctic), a required `accent_color`, an optional `outbox_accent_color` and optional `message_colors` [10]. The page does not explain how bubble colours are derived from them [10].

### Notion

- Appearance offers "Use system setting", "Light" and "Dark" on desktop and mobile, with cmd/ctrl + shift + L to toggle dark mode [6].
- "High contrast" is a separate setting ("Use system setting", "Standard", "High contrast"), desktop and web only, saved per device [6].

### Linear

- The 2024 redesign replaced 98 variables per theme with three inputs: a base colour, an accent colour and a contrast value, and generates surfaces, text, icons and controls from them in the LCH colour space, chosen because equal lightness looks equally light [4].
- The contrast input lets Linear produce very high-contrast themes for accessibility; the default light and dark themes use the same generator [4].
- The later refresh moved from a cool, blue-leaning grey to "a warmer gray", warning that too warm "risks looking muddy"; borders and separators were softened and the sidebar made "a few notches dimmer" so the content area leads [5]. No contrast figures are given [5].

### GitHub Primer

- Three token levels: base tokens (raw scale values, never used directly), functional tokens (`bgColor-default`, `fgColor-default`, borders, shadows) that change with the colour mode, and a few component tokens such as `focus-outlineColor` [1, 2].
- A 14-step neutral scale: steps 0–6 backgrounds, 7–8 borders and dividers, 9–10 text and icons, 11–13 high-contrast text [1].
- Minimum contrast is stated per role and measured against the darker muted background: step 8 for interactive control borders against `bgColor-muted`, step 9 for text on steps 0–4, step 10 on steps 5–6 [1, 2].
- Muted backgrounds pair with semantic foregrounds (`fgColor-{role}`) for subtle emphasis; emphasis backgrounds pair with `fgColor-onEmphasis` [1].
- High-contrast themes aim for at least 7:1 for most text and interactive elements [1, 2]. GitHub offers light, dark, high-contrast and colour-blind themes, a single theme or separate day and night themes that follow the system [3].

### Apple Human Interface Guidelines

- Use semantic colours that adapt to the appearance and avoid hard-coded colours; custom colours get bright and dim variants [11].
- Contrast "no lower than 4.5:1", and for custom colours "strive for" 7:1, especially in small text [11]. The accessibility table gives 4.5:1 up to 17 pt and 3:1 at 18 pt or bold [12].
- Dark Mode uses dimmer base and brighter elevated backgrounds to show layering [11].
- Avoid an app-specific appearance setting that ignores the system choice [11]; test both appearances with Increase Contrast and Reduce Transparency [11, 12].
- Do not convey information with colour alone; add shapes or icons for state [12].

### Material Design 3 (supplementary)

- Content colours are paired with container roles (`onPrimaryContainer` on `primaryContainer`, `onSurface` on `surface`), with five `surfaceContainer` levels by emphasis [13].
- `outline` "adds contrast for accessibility purposes"; `outlineVariant` is for decorative boundaries "when strong contrast is not required" [13].

### WCAG 2.2

- SC 1.4.3 (AA): 4.5:1 for text, 3:1 for large text (18 pt, or 14 pt bold); the ratio is a threshold, so 4.499:1 fails; disabled controls and decoration are exempt [14].
- SC 1.4.6 (AAA): 7:1, and 4.5:1 for large text [15].
- SC 1.4.11 (AA): 3:1 against adjacent colours for visual information needed to identify a control or its state; a boundary is needed only when nothing else shows the control is there; author-added hover effects need not meet 3:1 [16].

## Comparison

| Question | Telegram | Notion | Linear | Primer | Apple HIG | OVC target |
|---|---|---|---|---|---|---|
| Light, dark, system | Day and night per theme; system sync [8, 9] | System, Light, Dark [6] | Light, dark, custom [4] | Single or day/night by system [3] | Follow the system [11] | Keep light, dark, system |
| Palette organisation | Base theme plus accent and outgoing colours [10] | Not documented | Three inputs, LCH-generated aliases [4] | Base, functional and component tokens [1] | Semantic, adaptive colours [11] | Keep semantic tokens; ten roles fixed by the owner |
| Neutral warmth | Unverified | Not documented | Warmer grey, not muddy [5] | Neutral scale [1] | Not documented | Warm paper (items 2–3) |
| Page background | Patterned wallpapers in chats [8] | Not documented | Dimmer sidebar [5] | `bgColor-default` [1] | System backgrounds [11] | Flat colour (item 4) |
| Outgoing bubble | Separate "My Messages" colour, gradients [9] | n/a | n/a | n/a | n/a | Accent tint, `bubble` mapped to `mine` |
| Contrast proof | Not documented | High-contrast setting [6] | Contrast input [4] | Per-role minima against the muted background [1] | 4.5:1, aim 7:1 [11] | Test every actual pair, thresholds unchanged |
| Control boundaries | Not documented | Not documented | Softer separators [5] | Step 8 minimum [1] | Not documented | `border-strong` at 3:1, `border` decorative |
| High-contrast option | Not documented | Separate setting [6] | Contrast input [4] | 7:1 themes [1] | Increase Contrast [12] | Out of R0 scope |

## What OVC adopts

- **Semantic tokens with derived tints** (Primer [1, 2], Apple [11], Material [13]): map `bubble` to `mine` and add tint and live-surface tokens only where a component needs one, as R0's scope says; components never take the board's sample names.
- **Per-role minima measured against the darkest surface the role sits on** (Primer [1]): extend `palette.test.ts` from tutor colours to every text, status and boundary pair on `bg`, `surface`, `sidebar`, `field`, `mine` and each new tint, in both themes. This is R0's acceptance criterion; the darkest light surfaces are `bubble` and `sidebar`.
- **Usage fixes before value changes** (WCAG 1.4.3, 1.4.11 [14, 16]; Material's `outline`/`outlineVariant` split [13]): time in the light outgoing bubble uses `text` (11.88:1); the live label on the light sidebar uses `text` with the live dot, so colour is not the only cue (Apple [12]); control boundaries keep `border-strong`, with the board's `strong` treated as decorative.
- **Separate outgoing-bubble colour** (Telegram's "My Messages" [9]): one flat accent-tinted fill without gradients, matching item 5 and ADR-0010's quiet visual language.
- **Warm neutrals tuned against muddiness and a quieter sidebar** (Linear [5]): consistent with items 2–4; the rendered review in both themes checks this.
- **System as the default choice** (Apple [11], GitHub [3], Notion [6]): the existing `system` option and no-flash initialisation stay.

## Where OVC differs and why

- **No wallpaper, pattern or gradient** (unlike Telegram [8, 9]): item 4 and ADR-0010 require one flat background; an acceptance test asserts `background-image: none`.
- **No custom themes or generated palettes** (unlike Linear [4] and Telegram [9]): the owner fixed exact values; a generator would replace them silently, which the plan forbids.
- **The app keeps its own theme setting** although Apple advises against one [11]: OVC is a web app, its `system` default follows the device, and the explicit setting is an existing contract (plan item 15).
- **AA, not AAA** (Apple's 7:1 aim [11], Primer's high-contrast 7:1 [1], SC 1.4.6 [15]): R0 keeps AA thresholds unchanged; a high-contrast theme like Notion's [6] would be a separate proposal.
- **No rounding**: 4.39:1 fails although the board's text rounds generously; SC 1.4.3 treats the ratio as a threshold [14].

## Open points for the task plan

- Which of the newly found light failures matter in use: `live` as text on `livepanel` (4.395:1), on `tint` (4.444:1) and inside a bubble (3.994:1). If the live panel shows "Live now" in the live colour, the usage fix of the sidebar (text colour with the dot) or an owner-approved value is needed.
- Whether any control sits on `sidebar` or a tint (a sidebar search field, a selected chat-list row with a button, the sidebar collapse control): the current `border-strong` gives 2.941:1 on `sidebar` and 2.991:1 on `tint`. Options are a usage rule (controls in the sidebar sit on `field`) or an alternative value brought to the owner.
- New values for `field`, `hover`, `icon`, `focus`, `accent-hover`, `border-strong` and the status colours, which the owner's table does not give; each needs a proven pair or an owner choice, and hovered controls must still not fill with a colour that drops their boundary below 3:1.
- Whether the eight tutor colours keep CIE76 distance 20/12 and 4.5:1 on the new `bg` and `surface` (both changed), and also on `sidebar` and `mine` if dots appear there.
- Exact token names for `tint` and `livepanel` (for example `selected` and `live-surface`) and which components consume them.
- Whether the message time and read ticks in incoming bubbles (`surface`, white) keep `text-muted` (5.871:1) while outgoing ones switch to `text`, and whether the ticks, as icons, need only 3:1.

## Unverified

- That Telegram's default day theme tints outgoing bubbles green and draws the time inside them in a tinted shade of the bubble.
- That GitHub offers a dark dimmed theme and Tritanopia themes; the theme page names only light, dark, high-contrast and colour-blind themes [3].
- Notion's actual dark palette values and whether it uses warm neutrals.
- Material 3's statement of contrast levels and tone-based pairing rules; `m3.material.io` did not render for the fetcher, so only the Compose reference [13] is cited.
- That Linear's current generator still uses LCH after the later refresh; the refresh post mentions hue, chroma and lightness controls without naming a colour space [5].
- Linear's custom themes documentation page (the address tried returned 404).

## Sources

1. GitHub Primer — Color overview — <https://primer.style/foundations/color/overview> — FP — checked 2026-11-27 — the nine themes are not named.
2. GitHub Primer — Color usage — <https://primer.style/product/getting-started/foundations/color-usage/> — FP — checked 2026-11-27 — page truncated in the fetch; `fgColor-muted` not found.
3. GitHub — Managing your theme settings — <https://docs.github.com/en/account-and-profile/setting-up-and-managing-your-personal-account-on-github/managing-personal-account-settings/managing-your-theme-settings> — FP — checked 2026-11-27.
4. Linear — How we redesigned the Linear UI — <https://linear.app/now/how-we-redesigned-the-linear-ui> — FP — checked 2026-11-27.
5. Linear — Behind the latest design refresh — <https://linear.app/now/behind-the-latest-design-refresh> — FP — checked 2026-11-27 — no contrast figures.
6. Notion — Account settings — <https://www.notion.com/help/account-settings> — FP — checked 2026-11-27.
7. Notion — Dark mode (guessed address) — <https://www.notion.com/help/dark-mode> — FP — checked 2026-11-27 — returned 404; not cited for any claim.
8. Telegram — Chat Themes, Interactive Emoji, Read Receipts in Groups and Live Stream Recording — <https://telegram.org/blog/chat-themes-interactive-emoji-read-receipts> — FP — checked 2026-11-27.
9. Telegram — Verifiable Builds, New Theme Editor, Send When Online and So Much More — <https://telegram.org/blog/verifiable-apps-and-more> — FP — checked 2026-11-27.
10. Telegram — Themes (API) — <https://core.telegram.org/api/themes> — FP — checked 2026-11-27 — field semantics not explained.
11. Apple — Human Interface Guidelines: Dark Mode — <https://developer.apple.com/design/human-interface-guidelines/dark-mode> — FP — checked 2026-11-27 — read through its JSON data address because the page renders with scripts.
12. Apple — Human Interface Guidelines: Accessibility — <https://developer.apple.com/design/human-interface-guidelines/accessibility> — FP — checked 2026-11-27 — read through its JSON data address.
13. Android Developers — Material 3 `ColorScheme` reference — <https://developer.android.com/reference/kotlin/androidx/compose/material3/ColorScheme> — FP — checked 2026-11-27.
14. W3C — Understanding SC 1.4.3 Contrast (Minimum), WCAG 2.2 — <https://www.w3.org/WAI/WCAG22/Understanding/contrast-minimum.html> — FP — checked 2026-11-27.
15. W3C — Understanding SC 1.4.6 Contrast (Enhanced), WCAG 2.2 — <https://www.w3.org/WAI/WCAG22/Understanding/contrast-enhanced.html> — FP — checked 2026-11-27.
16. W3C — Understanding SC 1.4.11 Non-text Contrast, WCAG 2.2 — <https://www.w3.org/WAI/WCAG22/Understanding/non-text-contrast.html> — FP — checked 2026-11-27.
