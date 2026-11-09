import { type UseQueryResult, useQuery } from '@tanstack/react-query';
import { type ReactElement, useId } from 'react';
import { useTranslation } from 'react-i18next';
import { Link } from 'react-router';

import { useScheduler } from '../../app/clock.ts';
import { linkClass } from '../../components/ui/Form.tsx';
import { DelayedLoading, LoadFailed } from '../../components/ui/LoadState.tsx';
import { asApiError, useApiClient } from '../../lib/api/client.ts';
import type { Me } from '../../lib/api/session.ts';
import { type Calendar, type CalendarItem, fetchLesson } from '../schedule/api.ts';
import { type JoinLesson, JoinControl } from '../schedule/JoinControl.tsx';
import { lessonKey } from '../schedule/keys.ts';
import { LessonList } from '../schedule/LessonList.tsx';
import { phaseOf, useLessonClock } from '../schedule/lessonTiming.ts';

const weekMilliseconds = 7 * 24 * 60 * 60 * 1000;

function upcoming(items: readonly CalendarItem[], now: number): CalendarItem[] {
  return items.filter((item) => item.status !== 'cancelled' && Date.parse(item.ends_at) > now);
}

function Loaded({
  query,
  render,
}: {
  readonly query: UseQueryResult<Calendar>;
  readonly render: (calendar: Calendar) => ReactElement;
}): ReactElement {
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
  return render(query.data);
}

function NextLessonJoin({ item, me }: { readonly item: CalendarItem; readonly me: Me }): ReactElement {
  const client = useApiClient();
  const phase = phaseOf(item, useLessonClock(item));
  const roomMayOpen = phase === 'window' || phase === 'live';
  const detail = useQuery({
    queryKey: lessonKey(item.id),
    queryFn: () => fetchLesson(client, item.id),
    enabled: roomMayOpen,
    retry: false,
  });
  const fromCalendar: JoinLesson = {
    id: item.id,
    status: item.status,
    starts_at: item.starts_at,
    ends_at: item.ends_at,
    room_open: false,
  };
  const lesson: JoinLesson =
    roomMayOpen && detail.data !== undefined
      ? {
          id: item.id,
          status: detail.data.status,
          starts_at: detail.data.starts_at,
          ends_at: detail.data.ends_at,
          room_open: detail.data.room_open,
        }
      : fromCalendar;
  if (roomMayOpen && detail.isError && detail.data === undefined) {
    return (
      <LoadFailed
        error={asApiError(detail.error)}
        onRetry={() => {
          void detail.refetch();
        }}
      />
    );
  }
  return <JoinControl key={item.id} lesson={lesson} me={me} showsOpensAt />;
}

export function NextLesson({ me, query }: { readonly me: Me; readonly query: UseQueryResult<Calendar> }): ReactElement {
  const { t } = useTranslation();
  const headingId = useId();
  const scheduler = useScheduler();
  return (
    <section aria-labelledby={headingId} className="flex flex-col gap-3">
      <h2 id={headingId} className="text-heading">
        {t('home.nextLesson')}
      </h2>
      <Loaded
        query={query}
        render={(calendar) => {
          const next = upcoming(calendar.items, scheduler.wallClock()).at(0);
          return next === undefined ? (
            <p className="text-text-muted">{t('home.noNextLesson')}</p>
          ) : (
            <LessonList
              items={[next]}
              label={t('home.nextLesson')}
              me={me}
              showAudience
              showTutor
              rowAction={(item) => <NextLessonJoin key={item.id} item={item} me={me} />}
            />
          );
        }}
      />
    </section>
  );
}

export function NextSevenDays({
  me,
  query,
  scheduleAddress,
}: {
  readonly me: Me;
  readonly query: UseQueryResult<Calendar>;
  readonly scheduleAddress: string | null;
}): ReactElement {
  const { t } = useTranslation();
  const headingId = useId();
  const scheduler = useScheduler();
  return (
    <section aria-labelledby={headingId} className="flex flex-col gap-3">
      <h2 id={headingId} className="text-heading">
        {t('home.nextSevenDays')}
      </h2>
      <Loaded
        query={query}
        render={(calendar) => {
          const now = scheduler.wallClock();
          const week = upcoming(calendar.items, now).filter(
            (item) => Date.parse(item.starts_at) < now + weekMilliseconds,
          );
          return week.length === 0 ? (
            <p className="text-text-muted">{t('home.noLessonsThisWeek')}</p>
          ) : (
            <LessonList items={week} label={t('home.nextSevenDays')} me={me} showAudience={false} showTutor />
          );
        }}
      />
      {scheduleAddress === null ? null : (
        <Link to={scheduleAddress} className={linkClass}>
          {t('home.openSchedule')}
        </Link>
      )}
    </section>
  );
}
