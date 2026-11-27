# Task plan: the redesign changes join the interface rework stage through a supplementary slice review

Status: revision 5 (2026-11-27), approved by the owner on 2026-11-27 (exact wording included, pending owner review in `docs/`). Branch `chore/ui-redesign-review-scope`. Revisions 2 to 5 applied the standard and security-profile planning reviews and three scoped re-reviews (summarised at the end). Link targets inside the quoted wording that are relative to other files are omitted here.

## Problem

The owner approved the [visual redesign plan](redesign.md) on 2026-11-27 (branch `docs/redesign-plan`, commit subject "Plan the visual redesign from the owner-approved mockups", not on `main` when this plan was written). It places the changes R0 to R11 inside the interface rework stage (`UI`), before the stage closes and before the finance stage (`FIN`). The stage's slice review has already run (record `knowledge/reviews/slice-10-ui-slice-review.md` on the unmerged branch `docs/ui-rework-slice-review`); that record says its closing commit follows the review's follow-ups, so the stage has not closed.

Under the sub-slice rule a commit in no list joins the scope of the earliest sub-slice whose slice review has not started, which is `FIN`. On 2026-11-27 the owner chose option A of the redesign plan's section 10 instead: a supplementary slice review over the redesign changes only runs before `UI` closes. The redesign plan's sections 7 and 10 require one change before any R-change that (a) revises `knowledge/plans/ui-rework.md` to list every R-change, the redesign plan and this revision, with the commit-mapping rule, and (b) amends the verification policy accordingly.

## Where the rule is written (checked 2026-11-27 on `main` `60edbaa`, rechecked at `512765d`)

The redesign plan's sections 7 and 10 name `AGENTS.md` and ADR-0018 as the files option A amends. The sub-slice mapping rule is written in full in `AGENTS.md` (Development verification, Pick a tier, Slice), in [ADR-0011](../../docs/decisions/0011-tiered-verification.md) (Decision, Slice bullet), in `.claude/skills/ovc-verify/SKILL.md` (Determine scope (slice tier)) and in [the development guide](../../docs/development.md#local-review), summarised in T-41 of [the threat model](../../docs/threat-model.md), and restated in `knowledge/plans/ui-rework.md` (section 3, the stage rule under D1) and `knowledge/plans/slice-10.md` (D1, D49). ADR-0018 holds no mapping rule; its section "Gates of the interface rework stage" records gates only. A decision record wins over `AGENTS.md`, and T-41 requires policy changes to be decided in ADR-0011, so an exception written only in `AGENTS.md` and ADR-0018 would leave ADR-0011 stating the default rule. The owner chose on 2026-11-27 to write the exception in full in `AGENTS.md` and ADR-0011, with pointers elsewhere, and to leave ADR-0018 unchanged.

## Desired outcome

- `knowledge/plans/ui-rework.md` revision 8 lists in section 7 every redesign row as its own table row and states the mapping rule below; its section 3 stage rule points to it; it records that ADR-0018 is not amended (departure from the redesign plan's wording, by the owner's decision of 2026-11-27 recorded in this plan).
- The verification policy states the exception in `AGENTS.md` and ADR-0011 in full, and points to it from ovc-verify, the development guide, T-41 and `slice-10.md` D49.
- Nothing else in the slice tier changes: the sub-slice order, the 10.5 cumulative rules, the known-limitations file, the closing-record and follow-up rule, ADR-0018's temporary 720 s allowance and its end conditions, and FIN's scope for every other commit.

## Tier, security trigger and threat mapping

Standard tier. The security trigger is met: the change edits agent configuration and verification policy (`AGENTS.md`, `.claude/skills/ovc-verify/SKILL.md`, ADR-0011, T-41) and changes which slice review covers a set of commits. Threats: **T-41 affected and updated** (policy weakened so that unsafe changes merge without the review they need); T-39, T-18 and T-19 not affected (no dependency, tool, hook, permission, outbound call or secret changes; the redesign plan keeps their controls for its later rows); no runtime trust boundary changes. Reviews: one fresh standard and one fresh security-profile planning reviewer before approval (done for revision 1), then one standard and one security-profile reviewer over the complete diff.

## Mapping rule (the content of the exception)

