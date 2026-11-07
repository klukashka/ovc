import type { Me } from '../../lib/api/session.ts';
import type { LessonDetail } from './api.ts';
import type { LessonPhase } from './lessonTiming.ts';

export type JoinLabel = 'lesson.join' | 'lesson.joinStudent' | 'lesson.rejoin';

export type RoomOffer =
  | { readonly kind: 'none' }
  | { readonly kind: 'opensAt' }
  | { readonly kind: 'waitForTutor' }
  | { readonly kind: 'join'; readonly label: JoinLabel };

type RoomLesson = Pick<LessonDetail, 'status' | 'room_open'>;

function tutorOffer(phase: LessonPhase): RoomOffer {
  switch (phase) {
    case 'before':
      return { kind: 'opensAt' };
    case 'window':
      return { kind: 'join', label: 'lesson.join' };
    case 'live':
      return { kind: 'join', label: 'lesson.rejoin' };
    case 'past':
    case 'ended':
    case 'cancelled':
      return { kind: 'none' };
  }
}

function studentOffer(phase: LessonPhase, roomOpen: boolean): RoomOffer {
  switch (phase) {
    case 'before':
      return { kind: 'opensAt' };
    case 'window':
      return { kind: 'waitForTutor' };
    case 'live':
      return roomOpen ? { kind: 'join', label: 'lesson.joinStudent' } : { kind: 'waitForTutor' };
    case 'past':
    case 'ended':
    case 'cancelled':
      return { kind: 'none' };
  }
}

export function roomOfferOf(lesson: RoomLesson, phase: LessonPhase, role: Me['role']): RoomOffer {
  return role === 'tutor' ? tutorOffer(phase) : studentOffer(phase, lesson.room_open);
}

export type RoomSession =
  | { readonly kind: 'outside' }
  | { readonly kind: 'joining' }
  | { readonly kind: 'inTab'; readonly roomSeenOpen: boolean }
  | {
      readonly kind: 'embedded';
      readonly url: string;
      readonly entry: number;
      readonly roomSeenOpen: boolean;
    };

type EnteredRoom = Extract<RoomSession, { readonly kind: 'inTab' | 'embedded' }>;

export function sessionAfterLessonChange(session: RoomSession, lesson: RoomLesson): RoomSession {
  if (session.kind !== 'inTab' && session.kind !== 'embedded') {
    return session;
  }
  const entered: EnteredRoom = session;
  if (lesson.status === 'ended' || lesson.status === 'cancelled') {
    return { kind: 'outside' };
  }
  if (lesson.room_open) {
    return entered.roomSeenOpen ? entered : { ...entered, roomSeenOpen: true };
  }
  return entered.roomSeenOpen ? { kind: 'outside' } : entered;
}

export function isRoomAddress(url: string): boolean {
  if (!URL.canParse(url)) {
    return false;
  }
  const { protocol } = new URL(url);
  return protocol === 'https:' || protocol === 'http:';
}
