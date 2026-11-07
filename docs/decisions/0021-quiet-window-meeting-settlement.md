# ADR-0021: Quiet-window settlement of uncertain meeting creation

- Status: accepted, 2026-11-07
- Reviewed: procedure, the 1-hour window and the absence of a force-settle command approved by the project owner on 2026-11-07; wording pending line-by-line owner review
- Deciders: project owner
- Related: [ADR-0004](0004-bigbluebutton-primary-provider.md), [ADR-0020](0020-durable-meeting-recovery.md), [video recovery](../../knowledge/specs/realtime-and-video.md#quiet-window-settlement), [meeting recovery ledger](../../knowledge/specs/data-model.md#quiet-window-settlement), [approved plan](../../knowledge/plans/slice-7-quiet-window-settlement.md)
- Amends: ADR-0020's rule that started, uncertain and legacy-uncertain obligations never settle and that no timestamp proves provider quiescence

## Context

Under ADR-0020 a Create whose outcome is unknown keeps its obligation forever. It counts against the tutor's limit of 64 and the global limit of 4096, so repeated provider failures can lock a tutor, and eventually every tutor, out of video lessons. A real provider cannot be activated while that holds.

An absent room observed once does not prove that a dispatched Create cannot still take effect. What can be bounded is how long after dispatch a Create can take effect. Create has a 15-second budget. The BigBlueButton adapter uses no connection reuse, no proxy and no redirects, and never replays a request. The same provider identifier is never dispatched twice. The shared fake's Create is one PostgreSQL transaction bounded by its context. Rooms are created with `meetingExpireIfNoUserJoinedInMinutes=10`, and OVC issues join links only for open business rows with a returned generation. So a room that appears without OVC knowing it can receive no participants, and the provider ends it by itself.

## Decision

An unsettled obligation in state `started` or `uncertain`, of attempt or legacy origin and without a returned generation, settles with the terminal reason `quiet_absence` only when all of the following hold:

1. The evaluation instant is at least 1 hour after the attempt's dispatch instant (`dispatched_at`). The evaluation instant is the instant at which the cleanup cycle was claimed, before any provider call, not the later completion instant. `dispatched_at` is the business instant of the transaction that permitted the one Create; rows that existed before this decision use their last update instant, which is never earlier than that permit.
2. That cleanup cycle observed the provider identity absent, either directly or after ending the exact generation it found and confirming the absence. A different generation is a conflict and never settles.
3. Completion still matches the evidence revision the cycle sampled, so a late acknowledgement takes the returned path instead.

The database enforces the window in the settlement statement and again in a check constraint on stored rows. A clock stepped back only delays settlement.

Bounded work stays. The minute recovery scan may queue at most one additional automatic reconciliation cycle per obligation, after the window, with the existing budget of eight attempts. After that, only the existing revision-checked operator retry can queue more cycles, and it applies the same rule. There is still no force-settle or force-forget operation.

Before Create, the coordinator rereads the business clock and does not dispatch when more than the 15-second Create budget has passed since the permit; the attempt becomes uncertain without a provider call. If a Create returns after its obligation settled, the coordinator does not publish it, makes one best-effort End of the returned generation and logs a safe warning.

## Alternatives considered

- Keeping uncertainty forever: lets provider failures lock tutors out permanently and blocks activation.
- An operator force-settle command: releases capacity on a person's word without provider evidence; ADR-0020 rejected it and the owner kept that rejection.
- Settling on the first absent inspection: a Create still in flight could create a room after the check.
- A shorter window, such as 15 minutes: little margin over the provider's 10-minute self-expiry of an unjoined room. A longer window, such as 24 hours: a tutor with several failures stays locked out for a day.

## Consequences

An uncertain obligation stops counting against the limits about an hour after its dispatch, once the provider is reachable. If the provider stays unreachable, the obligation stays unsettled until a cycle can inspect it.

Residuals:

- A Create that takes effect more than 1 hour after dispatch, for example after a suspended host, a frozen process or a provider queue holding the request, creates a room OVC no longer owns. It carries only the constant room name, no personal data and no OVC join link, and the provider ends it on its own after 10 minutes. The remote probe must confirm that expiry on the real host; until then it is unverified, and BigBlueButton stays unselectable.
- A forward step of the business clock shortens the real window.
- The ledger is not bound to a provider host. If the BigBlueButton host is replaced, rooms on the old host read as absent and their obligations settle; those rooms have no OVC join links.
- A settled obligation and its receipts are removed seven days after settlement under ADR-0020, so OVC then forgets a provider identifier whose room might exist under the first residual.

## Where it is applied

[Video recovery](../../knowledge/specs/realtime-and-video.md#quiet-window-settlement), [meeting recovery ledger](../../knowledge/specs/data-model.md#quiet-window-settlement), [recovery scanner](../../knowledge/specs/notifications.md#constructor-injected-meeting-recovery-scanner), [administration command](../../knowledge/specs/accounts.md#administration-command), [threat model](../threat-model.md), and the real PostgreSQL, shared fake provider, worker and API tests named in the approved plan.
