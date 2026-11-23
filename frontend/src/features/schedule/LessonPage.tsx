import { useQuery, useQueryClient } from '@tanstack/react-query';
import type { TFunction } from 'i18next';
import { type ReactElement, type ReactNode, type RefObject, useCallback, useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Link, useParams } from 'react-router';

import { useScheduler } from '../../app/clock.ts';
import { useAction } from '../../components/ui/action.ts';
import { ActionMenu, type MenuItem } from '../../components/ui/ActionMenu.tsx';
import { linkClass, quietButtonClass } from '../../components/ui/Form.tsx';
import { CalendarIcon, CheckIcon, CrossIcon, DashIcon, DoorIcon, MoreIcon } from '../../components/ui/icons.tsx';
import { useFormResult, useFormReturned, useOpenFormRoute, useReturnFocus } from '../../components/ui/formRoute.ts';
import { FocusedNotice, LiveNotice } from '../../components/ui/LiveNotice.tsx';
import { ReturnedNotice } from '../../components/ui/ReturnedNotice.tsx';
import { DelayedLoading, LoadFailed, RefetchFailed, focusHeadingIfLost } from '../../components/ui/LoadState.tsx';
import { Page } from '../../components/ui/Page.tsx';
import { PageColumns } from '../../components/ui/PageColumns.tsx';
import { type PaletteColour, paletteDotClass } from '../../components/ui/palette.ts';
import { asApiError, useApiClient } from '../../lib/api/client.ts';
import { type Me, useSignedInAccount } from '../../lib/api/session.ts';
import { formatDate, localeOf } from '../../lib/format/dates.ts';
import { type LocalDate, isLocalDate, localDateOf, utcOffsetText } from '../../lib/time/localDates.ts';
import { cityOf } from '../../lib/time/timezoneCities.ts';
import { NotFoundContent } from '../../routes/NotFoundPage.tsx';
import { type ChangedLessons, type LessonDetail, type LessonSeries, type LessonView, fetchLesson } from './api.ts';
import type { Audience } from './AudiencePicker.tsx';
import { formatClock, formatFullDate, formatLongDay, formatWeekdayName } from './calendarFormat.ts';
import { calendarKey, lessonKey, lessonsKey } from './keys.ts';
import { CancelLessonDialog, EditLessonDialog } from './LessonDialogs.tsx';
import { audienceLabel } from './LessonParts.tsx';
import { LessonWork } from '../assignment/LessonWork.tsx';
import { IssuedNotice, useIssuedResult } from '../homework/IssueAssignment.tsx';
import { LessonIssues } from '../homework/LessonIssues.tsx';
import { withIssuesOf } from './lessonCache.ts';
import { studentWorkOf, tutorIssuesOf } from './lessonIssues.ts';
import { LessonMaterials } from './LessonMaterials.tsx';
import { LessonRoom } from './LessonRoom.tsx';
import { type LessonPhase, dialogOutdated, isEditable, phaseOf, useLessonClock } from './lessonTiming.ts';
import { addressRenewalDelay } from './materials.ts';
import { type LessonPrefill, newLessonAddress } from './newLesson.ts';
import { seriesTimingIn } from './seriesTiming.ts';

type DialogKind = 'edit' | 'cancel' | 'endSeries';

interface OpenDialog {
  readonly kind: DialogKind;
  readonly lesson: LessonDetail;
}

interface PageNotice {
  readonly lessonId: string;
  readonly kind: 'changed' | 'cancelled' | 'ended';
  readonly count: number;
  readonly afterRouteChange?: boolean;
}

