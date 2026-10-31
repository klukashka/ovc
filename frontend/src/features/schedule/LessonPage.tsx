import { useQuery, useQueryClient } from '@tanstack/react-query';
import type { TFunction } from 'i18next';
import { type ReactElement, type ReactNode, type RefObject, useEffect, useId, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Link, useParams } from 'react-router';

import { useAction } from '../../components/ui/action.ts';
import { dangerButtonClass, linkClass, quietButtonClass } from '../../components/ui/Form.tsx';
import { CalendarIcon, CrossIcon, DashIcon, DoorIcon } from '../../components/ui/icons.tsx';
import { FocusedNotice, LiveNotice } from '../../components/ui/LiveNotice.tsx';
import { DelayedLoading, LoadFailed, RefetchFailed, focusHeadingIfLost } from '../../components/ui/LoadState.tsx';
import { Page } from '../../components/ui/Page.tsx';
import { type PaletteColour, paletteDotClass } from '../../components/ui/palette.ts';
import { asApiError, useApiClient } from '../../lib/api/client.ts';
import { type Me, useSignedInAccount } from '../../lib/api/session.ts';
import { formatDate, localeOf } from '../../lib/format/dates.ts';
import { type LocalDate, isLocalDate, localDateOf, utcOffsetText } from '../../lib/time/localDates.ts';
import { cityOf } from '../../lib/time/timezoneCities.ts';
import { NotFoundContent } from '../../routes/NotFoundPage.tsx';
import { type ChangedLessons, type LessonDetail, type LessonSeries, fetchLesson } from './api.ts';
import type { Audience } from './AudiencePicker.tsx';
import { formatClock, formatFullDate, formatLongDay, formatWeekdayName } from './calendarFormat.ts';
import { calendarKey, lessonKey, lessonsKey } from './keys.ts';
import { CancelLessonDialog, EditLessonDialog } from './LessonDialogs.tsx';
import { audienceLabel } from './LessonParts.tsx';
import { type LessonPhase, dialogOutdated, isEditable, phaseOf, roomOpensAt, useLessonClock } from './lessonTiming.ts';
import { type LessonPrefill, NewLessonDialog, nextFullHour } from './NewLessonDialog.tsx';
import { seriesTimingIn } from './seriesTiming.ts';

type DialogKind = 'edit' | 'cancel' | 'endSeries';

interface OpenDialog {
  readonly kind: DialogKind;
  readonly lesson: LessonDetail;
}

interface PageNotice {
  readonly lessonId: string;
  readonly kind: 'changed' | 'cancelled';
  readonly count: number;
}

function ColourDot({ colour }: { readonly colour: PaletteColour | null }): ReactElement | null {
  return colour === null ? null : (
    <span aria-hidden="true" className={`size-2 shrink-0 rounded-full ${paletteDotClass[colour]}`} />
  );
}

function PhaseWord({
  phase,
  lesson,
  me,
}: {
  readonly phase: LessonPhase;
  readonly lesson: LessonDetail;
  readonly me: Me;
}): ReactElement {
  const { t } = useTranslation();
  switch (phase) {
    case 'before':
      return (
        <span className="inline-flex items-center gap-1.5 text-info">
          <CalendarIcon size={14} />
          {t('lesson.status.scheduled')}
        </span>
      );
    case 'window':
      return (
        <span className="inline-flex items-center gap-1.5 text-accent-text">
          <DoorIcon size={14} />
          {t('lesson.startingSoon')}
        </span>
      );
    case 'live':
      return (
        <span className="inline-flex items-center gap-1.5 text-live">
          <span aria-hidden="true" className="size-2 shrink-0 rounded-full bg-live motion-safe:animate-pulse" />
          {t('lesson.status.live')}
        </span>
      );
    case 'past':
    case 'ended':
      return (
        <span className="inline-flex items-center gap-1.5 text-text-muted">
          <DashIcon size={14} />
          {t('lesson.status.ended')}
        </span>
      );
    case 'cancelled':
      return (
        <span className="inline-flex items-center gap-1.5 text-danger">
          <CrossIcon size={14} />
          {lesson.cancelled_at === null
            ? t('lesson.status.cancelled')
            : t('lesson.cancelledOn', { date: formatDate(lesson.cancelled_at, me.language, me.timezone) })}
        </span>
      );
  }
}