1. **Redesign rows.** The rows of `ui-rework.md` section 7 under "Redesign rows": RP (the redesign plan), RM (this change), RS (each reference study an R-change cites, by the owner's answer on reference studies of 2026-11-27) and R0 to R11 with their splits. RX, the supplementary review's record, is not a redesign row.
2. **Admission.** The redesign plan's commit (RP) is the one exception: it merges before RM, and RM's revision names it by its merged commit hash. Every other commit is a redesign row's commit only if that row is on `main` before the commit merges or is added by that same commit; the generic RS row admits no commit, and each study maps only to its own `RS-<screen>` row; and the row names the `ui-rework.md` revision that added it, whose changelog line records the owner's approval date. A row is added, split or removed only by an owner-approved revision of `ui-rework.md`, normally by the owner-approved change of the R-change or of the study itself; a row is removed only before its commit merges. A commit that does not meet this keeps the default rule.
3. **The list closes when the supplementary review starts.** No redesign row is added, split or removed after the supplementary review started. A redesign row's commit that merges after it started joins the scope of the earliest sub-slice whose slice review has not started.
4. **The supplementary review** follows every rule of the slice review in `AGENTS.md`, over the redesign rows' commits only; at least these reviewers run: the frontend (correctness, browser and performance, and the rendered-screen review over every screen the rows changed, directly or through shared tokens, components or the frame); security across all of them, subdivided by trust-boundary family, with agent configuration as its own family because RM changes it, reporting each affected threat as examined, not affected or unverified; development tooling with agent configuration, which always runs because RM changes `AGENTS.md` and `.claude/` and which re-verifies every `rerun` and `after-fix` row of the redesign rows; and the backend when directly or transitively affected. Each layer reviewer re-verifies its layer's entries of the redesign rows in `knowledge/reviews/slice-10-known-limitations.md` (sub-slice value `UI`), and the review is resolved only when each is fixed, accepted by the owner or carried into a named later sub-slice or slice.
5. **Its record** (`knowledge/reviews/slice-10-ui-redesign-slice-review.md`, row RX) states the commit on `main` at which the review started, maps each redesign commit to exactly one row and shows each is an ancestor of that start commit and meets the admission rule; names each integration boundary with commits of FIN's list that touch the same screens or contracts (the first review's follow-ups among them) and who reviews it; states that it supplements the first review without claiming that review covered these commits; and, as the stage's closing record, maps every commit on `main` after those the first review's record mapped, up to its start commit, to exactly one sub-slice's list, naming each commit that goes to FIN's list, whose record then lists those commits in its scope.
6. **Closing.** RX is `UI`'s closing record and replaces the first record's statement (that branch replaces it before it merges, see Dependencies) that its closing commit follows that review's follow-ups. The first review's record (row R, no longer called a closing record) is a review record like a closing record: under the existing rule, its commits, RX and the follow-ups of both `UI` reviews go to FIN's list, and RX lists them as such. 10.5's coverage check is unchanged.
7. **Order.** Because a stage starts when the previous stage's planned changes have merged (slice 10 plan, D1), the redesign rows are planned changes of `UI`, so FIN starts after they merge, as the redesign plan says. A change that merges meanwhile and is in no list (the benchmark audit `512765d`, an input to the redesign; the notifications change that R1a's counts and R4 wait for; the gate speed-up) joins FIN's scope by the default rule, and RX names its integration boundary with the redesign rows. The exception reorders no sub-slice and does not extend or end ADR-0018's temporary 720 s allowance.

## Exact wording

Link targets are relative to each file.

### 1. `AGENTS.md`, Development verification, Pick a tier, Slice

After "(decided by the owner on 2026-11-16 with the slice 10 plan's D1 and K17, pending owner review)." append:

