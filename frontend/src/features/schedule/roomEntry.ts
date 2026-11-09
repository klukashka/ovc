import { useQueryClient } from '@tanstack/react-query';
import type { TFunction } from 'i18next';
import { useCallback, useEffect, useRef, useState } from 'react';

import { type ActionState, refusedBy } from '../../components/ui/action.ts';
import { refusalText } from '../../components/ui/Refusal.tsx';
import { type ApiError, apiErrorFrom, asApiError, useApiClient } from '../../lib/api/client.ts';
import { type Me, useSession } from '../../lib/api/session.ts';
import type { LessonDetail } from './api.ts';
import { formatClock } from './calendarFormat.ts';
import { calendarKey, lessonKey } from './keys.ts';
import { type LessonTimes, roomOpensAt } from './lessonTiming.ts';
import { type RoomOffer, type RoomSession, isRoomAddress, sessionAfterLessonChange } from './roomState.ts';
import { type RoomTab, useOpenRoomTab } from './roomTab.ts';

export type JoinPath = '/api/v1/lessons/{id}/join' | '/api/v1/lessons/{id}/restart-room';

export type EntryOutcome =
  | { readonly kind: 'entered' }
  | { readonly kind: 'abandoned' }
  | { readonly kind: 'popupRefused' }
  | { readonly kind: 'embedUnavailable' }
  | { readonly kind: 'refused'; readonly state: Extract<ActionState, { readonly kind: 'refused' }> };

export type RoomRefusal =
  | { readonly kind: 'popup' }
  | { readonly kind: 'unavailable' }
  | { readonly kind: 'api'; readonly error: ApiError; readonly retryAfter: number | null };

interface Attempt {
  readonly tab: RoomTab | null;
}

export function refusalMessage(t: TFunction, refusal: RoomRefusal): string | null {
  if (refusal.kind === 'popup') {
    return t('lesson.popupRefused');
  }
  if (refusal.kind === 'unavailable') {
    return t('lesson.joinUnavailable');
  }
  switch (refusal.error.code) {
    case 'provider_unavailable':
      return t('lesson.joinUnavailable');
    case 'not_found':
      return t('lesson.cannotJoin');
    case 'lesson_not_live':
      return null;
    default:
      return refusalText(t, refusal.error, refusal.retryAfter, undefined);
  }
}

export function refusalOf(outcome: EntryOutcome): RoomRefusal | null {
  switch (outcome.kind) {
    case 'entered':
    case 'abandoned':
      return null;
    case 'popupRefused':
      return { kind: 'popup' };
    case 'embedUnavailable':
      return { kind: 'unavailable' };
    case 'refused':
      return { kind: 'api', error: outcome.state.error, retryAfter: outcome.state.retryAfter };
  }
}

