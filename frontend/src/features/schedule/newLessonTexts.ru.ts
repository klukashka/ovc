import type { NewLessonFormTexts } from './newLessonTexts.ts';

export const newLessonFormTexts: NewLessonFormTexts = {
  back: {
    schedule: 'Назад к расписанию',
    today: 'Назад к странице «Сегодня»',
    student: 'Назад к ученику',
    group: 'Назад к группе',
    lesson: 'Назад к уроку',
  },
  defaultTitle: 'Урок',
  defaultGroupTitle: 'Урок {group}',
  durationPresets: 'Обычная длительность',
  minutes: '{count} мин',
  addDescription: 'Добавить описание',
  repeat: 'Повторять каждую неделю',
  weekdays: 'Дни недели',
  ends: 'Окончание серии',
  endsOn: 'В дату',
  endDate: 'Дата окончания',
  afterCount: 'После нескольких уроков',
  count: 'Число уроков',
  submit: 'Создать урок',
  audienceRequired: 'Выберите ученика или группу.',
  weekdaysRequired: 'Выберите хотя бы один день недели.',
  audienceChanged: 'Этот ученик или группа больше недоступны для уроков. Выберите другого.',
};
