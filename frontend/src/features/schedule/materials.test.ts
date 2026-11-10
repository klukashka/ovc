import { expect, test } from 'vitest';

import { type LessonMaterial, materialAddress } from './materials.ts';

const origin = 'https://ovc.example';
const attachment = '00000000-0000-4000-8000-0000000000a1';

const material: LessonMaterial = {
  attachment_id: attachment,
  material_id: '00000000-0000-4000-8000-0000000000b1',
  title: 'Рабочий лист',
  filename: 'Рабочий лист.pdf',
  media_type: 'application/pdf',
  size_bytes: 1200,
  width: null,
  height: null,
  url: `/api/v1/files/${attachment}?t=token`,
  expires_at: '2026-11-10T12:10:00Z',
};

const refusedAddresses: readonly string[] = [
  `https://outside.example/api/v1/files/${attachment}?t=token`,
  `javascript:alert(1)//api/v1/files/${attachment}?t=token`,
  `data:text/html,<b>${attachment}</b>`,
  `https://user:secret@ovc.example/api/v1/files/${attachment}?t=token`,
  `/api/v1/files/${attachment}?t=token#fragment`,
  `/api/v1/files/${attachment}?t=token&disposition=inline`,
  `/api/v1/files/${attachment}?t=one&t=two`,
  `/api/v1/files/00000000-0000-4000-8000-0000000000a2?t=token`,
  '/api/v1/auth/logout?t=token',
];

test('a lesson material links only to its own download address on this origin', () => {
  const accepted = materialAddress(material, origin);

  const refused = refusedAddresses.map((url) => materialAddress({ ...material, url }, origin));

  expect(accepted).toBe(`${origin}/api/v1/files/${attachment}?t=token`);
  expect(refused).toEqual(refusedAddresses.map(() => null));
});
