# Design slice step D6: Join on Today and Home

Status: task plan approved by the owner on 2026-11-09 after its security-profile planning review, with the join lifecycle moved into a shared module. On approval it is committed as `knowledge/plans/design-step-d6.md` in this step's change. It carries out step D6 of the [interface decisions](ui-ux-decisions-2026-11-06.md#order-of-work) and the [design slice plan](design-slice.md), whose section 3 lists D6's threat obligations. Base: `main` after step D5 merges. Precondition met: stage 4 of the [slice 7 local plan](slice-7-local-completion.md#4-lesson-ui-and-local-pilot-scenarios), the lesson room interface, merged as `3a996bb`, with follow-ups `a87666e` and `0a16bd1`.

## 1. Problem and outcome

**Problem.** Join exists only on the lesson page, so:

- a tutor opens Today, then the lesson, then joins;
- pilot scenario 1 steps 9 and 11 expect Join on Anna's Home and are proven on the lesson page instead.

**Outcome.** Join and its states appear on Today (tutor) and Home (student). They follow the [lesson page's state table](../design/web/pages.md#lesson) and use its join lifecycle, through a new tab only. Embedding stays on the lesson page, test-only as today.

- **Tutor, Today:**
  - each of today's lessons in its window (`scheduled`, from 10 minutes before the start) or `live` shows "Join lesson" or "Rejoin";
  - the next lesson that is not yet in its window shows "Room opens at {time}";
  - a cancelled or ended lesson shows no control.
- **Student, Home** (the next-lesson panel):
  - "Room opens at {time}" before the window;
  - "The room opens when your tutor joins" in the window, and while the lesson is `live` with its room closed;
  - "Join" only while the lesson is `live` with `room_open`, as the state table requires;
  - updates through live refetch without a reload.
- **One button through the whole join**, as on the lesson page:
  - "Join…" becomes "Joining…" (`aria-busy`, refusing presses, announced politely), then "Open again", with focus kept;
  - a refusal shows the lesson page's state or message:
    - `lesson_not_live` gives the waiting text, through the refetched detail;
    - `not_found` gives "You can't join this lesson" and no button, held for that lesson;
    - `outside_join_window` and `provider_unavailable` give their messages;
    - `429` and the rest give the shared refusal text.

## 2. Verification tier and security trigger

Standard tier. The security trigger is **met**: Join drives join links and the meeting provider (T-10, T-20, T-21, T-25, T-27, T-44). This plan had a security-profile planning review, whose findings are folded in. The change gets one standard reviewer and one security-profile reviewer, and updates [the threat model](../../docs/threat-model.md) in the same change. Evidence comes from the fake provider and is reported as `unverified` for a live server.

How the design slice plan's obligations are kept:

- **T-10, T-21, T-44.** The API alone decides who joins. The page hides Join where the API would refuse and never grants anything. Every refusal (`403`, `404`, `409`, `429`, `503`) is handled as on the lesson page. A `live` lesson seen only through a frozen roster can show Join; the press answers `404`, the tab closes and "You can't join this lesson" replaces the button.
- **T-20, the data source.**
  - Today reads `/calendar` only and issues no `GET /lessons/{id}`: the tutor's offer needs only `status` and the times.
  - Home reads one `GET /lessons/{id}`, for its single next lesson only, and only while the calendar shows it in its window or `live`. That bounds it to one detail read per page. The read is refetched by the lesson's `entity.changed` frames as on the lesson page, so `room_open` comes from the detail.
- **T-25, T-27.**
  - Join is a `POST` on an explicit click, never on load or through a link.
  - The blank tab opens synchronously on the click, with its opener cleared before navigation.
  - The tab closes on a failed join, on `pagehide`, on unmount, on an account change (which unmounts the page through the signed-out state) and when the control's lesson changes.
  - A late answer is dropped and closes its tab.
  - The join address is used only for that tab's navigation, never stored in the query cache, Web Storage, history state, an OVC address or diagnostics.
  - Only addresses that `isRoomAddress` accepts are opened.
  - `Referrer-Policy: no-referrer` already holds (`frontend/security-headers.ts`, backend middleware).
- **Provisional join is settled.**
  - The join rule admits a `provisional` student for a one-to-one lesson ([schedule](../specs/schedule.md#join-rule), ADR-0014), and `POST /lessons/{id}/join` is `unverified ok` ([API](../specs/api.md#schedule)), since the slice 7 lifecycle change `e949f14`, whose backend test `testJoinProvisional` covers it.
  - The end-to-end test below proves it for an unconfirmed account.
  - The design slice plan's "left to the owner before D6" note is marked resolved in this change.

## 3. Question for the owner, asked with this plan

1. **Sharing the join lifecycle.** The recommended option: move `useRoomEntry` and its refusal mapping out of `LessonRoom.tsx` into `features/schedule/roomEntry.ts` unchanged, and give Today and Home a narrower wrapper, `useTabJoin(lessonId)`, that can only join through a new tab. The lesson room's own tests must pass unchanged. The alternative is a copy for Today and Home, which would duplicate the lifecycle slice 7 reviewed.

## 4. What changes

- **New `features/schedule/roomEntry.ts`:**
  - `useRoomEntry`, `refusalOf` and the refusal messages, moved unchanged;
  - `useTabJoin(lessonId)`, which joins only through `POST /lessons/{id}/join` in a new tab and never offers restart or embedding.
- **`features/schedule/LessonRoom.tsx`:** imports them; no behavior change.
- **New `features/schedule/JoinControl.tsx`:**
  - a compact control mounted with `key={lessonId}`, so a change of lesson remounts it and abandons a held join;
  - its offer comes from `phaseOf` and `roomOfferOf`: the tutor's from the calendar item, with `room_open` taken as `status === 'live'`; the student's from the lesson detail;
  - `useLessonClock` re-renders it at the window's boundaries;
  - it holds "You can't join this lesson" for its lesson.
- **`features/home/TodayLessons.tsx`:**
  - the control renders as a sibling of each row's link inside the list item, never inside the link;
  - "Room opens at {time}" appears on the next row before its window;
  - the shared `LessonList` used by the Schedule is not changed.
- **`features/home/HomeLessons.tsx` (`NextLesson`):** the detail read for the next lesson while it is in its window or `live`, and the control or the texts.
- **`lib/i18n`:** reuse the lesson page's keys, adding only what is missing.
- **Tests:**
  - **`e2e/today.spec.ts`:**
    - "Join lesson" while a lesson's window is open, and "Room opens at {time}" with no button before it;
    - clicking opens the fake room in exactly one new tab whose opener is `null`, the address stays unchanged, and the button reads "Open again" with focus;
    - a cancelled lesson shows no control;
    - Today issues no `GET /lessons/{id}`.
  - **`e2e/home.spec.ts`:**
    - the waiting text and no button in the window; "Join" appears without reload once the tutor joined; the fake room opens as viewer with her name;
    - an unconfirmed student with a provisional link joins from Home;
    - a `reclaim_pending` account's Home shows no lesson and no Join;
    - a live lesson seen only through the roster after the relationship ended, and after a group removal: pressing Join closes the tab and shows "You can't join this lesson" with no button;
    - future lessons cancelled by an ending are not the next lesson;
    - Home issues at most one `GET /lessons/{id}`, for the next lesson;
    - during a held join, an account change closes the tab and the next account sees no data of the previous one;
    - no diagnostics record contains the join address.
  - **Pilot scenario 1 steps 9 and 11:** the existing `lesson-room.spec.ts` test `pilot-1 › steps 8–11` is changed so that Anna's steps 9 and 11 run on Home, keeping every assertion. Maria's steps 8 and 10 stay on the lesson page. No two tests claim the same steps.
  - **Component tests for `JoinControl` and `useTabJoin`, with the fake clock and the fake transport:**
    - offers per phase and role;
    - `outside_join_window`, `lesson_not_live`, `not_found` and `429` with `Retry-After`, each closing the tab;
    - a join held while the control's lesson changes (a filter change, a cancellation) closes the tab, which never navigates;
    - `pagehide` closes every held tab of several controls on Today;
    - no frame is rendered.
- **Specifications and the threat model** (pending owner review):
  - **`knowledge/design/web/pages.md`:** Today and Home Join, replacing "Join and Room opens at arrive with step D6"; that each Today row's control is independent, under the shared rule on several controls; that Home's student Join keeps the state table's `room_open` rule.
  - **`knowledge/design/web/pilot-scenarios.md`:** scenario 1 steps 9 and 11 on Home.
  - **`knowledge/plans/design-slice.md`:** the provisional note marked resolved.
  - **`docs/threat-model.md`:**
    - T-10 and T-27: the new entry points reuse the same lifecycle.
    - T-20: Home's one bounded detail read.
    - T-21: Today and Home as entry points for lessons seen through a frozen roster, refused with `404`.
    - T-25: the same tab rules, with the new tests cited.
    - T-44: a provisional student joins one-to-one live lessons from the lesson page and Home; the provider receives the account-chosen display name; a reclaim or removal stops further joins but not a connected session until the tutor restarts the room.

## 5. Acceptance criteria

1. In Chromium, Firefox and WebKit, `today.spec.ts` › the tutor's Join while the window is open passes; it shows "Room opens at {time}" and no button before the window, and no control on a cancelled lesson.
2. In all three browsers, `lesson-room.spec.ts` › `pilot-1 › steps 8–11` passes with Anna's steps on Home: the waiting text and no button in the window; Join without reload once the tutor joined; the fake room opens as viewer.
3. Clicking Join opens exactly one new tab whose opener is `null` and whose address is the fake room's. The page's address does not change, and the button reads "Open again" with focus.
4. A provisional, unconfirmed student joins from Home. A `reclaim_pending` account's Home shows no Join.
5. A live lesson seen only through the roster, after an ending and after a group removal: the press closes the tab and shows "You can't join this lesson" with no button.
6. During a held join, an account change closes the tab and the next account sees nothing of the previous one. A change of the control's lesson also closes the tab, proven by component test.
7. Today issues no `GET /lessons/{id}`; Home issues at most one, for its next lesson.
8. No diagnostics record, query cache entry or Web Storage value contains the join address.
9. The `JoinControl` and `useTabJoin` component tests pass, and the lesson room's existing component tests pass unchanged.
10. The change checks pass, including the whole `pnpm e2e` and `tools/docs/check-docs.sh`. The readiness report states that live-server evidence is `unverified`.

## 6. Order of work

1. Worktree on `feat/design-step-d6-join` from `main`.
2. Move the lifecycle into `roomEntry.ts` and add `useTabJoin`; the lesson room tests pass.
3. `JoinControl` with its component tests.
4. Today and Home, the end-to-end and pilot tests.
5. The specifications and the threat model.
6. Change checks, the standard and security reviewers, fixes under the after-fix rule, the readiness report, the gate, the merge and finish.

## 7. Observability

None. No backend change; the join route's existing logging covers the new entry points, and the frontend logs nothing about joins.

## 8. Risks

- **Rate limits.** Repeated presses after refusals count against the 30-per-minute join limit. The refusal states remove the button where the lesson page does, which bounds this.
- **Several controls on Today.** Each row has its own attempt; two rows can each open one tab on explicit clicks.
- **Slice 7 coupling.** The move touches the lesson room file, and its tests are the guard. The pilot test change keeps every assertion.

## 9. As built

- The Join control renders nothing for a cancelled or ended lesson, so a join held while the lesson is cancelled or ends closes its tab; when the removed control had focus, focus moves to the page heading.
- Home shows the load failure with Retry when the next lesson's detail cannot be read, instead of falling back to the calendar.
- The lesson page's join lifecycle moved unchanged into `roomEntry.ts`; the only edits are wider parameter types.
- Live-server evidence is `unverified`: every end-to-end test runs against the fake provider.
- Known limitations of this step are in `knowledge/reviews/slice-design-known-limitations.md`.
