import { describe, expect, test } from 'vitest';

import type { LessonPhase } from './lessonTiming.ts';
import { type RoomOffer, type RoomSession, isRoomAddress, roomOfferOf, sessionAfterLessonChange } from './roomState.ts';

const offers: readonly (readonly [LessonPhase, boolean, RoomOffer, RoomOffer])[] = [
  ['before', false, { kind: 'opensAt' }, { kind: 'opensAt' }],
  ['window', false, { kind: 'join', label: 'lesson.join' }, { kind: 'waitForTutor' }],
  ['live', true, { kind: 'join', label: 'lesson.rejoin' }, { kind: 'join', label: 'lesson.joinStudent' }],
  ['live', false, { kind: 'join', label: 'lesson.rejoin' }, { kind: 'waitForTutor' }],
  ['past', false, { kind: 'none' }, { kind: 'none' }],
  ['ended', false, { kind: 'none' }, { kind: 'none' }],
  ['cancelled', false, { kind: 'none' }, { kind: 'none' }],
];

describe('the room offer of each lesson state', () => {
  test.each(offers)('%s with room open %s', (phase, roomOpen, forTutor, forStudent) => {
    const lesson = { status: 'live', room_open: roomOpen } as const;

    const tutor = roomOfferOf(lesson, phase, 'tutor');
    const student = roomOfferOf(lesson, phase, 'student');

    expect(tutor).toEqual(forTutor);
    expect(student).toEqual(forStudent);
  });
});

describe('the room session after a lesson change', () => {
  const unseen: RoomSession = { kind: 'inTab', roomSeenOpen: false };
  const seen: RoomSession = { kind: 'inTab', roomSeenOpen: true };

  test('a first join keeps the session until the lesson data catches up with the opened room', () => {
    const lesson = { status: 'scheduled', room_open: false } as const;

    const session = sessionAfterLessonChange(unseen, lesson);

    expect(session).toBe(unseen);
  });

  test('seeing the room open arms the session, and seeing it closed afterwards leaves the room', () => {
    const open = { status: 'live', room_open: true } as const;
    const closed = { status: 'live', room_open: false } as const;

    const armed = sessionAfterLessonChange(unseen, open);
    const left = sessionAfterLessonChange(armed, closed);

    expect(armed).toEqual(seen);
    expect(left).toEqual({ kind: 'outside' });
  });

  test.each(['ended', 'cancelled'] as const)('a lesson %s leaves the room at once', (status) => {
    const lesson = { status, room_open: false };

    const session = sessionAfterLessonChange(unseen, lesson);

    expect(session).toEqual({ kind: 'outside' });
  });
});

describe('the room address', () => {
  test.each([
    ['https://room.example/join', true],
    ['http://127.0.0.1:8000/__test__/fake-room', true],
    ['http://[::1]:8000/__test__/fake-room', true],
    ['https://127.0.0.1:8443/__test__/fake-room', true],
    ['http://room.example/join', false],
    ['http://localhost:8000/__test__/fake-room', false],
    ['http://127.0.0.1.room.example/join', false],
    ['javascript:alert(1)', false],
    ['data:text/html,x', false],
    ['/relative', false],
  ])('%s', (address, accepted) => {
    const result = isRoomAddress(address);

    expect(result).toBe(accepted);
  });
});
