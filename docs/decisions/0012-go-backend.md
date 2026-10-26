# ADR-0012: Go backend

- Status: accepted, 2026-10-20; supersedes the backend and Python tooling rows of [ADR-0002](0002-implementation-stack.md)
- Reviewed: decided by the project owner on 2026-10-20 before any backend code existed; wording pending line-by-line owner review
- Deciders: project owner
- Related: [ADR-0002](0002-implementation-stack.md), [ADR-0016](0016-modular-monolith.md) (pending owner review), [ADR-0006](0006-background-worker-and-email.md), [stack per component](../../knowledge/architecture/detailed.md#stack-per-component), [package layers](../../knowledge/architecture/detailed.md#package-layers)

## Context

ADR-0002 chose Python and FastAPI for the backend because Python was assumed to be the team's strongest backend language. Before the first line of backend code, the owner, who works in both Python and Go, chose Go instead. The backend is an I/O-bound REST API with a WebSocket gateway for chat, presence and live updates, a worker with a PostgreSQL outbox, email, object storage and a conferencing integration; media never passes through it. Go gives a compiler that catches more mistakes before runtime, goroutines that suit the long-lived WebSocket connections, and small single-binary deployments. The specifications were written stack-neutral apart from a few names, so the switch costs documentation edits only.

## Decision

- **Language:** Go, the latest stable release when slice 1 starts, pinned through `go` and `toolchain` in `go.mod`.
- **HTTP and API contract:** [huma v2](https://huma.rocks) on the standard library's `net/http` router. Handlers take and return typed structs; huma validates input and generates the OpenAPI 3.1 document that the typed frontend client is generated from. huma's default problem-details errors are replaced with the error shape of the [API specification](../../knowledge/specs/api.md#conventions).
- **Database:** PostgreSQL through `pgx`, with queries written in SQL and compiled to typed Go functions by `sqlc`. Repositories wrap the generated queries and are the only code that touches business data.
- **Migrations:** `goose` with plain SQL files, embedded in the binaries with `//go:embed`.
- **WebSockets:** `coder/websocket`.
- **Logging:** the standard library's `log/slog` with JSON output.
- **Tooling:** `gofmt`, `go vet`, golangci-lint with a committed configuration, `govulncheck`, and the standard `testing` package. Test doubles are fakes behind interfaces; the mocking and patching libraries listed in `AGENTS.md` are forbidden.
- **Binaries:** `cmd/api`, `cmd/worker` and `cmd/ovc-admin` in one Go module under `backend/`, sharing packages under `backend/internal/`.
- **Unchanged from ADR-0002:** the frontend, the frontend tooling, PostgreSQL and Docker Compose for local infrastructure.

Remaining libraries (configuration parsing, JWT, password hashing, the S3 and SMTP clients) are chosen in the slice that first needs them and recorded in the stack table, as before.

## Alternatives considered

- **Keep Python and FastAPI:** faster to write CRUD with Pydantic and automatic OpenAPI, but weaker compile-time guarantees, heavier deployments, and a concurrency model that is easier to misuse in a WebSocket-heavy service.
- **Spec-first OpenAPI with oapi-codegen:** an explicit contract, but every endpoint is written twice (YAML and Go); huma keeps one source of truth in code.
- **An ORM (ent, bun, gorm):** quicker for simple CRUD, but hides the SQL that the outbox (`SKIP LOCKED`), keyset pagination and the scoped queries depend on.
- **Atlas or golang-migrate for migrations:** Atlas adds declarative planning with parts behind a paid tier; golang-migrate is similar to goose but less convenient as a library.

## Consequences

- `AGENTS.md` rules are restated for Go: no comments or linter directives (only `//go:embed` and `//go:build`, shebangs and goose's `-- +goose Up/Down` markers are allowed), no `any` outside generic constraints, no discarded errors, and the Go mocking and patching techniques are forbidden.
- The package-layer table moves from `ovc.*` Python packages to `backend/internal/*` Go packages; the architecture test checks Go imports.
- Specifications that named Pydantic models or `ovc.*` packages now name Go structs and `internal/*` packages; their behavior is unchanged.
- The local toolchain needs Go instead of Python and uv; the agent hooks stay Python scripts run by the system interpreter.

## Where it is applied

- [AGENTS.md, code](../../AGENTS.md#code) and [testing](../../AGENTS.md#testing)
- [Stack per component](../../knowledge/architecture/detailed.md#stack-per-component) and [package layers](../../knowledge/architecture/detailed.md#package-layers)
- [Development guide](../development.md)
- [Roadmap, slice 1](../roadmap.md#slice-1-scaffold-and-local-loop)
