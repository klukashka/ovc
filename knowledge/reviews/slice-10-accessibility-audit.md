# Slice 10 accessibility audit (change A1)

Status: agent record of slice 10 change A1, the accessibility pass (decision D44 of the [slice 10 plan](../plans/slice-10.md)), written against `main` at `ea97b12` on 2026-11-22, with the owner's answers of the same day. The VoiceOver journeys and the checks on Safari and iOS devices are a person's checks and stay `unverified` until the owner runs them.

## Method

The audit adds no dependency (D44, Q12 A). It ran in three steps on 2026-11-22:

1. **Measured facts.** A temporary probe, never committed, hooked into the existing 360 px sweeps of `frontend/e2e/desktop-layout.spec.ts`, so it saw every page of [pages](../design/web/pages.md) with the sweeps' seeds (60-character unbroken names and names with markup, two tutors, issued, submitted, checked and reviewed homework, long chats). For each page it measured, in Chromium and WebKit, the light and dark themes at 1440 × 900 and 360 × 780, and the light theme at 720 × 450 (the 1440 layout at 200 % zoom), 320 × 640 (WCAG 1.4.10 reflow) and 780 × 360 (a phone turned to landscape):
   - sideways scrolling (`scrollWidth > clientWidth`), with the elements and text nodes that cause it;
   - every interactive element under 24 × 24 px (2.5.8), and below 768 px every one in `main` under 44 px high (the phone rule of [pages](../design/web/pages.md#shared-behavior));
   - interactive elements without an accessible name (4.1.2);
   - the number of `main` landmarks, the visible `h1` and skipped heading levels (1.3.1, 2.4.6);
   - each interactive element focused in turn, checking the focus indicator (2.4.7) and whether it ends behind the phone frame's fixed bottom bar (2.4.11);
   - a real keyboard walk of 40 Tab presses at 360 px (`Alt+Tab` in WebKit, which otherwise skips links), recording every focused element that ends below the bottom bar's top edge;
   - clipping and sideways scrolling under the WCAG 1.4.12 text-spacing style (line height 1.5, letter spacing 0.12 em, word spacing 0.16 em, paragraph spacing 2 em), applied as a constructable style sheet at 1440, 360 and 320 px;
   - a screenshot of each page at 1440 and 360 px in both themes in Chromium, 156 in all, reviewed by eye.
2. **Contrast.** Text and non-text contrast come from the palette: `frontend/scripts/palette.test.ts` proves every text pair at 4.5:1, and [tokens](../design/web/tokens.md#colours) records the 2 px `focus` ring at 5.3 to 7.5:1 on `bg` and `surface` in both themes, with the controls' borders and icons at 3:1 (1.4.3, 1.4.11). The probe confirmed that every focused control shows that ring.
3. **Review against the specifications.** Each page's focus moves, dialogs, orientation and keyboard order against [navigation](../design/web/navigation.md), [pages](../design/web/pages.md) and [states](../design/web/states.md), and the rows of [the slice 10 known limitations](slice-10-known-limitations.md) carried into A1.

### Probe runs

| Run | Start (load, power) | Command | Result |
|---|---|---|---|
| 1 | 2.53 / 2.65, AC | `pnpm e2e e2e/desktop-layout.spec.ts <experiment spec> --project=chromium --project='webkit-*' --trace=off --retries=0` with the probe | 42 passed, 20 failed |
| 2 | 4.01 / 3.74, AC | the same without the experiment spec, the probe corrected | 58 passed |
| 3 | 3.02 / 3.33, AC | the same with the smoke spec, `-g "360 px sweeps\|Tab focuses behind\|by the browser itself"` | 24 passed |

Run 1's 20 failures split as follows:

- **18 sweeps, nine per engine.** Every assertion of their bodies passed. They failed only in the browser diagnostics fixture with `console_error` records, because the first text-spacing probe added an inline `<style>` element, which the content security policy refuses and reports on the console. Run 2 used the same seeds and tests with the probe applying a constructable style sheet, which the policy allows. All 58 tests passed, the nine sweeps per engine among them, so these 18 failures were caused by the probe. The change's own check run, without any probe, runs the same sweeps again (see the readiness report).
- **2 experiment tests in Chromium.** They failed with `ENOENT` writing their log file, which had not been created yet; the experiment ran only in WebKit. They are not tests of the product.

The content security policy was not changed. The experiment spec and the probe were deleted before the commit.

## Results by page

Each cell gives the result for both themes; the dark theme was measured at 1440 and 360 px. The probe ran in Chromium and WebKit only; Firefox was not audited, and its results come from the sweeps of the change checks (no sideways scrolling at 360, 320 and 780 px). Dialogs other than those named were not opened by the probe; their full-screen and focus rules rest on the existing tests of each dialog. "Pass" means no failure of the checklist above: keyboard reach and order, visible focus and its contrast, names and roles, headings and landmarks, the specified focus moves, target size, text spacing, 200 % zoom, orientation and reflow at 320 px. A failure names its criterion and its fix.

| Page | Computer, 1440 px | Phone, 360 px | Reflow, 320 px | Notes |
|---|---|---|---|---|
| Sign in | Pass | Pass | Pass | |
| Sign up (choice, tutor, student) | Pass | Pass | Pass | The privacy policy link inside the consent sentence is 17 px high, an inline link that 2.5.8 exempts; the checkbox has its 44 px target |
| Accept invitation | Pass (measured state only) | Pass (measured state only) | Pass (measured state only) | Measured only in the state without a token, "Open the link from your tutor again"; the states with a tutor (Create account, Sign in to accept, Accept and Use another account stacked) are `unverified` by the probe. The **Phone** rule now names "Use another account" a full-width text button (owner's decision) |
| Confirmation, password reset, unsubscribe | Pass (measured state only) | Pass (measured state only) | Pass (measured state only) | Measured only in their invalid-link states, which the sweeps reach without a token; the forms behind a valid token are `unverified` by the probe |
| Today | Pass | Pass | Pass | Join stands at the full width under its lesson's link at 360 px, now asserted |
| Schedule (tutor) | Pass | Pass | Pass | New lesson's dialog fills the screen at 360 px, now asserted |
| Lesson | Pass | Pass | Pass | The back link follows the title and the header's actions on every width, a meaningful order (2.4.3); the **Phone** rule and the mockup now show it (owner's decision of 2026-11-22) |
| Students | Pass | Pass | Pass | **Failed at 780 px**: the "Open assignments" column heading overflowed its `w-1/12` column by 14 px, so the page scrolled sideways (NFR-5.2). Fixed: column headings wrap (`frontend/src/components/ui/DataTable.tsx`) |
| Student card | Pass | Pass | Pass | 2.4.11 in WebKit for Notes, see the shared failure below |
| Groups, group page | Pass | Pass | Pass | Long and markup names are cut with an ellipsis in table rows at every spacing; the full name is the next page's heading |
| Library | Pass | Pass | Pass | |
| Builder | Pass | Pass | Pass | **Failed 1.3.1**: the block headings were `h3` under the page's `h1`, skipping a level. Fixed: they are `h2` (`frontend/src/features/builder/BlockCard.tsx`). The "Correct" radios are 20 × 20 px, spaced beyond the 24 px circle of 2.5.8. 2.4.11 in WebKit, see below. The back link follows the title; the spec now says so (owner's decision of 2026-11-22) |
| Issue dialog | `unverified` | Pass (full screen only) | `unverified` | Not a page and not opened by the probe; the existing 360 px test asserts it fills the screen and its focus |
| Issued work, issue page | Pass | Pass | Pass | The back link follows the page header, a meaningful order (2.4.3); the spec now says so (owner's decision of 2026-11-22) |
| Review queue, review | Pass | Pass | Pass | 2.4.11 in WebKit for the comment and points fields, see below. The back link follows the title; the spec now says so (owner's decision of 2026-11-22) |
| Messages (both roles) | Pass | Pass | Pass | **Failed at 780 px**: a channel's 60-character unbroken name in the conversation heading did not wrap above 767 px, so the page scrolled 388 px sideways (NFR-5.2). Fixed: the heading wraps on every width (`frontend/src/features/messages/Conversation.tsx`) |
| Settings (both roles) | Pass | Pass | Pass | 2.4.11 in WebKit for Profile's fields, see below |
| Notifications | Pass | Pass | Pass | The student's sweep may measure the empty state, a known limitation of G1 |
| Home | Pass | **Failed** | Pass | **Failed the phone rule of 44 px targets**: each tutor's "Messages" link in My tutors was 24 px high below 768 px (2.5.8 itself passes at 24 px). Fixed: a 44 px target below 768 px (`frontend/src/features/tutors/TutorList.tsx`), asserted by the student's 360 px sweep |
| Schedule (student) | Pass | **Failed** | Pass | **Failed 2.4.3 and 1.3.2** below 768 px: the tutor filter was shown above the toolbar but came after it in the document. Fixed by the owner's decision: the filter follows the toolbar on phones too (`frontend/src/features/schedule/SchedulePage.tsx`), asserted by the student's 360 px sweep |
| Homework (student), assignment | Pass | Pass | Pass | 2.4.11 in WebKit for answer fields, see below. The back link follows the page header; the spec now says so (owner's decision of 2026-11-22) |
| My tutors | Pass | Pass | Pass | |
| Keep or drop earlier links | Pass | Pass | Pass | Measured with nothing waiting |

### Failures shared by several pages

- **2.4.11 Focus Not Obscured (WebKit, below 768 px).** A text field or text area that Tab focused below the visible part of the page ended entirely behind the fixed bottom bar in WebKit. For example, Profile's "About me" ended 246 px below the bar's top edge, and the builder's and review's fields up to 136 px below it. Links and buttons were scrolled clear. Chromium honours the document's `scroll-padding-bottom` for every focused element; WebKit ignores it when it reveals a focused text field. Fixed: the bottom bar scrolls a focused element outside a dialog above itself after the browser's own scroll (`useFocusAboveBar` in `frontend/src/components/frame/PhoneFrame.tsx`). The test "a field that Tab focuses behind the bottom bar is scrolled above it" in `frontend/e2e/smoke.spec.ts` asserts the focused field and its box above the bar in every engine. Without the fix, the probe measured the same field 246 px behind the bar in WebKit, and in a run on 2026-11-22 with the fix temporarily removed the test failed in WebKit ("a field that Tab focuses behind the bottom bar is scrolled above it": the field never ended above the bar within the poll's deadline). In the same run, with the Messages heading fix removed, both Messages sweeps failed with the conversation "at 780 px" scrolling sideways. Safari on iOS is `unverified`.
- **`LoadFailed`'s focus (2.4.3).** When a live refetch removed a load failure while focus was on its Retry, focus fell to the body. Carried from the 10.2 slice review. Fixed: focus moves to the page heading, or a panel's heading for a failure inside a panel (`frontend/src/components/ui/LoadState.tsx`). This is proven by `frontend/src/components/ui/LoadState.test.tsx` with `document.activeElement`, and the `pages.md` shared behavior says so.

### Passing observations

- **Landmarks and headings:** every page has one `main` and one visible `h1`; the bottom bar is the `nav` named "Main navigation"; after the builder fix no page skips a heading level.
- **Names:** no interactive element without an accessible name. The visually hidden file inputs of Profile and the assignment are named by their labels.
- **Reflow at 320 px:** no page scrolls sideways at 320 × 640 in either engine. The bottom bar's labels wrap as [navigation](../design/web/navigation.md#frames) specifies. The sweeps now assert 320 and 780 px for every swept page. After each width change they wait until the frame for that width is shown and no API read of the page is still in flight, so the measurement never sees the previous width's layout.
- **Text spacing (1.4.12):** no page scrolls sideways under the spacing style at 1440, 360 or 320 px. The clipped elements the probe listed are visually hidden texts ("Actions", "Unread", "(opens in a new tab)", the step states, the title hidden on computers) and names cut with an ellipsis in table rows and the agenda. The names are cut at normal spacing too, and each is shown in full on the page it links to.
- **200 % zoom (1.4.4) and orientation (1.3.4):** the 1440 layout at 720 × 450 shows the phone frame and passes as the phone layout does. Nothing locks the orientation. At 780 × 360 only the Messages and Students overflows above failed, both now fixed.
- **Dialogs and overlays:** focus moves into each dialog, stays inside, and returns to its opener as [pages](../design/web/pages.md#shared-behavior) requires. Every modal dialog fills the screen below 768 px. New tests assert it for Stop studying and New lesson; Issue assignment and Preview were already asserted.
- **Back link order:** on every page with a back link (the Lesson page, the group page, the Student card, the builder, the issue page, the review screen and the student's assignment page), the back link follows the page header on every width, so the reading and focus order match what is shown (2.4.3). The owner kept this order on 2026-11-22, and `pages.md` and the phone mockups now say so.

## Focus return after a native close (slice 1 carry-over)

When the sidebar overlay (768 to 1023 px) or the More sheet (below 768 px) is closed by the browser itself, with no cancel event the page could handle, the dialog's `close` handler returns focus to "Show sidebar" or "More". Older Safari closes a modal dialog on `Esc` this way.

D44 asks for a WebKit test that closes the overlay by `Esc` handled by the browser. The audit's experiment showed this cannot be produced in Playwright's WebKit:

- `Esc` there always dispatches `cancel`, also a second `Esc` with no user activation between them;
- the page cancels it and closes the dialog through its own handler, which the existing `Esc` tests already cover.

So, by the owner's decision of 2026-11-22 on this departure from D44's wording, the new tests close each dialog with the dialog's own `close()`, the same close algorithm the browser runs when it closes the dialog natively without `cancel`. They assert the focused element in every engine, WebKit included:

- "a close by the browser itself, without a cancel event, returns focus to the show button" (`frontend/e2e/smoke.spec.ts`, at 800 px);
- "a close by the browser itself, without a cancel event, returns focus to More" (at 360 px).

In Chromium and Firefox a click may already have focused the opener, so the browser's own focus restoration could satisfy the assertion there too. In WebKit a click does not focus a button, so the WebKit run proves the page's `close` handler, which is the case the carry-over is about.

| Check by a person | Result |
|---|---|
| Native close of the sidebar overlay in Safari on macOS, then "Show sidebar" focused | `unverified` |
| The same for the More sheet in Safari on iOS | `unverified` (no device check yet) |
| Bottom bar and More sheet above the home indicator on iOS without `viewport-fit=cover` | `unverified`. By the owner's decision of 2026-11-22 nothing changes: without `viewport-fit=cover` Safari keeps the page inside the safe area |

## VoiceOver journeys

| Journey (both on Safari with VoiceOver, macOS) | Result |
|---|---|
| Tutor: sign in, Today, schedule a lesson, open the Student card, issue an assignment, check a submission, send a message, sign out | `unverified` |
| Student: sign in, Home, open a lesson, answer and submit an assignment, read the results, send a message, sign out | `unverified` |

## Not fixed in A1

- **The More sheet's in-sheet sign-out failure** still has no test: jsdom has no `showModal`, and the real logout cannot be made to fail end to end. The branch is unchanged and passes by inspection: its alert sits inside the sheet, and its Sign out button keeps focus through `aria-disabled`. The row stays open in [the known limitations](slice-10-known-limitations.md) for the owner's decision at the 10.3 slice review.
- **The Messages conversation screen while the chat texts load or fail** shows no Back to the list on phones. This is a known limitation of G5 and is not an audit failure: Retry is focused and reachable, and the browser's back works.
