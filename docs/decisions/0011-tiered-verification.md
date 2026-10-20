# ADR-0011: Tiered verification

- Status: accepted, 2026-10-20
- Reviewed: decided by the project owner on 2026-10-20, including the explicit approval this policy reduction needs; the after-fix rule amended on 2026-10-21 with the owner's approval; wording pending line-by-line owner review
- Deciders: project owner
- Related: [AGENTS.md](../../AGENTS.md#development-verification), [ovc-verify](../../.claude/skills/ovc-verify/SKILL.md), [roadmap](../roadmap.md), [ADR-0009](0009-owner-approved-exceptions-and-visibility.md)

## Context

Every nontrivial change used to need a fresh independent reviewer for each affected component and each applicable profile (correctness, security, browser, performance), both on the plan and before readiness, and again after every fix. A documentation-only change to the visual language took five review rounds and about a dozen reviewer runs. Most findings were minor wording issues, and the cost would grow with every code change in the coming slices. The owner wants routine work reviewed lightly and the full matrix kept for moments when a whole roadmap slice is complete.

## Decision

Verification has three tiers. The coordinating agent picks the tier and decides whether the change meets the security trigger, states both in the plan, rechecks them against the final diff, and picks the higher tier and assumes the trigger is met when in doubt.

- **Trivial** (a typo, a broken link, an obvious one-line bug): the checks from the development guide, no independent review. A change that meets the security trigger is never trivial.
- **Standard** (every other change inside a slice): the checks, plus one fresh independent reviewer who covers the complete diff with the short [standard checklist](../../.claude/skills/ovc-verify/references/standard.md), applying every relevant profile in one context.
  - A change that meets the **security trigger** also gets a separate fresh reviewer with the full [security profile](../../.claude/skills/ovc-verify/references/security.md). The trigger, defined once in [AGENTS.md](../../AGENTS.md#development-verification), covers any change, including a specification-only one, that touches a trust boundary or maps to a threat in the [threat model](../threat-model.md); agent configuration and verification policy always meet it.
  - After fixes, the checks rerun. A fresh reviewer, scoped to the fixes, runs only when a blocker or major finding was fixed, a contract changed, or the fixes themselves meet the security trigger, and uses the security profile when such a finding concerned security, from either reviewer, or the fixes meet the trigger; other fixes to minor findings need only the checks. Severity is the reviewer's; the coordinator never downgrades it. (Amended 2026-10-21: re-reviewing every minor security fix produced an endless chain of new minor findings.)
- **Slice** (when every planned change of a roadmap slice has merged, before the slice is closed): the full matrix of one fresh reviewer per affected component and profile, including performance measurements, over the cumulative diff since the previous slice closed. Findings are fixed in follow-up changes, and affected pairs are re-reviewed as before.

The plan for a whole slice gets one standard reviewer before work starts; a slice or task plan that meets the security trigger also gets a security-profile reviewer before approval; other task plans are approved by the owner without a planning review. The commit that closes a slice is recorded under that slice in the roadmap and is the base of the next slice review.

## Alternatives considered

- **Keep the full matrix for every change:** thorough, but its cost dominated small changes and mostly produced minor findings.
- **Self-review only for standard changes:** cheapest, but loses the independent look that catches what the implementing context is blind to.
- **Checks only:** documentation and code checks cannot judge contract consistency, threat coverage or design quality.
- **Full matrix whenever a new package or architectural layer appears:** harder to decide consistently than the slice boundary, which the roadmap already defines.

## Consequences

- Routine changes need one reviewer run (two when the security trigger is met) instead of one per component and profile.
- Problems that span several changes in a slice may surface only at the slice review; slices stay small enough for that review to be practical. Once OVC is deployed, a slice is not deployed before its slice review is resolved.
- Performance measurements run at slice reviews; standard reviewers only flag performance risks.
- `AGENTS.md`, the ovc-verify skill, the development guide, the roadmap and the verification policy record describe the tiers.

## Where it is applied

- [AGENTS.md, development verification](../../AGENTS.md#development-verification)
- [ovc-verify](../../.claude/skills/ovc-verify/SKILL.md)
- [Development guide, local review](../development.md#local-review)
- [Roadmap](../roadmap.md)
- [AI development verification](../../knowledge/reviews/ai-development-setup.md)
