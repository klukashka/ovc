# ADR-0008: Hosting and personal data in Russia

- Status: accepted, 2026-10-20
- Reviewed: pending line-by-line owner review
- Deciders: project owner
- Amended by: [ADR-0015](0015-web-push.md), with one exception for web push metadata to the browser vendors' push services (pending owner review)
- Related: [ADR-0004](0004-bigbluebutton-primary-provider.md), [ADR-0006](0006-background-worker-and-email.md), NFR-1, [threat model](../threat-model.md)

## Context

OVC's users are mainly in Russia. Russian personal-data law (Federal Law 152-FZ) requires the personal data of Russian citizens to be recorded and stored in databases located in Russia. OVC stores names, emails, chat messages, homework, files and lesson recordings with voices and faces, all of which are personal data.

## Decision

- Every component that stores or processes user data is hosted in Russia: the API and worker, PostgreSQL and its backups, object storage for files and recordings, the BigBlueButton server, and the email provider.
- Object storage is an S3-compatible service from a Russian provider and is part of the MVP, replacing the local-disk-only plan, because per-tutor quotas and recordings need it.
- Third-party services outside Russia receive no personal data: no foreign analytics, error-tracking or font CDNs that log user data. Fonts and other static assets are self-hosted.
- The privacy policy, consent at sign-up and the operator's registration with the data-protection authority are owner obligations outside this repository; the product supports them with a consent checkbox at registration and account deletion (FR-1.7).

## Alternatives considered

- **Hosting outside Russia:** conflicts with the data-localisation requirement for the primary audience.
- **Hybrid (data in Russia, services abroad):** every foreign service becomes a cross-border transfer to assess; the MVP avoids them entirely.

## Consequences

- Provider choice for hosting, storage and email is constrained to providers with Russian data centres; their names and checks go into `knowledge/research/` when selected.
- Development and tests never use production providers; local development uses a local S3-compatible server, the fake meeting provider and a mail sink.
- Retention is bounded everywhere personal data persists: database backups are kept 30 days, the BigBlueButton host keeps its logs 14 days and deletes provider-side recordings once OVC holds its copy, and recordings expire after at most 365 days.

## Where it is applied

[Stack and deployment](../../knowledge/architecture/detailed.md#deployment-shapes), [non-functional requirements](../requirements/non-functional.md), [threat model](../threat-model.md), [web push](0015-web-push.md) (pending owner review).
