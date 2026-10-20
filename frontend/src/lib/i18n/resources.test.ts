import { IntlMessageFormat } from 'intl-messageformat';
import { describe, expect, test } from 'vitest';

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

const catalogues = { ru: messages(ru), en: messages(en) };

describe('translation resources', () => {
  test('both languages define the same keys', () => {
    const russianKeys = [...catalogues.ru.keys()].sort();

    const englishKeys = [...catalogues.en.keys()].sort();

    expect(englishKeys).toEqual(russianKeys);
  });

  test.each(Object.entries(catalogues))('every %s message is valid ICU and formats plurals', (language, catalogue) => {
    const formatted = [...catalogue.entries()].map(([key, message]) => {
      const format = new IntlMessageFormat(message, language);
      return [key, [0, 1, 2, 5, 21].map((count) => String(format.format({ count })))] as const;
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
