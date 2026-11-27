# Task plan: R3a, the Messages chat list with avatars, four filters and list read ticks

Branch: `feat/redesign-message-list`, cut from `main` when code starts (planned against `cf8b847`). Revision 4 (2026-11-27), approved by the owner on 2026-11-27, applies the further scoped review of revision 3 (1 major, 3 minor); revision 3 applied the scoped re-review of revision 2 (1 major, 7 minor); revision 2 applied the standard planning review (7 major, 7 minor) and the security-profile planning review (2 major, 5 minor) of revision 1; changes are listed at the end. After approval this plan is committed as `knowledge/plans/r3a-message-list.md` in the change. Row R3a of the interface rework plan's redesign rows (revision 8); scope from section 7 R3a of the [visual redesign plan](redesign.md), the owner's decisions 5, 6, 14 and 16 of its section 3, and the mockups `messenger.html` and `messenger-phone.html`.

**Reference study:** `knowledge/research/ui-reference-messages-2026-11-27.md`, on `main` since `b5757fd` with its row RS-messages (revision 10 of the interface rework plan), as ovc-verify requires. It compares Telegram, WhatsApp, Slack and Signal; this plan resolves its R3a open points and carries its R3b open points to the owner as Q4 and Q5.

## Problem

The chat list shows a label, a presence dot, an unread count, a preview with its sender and the time, with search by label and no filters, avatars or list read state (`pages.md` Messages, `ChannelList.tsx`). The owner decided on 2026-11-27 (redesign plan, section 3, item 5) a Telegram-like list: avatar rows with the last message, time, unread badge and read ticks, and the filters All / Students / Groups / Unread. Section 7 adds that filters cover the whole authorized result set, not loaded pages; that students receive no additional peer identity, roster or group read receipts; and that a separate backend branch is needed if missing fields make the scope too large. The missing pieces are small (a filter parameter, one field and one batch query), so they stay in R3a.

## Scope

**In R3a:** initials avatars (decision 1); the four filters, applied by the server over all of the caller's channels, composing with search and pagination, carried through Messages navigation (decisions 2–4); a compact two-line row (decision 5); read ticks on the caller's own last message in direct rows only (decision 6); per-filter empty states; the specifications, the threat model (T-6, T-21, T-44 clauses), the generated client and tests.