> The interface rework stage's first slice review ran before the visual redesign changes merged, so the stage's list also holds the redesign rows of the [interface rework plan](ui-rework.md#7-changes-in-order) (the [visual redesign plan](redesign.md), named there by its commit, the revision that lists the rows, each reference study they cite under its own row, and R0 to R11 with their splits), which a supplementary slice review covers as set out below, although they merge after the first review started; apart from the redesign plan's commit, a commit counts as a redesign row's only if its row, added by an owner-approved revision of the interface rework plan, is on `main` before the commit merges or is added by that commit, and no row is added, split or removed once the supplementary slice review has started, after which a redesign row's commit joins the earliest sub-slice whose slice review has not started; before the stage closes, the supplementary slice review covers the redesign rows' commits only and follows every rule of the slice tier in this bullet and of the Slice review bullet below, with at least the frontend reviewer, whose rendered-screen review covers every screen the redesign commits changed directly or through shared tokens, components or the frame, the security reviewer across all of them, the development tooling reviewer, and the backend reviewer when directly or transitively affected; its record states the commit at which it started, maps each redesign commit to its row and shows that it is an ancestor of that start commit and meets the admission rule above, names their integration boundaries with the finance stage's commits and who reviews each, supplements the first review without claiming it covered them, and, as the stage's closing record, maps every commit on `main` after those the first review's record mapped, up to that start commit, to exactly one sub-slice's list; every other commit in no list that merges after the first review started joins the earliest sub-slice whose slice review has not started, the finance stage until its review starts; the first review's record, the supplementary review's record and the follow-ups of both reviews go to the finance stage's list as a closing record's do, and the finance stage's record lists the commits the supplementary review's record sent to it and maps every commit on `main` after the supplementary review's start commit ([ADR-0011](../../docs/decisions/0011-tiered-verification.md), decided by the owner on 2026-11-27, pending owner review).

### 2. `docs/decisions/0011-tiered-verification.md` (pending owner review)

- Decision, Slice bullet: after "(decided by the owner on 2026-11-16 with the slice 10 plan's D1 and K17, pending owner review)." and before " (Amended 2026-10-22:", insert the sentence of edit 1 with link targets relative to `docs/decisions/`, "of the slice tier in this bullet and of the Slice review bullet below" replaced by "of this bullet", and the closing parenthesis "(decided by the owner on 2026-11-27, pending owner review)".
- Reviewed line: append "; amended 2026-11-27 with the supplementary slice review of the visual redesign changes inside the interface rework stage, decided by the project owner that day, wording pending owner review".

### 3. `.claude/skills/ovc-verify/SKILL.md`, Determine scope (slice tier)

After "(decided by the owner on 2026-11-16 with the slice 10 plan's D1 and K17, pending owner review)." append: "The interface rework stage's list also holds the redesign rows of the interface rework plan, which a supplementary slice review under every slice review rule covers before the stage closes, as `AGENTS.md` and ADR-0011 state; every other commit in no list after the stage's first review started joins the earliest sub-slice whose slice review has not started, the finance stage until its review starts."

### 4. `docs/development.md`, Local review (pending owner review)

After the same anchor in its copy of the rule, append the sentence of edit 3 with link targets relative to `docs/` and " (pending owner review)". Status line: append "Amended 2026-11-27 with the supplementary slice review of the visual redesign changes, pending owner review."

### 5. `docs/threat-model.md`, T-41 (pending owner review)

After "a commit in no list joining the earliest sub-slice whose review has not started" and before ", or, after 10.5's review started" insert " (the visual redesign changes listed in the interface rework plan join that stage's list through a supplementary slice review over them only, before the stage closes, under every slice review rule, with the frontend, security and tooling reviewers always and the backend reviewer when affected; the list closes when that review starts; added 2026-11-27, pending owner review)", so the existing sentence continues unchanged after it. Status line: append "T-41 amended 2026-11-27 with the supplementary slice review of the visual redesign changes, pending owner review."

### 6. `knowledge/plans/slice-10.md`, D49

Append to D49: "On 2026-11-27 the owner decided that the stage's list also holds the visual redesign rows of the interface rework plan, reviewed by a supplementary slice review before the stage closes (ADR-0011)." Changelog line for the same date.

### 7. `knowledge/plans/ui-rework.md`, revision 8

- Status line: revision 8 (2026-11-27) adds the redesign rows and the supplementary review by the owner's decision of 2026-11-27 (redesign plan section 10, option A); the labels RP, RM, RS and RX are defined here and R0 to R11 in the redesign plan; row R stays the stage's first slice review and is not one of R0 to R11.
- Row R: its dependency cell becomes "U0 to P2 and L0 to L4 merged", and its content cell becomes "The stage's first slice review by layer, as `AGENTS.md` defines it for a sub-slice; its record goes to the finance stage's list, and RX is the closing record".
- Section 3, the stage rule under D1: after "a commit in no list joins the earliest named stage whose slice review has not started" insert ", except the redesign rows of section 7, which stay in this stage's list under the rule stated there".
- Section 7, after row R: sub-heading "Redesign rows (owner's decision of 2026-11-27)", the mapping rule above (clauses 1 to 7), and one table row per label, each with its branch from redesign plan section 7, a short content cell linking there, its trigger and its dependency, and an "Added by" column naming revision 8:

  RP `docs/redesign-plan` (named by its merged commit hash, admitted by RM); RM `chore/ui-redesign-review-scope`; RS `docs/redesign-reference-<screen>` (a placeholder that admits no commit; each study gets its own row, `RS-<screen>`, added by the study's own owner-approved change or earlier); R0 `feat/redesign-themes`; R1a `feat/redesign-sidebar-rail`; R1b `feat/redesign-panel-resizing`; R2a `feat/redesign-shared-actions`; R2b `feat/redesign-feature-actions`; R3a `feat/redesign-message-list`; R3b `feat/redesign-message-actions`; R4a `feat/redesign-today`; R4b `feat/redesign-student-home`; R5 `feat/redesign-schedule` (replaced by R5a and R5b by a revision if its task plan splits it); R6a `feat/redesign-students`; R6b `feat/redesign-student-card`; R7a `feat/redesign-homework-library`; R7b `feat/redesign-homework-builder`; R8 `feat/redesign-settings`; R9 `feat/redesign-lesson-lobby`; R10 `feat/redesign-welcome`; R11 `docs/redesign-bigbluebutton-theming`.
  After the table, row RX `docs/ui-redesign-slice-review`, outside the redesign rows: the supplementary slice review and the stage's closing record, mapping every commit since the first review's mapping, its own commit going to FIN's list, trigger met, depends on every redesign row merged and on the first review's record merged.
- A note that, by the owner's decision of 2026-11-27 recorded in `knowledge/plans/ui-redesign-review-scope.md`, ADR-0018 is not amended, because it holds no mapping rule, although the redesign plan's sections 7 and 10 name it.
- Section 8: an RM criterion pointing to this plan's criteria.
- Changelog line for revision 8.

### Files

`AGENTS.md`, `docs/decisions/0011-tiered-verification.md`, `.claude/skills/ovc-verify/SKILL.md`, `docs/development.md`, `docs/threat-model.md`, `knowledge/plans/slice-10.md`, `knowledge/plans/ui-rework.md`, `knowledge/plans/ui-redesign-review-scope.md`. ADR-0018 and `knowledge/plans/redesign.md` unchanged.

## Dependencies and order

1. `docs/redesign-plan` (RP) merges first, because the new links target `knowledge/plans/redesign.md` and `tools/docs/check-docs.sh` refuses broken links. If it has not merged when implementation starts, this branch waits and is moved onto the new `main` with `git switch -c` and `git merge --squash`.
2. `docs/ui-rework-slice-review` may merge before or after this change; no edit links to it. Before it merges, its status sentence "This record's closing commit follows those follow-ups" is replaced by one saying that its record goes to FIN's list and that the supplementary review's record closes the stage; once both have merged, `git cat-file -e main:knowledge/reviews/slice-10-ui-slice-review.md && git show main:knowledge/reviews/slice-10-ui-slice-review.md | grep -c 'closing commit follows those follow-ups'` prints 0. The coordinator owns that edit on that branch.
3. This change merges before any R-change or reference study.

## Acceptance criteria

1. `grep -c 'supplementary slice review' AGENTS.md docs/decisions/0011-tiered-verification.md .claude/skills/ovc-verify/SKILL.md docs/development.md knowledge/plans/slice-10.md knowledge/plans/ui-rework.md` prints at least 1 per file, and `grep '^| T-41 ' docs/threat-model.md | grep -c 'supplementary slice review'` prints 1.
2. `grep -c 'no row is added, split or removed once the supplementary slice review has started' AGENTS.md docs/decisions/0011-tiered-verification.md` prints 1 per file, and `grep -c 'follows every rule of the slice tier in this bullet and of the Slice review bullet below' AGENTS.md` prints 1, `grep -c 'follows every rule of this bullet' docs/decisions/0011-tiered-verification.md` prints 1, and each of ``'maps every commit on `main` after those the first review'``, `'after the supplementary review.s start commit'` and `'meets the admission rule above'` gives `grep -c` 1 in each of `AGENTS.md` and ADR-0011.
3. `grep -n 'the finance stage until its review starts' AGENTS.md docs/decisions/0011-tiered-verification.md .claude/skills/ovc-verify/SKILL.md docs/development.md` finds each file.
4. For each label in R0 R1a R1b R2a R2b R3a R3b R4a R4b R5 R6a R6b R7a R7b R8 R9 R10 R11 RP RM RS RX, `grep -c "^| $label |" knowledge/plans/ui-rework.md` prints 1. (If a later revision splits R5, it replaces `R5` by `R5a` and `R5b` in this check.) `grep '^| R |' knowledge/plans/ui-rework.md | grep -c 'and its closing record'` prints 0, `grep '^| R |' knowledge/plans/ui-rework.md | grep -c 'finance stage'` prints 1, `grep '^| R |' knowledge/plans/ui-rework.md | grep -c 'every change of this section'` prints 0, and `grep -c 'admits no commit' knowledge/plans/ui-rework.md` prints at least 1.
5. Only additions in the policy files: `git diff main --word-diff=porcelain --word-diff-regex='[[:alnum:]]+|[^[:space:]]' -- AGENTS.md docs/decisions/0011-tiered-verification.md .claude/skills/ovc-verify/SKILL.md docs/development.md docs/threat-model.md knowledge/plans/slice-10.md | grep -v '^---' | grep -c '^-'` prints 0.
6. The sub-slice order is unchanged: `grep -cF '10.1, 10.2, 10.3, the interface rework stage (`UI`), the finance stage (`FIN`), the booking stage (`BOOK`)'` prints 1 for each of `AGENTS.md`, ADR-0011 and ovc-verify, on `main` and on the branch.
7. With the plan file staged or committed, `git diff main --name-only` lists exactly the eight files above; `git diff main --name-only | grep -c -e 0018 -e redesign.md` prints 0.
8. Change checks pass, each exit 0, after both the 1- and 5-minute load are below 6: `tools/docs/check-docs.sh` with no ERROR line, and Gitleaks over staged changes (the "Anything" row); for the `.claude/` row `python3 .claude/hooks/check_guard_secrets.py`, `tools/check-dev-env.sh`, `tools/check-merge-gate.sh` and Gitleaks over history.

## Merge gate scope

Not Markdown-only: `AGENTS.md` and `.claude/` are outside `docs/` and `knowledge/`, so the gate runs the full local check run. Heavy runs only in a slot the coordinator gives.

## Risks

- **Mislabelling to keep a commit out of FIN's review:** only rows on `main` before the commit, added by owner-approved revisions, count, and the generic RS row admits nothing; RX checks admission and ancestry for every commit; everything else joins FIN.
- **The first review's own record escaping every review:** it goes to FIN's list like a closing record, and RX lists it.
- **A redesign commit after the review started escaping every review:** the list closes when RX starts, and such a commit joins the next unstarted sub-slice.
- **A weaker review than FIN's would have been:** every slice review rule applies, the named reviewers are a minimum, tooling and security always run, and known-limitations rows are re-verified and resolved before `UI` closes.
- **Interleaved commits reviewed in different lists:** RX names each integration boundary with FIN-list commits and its reviewer.
- **Copies of the rule drifting:** the full text is in `AGENTS.md` and ADR-0011; the other five places point to it; criteria 1 to 3 check them.
- **`UI` stays open longer and FIN waits:** expected by the redesign plan's placement before FIN; the gate allowance is unaffected.
- **No rendered-screen review** applies to this change: it alters no rendered screen (`not applicable`).

## Owner's answers (2026-11-27)

- **Where the exception is written:** in full in `AGENTS.md` and ADR-0011, with pointers in ovc-verify, the development guide, T-41, `slice-10.md` and `ui-rework.md`; ADR-0018 unchanged; `ui-rework.md` records the departure from the redesign plan's wording (option A of two alternatives: also a sentence in ADR-0018, or only `AGENTS.md` and ADR-0018).
- **Reference studies:** each is its own `RS-<screen>` redesign row and joins the supplementary review (option A; the alternative sent them to FIN's scope).

## Changes after the planning reviews of revision 1

- The list closes when the supplementary review starts; later redesign commits join the next unstarted sub-slice.
- The supplementary review follows every slice review rule; named reviewers are a minimum; "directly or transitively"; rendered review includes screens changed through shared tokens, components or the frame; tooling and security always run, agent configuration its own family; integration boundaries with FIN-list commits named.
- Admission: a row must be on `main` before its commit or be added by it, names its revision, and the revision's changelog records the owner's approval.
- RX is the closing record, outside the redesign rows, going to FIN's list under the existing rule, and replaces the first record's closing statement.
- Criteria rewritten: additions only by word diff; order sentence by `grep -cF`; one table row per label; T-41 checked inside its row.
- ADR-0011 anchor corrected; `ui-rework.md` section 3 and `slice-10.md` D49 updated; the departure from the redesign plan's naming of ADR-0018 recorded; the development guide's status line amended.
- Order stated from the slice 10 plan's D1, with the notifications change and the 720 s allowance.
- Wording: "the stage's list"; no session-local reference; the variant without reference-study rows given; criterion 8 names the "Anything" row.

## Changes after the scoped re-review of revision 2

- The first review's record (row R) goes to FIN's list like a closing record; row R's content cell no longer calls it the closing record; edit 1 names it; a criterion checks row R.
- Criterion 5 uses `--word-diff-regex='[[:alnum:]]+|[^[:space:]]'`, so the mid-sentence T-41 insertion counts as an addition only; checked in a scratch repository: the old regex prints 1, the new prints 0.
- The generic RS row admits no commit; "joins the finance stage" bounded as "the earliest sub-slice whose slice review has not started, the finance stage until its review starts"; "above" replaced by "in the Slice review bullet below" in `AGENTS.md` and "of this bullet" in ADR-0011; criterion 4 adjusted for the variant without reference-study rows; anchors rechecked at `512765d`, and the benchmark audit named in clause 7; the ovc-verify section's name and criterion 7's staging condition corrected.
- No further planning reviewer: the two major fixes are a one-clause mapping addition and a criterion verified by a scratch reproduction; the owner may ask for one before approval.

## Changes after the owner's answers and the scoped re-review of revision 3

- RP is admitted by RM by name, as the one exception to the admission rule, since it merges before RM.
- Row R's criterion checks the old wording "and its closing record" is gone and "finance stage" is present.
- RX, as the closing record, maps every commit on `main` after the first review's mapping up to its start commit to exactly one sub-slice's list, and FIN's record lists the commits sent to it; edit 1 and clause 5 say so, with a criterion.
- "every other commit in no list"; `AGENTS.md` names the rules "of the slice tier in this bullet and of the Slice review bullet below"; a study's own owner-approved change adds its `RS-<screen>` row; RX's row says "supplementary slice review"; owner's answers recorded, the alternatives removed, and no bare "Q1" label that clashes with `ui-rework.md`'s Q1 to Q4; the first review's record replaces its closing sentence before it merges, owned by the coordinator, with a check; the T-41 insertion is a parenthetical so the existing sentence continues.
- Further reviewer: the coordinator's rule for this change sends that decision to the owner.

## Changes after the scoped re-review of revision 4

- Criterion 2 checks each copy's own wording: "of the slice tier in this bullet and of the Slice review bullet below" in `AGENTS.md`, "of this bullet" in ADR-0011.
- Row R depends on "U0 to P2 and L0 to L4 merged", so it no longer waits for the redesign rows and RX; edit 1 sends the supplementary review's record to FIN's list and says the finance stage's record maps every commit after the supplementary review's start commit; the admission and ancestry check is in edit 1, each with a criterion.
- Edit 1 introduces "the stage's first slice review" and "a supplementary slice review" before using them and says "the redesign commits changed"; the T-41 parenthetical sits right after "whose review has not started"; the record-sentence check fails if the file is not on `main`.
- Further reviewer: decided by the owner.

## Changes after the readiness reviews

- The standard reviewer found that naming RP by its subject, which this change first did because RP was not yet on `main`, departed from clause 2; the owner decided on 2026-11-27 to keep the hash: RP merged first and its hash on `main` is written in row RP and clause 2 of `ui-rework.md`.
- "added by an owner-approved revision of the interface rework plan" replaces "of that plan", whose antecedent could be read as the redesign plan; the finance stage's record also lists the commits the supplementary review's record sent to it.
- In `ui-rework.md`: clause 5 keeps the words on the commits sent to the finance stage's list; section 8's R criterion is limited to the commits that merged before that review started, and an RX criterion is added; clause 7 names R4a and R4b.
- In this file: the owner's answer on reference studies replaces a question label, and the redesign plan is named by branch and subject instead of a commit that will not reach `main`.
