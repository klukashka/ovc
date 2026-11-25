import { type ReactElement, type RefObject, useId } from 'react';
import { useTranslation } from 'react-i18next';
import { Link } from 'react-router';

import { useScheduler } from '../../app/clock.ts';
import { linkClass } from '../../components/ui/Form.tsx';
import type { Me } from '../../lib/api/session.ts';
import type { CalendarItem } from '../schedule/api.ts';
import { JoinControl } from '../schedule/JoinControl.tsx';
import { LessonList } from '../schedule/LessonList.tsx';
import { type LessonPhase, phaseOf, useLessonClock } from '../schedule/lessonTiming.ts';

const upcomingPhases: ReadonlySet<LessonPhase> = new Set(['before', 'window', 'live']);

export function TodayLessons({
  me,
  lessons,
  headingRef,
}: {
  readonly me: Me;
  readonly lessons: readonly CalendarItem[];
  readonly headingRef: RefObject<HTMLHeadingElement | null>;
}): ReactElement {
  const { t } = useTranslation();
  const headingId = useId();
  const scheduler = useScheduler();
  const now = scheduler.wallClock();
  const nextBeforeWindow = lessons.find((item) => phaseOf(item, now) === 'before');
  useLessonClock(nextBeforeWindow);
  const nextLesson = lessons.find((item) => upcomingPhases.has(phaseOf(item, now)));
  useLessonClock(nextLesson);

  function body(): ReactElement {
    if (lessons.length === 0) {
      return (
        <div className="flex flex-col items-start gap-2">
          <p className="text-text-muted">{t('today.noLessons')}</p>
          <Link to="/schedule" className={linkClass}>
            {t('today.openSchedule')}
          </Link>
        </div>
      );
    }
    return (
      <LessonList
        items={lessons}
        label={t('today.lessons')}
        me={me}
        showAudience
        rowAction={(item) => (
          <JoinControl
            key={item.id}
            lesson={{
              id: item.id,
              status: item.status,
              starts_at: item.starts_at,
              ends_at: item.ends_at,
              room_open: item.status === 'live',
            }}
            me={me}
            showsOpensAt={item.id === nextBeforeWindow?.id}
            emphasis={item.id === nextLesson?.id ? 'filled' : 'quiet'}
          />
        )}
      />
    );
  }

  return (
    <section aria-labelledby={headingId} className="flex flex-col gap-3">
      <h2 ref={headingRef} id={headingId} tabIndex={-1} className="text-heading outline-none">
        {t('today.lessons')}
      </h2>
      {body()}
    </section>
  );
}
