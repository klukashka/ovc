# Message search measurement (2026-11-25)

This record measures change M4 of the [interface rework plan](../plans/ui-rework.md), search in a conversation. It gives the query plan and time of a search at 50,000 messages in one channel, and decides whether a trigram index is needed under the owner's threshold, decided on 2026-11-25: add the index only if the route's 95th percentile exceeds NFR-2.1's read target of 200 ms, in the median of three samples, for the slowest shape.

## Setup

- **Code:** branch `feat/messages-search` at `3a79247` on main `4e3db81`, with fixes not yet committed: the measurement's placeholder substitution and test-only fixes. The search query, route and service are the ones under test.
- **Machine:** the owner's development computer, macOS 26.2, 12 cores, 24 GB of memory, on mains power. The 1-, 5- and 15-minute load was 3.54 3.66 3.22 at the start and 4.02 3.77 3.27 at the end. Nothing else ran.
- **Database:** the local PostgreSQL 18.6 container, with collation and character type `en_US.utf8`. This is the locale under which `lower()` folds Cyrillic, which `TestSearchFoldsCyrillicAndLatinCase` proves.
- **Test:** `TestMessagesSearchMeasurement`, build tag `messages_search_measurement`. It seeds one direct channel with 50,000 messages:
  - every 10th message holds "Frequent";
  - message 7 alone holds "seldomword";
  - "отсутствует" and the 2-character "щъ" occur nowhere.
- **Command:**

  ```sh
  tools/dev-env.sh bash -c 'cd backend && go test -tags messages_search_measurement ./internal/api -run "^TestMessagesSearchMeasurement$" -count=3 -parallel=1 -timeout=30m -v'
  ```

- **Result:** exit 0, `ok ovc/backend/internal/api 10.156s`, three `--- PASS: TestMessagesSearchMeasurement` lines.
- **What one sample does:** it runs `EXPLAIN (ANALYZE, BUFFERS)` of the real `SearchMessages` statement, read from `backend/queries/messages.sql`, for each shape. It then times 20 calls of `POST /channels/{id}/messages/search` per shape, after 3 warm-up calls, from the tutor. The shared clock moves past the rate-limit window before each batch, so no call is refused.

## Route timings

Times are milliseconds over the real HTTP route, including authentication, the read transaction, the message views and the JSON response.

| Shape | Sample 1 median / p95 | Sample 2 median / p95 | Sample 3 median / p95 | Median of the p95s |
|---|---|---|---|---|
| Frequent term, first page (30 of 5,000 matches) | 3.152 / 23.100 | 2.759 / 22.893 | 2.947 / 23.998 | 23.100 |
| Rare term, only the oldest message | 27.914 / 29.193 | 28.584 / 31.359 | 27.915 / 38.200 | 31.359 |
| No match | 28.320 / 29.407 | 29.467 / 30.269 | 29.322 / 30.203 | 30.203 |
| 2-character no match | 26.568 / 29.404 | 27.389 / 28.714 | 27.031 / 28.165 | 28.714 |
| Frequent term, deep cursor (seq below 5,000) | 5.645 / 6.442 | 5.739 / 6.688 | 6.121 / 6.987 | 6.688 |

## Decision

The slowest shape is the rare term that matches only the oldest message. Its median p95 is 31.4 ms, about a sixth of the 200 ms threshold. The 2-character no-match, which a trigram index could not serve, is 28.7 ms.

**No index is added**, under the owner's threshold of 2026-11-25. M4 adds no migration, no `pg_trgm` extension and no stack-table row.

## Observations

- **No-match plans:** without an index, a no-match search scans the channel's live messages: a sequential scan, or a backward scan of `messages_channel_seq_key` for the 2-character term. It costs about 17 to 19 ms of execution at 50,000 messages, so the cost grows roughly linearly with the channel's length.
- **The statement timeout:** 2 seconds, so it would end such a scan at roughly 60 times this length, which is far beyond any expected chat.
- **The frequent-term first page:** it sorts 5,000 matches before taking 30, because the planner estimates the `LIKE` selectivity poorly. This explains its p95 of about 23 ms against a median of about 3 ms.
- **These are development-machine figures** and do not certify the reference deployment. NFR-2.1's target applies to the reference deployment at MVP scale.

## Plans of the first sample

### `frequent_first_page`

