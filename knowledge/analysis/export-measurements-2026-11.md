# Export measurements

Status: agent record, started 2026-11-26 with change E1d1 of the [interface rework plan](../plans/ui-rework.md); the figures are added by the runs below, and the build-time measurement by change E1d2.

## Memory for one 100 MB file

The [export specification](../specs/files.md#building) requires that a build's memory does not grow with file size (NFR-2.3) and that E1 measures one 100 MB file once. `TestExportMemoryMeasurement` (build tag `export_measurement`, in `backend/internal/api/export_measurement_test.go`) seeds a tutor and a student through the real routes, uploads one incompressible library file, attaches it to a template, issues the template to the student and builds the student's export through the real export service against PostgreSQL and the local S3-compatible server. It runs twice per test run, for a 25 MB and a 100 MB file, and for each build records the peak of the runtime metric `/memory/classes/heap/objects:bytes` over its value just before the build, sampled every 10 ms, the bytes allocated during the build, the archive's size and its number of 16 MiB parts. The test fails when the peak for the 100 MB file exceeds the peak for the 25 MB file by more than 10 %.

Commands and the rules for the run are in the [development guide](../../docs/development.md#export-measurements). The smoke, with files of 10 MB and 2.5 MB, is never evidence.

| Run | Date | Tree | File | Peak heap over baseline | Allocated | Archive | Parts | Result |
|---|---|---|---|---|---|---|---|---|
| 1 | 2026-11-26 | commit `66d9583`, tree `903ca71` | 25 MB | 31.01 MB | 31.64 MB | 25.01 MB | 2 | passed |
| 1 | 2026-11-26 | commit `66d9583`, tree `903ca71` | 100 MB | 31.24 MB | 31.93 MB | 100.01 MB | 6 | passed |
| 2 | 2026-11-26 | commit `66d9583`, tree `903ca71` | 25 MB | 30.93 MB | 31.58 MB | 25.01 MB | 2 | passed |
| 2 | 2026-11-26 | commit `66d9583`, tree `903ca71` | 100 MB | 31.22 MB | 31.93 MB | 100.01 MB | 6 | passed |
| 3 | 2026-11-26 | commit `66d9583`, tree `903ca71` | 25 MB | 30.92 MB | 31.61 MB | 25.01 MB | 2 | passed |
| 3 | 2026-11-26 | commit `66d9583`, tree `903ca71` | 100 MB | 31.25 MB | 31.94 MB | 100.01 MB | 6 | passed |

One run of three samples (`-count=3 -parallel=1`), on mains power on an idle machine (load averages 1.91 and 1.73 at the start), through the main checkout's `tools/dev-env.sh`. The test passed every sample. The peak for the 100 MB file stays within 1.1 % of the 25 MB file's peak (at most 31.25 MB against 30.92–31.01 MB), well under the 10 % bound, so a build's memory does not grow with file size. The smoke (10 MB and 2.5 MB, `smoke=true`) also passed and is not evidence.

## Query cost to watch

A student's build reads each submission's detail in four passes (the file inventory, the Answers and Feedback sheets and the data copy's `submissions`) and each lesson's detail in four passes (the inventory, the Lessons sheet, the calendar and the data copy), each detail with several queries, in page transactions of up to 500 items; the Homework sheet and the `homework` array use the listing alone. No measurement counts queries yet; E1d2's build-time measurement is to record the number of queries per build, and if the build time nears the 2-hour budget, a later change batches the detail reads with the same predicates (found by the standard readiness review of change E1d1).

## Build time of the largest default-quota tutor export

The [export specification](../specs/files.md#building) requires the build time of the largest export a tutor's default quota allows to be measured once, with the decision rule that a build over 1 hour (half the 2-hour budget) leads to a later change that splits the files into a second archive, and that table bytes reaching 50 % of the 200 MB allowance make the allowance an owner question. `TestExportBuildTimeMeasurement` (build tag `export_measurement`, added by change E1d2) seeds one tutor through the real routes up to the default 5 GB file quota, with the bulk rows (cards, groups, lessons, issues, submissions and messages) inserted through repository helpers in the measurement file only, and builds the tutor's export through the real export service. It records the wall time, the peak heap, the archive size and part count, the rows per sheet, the bytes of `tables.xlsx` and `data.json`, and the share of the time spent streaming files. Which rows came from routes and which from helpers is stated with each run.

| Run | Date | Tree | Wall time | Peak heap | Archive | Parts | `tables.xlsx` | `data.json` | Streaming share | Result |
|---|---|---|---|---|---|---|---|---|---|---|
| 1 | 2026-11-26 | `41af16b` plus the staged E1d2 change | — | — | — | — | — | — | — | failed in seeding: the 100th issue answered `429`, because the seeding advanced the clock after every 100 issues while two issues seeded earlier in the same hour counted toward `IssuesPerHour`; no build ran (`--- FAIL: TestExportBuildTimeMeasurement (65.58s)`); the seeding now advances after every 50 issues |
| 2 | 2026-11-26 | `41af16b` plus the staged E1d2 change | 170.7 s (build), 246.5 s with seeding | 45.2 MB over baseline | 4,980,667,294 bytes | 297 | 4,468,089 bytes (1,429,577 compressed) | 41,918,094 bytes (3,683,848 compressed) | 0.609 | passed (`--- PASS: TestExportBuildTimeMeasurement (246.49s)`) |

Run 2 seeded 49 library materials of 100 MB each, attached to lessons with the first ten also in an issued template, and one chat file, one answer file and one corrected file of 25 MB each, all through the routes. Through repository helpers in the measurement file only, it seeded 300 cards, 5,000 lessons, 100,000 messages and the submitted state of the submissions. Through the routes, it created 30 groups of 10 members and 500 issues to those groups, which gave about 5,000 submissions instead of the plan's estimate of 500 issues to 300 students each.

Rows per sheet were students 302, groups 31, group members 302, lessons 5,049, series 0, issues 502, submissions 5,002, answers 1, reviews 2, library 52, messages 99,991 and files 52. The number of authorization queries is not measured, because there is no hook for it. The streaming share comes from progress samples at 10 s resolution and is approximate.

The decision rules give these results:

- **Build time.** 170.7 s is far under 1 hour (half the 2-hour budget), so no split into a second archive is needed.
- **Table bytes.** `tables.xlsx` and `data.json` together hold about 46 MB, under 50 % of the 200 MB allowance, so the allowance is not an owner question.
- **Peak heap.** 45 MB, so it does not grow with the 5 GB of files.

The repeated issue and submission reads stay well inside the budget at this workload. A tutor with 500 issues to 300 students each would read about 30 times more submission details. That case is not measured; by these figures it would still stay under 1 hour if the time scaled linearly with the submission count.
