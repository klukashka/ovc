# AI development verification

Active policy for how coding agents verify OVC changes. [AGENTS.md](../../AGENTS.md) requires the checkpoints; [ovc-verify](../../.claude/skills/ovc-verify/SKILL.md) owns routing and evidence. Product behavior remains in the specifications and their cited decisions. This file is the exception in `reviews/`: it is current policy, not a historical record.

## Minimal inventory

- `AGENTS.md` at the repository root holds every rule; `CLAUDE.md` imports it so Claude Code loads the same text.
- One skill, `.claude/skills/ovc-verify`, with four review profiles: correctness, security, browser and performance.
- `.claude/settings.json` sets permissions and one post-edit hook that reports Python syntax errors back to the agent. Secrets, private keys and credential directories are unreadable, which also blocks recognized shell readers such as `cat` but not a script that opens a file itself; system directories are not editable; commits run without confirmation, while push, merge, rebase and reset require it. Absolute paths in rules use the `//` prefix, because a single leading `/` is relative to the settings file.

There are no permanent specialist agents. Correctness spans every component contract, so separate architect, API, database, test-writing or frontend agents would duplicate ownership and drift from `AGENTS.md`. Browser and performance stay separate profiles because performance needs an uncontended machine and an explicit measured conclusion beyond successful interactions. Security owns threat-based reasoning beyond correctness. Each review runs in a fresh general-purpose subagent per component/profile pair, so reviewers do not inherit the implementation conversation.

The repository is local only: no remote, no CI service and no automated pull-request review. The checks in [the development guide](../../docs/development.md) are the gate, and the readiness report to the owner carries the evidence. When a remote and CI are adopted, the same commands become required CI jobs.

## Component checklist

Use [the knowledge index](../README.md) for exact contracts. This checklist names navigation families, not indivisible review assignments: split a family into the actual affected components when they own distinct contracts, state or failure boundaries. Every affected component receives correctness review; the skill selects other profiles from its effects. The last column selects measurements under the performance profile's [resource lifetime method](../../.claude/skills/ovc-verify/references/performance.md#resource-lifetime-measurements); runtime checks activate with the implementation, not because this table names them.

| Component family | Check focus | Performance and resource-lifetime workload |
|---|---|---|
| Authentication and sessions | Hashing, token lifetime, refresh rotation and reuse detection, logout, role claims | Repeat login, refresh and logout cycles; measure hashing latency, query count per request and session rows retained versus revoked |
| Authorization guards | Role and ownership dependencies on every route and frame, 404-versus-403 rule | Count extra queries per guarded request; confirm guards do not load whole collections |
| Courses and enrolment | Ownership, join codes, invitations, removal, archived courses, per-role listings | List courses with many members and courses per user; query count and payload size |
| Calendar and lessons | UTC storage, timezone display, range boundaries, status transitions, rescheduling | Range queries over many lessons; index use and payload size |
| Chat and WebSocket gateway | Persist-then-fan-out, membership, pagination stability, reconnect recovery, close paths | Many connected members and idle sockets; fan-out latency, memory and tasks per connection, registry cleanup after error and abrupt close |
| Notifications and broadcaster | Recipient selection, hint-only payloads, no lost frames for connected users | Burst publishing; queue sizes and cleanup |
| Meetings and provider adapters | Room creation, join authorization, moderator rule, end room, shared contract tests across Jitsi, BigBlueButton and fake | Repeated create, join and end cycles; outbound client connections closed, provider timeouts bounded |
| Homework | Assignment lifecycle, submission state machine, grading arithmetic, return and resubmit | Grading queues with many submissions; query count per page |
| Uploads, storage and downloads | Size and type bounds while streaming, random keys, unreferenced-attachment cleanup, authorized downloads | Large and concurrent uploads and downloads; memory independent of file size, temporary files and descriptors released on success, rejection and disconnect |
| Data model and migrations | Constraints, indexes, migration up and down on a populated database | Migration time on representative data |
| Configuration and process | Settings validation, startup refusal on invalid configuration, logging bootstrap, graceful shutdown | Startup time; connections and tasks closed on shutdown |
| Frontend shell, routing and API client | Thin client, typed errors, token refresh, route guards, loading, empty and error states | Bundle size, loading milestones, query cache bounds |
| Frontend features (courses, calendar, chat, video, homework) | Journeys for both roles, untrusted content rendering, accessibility, responsive layout | Long message lists, calendar navigation, embed mount and dispose cycles; listeners, subscriptions, timers and heap after unmount |
| Development configuration and agent setup | Rule consistency, routing completeness, permissions, preserved checks | Check run duration; instruction-only edits need methodology review, not fabricated runtime samples |

## Activation

Checks activate with the capability they verify, following the [roadmap](../../docs/roadmap.md):

- **Now (documentation only):** Markdown lint, offline link and anchor check, and Mermaid parsing over all Markdown are active and required.
- **Backend scaffold (slice 1):** `uv lock --check`, ruff format and lint, pyright strict, pytest including the package-layer architecture test, and the integration test database container activate. Security scanning tools are selected and registered here.
- **Frontend scaffold (slice 1):** pnpm frozen install, TypeScript check, ESLint, Prettier check and production build activate. The first runnable shell activates Playwright boot, diagnostics and accessibility smoke and loading-performance measurement.
- **Each later slice:** adds its integration tests, its Playwright journeys, its observability assertions and its performance workloads from the table above, registered as commands in the development guide in the same change.

Unimplemented capabilities are `pending implementation` with the slice that activates them; an existing capability that cannot be checked is `blocked`. Missing tools are never silently installed or counted as passing; dependency adoption follows `AGENTS.md`.

Prompt routing is procedural, not enforced by tooling. Reviewers inspect source and actual command output; an implementation-authored report alone does not prove independent review.
