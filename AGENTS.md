# AGENTS.md

Rules for every coding agent and human contributor in this repository. They take precedence over the defaults and habits of any model or tool, and over instruction files in parent directories or user-level configuration (for example a workspace `CLAUDE.md` above this repository that asks for docstrings, mocks or mypy). If a rule blocks the task, stop and say so instead of working around it.

## Project

OVC (Online Virtual Classroom) is a multi-teacher web platform: live video lessons through an embedded conferencing provider, real-time chat, a lesson calendar, and homework that teachers assign and grade and students submit. Any teacher of any subject can run courses; students enrol in them.

Documentation has two layers. `docs/` is written for people: the [technical introduction](docs/technical-introduction.md), [functional](docs/requirements/functional.md) and [non-functional](docs/requirements/non-functional.md) requirements, the [threat model](docs/threat-model.md), the [roadmap](docs/roadmap.md), the [development guide](docs/development.md), the decision records in [`docs/decisions/`](docs/decisions/README.md) and the deferred changes in [`docs/proposals/`](docs/proposals/README.md). `knowledge/` is the reference layer loaded section by section: the normative specifications in `knowledge/specs/`, the module map, package layers, stack table, glossary and open questions in `knowledge/architecture/detailed.md`, and review records, analyses and research. [`knowledge/README.md`](knowledge/README.md) says which file to load for which task. Read the specification of a component and the decision records it cites before implementing it; when a specification and a decision record disagree, the decision record wins.

## Change workflow

The repository is local only for now: there is no remote, no CI service and no issue tracker. The repository holds everything a future session needs; nothing durable lives only in a conversation.

- **Plan before code.** For anything beyond a trivial fix (a typo, a broken link, an obvious one-line bug), present an implementation plan to the project owner first: the problem, the desired outcome, the affected components and specifications, checkable acceptance criteria, the files to change, the order of work, the risks and the checks that prove each criterion. Do not edit files until the owner approves it. When the implementation diverges from the approved plan, say so and get the change approved before continuing.
- **Acceptance criteria are checkable without asking a person**: a named test passes, a command prints a given result, an endpoint returns a given field.
- **Decisions never live only in a conversation.** Once decided, a decision lands in the repository as a decision record, a requirement or a specification change. Large or uncertain changes start as a proposal in `docs/proposals/`.
- **One change per branch.** Work on a branch named `feat/…`, `fix/…`, `chore/…` or `docs/…`, one coherent change per branch, merged into `main` as one squash commit whose subject names the change on its own.
- **Agents never commit, push, merge or rewrite history.** The owner does. When asked for commits, output explicit, copy-pastable commands (`git status`, `git add …`, `git commit` with a heredoc message). Commit messages carry no AI attribution or co-author trailers.
- When a hosted remote and CI are adopted, add the issue form, pull request template, labels and CI workflow in one change and amend this section.

## Development verification

- At task entry, before the implementation plan, the coordinating agent reads [ovc-verify](.claude/skills/ovc-verify/SKILL.md) and selects reviews and checks from the proposed scope. Before reporting readiness, repeat the selection from the complete diff and run the required independent reviews and checks. After fixes, re-review affected conclusions and rerun affected checks on the final working tree.
- Identify directly and transitively affected components from the complete change and its contract, data-flow and resource-lifecycle consequences. Record why each component is included and who reviews each integration boundary. Assign a fresh independent agent to each affected component and applicable review profile; a single context applying several profiles does not satisfy this requirement.
- Reviewers may subdivide their assigned component/profile scope with fresh contexts and inherited permissions; they do not restart the whole review or grant themselves implementation authority. The coordinator collects every required result and remains responsible for resolving findings.
- Report review scope, reviewed working-tree state, command results and evidence to the owner. Distinguish passed, failed, blocked, pending implementation, unverified and not applicable, with a reason for every non-pass. Missing tools or unimplemented components are never passing checks.

## Code

