import type { Locator, Page } from '@playwright/test';

import type { TestAccount } from './accounts.ts';
import type { Deployment } from './deployment.ts';
import { accessToken, callApi } from './tutoring.ts';

export type LocalDate = string;

export interface LessonRequest {
  readonly relationship_id?: string;
  readonly group_id?: string;
  readonly title: string;
  readonly date: LocalDate;
  readonly start: string;
  readonly duration_minutes: number;
  readonly weekdays?: readonly number[];
  readonly count?: number;
  readonly end_date?: LocalDate;
}

const dayMilliseconds = 86_400_000;

export function localDate(instant: number, timeZone: string): LocalDate {
  return new Intl.DateTimeFormat('en-CA', { timeZone, year: 'numeric', month: '2-digit', day: '2-digit' }).format(
    instant,
  );
}

export function localClock(instant: number, timeZone: string): string {
  return new Intl.DateTimeFormat('en-GB', { timeZone, hour: '2-digit', minute: '2-digit', hourCycle: 'h23' }).format(
    instant,
  );
}

function noon(date: LocalDate): number {
  return Date.parse(`${date}T12:00:00Z`);
}

export function addDays(date: LocalDate, days: number): LocalDate {
  return new Date(noon(date) + days * dayMilliseconds).toISOString().slice(0, 10);
}

export function isoWeekday(date: LocalDate): number {
  const weekday = new Date(noon(date)).getUTCDay();
  return weekday === 0 ? 7 : weekday;
}

export function nextWeekday(from: LocalDate, weekdays: readonly number[]): LocalDate {
  for (let offset = 0; offset < 7; offset += 1) {
    const candidate = addDays(from, offset);
    if (weekdays.includes(isoWeekday(candidate))) {
      return candidate;
    }
  }
  throw new Error('no weekday was chosen');
}

export function seriesDates(first: LocalDate, weekdays: readonly number[], count: number): LocalDate[] {
  const dates: LocalDate[] = [];
  for (let day = first; dates.length < count; day = addDays(day, 1)) {
    if (weekdays.includes(isoWeekday(day))) {
      dates.push(day);
    }
  }
  return dates;
}

export function moscowInstant(date: LocalDate, clock: string): number {
  return Date.parse(`${date}T${clock}:00+03:00`);
}

function field(value: unknown, name: string): unknown {
  if (typeof value !== 'object' || value === null || !(name in value)) {
    throw new Error(`the answer has no ${name}`);
  }
  return (value as Record<string, unknown>)[name];
}

export async function scheduleLessons(
  deployment: Deployment,
  tutor: TestAccount,
  request: LessonRequest,
): Promise<readonly string[]> {
  const answer = await callApi(deployment, await accessToken(deployment, tutor), 'POST', '/lessons', request);
  if (answer.status !== 201) {
    throw new Error(`scheduling answered ${String(answer.status)}`);
  }
  const lessons = field(answer.body, 'lessons');
  if (!Array.isArray(lessons)) {
    throw new Error('the created lessons are not a list');
  }
  return lessons.map((lesson: unknown) => {
    const startsAt = field(lesson, 'starts_at');
    if (typeof startsAt !== 'string') {
      throw new Error('a created lesson has no start');
    }
    return startsAt;
  });
}

export async function setTimezone(deployment: Deployment, account: TestAccount, timezone: string): Promise<void> {
  const answer = await callApi(deployment, await accessToken(deployment, account), 'PATCH', '/users/me', { timezone });
  if (answer.status !== 200) {
    throw new Error(`changing the timezone answered ${String(answer.status)}`);
  }
}

export function lessonLinks(page: Page): Locator {
  return page.getByRole('main').locator('a[href^="/lessons/"]');
}

export async function lessonAddresses(page: Page): Promise<string[]> {
  return lessonLinks(page).evaluateAll((links) => links.map((link) => link.getAttribute('href') ?? ''));
}

export interface ScheduledLesson {
  readonly id: string;
  readonly startsAt: string;
  readonly title: string;
  readonly status: string;
  readonly version: number;
}

function scheduledLessonOf(value: unknown): ScheduledLesson {
  const id = field(value, 'id');
  const startsAt = field(value, 'starts_at');
  const title = field(value, 'title');
  const status = field(value, 'status');
  const version = field(value, 'version');
  if (typeof id !== 'string' || typeof startsAt !== 'string' || typeof title !== 'string') {
    throw new Error('a lesson has no identifier, start or title');
  }
  if (typeof status !== 'string' || typeof version !== 'number') {
    throw new Error('a lesson has no status or version');
  }
  return { id, startsAt, title, status, version };
}

export async function scheduleLessonsWithIds(
  deployment: Deployment,
  tutor: TestAccount,
  request: LessonRequest,
): Promise<readonly ScheduledLesson[]> {
  const answer = await callApi(deployment, await accessToken(deployment, tutor), 'POST', '/lessons', request);
  if (answer.status !== 201) {
    throw new Error(`scheduling answered ${String(answer.status)}`);
  }
  const lessons = field(answer.body, 'lessons');
  if (!Array.isArray(lessons)) {
    throw new Error('the created lessons are not a list');
  }
  return lessons.map(scheduledLessonOf);
}

export async function readLesson(deployment: Deployment, account: TestAccount, id: string): Promise<ScheduledLesson> {
  const answer = await callApi(deployment, await accessToken(deployment, account), 'GET', `/lessons/${id}`);
  if (answer.status !== 200) {
    throw new Error(`reading the lesson answered ${String(answer.status)}`);
  }
  return scheduledLessonOf(answer.body);
}

export async function editThisLesson(
  deployment: Deployment,
  tutor: TestAccount,
  id: string,
  changes: Readonly<Record<string, string | number>>,
): Promise<void> {
  const { version } = await readLesson(deployment, tutor, id);
  const answer = await callApi(deployment, await accessToken(deployment, tutor), 'PATCH', `/lessons/${id}`, {
    scope: 'this',
    version,
    ...changes,
  });
  if (answer.status !== 200) {
    throw new Error(`editing the lesson answered ${String(answer.status)}`);
  }
}

export async function cancelThisLesson(deployment: Deployment, tutor: TestAccount, id: string): Promise<void> {
  const { version } = await readLesson(deployment, tutor, id);
  const answer = await callApi(deployment, await accessToken(deployment, tutor), 'POST', `/lessons/${id}/cancel`, {
    scope: 'this',
    version,
  });
  if (answer.status !== 200) {
    throw new Error(`cancelling the lesson answered ${String(answer.status)}`);
  }
}

export async function cardLessonStatuses(
  deployment: Deployment,
  tutor: TestAccount,
  cardId: string,
): Promise<readonly string[]> {
  const answer = await callApi(
    deployment,
    await accessToken(deployment, tutor),
    'GET',
    `/students/${cardId}/lessons?limit=100`,
  );
  if (answer.status !== 200) {
    throw new Error(`reading the card's lessons answered ${String(answer.status)}`);
  }
  const items = field(answer.body, 'items');
  if (!Array.isArray(items)) {
    throw new Error("the card's lessons are not a list");
  }
  return items.map((item: unknown) => {
    const status = field(item, 'status');
    if (typeof status !== 'string') {
      throw new Error('a lesson has no status');
    }
    return status;
  });
}
