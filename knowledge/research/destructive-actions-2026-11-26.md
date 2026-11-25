# Destructive and rare actions: placement and confirmation (research)

Researched 2026-11-26 for the interface rework stage of slice 10; agent research, not a decision.

Every claim below was checked on 2026-11-26 against the source linked next to it, and the full list is in [Sources](#6-sources). Product interfaces change often. Where only a third-party guide or a search-index summary could be reached, the claim is marked **unverified** or **partly verified**. Several design-system pages (Material Design 3, Apple HIG's HTML pages, Discord support) render on the client or refuse automated fetches. For Apple, the HIG's JSON data endpoint was read instead.

The question is how established products place rare, dangerous actions (deletions and actions that end a relationship or a membership) and how they confirm them. The aim is one consistent rule for a small tutoring web app. Its action set includes stopping study with a tutor, removing a student from a group, ending work with a student, archiving a group, deleting a folder, material or assignment, withdrawing an assignment, deleting a data export and deleting an account.

## 1. Summary of the common patterns

- **Separate dangerous actions from ordinary ones.** Two placements dominate, both away from the everyday buttons:
  - an overflow (kebab or "More") menu on the item, used for item-level actions such as removing a member, archiving a class or moving a page to trash;
  - a "Danger zone" section at the bottom of a settings page, used for high-impact, whole-object actions such as deleting a repository, organization, workspace or server, or leaving a workspace.

  NN/g lists placing consequential options next to benign ones among the top application design mistakes. Primer (GitHub) says to put high-impact deletions in a danger zone within settings and keep low-impact ones inline or in a contextual menu.
- **Match friction to the blast radius.** Primer states this outright, and NN/g, Apple and GOV.UK each say a version of it:
  - reversible actions get no confirmation, only undo;
  - consequential actions on one item get a confirmation dialog;
  - irreversible, wide-impact actions get a stronger step, such as typing the name, a checkbox, a password or a separate confirmation page.
- **Don't confirm everything.** NN/g, Apple, Primer and GOV.UK all warn that routine confirmations teach people to click through without reading. Apple says to avoid alerts for common, undoable actions, even destructive ones.
- **Make the confirmation specific.**
  - The title is a question that names the action and the object, such as "Delete repository?". Avoid "Are you sure?".
  - The body states the consequence: what is lost, who is affected, and whether it can be undone.
  - The confirm button names the outcome ("Delete file", "Remove", "Archive channel"), never "Yes" or "OK".
- **Give the destructive control a distinct look, but don't rely on color alone.** Danger red is universal: Primer's `danger` variant, Apple's destructive role and GOV.UK's warning button. Every system adds that text and context must carry the meaning without the color. GOV.UK keeps the red warning button for the final confirmation step only and uses a different button style for the first action.
- **Make the safe choice easy and the default.**
  - Cancel is always present. Apple says never to make Cancel the default button, and to avoid any default button when people should read the alert rather than press Return.
  - Initial keyboard focus goes on Cancel, the least destructive action. Both WAI-ARIA APG and Primer say so.
  - Escape closes the dialog without acting.
- **Typing the name confirms the heaviest deletions.** GitHub uses it for deleting a repository or an organization (the repository flow has two extra acknowledgement steps before it), Notion for deleting a workspace, and Discord for deleting a server on desktop (partly verified). Slack uses a checkbox plus the password to delete a workspace, and a checkbox to delete a channel.
- **Often, "archive" or "trash" comes first and permanent deletion second.**
  - Google Classroom requires a class to be archived before it can be deleted.
  - Slack separates archiving a channel from deleting it.
  - Notion and Gmail move items to a trash that lasts 30 days.
  - GitHub restores some deleted repositories within 90 days.

  Each turns a destructive action into a reversible one for most of its life.
- **Relationship-ending actions (remove a member, leave a team) get a simple confirmation dialog.** Products do this through the item's overflow menu: Google Classroom, Slack and Microsoft Teams. Slack also notifies the removed person. Leaving a whole workspace sits in a danger zone in Notion.
- **On mobile the patterns are the same.** Item actions go behind a "More" menu (Google Classroom on Android), destructive choices sit in an action sheet with Cancel at the bottom (Apple), and some apps use swipe actions (Telegram on iPhone, unverified). Some heavy actions are web-only. Zoom account deletion is done on the web portal, not in the apps.

## 2. Design-system guidance

### Nielsen Norman Group

- *Confirmation Dialogs Can Prevent User Errors — If Not Overused* (Jakob Nielsen, published 2018-02-18):
  - Reserve confirmation for serious consequences, irreversible actions most of all.
  - Avoid it for routine actions, because frequent prompts get approved automatically.
  - Be specific about the effect and what is affected, not a generic "Are you sure?".
  - Label buttons with the action ("Delete file" and "Keep file", not Yes and No) and don't make "Yes" the default.
  - For the most dangerous, rare actions, require an unusual step such as typing a word.
  - Offer undo where possible, because some errors get through anyway.
  - Source: <https://www.nngroup.com/articles/confirmation-dialog/>
- *Dangerous UX: Consequential Options Close to Benign Options* (Page Laubheimer, 2021-02-14):
  - Consequential actions placed next to routine ones cause slips, especially in repetitive work.
  - The article calls this one of the top application design mistakes the authors see each year.
  - Its examples include "Archive Card" sitting next to "Watch" in a mobile overflow menu, and "Delete" sitting near "Confirm and Send".
  - It recommends spatial separation, accepting a slightly longer reach by Fitts' law, and redundant visual coding (color, icon, size, alignment) so the options can be told apart without close attention.
  - Source: <https://www.nngroup.com/articles/proximity-consequential-options/>
- *Preventing User Errors: Avoiding Conscious Mistakes* (Page Laubheimer, 2015-09-07):
  - Provide undo.
  - Confirm destructive actions with a specifically labelled button, used sparingly and not as the only safeguard.
  - Source: <https://www.nngroup.com/articles/user-mistakes/>

### Apple Human Interface Guidelines

Read from the HIG JSON data behind the Alerts and Action sheets pages.

- **Alerts:**
  - Use the destructive style for a button that performs a destructive action people didn't deliberately choose. Don't apply it when the action is the person's original intent: Empty Trash does not mark its button destructive.
  - When there is a destructive action, include a button titled "Cancel" as a clear, safe way out. Never make Cancel the default button.
  - To make people read an alert rather than press Return, avoid making any button the default.
  - Avoid alerts for common, undoable actions, even destructive ones. Deleting an email or a file needs no alert because it can be undone. Alerts are for uncommon destructive actions that can't be undone.
  - Button titles are one or two words that describe the result ("Delete", "Erase"). Avoid "Yes" and "No".
  - The likely choice goes on the trailing side of a row or the top of a stack. Cancel goes on the leading side or the bottom.
  - Source: <https://developer.apple.com/design/human-interface-guidelines/alerts> (data: <https://developer.apple.com/tutorials/data/design/human-interface-guidelines/alerts.json>)
- **Action sheets:**
  - Use an action sheet, not an alert, for choices related to an intentional action, such as deleting a draft or saving it. Use them sparingly.
  - Give destructive buttons the destructive style and put them at the top, where they are most noticeable.
  - Put Cancel at the bottom.
  - The SwiftUI confirmation dialog includes a Cancel button by default.
  - Source: <https://developer.apple.com/design/human-interface-guidelines/action-sheets> (data: <https://developer.apple.com/tutorials/data/design/human-interface-guidelines/action-sheets.json>)

### GOV.UK Design System and related UK government systems

- **Warning button (GOV.UK button component):**
  - Use it sparingly, only for actions with serious destructive consequences that users can't easily undo, such as permanently deleting an account. It is red.
  - Add a separate confirmation step: the first action uses a different button style, and the warning button appears only on the final confirmation.
  - Don't rely on color, because the button text and context must make the consequence clear.
  - Source: <https://design-system.service.gov.uk/components/button/>
- **"Confirm an action" pattern (Ministry of Justice Design System, experimental):**
  - A separate page, not a modal, for assigning, removing, reassigning and deleting when the action is hard to undo or has consequences to highlight.
  - It shows the key details of the item in a summary list or card so users can check they have the right item.
  - It offers the primary action or a way back, and the GOV.UK warning button for destructive actions.
  - A notification banner on the next page confirms the result.
  - Source: <https://design-patterns.service.justice.gov.uk/patterns/confirm-an-action/>
- No page in the core GOV.UK Design System describes a generic "Are you sure?" deletion page. The separate-step rule above is the core guidance. **Unverified:** whether GOV.UK publishes a dedicated delete pattern beyond this.

### Primer (GitHub's design system)

- **Delete scenario pattern:**
  - "Match the amount of friction to the blast radius":
    - type-to-confirm for irreversible, organization-wide or repository-wide deletions;
    - a simple confirmation dialog for single resources;
    - no confirmation for reversible actions such as removing a label, and an undo window instead.
  - Place high-impact deletions in a danger zone within settings, and low-impact ones inline or in a contextual menu.
  - The confirm button uses the `danger` variant with a text label such as "Delete repository". Menu items use the `danger` variant with a trash icon.
  - Use "Remove" when the item still exists elsewhere and "Delete" when it is destroyed.
  - For high-value resources, offer a reversible "Disable" beside "Delete".
  - If deletion fails, keep the dialog open with an error banner.
  - After deletion, move focus to the previous equivalent control or a stable parent, and announce the result through a live region.
  - Source: <https://primer.style/product/scenario-patterns/delete/>
- **ConfirmationDialog guidelines:**
  - Use it for serious, permanent or widespread impact, not for low-risk, easily undone or frequent actions.
  - Titles are specific questions ("Delete repository?"), never generic "Are you sure?".
  - Labels are 1 to 3 action words.
  - Source: <https://primer.style/product/components/confirmation-dialog/guidelines/>
- **ConfirmationDialog accessibility:**
  - With a danger confirm button, initial focus goes to Cancel. Otherwise it goes to Confirm.
  - Focus returns to the triggering element on close.
  - Source: <https://primer.style/product/components/confirmation-dialog/accessibility/>

### Atlassian Design System (modal dialog)

- Modals interrupt, so use them sparingly, for critical or warning information that needs a response.
- Don't rely on color alone to show severity, and give warning icons an accessible label.
- The primary button matches the title: a "Fork <repository name>" title pairs with a "Fork repository" button. Footer buttons sit on the right with the primary rightmost.
- Always provide a header close button. Return focus to the trigger on close. Never open a dialog from a dialog.
- The usage page fetched had no danger-specific guidance. **Unverified:** the danger appearance of Atlassian modals; it is documented elsewhere in the system and was not checked.
- Source: <https://atlassian.design/components/modal-dialog/usage>

### Microsoft (Windows app dialogs, Fluent)

- Every dialog has at least one safe, non-destructive button ("Cancel", "Close"). It also answers Escape and the system back button.
- Button text gives specific responses to the main instruction. The deletion example is titled "Delete file permanently?", with the body "If you delete this file, you won't be able to recover it…" and the buttons "Delete" and "Cancel".
- A default button is optional and none is set unless the app sets one. The default button takes the accent style, responds to Enter and receives focus on open.
- Microsoft places the "do it" button on the left and the safe button on the right, the reverse of Apple's ordering. This is a platform convention, not a rule to copy.
- Source: <https://learn.microsoft.com/en-us/windows/apps/design/controls/dialogs-and-flyouts/dialogs> (page dated 2026-07-15)

### Material Design

**Partly verified:** the Material Design 3 pages render on the client and could not be read directly. A search-index summary of the M3 dialogs guidelines says:

- dialogs should be rare because they interrupt;
- the confirming button names the outcome plainly ("Send", "Create") and sits closest to the edge.

The older Material Design 1 dialog page notes that affirmative actions may be destructive ("Delete", "Remove"), and asks for explicit confirm and cancel buttons so that Back is unambiguous. Material's snackbar guidance says to offer an "Undo" action so users can amend a choice, as when an email app shows "email archived" with Undo.

- Sources: <https://m3.material.io/components/dialogs/guidelines>, <https://m1.material.io/components/dialogs.html>, <https://material.io/components/snackbars>, <https://developer.android.com/training/snackbar/action.html>

## 3. Product examples

### GitHub

- **Placement:** Repository and organization deletion live in a **"Danger Zone"** section at the bottom of the General settings page. If the Settings tab is hidden, it is under the More (kebab) dropdown.
- **Delete a repository:**
  1. Select "I want to delete this repository".
  2. Read the warnings and select "I have read and understand these effects".
  3. Type the repository's name.
  4. Select "Delete this repository".

  Some deleted repositories can be restored within 90 days. Team permissions are deleted permanently, and deleting a private repository deletes its forks.
- **Delete an organization:** In the Danger zone, read the warning, type the organization's name, and confirm with a button that names the outcome: "Cancel plan and delete the organization". It cannot be undone, and the name stays reserved for 90 days.
- **Treatment:** Red danger buttons and a red-bordered danger section, per the Primer `danger` variant above. **Unverified:** the exact current border styling.
- Sources: <https://docs.github.com/en/repositories/creating-and-managing-repositories/deleting-a-repository>, <https://docs.github.com/en/organizations/managing-organization-settings/deleting-an-organization-account>

### Google Classroom (Google Workspace for Education)

- **Remove a student:**
  - On a computer: open the class and then People, check the box next to the student, choose Actions and then Remove, and confirm with Remove.
  - On Android: next to the student's name, tap More and then Remove, then confirm with Remove.
  - Consequences: the student's grades are removed from the gradebook and they lose access to the assignments. Their Drive files stay in the class folder, and their posts and comments stay visible in the stream.
  - Students can unenroll themselves unless an admin restricts it.
- **Archive a class:** From the class card's **More** menu, choose Archive and confirm. Archived classes can be restored with their posts and materials. Archiving also stops students from unenrolling.
- **Delete a class:** A class must be **archived first**, and only the primary teacher can delete it. Deletion is permanent: posts and comments are lost, but files stay in Drive.
- **Mobile:** The same More menu and confirmation on Android and iOS.
- Sources: <https://support.google.com/edu/classroom/answer/6069576?hl=en&co=GENIE.Platform%3DDesktop>, <https://support.google.com/edu/classroom/answer/6069576?hl=en&co=GENIE.Platform%3DAndroid>, <https://support.google.com/edu/classroom/answer/6149813?hl=en&co=GENIE.Platform%3DDesktop>, <https://support.google.com/edu/classroom/answer/6069981?hl=en&co=GENIE.Platform%3DDesktop>

### Gmail (Google Workspace)

- Deleting moves messages to Trash, where they stay for 30 days and can be moved back. After that they are gone. "Delete forever" and "Empty Trash" are separate, explicit actions inside Trash.
- **Unverified:** the details of the immediate "Undo" notice after deleting (its duration and wording). The help pages describe only the Trash recovery.
- Source: <https://support.google.com/mail/answer/7401?hl=en&co=GENIE.Platform%3DDesktop>

### Slack

- **Remove someone from a channel:** Open the member list, hover over the person, open the three-dot menu next to their name and choose "Remove from channel". A confirmation dialog follows: "Click Remove to confirm". Slack notifies the removed person. On mobile, the help page describes a `/remove @member` command instead.
- **Archive a channel:** Open the channel name in the header, then the Settings tab, and choose "Archive channel for everyone". Confirm with "Archive channel". Archiving is reversible.
- **Delete a channel:** In the same Settings tab, choose "Delete channel" and then "Delete this channel". Tick "Yes, permanently delete the channel", then select "Delete Channel". By default only workspace owners and admins can delete.
- **Delete a workspace:** Only the Primary Owner can do it, in Admin, then Workspace settings, in a "Delete Workspace" section at the bottom. They tick a confirmation checkbox, enter their password, and select "Yes, delete my workspace". It is permanent, and Slack suggests exporting first.
- **Deactivate own account:** In Account settings, a "Deactivate account" section at the bottom. They re-enter the password and give a final confirmation.
- **Mobile:** Archive is available from the channel header on mobile. The help page doesn't describe deleting a channel there.
- Sources: <https://slack.com/help/articles/201898668-Remove-someone-from-a-channel>, <https://slack.com/help/articles/213185307-Archive-or-delete-a-channel>, <https://slack.com/help/articles/204067366-Delete-a-workspace>, <https://slack.com/help/articles/203953146>

### Notion

- **Delete a page:** "•••" on the page (sidebar hover or top right) or right-click, then "Move to Trash".
  - The page is not gone right away. It stays in Trash for 30 days by default and can be restored to its old location.
  - After a further retention period (30 days by default; Enterprise owners can set it) it is permanently deleted.
  - The help page doesn't mention a confirmation dialog for moving to Trash. **Unverified:** an in-app undo notice.
- **Leave a workspace:** Settings, then Workspace, then General, scroll to **"Danger zone"** and select "Leave workspace". Leaving drops the workspace from your switcher, and an admin can re-add you.
- **Delete a workspace:** In the same Danger zone, choose "Delete entire workspace" and **type the workspace name** to confirm.
- **Leave a teamspace:** "•••" on the teamspace in the sidebar, then "Leave teamspace". It is an item-level overflow action.
- Sources: <https://www.notion.com/help/duplicate-delete-and-restore-content>, <https://www.notion.com/help/create-delete-and-switch-workspaces>, <https://www.notion.com/help/delete-a-workspace>, <https://www.notion.com/help/workspace-settings>

### Telegram

- **Delete a chat or messages:** The official FAQ says any message in a one-on-one chat can be deleted for both sides. You select the message, or hold to select chats, tap the delete icon and tick the option to delete for both parties. In groups, users can delete only their own messages for everyone.
- **Leave a group as the owner:** The FAQ advises picking a new owner at the same time. The API documentation adds that the chosen admin becomes owner after 7 days unless the old owner rejoins.
- **Deleting a group for all members:** The UI strings "Delete the group for all members" and "Delete and leave" exist in Telegram's translation files. **Unverified:** their exact flow.
- **Remove a member:** Third-party guides describe swiping left on the member's name on iPhone to reveal "Delete", and an "X" with confirmation on desktop. **Unverified** by official sources.
- Sources: <https://telegram.org/faq>, <https://core.telegram.org/api/channel>, <https://translations.telegram.org/or/weba/groups_and_channels/>, <https://www.technipages.com/telegram-groups> (third party)

### Discord

- **Delete a server:**
  - Only the owner can do it.
  - On desktop: Server Settings, then "Delete Server" at the bottom of the left sidebar.
  - On mobile: Server Settings, scroll down, then "Delete Server".
  - Third-party guides say desktop requires **typing the server name**, plus a two-factor code when it is enabled. One guide says iPhone asks only for a yes/no confirmation.
  - It cannot be undone.
- **Partly verified:** Discord's support article returned HTTP 403 to automated fetches, so the details come from third-party guides and a search-index summary of the official article.
- Sources: <https://support.discord.com/hc/en-us/articles/213595197> (not fetchable), <https://www.techradar.com/how-to/how-to-delete-a-discord-server>, <https://www.digitaltrends.com/computing/how-to-delete-a-discord-server/>

### Zoom

- **Delete an account:** Only on the web portal, under Account Management, then Account Profile, then "Terminate My Account". A confirmation prompt follows; one source says it asks for the password first. A paid subscription must be cancelled before the option appears.
- **Unverified** by Zoom's own support documentation: sourced from Zoom community answers and third-party guides.
- Sources: <https://community.zoom.com/t5/Billing-Account-Management/Zoom-account-delete/td-p/215669>, <https://www.makeuseof.com/delete-zoom-account-permanently/> (third party)

### Microsoft Teams

- **Leave a team:** Open the team name's **More options** menu, choose "Leave the team" and confirm. Only admins can remove people from org-wide teams.
- **Remove a member:** More team options, then Manage team, then the Members tab, then the **X** at the far right of the person's row. **Unverified:** whether a confirmation follows.
- Sources: <https://support.microsoft.com/en-us/office/leave-a-team-in-microsoft-teams-e481005d-3ec6-4694-b300-375472ba4076>, <https://support.microsoft.com/en-us/teams/teams-channels/remove-someone-from-a-team-in-microsoft-teams>

### Banking app pattern (Monzo as one example)

- **Close an account:**
  - The action is reached through Profile, then Settings, then "Close account", at the bottom of the settings, after scrolling down.
  - Prerequisites gate it: cancel subscriptions, bring the balance to zero and close savings pots. The business account flow says the option may be missing until something is completed.
  - A closed account can't be reopened.
  - The help pages don't describe the confirmation screens. **Unverified:** the exact confirmation step.
- Cancelling a scheduled payment was not researched in a citable source. **Unverified.**
- Source: <https://monzo.com/help/account-and-profile/close-account>

## 4. Accessibility notes

- **WCAG 2.2 SC 3.3.4 Error Prevention (Legal, Financial, Data), level AA:**
  - It applies to pages that cause legal commitments or financial transactions, modify or delete user-controllable data, or submit test responses.
  - At least one must hold: the submission is **reversible**, it is **checked** for input errors, or it is **confirmed** after a chance to review.
  - Deletion techniques include G99 (recover deleted information), G168 (request confirmation before proceeding) and G155 (a checkbox in addition to the submit button).
  - It does not require confirming every save or simple edit.
  - Source: <https://www.w3.org/WAI/WCAG22/Understanding/error-prevention-legal-financial-data.html>
- **SC 3.3.6 Error Prevention (All), level AAA,** extends the same rule to all submissions. It was not fetched in this research, so its text is **unverified** here.
- **Focus on open:** The WAI-ARIA APG modal dialog pattern says focus generally goes to the first focusable element. It may be better to set it on "the least destructive action, especially if undoing the action is difficult or impossible". Primer does this by default for danger confirmations. Source: <https://www.w3.org/WAI/ARIA/apg/patterns/dialog-modal/>
- **Focus containment and Escape:**
  - Tab and Shift+Tab cycle inside a modal dialog, and Escape closes it.
  - Microsoft maps Escape and the system back button to the safe button.
  - Sources: APG modal dialog above, and the Microsoft dialog page in section 2.
- **Focus return:**
  - On close, focus returns to the element that opened the dialog.
  - If that element no longer exists, focus goes to another element that keeps a logical workflow. That is the usual case after deletion or removal.
  - Primer specifies the previous equivalent control or a stable parent such as the list heading.
  - Sources: APG modal dialog, Primer Delete pattern, Atlassian modal dialog.
- **Roles:**
  - Use `role="alertdialog"` for an interrupting confirmation.
  - Name it with `aria-labelledby` (the title) and describe it with `aria-describedby` (the consequence text).
  - Source: <https://www.w3.org/WAI/ARIA/apg/patterns/alertdialog/>
- **Don't rely on color (WCAG SC 1.4.1 Use of Color):**
  - GOV.UK, Primer and Atlassian each say the red treatment needs a text label that names the outcome.
  - Primer pairs danger menu items with a trash icon. Atlassian requires accessible labels on warning icons.
- **Announce results:** Primer recommends announcing a completed deletion through a live region instead of a visual success banner. When the deletion fails, keep the dialog open with an error so the person can retry.
- **Static, predictable headings:** The MoJ confirm pattern advises a static heading on the confirm page, because dynamic headings have unpredictable length.

## 5. Patterns by severity

| Severity | Typical treatment | Where it lives | Examples found |
| --- | --- | --- | --- |
| **Reversible** (trash, archive, routine removal of a label) | No confirmation. Act at once, and offer **Undo** in a transient notice and/or a restorable trash or archive. | Inline or in the item's overflow menu. | Notion "Move to Trash" (30-day trash); Gmail delete (30-day Trash; immediate Undo unverified); Primer: reversible deletions such as labels get no dialog, only undo; Apple: no alert for undoable deletes of emails and files; Material snackbar with Undo. |
| **Consequential but recoverable** (remove a member, leave a team, archive a shared object, an action that notifies or affects another person) | **Confirmation dialog**: a question title naming the action and object; a body stating the consequence and who is affected; a confirm button naming the outcome; Cancel present and focused for danger actions; Escape cancels. | Item overflow menu (Slack, Classroom, Teams), or the object's settings. | Google Classroom remove student and archive class; Slack remove from channel ("Remove") and archive channel ("Archive channel"); Microsoft Teams leave team; Primer: single-resource deletions get a simple dialog. |
| **Irreversible data loss or wide impact** (delete a class, workspace, organization, account, or permanently delete a channel) | **A stronger, explicit second step**: type the object's name (GitHub, Notion, Discord desktop), tick an acknowledgement checkbox (Slack, GitHub repository), re-enter the password (Slack workspace, Slack deactivation, Zoom per one source), or a separate confirmation page with the red warning button only on the final step (GOV.UK, MoJ). Often gated behind an earlier reversible state. | A **"Danger zone"** at the bottom of the settings page (GitHub, Notion, Slack workspace, Discord), or a separate page or flow. | GitHub delete repository (two acknowledgements plus the typed name) and delete organization (typed name); Notion delete workspace (typed name); Slack delete channel (checkbox) and delete workspace (checkbox plus password); Google Classroom delete class (archive first; primary teacher only); Monzo close account (prerequisites; can't reopen). |

