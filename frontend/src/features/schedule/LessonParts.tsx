import type { TFunction } from 'i18next';
import { type ReactElement, createContext, useContext } from 'react';
import { useTranslation } from 'react-i18next';
import { Link } from 'react-router';

import { CalendarIcon, CheckIcon, CrossIcon, DashIcon } from '../../components/ui/icons.tsx';
import { paletteDotClass, paletteEdgeClass } from '../../components/ui/palette.ts';
import type { Language } from '../../lib/i18n/resources.ts';
import type { CalendarAudience, CalendarItem, LessonStatus } from './api.ts';
import { formatClock } from './calendarFormat.ts';

interface LessonState {
  readonly status: LessonStatus;
  readonly took_place: boolean;
}

function statusToneOf({ status, took_place }: LessonState): string {
  switch (status) {
    case 'scheduled':
      return 'text-info';
    case 'live':
      return 'text-live';
    case 'ended':
      return took_place ? 'text-success' : 'text-text-muted';
    case 'cancelled':
      return 'text-danger';
  }
}

function statusWord(t: TFunction, { status, took_place }: LessonState): string {
  return status === 'ended' && !took_place ? t('lesson.didNotTakePlace') : t(`lesson.status.${status}`);
}

function StatusIcon({ status, took_place }: LessonState): ReactElement {
  switch (status) {
    case 'scheduled':
      return <CalendarIcon size={12} />;
    case 'live':
      return <span aria-hidden="true" className="size-2 shrink-0 rounded-full bg-live motion-safe:animate-pulse" />;
    case 'ended':
      return took_place ? <CheckIcon size={12} /> : <DashIcon size={12} />;
    case 'cancelled':
      return <CrossIcon size={12} />;
  }
}

export const LessonFocusContext = createContext<(lesson: string) => void>(() => undefined);

export type BlockLayout = 'stacked' | 'fill' | 'compact';

const layoutClass: Readonly<Record<'stacked' | 'fill', string>> = {
  stacked: 'min-h-11 py-0.5',
  fill: 'h-full py-0.5',
};

const compactTime: Readonly<Record<LessonStatus, string>> = {
  scheduled: 'shrink-0 text-caption',
  live: 'shrink-0 text-caption',
  ended: 'shrink-0 text-caption',
  cancelled: 'shrink-0 text-caption line-through',
};

export function LessonTitle({
  title,
  cancelled,
}: {
  readonly title: string;
  readonly cancelled: boolean;
}): ReactElement {
  return (
    <span
      dir="auto"
      className={
        cancelled ? 'block break-words whitespace-pre-line line-through' : 'block break-words whitespace-pre-line'
      }
    >
      {title}
    </span>
  );
}

export function audienceLabel(t: TFunction, audience: CalendarAudience): string {
  if (audience.label !== null) {
    return audience.label;
  }
  return audience.label_kind === null ? '' : t(`lesson.${audience.label_kind}`);
}

export function LessonStatusWord({ lesson }: { readonly lesson: LessonState }): ReactElement {
  const { t } = useTranslation();
  return (
    <span className={`inline-flex items-center gap-1.5 text-small ${statusToneOf(lesson)}`}>
      <StatusIcon status={lesson.status} took_place={lesson.took_place} />
      {statusWord(t, lesson)}
    </span>
  );
}

const statusText: Readonly<Record<LessonStatus, string>> = {
  scheduled: 'text-text',
  live: 'text-text',
  ended: 'text-text',
  cancelled: 'text-text-muted',
};

function TutorLabel({ item }: { readonly item: CalendarItem }): ReactElement | null {
  if (item.tutor === null) {
    return null;
  }
  return (
    <span className="flex min-w-0 items-center gap-1.5">
      {item.tutor.colour === null ? null : (
        <span aria-hidden="true" className={`size-2 shrink-0 rounded-full ${paletteDotClass[item.tutor.colour]}`} />
      )}
      <span dir="auto" className="truncate">
        {item.tutor.name}
      </span>
    </span>
  );
}

function CompactBlock({
  item,
  times,
  start,
}: {
  readonly item: CalendarItem;
  readonly times: string;
  readonly start: string;
}): ReactElement {
  const { t } = useTranslation();
  const edge = item.audience.colour === null ? 'border-l-border-strong' : paletteEdgeClass[item.audience.colour];
  const status = statusWord(t, item);
  const focused = useContext(LessonFocusContext);
  return (
    <Link
      onFocus={() => {
        focused(item.id);
      }}
      to={`/lessons/${encodeURIComponent(item.id)}`}
      data-layout="compact"
      data-lesson-id={item.id}
      title={`${times} ${item.title} · ${status}`}
      className={`flex h-full min-w-0 items-center gap-1 overflow-hidden rounded-sm border border-l-4 border-border bg-surface px-1 whitespace-nowrap hover:border-border-strong ${edge} ${statusText[item.status]}`}
    >
      <span aria-hidden="true" data-part="time" className={compactTime[item.status]}>
        {start}
      </span>
      <span aria-hidden="true" data-part="status" className={`inline-flex shrink-0 items-center ${statusToneOf(item)}`}>
        <StatusIcon status={item.status} took_place={item.took_place} />
      </span>
      <span className="sr-only">
        {times} <LessonTitle title={item.title} cancelled={item.status === 'cancelled'} />{' '}
        {audienceLabel(t, item.audience)} {item.tutor?.name ?? ''} {status}
      </span>
    </Link>
  );
}

export function LessonBlock({
  item,
  language,
  timeZone,
  layout = 'stacked',
}: {
  readonly item: CalendarItem;
  readonly language: Language;
  readonly timeZone: string;
  readonly layout?: BlockLayout;
}): ReactElement {
  const { t } = useTranslation();
  const struck = item.status === 'cancelled';
  const edge = item.audience.colour === null ? 'border-l-border-strong' : paletteEdgeClass[item.audience.colour];
  const start = formatClock(item.starts_at, language, timeZone);
  const times = t('lesson.timeRange', { start, end: formatClock(item.ends_at, language, timeZone) });
  const focused = useContext(LessonFocusContext);
  if (layout === 'compact') {
    return <CompactBlock item={item} times={times} start={start} />;
  }
  return (
    <Link
      onFocus={() => {
        focused(item.id);
      }}
      to={`/lessons/${encodeURIComponent(item.id)}`}
      data-layout={layout}
      data-lesson-id={item.id}
      className={`flex min-w-0 flex-col gap-0.5 overflow-hidden rounded-sm border border-l-4 border-border bg-surface px-2 text-small hover:border-border-strong ${layoutClass[layout]} ${edge} ${statusText[item.status]}`}
    >
      <span className="text-caption text-text-muted">{times}</span>
      <span className="text-strong">
        <LessonTitle title={item.title} cancelled={struck} />
      </span>
      <span dir="auto" className="truncate text-text-muted">
        {audienceLabel(t, item.audience)}
      </span>
      <TutorLabel item={item} />
      <LessonStatusWord lesson={item} />
    </Link>
  );
}
