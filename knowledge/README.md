# OVC knowledge index

The section-by-section reference behind the [human-facing documentation](../docs/README.md). Start with the [technical introduction](../docs/technical-introduction.md) for orientation and the [product overview](../docs/product.md) for intent, then load the entry sections below for the task at hand. Decision records govern when a specification disagrees with them. Research and review records supply evidence, not decisions. A specification describes required behavior; it is not evidence that the behavior is implemented.

## Current contracts by task

Load the entry sections, follow their links, and read the governing decision before implementing. There is no need to preload unrelated files.

| Task | Entry sections and governing context |
|---|---|
| Locate a component or package | [Module and contract map](architecture/detailed.md#module-and-contract-map), [package layers](architecture/detailed.md#package-layers), [glossary](architecture/detailed.md#glossary) |
| Choose or add a dependency or tool | [Stack per component](architecture/detailed.md#stack-per-component), [ADR-0002](../docs/decisions/0002-implementation-stack.md), dependency rules in [AGENTS.md](../AGENTS.md#code) |
| Select required reviews and checks | [AI development verification](reviews/ai-development-setup.md), [ovc-verify](../.claude/skills/ovc-verify/SKILL.md), [development commands](../docs/development.md) |
| Add or change configuration or deployment | [Configuration](architecture/detailed.md#configuration), [deployment shapes](architecture/detailed.md#deployment-shapes), [ADR-0008](../docs/decisions/0008-hosting-and-data-residency.md) |
| Change entities, constraints or state values | [Data model](specs/data-model.md), then the domain specification that owns the behavior |
| Add or change an HTTP route or error code | [API conventions](specs/api.md#conventions), the route's section in [the API](specs/api.md), [functional requirements](../docs/requirements/functional.md) |
| Registration, sessions, passwords, deletion, `ovc-admin` | [Accounts](specs/accounts.md), [ADR-0005](../docs/decisions/0005-tutor-centric-domain.md), [threat model](../docs/threat-model.md) |
| Student cards, invitations, tutor codes, join requests, groups | [Relationships](specs/accounts.md#tutorstudent-relationships), [invitations](specs/accounts.md#invitations), [tutor code](specs/accounts.md#tutor-code-and-join-requests), [groups](specs/accounts.md#groups) |
| Lessons, series, timezones, status, calendar | [Schedule](specs/schedule.md), [ADR-0007](../docs/decisions/0007-recurring-lessons.md) |
| Video rooms, join links, recordings | [Video](specs/realtime-and-video.md#video), [recordings](specs/realtime-and-video.md#recordings), [ADR-0004](../docs/decisions/0004-bigbluebutton-primary-provider.md), [BigBlueButton research](research/bigbluebutton-2026-10-20.md) |
| Chat, WebSocket frames, presence, live updates | [Chat](specs/realtime-and-video.md#chat), [frame format](specs/realtime-and-video.md#frame-format) |
| Library, builder, block types, normalisation | [Library](specs/homework.md#library), [assignment blocks](specs/homework.md#assignment-blocks), [normalisation](specs/homework.md#short-answer-normalisation) |
| Issuing, answer visibility, submitting, reviewing, analytics | [Homework](specs/homework.md), especially [answer visibility](specs/homework.md#answer-visibility) |
| Emails, reminders, digests, preferences, outbox, worker jobs | [Notifications](specs/notifications.md), [ADR-0006](../docs/decisions/0006-background-worker-and-email.md) |
| Uploads, quotas, downloads | [Files](specs/files.md) |
| Logging, levels, correlation, redaction | [Observability](specs/observability.md) |
| Assess a trust boundary or security claim | [Threat model](../docs/threat-model.md), then its linked contracts |
| Build or change a page | [Pages](design/web/pages.md), [navigation](design/web/navigation.md), [states](design/web/states.md), [tokens](design/web/tokens.md), the [clickable prototype](design/web/prototype/README.md) as a visual reference, [frontend module map](architecture/detailed.md#frontend-frontendsrc) |
| Write or extend an end-to-end test | [Pilot scenarios](design/web/pilot-scenarios.md), the [test harness](../docs/development.md#end-to-end-test-harness), then the pages and routes each step names |
| Plan the next slice | [Roadmap](../docs/roadmap.md), [open implementation questions](architecture/detailed.md#open-implementation-questions), [non-functional requirements](../docs/requirements/non-functional.md) |

## Layout

- `specs/` owns behavior.
- `design/` owns detailed surface design: pages, navigation, states, tokens, pilot scenarios and the clickable prototype.
- `architecture/` owns maps, layers, stack and terminology.
- `reviews/` holds review records, with `reviews/ai-development-setup.md` as the active-policy exception; [`reviews/slice-1-review.md`](reviews/slice-1-review.md) records the slice 1 review and where each remaining finding goes, and `reviews/slice-<n>-known-limitations.md` collects the minor findings accepted during a slice, which its slice review re-verifies.
- `plans/` holds approved slice implementation plans written by agents; they record how a slice is built and are not decisions.
- `research/` holds dated external evidence such as checked provider facts and versions; `analysis/` holds dated analyses such as measurement studies, starting with [the slice 1 performance baselines](analysis/slice-1-baselines-2026-10-21.md).