function dateAndTime(t: TFunction, lesson: LessonDetail, me: Me): string {
  const day = formatLongDay(localDateOf(Date.parse(lesson.starts_at), me.timezone), me.language);
  const times = t('lesson.timeRange', {
    start: formatClock(lesson.starts_at, me.language, me.timezone),
    end: formatClock(lesson.ends_at, me.language, me.timezone),
  });
  return `${day}, ${times}`;
}

export function otherZoneTime(t: TFunction, lesson: LessonDetail, me: Me): string | null {
  const zone = lesson.other_timezone;
  if (zone === null) {
    return null;
  }
  const time = formatClock(lesson.starts_at, me.language, zone);
  const local = formatClock(lesson.starts_at, me.language, me.timezone);
  const city = cityOf(zone, me.language);
  return city === null
    ? t('time.other_offset', { time, offset: utcOffsetText(Date.parse(lesson.starts_at), zone), local })
    : t('time.other_zone', { time, city, local });
}

function seriesRule(t: TFunction, series: LessonSeries, lesson: LessonDetail, me: Me): string {
  const weekdays = new Intl.ListFormat(localeOf(me.language), { style: 'long', type: 'conjunction' }).format(
    series.weekdays.map((weekday) => formatWeekdayName(weekday, me.language, 'long')),
  );
  const values = { weekdays, time: series.start };
  const rule =
    series.count !== null
      ? t('lesson.seriesCount', { ...values, count: series.count })
      : isLocalDate(series.end_date)
        ? t('lesson.seriesUntil', { ...values, date: formatFullDate(series.end_date, me.language) })
        : t('lesson.seriesWeekly', values);
  if (series.timezone === me.timezone) {
    return rule;
  }
  const city = cityOf(series.timezone, me.language);
  return city === null
    ? t('lesson.seriesOffset', { rule, offset: utcOffsetText(Date.parse(lesson.starts_at), series.timezone) })
    : t('lesson.seriesZone', { rule, city });
}

function RoomState({
  phase,
  lesson,
  me,
}: {
  readonly phase: LessonPhase;
  readonly lesson: LessonDetail;
  readonly me: Me;
}): ReactElement | null {
  const { t } = useTranslation();
  const text =
    phase === 'before'
      ? t('lesson.roomOpensAt', {
          time: formatClock(new Date(roomOpensAt(lesson)).toISOString(), me.language, me.timezone),
        })
      : phase === 'window' && me.role === 'student'
        ? t('lesson.roomOpensWithTutor')
        : null;
  return text === null ? null : (
    <p className="rounded-lg border border-border bg-surface p-4 text-strong dark:border-border-strong">{text}</p>
  );
}

function AudienceLine({ lesson }: { readonly lesson: LessonDetail }): ReactElement {
  const { t } = useTranslation();
  return (
    <p className="flex flex-wrap items-center gap-x-3 gap-y-1">
      <span className="inline-flex min-w-0 items-center gap-1.5">
        <ColourDot colour={lesson.audience.colour} />
        <span dir="auto" className="break-words">
          {audienceLabel(t, lesson.audience)}
        </span>
      </span>
      {lesson.tutor === null ? null : (
        <span className="inline-flex min-w-0 items-center gap-1.5 text-text-muted">
          <ColourDot colour={lesson.tutor.colour} />
          <span dir="auto" className="break-words">
            {lesson.tutor.name}
          </span>
        </span>
      )}
    </p>
  );
}

function Detail({ term, children }: { readonly term: string; readonly children: ReactNode }): ReactElement {
  return (
    <div className="flex flex-col gap-0.5 border-t border-border py-3">
      <dt className="text-small text-text-muted">{term}</dt>
      <dd className="break-words">{children}</dd>
    </div>
  );
}

export function MemberLabel({
  label,
  kind,
}: {
  readonly label: string | null;
  readonly kind: 'deleted_account' | null;
}): ReactElement {
  const { t } = useTranslation();
  const kindText = kind === null ? null : t(`lesson.${kind}`);
  if (label === null || label === '') {
    return <span>{kindText}</span>;
  }
  return (
    <>
      <span dir="auto">{label}</span>
      {kindText === null ? null : <span className="text-small text-text-muted"> · {kindText}</span>}
    </>
  );
}

