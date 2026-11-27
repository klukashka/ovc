# Plan: deny the design plugins' remote tools in the project settings

Branch: `chore/deny-design-plugin-remote-tools`, cut from `main` at `4de0ac4`. After approval this plan is committed as `knowledge/plans/deny-design-plugin-remote-tools.md`. Revision 2, approved by the owner on 2026-11-27, applies the security-profile plan review: mods and other connectors as residuals, the documented `__*` form, checkable criteria and the kept link.

## Problem

Since `4de0ac4`, `AGENTS.md` says no design tool's remote service or connector is used for OVC work unless an owner-approved decision record names it, and T-39 in `docs/threat-model.md` records that "no setting blocks those tools yet". The rule rests on agent instructions alone. The design plugins installed for the owner bring remote MCP servers that would receive whatever an agent sends them, screenshots included, and some of them are outside Russia (ADR-0008). They are only unauthorized for now; once someone signs in, nothing stops a call.

## Desired outcome

`.claude/settings.json` denies every tool of the design plugins' remote MCP servers, so Claude Code refuses those calls in this repository even where a user-level allow rule or a hook would allow them (residuals below: an installed mod, bypass mode, other connectors), while the plugins' skills (`ui-design-review:ui-review`, `design:design-critique`, `design:accessibility-review` and the others) keep working. T-39 states the setting instead of "no setting blocks those tools yet".

## Tier and security trigger

Standard tier. The security trigger is met: the change edits agent configuration (`.claude/settings.json`) and maps to T-39 (third-party plugins as tools), T-18 (personal data reaching foreign services) and T-41 (agent configuration; this change only tightens it). Reviews: one fresh security-profile plan reviewer before approval, then one standard and one security-profile reviewer over the diff.

## Facts the plan relies on (checked 2026-11-27)

- Claude Code permission rules: `mcp__<server>` and `mcp__<server>__*` match every tool of one server; deny rules from any settings level are evaluated before allow rules, so a user-level or narrower allow cannot override a project deny (Claude Code documentation, permissions page, section "MCP" and "Settings precedence", read 2026-11-27).
- A plugin's MCP server is named `plugin_<plugin>_<server>`, so its tools are `mcp__plugin_<plugin>_<server>__<tool>`; hyphens in the plugin name are kept (Claude Code documentation, MCP page, section "Plugin-provided MCP servers", read 2026-11-27).
- The remote servers this session's environment lists for the design plugins: `design` brings `asana`, `atlassian`, `figma`, `intercom`, `linear`, `notion` and `slack`; `ui-design-review` brings `uxkin`. Deny rules do not disable the plugins' skills (`enabledPlugins` would).
- Precedence among rules, hooks and mods (same page): without managed settings, a managed MCP file or a Team or Enterprise login, an installed mod's tool check can approve some calls that a deny rule refuses; this machine has a mod installed.
- A bare-name or glob deny removes the server's tools from the agent's context (same page).
- Not confirmed in the documentation: that deny rules hold in `bypassPermissions` mode (the mode table names no exception; the rule order says deny is checked first). Recorded as a residual, see Risks.

## Exact change

### 1. `.claude/settings.json`, `permissions.deny`, append in this order

```json
"mcp__plugin_design_asana__*",
"mcp__plugin_design_atlassian__*",
"mcp__plugin_design_figma__*",
"mcp__plugin_design_intercom__*",
"mcp__plugin_design_linear__*",
"mcp__plugin_design_notion__*",
"mcp__plugin_design_slack__*",
"mcp__plugin_ui-design-review_uxkin__*"
```

The `__*` form follows the documentation's own plugin example (`mcp__plugin_my-plugin_database-tools__*`); names containing `_` get no start-up warning when they match no tool, so criterion 1 checks each name against the session's server list. No wildcard across plugins (`mcp__plugin_*`) is used: its matching is not documented, and it would also block plugins unrelated to design.

### 2. `docs/threat-model.md`, T-39 (pending owner review)

Replace, in the sentence added on 2026-11-27, "this is an instruction to agents in AGENTS.md, and no setting blocks those tools yet" with "AGENTS.md requires it, and `.claude/settings.json` denies every tool of the design plugins' remote MCP servers by name, so a newly installed plugin's server is blocked only once it is added there; the servers still connect and may start authorization at session start, the setting does not cover claude.ai connectors or servers a user configures, and an installed mod can still approve a denied call unless managed settings are present (amended 2026-11-27, pending owner review)". Append to the status line: "T-39 amended 2026-11-27 with the project setting that denies the design plugins' remote tools, pending owner review."

