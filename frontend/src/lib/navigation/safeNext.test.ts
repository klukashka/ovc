import { describe, expect, test } from 'vitest';

import { safeNext } from './safeNext.ts';

const origin = 'https://ovc.example';

describe('safeNext', () => {
  test.each([
    ['/schedule?week=3#today', '/schedule?week=3#today'],
    ['/lessons/42', '/lessons/42'],
    ['/a/../settings/profile', '/settings/profile'],
    ['/%2F%2Fevil.example', '/%2F%2Fevil.example'],
    ['/%5C%5Cevil.example', '/%5C%5Cevil.example'],
  ])('keeps the same-origin address %s as a path', (value, expected) => {
    const result = safeNext(value, origin);

    expect(result).toBe(expected);
  });

  test.each([
    ['//evil.example'],
    ['/\\evil.example'],
    ['https://evil.example/today'],
    ['javascript:alert(1)'],
    ['/\tevil'],
    ['/\n/evil.example'],
    ['/.//evil.example'],
    ['/./\\evil.example'],
    ['/a/..//evil.example'],
    ['/%2e//evil.example'],
    ['today'],
    [''],
  ])('refuses %s', (value) => {
    const result = safeNext(value, origin);

    expect(result).toBeNull();
  });

  test('refuses a missing value', () => {
    const result = safeNext(null, origin);

    expect(result).toBeNull();
  });
});
