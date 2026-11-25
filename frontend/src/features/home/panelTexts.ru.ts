import type { HomeworkPanelTexts } from './panelTexts.ts';

export const homeworkPanelTexts: HomeworkPanelTexts = {
  todo: 'Нужно сделать',
  nothingToDo: 'Делать нечего',
  nothingToDoSentence: 'Здесь появятся новые задания от ваших репетиторов.',
  start: 'Начать',
  continue: 'Продолжить',
  allHomework: 'Все задания, нужно сделать: {count}',
  noDeadline: 'Без срока',
  needsChecking: 'На проверке',
  openQueue: 'Открыть очередь проверки',
  nothingToCheck: 'Проверять нечего',
  late: 'С опозданием',
  reviewInProgress: 'Проверка начата',
  deadlines: 'Ближайшие сроки',
  submittedProgress: 'Сдали {submitted} из {total}',
  allIssued: 'Все выданные задания',
  noDeadlines: 'В ближайшие 3 дня сроков нет',
  cue: 'Выдано до восстановления аккаунта',
  recentlyChecked: 'Недавно проверенные',
  nothingChecked: 'Проверенных работ пока нет',
  nothingCheckedSentence: 'Отзыв появится здесь, когда репетитор проверит работу.',
  noNextLessonSentence: 'Здесь появится следующий урок, когда репетитор его запланирует.',
  correctedFile: 'Есть исправленный файл',
  checkedOn: 'Проверено {date}',
  newFeedback: 'Новое',
  newFeedbackName: 'Новый отзыв',
  unconfirmed: {
    heading: 'После подтверждения email',
    withoutLink:
      'Расписание, задания, сообщения и уведомления появятся здесь после подтверждения email. А пока вы можете ввести код репетитора, посмотреть своих репетиторов и изменить настройки.',
    withLink:
      'После подтверждения email в этом браузере вам станут доступны ещё уведомления, групповые уроки и чаты и ответы в виде файлов.',
  },
  states: {
    todo: 'Нужно сделать',
    dueSoon: 'Скоро срок',
    overdue: 'Срок прошёл',
    inProgress: 'В работе',
    returned: 'На доработке',
  },
};
