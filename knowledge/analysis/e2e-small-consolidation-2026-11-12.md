# Small end-to-end consolidation C1: coverage record and measurements

Recorded on 2026-11-12 for change C1 of the [slice 9 plan](../plans/slice-9.md), D4's consolidation after the gate of the change merged as `a5dfb41` took about 557 s. C1 removes end-to-end tests whose assertions Go tests hold, moves browserless checks to the Chromium-only contract suite, and shares one arrangement that does not depend on the date. It is meant to drop no coverage. This record is the D3 coverage record and the change's measurement.

## Coverage record

Every removed, moved or changed end-to-end test, and what now holds its assertions. Line numbers refer to `main` at `3e978bc`, the change's base.

### Removed, held by existing or extended Go tests

| End-to-end test (engines) | What it asserted | Now held by |
|---|---|---|
| `today.spec.ts:346`, the loop "one tutor's Today never shows another tutor's lessons, for a student they share, seen by Maria / Pavel" (2 tests × 3) | Each tutor's Today lists only that tutor's lesson for a shared student. Today reads the calendar route. | `TestLessonRoutesDenyASecondTutorSharingTheStudent` (`backend/internal/api/lesson_visibility_test.go`). It already asserted that the second tutor's calendar lists only their own lesson. **Extended:** the first tutor's calendar lists the first tutor's lesson and not the second's. Today's display of a lesson stays in `today.spec.ts:112`. |
| `groups.spec.ts:680`, other-tutor half (× 3) | Another tutor's group shows Not found and nothing of the group. | `TestTutorCannotReachAnotherTutorsGroup` (`groups_test.go`): another tutor's group answers `404` with the same refusal as a random identifier for read, rename, archive and unarchive. `refusal()` compares the status, the body without `request_id`, and every header except `X-Request-Id`, `Date` and `Content-Length`. The unknown-group half stays, renamed "groups › an unknown group is not found", as the frontend's rendering of that refusal. |
| `groups.spec.ts:654`, other-tutor half (× 3) | Another tutor's group in the Students filter is not available. | The same Go test, `?group=` case. **Extended** with the frontend's own query, `state=active&group=`, for another tutor's group against a random identifier. The unknown-group half stays, renamed "students › an unknown group in the filter is not available". |
| `invitations.spec.ts:566` (× 3) | A removed provisional student's card address shows Not found. | `TestRemovedProvisionalStudentLosesTheTutorAndTheTutorGetsANewCard` (`invitation_signup_test.go`): the old card answers `404`. The card page's Not found rendering stays in `students.spec.ts:379`. |
| `join-requests.spec.ts:318` (× 3) | A blocked student's new request does not appear on Students. | `TestDeclineAndBlockKeepsLaterRequestsPendingForTheStudent` (`join_requests_test.go`): the tutor's request list is empty. |
| `meeting-room.spec.ts:361` (× 3) | The room is served with `no-store`, `no-referrer`, `nosniff`, a CSP with `frame-ancestors` of the app origin and `default-src 'none'`, and no `X-Frame-Options` or CORS header. | The moved room test in `api-contracts.spec.ts` (below), which keeps every one of these assertions on the room origin of the built deployment. The subtest "the page and its assets carry the room headers" of `backend/internal/testcontrol/fake_room_test.go` holds the exact CSP. The iframe tests of `lesson-room.spec.ts` prove the framing in a browser. |

### Removed after a new Go test

