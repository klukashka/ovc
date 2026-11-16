# Student notes autosave flake: cause and fix

Recorded on 2026-11-16 for change X1, `fix/student-notes-autosave-flake`, of the [slice 10 plan](../plans/slice-10.md), decision D13. It resolves the first row of [the slice 9 known limitations](../reviews/slice-9-known-limitations.md): the Firefox end-to-end test `student card › notes corrected after a refusal are saved` (`frontend/e2e/students.spec.ts`, line 557 at `787deb2`) once read `""` from the stored notes after the form appeared to show `Saved.`. Line numbers below refer to `main` at `787deb2`, whose tree is `4456de52266673fd13810f7abbbdc031e35b2353`.

## Verdict

The fix is in the test, not the product. The product's autosave sends exactly one save carrying the corrected text, and shows `Saved.` only after the server has answered it. The test waited for the wrong thing: `form.getByRole('status').filter({ hasText: 'Saved.' }).waitFor()`. A string `hasText` is a case-insensitive substring match, so the filter also matched the status already on screen, `Notes were not saved.`, and the wait returned at once. The next assertion, that the `Try again` button is gone, passes as soon as the debounced save starts and the status turns to `Saving…`. The database was then read while the save was still in flight, and returned the old empty notes.

The recorded hypothesis was that `fill` clears the field first, so an autosave of the empty value reports `Saved.` before the final text is saved. That hypothesis is refuted: in Firefox, `fill` fired one `input` event carrying the final text, and the page sent one save, also with the final text. A transient empty value would in any case have been absorbed by the 1-second save delay.

The cause lies in no authorization or session handling, so the change does not meet the security trigger.

## Evidence

All runs were made from `frontend/` through the owner's settings wrapper, Firefox projects only, with `--trace=off`.

1. **Plain repetition does not reproduce the flake.** `pnpm e2e --project='firefox-*' --repeat-each=30 --trace=off e2e/students.spec.ts -g 'notes corrected after a refusal are saved'` on the unfixed tree gave 30 passed, exit 0. The race window is the few milliseconds between the save request leaving the page and its commit.
2. **A deterministic diagnostic shows the mechanism.** A temporary test, which was never committed, repeated the test's steps. Before the corrected text was typed, it installed a `page.route` handler that only held the save request and then passed it on with `route.continue()`, as ADR-0009 item 9 permits. It also recorded every `input` event of the notes field and every save body. It then ran the original steps and read the status text and the stored notes while the save was still held. On the unfixed tree (exit 0) it printed:
   `{"statusWhenWaitResolved":"Notes were not saved.","statusWhenButtonGone":"Saving…","storedWhileHeld":"","inputs":["Prefers mornings."],"bodies":[{"notes":"Prefers mornings."}]}`
   The original wait returned while the status read `Notes were not saved.`, the `Try again` button was gone while the status read `Saving…`, and the stored notes were still `""`, which is exactly the recorded failure.
3. **The fixed assertion waits for the real save.** The same diagnostic was run with the fixed assertion, `await expect(form.getByRole('status')).toHaveText('Saved.')`, given 3 seconds while the save was held. It printed `{"heldOutcome":"did not pass while held","statusWhileHeld":"Saving…","storedWhileHeld":"", …}`. After release, the assertion passed and the stored notes were `Prefers mornings.` (exit 0).
4. **Acceptance run.** The plan's command, with `--repeat-each=30`, gave 30 passed, exit 0, on the fixed tree.

## Other status waits with a string `hasText`

Every string `hasText` filter in `frontend/e2e/*.spec.ts` on a `status` or `alert` locator, or on an `aria-live` region, was checked against every interface text in `frontend/src/lib/i18n/en.ts`, `ru.ts` and `frontend/src/features/*/texts.*.ts`. The check looked for another text that contains the filter text, ignoring case. The filters that pass a string constant were checked the same way: `cancelledNotice` and `changedMeanwhile` in `lesson.spec.ts` (defined at lines 277 and 278, used at lines 471, 616, 677, 705, 751, 764, 1187 and 1219). No other text contains either constant, so neither can match falsely. Filters with no such text cannot match falsely and are not listed. Those that have one:

| Place at `787deb2` | Filter text | Containing texts | Decision |
|---|---|---|---|
| `students.spec.ts:576` | `Saved.` | `Notes were not saved.` (the same status element) | Converted to `toHaveText('Saved.')` on the notes form's status; this is the flake itself |
| `builder.spec.ts:388` | `Saved` | `Not saved` (the builder's own status element when a save fails) | Converted to `filter({ hasText: /^Saved$/ })` |
| `builder.spec.ts:416` | `Saved` | `Not saved` | Converted to `filter({ hasText: /^Saved$/ })` |
| `builder.spec.ts:1048` | `Сохранено` | `Не сохранено` (the builder's own status element when a save fails) | Converted to `filter({ hasText: /^Сохранено$/ })` |
| `students.spec.ts:607`, `:632` | `Saved.` | `Notes were not saved.` (the notes status on the same card page) | Kept. The tests never edit the notes, so that status stays empty. A false match could only add a second, unfocused element, which makes `toBeFocused` fail, never pass falsely |
| `groups.spec.ts:425` | `Saved.` | `The setting for “{group}” was not saved.`, `Notes were not saved.`, `Not saved. Trying again…` | Kept. None of these appears on the group page, and the locator is already asserted with the exact `toHaveText('Saved.')` |
| `invitations.spec.ts:428`, `:456`, `:475`, `:522`; `live-updates.spec.ts:135` | `This card changed in the meantime.` | `This card changed in the meantime. Its current state is shown below.` | Kept. The only containing text is the intended notice itself, so the filter is a deliberate prefix |
| `invitations.spec.ts:551` | `The link was removed.` | `The link was removed. This new card has a fresh invitation link.` | Kept. The only containing text is the intended notice itself |
| `lesson.spec.ts:392` | `Урок изменён` | `Связанный урок изменён` | Kept. That text is a homework announcement and is not rendered on the lesson page |
| `review.spec.ts:468` | `Returned for revision.` | `This work can no longer be returned for revision.`, `Students see the correct answers as soon as they submit, so work cannot be returned for revision.` | Kept. The first is shown as an `alert`, which a `status` locator does not match. The second is a note in the issue dialog, not on the review page |

A string `toHaveText` matches the whole text exactly and is case-sensitive. Where the locator may resolve to one of several `status` elements on a page, the anchored regular expression keeps the filter and makes it exact.