```text
Limit  (cost=2028.50..2028.51 rows=5 width=273) (actual time=21.528..21.531 rows=31.00 loops=1)
  Buffers: shared hit=6059
  ->  Sort  (cost=2028.50..2028.51 rows=5 width=273) (actual time=21.528..21.529 rows=31.00 loops=1)
        Sort Key: messages.seq DESC
        Sort Method: top-N heapsort  Memory: 41kB
        Buffers: shared hit=6059
        ->  Nested Loop Left Join  (cost=8.18..2028.44 rows=5 width=273) (actual time=0.028..20.789 rows=5000.00 loops=1)
              Buffers: shared hit=6059
              ->  Nested Loop Left Join  (cost=0.00..1987.40 rows=5 width=264) (actual time=0.022..17.896 rows=5000.00 loops=1)
                    Join Filter: (senders.id = messages.sender_id)
                    Buffers: shared hit=1059
                    ->  Nested Loop  (cost=0.00..1960.77 rows=5 width=200) (actual time=0.019..17.282 rows=5000.00 loops=1)
                          Buffers: shared hit=1058
                          ->  Nested Loop Left Join  (cost=0.00..29.73 rows=1 width=56) (actual time=0.006..0.007 rows=1.00 loops=1)
                                Join Filter: (direct.id = channels.relationship_id)
                                Buffers: shared hit=2
                                ->  Seq Scan on channels  (cost=0.00..15.00 rows=1 width=56) (actual time=0.004..0.004 rows=1.00 loops=1)
                                      Filter: (id = '96aa5f98-a2d8-4623-bfff-ef1f30c2f490'::uuid)
                                      Rows Removed by Filter: 1
                                      Buffers: shared hit=1
                                ->  Seq Scan on relationships direct  (cost=0.00..12.10 rows=210 width=32) (actual time=0.001..0.001 rows=1.00 loops=1)
                                      Buffers: shared hit=1
                          ->  Seq Scan on messages  (cost=0.00..1931.00 rows=5 width=160) (actual time=0.012..16.984 rows=5000.00 loops=1)
                                Filter: ((deleted_at IS NULL) AND (erased_at IS NULL) AND (body IS NOT NULL) AND (channel_id = '96aa5f98-a2d8-4623-bfff-ef1f30c2f490'::uuid) AND (lower(body) ~~ '%frequent%'::text))
                                Rows Removed by Filter: 45000
                                Buffers: shared hit=1056
                    ->  Materialize  (cost=0.00..12.85 rows=190 width=80) (actual time=0.000..0.000 rows=1.00 loops=5000)
                          Storage: Memory  Maximum Storage: 17kB
                          Buffers: shared hit=1
                          ->  Seq Scan on users senders  (cost=0.00..11.90 rows=190 width=80) (actual time=0.002..0.002 rows=1.00 loops=1)
                                Buffers: shared hit=1
              ->  Limit  (cost=8.18..8.18 rows=1 width=41) (actual time=0.000..0.000 rows=0.00 loops=5000)
                    Buffers: shared hit=5000
                    ->  Sort  (cost=8.18..8.18 rows=1 width=41) (actual time=0.000..0.000 rows=0.00 loops=5000)
                          Sort Key: ((relationships.state = 'discarded'::text)), relationships.created_at DESC
                          Sort Method: quicksort  Memory: 25kB
                          Buffers: shared hit=5000
                          ->  Index Scan using relationships_student_id_idx on relationships  (cost=0.14..8.17 rows=1 width=41) (actual time=0.000..0.000 rows=0.00 loops=5000)
                                Index Cond: (student_id = messages.sender_id)
                                Filter: (tutor_id = channels.tutor_id)
                                Index Searches: 5000
                                Buffers: shared hit=5000
Planning:
  Buffers: shared hit=95
Planning Time: 0.232 ms
Execution Time: 21.550 ms
```

### `rare_oldest`

