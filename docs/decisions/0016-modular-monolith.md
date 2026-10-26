# ADR-0016: A modular monolith

- Status: accepted, 2026-10-26
- Reviewed: records what ADR-0002, ADR-0006, ADR-0012 and ADR-0013 already decide, plus the extraction signals and the per-process secret rule, which are new; wording pending owner review, line by line
- Deciders: project owner
- Related: [ADR-0002](0002-implementation-stack.md), [ADR-0006](0006-background-worker-and-email.md), [ADR-0012](0012-go-backend.md), [ADR-0013](0013-pilot-deployment.md), [package layers](../../knowledge/architecture/detailed.md#package-layers), [deployment shapes](../../knowledge/architecture/detailed.md#deployment-shapes), [hosting cost and load analysis](../../knowledge/analysis/hosting-cost-and-load-2026-10-25.md)

## Context

OVC's architecture has been decided piece by piece: the stack and PostgreSQL fan-out in ADR-0002, the worker and the outbox in ADR-0006, the Go backend and its layering in ADR-0012, and one application VM for the pilot in ADR-0013. No record names the shape these add up to, or says when it should change. The [hosting cost and load analysis](../../knowledge/analysis/hosting-cost-and-load-2026-10-25.md) found that the application tier uses little memory and that its hot queries answer in milliseconds, so one small VM carries far more than the pilot, and that the slow part of growth is cost and operation, not code structure.

## Decision

- **One codebase, two processes, one database.** The API server and the worker are built from one Go module and share one PostgreSQL database and its migrations ([ADR-0006](0006-background-worker-and-email.md), [ADR-0012](0012-go-backend.md)). `ovc-admin` is a third command from the same module. There are no separately deployed services.
- **Modules are enforced, not assumed.** Code is layered as routers and WebSocket handlers, services, repositories and adapters, as tabled in [package layers](../../knowledge/architecture/detailed.md#package-layers), and an architecture test fails the build when a package imports outside its row. Only repositories touch business data, and only adapters touch storage, the meeting provider and outbound notifications.
- **Consistency comes from transactions.** Every external effect is an outbox row written in the same transaction as the business change that causes it ([ADR-0006](0006-background-worker-and-email.md)); live signals are written inside the committing transaction and fanned out to every API instance through PostgreSQL `LISTEN/NOTIFY` ([ADR-0002](0002-implementation-stack.md)). No message broker and no distributed transaction exist.
- **One VM for the pilot.** The pilot runs the reverse proxy, the API, the worker and PostgreSQL on one application VM ([ADR-0013](0013-pilot-deployment.md)). The leased outbox and per-instance listeners already allow more API instances and workers without a change of shape.
- **Per-process secrets.** One typed settings object serves every process, but a secret is loaded only by the processes whose work uses it: the settings loader reads the SMTP credentials only for the worker and the token signing secret only for the API, and the [configuration table](../../knowledge/architecture/detailed.md#configuration) names the processes for most of the secrets it lists. Each new secret names its processes when it is added, and in a deployment each process receives only its own secrets; the web push signing key will be held only by the worker, and the API refuses to start when it is set ([ADR-0015](0015-web-push.md)). The two processes still share one blast radius at the database level, because both connect with the same database role ([ADR-0013](0013-pilot-deployment.md)).
- **Signals that justify extracting a service.** A part of OVC moves into its own service only through a new decision record, and only when at least one of these holds and tuning, a larger VM or more instances of the same processes cannot answer it:
  - it needs to scale or be sized independently, for example heavy media or recording processing;
  - its failures must be isolated from lessons and chat in a way separate processes cannot give;
  - it needs different data residency, retention or access rules from the rest of OVC;
  - a separate team or release cycle owns it.

## Alternatives considered

- **Services from the start:** independent deployment and scaling, at the cost of network contracts, distributed consistency instead of one transaction, more hosts to pay for and operate, and slower changes while the domain is still moving.
- **One process with the worker inside the API:** one fewer process, but background work would compete with requests for the same resources and stop with every API restart, and the worker's lanes could not be scaled or observed on their own.

## Consequences

- A change that spans modules lands as one commit and one deployment; contracts between modules are Go types checked by the compiler.
- The database is shared, so a schema change is coordinated between the API and the worker in the same release, and a database outage stops both.
- Extracting a module later is easier because the layers and adapters already isolate its dependencies; the outbox is the natural seam for an extracted consumer.

## Where it is applied

[Package layers](../../knowledge/architecture/detailed.md#package-layers), [deployment shapes](../../knowledge/architecture/detailed.md#deployment-shapes), [configuration](../../knowledge/architecture/detailed.md#configuration).
