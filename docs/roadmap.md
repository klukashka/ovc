# Roadmap

Status: draft written 2026-10-20; pending line-by-line owner review.

Build order, slice by slice. Each slice is runnable and reviewable on its own and ends with its checks registered in the [development guide](development.md). A slice is done when its feature works end to end locally through the real backend and the real frontend, its checks pass, and the documents it affects are updated.

## Slice 0: foundation

Repository, agent rules ([AGENTS.md](../AGENTS.md)), the [ovc-verify](../.claude/skills/ovc-verify/SKILL.md) skill, the two documentation layers, requirements, threat model and initial decision records. No application code. Active checks: Markdown lint, links and anchors, Mermaid diagrams.

## Slice 1: scaffold and local loop

- Backend: `uv` project, FastAPI application factory, typed settings, async database session, Alembic with a comment-free migration template, health route, structured logging bootstrap, request identifiers, the package-layer architecture test, the integration-test database fixture.
- Frontend: `pnpm` project, Vite, React, strict TypeScript, Tailwind, router, typed API client, base layout, ESLint and Prettier, a Playwright smoke test.
- Docker Compose for PostgreSQL. Security scanning tools selected and registered.
- Done when the backend health route answers, the frontend renders its shell against it, and every slice-1 check in the development guide passes.

## Slice 2: accounts and sessions

FR-1. Registration, login, refresh rotation with reuse detection, logout, current user, profile editing, avatar upload, role guards, admin deactivation. Frontend auth pages, token handling, route guards, profile page.

## Slice 3: courses and enrolment

FR-2. Course creation and editing, join codes, adding and removing students, per-role course lists and membership.

## Slice 4: calendar and lessons

FR-3. Lesson scheduling, editing and cancellation, the calendar across courses, timezone display, lesson status.

## Slice 5: chat

FR-5 and FR-7. WebSocket gateway with first-frame authentication, connection registry, in-process broadcaster, course and direct channels, persisted messages with history, presence and typing, notification frames. Frontend realtime client and chat UI.

## Slice 6: video lessons

FR-4. Meeting provider interface with Jitsi and fake implementations and shared contract tests, meeting creation for lessons, join authorization, moderator rule, ending lessons. Frontend embed with join and dispose lifecycle.

## Slice 7: homework

FR-6. Uploads with bounds and cleanup, authorized downloads, assignments, submissions, grading and returning, the teacher's grading queue and the student's view of results.

## Slice 8: hardening and polish

Accessibility pass, empty and error states, rate limits, demo seed data, performance baselines and budgets for NFR-2, a BigBlueButton adapter if needed.

## After the MVP

Several backend instances with a cross-instance broadcaster, S3-compatible storage in production, email notifications, recordings, calendar export, an admin console, internationalization. Each starts as a [proposal](proposals/README.md) when it is designed but not scheduled.