Recurring rules across all three tiers:

- Name the outcome on the button.
- State whether the action can be undone.
- Never put the dangerous control beside the everyday ones.
- Keep the red treatment for the final destructive control, with a text label.

## 6. Sources

All checked 2026-11-26.

Design systems and standards:

- Nielsen Norman Group, *Confirmation Dialogs Can Prevent User Errors — If Not Overused*: <https://www.nngroup.com/articles/confirmation-dialog/>
- Nielsen Norman Group, *Dangerous UX: Consequential Options Close to Benign Options*: <https://www.nngroup.com/articles/proximity-consequential-options/>
- Nielsen Norman Group, *Preventing User Errors: Avoiding Conscious Mistakes*: <https://www.nngroup.com/articles/user-mistakes/>
- Apple HIG, Alerts: <https://developer.apple.com/design/human-interface-guidelines/alerts> (read via <https://developer.apple.com/tutorials/data/design/human-interface-guidelines/alerts.json>)
- Apple HIG, Action sheets: <https://developer.apple.com/design/human-interface-guidelines/action-sheets> (read via <https://developer.apple.com/tutorials/data/design/human-interface-guidelines/action-sheets.json>)
- GOV.UK Design System, Button (warning button): <https://design-system.service.gov.uk/components/button/>
- Ministry of Justice Design System, Confirm an action: <https://design-patterns.service.justice.gov.uk/patterns/confirm-an-action/>
- Primer, Delete scenario pattern: <https://primer.style/product/scenario-patterns/delete/>
- Primer, ConfirmationDialog guidelines: <https://primer.style/product/components/confirmation-dialog/guidelines/>
- Primer, ConfirmationDialog accessibility: <https://primer.style/product/components/confirmation-dialog/accessibility/>
- Atlassian Design System, Modal dialog usage: <https://atlassian.design/components/modal-dialog/usage>
- Microsoft Learn, Dialog controls (Windows apps): <https://learn.microsoft.com/en-us/windows/apps/design/controls/dialogs-and-flyouts/dialogs>
- Material Design 3, Dialogs guidelines (partly verified, via search index): <https://m3.material.io/components/dialogs/guidelines>
- Material Design 1, Dialogs: <https://m1.material.io/components/dialogs.html>
- Material, Snackbars: <https://material.io/components/snackbars>
- Android Developers, Add an action to a message (snackbar): <https://developer.android.com/training/snackbar/action.html>
- W3C, Understanding SC 3.3.4 (WCAG 2.2): <https://www.w3.org/WAI/WCAG22/Understanding/error-prevention-legal-financial-data.html>
- WAI-ARIA APG, Dialog (modal) pattern: <https://www.w3.org/WAI/ARIA/apg/patterns/dialog-modal/>
- WAI-ARIA APG, Alert dialog pattern: <https://www.w3.org/WAI/ARIA/apg/patterns/alertdialog/>

