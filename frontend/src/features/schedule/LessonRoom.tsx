import { useQueryClient } from '@tanstack/react-query';
import type { TFunction } from 'i18next';
import { type ReactElement, type RefObject, useCallback, useEffect, useId, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';

import { useScheduler } from '../../app/clock.ts';
import { useMediaQuery, wideLayoutQuery } from '../../app/context.ts';
import { type Action, type ActionState, done, refusedBy, useAction } from '../../components/ui/action.ts';
import { ModalDialog } from '../../components/ui/Dialog.tsx';
import {
  ActionButton,
  FormAlert,
  dangerButtonClass,
  primaryButtonClass,
  quietButtonClass,
  textButtonClass,
} from '../../components/ui/Form.tsx';
import { LiveNotice } from '../../components/ui/LiveNotice.tsx';
import { focusHeadingIfLost, focusIsLost } from '../../components/ui/LoadState.tsx';
import { refusalText } from '../../components/ui/Refusal.tsx';
import { type ApiError, apiErrorFrom, asApiError, useApiClient } from '../../lib/api/client.ts';
import type { Me } from '../../lib/api/session.ts';
import type { LessonDetail } from './api.ts';
import { formatClock } from './calendarFormat.ts';
import { calendarKey, lessonKey } from './keys.ts';
import { replaceLessonDetail } from './lessonCache.ts';
import { roomOpensAt, type LessonPhase } from './lessonTiming.ts';
import { type RoomOffer, type RoomSession, isRoomAddress, roomOfferOf, sessionAfterLessonChange } from './roomState.ts';
import { type RoomTab, useOpenRoomTab } from './roomTab.ts';

export const embedLoadHintMilliseconds = 15_000;

type JoinPath = '/api/v1/lessons/{id}/join' | '/api/v1/lessons/{id}/restart-room';

type EntryOutcome =
  | { readonly kind: 'entered' }
  | { readonly kind: 'abandoned' }
  | { readonly kind: 'popupRefused' }
  | { readonly kind: 'embedUnavailable' }
  | { readonly kind: 'refused'; readonly state: Extract<ActionState, { readonly kind: 'refused' }> };

type RoomRefusal =
  | { readonly kind: 'popup' }
  | { readonly kind: 'unavailable' }
  | { readonly kind: 'api'; readonly error: ApiError; readonly retryAfter: number | null };

interface Attempt {
  readonly tab: RoomTab | null;
}

function refusalMessage(t: TFunction, refusal: RoomRefusal): string | null {
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

function refusalOf(outcome: EntryOutcome): RoomRefusal | null {
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

function useRoomEntry(lessonId: string): {
  readonly session: RoomSession;
  readonly leaveRoom: () => void;
  readonly enter: (path: JoinPath, viaTab: boolean) => Promise<EntryOutcome>;
  readonly observeLesson: (lesson: LessonDetail) => void;
} {
  const client = useApiClient();
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
    const answered = await client
      .POST(path, { params: { path: { id: lessonId } } })
      .then((answer) => ({ answer, thrown: null }))
      .catch((thrown: unknown) => ({ answer: null, thrown }));
    if (attempt.current !== current) {
      tab?.close();
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

  const observeLesson = (lesson: LessonDetail): void => {
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

function offerText(t: TFunction, offer: RoomOffer, lesson: LessonDetail, me: Me): string | null {
  if (offer.kind === 'opensAt') {
    return t('lesson.roomOpensAt', {
      time: formatClock(new Date(roomOpensAt(lesson)).toISOString(), me.language, me.timezone),
    });
  }
  return offer.kind === 'waitForTutor' ? t('lesson.roomOpensWithTutor') : null;
}

function EmbeddedRoom({
  url,
  title,
  acceptsEscape,
  onOpenInNewTab,
}: {
  readonly url: string;
  readonly title: string;
  readonly acceptsEscape: boolean;
  readonly onOpenInNewTab: () => void;
}): ReactElement {
  const { t } = useTranslation();
  const scheduler = useScheduler();
  const frame = useRef<HTMLIFrameElement>(null);
  const area = useRef<HTMLDivElement>(null);
  const [focusMode, setFocusMode] = useState(false);
  const [loaded, setLoaded] = useState(false);
  const [slow, setSlow] = useState(false);

  useEffect(() => {
    frame.current?.focus();
  }, []);

  useEffect(() => {
    if (loaded) {
      return undefined;
    }
    return scheduler.schedule(embedLoadHintMilliseconds, () => {
      setSlow(true);
    });
  }, [loaded, scheduler]);

  useEffect(() => {
    if (!focusMode) {
      return undefined;
    }
    area.current?.scrollIntoView({ block: 'start' });
    if (!acceptsEscape) {
      return undefined;
    }
    const onKeyDown = (event: KeyboardEvent): void => {
      if (event.key === 'Escape') {
        setFocusMode(false);
      }
    };
    document.addEventListener('keydown', onKeyDown);
    return (): void => {
      document.removeEventListener('keydown', onKeyDown);
    };
  }, [focusMode, acceptsEscape]);

  return (
    <div
      ref={area}
      data-part="room"
      data-focus-mode={focusMode ? '' : undefined}
      className={focusMode ? 'flex h-dvh flex-col gap-2' : 'flex flex-col gap-2'}
    >
      <div className="flex flex-wrap items-center gap-3">
        <button
          type="button"
          onClick={(event) => {
            event.currentTarget.focus();
            setFocusMode((current) => !current);
          }}
          className={quietButtonClass}
        >
          {t(focusMode ? 'lesson.exitFocus' : 'lesson.focusMode')}
        </button>
        <button
          type="button"
          onClick={onOpenInNewTab}
          className={slow && !loaded ? primaryButtonClass : quietButtonClass}
        >
          {t(slow && !loaded ? 'lesson.embedTrouble' : 'lesson.openInNewTab')}
        </button>
      </div>
      <iframe
        ref={frame}
        src={url}
        title={title}
        allow="camera; microphone; display-capture; fullscreen; autoplay"
        referrerPolicy="no-referrer"
        onLoad={() => {
          setLoaded(true);
        }}
        className={
          focusMode
            ? 'min-h-0 w-full flex-1 rounded-md border border-border'
            : 'aspect-video w-full rounded-md border border-border'
        }
      />
    </div>
  );
}

interface ConfirmTexts {
  readonly title: string;
  readonly consequence: string;
  readonly confirm: string;
}

function RoomDialog({
  open,
  texts,
  action,
  returnFocus,
  onConfirm,
  onDismiss,
  refusal,
}: {
  readonly open: boolean;
  readonly texts: ConfirmTexts;
  readonly action: Action;
  readonly returnFocus: RefObject<HTMLElement | null>;
  readonly onConfirm: () => void;
  readonly onDismiss: () => void;
  readonly refusal: string | null;
}): ReactElement {
  const { t } = useTranslation();
  const headingId = useId();
  const consequenceId = useId();
  const dismissButton = useRef<HTMLButtonElement>(null);
  return (
    <ModalDialog
      open={open}
      busy={action.busy}
      labelledBy={headingId}
      describedBy={consequenceId}
      returnFocus={returnFocus}
      initialFocus={dismissButton}
      onClose={onDismiss}
    >
      {open ? (
        <div className="flex flex-col gap-4">
          <h2 id={headingId} className="font-serif text-title">
            {texts.title}
          </h2>
          <p id={consequenceId}>{texts.consequence}</p>
          {refusal === null ? null : <FormAlert>{refusal}</FormAlert>}
          <div className="flex flex-wrap gap-3">
            <ActionButton busy={action.busy} onClick={onConfirm} className={dangerButtonClass}>
              {texts.confirm}
            </ActionButton>
            <ActionButton
              ref={dismissButton}
              busy={false}
              unavailable={action.busy}
              onClick={onDismiss}
              className={textButtonClass}
            >
              {t('newLesson.cancel')}
            </ActionButton>
          </div>
        </div>
      ) : null}
    </ModalDialog>
  );
}

type RoomDialogKind = 'end' | 'restart';

function isLessonChange(error: ApiError): boolean {
  return error.code === 'conflict' || error.code === 'lesson_not_live' || error.code === 'not_found';
}

export function LessonRoom({
  lesson,
  me,
  phase,
  onChangedMeanwhile,
  onEnded,
}: {
  readonly lesson: LessonDetail;
  readonly me: Me;
  readonly phase: LessonPhase;
  readonly onChangedMeanwhile: () => void;
  readonly onEnded: () => void;
}): ReactElement | null {
  const { t } = useTranslation();
  const client = useApiClient();
  const queryClient = useQueryClient();
  const wide = useMediaQuery(wideLayoutQuery);
  const { session, enter, observeLesson, leaveRoom } = useRoomEntry(lesson.id);
  const [refusal, setRefusal] = useState<RoomRefusal | null>(null);
  const [outOfScope, setOutOfScope] = useState(false);
  const [dialog, setDialog] = useState<RoomDialogKind | null>(null);
  const [dialogRefusal, setDialogRefusal] = useState<string | null>(null);
  const action = useAction();
  const mainButton = useRef<HTMLButtonElement>(null);
  const endButton = useRef<HTMLButtonElement>(null);
  const restartButton = useRef<HTMLButtonElement>(null);
  const offer = roomOfferOf(lesson, phase, me.role);
  const tutorWrites = me.role === 'tutor' && me.state === 'active';
  const live = lesson.status === 'live';
  const viaTab = !lesson.embed_allowed || !wide;

  observeLesson(lesson);

  const [outdatedDialogs, setOutdatedDialogs] = useState(0);
  if (dialog !== null && !action.busy && !live) {
    setDialog(null);
    setOutdatedDialogs(outdatedDialogs + 1);
  }
  useEffect(() => {
    if (outdatedDialogs > 0) {
      onChangedMeanwhile();
    }
  }, [outdatedDialogs, onChangedMeanwhile]);

  const shown = outOfScope ? 'none' : session.kind === 'outside' ? offer.kind : session.kind;
  const shownBefore = useRef(shown);
  useEffect(() => {
    const before = shownBefore.current;
    shownBefore.current = shown;
    if (before === shown || before === 'opensAt' || before === 'waitForTutor' || before === 'none') {
      return;
    }
    if (focusIsLost()) {
      if (mainButton.current === null) {
        focusHeadingIfLost();
      } else {
        mainButton.current.focus();
      }
    }
  }, [shown]);

  function join(tabOnly: boolean): void {
    setRefusal(null);
    void enter('/api/v1/lessons/{id}/join', tabOnly || viaTab).then((outcome) => {
      const refused = refusalOf(outcome);
      setRefusal(refused);
      if (refused?.kind === 'api' && refused.error.code === 'not_found') {
        setOutOfScope(true);
      }
    });
  }

  function dismissDialog(): void {
    setDialog(null);
    setDialogRefusal(null);
    action.reset();
  }

  function endLesson(): void {
    action.run(async (): Promise<ActionState> => {
      setDialogRefusal(null);
      const answered = await client
        .POST('/api/v1/lessons/{id}/end', { params: { path: { id: lesson.id } } })
        .then((answer) => ({ answer, thrown: null }))
        .catch((thrown: unknown) => ({ answer: null, thrown }));
      if (answered.answer === null) {
        const failure = asApiError(answered.thrown);
        setDialogRefusal(refusalText(t, failure, null, undefined));
        return { kind: 'refused', error: failure, retryAfter: null };
      }
      const { data, error, response } = answered.answer;
      if (data === undefined) {
        const refused = refusedBy(response, error);
        if (refused.kind === 'refused' && isLessonChange(refused.error)) {
          setDialog(null);
          onChangedMeanwhile();
          void queryClient.invalidateQueries({ queryKey: lessonKey(lesson.id) });
          return refused;
        }
        if (refused.kind === 'refused') {
          setDialogRefusal(refusalText(t, refused.error, refused.retryAfter, undefined));
        }
        return refused;
      }
      await replaceLessonDetail(queryClient, data);
      void queryClient.invalidateQueries({ queryKey: calendarKey });
      leaveRoom();
      setDialog(null);
      onEnded();
      return done;
    });
  }

  function restartRoom(): void {
    action.run(async (): Promise<ActionState> => {
      setDialogRefusal(null);
      setRefusal(null);
      const outcome = await enter('/api/v1/lessons/{id}/restart-room', viaTab);
      const refused = refusalOf(outcome);
      if (refused === null) {
        setDialog(null);
        return done;
      }
      if (refused.kind === 'api' && isLessonChange(refused.error)) {
        setDialog(null);
        onChangedMeanwhile();
        return { kind: 'refused', error: refused.error, retryAfter: null };
      }
      setDialogRefusal(refusalMessage(t, refused));
      return done;
    });
  }

  const text = outOfScope ? null : offerText(t, offer, lesson, me);
  const refusalShown = refusal === null || (offer.kind !== 'join' && !outOfScope) ? null : refusalMessage(t, refusal);
  const showsButton = !outOfScope && (session.kind === 'joining' || session.kind === 'inTab' || offer.kind === 'join');
  const manages = tutorWrites && live;
  if (session.kind !== 'embedded' && !showsButton && text === null && refusalShown === null && !manages) {
    return null;
  }
  const inTab = session.kind === 'inTab';
  const buttonLabel =
    session.kind === 'joining'
      ? t('lesson.joining')
      : inTab
        ? t('lesson.openAgain')
        : offer.kind === 'join'
          ? t(offer.label)
          : t('lesson.join');
  return (
    <section aria-label={t('lesson.room')} className="flex flex-col gap-3" data-part="lesson-room">
      {session.kind === 'embedded' ? (
        <>
          <EmbeddedRoom
            key={session.entry}
            url={session.url}
            title={lesson.title}
            acceptsEscape={dialog === null}
            onOpenInNewTab={() => {
              join(true);
            }}
          />
          {refusalShown === null ? null : (
            <p role="alert" className="text-danger">
              {refusalShown}
            </p>
          )}
        </>
      ) : (
        <div className="flex flex-col items-start gap-3 rounded-lg border border-border bg-surface p-4 dark:border-border-strong">
          {text === null ? null : <p className="text-strong">{text}</p>}
          {inTab ? <p className="text-strong">{t('lesson.openInAnotherTab')}</p> : null}
          {showsButton ? (
            <ActionButton
              ref={mainButton}
              busy={session.kind === 'joining'}
              onClick={() => {
                join(inTab);
              }}
              className={primaryButtonClass}
            >
              {buttonLabel}
            </ActionButton>
          ) : null}
          {refusalShown === null ? null : (
            <p role="alert" className="text-danger">
              {refusalShown}
            </p>
          )}
        </div>
      )}
      <LiveNotice message={session.kind === 'joining' ? t('lesson.joining') : null} />
      {manages ? (
        <div className="flex flex-wrap gap-3">
          <ActionButton
            ref={restartButton}
            busy={false}
            unavailable={session.kind === 'joining'}
            onClick={() => {
              setDialogRefusal(null);
              setDialog('restart');
            }}
            className={quietButtonClass}
          >
            {t('lesson.restartRoom')}
          </ActionButton>
          <ActionButton
            ref={endButton}
            busy={false}
            unavailable={session.kind === 'joining'}
            onClick={() => {
              setDialogRefusal(null);
              setDialog('end');
            }}
            className={dangerButtonClass}
          >
            {t('lesson.end')}
          </ActionButton>
        </div>
      ) : null}
      {me.role === 'tutor' ? (
        <>
          <RoomDialog
            open={dialog === 'end'}
            texts={{
              title: t('lessonEnd.title'),
              consequence: t('lessonEnd.consequence'),
              confirm: t('lessonEnd.confirm'),
            }}
            action={action}
            returnFocus={endButton}
            onConfirm={endLesson}
            onDismiss={dismissDialog}
            refusal={dialogRefusal}
          />
          <RoomDialog
            open={dialog === 'restart'}
            texts={{
              title: t('lessonRestart.title'),
              consequence: t('lessonRestart.consequence'),
              confirm: t('lessonRestart.confirm'),
            }}
            action={action}
            returnFocus={restartButton}
            onConfirm={restartRoom}
            onDismiss={dismissDialog}
            refusal={dialogRefusal}
          />
        </>
      ) : null}
    </section>
  );
}
