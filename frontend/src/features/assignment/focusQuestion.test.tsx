import { cleanup, render, screen } from '@testing-library/react';
import { I18nextProvider } from 'react-i18next';
import { afterEach, describe, expect, test } from 'vitest';

import { createI18n } from '../../lib/i18n/i18n.ts';
import { type Answer, type QuestionBlock, emptyAnswer } from './api.ts';
import { firstFieldOf } from './focusQuestion.ts';
import { type Answering, StudentBlocks } from './StudentBlocks.tsx';
import { assignmentTexts } from './texts.en.ts';

afterEach(() => {
  cleanup();
});

const choice: QuestionBlock = {
  type: 'single_choice',
  id: '0d3c3c1e-2b7a-4c55-9a21-5d2e8f4a6b70',
  prompt: 'Yesterday I ___ to school.',
  images: [],
  options: [
    { id: 'b1', text: 'goed' },
    { id: 'b2', text: 'went' },
  ],
};

const text: QuestionBlock = {
  type: 'short_text',
  id: '7e1d4c2a-0b7d-4c55-9a21-5d2e8f4a6b71',
  prompt: 'Write the past form of go.',
  images: [],
};

function answering(): Answering {
  return {
    answers: new Map<string, Answer>([[text.id, emptyAnswer]]),
    editable: new Set([choice.id, text.id]),
    invalid: new Set([text.id]),
    onChange: () => undefined,
    fileControl: () => null,
    below: () => null,
  };
}

async function renderQuestions(): Promise<void> {
  const i18n = await createI18n('en');
  i18n.addResourceBundle('en', 'translation', { assignment: assignmentTexts }, true, true);
  render(
    <I18nextProvider i18n={i18n}>
      <StudentBlocks
        assignment={{ title: 'Past simple', blocks: [choice, text] }}
        origin="https://ovc.example"
        answering={answering()}
        showTitle={false}
      />
    </I18nextProvider>,
  );
}

describe('the field a refused question moves focus to', () => {
  test('is the answer field of a text question, marked invalid', async () => {
    await renderQuestions();

    const field = firstFieldOf(text.id);

    expect(field).toBe(screen.getByRole('group', { name: 'Question 2' }).querySelector('input'));
    expect(field?.getAttribute('aria-invalid')).toBe('true');
  });

  test('is the first option of a choice question', async () => {
    await renderQuestions();

    const field = firstFieldOf(choice.id);

    expect(field).toBe(screen.getByRole('radio', { name: 'goed' }));
  });
});
