# Design slice known limitations

Status: agent record, started 2026-11-08. Minor review findings of design slice changes that were accepted rather than fixed, in the format of the numbered slices' files. Each entry is re-verified by the slice review, which resolves it as fixed, accepted by the owner or carried into a named later slice.

| Finding | Severity | Change | Why it was accepted |
|---|---|---|---|
| On the notifications page, a row's link is named only by the event's one-line text, so a page can hold up to 30 links with the same name, such as "New join request"; the label sits in the next cell. | Minor | Step L1, the desktop layout standard | Inside a table each link is read with its row, and the event text never interpolates a name by design ([states](../design/web/states.md#notifications)); adding the label to the link name with `aria-describedby` is a cheap later improvement. |
| A tutor's bio is clamped to two lines in My tutors, and its full text is offered visually only through the `title` tooltip, which keyboard and touch users cannot open; screen readers read the whole bio. | Minor | Step L1, the desktop layout standard | `pages.md` specifies the clamp with the tooltip; a "Show more" for the bio is left to the phone layouts of slice 10 or a later review. |
| Names that are the row's link in the Students and Groups tables are cut with an ellipsis on one line and have no tooltip, as they were before the tables. | Minor | Step L1, the desktop layout standard | Unchanged behaviour of these lists; the full name is on the card or group page the link opens. |
