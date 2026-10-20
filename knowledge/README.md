# OVC knowledge index

The section-by-section reference behind the [human-facing documentation](../docs/README.md). Start with the [technical introduction](../docs/technical-introduction.md) for orientation, then load the entry sections below for the task at hand. Decision records govern when a specification disagrees with them. Research and review records supply evidence, not decisions. A specification describes required behavior; it is not evidence that the behavior is implemented.

## Current contracts by task

Load the entry sections, follow their links, and read the governing decision before implementing. There is no need to preload unrelated files.

| Task | Entry sections and governing context |
|---|---|
| Locate a component or package | [Module and contract map](architecture/detailed.md#module-and-contract-map), [package layers](architecture/detailed.md#package-layers), [glossary](architecture/detailed.md#glossary) |
| Choose or add a dependency or tool | [Stack per component](architecture/detailed.md#stack-per-component), [ADR-0002](../docs/decisions/0002-implementation-stack.md), dependency rules in [AGENTS.md](../AGENTS.md#code) |
| Select required reviews and checks | [AI development verification](reviews/ai-development-setup.md), [ovc-verify](../.claude/skills/ovc-verify/SKILL.md), [development commands](../docs/development.md) |
| Add or change configuration | [Configuration](architecture/detailed.md#configuration), [deployment shapes](architecture/detailed.md#deployment-shapes) |
| Change entities, constraints or state transitions | [Data model](specs/data-model.md), its [notes and decisions](specs/data-model.md#notes--decisions) |
| Add or change an HTTP route or error | [API conventions](specs/api.md#conventions), the route's section in [the API specification](specs/api.md), [functional requirements](../docs/requirements/functional.md) |
| Authentication, sessions or roles | [Auth routes](specs/api.md#auth), [session entity](specs/data-model.md#session), [threat model](../docs/threat-model.md) |
| Chat, WebSocket frames, presence or notifications | [Chat](specs/realtime-and-video.md#chat), [frame format](specs/realtime-and-video.md#frame-format), [chat routes](specs/api.md#chat) |
| Video lessons or a meeting provider | [Video](specs/realtime-and-video.md#video), [provider interface](specs/realtime-and-video.md#provider-interface), [ADR-0003](../docs/decisions/0003-embedded-video-provider.md) |
| Calendar and lessons | [Lesson routes](specs/api.md#calendar--lessons), [lesson entity](specs/data-model.md#lesson), [open questions](architecture/detailed.md#open-implementation-questions) |
| Homework, submissions and grading | [Assignment routes](specs/api.md#homework--assignments), [submission routes](specs/api.md#homework--submissions--grading), [submission entity](specs/data-model.md#submission) |
| Uploads, storage and downloads | [File routes](specs/api.md#files--attachments), [attachment entity](specs/data-model.md#attachment), [threat model](../docs/threat-model.md) |
| Logging, levels, correlation or redaction | [Observability](specs/observability.md) |
| Assess a trust boundary or security claim | [Threat model](../docs/threat-model.md), then its linked contracts |
| Frontend structure or a feature module | [Frontend module map](architecture/detailed.md#frontend-frontendsrc), [API conventions](specs/api.md#conventions) |
| Plan the next slice | [Roadmap](../docs/roadmap.md), [open implementation questions](architecture/detailed.md#open-implementation-questions), [non-functional requirements](../docs/requirements/non-functional.md) |

## Layout

`specs/` owns behavior; `architecture/` owns maps, layers, stack and terminology; `reviews/` holds review records, with `reviews/ai-development-setup.md` as the active-policy exception; `research/` and `analysis/` hold dated external evidence such as checked versions and measurements, created when the first such record is written.
