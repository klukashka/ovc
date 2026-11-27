import type { TeachingSettingsTexts } from './teachingTexts.ts';

export const teachingSettingsTexts: TeachingSettingsTexts = {
  columns: {
    student: 'Ученик',
    blocked: 'Дата блокировки',
    actions: 'Действия',
  },
  storage: 'Использование хранилища',
  files: 'Файлы',
  storageAmount: '{used} из {quota}',
  code: 'Код репетитора',
  codeHint: 'Ученик вводит этот код, чтобы отправить вам заявку.',
  regenerate: 'Обновить код',
  regenerateHint: 'После обновления текущий код сразу перестанет работать.',
  codeActions: 'Действия с кодом',
  regenerateTitle: 'Обновить код репетитора?',
  regenerateConsequence:
    'Текущий код сразу перестанет работать для новых заявок. Уже отправленные и ожидающие заявки это не затронет; чтобы остановить человека, отклоните его заявку с блокировкой. Никто не получит уведомления. Это нельзя отменить, а обновлять код можно не больше 10 раз в день.',
  cancel: 'Отмена',
  regenerated: 'Создан новый код. Старый больше не работает.',
  blocked: 'Заблокированные ученики',
  blockedHint: 'Заявки от этих учеников к вам не приходят.',
  blockedEmpty: 'Вы никого не заблокировали.',
  blockedAt: 'В списке с {date}',
  deletedAccount: 'Удалённый аккаунт',
  unblock: 'Разблокировать',
  unblockNamed: 'Разблокировать: {name}',
  unblockDeleted: 'Разблокировать удалённый аккаунт, заблокированный {date}',
  unblocked: 'Ученик разблокирован.',
  unblockChanged: 'Этот ученик уже не заблокирован. Список обновлён.',
  showMore: 'Показать ещё',
};
