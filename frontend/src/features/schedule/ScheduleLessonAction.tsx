import { type ReactElement, type ReactNode, type RefObject, useEffect, useRef } from 'react';
import { useTranslation } from 'react-i18next';

import { primaryButtonClass, quietButtonClass } from '../../components/ui/Form.tsx';
import { useFormResult, useOpenFormRoute, useReturnFocus } from '../../components/ui/formRoute.ts';
import { LiveNotice } from '../../components/ui/LiveNotice.tsx';
import { focusIsLost } from '../../components/ui/LoadState.tsx';
import { useSignedInAccount } from '../../lib/api/session.ts';
import type { Audience } from './AudiencePicker.tsx';
import { type LessonPrefill, newLessonAddress, nextFullHour } from './newLesson.ts';

export function useLessonScheduled(onScheduled?: () => void): string | null {
  const { t } = useTranslation();
  const result = useFormResult('lessonScheduled');
  const reported = useRef(onScheduled);

  useEffect(() => {
    if (result !== null) {
      reported.current?.();
    }
  }, [result]);

  return result === null ? null : t('schedule.scheduled');
}

function useOpenNewLesson(audience: Audience, opener: string, button: RefObject<HTMLButtonElement | null>): () => void {
  const me = useSignedInAccount();
  const open = useOpenFormRoute(me.id);
  useReturnFocus(opener, button);
  return () => {
    const prefill: LessonPrefill = { ...nextFullHour(Date.now(), me.timezone), audience };
    open(newLessonAddress(), opener, prefill);
  };
}

export function useFocusHeadingWhenFilled(empty: boolean, heading: RefObject<HTMLElement | null>): void {
  const wasEmpty = useRef(empty);

  useEffect(() => {
    if (wasEmpty.current && !empty && focusIsLost()) {
      heading.current?.focus();
    }
    wasEmpty.current = empty;
  }, [empty, heading]);
}

export function NewLessonButton({
  audience,
  opener,
  children,
}: {
  readonly audience: Audience;
  readonly opener: string;
  readonly children: ReactNode;
}): ReactElement {
  const button = useRef<HTMLButtonElement>(null);
  const openNewLesson = useOpenNewLesson(audience, opener, button);
  return (
    <button ref={button} type="button" onClick={openNewLesson} className={quietButtonClass}>
      {children}
    </button>
  );
}

export function ScheduleLessonAction({
  audience,
  opener,
  onScheduled,
}: {
  readonly audience: Audience;
  readonly opener: string;
  readonly onScheduled: () => void;
}): ReactElement {
  const { t } = useTranslation();
  const announcement = useLessonScheduled(onScheduled);
  const button = useRef<HTMLButtonElement>(null);
  const openNewLesson = useOpenNewLesson(audience, opener, button);
  return (
    <div>
      <button ref={button} type="button" onClick={openNewLesson} className={primaryButtonClass}>
        {t('lessons.schedule')}
      </button>
      <LiveNotice message={announcement} />
    </div>
  );
}
