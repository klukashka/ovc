import type { UseQueryResult } from '@tanstack/react-query';
import { type ReactElement, useId } from 'react';
import { useTranslation } from 'react-i18next';
import { Link } from 'react-router';

import { useScheduler } from '../../app/clock.ts';
import { linkClass } from '../../components/ui/Form.tsx';
import { DelayedLoading, LoadFailed } from '../../components/ui/LoadState.tsx';
import { asApiError } from '../../lib/api/client.ts';
import type { Me } from '../../lib/api/session.ts';
import { type LocalDate, localDateOf } from '../../lib/time/localDates.ts';
import type { Calendar, CalendarItem } from '../schedule/api.ts';
import { JoinControl } from '../schedule/JoinControl.tsx';
import { LessonList } from '../schedule/LessonList.tsx';
import { phaseOf, useLessonClock } from '../schedule/lessonTiming.ts';

function lessonsOn(items: readonly CalendarItem[], day: LocalDate, timeZone: string): CalendarItem[] {
  return items.filter((item) => localDateOf(Date.parse(item.starts_at), timeZone) === day);
}

export function TodayLessons({
  me,
  today,
  query,
}: {
  readonly me: Me;
  readonly today: LocalDate;
  readonly query: UseQueryResult<Calendar>;
}): ReactElement {
  const { t } = useTranslation();
  const headingId = useId();
  const scheduler = useScheduler();
  const lessons = query.data === undefined ? [] : lessonsOn(query.data.items, today, me.timezone);
  const now = scheduler.wallClock();
  const nextBeforeWindow = lessons.find((item) => phaseOf(item, now) === 'before');
  useLessonClock(nextBeforeWindow);

  function body(): ReactElement {
    if (query.isPending) {
      return <DelayedLoading />;
    }
    if (query.isError) {
      return (
        <LoadFailed
          error={asApiError(query.error)}
          onRetry={() => {
            void query.refetch();
          }}
        />
      );
    }
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
          />
        )}
      />
    );
  }

  return (
    <section aria-labelledby={headingId} className="flex flex-col gap-3">
      <h2 id={headingId} className="text-heading">
        {t('today.lessons')}
      </h2>
      {body()}
    </section>
  );
}