**Not in R3a:** the separator and list width (R1b; rows are designed for 280–480 px and today's 300 px); the message menu, reactions, Copy text, emoji and round Send (R3b); photos (Q1 B); a stored filter (Q3 B); search term persistence of any kind (it stays in component state as today); "Mark as unread"; phone layout beyond the rows at 360 px (PHONE); Notifications counts.

## Facts the plan relies on (checked on `main` `cf8b847`)

- `GET /api/v1/channels` (`chat_routes.go:120-122`, `:400-411`) takes `q` (≤ 254), `limit` (1–100, default 30) and `cursor`; `channelItemBody` `:457-479`. `chat.List` (`chat.go:173`), `listChannels` `:216-251` asks for `limit + 1`, builds `next_cursor` from `(last_activity_at, channel_id)`, batch-loads last messages (`Messages.LastOf`, `messages.go:251`). `ListIn` (`chat.go:191`) feeds the student export (`services/export/student_source.go:163`).
- `ListChannelsOfMember` (`channels.sql:174-213`) reads `active_channel_members`, filters by optional channel id, role-dependent `pattern` (`:202-207`) and keyset; no kind or unread filter. `CountUnreadChannels` `:215-220` (cap 100) feeds `ChannelPage.unread_channels`.
- `ChannelItem` (`schemas/chat.go:26-39`) and `ChannelLastMessage` (`:16-24`; no read state). `ChannelDetail` embeds `ChannelItem` (`:62`, `GET /channels/{id}`, `chat_routes.go:430`); both exports serialize `ChannelItem` (`schemas/export_student.go:76`, `schemas/export_tutor.go:166`) through `services/export/student_views.go:236-252`. No avatar field in channel responses.
- For a tutor `colour` is the card's or group's colour; for a student it is `OwnColour`, the student's relationship colour (`chat.go:334`, `:344`); it is nullable (`schemas/chat.go:31`).
- Read state (M3): `channel_members.shared_read_seq` (`00047_read_ticks.sql:3`), not projected by `active_channel_members` (`00015_chat_channels.sql:42-48`). `GetPeerReadSeq` (`messages.sql:335-339`) reads the other row from `channel_members` with `channels.kind = 'direct'`, so a suspended row shows its frozen value (`realtime-and-video.md:91`); it is exempt by name from `TestEveryPathReadsMembersThroughTheActivePredicate` (`repositories/channels_test.go:35-41`, `:63`, `:156-165`). Frozen and lowered markers under reclaim and read-only: T-44 (threat model line 90) and T-21 (line 67). Existing tick tests for read-only, suspended and reactivated rows: `chat_read_ticks_test.go:284`, `:307`, `:332`. The conversation hides ticks on a deleted or erased own message (`MessageRow.tsx:52`).
- A `read.updated` frame reaches the other member of a direct channel (`realtime-and-video.md:197`, `:248`); on the client `read` schedules the coalesced refetch that invalidates first channel pages at most once a second (`refetch.ts:40-53`, `:81-83`).
- Labels (`realtime-and-video.md:29-47`): a student sees another student's name only on that student's messages (`:47`); the list preview already shows the last message's sender label (`ChannelList.tsx:179-189`, `views.go:82-104`).
- Avatars: `GET /avatars/{id}` is public by design (`files.md:234`); no route gives a student another student's `avatar_id`. `components/ui/Avatar.tsx` renders an image or one initial (`Array.from(name.trim()).at(0)`), `aria-hidden`, `bg-hover`; it is imported only by feature pages (lazy chunks), not by the frame.
- Client: search is component state, debounced 200 ms (`ChannelList.tsx:21-37`); first page key `['channels', q, null]` (`firstPage.ts:28-41`), continuation `['channels', q, scope, cursor]` (`:85-93`), detachment `:65-84`, de-duplication `:96-103`, removed-row focus `:105` (`liveRows.ts:41-49` sends a lost focus to `main h1`), pagination focus `:123-138`; row links `/messages/${id}` (`:155`); `/messages` and `/messages/:channelId` are separate routes (`router.tsx:106`); `RouteFocus` reacts to the pathname only (`RouteFocus.tsx:5-14`); `evictChannel` filters every cached `channels` page (`cache.ts:12-33`). The only list empty text is "No chats" (`texts.en.ts:4`, `texts.ru.ts:6`), shown for search too (`ChannelList.tsx:150`).
- Access records use route templates and the proxy replaces query strings (`observability.md:59`, `:62`).
- Tests: `messages.spec.ts:126`, `:410`, `:457`, `:513-540`, `:948-990`, `:992`; `message-attachments.spec.ts:139`; `desktop-layout.spec.ts:976-1039` (360 px sweeps, markup at `:1039`); `realtime-contract.spec.ts:203`; `home.spec.ts:488` (request list); Vitest `channelListScope.test.ts`, `MessagesPage.test.tsx:347-454`, `:864`; Go `chat_routes_test.go:79`, `:143`, `:164`, `:326`, `chat_read_ticks_test.go:100`, `:128`.
- Entry bundle 182.6 of 184.0 KB (`knowledge/reviews/u2-check-bundle-after-fix-2026-11-23.md:39-48`); Messages is a lazy route chunk with its own texts (ui-rework D12).

## Decisions proposed

1. **Avatar (Q1 = A recommended): initials from the existing `Avatar` component.** R3a extends `Avatar` rather than adding a second initials component: an optional `colour` prop (the row's existing `colour` as returned, no new colour field): the fill is that colour mixed 16 % into `--color-surface` with `color-mix(in srgb, …)` and the initial is drawn in `--color-text` (the colour itself on its tint falls to 3.75–4.31:1 for 8 of 16 colour-and-theme pairs; `--color-text` on the tint is at least 12.2:1 in the light theme and 9.3:1 in the dark theme for every palette colour on `main` at `3743ebb`; the earlier figures of 11.3:1 and 9.1:1 were measured before the palette change R0). The value comes from a new `Readonly<Record<PaletteColour, string>>` table in `components/ui/palette.ts` beside `paletteDotClass`, selected by the typed colour, so no runtime string reaches CSS; any unlisted value falls back to the neutral style; `null` keeps today's neutral `bg-hover` with `text-text-muted`, a `medium` size of 40 px for list rows, and a safer initial: the first grapheme (`Intl.Segmenter`) of the label after removing whitespace and `Cf` format characters, uppercased; one letter, as on every other page. A `label_kind` row (`deleted_user`, `former_student`, `removed_student`) or an empty result shows a neutral person glyph and never letters from any other field. Text node only, inside `dir="auto"`, `aria-hidden` (the row's label is its name). `frontend/scripts/palette.test.ts` gains "the text colour on each relationship colour's 16 % sRGB tint of surface reaches 4.5:1 in both themes", computing the mix in sRGB as the CSS does; the test reads the tokens, so it also guards R0's palette change. Pages already using `Avatar` get the safer initial and no visual change otherwise.
2. **Filters: `filter` on `GET /channels`, enum `all | direct | group | unread`, default `all`.** `ListChannelsOfMember` gains optional `kind` and `unread_only` arguments (`sqlc.narg`) applied before the keyset and limit, so pages and `next_cursor` stay inside the filtered set. Unknown values are `422` with the existing validation shape. `direct` is role-neutral; the interface names it per role (Q2). `q` composes in the same statement. `unread` means `last_seq > last_read_seq` at query time; a chat read while paging may leave later pages (cursor detachment and de-duplication keep the list consistent), never duplicate. `unread_channels` stays the unfiltered capped count. The export passes `all`.
3. **Filter control and address.** Four toggle buttons above the search field in a `role="group"` named "Chat filters", each with `aria-pressed`; All pressed by default. Unread shows `unread_channels` as a badge ("99+" above 99, hidden at zero) and its accessible name includes it ("Unread, 3"). **Only the filter goes in the page address**, as `?filter=direct|group|unread` (absent for All). The client parses it into the four-value enum; any other value is treated as All, is not sent and is never rendered. **The search term never enters the address, history or storage**; it stays in component state as today. Nothing is written to browser storage (Q3 = A).
4. **Filter through Messages navigation and focus.** Row links go to `/messages/{id}?filter=…` and Back to the list to `/messages?filter=…`; the list beside or before a conversation reads the filter from the address, so opening a chat in either layout keeps the filter. Choosing a filter is a `navigate` with `replace: false` that keeps focus on the pressed button. After a history-driven change of only the filter (Back or Forward), focus stays where it was if that element still exists; if it was inside the list, `ChannelList` moves it to the pressed filter button in a layout effect on the filter change. That effect runs after the commit that removed the old pages (focus is then on `body`) and before the hook's deferred heading focus, which is a passive-effect cleanup that queues a microtask guarded by `focusIsLost()` (`liveRows.ts:34-52`); the effect acts only when `focusIsLost()` holds and `ChannelList`'s own `focusin` record says focus was inside the list. The shared `useFocusAfterRemovedRow` hook (used by 19 components) is not changed. Query keys gain the filter through one key builder in `keys.ts` (`channelPageKey(q, filter, scope, cursor)`, giving `['channels', q, filter, null]` and `['channels', q, filter, scope, cursor]`), used by `firstPage.ts`, `ChannelList.tsx` and the tests; `ChannelPages` is keyed by `q` and `filter`. The coalesced refetch still invalidates only first pages (its predicate tests `queryKey.at(-1) === null`), and `evictChannel` still filters every cached page. No client-side filtering of loaded rows, no per-row fetch.
5. **Row layout.** Two lines inside the existing link: line 1 the label (bold while unread, ellipsis) and the time at the end; line 2 the preview with today's sender prefix and at the end the unread badge or, for one's own last message in a direct row, the ticks. Presence, a student's group-row tutor label, "unconfirmed" and "read-only" keep their wording; a read-only row is dimmed and keeps its text. Compact time in the caller's timezone: `HH:mm` today, the short weekday within the past six days, else `d MMM` (with the year outside the current year); the full value stays in `<time datetime>`. Row height ≥ 56 px; rows fit 280 px without horizontal overflow.
6. **List read ticks: `ChannelLastMessage.peer_read`, `boolean | null`, from a new batch query.** `ListPeerReadSeqs(channel_ids uuid[], viewer uuid)` mirrors `GetPeerReadSeq` exactly (`channels.kind = 'direct'`, the other member row read from `channel_members` itself, so a suspended row's frozen value is returned and a gone row gives no result). It is called once per page, only with the ids `ListChannelsOfMember` already returned for this caller. Its name is added to `mirrorAndPurgeQueries` in `repositories/channels_test.go`: an approved addition to the exemption list, not a removed assertion; `ListChannelsOfMember` stays under the rule. `peer_read` is non-null only when the last message is the caller's own, not deleted or erased, the channel is direct and the other row exists; it is `true` when that row's `shared_read_seq ≥ last_message.seq`. It is therefore exactly `peer_read_seq ≥ seq` from the channel detail, frozen while the channel is read-only or the peer row suspended, and `null` in groups. **`GET /channels/{id}`** computes it by the same rule from its own `peer_read_seq`. **Both data exports** carry `peer_read: null` always (set explicitly in the export views; their specification says so). Ticks render as in `MessageRow.tsx:52-53` with the same hidden words "Sent" / "Read". A peer's read reaches the list through the existing frame and first-page refetch; continuation-page rows update at their next refetch.
7. **Empty states.** Per filter, inside the list region, without moving focus: "No unread chats", "No group chats", "No chats with students" (tutor) / "No chats with tutors" (student, Q2 = A); with a search term, and under All, the existing "No chats" stays.
8. **The open chat under Unread.** Opening a chat from Unread marks it read, and the next first-page refetch removes its row while the conversation stays open; this is accepted and specified in `pages.md` (the conversation keeps focus; if focus was on that row, the existing removed-row rule applies). The rendered review checks it.
9. **Phone (below 768 px).** The same rows and filters in the one-pane list; the filter row scrolls horizontally inside its own box at 320 px, never the page.

## Questions for the owner

**Answered by the owner on 2026-11-27** (multiple-choice question, with approval of revision 4): Q1 = A, Q2 = A, Q3 = A, Q4 = A, Q5 = A, and no further planning reviewer after revision 4. The options are kept below for the record; Q4 and Q5 are carried into R3b's task plan.

Each is a multiple-choice question with a recommended option. The answers are recorded in this plan's committed copy and, for Q4 and Q5, carried into R3b's task plan, which closes those study points.

**Q1. Row avatars.** A (recommended): initials on the existing relationship colour only; no API change, no photo request. B: photos where the person may already see them (a tutor sees a student's photo under the card's rules, a student sees their tutor's; group rows and others' rows show initials), adding `avatar_id` to direct rows, image requests from the list and a T-6/T-37 review. C: no avatars, as today.

**Q2. The second filter for a student.** A (recommended): the tutor sees "Students", the student sees "Tutors" («Репетиторы»). B: the student sees only All / Groups / Unread. C: "Students" for both roles.

**Q3. Remembering the filter.** A (recommended): only in the page address (Back, reload, link), nothing stored. B: also stored per device, adding a preference key, the storage contract and a T-37 amendment.

**Q4 (for R3b). The reaction row's six emoji.** The mockup draws thumbs up, heart, laughing, surprised, folded hands and clapping; M5 built thumbs up, heart, joy, wow, sad and check, stored as an enum. A (recommended): keep M5's six. B: replace sad and check with folded hands and clapping, changing the enum, the OpenAPI schema and stored reactions (a migration mapping or removing existing ones).

**Q5 (for R3b). Edit and Delete in the context menu.** The mockup's menu shows the reaction row, Reply, Copy text and Pin only. A (recommended): Edit stays, and Delete stays last after a separator with its confirmation, in both the context menu and the visible "Message actions" button (decisions 5 and 16). B: Edit and Delete only through the visible button.

## Contracts, documents and threat model

- `knowledge/specs/api.md`: `filter` on `GET /channels` (values, default, `422`); `peer_read` on `ChannelLastMessage` in the list, the `GET /channels/{id}` row and messenger fields; the OpenAPI document and generated client (`api:check`).
- `knowledge/specs/files.md`, the export content section (how a build reads `GET /channels` into `data.json`): `peer_read` is always `null` in both exports; the export schemas in the OpenAPI document are checked by `api:check`.
- `knowledge/specs/realtime-and-video.md`: the list's `peer_read` equals `peer_read_seq ≥ seq` of the detail, null in groups and for deleted or erased last messages, refreshed by the existing frame and first-page refetch; the filter applies before pagination over the caller's active memberships.
- `knowledge/design/web/pages.md` Messages and the student variant: rows, filters, the filter in row links and Back, focus after a filter change and after history, the open chat under Unread, empty states, compact time, ticks, targets; the search term stays out of the address. `states.md`: the new texts. `tokens.md`: the 40 px avatar size.
- `docs/threat-model.md` (pending owner review), status line amended:
  - T-6, chat clause: "List rows carry read state only for the caller's own, not deleted, last message in a direct channel, derived only from the other member row's `shared_read_seq`, the same value as `peer_read_seq` on the channel detail, so it stays frozen while the channel is read-only or the student's row is suspended and is null once the other row is gone; it is computed only for rows the caller's membership query returned and never in a group. Filters resolve only the caller's active memberships; only the closed filter value enters the page address."
  - T-44 and T-21: the frozen and lowered markers also govern the list's `peer_read` (one sentence each).
- Observability: no new event or field; the filter is a closed enum in the request line, which access records do not keep.

## Texts

In the Messages feature section, English and Russian: "Chat filters" / «Фильтр чатов»; "All" / «Все»; "Students" / «Ученики»; "Tutors" / «Репетиторы» (Q2 = A); "Groups" / «Группы»; "Unread" / «Непрочитанные»; "Unread, {count}"; "No unread chats" / «Нет непрочитанных чатов»; "No group chats" / «Нет групповых чатов»; "No chats with students" / «Нет чатов с учениками»; "No chats with tutors" / «Нет чатов с репетиторами». Weekday and date formats come from `Intl`. None in the entry.

## Files and order of work

1. Backend: `backend/queries/channels.sql` (`ListChannelsOfMember` arguments), `backend/queries/messages.sql` (`ListPeerReadSeqs`), sqlc output; `repositories/channels.go`, `repositories/messages.go`; `services/chat/chat.go` (filter, `peer_read` in list and detail); `services/export/student_views.go` (explicit null); `schemas/chat.go`; `api/chat_routes.go`; `repositories/channels_test.go` (the exemption list addition); the OpenAPI document.
2. Backend tests, `api:check`, the generated client.
3. Frontend: `components/ui/Avatar.tsx` (colour, medium size, grapheme initial, glyph), `features/messages/ChannelList.tsx` (including the history-driven filter focus), `firstPage.ts`, `keys.ts` (the key builder), new `ChannelFilters.tsx`, new `compactTime.ts`, row links and Back to the list, texts; `frontend/scripts/palette.test.ts` (avatar pairs). Existing Vitest files with literal channel keys switch to the builder with unchanged assertions: `app/liveRefetch.test.ts:80-81`, `:199-200`, `:333-334`; `features/messages/channelListScope.test.ts:23`; `MessagesPage.test.tsx:1474`, `:2527`.
4. Frontend tests and e2e specs.
5. Documents listed above and this plan as `knowledge/plans/r3a-message-list.md`.
6. Measure the Messages chunk, the entry and the new specs early; then the rendered-screen review, the change checks and the readiness reviews.

## Bundle

The entry is unaffected (Messages and `Avatar` users are lazy; no frame change). Estimate +1.5 to +2.5 KB gzip in the Messages chunk, about +0.2 KB in pages that use `Avatar`. `pnpm build` and `pnpm check:bundle` run through `tools/dev-env.sh` early and after the last fix; any entry growth above 0.1 KB is a defect to find before readiness.

## Tests (named; Arrange–Act–Assert, one Act each; no mocks)

**Go integration tests**, new file `backend/internal/api/chat_channel_filters_test.go` (real API and database; one seeded tutor with five direct and four group channels, three unread; `limit=2`; the single Act of each subtest is one helper call that follows `next_cursor` to the end):

- `TestChannelListFilterReturnsExactlyItsSetAcrossPages`, subtests `direct`, `group`, `unread` (one request walk each; exact ids in order, no duplicates).
- `TestChannelListFilterComposesWithSearchAcrossPages` (`filter=group` with a term).
- `TestChannelListRejectsAnUnknownFilter` (`422`).
- `TestChannelListFiltersStayWithinTheCallersMembership`, subtests per caller (first tutor, second tutor, student) sharing a student (T-5, T-6).

**Go integration tests**, `backend/internal/api/chat_read_ticks_test.go` (extended; one Act each):

- `TestAnOwnDirectLastMessageIsUnreadInTheListUntilThePeerReads` and `TestAnOwnDirectLastMessageIsReadInTheListAfterThePeerReads`.
- `TestTheListsPeerReadEqualsTheDetailsPeerReadSeq`, table subtests with one request pair each: read-only channel, suspended (`reclaim_pending`) peer, reactivated before a new report (modelled on `:284`, `:307`, `:332`).
- `TestATutorsOwnGroupLastMessageCarriesNoPeerRead` and `TestAStudentsOwnGroupLastMessageCarriesNoPeerRead`.
- `TestAnOthersLastMessageCarriesNoPeerRead`; `TestADeletedOwnLastMessageCarriesNoPeerRead`; `TestADirectRowWhosePeerRowIsGoneCarriesNoPeerRead` (after a purge).
- `TestOneChannelIsReadWithItsListFieldsAndHeaderLink` (`chat_routes_test.go:164`) gains the `peer_read` assertion; an existing export test gains "`peer_read` is null in every exported channel".

**Repository test**, `repositories/channels_test.go`: `TestListPeerReadSeqsEqualsGetPeerReadSeqForEveryPredicateCase`. In this test only, after `seedPredicateCases` (active, suspended, discarded, erase-pending, group), each member row gets distinct `last_read_seq` and `shared_read_seq` values (respecting `shared_read_seq <= last_read_seq`, with `last_seq` raised as needed), and a purged case is added locally by deleting one student's member row; equivalence is asserted with the tutor and with the student as viewer, the group channel's id included. The shared seed and the counts other tests assert are unchanged. `TestEveryPathReadsMembersThroughTheActivePredicate` keeps every assertion; only the exemption list grows by `ListPeerReadSeqs`.

**Vitest:**

- `compactTime.test.ts` (timezone arithmetic, a permitted unit test): today; yesterday across midnight in the caller's timezone; six and seven days back; the previous year; a timezone whose day differs from UTC.
- `Avatar.test.tsx`: "the initial is the first grapheme after format characters" (a label starting with U+202E and one starting with a surrogate-pair emoji); "a label-kind row shows the person glyph".
- `MessagesPage.test.tsx`: "choosing a filter requests the first page with that filter and keeps focus on the pressed filter"; "the filter is restored from the address"; "the search term is never written to the address" (asserts `location.search` has no `q`); "an unknown filter value in the address shows All and is not sent"; "the Unread filter's name and badge show 99+ above 99"; "a new message that moves a chat into the Unread first page detaches the loaded continuation pages" (live reorder under a filter); "losing a channel's scope removes its row from a Groups page and its continuation" (eviction under a filter); "a history change of only the filter moves focus from the list to the pressed filter" (asserting `document.activeElement` after microtasks are flushed, so the hook's deferred heading focus is shown not to override it).

**Playwright** (`messages.spec.ts`, describe "chat list filters and rows"). A worker-scoped seed (one tutor with many channels, like the pagination seed at `:496-514`) serves only tests in `messages.spec.ts` that write nothing: the Unread listing, Show more under Unread, the request list and the student's view; their describe sets `describe.configure({ mode: 'default' })` as the existing shared-seed describes do (`:513`, `:578-588`), so the seed is built once per project; it is never opened or messaged. The accessibility test seeds its own small set. Every test that opens a chat or sends a message (opening under Groups, Back, reload, the live tick, the phone Back) seeds its own small set of channels:

- "choosing Unread lists only unread chats and keeps focus on the filter";
- "Show more under Unread focuses the first new unread chat" (filter chosen in Arrange);
- "opening a chat under Groups keeps Groups pressed and the list filtered";
- "Back restores the previous filter" (two filter choices in Arrange) and "reload keeps the filter";
- "Back to the list at 360 px keeps the filter" (a chat opened under Groups in Arrange; Back is the Act);
- "a student sees All, Tutors, Groups and Unread, and a group row shows no tick and no member name beyond the last sender" (merged group-row test);
- "the tutor's own last message in a direct row shows Read after the student opens the chat" (live through the frame);
- "the Unread filter makes only its listed requests" (modelled on `home.spec.ts:488`: only `GET /api/v1/channels` pages and the existing counts, and no `/api/v1/avatars/` request).
- `accessibility.spec.ts`: "the chat filters are a named group of pressed-state buttons in the {light, dark} theme", one test per theme (the repository has no axe dependency, so the group's role, name, pressed state and focus are asserted directly and contrast is covered by the palette test; recorded as a deviation from revision 4).
- Folded into existing tests, no new test: the tutor's 360 px Messages sweep (`desktop-layout.spec.ts:976-1039`) asserts the rows fit without horizontal overflow and each row is ≥ 56 px at 360 px and at its existing 1024 px widening (`:1022`); the first test of the describe "the messenger layout on a computer" (`messages.spec.ts:734`, 1280 px) asserts the same at the list's current width (280 px once R1b has merged); the existing markup sweep (`:1039`) gains a group name with markup and one starting with U+202E, read in its row and avatar.
- Existing list tests (`:126`, `:457`, `:513-540`, `:948-990`, `:992`, `message-attachments.spec.ts:139`, `realtime-contract.spec.ts:203`) are kept; selectors reading old row text are updated, assertions are not weakened.

**E2E cost and projected gate time:** 10 new Playwright tests (9 messages, 1 accessibility), 30 runs over Chromium and the Firefox and WebKit spec-group projects, about 3–5 s each, plus seeding: the shared seed once per project (about 3–6 s, comparable to the pagination seed at `:502-505`) and six per-test seeds of about 1–2 s, so roughly +110 to +200 s serial. With `fullyParallel: true` (`playwright.config.ts:38`) the added wall time is about the added serial time divided by the gate machine's worker count, plus the imbalance of the spec group that holds `messages.spec.ts` per engine. The first implementation step measures the new tests and seeds and computes the projection against the per-group durations of a recent gate log; if it exceeds 720 s, tests are merged where one Act allows; if it still exceeds it, the owner gets a multiple-choice question before merge.

## Rendered-screen review

With the `ui-design-review:review` skill on the local stack with synthetic data only (no remote design service), in Russian, light and dark themes: the tutor's Messages under each filter, with search, with an open conversation (including the open chat leaving Unread); the student's Messages under each filter; empty states; a long Russian label, "99+", a read-only row, an unconfirmed provisional row, a deleted-user row, a row with a null colour, one's own last message read and unread; at 1440, 1280, 1024 (rail when R1a has merged), 800 and 360 px. Findings and resolutions go in the readiness report.

## Tier, trigger and threats

Standard tier. Security trigger met: channel scope and filters on a chat route, a field derived from read state, untrusted names rendered as initials. Threats:

- **T-5:** affected; filter arguments narrow the membership-scoped query; no route or identifier added; the two-tutor test covers it.
- **T-6:** affected and amended (wording above); `peer_read` only for one's own direct message, computed for already-scoped ids, null in groups; no member list; the existing group last-sender label is unchanged.
- **T-14:** examined; labels and initials are text nodes in `dir="auto"`, with `Cf` removed for the initial; the markup sweep covers U+202E.
- **T-18:** examined; only the closed filter value enters the address; the search term never does (tested).
- **T-20:** examined; one batch query per page, bounded by `limit ≤ 100`; filters add no oracle beyond `unread_count` and `unread_channels`.
- **T-21 and T-44:** examined and amended; the list shows exactly the frozen or lowered marker the detail shows (equivalence tests).
- **T-37:** not affected under Q1 = A and Q3 = A (no image request, no storage key); option B of either reopens it.
- T-19: not affected.

Reviews: standard and security-profile planning reviewers before approval (revision 1 reviewed, revision 2 re-reviewed, and one further scoped reviewer over the fixes of revision 3); standard and security-profile readiness reviewers over the complete diff.

## Change checks

Backend Go rows (gofmt, `go vet`, golangci-lint, `go test -race` of changed and importing packages, `internal/conventions` and `cmd/` tests through `tools/dev-env.sh`); the `backend/queries/` row (`sqlc diff` and the whole `go test -race ./...`); the generated OpenAPI and client row (`api:check` and the whole `pnpm e2e`); the `frontend/` rows (typecheck, lint, format, unit tests, build, class check, bundle check); the documentation checks; Gitleaks over staged changes. Heavy runs only in a slot the coordinator gives, with both load averages below 6. The merge gate runs the full local check run.

## Acceptance criteria

1. The four filter tests in `chat_channel_filters_test.go` pass.
2. The `peer_read` tests, the equivalence table and `TestListPeerReadSeqsEqualsGetPeerReadSeqForEveryPredicateCase` pass; the export test shows `peer_read` null; `TestEveryPathReadsMembersThroughTheActivePredicate` passes with only `ListPeerReadSeqs` added to its exemption list (`git diff` of `channels_test.go` shows one added name and no removed line).
3. The named Playwright tests pass in Chromium, Firefox and WebKit, with focus asserted by `toBeFocused` and pressed state by `aria-pressed`.
4. "the search term is never written to the address" passes.
5. "the Unread filter makes only its listed requests" passes.
6. The tutor's 360 px Messages sweep (with its 1024 px widening) and the first 1280 px test of "the messenger layout on a computer" assert no horizontal overflow and rows ≥ 56 px.
7. `pnpm check:bundle` passes with the entry within 0.1 KB of its figure before the change.
8. `api.md`, `files.md` (export content), `realtime-and-video.md`, `pages.md`, `states.md` and `tokens.md` describe the change; `grep -n 'same value as .peer_read_seq' docs/threat-model.md` finds the T-6 amendment.
9. The rendered-screen review table covers every screen, state, width and theme listed above.

## Risks

- **Unread and paging:** a chat read while paging leaves the filtered set; detachment and de-duplication keep the list consistent, and the specification says so.
- **Gate time** may exceed 720 s; the e2e section sets the response.
- **Conventions test:** one approved name added to its exemption list, guarded by the equivalence test.
- **Shared focus hook:** not changed; the history-driven filter focus lives in `ChannelList`, tested in `MessagesPage.test.tsx`.
- **Shared seeds:** only read-only tests share the worker seed; writing tests seed their own channels, so order and parallelism cannot change their results.
- **`Avatar` change** reaches eight other pages; only the initial's extraction and an optional prop change, covered by its test and the rendered review of one other page (Students).
- **R1b interaction:** rows must fit 280 px; the width assertion follows whichever list width is on `main`.
- **Mockup details not adopted:** two-letter initials, the drawn tab count style, presence for students and the "Chats" phone label.

## Changes after the planning reviews

Revision 2 (after the standard and security-profile reviews of revision 1):

- Decision 6: `peer_read` comes from the new batch query `ListPeerReadSeqs`, mirroring `GetPeerReadSeq`, added to the conventions test's exemption list as an approved addition; null for deleted or erased last messages; computed on `GET /channels/{id}` by the same rule; both exports carry null; equivalence tests for read-only, suspended and reactivated rows and a repository equivalence test; T-21 and T-44 mapped and amended; T-6 wording ties the value to `peer_read_seq`.
- Decision 3: only the filter goes in the address; the search term stays in component state (tested); the address value is parsed into the enum and anything else is All.
- Decision 4: the filter is carried in row links and Back to the list; focus after a history-driven filter change is specified.
- Decision 1: `Avatar` is extended instead of a new component; grapheme-safe initial with `Cf` removed; label-kind glyph; colour corrected for students and its null case.
- Decisions 7 and 8: "No chats" stays for search and All; the open chat leaving Unread is accepted and specified.
- Tests split to one Act each; tests added for live reorder and eviction under a filter, history focus, the request list (no avatar requests), the search term, the unknown filter, deleted last messages and the exports; the width check folded into the existing sweeps; the group-row tests merged; the markup check folded into the existing sweep; the projected gate wall time stated.
- Q4 and Q5: where their answers are recorded.

Revision 3 (after the scoped re-review of revision 2):

- Playwright seeds: the worker-scoped seed serves only tests that write nothing; tests that open or message a chat seed their own channels.
- E2E cost: seeding counted; the projection uses `fullyParallel` and the worker count, checked against a recent gate's per-group durations; a counted test "Back to the list at 360 px keeps the filter" added.
- Folded width checks moved to real host tests (the tutor's 360 px sweep with its 1024 px widening, and `messages.spec.ts:734` at 1280 px); AC6 updated.
- The repository equivalence test sets distinct markers, adds a purged case locally and asserts both viewers.
- A key builder in `keys.ts`; the existing Vitest files with literal keys are listed.
- The avatar fill and text colours and the 4.5:1 palette test are specified.
- The history-driven filter focus lives in `ChannelList`; the shared `liveRows.ts` hook is not changed.
- The export contract is named: `files.md` export content and the OpenAPI export schemas.

Revision 4 (after the further scoped review of revision 3):

- Decision 1: the avatar initial is drawn in `--color-text` on the colour's 16 % sRGB tint (computed at least 12.2:1 light and 9.3:1 dark on `main` at `3743ebb`; 11.3:1 and 9.1:1 before the palette change R0); the tint comes from a typed table in `palette.ts`; the palette test computes the sRGB mix.
- Decision 4: the focus mechanism is stated in React's actual order, guarded by `focusIsLost()` and a `focusin` record; the Vitest case flushes microtasks.
- Playwright: the shared-seed describe runs in `mode: 'default'`; the accessibility test seeds its own set; the count is 10 tests and 30 runs.
