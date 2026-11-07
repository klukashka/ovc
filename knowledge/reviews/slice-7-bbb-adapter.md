# BigBlueButton adapter verification

Status: implemented on 2026-11-07; change checks and independent reviews complete. The exact-tree gate is reported separately in the merge report.

Scope: section 5 of the [approved local plan](../plans/slice-7-local-completion.md). Tier: standard. Security trigger: met (meeting provider, secrets, outbound network calls, join capabilities), so a separate security-profile review applies. Observability impact: the existing provider events, with five new fixed error kinds.

## Threats

T-11, T-12, T-18, T-19, T-20, T-25, T-26 and T-41, as mapped in the [threat model](../../docs/threat-model.md#bigbluebutton-adapter-prepared-locally).

## Acceptance evidence

All of these run against a real in-process TLS server, in `backend/internal/adapters/meeting`, except where another package is named.

| Plan requirement | Test |
|---|---|
| Shared provider contract | `TestBigBlueButtonProviderContract` |
| HTTPS-only configuration, redacting secret, no SHA-1, 15-second and 1 MiB bounds | `TestBigBlueButtonConstructionRefusals`, `TestBigBlueButtonDiagnostics` |
| Recording flags, callback routing, exact parameter set, checksum algorithms | `TestBigBlueButtonCreateRequest` |
| The coordinator supplies routing key and lesson | `TestMeetingCreateOwnership/Create_carries_the_meeting's_own_routing_key_and_lesson` in `internal/repositories`, recorded by the in-memory fake |
| Duplicate Create | `TestBigBlueButtonDuplicateCreate` |
| Unavailable, malformed and size cases, wrong meeting, exact 1 MiB boundary | `TestBigBlueButtonResponseFailures` |
| Redirect rejection | `TestBigBlueButtonRefusesRedirects`, `redirect status` in `TestBigBlueButtonResponseFailures` |
| Injected trust roots | `TestBigBlueButtonTrustsOnlyInjectedRoots` |
| No Create retry | `TestBigBlueButtonCreateIsNotReplayed` |
| Deadline and cancellation, before headers and inside a body | `TestBigBlueButtonDeadlinesAndCancellation` |
| Inspect missing and current, stale End, generation binding | `TestBigBlueButtonProviderContract`, `TestBigBlueButtonGenerationBinding` |
| Asynchronous End and ended-room visibility | `TestBigBlueButtonEndConfirmation` |
| Links: origin, binding, tampering, End and replacement, encoding | `TestBigBlueButtonJoinLinks` |
| Safe diagnostics | `TestBigBlueButtonDiagnostics` |
| Production start refused | `TestLoadRefusesTheBigBlueButtonProviderInEveryProcess` in `internal/core`, `TestBigBlueButtonProcessSelectionDisabled` |

Negative control: with `DisableKeepAlives` set to false in the working copy, `TestBigBlueButtonCreateIsNotReplayed` failed with two received Create requests, proving the test detects a transport replay. The setting was restored before the checks.

## Reviews

One standard reviewer and one security-profile reviewer covered the complete diff, and one fresh standard reviewer covered the fixes, because the first review found a major issue: short real-time deadlines covered connection setup in the deadline tests. Those tests now expire an injected bound after the server is reached. Minor findings fixed: a protocol fake answer contradicting recorded server behavior, an unqualified presentation line, an undocumented log event, a wrapper test double, lax XML before the root and repeated fields, no minimum shared-secret length, an unmapped lesson identifier in `logoutURL`, and coverage gaps. No security finding touches an active trust boundary, because no process can select the adapter.

## Not verified

Everything needing a live server stays `unverified`, as listed in the [dated facts](../research/bigbluebutton-api-2026-11-07.md#unverified-until-the-remote-probe). Opt-in remote commands were not prepared.