export function useRoomEntry(lessonId: string): {
  readonly session: RoomSession;
  readonly leaveRoom: () => void;
  readonly enter: (path: JoinPath, viaTab: boolean) => Promise<EntryOutcome>;
  readonly observeLesson: (lesson: Pick<LessonDetail, 'status' | 'room_open'>) => void;
} {
  const client = useApiClient();
  const signIn = useSession();
  const queryClient = useQueryClient();
  const openTab = useOpenRoomTab();
  const [session, setSession] = useState<RoomSession>({ kind: 'outside' });
  const attempt = useRef<Attempt | null>(null);
  const entries = useRef(0);

  const abandon = useCallback((): void => {
    attempt.current?.tab?.close();
    attempt.current = null;
  }, []);

  useEffect(() => {
    const onPageHide = (): void => {
      abandon();
      setSession({ kind: 'outside' });
    };
    window.addEventListener('pagehide', onPageHide);
    return (): void => {
      window.removeEventListener('pagehide', onPageHide);
      abandon();
    };
  }, [abandon]);

  const enter = async (path: JoinPath, viaTab: boolean): Promise<EntryOutcome> => {
    if (attempt.current !== null) {
      return { kind: 'abandoned' };
    }
    const tab = viaTab ? openTab() : null;
    if (viaTab && tab === null) {
      return { kind: 'popupRefused' };
    }
    const current: Attempt = { tab };
    attempt.current = current;
    setSession({ kind: 'joining' });
    const epoch = signIn.currentEpoch();
    const answered = await client
      .POST(path, { params: { path: { id: lessonId } } })
      .then((answer) => ({ answer, thrown: null }))
      .catch((thrown: unknown) => ({ answer: null, thrown }));
    if (attempt.current !== current) {
      tab?.close();
      return { kind: 'abandoned' };
    }
    if (signIn.currentEpoch() !== epoch) {
      attempt.current = null;
      tab?.close();
      setSession({ kind: 'outside' });
      return { kind: 'abandoned' };
    }
    attempt.current = null;
    void queryClient.invalidateQueries({ queryKey: lessonKey(lessonId) });
    void queryClient.invalidateQueries({ queryKey: calendarKey });
    const fail = (outcome: EntryOutcome): EntryOutcome => {
      tab?.close();
      setSession({ kind: 'outside' });
      return outcome;
    };
    if (answered.answer === null) {
      return fail({
        kind: 'refused',
        state: { kind: 'refused', error: asApiError(answered.thrown), retryAfter: null },
      });
    }
    const { data, error, response } = answered.answer;
    if (data === undefined) {
      const refused = refusedBy(response, error);
      return fail(
        refused.kind === 'refused'
          ? { kind: 'refused', state: refused }
          : {
              kind: 'refused',
              state: { kind: 'refused', error: apiErrorFrom(response.status, error), retryAfter: null },
            },
      );
    }
    if (!isRoomAddress(data.url)) {
      return fail({ kind: 'embedUnavailable' });
    }
    if (tab !== null) {
      if (!tab.navigate(data.url)) {
        return fail({ kind: 'abandoned' });
      }
      setSession({ kind: 'inTab', roomSeenOpen: false });
      return { kind: 'entered' };
    }
    if (!data.embed_allowed) {
      return fail({ kind: 'embedUnavailable' });
    }
    entries.current += 1;
    setSession({ kind: 'embedded', url: data.url, entry: entries.current, roomSeenOpen: false });
    return { kind: 'entered' };
  };

  const observeLesson = (lesson: Pick<LessonDetail, 'status' | 'room_open'>): void => {
    const next = sessionAfterLessonChange(session, lesson);
    if (next !== session) {
      setSession(next);
    }
  };

  return {
    session,
    enter,
    observeLesson,
    leaveRoom: (): void => {
      abandon();
      setSession({ kind: 'outside' });
    },
  };
}

export function offerText(t: TFunction, offer: RoomOffer, lesson: LessonTimes, me: Me): string | null {
  if (offer.kind === 'opensAt') {
    return t('lesson.roomOpensAt', {
      time: formatClock(new Date(roomOpensAt(lesson)).toISOString(), me.language, me.timezone),
    });
  }
  return offer.kind === 'waitForTutor' ? t('lesson.roomOpensWithTutor') : null;
}

export type TabSession = 'outside' | 'joining' | 'inTab';

function tabSessionOf(session: RoomSession): TabSession {
  if (session.kind === 'embedded') {
    throw new Error('a join through a new tab never embeds the room');
  }
  return session.kind;
}

export function useTabJoin(lessonId: string): {
  readonly session: TabSession;
  readonly join: () => Promise<RoomRefusal | null>;
  readonly observeLesson: (lesson: Pick<LessonDetail, 'status' | 'room_open'>) => void;
} {
  const { session, enter, observeLesson } = useRoomEntry(lessonId);
  return {
    session: tabSessionOf(session),
    observeLesson,
    join: async (): Promise<RoomRefusal | null> => refusalOf(await enter('/api/v1/lessons/{id}/join', true)),
  };
}
