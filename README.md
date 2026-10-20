# OVC — Online Virtual Classroom

A multi-teacher online classroom. Any teacher, of any subject, runs courses with live video lessons, a course chat, a lesson calendar, and homework that students submit and teachers grade.

Status: foundation. The design, rules and agent configuration are in place; application code starts with [roadmap slice 1](docs/roadmap.md#slice-1-scaffold-and-local-loop).

## Features

| Area | What it does |
|---|---|
| Video lessons | Embedded Jitsi Meet rooms (BigBlueButton as an alternative) with the teacher as moderator |
| Chat | Live course channels and direct conversations, saved as history |
| Calendar | Lesson scheduling across courses, shown in each user's timezone |
| Homework | Assignments with files, student submissions, grading with feedback |

## Stack

FastAPI, PostgreSQL and SQLAlchemy on Python 3.11; React, Vite and TypeScript; uv, pnpm, ruff, pyright strict, pytest and Playwright. See [ADR-0002](docs/decisions/0002-implementation-stack.md).

## Repository layout

```
ovc/
├── AGENTS.md       rules for every contributor and coding agent (CLAUDE.md imports it)
├── docs/           human-validated documentation: introduction, requirements, threat model,
│                   decisions, proposals, roadmap, development guide
├── knowledge/      agent reference layer: specifications, architecture map, reviews, research
├── backend/        FastAPI service (slice 1)
├── frontend/       React application (slice 1)
└── .claude/        Claude Code settings, hook and the ovc-verify review skill
```

## Start here

- People: [docs/README.md](docs/README.md), then the [technical introduction](docs/technical-introduction.md).
- Coding agents and contributors: [AGENTS.md](AGENTS.md), then the [knowledge index](knowledge/README.md).
- Running checks: [development guide](docs/development.md).