### 3. `AGENTS.md`, the UI work bullet

Append to its last sentence: "; `.claude/settings.json` denies the remote tools of the installed design plugins, and a new design plugin's remote servers are added there in the change that adopts it".

## Acceptance criteria

1. `python3 -c "import json;d=json.load(open('.claude/settings.json'));s=set(d['permissions']['deny']);need={'mcp__plugin_design_asana','mcp__plugin_design_atlassian','mcp__plugin_design_figma','mcp__plugin_design_intercom','mcp__plugin_design_linear','mcp__plugin_design_notion','mcp__plugin_design_slack','mcp__plugin_ui-design-review_uxkin'};print(need<=s, len(d['permissions']['deny']))"` with each name ending in `__*` prints `True 38`, and the readiness report maps each entry, after dropping `mcp__plugin_` and `__*`, to a `plugin:<plugin>:<server>` name in the session's MCP server list, quoted there.
2. `git diff main -U0 -- .claude/settings.json | grep '^-[^-]'` prints at most the one line whose trailing comma changes; no existing allow, ask or deny entry and no hook changes.
3. `grep -n 'denies every tool of the design plugins' docs/threat-model.md` finds T-39, `grep -c 'no setting blocks those tools yet' docs/threat-model.md` prints 0, and `grep -n 'denies the remote tools of the installed design plugins' AGENTS.md` finds the bullet.
4. `git diff main -- .claude/settings.json | grep -c enabledPlugins` prints 0, so no plugin and none of its skills is disabled; the session's skills list (`ui-design-review:ui-review`, `design:design-critique` still listed) is quoted in the readiness report as supporting evidence only.
5. Runtime refusal of a denied call: `unverified` while the servers are unauthorized (no tool of theirs is loaded to call). The readiness report says so; the enforcement rests on the documented rule order and on the documented removal of denied tools from the agent's context.
6. Change checks pass, each exit 0: `tools/docs/check-docs.sh`; Gitleaks over staged changes; and, for the `.claude/` row, `python3 .claude/hooks/check_guard_secrets.py`, `tools/check-dev-env.sh`, `tools/check-merge-gate.sh` and Gitleaks over history. `python3 -m json.tool .claude/settings.json` exits 0.

## Files

`.claude/settings.json`, `docs/threat-model.md`, `AGENTS.md`, `knowledge/plans/deny-design-plugin-remote-tools.md`.

## Order of work

1. Security-profile plan review (read-only); fix the plan; send it to the coordinator for owner approval.
2. After approval: commit the plan file, make edits 1–3, run the change checks (both the 1- and 5-minute load below 6 first).
3. One fresh standard reviewer and one fresh security-profile reviewer over the diff; fix findings under the after-fix rule.
4. One commit, no AI attribution; send the commit and tree to the coordinator. No merge.

## Merge gate scope

Not Markdown-only: `.claude/settings.json` and `AGENTS.md` are outside `docs/` and `knowledge/`, so the gate runs the full local check run.

## Risks

- A plugin update or a newly installed design plugin adds a server under another name, which the list does not cover: the AGENTS.md sentence makes adding it part of adopting the plugin; a wildcard is not used because its matching is undocumented.
- Deny rules in `bypassPermissions` mode are not confirmed by the documentation: residual, recorded in T-39's wording as "denies" without a claim about every mode; the repository's default mode is `acceptEdits`.
- A blocked server still connects (and asks for authorization) at session start; only calls are refused. The owner can also switch the servers off per project with `/mcp`, which lives in the user's own configuration, not in the repository.
- An installed mod can approve some calls that a deny rule refuses unless managed settings, a managed MCP file or a Team or Enterprise login are present: residual, stated in T-39; no mod in use here is meant to approve MCP calls.
- The setting covers only these plugin servers: claude.ai connectors (`mcp__claude_ai_<server>__*`), servers a user configures and the start-up connection with its authorization request are not covered; the AGENTS.md rule still applies to them, and `/mcp` disconnects a server in the user's own configuration.
- The engineering plugin's remote servers (`github`, `datadog`, `pagerduty`) are outside this change's scope, which the owner set to design plugins; covering them would be a separate decision.

## Changes after the readiness reviews

The standard and security-profile reviewers of the diff found only minor findings, both about T-39's residuals; T-39 now also states that a mod can make a denied call itself, that deny rules in bypass mode are not confirmed by the documentation, and that other plugins' remote servers are not denied, as the Risks above already listed.
