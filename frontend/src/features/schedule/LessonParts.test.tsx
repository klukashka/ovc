import { cleanup, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, test } from 'vitest';

import { createI18n } from '../../lib/i18n/i18n.ts';
import { LessonTitle, audienceLabel } from './LessonParts.tsx';

afterEach(() => {
  cleanup();
});

describe('lesson title', () => {
  test('renders in an element isolated with dir="auto"', () => {
    const title = 'שיעור <b>1</b>';

    render(
      <p>
        18:00 <LessonTitle title={title} cancelled={false} /> B1
      </p>,
    );

    const element = screen.getByText(title);
    expect(element.getAttribute('dir')).toBe('auto');
    expect(element.textContent).toBe(title);
    expect(element.querySelector('b')).toBeNull();
    expect(element.parentElement?.textContent).toBe(`18:00 ${title} B1`);
  });
});

describe('audience label', () => {
  test("a one-to-one lesson whose card name a student's purge erased shows the fixed former student text", async () => {
    const i18n = await createI18n('ru');

    const label = audienceLabel(i18n.t, {
      kind: 'one_to_one',
      label: null,
      label_kind: 'former_student',
      colour: null,
    });

    expect(label).toBe('Бывший ученик');
  });
});
