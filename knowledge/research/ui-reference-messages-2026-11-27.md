# Messages: reference study, checked 2026-11-27

Status: agent research, checked 2026-11-27 from public first-party sources; decides nothing. Product behavior is volatile; recheck before relying on it. Supplies the reference study that the AGENTS.md UI work rule requires for R3a `feat/redesign-message-list` and R3b `feat/redesign-message-actions` of the umbrella redesign plan `knowledge/plans/redesign.md` (revision 3 at commit `8b3a8b0`, approved by the owner on 2026-11-27 as a sequencing plan). "FP" marks a first-party source, "TP" a third-party one; numbers refer to the source list at the end.

## Scope and OVC baseline

The screen is Messages for both roles: the channel list (rows, filters, search, unread state, list read ticks), the conversation (bubbles, grouping, day separators, reply quotes, reactions, file cards, the pinned bar), the message menu and its gestures, the composer, and the phone split between list and conversation. Panel resizing belongs to R1b and the frame study; it is touched here only where Messages switches to one pane.

What OVC has today, from [pages.md, Messages](../design/web/pages.md#messages) and the student variant in [Messages and settings (student)](../design/web/pages.md#messages-and-settings-student), implemented under `frontend/src/features/messages/`:

- Channel list (`ChannelList.tsx`): label per [labels per role](../specs/realtime-and-video.md#labels-per-role), last activity, preview with sender, unread count capped at "99+", bold name while unread, read-only mark, search by label, pagination by last activity with first-page refetch at most once a second and cursor detachment. No avatars, no filters, no list ticks.
- Conversation (`MessageRow.tsx`, `messageGroups.ts`, `Quote.tsx`, `Reactions.tsx`, `Attachments.tsx`): bubbles from change M1 (own on the right on `mine`, others on the left, 72 % / 85 % width, one-minute grouping, date chips), picture albums and file cards, Jump to latest, replies and a tutor-only pinned bar (M2), search in a conversation (M4), six fixed reactions with chips under the bubble (M5), and direct-chat read ticks only (M3: one mark "Sent", two marks "Read" from `peer_read_seq`; none in groups).
- Message menu (`MessageMenu.tsx`): opened by the always-focusable "Message actions" button beside each bubble (transparent on fine pointers until hover or focus); the spec also names long press on touch. Items: Reply, React, Edit, Pin/Unpin (tutor), and Delete last after a separator, confirmed by "Delete this message?". No right-click, no reaction row at the top, no Copy text.
- Composer (`UploadComposer.tsx`): one field growing from one to eight lines, the icon button "Attach files", the text button "Send" (owner's decision of 2026-11-24; plan item 5 and the plan's existing-versus-target table make the round Send of 2026-11-27 supersede its appearance, not its accessible name), Enter sends from 768 px. No emoji picker.
- Phone: below 768 px list and conversation are separate screens (change G5), with "Back to the list".

Owner's decisions that bind this study (plan section 3): item 5 (Telegram-like layout, the full feature list above, menu by right click or long press with a reaction row, Reply, Copy text and Pin, destructive items separated and confirmed, one composer with attach, emoji and a round Send, phones one pane at a time), item 6 (no underlined actions; message-text links stay underlined), item 7 and 10 (chat list 280–480 px, default 340 px, one-pane Messages from available width with a 480 px conversation minimum), item 14 (initials plus already authorized avatar fields, no expanded audience) and item 16 (destructive actions in confirmed menus, one primary action). The plan's approaches table keeps a visible named message-menu trigger beside the gestures. R3a acceptance forbids extra peer identity, rosters or group read receipts for students and requires filters over the whole authorized result set.

Mockups (static, non-normative): `knowledge/design/web/prototype/redesign/messenger.html` and `messenger-phone.html` at `8b3a8b0`. The desktop board shows avatar rows with initials, tabs All / Students / Groups / Unread with a count on Unread, a double tick on the caller's own last message in a direct row, a "Name:" sender prefix in a group row's preview, a read-only row dimmed, a header with avatar, search, student-card and More icon buttons, a pinned bar with Unpin, a reply quote inside an outgoing bubble, a reaction chip, a file card, and an open menu with six emoji (thumbs up, heart, laughing, surprised, folded hands, clapping) above Reply, Copy text and Pin; the drawn menu shows no Edit or Delete and the drawn conversation has no visible menu trigger. The phone board shows only the list screen with search and the bottom tab bar labelled "Chats".

## How the compared products solve it

### Telegram (Web, Desktop and mobile)

- Read state: one check means the message reached the cloud; two checks mean the recipient opened the conversation [1]. Exact read times are shown only in one-on-one chats and small groups, are kept for 7 days, and are hidden from people from whom one hides one's last-seen time [1]. Telegram therefore exposes read state in small groups, unlike OVC's policy.
- Chat list: pinned chats stay at the top [1]; chat folders add tabs to the chat list on mobile and a sidebar with icons on desktop, and a folder can include all chats of a type such as Unread [2]. Telegram Desktop later added folders above the chat list and a horizontal strip mode (5.7.3 beta, 5.8) [5]. Chats can be marked Read or Unread from the list (1.3.9) [5].
- Panes: Telegram Desktop added a single-column layout for a small window (0.6.18), resizing the chat list by dragging (1.0.1), shrinking it to a column of profile pictures (1.0.2) and saving its width across relaunches (2.1.15 beta) [5].
- Replies: a reply can quote a selected part of a message; tapping the quote jumps to that spot in the original [3].
- Reactions: double tap sends a quick reaction (default thumbs up); a single tap on Android or press-and-hold on iOS opens the wider set; group admins choose whether reactions are on and which emoji are offered [4]. Telegram Desktop shows a reaction selector above the right-click menu (4.1.2 beta) and an expandable reaction menu (4.2) [5].
- Keyboard: Ctrl+Up and Ctrl+Down select a message to reply to (1.2.7); the Up arrow edits the last sent message (0.9.49); Ctrl+C and the context menu copy selected text (0.3.12) [5].
- Pinning: messages can be pinned in any chat, including one-on-one chats, several at a time (2.4.5) [5].

### WhatsApp (Web and mobile)

- Filters: All, Unread and Groups sit at the top of the chat list; Unread holds chats not yet opened plus chats marked unread, and Groups holds all group chats including community subgroups [6].
- Read state: one grey tick when delivered, two grey ticks when received, two blue ticks when read, and Message info lists who read a message and when, opened by long press on Android or a swipe on iOS [9] (TP). WhatsApp shows per-member read state in groups, unlike OVC's policy. The first-party help page on read receipts did not render its text when fetched [7].
- Pinning: long press, Pin, then a duration of 24 hours, 7 days (default) or 30 days; the pinned message shows at the top of the chat; group admins decide who may pin [8] (TP).
- Reactions: the full emoji keyboard with a skin tone selector is available for reactions [10]. The help page on replying on WhatsApp Web did not render its text when fetched [11], so the hover-arrow menu is unverified.

### Slack (desktop)

- Message actions: hovering a message reveals a toolbar whose three-dots icon opens save, share, add reaction, pin, reminder, thread and more [12, 13]. Reactions are added from the Add reaction icon in the hover toolbar, the caller's own are highlighted, clicking one's own removes it, and hovering a reaction shows who reacted [14].
- Pins: Pin to channel or Pin to this conversation from the three-dots menu; pins are listed under a Pins tab in the conversation header [13].
- Unread: an Unreads view at the top of the sidebar, sortable and filterable by sidebar section on desktop, with Mark as read, Mark all messages read with Undo, and shortcuts Ctrl/Cmd+Shift+A (open Unreads), F6 (focus the message list), Up/Down (move between messages) and Alt/Option+click (mark unread) [12].
- Read state: Slack shows no per-person read receipts in channels or direct messages [16] (TP); its first-party "message activity" shows owners and admins a count of people who interacted with a message, and is not available in private channels on Business+ [15].

### Signal (Desktop and mobile)

- Reactions: on mobile, press and hold a message to open the reaction window, with an ellipsis that opens the full emoji keyboard; on Desktop, hover a message, choose the emoji-add icon and use the ellipsis for more [17]. In groups every participant sees reactions, and reaction notifications go only to the message's author [18].
- Folders: Signal Desktop sorts chats into folders for one-to-one chats, group chats and unread messages, plus custom folders, managed in Chats settings or by right-clicking a chat [19] (TP). Release 8.32.0 extended the unread filter to folders and search [20].
- Read state: read receipts are optional and must be enabled on both sides (search-result excerpt of Signal's help page; the page returned HTTP 403 when fetched) [21].

### NN/g on contextual menus

Contextual menus suit secondary actions, not frequent ones; their kebab or meatball triggers should be large, high-contrast and visible without hover, placed next to the element they affect, with labels or specific tooltips; right click and long press are common openers for which the icon is the visual cue; menus must work by keyboard and screen reader [22]. The existing [UI guidelines research](ui-guidelines-2026-11-23.md) records the same article.

## Comparison

| Design question | Telegram | WhatsApp | Slack | Signal | OVC decision/target |
|---|---|---|---|---|---|
| List filters | Folder tabs (mobile), sidebar or strip (desktop); Unread is a type [2, 5] | All / Unread / Groups chips [6] | Unreads view filtered by sidebar section [12] | 1:1 / Groups / Unread folders [19, 20] | All / Students / Groups / Unread (item 5), server-side over all authorized channels (R3a) |
| Read state in groups | Small groups, read time 7 days [1] | Per-member Message info [9] | None for members; admin interaction count [15, 16] | Optional, both sides [21] | None in groups; direct chats only (M3, R3a) |
| Read ticks | One / two checks [1] | Grey / blue ticks [9] | None [16] | Optional [21] | One "Sent" / two "Read" in direct chats; add to own last message in direct list rows (R3a) |
| Menu opener | Right click with reaction selector above (desktop) [5]; tap or hold (mobile) [4] | Long press (mobile) [8]; web unverified | Hover toolbar, three-dots [12, 13] | Hover emoji icon (desktop), hold (mobile) [17] | Right click and long press plus the visible "Message actions" button (item 5, approaches table, [22]) |
| Reaction set | Admin-chosen in groups [4] | Full keyboard [10] | Full picker [14] | Default row plus full keyboard [17] | Fixed six (M5); the mockup's six differ |
| Who reacted | Unverified | Unverified | Hover shows names [14] | All participants see [18] | Tutor sees names; a student's chip names nobody (M5) |
| Copy text | Context menu and Ctrl+C (desktop) [5] | Unverified | Unverified | Unverified | Copy text of the displayed message (R3b) |
| Pinned message | Several per chat [5] | Up to 3, timed (TP) [8] | Pins tab in header [13] | Unverified | One pinned bar, tutor pins (M2) |
| Keyboard | Ctrl+Up reply, Up edits last [5] | Unverified | F6, Up/Down, shortcuts list [12] | Unverified | Tab to "Message actions", menu keys, Escape returns focus (pages.md) |
| Phone | Single column (desktop narrow) [5]; mobile list then chat | Unverified for web | Not compared | Not compared | List or conversation one at a time (G5, item 5) |

## What OVC adopts

- Filter row above the list → WhatsApp's All / Unread / Groups chips [6] and Signal's 1:1 / Groups / Unread folders [19] → item 5's four filters; all four products put unread filtering at list level, which supports R3a's server-side filtering rather than filtering loaded pages.
- One check versus two checks inside the bubble after the time → Telegram [1] and WhatsApp [9] → already M3; R3a extends the same marks to the caller's own last message in a direct list row, with the same visually hidden "Sent"/"Read".
- Reaction row at the top of the right-click menu → Telegram Desktop's selector above the context menu [5] → item 5's reaction row; it replaces M5's separate React step only if the task plan keeps a keyboard path to every reaction and Remove reaction.
- Right click on desktop and long press on touch as accelerators → Telegram [4, 5], Signal [17], WhatsApp [8] → item 5; the visible trigger stays as NN/g recommends [22] and as Slack's always-available three-dots toolbar shows [12].
- Copy text in the menu → Telegram Desktop [5] → item 5; copies only the displayed, authorized body (R3b acceptance).
- Who-reacted on hover or in the chip name for the tutor → Slack [14] → already M5 for tutors; students stay anonymous.
- Pinned bar under the header with its own jump → Telegram, WhatsApp's top-of-chat pin [8] → already M2; no change of audience.
- Single-pane switch from available width and remembered list width → Telegram Desktop's single-column layout and saved list width [5] → items 7 and 10 (R1b owns the separator).

## Where OVC differs and why

- No group read receipts and no per-member read lists. Telegram [1] and WhatsApp [9] expose read state in groups; OVC shows ticks in direct chats only, because a student must not learn other students' activity ([ADR-0005](../../docs/decisions/0005-tutor-centric-domain.md) relationship scope, R3a acceptance, T-5/T-6 in the [threat model](../../docs/threat-model.md)).
- No peer identity for students. The mockup's group preview "Name: …" prefix and initials must follow the existing per-message sender-label policy and [labels per role](../specs/realtime-and-video.md#labels-per-role); a student's Students filter, group avatars and typing line must reveal no member roster. Telegram, WhatsApp and Signal show members freely; OVC does not.
- Fixed reaction set, not admin-chosen or full keyboard. Telegram [4], WhatsApp [10] and Signal [17] offer wider sets; OVC keeps M5's six fixed kinds (an enum, no external emoji assets). The emoji picker in the composer is for text only and must ship in-repo or native (R3b), with no remote emoji service ([ADR-0008](../../docs/decisions/0008-hosting-and-data-residency.md) residency, no third-party service).
- Pin stays tutor-only, one message, no timer. WhatsApp's timed multi-pin [8] and Telegram's several pins [5] are not adopted; OVC's M2 contract is unchanged.
- No double-tap quick reaction and no swipe-to-reply. Telegram's double tap [4] conflicts with text selection and zoom; R3b acceptance requires that no gesture suppress scrolling or selection.
- Destructive items remain confirmed. None of the sources documented a delete confirmation; OVC keeps "Delete this message?" with Delete last after a separator (item 16, D2).
- Keyboard and focus are contractual. Products document shortcuts [5, 12]; OVC additionally specifies focus targets with tests (pages.md Focus), and WCAG 2.2 requires the visible trigger and a non-pointer path for every gesture (2.5.1 Pointer Gestures, 2.1.1 Keyboard).
- No read-time detail, no Mark as unread. Telegram [5], Slack [12] and Signal [20] let people mark a chat unread; the owner's decisions do not include it, and it would need a new contract.

## Open points for the task plan

- R3a: role wording of the "Students" filter for a student (a student's direct chats are with tutors); whether Unread counts channels or messages and how its count relates to `GET /channels/unread`; how each filter paginates with cursor detachment and live reorder; which API field gives the caller's own last message and its read state per direct row without per-row history fetches.
- R3a: initials source (the role label only), colour source for a group avatar, and whether any authorized avatar image exists for each row kind (item 14).
- R3a: whether a group row's "Sender:" preview prefix for a student is already the existing policy or a new disclosure.
- R3b: the reaction row's six emoji: the mockup draws folded hands and clapping, M5 has Sad and Done; and whether the row replaces React or both exist.
- R3b: whether Edit and Delete stay in the menu under the reaction row (the mockup omits them; item 5 says destructive items sit at the bottom, confirmed).
- R3b: long-press timing and cancellation against scroll and text selection on touch; right click inside a selection or a link (the native menu may be needed there).
- R3b: Copy text for messages with attachments only, placeholders or the reclaim cue; clipboard refusal wording and focus.
- R3b: emoji picker source (native OS picker versus an in-repo set), its keyboard model and insertion at the selection.
- R3b: how the round Send keeps the accessible name "Send" of the 2026-11-24 decision, whose appearance the plan supersedes, and its target size at both pointer types.
- Phone: Messages vs "Chats" naming (the phone board labels the tab "Chats").

## Unverified

- Telegram: read ticks in the chat list rows; the exact Telegram Desktop context-menu item order (a user post on Telegram's suggestion tracker lists Reply, Pin, Copy Text, Copy Message Link, Forward, Delete, Report, Select; not checked); who-reacted lists.
- WhatsApp Web: the hover arrow menu with Reply, React and Pin; grey ticks turning blue in groups only once every member read (third-party claim only); right-click behavior.
- Signal Desktop: message context menu items (Reply, Copy text, delete), pinned messages, check-mark appearance, where folders appear (tabs or sidebar).
- Slack: whether unread conversations show bold names or badges in the sidebar (the unread article did not cover it).
- All products: delete-confirmation dialogs and the separation of destructive menu items.

## Sources

1. Telegram — Telegram FAQ — <https://telegram.org/faq> — FP — checked 2026-11-27 — first 100 000 characters read; reply behavior not covered.
2. Telegram — Chat Folders (blog) — <https://telegram.org/blog/folders> — FP — checked 2026-11-27 — default folder names not stated.
3. Telegram — Reply Revolution (blog) — <https://telegram.org/blog/reply-revolution> — FP — checked 2026-11-27 — gestures to start a reply not stated.
4. Telegram — Reactions, Spoilers, Translations and QR Codes (blog) — <https://telegram.org/blog/reactions-spoilers-translations> — FP — checked 2026-11-27 — reaction placement and who-reacted not stated.
5. Telegram Desktop — changelog.txt in the official repository — <https://raw.githubusercontent.com/telegramdesktop/tdesktop/dev/changelog.txt> — FP — checked 2026-11-27 — release notes, not current UI descriptions.
6. WhatsApp — Find messages faster with chat filters (blog) — <https://blog.whatsapp.com/find-messages-faster-with-chat-filters> — FP — checked 2026-11-27.
7. WhatsApp — How to check read receipts (Help Center) — <https://faq.whatsapp.com/665923838265756/?locale=en_US> — FP — checked 2026-11-27 — body did not render; confirms nothing.
8. WhatsApp — "WhatsApp now lets you pin messages in individual and group chats" (TechCrunch) — <https://techcrunch.com/2023/12/12/whatsapp-now-lets-you-pin-messages-in-individual-and-group-chats/> — TP — checked 2026-11-27 — 2023 launch state; later limits differ.
9. WhatsApp — "WhatsApp: 15 tips and shortcuts" (Yahoo News) — <https://malaysia.news.yahoo.com/whatsapp-15-tips-shortcuts-most-152034863.html> — TP — checked 2026-11-27 — group behavior not covered.
10. WhatsApp — Reactions, 2GB file sharing, 512 groups (blog) — <https://blog.whatsapp.com/reactions-2gb-file-sharing-512-groups> — FP — checked 2026-11-27 — how to add a reaction not stated.
11. WhatsApp — How to reply to a message (Help Center) — <https://faq.whatsapp.com/en/general/26000014> — FP — checked 2026-11-27 — body did not render; confirms nothing.
12. Slack — View all your unread messages — <https://slack.com/help/articles/226410907-View-all-your-unread-messages> — FP — checked 2026-11-27.
13. Slack — Pin messages and bookmark links — <https://slack.com/help/articles/205239997-Pin-messages-and-bookmark-links> — FP — checked 2026-11-27.
14. Slack — Use emoji reactions — <https://slack.com/help/articles/202931348-Use-emoji-reactions> — FP — checked 2026-11-27.
15. Slack — Manage message activity — <https://slack.com/help/articles/4403584815507> — FP — checked 2026-11-27 — does not say whether names are shown.
16. Slack — "Slack read receipts" (Suptask blog) — <https://www.suptask.com/blog/slack-read-receipts> — TP — checked 2026-11-27 — search-result excerpt only, page not fetched.
17. Signal — It's OK to be a little Emojional (blog) — <https://signal.org/blog/more-reactions/> — FP — checked 2026-11-27.
18. Signal — Give reactions a thumbs up (blog) — <https://signal.org/blog/reactions/> — FP — checked 2026-11-27 — 2020 beta announcement.
19. Signal — "Signal Desktop adds customizable chat folders" (AlternativeTo) — <https://alternativeto.net/news/2025/10/signal-desktop-adds-customizable-chat-folders-for-better-organization> — TP — checked 2026-11-27 — placement of folders not stated.
20. Signal Desktop — Releases (official repository) — <https://github.com/signalapp/Signal-Desktop/releases> — FP — checked 2026-11-27.
21. Signal — How do I know if my message was delivered or read? (Support) — <https://support.signal.org/hc/en-us/articles/360007320751-How-do-I-know-if-my-message-was-delivered-or-read> — FP — checked 2026-11-27 — HTTP 403 when fetched; only a search-result excerpt confirms the optional, both-sides rule.
22. NN/g — Designing Effective Contextual Menus: 10 Guidelines — <https://www.nngroup.com/articles/contextual-menus-guidelines/> — TP (research) — checked 2026-11-27.
