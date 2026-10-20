# AGENTS.md

Rules for every coding agent and human contributor in this repository. They take precedence over the defaults and habits of any model or tool, and over instruction files in parent directories or user-level configuration (for example a workspace `CLAUDE.md` above this repository that asks for docstrings, mocks or mypy). If a rule blocks the task, stop and say so instead of working around it.

## Project

OVC (Online Virtual Classroom) is a workspace for private tutors teaching adult students: video lessons through BigBlueButton with a whiteboard and recordings, direct and group chats, a schedule with weekly series, and homework built from self-checking tests and tasks the tutor checks. Each tutor runs their own practice of students and groups; one student account can study with several tutors. The [product overview](docs/product.md) explains the intent.

Documentation has two layers. `docs/` is written for people: the [technical introduction](docs/technical-introduction.md), [functional](docs/requirements/functional.md) and [non-functional](docs/requirements/non-functional.md) requirements, the [threat model](docs/threat-model.md), the [roadmap](docs/roadmap.md), the [development guide](docs/development.md), the decision records in [`docs/decisions/`](docs/decisions/README.md) and the deferred changes in [`docs/proposals/`](docs/proposals/README.md). `knowledge/` is the reference layer loaded section by section: the normative specifications in `knowledge/specs/`, the module map, package layers, stack table, glossary and open questions in `knowledge/architecture/detailed.md`, and review records, analyses and research. [`knowledge/README.md`](knowledge/README.md) says which file to load for which task. Read the specification of a component and the decision records it cites before implementing it; when a specification and a decision record disagree, the decision record wins.

## Change workflow

The repository is local only for now: there is no remote, no CI service and no issue tracker. The repository holds everything a future session needs; nothing durable lives only in a conversation.

