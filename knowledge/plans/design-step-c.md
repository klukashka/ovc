# Design slice step C: colour by swatch and a palette that can be told apart

Status: approved by the owner on 2026-11-08, after seeing the palette samples in both themes. It carries out decisions 8 and 9 of the [interface decisions](ui-ux-decisions-2026-11-06.md#decisions) and step C of the [design slice plan](design-slice.md). Base: `main` at `8529dfe`. No slice 7 branch is open.

## 1. Problem and outcome

**Problem.**

- The group colour choice shows a radio, a dot and a name for each of eight colours ("Clay", "Slate"…). The names are poetic and do not help, and the group page repeats the name.
- Some palette colours are hard to tell apart, measured with CIE76 on the current values:
  - pairs: `slate`–`ink` 12.7 light and 11.6 dark, `clay`–`rust` 15.0 light and 11.3 dark;
  - state colours: `slate` equals `info` (0.0 light, 1.2 dark), `ochre` is close to `warning` (6.7 and 7.1), `rust` is close to `danger` and `live` (8.2 and 4.6).

  A lesson block carries the audience colour and its status together, so these collide on the week view.

**Outcome.**

- Colour is chosen by round swatches with no visible names. A free colour is preselected, as today.
- Screen readers and the tooltip use plain colour words.
- The group page no longer names its colour.
- The eight palette values move so that every pair, and every value against the state colours, can be told apart in both themes. Each value keeps at least 4.5:1 on `bg` and `surface`.
- The stored keys (`clay`, `slate`, `moss`, `plum`, `ochre`, `teal`, `rust`, `ink`) stay. There is no migration and no API change, and every existing group and tutor label recolours at once.

## 2. Verification tier and security trigger

Standard tier, security trigger **not met**, as the design slice plan classifies C:

- the stored keys stay;
- no route, handler, schema or authorization changes;
- the swatches use classes, never `style` attributes, so the `style-src 'self'` content security policy holds.

A renamed key would reopen the classification, and this plan renames none. One standard reviewer reviews the change. Task plans that do not meet the trigger need the owner's approval only.

## 3. The proposed palette

The samples page `palette-samples-2026-11-08.html` in the shared notes folder shows the current and proposed values side by side in both themes. It shows the swatches, the picker, rows with an 8 px dot and lesson blocks with a status word beside them. Decision 9 requires the owner to see these samples before the palette lands.

| Key | Word (ru / en) | Light now → proposed | Dark now → proposed |
|---|---|---|---|
| `clay` | коричневый / brown | `#9A5B3C` → `#6E3D22` | `#D49A7B` → `#C29582` |
| `slate` | синий / blue | `#3F5E7A` → `#13759D` | `#8FB0CE` → `#5E9AC8` |
| `moss` | зелёный / green | `#4F6B4A` → `#2B5037` | `#9DBB95` → `#848F7E` |
| `plum` | фиолетовый / purple | `#7A4A6B` → `#7A5E81` | `#C99BBB` → `#B183AE` |
| `ochre` | горчичный / mustard | `#86621A` → `#7F6B37` | `#D3B064` → `#CFB87C` |
| `teal` | бирюзовый / teal | `#2F6B6B` → `#2F7475` | `#7FBDBA` → `#54AEA4` |
| `rust` | розовый / pink | `#A0442E` → `#9D5264` | `#E3907B` → `#F5ABBD` |
| `ink` | серый / grey | `#4A4A5E` → `#50555E` | `#AEAEC4` → `#BFC0CD` |

Measured on the proposed values:

| Measure | Light | Dark |
|---|---|---|
| Closest pair, CIE76 | at least 21.7 | at least 24.4 |
| Closest value to `info`, `success`, `warning`, `danger`, `live` or `accent`, CIE76 | at least 12.4 | at least 14.9 |
| Contrast on `bg` and `surface` | at least 4.7:1 | at least 4.7:1 |

`rust` moves from red-orange to a muted pink, away from `danger` and `live`. `ink` becomes a near-neutral grey, away from `slate`. `clay` becomes a darker brown, away from the new pink. The keys keep their old names in code because they are stored. The words shown to people follow the new colours.

The thresholds this step fixes are at least 20 between any two palette values, and at least 12 between a palette value and a state colour, in both themes.

As built, after the standard review: `success` joins the state colours measured (it marks a lesson that took place, beside the audience colour on the same block), and dark `moss` moves from `#A9C6A2` to `#848F7E` to keep 12 from it; the owner chose this on 2026-11-08.

## 4. What changes

- **`frontend/src/styles.css`:** the eight `--color-<key>` values in the light and dark blocks.
- **`frontend/src/features/groups/ColourField.tsx` becomes a swatch picker:**
  - native radio inputs, visually hidden but focusable, each labelled by its colour word;
  - a round 32 px swatch for each radio, inside a 44 px target on coarse pointers and 40 px on fine pointers, as `tokens.md` requires;
  - the checked swatch has a 2 px `text`-coloured ring and a check mark that does not depend on hue;
  - the focus ring follows the shared `focus` token;
  - the tooltip (`title`) carries the colour word;
  - arrow-key navigation and the group's legend stay native.
- **`GroupPage.tsx`:** the colour name beside the group's dot is removed (decision 8).
- **i18n:** `colours.*` in `en.ts` and `ru.ts` change to the plain words in section 3.
- **`knowledge/design/web/tokens.md`:**
  - the tutor colours table gets the new values, their measured contrast and the CIE76 measurements against each other and against the state colours, with the date;
  - a "Swatch picker" component entry.
- **`knowledge/design/web/pages.md`:** Groups and the New group dialog describe the swatch picker, and the group page no longer names its colour. Pending owner review.
- **Owner decision of 2026-11-08 recorded with this change:** `knowledge/plans/design-slice.md` (Carried from slice 7), `knowledge/plans/design-step-l.md` section 8, `knowledge/reviews/slice-7-known-limitations.md` and `knowledge/reviews/design-slice-webkit-failures.md` record that the two carried items move to Slice 10 and that the stress readiness check, the WebKit-under-load diagnosis and performance measurements wait until OVC is deployed on a server. Merge gates and change checks stay.
- **This plan** as `knowledge/plans/design-step-c.md`.

## 5. Acceptance criteria

1. `frontend/scripts/palette.test.ts` (as built in `scripts/`, the node test project, because the browser-mode test environment does not load CSS text) › "every palette pair is at least 20 apart and every value at least 12 from a state colour, in both themes":
   - it reads the light and dark `--color-*` values from `frontend/src/styles.css`;
   - it computes CIE76 from sRGB through CIE Lab with D65;
   - it asserts both minimums.
2. Same test › "every palette value keeps 4.5:1 on bg and surface in both themes", computed with the WCAG 2.2 relative-luminance formula from the same file.
3. `ColourField.test.tsx`:
   - › "the group colour swatches show no visible colour names": no colour word is visible text, and each radio's accessible name is its word;
   - › "use eight native radios in one named group" (the arrow keys are proved in a real browser, below): eight `input[type=radio]` in one named group, and arrow keys move the checked radio;
   - the selection mark is proved in a real browser instead (see below); the 2 px ring is decorative and not asserted, while the check mark, which does not depend on hue, is;
   - › "give eight distinct accessible names in ru" and "… in en": the names in `ru` and in `en` are each eight distinct words.
   As built, the arrow-key movement and the visible check mark are proved in a real browser, because jsdom implements neither radio arrow keys nor CSS: `groups.spec.ts` › "the arrow keys move the chosen colour like native radios" and › "only the chosen swatch shows its check mark".
4. `frontend/e2e/groups.spec.ts` › "the colour swatches are at least 44 px on a coarse pointer". It runs in the coarse-pointer project tag the configuration already has: every swatch target's box is at least 44 × 44.
5. The existing groups, students, schedule and notifications specs pass, after their colour locators move from the old names to the new words. No assertion is removed or loosened.
6. `tools/docs/check-docs.sh` passes, and `tokens.md` and `pages.md` carry the new values and wording, marked pending owner review.
7. The frontend change checks pass:
   - `typecheck`, `lint`, `format:check`, `test`, `build`, `check:classes` and `check:bundle`;
   - the whole `pnpm e2e`, because `frontend/e2e/` changes.

## 6. Order of work

1. Worktree outside `/private/tmp` on `feat/design-step-c-palette` from `main`.
2. Palette values and the palette test.
3. The swatch picker and its component tests.
4. The group page, i18n, the e2e locators and the coarse-pointer test.
5. `tokens.md`, `pages.md`, the decision record of the deferred heavy runs, and this plan.
6. Change checks, the standard reviewer, fixes under the after-fix rule, the readiness report, the merge gate, the merge and finish.

## 7. Observability

None: no backend change, no new request.

## 8. Risks

- **Existing groups and tutor labels change colour at once.** This is intended. The samples show the owner the result before it lands.
- **The colour words become the e2e locators.** Specs that select a colour by its old name move to the new word in the same change.
- **A muted pink beside the olive accent may read as too warm.** The owner judges it from the samples, and any value can be adjusted under the same thresholds before the change starts.
