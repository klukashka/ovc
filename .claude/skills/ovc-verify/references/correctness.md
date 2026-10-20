# Correctness review

Act as an independent reviewer of the supplied scope. Read `AGENTS.md`, then the affected contracts and cited decisions through `knowledge/README.md`. Review only the assigned component/profile. You may delegate narrower work in fresh contexts with the same scope and permissions; do not restart the whole router. Own the combined conclusion and report descendant assignments. Do not edit source, commit or publish. Return findings and evidence in the format specified by the parent skill.

Check behavior against the accepted contracts, the acceptance criteria and the complete diff. Check the supplied direct and transitive impact edges and integration-boundary ownership, including documentation changes. Report newly affected components to the coordinator and inspect each side of an assigned boundary. Verify that every acquired resource has its specified release owner across success, failure, cancellation and shutdown; performance review measures retained resources separately. Pending proposals do not override accepted specifications. Review changed verification rules against the stricter approved base obligations.

- Authentication and users: registration, login, refresh rotation and revocation, token expiry, password hashing, role assignment, profile updates. Exercise expired, revoked and replayed tokens.
- Courses and enrolment: ownership, join codes, invitations, removal, archived courses, and what each role sees in listings.
- Calendar and lessons: UTC storage with per-user timezone display, date-range queries at range boundaries, lesson status transitions, rescheduling and cancellation, overlapping lessons.
- Chat: persistence before fan-out, membership resolution, keyset pagination stability, edit and soft delete, reconnect recovering missed messages from history, presence and typing as ephemeral state.
- Video lessons: meeting creation per lesson, join authorization, moderator flag for the course teacher only, room names never derived from client input, ending a room, provider adapter contract shared by every implementation.
- Homework: assignment draft and publish, due dates, submission status machine (`draft`, `submitted`, `returned`, `graded`), resubmission, grading arithmetic within `max_points`, attachment ownership and idempotent retries.
- API and frontend client: request and response schemas agree with the specification, structured errors, pagination cursors, status codes, the typed client and the features that consume it.
- Every changed executable component: apply [the observability specification](../../../../knowledge/specs/observability.md). Check event names, levels, one reporting owner per event, request and connection correlation, and that forbidden content never reaches a log.

Use the repository's real check commands from `docs/development.md`; do not invent commands for an absent scaffold. Most tests should integrate real components; enforce the no-mocks and deterministic-test rules in `AGENTS.md`. Confirm that checks cover failures and recovery, not only successful return values. State exactly which obligations remain unimplemented or untested.
