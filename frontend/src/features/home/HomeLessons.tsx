import { useQuery } from '@tanstack/react-query';
import { type ReactElement, useId } from 'react';
import { useTranslation } from 'react-i18next';
import { Link } from 'react-router';

import { useScheduler } from '../../app/clock.ts';
import { linkClass } from '../../components/ui/Form.tsx';
import { LoadFailed } from '../../components/ui/LoadState.tsx';
import { asApiError, useApiClient } from '../../lib/api/client.ts';
import type { Me } from '../../lib/api/session.ts';
import { type CalendarItem, fetchLesson } from '../schedule/api.ts';
import { type JoinLesson, JoinControl } from '../schedule/JoinControl.tsx';
import { lessonKey } from '../schedule/keys.ts';
import { LessonList } from '../schedule/LessonList.tsx';
import { phaseOf, useLessonClock } from '../schedule/lessonTiming.ts';

const weekMilliseconds = 7 * 24 * 60 * 60 * 1000;

function upcoming(items: readonly CalendarItem[], now: number): CalendarItem[] {
  return items.filter((item) => item.status !== 'cancelled' && Date.parse(item.ends_at) > now);
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

export function NextLesson({ me, item }: { readonly me: Me; readonly item: CalendarItem | null }): ReactElement {
  const { t } = useTranslation();
  const headingId = useId();
  const scheduler = useScheduler();
  const next = item === null ? undefined : upcoming([item], scheduler.wallClock()).at(0);
  return (
    <section aria-labelledby={headingId} className="flex flex-col gap-3">
      <h2 id={headingId} className="text-heading">
        {t('home.nextLesson')}
      </h2>
      {next === undefined ? (
        <p className="text-text-muted">{t('home.noNextLesson')}</p>
      ) : (
        <LessonList
          items={[next]}
          label={t('home.nextLesson')}
          me={me}
          showAudience
          showTutor
          rowAction={(row) => <NextLessonJoin key={row.id} item={row} me={me} />}
        />
      )}
    </section>
  );
}

export function NextSevenDays({
  me,
  items,
  scheduleAddress,
}: {
  readonly me: Me;
  readonly items: readonly CalendarItem[];
  readonly scheduleAddress: string | null;
}): ReactElement {
  const { t } = useTranslation();
  const headingId = useId();
  const scheduler = useScheduler();
  const now = scheduler.wallClock();
  const week = upcoming(items, now).filter((item) => Date.parse(item.starts_at) < now + weekMilliseconds);
  return (
    <section aria-labelledby={headingId} className="flex flex-col gap-3">
      <h2 id={headingId} className="text-heading">
        {t('home.nextSevenDays')}
      </h2>
      {week.length === 0 ? (
        <p className="text-text-muted">{t('home.noLessonsThisWeek')}</p>
      ) : (
        <LessonList items={week} label={t('home.nextSevenDays')} me={me} showAudience={false} showTutor />
      )}
      {scheduleAddress === null ? null : (
        <Link to={scheduleAddress} className={linkClass}>
          {t('home.openSchedule')}
        </Link>
      )}
    </section>
  );
}
