# ADR-0013: Pilot deployment

- Status: accepted, 2026-10-22
- Reviewed: decided by the project owner on 2026-10-22 from the [hosting research](../../knowledge/research/hosting-2026-10-22.md); wording pending line-by-line owner review
- Deciders: project owner
- Amends: [ADR-0004](0004-bigbluebutton-primary-provider.md) in its BigBlueButton host size, for the pilot only
- Related: [ADR-0004](0004-bigbluebutton-primary-provider.md), [ADR-0016](0016-modular-monolith.md) (pending owner review), [ADR-0006](0006-background-worker-and-email.md), [ADR-0008](0008-hosting-and-data-residency.md), [deployment shapes](../../knowledge/architecture/detailed.md#deployment-shapes), [threat model](../threat-model.md)

## Context

The first deployment, after roadmap slice 10, is a pilot of at most two tutors and their students, with one-to-one lessons only: groups stay in the product, but the pilot does not use them. At most two lessons run at the same time, and lessons do not run back to back. The owner wants no loss of performance for this pilot and the lowest cost that allows it. The reference sizes in the non-functional requirements and the official BigBlueButton production minimum are set for far larger loads.

## Decision

- **Application tier:** one Yandex Cloud VM in a Russian region, 2 vCPU at 100% share, 4 GB RAM, 40 GB SSD, running the TLS reverse proxy that serves the frontend, the API, the worker and PostgreSQL 18.
- **Host exposure:** PostgreSQL listens only on a Unix socket and loopback with SCRAM authentication and has no public port; the API and the worker run on the host network or with the socket mounted, never with PostgreSQL bound to a container bridge. The application VM's security group admits only TCP 80 and 443 from the internet; the BigBlueButton host admits only the ports its installation requires; both admit SSH with keys only from the operator's named addresses. Neither VM has a cloud service account attached, so no process on it can fetch cloud tokens from the metadata service.
- **Database roles and logs:** the API, the worker and `ovc-admin` connect as a non-superuser role that owns only OVC's schema; WAL-G runs as the `postgres` operating-system user, whose backup credentials and key no application process can read. PostgreSQL logs no statements beyond DDL (`log_statement` at `ddl` or `none`), logs no parameter values on error (`log_parameter_max_length_on_error=0`), and rotates its logs within 14 days, as ADR-0008 sets for the conferencing host. The production database name never ends in `_test` or `_dev`, because on this VM the loopback host check of the test-control guard gives no separation and only the name rule refuses test control ([T-36](../threat-model.md#threats-and-controls)).
- **Database backups:** WAL-G writes daily base backups and the continuous WAL archive, client-side encrypted, to a dedicated private Yandex Object Storage bucket, which allows point-in-time recovery; `archive_timeout` is 60 seconds, so losing the VM's disk loses at most about a minute of committed writes. The recovery window is 30 days and older backups are deleted, so no backup is more than 31 days old: the daily base backup that starts the window is the one-day margin over [ADR-0008](0008-hosting-and-data-residency.md)'s 30 days, accepted for the pilot. The operator keeps a copy of the WAL-G encryption key outside the VM and outside the cloud account, and before the pilot starts a restore is tested on a fresh VM using only that copy and the bucket. The backup bucket has its own service account that reaches only that bucket, and the application's storage keys cannot reach it.
- **File storage:** Yandex Object Storage in the same region; how downloads carry `nosniff` stays an [open question](../../knowledge/architecture/detailed.md#open-implementation-questions) to settle before the deployment.
- **BigBlueButton 4.0:** a VK Cloud VM with 4 standard vCPU, 8 GB RAM and 50 GB SSD on Ubuntu 24.04, always running. This is below the official production minimum (8 cores, 16 GB, dedicated CPU) and equals its development minimum; the owner accepts it for two concurrent one-to-one lessons. The slice 7 probe confirms it on a 4 vCPU / 8 GB host with two simultaneous one-to-one lessons with webcams, together with the VM's bandwidth against the official 250 Mbit/s, and the host grows to the production size before the pilot adds group lessons or more tutors.
- **Email:** Yandex Cloud Postbox over SMTP, on the condition that Postbox confirms it keeps message logs for at most 30 days ([notifications](../../knowledge/specs/notifications.md)); otherwise Unisender Go under the same condition, whose bounces arrive as inbound webhooks and then need an authenticated webhook route and its own threat-model entry before deployment. Postbox's default quota (200 emails a day, one a second) suffices for the pilot; a raise is requested before the pilot grows. Postbox reports bounces and complaints through a Yandex Data Streams stream in a dedicated folder whose access policy lets only Postbox's own service account write to it and only the intake's credential read it; no other principal, including the application's and the backup keys, can write to it, and the stream keeps events for its shortest available retention. The bounce intake runs in the worker as an adapter, treats each event as untrusted data, marks an address only when the event names OVC's sending identity, and processes each event idempotently by its identifier; it is built before the first deployment.
- **Domain and TLS:** a `.ru` domain, Let's Encrypt certificates, DNS at a Russian provider.
- **Growth:** when the pilot passes two tutors or starts group lessons, PostgreSQL moves to the managed service and BigBlueButton to the production minimum, as the larger setups in the hosting research describe.

## Alternatives considered

- **Managed PostgreSQL from the start:** the provider owns backups, point-in-time recovery and upgrades; it adds about 5,900 roubles a month.
- **High-frequency vCPU for BigBlueButton:** about 1,900 roubles a month more; better single-thread performance, which matters more as lessons grow.
- **Starting the BigBlueButton VM only for lessons:** about half the cost of the host, but it needs a cloud API credential, a new start-up job and a boot before every lesson, and an unscheduled lesson would wait.
- **Unisender Go for email:** 800 roubles a month with ordinary webhooks for bounces, against Postbox's free tier at this volume and its Data Streams events.

## Consequences

- The pilot costs about 9,500 roubles a month including VAT, by the hosting research's unit prices.
- The application VM is a single point of failure for the application and the database together, without failover; a host loss is recovered from the backups, losing at most about a minute of committed writes, an accepted deviation from NFR-4.1 for the pilot.
- PostgreSQL shares 2 vCPU and 4 GB with the API and the worker, so the NFR-2 latencies are measured on the pilot VM after deployment rather than assumed from the reference deployment.
- The operator owns WAL archiving, the backup retention deletion, restore drills and PostgreSQL minor upgrades.
- New secrets: the backup bucket's credentials, the WAL-G encryption key, the Postbox SMTP credentials and the Data Streams reader credential, held as `Secret` settings outside the repository ([T-19](../threat-model.md#threats-and-controls)). The backup credentials on the VM can also delete backups, because retention needs that, so a compromised VM can destroy recovery; accepted for the pilot.
- Backups in the same provider as the database do not survive the loss of the cloud account; accepted for the pilot.
- BigBlueButton below its production minimum may degrade audio or video under load beyond the pilot's; the slice 7 probe is the gate.

## Where it is applied

- [Deployment shapes](../../knowledge/architecture/detailed.md#deployment-shapes)
- [Roadmap, slice 10](../roadmap.md#slice-10-results-home-pages-and-hardening)
- [Hosting research](../../knowledge/research/hosting-2026-10-22.md)
- [Threat model](../threat-model.md)
