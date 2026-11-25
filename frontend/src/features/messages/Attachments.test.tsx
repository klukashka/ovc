import { act, cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, expect, test } from 'vitest';

import { SchedulerContext } from '../../app/clock.ts';
import { manualScheduler } from '../../lib/api/fakeTransport.ts';
import { Attachments, albumLayout, attachmentAddress, useAttachmentExpiry } from './Attachments.tsx';
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
  reply_to: null,
  reactions: [],
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
  const view = render(<Attachments message={message} texts={chatTexts} language="en" refresh={refresh} />);

  await act(async () => {
    fireEvent.error(screen.getByRole('img'));
    await Promise.resolve();
  });
  fireEvent.error(screen.getByRole('img'));
  view.rerender(
    <Attachments
      message={{ ...message, attachments: message.attachments.map((file) => ({ ...file, url: `${file.url}2` })) }}
      texts={chatTexts}
      language="en"
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
      language="en"
      refresh={() => Promise.resolve(true)}
    />,
  );

  expect(screen.queryByRole('img')).toBeNull();
  expect(screen.queryByRole('link')).toBeNull();
  expect(screen.getAllByText(chatTexts.fileUnavailable)).toHaveLength(2);
});

function picture(id: string): Message['attachments'][number] {
  return {
    id,
    filename: `${id}.png`,
    media_type: 'image/png',
    size_bytes: 20,
    width: 40,
    height: 30,
    url: `/api/v1/files/${id}?t=token`,
    expires_at: new Date(60_000).toISOString(),
  };
}

function document(id: string, filename: string, size: number): Message['attachments'][number] {
  return {
    id,
    filename,
    media_type: 'application/pdf',
    size_bytes: size,
    width: null,
    height: null,
    url: `/api/v1/files/${id}?t=token`,
    expires_at: new Date(60_000).toISOString(),
  };
}

function renderAttachments(files: Message['attachments']): void {
  render(
    <Attachments
      message={{ ...message, attachments: files }}
      texts={chatTexts}
      language="en"
      refresh={() => Promise.resolve(true)}
    />,
  );
}

test('pictures of one message form one album whose cells reserve their shape and keep the picture dimensions', () => {
  const pictures = [picture('first'), picture('second'), picture('third')];

  renderAttachments(pictures);

  const album = screen.getByRole('group', { name: 'Album, 3 pictures' });
  const images = Array.from(album.querySelectorAll('img'));
  expect(images).toHaveLength(3);
  for (const [index, image] of images.entries()) {
    const file = pictures[index];
    expect(image.getAttribute('referrerpolicy')).toBe('no-referrer');
    expect(image.getAttribute('src')).toBe(file === undefined ? null : attachmentAddress(file, window.location.origin));
    expect(image.getAttribute('alt')).toBe(file?.filename);
    expect(image.getAttribute('width')).toBe('40');
    expect(image.getAttribute('height')).toBe('30');
    expect(image.parentElement?.style.aspectRatio).toBe(index === 0 ? '2 / 1' : '1 / 1');
  }
});

test('album columns put the leftover pictures into a wider first cell', () => {
  const layouts = [2, 3, 4, 5, 7, 10].map((count) => albumLayout(count));

  expect(layouts).toEqual([
    { columns: 2, leadSpan: 1 },
    { columns: 2, leadSpan: 2 },
    { columns: 2, leadSpan: 1 },
    { columns: 3, leadSpan: 2 },
    { columns: 3, leadSpan: 3 },
    { columns: 3, leadSpan: 3 },
  ]);
});

test('a file card links its exact name to its own address without a new tab and shows its size and type', () => {
  const notesFile = document('notes', 'lesson notes.pdf', 2_400_000);

  renderAttachments([notesFile, document('odd', 'x.<svg>', 830)]);

  const notes = screen.getByRole('link', { name: 'lesson notes.pdf' });
  expect(notes.getAttribute('href')).toBe(attachmentAddress(notesFile, window.location.origin));
  expect(notes.getAttribute('referrerpolicy')).toBe('no-referrer');
  expect(notes.getAttribute('target')).toBeNull();
  expect(notes.getAttribute('dir')).toBe('auto');
  expect(screen.getByText('2.4 MB')).toBeTruthy();
  expect(screen.getByText('PDF')).toBeTruthy();
  expect(screen.getByRole('link', { name: 'x.<svg>' })).toBeTruthy();
  expect(screen.getByText('830 byte')).toBeTruthy();
  expect(screen.queryByText('<SVG>')).toBeNull();
});

test('a file card with a refused address offers no link', () => {
  const refused = {
    ...document('notes', 'lesson notes.pdf', 10),
    url: 'https://outside.example/api/v1/files/notes?t=x',
  };

  renderAttachments([refused]);

  expect(screen.queryByRole('link')).toBeNull();
  expect(screen.getByText(chatTexts.fileUnavailable)).toBeTruthy();
});