function AboutLesson({ lesson, me }: { readonly lesson: LessonDetail; readonly me: Me }): ReactElement {
  const { t } = useTranslation();
  const headingId = useId();
  const group = lesson.audience.kind === 'group';
  const other = otherZoneTime(t, lesson, me);
  const members = lesson.audience.members;
  return (
    <aside aria-labelledby={headingId} className="flex flex-col gap-2">
      <h2 id={headingId} className="text-heading">
        {t('lesson.about')}
      </h2>
      <dl className="border-b border-border">
        <Detail term={t('lesson.dateAndTime')}>
          <span className="block">{dateAndTime(t, lesson, me)}</span>
          {other === null ? null : <span className="block text-small text-text-muted">{other}</span>}
        </Detail>
        <Detail term={t('lesson.format')}>{t(group ? 'lesson.formatGroup' : 'lesson.formatIndividual')}</Detail>
        {me.role === 'tutor' || group ? (
          <Detail term={t(group ? 'lesson.groupName' : 'lesson.student')}>
            <span dir="auto">{audienceLabel(t, lesson.audience)}</span>
          </Detail>
        ) : null}
        {members === null || members.length === 0 ? null : (
          <Detail term={t('lesson.members')}>
            <ul className="flex flex-col gap-1">
              {members.map((member, index) => (
                <li key={`${member.label}-${String(index)}`}>
                  <MemberLabel label={member.label} kind={member.label_kind} />
                </li>
              ))}
            </ul>
          </Detail>
        )}
        {lesson.tutor === null ? null : (
          <Detail term={t('lesson.tutor')}>
            <span className="inline-flex items-center gap-1.5">
              <ColourDot colour={lesson.tutor.colour} />
              <span dir="auto">{lesson.tutor.name}</span>
            </span>
          </Detail>
        )}
        {lesson.series === null ? null : (
          <Detail term={t('lesson.series')}>{seriesRule(t, lesson.series, lesson, me)}</Detail>
        )}
      </dl>
    </aside>
  );
}

function useHeadingWhenControlsLeave(shown: boolean): void {
  const shownBefore = useRef(shown);
  useEffect(() => {
    if (shownBefore.current && !shown) {
      focusHeadingIfLost();
    }
    shownBefore.current = shown;
  }, [shown]);
}

export function LessonView({
  lesson,
  me,
  now,
  controls = null,
}: {
  readonly lesson: LessonDetail;
  readonly me: Me;
  readonly now: number;
  readonly controls?: ReactNode;
}): ReactElement {
  const { t } = useTranslation();
  const phase = phaseOf(lesson, now);
  const other = otherZoneTime(t, lesson, me);
  const controlsShown = controls !== null && isEditable(lesson, now);
  useHeadingWhenControlsLeave(controlsShown);
  return (
    <>
      <p>
        <Link to="/schedule" className={linkClass}>
          {t('lesson.back')}
        </Link>
      </p>
      <div className="grid grid-cols-1 gap-8 desktop:grid-cols-[minmax(0,1fr)_300px] desktop:gap-16">
        <div className="flex min-w-0 flex-col gap-6">
          <div className="flex flex-col gap-2">
            <p role="status" className="text-strong">
              <PhaseWord phase={phase} lesson={lesson} me={me} />
            </p>
            <p>{dateAndTime(t, lesson, me)}</p>
            {other === null ? null : <p className="text-small text-text-muted">{other}</p>}
            <AudienceLine lesson={lesson} />
          </div>
          {lesson.description === null ? null : (
            <p dir="auto" data-part="description" className="break-words whitespace-pre-line">
              {lesson.description}
            </p>
          )}
          {controlsShown ? controls : null}
          <RoomState phase={phase} lesson={lesson} me={me} />
        </div>
        <AboutLesson lesson={lesson} me={me} />
      </div>
    </>
  );
}

function sameAudience(t: TFunction, audience: LessonDetail['audience']): Audience | null {
  const name = audienceLabel(t, audience);
  if (audience.relationship_id !== undefined) {
    return { kind: 'one_to_one', id: audience.relationship_id, name };
  }
  if (audience.group_id !== undefined) {
    return { kind: 'group', id: audience.group_id, name };
  }
  return null;
}

interface EndedOccurrence {
  readonly lesson: LessonDetail;
  readonly series: LessonSeries;
  readonly date: LocalDate;
}

function endedOccurrenceOf(lesson: LessonDetail): EndedOccurrence | null {
  const date = lesson.occurrence_date ?? null;
  if (lesson.series === null || !isLocalDate(date)) {
    return null;
  }
  return { lesson, series: lesson.series, date };
}

