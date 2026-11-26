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

Measured by change E1d2.
