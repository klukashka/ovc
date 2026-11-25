import { type ReactElement, useEffect, useLayoutEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';

import { ActionButton, primaryButtonClass, quietButtonClass } from '../../components/ui/Form.tsx';
import { LiveNotice } from '../../components/ui/LiveNotice.tsx';
import { focusHeadingIfLost, focusIsLost } from '../../components/ui/LoadState.tsx';
import type { Me } from '../../lib/api/session.ts';
import type { LessonDetail } from './api.ts';
import { phaseOf, useLessonClock } from './lessonTiming.ts';
import { type RoomRefusal, offerText, refusalMessage, useTabJoin } from './roomEntry.ts';
import { roomOfferOf } from './roomState.ts';

export type JoinLesson = Pick<LessonDetail, 'id' | 'status' | 'starts_at' | 'ends_at' | 'room_open'>;

export type JoinEmphasis = 'filled' | 'quiet';

const emphasisClass: Readonly<Record<JoinEmphasis, string>> = {
  filled: primaryButtonClass,
  quiet: quietButtonClass,
};

interface JoinControlProps {
  readonly lesson: JoinLesson;
  readonly me: Me;
  readonly showsOpensAt: boolean;
  readonly emphasis?: JoinEmphasis;
}

export function JoinControl(props: JoinControlProps): ReactElement | null {
  return props.lesson.status === 'cancelled' || props.lesson.status === 'ended' ? null : <OpenJoinControl {...props} />;
}

function OpenJoinControl({ lesson, me, showsOpensAt, emphasis = 'filled' }: JoinControlProps): ReactElement | null {
  const { t } = useTranslation();
  const now = useLessonClock(lesson);
  const offer = roomOfferOf(lesson, phaseOf(lesson, now), me.role);
  const { session, join, observeLesson } = useTabJoin(lesson.id);
  const [refusal, setRefusal] = useState<RoomRefusal | null>(null);
  const [outOfScope, setOutOfScope] = useState(false);
  const button = useRef<HTMLButtonElement>(null);
  const container = useRef<HTMLDivElement>(null);

  useLayoutEffect(() => {
    const shownControl = container;
    return (): void => {
      if (shownControl.current?.contains(document.activeElement) === true) {
        requestAnimationFrame(() => {
          focusHeadingIfLost();
        });
      }
    };
  }, []);

  observeLesson(lesson);

  const shown = outOfScope ? 'none' : session === 'outside' ? offer.kind : session;
  const shownBefore = useRef(shown);
  useEffect(() => {
    const before = shownBefore.current;
    shownBefore.current = shown;
    if (before === shown || before === 'opensAt' || before === 'waitForTutor' || before === 'none') {
      return;
    }
    if (focusIsLost()) {
      if (button.current === null) {
        focusHeadingIfLost();
      } else {
        button.current.focus();
      }
    }
  }, [shown]);

  function press(): void {
    setRefusal(null);
    void join().then((refused) => {
      setRefusal(refused);
      if (refused?.kind === 'api' && refused.error.code === 'not_found') {
        setOutOfScope(true);
      }
    });
  }

  const text = outOfScope || (offer.kind === 'opensAt' && !showsOpensAt) ? null : offerText(t, offer, lesson, me);
  const refusalShown = refusal === null || (offer.kind !== 'join' && !outOfScope) ? null : refusalMessage(t, refusal);
  const showsButton = !outOfScope && (session !== 'outside' || offer.kind === 'join');
  if (!showsButton && text === null && refusalShown === null) {
    return null;
  }
  const label =
    session === 'joining'
      ? t('lesson.joining')
      : session === 'inTab'
        ? t('lesson.openAgain')
        : offer.kind === 'join'
          ? t(offer.label)
          : t('lesson.join');
  return (
    <div
      ref={container}
      className="mt-2 flex flex-col items-stretch gap-2 tablet:items-start"
      data-part="join"
      data-emphasis={emphasis}
    >
      {text === null ? null : <p className="text-small text-strong">{text}</p>}
      {session === 'inTab' ? <p className="text-small text-text-muted">{t('lesson.openInAnotherTab')}</p> : null}
      {showsButton ? (
        <ActionButton ref={button} busy={session === 'joining'} onClick={press} className={emphasisClass[emphasis]}>
          {label}
        </ActionButton>
      ) : null}
      {refusalShown === null ? null : (
        <p role="alert" className="text-small text-danger">
          {refusalShown}
        </p>
      )}
      <LiveNotice message={session === 'joining' ? t('lesson.joining') : null} />
    </div>
  );
}
