import type { EmailSettingsTexts } from './emailSettingsTexts.ts';

export const emailSettingsTexts: EmailSettingsTexts = {
  heading: 'Необязательные письма',
  required:
    'Обязательные письма отключить нельзя: подтверждение адреса, сброс пароля, приглашения, завершение совместных занятий и удаление аккаунта репетитора.',
  saveFailed: 'Настройка «{group}» не сохранена.',
  retry: 'Повторить',
  groups: {
    lesson_reminders: 'Напоминания об уроках',
    lesson_changes: 'Изменения в расписании',
    homework: 'Новые задания',
    results: 'Проверенные работы',
    submissions: 'Сданные работы',
    message_digest: 'Непрочитанные сообщения',
  },
  hints: {
    lesson_reminders: 'Перед началом каждого урока.',
    lesson_changes: 'Когда урок переносят или отменяют.',
    homework: 'Когда репетитор выдаёт задание.',
    results: 'Когда репетитор проверил вашу работу.',
    submissions: 'Когда ученик сдаёт работу.',
    message_digest: 'Сводка сообщений, которые вы не прочитали.',
  },
};
