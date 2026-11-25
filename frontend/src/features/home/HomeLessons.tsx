import { useQuery } from '@tanstack/react-query';
import { type ReactElement, useId } from 'react';
import { useTranslation } from 'react-i18next';
import { Link } from 'react-router';

import { useScheduler } from '../../app/clock.ts';
import { EmptyState } from '../../components/ui/EmptyState.tsx';
import { linkClass } from '../../components/ui/Form.tsx';
import { DelayedLoading, LoadFailed } from '../../components/ui/LoadState.tsx';
import { SectionHead } from '../../components/ui/SectionHead.tsx';
import { asApiError, useApiClient } from '../../lib/api/client.ts';
import type { Me } from '../../lib/api/session.ts';
import { useFeatureTexts } from '../../lib/i18n/featureTexts.ts';
import { type CalendarItem, fetchLesson } from '../schedule/api.ts';
import { type JoinLesson, JoinControl } from '../schedule/JoinControl.tsx';
import { lessonKey } from '../schedule/keys.ts';
import { LessonList } from '../schedule/LessonList.tsx';
import { phaseOf, useLessonClock } from '../schedule/lessonTiming.ts';
import { homeworkPanelTextLoaders } from './panelTexts.ts';
import { type HomeRefetch, useRefetchAt } from './refetchAt.ts';

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

function ScheduleLink({ address }: { readonly address: string | null }): ReactElement | null {
  const { t } = useTranslation();
  if (address === null) {
    return null;
  }
  return (
    <Link to={address} className={linkClass}>
      {t('home.openSchedule')}
    </Link>
  );
}

function NoNextLesson({ scheduleAddress }: { readonly scheduleAddress: string | null }): ReactElement {
  const { t } = useTranslation();
  const texts = useFeatureTexts('homeworkPanels', homeworkPanelTextLoaders);
  if (!texts.ready) {
    return texts.failed ? <LoadFailed error={asApiError(undefined)} onRetry={texts.retry} /> : <DelayedLoading />;
  }
  return (
    <EmptyState level={3} title={t('home.noNextLesson')} sentence={t('homeworkPanels.noNextLessonSentence')}>
      {scheduleAddress === null ? undefined : <ScheduleLink address={scheduleAddress} />}
    </EmptyState>
  );
}

export function NextLesson({
  me,
  item,
  homeRefetch,
  scheduleAddress,
}: {
  readonly me: Me;
  readonly item: CalendarItem | null;
  readonly homeRefetch: HomeRefetch;
  readonly scheduleAddress: string | null;
}): ReactElement {
  const { t } = useTranslation();
  const headingId = useId();
  const next = item === null || item.status === 'cancelled' ? undefined : item;
  useRefetchAt(next?.ends_at ?? null, homeRefetch);
  return (
    <section aria-labelledby={headingId} className="flex flex-col gap-3">
      <SectionHead headingId={headingId} title={t('home.nextLesson')} />
      {next === undefined ? (
        <NoNextLesson scheduleAddress={scheduleAddress} />
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
      <SectionHead
        headingId={headingId}
        title={t('home.nextSevenDays')}
        action={<ScheduleLink address={scheduleAddress} />}
      />
      {week.length === 0 ? (
        <p className="text-text-muted">{t('home.noLessonsThisWeek')}</p>
      ) : (
        <LessonList items={week} label={t('home.nextSevenDays')} me={me} showAudience={false} showTutor />
      )}
    </section>
  );
}