- **Plan before code.** For anything beyond a trivial fix (a typo, a broken link, an obvious one-line bug), present an implementation plan to the project owner first: the problem, the desired outcome, the affected components and specifications, checkable acceptance criteria, the files to change, the order of work, the risks and the checks that prove each criterion. Do not edit files until the owner approves it. When the implementation diverges from the approved plan, say so and get the change approved before continuing.
- **Acceptance criteria are checkable without asking a person**: a named test passes, a command prints a given result, an endpoint returns a given field.
- **Decisions never live only in a conversation.** Once decided, a decision lands in the repository as a decision record, a requirement or a specification change. Large or uncertain changes start as a proposal in `docs/proposals/`.
- **One change per branch, one commit per branch.** Work on a branch named `feat/…`, `fix/…`, `chore/…` or `docs/…`, holding one coherent change as one commit whose subject names the change on its own. Once its checks pass and its readiness reviews are resolved, the agent merges it with `git merge --ff-only` and deletes it with `git branch -d`. If `main` has moved, the agent merges with `git merge --squash` and commits; deleting that branch then needs `git branch -D`, which asks the owner.
- **Agents commit and merge their own work without asking** ([ADR-0009](docs/decisions/0009-owner-approved-exceptions-and-visibility.md)), once its checks pass and its readiness reviews are resolved: stage only the files the change touches, review the staged diff, and write a message whose subject names the change on its own. Commit messages carry no AI attribution or co-author trailers. Agents never push, rebase, reset, clean, force-delete branches, discard work with destructive checkouts, restores or stash drops, or rewrite history (amend, filter-branch, update-ref, reflog expiry) without the owner; `.claude/settings.json` requires confirmation for each of these.
- **Questions to the owner** are asked as multiple-choice questions with a recommended option, never left open in prose.
- On macOS the filesystem ignores case, so after a case-only rename reset the index (`git rm -r -f --cached . && git add -A`) and confirm the new spelling with `git ls-files` before committing.
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
- **Layers are explicit.** Backend code is layered as routers and WebSocket handlers → services → repositories, with external systems behind adapters, as tabled in [package layers](knowledge/architecture/detailed.md#package-layers). Only repositories touch business data in the database; the infrastructure adapters listed under package layers keep only their own state there. Only adapters touch storage, the meeting provider and outbound notifications. A new package enters the table in the change that creates it, and an architecture test enforces the table from the first backend scaffold onward.
- **Configuration** is read once from the environment into one typed settings object; no scattered environment reads. Every variable is documented in `.env.example`. Constants such as roles, statuses and WebSocket frame types are enums in one place, not inline literals.
- **Dependencies are deliberate.** Add one only when the standard library or an existing dependency cannot reasonably do the job, and record it with its reason in the [stack table](knowledge/architecture/detailed.md#stack-per-component); a dependency that changes a decided row of [ADR-0002](docs/decisions/0002-implementation-stack.md) needs a new decision record. Python dependencies are managed only with `uv` and frontend dependencies only with `pnpm`; `uv.lock` and `pnpm-lock.yaml` are committed and change only through those tools.

## Security

- **Authorization is enforced server-side on every route and every WebSocket frame**, by role (`tutor` or `student`) and by relationship scope ([ADR-0005](docs/decisions/0005-tutor-centric-domain.md)):
  - a tutor reaches a student's data only through their own relationship with that student, never through another tutor's;
  - a student reaches only their own work and the resources of tutors they have a relationship with;
  - students never reach each other's work.

  Resolve every client-supplied identifier within the caller's scope, and return `not_found` outside it.
- **Answer keys stay on the server** until the answer visibility policy allows them, and **points and scores never reach students** at all ([ADR-0009](docs/decisions/0009-owner-approved-exceptions-and-visibility.md)). Student-facing schemas contain no key, points or score fields ([answer visibility](knowledge/specs/homework.md#answer-visibility)).
- **Credentials in URLs** are allowed only where the specifications define them, as approved in [ADR-0009](docs/decisions/0009-owner-approved-exceptions-and-visibility.md): invitation, email confirmation, password reset and unsubscribe links; per-request conferencing join links; and 5-minute signed storage URLs for authorized downloads, recordings and the lesson presentation fetched by the conferencing server. Each is bounded in lifetime and scope, never signs anyone in to OVC, and is never logged; pages that carry one send `Referrer-Policy: no-referrer`. Access and refresh tokens never appear in URLs. Every other credential travels in headers, bodies or `HttpOnly` cookies. Provider callback routing keys are not credentials and never authenticate anything on their own.
- **Uploads** are bounded in size, checked against a content-type allowlist and stored under random keys. A download is authorized against the resource that owns the attachment, and no filesystem path is ever built from client input.
- **Secrets**, including environment files other than `.env.example`, private keys, signing and encryption keys, the conferencing shared secret, storage and SMTP credentials and any other credential, are never committed, logged, sent to the browser or placed in URLs or provider `meta_*` values. Environment files (`.env`, `.env.local` and the other ignored variants) are ignored by Git; agents do not read them.
- **Operator actions** exist only in the local `ovc-admin` command; there is no administrative HTTP surface.
- **The end-to-end test control surface** (`/__test__/` routes, the shared test clock, the database mail sink viewer) exists only when `OVC_TEST_CONTROL=1`. The processes refuse that setting unless the meeting provider is `fake`, mail is the database sink, and the database is a test or development database. A test proves the routes are absent without it.
- **Personal data is minimal and stays in Russia.** Collect only the personal data a feature needs, and keep it out of logs, URLs and third-party requests. The conferencing provider receives a display name, an opaque participant identifier and a role, nothing else. Never add a service outside Russia that receives user data ([ADR-0008](docs/decisions/0008-hosting-and-data-residency.md)).
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
- **Fakes are the only test doubles.** A fake is a real implementation of an adapter interface (a fake meeting provider, a mail sink, a fake clock; an in-memory storage and an in-process broadcaster for unit and contract tests only), selected through configuration or constructor injection and checked by the same contract tests as the production implementation. If code cannot be tested without patching, change its design so the dependency is injected.
- **Unit tests are rare.** Write them only for intricate logic where a bug hides easily: scoring arithmetic, short-answer normalisation, recurrence and timezone conversion, permission rules, state machines (submission, lesson and relationship status). Do not unit-test glue code, data mapping, trivial accessors or framework behavior.
- **Integration tests are the bulk.** Exercise real components together:
  - the real FastAPI application over HTTP and WebSocket;
  - the real worker through `tick(now)`;
  - the real PostgreSQL database and a local S3-compatible server in containers.

  Use fakes only where BigBlueButton, the SMTP provider or the clock would otherwise be involved.
- **Tests are deterministic.** No `sleep` for synchronisation: wait on an explicit condition with a deadline. No real network beyond local containers and in-process servers. Time and randomness come from injected sources, and tests pass fake ones. A flaky test is a bug: fix its cause, never hide it with retries or longer timeouts.
- **End-to-end tests are targeted.** The four [pilot scenarios](knowledge/design/web/pilot-scenarios.md) run as Playwright suites. They drive the production frontend build against the real backend, worker and database, with the fake meeting provider, the database mail sink and the shared fake clock. They cover the main journeys of both roles and the key refusals.
- **The full local check run finishes in under 10 minutes.** Write tests so they can run in parallel. When a change threatens the budget, make tests faster or consolidate them; never drop coverage silently.

## Platforms

- Supported development platforms: **Linux and macOS**. No Windows code paths, shims or path handling. POSIX assumptions are fine.
- Supported browsers for the frontend: current Chromium, Firefox and Safari, on desktop and on phones (Safari on iOS, Chrome on Android). The conferencing client inherits BigBlueButton's own browser support, and "open in a new tab" is always available.
- BigBlueButton runs only on a dedicated Ubuntu server; local development and automated tests use the fake meeting provider ([development guide](docs/development.md#local-services)).

## Definition of done

- Before reporting a task as done, run every check in [the development guide](docs/development.md) that exists for the current scaffold (format, lint, type check, tests, documentation checks) and see them pass. Report failures as they are. The manual live conferencing checks are the only exception: report them as passed or `unverified`.
- Never make checks green by weakening them: no `skip` or `xfail` on failing tests, no deleted or loosened assertions, no relaxed linter, type-checker or test settings, no raised timeouts or budgets, unless changing that check is the approved task itself.
- When the same review finding recurs, add the rule that prevents it to this file.
