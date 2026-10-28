import {
  type LocalDate,
  addDays,
  dateOfUtc,
  isoWeekday,
  localDateOf,
  localMidnight,
  localMinutesOf,
  utcNoon,
} from '../../lib/time/localDates.ts';

export type CalendarView = 'week' | 'month';

export const calendarViews: readonly CalendarView[] = ['week', 'month'];

export interface CalendarRange {
  readonly view: CalendarView;
  readonly anchor: LocalDate;
  readonly days: readonly LocalDate[];
  readonly from: number;
  readonly to: number;
}

const minutesPerDay = 24 * 60;
const daysPerWeek = 7;
const monthGridDays = 6 * daysPerWeek;

function weekStart(date: LocalDate): LocalDate {
  return addDays(date, 1 - isoWeekday(date));
}

function monthStart(date: LocalDate): LocalDate {
  return `${date.slice(0, 8)}01` as LocalDate;
}

function addMonths(date: LocalDate, months: number): LocalDate {
  const moment = utcNoon(monthStart(date));
  moment.setUTCMonth(moment.getUTCMonth() + months);
  return dateOfUtc(moment);
}

export function rangeOf(view: CalendarView, anchor: LocalDate, timeZone: string): CalendarRange {
  const first = view === 'week' ? weekStart(anchor) : weekStart(monthStart(anchor));
  const length = view === 'week' ? daysPerWeek : monthGridDays;
  const days = Array.from({ length }, (_, index) => addDays(first, index));
  return {
    view,
    anchor,
    days,
    from: localMidnight(first, timeZone),
    to: localMidnight(addDays(first, length), timeZone),
  };
}

export function shifted(view: CalendarView, anchor: LocalDate, direction: -1 | 1): LocalDate {
  return view === 'week' ? addDays(weekStart(anchor), direction * daysPerWeek) : addMonths(anchor, direction);
}

export interface Timed {
  readonly starts_at: string;
  readonly ends_at: string;
}

export interface DaySegment<Item extends Timed> {
  readonly item: Item;
  readonly top: number;
  readonly bottom: number;
  readonly lane: number;
  readonly lanes: number;
}

interface Placed<Item extends Timed> {
  readonly item: Item;
  readonly top: number;
  readonly bottom: number;
}

function placedOn<Item extends Timed>(
  items: readonly Item[],
  dayStart: number,
  dayEnd: number,
  timeZone: string,
  minimumMinutes: number,
): Placed<Item>[] {
  return items.flatMap((item) => {
    const start = Date.parse(item.starts_at);
    const end = Date.parse(item.ends_at);
    if (start >= dayEnd || end <= dayStart) {
      return [];
    }
    const top = start <= dayStart ? 0 : localMinutesOf(start, timeZone);
    const ending = end >= dayEnd ? minutesPerDay : localMinutesOf(end, timeZone);
    const shownTop = Math.min(top, minutesPerDay - minimumMinutes);
    return [{ item, top: shownTop, bottom: Math.max(ending, shownTop + minimumMinutes) }];
  });
}

function withLanes<Item extends Timed>(placed: readonly Placed<Item>[]): DaySegment<Item>[] {
  const sorted = [...placed].sort((left, right) => left.top - right.top || right.bottom - left.bottom);
  const segments: DaySegment<Item>[] = [];
  let cluster: { readonly placed: Placed<Item>; readonly lane: number }[] = [];
  let laneEnds: number[] = [];
  let clusterEnd = -1;
  const flush = (): void => {
    for (const member of cluster) {
      segments.push({ ...member.placed, lane: member.lane, lanes: laneEnds.length });
    }
    cluster = [];
    laneEnds = [];
  };
  for (const entry of sorted) {
    if (entry.top >= clusterEnd) {
      flush();
    }
    const free = laneEnds.findIndex((laneEnd) => laneEnd <= entry.top);
    const lane = free === -1 ? laneEnds.length : free;
    laneEnds[lane] = entry.bottom;
    cluster.push({ placed: entry, lane });
    clusterEnd = Math.max(clusterEnd, entry.bottom);
  }
  flush();
  return segments;
}

export function daySegments<Item extends Timed>(
  items: readonly Item[],
  day: LocalDate,
  timeZone: string,
  minimumMinutes: number,
): DaySegment<Item>[] {
  const dayStart = localMidnight(day, timeZone);
  return withLanes(placedOn(items, dayStart, localMidnight(addDays(day, 1), timeZone), timeZone, minimumMinutes));
}

export function byStartDay<Item extends Timed>(items: readonly Item[], timeZone: string): Map<LocalDate, Item[]> {
  const days = new Map<LocalDate, Item[]>();
  for (const item of items) {
    const day = localDateOf(Date.parse(item.starts_at), timeZone);
    days.set(day, [...(days.get(day) ?? []), item]);
  }
  return days;
}

export const dayMinutes = minutesPerDay;
