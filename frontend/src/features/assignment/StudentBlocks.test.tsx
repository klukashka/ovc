import { cleanup, render, screen } from '@testing-library/react';
import { I18nextProvider } from 'react-i18next';
import { afterEach, describe, expect, test } from 'vitest';

import { createI18n } from '../../lib/i18n/i18n.ts';
import { StudentBlocks } from './StudentBlocks.tsx';
import type { StudentBlock } from './studentView.ts';
import { assignmentTexts } from './texts.ru.ts';

afterEach(() => {
  cleanup();
});

const prompt = 'Переведите слово «кошка»';

const questions: readonly [string, StudentBlock][] = [
  ['a short answer', { type: 'short_text', id: 'block-1', prompt, images: [] }],
  [
    'a single choice',
    {
      type: 'single_choice',
      id: 'block-1',
      prompt,
      images: [],
      options: [
        { id: 'option-1', text: 'cat' },
        { id: 'option-2', text: 'dog' },
      ],
    },
  ],
];

async function renderQuestion(block: StudentBlock): Promise<void> {
  const i18n = await createI18n('ru');
  i18n.addResourceBundle('ru', 'translation', { assignment: assignmentTexts }, true, true);
  render(
    <I18nextProvider i18n={i18n}>
      <StudentBlocks assignment={{ title: 'Домашнее задание', blocks: [block] }} origin="https://ovc.example" />
    </I18nextProvider>,
  );
}

describe('a question of the student view', () => {
  test.each(questions)('announces %s with its prompt as the description of its group', async (_name, block) => {
    await renderQuestion(block);

    expect(screen.queryByRole('group', { name: 'Вопрос 1', description: prompt })).not.toBeNull();
  });
});
