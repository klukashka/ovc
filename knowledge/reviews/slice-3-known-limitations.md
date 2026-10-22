# Slice 3 known limitations

Minor findings accepted during slice 3 instead of starting another review round ([ADR-0011](../../docs/decisions/0011-tiered-verification.md)). The slice review re-verifies each entry; each is fixed, accepted by the owner or carried into a named later slice.

| Finding | Severity | Change | Why it was accepted |
|---|---|---|---|
| The student list and its search read and sort all of a tutor's matching cards for every page, because the `state = ANY(...)` filter keeps PostgreSQL from using `relationships_tutor_list_idx` for the ordering | minor | Student cards and invitation links | The work is bounded by one tutor's cards; the slice review's performance pass measures the query plan on 50,000 relationships, and an index on `(tutor_id, created_at DESC, id DESC) WHERE state <> 'discarded'` is the likely fix |
| During the 30 days before a deleted student's purge, an ended card still shows that student's account name and email to the tutor | minor | Student cards and invitation links | This matches the current specification, which shows the linked account on `provisional`, `active` and `ended` cards until the purge; whether a deleted account should be hidden earlier is to be decided explicitly under [T-34](../../docs/threat-model.md) at the slice review |