| End-to-end test (engines) | What it asserted | New Go test |
|---|---|---|
| `meeting-room.spec.ts:395` (× 3) | The embed override reaches the lesson detail and the join answer until the next override: off, on, off. | `TestEmbedOverrideReachesTheLessonDetailAndTheJoinAnswerUntilTheNextOverride` (`meeting_embed_test.go`): one switch shared by the schedule and meeting services, as `cmd/api/main.go` wires it, gives `false`, `true`, `false` on both the lesson detail and the join answer. The test-control route's wiring to that switch in the built server stays proven by `lesson-room.spec.ts:159` and its iframe tests. |
| `library.spec.ts:683` (× 3) | Deleting every assignment makes the getting-started step undone again (`data-done="false"`, from the library's `template_count`). | `TestTheTemplateCountReturnsToZeroAfterEveryTemplateIsDeleted` (`templates_test.go`): the count is 1 and becomes 0 after the delete. The undone display stays in `library.spec.ts:661` and `today.spec.ts:141`. |
| `homework.spec.ts:714` (× 3) | The card's Homework tab of a shared student lists only this tutor's work, showing its title and "Not submitted". | `TestTheIssueListOfASharedStudentHoldsOnlyTheCallersWork` (`issues_test.go`): each tutor's `GET /issues?student=` lists only their own issue, with its submission. The display moved to the card-tab Show more test, which now also asserts that its rows show "Fractions" and "Not submitted". |

### Moved to the Chromium-only `api-contracts.spec.ts`

These tests open no page and assert only HTTP answers. They run in Chromium once instead of in three engines, against the same worker deployment and preview origin, with the same fixtures.

- `meeting-room.spec.ts:375`, "the room origin forwards only the room", merged with the header assertions of `:361` into "the room origin forwards only the room, served with the no-store, no-referrer and framing headers".
- `meeting-room.spec.ts:385`, "no recording control exists". It is kept as an end-to-end check because the Go subtest builds the test-control routes with only the room and embed dependencies, so only the built deployment proves that the routes are absent.
- `smoke.spec.ts:67`, "the preview server has no build manifest to serve", with the same three address variants.
- `smoke.spec.ts:54` and `:245` merged into "every page, asset and proxied API response carries the security headers". It keeps:
  - `/today` with the exact CSP;
  - `/api/v1/health/live` answering 200;
  - `no-referrer` and `nosniff` on `/today`, `/api/v1/health/live` and the six paths of `:245`;
  - `/api/v1/no-such-route` answering 404.
  
  The module script address is read from the entry HTML instead of a page.

### Shared arrangements

One arrangement is seeded once per worker instead of once per test: `messages.spec.ts`, `longHistory`. It does not depend on the date, so the shared fake clock, which other specs move, cannot invalidate it.

- It is shared by three tests: the composer test at 1440 × 900, "an opened conversation shows its latest message", and "a history shown at its bottom stays at its bottom when the window gets shorter".
- Each opening of the conversation writes the tutor's read cursor through `POST /channels/{id}/read`. This is harmless: none of the three asserts unread state, and the conversation does not anchor on unread messages.
- The fourth `longHistory` test asserts unread state, so it keeps its own seed.

The plan also shared the 21 issues of the two Show more tests in `homework.spec.ts` (the Issued tab and the card's Homework tab).

- In the first three branch runs, the Issued tab test failed twice with an unexpected request failure in the browser diagnostics, once in WebKit and once in Chromium; it never failed per test on `main`.
- So the sharing was reverted: both tests seed per test as on `main`, and only the card-tab test's two new display assertions remain.
- Both failures were `request_failure` records raised through the diagnostics fixture's unseen-abort check; the test output folders with the attached details were deleted between runs, so which request failed is not known.

The date-dependent arrangements (`home.spec.ts` `studentWithTwoTutors`, and `homework.spec.ts` `:450`/`:618` with a lesson tomorrow) keep their per-test seeds.

### Proof that the scope tests catch a leak

For each extended or new Go test, the product behaviour it guards was broken temporarily, the test run, and the file restored. `git diff -- backend/internal/dbgen backend/internal/services` was empty afterwards, and all five tests passed again on the restored product.

| Broken condition (in `backend/internal/dbgen`) | Test | Result |
|---|---|---|
| `ListTutorCalendar`: `lessons.tutor_id = $1` replaced by `$1::uuid IS NOT NULL` | `TestLessonRoutesDenyASecondTutorSharingTheStudent` | failed: "the first tutor's calendar lists" both lessons |
| `GetTutorGroup`: `tutor_id = $2` replaced by `$2::uuid IS NOT NULL` | `TestTutorCannotReachAnotherTutorsGroup` | failed: another tutor's group answered 200 with the group, and both the `?group=` and the `state=active&group=` filters answered 200 (the `?group=` check reports with `Errorf`, so the run reaches the new case) |
| `ListTutorIssues`: tutor scope removed **and** the student filter matched by student instead of by card | `TestTheIssueListOfASharedStudentHoldsOnlyTheCallersWork` | failed: the first tutor's list held both tutors' issues |
| `ListTutorIssues`: tutor scope removed only | the same test, after adding its unfiltered-list assertion | failed: the second tutor's unfiltered list held both tutors' issues |
| `views.go` `tutorViews`: `EmbedAllowed` set to `false` instead of the switch | `TestEmbedOverrideReachesTheLessonDetailAndTheJoinAnswerUntilTheNextOverride` | failed: the lesson detail stayed `false` while the switch was on |
| `CountLiveTemplatesOfTutor`: `deleted_at IS NULL` removed | `TestTheTemplateCountReturnsToZeroAfterEveryTemplateIsDeleted` | failed: the count was 1 and stayed 1 |

Removing only the tutor condition of `ListTutorIssues` opens no leak through the student filter, which already matches the caller's own card, but it leaks every tutor's issues into the unfiltered list (`GET /issues`), which no test covered before this change. The test now also asserts the second tutor's unfiltered list, which closes that gap.

### Inventory

`pnpm exec playwright test --list` lists 1894 runs in 35 files on `main` and 1859 on the branch. Normalising project names, the difference is exactly:

- the titles above, 3 runs each;
- the four moved titles, which appear once under `chromium` in `api-contracts.spec.ts`;
- the renamed groups halves;
- the three `messages.spec.ts` tests of the shared arrangement, which moved into their own `describe` block with a new parent title.

No `pilot-` title differs.

## Measurements

Method: `tools/dev-env.sh pnpm e2e --trace=off`, on `main` at `3e978bc` (a detached worktree) and on the branch, alternating, on mains power under `caffeinate -dis`, with no merge gate, other end-to-end run or full backend run beside them. Each run started only once the one-minute load average was below 6. `pmset -g log` showed no sleep or dark wake.

| Pair | `main` (1894 runs) | Branch (1859 runs) |
|---|---|---|
| 1 | 271 s, all passed | 273 s, all passed |
| 2 | 273 s, all passed | 260 s, all passed |
| 3 | 262 s, all passed | 264 s, all passed |
| Mean | 268.7 s | 265.7 s |

- The branch is 3 s faster on average, within the run-to-run spread; the criterion (the branch mean at most 5 s above `main`'s) holds. The expected saving of about 7 to 10 s did not show above the noise of three pairs.
- `go test -race -count=1 ./internal/api/... ./internal/testcontrol/...` passed on the final tree (113.3 s and 7.3 s).
- After the paired runs, the shared `messages.spec.ts` arrangement stopped carrying an unused student token. Its three tests then passed in all three engines (9 runs). The typecheck and lint passed on the final tree. The rest of the end-to-end suite is deferred to the merge gate for that one edit.
- Earlier runs on the same day, started without waiting for a quiet machine (one-minute load 9 to 27), are kept for the failure rates below but not for timing.
  - `main`: 274, 263 and 264 s.
  - The first branch version, with the shared homework arrangement: 264, 262 and 292 s.
  - The branch after reverting that sharing, unpaired: 345, 312 and 320 s.
  - A process outside this repository kept one core at 100 % during part of that period.

WebKit's `lesson-room.spec.ts:473` ("signed out, the lesson address asks to sign in and shows nothing private"), a flake recorded in the slice 7 known limitations, fails with `page_load_failed` as an unexpected console error.

| Runs | Busy machine | Quiet machine | All |
|---|---|---|---|
| `main` | 1 of 3 | 0 of 3 | 1 of 6 |
| Branch | 4 of 6 | 0 of 3 | 4 of 9 |

C1 changes neither that spec nor application code. The rate follows the machine's load rather than the change. The branch's busy runs had the highest loads of the day.

One other failure occurred once, on the branch at load 9: `lesson-form-layout.spec.ts:148` in WebKit, a layout bound off by 4 px. It is another file C1 does not touch.

## Threat model mapping

- T-5 (cross-tutor scope): examined. The API equivalence is proven above, including against a broken scope filter, and one frontend rendering of each refusal kind remains.
- T-16 and T-27 (framing and referrer headers): examined. The room's headers are kept on the built deployment by the moved room test and in Go by `fake_room_test.go`. The application's exact CSP, with `frame-ancestors 'none'`, which T-16 cites as asserted by the Playwright smoke, is now asserted by the moved header test in `api-contracts.spec.ts`, in Chromium only; the header is the server's and does not depend on the engine. T-27's entry also cites the smoke for the preview server's missing manifest, which now runs from `api-contracts.spec.ts` as well. The threat model's wording can name the new file in both entries in a later reviewed change to `docs/`.
- T-36 (no recording control): examined. The test is moved, not removed.
- T-37 and the served-content rule (CSP, the build manifest): examined. Both are moved with every assertion.
- T-41: examined. No test is skipped, no assertion is loosened, and no timeout, retry or worker setting changes.
