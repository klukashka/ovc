import { useQuery } from '@tanstack/react-query';
import { type ReactElement, useEffect, useId, useMemo, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useSearchParams } from 'react-router';

import { useMediaQuery } from '../../app/context.ts';
import { ConfirmationPrompt } from '../../components/ui/ConfirmationPrompt.tsx';
import { SelectChevron, primaryButtonClass, quietButtonClass, selectClass } from '../../components/ui/Form.tsx';
import { ChevronLeftIcon, ChevronRightIcon } from '../../components/ui/icons.tsx';
import { LiveNotice } from '../../components/ui/LiveNotice.tsx';
import { DelayedLoading, LoadFailed, RefetchFailed, focusIsLost } from '../../components/ui/LoadState.tsx';
import { Page } from '../../components/ui/Page.tsx';
import { asApiError, useApiClient } from '../../lib/api/client.ts';
import { type Me, useSignedInAccount } from '../../lib/api/session.ts';
import { fetchMyTutors, myTutorsKey } from '../tutors/api.ts';
import { Agenda } from './Agenda.tsx';
import { type Calendar, calendarRangeKey, fetchCalendar } from './api.ts';
import { formatRangeTitle } from './calendarFormat.ts';
import {
  type LocalDate,
  addDays,
  isLocalDate,
  localDateOf,
  localMinutesOf,
  localTimeOf,
} from '../../lib/time/localDates.ts';
import { type CalendarRange, type CalendarView, calendarViews, rangeOf, shifted } from './calendarTime.ts';
import { LessonFocusContext } from './LessonParts.tsx';
import { MonthGrid } from './MonthGrid.tsx';
import { type LessonPrefill, NewLessonDialog } from './NewLessonDialog.tsx';
import { type SlotChosen, WeekGrid } from './WeekGrid.tsx';

const tabletQuery = '(min-width: 768px)';
const timeGridQuery = '(min-width: 1024px) and (pointer: fine)';

const segmentClass =
  'inline-flex min-h-11 items-center px-4 text-small text-text-muted aria-pressed:rounded-md aria-pressed:border aria-pressed:border-border-strong aria-pressed:bg-field aria-pressed:text-strong aria-pressed:text-text';

function nextFullHour(now: number, timeZone: string): LessonPrefill {
  const today = localDateOf(now, timeZone);
  const hour = Math.floor(localMinutesOf(now, timeZone) / 60) + 1;
  return hour >= 24 ? { date: addDays(today, 1), start: '00:00' } : { date: today, start: localTimeOf(hour * 60) };
}

function TutorFilter({
  tutor,
  onChange,
}: {
  readonly tutor: string | null;
  readonly onChange: (tutor: string | null) => void;
}): ReactElement | null {
  const { t } = useTranslation();
  const client = useApiClient();
  const selectId = useId();
  const tutors = useQuery({ queryKey: myTutorsKey, queryFn: () => fetchMyTutors(client), retry: false });
  if (tutors.data === undefined) {
    return null;
  }
  return (
    <div className="flex flex-col gap-1">
      <label htmlFor={selectId} className="text-label">
        {t('schedule.tutorFilter')}
      </label>
      <div className="relative w-64 max-w-full">
        <select
          id={selectId}
          value={tutor ?? ''}
          onChange={(event) => {
            onChange(event.target.value === '' ? null : event.target.value);
          }}
          className={selectClass}
        >
          <option value="">{t('schedule.allTutors')}</option>
          {tutors.data.map((relationship) => (
            <option key={relationship.relationship_id} value={relationship.relationship_id}>
              {relationship.tutor.name}
            </option>
          ))}
        </select>
        <SelectChevron />
      </div>
    </div>
  );
}

function Toolbar({
  range,
  me,
  today,
  onView,
  onDate,
}: {
  readonly range: CalendarRange;
  readonly me: Me;
  readonly today: LocalDate;
  readonly onView: (view: CalendarView) => void;
  readonly onDate: (date: LocalDate) => void;
}): ReactElement {
  const { t } = useTranslation();
  const week = range.view === 'week';
  return (
    <div role="group" aria-label={t('schedule.toolbar')} className="flex flex-wrap items-center gap-3">
      <div role="group" aria-label={t('schedule.views')} className="inline-flex rounded-md bg-hover p-0.5">
        {calendarViews.map((view) => (
          <button
            key={view}
            type="button"
            aria-pressed={range.view === view}
            onClick={() => {
              onView(view);
            }}
            className={segmentClass}
          >
            {t(`schedule.${view}`)}
          </button>
        ))}
      </div>
      <button
        type="button"
        onClick={() => {
          onDate(today);
        }}
        className={quietButtonClass}
      >
        {t('schedule.today')}
      </button>
      <div className="inline-flex gap-1">
        <button
          type="button"
          aria-label={t(week ? 'schedule.previousWeek' : 'schedule.previousMonth')}
          onClick={() => {
            onDate(shifted(range.view, range.anchor, -1));
          }}
          className={`${quietButtonClass} w-11 px-0`}
        >
          <ChevronLeftIcon />
        </button>
        <button
          type="button"
          aria-label={t(week ? 'schedule.nextWeek' : 'schedule.nextMonth')}
          onClick={() => {
            onDate(shifted(range.view, range.anchor, 1));
          }}
          className={`${quietButtonClass} w-11 px-0`}
        >
          <ChevronRightIcon />
        </button>
      </div>
      <h2 className="text-heading">{formatRangeTitle(range, me.language)}</h2>
    </div>
  );
}

