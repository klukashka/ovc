import { describe, expect, test } from 'vitest';

import { channelLabel } from './channelLabel.ts';
import { chatTexts } from './texts.ru.ts';

describe('channel label', () => {
  test("a direct channel whose card name a student's purge erased shows the fixed former student text", () => {
    const channel = { label: null, label_kind: 'former_student' as const };

    const label = channelLabel(channel, chatTexts);

    expect(label).toBe('Бывший ученик');
  });
});
