# Slice 4 known limitations

Minor findings accepted during slice 4 instead of starting another review round ([ADR-0011](../../docs/decisions/0011-tiered-verification.md)). The slice review re-verifies each entry; each is fixed, accepted by the owner or carried into a named later slice.

| Finding | Severity | Change | Why it was accepted |
|---|---|---|---|
| During the transition to the per-user end-to-end run lock, a worktree whose branch still carries the earlier harness takes the old per-checkout lock in its own `frontend/test-results/e2e-run.lock` and does not contend with the per-user lock in the user's cache folder, so it can run at the same time as a run on the new harness and share the per-worker ports and the `ovc-e2e` bucket with it | minor | Per-user end-to-end run lock | The window closes once every active worktree has merged `main` after this change; until then, sessions start an end-to-end run in an older worktree only after merging `main` into it |
| No test proves that a failed clock read ends a run; a failed claim and a failed retry update are tested | minor | Per-lane worker loops | The system clock that the continuous loops use never fails to read, and `Tick` uses the instant it is given; a failed clock read while claiming takes the branch of the tested failed claim, and one while completing takes the branch of the tested failed retry update |
| A panic in the worker's own claiming code after a claim committed leaves the rows it had not yet started `in_progress` until their 5-minute lease expires, and they are then sent; the lane records the panic, stops claiming, lets its started effects finish and ends the run | minor | Per-lane worker loops | It needs a programming error in the claim path; delivery stays at least once and no row is lost |
