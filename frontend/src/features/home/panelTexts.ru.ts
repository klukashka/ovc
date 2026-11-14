import type { HomeworkPanelTexts } from './panelTexts.ts';

export const homeworkPanelTexts: HomeworkPanelTexts = {
  todo: 'Нужно сделать',
  nothingToDo: 'Делать нечего',
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
  states: {
    todo: 'Нужно сделать',
    dueSoon: 'Скоро срок',
    overdue: 'Срок прошёл',
    inProgress: 'В работе',
    returned: 'На доработке',
  },
};
