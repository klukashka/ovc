# Slice 7 known limitations

Status: working review inventory. Entries are evidence for cumulative slice review, not automatic owner acceptance or permission to close the slice.

## Cleanup reinspection interleaving

Severity: minor, identified by the independent security review of the constructor-injected cleanup executor based on `25e2066bcb5583048675ecda32f8e79165e626b4`.

The executor integration tests do not deterministically replace a room between successful Inspect and End, so the stale-End reinspection outcomes (absence, replacement and unchanged generation) are not directly exercised together. Static review found the branch correct; real provider contract tests prove stale-generation refusal, and the executor test plus negative control proves the initial recorded-generation guard. These are supporting evidence, not proof of this exact interleaving.

Disposition: retained for the named slice 7 provider-lane runtime activation follow-up, before enabling the executor in process composition. The current constructor-injected change is used only by tests and exposes no route. Adding a behavior-replacing wrapper would violate the test policy; the follow-up must use an approved real synchronization mechanism and preserve all assertions and deadlines. This is not owner acceptance for slice closure. The backend and security cumulative reviewers must recheck the entry. See the [implementation evidence](slice-7-meeting-cleanup-worker.md).
