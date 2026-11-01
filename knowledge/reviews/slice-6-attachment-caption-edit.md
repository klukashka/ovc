# Attachment-caption editing verification

Status: implementation complete; affected checks and independent readiness reviews in progress. Base `ecd74d8e165b54f5cde41f45884a48cb122ea8b2`, branch `fix/attachment-caption-edit`. The owner approved the implementation plan on 2026-11-01 after its security planning review. Standard tier, security trigger met by message validation and attachment-reference integration.

## Behavior and boundaries

Edit applies the existing body validator using the message scope's attachment presence. Author, live-message, membership, writable-channel, edit-window and version checks still precede validation. Attachment identity is immutable for a live message; a concurrent deletion/erasure is serialized by the channel/message locks and the existing state refusal. The scope itself is read before those locks and is not described as a locked snapshot. No rebinding, quota mutation, download authorization or new diagnostics is introduced.

The API PATCH contract is reconciled with the Messages rule: empty/whitespace-only text is allowed with an existing attachment. The threat model records the retained trust boundaries and pending owner review of wording. Existing `chat.message.edited` remains content-free.

## Regression evidence

The new `TestAttachmentCaptionsCanBeEditedToEmptyAndWhitespace` first failed against the unchanged implementation: HTTP 422 `body.body:required` instead of 200; negative-control run exit 1, 5.194 seconds. With the production correction it verifies empty and whitespace-only edits, version increments, edited timestamps, unchanged attachment identity/reference/charge/usage and real downloadable bytes. `TestAttachmentCaptionEditsKeepBodyValidation` checks empty text-only edits, the 4,000-code-point boundary and forbidden controls, retaining stored state and ownership on refusal.

The first combined run exposed an existing fixture issue: `editRequest` formatted strings with Go `%q`, producing an invalid JSON escape for NUL, so the API correctly answered malformed-request 400 before body validation. The helper now encodes its precise request struct with `encoding/json`, matching the existing send helper. The regression still requires the intended structured 422 validation refusal. This was a fixture correction, not an assertion relaxation. A formatting command also initially used repository-relative paths from the backend directory and failed to find them; it was rerun with the correct paths.

The corrected targeted race run passed, exit 0, 2.856 seconds:

```sh
go test -race ./internal/api -run 'TestAttachmentCaption|TestEditsWithinTwentyFourHoursAndEditWindowPassedAfter|TestAuthorsDeleteButDoNotEditInReadOnlyChannels|TestTutorChatWritesRacingADeletionOrBlockAnswer401' -count=1
```

The complete backend race suite (`go test -race ./...`, through the main checkout's `tools/dev-env.sh`) passed, exit 0. `go vet ./...`, `../bin/golangci-lint run ./...` (zero issues), `../bin/sqlc -f sqlc.yaml diff` and the changed Go files' `gofmt -l` check passed, exit 0. The first sandboxed static attempt stopped at golangci-lint with `context loading failed: no go files to analyze`; the same commands with access to installed caches passed. No assertion failed in that environment refusal.

Offline frozen installs reused 262 documentation packages and 399 frontend packages, downloading zero. Initial sandboxed installs could not access complete local cache snapshots and failed with `ERR_PNPM_NO_OFFLINE_TARBALL`; the authorized cache-access run passed without dependency changes. `tools/docs/check-docs.sh` passed: 2,142 links, zero errors, and all six diagram sources rendered.

The affected production browser journeys passed in Chromium, Firefox and WebKit, six tests, 10.7 seconds, exit 0:

```sh
pnpm e2e --grep 'editing and deleting own messages|dropped documents and pasted pictures'
```

This run used `caffeinate -i` and the main checkout's `tools/dev-env.sh`, with real local database/storage and the normal production-build harness. Its scope proves existing edit/delete and attachment upload/download journeys; the new empty-caption behavior is exercised by the real API regression above.

The independent standard reviewer inspected the complete implementation, tests, plan and contract diff against the base above and reported no findings. It traced the immutable-attachment premise through channel/message locking, deletion/erasure and retained authorization, validation and rate semantics. The verification record was added after that inspection; the reviewer did not run runtime checks.

The separate fresh full-security reviewer inspected the complete tracked and relevant untracked diff and reported no findings. T-5/T-21 authorization, T-34 lifecycle, T-14/T-20 text validation/resource bounds and T-18 diagnostics were examined, together with reference/quota/download integration. Unchanged upload inspection, credentials, conferencing, answer keys, CSP, served directories, test-control configuration, dependencies and residency were not affected. The reviewer confirmed closure of the empty-caption refusal; no special concurrent empty-edit/deletion test was run, and the new oversized-body case checks 4,001 code points rather than successful exactly-4,000 input. Those are evidence limits, not claims of independent runtime execution. The reviewed tracked binary diff SHA-256 was `f1e833ba151dec79cfb5d28309d8dfeb4a567c5ad8140adb8556f28ff2f92dab`; the new test's SHA-256 was `dcd1aff4303c353e2e42f4db96d6a6be6505b8e62d734b43f9f36bd3739cee07`.

Final classification remains standard with the security trigger met; no security review finding required a fix. Other full-run checks are deferred to the exact-tree merge gate. The separate cumulative tutor-session restoration finding is addressed by its own approved plan; this narrowly scoped change makes no claim to closing slice 6 or that separate finding. The gate/finish report is retained outside the tested tree after execution, so no post-gate edit invalidates that tree.
