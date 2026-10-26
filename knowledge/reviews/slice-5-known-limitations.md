# Slice 5 known limitations

Minor findings accepted during slice 5 instead of starting another review round ([ADR-0011](../../docs/decisions/0011-tiered-verification.md)). The slice review re-verifies each entry; each is fixed, accepted by the owner or carried into a named later slice.

| Finding | Severity | Change | Why it was accepted |
|---|---|---|---|
| No test proves that the stale-template cleanup of the integration-test fixture keeps a template-named database owned by another role, or that a wait for the template lock ends after one minute; the age and name rules, the process's own template and clone independence are tested | minor | Test database template | The owner filter is one condition in the query that lists the candidates, and the bound is a context deadline on the lock statement; a test of either would need a second login role on the shared local server or a lock held for a minute, which would disturb other test processes and worktrees sharing that server |
