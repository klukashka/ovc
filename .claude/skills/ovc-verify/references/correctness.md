# Correctness review

Act as an independent reviewer of the supplied scope. Read `AGENTS.md`, then the affected contracts and cited decisions through `knowledge/README.md`. Review only the assigned layer or component and its profiles. You may delegate narrower work in fresh contexts with the same scope and permissions; do not restart the whole router. Own the combined conclusion and report descendant assignments. Do not edit source, commit or publish. Return findings and evidence in the format specified by the parent skill.

Check behavior against the accepted contracts, the acceptance criteria and the complete diff. Check the supplied direct and transitive impact edges and integration-boundary ownership, including documentation changes. Report newly affected components to the coordinator and inspect each side of an assigned boundary. Verify that every acquired resource has its specified release owner across success, failure, cancellation and shutdown; performance review measures retained resources separately. Pending proposals do not override accepted specifications. Review changed verification rules against the stricter approved base obligations.

- **Accounts:**
  - registration with consent, verification and enumeration resistance;
  - throttling, refresh rotation and reuse detection;
  - reset and change, sign out everywhere;
  - deletion and purge effects, and `ovc-admin`.

  Exercise expired, revoked, used and replayed tokens.
- **Relationships and groups:**
  - invitation states and email-bound acceptance, the tutor code and join requests;
  - ending and reactivation effects applied in one transaction;
  - group membership effects on chat, rosters and issues;
  - isolation between two tutors of the same student.
- **Schedule:**
  - local-to-UTC conversion across daylight-saving gaps and overlaps, and per-user timezone display;
  - series materialisation and horizon extension;
  - "this" versus "following" edits and splits, overlap warnings;
  - the status machine, the join rule and frozen rosters;
  - calendar range queries at their boundaries.
- **Video and recordings:**
  - join authorization, the join-link handling rules, and meeting identifiers never derived from client input;
  - meeting creation, reuse and ending a meeting, including the provider-side end and restarting the room;
  - the moderator role only for the lesson's tutor;
  - callback signature verification and idempotent upserts;
  - reconciliation, copy, retention, playback authorization;
  - the provider contract shared by the BigBlueButton and fake adapters.
- **Chat:**
  - persistence before fan-out, membership and writability;
  - keyset pagination stability, read markers;
  - edit windows and deletes, reconnect recovery;
  - presence and typing as ephemeral state.
- **Homework:**
  - block schema validation and normalisation version 1;
  - snapshots and the answer visibility table, including the key-leak contract test;
  - autosave versions and the submission state machine;
  - partial and override scoring, returned-work rules, analytics from final scores;
  - idempotent retries of autosave, submit, check and return;
  - deadlines and late-flag recomputation; manual and override points within 0 to the question's points.
- **Notifications and worker:**
  - outbox writes in the same transaction;
  - claiming, backoff and dead rows, idempotency keys;
  - reminders built from current lesson state, the digest conditions;
  - unsubscribe tokens and determinism of `Tick(now)`.
- **Files:** streaming limits, content-type detection, binding rules, quotas and usage, cleanup of unbound and orphaned objects, signed URLs.
- **API and frontend client:**
  - request and response schemas agree with the specification;
  - structured error codes and their translations, pagination cursors, status codes;
  - the generated client and the features that consume it, in both languages.
- Every changed executable component: apply [the observability specification](../../../../knowledge/specs/observability.md). Check event names, levels, one reporting owner per event, request and connection correlation, and that forbidden content never reaches a log.

Use the repository's real check commands from `docs/development.md`; do not invent commands for an absent scaffold. Most tests should integrate real components; enforce the no-mocks and deterministic-test rules in `AGENTS.md`. Confirm that checks cover failures and recovery, not only successful return values. State exactly which obligations remain unimplemented or untested.
