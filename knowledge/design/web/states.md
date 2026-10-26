# States, colours and words

One vocabulary for every status in OVC. Each state has exactly one colour token from [tokens](tokens.md#colours) and one word per language, used identically in rows, panels, badges, emails and notifications; the lesson table also fixes icons, and the component library maps every other state to one icon. Status is never shown by colour alone: the word is always present. A "badge" in these documents is the state word in its colour, optionally after a small dot; a "count badge" is a number in `text-muted` beside a sidebar item; neither is a filled pill ([tokens](tokens.md#principles)). Phrases avoid interpolating names into positions that need a grammatical case in Russian.

Recording states apply once recordings land [after the MVP](../../../docs/roadmap.md#after-the-mvp) (owner's decision of 2026-10-22).

## Lessons

| State | When | Token | Icon | English | Русский |
|---|---|---|---|---|---|
| `scheduled` | before the tutor joins | `info` | calendar | Scheduled | Запланирован |
| `scheduled`, join window open | 10 minutes before start | `accent-text` | door | Starting soon | Скоро начало |
| `live` | the tutor has joined | `live` | pulsing dot | Live now | Идёт сейчас |
| `ended`, took place | after it ran | `success` | check | Ended | Завершён |
| `ended`, did not take place | nobody joined | `text-muted` | dash | Did not take place | Не состоялся |
| `cancelled` | cancelled | `danger` | cross | Cancelled | Отменён |

While the window is open and the lesson is not yet live, students see "The room opens when your tutor joins" / "Комната откроется, когда подключится преподаватель"; the tutor's name appears separately in the lesson header.

## Homework (student view)

| Submission state | Token | English | Русский |
|---|---|---|---|
| `not_started` | `info` | To do | Нужно сделать |
| `not_started` or `draft`, deadline within 24 hours | `warning` | Due soon | Скоро срок |
| `not_started` or `draft`, deadline passed | `danger` | Overdue | Срок прошёл |
| `draft` | `info` | In progress | В работе |
| `submitted` | `info` | Submitted | Сдано |
| `submitted`, late | `warning` | Submitted late | Сдано с опозданием |
| `returned` | `warning` | Returned for revision | На доработке |
| `checked` | `success` | Checked | Проверено |
| `closed` | `text-muted` | Closed | Закрыто |

## Homework (tutor view)

| State | Token | English | Русский |
|---|---|---|---|
| `submitted` | `accent-text` | Needs checking | Нужно проверить |
| `checked` without manual questions | `success` | Checked automatically | Проверено автоматически |
| `returned` | `warning` | Returned | Возвращено |
| `checked` | `success` | Checked | Проверено |
| `not_started`, `draft` | `text-muted` | Not submitted | Не сдано |
| late flag | `warning` | Late | С опозданием |
| submission with a review draft | `info` | Review in progress | Проверка начата |
| `closed` | `text-muted` | Closed | Закрыто |

## Relationships, requests and groups

| State | Token | English | Русский |
|---|---|---|---|
| relationship `invited` | `info` | Invitation sent | Приглашение отправлено |
| relationship `provisional` | `warning` | Email not confirmed | Email не подтверждён |
| relationship `reclaim_pending` | `info` | Waiting for the student | Ждём ответа ученика |
| relationship `active` | — (no badge) | — | — |
| relationship `ended` | `text-muted` | Ended | Занятия завершены |
| join request `pending` | `accent-text` | Wants to join | Хочет заниматься |
| invitation expired | `warning` | Invitation expired | Приглашение истекло |
| invitation revoked after five wrong emails | `danger` | Link revoked after repeated wrong emails | Ссылка отозвана после нескольких попыток с другим email |
| sent request `pending` (student view) | `info` | Request sent | Запрос отправлен |
| sent request `not_accepted` (student view) | `text-muted` | Not accepted | Не принят |
| sent request `expired` (student view) | `text-muted` | Expired | Истёк |
| group archived | `text-muted` | Archived | В архиве |

## Recordings

| State | Token | English | Русский |
|---|---|---|---|
| `processing`, `copying` | `info` | Processing… | Обрабатывается… |
| `available` | `success` | Recording · until {date} | Запись · до {date} |
| `failed` | `danger` | Recording failed | Запись не сохранилась |
| `held_quota` | `warning` | Storage full · Retry | Хранилище заполнено · Повторить |
| `failed`, quota | `danger` | Storage full | Хранилище заполнено |
| `expired`, `deleted` | `text-muted` | Recording removed | Запись удалена |

## Messages and presence

| State | Presentation | English | Русский |
|---|---|---|---|
| online | `success` dot on the avatar plus "online" in the header | online | в сети |
| typing | animated dots plus text | {name} is typing… | {name} печатает… |
| edited | caption after the time | edited | изменено |
| deleted | muted italic placeholder | Message deleted | Сообщение удалено |
| read-only channel | banner above the composer, composer disabled | This chat is read-only | Чат доступен только для чтения |
| unread | count badge beside Messages, and a bold channel name in the channel list | — | — |

## Notifications

Slice 4. The texts in this section are a proposal to confirm when F2 (notifications) and F3 (settings and unsubscribe) build the screens; the rule that a row shows its label (a name) and its text as separate elements is the contract, so no name stands in a position that needs a grammatical case in Russian; labels are user-written and shown as text only.

| Event | Recipient | English | Русский |
|---|---|---|---|
| `invitation.accepted` | tutor | Invitation accepted | Приглашение принято |
| `join_request.created` | tutor | New request to study with you | Новый запрос на занятия |
| `join_request.accepted` | student | Your request was accepted | Ваш запрос принят |
| `join_request.declined` | student | Your request was not accepted | Ваш запрос не принят |
| `relationship.ended` | tutor or student | Working together has ended | Совместные занятия завершены |

| Element | English | Русский |
|---|---|---|
| Panel and page heading | Notifications | Уведомления |
| Count above 99 | 99+ | 99+ |
| Mark all as read | Mark all as read | Отметить все как прочитанные |
| Link from the panel | Show all notifications | Все уведомления |
| Empty | No notifications yet | Уведомлений пока нет |
| Polite announcement | New notification | Новое уведомление |
| Accessible name of the bell above 99 | Notifications, more than 99 unread | Уведомления, больше 99 непрочитанных |
| Unread row, for screen readers | Unread | Не прочитано |
| Result of Mark all as read | All notifications are marked as read. | Все уведомления отмечены как прочитанные. |
| Label of a deleted requester | Deleted account | Удалённый аккаунт |

**Email groups,** in Settings → Notifications and on the unsubscribe page:

| Group | English | Русский |
|---|---|---|
| `lesson_reminders` | Lesson reminders | Напоминания об уроках |
| `lesson_changes` | Lesson changes | Изменения в расписании |
| `homework` | New assignments | Новые задания |
| `results` | Checked work | Проверенные работы |
| `submissions` | Submitted work | Сданные работы |
| `message_digest` | Unread messages | Непрочитанные сообщения |

**Unsubscribe page and sign-out messages:**

| Element | English | Русский |
|---|---|---|
| Group line | Emails: {group} | Письма: {group} |
| Confirm button | Turn off these emails | Отключить эти письма |
| Result | These emails are turned off | Эти письма отключены |
| Link to settings | All email settings | Все настройки писем |
| Invalid link | This link is no longer valid | Ссылка больше не действует |
| Signed out after an unfixable `4401` (`sessionEnded`) | You were signed out on this device | Вы вышли из аккаунта на этом устройстве |
| A student card that a live refetch found gone | This student is no longer on your list. | Этого ученика больше нет в вашем списке. |

`{group}` is the group's name from the table above, which stands after a colon and needs no case.

## Tutor labels

In every student view, items carry their tutor's label: an 8 px dot in the relationship colour plus the tutor's name (or initials in narrow layouts). The filter "All tutors / {name}" sits at the top of Home, Schedule and Homework.

## Time and scores

Strings with fixed keys, so tests assert by key and both languages stay in step. Score strings appear only in tutor views.

| Key | English | Русский |
|---|---|---|
| `homework.auto_points` | {count, plural, one {# automatic point} other {# automatic points}} | {count, plural, one {# балл за автопроверку} few {# балла за автопроверку} many {# баллов за автопроверку} other {# балла за автопроверку}} |
| `homework.score` | {earned} of {max} ({percent}) | {earned} из {max} ({percent}) |
| `homework.not_scored` | Not scored | Без оценки |
| `time.other_zone` | {time} {city} time ({local} your time) | {time} по времени {city} ({local} по вашему) |

The bundled timezone table gives each city in the form the string needs: "Moscow" in English and the genitive "Москвы" in Russian.

## Feedback messages

- **Toasts:**
  - confirm completed actions in one short sentence ("Lesson cancelled" / "Урок отменён");
  - errors persist until dismissed and offer a retry when the action is retryable.
- **Inline validation** appears below the field after the first blur or submit, in `danger`, with an icon.
- **Destructive actions** (ending a relationship from either side, removing a provisional student, cancelling a series, deleting a recording, deleting the account) need a confirmation dialog that names the consequence. Deleting the account additionally asks for the password.

## Localisation

- **Libraries:** frontend strings live in i18next resource files per language (`ru`, `en`) with ICU message format, including plurals. Russian uses one / few / many / other: "1 урок, 2 урока, 5 уроков".
- **API error codes** map to messages in the resource files; an unknown code shows a generic message with the request identifier.
- **Formats:**
  - dates and times use `Intl.DateTimeFormat` in the user's language and timezone;
  - Russian uses 24-hour time and day-month order ("12 нояб., 18:30"); English uses the `en-GB` format ("12 Nov, 18:30") so dates read the same for every English user and in tests;
  - relative times use `Intl.RelativeTimeFormat`;
  - numbers and percentages use `Intl.NumberFormat`, including the `{percent}` in tutor score strings.
- **Emails** use server-side templates with the same keys and plural rules.
- **User content is never translated:** names, titles, messages, assignment text.
- **Text expansion:** layouts tolerate 30 % longer strings; buttons do not truncate labels.
