# Sub-slice 10.1 slice review

Status: review completed 2026-11-17; resolved 2026-11-19 when its follow-up changes had merged and the owner decided the remaining entries, the coordinator's deviation and the consolidation question, as listed below. Agent record.

## Scope

- **Base:** `c218028f7e83ee73b3b0cf7fb49192f6539eeb2e` ("Record the slice 9 slice review and close the slice"), the commit that closed slice 9, as the sub-slice rule of [`AGENTS.md`](../../AGENTS.md#development-verification) names it for 10.1.
- **Reviewed head:** `2ad5c2ed8ee5c19979f4b732acd231e104bed571` ("Narrow the homework live refetch to the identifiers each frame names"), the last planned change of 10.1. The review ran over `c218028..2ad5c2e`.
- **Resolved head:** `main` at `e11f01f` ("Show the issuing hint for after-checking work with no manual question") when this record was written; this record's own commit closes the sub-slice.
- **Sub-slice rule.** Each sub-slice's review covers its own list of changes in section 7 of the [slice 10 plan](../plans/slice-10.md#7-changes-in-order), with any inserted consolidation, its plan revision and the follow-ups of the previous sub-slice's review that merged after that review started (D1). Every commit on `main` since `c218028` falls in exactly one list below; P0 and the picomatch acceptance, which are in no sub-slice's list of section 7, join 10.1 as the earliest sub-slice whose review had not started. The follow-ups of this review merged after it started, so they are in 10.2's list and its slice review reviews them; this record names, for each finding, the follow-up that resolved it.
- **10.1's list (reviewed by this review):**

  | Plan step | Commit | Change |
  | --- | --- | --- |
  | P0 (no sub-slice list) | `f3aad2e` | Add the slice 10 implementation plan: results, home pages and hardening |
  | C0a | `8d59d4c` | Print the merge gate's power source and load at its start and end |
  | T1 | `503bb17` | Require evidence for after-fix gates and review slice 10 by sub-slice |
  | C0b | `787deb2` | Consolidate the end-to-end suite and advance its save delays with the page clock |
  | E1 | `3c56502` | Drop student homework emails while the issuing tutor is not active |
  | X1 | `d42fcef` | Fix the student notes autosave flake and make status waits exact |
  | X2 | `5090ba7` | Commit Back and Forward navigations synchronously |
  | P2 | `940369b` | Run the pilot 2 actors with their own language, timezone and theme |
  | F-a | `2ac0c1b` | Move shared notice focus after commit and fake the submission routes |
  | L1 | `899d50e` | Save language choices on public pages in the order made |
  | Outside the plan (no sub-slice list) | `3e37152` | Accept the picomatch advisory GHSA-fc5q-39xc-vjpf until 2026-12-01 |
  | F-b | `2ad5c2e` | Narrow the homework live refetch to the identifiers each frame names |

- **10.2's list, as far as it has merged (reviewed by the 10.2 slice review):**

  | Plan step | Commit | Change |
  | --- | --- | --- |
  | 10.1 review follow-up | `38eb0c9` | Log the merge gate's wall time, phase loads and other containers |
  | 10.1 review follow-up | `8f112a0` | Tighten after-fix evidence and record the idle-machine gate rule |
  | 10.1 review follow-up | `65a9289` | Resolve the sub-slice 10.1 slice review findings |
  | S2 | `528db96` | Specify the results and home pages contract of slice 10.2 |
  | B1 | `cb73714` | Serve the tutor's Today and the student's Home from one GET /home |
  | B2 | `bc400a9` | Dismiss the tutor's onboarding checklist and count open assignments on the student list |
  | B4 | `4da7cba` | End a student's reads of a deleted tutor's work at account_closed_until |
  | 10.1 review follow-up | `f1855cf` | Record the lane A profile and the slower parallel parent tests |
  | 10.1 review follow-up | `1a4fe67` | Propose letting the subtests of one integration test share a test database |
  | B3 | `bfff5c0` | Mark checked feedback as seen with feedback_new and POST /submissions/{id}/feedback-seen |
  | F2 | `e11f01f` | Show the issuing hint for after-checking work with no manual question |
  | 10.1 review follow-up | this commit | Record the sub-slice 10.1 slice review and close the sub-slice |

  The 10.2 slice review maps every commit since this record's commit and lists the commits above again in its scope, with F1, the inserted proposal 0005 pilot, `fix/issued-notice-focus-after-commit` and `chore/gate-power-log-assertion` once they merge.
- **Tier:** slice, under [ADR-0011](../../docs/decisions/0011-tiered-verification.md), with each sub-slice of slice 10 counting as a roadmap slice. Most changes met the security trigger: verification policy (C0a, T1, C0b, the picomatch acceptance), WebSocket frame contents (F-b), email content and dispatch (E1), and the end-to-end tests and fakes of trust boundaries (C0b, F-a).
- **Deferred:** performance measurements wait for 10.5 H1 locally and for the deployment, so reviewers applied the performance profile by reading code. The end-to-end suites and the whole backend run were not run by reviewers; the passing merge gates of each change ran them.

## Reviewers and coverage

Six reviewers ran in fresh contexts on 2026-11-17. Security was split into three families of trust boundaries, because the sub-slice touched more than one, and the frontend reviewer delegated end-to-end test quality to one fresh sub-review that it re-checked.

| Reviewer | Profiles | Components covered | Why |
| --- | --- | --- | --- |
| Backend | Correctness, performance (code-level) | The homework email builders and the dispatcher's drop order (E1); the entity signal extras, encoder and parser, the signals repository, the gateway payload and every homework frame producer and its callers' ordering (F-b); the Go tests that took over C0b's removed browser tests | Only E1, F-b and C0b change `backend/` |
| Frontend, with an end-to-end sub-review | Correctness, browser (dark theme, Firefox, Safari, by reading), performance | C0b's specs and page-clock save delays; X1's exact status waits; X2's synchronous route commits; P2's actor contexts; F-a's `FocusedNotice`, fake submissions routes, transport contract and notification rows; L1's ordered saves; F-b's live refetch, strict frame parser and deadline scheduler | Every frontend file the range changes |
| Security: WebSocket frames, live refetch and email dispatch | Full security | F-b's per-recipient identifiers, the one-recipient invariant, lesson visibility at commit, the client parser; E1's drop before the preference and the cap | T-5, T-6, T-9, T-18, T-21, T-29, T-30, T-31 |
| Security: agent configuration and verification policy | Full security | C0a's gate lines and their self-test; T1's after-fix evidence and sub-slice rules in every copy; C0b's moved assertions and threat citations; the picomatch acceptance; the page clock decision; the policy rows of the known limitations | T-6, T-9, T-10, T-19, T-21, T-25, T-29, T-30, T-36, T-38, T-39, T-41 |
| Security: client sessions, test-control surface and fakes | Full security | L1's session epoch at send and answer time; F-a's fake routes and contract and the lint guard against production imports of fakes; C0b's seeding and the test-control routes; P2's sign-up and preference calls | T-5, T-6, T-9, T-14, T-19, T-22, T-29, T-30, T-36, T-37, T-44 |
| Development tooling and agent configuration | Correctness, security for verification policy | C0a's script and self-test; T1's wording; C0b's measurement record; every gate duration of the range from the gate reports and `merge-gate.log`; every development tooling and budget row of the known limitations, including the rerun row | Development tooling and verification policy changed, and the sub-slice rule asks this reviewer to re-verify every `rerun` and `after-fix` row |

## Integration boundaries and who reviewed them

| Boundary | Reviewer |
| --- | --- |
| Homework producers, listener and gateway | Backend; frames security |
| Gateway and the browser's strict frame parser | Frontend; frames security |
| Notification outbox and the homework email builders | Backend; frames security |
| Public frame language saves and the client session | Frontend; session security |
| Fake transport, its shared contract and the real backend | Frontend; session security |
| Merge gate, its log and the check-budget and rerun rules | Tooling; agent-configuration security |
| End-to-end suite and the Go tests that took over its assertions | Backend and frontend; agent-configuration security for threat citations |

## Results

| Reviewer | Blocker | Major |
| --- | --- | --- |
| Backend | 0 | 0 |
| Frontend, with the end-to-end sub-review | 0 | 0 |
| Security: frames, live refetch and email | 0 | 0 |
| Security: agent configuration and verification policy | 0 | 1 |
| Security: client sessions, test control and fakes | 0 | 0 |
| Development tooling and agent configuration | 0 | 2 |

**Majors:**

- **Security, agent configuration: after-fix evidence recorded only exit statuses.** A before run that failed for an unrelated reason, or a command that selected only a test the fix adds ("No tests found" before, a pass after), would have counted as evidence, so a flaky gate failure could be re-rolled without the owner.
- **Tooling: the consolidation trigger fired at F-a's merge and L1 merged next without a consolidation or an owner decision on the median.** The record did not order the owner's idle-machine decision relative to L1's merge.
- **Tooling: gates of 600 s or more with no record of the owner's answers.** C0a's 747.4 s, L1's 645.1 s and F-b's 607.3 s gates went to the owner, but no answer was in the repository.

**Minor findings and observations:**

- **Backend:** no WebSocket-level test carried the new frame keys over a real socket; one lesson-visibility query runs per student recipient inside the producer's transaction while the tutor's locks are held; the student frame check passed when no frame was sent.
- **Frontend:** the ordered language-save test read the account while the released first save could still be in flight; the notes autosave test that X1 touched had four phases; the Upcoming deadlines refetch stopped after one failed scheduled refetch; the C0b record's "No other test installs a clock" was false as written.
- **Security, frames and email:** a student's submission could name a group lesson the student lost in a concurrent group removal, against the serialisation claim of `realtime-and-video.md` and T-21; the `relationship.ended` email has no tutor-state check.
- **Security, agent configuration:** the series size of intermittent after-fix evidence could be chosen after the first before run; the idle-machine rule lived only in a known-limitations row; L1's 600 s question was not shown to precede its merge; the attempt log held no gate duration; the page clock decision lived only in the development guide.
- **Security, sessions and fakes:** the lint guard against production imports of fakes missed `fakeSubmissions.ts` and `fakeChat.ts`; no test proved that a queued public-page language choice is dropped after a sign-out or an account change.
- **Tooling:** C0a's criterion that its own gate quotes both lines could not be met, because the gate runs `main`'s copy of the script; the self-test accepts `unknown` power fields; the attempt log's power lines carried no time, tree or duration; the after-fix wording was copied into four files with one inconsistency about the infrastructure rerun; C0b's paired measurement ran `main` first in every pair, measured an uncommitted branch, had one failed lane A run and projected the gate from standalone times; the deployment teardown "SIGTERM after SIGTERM" flake needs its sender found.

## Findings and their resolution

Each follow-up is a standard-tier change under the after-fix rule that names the findings it resolves, and its readiness reviewers confirmed them closed. The follow-ups are in 10.2's list, whose slice review reviews them again.

| Resolution | Findings | Confirmed closed by | Merged |
| --- | --- | --- | --- |
| Log the merge gate's wall time, phase loads and other containers | The attempt log without durations (tooling and agent-configuration security): the result line carries `wall=` and per-phase times, each phase logs its loads, and the start line counts other containers; the first part of the consolidation | That change's standard and security-profile readiness reviewers | `38eb0c9` |
| Tighten after-fix evidence and record the idle-machine gate rule | The after-fix evidence major: a match key, a command that selects the failed test without retries or cache, the output lines of every run and the gate log's SHA-256; the series fixed at 10 runs before the first run and always 10 after runs; the idle-machine rule as a dated policy reduction in ADR-0018 and the development guide; the browser's fake clock as ADR-0009 item 13; one canonical text in ADR-0018's Failures bullet with one summary in `AGENTS.md`, ovc-verify and the development guide, and a row for every failed gate, the infrastructure rerun included | That change's standard and security-profile readiness reviewers and a scoped re-review of its fixes | `8f112a0` |
| Resolve the sub-slice 10.1 slice review findings | A group membership change locks its relationships `FOR NO KEY UPDATE`, proven by `TestGroupRemovalSerialisesWithASubmit` (T-21); the student and tutor frames checked over real sockets with exact keys; a helper that fails when no student frame arrived; the language-save test awaits the first save; three phases in the notes test; the deadline refetch keeps its schedule after a failure; the lint guard refuses every fake and contract module outside tests; a `PublicFrame` test of the dropped choice across sign-out and adoption; the `relationship.ended` acceptance written into T-18 | That change's standard and security-profile readiness reviewers and a scoped re-review of its fixes | `65a9289` |
| Record the lane A profile and the slower parallel parent tests; propose shared test databases | The consolidation question, below | That change's own reviews | `f1855cf`, `1a4fe67` |
| This record | The deviation and the gates of 600 s or more (below); C0a's criterion, met by T1's gate `70aa4ea`, the first run of the merged script, whose lines are `gate start: power: mains; load: 2.53 5.93 9.03` and `gate end: power: mains; load: 30.26 23.58 16.59; battery-during-run: no; sleep: no`, and from now on a gate change's criterion never asks its own gate to show that change's behaviour; the C0b record's sentence corrected to "No other autosave test installs a clock"; C0b's measurement method kept as guidance for the next consolidation (alternate which tree runs first in each pair, rerun a lane A run with a failed package, measure the committed tree and project from in-gate times), with no fix because the owner accepted C0b's result | The standard and security-profile reviewers of this change | this commit |

The remaining findings have these resolutions:

- **The `relationship.ended` email without a tutor-state check:** accepted by the owner on 2026-11-17 as intended behaviour, with the reason in T-18.
- **One lesson-visibility query per student recipient under the tutor's locks:** carried into 10.5 H1's measurements by the owner's decision of 2026-11-19; a set-based query only if H1 measures a problem.
- **The self-test accepting `unknown` power fields:** carried into 10.2 as `chore/gate-power-log-assertion` by the owner's decision of 2026-11-19.
- **The teardown flake:** carried into 10.5 H6 by the owner's decision of 2026-11-17, with a diagnostic loop to find the sender of the second signal first.

## Known-limitations entries and their resolution

Every row of the [slice 10 known limitations](slice-10-known-limitations.md) whose change is in 10.1's list was given to the reviewers of its layer as an item to re-verify. Rows of the follow-ups' changes moved to sub-slice 10.2, whose review re-verifies them, under the rule the file's header now states.

| Entry | Re-verified by | Resolution |
| --- | --- | --- |
| C0a: the Linux readers and an unreadable `pmset` are not reached by the self-test | Tooling; agent-configuration security | Accepted by the owner on 2026-11-19 until a Linux machine gates OVC |
| C0a: the self-test's passing gate accepts `unknown` run fields | Tooling; agent-configuration security | Carried into 10.2, `chore/gate-power-log-assertion` (owner, 2026-11-19) |
| C0b: 8 to 11 % saved, not 20 % | Tooling; agent-configuration security | Accepted by the owner on 2026-11-16 |
| `lesson-room.spec.ts:473` in Firefox | Tooling; frontend (unverified, no end-to-end run) | Not seen in the nine full gates from C0b to F-b; carried into 10.5 H3 (owner, 2026-11-19) |
| C0b: the removed member's restart evidence | Backend; agent-configuration security | Accepted by the owner on 2026-11-19 |
| E1: the test does not assert the in-app rows | Backend; frames security | Accepted by the owner on 2026-11-19: the rows are written in the producer's transaction |
| E1: the `relationship.ended` email | Backend; frames security | Accepted by the owner on 2026-11-17, reason in T-18 |
| F-a: `IssuedNotice` and the removed-row fallback on the next frame | Frontend | Carried into 10.2, `fix/issued-notice-focus-after-commit` (owner, 2026-11-17) |
| F-a: the teardown "SIGTERM after SIGTERM" flake | Tooling; session security | Carried into 10.5 H6 (owner, 2026-11-17) |
| F-a: the fake submissions routes' missing refusals | Frontend; session security | Carried into 10.2 F1 (owner, 2026-11-17 and 2026-11-18) |
| F-a: the dialog paths of `FocusedNotice` without a component test | Frontend | Accepted by the owner on 2026-11-19 |
| L1: started beyond the concurrency limit | Tooling; session security | Accepted by the owner on 2026-11-17 |
| L1: the ordered-save test's evidence | Frontend; session security | Fixed by `65a9289` |
| F-b: a student who loses a lesson gets no frame naming it | Backend; frontend; frames security | Accepted by the owner on 2026-11-17 |
| F-b: the fake `deadline_before` filter outside the shared contract | Frontend; frames and session security | Carried into 10.2 F1 (owner, 2026-11-18) |
| The median of 571.0 s at the gates of P2, F-a and L1 | Tooling; agent-configuration security | The idle-machine rule landed in ADR-0018 with `8f112a0`; the consolidation question is answered below |
| L1 merged while the median was 559.4 s (new row) | This record | Ratified by the owner on 2026-11-19 as a one-time deviation |
| F-b's rerun after OSV-Scanner reported the picomatch advisory, tree `d70ee4b` | Tooling; agent-configuration security | Consistent with the rerun definition: base and tree match `merge-gate.log`, the change's own diff is unchanged apart from its row (the threat model's status line merged again on the moved `main`), and the owner approved the rerun on 2026-11-17 |

Moved to 10.2 by the header rule, each for the 10.2 slice review: the `docker` probe's bound (`38eb0c9`), the browser-diagnostics flake under load and the rerun of the verification-policy follow-up's tree `7c3b858` (`8f112a0`), the evidence strength of `TestGroupRemovalSerialisesWithASubmit` (`65a9289`), and the lane A target (`f1855cf`).

## Gate record

From `merge-gate.log` and the coordinator's gate reports. Gates that ran a copy of `tools/merge-gate.sh` from before `38eb0c9` have no `wall=` in the log, so their wall time comes from the coordinator's gate report; C0a's gate ran the copy from before C0a and printed no power line.

| Gate | Base | Tested tree | Load at start | Wall time | Source of the time | Result | Power |
| --- | --- | --- | --- | --- | --- | --- | --- |
| P0 `f3aad2e` | `c218028` | `53af473` | | Markdown-only | | passed | |
| C0a `8d59d4c` | `f3aad2e` | `d3f4a70` | | 747.4 s | gate report | passed | not printed |
| T1 `503bb17` | `8d59d4c` | `70aa4ea` | 2.53 | 592.9 s | gate report | passed | mains |
| C0b `787deb2` | `503bb17` | `4456de5` | 1.34 | 549.6 s | gate report | passed | mains, battery during the run |
| E1 `3c56502` | `787deb2` | `daddbee` | 1.85 | 535.0 s | gate report | passed | mains |
| X1 `d42fcef` | `3c56502` | `b74b312` | 13.67 | 538.4 s | gate report | passed | mains |
| X2 `5090ba7` | `d42fcef` | `5284bc0` | 14.67 | 533.6 s | gate report | passed | mains |
| P2 `940369b` | `5090ba7` | `7c76014` | 6.09 | 559.4 s | gate report | passed | mains |
| F-a `2ac0c1b` | `940369b` | `184fb7c` | 10.29 | 571.0 s | gate report | passed | mains |
| L1 `899d50e` | `2ac0c1b` | `b68849f` | 13.42 | 645.1 s | gate report | passed | mains; unscheduled concurrent database test runs |
| F-b, first | `899d50e` | `d70ee4b` | 2.22 | | | failed, OSV-Scanner | mains |
| Picomatch acceptance `3e37152` | `899d50e` | `50e789f` | 1.68 | 597.1 s | gate report | passed | mains |
| F-b `2ad5c2e` | `3e37152` | `a0107e4` | 5.07 | 607.3 s | gate report | passed | mains |

The follow-ups are in 10.2's list, but the owner answered their gates of 600 s or more while this review was being resolved, so they are recorded here too; the 10.2 slice review re-verifies them.

| Gate | Base | Tested tree | Load at start | Wall time | Source of the time | Result | Power |
| --- | --- | --- | --- | --- | --- | --- | --- |
| Gate log visibility `38eb0c9` | `2ad5c2e` | `f5ae2fc` | 3.40 | 692.2 s | gate report | passed | mains |
| Verification-policy follow-up, first | `38eb0c9` | `7c3b858` | 5.15 | 610.1 s | log | failed, end-to-end | mains |
| Verification-policy follow-up `8f112a0` | `38eb0c9` | `f14bea2` | 5.41 | 768.3 s | log | passed | mains at the start, battery during the run |
| Review findings `65a9289` | `8f112a0` | `4d31c8a` | 2.25 | 601.2 s | log | passed | mains |
| Lane A record `f1855cf` | `4da7cba` | `cdffdac` | 2.27 | Markdown-only, 13.4 s | log | passed | mains |
| Proposal 0005 `1a4fe67` | `f1855cf` | `d20fbc6` | 4.39 | Markdown-only, 13.0 s | log | passed | mains |

**Gates of 600 s or more and the owner's answers.** Each went to the owner as a multiple-choice question before its change merged, and the owner answered before the merge, as ADR-0018 requires:

| Gate | Wall time | Date | Owner's answer | Before the merge | Source |
| --- | --- | --- | --- | --- | --- |
| C0a | 747.4 s | 2026-11-16 | Merge | before | coordinator's record of the owner's quiz answers |
| L1 | 645.1 s | 2026-11-17 | Merge | before | coordinator's record of the owner's quiz answers |
| F-b | 607.3 s | 2026-11-17 | Merge, with the gate time's reduction as the first item of the 10.1 slice review | before | coordinator's record of the owner's quiz answers |
| Gate log visibility | 692.2 s | 2026-11-18 | Merge | before | coordinator's record of the owner's quiz answers |
| Verification-policy follow-up, its rerun | 768.3 s | 2026-11-18 | Merge | before | coordinator's record of the owner's quiz answers |
| Review findings | 601.2 s | 2026-11-18 | Merge | before | coordinator's record of the owner's quiz answers |

The answers come from the coordinator's record of the owner's quiz answers. The 10.2 gates of 600 s or more so far (S2 628.0 s, B2 614.9 s, B4 608.9 s and B3 634.5 s) are for the 10.2 slice review's record.

**Failed gates.** The 10.1 list has one failed gate, F-b's tree `d70ee4b`, whose rerun row is re-verified in the table above. The verification-policy follow-up's tree `7c3b858` failed two load-sensitive browser diagnostics assertions while another session's end-to-end run was still loading the machine; its owner-approved rerun row is in the known limitations, now under sub-slice 10.2, for the 10.2 development tooling reviewer to re-verify.

**Idle-machine rule.** The owner's decision of 2026-11-17 falls after L1's result line in `merge-gate.log` and before F-b's first gate. Every full-run gate since then started at a one-minute load between 1.55 and 5.41, below 6. The rule's other half, that nothing else runs on the machine, cannot be checked from the log, so it rests on the coordinator's record. By that record no OVC session ran a check, test, build or measurement during any of these gates: the verification-policy follow-up's first gate started only after another session's full end-to-end run had ended (2154 passed), on a machine still loaded from it, and during the picomatch gate (`50e789f`) and F-b's merging gate (`a0107e4`) no other OVC session ran a check, test, build or measurement. Whether another project's runs overlapped those two afternoon gates, as the tooling reviewer observed other projects sharing the machine, is unverified: the coordinator cannot see other projects' processes, and those two gates ran before the log counted other containers.

## Coordinator deviation

After F-a merged on 2026-11-17, the last three passing full-run gates on mains power were X2 at 533.6 s, P2 at 559.4 s and F-a at 571.0 s, a median of 559.4 s. The check-budget rule of ADR-0018 as it then stood made a consolidation due before further feature work. L1 was gated next, took 645.1 s and merged after the owner answered its 600 s question, but the coordinator had not recomputed the median after F-a's merge report, so the median went to the owner neither as a consolidation nor as a question. The owner's decision to gate only on an idle machine and to send a high median to the owner at the slice review came after L1's gate. The owner ratified L1's merge on 2026-11-19 as a one-time deviation that sets no precedent; it is recorded in [ADR-0018](../../docs/decisions/0018-targeted-checks-and-merge-gate.md#gate-reruns-and-ratifications-of-sub-slice-101), in the threat model's T-41 section on sub-slice 10.1 and as a row of the known limitations. Under the rule since 2026-11-17 the coordinator recomputes the median from `merge-gate.log` after each merge report and raises it at the earliest unresolved slice review.

## Consolidation outcome

The review's consolidation, decided by the owner when F-b's 607.3 s gate merged, had two parts:

- **Gate log visibility (`38eb0c9`):** the attempt log now gives each gate's wall time, phase times and loads and the count of other containers, so the median and its battery exclusions are computed from `merge-gate.log` alone.
- **Lane A (`f1855cf`):** the profile of `go test -race ./...` on 2026-11-18 found `internal/api` at its `-parallel` cap for about 85 % of its run and bound by PostgreSQL's checkpointer. The only lever it supported, `t.Parallel` in the 19 top-level tests that run serially, was slower in all three alternating pairs (mean `./...` 146.7 s against 139.7 s on `main`), so the owner decided on 2026-11-18 not to merge it, and to defer subtests sharing a test database as [proposal 0005](../../docs/proposals/0005-shared-test-database-for-subtests.md) (`1a4fe67`). The [lane A profile record](../analysis/gate-speed-lane-a-profile-2026-11-18.md) holds the measurements.

At this record the last three passing full-run gates on mains power are B2 at 614.9 s, B4 at 608.9 s and B3 at 634.5 s, a median of 614.9 s; F2's 552.3 s gate ran on battery during the run and leaves the median. ADR-0018 sends that question to the earliest unresolved slice review, this one. The tooling reviewer added that the one-minute load at the start predicts a gate's time poorly, that other containers and other projects' work during the afternoon gates explained much of the growth, and that `internal/worker` took 57.9 s in F-b's gate against about 20 s before, from one sample. On 2026-11-19 the owner chose a measured pilot of proposal 0005, inserted into 10.2 as a consolidation after F1, with no outside load on the machine during gates; the pilot needs, before it is adopted, the decision record, the `AGENTS.md` amendment and the owner's decision that the proposal names.

## Threats examined

| Threat | Reviewers | Result |
| --- | --- | --- |
| T-5 A tutor reaches another tutor's work | Frames security; session security | Examined, not affected; the fake contract's scope check carried into F1 |
| T-6 A student reaches another student's work or a group they left | Frames, session and agent-configuration security | Examined, not affected |
| T-9 Answer keys leak before release | All three security reviewers | Examined, not affected; the removed browser tests are held by stronger Go checks |
| T-10, T-25 Lesson access and stale links | Agent-configuration security | Examined, not affected; the restart tests moved to Go |
| T-14 Script injection | Session security | Examined, not affected |
| T-18 Personal data in logs or emails | Frames security | Examined, improved by E1; the `relationship.ended` acceptance recorded in T-18 by `65a9289` |
| T-19 Secret disclosure | Agent-configuration and session security | Examined, not affected |
| T-21 A removed participant keeps access | Frames and agent-configuration security | Examined; the group-removal race fixed by `65a9289` |
| T-22 Operator misuse | Session security | Examined, not affected |
| T-29 Stale WebSocket or client access | Frames, agent-configuration and session security | Examined, not affected; the cross-account language test added by `65a9289` |
| T-30 Inferring the key from scores | All three security reviewers | Examined, not affected |
| T-31 Email abuse and caps | Frames security | Examined, improved by E1 |
| T-36 Test-control surface | Agent-configuration and session security | Examined, not affected; the lint guard for fakes widened by `65a9289` |
| T-37 Access token theft | Session security | Not affected |
| T-38 Local infrastructure | Agent-configuration security | Not affected |
| T-39 Dependencies and tools | Agent-configuration security | Examined; the picomatch acceptance dated in both scanner configurations and tied by lint, expiring on 2026-12-01 |
| T-44 Unconfirmed accounts and reclaim | Session security | Examined, not affected |
| T-41 Agent configuration and verification policy | Agent-configuration security; tooling | Examined; the evidence, idle-machine and page clock findings fixed by `8f112a0`, the log by `38eb0c9`, the gates of 600 s or more and the deviation recorded by this change |

## Owner decisions

Each was answered by a multiple-choice question.

- **2026-11-16, C0a's 747.4 s gate:** merge.
- **2026-11-17, L1's 645.1 s gate:** merge. The same day the owner allowed L1 to start beyond the concurrency limit.
- **2026-11-17, gating on an idle machine** instead of a consolidation before further feature work, with a high median going to the slice review; recorded in ADR-0018 by `8f112a0` as a policy reduction.
- **2026-11-17, F-b's 607.3 s gate:** merge, with the gate time's reduction as the first item of this review.
- **2026-11-17, closing the findings:** the verification-policy follow-up, the review-findings follow-up and the gate consolidation; the `relationship.ended` email accepted; the teardown flake carried into 10.5 H6; the `IssuedNotice` focus carried into 10.2 as its own change.
- **2026-11-18, the three follow-ups' gates** of 692.2 s, 768.3 s and 601.2 s: merge each.
- **2026-11-18, lane A:** merge only the profile record, not the parallel parents; proposal 0005 deferred as its own change.
- **2026-11-18, the roadmap edits of the slice 10 plan:** carried by this record, not by S2.
- **2026-11-19, the open entries:** accept the Linux readers, the restart evidence, the E1 rows and the `FocusedNotice` dialog paths; carry `lesson-room.spec.ts:473` into 10.5 H3 and the self-test's `unknown` fields into `chore/gate-power-log-assertion`.
- **2026-11-19, L1's merge while the median was 559.4 s:** ratified as a one-time deviation that sets no precedent.
- **2026-11-19, one lesson-visibility query per student recipient:** measured in 10.5 H1.
- **2026-11-19, the consolidation question:** a measured pilot of proposal 0005 inserted into 10.2 after F1, with no outside load during gates.

## Carried

- into 10.2: F1 with the fake submissions refusals and the `deadline_before` contract case; `fix/issued-notice-focus-after-commit`; `chore/gate-power-log-assertion`; the proposal 0005 pilot after F1; the re-verification of the rows that moved to 10.2 and of the follow-ups' gates; the 10.2 gates of 600 s or more;
- into 10.5: H1 with the per-recipient visibility query; H3 with `lesson-room.spec.ts:473` and the browser-diagnostics flake; H6 with the teardown flake;
- the picomatch acceptance, which expires on 2026-12-01 and is removed or renewed by the owner before then;
- the recordings boundary at `account_closed_until`, to the change that registers the recordings routes.

## Resolution

The review was resolved on 2026-11-19. The after-fix evidence major is closed by `8f112a0`, confirmed by its reviewers. The two tooling majors are closed by this record: the gates of 600 s or more each had the owner's answer before the merge, and the deviation is ratified by the owner and recorded in ADR-0018, T-41 and the known limitations. Every minor finding is fixed by `38eb0c9`, `8f112a0` or `65a9289`, recorded by this change, accepted by the owner, or carried into a named sub-slice change, as the tables above and the [slice 10 known limitations](slice-10-known-limitations.md) list.
