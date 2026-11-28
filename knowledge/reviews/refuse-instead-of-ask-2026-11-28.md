# Refuse instead of asking (2026-11-28)

Record of the owner's decision of 2026-11-28 on approval prompts from the agent configuration. The wording of the documents it changed in `docs/` is pending owner review.

## Problem

Approval prompts reached the owner from two places: the guard hook `.claude/hooks/guard_secrets.py`, which returned an `ask` decision for a `git push` or `gh` call outside the exact allowed forms, for such a call with a heredoc, for a local check command and whenever it could not read or check a call; and the 87 ask rules that `.claude/settings.json` kept after the owner removed, on 2026-11-27, those for push, rebase, reset, clean, `branch -D`, restore, stash drop and stash clear. The remaining rules cover the other destructive Git forms (`git checkout`, `--force`, `-f`, `--amend`, `filter-branch`, `prune`, forced worktree removal and others). The owner approved almost every one of these prompts, so they cost his attention without protecting anything.

## Decision

- Every call the guard hook asked the owner about is refused with the same reason. The hook exits with status 2 and a message that starts with `Refused:` and tells the agent to rewrite the call in an allowed form (a `gh` or `git push` call as the whole command, without `--repo`, `;`, `|`, `&&`, redirection or `$(…)`; read-only Git loops in a script file; no local checks) or to give the owner the exact command to run himself. The exception paths, where the hook could not check a call, refuse too, and an unexpected error inside the hook ends with status 2.
- Every ask rule of `.claude/settings.json` is a deny rule, and the file has no ask rules. The allow rules and the earlier deny rules are unchanged.
- Every deny path of the hook keeps its reason and its `Blocked:` message.
- The four forms an agent runs without asking under `AGENTS.md` stay allowed: the exact push forms, which the hook alone checks; `git branch -D` of a branch whose tip is an ancestor of `main`; `git reset --merge` with no revision argument after a failed squash gate; and `git restore --staged`. Under glob matching, as the self-test checks it with Python's `fnmatch`, no deny rule matches them.
- Open question: Claude Code's own matching of the deny rules `Bash(git branch * -D*)` and `Bash(git * branch * -D*)` is unverified. If its wildcard lets ` * ` collapse to a single space, these rules refuse every `git branch -D <branch>`, the agent form included. The owner tries `git branch -D` of an ancestor branch once in a session and records the result here; until then those rules stay, and a refused agent gives him the exact command.
- When an action really needs the owner (a push outside the exact forms, a remote branch deletion, a workflow push, any force or history rewrite, another `git branch -D`, a local check or gate), the agent gives him the exact command and he runs it in his own terminal.

The change is strictly tighter: no call that ran without a prompt starts running, and no call that needed approval can run.

## Verification

- `.claude/hooks/check_guard_secrets.py` expects a refusal (exit status 2, a `Refused:` message, nothing on standard output) wherever it expected `ask`, with the same reason fragments; fails any case that prints a decision on standard output; runs the exception paths of the call check (an invalid lock folder argument and an unexpected argument) and expects a `Refused:` refusal from each, and a malformed payload (a non-object tool input and a non-string tool name), which the secret guard blocks first, and expects its `Blocked:` denial; fails when the hook's source holds a `permissionDecision` or an `ask` literal; fails when `.claude/settings.json` holds an `ask` key or lacks the deny rules for `--force`, `--amend`, `-f`, `git checkout`, `filter-branch`, `update-ref` and `reflog expire`; and fails when a deny rule matches one of the four agent forms, or the hook does not pass `git branch -D`, `git reset --merge` or `git restore --staged`.
- `.claude/settings.json` stays valid JSON.

## Threat model

T-19 and T-41 of the [threat model](../../docs/threat-model.md) record the change and its residuals: an action that only the owner may take is refused to agents outright and run by the owner outside every agent control, and a refused agent may look for a spelling the text-matching controls do not recognise.

## Documents changed

`AGENTS.md`, the [development guide](../../docs/development.md#continuous-integration), [ADR-0009](../../docs/decisions/0009-owner-approved-exceptions-and-visibility.md) item 5, [ADR-0018](../../docs/decisions/0018-targeted-checks-and-merge-gate.md#ci-as-the-merge-gate), [ADR-0026](../../docs/decisions/0026-github-hosting-and-ci.md), the [threat model](../../docs/threat-model.md), [ovc-verify](../../.claude/skills/ovc-verify/SKILL.md) and [AI development verification](ai-development-setup.md).
