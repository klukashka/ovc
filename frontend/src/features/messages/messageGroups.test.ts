import { expect, test } from 'vitest';

import { messagePlaces } from './messageGroups.ts';

interface Sent {
  readonly own: boolean;
  readonly sender_label: string | null;
  readonly sender_label_kind: 'deleted_user' | null;
  readonly created_at: string;
  readonly sent_before_reclaim: boolean;
}

function from(sender: string | null, createdAt: string): Sent {
  return {
    own: false,
    sender_label: sender,
    sender_label_kind: null,
    created_at: createdAt,
    sent_before_reclaim: false,
  };
}

function mine(createdAt: string): Sent {
  return { own: true, sender_label: 'You', sender_label_kind: null, created_at: createdAt, sent_before_reclaim: false };
}

function groupStarts(messages: readonly Sent[], timeZone = 'Europe/Moscow'): boolean[] {
  return messagePlaces(messages, 'en', timeZone).map((place) => place.firstOfGroup);
}

test('messages of one sender within a minute of the previous one form one group', () => {
  const messages = [
    from('Anna', '2026-11-24T10:00:00Z'),
    from('Anna', '2026-11-24T10:00:59Z'),
    from('Anna', '2026-11-24T10:01:59Z'),
  ];

  const starts = groupStarts(messages);

  expect(starts).toEqual([true, false, false]);
});

test('a gap above one minute, another sender or the caller starts a new group', () => {
  const messages = [
    from('Anna', '2026-11-24T10:00:00Z'),
    from('Anna', '2026-11-24T10:01:00.001Z'),
    from('Boris', '2026-11-24T10:01:10Z'),
    mine('2026-11-24T10:01:20Z'),
    mine('2026-11-24T10:01:30Z'),
    from(null, '2026-11-24T10:01:40Z'),
    from(null, '2026-11-24T10:01:45Z'),
    { ...from(null, '2026-11-24T10:01:50Z'), sender_label_kind: 'deleted_user' as const },
    { ...from(null, '2026-11-24T10:01:55Z'), sender_label_kind: 'deleted_user' as const },
  ];

  const starts = groupStarts(messages);

  expect(starts).toEqual([true, true, true, true, false, true, true, true, true]);
});

test('a message sent before a reclaim never shares a group with one sent after it', () => {
  const messages = [
    { ...from('Anna', '2026-11-24T10:00:00Z'), sent_before_reclaim: true },
    from('Anna', '2026-11-24T10:00:10Z'),
    from('Anna', '2026-11-24T10:00:20Z'),
  ];

  const starts = groupStarts(messages);

  expect(starts).toEqual([true, true, false]);
});

test('midnight in the reader’s time zone starts a new day and a new group', () => {
  const messages = [from('Anna', '2026-11-24T20:59:40Z'), from('Anna', '2026-11-24T21:00:10Z')];

  const moscow = messagePlaces(messages, 'en', 'Europe/Moscow');
  const london = messagePlaces(messages, 'en', 'Europe/London');

  expect(moscow.map((place) => [place.firstOfDay, place.firstOfGroup])).toEqual([
    [true, true],
    [true, true],
  ]);
  expect(london.map((place) => [place.firstOfDay, place.firstOfGroup])).toEqual([
    [true, true],
    [false, false],
  ]);
});

test('the last message of each group is marked', () => {
  const messages = [mine('2026-11-24T10:00:00Z'), mine('2026-11-24T10:00:30Z'), from('Anna', '2026-11-24T10:00:40Z')];

  const ends = messagePlaces(messages, 'en', 'Europe/Moscow').map((place) => place.lastOfGroup);

  expect(ends).toEqual([false, true, true]);
});
