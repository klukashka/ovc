---
name: ovc-verify
description: Route OVC planning and readiness reviews to fresh subagents by affected component and review profile (correctness, security, browser, performance), following transitive impact, and collect check evidence. Use at task entry before an implementation plan and again on the complete diff before reporting a change as ready, for code, contracts, documentation and development configuration.
---

# OVC verification

Use the proposed scope at task entry and the actual diff before readiness. This skill owns review routing; [AGENTS.md](../../../AGENTS.md) owns the change workflow, [the knowledge index](../../../knowledge/README.md) owns contract lookup, and [the coverage checklist](../../../knowledge/reviews/ai-development-setup.md) maps component families, workloads and activation conditions. Load only the selected profiles. An assigned reviewer runs its profile directly and may subdivide that assignment without restarting the whole router.

## Determine scope

Identify the requested behavior, the acceptance criteria and the approved base (the last commit on `main`, or the empty tree before the first commit). Include commits since that base, staged and unstaged changes, relevant non-ignored untracked files and both paths of renames and deletions. Tests, fixtures, generated code, dependencies and configuration inherit the components they affect; specification-only changes can change the same contracts.

1. Map directly changed behavior to the component checklist and the canonical contracts. Its rows are navigation families: split them when parts own distinct contracts or resources (for example the REST routers and the WebSocket gateway, or the chat feature and the shared WebSocket client). Assign unmapped areas before continuing.
2. Follow changed contracts, imports and callers, schemas, persisted data, WebSocket frames, resource ownership and shared configuration to their consumers and providers. Repeat for newly affected components until no further effect remains. Trace semantic dependencies even without an import or a changed consumer file. Record a causal edge and its source or contract evidence for every transitive inclusion, and the unchanged boundary that stops propagation. Mere reachability does not require reviewing every component.
3. Assign an owner for each affected integration boundary. For example, a change to the API error body reaches the typed frontend API client and every feature that renders errors; a change to submission status reaches the homework service (scoring and review), the homework routes, the notification frames and the homework UI. Identify the affected behavior on each side rather than assigning one giant context.
4. Produce a matrix of component, direct or transitive reason, boundary owner, applicable profiles, checks and activation status. Select profiles additively for every component. Dispatch each selected pair separately and reconcile cross-component findings centrally. Recompute the matrix after fixes.

Use the stricter of the approved base policy and the proposed policy until the owner explicitly approves a reduction. Deleting a check, test or routing instruction does not deactivate a requirement. Changes to verification policy, check budgets or agent configuration require correctness and security review plus explicit owner review.

## Select independent reviews

| Profile | Before the implementation plan | Before readiness |
|---|---|---|
| [Correctness](references/correctness.md) | Contract or nontrivial design changes; a typo needs no design review | Every affected component, scaled to the change |
| [Security](references/security.md) | New or changed trust boundaries, including specification-only changes | Changes to authentication, authorization, uploads and downloads, WebSocket frames, the meeting provider, secrets, outbound network calls, the end-to-end test control surface, dependencies or agent configuration; every component records applicability |
| [Browser](references/browser.md) | New or changed frontend interactions, including specification-only changes | Every frontend or backend change once a runnable frontend exists: common smoke plus affected journeys |
| [Performance](references/performance.md) | Performance-sensitive architecture, resource ownership or contract changes | Every frontend or backend change and other resource-lifecycle changes: relevant measurements and leak checks, in a separate review |

Selection is additive: an authorization change that alters frontend behavior also requires browser and separate performance checks. For an unimplemented capability, record its checks as `pending implementation` with the roadmap slice that activates them; reserve `not applicable` for checks unrelated to the change. Existing executable requirements remain mandatory; activation is incremental only for capabilities not yet implemented.

Apply [the observability specification](../../../knowledge/specs/observability.md) to every changed executable component. Correctness checks its logging evidence. Changes to logging collection, redaction, correlation or sinks additionally require security and performance review.

## Dispatch and collect evidence

Use one fresh subagent per selected component/profile pair, not one subagent covering several profiles or all component families. In Claude Code, launch a non-fork `general-purpose` subagent through the Agent tool, never a conversation fork; send independent assignments in one message so they run concurrently. A concrete failure to dispatch leaves that assignment blocked; self-review is not independent review.

Give each subagent its component and profile, the repository path and the trusted instruction and profile paths, the exact base and working-tree scope, the causal edges and boundary ownership, the acceptance criteria, contract pointers and the evidence to inspect. Require full reads of `AGENTS.md` and the assigned profile, then the affected specifications and decision records. Give access to the complete diff for boundary checks without passing the implementation conversation, earlier review conclusions or a desired verdict.

Reviewers may delegate narrower parts of their assigned scope in fresh contexts with the same policy, scope and read-only restriction, and return discovered scope expansion to the coordinator. No reviewer edits source, commits or publishes. Local fixtures, containers and measurements are allowed when the assignment calls for them. Isolate databases, ports, browser sessions and output paths between concurrent reviewers; run performance measurements serially on an otherwise idle machine.

Each review returns its component/profile, the scope and working-tree state reviewed, files, contracts and boundaries examined, descendant assignments and results when used, findings with evidence and severity, missing coverage and a verdict. Each executable check records the exact command, exit result, tested working-tree state and environment. Use `passed`, `failed`, `blocked`, `pending implementation`, `unverified` or `not applicable`, and explain every non-pass. A design review pass does not imply runtime checks passed.

The coordinator resolves findings, reruns affected reviews and checks after changes, and maps the evidence to the acceptance criteria in the readiness report to the owner. A changed review input invalidates the affected conclusions. A report or checkbox does not replace check output.
