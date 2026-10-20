# OVC — Online Virtual Classroom

A workspace for private tutors teaching adult students online: video lessons with a whiteboard and recordings, private and group chats, a schedule with weekly lessons, and homework built from self-checking tests and tasks the tutor checks. One student can study with several tutors in one account. Russian and English.

Status: foundation and product definition. The product, design, rules and agent configuration are in place; application code starts with [roadmap slice 1](docs/roadmap.md#slice-1-scaffold-and-local-loop).

## Features

| Area | What it does |
|---|---|
| Students and groups | Invitation links and tutor codes, student cards with private notes, groups |
| Schedule | One-off and weekly lessons in the tutor's timezone, shown to everyone in their own |
| Video lessons | BigBlueButton rooms with whiteboard and presentations; recordings for participants |
| Homework | A library of reusable assignments: automatically checked questions, written and file answers, corrected files, feedback, analytics |
| Chat | Private and group chats with files and pictures |
| Notifications | In-app and email, with reminders before lessons |

## Stack

A Go API server and worker on PostgreSQL (huma, pgx, sqlc, goose); React, Vite and TypeScript; BigBlueButton; S3-compatible storage; everything hosted in Russia. Tooling: gofmt, go vet, golangci-lint, govulncheck and `go test`; pnpm, ESLint, Prettier and Playwright. See [ADR-0012](docs/decisions/0012-go-backend.md), [ADR-0002](docs/decisions/0002-implementation-stack.md) and the [decision index](docs/decisions/README.md).

## Repository layout

```
ovc/
├── AGENTS.md       rules for every contributor and coding agent (CLAUDE.md imports it)
├── docs/           human-validated documentation: product, introduction, requirements,
│                   threat model, decisions, proposals, roadmap, development guide
├── knowledge/      agent reference layer: specifications, page design, architecture map,
│                   reviews, research
├── backend/        Go API server, worker and ovc-admin (slice 1)
├── frontend/       React application (slice 1)
└── .claude/        Claude Code settings, hook and the ovc-verify review skill
```

## Start here

- People: [docs/README.md](docs/README.md), then the [product overview](docs/product.md).
- Coding agents and contributors: [AGENTS.md](AGENTS.md), then the [knowledge index](knowledge/README.md).
- Running checks: [development guide](docs/development.md).
