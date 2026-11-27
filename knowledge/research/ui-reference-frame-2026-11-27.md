# Signed-in frame: reference study, checked 2026-11-27

Status: agent research, checked 2026-11-27 from public first-party sources; decides nothing. Product behavior is volatile; recheck before relying on it. Supplies the reference study that the AGENTS.md UI work rule requires for R1a (`feat/redesign-sidebar-rail`) and R1b (`feat/redesign-panel-resizing`) of the umbrella redesign plan `knowledge/plans/redesign.md` (revision 3 at commit `8b3a8b0`, approved by the owner on 2026-11-27 as a sequencing plan). "FP" marks a first-party source, "TP" a third-party one; numbers refer to the source list at the end.

## Scope and OVC baseline

The study covers the signed-in desktop frame of both roles: the left sidebar, its collapse to an icon rail, and the resizable borders between the sidebar and the page and between the Messages channel list and the conversation. Phone and interim frames below 1024 px are out of scope except where Messages switches to one pane.

What OVC has today:

- [Navigation, Frames](../design/web/navigation.md#frames): a 248 px sidebar with the avatar and name, a "Hide sidebar" icon button, the role's sections with counts, Notifications and Settings; when hidden on a computer a "Show sidebar" button stays top-left with an `accent-text` dot and an accessible name ending in "new items"; the choice is remembered per browser (the `ovc.sidebar` key holding `shown` or `hidden`). From 768 to 1023 px a top bar and a modal sidebar overlay; below 768 px the phone frame with a bottom bar.
- [Tokens, layout](../design/web/tokens.md#spacing-radius-elevation-layout): sidebar 248 px, breakpoints 768/1024/1280 px, sidebar items 32 px tall and its icon buttons 28 × 28 px with a fine pointer from 1024 px, 44 × 44 px otherwise. [Tokens, Motion](../design/web/tokens.md#motion): 200 ms for sidebar collapse, transitions removed under `prefers-reduced-motion: reduce`.
- [Pages, Messages](../design/web/pages.md#messages): a fixed channel list (300 px in the implementation per plan section 4) and one-pane routing below 768 px; the Messages sidebar item counts channels with unread messages from `GET /channels/unread`.
- Components: `frontend/src/components/frame/SignedInFrame.tsx`, `Sidebar.tsx`, `PhoneFrame.tsx`, `FrameDialog.tsx`, `RouteFocus.tsx`; Messages layout in `frontend/src/features/messages/`.
- Threat model T-37 ([threat model](../../docs/threat-model.md)): the access token never sits in Web Storage; the plan requires R1a and R1b to amend T-37 and the storage contract for the new presentation keys.

What the owner already decided (plan section 3):

- Item 7: a separator handle appears on hover or keyboard focus; sidebar 200–320 px, chat list 280–480 px; double click resets; widths remembered per device; arrows change by 16 px; Home/End select the limits; an accessible separator exposes the value; a 64 px icon rail keeps count badges, tooltips and an expand control; every page uses the freed width; a 1024 px window collapses automatically; below the panel minima Messages shows one pane; collapse takes 200 ms with no animation under reduced motion.
- Item 10: defaults 232/340 px; automatic rail at 1024–1279 px; existing interim and phone frames below 1024 px; one-pane Messages chosen from available width with a 480 px conversation minimum including padding and borders; the tutor phone bottom bar stays until PHONE.
- Item 8: count changes in R1a wait for the notifications change; no global counter.
- Plan section 7: persist only typed collapse and width preferences, separate from automatic collapse; `shown` maps to expanded and `hidden` to rail; responsive clamping never rewrites the saved preference; keys survive sign-out because they hold no identity.

Mockup: `knowledge/design/web/prototype/redesign/panels.html` at `8b3a8b0`, four static 1280 × 800 states: hover on the border (232/340/708 px), during a drag (232/420/628 px, a guide line and a width label following the border), the sidebar collapsed (64/380/836 px, badges on icons, "Expand menu"), and the drawn notes on keyboard steps, limits and per-device memory.

## How the compared products solve it

### Telegram Desktop and Telegram Web

- Telegram Desktop 1.0.1 alpha (16 January 2017) added resizing of the chats list with mouse press-and-drag, and 1.0.2 (19 January 2017) added an option to minimize the chat list into a column of profile pictures by clicking and dragging [1].
- Version 2.1.15 beta (30 June 2020) fixed saving the chats list width between application relaunches, so the width is a persisted preference [1].
- Version 2.0 (30 March 2020) added a folders side bar to the left of the chats list [1]; the folders announcement says the sidebar with folder icons exists because of the extra space on computer screens [2].
- Version 5.0.2 (24 May 2024) allows a wide chats list in an empty window while the narrow list is kept when a chat is opened, so the narrow state belongs to the two-pane layout [1].
- Since December 2014 (0.6.18) Telegram Desktop has a single-column layout for a small window [1]; the compact-mode post says the window can be resized until only one column remains, for smaller laptop screens [3].
- Neither source gives pixel widths, minima or keyboard resizing. Telegram Web was not confirmed by any first-party page (see Unverified).

### Slack

- The sidebar is resized by clicking and dragging its edge [4].
- Right-clicking the sidebar chooses between icons only and icons and text [4].
- With the keyboard, F6 moves focus to the left sidebar section and Left/Right arrows then resize it; Shift+F6 moves focus back [5].
- Cmd+. (Ctrl+. on Windows and Linux) hides the right sidebar; Slack's own shortcut list names no shortcut that hides the left sidebar [5].
- Slack's pages state no widths, limits or whether sidebar preferences sync across devices [4].

### Visual Studio Code

- Cmd+B (Ctrl+B on Windows and Linux) toggles the Primary Side Bar's visibility (`workbench.action.toggleSidebarVisibility`) [6, 7]; Option+Cmd+B (Ctrl+Alt+B) toggles the Secondary Side Bar [8].
- Views and editor groups are resized by grabbing the sash between them [8]; `workbench.sash.size` sets the size of the drag area in pixels, to be raised when resizing with the mouse is hard (version 1.46) [9]; `sash.hoverBorder` colours the sash on hover (version 1.52) [10].
- VS Code remembers the layout of views and panels across sessions; View: Reset View Locations restores positions [8]; the workbench reopens with the previous folder, layout and files [7].
- F6 (Focus Next Part) moves between the major areas of the workbench [11].
- The pages read state no default sash size, no double-click behavior and no keyboard resizing of the side bar.

### Linear

- The left and right sidebars are resized by clicking and dragging their edges (changelog of 27 April 2020) [12].
- The sidebar collapses fully with the `[` key, a click on the sidebar border or "Collapse" in the command menu; `[` again, the top-left sidebar icon or "Expand" restores it (changelog dated 12 January 2023) [13].
- The 12 March 2026 refresh made navigation sidebars slightly dimmer so the content stands out, and fixed a collapsed sidebar that sometimes did not open on hover, which shows that a collapsed sidebar opens on hover [14].
- Items can be hidden behind a More menu, and unread notifications can show as a count or a dot (changelog of 18 December 2024) [15].
- Linear collapses fully rather than to an icon rail; no widths or limits are documented.

### Notion

- The sidebar is resized by clicking and dragging its right edge [16].
- The `<<` icon collapses the sidebar and `>>` opens it; Cmd+\ (Ctrl+\ on Windows) opens and closes it [16].
- Notion's page states no widths, limits, hover behavior or phone behavior [16].

### WAI-ARIA Authoring Practices: window splitter

- A window splitter is a focusable element with role `separator`; `aria-valuenow`, `aria-valuemin` and `aria-valuemax` give the position of the primary pane, `aria-controls` references the primary pane, and the name comes from `aria-labelledby` (the pane's visible label) or `aria-label` [17].
- Left/Right arrows move a vertical splitter; Enter collapses the primary pane and restores the previous position; Home (smallest primary pane) and End (largest) are optional; F6 to cycle panes is optional [17].
- The page notes that the pattern is under review and its example is still in development [17].
- WCAG 2.2 success criterion 2.5.7 Dragging Movements (AA) requires every dragging operation to be achievable by a single pointer without dragging; a keyboard alternative alone does not satisfy it [18].

## Comparison

| Question | Telegram Desktop | Slack | VS Code | Linear | Notion | OVC decision/target |
|---|---|---|---|---|---|---|
| Resize by dragging the border | Chats list, since 2017 [1] | Sidebar edge [4] | Sash between views [8] | Left and right sidebars [12] | Right edge [16] | Sidebar and chat list, handle on hover/focus (item 7) |
| Documented widths and limits | None found | None found | Drag area size setting [9] | None found | None found | 200–320 / 280–480, defaults 232/340 (items 7, 10) |
| Collapsed form | Column of profile pictures [1] | Icons only, via right click [4] | Hidden entirely [6] | Hidden entirely, opens on hover [13, 14] | Hidden entirely [16] | 64 px icon rail with badges, tooltips, expand (item 7) |
| Collapse control | Drag to minimum [1] | Context menu [4] | Cmd/Ctrl+B [6] | `[`, border click, command menu, icon [13] | `<<`/`>>`, Cmd/Ctrl+\ [16] | Collapse/Expand button; shortcut open |
| Keyboard resize | Not documented | F6 then arrows [5] | Not documented | Not documented | Not documented | Focusable separator, arrows 16 px, Home/End (item 7; APG [17]) |
| Reset | Not documented | Not documented | Reset View Locations [8] | Not documented | Not documented | Double click to default (item 7) |
| Persistence | Width across relaunches [1] | Not documented | Layout across sessions [8] | Not documented | Not documented | Per device; saved preference separate from responsive clamp (plan section 7) |
| Narrow window | Single column [1, 3] | Not documented | Not documented | Not documented | Not documented | Auto rail 1024–1279 px; one-pane Messages below the minima (item 10) |
| Counts when collapsed | Not documented | Not documented | Not applicable | Count or dot [15] | Not applicable | Count badges on rail icons (item 7) |

## What OVC adopts

- **Drag the border to resize** → Slack, Linear, Notion, Telegram Desktop, VS Code [1, 4, 8, 12, 16] → item 7's separator on the sidebar and chat-list borders; R1b's bounds are OVC's own because no product documents numbers.
- **Collapse to a narrow column that keeps identity** → Telegram's column of profile pictures and Slack's icons-only mode [1, 4] → item 7's 64 px rail with icons, count badges and tooltips, rather than the full hide of VS Code, Linear and Notion; R1a acceptance "rail is 64 px".
- **Explicit, visible collapse control** → Notion's `<<`/`>>` and Linear's top-left icon [13, 16] → item 7's expand control on the rail and the existing Hide control turned into Collapse.
- **Persisted width as a per-device preference** → Telegram Desktop fixed width persistence across relaunches; VS Code remembers layout [1, 8] → item 7 per-device memory; R1b acceptance "only approved primitive keys appear in storage".
- **Keyboard-operable separator** → Slack's F6 then arrows [5] and the APG window splitter [17] → item 7's focusable `separator` with `aria-valuenow`/`aria-valuemin`/`aria-valuemax`, arrows by 16 px and Home/End for the limits; R1b acceptance "ARIA values".
- **Larger grab area than the drawn line** → VS Code's `workbench.sash.size` [9] → the separator's hit area meets the frame's target sizes while the drawn handle stays thin.
- **Narrow windows lose a column instead of squeezing** → Telegram's single-column layout [1, 3] → item 10's one-pane Messages below a 480 px conversation minimum and the automatic rail at 1024–1279 px.
- **Counts survive collapse** → Linear's count-or-dot choice [15] → item 7 keeps counts as badges on rail icons, which replaces today's single dot on "Show sidebar".

## Where OVC differs and why

- **Rail instead of a full hide.** VS Code, Linear and Notion hide the sidebar entirely [6, 13, 16]; the owner chose a rail that keeps navigation and counts visible (item 7), and OVC's sections are few enough to fit as icons.
- **Defined numeric bounds.** No compared product documents limits; OVC fixes 200–320 and 280–480 px (item 7) so the conversation keeps its 480 px minimum (item 10) and pages keep their reading widths.
- **Saved versus effective width.** Products document only that width persists [1]; OVC separates the saved preference from the responsive clamp and the automatic rail, which never rewrites it (plan section 7).
- **Storage scope.** Only bounded presentation values are stored, per device, never identity or tokens, and they survive sign-out; T-37 forbids tokens in Web Storage and R1a/R1b amend it.
- **Enter on the separator.** The APG maps Enter to collapse and restore [17]; item 7 maps reset to double click and does not name Enter, so the keyboard reset path is an open point.
- **Single-pointer alternative.** WCAG 2.5.7 needs a non-dragging pointer path [18]; none of the products document one for width, and OVC's double click only resets.
- **No hover-only opening.** Linear's collapsed sidebar opens on hover [14]; OVC's rail opens only by an explicit control, because hover opening has no touch or keyboard equivalent and moves focus targets under the pointer.
- **No global counter.** Rail badges are section counts from their authorized sources, never a global total (item 8, plan section 7).
- **Focus is never lost.** AGENTS.md requires focus to survive layout changes; a disappearing separator or Collapse control moves focus to the documented surviving control (R1a/R1b acceptance), a rule none of the product pages describe.

## Open points for the task plan

- Whether a keyboard shortcut toggles the rail (compare Cmd/Ctrl+B [6], `[` [13], Cmd/Ctrl+\ [16]); conflicts with browser, screen reader and composer shortcuts must be checked.
- The keyboard equivalent of double-click reset (Enter, as APG suggests for collapse [17], or another key), and whether Enter on the sidebar separator collapses to the rail.
- The single-pointer alternative for width that WCAG 2.5.7 requires [18]: for example step buttons, a menu with presets, or a documented argument that collapse plus reset suffice.
- The separator's hit-area width against the 28 px fine-pointer and 44 px coarse-pointer target sizes, and whether coarse pointers get separators at all from 1024 px.
- The value exposed in `aria-valuenow` (pixels or percentage) and its spoken text (`aria-valuetext`, for example "232 pixels"), and the separator's accessible name in both languages.
- Whether the rail's tooltips also serve as accessible names, and how the count is spoken on a rail icon (reuse "Homework, 3 to check").
- The automatic-rail transition at 1024–1279 px: whether the expand control there opens an overlay or widens the sidebar temporarily, and what happens to focus when the window crosses 1280 px.
- How the 200 ms collapse avoids layout jumps in Messages (draft, scroll anchor) and which properties animate.
- Storage key names, value schema, and handling of invalid, non-finite or out-of-range stored values.

## Unverified

- Telegram Web's resizable left column and its limits: no first-party page found.
- Telegram Desktop's chat-list width limits and whether double click resets it: not documented in the changelog.
- Slack's Cmd/Ctrl+Shift+D to show or hide the left sidebar: reported by third-party shortcut lists in search results, absent from Slack's own shortcut page [5]; a third-party page returned HTTP 403.
- Slack's sidebar minimum and maximum widths, and whether the width syncs across devices.
- VS Code's double-click behavior on a sash, its default sash size and any sash hover delay setting: no first-party page confirmed them.
- Notion's hover peek of a collapsed sidebar and its sidebar width limits.
- Linear's sidebar width limits and whether its main sidebar width persists.

## Sources

1. Telegram Desktop — changelog.txt in the official repository (entries 0.6.18, 1.0.1, 1.0.2, 2.0, 2.1.15, 5.0.2) — <https://raw.githubusercontent.com/telegramdesktop/tdesktop/dev/changelog.txt> — FP — checked 2026-11-27
2. Telegram — Chat Folders blog post — <https://telegram.org/blog/folders> — FP — checked 2026-11-27 — confirms the desktop folder sidebar, not counters on its icons
3. Telegram — Telegram Desktop Adds Compact Mode — <https://telegram.org/blog/desktop-compact> — FP — checked 2026-11-27 — no year or widths shown
4. Slack — Adjust your sidebar preferences — <https://slack.com/help/articles/212596808-Adjust-your-sidebar-preferences> — FP — checked 2026-11-27
5. Slack — Slack keyboard shortcuts — <https://slack.com/intl/en-gb/help/articles/201374536-Slack-keyboard-shortcuts> — FP — checked 2026-11-27
6. Visual Studio Code — Default keyboard shortcuts reference — <https://code.visualstudio.com/docs/reference/default-keybindings> — FP — checked 2026-11-27
7. Visual Studio Code — User interface — <https://code.visualstudio.com/docs/getstarted/userinterface> — FP — checked 2026-11-27
8. Visual Studio Code — Custom layout — <https://code.visualstudio.com/docs/configure/custom-layout> — FP — checked 2026-11-27
9. Visual Studio Code — May 2020 (version 1.46) release notes — <https://code.visualstudio.com/updates/v1_46> — FP — checked 2026-11-27
10. Visual Studio Code — November 2020 (version 1.52) release notes — <https://code.visualstudio.com/updates/v1_52> — FP — checked 2026-11-27
11. Visual Studio Code — Accessibility — <https://code.visualstudio.com/docs/configure/accessibility/accessibility> — FP — checked 2026-11-27
12. Linear — Improved My Issues view (changelog, 2020-04-27) — <https://linear.app/changelog/2020-04-27-improved-my-issues-view> — FP — checked 2026-11-27
13. Linear — Collapsible sidebar (changelog, dated 2023-01-12) — <https://linear.app/changelog/unpublished-collapsible-sidebar> — FP — checked 2026-11-27 — the address contains "unpublished"
14. Linear — UI refresh (changelog, 2026-03-12) — <https://linear.app/changelog/2026-03-12-ui-refresh> — FP — checked 2026-11-27 — hover opening inferred from a bug fix
15. Linear — Personalized sidebar (changelog, 2024-12-18) — <https://linear.app/changelog/2024-12-18-personalized-sidebar> — FP — checked 2026-11-27
16. Notion — Navigate with the sidebar — <https://www.notion.com/help/navigate-with-the-sidebar> — FP — checked 2026-11-27
17. W3C WAI — ARIA Authoring Practices Guide, Window Splitter Pattern — <https://www.w3.org/WAI/ARIA/apg/patterns/windowsplitter/> — FP — checked 2026-11-27
18. W3C WAI — Understanding SC 2.5.7 Dragging Movements (WCAG 2.2) — <https://www.w3.org/WAI/WCAG22/Understanding/dragging-movements.html> — FP — checked 2026-11-27
