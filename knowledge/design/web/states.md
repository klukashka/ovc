# States, colours and words

One vocabulary for every status in OVC. Each state has exactly one colour token from [tokens](tokens.md#colours) and one word per language, used identically in lists, cards, badges, emails and notifications; the lesson table also fixes icons, and the component library maps every other state to one icon. Status is never shown by colour alone: the word is always present. Phrases avoid interpolating names into positions that need a grammatical case in Russian.

## Lessons

| State | When | Token | Icon | English | Русский |
|---|---|---|---|---|---|
| `scheduled` | before the tutor joins | `info` | calendar | Scheduled | Запланирован |
| `scheduled`, join window open | 10 minutes before start | `accent` | door | Starting soon | Скоро начало |
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
| `checked` | `success` | Checked · {score} | Проверено · {score} |
| `checked`, maximum 0 | `success` | Checked · not scored | Проверено · без оценки |
| `closed` | `text-muted` | Closed | Закрыто |

## Homework (tutor view)

| State | Token | English | Русский |
|---|---|---|---|
| `submitted` | `accent` | Needs checking | Нужно проверить |
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
| relationship `active` | — (no badge) | — | — |
| relationship `ended` | `text-muted` | Ended | Занятия завершены |
| join request `pending` | `accent` | Wants to join | Хочет заниматься |
| invitation expired | `warning` | Invitation expired | Приглашение истекло |
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
| unread | `accent` count badge | — | — |

## Tutor labels

In every student view, items carry their tutor's label: a 4 px stripe or dot in the relationship colour plus the tutor's name (or initials in narrow layouts). The filter "All tutors / {name}" sits at the top of Home, Schedule and Homework.

## Time and scores

Strings with fixed keys, so tests assert by key and both languages stay in step.

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
- **Destructive actions** (ending a relationship, cancelling a series, deleting a recording, deleting the account) need a confirmation dialog that names the consequence. Deleting the account additionally asks for the password.

## Localisation

- **Libraries:** frontend strings live in i18next resource files per language (`ru`, `en`) with ICU message format, including plurals. Russian uses one / few / many / other: "1 урок, 2 урока, 5 уроков".
- **API error codes** map to messages in the resource files; an unknown code shows a generic message with the request identifier.
- **Formats:**
  - dates and times use `Intl.DateTimeFormat` in the user's language and timezone;
  - Russian uses 24-hour time and day-month order ("12 нояб., 18:30"); English uses the `en-GB` format ("12 Nov, 18:30") so dates read the same for every English user and in tests;
  - relative times use `Intl.RelativeTimeFormat`;
  - numbers and percentages use `Intl.NumberFormat`; `{score}` above is a percentage formatted this way.
- **Emails** use server-side templates with the same keys and plural rules.
- **User content is never translated:** names, titles, messages, assignment text.
- **Text expansion:** layouts tolerate 30 % longer strings; buttons do not truncate labels.
