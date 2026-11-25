import { expect, test } from 'vitest';

import { FakeAuthServer, manualScheduler, memoryLocks } from './fakeTransport.ts';
import { Session } from './session.ts';
import { type UploadContractSubject, uploadContract } from './uploadContract.ts';

const origin = 'https://ovc.example';

async function subject(): Promise<UploadContractSubject> {
  const server = new FakeAuthServer();
  server.addAccount({ id: 'tutor', name: 'Tutor', email: 'tutor@example.org', password: 'a long password' });
  server.addAccount({ id: 'other', name: 'Other tutor', email: 'other@example.org', password: 'another password' });
  for (const owner of ['tutor', 'other']) {
    server.chat.addChannel({
      channel: {
        id: owner,
        kind: 'group',
        label: 'Group',
        label_kind: null,
        colour: null,
        tutor: null,
        last_message: null,
        unread_count: 0,
        pinned: null,
        writable: true,
        online: null,
        unconfirmed: false,
        user_id: null,
      },
      members: [{ accountId: owner }],
    });
  }
  const session = new Session({
    origin,
    transport: server.transport(),
    locks: memoryLocks(),
    scheduler: manualScheduler(),
  });
  await session.signIn('tutor@example.org', 'a long password');
  return {
    upload: (file, scope, signal, progress): Promise<Response> => {
      const body = new FormData();
      body.append('file', new File([new Uint8Array(file.bytes)], file.name, { type: file.mediaType }));
      return session.authorizedUpload(
        new Request(`${origin}/api/v1/uploads?context=chat&scope=channel:${scope === 'member' ? 'tutor' : 'other'}`, {
          method: 'POST',
          signal,
          headers: { 'Content-Type': 'application/json' },
        }),
        { body, progress },
      );
    },
  };
}

for (const [name, scenario] of Object.entries(uploadContract)) {
  test(`fake multipart transport keeps the shared contract: ${name}`, async () => {
    const adapter = await subject();

    const run = scenario(adapter);

    await expect(run).resolves.toBeUndefined();
  });
}
