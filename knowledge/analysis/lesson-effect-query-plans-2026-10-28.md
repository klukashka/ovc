# Lesson effect query plans

Checked on 2026-10-28 for the change that splits the lesson effect statements by scope and drives the roster listings of the aggregated emails from the roster's student index, which resolves the effect query findings of the [slice 5 review](../reviews/slice-5-review.md). Agent-produced evidence, pending owner review.

## Method

- **Database:** a fresh migrated test database on the Compose PostgreSQL 18, created through the integration-test fixture by a temporary test file in `internal/dbgen` that was removed afterwards and is not committed.
- **Data:** 100 tutors with 40 active relationships, 10 groups of 4 members and 10 one-to-one series (half of them with `ended_at` set) each, 200 lessons per relationship and per group (150 ended, 50 scheduled, about a tenth of the scheduled ones cancelled by the tutor), roster rows for every ended and cancelled one-to-one lesson; one tutor with 250 more lessons per relationship, all cancelled as `tutor_deleted` at one instant with their roster rows, about 12,300 lessons; 2,000 more tutors with 20 relationships and 5 groups each, for the member query; then `ANALYZE`.
- **Plans:** each statement prepared and explained with `plan_cache_mode` set to `force_generic_plan` and then to `force_custom_plan`; the reads also with `EXPLAIN (ANALYZE, BUFFERS)`. The statements before the change were explained on the same database, so with the new indexes present.

## Results

| Statement | Before the change | After the change |
|---|---|---|
| Cancel the lessons of an ending | one statement for every scope; its `OR` over the scope flags left a tutor index as the only access path, filtering every future scheduled lesson of the tutor | relationship statement: `lessons_relationship_starts_idx` on `(relationship_id, starts_at > instant)` |
| Cancel the lessons of an archiving | the same tutor-wide scan | group statement: `lessons_group_scheduled_idx` |
| Cancel the lessons of a tutor deletion | the tutor index over every future lesson, cancelled ones included | tutor statement: the new partial `lessons_tutor_scheduled_idx` |
| End the series of an effect | the `OR` statement through the tutor's series, locking series whose end date had passed | relationship and group statements through their own indexes; the tutor statement through the new partial `lesson_series_tutor_open_idx`; series whose end date has passed are no longer locked |
| Lessons under way of a tutor | (new) | `lessons_tutor_scheduled_idx` with `starts_at <= instant`, never reading the tutor's ended or cancelled lessons: 0.35 ms, 53 buffers |
| Group members of a tutor | a filter on `group_members.tutor_id`, a sequential scan on the small data set and the relationship index on the larger one | `groups_tutor_id_idx`, then the `group_members` key per group |
| `tutor.deleted` listing for one recipient | the cancellation index matched all 12,300 lessons of the deletion, with one roster probe each: 37,708 buffers, 23.7 ms | the recipient's 450 later roster rows through `lesson_roster_student_idx`, each lesson by its key: 2,674 buffers, 3.6 ms with the generic plan and 0.95 ms with the custom plan |
| `group.archived` listing | the same statement as above, narrowed by the group | the recipient's later roster rows, each lesson by its key: 300 rows, 1,325 buffers, 0.4 ms |
| `relationship.ended` listing | the same statement, narrowed by the relationship | `lessons_relationship_starts_idx` from the ending's instant combined with the cancellation index |

With both plan cache modes, a roster listing joined to the lessons without a lateral lookup was still planned from the cancellation index, because both plans estimate the lessons cancelled at one instant at a few hundred rows, not the 12,300 the deletion cancelled; the roster listings therefore look each roster row's lesson up in a lateral subquery by its key, which keeps the plan on the roster index.
