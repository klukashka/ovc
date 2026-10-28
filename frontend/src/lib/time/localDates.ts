export type LocalDate = `${number}-${number}-${number}`;

interface ZonedParts {
  readonly year: number;
  readonly month: number;
  readonly day: number;
  readonly hour: number;
  readonly minute: number;
}

const millisecondsPerMinute = 60_000;
const millisecondsPerHour = 60 * millisecondsPerMinute;
const daysPerWeek = 7;
const offsetProbes = [-15 * millisecondsPerHour, 13 * millisecondsPerHour];
const refinements = 2;

const zoneFormats = new Map<string, Intl.DateTimeFormat>();

function zoneFormat(timeZone: string): Intl.DateTimeFormat {
  const existing = zoneFormats.get(timeZone);
  if (existing !== undefined) {
    return existing;
  }
  const format = new Intl.DateTimeFormat('en-US', {
    timeZone,
    year: 'numeric',
    month: 'numeric',
    day: 'numeric',
    hour: 'numeric',
    minute: 'numeric',
    hourCycle: 'h23',
  });
  zoneFormats.set(timeZone, format);
  return format;
}

function zonedParts(instant: number, timeZone: string): ZonedParts {
  const values = new Map<string, number>();
  for (const part of zoneFormat(timeZone).formatToParts(instant)) {
    if (part.type !== 'literal') {
      values.set(part.type, Number(part.value));
    }
  }
  const read = (type: string): number => {
    const value = values.get(type);
    if (value === undefined || Number.isNaN(value)) {
      throw new Error(`the zone format gave no ${type}`);
    }
    return value;
  };
  return { year: read('year'), month: read('month'), day: read('day'), hour: read('hour'), minute: read('minute') };
}

function twoDigits(value: number): string {
  return String(value).padStart(2, '0');
}

function dateOfParts(year: number, month: number, day: number): LocalDate {
  return `${String(year).padStart(4, '0')}-${twoDigits(month)}-${twoDigits(day)}` as LocalDate;
}

export function utcNoon(date: LocalDate): Date {
  const [year, month, day] = date.split('-').map(Number);
  return new Date(Date.UTC(year ?? 0, (month ?? 1) - 1, day ?? 1, 12));
}

export function dateOfUtc(moment: Date): LocalDate {
  return dateOfParts(moment.getUTCFullYear(), moment.getUTCMonth() + 1, moment.getUTCDate());
}

export function isLocalDate(value: string | null): value is LocalDate {
  if (value === null || !/^\d{4}-\d{2}-\d{2}$/.test(value)) {
    return false;
  }
  return dateOfUtc(utcNoon(value as LocalDate)) === value;
}

export function localDateOf(instant: number, timeZone: string): LocalDate {
  const parts = zonedParts(instant, timeZone);
  return dateOfParts(parts.year, parts.month, parts.day);
}

export function localMinutesOf(instant: number, timeZone: string): number {
  const parts = zonedParts(instant, timeZone);
  return parts.hour * 60 + parts.minute;
}

export function localTimeOf(minutes: number): string {
  return `${twoDigits(Math.floor(minutes / 60))}:${twoDigits(minutes % 60)}`;
}

function offsetAt(instant: number, timeZone: string): number {
  const parts = zonedParts(instant, timeZone);
  const wallClock = Date.UTC(parts.year, parts.month - 1, parts.day, parts.hour, parts.minute);
  return wallClock - Math.floor(instant / millisecondsPerMinute) * millisecondsPerMinute;
}

export function utcOffsetText(instant: number, timeZone: string): string {
  const minutes = Math.round(offsetAt(instant, timeZone) / millisecondsPerMinute);
  const magnitude = Math.abs(minutes);
  return `UTC${minutes < 0 ? '-' : '+'}${twoDigits(Math.floor(magnitude / 60))}:${twoDigits(magnitude % 60)}`;
}

export function localMidnight(date: LocalDate, timeZone: string): number {
  const wallMidnight = utcNoon(date).getTime() - 12 * millisecondsPerHour;
  const candidates = offsetProbes.map((probe) => {
    let candidate = wallMidnight - offsetAt(wallMidnight + probe, timeZone);
    for (let step = 0; step < refinements; step += 1) {
      candidate = wallMidnight - offsetAt(candidate, timeZone);
    }
    return candidate;
  });
  const sorted = [...new Set(candidates)].sort((left, right) => left - right);
  const start = sorted.find((candidate) => localDateOf(candidate, timeZone) >= date);
  if (start === undefined) {
    throw new Error(`no instant starts ${date} in ${timeZone}`);
  }
  return start;
}

const instantProbes = [-24 * millisecondsPerHour, 24 * millisecondsPerHour];

export function localInstant(date: LocalDate, minutes: number, timeZone: string): number {
  const wallClock = utcNoon(date).getTime() - 12 * millisecondsPerHour + minutes * millisecondsPerMinute;
  const candidates = instantProbes.map((probe) => wallClock - offsetAt(wallClock + probe, timeZone));
  const existing = candidates.filter((candidate) => wallClock - candidate === offsetAt(candidate, timeZone));
  return existing.length > 0 ? Math.min(...existing) : Math.max(...candidates);
}

export function daysBetween(from: LocalDate, to: LocalDate): number {
  return Math.round((utcNoon(to).getTime() - utcNoon(from).getTime()) / (24 * millisecondsPerHour));
}

export function addDays(date: LocalDate, days: number): LocalDate {
  const moment = utcNoon(date);
  moment.setUTCDate(moment.getUTCDate() + days);
  return dateOfUtc(moment);
}

export function isoWeekday(date: LocalDate): number {
  const weekday = utcNoon(date).getUTCDay();
  return weekday === 0 ? daysPerWeek : weekday;
}

export function sameMonth(left: LocalDate, right: LocalDate): boolean {
  return left.slice(0, 7) === right.slice(0, 7);
}