export function CalendarBody({
  calendar,
  range,
  me,
  today,
  onSlot,
}: {
  readonly calendar: Calendar;
  readonly range: CalendarRange;
  readonly me: Me;
  readonly today: LocalDate;
  readonly onSlot: SlotChosen | null;
}): ReactElement {
  const { t } = useTranslation();
  const wide = useMediaQuery(tabletQuery);
  const timeGrid = useMediaQuery(timeGridQuery);
  const shared = { range, items: calendar.items, timeZone: me.timezone, language: me.language, today };
  const layout = range.view === 'week' ? (timeGrid ? 'grid' : 'list') : wide ? 'month' : 'list';
  const container = useRef<HTMLDivElement>(null);
  const focusedLesson = useRef<string | null>(null);

  useEffect(() => {
    const lesson = focusedLesson.current;
    if (lesson === null || !focusIsLost()) {
      return;
    }
    const links = container.current?.querySelectorAll<HTMLElement>('a[data-lesson-id]') ?? [];
    [...links].find((link) => link.dataset.lessonId === lesson)?.focus();
  }, [layout]);

  return (
    <LessonFocusContext
      value={(lesson) => {
        focusedLesson.current = lesson;
      }}
    >
      <div ref={container} className="flex flex-col gap-6">
        {calendar.truncated ? <p className="text-small text-text-muted">{t('schedule.truncated')}</p> : null}
        {range.view === 'week' ? (
          timeGrid ? (
            <WeekGrid {...shared} onSlot={onSlot} />
          ) : (
            <Agenda {...shared} />
          )
        ) : wide ? (
          <MonthGrid {...shared} />
        ) : (
          <Agenda {...shared} />
        )}
      </div>
    </LessonFocusContext>
  );
}

export function SchedulePage(): ReactElement {
  const { t } = useTranslation();
  const me = useSignedInAccount();
  const client = useApiClient();
  const [params, setParams] = useSearchParams();
  const newLessonButton = useRef<HTMLButtonElement>(null);
  const opener = useRef<HTMLElement | null>(null);
  const [dialog, setDialog] = useState<LessonPrefill | null>(null);
  const [announcement, setAnnouncement] = useState<string | null>(null);
  const [shownAt] = useState(() => Date.now());
  const today = localDateOf(shownAt, me.timezone);
  const requestedDate = params.get('date');
  const view: CalendarView = params.get('view') === 'month' ? 'month' : 'week';
  const anchor = isLocalDate(requestedDate) ? requestedDate : today;
  const tutor = me.role === 'student' ? params.get('tutor') : null;
  const range = useMemo(() => rangeOf(view, anchor, me.timezone), [view, anchor, me.timezone]);
  const query = useQuery({
    queryKey: calendarRangeKey(range, tutor),
    queryFn: () => fetchCalendar(client, range, tutor),
    retry: false,
  });
  const tutorCanSchedule = me.role === 'tutor' && me.state === 'active';

  function changed(name: string, value: string | null): void {
    setParams((current) => {
      const next = new URLSearchParams(current);
      if (value === null) {
        next.delete(name);
      } else {
        next.set(name, value);
      }
      return next;
    });
  }

  function openDialog(prefill: LessonPrefill, control: HTMLElement | null): void {
    opener.current = control;
    setAnnouncement(null);
    setDialog(prefill);
  }

  return (
    <Page wide>
      <div className="flex flex-wrap items-end justify-between gap-4">
        <Toolbar
          range={range}
          me={me}
          today={today}
          onView={(chosen) => {
            changed('view', chosen);
          }}
          onDate={(date) => {
            changed('date', date);
          }}
        />
        {me.role === 'student' ? (
          <TutorFilter
            tutor={tutor}
            onChange={(chosen) => {
              changed('tutor', chosen);
            }}
          />
        ) : tutorCanSchedule ? (
          <button
            ref={newLessonButton}
            type="button"
            onClick={() => {
              openDialog(nextFullHour(Date.now(), me.timezone), newLessonButton.current);
            }}
            className={primaryButtonClass}
          >
            {t('schedule.newLesson')}
          </button>
        ) : (
          <ConfirmationPrompt reason="schedule" />
        )}
      </div>
      <LiveNotice message={announcement} />
      {query.data === undefined ? null : query.isError ? (
        <RefetchFailed
          error={asApiError(query.error)}
          onRetry={() => {
            void query.refetch();
          }}
        />
      ) : null}
      {query.isPending ? (
        <DelayedLoading />
      ) : query.data === undefined ? (
        <LoadFailed
          error={asApiError(query.error)}
          onRetry={() => {
            void query.refetch();
          }}
        />
      ) : (
        <CalendarBody
          calendar={query.data}
          range={range}
          me={me}
          today={today}
          onSlot={
            tutorCanSchedule
              ? (date, start): void => {
                  openDialog({ date, start }, newLessonButton.current);
                }
              : null
          }
        />
      )}
      {tutorCanSchedule ? (
        <NewLessonDialog
          open={dialog !== null}
          prefill={dialog ?? { date: today, start: '00:00' }}
          returnFocus={opener}
          onClose={() => {
            setDialog(null);
          }}
          onScheduled={() => {
            setDialog(null);
            setAnnouncement(t('schedule.scheduled'));
          }}
        />
      ) : null}
    </Page>
  );
}