Products:

- GitHub Docs, Deleting a repository: <https://docs.github.com/en/repositories/creating-and-managing-repositories/deleting-a-repository>
- GitHub Docs, Deleting an organization account: <https://docs.github.com/en/organizations/managing-organization-settings/deleting-an-organization-account>
- Google Classroom Help, Remove a student or co-teacher (computer): <https://support.google.com/edu/classroom/answer/6069576?hl=en&co=GENIE.Platform%3DDesktop>
- Google Classroom Help, Remove a student or co-teacher (Android): <https://support.google.com/edu/classroom/answer/6069576?hl=en&co=GENIE.Platform%3DAndroid>
- Google Classroom Help, Archive or delete a class: <https://support.google.com/edu/classroom/answer/6149813?hl=en&co=GENIE.Platform%3DDesktop>
- Google Classroom Help, Unenroll from a class: <https://support.google.com/edu/classroom/answer/6069981?hl=en&co=GENIE.Platform%3DDesktop>
- Gmail Help, Delete or recover deleted messages: <https://support.google.com/mail/answer/7401?hl=en&co=GENIE.Platform%3DDesktop>
- Slack Help, Remove someone from a channel: <https://slack.com/help/articles/201898668-Remove-someone-from-a-channel>
- Slack Help, Archive or delete a channel: <https://slack.com/help/articles/213185307-Archive-or-delete-a-channel>
- Slack Help, Delete a workspace: <https://slack.com/help/articles/204067366-Delete-a-workspace>
- Slack Help, Deactivate your account: <https://slack.com/help/articles/203953146>
- Notion Help, Delete & restore content: <https://www.notion.com/help/duplicate-delete-and-restore-content>
- Notion Help, Create, join & leave workspaces: <https://www.notion.com/help/create-delete-and-switch-workspaces>
- Notion Help, Delete a workspace: <https://www.notion.com/help/delete-a-workspace>
- Notion Help, Workspace settings: <https://www.notion.com/help/workspace-settings>
- Telegram FAQ: <https://telegram.org/faq>
- Telegram API, Channels and groups: <https://core.telegram.org/api/channel>
- Telegram translations, Groups and channels strings: <https://translations.telegram.org/or/weba/groups_and_channels/>
- Technipages, Telegram groups (third party): <https://www.technipages.com/telegram-groups>
- Discord Support, Deleting a server (HTTP 403 to automated fetch): <https://support.discord.com/hc/en-us/articles/213595197>
- TechRadar, How to delete a Discord server (third party): <https://www.techradar.com/how-to/how-to-delete-a-discord-server>
- Digital Trends, How to delete a Discord server (third party): <https://www.digitaltrends.com/computing/how-to-delete-a-discord-server/>
- Zoom Community, Zoom account delete: <https://community.zoom.com/t5/Billing-Account-Management/Zoom-account-delete/td-p/215669>
- MakeUseOf, Delete Zoom account permanently (third party): <https://www.makeuseof.com/delete-zoom-account-permanently/>
- Microsoft Support, Leave a team in Microsoft Teams: <https://support.microsoft.com/en-us/office/leave-a-team-in-microsoft-teams-e481005d-3ec6-4694-b300-375472ba4076>
- Microsoft Support, Remove someone from a team: <https://support.microsoft.com/en-us/teams/teams-channels/remove-someone-from-a-team-in-microsoft-teams>
- Monzo Help, Closing your account: <https://monzo.com/help/account-and-profile/close-account>
