# M2 after-fix evidence (2026-11-25)

This is the evidence for the `after-fix` row of change M2 of the interface rework plan, replies and the pinned message, in the [slice 10 known limitations](slice-10-known-limitations.md). It follows the Failures bullet of [ADR-0018](../../docs/decisions/0018-targeted-checks-and-merge-gate.md#decision).

## The failed gate

- **Tested tree:** `8b268da05afd0cb4f0ac6292801611a1bc6953b7`, commit `37d8947` on base `0d37d62`, branch `feat/messages-replies-pin`. The gate ran for 205.3 s.
- **Gate lines:**
  - `gate start: power: mains; load: 2.87 4.16 8.65`
  - `gate end: power: mains; load: 10.28 7.60 9.20; battery-during-run: no; sleep: no`
- **Command log:** `/private/tmp/claude-501/-Users-klukashka-Work-ovc/c744c45d-365b-4162-bc60-45db36f711a8/scratchpad/gate-m2.txt`, SHA-256 `8cc8d133d27bca7e9079cd809009c6863c27ec4716afbaba0da7271c705edb02`.
- **The one failure**, in `go test -race ./...` (lane A); the end-to-end lane did not run:

  ```
  --- FAIL: TestEveryScopedRouteHasADenialCase (0.03s)
      tutoring_test.go:560: PUT /api/v1/channels/{id}/pin has a path identifier and no denial case
      tutoring_test.go:560: DELETE /api/v1/channels/{id}/pin has a path identifier and no denial case
  ```

- **Match key:** package `ovc/backend/internal/api`, test `TestEveryScopedRouteHasADenialCase`, failure location `backend/internal/api/tutoring_test.go:560`.

## The fix

- **Hunk:** `backend/internal/api/tutoring_test.go`, lines 395 to 400 of the after tree. It adds the denial cases `PUT /api/v1/channels/{id}/pin` and `DELETE /api/v1/channels/{id}/pin` to `denialCases`, built with the existing `anotherTutorsMessages` helper.
- **What the new cases check:** a tutor who shares the student with the channel's tutor pins or unpins in that tutor's direct channel. The answer must be `404 not_found`, identical to the answer for a random channel identifier.
- **No application code changed,** and no test, helper or check configuration was loosened.

## Command

Both trees use the same command, through the main checkout's `tools/dev-env.sh`, with the pinned tools of `bin/` on `PATH`:

```
cd backend && go test -race -count=1 -v -run '^TestEveryScopedRouteHasADenialCase$' ./internal/api/
```

## Before tree

- **Tree:** `8b268da05afd0cb4f0ac6292801611a1bc6953b7`, the failed gate's tested tree. Its test is unchanged.
- **Result:** one run, exit 1. It reproduced the match key, so the before series is that one run:

  ```
      tutoring_test.go:560: DELETE /api/v1/channels/{id}/pin has a path identifier and no denial case
      tutoring_test.go:560: PUT /api/v1/channels/{id}/pin has a path identifier and no denial case
  --- FAIL: TestEveryScopedRouteHasADenialCase (0.03s)
  FAIL    ovc/backend/internal/api    4.596s
  ```

- **Log SHA-256:** `b1fd85edb5fcf8e6a7a854c9d2c819e6d4cb2767d5b58957aa39dd7f0813dcc8`.

## After tree

- **Tree:** `8379e22ae6c0d24f348e32373a1385e1919c87e2`, the failed tree plus only the denial-case hunk. The commit's tree also holds this record and the `after-fix` row.
- **Result:** 10 runs, all exit 0. Every run printed `--- PASS: TestEveryScopedRouteHasADenialCase`, `--- PASS: TestEveryScopedRouteHasADenialCase/PUT_/api/v1/channels/{id}/pin` and `--- PASS: TestEveryScopedRouteHasADenialCase/DELETE_/api/v1/channels/{id}/pin`. For example, run 1:

  ```
  --- PASS: TestEveryScopedRouteHasADenialCase (0.03s)
      --- PASS: TestEveryScopedRouteHasADenialCase/PUT_/api/v1/channels/{id}/pin (1.31s)
      --- PASS: TestEveryScopedRouteHasADenialCase/DELETE_/api/v1/channels/{id}/pin (0.59s)
  ok      ovc/backend/internal/api    5.683s
  ```

| Run | Exit | Log SHA-256 |
|---|---|---|
| 1 | 0 | `87c666d5a6bd48a2f83661d58f177149b61288a4f61965f4a1fb3c01a7fc0064` |
| 2 | 0 | `2ea8886779b7a25f8b53a98d77e7a8d8795945e1cfb057cb450b8c44b7f0fed7` |
| 3 | 0 | `acc055dcb1feac1a19c46a8c99aa64253f85142daea834e14e261c08459552e5` |
| 4 | 0 | `f851ea9a5b63e329e2a71c25619bfa355e577b35bcfb641bca41ab218bc7473b` |
| 5 | 0 | `3c950da803fbdfa500fd8d251198519634219db6cd3a4f6b5bbc07a55c20afe5` |
| 6 | 0 | `4f19931f1b89346863d36af41c67f6c439905423a697d19773c2c40466238ac5` |
| 7 | 0 | `cc887fa28119a6c83c9a13280ed2de336eb840d674fa43fd1b577c5292755c60` |
| 8 | 0 | `c502e310e5fe51e2437a13ae90bb7ebc6b767b7638377e3d2af0fa193743ebc4` |
| 9 | 0 | `aad8bef4782b25d983f4147d4b230c097582b859b2fd26c4638a7577aba72b95` |
| 10 | 0 | `0b2333d83a6e2d8b990519e54027127189d6e988e21254a86f9aa3c9e99a8246` |

## Change checks of the fix

All ran on the after tree through the main checkout's `tools/dev-env.sh`, and each exited 0:

- `gofmt -l .`
- `go vet ./...`
- `golangci-lint run ./...`
- `go test -race -count=1 ./internal/api/ ./internal/conventions/... ./cmd/...`: `ok ovc/backend/internal/api 148.993s`, `ok ovc/backend/internal/conventions 3.415s`, `ok ovc/backend/cmd/internal/startup 50.108s`
