import { act, cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, expect, test } from 'vitest';

import { SchedulerContext } from '../../app/clock.ts';
import { manualScheduler } from '../../lib/api/fakeTransport.ts';
import { Attachments, useAttachmentExpiry } from './Attachments.tsx';
import type { Message } from './api.ts';
import { chatTexts } from './texts.en.ts';

const message: Message = {
  id: 'message',
  seq: 1,
  body: '',
  own: true,
  sender_label: 'You',
  sender_label_kind: null,
  created_at: '2026-10-31T10:00:00Z',
  edited_at: null,
  version: 1,
  deleted: false,
  erased: false,
  sent_before_reclaim: false,
  attachments: [
    {
      id: 'picture',
      filename: 'picture.png',
      media_type: 'image/png',
      size_bytes: 20,
      width: 50,
      height: 40,
      url: '/api/v1/files/picture?t=token',
      expires_at: new Date(60_000).toISOString(),
    },
  ],
};
afterEach(cleanup);

test('a picture that fails to load fetches fresh addresses once even when refetch returns the same address', async () => {
  let refreshes = 0;
  const refresh = (): Promise<boolean> => {
    refreshes += 1;
    return Promise.resolve(true);
  };
  const view = render(<Attachments message={message} texts={chatTexts} refresh={refresh} />);

  await act(async () => {
    fireEvent.error(screen.getByRole('img'));
    await Promise.resolve();
  });
  fireEvent.error(screen.getByRole('img'));
  view.rerender(
    <Attachments
      message={{ ...message, attachments: message.attachments.map((file) => ({ ...file, url: `${file.url}2` })) }}
      texts={chatTexts}
      refresh={refresh}
    />,
  );

  expect(refreshes).toBe(1);
  expect(screen.queryByRole('img')).toBeNull();
  expect(screen.getByText(chatTexts.pictureFailed)).toBeTruthy();
});

test('identical expired addresses do not create an immediate refresh loop and unmount cancels deadlines', async () => {
  const clock = manualScheduler();
  let refreshes = 0;
  function View(): null {
    useAttachmentExpiry(
      [message],
      () => {
        refreshes += 1;
        return Promise.resolve(true);
      },
      () => true,
    );
    return null;
  }
  const view = render(
    <SchedulerContext value={clock}>
      <View />
    </SchedulerContext>,
  );

  await act(async () => {
    clock.pass(30_000);
    await Promise.resolve();
  });
  view.rerender(
    <SchedulerContext value={clock}>
      <View />
    </SchedulerContext>,
  );
  await act(async () => {
    clock.pass(600_000);
    await Promise.resolve();
  });
  view.unmount();
  clock.pass(600_000);

  expect(refreshes).toBe(1);
});

test('signed addresses never load from another origin or an unrelated same-origin route', () => {
  const files = message.attachments.flatMap((file) => [
    { ...file, id: 'external', url: 'https://outside.example/api/v1/files/external?t=secret' },
    { ...file, id: 'redirect', url: '/api/v1/auth/logout?t=secret' },
  ]);

  render(
    <Attachments
      message={{ ...message, attachments: files }}
      texts={chatTexts}
      refresh={() => Promise.resolve(true)}
    />,
  );

  expect(screen.queryByRole('img')).toBeNull();
  expect(screen.queryByRole('link')).toBeNull();
  expect(screen.getAllByText(chatTexts.fileUnavailable)).toHaveLength(2);
});
