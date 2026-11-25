# States, colours and words

One vocabulary for every status in OVC. Each state has exactly one colour token from [tokens](tokens.md#colours) and one word per language, used identically in rows, panels, badges, emails and notifications; the lesson table also fixes icons, and the component library maps every other state to one icon. Status is never shown by colour alone: the word is always present, with one exception: in calendar blocks and lesson lists an ordinary `scheduled` lesson shows no status word and no status icon, because it is the default state, and carries no status colour either (design slice step D4, owner's decision of 2026-11-09, pending owner review); the lesson page still shows "Scheduled". A "badge" in these documents is the state word in its colour, optionally after a small dot; a "count badge" is a number in `text-muted` beside a sidebar item; neither is a filled pill ([tokens](tokens.md#principles)). Phrases avoid interpolating names into positions that need a grammatical case in Russian.

Recording states apply once recordings land [after the MVP](../../../docs/roadmap.md#after-the-mvp) (owner's decision of 2026-10-22).

## Lessons

| State | When | Token | Icon | English | Русский |
|---|---|---|---|---|---|
| `scheduled` | before the tutor joins; on the lesson page only (design slice step D4) | `info` | calendar | Scheduled | Запланирован |
| `scheduled`, join window open | 10 minutes before start | `accent-text` | door | Starting soon | Скоро начало |
| `live` | the tutor has joined | `live` | pulsing dot | Live now | Идёт сейчас |
| `ended`, took place | after it ran | `success` | check | Ended | Завершён |
| `ended`, did not take place | nobody joined | `text-muted` | dash | Did not take place | Не состоялся |
| `cancelled` | cancelled | `danger` | cross | Cancelled | Отменён |

While the window is open and the lesson is not yet live, students see "The room opens when your tutor joins" / "Комната откроется, когда подключится преподаватель"; the tutor's name appears separately in the lesson header. In slice 5, before joining exists, a `scheduled` lesson shows "Starting soon" inside the window and becomes "Did not take place" 30 minutes after its end ([schedule](../../specs/schedule.md#lesson-status-and-join-window)).

**Schedule and lesson texts** (slice 5; a proposal to confirm when the schedule and lesson pages are built; the labels are fixed texts that stand in for names, so they never carry a card name to students):

| Key or element | English | Русский |
|---|---|---|
| `lesson.individual` (`label_kind` `individual`), a student's label of a one-to-one lesson | Individual lesson | Индивидуальный урок |
| `lesson.group` (`label_kind` `group`), a former member's label of a group lesson and of its `group.archived` row | Group lesson | Групповой урок |
| `lesson.removed_student` (`label_kind` `removed_student`), the tutor's label of a card discarded by a removal, a dropped link or a tutor deletion, and of any other card whose name was erased other than by a student's purge | Removed student | Удалённый ученик |
| `lesson.former_student` (`label_kind` `former_student`), the tutor's label, in lessons, chats, issues and notification rows, of a card whose name a student's purge erased ([ADR-0024](../../../docs/decisions/0024-legal-documents-and-student-data.md), change L0 of the [interface rework plan](../../plans/ui-rework.md), pending owner review) | Former student | Бывший ученик |
| `lesson.deleted_account` (`label_kind` `deleted_account`), shown beside a member's card name when the member's account is deleted | Deleted account | Удалённый аккаунт |
| Before the join window | Room opens at {time} | Комната откроется в {time} |
| Default title: alone for a one-to-one lesson, followed by the group's name for a group lesson, never by a card name | Lesson | Урок |
| Hint below the title | Students see the title | Ученики видят название |
| Overlap warning action | Schedule anyway | Всё равно запланировать |
| Overlapping lessons left out of the list | {count, plural, one {and # more lesson} other {and # more lessons}} | {count, plural, one {и ещё # урок} few {и ещё # урока} many {и ещё # уроков} other {и ещё # урока}} |
| Scope choice | This lesson / This and following lessons | Этот урок / Этот и следующие уроки |
| Weekday change | End this series and create a new one | Завершить серию и создать новую |
| Before the overlapping lessons (proposal of 2026-10-28) | This lesson overlaps your other lessons: | Этот урок пересекается с другими вашими уроками: |
| Refusal beyond the per-tutor caps, `too_many` at `body` (proposal of 2026-10-28) | You have reached the limit of 100 open series or 3,000 future lessons. | Достигнут предел: 100 действующих серий или 3 000 будущих уроков. |
| The field code `too_many` elsewhere (proposal of 2026-10-28) | Limit reached. | Достигнут предел. |
| A period of the agenda without lessons (proposal of 2026-10-28) | No lessons in this period | В этом периоде нет уроков |
| A calendar response with `truncated: true` (proposal of 2026-10-28) | Only the first 1,000 lessons of this period are shown. Open a week to see the rest. | Показаны только первые 1 000 уроков этого периода. Откройте неделю, чтобы увидеть остальные. |
| Video area of the lesson page (slice 7, pending owner review) | Video room | Видеокомната |
| Tutor's and student's join (slice 7, pending owner review) | Join lesson / Join / Rejoin | Подключиться к уроку / Подключиться / Подключиться снова |
| While joining (slice 7, pending owner review) | Joining… | Подключение… |
| `provider_unavailable` on a join, a restart or a link the page cannot use (slice 7, pending owner review) | The video service is unavailable, try again | Видеосвязь недоступна, попробуйте ещё раз |
| `not_found` on a join (slice 7, pending owner review) | You can't join this lesson | Вы не можете подключиться к этому уроку |
| Blocked pop-up (slice 7, pending owner review) | Your browser blocked the new tab. Allow pop-ups for this site and try again. | Браузер заблокировал новую вкладку. Разрешите всплывающие окна для этого сайта и попробуйте ещё раз. |
| In room, new tab (slice 7, pending owner review) | The lesson is open in another tab / Open again | Урок открыт в другой вкладке / Открыть снова |
| In room, embedded (slice 7, pending owner review) | Open in new tab / Having trouble? Open in a new tab / Focus mode / Exit focus | Открыть в новой вкладке / Возникли проблемы? Откройте в новой вкладке / Режим фокуса / Выйти из режима фокуса |
| End lesson confirmation (slice 7, pending owner review) | End lesson; End the lesson; The room closes for everyone and the lesson ends. It cannot be opened again. | Завершить урок; Завершить урок; Комната закроется для всех, и урок завершится. Открыть его снова будет нельзя. |
| After ending (slice 7, pending owner review) | Lesson ended | Урок завершён |
| Restart confirmation (slice 7, pending owner review) | Restart the room; The room closes for everyone and a new one opens. Everyone must rejoin, and earlier links stop working. | Перезапустить комнату; Комната закроется для всех, и откроется новая. Всем нужно будет подключиться снова, а прежние ссылки перестанут работать. |
| Polite announcement after creating | Lesson scheduled | Урок запланирован |
| Polite announcement after editing | Lesson changed | Урок изменён |
| Changed-meanwhile notice | This lesson changed in the meantime. Its current state is shown below. | Этот урок тем временем изменился. Ниже показано его текущее состояние. |
| Cancelled, with its date | Cancelled {date} | Отменён {date} |
| Archive confirmation | {count, plural, one {# future lesson will be cancelled.} other {# future lessons will be cancelled.}} Unarchiving the group restores neither lessons nor series. | {count, plural, one {# будущий урок будет отменён.} few {# будущих урока будут отменены.} many {# будущих уроков будут отменены.} other {# будущего урока будут отменены.}} Возврат группы из архива не восстановит ни уроки, ни серии. |
| Back link on the lesson page (proposal of 2026-10-28) | Back to the schedule | К расписанию |
| Side column of the lesson page and its terms (proposal of 2026-10-28) | About the lesson: Date and time, Format, Student, Group, Members, Tutor, Series | Об уроке: Дата и время, Формат, Ученик, Группа, Участники, Репетитор, Серия |
| Lesson formats (proposal of 2026-10-28) | Individual / Group | Индивидуальный / Групповой |
| Series rule with a count (proposal of 2026-10-28; `{weekdays}` is the list of weekday names) | Every week: {weekdays} at {time}, {count, plural, one {# lesson in all} other {# lessons in all}} | Каждую неделю: {weekdays} в {time}, {count, plural, one {всего # урок} few {всего # урока} many {всего # уроков} other {всего # урока}} |
| Series rule with an end date, or with neither (proposal of 2026-10-28) | Every week: {weekdays} at {time}, until {date} / Every week: {weekdays} at {time} | Каждую неделю: {weekdays} в {time}, до {date} / Каждую неделю: {weekdays} в {time} |
| Series rule of a series whose timezone is not the viewer's (proposal of 2026-10-28) | {rule} ({city} time) / {rule} ({offset}) | {rule} (по времени {city}) / {rule} (по {offset}) |
| The tutor's lesson actions (proposal of 2026-10-28) | Edit lesson / Cancel lesson | Изменить урок / Отменить урок |
| Edit dialog: heading, scope legend and save (proposal of 2026-10-28) | Edit lesson / Lessons to change / Save | Изменение урока / Какие уроки изменить / Сохранить |
| Edit dialog hint when the lesson's tutor-side timezone is not the viewer's (proposal of 2026-10-28) | Date and time are in {city} time. / Date and time are in {offset}. | Дата и время указаны по времени {city}. / Дата и время указаны по {offset}. |
| Cancel dialog: heading and scope legend (proposal of 2026-10-28) | Cancel the lesson / Lessons to cancel | Отмена урока / Какие уроки отменить |
| Cancel dialog consequence for this lesson (proposal of 2026-10-28) | The lesson is cancelled for everyone and stays in the history as cancelled. | Урок будет отменён для всех и останется в истории как отменённый. |
| Cancel dialog consequence for this and following lessons (proposal of 2026-10-28) | This lesson and every following lesson of the series are cancelled, and the series ends. | Этот и все следующие уроки серии будут отменены, а серия завершится. |
| Cancel dialog confirmation, one lesson or this and following (proposal of 2026-10-28) | Cancel lesson / Cancel lessons | Отменить урок / Отменить уроки |
| Notice after a cancellation, which takes focus because the cancelled lesson's actions are gone (proposal of 2026-10-28) | Lesson cancelled | Урок отменён |
| A Students or group row's next lesson, and a row without one (proposal of 2026-10-28) | Next lesson / No lesson scheduled | Следующий урок / Урок не запланирован |
| The next lessons of the student card and the group page, and their empty state (proposal of 2026-10-28) | Next lessons / No lessons scheduled. | Ближайшие уроки / Уроков не запланировано. |
| The student card's sections and its empty Lessons tab (proposal of 2026-10-28) | Card sections: Overview, Lessons / No lessons yet. | Разделы карточки: Обзор, Уроки / Уроков пока нет. |
| The card's and the group's actions (proposal of 2026-10-28) | Schedule a lesson / Open the schedule | Запланировать урок / Открыть расписание |
| The New lesson page's preselected card or group when the loaded choices do not list it (proposal of 2026-10-28) | Chosen | Выбрано |
| Archive confirmation heading (proposal of 2026-10-28) | Archive the group? | Отправить группу в архив? |

The API sends the fixed labels above as codes: `label` is `null` and `label_kind` names the fixed label, and the interface renders its key in the viewer's language, as it renders "Deleted account" for `label_deleted` on notification rows ([labels per role](../../specs/schedule.md#labels-per-role)). Lesson titles are shown as text in their own element isolated with `dir="auto"` ([titles and descriptions](../../specs/schedule.md#titles-and-descriptions)).

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
| issue `withdrawn` (from slice 9) | `text-muted` | Withdrawn | Отозвано |

The late flag of a submitted, returned or checked submission shows "Late" beside its state word, as in the tutor view; a submission flagged as issued before a reclaim shows, in both roles' views, the fixed cue below its title or name, in `text-muted`: "Issued before the account was reclaimed" / "Выдано до восстановления аккаунта", never with the earlier holder's name or other account data (from slice 9).

**Answer visibility policies** (from slice 9), shown on the Issuing page (the Issue dialog until the interface rework stage) and on the issue page:

| Policy | English | Русский |
|---|---|---|
| `never` | Never | Никогда |
| `after_submission` | After submission | После сдачи |
| `after_check` | After checking | После проверки |

## Homework texts

From slice 9. Each table names the text section it belongs to: the global language chunk holds only the first table, and the others load lazily with their pages through feature text sections ([localisation](#localisation)). Titles, comments, feedback and file names are user content and never translated; no text interpolates a name into a position that needs a grammatical case in Russian. The texts the pages share with their own sections (the cue, the state words, the refetch notice) are copied into each section that shows them.

**Global language chunk** (the tutor's tab names and the unconfirmed-tutor prompt moved to the `homework` section, and the student's tab names to the `assignment` section, which every page showing them already loads, to keep the first load within `pnpm check:bundle`; agent changes pending owner review):

| Element | English | Русский |
|---|---|---|
| Tutor's count, accessible name | Homework, {count} to check / Homework, more than 99 to check | Задания, ждут проверки: {count} / Задания, ждут проверки: больше 99 |
| Student's count, accessible name | Homework, {count} to do / Homework, more than 99 to do | Задания, нужно сделать: {count} / Задания, нужно сделать: больше 99 |

**`homework` section** (the tutor's Issuing page, issued list, issue page, review queue and review screen, and the homework parts of the student card, the group page and the lesson page), which also holds the score keys `homework.auto_points`, `homework.score`, `homework.not_scored` and `homework.points` of [time and scores](#time-and-scores):

| Element | English | Русский |
|---|---|---|
| Tutor's Homework tabs | Library / Issued / Needs checking | Библиотека / Выданные / На проверке |
| Unconfirmed tutor on the Issued and Needs checking tabs | Confirm your email to issue and check assignments. | Подтвердите адрес почты, чтобы выдавать и проверять задания. |
| Action that opens the Issuing page, and the page's heading | Issue assignment | Выдать задание |
| Dialog fields | Assignment / Student or group / Linked lesson / No linked lesson / Set a deadline / Deadline / Answer visibility | Задание / Ученик или группа / Связанный урок / Без урока / Указать срок / Срок / Когда ученики видят ответы |
| Note under "After submission" | Students see the correct answers as soon as they submit, so work cannot be returned for revision. | Ученики увидят правильные ответы сразу после сдачи, поэтому вернуть работу на доработку будет нельзя. |
| Incomplete template in the picker | Only a ready assignment can be issued. | Выдать можно только готовое задание. |
| Hint under "After checking" for a template with no manual question (from slice 10, pending owner review) | This assignment has no questions you check by hand, so students see the answers right after submitting. | В этом задании нет вопросов, которые вы проверяете вручную, поэтому ученики увидят ответы сразу после отправки. |
| Summary | {count, plural, one {# student will receive this} other {# students will receive this}} | {count, plural, one {Задание получит # ученик} few {Задание получат # ученика} many {Задание получат # учеников} other {Задание получат # ученика}} |
| Submit, and the result notice | Issue / Assignment issued. / Open the issued assignment | Выдать / Задание выдано. / Открыть выданное задание |
| `not_editable` on issuing | This assignment is no longer ready to issue. Open it in the builder to fix it. / Open in the builder | Задание больше не готово к выдаче. Откройте его в конструкторе и исправьте. / Открыть в конструкторе |
| `relationship_not_active` on issuing | This student can no longer get assignments. | Этому ученику больше нельзя выдавать задания. |
| `audience_not_active` on issuing | This group is archived or has no members. | Группа в архиве или в ней нет учеников. |
| `not_found` on issuing | The assignment, student, group or lesson is no longer available. | Задание, ученик, группа или урок больше недоступны. |
| Deadline in the past | Choose a time in the future. | Выберите время в будущем. |
| Lesson link limit | A lesson links at most 20 assignments. | К уроку можно привязать не больше 20 заданий. |
| Snapshot budget (`too_many` at `body`) | Your issued assignments have reached their storage limit. | Выданные задания заняли всё отведённое им место. |
| Issued list | Active / Withdrawn / Assignment / Audience / Issued / Deadline / Progress / No deadline | Действующие / Отозванные / Задание / Кому / Выдано / Срок / Ход работы / Без срока |
| Progress | {submitted}/{total} submitted / {count} checked / {count} late / {count} to check | Сдали {submitted} из {total} / Проверено: {count} / С опозданием: {count} / Ждут проверки: {count} |
| Empty lists | Nothing issued yet / Open the library / No withdrawn assignments. / No assignments yet. / No open assignments. / No assignments for this group. / No assignments for this lesson | Выданных заданий пока нет / Открыть библиотеку / Отозванных заданий нет. / Заданий пока нет. / Открытых заданий нет. / У группы нет заданий. / К этому уроку нет заданий |
| Card, group and lesson sections | Homework / Open assignments / All homework / Assignments | Задания / Открытые задания / Все задания / Задания |
| Note in the group's Add students dialog | Students you add now do not get assignments issued earlier. | Ученики, которых вы добавите сейчас, не получат задания, выданные раньше. |
| Issue page | Back to issued work / Assignment actions / About the assignment / Student / State / Points / Contains questions you check by hand / Withdrawn {date} | К выданным заданиям / Действия с заданием / О задании / Ученик / Состояние / Баллы / Есть вопросы, которые вы проверяете сами / Отозвано {date} |
| Issue actions | Change deadline / Change linked lesson / Withdraw | Изменить срок / Изменить связанный урок / Отозвать |
| Issue page parts | Submissions / {count, plural, one {# point} other {# points}} (maximum points) | Работы учеников / {count, plural, one {# балл} few {# балла} many {# баллов} other {# балла}} |
| Issued list labels | Issued assignments (table) / Show (status filter) | Выданные задания / Показать |
| Deadline fields | Date / Time | Дата / Время |
| Template picker on the Issuing page | Library / Folder path / Folders / Assignments / Search the library / Ready / Incomplete / No assignments here / Nothing found / Change | Библиотека / Путь к папке / Папки / Задания / Поиск по библиотеке / Готово / Не готово / Здесь нет заданий / Ничего не найдено / Изменить |
| Dialog buttons | Save / Cancel / Show more | Сохранить / Отмена / Показать ещё |
| Polite announcements | Deadline changed / Linked lesson changed | Срок изменён / Связанный урок изменён |
| Changed-meanwhile notice of an issue | This assignment changed in the meantime. Its current state is shown below. | Это задание тем временем изменилось. Ниже показано его текущее состояние. |
| Withdraw confirmation | Withdraw this assignment? / Students no longer see it, and it leaves their lists. This cannot be undone. / Withdraw | Отозвать задание? / Ученики больше не увидят его, и оно пропадёт из их списков. Отменить это нельзя. / Отозвать |
| After withdrawing | The assignment is withdrawn. | Задание отозвано. |
| `issue_has_submissions` | A student has already submitted this assignment, so it can no longer be withdrawn. | Ученик уже сдал это задание, поэтому отозвать его нельзя. |
| `not_found` on a lesson choice | This lesson is no longer available. | Этот урок больше недоступен. |
| Review queue | Needs checking / Group / All groups / Assignment: {title} / Student: {name} / Clear filter / Submitted / first {date} | На проверке / Группа / Все группы / Задание: {title} / Ученик: {name} / Сбросить фильтр / Сдано / впервые {date} |
| Queue filter refused | This filter is not available. / All work to check | Этот фильтр недоступен. / Все работы на проверку |
| Empty queue | Nothing to check | Проверять нечего |
| Review screen | Back to the queue / Next submission / Your answer / Correct answer / Normalised: {answer} | К очереди / Следующая работа / Ответ ученика / Правильный ответ / После нормализации: {answer} |
| Scoring | Points / {count, plural, one {out of # point} other {out of # points}} / Override / Override points / Reason / Remove override / Comment / Attach corrected file / Remove | Баллы / {count, plural, one {из # балла} few {из # баллов} many {из # баллов} other {из # балла}} / Изменить оценку / Новые баллы / Причина / Отменить изменение / Комментарий / Прикрепить исправленный файл / Удалить |
| Feedback | Overall feedback / Write / Preview / Total | Общий отзыв / Текст / Предпросмотр / Итого |
| Draft status and actions | Saving… / Saved / Not saved / Retry / Save draft | Сохраняется… / Сохранено / Не сохранено / Повторить / Сохранить черновик |
| Publishing | Mark checked / Return for revision | Отметить проверенной / Вернуть на доработку |
| Missing manual points | Enter points for this question / Enter points for every question you check by hand. | Поставьте баллы за этот вопрос / Поставьте баллы за каждый вопрос, который вы проверяете сами. |
| Return without a comment | Write a comment or feedback before returning the work. | Прежде чем вернуть работу, напишите комментарий или отзыв. |
| `return_not_allowed` | This work can no longer be returned for revision. | Эту работу больше нельзя вернуть на доработку. |
| Revision limit (`too_many` at `body`) | This review has reached its limit of revisions. | У этой проверки больше не может быть новых версий. |
| Stale draft (`conflict`) | This changed elsewhere / Reload | Проверку изменили в другом месте / Обновить |
| `not_editable` on a review | This work changed in the meantime. Its current state is shown below. | Эта работа тем временем изменилась. Ниже показано её текущее состояние. |
| After publishing | Checked. The student can see your review. / Returned for revision. | Проверено. Ученик видит вашу проверку. / Возвращено на доработку. |
| Not reviewable | This work has not been submitted yet. / This work is closed. | Работа ещё не сдана. / Работа закрыта. |
| Gone submission or issue | This assignment is no longer available. | Это задание больше недоступно. |
| Review screen details (agent addition pending owner review) | Question {number} / No answer / Chosen / Correct / Incorrect / Remove {name} (accessible name) / Uploading {percent} / Work to check (queue table) | Вопрос {number} / Нет ответа / Выбрано / Верно / Неверно / Удалить {name} / Загрузка {percent} / Работы на проверку |
| Corrected file refusals (agent addition pending owner review) | File is larger than 25 MB / WebP pictures can be at most 8 MB / This file type is not allowed here / Storage is full / Uploads are busy right now. Try again in a minute. / The file could not be uploaded. Try again. / At most 5 files / A corrected file is no longer available. Attach it again. | Файл больше 25 МБ / Изображения WebP могут быть не больше 8 МБ / Файлы этого типа здесь нельзя прикреплять / Хранилище заполнено / Сейчас загрузок слишком много. Попробуйте через минуту. / Не удалось загрузить файл. Попробуйте ещё раз. / Не больше 5 файлов / Исправленный файл больше недоступен. Прикрепите его снова. |

**`assignment` section** (the student's Homework list, the assignment page and its results, and the student's part of the lesson page; it extends the slice 8 student view texts):

| Element | English | Русский |
|---|---|---|
| List | Back to homework / All homework / No deadline / Submitted {date} / Checked {date} | К заданиям / Все задания / Без срока / Сдано {date} / Проверено {date} |
| Student's Homework tabs (moved from the global language chunk; agent change pending owner review) | To do / Submitted / Checked / Closed | Нужно сделать / Сданные / Проверенные / Закрытые |
| List labels (agent addition pending owner review) | Homework lists (tab list) / Assignments (table) / Assignment / Tutor / State / Date / Show more | Списки заданий / Задания / Задание / Репетитор / Состояние / Дата / Показать ещё |
| Empty tabs | Nothing to do / Nothing submitted / Nothing checked yet / Nothing closed | Делать нечего / Сданных работ нет / Проверенных работ пока нет / Закрытых заданий нет |
| Tutor filter refused | This tutor is not available. / All tutors | Этот репетитор недоступен. / Все репетиторы |
| Progress | {answered}/{total} answered | Отвечено: {answered} из {total} |
| Save status | Saving… / Saved / Not saved. Trying again… / Try now | Сохраняется… / Сохранено / Не сохранено. Пробуем ещё раз… / Повторить сейчас |
| Conflict banner | Your answers changed on another device / Load the latest | Ваши ответы изменились на другом устройстве / Загрузить последние |
| Kept unsaved answer | Your unsaved answer: {answer} / Use this answer | Ваш несохранённый ответ: {answer} / Вернуть этот ответ |
| Changed-meanwhile notice | This assignment changed in the meantime. Its current state is shown below. | Это задание тем временем изменилось. Ниже показано его текущее состояние. |
| File answers | Attach files / Take a photo / Remove / Confirm your email to attach files. / from 1024 px (H2, pending owner review): Choose files / {kinds}; you can add {count} more files / You have added the maximum of {count} files | Прикрепить файлы / Сфотографировать / Удалить / Подтвердите адрес почты, чтобы прикреплять файлы. / от 1024 px (H2): Выбрать файлы / {kinds}; можно добавить ещё {count} файлов / Добавлено максимальное число файлов: {count} |
| File refusals | Your tutor's storage is full, so this file cannot be added. / Uploads are busy right now. Try again in a minute. / {count, plural, one {At most # file} other {At most # files}} | Хранилище репетитора заполнено, этот файл добавить нельзя. / Сейчас загрузок слишком много. Попробуйте через минуту. / {count, plural, one {Не больше # файла} few {Не больше # файлов} many {Не больше # файлов} other {Не больше # файла}} |
| File answer details (agent addition pending owner review) | Remove {name} (accessible name) / Uploading {percent} / File is larger than 25 MB / WebP pictures can be at most 8 MB / This file type is not allowed here / The file could not be uploaded. Try again. / This file is no longer available. Attach it again. | Удалить {name} / Загрузка {percent} / Файл больше 25 МБ / Изображения WebP могут быть не больше 8 МБ / Файлы этого типа здесь нельзя прикреплять / Не удалось загрузить файл. Попробуйте ещё раз. / Этот файл больше недоступен. Прикрепите его снова. |
| Submit | Submit / Submit again | Сдать / Сдать снова |
| Submit confirmation | Submit your work? / These questions have no answer: / Question {number} / After you submit, you cannot change your answers unless your tutor returns the work. / Submit / Cancel | Сдать работу? / На эти вопросы нет ответа: / Вопрос {number} / После сдачи изменить ответы можно будет, только если репетитор вернёт работу. / Сдать / Отмена |
| After submitting | Submitted | Сдано |
| Deadline passed | The deadline has passed. You can still submit; the work will be marked late. | Срок прошёл. Сдать работу ещё можно, но она будет отмечена как сданная с опозданием. |
| Revealed answers | Correct answer / Accepted answers: {answers} / Correct value: {value} ± {tolerance} / Correct value: {value} / Explanation | Правильный ответ / Правильные ответы: {answers} / Правильное значение: {value} ± {tolerance} / Правильное значение: {value} / Пояснение |
| Review content | Tutor's comment / Corrected files / Overall feedback / A corrected file came back | Комментарий репетитора / Исправленные файлы / Общий отзыв / Есть исправленный файл |
| Returned work | Your tutor returned this work for revision. | Репетитор вернул работу на доработку. |
| Closed work | This assignment is closed and can no longer be submitted. | Задание закрыто, сдать его больше нельзя. |
| Gone submission | This assignment is no longer available. | Это задание больше недоступно. |

**`homeworkPanels` section** (Today's Needs checking and Upcoming deadlines, Home's To do and Recently checked):

| Element | English | Русский |
|---|---|---|
| Today's panels | Needs checking / Open review queue / Nothing to check / Upcoming deadlines / All issued work / No deadlines in the next 3 days / {submitted}/{total} submitted | На проверке / Открыть очередь проверки / Проверять нечего / Ближайшие сроки / Все выданные задания / В ближайшие 3 дня сроков нет / Сдали {submitted} из {total} |
| Today's row words (agent addition pending owner review) | Late / Review in progress / Issued before the account was reclaimed | С опозданием / Проверка начата / Выдано до восстановления аккаунта |
| Home's panels | To do / Recently checked / Nothing to do / Nothing checked yet / A corrected file came back | Нужно сделать / Недавно проверенные / Делать нечего / Проверенных работ пока нет / Есть исправленный файл |
| Recently checked row date (agent addition pending owner review) | Checked {date} | Проверено {date} |
| Recently checked badge for new feedback, and its accessible name (from slice 10, pending owner review) | New / New feedback | Новое / Новый отзыв |
| Row actions, with the assignment's title as hidden text for screen readers | Start / Continue | Начать / Продолжить |
| Link to the list; `{count}` is the number, or "99+" above 99 | All homework, {count} to do | Все задания, нужно сделать: {count} |

## Home pages and the Students list

From slice 10 (pending owner review). Today's checklist and the Students list load with the global language chunk, as their other texts do, unless `pnpm check:bundle` requires a feature section; the refusal of a dismissal shows its error code's message. F1 placed both texts in the global language chunk (owner's decision of 2026-11-19 on F1's task plan); the badge "New" and its accessible name load with the `homeworkPanels` feature section, as the rest of Recently checked does.

| Element | English | Русский |
|---|---|---|
| Today's checklist action | Dismiss | Скрыть |
| Students list column, and the row's value shown under the name below 768 px | Open assignments / Open assignments: {count} | Открытые задания / Открытые задания: {count} |

## Library and builder

From slice 8 (pending owner review). A template's readiness ([validation](../../specs/homework.md#validation)) is the only library state:

| State | Token | English | Русский |
|---|---|---|---|
| `ready` true | `success` | Ready | Готово |
| `ready` false | `warning` | Incomplete | Не готово |

**Library, builder and lesson material texts** (a proposal to confirm when the pages are built; titles, folder names and file names are user content and never translated):

| Element | English | Русский |
|---|---|---|
| Page heading and tree root | Library | Библиотека |
| Main action, and the title of a new template | New assignment | Новое задание |
| Menu actions | Upload material / New folder | Загрузить материал / Новая папка |
| Row actions | Open / Rename / Move / Duplicate / Delete | Открыть / Переименовать / Переместить / Дублировать / Удалить |
| Search field | Search the library | Поиск по библиотеке |
| Duplicate's proposed title | {title} (copy) | {title} (копия) |
| `library.question_count` | {count, plural, one {# question} other {# questions}} | {count, plural, one {# вопрос} few {# вопроса} many {# вопросов} other {# вопроса}} |
| Empty library | Build your first assignment | Создайте первое задание |
| Empty folder | This folder is empty | Папка пуста |
| No search results | Nothing found | Ничего не найдено |
| Move result notice | Moved. | Перемещено. |
| Name taken (`taken`) | A folder with this name already exists here. | Папка с таким названием здесь уже есть. |
| Depth limit | Folders can be nested at most three levels deep. | Папки можно вкладывать не глубже трёх уровней. |
| Caps (`too_many`) | The library has reached its limit. | В библиотеке больше нет места для новых элементов. |
| Folder not empty (`not_editable`) | Only an empty folder can be deleted. | Удалить можно только пустую папку. |
| Delete a material | Delete this material? Lessons and assignments that use it keep the file. | Удалить материал? Уроки и задания, в которых он есть, сохранят файл. |
| Delete an assignment | Delete this assignment? Files used only in it are deleted too. | Удалить задание? Файлы, которые есть только в нём, тоже удалятся. |
| Delete a folder | Delete this folder? | Удалить папку? |
| Gone item | This item is no longer in your library. | Этого элемента больше нет в библиотеке. |
| Unconfirmed tutor's upload | Confirm your email to upload files. | Подтвердите адрес почты, чтобы загружать файлы. |
| Autosave status | Saving… / Saved / Not saved | Сохраняется… / Сохранено / Не сохранено |
| Readiness | Ready to issue / {count, plural, one {# problem} other {# problems}} | Готово к выдаче / {count, plural, one {# недочёт} few {# недочёта} many {# недочётов} other {# недочёта}} |
| Blank title | Enter a title | Введите название |
| Body too large (`413`) | This assignment is too large to save | Задание слишком большое, сохранить его нельзя |
| Template JSON budget (`too_many` on a save) | Your assignments have reached their storage limit | Задания заняли всё отведённое им место |
| Stale save (`conflict`) | This changed elsewhere / Reload | Задание изменили в другом месте / Обновить |
| Block menu | Add block | Добавить блок |
| Block types | Instruction / Material / Single choice / Multiple choice / Short text / Number / Long text / File | Инструкция / Материал / Один вариант / Несколько вариантов / Короткий ответ / Число / Развёрнутый ответ / Файл |
| Block buttons | Move up / Move down / Duplicate / Delete | Выше / Ниже / Дублировать / Удалить |
| Preview and its frames | Preview / Phone / Computer | Предпросмотр / Телефон / Компьютер |
| `no_blocks` | Add at least one block | Добавьте хотя бы один блок |
| `text_missing` | Write the instruction | Напишите текст инструкции |
| `prompt_missing` | Write the question | Напишите вопрос |
| `too_few_options` | Add at least two options | Добавьте хотя бы два варианта |
| `option_empty` | Write the option's text | Напишите текст варианта |
| `correct_count` | Mark exactly one correct option | Отметьте ровно один верный вариант |
| `no_correct_option` | Mark at least one correct option | Отметьте хотя бы один верный вариант |
| `no_accepted_answer` | Add an accepted answer | Добавьте правильный ответ |
| `accepted_answer_empty` | An accepted answer is empty | Один из правильных ответов пуст |
| `value_invalid` | Enter a number with at most 6 decimal places | Введите число, не больше 6 знаков после запятой |
| `tolerance_invalid` | Enter a tolerance of zero or more, with at most 6 decimal places | Введите погрешность от нуля, не больше 6 знаков после запятой |
| Today's checklist step | Create an assignment | Создайте задание |
| Lesson materials block | Materials / No materials | Материалы / Материалов нет |
| Lesson material actions | Attach material / Detach / Download | Прикрепить материал / Открепить / Скачать |
| Library picker (change F4, pending owner review) | Search the library / Folder path / Folders / No materials here / A lesson holds at most {limit} materials. | Поиск по библиотеке / Путь к папке / Папки / Здесь нет материалов / К уроку можно прикрепить не больше {limit} материалов. |
| New lesson refused with materials chosen (`not_found`, change F4, pending owner review) | A chosen material is no longer in the library, or this student or group can no longer get lessons. | Выбранного материала больше нет в библиотеке, или этот ученик или группа больше недоступны для уроков. |

**Builder and student view texts as built** (slice 8 change F3, pending owner review); they load with the builder's code, as the library's texts load with the library page:

| Element | English | Русский |
|---|---|---|
| Block heading | {position}. {type} | {position}. {type} |
| Fields | Title / Text / Question / Points / Explanation | Название / Текст / Вопрос / Баллы / Пояснение |
| Markdown hint | Bold \*\*text\*\*, italic \*text\*, lists, \`code\` and https links. | Жирный \*\*текст\*\*, курсив \*текст\*, списки, \`код\` и ссылки https. |
| Explanation hint | Shown to students only when answers are visible. | Ученики видят его, только когда ответы открыты. |
| Options | Options / Option {number} / Correct: option {number} / Add option / Remove option {number} / At most 10 options | Варианты / Вариант {number} / Верный: вариант {number} / Добавить вариант / Удалить вариант {number} / Не больше 10 вариантов |
| Scoring | Scoring / All or nothing / Partial credit | Оценивание / Всё или ничего / Частичный балл |
| Accepted answers | Accepted answers / New accepted answer / Add answer / Remove the answer {answer} / At most 20 accepted answers | Правильные ответы / Новый правильный ответ / Добавить ответ / Удалить ответ {answer} / Не больше 20 правильных ответов |
| Number | Correct value / Tolerance | Правильное значение / Погрешность |
| Long text and file | Maximum length in characters / Allowed file types / Pictures, PDF, Office documents, Audio / Maximum number of files | Максимальная длина в символах / Разрешённые типы файлов / Изображения, PDF, Офисные документы, Аудио / Максимум файлов |
| Number ranges | Enter a whole number from 0 to 100 / from 1 to 20,000 / from 1 to 5 | Введите целое число от 0 до 100 / от 1 до 20 000 / от 1 до 5 |
| Prompt pictures | Pictures / Add picture / Remove the picture {filename} / Uploading {filename} / {filename} appears once the assignment is saved | Изображения / Добавить изображение / Удалить изображение {filename} / Загрузка: {filename} / {filename} появится после сохранения задания |
| Picture refusals | File is larger than 5 MB / This file type is not allowed here / Storage is full | Файл больше 5 МБ / Этот тип файла здесь не разрешён / Хранилище заполнено |
| Material block | Choose material / Change material / Download / Deleted from the library; students still receive the file. | Выбрать материал / Заменить материал / Скачать / Удалён из библиотеки; ученики всё равно получат файл. |
| Material picker | Choose a material / Search the library / Folder path / Folders / Materials / No materials here / Nothing found / Show more / Cancel | Выбор материала / Поиск по библиотеке / Путь к папке / Папки / Материалы / Здесь нет материалов / Ничего не найдено / Показать ещё / Отмена |
| Other | Retry / Blocks / Drag to reorder / Close / Preview width | Повторить / Блоки / Перетащите, чтобы изменить порядок / Закрыть / Ширина предпросмотра |
| Student view | Question {number} / Question {number} · Checked automatically / Question {number} · Your tutor checks it (H2, while the work can be answered, pending owner review) / Your answer / Up to {count} characters / {kinds}; up to {count} files / Attach files / Material / Download | Вопрос {number} / Вопрос {number} · Проверится автоматически / Вопрос {number} · Проверит преподаватель (H2) / Ваш ответ / Не больше {count} символов / {kinds}; не больше {count} файлов / Прикрепить файлы / Материал / Скачать |

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
| typing, another student (a student's view, slice 6) | animated dots plus text | Someone is typing… | Кто-то печатает… |
| edited | caption after the time | edited | изменено |
| deleted | muted italic placeholder | Message deleted | Сообщение удалено |
| erased by a purge (slice 6) | muted italic placeholder | Message removed | Сообщение стёрто |
| deleted or purged sender (slice 6) | sender label in `text-muted` | Deleted user | Удалённый пользователь |
| own message (slice 6) | sender label | You | Вы |
| sent before a reclaim (slice 6) | caption under the message | Sent before the account was reclaimed | Отправлено до восстановления аккаунта |
| unconfirmed link (slice 6, tutor only) | caption beside the label in the list and the header | Email not confirmed | Email не подтверждён |
| read-only channel | banner above the composer, composer disabled | This chat is read-only | Чат доступен только для чтения |
| chat no longer available (slice 6) | notice in place of the conversation, takes focus | This chat is no longer available. | Этот чат больше недоступен. |
| attaching while unconfirmed (slice 6) | text in place of the attach button | Confirm your email to send files | Подтвердите email, чтобы отправлять файлы |
| unread | count badge beside Messages, and a bold channel name in the channel list; counts above 99 show "99+" | — | — |
| offline (slice 6) | banner in the signed-in frame; the reason on disabled chat writes | No connection — reconnecting… / No connection | Нет соединения — переподключаемся… / Нет соединения |

The slice 6 texts are a proposal to confirm when the Messages pages are built (pending owner review).

## Notifications

Slice 4. The texts in this section are a proposal to confirm when F2 (notifications) and F3 (settings and unsubscribe) build the screens; the rule that a row shows its label (a name) and its text as separate elements is the contract, so no name stands in a position that needs a grammatical case in Russian; labels are user-written and shown as text only.

| Event | Recipient | English | Русский |
|---|---|---|---|
| `invitation.accepted` | tutor | Invitation accepted | Приглашение принято |
| `join_request.created` | tutor | New request to study with you | Новый запрос на занятия |
| `join_request.accepted` | student | Your request was accepted | Ваш запрос принят |
| `join_request.declined` | student | Your request was not accepted | Ваш запрос не принят |
| `relationship.ended` | tutor or student | Working together has ended | Совместные занятия завершены |
| `lesson.reminder_1h` (slice 5) | tutor or student | The lesson starts in an hour | Урок начнётся через час |
| `lesson.moved` (slice 5) | student | The lesson was moved | Урок перенесён |
| `lesson.cancelled` (slice 5) | student | The lesson was cancelled | Урок отменён |
| `lesson.series_changed` (slice 5) | student | Lessons of a series changed | Уроки серии изменились |
| `group.archived` (slice 5) | student | The group was archived | Группа перенесена в архив |
| `issue.created` (slice 9) | student | New assignment | Новое задание |
| `issue.withdrawn` (slice 9) | student | An assignment was withdrawn | Задание отозвано |
| `submission.submitted` (slice 9) | tutor | Work submitted for checking | Работа сдана на проверку |
| `submission.checked` (slice 9) | student | Your work was checked | Ваша работа проверена |
| `submission.returned` (slice 9) | student | Your work was returned for revision | Работа возвращена на доработку |
| `submission.review_updated` (slice 9) | student | Your tutor updated the review | Репетитор обновил проверку |
| `export.ready` (interface rework stage) | tutor or student | Your data export is ready | Выгрузка ваших данных готова |
| `export.failed` (interface rework stage) | tutor or student | Your data export could not be completed | Не удалось подготовить выгрузку данных |

The slice 5 rows are a proposal to confirm when the notification rows of the lesson events are built; their label is the lesson's title, or the group's label from the [label table](../../specs/schedule.md#labels-per-role), shown beside the text as for the slice 4 rows.

The export rows (specified 2026-11-23 by change U1b of the interface rework plan, pending owner review; pending implementation by E1) have no label and link to My data; an `export.failed` row adds the reason in a second line while the export exists ("Some data is no longer available", "It took too long", "It grew too large", "Something went wrong"). The slice 9 rows are a proposal to confirm when the notification rows of the homework events are built. A student's label is the assignment's title with the tutor's colour while the student can still read the submission, and is otherwise absent, with no link (a withdrawn assignment's row shows its text alone); the tutor's label is the student card's display name while the card is in scope. They load with the notification panel and page in a lazily loaded `notificationEvents` feature section (named apart from the global `notifications` texts, whose section key a feature section cannot share) when adding them to the global language chunk would leave less than 1 KB of the `pnpm check:bundle` headroom, and in the global chunk otherwise ([localisation](#localisation)). With the six texts in the global chunk the slice 9 issues change measured 0.9 KB of headroom (179.1 KB of 180.0 KB), so they load in the feature section. Each list (the panel's list and the page's table) loads the section once for all its rows. Until it loads, a homework row shows the generic label "Homework" / "Домашнее задание" from the global chunk in place of its text, so its link always has a non-empty accessible name; when the section fails to load, the rows keep that label and the list shows the shared load failure with "Try again" above them, which loads the section again; once the failure is gone, focus moves to the page's heading, or in the panel to the panel's heading, when it was lost (slice 10 change F-a, agent change pending owner review).

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

**Email groups,** in Settings → Notifications and on the unsubscribe page, each with the one-line description Settings shows below its name:

| Group | English | Русский | Description (English) | Description (Русский) |
|---|---|---|---|---|
| `lesson_reminders` | Lesson reminders | Напоминания об уроках | Before each lesson starts. | Перед началом каждого урока. |
| `lesson_changes` | Lesson changes | Изменения в расписании | When a lesson is moved or cancelled. | Когда урок переносят или отменяют. |
| `homework` | New assignments | Новые задания | When a tutor gives you an assignment. | Когда репетитор выдаёт задание. |
| `results` | Checked work | Проверенные работы | When a tutor has checked your work. | Когда репетитор проверил вашу работу. |
| `submissions` | Submitted work | Сданные работы | When a student hands in work. | Когда ученик сдаёт работу. |
| `message_digest` | Unread messages | Непрочитанные сообщения | A summary of messages you have not read. | Сводка сообщений, которые вы не прочитали. |

**Settings → Notifications:**

| Element | English | Русский |
|---|---|---|
| Section heading | Optional emails | Необязательные письма |
| Required emails | Required emails cannot be turned off: email confirmation, password reset, invitations, the end of working together and a tutor’s account deletion. | Обязательные письма отключить нельзя: подтверждение адреса, сброс пароля, приглашения, завершение совместных занятий и удаление аккаунта репетитора. |
| Failed save, followed by the error's own message | The setting for “{group}” was not saved. | Настройка «{group}» не сохранена. |
| Retry | Try again | Повторить |

**Unsubscribe page and sign-out messages:**

| Element | English | Русский |
|---|---|---|
| While the preview loads | Checking the link | Проверяем ссылку |
| Group line | Emails: {group} | Письма: {group} |
| Confirm button | Turn off these emails | Отключить эти письма |
| Result | These emails are turned off | Эти письма отключены |
| Link to settings | All email settings | Все настройки писем |
| Invalid link | This link is no longer valid | Ссылка больше не действует |
| Signed out after an unfixable `4401` (`sessionEnded`) | You were signed out on this device | Вы вышли из аккаунта на этом устройстве |
| A student card that a live refetch found gone | This student is no longer on your list. | Этого ученика больше нет в вашем списке. |

`{group}` is the group's name from the table above, which stands after a colon and needs no case.

## Phone frame

Texts of the [phone frame](navigation.md#frames) (slice 10 change G0, owner's decisions of 2026-11-21, pending owner review; built in change G1). Every other text of the phone frame and of the phone rules in [pages](pages.md) is an existing text: the bottom bar's other labels are the section names, the Homework count's accessible names are the sidebar's, and the More sheet reuses Notifications, Settings, Sign out, "Signing out…" and the sign-out failure. No page's **Phone** rule adds a text.

| Key | English | Russian | Chunk | Added gzip bytes (Russian / English chunk) |
|---|---|---|---|---|
| `section.chats` | Chats | Чаты | global language chunk | 15 / 13 |
| `frame.chatsWithCount` | Chats, {count} unread | Чаты, непрочитанных: {count} | global language chunk | 22 / 17, with `frame.chatsOverCap` |
| `frame.chatsOverCap` | Chats, more than 99 unread | Чаты, непрочитанных: больше 99 | global language chunk | (counted with `frame.chatsWithCount`) |
| `frame.more` | More | Ещё | global language chunk | 24 / 13, with `frame.moreClose` |
| `frame.moreClose` | Close | Закрыть | global language chunk | (counted with `frame.more`) |

The counted names of Chats begin with its visible label, as WCAG 2.5.3 requires; the Homework item keeps the sidebar's names, which already begin with "Homework" / "Задания". The five keys add 61 bytes to the gzip size of the Russian chunk, the largest language chunk that `pnpm check:bundle` counts in the entry, measured on the build of `main` at `ef9a98d` by inserting them into the built chunk; the entry then had 629 bytes of headroom ([phone design record](../../reviews/slice-10-phone-design.md)). They are frame texts needed on every page, so they belong to the global chunk (K21 of the [slice 10 plan](../../plans/slice-10.md)).

## Tutor labels

In every student view, items carry their tutor's label: an 8 px dot in the relationship colour plus the tutor's name (or initials in narrow layouts). The filter "All tutors / {name}" sits at the top of Home, Schedule and Homework.

## Time and scores

Strings with fixed keys, so tests assert by key and both languages stay in step. Score strings appear only in tutor views. From slice 9 the four `homework.*` keys below belong to the `homework` feature text section ([homework texts](#homework-texts)) and load with the tutor's pages that show them, never with the global language chunk or a student page; the `time.*` keys stay in the global language chunk.

| Key | English | Русский |
|---|---|---|
| `homework.auto_points` | {count, plural, one {# automatic point} other {# automatic points}} | {count, plural, one {# балл за автопроверку} few {# балла за автопроверку} many {# баллов за автопроверку} other {# балла за автопроверку}} |
| `homework.score` | {earned} of {max} ({percent}) | {earned} из {max} ({percent}) |
| `homework.not_scored` | Not scored | Без оценки |
| `homework.points` (the student card's Homework tab) | {earned} of {max} | {earned} из {max} |
| `time.other_zone` | {time} {city} time ({local} your time) | {time} по времени {city} ({local} по вашему) |
| `time.other_offset` | {time} {offset} ({local} your time) | {time} по {offset} ({local} по вашему) |

The bundled timezone table gives each city in the form the string needs: "Moscow" in English and the genitive "Москвы" in Russian; its nominative "Москва" names the zone in the account settings' timezone choice (design slice step D1, pending owner review). A zone the table does not list uses `time.other_offset`, where `{offset}` is the zone's offset at that instant in the form `UTC±hh:mm` (from slice 5; the wording is a proposal to confirm with the lesson page).

## Feedback messages

- **Toasts:**
  - confirm completed actions in one short sentence ("Lesson cancelled" / "Урок отменён");
  - errors persist until dismissed and offer a retry when the action is retryable.
- **A failed refetch of a loaded page** keeps the page and says: "The latest data could not be loaded. The page shows what was loaded before." / "Не удалось обновить данные. На странице показано то, что загрузилось раньше.", with Retry ("Try again" / "Повторить").
- **Inline validation** appears below the field after the first blur or submit, in `danger`, with an icon.
- **Destructive actions** (ending a relationship from either side, removing a provisional student, cancelling a series, withdrawing an issued assignment, deleting a recording, deleting the account) need a confirmation dialog that names the consequence. Deleting the account additionally asks for the password.

## Localisation

- **Libraries:** frontend strings live in i18next resource files per language (`ru`, `en`) with ICU message format, including plurals. Russian uses one / few / many / other: "1 урок, 2 урока, 5 уроков".
- **Loading:** each language's resource file is its own chunk. Before the first render the entry loads only the active language: the stored preference, else the browser's language when it is Russian, else English. A language change loads the other language's chunk and applies only once it has loaded, so no untranslated key ever shows; a later choice overtakes an earlier one that is still loading, and a failed load keeps the current language and logs `language_load_failed` with the error kind. The stored preference is written, and a signed-in person's choice on a public page is saved to the account, only once the switch has applied; the account's language applied at sign-in is stored the same way. The language switch is an exception to the busy-state rule of [shared behavior](pages.md#shared-behavior): because a later choice overtakes an earlier one, its buttons stay available while a language loads and show no busy state, and the pressed button changes when the new language applies. `pnpm check:bundle` counts the largest language chunk in the entry and page budgets.
- **Feature text sections** (from slice 8 change F3, extended in slice 9 by the owner's decision of 2026-11-11): a page's own texts are a feature section that `featureTexts.ts` adds to the catalogue, in both languages, when the page first loads, through `useFeatureTexts`, so they stay out of the language chunk of the first load. Slice 8 has `library`, `builder` and `assignment`; slice 9 adds `homework` (the tutor's Issue dialog, issued and review pages, and the card, group and lesson parts) and `homeworkPanels` (Today's and Home's homework panels), and extends `assignment` with the student's Homework list, assignment page and results ([homework texts](#homework-texts)). From slice 9 every new page's texts load this way: the global language chunk gains only the Homework counts' accessible names and the section and student tab names (the tutor's tab names and unconfirmed-tutor prompt are in the `homework` section; no score key: the `homework.*` score keys are in the `homework` section), and the homework notification texts unless they need their own `notifications` section; slice 9 adds no API error code and no field code, so refusals specific to these pages are worded in their sections. `pnpm check:bundle` asserts that the review screen's modules and the `homework` texts are absent from the entry and from every student page chunk. The lesson page (`src/features/schedule/LessonPage.tsx`) is shared by both roles and is the one exception: it statically imports the tutor's lesson issue parts, which carry the `homework` texts, and renders them only for tutors, so `check:bundle` checks it separately, failing when it reaches any review module or any `src/features/homework/` module outside its pinned list, and also fails when a checked folder holds no module, so a renamed folder cannot make the student page check pass vacuously (added 2026-11-15 by the slice 9 review backend follow-up, agent change pending owner review). From slice 9 change F3 the account reclaim page's texts are the `reclaim` feature section, moved out of the global language chunk unchanged so the first load keeps room for the tutor's Homework count; the reclaim page shows its choices only once that section has loaded (agent change pending owner review). From change U2 of the interface rework stage the student card's texts are the `studentCard` section and the Groups and group pages' texts the `groups` section, moved out of the global language chunk unchanged so the first load keeps room for the stage's messenger and homework texts (the owner's decision of 2026-11-23); both pages keep their page header and show the delayed loading state inside the page until their section has loaded, so the heading and its focus survive, and the material picker's "Chosen: {count} of {limit}" moved to `newLesson.materialsChosen` (pending owner review).
- **API error codes** map to messages in the resource files; an unknown code shows a generic message with the request identifier.
- **Formats:**
  - dates and times use `Intl.DateTimeFormat` in the user's language and timezone;
  - Russian uses 24-hour time and day-month order ("12 нояб., 18:30"); English uses the `en-GB` format ("12 Nov, 18:30") so dates read the same for every English user and in tests;
  - relative times use `Intl.RelativeTimeFormat`;
  - numbers and percentages use `Intl.NumberFormat`, including the `{percent}` in tutor score strings.
- **Emails** use server-side templates with the same keys and plural rules.
- **User content is never translated:** names, titles, messages, assignment text.
- **Text expansion:** layouts tolerate 30 % longer strings; buttons do not truncate labels.