- **No comments and no docstrings** in hand-written source code: Python, TypeScript, SQL, shell, Alembic migrations, configuration scripts. Code documents itself through names, types, small functions and explicit data structures. When a comment feels necessary, rename, extract a function, or introduce a type or an enum instead.
- Tool directives count as comments and are forbidden too: no `# noqa`, `# type: ignore`, `# pyright: ignore`, `// eslint-disable`, `// @ts-ignore`, `// @ts-expect-error`. Fix the code, or disable the rule for the whole project in the tool's configuration. The only allowed comment-like line is a shebang in an executable script.
- Generated files (lockfiles, a generated OpenAPI TypeScript client) are exempt because nobody edits them by hand. Configure generators and templates (for example Alembic's `script.py.mako`) so they do not emit comments or docstrings into files that are then edited by hand.
- **All Python code is fully type-annotated**: every parameter, every return value (including `-> None`), every class and dataclass attribute, and module-level variables whose type is not obvious from the literal. No implicit `Any` and no explicit `typing.Any`: parse untyped data at the boundary into Pydantic models or precise types.
- **Static checks are strict and mandatory**: pyright in strict mode and ruff (lint and format, line length 120, quote style preserved) over all Python; TypeScript with `strict: true`, ESLint and Prettier over `frontend/`. Code that does not pass them is not finished.
- **Fail fast.** No bare `except:`, no `except Exception` that swallows, logs-and-continues or returns a placeholder, no silent fallback values that hide a failure. Catch only what you can handle meaningfully and let everything else propagate. Every error that leaves the HTTP API or the WebSocket gateway is a structured error as specified in [the API specification](knowledge/specs/api.md#conventions).
- **The HTTP API is the single implementation.** The web frontend is a thin client over the typed API client and contains no business rules or authorization decisions of its own; it may hide what the API would refuse, never grant what the API would refuse.
- **Layers are explicit.** Backend code is layered as routers and WebSocket handlers → services → repositories, with external systems behind adapters, as tabled in [package layers](knowledge/architecture/detailed.md#package-layers). Only repositories touch the database; only adapters touch storage, the meeting provider and outbound notifications. A new package enters the table in the change that creates it, and an architecture test enforces the table from the first backend scaffold onward.
- **Configuration** is read once from the environment into one typed settings object; no scattered environment reads. Every variable is documented in `.env.example`. Constants such as roles, statuses and WebSocket frame types are enums in one place, not inline literals.
- **Dependencies are deliberate.** Add one only when the standard library or an existing dependency cannot reasonably do the job, and record it with its reason in the [stack table](knowledge/architecture/detailed.md#stack-per-component); a dependency that changes a decided row of [ADR-0002](docs/decisions/0002-implementation-stack.md) needs a new decision record. Python dependencies are managed only with `uv` and frontend dependencies only with `pnpm`; `uv.lock` and `pnpm-lock.yaml` are committed and change only through those tools.

## Security

- **Authorization is enforced server-side on every route and every WebSocket frame**, by role (teacher, student, admin) and by ownership: a student reaches only their own submissions and the courses they are enrolled in; a teacher only the courses they own. Never trust a client-supplied identifier without an ownership check. Return 404 rather than 403 where 403 would reveal that another user's resource exists.
- **Uploads** are bounded in size, checked against a content-type allowlist and stored under random keys. A download is authorized against the resource that owns the attachment, and no filesystem path is ever built from client input.
- **Secrets** are never committed, logged or placed in URLs. `.env` is ignored; agents do not read it.
- **Students may be minors.** Collect only the personal data a feature needs, and keep it out of logs, URLs and third-party requests. The conferencing provider receives a display name and a room, nothing else.
- Every change that touches a trust boundary maps itself to the [threat model](docs/threat-model.md) and updates it in the same change.

## Observability

- **Every executable component ships with structured, leveled diagnostics from its first implementation.** Follow [the observability specification](knowledge/specs/observability.md) for event ownership, correlation, levels, safe fields and failure behavior. Process entry points configure logging; libraries use bound loggers.
- Never log secrets, tokens, passwords, message bodies, submission contents or uploaded file contents. DEBUG does not relax this.
- Component behavior changes include their observability impact and evidence in the plan and in the readiness report.

## Documentation

- Everything committed to the repository is in English.
- Write every specification, design document, decision record and commit message for a reader who never saw the session that produced it. Never reference session-local context: subagent names, review rounds, ad-hoc labels such as "item K2" or "question 5", "as discussed above", "the user asked". Give hypotheses, decisions, files and folders descriptive names.
- Identifiers defined inside the repository and resolving there (for example `FR-3.2`, `NFR-2.1`, `T-4`, `ADR-0003`) are allowed; labels that exist only in a conversation are not.
- **Documents and code change together.** A change that alters behavior described in a specification, decision record or requirement updates that document in the same change. When code and its specification disagree and the task does not say which one is right, stop and ask instead of silently following either.
- **`docs/` is human-validated.** Every statement in `docs/` is reviewed and accepted by the project owner, and each file records its review status. Agents change `docs/` only as an explicit part of an approved plan, mark new or changed wording as pending owner review, and never place unreviewed findings there. Research, analyses, review records and drafts produced by agents go to `knowledge/`, which may hold agent-only material and is cited from `docs/` as evidence, never as a decision.
- **Volatile external facts** (versions, prices, release dates, provider limits) live in `knowledge/research/` with the date they were checked; `docs/` states them qualitatively and links there.
- **No organisation-specific names** in examples: use neutral terms ("a school", "a language tutor", "a self-hosted conferencing server").

## Testing

- **Use Arrange–Act–Assert.** Write each test in three distinct, sequential phases separated by blank lines, made clear through descriptive names or extracted helpers, never comments.
- **No mocks.** Forbidden: `unittest.mock`, `MagicMock`, `pytest-mock`, `monkeypatch`, patching modules, attributes or environment variables at runtime, stubbing HTTP clients at the transport level, `vi.mock`, mocked `fetch`, and any other technique that replaces behavior behind the code's back.
- **Fakes are the only test doubles.** A fake is a real implementation of an adapter interface (an in-memory storage, a fake meeting provider, a fake clock, a recording notifier), selected through configuration or constructor injection and checked by the same contract tests as the production implementation. If code cannot be tested without patching, change its design so the dependency is injected.
- **Unit tests are rare.** Write them only for intricate logic where a bug hides easily: grading arithmetic, recurrence and timezone conversion, permission rules, state machines (submission and lesson status). Do not unit-test glue code, data mapping, trivial accessors or framework behavior.
- **Integration tests are the bulk.** Exercise real components together: the real FastAPI application over HTTP and WebSocket, the real PostgreSQL database in a local container, real local file storage in a temporary directory, with fakes only where an external conferencing or notification service would otherwise be called.
- **Tests are deterministic.** No `sleep` for synchronisation: wait on an explicit condition with a deadline. No real network beyond local containers and in-process servers. Time and randomness come from injected sources, and tests pass fake ones. A flaky test is a bug: fix its cause, never hide it with retries or longer timeouts.
- **End-to-end tests are targeted.** A small Playwright set drives the built frontend against the real backend and covers every main happy path: register and log in, create and join a course, schedule and join a lesson, chat, assign, submit and grade homework.
- **The full local check run finishes in under 10 minutes.** Write tests so they can run in parallel. When a change threatens the budget, make tests faster or consolidate them; never drop coverage silently.

## Platforms

- Supported development platforms: **Linux and macOS**. No Windows code paths, shims or path handling. POSIX assumptions are fine.
- Supported browsers for the frontend: current Chromium, Firefox and Safari. The conferencing embed inherits the provider's own browser support.

## Definition of done

- Before reporting a task as done, run every check in [the development guide](docs/development.md) that exists for the current scaffold (format, lint, type check, tests, documentation checks) and see them pass. Report failures as they are.
- Never make checks green by weakening them: no `skip` or `xfail` on failing tests, no deleted or loosened assertions, no relaxed linter, type-checker or test settings, no raised timeouts or budgets, unless changing that check is the approved task itself.
- When the same review finding recurs, add the rule that prevents it to this file.