function nextSeriesPrefill(t: TFunction, ended: EndedOccurrence, timeZone: string): LessonPrefill {
  const { lesson, series } = ended;
  const audience = sameAudience(t, lesson.audience);
  const timing = seriesTimingIn(series, ended.date, timeZone);
  return {
    date: timing.date,
    start: timing.start,
    durationMinutes: series.duration_minutes,
    weekdays: timing.weekdays,
    ...(audience === null ? {} : { audience }),
  };
}

function seriesEndsBefore(changed: ChangedLessons, occurrenceDate: LocalDate): boolean {
  const ended = changed.series;
  return ended !== null && ended.count === null && ended.end_date !== null && ended.end_date < occurrenceDate;
}

export function nextSeriesAfterEnding(
  t: TFunction,
  lesson: LessonDetail,
  changed: ChangedLessons,
  timeZone: string,
): LessonPrefill | null {
  const ended = endedOccurrenceOf(lesson);
  if (ended === null || !seriesEndsBefore(changed, ended.date)) {
    return null;
  }
  return nextSeriesPrefill(t, ended, timeZone);
}

function TutorControls({
  editButton,
  cancelButton,
  endSeriesButton,
  onOpen,
}: {
  readonly editButton: RefObject<HTMLButtonElement | null>;
  readonly cancelButton: RefObject<HTMLButtonElement | null>;
  readonly endSeriesButton: RefObject<HTMLButtonElement | null> | null;
  readonly onOpen: (kind: DialogKind) => void;
}): ReactElement {
  const { t } = useTranslation();
  return (
    <div className="flex flex-wrap gap-3">
      <button
        ref={editButton}
        type="button"
        onClick={() => {
          onOpen('edit');
        }}
        className={quietButtonClass}
      >
        {t('lesson.edit')}
      </button>
      <button
        ref={cancelButton}
        type="button"
        onClick={() => {
          onOpen('cancel');
        }}
        className={dangerButtonClass}
      >
        {t('lesson.cancel')}
      </button>
      {endSeriesButton === null ? null : (
        <button
          ref={endSeriesButton}
          type="button"
          onClick={() => {
            onOpen('endSeries');
          }}
          className={quietButtonClass}
        >
          {t('lesson.endSeries')}
        </button>
      )}
    </div>
  );
}

export function headingOf(lesson: LessonDetail | undefined, notFound: boolean): Parameters<typeof Page>[0] {
  if (notFound) {
    return { title: 'page.notFound', focusTitle: true, wide: true };
  }
  return lesson === undefined ? { wide: true } : { headingText: lesson.title, headingDir: 'auto', wide: true };
}

