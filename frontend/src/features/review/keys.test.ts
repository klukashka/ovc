import { describe, expect, test } from 'vitest';

import { reviewCountKey } from '../../app/homeworkCount.ts';
import { queueKey, queuePanelKey, reviewKey, reviewSubmissionKey } from './keys.ts';

describe('review query keys', () => {
  test('every review query lives under the key that live refetch invalidates', () => {
    const keys = [
      reviewCountKey,
      queueKey({ issue: null, group: null, student: null }),
      queuePanelKey(5),
      reviewSubmissionKey('submission-1'),
    ];

    const prefixes = keys.map((key) => key.slice(0, reviewKey.length));

    expect(prefixes).toEqual(keys.map(() => [...reviewKey]));
  });
});
