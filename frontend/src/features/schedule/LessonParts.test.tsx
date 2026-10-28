import { cleanup, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, test } from 'vitest';

import { LessonTitle } from './LessonParts.tsx';

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