export function LessonPage(): ReactElement {
  const { lessonId = '' } = useParams();
  const me = useSignedInAccount();
  const client = useApiClient();
  const queryClient = useQueryClient();
  const { t } = useTranslation();
  const key = lessonKey(lessonId);
  const editButton = useRef<HTMLButtonElement>(null);
  const cancelButton = useRef<HTMLButtonElement>(null);
  const endSeriesButton = useRef<HTMLButtonElement>(null);
  const [dialog, setDialog] = useState<OpenDialog | null>(null);
  const [nextSeries, setNextSeries] = useState<LessonPrefill | null>(null);
  const [closedPrefill] = useState<LessonPrefill>(() => nextFullHour(Date.now(), me.timezone));
  const [notice, setNotice] = useState<PageNotice | null>(null);
  const [announcement, setAnnouncement] = useState<string | null>(null);
  const action = useAction();
  const query = useQuery({ queryKey: key, queryFn: () => fetchLesson(client, lessonId), retry: false });
  const now = useLessonClock(query.data);
  const queryError = query.isError ? asApiError(query.error) : null;
  const refetchError =
    queryError !== null && queryError.code !== 'not_found' && query.data !== undefined ? queryError : null;
  const error = refetchError === null ? queryError : null;
  const notFound = error?.code === 'not_found';
  const lesson = error === null ? query.data : undefined;
  const shownNotice = notice !== null && notice.lessonId === lessonId && !notFound ? notice : null;

  function noticeOf(kind: PageNotice['kind']): void {
    setNotice((previous) => ({ lessonId, kind, count: (previous?.count ?? 0) + 1 }));
  }

  if (dialog !== null && !action.busy && dialogOutdated(dialog.lesson, lesson, now)) {
    setDialog(null);
    noticeOf('changed');
  }

  function open(kind: DialogKind): void {
    if (lesson === undefined) {
      return;
    }
    action.reset();
    setAnnouncement(null);
    setNotice(null);
    setDialog({ kind, lesson });
  }

  function saved(kind: DialogKind, changed: ChangedLessons): void {
    const updated = changed.lessons.find((candidate) => candidate.id === lessonId);
    if (updated !== undefined) {
      queryClient.setQueryData(key, updated);
    }
    void queryClient.invalidateQueries({ queryKey: calendarKey });
    void queryClient.invalidateQueries({ queryKey: lessonsKey, refetchType: 'none' });
    setDialog(null);
    if (kind === 'edit') {
      setAnnouncement(t('lesson.changed'));
    }
    if (kind === 'cancel') {
      noticeOf('cancelled');
    }
  }

  function seriesEnded(lesson: LessonDetail, changed: ChangedLessons): void {
    const prefill = nextSeriesAfterEnding(t, lesson, changed, me.timezone);
    if (prefill === null) {
      stale();
      return;
    }
    saved('endSeries', changed);
    setNextSeries(prefill);
  }

  function nextSeriesClosed(): void {
    if (nextSeries === null) {
      return;
    }
    setNextSeries(null);
    noticeOf('cancelled');
  }

  function stale(): void {
    setDialog(null);
    noticeOf('changed');
    void queryClient.invalidateQueries({ queryKey: key });
    void queryClient.invalidateQueries({ queryKey: calendarKey });
  }

  function dismissed(): void {
    setDialog(null);
  }

  function body(): ReactElement {
    if (error !== null) {
      return notFound ? (
        <NotFoundContent />
      ) : (
        <LoadFailed
          error={error}
          onRetry={() => {
            void query.refetch();
          }}
        />
      );
    }
    if (lesson === undefined) {
      return <DelayedLoading />;
    }
    const tutorWrites = me.role === 'tutor' && me.state === 'active';
    return (
      <LessonView
        lesson={lesson}
        me={me}
        now={now}
        controls={
          tutorWrites ? (
            <TutorControls
              editButton={editButton}
              cancelButton={cancelButton}
              endSeriesButton={lesson.series === null ? null : endSeriesButton}
              onOpen={open}
            />
          ) : null
        }
      />
    );
  }

  const tutor = me.role === 'tutor';
  return (
    <Page {...headingOf(lesson, notFound)}>
      {shownNotice === null ? null : (
        <FocusedNotice
          key={shownNotice.count}
          message={t(shownNotice.kind === 'changed' ? 'lesson.changedMeanwhile' : 'lesson.cancelledNotice')}
        />
      )}
      {refetchError === null ? null : (
        <RefetchFailed
          error={refetchError}
          onRetry={() => {
            void query.refetch();
          }}
        />
      )}
      {lesson?.channel_id != null ? (
        <Link to={`/messages/${encodeURIComponent(lesson.channel_id)}`} className="text-accent-text underline">
          {t('section.messages')}
        </Link>
      ) : null}
      <LiveNotice message={announcement} />
      {body()}
      {tutor ? (
        <>
          <EditLessonDialog
            lesson={dialog?.kind === 'edit' ? dialog.lesson : null}
            me={me}
            action={action}
            returnFocus={editButton}
            handlers={{
              onDismiss: dismissed,
              onSaved: (changed) => {
                saved('edit', changed);
              },
              onStale: stale,
            }}
          />
          <CancelLessonDialog
            lesson={dialog?.kind === 'cancel' ? dialog.lesson : null}
            action={action}
            returnFocus={cancelButton}
            handlers={{
              onDismiss: dismissed,
              onSaved: (changed) => {
                saved('cancel', changed);
              },
              onStale: stale,
            }}
          />
          <CancelLessonDialog
            lesson={dialog?.kind === 'endSeries' ? dialog.lesson : null}
            purpose="endSeries"
            action={action}
            returnFocus={endSeriesButton}
            handlers={{
              onDismiss: dismissed,
              onSaved: (changed) => {
                if (dialog !== null) {
                  seriesEnded(dialog.lesson, changed);
                }
              },
              onStale: stale,
            }}
          />
          <NewLessonDialog
            open={nextSeries !== null}
            prefill={nextSeries ?? closedPrefill}
            returnFocus={endSeriesButton}
            onClose={nextSeriesClosed}
            onScheduled={() => {
              nextSeriesClosed();
              setAnnouncement(t('schedule.scheduled'));
            }}
          />
        </>
      ) : null}
    </Page>
  );
}
