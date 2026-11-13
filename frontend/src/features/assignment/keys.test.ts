import { describe, expect, test } from 'vitest';

import { assignmentCountKey } from './count.ts';
import { assignmentsKey } from './keys.ts';

describe('student homework query keys', () => {
  test('the sidebar count lives under the key that live refetch invalidates', () => {
    const prefix = assignmentCountKey.slice(0, assignmentsKey.length);

    expect(prefix).toEqual([...assignmentsKey]);
  });
});
