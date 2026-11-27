# Settings: reference study, checked 2026-11-27

Status: agent research, checked 2026-11-27 from public first-party sources; decides nothing. Product behavior is volatile; recheck before relying on it. Supplies the reference study that the AGENTS.md UI work rule requires for R8 (`feat/redesign-settings`) of the umbrella redesign plan `knowledge/plans/redesign.md` (revision 3 at commit `8b3a8b0`, approved by the owner on 2026-11-27 as a sequencing plan). "FP" marks a first-party source, "TP" a third-party one; numbers refer to the source list at the end.

## Scope and OVC baseline

The screen is Settings for both roles: section navigation, the profile form, account preferences (language, timezone, theme), the tutor's Teaching section, optional emails, My data (the export) and the security actions (change password, sign out everywhere, delete account). The design questions are where each function lives, how a section is addressed and restored by Back, how Save behaves and where focus goes on success and failure, and how the destructive actions are placed and confirmed.

OVC today, per [pages: Settings](../design/web/pages.md#settings) and the code in `frontend/src/features/settings/`:

- Sections are routes: `/settings/profile`, `/settings/account`, `/settings/notifications`, `/settings/teaching` (tutor only) and `/settings/data` (`navigation.md`, tutor and student address tables). `SettingsNav.tsx` renders them as a wrapping row of `NavLink`s above the section, named by `settings.sections`.
- Profile holds name, avatar, bio and subjects (tutor). Account holds language, timezone (a search field above a select of IANA zones) and theme (system, light, dark) with one Save, whose success is announced by a live region (`AccountSettingsPage.tsx`, `LiveNotice`), plus the Password row whose dialog closes onto a focused notice "Your password has been changed."
- "Sign out everywhere" and "Delete account" are items of the header menu "Account actions", in `danger` text; each opens a confirmation dialog (sign out everywhere opens on Cancel; deletion asks for the password and names the role's consequences).
- Optional emails save per switch; a failed save puts the switch back, keeps focus and shows an alert with Retry.
- My data requests the export through a password dialog and puts "Delete export" in the menu "Export actions" ([pages](../design/web/pages.md), desktop contracts, My data item).
- Layout: settings rows with a 240 px label column and the control on the right from 768 px; on phones the section links wrap as 44 px links and rows stack.

Owner's decisions that bind R8 (plan section 3): item 15 retains every Settings feature the drawing omits, including Teaching and the theme controls; item 16 places destructive actions in confirmed menus, keeps one primary action per view, hides internal identifiers and keeps the shareable tutor code; item 6 forbids underlined action links; items 2–4 set the palettes and flat background. Plan section 4 asks for "section navigation and calmer profile form" while retaining teaching, theme, export, password and deletion contracts. R8 is standard tier with the security trigger met (account, avatar, export and deletion surfaces).

Mockup: `knowledge/design/web/prototype/redesign/settings.html` at `8b3a8b0`, 1280 × 800, dark. It shows a vertical section list in a 218 px column (Profile, Email notifications, Security, My data, Language) beside the Profile section: photo row with initials and "Change photo", Name with a hint, Email as text with "Change" and a "Confirmed" mark, Timezone as a select with a hint, a right-aligned primary Save, and a footer line pointing to Security for password and devices. A note under the list says account deletion is in Security, at the bottom, in a danger zone. It omits Account preferences as a section, Teaching, bio, subjects and the theme.

## How the compared products solve it

### GitHub (personal settings)

- Settings opens from the profile picture menu and is a page with a left sidebar of sections; the sidebar has labelled groups, for example an "Access" group holding "Password and authentication" [1, 2, 3].
- Profile fields sit under "Public profile": Name, Bio (160 characters), Location, an optional "Display current local time" and a "Time zone" dropdown, all saved together by one "Update profile" button [3].
- The profile picture has its own flow outside that Save: "Edit" → "Upload a photo..." → crop → "Set new profile picture", and "Remove photo" reverts to the generated identicon [3].
- Theme lives in its own "Appearance" section with a "Theme mode" dropdown: "Single theme", or day and night themes that follow the system setting [4]. The page does not say whether a change needs saving [4].
- Password change is a form under "Change password" (old, new, confirm) with "Update password" [2].
- Data export sits in the Account section under "Export account data" with "Start export" / "New export"; the archive link is emailed, expires after seven days by default and can be withdrawn earlier with "Delete" [5].
- Account deletion is at the bottom of the Account section under a "Delete account" heading; "Delete your account" opens the dialog "Make sure you want to do this", which asks for the username or email and a typed phrase; GitHub advises backing up data first [1].
- The red "Danger Zone" block with a multi-step confirmation (two acknowledgements and the typed repository name) is documented for repository settings, not for personal account settings [6].

### Notion

- Account items sit under Settings → the person's name, and app preferences under Settings → Preferences; on phones Settings opens from the sidebar menu [7].
- Profile: the photo (or initial) with "Add photo", "Preferred name"; email is managed under "Account security" → "Manage emails", where a new address is verified with a code before "Make primary" [7].
- Preferences: "Appearance" with "Use system setting", "Light" or "Dark" and a keyboard shortcut to toggle dark mode; "High contrast", saved per device; "Language & Time" with Language, "Start week on Monday" and either a manual time zone or "Automatically update time zone" [7].
- "Log out of all devices" keeps the current session active; a session table has a "Log out" button per device [8].
- "Delete my account" is in the account section; its confirmation lists the workspaces that will be deleted, requires typing the account email, and its button names the consequence, "Permanently delete account and {#} workspaces"; the help page says to export data first and that content may be restorable within 30 days through support [9].

### Slack

- The profile editor opens from the profile picture → "Profile" → "Edit"; it holds full name, display name, title, pronouns, phone and time zone, and saves with "Save Changes" (desktop) or "Save" (mobile) [10].
- Theme is under Preferences → Appearance → "Color Mode" with Light, Dark or System; the preference is device-specific; the article only says to close the panel when done [11].
- Deactivation is in Account settings near the bottom under a "Deactivate account" heading: "Deactivate your account" → re-enter the password → "Yes, deactivate my account" [12].

### Linear

- Account settings are split into Profile, Preferences and Security & access [13, 14, 15].
- Profile shows initials as the default avatar, "Change avatar" and removal; the email is changed through a pencil icon on hover and confirmed through emails to both the old and the new address [13].
- Preferences hold the default home view, font size and the theme: presets or custom, Light, Dark or "system preference" [14].
- Security & access lists sessions with location and last seen, "Revoke access" per session and "Revoke all", which keeps the current session [15].
- The Profile page documents "Leave workspace" there; account deletion is described only as an admin scheduling the workspace for deletion [13].

### GOV.UK Design System (patterns, not a settings product)

- A summary list shows key facts as rows of a key, a value and optional actions such as "Change"; each change link carries visually hidden text so it reads as "Change name"; irreversible card actions call for a warning or confirmation [16].
- A green notification banner confirms an expected outcome, sits immediately before the `h1`, takes focus through `role="alert"`, is removed on the next page, and never shares a page with an error summary [17].
- An error summary sits at the top of `main`, takes focus, links each error to its field, repeats the inline wording and prefixes the page title with "Error:" [18].
- One primary button per page and a warning button only for serious, hard-to-undo actions are summarised with sources in `knowledge/research/ui-guidelines-2026-11-23.md` (button and question-page guidance; not re-fetched today beyond [16]–[18]).

## Comparison

| Question | GitHub | Notion | Slack | Linear | OVC decision/target |
|---|---|---|---|---|---|
| Section navigation | Left sidebar, grouped [2] | Sections in the settings sidebar [7] | Separate Profile, Preferences, Account entries [10, 11, 12] | Profile, Preferences, Security & access [13]–[15] | Vertical section list as drawn, one route per section (existing addresses) |
| Profile save | One "Update profile" [3] | Not documented | "Save Changes" [10] | Not documented | One primary Save per form |
| Avatar | Separate upload, crop, set, remove [3] | "Add photo" [7] | Separate article | Initials default, change, remove [13] | Keep existing avatar contract and initials fallback (item 14) |
| Email change | Not checked | Verify by code, then primary [7] | Not checked | Confirm old and new by email [13] | Not in FR-1.6; drawn "Change" needs its own decision |
| Time zone | Dropdown in profile [3] | Manual or automatic [7] | Profile field [10] | Not documented | Existing searchable zone choice; placement open |
| Theme | Own Appearance section [4] | Preferences, system/light/dark [7] | System/light/dark, per device [11] | System/light/dark [14] | Retain system/light/dark (item 15) |
| Immediate apply of theme | Not documented | Shortcut implies it [7] | Not documented | Not documented | Open point |
| Password | Form in Password and authentication [2] | "Change password" [7] | Not checked | Passkeys, no password documented | Retain the dialog |
| Sign out of devices | Not checked | All other devices, current kept [8] | Not checked | "Revoke all", current kept [15] | Retain: every session including this one, confirmed |
| Data export | Account → "Export account data", 7-day link [5] | Advised before deletion [9] | Not checked | Not checked | Retain My data section and its menu |
| Account deletion | Bottom of Account, typed confirmation [1] | Account section, typed email, consequence button [9] | Bottom of Account, password [12] | Admin workspace deletion only [13] | Confirmed menu item (item 16), password dialog, role consequences |
| Success feedback | Not documented | Not documented | Not documented | Not documented | GOV.UK banner pattern [17] vs today's live region: open |
| Failed save | Not documented | Not documented | Not documented | Not documented | Field errors plus summary [18]; keep typing |

## What OVC adopts

- **Vertical section list beside the section**, one route per section, the current one marked → GitHub [2], Notion [7], Linear [13]–[15] → the mockup's 218 px list; keeps today's addresses so Back and reload restore the section (plan section 4 "section navigation").
- **Grouped sections**: Profile, then preferences, then security last → GitHub's grouped sidebar [2], Linear's three-way split [13]–[15] → matches the mockup's order while retaining Account and Teaching (item 15).
- **Calm profile rows with one Save** → GitHub "Update profile" [3], Slack "Save Changes" [10] → one primary action per view (item 16); hints under fields as in the mockup.
- **Avatar changed outside the form Save, initials by default** → GitHub's separate picture flow [3], Linear's initials default [13] → item 14; the existing upload contract stays.
- **Theme as system, light or dark** → Notion [7], Slack [11], Linear [14] → item 15 retains it; GitHub shows that Appearance can be its own section [4].
- **Deletion placed last and confirmed with a consequence-naming button, export suggested first** → GitHub [1], Notion [9], Slack [12] → OVC keeps the password dialog, the role's consequences and a link or sentence toward My data; placement stays a confirmed menu item per item 16.
- **Change actions named for screen readers** ("Change email", "Change photo") → GOV.UK summary list [16]; the mockup already uses `aria-label` on its email "Change".
- **Failed form saves keep the typing, report at the field and summarise when there are several fields** → GOV.UK error summary [18] and gap G9 of the guidelines file.

## Where OVC differs and why

- **Destructive actions stay in confirmed menus, not in an always-visible red block.** Item 16 overrides the mockup's "danger zone" note. GitHub's red Danger Zone is a repository pattern [6]; its account deletion is a heading at the bottom of Account [1]. The copy may still call the menu or its group a danger zone, but the control is a menu item with a confirmation dialog. The benchmark audit's suggestion to move deletion out of "···" is superseded by item 16.
- **Sign out everywhere ends the current session too**, unlike Notion [8] and Linear [15], because FR-1.4 and the existing dialog say so; R8 changes appearance only.
- **No typed-name confirmation**: OVC asks for the password (as Slack does [12]) rather than a typed email or phrase [1, 9]; the password also proves possession for the export.
- **No 30-day restore promise** like Notion's [9]: FR-1.7 deletes uploads, and any retention wording needs approved legal text (item 11's rule on legal assurances applies by analogy).
- **Theme is an account setting, not per device** as in Slack [11] and Notion's high contrast [7]: FR-1.6 lists it with the account's preferences, and the typed preference store and theme initializer (T-37) only mirror it.
- **No session list, passkeys, API keys or connected accounts** (Linear [15], GitHub [2]): none are in the requirements and each would cross a trust boundary.
- **Students see no Teaching section and no other student's data**; the profile hint "how students see you" applies to tutors only (ADR-0005 relationship scope).
- **Email shown with "Confirmed" and a "Change" button** is drawn but email change is not in FR-1.6 and has no API; showing the address read-only is the most the current contracts allow.
- **Underlined links are removed** from the section list and footers (item 6); the section list uses non-underlined navigation items with `aria-current="page"`.

## Open points for the task plan

- Which sections the list shows and in what order, given item 15 keeps Account (language, timezone, theme) and Teaching while the mockup draws Language and Security as their own sections; whether "Security" becomes a section holding password, sign out everywhere and deletion, and whether addresses change (`navigation.md` address tables).
- Where the confirmed menu for sign out everywhere and deletion sits once Security is a section (header "Account actions" as today, or a menu in that section), and whether its group is labelled a danger zone in copy.
- Whether the Profile form gains timezone, as drawn, or timezone stays with language and theme in Account; one Save per section either way.
- Whether email is shown read-only in Profile; an email change flow would be a separate requirement, API and threat-model change outside R8.
- Whether the theme applies at once (as the benchmark audit suggested) or stays under Account's Save; reduced-motion and no-flash initialisation must hold either way.
- Save focus: keep today's polite live region for success, or a focused notice before the heading as GOV.UK does [17]; failed saves: field errors with focus on the first invalid field, or an error summary [18]; typed values never cleared.
- Section list behaviour at the 1024–1279 px rail width and below 1024 px, where today's phone rule wraps links above the section (item 10 keeps existing interim and phone frames).
- How the profile hint and the photo row read for a student, and the initials fallback for a deleted avatar.
- Named suites per plan section 7 R8: teaching-settings, email-settings, my-data, account-edge-cases and session, plus focus assertions for each section change and Save outcome.

## Unverified

- That GitHub, Notion, Slack or Linear apply a theme change immediately without a Save button (no first-party page states it).
- That Notion's settings open as a modal dialog rather than a page.
- How GitHub, Notion, Slack and Linear report a failed settings save and where they move focus after a successful one.
- That Slack's preferences open as a dialog with a left list of sections (the theme help page at the guessed address returned 404; the dark-mode article only says to close "the panel").
- GitHub's exact control for changing the account email and its personal-account deletion prerequisites (the fetched page truncated the list).

## Sources

1. GitHub — Deleting your personal account — <https://docs.github.com/en/account-and-profile/setting-up-and-managing-your-personal-account-on-github/managing-your-personal-account/deleting-your-personal-account> — FP — checked 2026-11-27 — prerequisite list truncated in the fetch.
2. GitHub — Updating your GitHub access credentials — <https://docs.github.com/en/authentication/keeping-your-account-and-data-secure/updating-your-github-access-credentials> — FP — checked 2026-11-27 — does not say what happens to sessions.
3. GitHub — Personalizing your profile — <https://docs.github.com/en/account-and-profile/setting-up-and-managing-your-github-profile/customizing-your-profile/personalizing-your-profile> — FP — checked 2026-11-27 — no save button named for Name alone.
4. GitHub — Managing your theme settings — <https://docs.github.com/en/account-and-profile/setting-up-and-managing-your-personal-account-on-github/managing-personal-account-settings/managing-your-theme-settings> — FP — checked 2026-11-27 — immediacy not stated.
5. GitHub — Requesting an archive of your personal account's data — <https://docs.github.com/en/get-started/archiving-your-github-personal-account-and-public-repositories/requesting-an-archive-of-your-personal-accounts-data> — FP — checked 2026-11-27.
6. GitHub — Deleting a repository — <https://docs.github.com/en/repositories/creating-and-managing-repositories/deleting-a-repository> — FP — checked 2026-11-27.
7. Notion — Account settings — <https://www.notion.com/help/account-settings> — FP — checked 2026-11-27 — modal versus page not stated.
8. Notion — Log in and out — <https://www.notion.com/help/log-in-and-out> — FP — checked 2026-11-27 — position of the control not described.
9. Notion — Delete your account — <https://www.notion.com/help/delete-your-account> — FP — checked 2026-11-27.
10. Slack — Edit your profile — <https://slack.com/help/articles/204092246-Edit-your-profile> — FP — checked 2026-11-27 — editor format not stated.
11. Slack — Use dark mode in Slack — <https://slack.com/help/articles/360019434914> — FP — checked 2026-11-27 — immediacy not stated.
12. Slack — Deactivate your Slack account — <https://slack.com/help/articles/203953146-Deactivate-your-Slack-account> — FP — checked 2026-11-27.
13. Linear — Profile — <https://linear.app/docs/profile> — FP — checked 2026-11-27 — also read as <https://linear.app/docs/profile.md>; saving not described.
14. Linear — Account preferences — <https://linear.app/docs/account-preferences> — FP — checked 2026-11-27.
15. Linear — Security & Access — <https://linear.app/docs/security-and-access.md> — FP — checked 2026-11-27.
16. GOV.UK Design System — Summary list — <https://design-system.service.gov.uk/components/summary-list/> — FP — checked 2026-11-27.
17. GOV.UK Design System — Notification banner — <https://design-system.service.gov.uk/components/notification-banner/> — FP — checked 2026-11-27 — the auto-focus opt-out was not on the page.
18. GOV.UK Design System — Error summary — <https://design-system.service.gov.uk/components/error-summary/> — FP — checked 2026-11-27.
