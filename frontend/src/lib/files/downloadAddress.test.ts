import { describe, expect, test } from 'vitest';

import { studentFileAddress } from './downloadAddress.ts';

const origin = 'https://ovc.example';
const fileId = '0f8b6c1e-7a3d-4c55-9b21-5d2e8f4a6b70';

describe('student file addresses', () => {
  test('a signed address of the own origin is kept', () => {
    const url = `/api/v1/files/${fileId}?t=signed`;

    const address = studentFileAddress(url, origin);

    expect(address).toBe(`${origin}/api/v1/files/${fileId}?t=signed`);
  });

  test.each([
    ['another origin', `https://outside.example/api/v1/files/${fileId}?t=signed`],
    ['a protocol-relative host', `//outside.example/api/v1/files/${fileId}?t=signed`],
    ['user information', `https://user:secret@ovc.example/api/v1/files/${fileId}?t=signed`],
    ['a fragment', `/api/v1/files/${fileId}?t=signed#part`],
    ['another route', '/api/v1/auth/logout?t=signed'],
    ['a path below the file', `/api/v1/files/${fileId}/raw?t=signed`],
    ['an upper-case identifier', `/api/v1/files/${fileId.toUpperCase()}?t=signed`],
    ['a missing token', `/api/v1/files/${fileId}`],
    ['two tokens', `/api/v1/files/${fileId}?t=one&t=two`],
    ['another parameter', `/api/v1/files/${fileId}?t=signed&disposition=inline`],
    ['a script address', 'javascript:alert(1)'],
    ['a data address', 'data:image/png;base64,AAAA'],
  ])('an address with %s is refused', (_, url) => {
    const address = studentFileAddress(url, origin);

    expect(address).toBeNull();
  });
});
