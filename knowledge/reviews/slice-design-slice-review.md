# Design slice slice review

Status: review completed 2026-11-09; resolved 2026-11-10 when its follow-up changes merged, as listed below. Agent record.

## Scope

- **Base:** `e201e16f7dcc3382af0ebb19487f07f1a57f3048`, the accepted slice 6 baseline ([slice 6 accepted state](slice-6-accepted-state.md)), as the [design slice plan](../plans/design-slice.md#5-closing-the-slice) requires.
- **Head:** `30e3def796c495cef8e78c7b9c4e77d674f38ea6` ("Put the lesson page on the layout standard with the join block first"), 56 commits and 339 files. The range holds slice 7's local video-lesson work, the fixes between the two slices, and design slice steps D0 to D7.
- **Tier:** slice, under [ADR-0011](../../docs/decisions/0011-tiered-verification.md). Many changes met the security trigger.
- **Reviewer rules:**
  - Every reviewer worked read-only on `main` at the head commit.
  - Reviewers ran targeted unit tests, Go tests and the merge gate's self-test in scratch copies. They ran no end-to-end suite, full check run or merge gate, because those ran at each change's merge gate in Chromium, Firefox and WebKit.
- **Deferred to slice 10:** the owner decided on 2026-11-08 to defer performance measurements and the diagnosis of WebKit failures under load to slice 10. The performance profile was applied as a code-level risk review.
- **Unverified:** live BigBlueButton behavior, because there is no remote server.

## Reviewers and coverage

| Reviewer | Profiles | Components covered | Why |
| --- | --- | --- | --- |
| Backend | Correctness, performance (code-level) | Meeting provider adapters and process wiring; creation coordinator, lifecycle and routes; recovery, cleanup and worker; the non-meeting backend commits (download slot tests, harness pools, upload settlement lock test, presence deadlines, timezone table). Split into three fresh sub-reviews. | Every backend package the range changes, and the consumers it reaches: emails, status job, deletion and purge, worker lanes |
| Frontend | Correctness, browser (dark theme, Firefox, Safari, phones, by reading code and end-to-end coverage), performance (code-level) | Settings, swatches, cards and lists; lessons, Today, Home and realtime; Messages, layout primitives and test policy. Split into three fresh sub-reviews. | Every frontend component the range changes, and the shared primitives reached through their callers |
| Security | Full security | Five families: meeting provider backend; lesson room and Join in the browser; accounts and scope in the design slice; the end-to-end test control surface and the fake room; tooling and dependencies. Four fresh sub-reviews, plus the reviewer for tooling. | The slice touched several families of trust boundaries |
| Tooling and agent configuration | Correctness, security for verification policy and dependencies | Merge gate and self-test changes, OSV acceptance, tool dependencies, end-to-end diagnostics infrastructure, frontend check scripts, test harness pools. Agent configuration (`.claude/`, `AGENTS.md`, `CLAUDE.md`) did not change. | Development tooling changed; agent configuration did not |

## Integration boundaries and who reviewed them

| Boundary | Reviewer |
| --- | --- |
| API, meetings service, repositories, PostgreSQL | Backend |
| Meetings service and the provider adapters | Backend |
| Worker and outbox, recovery executor | Backend |
| `ovc-admin` and the operator service | Backend |
| Lesson join, end and restart responses, as the typed client and lesson room consume them | Frontend |
| Calendar `tutor` filter, and the realtime frames the frontend consumes | Frontend |
| Join links, CSP and frame policy, ended hint, test-control gating, BigBlueButton checksum and secrets, threat model mapping | Security |
| Merge gate and diagnostics evidence | Tooling, with security for redaction |

## Results

| Layer | Blocker | Major | Minor |
| --- | --- | --- | --- |
| Backend | 0 | 0 | 11 |
| Frontend | 0 | 1 | 9 |
| Security | 0 | 1 | 10 |
| Tooling | 0 | 0 | 5 |

**Majors:**

- **Frontend: Today and Home replace loaded lessons when a background refetch fails.** React Query v5 keeps `data` and sets `isError` on a failed refetch. `TodayLessons` and Home's `Loaded` checked the error first, so one transient failure removed the lessons and their Join controls. That closed a held join tab and lost focus. The first-run checklist reappeared as an error.
- **Security: notes autosave could write one card's notes into another card of the same tutor.** The autosave was not keyed by card. Navigating from card A to a cached card B kept A's pending or refused text, and "Try again" sent it to B.

**Owner decisions on 2026-11-09:**

- Fix the majors and the cheap minors in grouped follow-up changes, and record the rest as known limitations.
- The join window closes 30 minutes after the scheduled end, as the backend code and tests do; `schedule.md` was amended. The frontend still hides Join once the scheduled end has passed, which is recorded as a known limitation.
- Add `typecheck`, `lint`, `format:check` and `test` to the `frontend/e2e/` row of the change checks.
- Remove the minimatch 10.x consumers before the advisory acceptance ends on 2026-11-21.

## Follow-up changes

Each is a standard-tier change under the after-fix rule. Each names the finding it resolves, and its reviewer confirmed the finding closed. Two flaky tests found by the follow-ups' merge gates were fixed as changes of their own.

| Change | Findings | Merged |
| --- | --- | --- |
| Keep loaded data on a failed refetch (Today lessons, Home lessons, the first-run checklist, Notifications) | Frontend major; frontend minor on Notifications | `9c39d73` |
| Notes autosave per card | Security major; frontend minor on Arrange–Act–Assert in the card tests | `43cf100` |
| Frontend minors | `Page` never swaps its heading; the check icon drawn on the 16-unit grid; Students emails wrap; Messages stays pinned to the bottom when the typing indicator or images grow the history; a late join answer from an earlier session epoch is dropped | `c31de3c` |
| Meeting backend minors | Invalid recovery outcome as a typed error; per-call provider budgets for the ended hint and the best-effort End; accurate log events and a test for `meeting.recovery.retry.queued`; dead guard removed; a `reclaim_pending` join test; a settings test refusing test control with SMTP mail | `fee2927` |
| Diagnostics redaction and gate self-tests | Wider secret redaction (named tokens, JSON keys, cookies); fixture failure paths redacted; empty page errors keep their name; a gate self-test assertion that could not fail; a self-test for the fail-closed end-to-end marker | `f1c8894` |
| The embedded room test waits for the room to load and leaves it (a flaky Firefox test of step D7) | Merge-gate finding | `c6671e9` |
| The presence capacity test proves its refusal without the real clock (a flaky backend test of slice 6) | Merge-gate finding | `33595cb` |
| This documentation change, which also closes the slice | Threat model: D6 status line, D5 mapping to T-7 and T-37, T-18 command-log rule, T-39 tool update and minimatch wording, the shared fake's participant-name sentence; `schedule.md` join window; `development.md` change checks; `detailed.md` `OVC_FAKE_ROOM_ORIGIN`; slice 7 "Join on Home and Today" resolved; this record | this commit |

## Known limitations re-verified

- **Slice 7 entries:** every backend, frontend, security and tooling entry of [`slice-7-known-limitations.md`](slice-7-known-limitations.md) was re-verified as still accurate, except "Join on Home and Today", which step D6 resolved.
- **Design slice entries:** every entry of [`slice-design-known-limitations.md`](slice-design-known-limitations.md) was re-verified as still accurate.
- **WebKit failures:** the entries of [the WebKit failure record](design-slice-webkit-failures.md) are correctly classified as load-dependent WebKit failures carried into slice 10.
- **Not fixed:** findings the owner did not ask to fix are added to the design slice known limitations.

## Resolution

The review was resolved on 2026-11-10: every finding the owner asked to fix is fixed by the changes above, each confirmed closed by its reviewer, and every other finding is recorded in [the design slice known limitations](slice-design-known-limitations.md). The follow-ups' merge gates met further load-dependent browser failures, recorded in [the WebKit failure record](design-slice-webkit-failures.md), and the owner approved each rerun.
