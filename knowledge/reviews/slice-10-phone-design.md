# Slice 10 phone design record (change G0)

Status: agent record of slice 10 change G0 (decision D28 of the [slice 10 plan](../plans/slice-10.md)), written against `main` at `ef9a98d` on 2026-11-21, with the owner's answers of the same day.

## What G0 delivers

- The phone frame below 768 px in [navigation](../design/web/navigation.md#frames): the top bar, the bottom bar with its counts, the More sheet with the account, the remaining sections, Notifications, Settings and Sign out, the unconfirmed accounts' reduced bars, the focus rules and the 44 px targets; from 768 to 1023 px the interim frame stays.
- A **Phone** rule under every page heading of [pages](../design/web/pages.md), 24 in all, each naming its page group of D42, and the phone layout standard under shared behavior.
- The bottom bar and sheet components and 20 px icons on phones in [tokens](../design/web/tokens.md).
- The new texts with their chunk and bytes in [states](../design/web/states.md#phone-frame).
- Static mockups at 360 px in [`prototype/phone.html`](../design/web/prototype/phone.html), one or more screens per page group, in both themes.

G0 changes no code and nothing under `docs/`. G1 to G5 build the phone layouts by page group.

## Owner's decisions of 2026-11-21

Asked by multiple-choice questions, each answered with the recommended option.

| Question | Answer |
|---|---|
| Does the task plan of G0 go ahead (specifications in `knowledge/` only, mockups of the frame and of every page group)? | Approved |
| What replaces the interim frame on phones? | A bottom bar below 768 px (Today or Home, Schedule, Homework, Chats, More) with counts, and a thin top bar with the Notifications button; 768 to 1023 px keeps the interim frame |
| Where do More, the account, Settings and Sign out go on phones? | A modal sheet from the bottom edge, opened by More, holding the account's name and role, the remaining sections, Notifications, Settings and Sign out, closed by `Esc`, a tap on the dimmed page or Close, returning focus to More; no new address |
| Which short label does Messages get in the bottom bar? | "Чаты" / "Chats", as `navigation.md` listed; the other labels are the existing section names |
| Are the mockups and the frame and page specifications approved as the basis of G1 to G5? | Approved |
| What stands at the left of the phone top bar? | The OVC logo, so the page title is shown once, by the page's own heading |
| Are the three new frame texts in the global language chunk approved? | Approved: "Чаты" / "Chats", "Ещё" / "More", "Закрыть" / "Close"; the two counted names of Chats were added after the standard review so that the accessible name begins with the visible label (WCAG 2.5.3), see below |
| Does each page keep its page group of D42? | Confirmed, unchanged |

## Entry chunk headroom (K21)

Measured in this worktree at `ef9a98d` with `pnpm install --frozen-lockfile`, `pnpm build` and `pnpm check:bundle`, which printed:

```text
entry and its static imports: 168.1 KB gzip of JavaScript, with the largest language chunk 12.2 KB: 180.4 KB (limit 181.0 KB)
```

The exact sizes, from gzipping the files the build manifest lists for the entry as `check-bundle.ts` does, were 168,128 bytes of static JavaScript and 12,243 bytes for the Russian language chunk (the English one is 9,364), so the entry held 180,371 of 181,000 bytes and 629 bytes were left. Inserting the five new keys into the built chunks added 61 bytes to the Russian chunk's gzip size (15 for `section.chats`, 46 for `frame.more`, `frame.moreClose`, `frame.chatsWithCount` and `frame.chatsOverCap` together) and 43 to the English one's, which leaves about 568 bytes. This is an estimate on the built file; G1 confirms it with `pnpm check:bundle` on its own build, and no budget changes. The same headroom must also hold G1's own entry-chunk code (the bottom bar, the sheet and the new icons), which is the risk K21 names: if it does not fit, the budget change is the owner's separate decision, not part of G1.

## Page groups

| Group | Pages with a **Phone** rule |
|---|---|
| G1 | The phone frame of both roles, the public frame, Notifications |
| G2 | Today, Schedule, Lesson, Students, Student card, Groups and group page, Settings (the tutor's) |
| G3 | Sign in, Sign up, Accept invitation, Confirmation, password reset, unsubscribe, Home, Schedule (student), My tutors, Keep or drop earlier links, the student's settings |
| G4 | Library, Builder, Issue dialog, Issued work and issue page, Review queue and review, Homework (student) |
| G5 | Messages of both roles |

## Trust boundaries the phone specifications keep

Each rule that touches a boundary says that the phone layout changes only the layout:

- the More sheet's Sign out keeps the sign-out rules, and the sheet unmounts on an account change (T-2, T-29);
- the bottom bar and the sheet hide only what the API refuses, by the existing access rule (T-44);
- the token pages keep the address replacement, the in-memory token and `Referrer-Policy: no-referrer` (T-27, T-28);
- the Lesson page keeps the new-tab join with the pre-opened tab and the cleared opener, and phones never embed the room (T-10, T-16, T-24, T-25, T-37);
- the student's homework pages and Home show no key, points or score on any width (T-9, T-30), and uploads and downloads keep their routes and signed addresses (T-13, T-35);
- Messages keeps its rendering of bodies, links and attachments (T-14).

No rule changes the content security policy, response headers or served files. The mockups are under `knowledge/`, outside every served directory, load only the font host the existing prototype loads, and show only invented data with email addresses under `example.com` and `.test`.

## Reviews

Standard tier with the security trigger met: one standard reviewer and one security-profile reviewer over the complete working-tree diff against `ef9a98d`, both fresh and read-only.

- **Security-profile reviewer:** no blocker or major finding; every affected threat examined and not weakened (T-2, T-29, T-44, T-27, T-28, T-10, T-16, T-24, T-25, T-37, T-9, T-30, T-13, T-35, T-14), T-11 and T-18 not affected; no threat-model update needed, because no actor, asset, data flow, request, address, storage, header or served file changes. Its minor finding (the unconfirmed student's line in `pages.md` named only the sidebar and the top bar) was fixed, and its two notes (a sign-out failure after the sheet was closed, an open sheet when the window crosses 768 px) were specified. None of these fixes touches a trust boundary.
- **Standard reviewer:** four major findings, all fixed in the specifications and the mockup: the Chats item's accessible name did not begin with its visible label (WCAG 2.5.3), fixed with two counted names that begin with "Chats" / "Чаты"; Back to the list on phones had gained a focus rule beyond layout, removed so that the route change moves focus to the heading as before; crossing 768 px had no focus rule, now specified (open sheets and panels close, focus inside the page stays, focus on a frame control the other frame lacks moves to the page heading); the mockup's explanations were in Russian, now in English with the interface copy kept Russian. Its minor findings were fixed: labels that do not fit wrap, lesson pages always mark Schedule, More's current state is visual and the sheet's row carries `aria-current`, the sheet's first focus target, the sheet's Sign out among the busy sign-out controls, the older **Phone** bullets marked as kept beside the new ones, dialogs on phones add no close control, Keep or drop keeps its radio choice, the headroom note on G1's own code, the mockup's texts, toasts and menus above the bar, and the current item's label at weight 600. The reviewer's remark that WebKit's handling of `scroll-padding-bottom` under Tab is unverified is left to G1's tests.

- **Scoped re-review of the fixes** (standard checklist, as the after-fix rule requires after major fixes): all four major findings closed and the minor fixes present; three new minor findings (the sheet's Notifications count against the counts sentence, the review screen's older **Phone** bullet, the mockup's current label weight) were fixed at once and start no further round.
- **Owner, 2026-11-21:** approved the two counted names of Chats added by the WCAG 2.5.3 fix, bringing the new texts to 61 bytes of the Russian chunk's gzip size.

`tools/docs/check-docs.sh` passed after the fixes.
