# OVC backend

The Go backend ([ADR-0012](../docs/decisions/0012-go-backend.md)): the API server with its REST API and WebSocket gateway, the worker and the `ovc-admin` command, with database access, storage and conferencing adapters. Created in [roadmap slice 1](../docs/roadmap.md#slice-1-scaffold-and-local-loop).

- Packages and their allowed imports: [package layers](../knowledge/architecture/detailed.md#package-layers).
- Configuration variables: [configuration](../knowledge/architecture/detailed.md#configuration).
- Behavior: [API](../knowledge/specs/api.md), [data model](../knowledge/specs/data-model.md), [realtime and video](../knowledge/specs/realtime-and-video.md), [observability](../knowledge/specs/observability.md).
- Rules: [AGENTS.md](../AGENTS.md). Commands: [development guide](../docs/development.md).
