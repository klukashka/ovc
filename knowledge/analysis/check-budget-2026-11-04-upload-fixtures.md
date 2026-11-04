# Upload refusal fixture consolidation evidence

Status: implementation, runtime checks and independent reviews complete. Not approved for a gate or merge.
Owner-approved plan: [upload fixture budget](../plans/slice-6-upload-fixture-budget.md).
Standard tier; security trigger T-13/T-20/T-35/T-36/T-41. Production behavior and
accepted residual risks are unchanged. Main remains 4d9ba1e; candidate 0880789
and every prior failed execution remain preserved.

## Baseline

On 2026-11-04 the original three upload tables passed all 24 named cases in
5.063 seconds using `go test -json -race -count=1 ./internal/api` with an anchored
expression selecting only those tables. The observed wrapper took 5.176 seconds,
recorded three safe samples and verified unchanged source. Tool provenance passed
before execution. The complete backend baseline is the already recorded successful
86.128-second execution; it is not repeated for a favorable comparison.

Hardware has 12 physical and 12 logical CPUs. Build, dependency and operating-system
caches were retained; this is not a cold measurement. `-count=1` prevents test-result
reuse, not compilation-cache reuse. Exact cache residency is unknown. Heavy commands
run sequentially with owned sleep/display assertions and continuous monitoring;
other chat activity is checked before scheduling. Overlapping per-test elapsed times
are not summed to claim a saving.

## Isolation and negative-control ordering

Each chat case retains a distinct tutor/student, relationship and group, and its
own uploader-scoped row and cleanup assertions. The avatar table uses deterministic
source order with the successful exactly-4096-pixel case last. Avatar admission
refuses an account with three pending uploads before content validation. In the
negative control, two deliberately accepted disguised pictures plus the final
valid picture reach three only after all refusal cases have run. Otherwise a
quota refusal could mask the intended media-type assertions. Reversed positive
traversal creates only one pending avatar and must still pass every case.

## Readiness boundary

No new acceptance E2E, gate, merge or long measurement series is authorized.
The historical Chromium ERR_ABORTED remains unexplained by the later successful
diagnostic; this fixture-only change cannot resolve it.

## Preserved case mapping

Each row retains its original inputs, filename/body bytes and assertions.

| Avatar case | Expected HTTP |
| --- | --- |
| over 5 MB | 413 |
| GIF | 415 |
| SVG | 415 |
| text named PNG | 415 |
| PNG named JPEG | 415 |
| WebP named PNG | 415 |
| wider than 4096 pixel | 415 |
| 16-bit PNG | 415 |
| exactly 4096 pixels | 201 |

All seven disguised chat cases retain HTTP 415, unsupported_media_type, exactly
one uploader-owned attachment row and assertLeftNothing inside the child:
a PNG named as a JPEG; markup named as a page; markup named as a picture; text
without an extension; a program; a generic ZIP named as a document; text with
a NUL beyond its first part. Their stable IDs are 1 through 7 in this order.

| Multipart case | HTTP | Field | Rows |
| --- | --- | --- | --- |
| two file parts | 422 | body.file:invalid | 1 |
| a file then a field | 422 | body.file:invalid | 1 |
| a field before the file | 422 | body.file:required | 0 |
| part headers over 8 KiB | 422 | body.file:required | 0 |
| no part at all | 422 | body.file:required | 0 |
| a malformed body | 400 | none | 0 |
| a malformed trailing part | 400 | none | 1 |
| a second part with headers beyond the allowance | 422 | body.file:invalid | 1 |

Multipart IDs are 1 through 8 in this order. Exact cardinality and absence of
extra rows are preserved, and every resulting row is checked with assertLeftNothing
in its child. Case n uses tutor 2n-1 and student 2n, distinct per-case emails and
group; each chat table has its own database, so IDs do not collide across tables.

## Source and executed controls

Reviewed/tested Git blob identities:

- avatars_test.go: 5f7fb37ca0ede137315587ba901c621aab057c89
- chat_uploads_test.go: b0ce2d92b148577dd657d9c175ae70b589705713
- upload_refusal_parties_test.go: f471132e73f03e2a9e7b00c1b9ff5dffb0e63929

All executions use go test -json -race -count=1. Changed and control targeted
commands use the same anchored selection of the three tables as the baseline.
The normal changed run passed all 24 names in 6.664 seconds; reverse traversal
passed all 24 in 10.849 seconds. Both successful case-name sets exactly match
the baseline. Compilation caches were retained but isolated source paths and
changed code require compilation; these targeted durations are not isolated
fixture-cost measurements and do not establish a performance gain.

The negative control returned exit 1 as intended: only PNG named JPEG and WebP
named PNG failed at the retained status assertion with HTTP 201 instead of 415;
22 cases passed. Source manifests for both isolated copies remained unchanged.
The normal, reverse and negative observed runs took 6.776, 10.956 and 10.012
seconds, with 3, 4 and 3 safe samples respectively and unchanged primary source.
No unexpected test failed in these executions.

Private command results, JSONL and monitoring live under the private temporary
directory in ovc-upload-fixture-{baseline,changed,reverse,negative,backend}-results
and corresponding observed directories. Source manifests bind every execution.
Artifacts remain outside served roots; no raw request bodies or credentials are
copied into this record.

## Complete backend result

The complete backend race command passed all 1879 tests, exit 0, in 79.229
seconds. The observed wrapper took 79.416 seconds with 17 safe samples and
unchanged source. This is 6.899 seconds below the immediate 86.128-second baseline
and 8.064 seconds below the original pre-registration 87.293-second baseline.
No full-backend retry was needed for this upload consolidation.

The old gate exceeded its unchanged 260-second budget by 8.8 seconds. The
standalone difference is smaller than that overrun and cannot be transferred
exactly to concurrent gate lanes. This result supports a useful observed
improvement, but does not establish that a new gate will fit. No new gate has
been attempted; a separate owner readiness decision is required.

## Independent reviews and static evidence

Separate fresh standard and full-security reviews examined the new delta and
its fixture/authorization/storage integration. Both found no blocker, major or
minor findings. They confirmed preserved inputs, assertions, cardinality,
identity/rate/quota isolation and per-child cleanup; no production trust boundary
changed. The standard review read the targeted/control logs. The security review
verified isolated copy diffs/manifests and mapped T-13/T-20/T-35/T-36/T-41; it did
not independently verify the later complete backend/static results. Those results
above and below are coordinating execution evidence, not reviewer-run checks.

Gofmt over the three changed Go files, go vet ./... and golangci-lint run ./...
all passed with exit 0. Tool provenance passed before each runtime batch.
Final documentation and staged secret scans are executed after this record is
written; their command logs are retained under ovc-upload-fixture-static-checks
in the private temporary directory. These scans do not authorize a merge.

## Owner readiness decision

On 2026-11-04 the owner authorized one new full gate on the consolidated tree,
accepting the limited uncertainty of the historical ERR_ABORTED. The explicit
proposal disclosed that the observed 8.064-second standalone improvement is
smaller than the previous 8.8-second gate overrun and does not guarantee the
260-second limit. All assertions, timeouts and the threshold remain unchanged.
The original failed commit and results remain preserved. A new coherent commit
may be created without rewriting history; the completed gate lock may be
released through the gate command. Only a passing gate authorizes fast-forward
merge and finish. A new failure stops progress without an automatic retry.
After success, remaining slice checks and closure work continue under their
approved scopes and device/session constraints.

The preceding final documentation and staged Gitleaks scans both passed with
exit 0. The gate decision is recorded before freezing the new candidate.
