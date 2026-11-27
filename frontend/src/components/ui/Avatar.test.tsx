import { cleanup, render } from '@testing-library/react';
import { afterEach, expect, test } from 'vitest';

import { Avatar } from './Avatar.tsx';

afterEach(cleanup);

test.each([
  { name: '\u202e a\u0301lice', initial: 'A\u0301' },
  { name: ' \u200d👩🏽‍🏫 Teacher', initial: '👩🏽‍🏫' },
  { name: ' 😀 Teacher', initial: '😀' },
])('the initial is the first grapheme after format characters: $name', ({ name, initial }) => {
  const avatar = <Avatar name={name} avatarId={null} colour="moss" size="medium" />;

  const { container } = render(avatar);

  expect(container.textContent).toBe(initial);
  expect(container.firstElementChild?.getAttribute('dir')).toBe('auto');
  expect(container.firstElementChild?.getAttribute('aria-hidden')).toBe('true');
});

test('a label-kind row shows the person glyph', () => {
  const avatar = <Avatar name="" avatarId={null} colour="moss" />;

  const { container } = render(avatar);

  expect(container.querySelector('svg')).not.toBeNull();
  expect(container.textContent).toBe('');
  expect(container.firstElementChild?.classList.contains('bg-hover')).toBe(true);
});