```text
Limit  (cost=2028.50..2028.51 rows=5 width=273) (actual time=17.096..17.099 rows=1.00 loops=1)
  Buffers: shared hit=1060
  ->  Sort  (cost=2028.50..2028.51 rows=5 width=273) (actual time=17.095..17.098 rows=1.00 loops=1)
        Sort Key: messages.seq DESC
        Sort Method: quicksort  Memory: 25kB
        Buffers: shared hit=1060
        ->  Nested Loop Left Join  (cost=8.18..2028.44 rows=5 width=273) (actual time=0.019..17.094 rows=1.00 loops=1)
              Buffers: shared hit=1060
              ->  Nested Loop Left Join  (cost=0.00..1987.40 rows=5 width=264) (actual time=0.016..17.090 rows=1.00 loops=1)
                    Join Filter: (senders.id = messages.sender_id)
                    Buffers: shared hit=1059
                    ->  Nested Loop  (cost=0.00..1960.77 rows=5 width=200) (actual time=0.013..17.086 rows=1.00 loops=1)
                          Buffers: shared hit=1058
                          ->  Nested Loop Left Join  (cost=0.00..29.73 rows=1 width=56) (actual time=0.006..0.007 rows=1.00 loops=1)
                                Join Filter: (direct.id = channels.relationship_id)
                                Buffers: shared hit=2
                                ->  Seq Scan on channels  (cost=0.00..15.00 rows=1 width=56) (actual time=0.004..0.005 rows=1.00 loops=1)
                                      Filter: (id = '96aa5f98-a2d8-4623-bfff-ef1f30c2f490'::uuid)
                                      Rows Removed by Filter: 1
                                      Buffers: shared hit=1
                                ->  Seq Scan on relationships direct  (cost=0.00..12.10 rows=210 width=32) (actual time=0.001..0.001 rows=1.00 loops=1)
                                      Buffers: shared hit=1
                          ->  Seq Scan on messages  (cost=0.00..1931.00 rows=5 width=160) (actual time=0.007..17.077 rows=1.00 loops=1)
                                Filter: ((deleted_at IS NULL) AND (erased_at IS NULL) AND (body IS NOT NULL) AND (channel_id = '96aa5f98-a2d8-4623-bfff-ef1f30c2f490'::uuid) AND (lower(body) ~~ '%seldomword%'::text))
                                Rows Removed by Filter: 49999
                                Buffers: shared hit=1056
                    ->  Materialize  (cost=0.00..12.85 rows=190 width=80) (actual time=0.002..0.003 rows=1.00 loops=1)
                          Storage: Memory  Maximum Storage: 17kB
                          Buffers: shared hit=1
                          ->  Seq Scan on users senders  (cost=0.00..11.90 rows=190 width=80) (actual time=0.001..0.002 rows=1.00 loops=1)
                                Buffers: shared hit=1
              ->  Limit  (cost=8.18..8.18 rows=1 width=41) (actual time=0.002..0.003 rows=0.00 loops=1)
                    Buffers: shared hit=1
                    ->  Sort  (cost=8.18..8.18 rows=1 width=41) (actual time=0.002..0.003 rows=0.00 loops=1)
                          Sort Key: ((relationships.state = 'discarded'::text)), relationships.created_at DESC
                          Sort Method: quicksort  Memory: 25kB
                          Buffers: shared hit=1
                          ->  Index Scan using relationships_student_id_idx on relationships  (cost=0.14..8.17 rows=1 width=41) (actual time=0.001..0.001 rows=0.00 loops=1)
                                Index Cond: (student_id = messages.sender_id)
                                Filter: (tutor_id = channels.tutor_id)
                                Index Searches: 1
                                Buffers: shared hit=1
Planning:
  Buffers: shared hit=11
Planning Time: 0.230 ms
Execution Time: 17.121 ms
```

### `no_match`

