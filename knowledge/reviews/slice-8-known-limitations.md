# Slice 8 known limitations

Status: agent record, started 2026-11-10. Minor review findings of slice 8 changes that were accepted rather than fixed, in the format of the numbered slices' files. Each entry is re-verified by the slice review, which resolves it as fixed, accepted by the owner or carried into a named later slice.

| Finding | Severity | Change | Why it was accepted |
|---|---|---|---|
| Each template autosave reserves two rate-limit rows (`template_saves_second` and `template_saves_hour`) in their own committed step under the tutor's profile lock and then takes the profile lock again for the save, so an open builder costs up to two such pairs of short transactions per second. | Minor (performance risk) | S1, the library and builder contract | The pattern is the one lesson writes use and is bounded by the two-per-second limit; the backend reviewers of B4 and the slice review check the per-save cost, and measurements wait for the deployment by the owner's decision of 2026-11-08. |
| The presentation's 5-minute signed storage URL may stay in the self-hosted conferencing server's own logs, while the credentials-in-URLs rule of `AGENTS.md` says such URLs are never logged; OVC itself never logs or stores it. | Minor | S1, the library and builder contract | The URL stops working after 5 minutes and grants one file only; the residual is recorded in T-11 and the presentation section, pending owner review, and the remote BigBlueButton activation decides that server's log hygiene. |
