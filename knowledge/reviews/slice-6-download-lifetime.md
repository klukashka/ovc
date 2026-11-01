# Slice 6 download lifetime

Status: implementation and independent reviews complete; change checks passed on the current integration base; exact-tree merge gate pending. Baseline: `f9c7450928a17017bc34654159a5d27b9299147c`. Original branch: `fix/chat-download-lifetime`; integration branch: `fix/chat-download-deadline`. The owner approved the implementation and the narrow local-S3 response-holding exception on 2026-11-01.

## Scope and acceptance

This standard-tier follow-up addresses major finding S6-BE-02: opening previously stopped the only upstream cancellation timer, while the downstream write deadline could not interrupt a blocked S3 body read. Security trigger: yes, resource lifetime, downloads and test policy affect T-20/T-13/T-27/T-41. The fix touches the resource-lifetime boundary; token validation and authorization are unchanged.

The service retains a 30-second opening deadline covering its attachment query and storage open. After opening, a size-based budget at 128 KiB/s, with a 60-second minimum and 10-minute maximum, becomes one absolute monotonic deadline shared with the HTTP writer. It cancels the storage context through reads and the final size probe. Body Close stops the timer and closes/releases once, including concurrent Close calls. Timeout diagnostics preserve safe error kinds without addresses or file metadata.

The real-storage forwarder holds the actual response body without substituting status, headers or content. It uses the fixture-validated local storage endpoint and an ephemeral loopback listener; cancellation closes upstream and cleanup releases the barrier. Its narrow policy exception is recorded in AGENTS, ADR-0009 item 11, development guidance and T-41.

## Checks

From `backend/`, `/Users/klukashka/Work/ovc/tools/dev-env.sh go test -race ./internal/api -run '^TestChatDownload' -count=1 -parallel=2` passed (exit 0, 3.179 seconds). Four integration tests cover a real stalled S3 body through HTTP, explicit cancellation with repeated Close, initial database waits, and cancellation before the final probe with concurrent Close. After each interruption all 64 slots can be acquired and the 65th is refused; this checks release and preservation of admission limits. The HTTP timeout case also verifies an incomplete response, a subsequent complete download and the safe timeout event.

`go vet ./...` and `/Users/klukashka/Work/ovc/bin/golangci-lint run ./...` passed, exit 0 with zero lint issues. These executions precede a small error-classification refinement that preserves an ordinary storage failure instead of joining the cleanup's cancellation cause; final checks must cover that refinement.

The first independent standard review found a major acceptance gap: cancelling before `Overruns()` did not prove cancellation of an already blocked final probe. It also reported a minor missing Close/deadline race. The fix adds a real oversized-object fixture: storage contains 1,025 bytes while attachment metadata records 1,024. The transparent forwarder passes the first 1,024 unchanged bytes and holds the next actual byte, preserving the real Content-Length. After reading the recorded body, `Overruns()` waits on that real stream and is cancelled by the deadline. No EOF, headers or body are fabricated. Another test schedules Close at the actual returned deadline over eight lifecycles under the race detector, then checks all admission slots. Both findings require the fresh security reviewer to apply the standard checklist to these fixes as well as the full security profile to the complete diff.

After the error-classification refinement and both added regressions, the same focused race command passed (exit 0, 4.758 seconds), now covering six tests. Final `go vet ./...` and golangci-lint both passed after these fixes, exit 0 and zero lint issues. Broader checks are still being collected.

Broader checks and integration evidence are recorded below; exact-tree gate/finish remain pending. Checks outside the changed rows are deferred to the merge gate. No commit, merge or cumulative performance claim has been made.

## Independent review disposition

The fresh security reviewer inspected the complete final diff and also applied the standard checklist to the fixes. It found no new issues and confirmed both the earlier major blocked-probe coverage gap and minor Close/deadline gap closed. T-20/T-13/T-27/T-41 were examined, including live referencing-message authorization, signed-token binding, downstream headers, upstream cancellation and safe error classification. Unrelated security families were not affected. The production fix touches the storage/HTTP resource-lifetime boundary; the additional finding fixes change tests of that boundary. The reviewer ran no tests, and its verdict does not replace the remaining checks or exact-tree gate.

## Integration on current main

The reviewed uncommitted change was transferred without rewriting history onto `c1cb4aa9b15fd4cbd9b0da8bd5d51993a9ab5611`. The original checkout remains intact. The only merge conflict was the appended threat-model section: both the already merged erasure section and the reviewed download section were retained. All production download files and the six-test file are byte-identical to the reviewed implementation. No new review finding or contract change was introduced by integration.

`caffeinate -i /Users/klukashka/Work/ovc/tools/dev-env.sh go test -race ./...` from backend passed (exit 0): API 75.120 s, file service 9.653 s, conventions 2.533 s. `go vet ./...`, `../bin/golangci-lint run ./...` (zero issues), and `../bin/sqlc -f sqlc.yaml diff` passed. Frontend `pnpm build`, `pnpm check:bundle` and `pnpm api:check` passed. `pnpm e2e message-attachments.spec.ts upload-transport.spec.ts smoke.spec.ts`, through the main settings wrapper with idle-sleep prevention, passed all 93 cases across Chromium, Firefox and WebKit; log `/private/tmp/ovc-download-e2e.log`.

Final documentation checks passed after removing an extra blank line in this record. `python3 .claude/hooks/check_guard_secrets.py` passed (198 checks), `tools/check-dev-env.sh` passed, and `tools/check-merge-gate.sh` passed. Formatting and `git diff --check` passed. Earlier review findings and their closure remain as recorded above; source was not changed after review. The full gate still owns the remaining scanners and complete browser run.