```text
Limit  (cost=2028.50..2028.51 rows=5 width=273) (actual time=16.794..16.795 rows=0.00 loops=1)
  Buffers: shared hit=1058
  ->  Sort  (cost=2028.50..2028.51 rows=5 width=273) (actual time=16.794..16.794 rows=0.00 loops=1)
        Sort Key: messages.seq DESC
        Sort Method: quicksort  Memory: 25kB
        Buffers: shared hit=1058
        ->  Nested Loop Left Join  (cost=8.18..2028.44 rows=5 width=273) (actual time=16.792..16.793 rows=0.00 loops=1)
              Buffers: shared hit=1058
              ->  Nested Loop Left Join  (cost=0.00..1987.40 rows=5 width=264) (actual time=16.792..16.793 rows=0.00 loops=1)
                    Join Filter: (senders.id = messages.sender_id)
                    Buffers: shared hit=1058
                    ->  Nested Loop  (cost=0.00..1960.77 rows=5 width=200) (actual time=16.792..16.792 rows=0.00 loops=1)
                          Buffers: shared hit=1058
                          ->  Nested Loop Left Join  (cost=0.00..29.73 rows=1 width=56) (actual time=0.005..0.006 rows=1.00 loops=1)
                                Join Filter: (direct.id = channels.relationship_id)
                                Buffers: shared hit=2
                                ->  Seq Scan on channels  (cost=0.00..15.00 rows=1 width=56) (actual time=0.003..0.004 rows=1.00 loops=1)
                                      Filter: (id = '96aa5f98-a2d8-4623-bfff-ef1f30c2f490'::uuid)
                                      Rows Removed by Filter: 1
                                      Buffers: shared hit=1
                                ->  Seq Scan on relationships direct  (cost=0.00..12.10 rows=210 width=32) (actual time=0.001..0.001 rows=1.00 loops=1)
                                      Buffers: shared hit=1
                          ->  Seq Scan on messages  (cost=0.00..1931.00 rows=5 width=160) (actual time=16.786..16.786 rows=0.00 loops=1)
                                Filter: ((deleted_at IS NULL) AND (erased_at IS NULL) AND (body IS NOT NULL) AND (channel_id = '96aa5f98-a2d8-4623-bfff-ef1f30c2f490'::uuid) AND (lower(body) ~~ '%отсутствует%'::text))
                                Rows Removed by Filter: 50000
                                Buffers: shared hit=1056
                    ->  Materialize  (cost=0.00..12.85 rows=190 width=80) (never executed)
                          ->  Seq Scan on users senders  (cost=0.00..11.90 rows=190 width=80) (never executed)
              ->  Limit  (cost=8.18..8.18 rows=1 width=41) (never executed)
                    ->  Sort  (cost=8.18..8.18 rows=1 width=41) (never executed)
                          Sort Key: ((relationships.state = 'discarded'::text)), relationships.created_at DESC
                          ->  Index Scan using relationships_student_id_idx on relationships  (cost=0.14..8.17 rows=1 width=41) (never executed)
                                Index Cond: (student_id = messages.sender_id)
                                Filter: (tutor_id = channels.tutor_id)
                                Index Searches: 0
Planning:
  Buffers: shared hit=11
Planning Time: 0.215 ms
Execution Time: 16.808 ms
```

### `two_character_no_match`

```text
Limit  (cost=9.04..619.66 rows=31 width=273) (actual time=19.300..19.302 rows=0.00 loops=1)
  Buffers: shared hit=1305
  ->  Nested Loop Left Join  (cost=9.04..7888.04 rows=400 width=273) (actual time=19.300..19.302 rows=0.00 loops=1)
        Buffers: shared hit=1305
        ->  Nested Loop Left Join  (cost=0.86..4605.04 rows=400 width=264) (actual time=19.299..19.301 rows=0.00 loops=1)
              Buffers: shared hit=1305
              ->  Nested Loop  (cost=0.71..4594.51 rows=400 width=200) (actual time=19.299..19.300 rows=0.00 loops=1)
                    Buffers: shared hit=1305
                    ->  Index Scan Backward using messages_channel_seq_key on messages  (cost=0.41..4573.10 rows=400 width=160) (actual time=19.298..19.298 rows=0.00 loops=1)
                          Index Cond: (channel_id = '96aa5f98-a2d8-4623-bfff-ef1f30c2f490'::uuid)
                          Filter: ((deleted_at IS NULL) AND (erased_at IS NULL) AND (body IS NOT NULL) AND (lower(body) ~~ '%щъ%'::text))
                          Rows Removed by Filter: 50000
                          Index Searches: 1
                          Buffers: shared hit=1305
                    ->  Materialize  (cost=0.29..16.41 rows=1 width=56) (never executed)
                          ->  Nested Loop Left Join  (cost=0.29..16.40 rows=1 width=56) (never executed)
                                ->  Index Scan using channels_pkey on channels  (cost=0.15..8.17 rows=1 width=56) (never executed)
                                      Index Cond: (id = '96aa5f98-a2d8-4623-bfff-ef1f30c2f490'::uuid)
                                      Index Searches: 0
                                ->  Index Scan using relationships_id_tutor_key on relationships direct  (cost=0.14..8.16 rows=1 width=32) (never executed)
                                      Index Cond: (id = channels.relationship_id)
                                      Index Searches: 0
              ->  Memoize  (cost=0.15..0.29 rows=1 width=80) (never executed)
                    Cache Key: messages.sender_id
                    Cache Mode: logical
                    ->  Index Scan using users_pkey on users senders  (cost=0.14..0.28 rows=1 width=80) (never executed)
                          Index Cond: (id = messages.sender_id)
                          Index Searches: 0
        ->  Limit  (cost=8.18..8.18 rows=1 width=41) (never executed)
              ->  Sort  (cost=8.18..8.18 rows=1 width=41) (never executed)
                    Sort Key: ((relationships.state = 'discarded'::text)), relationships.created_at DESC
                    ->  Index Scan using relationships_student_id_idx on relationships  (cost=0.14..8.17 rows=1 width=41) (never executed)
                          Index Cond: (student_id = messages.sender_id)
                          Filter: (tutor_id = channels.tutor_id)
                          Index Searches: 0
Planning:
  Buffers: shared hit=11
Planning Time: 0.239 ms
Execution Time: 19.325 ms
```

