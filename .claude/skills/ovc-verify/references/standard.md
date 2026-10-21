# Standard review checklist

Act as the one independent reviewer of a standard-tier change ([ADR-0011](../../../../docs/decisions/0011-tiered-verification.md)). Read `AGENTS.md`, `docs/threat-model.md`, and the specifications and decision records the change touches, through `knowledge/README.md`. The coordinator tells you the tier, whether it classified the change as meeting the security trigger, and whether it dispatched a security reviewer. Review the complete diff against its base, including untracked files that belong to the change. Do not edit, commit or publish. The Security section is always mandatory. Skip another section only when it does not apply, and say so in one line.

## Correctness

- The change does what its plan and acceptance criteria say, and each criterion is met with evidence.
- Contracts stay consistent: specifications, requirements, decision records, API schemas, frames and the code that implements them agree; documents changed with behavior.
- Edge cases and failure paths: empty, boundary and invalid input, concurrency and retries, partial failure, idempotency, time zones.
- Repository rules from `AGENTS.md`: no comments or tool directives in source, precise types (no `any`, `interface{}`, `unsafe` or `reflect`), layers and adapters, fail fast, no mocks, Arrange–Act–Assert, deterministic tests, observability events and safe fields.
- Tests cover the new behavior at the level `AGENTS.md` asks for, and acceptance criteria are checkable without asking a person.

## Security

- Every route and frame stays relationship-scoped, returns `not_found` outside scope, and never sends points, scores or unrevealed answer keys to students.
- No secret, token or personal data reaches logs, URLs, the browser or third parties beyond what the specifications allow.
- Untrusted content is rendered safely; the end-to-end test control surface stays behind `OVC_TEST_CONTROL=1` and its refusals.
- A change that touches a trust boundary maps itself to threat identifiers and updates the threat model in the same change.
- Decide independently whether the diff meets the security trigger defined in `AGENTS.md`. If it does and no security reviewer was dispatched, or the change was classified as trivial, report a blocker.

## Browser

- Pages follow `knowledge/design/web` (tokens, states, navigation, pages): status words, keyboard access, focus, labels, contrast, target sizes, Russian and English strings by key.
- Once a frontend exists, the common smoke and the affected pilot steps from the development guide pass.

## Performance

- Flag risks only: query counts that grow with data, unbounded lists or payloads, work in request paths that belongs in the worker, leaked connections, timers or subscriptions. Measurements belong to the slice review.

## Report

Return the scope and working-tree state reviewed, the files and contracts examined, findings with file and line evidence and severity (blocker, major, minor), the commands you ran with exit results, missing coverage, and a verdict: passed or failed, with every non-pass explained.
