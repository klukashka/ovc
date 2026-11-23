import { type ReactElement, useEffect, useRef } from 'react';
import { useTranslation } from 'react-i18next';

import { primaryButtonClass } from '../../components/ui/Form.tsx';
import { useFormResult, useOpenFormRoute, useReturnFocus } from '../../components/ui/formRoute.ts';
import { LiveNotice } from '../../components/ui/LiveNotice.tsx';
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
  const me = useSignedInAccount();
  const open = useOpenFormRoute(me.id);
  const announcement = useLessonScheduled(onScheduled);
  const button = useRef<HTMLButtonElement>(null);
  useReturnFocus(opener, button);
  return (
    <div>
      <button
        ref={button}
        type="button"
        onClick={() => {
          const prefill: LessonPrefill = { ...nextFullHour(Date.now(), me.timezone), audience };
          open(newLessonAddress(), opener, prefill);
        }}
        className={primaryButtonClass}
      >
        {t('lessons.schedule')}
      </button>
      <LiveNotice message={announcement} />
    </div>
  );
}