const noticeKeys = {
  changed: 'lesson.changedMeanwhile',
  cancelled: 'lesson.cancelledNotice',
  ended: 'lesson.endedNotice',
} as const;

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
      return lesson.took_place ? (
        <span className="inline-flex items-center gap-1.5 text-success">
          <CheckIcon size={14} />
          {t('lesson.status.ended')}
        </span>
      ) : (
        <span className="inline-flex items-center gap-1.5 text-text-muted">
          <DashIcon size={14} />
          {t('lesson.didNotTakePlace')}
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
  readonly kind: 'deleted_account' | 'removed_student' | 'former_student' | null;
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
  const group = lesson.audience.kind === 'group';
  const audienceDetailShown = me.role === 'tutor' || group;
  const other = otherZoneTime(t, lesson, me);
  const members = lesson.audience.members;
  return (
    <div className="flex flex-col gap-2">
      <h2 className="text-heading">{t('lesson.about')}</h2>
      <dl className="border-b border-border">
        <Detail term={t('lesson.dateAndTime')}>
          <span className="block">{dateAndTime(t, lesson, me)}</span>
          {other === null ? null : <span className="block text-small text-text-muted">{other}</span>}
        </Detail>
        <Detail term={t('lesson.format')}>
          {audienceDetailShown
            ? t(group ? 'lesson.formatGroup' : 'lesson.formatIndividual')
            : audienceLabel(t, lesson.audience)}
        </Detail>
        {audienceDetailShown ? (
          <Detail term={t(group ? 'lesson.groupName' : 'lesson.student')}>
            <span dir="auto">{audienceLabel(t, lesson.audience)}</span>
          </Detail>
        ) : null}
        {members === null || members.length === 0 ? null : (
          <Detail term={t('lesson.members')}>
            <ul className="flex flex-col gap-1">
              {members.map((member, index) => (
                <li key={`${member.label ?? member.label_kind ?? ''}-${String(index)}`}>
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
    </div>
  );
}

export function lessonControlsShown(me: Me, lesson: LessonDetail | undefined, now: number): boolean {
  return me.role === 'tutor' && me.state === 'active' && lesson !== undefined && isEditable(lesson, now);
}

export function useHeadingWhenControlsLeave(shown: boolean): void {
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
  room = null,
  materials = null,
  assignments = null,
}: {
  readonly lesson: LessonDetail;
  readonly me: Me;
  readonly now: number;
  readonly room?: ReactNode;
  readonly materials?: ReactNode;
  readonly assignments?: ReactNode;
}): ReactElement {
  const { t } = useTranslation();
  const phase = phaseOf(lesson, now);
  return (
    <>
      <p>
        <Link to="/schedule" className={linkClass}>
          {t('lesson.back')}
        </Link>
      </p>
      <PageColumns
        asideLabel={t('lesson.about')}
        main={
          <>
            <div data-part="join" className="flex flex-col gap-3">
              <p role="status" className="text-strong">
                <PhaseWord phase={phase} lesson={lesson} me={me} />
              </p>
              {room}
            </div>
            {lesson.description === null ? null : (
              <p dir="auto" data-part="description" className="max-w-running-text break-words whitespace-pre-line">
                {lesson.description}
              </p>
            )}
            {materials}
            {assignments}
          </>
        }
        aside={<AboutLesson lesson={lesson} me={me} />}
      />
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

export function LessonEditButton({
  editButton,
  onOpen,
}: {
  readonly editButton: RefObject<HTMLButtonElement | null>;
  readonly onOpen: (kind: DialogKind) => void;
}): ReactElement {
  const { t } = useTranslation();
  return (
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
  );
}

const lessonActionsOpener = 'lesson-actions';
const nextSeriesContext = 'next-series';

export function LessonActionsMenu({
  menuButton,
  series,
  onOpen,
}: {
  readonly menuButton: RefObject<HTMLButtonElement | null>;
  readonly series: boolean;
  readonly onOpen: (kind: DialogKind) => void;
}): ReactElement {
  const { t } = useTranslation();
  useReturnFocus(lessonActionsOpener, menuButton);
  const cancel: MenuItem = {
    label: t('lesson.cancel'),
    onChoose: (): void => {
      onOpen('cancel');
    },
  };
  const endSeries: MenuItem = {
    label: t('lesson.endSeries'),
    onChoose: (): void => {
      onOpen('endSeries');
    },
  };
  return (
    <ActionMenu
      label={t('lesson.actions')}
      unavailable={false}
      items={series ? [cancel, endSeries] : [cancel]}
      buttonRef={menuButton}
    >
      <MoreIcon />
    </ActionMenu>
  );
}

export function headingOf(lesson: LessonDetail | undefined, notFound: boolean): Parameters<typeof Page>[0] {
  if (notFound) {
    return { title: 'page.notFound', focusTitle: true };
  }
  return lesson === undefined ? {} : { headingText: lesson.title, headingDir: 'auto' };
}

export function LessonPage(): ReactElement {
  const { lessonId = '' } = useParams();
  const me = useSignedInAccount();
  const client = useApiClient();
  const queryClient = useQueryClient();
  const scheduler = useScheduler();
  const { t } = useTranslation();
  const key = lessonKey(lessonId);
  const editButton = useRef<HTMLButtonElement>(null);
  const menuButton = useRef<HTMLButtonElement>(null);
  const [dialog, setDialog] = useState<OpenDialog | null>(null);
  const openNewLesson = useOpenFormRoute(me.id);
  const nextSeriesReturned = useFormReturned(nextSeriesContext);
  const nextSeriesScheduled = useFormResult('lessonScheduled');
  const issuedId = useIssuedResult();
  const [notice, setNotice] = useState<PageNotice | null>(() =>
    nextSeriesReturned ? { lessonId, kind: 'cancelled', count: 1, afterRouteChange: true } : null,
  );
  const [announcement, setAnnouncement] = useState<string | null>(() =>
    nextSeriesScheduled === null ? null : t('schedule.scheduled'),
  );
  const [issued] = useState<{ readonly lessonId: string; readonly count: number } | null>(() =>
    issuedId === null ? null : { lessonId, count: 1 },
  );
  const action = useAction();
  const query = useQuery({
    queryKey: key,
    queryFn: () => fetchLesson(client, lessonId),
    retry: false,
    refetchInterval: (current) => addressRenewalDelay(current.state.data, scheduler.wallClock()),
  });
  const now = useLessonClock(query.data);
  const queryError = query.isError ? asApiError(query.error) : null;
  const refetchError =
    queryError !== null && queryError.code !== 'not_found' && query.data !== undefined ? queryError : null;
  const error = refetchError === null ? queryError : null;
  const notFound = error?.code === 'not_found';
  const lesson = error === null ? query.data : undefined;
  const shownNotice = notice !== null && notice.lessonId === lessonId && !notFound ? notice : null;
  const controlsShown = lessonControlsShown(me, lesson, now);
  useHeadingWhenControlsLeave(controlsShown);

  const noticeOf = useCallback(
    (kind: PageNotice['kind']): void => {
      setNotice((previous) => ({ lessonId, kind, count: (previous?.count ?? 0) + 1 }));
    },
    [lessonId],
  );
  const roomChangedMeanwhile = useCallback((): void => {
    noticeOf('changed');
  }, [noticeOf]);
  const roomEnded = useCallback((): void => {
    noticeOf('ended');
  }, [noticeOf]);

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
      queryClient.setQueryData(key, (previous: LessonView | undefined) => withIssuesOf(previous, updated));
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
    openNewLesson(newLessonAddress(), lessonActionsOpener, prefill, nextSeriesContext);
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

  useEffect(() => {
    if (issuedId !== null) {
      void queryClient.invalidateQueries({ queryKey: key });
    }
  }, [issuedId, key, queryClient]);

  const tutor = me.role === 'tutor';
  const issueAudience = lesson === undefined ? null : sameAudience(t, lesson.audience);

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
    return (
      <LessonView
        lesson={lesson}
        me={me}
        now={now}
        room={
          <LessonRoom
            key={lesson.id}
            lesson={lesson}
            me={me}
            phase={phaseOf(lesson, now)}
            onChangedMeanwhile={roomChangedMeanwhile}
            onEnded={roomEnded}
          />
        }
        materials={
          <LessonMaterials
            lesson={lesson}
            me={me}
            handlers={{
              onSaved: (updated) => {
                queryClient.setQueryData(key, (previous: LessonView | undefined) => withIssuesOf(previous, updated));
              },
              onStale: stale,
            }}
          />
        }
        assignments={
          tutor ? (
            <LessonIssues
              issues={tutorIssuesOf(lesson.issues)}
              now={now}
              preset={
                me.email_verified && issueAudience !== null
                  ? {
                      audience: issueAudience,
                      lesson: { id: lesson.id, title: lesson.title, startsAt: lesson.starts_at },
                    }
                  : null
              }
            />
          ) : (
            <LessonWork work={studentWorkOf(lesson.issues)} now={now} />
          )
        }
      />
    );
  }

  return (
    <Page
      {...headingOf(lesson, notFound)}
      actionsBelowTitleOnPhones
      action={
        <>
          {lesson?.channel_id != null ? (
            <Link to={`/messages/${encodeURIComponent(lesson.channel_id)}`} className={linkClass}>
              {t('section.messages')}
            </Link>
          ) : null}
          {controlsShown ? <LessonEditButton editButton={editButton} onOpen={open} /> : null}
        </>
      }
      menu={
        controlsShown && lesson !== undefined ? (
          <LessonActionsMenu menuButton={menuButton} series={lesson.series !== null} onOpen={open} />
        ) : null
      }
    >
      {shownNotice === null ? null : shownNotice.afterRouteChange === true ? (
        <ReturnedNotice key={shownNotice.count} message={t(noticeKeys[shownNotice.kind])} />
      ) : (
        <FocusedNotice key={shownNotice.count} message={t(noticeKeys[shownNotice.kind])} />
      )}
      {issued?.lessonId !== lessonId || notFound ? null : <IssuedNotice key={issued.count} issueId={null} />}
      {refetchError === null ? null : (
        <RefetchFailed
          error={refetchError}
          onRetry={() => {
            void query.refetch();
          }}
        />
      )}
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
            returnFocus={menuButton}
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
            returnFocus={menuButton}
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
        </>
      ) : null}
    </Page>
  );
}
