import { IntlMessageFormat } from 'intl-messageformat';
import { describe, expect, test } from 'vitest';

import { assignmentTexts as assignmentEnglish } from '../../features/assignment/texts.en.ts';
import { assignmentTexts as assignmentRussian } from '../../features/assignment/texts.ru.ts';
import { invitationPageTexts as invitationPageEnglish } from '../../features/auth/invitationTexts.en.ts';
import { invitationPageTexts as invitationPageRussian } from '../../features/auth/invitationTexts.ru.ts';
import { builderTexts as builderEnglish } from '../../features/builder/texts.en.ts';
import { builderTexts as builderRussian } from '../../features/builder/texts.ru.ts';
import { groupsTexts as groupsEnglish } from '../../features/groups/texts.en.ts';
import { groupsTexts as groupsRussian } from '../../features/groups/texts.ru.ts';
import { homeworkPanelTexts as homeworkPanelsEnglish } from '../../features/home/panelTexts.en.ts';
import { homeworkPanelTexts as homeworkPanelsRussian } from '../../features/home/panelTexts.ru.ts';
import { libraryTexts as libraryEnglish } from '../../features/library/texts.en.ts';
import { libraryTexts as libraryRussian } from '../../features/library/texts.ru.ts';
import { newLessonFormTexts as newLessonFormEnglish } from '../../features/schedule/newLessonTexts.en.ts';
import { newLessonFormTexts as newLessonFormRussian } from '../../features/schedule/newLessonTexts.ru.ts';
import { emailSettingsTexts as emailSettingsEnglish } from '../../features/settings/emailSettingsTexts.en.ts';
import { emailSettingsTexts as emailSettingsRussian } from '../../features/settings/emailSettingsTexts.ru.ts';
import { myDataTexts as myDataEnglish } from '../../features/settings/myDataTexts.en.ts';
import { myDataTexts as myDataRussian } from '../../features/settings/myDataTexts.ru.ts';
import { settingsTexts as settingsEnglish } from '../../features/settings/settingsTexts.en.ts';
import { settingsTexts as settingsRussian } from '../../features/settings/settingsTexts.ru.ts';
import { teachingSettingsTexts as teachingSettingsEnglish } from '../../features/settings/teachingTexts.en.ts';
import { teachingSettingsTexts as teachingSettingsRussian } from '../../features/settings/teachingTexts.ru.ts';
import { unsubscribeTexts as unsubscribeEnglish } from '../../features/settings/unsubscribeTexts.en.ts';
import { unsubscribeTexts as unsubscribeRussian } from '../../features/settings/unsubscribeTexts.ru.ts';
import { studentCardTexts as studentCardEnglish } from '../../features/students/cardTexts.en.ts';
import { studentCardTexts as studentCardRussian } from '../../features/students/cardTexts.ru.ts';
import { studentListTexts as studentListEnglish } from '../../features/students/listTexts.en.ts';
import { studentListTexts as studentListRussian } from '../../features/students/listTexts.ru.ts';
import { myTutorsTexts as myTutorsEnglish } from '../../features/tutors/texts.en.ts';
import { myTutorsTexts as myTutorsRussian } from '../../features/tutors/texts.ru.ts';
import { en } from './en.ts';
import { ru } from './ru.ts';

type MessageTree = Readonly<{ [key: string]: string | MessageTree }>;

function messages(tree: MessageTree, prefix = ''): Map<string, string> {
  const entries = new Map<string, string>();
  for (const [key, value] of Object.entries(tree)) {
    const path = prefix === '' ? key : `${prefix}.${key}`;
    if (typeof value === 'string') {
      entries.set(path, value);
    } else {
      for (const [nested, message] of messages(value, path)) {
        entries.set(nested, message);
      }
    }
  }
  return entries;
}

const catalogues = {
  ru: messages({
    ...ru,
    library: libraryRussian,
    builder: builderRussian,
    assignment: assignmentRussian,
    studentCard: studentCardRussian,
    groups: groupsRussian,
    myData: myDataRussian,
    myTutors: myTutorsRussian,
    homeworkPanels: homeworkPanelsRussian,
    invitationPage: invitationPageRussian,
    settings: settingsRussian,
    emailSettings: emailSettingsRussian,
    unsubscribe: unsubscribeRussian,
    teachingSettings: teachingSettingsRussian,
    studentList: studentListRussian,
    newLessonForm: newLessonFormRussian,
  }),
  en: messages({
    ...en,
    library: libraryEnglish,
    builder: builderEnglish,
    assignment: assignmentEnglish,
    studentCard: studentCardEnglish,
    groups: groupsEnglish,
    myData: myDataEnglish,
    myTutors: myTutorsEnglish,
    homeworkPanels: homeworkPanelsEnglish,
    invitationPage: invitationPageEnglish,
    settings: settingsEnglish,
    emailSettings: emailSettingsEnglish,
    unsubscribe: unsubscribeEnglish,
    teachingSettings: teachingSettingsEnglish,
    studentList: studentListEnglish,
    newLessonForm: newLessonFormEnglish,
  }),
};

function sampleValues(message: string, count: number): Record<string, number> {
  const names = [...message.matchAll(/\{\s*(\w+)/g)].map((match) => match[1] ?? '');
  return Object.fromEntries([...names, 'count'].map((name) => [name, count]));
}

describe('translation resources', () => {
  test('both languages define the same keys', () => {
    const russianKeys = [...catalogues.ru.keys()].sort();

    const englishKeys = [...catalogues.en.keys()].sort();

    expect(englishKeys).toEqual(russianKeys);
  });

  test.each(Object.entries(catalogues))('every %s message is valid ICU and formats plurals', (language, catalogue) => {
    const formatted = [...catalogue.entries()].map(([key, message]) => {
      const format = new IntlMessageFormat(message, language);
      return [key, [0, 1, 2, 5, 21].map((count) => String(format.format(sampleValues(message, count))))] as const;
    });

    for (const [key, outputs] of formatted) {
      for (const output of outputs) {
        expect(output, key).not.toMatch(/[{}]/);
      }
    }
  });

  test('Russian plurals use the few and many forms', () => {
    const format = new IntlMessageFormat(ru.frame.notificationsWithCount, 'ru');

    const outputs = [1, 3, 5, 21].map((count) => format.format({ count }));

    expect(outputs).toEqual([
      'Уведомления, 1 непрочитанное',
      'Уведомления, 3 непрочитанных',
      'Уведомления, 5 непрочитанных',
      'Уведомления, 21 непрочитанное',
    ]);
  });
});