### `frequent_deep_cursor`

```text
Limit  (cost=1351.12..1351.12 rows=1 width=273) (actual time=2.703..2.705 rows=31.00 loops=1)
  Buffers: shared hit=2624
  ->  Sort  (cost=1351.12..1351.12 rows=1 width=273) (actual time=2.702..2.704 rows=31.00 loops=1)
        Sort Key: messages.seq DESC
        Sort Method: top-N heapsort  Memory: 41kB
        Buffers: shared hit=2624
        ->  Nested Loop Left Join  (cost=158.51..1351.11 rows=1 width=273) (actual time=0.088..2.625 rows=499.00 loops=1)
              Buffers: shared hit=2624
              ->  Nested Loop Left Join  (cost=150.33..1342.90 rows=1 width=264) (actual time=0.084..2.322 rows=499.00 loops=1)
                    Join Filter: (senders.id = messages.sender_id)
                    Buffers: shared hit=2125
                    ->  Nested Loop Left Join  (cost=150.33..1328.63 rows=1 width=200) (actual time=0.083..2.112 rows=499.00 loops=1)
                          Join Filter: (direct.id = channels.relationship_id)
                          Buffers: shared hit=1626
                          ->  Nested Loop  (cost=150.33..1313.90 rows=1 width=200) (actual time=0.079..1.880 rows=499.00 loops=1)
                                Buffers: shared hit=1127
                                ->  Bitmap Heap Scan on messages  (cost=150.19..1305.73 rows=1 width=160) (actual time=0.078..1.665 rows=499.00 loops=1)
                                      Recheck Cond: ((channel_id = '96aa5f98-a2d8-4623-bfff-ef1f30c2f490'::uuid) AND (seq < '5000'::bigint))
                                      Filter: ((deleted_at IS NULL) AND (erased_at IS NULL) AND (body IS NOT NULL) AND (lower(body) ~~ '%frequent%'::text))
                                      Rows Removed by Filter: 4500
                                      Heap Blocks: exact=102
                                      Buffers: shared hit=129
                                      ->  Bitmap Index Scan on messages_channel_seq_key  (cost=0.00..150.19 rows=4977 width=0) (actual time=0.066..0.066 rows=4999.00 loops=1)
                                            Index Cond: ((channel_id = '96aa5f98-a2d8-4623-bfff-ef1f30c2f490'::uuid) AND (seq < '5000'::bigint))
                                            Index Searches: 1
                                            Buffers: shared hit=27
                                ->  Index Scan using channels_pkey on channels  (cost=0.15..8.17 rows=1 width=56) (actual time=0.000..0.000 rows=1.00 loops=499)
                                      Index Cond: (id = '96aa5f98-a2d8-4623-bfff-ef1f30c2f490'::uuid)
                                      Index Searches: 499
                                      Buffers: shared hit=998
                          ->  Seq Scan on relationships direct  (cost=0.00..12.10 rows=210 width=32) (actual time=0.000..0.000 rows=1.00 loops=499)
                                Buffers: shared hit=499
                    ->  Seq Scan on users senders  (cost=0.00..11.90 rows=190 width=80) (actual time=0.000..0.000 rows=1.00 loops=499)
                          Buffers: shared hit=499
              ->  Limit  (cost=8.18..8.18 rows=1 width=41) (actual time=0.000..0.000 rows=0.00 loops=499)
                    Buffers: shared hit=499
                    ->  Sort  (cost=8.18..8.18 rows=1 width=41) (actual time=0.000..0.000 rows=0.00 loops=499)
                          Sort Key: ((relationships.state = 'discarded'::text)), relationships.created_at DESC
                          Sort Method: quicksort  Memory: 25kB
                          Buffers: shared hit=499
                          ->  Index Scan using relationships_student_id_idx on relationships  (cost=0.14..8.17 rows=1 width=41) (actual time=0.000..0.000 rows=0.00 loops=499)
                                Index Cond: (student_id = messages.sender_id)
                                Filter: (tutor_id = channels.tutor_id)
                                Index Searches: 499
                                Buffers: shared hit=499
Planning:
  Buffers: shared hit=11
Planning Time: 0.212 ms
Execution Time: 2.721 ms
```
