# ADR-0002: Implementation stack

- Status: accepted, 2026-10-20
- Reviewed: pending line-by-line owner review
- Deciders: project owner
- Related: [ADR-0003](0003-embedded-video-provider.md), [stack per component](../../knowledge/architecture/detailed.md#stack-per-component)

## Context

OVC needs a REST API, real-time chat, file handling and embedded video, built by a team whose strongest backend experience is Python and FastAPI. The project values fast delivery, strict typing and a conventional stack over novelty, and is verified mostly through integration tests against real components.

## Decision

- **Backend:** Python 3.11, FastAPI, Pydantic v2, SQLAlchemy 2.0 async with Alembic, PostgreSQL, native WebSockets. Layered as routers and gateway → services → repositories, with adapters for storage, conferencing and notifications.
- **Frontend:** React with Vite and strict TypeScript, Tailwind CSS, TanStack Query for server state, React Router.
- **Python tooling:** uv for environments and dependencies, ruff for lint and format, pyright in strict mode, pytest.
- **Frontend tooling:** pnpm, ESLint, Prettier, Vitest with Testing Library, Playwright.
- **Local infrastructure:** Docker Compose.

## Alternatives considered

- **Django:** complete, but its synchronous core and server-rendered defaults fit a decoupled SPA with WebSockets less naturally than FastAPI, and the team knows FastAPI.
- **Node.js backend:** one language across the stack, but discards the team's Python depth and tooling.
- **Next.js full stack:** strong developer experience, but couples the API to the frontend framework and complicates a Python backend.
- **mypy instead of pyright; pip or Poetry instead of uv:** workable, but pyright strict is faster and stricter by default, and uv gives one fast tool for Python versions, environments and locked dependencies.

## Consequences

- Two languages, each with strict static checks that every change must pass.
- PostgreSQL covers relational academic data, keyset pagination and later cross-instance fan-out through `LISTEN/NOTIFY` without new infrastructure.
- Async SQLAlchemy and WebSockets add a learning curve that matches the real-time requirements.

## Where it is applied

[Stack per component](../../knowledge/architecture/detailed.md#stack-per-component), [package layers](../../knowledge/architecture/detailed.md#package-layers), [development guide](../development.md), [AGENTS.md](../../AGENTS.md#code).
