# Task plan: R3b, message actions: context menu with a reaction row, Copy text, emoji picker and round Send

Branch: `feat/redesign-message-actions`, cut from `main` when code starts (planned against `3743ebb`). Revision 4 (2026-11-27), approved by the owner on 2026-11-27 (see Owner's answers below). Revision 4 applies the final scoped re-review of revision 3 (1 major, 6 minor, 4 nit); revision 3 applied the scoped re-review of revision 2 (3 major, 6 minor, 4 nit); revision 2 applied the standard planning review (12 major, 10 minor, 2 nit) and the security-profile planning review (1 major, 6 minor, 2 nit) of revision 1; changes are listed at the end. After approval this plan is committed as `knowledge/plans/r3b-message-actions.md` in the change. Row R3b of the interface rework plan's redesign rows; scope from section 7 R3b of the [visual redesign plan](redesign.md), the owner's decisions 5, 6 and 16 of its section 3, the mockup `knowledge/design/web/prototype/redesign/messenger.html`, and two owner's decisions of 2026-11-27 taken with the [R3a task plan](r3a-message-list.md):

- **Reaction set:** the reaction row keeps the six reactions of change M5 (thumbs_up, heart, joy, wow, sad, check); the mockup's folded hands and clapping are not adopted; no enum, schema or stored-reaction change.
- **Edit and Delete placement:** Edit stays in the menu, and Delete stays last after a separator with its confirmation, in both the context menu and the visible "Message actions" button.

**Reference study:** `knowledge/research/ui-reference-messages-2026-11-27.md` on `main` (row RS-messages). It compares Telegram, WhatsApp, Slack, Signal and NN/g's contextual-menu guidance; this plan resolves its R3b open points: the reaction set and Edit/Delete by the two decisions above, the rest in the decisions below or as the five owner questions.

## Problem

The message menu opens only from the "Message actions" button beside each bubble (and a long press on that button on touch). Its items are Reply, React (a second step opening six reaction buttons), Edit, Pin/Unpin and Delete last after a separator (`MessageMenu.tsx`; `pages.md` Messages, Reactions, Replies). There is no right click or long press on the bubble, no reaction row, no Copy text, no emoji picker, and Send is the text button "Send" (owner's decision of 2026-11-24). The owner decided on 2026-11-27 (redesign plan, section 3, item 5) a menu opened by right click or long press with a reaction row, Reply, Copy text and Pin, destructive items separated and confirmed, and one composer with attach, emoji and a round Send. Section 7 adds: no gesture suppresses ordinary scrolling or selection, no external emoji assets, copied text comes only from the authorized displayed message, the deletion confirmation and visible menu trigger stay, send keeps composer focus, menu cancellation restores the correct trigger, and reflow preserves grouping, pin and scroll/read semantics. Section 8 names the executable tests `message menu gestures and keyboard` and `clipboard refusal preserves focus`; this plan maps them below.

## Scope

**In R3b:** context opening of the existing menu (right click on the bubble, the context-menu key and Shift+F10 on a focused message row, long press on the bubble); the reaction row at the top of the menu replacing the React step; Copy text with its notices; an in-repo emoji picker in the composer inserting at the selection; the round Send with the accessible name "Send"; the specification and threat-model wording listed under Contracts; feature texts (en, ru); tests.

**Not in R3b:** the reaction set or any backend, schema, API or frame change; a full emoji keyboard, skin tones, emoji search or recently used emoji (would need storage, T-37); emoji as reactions; double-tap reactions or swipe-to-reply (the study's differences); Mark as unread; the list (R3a); panel widths (R1b); the phone redesign of the conversation beyond what keeps working at 360 px (PHONE).

## Facts the plan relies on (checked on `main` `3743ebb`)

1. `MessageMenu.tsx` owns `open` and `picking` state and its opener button; it opens on its first button; an outside pointer down only closes it (focus follows the click); Escape closes it onto the button (Escape inside the reaction picker returns to React); when its container unmounts while holding focus, focus goes to the row. A 500 ms touch long press on the button itself (via `useScheduler`) opens it and swallows the click that ends the press (component test `touch long press keeps the menu open through pointerup and its generated click`).
2. `Conversation.tsx` `menuOf` builds the menu as a `ReactNode` passed to `MessageRow`: Reply only in writable chats and not while editing (`onReply={editing === null ? reply : null}`); React only in writable chats; Edit for own messages within 24 hours in writable chats; Pin (writable) or Unpin for the tutor; Delete for own messages or for the tutor; no menu on deleted or erased messages; no menu when no item applies (a student on another member's message in a read-only chat). The Delete separator shows when any of Edit, Pin, React or Reply shows.
3. `MessageRow.tsx` renders `li[data-row][data-own]` (`tabIndex=-1`) holding the bubble `div[data-bubble]` and the menu beside it. Interactive content inside the bubble: body links (`MessageBody`, http/https only, `rel="noopener noreferrer nofollow"`, text nodes only), the quote button (`Quote`), pictures and file-card links (`Attachments`), and in writable chats reaction chips as buttons (`Reactions`).
4. `lib/clipboard.ts` `copyText(clipboard, text)` calls `writeText` (text/plain) and returns `'copied' | 'failed'`, `'failed'` also when `clipboard` is undefined. Its callers (tutor code, Today, invitation link) pass `navigator.clipboard` and show a notice that takes focus. jsdom (the component-test environment) has no `navigator.clipboard`.
5. pages.md Shared behavior: "a notice that reports a refusal takes focus when it appears, and again each time it reports a new refusal".
6. The composer `textarea` (React-controlled `draft`) and the submit `ActionButton` (`primaryButtonClass`, text "Send" or "Save") live in `Conversation.tsx`'s `field`; `UploadComposer.tsx` lays out the attach button (44 px, 40 px on fine pointers from 1024 px) and the field. Sending keeps focus in the composer and empties it. The composer is `readOnly` when the chat is not writable or busy, and `aria-disabled` (still typable) offline.
7. `frontend/security-headers.ts` sends no `Permissions-Policy`; CSP has `default-src 'self'`, `script-src`/`font-src`/`img-src`/`media-src` `'self'`, `object-src 'none'`. Existing Copy buttons work under it, so no header change is needed and none is added.
8. `frontend/src/components/ui/icons.tsx` is imported by the entry chunk (about 182.8 of its 184 KB); `features/messages/icons.tsx` is in the Messages chunk.
9. Message bodies refuse only NUL and control characters (`realtime-and-video.md`); they may hold Unicode format characters (bidi overrides, zero-width characters, tag characters).
10. `knowledge/specs/realtime-and-video.md` line 116 says "React in the message menu offers all six and Remove reaction".

## Decisions proposed

1. **One open menu per conversation.** `Conversation` holds `openMenuMessageId`; `MessageMenu` becomes controlled (`open`, `onOpenChange`) and `MessageRow` receives the bubble openers through props. Opening one message's menu closes any other. The menu stays anchored beside its message's "Message actions" button (as today), not at the pointer, so placement needs no new logic. The "Message actions" button and its touch long press stay as they are.
2. **Right click.** On every `pointerdown` inside the bubble, the row records (replacing any earlier record) the pointer type, the button, `ctrlKey`, `shiftKey` and the page's selection at that moment (its anchor and focus, and whether it is non-collapsed and intersects the bubble) (macOS browsers select the word under the cursor before `contextmenu`, so the selection at `contextmenu` time is not used). On `contextmenu` inside the bubble, OVC prevents the browser menu and opens its own only when: the last recorded pointer down was a mouse press on this bubble with button 2, or button 0 with `ctrlKey` (macOS Ctrl+click), without `shiftKey` (a Shift press always gets the browser's menu, which Firefox shows even when the event is prevented, and may extend a selection), and the event itself, where it is a `PointerEvent`, has `pointerType` `mouse`; the message has a menu; the target has no ancestor inside the bubble matching `a, button, input, textarea, select, summary, img, [role="button"], [tabindex]` (body links, the quote, pictures, file links, reaction chips); and no selection was recorded at pointer down. Otherwise nothing is prevented and the browser's own menu shows (link destinations, image and selection menus); in particular a `contextmenu` that follows a touch or pen press is never prevented. When OVC opens its menu, it removes a selection only if that selection is non-collapsed, its anchor and focus both lie inside this bubble, and it differs from the selection recorded at pointer down (that is, the browser added it during this click, as the macOS word selection does), so consecutive right clicks behave the same; any other selection, in another bubble or elsewhere on the page, is left as it is. Each `contextmenu` that reads the record clears it (not on pointer up, because Windows sends `contextmenu` after pointer up), so a later `contextmenu` without a new pointer down (an assistive technology's context-menu command) gets the browser's menu. A right click on the bubble of the message whose menu is already open is not "outside": the menu's outside-pointer handler ignores a button-2 or Ctrl+button-0 press inside its own message's bubble, and the open request carries a counter, so the menu stays mounted and its first item is focused again. The row-target keyboard path of decision 4 never reads this record. A right click on an open message's bubble keeps the menu open and refocuses its first item. A right click on "Message actions" itself is excluded as a button and gets the browser menu.
3. **Long press on touch** (owner question "Long press"). Recommended option: a `touch` `pointerdown` inside the bubble outside the excluded targets starts a 500 ms timer through `useScheduler`; a move beyond 10 px, `pointerup`, `pointercancel` or a `scroll` of the history cancels it. Nothing prevents `pointerdown`, `pointermove`, `pointerup` or `pointercancel`, no `contextmenu` from a touch press is ever prevented (the only event OVC stops is the click of the same long press, after the menu has opened), and `touch-action` and `user-select` are unchanged, so scrolling, text selection and link callouts proceed as before. When the timer fires, the menu opens, and a swallow flag is armed so that the `click` of that same press, if the browser sends one, is swallowed on the bubble in the capture phase; only pointer-generated clicks (`detail > 0`) are swallowed, and the flag is cleared when it swallows a click, by the next `pointerdown` (before the excluded-target check, so a press on a chip clears it too), by `pointercancel` and by `keydown`, so a later tap or keyboard activation is never swallowed (iOS and Android often send no click after a long press). A press on an excluded target (a chip, the quote, a link) starts no timer and arms nothing, so its ordinary activation is unchanged. On Chrome for Android, whose own long press raises `contextmenu` and may start a native selection about the same time, both may show; this is the accepted cost of never suppressing selection, and the real-device behaviour is reported `unverified` unless checked on a phone.
4. **Keyboard opener.** On a focused message row (`event.target === row`), `keydown` of the ContextMenu key or Shift+F10 is prevented and opens the menu; a `contextmenu` event whose target is the row (raised by those keys in some browsers) opens it too and is prevented. With focus on a link, the quote, a chip or "Message actions", these keys do nothing new (the browser's menu; "Message actions" opens with Enter or Space as today).
5. **Focus.** Opening from any opener focuses the menu's first action item with `focus({ preventScroll: true })` (owner question "Menu focus"; recommended: Reply where shown, otherwise the first item; the reaction row precedes it in reading order and is reached with Shift+Tab or by pointer). Escape closes it onto that message's "Message actions" for every opener. An outside pointer down only closes it, and focus follows the click (as today). Choosing an item keeps today's per-item rules (Reply and Edit to the composer; reactions, Remove reaction, Pin/Unpin and Copy text to "Message actions"; Delete opens its dialog, whose dismissal returns to "Message actions"). When the focused item disappears while the menu stays open (the chat becomes read-only), focus moves to the menu's first remaining item; when no item remains, `menuOf` returns nothing and the whole menu unmounts (for example when the message is deleted or erased live, or the chat becomes read-only on a message with no body), and focus goes to the row, as today.
6. **Reaction row.** In writable chats the menu starts with a labelled group "Reactions" of the six M5 reaction buttons (44 px targets, the current one pressed), wrapping onto a second line when the menu is narrower than six targets; "Remove reaction", when the caller has a reaction, sits on its own line under the row. The React item and its second step are removed. Choosing a reaction or Remove reaction closes the menu onto "Message actions" (M5's rule). Every reaction and Remove reaction keep a keyboard path. Read-only chats show no row. The menu stays within the viewport at 360 px.
7. **Copy text.** A "Copy text" item follows Reply (before Edit and Pin), in writable and read-only chats, offline too (not behind the write guard), shown only for a message that is neither deleted nor erased and has a non-empty body; it counts for the Delete separator. It calls `copyText(navigator.clipboard, message.body)` synchronously in the click handler (so the write runs under the user activation, which Safari requires), writing exactly the loaded `message.body` as `text/plain` only (never `clipboard.write`, `ClipboardItem` or `text/html`): never quotes, sender labels, file names, placeholders or the reclaim cue. The body is copied verbatim, including any invisible format characters it holds (stripping them would break joined emoji sequences); this is recorded as a T-14 residual. The menu closes onto "Message actions"; then (owner question "Copy notices"; recommended option; pages.md Shared behavior's result-notice rule) success shows the status "Text copied." in a notice line above the composer that does not take focus, and refusal shows "Couldn't copy the text. Select it and copy it." as an alert that takes focus, as the Shared behavior rule requires; each new copy result replaces the notice and is announced again (keyed by a count), and the notice is removed when a message is sent, another menu opens or the conversation changes. The success status does not take focus because focus has already been placed on "Message actions" when the chosen item disappeared, which conforms to the shared rule rather than excepting from it. A student on another member's message in a read-only chat now gets a menu holding only Copy text. No log or diagnostic carries the body, and the notices are fixed strings.
8. **Emoji picker** (owner question "Emoji set"). An in-repo set of Unicode text constants in `features/messages/emoji.ts`, each with an English and a Russian accessible name in the feature texts; no images, fonts, network or storage. An icon button "Emoji" (44 px, 40 px on fine pointers from 1024 px, like attach) sits between attach and the field in send and edit modes, available whenever the chat is writable (`channel.writable`); while the conversation is busy (an own send, reaction, pin or edit) it stays rendered as `aria-disabled` so nothing shifts and focus is not lost, and offline does not disable it (typing continues offline); an open picker stays open while busy with its emoji buttons `aria-disabled`, inserting nothing until busy ends. It opens a popover holding a labelled group "Emoji" of buttons in rows of eight with one tab stop (roving `tabIndex`; arrow keys, Home and End move), focusing the first emoji on opening. Choosing one reads `selectionStart`/`selectionEnd` from the textarea at that moment, computes the new draft (replacing any selection), calls `setDraft` through the same path as typing (typing presence included), closes the picker, focuses the composer and restores the caret after the inserted emoji once rendered. Escape closes it onto the "Emoji" button; Tab out of it or an outside pointer down closes it and focus follows; it closes onto the composer if the chat stops being writable while it is open. The native OS picker is not used: no web API opens it. Choosing an emoji logs nothing.
9. **Round Send** (owner question "Send size"). The submit button becomes a round filled icon button with an in-repo send icon in `features/messages/icons.tsx` and the accessible name "Send" (`aria-label`), keeping `type="submit"`, busy and unavailable states, Enter behaviour, "Send stays inside the window" and "sending keeps focus in the composer"; editing keeps the text buttons "Save" and "Cancel". This supersedes the appearance part of the owner's 2026-11-24 decision (redesign plan item 5), not its name.
10. **No reflow side effects.** The bubble handlers add no element and no layout; grouping, the pinned bar, Jump to latest and read marking are untouched; opening the menu does not scroll the history (decision 5's `preventScroll`).

## Questions for the owner

**Owner's answers (2026-11-27):** option A for each of the five questions below (long press anywhere on the bubble except its interactive parts; focus on the first action item; copy success as a status without moving focus and refusal as an alert that takes focus; 40 emoji; Send 44 px, 40 px on fine pointers from 1024 px), and no further planning reviewer after revision 4.

Each with a recommended option first.

- **Long press.** On touch, where does a long press open the message menu?
  - A (recommended): anywhere on the bubble except links, pictures, file cards, the quote and reaction chips, after 500 ms, never preventing scrolling, selection or the browser's own long-press menu (on Android both may appear over text).
  - B: only on the "Message actions" button, as today (no conflict with native selection, but no gesture on the bubble).
  - C: on the bubble, suppressing native selection and the browser's long-press menu in bubbles (conflicts with the redesign plan's acceptance).
- **Menu focus.** Where is focus when the menu opens?
  - A (recommended): the first action item (Reply where shown), as change M2 specified; the reaction row comes before it and is reached with Shift+Tab.
  - B: the first reaction button (the current one when set), as M5's picker did; M2's focus rule and its tests change.
- **Copy notices.** How do Copy text's results show?
  - A (recommended): success as a status above the composer without moving focus; refusal as an alert that takes focus, as the shared rule for refusals requires.
  - B: both take focus, like the existing Copy buttons for the tutor code and invitation links (consistent, but moves focus away from the conversation on every copy).
  - C: neither takes focus, adding a named exception to the shared rule for refusals.
- **Emoji set.**
  - A (recommended): 40 common emoji without categories, search or storage.
  - B: about 120 emoji in four labelled categories (about 240 translated names and a larger Messages chunk).
  - C: no picker; leave emoji to the system keyboard (contradicts decision 5 of the redesign plan).
- **Send size.**
  - A (recommended): 44 px, 40 px on fine pointers from 1024 px, matching attach and Emoji.
  - B: 44 px everywhere.

## Contracts, documents and threat model

All new or changed wording is marked "(redesign R3b of the visual redesign plan, owner's decisions of 2026-11-27, pending owner review)".

- `knowledge/design/web/pages.md`:
  - Messages, the Conversation "message menu (hover or focus on desktop, long press on touch)" sentence: openers, one open menu, the item order (reaction row, Reply, Copy text, Edit, Pin/Unpin, separator, Delete), exclusions that keep the browser's menu.
  - Messages, the composer bullet: attach, Emoji and the round Send; the Copy notice line.
  - Messages, the "Focus:" bullet: openers, Escape and outside-click rules, the removed-item rule, Copy and picker focus.
  - Messages, the "Read-only channels" bullet: a student's menu with Copy text only on another member's message.
  - Messenger layout: Send's appearance, the 2026-11-24 decision's name kept.
  - Reactions: the row replaces React; its focus rules.
  - Replies: the menu's first item.
  - "Interface rework stage: desktop contracts", the Messages item listing React.
- `knowledge/specs/realtime-and-video.md` line 116: "the reaction row of the message menu offers all six and Remove reaction".
- `docs/threat-model.md` (a `docs/` change, part of this plan's approval, marked pending owner review):
  - **T-14** adds: "Copy text writes a message's stored body verbatim through `writeText` as plain text only, never HTML or rich clipboard items; invisible Unicode format characters in a body (bidi overrides, zero-width and tag characters) are copied as stored, so pasted text may read differently from what the chat showed (residual). Right click, long press and the context-menu key open the message menu only outside links, pictures, file links, the quote and reaction chips, so the browser's own link menu, which shows a link's destination, stays available."
  - **T-18** adds: "Copied text enters no log, diagnostic or request; the emoji picker makes no request and logs nothing, and uses in-repo Unicode text with no fonts, images or remote service."
- Not changed: `knowledge/specs/api.md`, the frame specification, `docs/development.md` (no `page.clock` use is added), `knowledge/architecture/detailed.md` (no dependency).

## Texts

New en/ru keys in `texts.en.ts`/`texts.ru.ts`: Copy text / Копировать текст; Text copied. / Текст скопирован.; Couldn't copy the text. Select it and copy it. / Не удалось скопировать текст. Выделите его и скопируйте.; Emoji / Эмодзи (button and group); one name per emoji in both languages. Removed: React / the React step's text if unused elsewhere. "Send" stays as the accessible name.

## Files and order of work

1. `frontend/src/features/messages/MessageMenu.tsx`: controlled open state; the open effect's focus target changed from the first `button` (which would now be a reaction) to the first action item; reaction row; Copy text item and separator rule; removed-item focus rule; `preventScroll`.
2. `frontend/src/features/messages/MessageRow.tsx`: bubble right-click, long-press and keyboard openers with the exclusions; capture-phase click swallow.
3. `frontend/src/features/messages/Conversation.tsx`: `openMenuMessageId`; `menuOf` (Copy text, the read-only menu); copy handler and notice line; Emoji button and insertion; round Send.
4. New `frontend/src/features/messages/EmojiPicker.tsx` and `emoji.ts`; send and emoji icons in `frontend/src/features/messages/icons.tsx` only (not `components/ui/icons.tsx`, which the entry chunk imports).
5. `frontend/src/features/messages/UploadComposer.tsx`: the Emoji slot.
6. Texts; `pages.md`; `realtime-and-video.md`; `docs/threat-model.md`; tests; the plan's committed copy.

## Bundle

The picker, emoji list and icons live in the Messages chunk (estimate under 2 KB gzip); the entry chunk does not change. `pnpm check:bundle` must pass.

## Tests (named; Arrange–Act–Assert, one Act each; no mocks, no skip)

The redesign plan's `message menu gestures and keyboard` maps to the end-to-end describe of that name below; `clipboard refusal preserves focus` maps to the component test of that name.

End-to-end, `frontend/e2e/messages.spec.ts`, all three browser projects:

- **1.** `message menu gestures and keyboard › a right click on a bubble's text opens its menu on its first item without scrolling the history` — Reply focused, the reaction row before it, `scrollTop` unchanged; passes in all three engines (whether Playwright's synthetic right click reproduces real Safari's word selection is not claimed; real Safari ordering is `unverified` unless checked by hand).
- **2.** `message menu gestures and keyboard › Escape closes a menu opened by right click onto the message's Message actions button`.
- **3.** `message menu gestures and keyboard › a right click on a link in a bubble opens no message menu and is not prevented` — `defaultPrevented` observed by a `contextmenu` listener added in Arrange (observation only).
- **4.** `message menu gestures and keyboard › a right click on a reaction chip or the quote opens no message menu and is not prevented`.
- **5.** `message menu gestures and keyboard › a right click inside a text selection made before it opens no message menu`.
- **6.** `message menu gestures and keyboard › Shift+F10 on a focused message opens its menu on its first item`.
- **7.** `message menu gestures and keyboard › the context-menu key on a focused message opens its menu on its first item`.
- **8.** `message menu gestures and keyboard › Shift+F10 on a focused link in a bubble opens no message menu and is not prevented`.
- **9.** `message menu gestures and keyboard › opening a second message's menu closes the first`.
- **10.** `message menu gestures and keyboard › a click outside the menu closes it and leaves focus where the click put it` — clicking the composer leaves focus in the composer.
- **11.** `message menu gestures and keyboard › a touch long press on a bubble opens its menu, with its pointer down not prevented and touch-action and user-select unchanged` — Arrange captures the bubble's computed `touch-action` and `user-select`; the one Act is a single touch `pointerdown` built with `bubbles: true`, `cancelable: true`, `pointerType: 'touch'` and a pointer id, dispatched on the bubble through `page.evaluate`, returning `dispatchEvent`'s result (meaningful only because the event is cancelable); Assert: the result shows it was not prevented, the menu appears (awaited by condition with a deadline against the real app timer, no sleep), Reply holds focus, and both styles equal the captured values. Release, cancellation and the click swallow are covered by component tests 27–32.
- **12.** `message menu gestures and keyboard › the reaction row reacts with one press and closes the menu onto Message actions`.
- **13.** `message menu gestures and keyboard › Remove reaction under the row removes the caller's reaction`.
- **14.** `message menu gestures and keyboard › at 360 px the menu with its reaction row stays inside the window` (viewport 360 × 800).
- **15.** `copy text › Copy text announces the copy and returns focus to Message actions` — the status text and focus in every project; in the `chromium` project the context grants `clipboard-read` and `clipboard-write` and the clipboard is read back after the Act and equals the body exactly, with a body holding U+202E (the `browserName` branch is an assertion branch, not a skip, following `students.spec.ts`'s copy test).
- **16.** `copy text › a message with only attachments, a deleted placeholder and an erased placeholder offer no Copy text`.
- **17.** `copy text › in a read-only chat a student's menu on another member's message holds only Copy text`.
- **18.** `copy text › a chat that becomes read-only while the menu is open drops its write items and moves focus to Copy text`.
- **19.** `the composer › the emoji picker inserts the chosen emoji over the selection and returns focus to the composer with the caret after it` — requests recorded from page load; after the Act none is a font or an image request and none goes outside the OVC origin (CSP-violation and console-error failures of the shared fixture apply).
- **20.** `the composer › Escape closes the emoji picker onto its button`.
- **21.** `the composer › arrow keys, Home and End move through the emoji picker`.
- **22.** Updated: `messages › Delete is the last item of the message menu after a separator and asks first, opening on Cancel` — order: row, Reply, Copy text, Edit, Pin, separator, Delete.
- **23.** Updated: `reactions in a group chat › a student reacts from the menu, …` — reacts from the row.
- **24.** Updated: `Send is the primary action and attach is a named icon button` — round Send named "Send", 44 × 44 (40 × 40 on a fine pointer at 1280 px, per the owner's answer), focus stays in the emptied composer after sending.
- **25.** `frontend/e2e/accessibility.spec.ts`: axe over an open menu with the reaction row and over the open emoji picker, light and dark.

Component tests, `frontend/src/features/messages/MessagesPage.test.tsx` (fake clock injected as today; jsdom's real absent `navigator.clipboard`, no double):

- **26.** `clipboard refusal preserves focus` — Copy text without a clipboard closes the menu, shows the refusal alert, which takes focus, and keeps the message and draft; the rejected-promise branch is the shared `copyText`'s, recorded as covered by its existing callers or `unverified`.
- **27.** `a touch long press on a bubble opens its menu after 500 ms`.
- **28.** `a touch press on a bubble that moves more than 10 px opens no menu after 500 ms`.
- **29.** `a touch press on a bubble cancelled by the browser opens no menu after 500 ms`.
- **30.** `a touch press on a bubble released before 500 ms opens no menu`.
- **31.** `a scroll of the history during a touch press on a bubble opens no menu after 500 ms`.
- **32.** `a 500 ms touch press on a reaction chip ended by pointerup opens no menu and sends no reaction request` (no click dispatched; real browsers send none after a long press).
- **32a.** `after a touch long press on a bubble released without a click, a tap on a reaction chip in that bubble toggles the reaction` (the swallow flag is cleared by the next pointer down).
- **32b.** `the click that ends a touch long press on a bubble's text does not toggle a reaction chip in that bubble` (Arrange: touch pointer down on the bubble text, 500 ms, pointer up; Act: a pointer-generated click on a chip of that bubble; Assert: the menu stays open and no reaction request is sent).
- **32g.** `a keyboard activation of a reaction chip after a touch long press released without a click toggles the reaction`.
- **32h.** `a right click on the bubble of the message whose menu is open keeps the menu and focuses its first item again`.
- **32i.** `a right click that opens the menu keeps a selection made before it in another bubble`.
- **32j.** `a Shift+right click on a bubble opens no message menu and is not prevented`.
- **34a.** `the emoji picker's buttons are unavailable while the conversation is busy and insert nothing`.
- **32c.** `a contextmenu after a touch pointer down on a bubble is not prevented and opens no menu`.
- **32d.** `a Ctrl+click contextmenu on a bubble's text opens its menu on its first item` (mouse pointer down with button 0 and `ctrlKey`, then `contextmenu`).
- **32e.** `a right click that opens the menu removes a selection added after its pointer down`.
- **32f.** `a right click after a selection made before its pointer down keeps that selection and opens no menu`.
- **33.** `a message deleted live while its menu opened by right click holds focus moves focus to its row` (the existing focus effect survives the controlled state).
- **34.** `the emoji picker closes onto the composer when the chat becomes read-only`.
- **35.** Changed: `menu opens on its first action and Escape returns to its button` (unchanged expectation, rerun).
- **36.** Replaced by row tests: `React opens the picker on its first reaction and choosing one shows a marked chip and returns to the actions` → `choosing a reaction in the row shows a marked chip and returns to the actions`; `Escape in the reaction picker returns focus to React` → removed (no picker step; Escape covered by 35); `React focuses the current reaction and Remove reaction removes the chip and returns to the actions` → `the row marks the current reaction and Remove reaction removes the chip and returns to the actions`; `a read-only chat offers no React and shows plain chips` → `a read-only chat shows no reaction row and plain chips`.
- **37.** Changed: `a student may only reply to or react to another member's message, and only in a writable chat` → expects the row, Reply and Copy text.
- **38.** Changed: `a student sees no message actions on another member's message in a read-only chat` → `a student's menu on another member's message in a read-only chat holds only Copy text`.
- **39.** Kept: `touch long press keeps the menu open through pointerup and its generated click` (the button's long press stays).
- **40.** Kept and rerun: `a remote deletion while its menu owns focus restores focus to the placeholder`, `read-only live updates preserve draft and focus but permit own deletion`, `offline blocks Send, Edit, Delete and every attach path and preserves the composer` (Send now found by its name), the Enter-send tests.

## Rendered-screen review

The menu opened by button and by right click, the reaction row (including two lines at 360 px), Copy text's notices, the emoji picker and the round Send, at 1280, 800 and 360 px, light and dark, tutor and student, with synthetic data on the local stack; run with `ui-design-review:ui-review` by a session that has it, or with `design:design-critique` and `design:accessibility-review` as the redesign plan allows; reported as blocked if neither is available.

## Tier, trigger and threats

Standard tier. Security trigger met: untrusted member text flows to the system clipboard, and new pointer and keyboard handlers sit on bubbles that render untrusted content, links and file controls. A standard and a security-profile planning review before the owner, and both reviews again on the final diff. Threats: T-5 and T-6 (examined, not affected: no new request, Copy text reads only the loaded message, the read-only Copy menu reveals nothing new about other members, reaction visibility unchanged); T-14 (affected, wording above); T-18 (examined, not affected given decision 7 and 8's no-diagnostics rule; wording above); T-20 (not affected); T-37 (not affected: no CSP or Permissions-Policy change, no storage, no external assets); T-16 (not affected). Real-device long-press and clipboard behaviour on iOS and Android: `unverified` unless checked on a phone.

## Change checks

Frontend: `pnpm typecheck`, `lint`, `format:check`, `test`, `build`, `check:classes`, `check:bundle`; end-to-end `messages.spec.ts`, `message-attachments.spec.ts`, `accessibility.spec.ts`, `desktop-layout.spec.ts` ("Send stays inside the window") and `ending.spec.ts` (Send `aria-disabled` by name) in all projects; documentation check; Gitleaks. Backend: none. Each run only in a slot the coordinator assigns.

## Acceptance criteria

1. End-to-end tests 1–14 pass: every opener opens the existing menu on its first item without scrolling; links, chips, the quote and a prior selection keep the browser's menu unprevented; Escape returns focus to "Message actions"; an outside click leaves focus where it went; one menu at a time; the long press prevents nothing and changes neither `touch-action` nor `user-select`; the row stays inside 360 px.
2. Component tests 27–33, 32a–32j and 34a pass: each long-press cancellation opens nothing; a chip press writes nothing; only the long press's own click is swallowed; a touch `contextmenu` is never prevented; Ctrl+click opens the menu; the browser-added word selection is removed and a prior selection kept; a live deletion focuses the row.
3. Tests 15–18 and 26 pass: Copy text copies exactly the body (Chromium read-back, U+202E included), is absent without a body, works in read-only chats, and a refusal alert takes focus.
4. Tests 22–23 and 36–38 pass: Edit stays; Delete last after a separator with its unchanged confirmation; the six M5 reactions in the row; React is gone.
5. Tests 19–21 and 34 pass: emoji inserted over the selection with the caret after it; no font, image or cross-origin request after choosing.
6. Test 24 passes and `desktop-layout.spec.ts` and `ending.spec.ts` pass: round Send named "Send", sizes as decided, focus kept.
7. Test 25 passes with no violations.
8. `rg -n 'emoji.*\.(png|svg|woff2?)|cdn|unpkg|jsdelivr|googleapis' frontend/src/features/messages` exits with status 1 (no match); `git diff --exit-code main -- frontend/package.json frontend/pnpm-lock.yaml` exits 0 (no dependency); `rg -n --pcre2 "^import .* from '(?!\.|react')" frontend/src/features/messages/emoji.ts frontend/src/features/messages/EmojiPicker.tsx` exits with status 1; `pnpm check:bundle` passes with the entry chunk unchanged.
9. Every other existing `messages.spec.ts` and `MessagesPage.test.tsx` test passes unchanged, apart from tests 22–24 and 35–38 above.

## Risks

- Touch long press differs between Chrome for Android and Safari on iOS; Playwright's dispatched events approximate neither, so the real-device check is `unverified` unless run on phones.
- Clipboard read-back works only in Chromium under Playwright; Firefox and WebKit assert the announcement and focus only.
- Controlling the menu from `Conversation` could break M2/M5 focus rules; tests 33 and 35–40 guard them.
- The emoji names add translation text to the Messages chunk; the larger set would add more.

## Changes after the final scoped re-review (revision 4)

- Selection removal is limited to a selection added during this click inside this bubble; any other selection is left; Shift presses get the browser's menu; tests 32i and 32j (major).
- The swallow flag swallows only pointer-generated clicks and clears on swallow, next pointer down (before the exclusion check), pointercancel and keydown; test 32b now proves the swallow on a chip; test 32g (minors).
- Decision 3 says exactly which event OVC stops; the pointer record is consumed by each `contextmenu`; a right click on the open message's bubble keeps its menu with an open-request counter (test 32h); the keyboard path never reads the record; the picker while busy (test 34a); decision 5's unmount cases; test 11's event construction; acceptance 8's import check as a command (minors and nits).

## Changes after the scoped re-review (revision 3)

- The right-click handler opens OVC's menu only after a recorded mouse press (button 2, or button 0 with Ctrl) and never for a touch or pen `contextmenu`; Ctrl+click is in scope; tests 32c and 32d (major).
- The long-press click swallow is armed only for the same press and cleared by the next pointer down and by pointercancel; tests 32a and 32b (major).
- Test 32 no longer contradicts the exclusions: a chip press asserts no menu and no request, with no click dispatched (major).
- Test 11 has one Act, a single dispatched pointer down; Emoji's availability follows `channel.writable` and stays rendered while busy; T-18 wording no longer says the emoji never reaches a request; the impossible "only Message actions remains" clause is dropped; the browser-added word selection is removed after OVC opens its menu, with tests; acceptance 8 checks exit status, dependencies and imports; the open effect's focus target, test 1's WebKit claim, the single `chromium` project and the result-notice citation are corrected (minors and nits).

## Changes after the planning reviews (revision 2)

- Gesture exclusions now cover every interactive descendant of the bubble (quote, chips, pictures, inputs), with a capture-phase click swallow after a long press and tests 4 and 32 (security major; standard minor).
- Touch `contextmenu` is never prevented; the Android overlap is stated in the long-press question and as `unverified` (security minor; standard major).
- Right-click selection is recorded at the right-button pointer down, for macOS word selection, with a WebKit expectation in test 1 (standard major).
- An outside pointer down only closes the menu or picker and focus follows the click; Escape returns to the trigger; a removed focused item moves focus to the first remaining item; live deletion focuses the row (standard majors).
- Copy refusal follows the shared rule (takes focus) by default, put to the owner with alternatives; notice placement, replacement, re-announcement and removal defined (standard major).
- The injected Clipboard double is dropped: the component test uses jsdom's absent clipboard; Chromium-only read-back in an all-project test (standard and security majors).
- Long-press cancellation moved to component tests with the injected fake clock; the end-to-end long press asserts nothing is prevented and the styles are unchanged (standard major).
- Every affected component test is listed with its new expectation; the button long press is kept (standard major).
- `realtime-and-video.md` line 116, the extra pages.md bullets and `docs/threat-model.md` T-14/T-18 wording are now in scope (standard major; security minors).
- Diagnostics rule for copy and emoji; `writeText` called synchronously; Copy outside the write guard; plain text only (security minors and nits).
- One open menu per conversation, Shift+F10 and the context-menu key scoped to the focused row, the 360 px row wrap, the React-controlled insertion with caret restore and selection read at insertion, the picker's focus and close rules, Emoji tied to `readOnly`, `preventScroll`, icons in the Messages chunk, the requests recorded from page load, the extra end-to-end specs in the checks, a concrete `rg` command, renamed owner questions and the review-skill fallback (standard minors and nits).

## Implementation notes (divergences from revision 4, reported to the owner with the change)

- Test 25 checks the reaction row and the emoji picker structurally (a named group of six named pressed-state buttons before Reply, which holds focus; a named group of 40 named buttons with one tab stop) instead of with axe, because `axe-core` is not a project dependency.
- Test 35's focus assertion is its own test, "opening Message actions focuses Reply after the reaction row", so each test keeps one Act.
- Decision 1's "placement needs no new logic" no longer holds: the menu and the emoji picker are shifted horizontally to stay inside the history pane and the window, which the 360 px criterion needs. An open menu covers the rows below it, so a mouse user closes it (outside click or Escape) or opens the menu of a message it does not cover; test 9 opens the lower message's menu and then clicks the upper message's button.
- Test 16 lives in `frontend/e2e/message-copy-retention.spec.ts`, because its worker-scoped far clock cannot be set inside a describe; the change checks run that spec as well.
- Test 32i is the component test "a right click on a bubble opens its menu while another bubble holds a selection", because jsdom clears a selection when focus moves to a button. OVC's own code never removes a selection outside the clicked bubble (it removes only a selection the browser added inside that bubble during the click, tests 32e and 32f), but the browser may: an end-to-end attempt on 2026-11-27 showed Chromium clearing a selection in another bubble on a right click elsewhere, so decision 2's "left as it is" means left by OVC, and no test claims that the browser keeps it.
- Test 9a, "Shift+F10 on another message while a menu is open closes the first menu and opens the other", proves the shared open state without a pointer.
- Decision 2's intersection check uses `Range.intersectsNode` over every range of the selection; a range that only touches the bubble's start without selecting any of its characters also counts, which gives the browser's menu (fails safe).
