# Check budget: consolidating the `lesson` end-to-end spec, closed without a spec change

Recorded on 2026-10-29 for change E5 of the [slice 6 plan](../plans/slice-6.md), step 2 of [proposal 0004](../../docs/proposals/0004-end-to-end-suite-cost.md) for `frontend/e2e/lesson.spec.ts`. It starts from the eight candidates of the [E1 record](check-budget-2026-10-29-e2e-cost-measurement.md#e5-lesson-2-navigation-only-prefix-extensions-25-s-6-prefix-extensions-127-s), checks each against the spec at `d22ca0c`, and records why the owner closed E5 without changing the spec. No test, fixture, check or budget changes.

## Baseline at `d22ca0c`

- `grep -o 'expect(' frontend/e2e/*.ts | wc -l` prints 1291 and `grep -o 'expect.poll(' frontend/e2e/*.ts | wc -l` prints 10.
- `lesson.spec.ts` holds 27 tests per engine (24 for the tutor, four of them from the `focusReturns` loop, and three for the student), 88 `expect(` and 16 `toBeFocused`. Its only poll, in the past-lesson test, is written as `expect` and `.poll(` on separate lines, so the grep does not count it; no candidate touches that test.
- **Sanity run, not an acceptance run:** one run of `tools/dev-env.sh pnpm --dir frontend e2e lesson` from the main checkout, started while the end-to-end run lock and the merge-gate lock were free, passed 81 tests in 21.9 s of wall time (Playwright reported 21.6 s). Other work was running on the machine (load average about 4.5), so the figure shows only that the spec passes at this commit; it is not one of the three runs with and three without a change that the slice plan requires.

## Candidates

Each row removes one test from all three engines. The gain is the removed test's summed time over the three engines from the E1 record. "Absorbing test" names the test that would take over the removed test's assertions. The E1 record proposed the `focusReturns` loop as the absorbing test for the first three rows, which would need an optional per-entry assertion step in that loop, and the shared helper `endedSeriesWithNewLessonOpen` for the fourth; the absorbing tests below need neither, because each already waits for the same element after the same prefix.

| # | Removed test | Absorbing test | Kind | Shared arrangement and actions | Gain |
|---|---|---|---|---|---|
| LES-1 | an edit dialog of a one-off lesson opens on its title | a lesson changed elsewhere closes the open dialog onto the notice (in place of its wait for `Название` after the Edit click) | prefix extension | Maria with Anna, one lesson tomorrow at 18:00, the tutor's lesson page, the Edit click | 1.9 s |
| LES-2 | cancelling opens on the dismiss button | a confirmed cancellation shows the cancelled lesson and its notice (after its Cancel click) | prefix extension | one lesson, the lesson page, the Cancel click | 2.0 s |
| LES-3 | a one-off lesson offers no series ending | a stale edit closes onto the notice (right after its first navigation) | navigation-only prefix extension | one lesson, the tutor's first navigation | 1.3 s |
| LES-4 | ending a series opens its confirmation on the dismiss button | ending a series refused because the lesson changed closes onto the changed notice (after its end-series click, before the edit through the API) | prefix extension | a weekly series of four, its second lesson, the end-series click | 1.9 s |
| LES-5 | editing a series occurrence asks for the scope | this and following lessons offers no date and moves the following lessons | prefix extension | a daily series of three, its second lesson, the Edit click | 1.9 s |
| LES-6 | a new series refused after the ending shows the refusal in the dialog | New lesson dismissed with its Cancel button after a refusal closes onto the cancelled notice (in place of its wait for the refusal) | prefix extension | `refusedNewSeries` and the `Запланировать` click | 2.5 s |
| LES-7 | ending a series in another timezone prefills the same instants in the profile timezone | a new series saved in another timezone starts at the instants of the cancelled lessons (in place of its wait for the search, before `Запланировать`) | prefix extension | `endSeriesInReykjavik` and the `Завершить серию` click | 2.4 s |
| LES-8 | a student sees Individual lesson, never the card name | an open page updates when the tutor moves the lesson (seeding the card name `Аня из вторника`; after its first navigation) | navigation-only prefix extension | Anna, one lesson tomorrow at 18:00, the student's first navigation | 1.2 s |

All eight would save about 15.2 s of summed time (12.7 s from prefix extensions, 2.5 s from navigation-only ones), about 2.8 s of wall time, and take the spec from 27 to 19 tests per engine.

## Assertion mapping

Every assertion would move unchanged; only `waitFor` calls, which the guard counts do not include, would be dropped. No removed test shares an assertion with its absorbing test, so none would be kept twice.

| # | Removed assertions | Place in the absorbing test |
|---|---|---|
| LES-1 | `Название` is focused; no radio; `Время начала` is 18:00 | after the Edit click |
| LES-2 | the dismiss button is focused; no radio; the dialog's description | after the Cancel click, before the confirming click |
| LES-3 | the Edit button is visible; no end-series button | after the first navigation, in this order, because a count of zero alone would pass before the page renders |
| LES-4 | the dismiss button is focused; no radio; the dialog's description | after the end-series click |
| LES-5 | two scope radios; `Этот урок` is focused and checked; `Этот и следующие уроки` is not checked; the date field is visible | after the Edit click |
| LES-6 | the refusal text is visible; the dialog is visible | after the `Запланировать` click |
| LES-7 | the date is the day before the second lesson; the time is 22:00; the weekdays are shifted back one day | after the `Завершить серию` click, the weekday check last because it does not retry |
| LES-8 | Individual lesson; Maria; no card name; the room opens at 17:50; no Edit or Cancel button | after the first navigation |

The 27 moved `expect(`, four of them `toBeFocused`, would keep the counts at 1291 and 10 and `lesson.spec.ts` at 88 `expect(` and 16 `toBeFocused`. The refusals the spec proves (Anna's view without the card name or edit and cancel controls, T-5 and T-44; the new series refused because the audience is gone; the ending refused because the lesson changed; the stale edit) would stay in every engine.

## Conclusion

- **Every candidate puts assertions into the absorbing test's arrange phase.** Proposal 0004's technique 1 merges tests with the same arrangement and the same action; no pair in `lesson.spec.ts` meets it. Each candidate instead moves the removed test's assertions ahead of the absorbing test's own action, so the merged test would assert, act and assert again. `AGENTS.md` allows acting and asserting in turns only in a step test of a pilot scenario suite; every other test keeps three phases.
- **The gain is below the run-to-run spread.** About 15.2 s of summed time and 2.8 s of wall time compare with a spread of 6 to 19 s between the E1 record's runs, so three runs with and three without the change could not show it in wall time.
- **Decision:** on 2026-10-29 the owner closed E5 as this record, without changing the spec.
