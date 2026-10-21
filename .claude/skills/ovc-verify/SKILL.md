---
name: ovc-verify
description: Choose the OVC verification tier (trivial, standard, slice) and whether the security trigger is met, then run it — one fresh reviewer with the standard checklist for routine changes, a security-profile reviewer whenever the trigger is met, and the full component-by-profile matrix of fresh subagents when a roadmap slice is complete — and collect check evidence. Use at task entry for the plan, and again on the complete diff before reporting a change as ready, for code, contracts, documentation and development configuration.
---

# OVC verification

Use the proposed scope at task entry and the actual diff before readiness. This skill owns review routing and the tiers decided in [ADR-0011](../../../docs/decisions/0011-tiered-verification.md); [AGENTS.md](../../../AGENTS.md) owns the change workflow, [the knowledge index](../../../knowledge/README.md) owns contract lookup, and [the coverage checklist](../../../knowledge/reviews/ai-development-setup.md) maps component families, workloads and activation conditions. Load only the selected profiles. An assigned reviewer runs its profile directly and may subdivide that assignment without restarting the whole router.

## Choose the tier

State the tier and whether the change meets the [security trigger](../../../AGENTS.md#development-verification), with reasons, in the plan, and recheck both against the final diff before readiness. When in doubt, pick the higher tier and assume the trigger is met. A change that meets the trigger is never trivial.

| Tier | When | Reviews before readiness | After fixes |
|---|---|---|---|
| Trivial | A typo, a broken link, an obvious one-line bug that does not meet the security trigger | None; run the checks | Rerun the checks |
| Standard | Every other change inside a slice | One fresh reviewer over the complete diff with the [standard checklist](references/standard.md); plus one fresh reviewer with the full [security profile](references/security.md) when the change meets the security trigger | Rerun the checks; a fresh reviewer scoped to the fixes only when a blocker or major finding was fixed, a contract changed, or the fixes meet the security trigger, with the security profile when such a finding concerned security or the fixes meet the trigger; other minor fixes need only the checks; when in doubt whether the fixes meet the trigger, assume they do, and say in the readiness report for each fixed security finding whether its fix meets the trigger and why; severity is the reviewer's and is never downgraded |
| Slice | Every planned change of a roadmap slice has merged and the slice is about to close | The full matrix below over the cumulative diff since the commit that closed the previous slice | Fix in follow-up changes; re-review the affected component/profile pairs |

The security trigger and its examples are defined once, in [AGENTS.md](../../../AGENTS.md#development-verification); link to it instead of restating it.

Planning reviews: a slice plan gets one standard reviewer before work starts, and a slice or task plan that meets the security trigger also gets a security-profile reviewer before approval, as [the security profile](references/security.md) requires; other task plans are approved by the owner without a planning review. Standard reviewers flag performance risks; measurements run only at the slice tier.

### Standard dispatch

Launch a non-fork `general-purpose` subagent through the Agent tool, in the background when other work can continue. Give it the repository path, the instruction to read `AGENTS.md` and [references/standard.md](references/standard.md) in full, the base commit and working-tree scope, the plan's acceptance criteria, the contracts touched, the tier, whether the change was classified as meeting the security trigger, and whether a security reviewer was dispatched. Do not pass the implementation conversation or a desired verdict. Dispatch the security reviewer, when required, in the same message with the security profile and the same scope. The rest of this page, except where it says otherwise, describes the slice tier; evidence and status rules apply to every tier.

## Determine scope (slice tier)

Identify the requested behavior, the acceptance criteria and the approved base (the last commit on `main`, or the empty tree before the first commit). Include commits since that base, staged and unstaged changes, relevant non-ignored untracked files and both paths of renames and deletions. Tests, fixtures, generated code, dependencies and configuration inherit the components they affect; specification-only changes can change the same contracts.

1. Map directly changed behavior to the component checklist and the canonical contracts. Its rows are navigation families: split them when parts own distinct contracts or resources (for example the REST routers and the WebSocket gateway, or the chat feature and the shared WebSocket client). Assign unmapped areas before continuing.
2. Follow changed contracts, imports and callers, schemas, persisted data, WebSocket frames, resource ownership and shared configuration to their consumers and providers. Repeat for newly affected components until no further effect remains. Trace semantic dependencies even without an import or a changed consumer file. Record a causal edge and its source or contract evidence for every transitive inclusion, and the unchanged boundary that stops propagation. Mere reachability does not require reviewing every component.
3. Assign an owner for each affected integration boundary. For example, a change to the API error body reaches the typed frontend API client and every feature that renders errors; a change to submission status reaches the homework service (scoring and review), the homework routes, the notification frames and the homework UI. Identify the affected behavior on each side rather than assigning one giant context.
4. Produce a matrix of component, direct or transitive reason, boundary owner, applicable profiles, checks and activation status. Select profiles additively for every component. Dispatch each selected pair separately and reconcile cross-component findings centrally. Recompute the matrix after fixes.

Use the stricter of the approved base policy and the proposed policy until the owner explicitly approves a reduction. Deleting a check, test or routing instruction does not deactivate a requirement. Changes to verification policy, check budgets or agent configuration meet the security trigger, so they are never trivial and always get a standard and a security reviewer, plus explicit owner review.

## Select independent reviews (slice tier)

| Profile | Slice plan | Slice review |
|---|---|---|
| [Correctness](references/correctness.md) | Contract or nontrivial design changes; a typo needs no design review | Every affected component, scaled to the change |
| [Security](references/security.md) | A separate security-profile reviewer when the plan meets the [security trigger](../../../AGENTS.md#development-verification) | Every change in the slice that meets the [security trigger](../../../AGENTS.md#development-verification); every component records applicability |
| [Browser](references/browser.md) | New or changed frontend interactions, including specification-only changes | Every frontend or backend change once a runnable frontend exists: common smoke plus affected journeys |
| [Performance](references/performance.md) | Performance-sensitive architecture, resource ownership or contract changes | Every frontend or backend change and other resource-lifecycle changes: relevant measurements and leak checks, in a separate review |

Selection is additive: an authorization change that alters frontend behavior also requires browser and separate performance checks. For an unimplemented capability, record its checks as `pending implementation` with the roadmap slice that activates them; reserve `not applicable` for checks unrelated to the change. Existing executable requirements remain mandatory; activation is incremental only for capabilities not yet implemented.

Apply [the observability specification](../../../knowledge/specs/observability.md) to every changed executable component. Correctness checks its logging evidence. Changes to logging collection, redaction, correlation or sinks additionally require security and performance review.

## Dispatch and collect evidence

At the slice tier, use one fresh subagent per selected component/profile pair, not one subagent covering several profiles or all component families. In Claude Code, launch a non-fork `general-purpose` subagent through the Agent tool, never a conversation fork; send independent assignments in one message so they run concurrently. A concrete failure to dispatch leaves that assignment blocked; self-review is not independent review.

Give each subagent its component and profile, the repository path and the trusted instruction and profile paths, the exact base and working-tree scope, the causal edges and boundary ownership, the acceptance criteria, contract pointers and the evidence to inspect. Require full reads of `AGENTS.md` and the assigned profile, then the affected specifications and decision records. Give access to the complete diff for boundary checks without passing the implementation conversation, earlier review conclusions or a desired verdict.

Reviewers may delegate narrower parts of their assigned scope in fresh contexts with the same policy, scope and read-only restriction, and return discovered scope expansion to the coordinator. No reviewer edits source, commits or publishes. Local fixtures, containers and measurements are allowed when the assignment calls for them. Isolate databases, ports, browser sessions and output paths between concurrent reviewers; run performance measurements serially on an otherwise idle machine.

Each review returns its component/profile, the scope and working-tree state reviewed, files, contracts and boundaries examined, descendant assignments and results when used, findings with evidence and severity, missing coverage and a verdict. Each executable check records the exact command, exit result, tested working-tree state and environment. Use `passed`, `failed`, `blocked`, `pending implementation`, `unverified` or `not applicable`, and explain every non-pass. A design review pass does not imply runtime checks passed.

The coordinator resolves findings, reruns affected checks and, as the tier requires, affected reviews after changes, and maps the evidence to the acceptance criteria in the readiness report to the owner. At the slice tier, a changed review input invalidates the affected conclusions; at the standard tier, the after-fix rule in the tier table decides. A report or checkbox does not replace check output.
