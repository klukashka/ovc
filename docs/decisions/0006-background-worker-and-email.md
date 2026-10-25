# ADR-0006: A background worker with a database outbox, and email as the external channel

- Status: accepted, 2026-10-20
- Reviewed: pending line-by-line owner review
- Deciders: project owner
- Related: FR-4.2, FR-5.6, FR-13, [notifications specification](../../knowledge/specs/notifications.md)

## Context

Several behaviors happen outside a user request:

- lesson reminders before a lesson starts;
- message digests after a user has been offline for a while;
- materialising occurrences of recurring lessons;
- reconciling and copying recordings;
- deleting expired recordings and unreferenced uploads;
- sending every email.

Sending email inside a request would couple user actions to the mail provider's availability, and losing an email when the process restarts is unacceptable for invitations and password resets.

## Decision

- A separate **worker process** runs from the same codebase and the same database. It has two loops: an outbox dispatcher and a scheduler of periodic jobs.
- **Outbox:** every external effect (an email, a recording copy, a provider call that can be retried) is written as an outbox row in the same transaction as the business change that causes it. The worker:
  - claims rows with `FOR UPDATE SKIP LOCKED` in a short transaction that sets a lease, then performs the effect outside any transaction, so long effects such as recording copies never hold a lock or a connection;
  - keeps separate lanes with their own concurrency limits for emails, provider calls and recording copies;
  - performs the effect with an idempotency key;
  - retries with exponential backoff;
  - moves a row to `dead` after a bounded number of attempts, with a diagnostic event.
- **Scheduled jobs** derive their work from current database state at run time. A reminder is built from the lesson as it is when the reminder is due, so a moved lesson never receives a stale reminder.
- **Time is injected.** The worker exposes a single `Tick(now)` step used by tests with a fake clock; production runs each outbox lane in its own loop beside a scheduler, and `Tick(now)` runs one pass of the same work for tests (pending owner review).
- **Email** is the only external notification channel in the MVP:
  - sent over SMTP through a provider hosted in Russia;
  - with SPF, DKIM and DMARC configured;
  - rendered per recipient in their language and timezone at send time;
  - optional emails carry a signed one-click unsubscribe link.
- No message broker is introduced: PostgreSQL is the queue, and PostgreSQL `NOTIFY` carries live-update signals from the worker and the API to every API instance's WebSocket registry.

## Alternatives considered

- **Celery or another broker-based task queue:** adds a broker to operate, and still needs an outbox to be transactional with business changes.
- **Sending inside requests:** loses emails on provider outages and slows requests.
- **Database triggers or `pg_cron`:** moves business logic into the database, out of the typed and tested code.

## Consequences

- Two processes to run and observe: the API and the worker.
- At-least-once delivery means every effect needs an idempotency key, and receivers (including the mail provider) may see a duplicate after a crash between sending and recording success.
- Worker behavior is testable deterministically without sleeps.

## Where it is applied

[Notifications](../../knowledge/specs/notifications.md), [schedule](../../knowledge/specs/schedule.md), [recordings](../../knowledge/specs/realtime-and-video.md#recordings), [module map](../../knowledge/architecture/detailed.md#module-and-contract-map).
