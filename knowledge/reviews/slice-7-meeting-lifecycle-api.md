# Authorized meeting lifecycle API verification

Status: implemented, change checks passed on 2026-11-06; independent standard and security reviews and the exact-tree gate pending. Base: the [creation coordinator](slice-7-meeting-create-coordinator.md) change on `088ca54`.

Scope: section 2 of the [approved local plan](../plans/slice-7-local-completion.md): `POST /lessons/{id}/join`, `/end`, `/restart-room`, `room_open` and `embed_allowed`, persisted join admission, opaque participants, the status job for live lessons, tutor deletion and the ended hint route. Tier: standard. Security trigger: met. No fake room, UI or BigBlueButton adapter is included, so the routes are not user-ready.

## Decisions refining the plan

Each is recorded in the specification named; none changes an approved contract.

- A student seen only through a frozen roster is outside the join scope and answers `404` without a slot ([join rule](../specs/schedule.md#join-rule)).
- `end` on a scheduled or cancelled lesson and `restart-room` on a lesson that is not live answer `409 lesson_not_live` ([join flow](../specs/realtime-and-video.md#join-flow)).
- "Publish no replacement" is enforced across requests through `meetings.revocation_confirmed_at`: while a restarted generation is neither confirmed ended nor settled, a tutor join repeats the exact End and answers `503 provider_unavailable` when it cannot.
- Tutor deletion closes rooms with the existing reason `tutor_end`; the close reason enum is unchanged.
- `embed_allowed` is a constant false in composition until the adapter stage reads its setting.
- The server always sends `room_open` and `embed_allowed`, but the published schema marks them optional for now. Marking them required breaks the type check of two lesson page test fixtures, and this change does not touch lesson page code while the design work runs. The lesson UI stage makes them required together with its fixtures.
- Provider calls run under a 15-second budget inside the caller's context, and evidence after them under a detached 10-second budget.

## Threats

T-10, T-21: scope, session and lesson rechecks after every wait; current audience only. T-25: restart revocation and generation binding. T-34: participant identifiers removed by purge, late results owned through the ledger. T-20: rate admission, tutor and global admission limits, provider and request budgets, hint permits and chains. T-18 also covers the neutral room name. T-18, T-19: only name, opaque identifier and role reach the provider; links stay out of logs, outbox and persistence. T-11, T-12, T-26: no recording route, and hints have no end authority. T-16, T-24, T-45: embedding stays false and no framing changes here. T-36: the routes refuse without test control. T-41: fake evidence only, remote acceptance stays unverified.

## Acceptance

Real API, PostgreSQL and worker through the shared fake:

- `TestMeetingJoinAuthorization`, `TestMeetingJoinSessionRaces`, `TestMeetingConcurrentJoin`, `TestMeetingRestartRevocation`, `TestMeetingEndAndStatus`, `TestMeetingEndedHints`, `TestMeetingParticipantIdentity`, `TestMeetingRateAdmission`.

Interleavings are reached through a clock gate and an entropy gate injected into the real fake, and a real row lock on its state singleton observed through the lock waiter query. No provider result is replaced. A late real-time deadline is never shared with setup.

`TestMeetingLifecycleProcess` runs the real API binary: with test control the join route reaches scope and the production provider lane runs the ended hint effect; without it the three routes answer `503 service_unavailable`, while the lesson route and the hint keep answering.

## Negative controls

Run in a disposable copy of the backend, with the original assertions and race detection; it did not enter the implementation tree. The student's generation comparison and the first-join refusal are removed: `TestMeetingConcurrentJoin/a_link_issued_before_a_restart_is_discarded` fails because the link of the replaced generation is released. With the close guard of the backward-clock fix restored in the query, `TestMeetingCloseAfterClockStepBack` fails with a 500. With the tutor profile lock removed from the production guard, `TestMeetingJoinAuthorization/two_joins_at_the_last_slot_admit_one_under_the_production_guard` fails because both joins are admitted while the capacity row is held. The coordinator's controls are in its [record](slice-7-meeting-create-coordinator.md#negative-controls).

## Review findings fixed

- A client abort leaves no uncertainty (`TestMeetingJoinSessionRaces/aborted_joins_leave_no_uncertain_obligation`).
- A clock stepped back cannot strand a room (`TestMeetingCloseAfterClockStepBack`).
- The tutor limit and the neutral room name are the owner's decisions of 2026-11-06, recorded in the [coordinator record](slice-7-meeting-create-coordinator.md#owner-decisions-of-2026-11-06) (`TestMeetingJoinAuthorization/the_tutor_limit_refuses_before_any_reservation`, `.../the_provider_never_receives_the_lesson_title`).
- A fresh uncertain outcome pauses joining its lesson (`TestMeetingJoinAuthorization/a_fresh_uncertain_outcome_pauses_joining_that_lesson`).
- One 30-second budget covers a join or restart (`.../the_request_budget_stops_work_on_a_held_provider`).
- Hints use four permits and a bounded database wait (`TestMeetingEndedHints/a_held_database_wait_is_bounded_and_still_answers_200`).
- Tutor deletion reports `meeting.closed_for_deletion` and closes creating rows too (`TestMeetingEndAndStatus/tutor_deletion_closes_access_and_queues_cleanup`).
- The API purge race runs through tutor deletion and the real purger (`TestMeetingJoinSessionRaces/purge_during_Create_keeps_ownership`). The storage-level `TestMeetingRecoveryPurgeRace` keeps a direct lesson delete, because its fixture has no purger composition.

## Observability

See [authorized meeting lifecycle](../specs/observability.md#authorized-meeting-lifecycle). Safe content is asserted by `TestMeetingJoinAuthorization`.
