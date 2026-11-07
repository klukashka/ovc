import type { Language } from '../../lib/i18n/resources.ts';
import { cityNameOf } from '../../lib/time/timezoneCities.ts';

export interface TimezoneChoice {
  readonly zone: string;
  readonly label: string;
  readonly searchText: string;
}

interface Offset {
  readonly minutes: number;
  readonly label: string;
}

const offsetPattern = /^GMT(?<sign>[+-])(?<hours>\d{2}):(?<minutes>\d{2})(?::(?<seconds>\d{2}))?$/;
const zeroOffset: Offset = { minutes: 0, label: 'UTC+00:00' };

function offsetFromName(name: string): Offset {
  if (name === 'GMT') {
    return zeroOffset;
  }
  const groups = offsetPattern.exec(name)?.groups;
  const sign = groups?.sign;
  const hours = groups?.hours;
  const minutes = groups?.minutes;
  if (sign === undefined || hours === undefined || minutes === undefined) {
    throw new Error(`unexpected timezone offset name ${name}`);
  }
  const seconds = groups?.seconds;
  const direction = sign === '-' ? -1 : 1;
  return {
    minutes: direction * (Number(hours) * 60 + Number(minutes) + Number(seconds ?? 0) / 60),
    label: seconds === undefined ? `UTC${sign}${hours}:${minutes}` : `UTC${sign}${hours}:${minutes}:${seconds}`,
  };
}

function offsetFormat(zone: string): Intl.DateTimeFormat | null {
  try {
    return new Intl.DateTimeFormat('en-US', { timeZone: zone, timeZoneName: 'longOffset' });
  } catch (thrown) {
    if (thrown instanceof RangeError) {
      return null;
    }
    throw thrown;
  }
}

function offsetOf(zone: string, instant: number): Offset | null {
  const format = offsetFormat(zone);
  if (format === null) {
    return null;
  }
  const name = format.formatToParts(instant).find((part) => part.type === 'timeZoneName')?.value;
  if (name === undefined) {
    throw new Error(`no offset name for the timezone ${zone}`);
  }
  return offsetFromName(name);
}

function placeName(zone: string, language: Language): string {
  return cityNameOf(zone, language) ?? zone.replaceAll('_', ' ');
}

function choiceOf(zone: string, offset: Offset | null, language: Language): TimezoneChoice {
  const label = offset === null ? zone : `${offset.label} · ${placeName(zone, language)}`;
  const searchText = [label, zone.replaceAll('_', ' '), cityNameOf(zone, 'en') ?? '', cityNameOf(zone, 'ru') ?? '']
    .join(' ')
    .toLocaleLowerCase();
  return { zone, label, searchText };
}

function choiceOutsideTheList(
  current: string,
  known: readonly string[],
  language: Language,
  instant: number,
): readonly TimezoneChoice[] {
  return known.includes(current) ? [] : [choiceOf(current, offsetOf(current, instant), language)];
}

export function timezoneChoices(current: string, language: Language, instant: number): readonly TimezoneChoice[] {
  const known = Intl.supportedValuesOf('timeZone');
  const collator = new Intl.Collator(language);
  const listed = known
    .map((zone) => ({ zone, offset: offsetOf(zone, instant), place: placeName(zone, language) }))
    .sort(
      (left, right) =>
        (left.offset?.minutes ?? 0) - (right.offset?.minutes ?? 0) || collator.compare(left.place, right.place),
    )
    .map(({ zone, offset }) => choiceOf(zone, offset, language));
  return [...choiceOutsideTheList(current, known, language, instant), ...listed];
}

export function matchingChoices(
  choices: readonly TimezoneChoice[],
  search: string,
  selected: string,
): readonly TimezoneChoice[] {
  const words = search
    .toLocaleLowerCase()
    .split(/\s+/)
    .filter((word) => word !== '');
  if (words.length === 0) {
    return choices;
  }
  return choices.filter(
    (choice) => choice.zone === selected || words.every((word) => choice.searchText.includes(word)),
  );
}
