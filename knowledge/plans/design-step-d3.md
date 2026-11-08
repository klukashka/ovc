# Design slice step D3: Messages fills the window

Status: approved by the owner on 2026-11-09, with an opened conversation starting at its latest messages. It carries out decision 4 (Messages) and step D3 of the [interface decisions](ui-ux-decisions-2026-11-06.md#order-of-work) and the [design slice plan](design-slice.md). Base: `main` at `3833f4e`. No slice 7 branch is open.

## 1. Problem and outcome

**Problem.** On a computer, Messages is a page with a large title and two equal columns that grow with the content. With a long history:

- the composer drops below the window, so writing means scrolling the whole page;
- the channel list takes half the width.

**Outcome.** From 1024 px, Messages fills the window's height and width:

- **Title:** no visible page title. The `h1` stays for screen readers, route focus and the skip link, visually hidden.
- **Channel list:** a 300 px column on the left that scrolls on its own.
- **Conversation:** fills the rest. Its header sits at the top and its message history scrolls on its own between the header and the composer.
- **Composer:** fixed at the bottom of the conversation, with:
  - a one-line field that grows with its text up to eight lines and then scrolls;
  - attach as an icon button with an accessible name;
  - Send as the primary action.
- **Rows:** messages and channel rows are divided by hairlines instead of boxed cards.

Below 1024 px the page keeps today's layout:

- 768–1023 px: two columns;
- below 768 px: separate list and conversation screens with Back.

## 2. Verification tier and security trigger

Standard tier. The security trigger is **not met**: this step only restyles and rearranges. The design slice plan reclassifies D3 if it changes message rendering or link parsing, the upload client, the confirm-email prompt before files, the access-loss notice or the student group header (T-6, T-13, T-14, T-21). This plan changes none of them:

- messages render through the same restricted renderer;
- uploads go through the same `UploadComposer` logic;
- the notices and prompts keep their texts and focus rules.

If the final diff touches any of them, the change is reclassified and gets a security reviewer.

One standard reviewer reviews the change, followed by its change checks and its merge gate.

## 3. Question for the owner, asked with this plan

- **Where an opened conversation starts.** Today a long history runs down the page from the oldest loaded message. In a history that scrolls on its own, the recommended choice is that an opened conversation shows its latest messages, scrolled to the bottom. Sending a message, or receiving one while the reader is already at the bottom, keeps it at the bottom. Otherwise the position is kept, as the shared rule "Lists keep the scroll position" says.

  Focus still moves to the conversation's heading on opening, as specified.

## 4. What changes

- **`features/messages/MessagesPage.tsx`:**
  - From 1024 px the page renders a grid `[300px_1fr]` whose height is the window height minus the frame's padding: `h-[calc(100dvh-…)]` built from spacing tokens, so the page itself never scrolls.
  - The `Page` title becomes visually hidden, through a new `titleHidden` prop on `Page` that keeps the `h1` and its focus.
- **`features/messages/ChannelList.tsx`:** the list scrolls inside its column, and rows become hairline-separated items with the unread count on the right.
- **`features/messages/Conversation.tsx`:**
  - the header (heading, presence, links, notices) stays at the top;
  - the history `ol` becomes the scroll container (`overflow-y-auto`, `min-h-0`, `flex-1`);
  - messages are hairline-separated;
  - the composer form stays at the bottom (`shrink-0`);
  - the textarea starts at one line and grows to eight lines through its `rows` and `scrollHeight`, with no inline `style` (the content security policy), using a class and the computed row count;
  - Send uses the primary button style;
  - the opening and stick-to-bottom behavior follows section 3.
- **`features/messages/UploadComposer.tsx`:** the attach control becomes an icon button, a paperclip from `components/ui/icons.tsx`, with the accessible name and tooltip "Attach files" (the existing text), keeping its busy, unavailable and refusal behavior.
- **`frontend/e2e/messages.spec.ts`:** › "at 1440 × 900 the composer's bottom edge stays inside the window with a long history". It seeds 60 messages, opens the conversation, and asserts that the composer's bounding box ends within the viewport and that the document does not scroll vertically.
- **Specifications** (pending owner review):
  - `knowledge/design/web/pages.md` Messages gets a "Computer layout" bullet and the opening position;
  - `knowledge/design/web/tokens.md` gets the composer and the hairline message rows under Components.
- **The WebKit record correction the owner asked for on 2026-11-09:** `knowledge/reviews/design-slice-webkit-failures.md` gets its pairing paragraph corrected. The first failure of that run was a smoke test before the upload test, so WebKit degrades over a long single-worker run, and the item belongs to Slice 10 with the other load diagnoses.

## 5. Acceptance criteria

1. `messages.spec.ts` › "at 1440 × 900 the composer's bottom edge stays inside the window with a long history" passes in Chromium, Firefox and WebKit.
2. › "an opened conversation shows its latest message": the last seeded message is inside the history's visible area. This follows the owner's answer to section 3.
3. › "the composer grows with its text up to eight lines": a one-line field grows when four lines are typed and stops growing at eight.
4. › "Send stays the primary action and attach is a named icon button": the attach button's accessible name is "Attach files" and it has no visible text.
5. The existing focus and behavior tests pass unchanged in meaning: `messages.spec.ts`, `message-attachments.spec.ts`, `upload-transport.spec.ts`, `offline.spec`, the chat parts of `realtime.spec.ts`, and the `MessagesPage.test.tsx` component tests. Locators move only where a visible title or a button text moves. No assertion is removed or loosened.
6. `tools/docs/check-docs.sh` passes.
7. The frontend change checks pass: `typecheck`, `lint`, `format:check`, `test`, `build`, `check:classes`, `check:bundle` and the whole `pnpm e2e`.

## 6. Order of work

1. Worktree outside `/private/tmp` on `feat/design-step-d3-messages` from `main`.
2. The `Page` `titleHidden` prop and the page grid.
3. The channel list and conversation layout, including the history scroll container and the opening position.
4. The composer: the growing field, the primary Send and the attach icon.
5. Tests, then the specifications and the WebKit record correction.
6. Change checks, the standard reviewer, fixes under the after-fix rule, the readiness report, the gate, the merge and finish.

## 7. Observability

None. No backend change and no new request.

## 8. Risks

- **The read markers decide what is shown from the viewport.** In a scroll container they must still count only the visible messages. The existing read tests cover this, and `readMarkers.ts` already listens to scroll in the capture phase.
- **Focus and scroll interplay.** "Show older" must still move focus to the first newly shown message without jumping the container. The existing focus test covers it.
- **Heights on phones.** Phones keep their current layout, so `100dvh` applies only from 1024 px.

## 9. As built

- The attach button's name "Attach files" is its accessible name; the shared `ActionButton` shows a tooltip only while the button is unavailable, where it carries the reason, so the plan's always-visible "Attach files" tooltip was not added.
- The page title is visually hidden only from 1024 px (`titleHiddenOnDesktop` on `Page`); below that it stays visible, as before.
- The layout fills the page width, which `main` caps, not the whole window on very wide screens.
- The history scroll container is a `div` around the history `ol` and its notices, not the `ol` itself. The channel rows keep the unread count after the name, as before; they are not moved to the right edge.
- Read markers now count a message as shown only inside every clipping ancestor of the history, so a message under the pane's edge stays unread; `messages.spec.ts` › "a message that arrives below the history pane edge stays unread until it is scrolled into view" proves it, and fails without the fix.
- The growing composer keeps the history at the bottom when it was there.
- The fourth acceptance test is titled "Send is the primary action and attach is a named icon button".
- The long-history tests seed twelve five-line messages from the tutor and the student in turn instead of sixty one-line messages, which stays under the message rate limit and still overflows a 900 px window.
