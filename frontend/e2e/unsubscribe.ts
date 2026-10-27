import type { TestAccount } from './accounts.ts';
import type { Deployment } from './deployment.ts';
import type { TestControl } from './fixtures.ts';
import { accessToken, callApi, seedActiveStudent } from './tutoring.ts';

const moscowOffsetMilliseconds = 3 * 60 * 60 * 1000;
const lessonLeadMilliseconds = 3 * 60 * 60 * 1000;
const reminderLeadMilliseconds = 59 * 60 * 1000;

async function sharedClockNow(deployment: Deployment): Promise<number> {
  const response = await fetch(`${deployment.apiUrl}/__test__/clock`, {
    signal: AbortSignal.timeout(10_000),
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ advance_seconds: 1 }),
  });
  if (!response.ok) {
    throw new Error(`/__test__/clock answered ${String(response.status)}`);
  }
  const body: unknown = await response.json();
  if (typeof body !== 'object' || body === null || !('now' in body) || typeof body.now !== 'string') {
    throw new Error('the clock route answered an unexpected shape');
  }
  return Date.parse(body.now);
}

function moscowDateAndTime(instant: number): { readonly date: string; readonly start: string } {
  const local = new Date(instant + moscowOffsetMilliseconds).toISOString();
  return { date: local.slice(0, 10), start: local.slice(11, 16) };
}

export async function unsubscribeTokenFromAReminder(
  deployment: Deployment,
  control: TestControl,
  tutor: TestAccount,
): Promise<string> {
  const { cardId } = await seedActiveStudent(deployment, tutor, 'Reminded student');
  const now = await sharedClockNow(deployment);
  const startsAt = Math.ceil((now + lessonLeadMilliseconds) / 60_000) * 60_000;
  const created = await callApi(deployment, await accessToken(deployment, tutor), 'POST', '/lessons', {
    relationship_id: cardId,
    title: 'Reminder lesson',
    ...moscowDateAndTime(startsAt),
    duration_minutes: 60,
    allow_overlap: true,
  });
  if (created.status !== 201) {
    throw new Error(`scheduling the reminded lesson answered ${String(created.status)}`);
  }
  await control.advance(Math.ceil((startsAt - reminderLeadMilliseconds - now) / 1000));
  await control.tick();
  const links = (await control.mail(tutor.email)).flatMap((message) => message.links);
  const token = links
    .map((link) => new URL(link))
    .filter((link) => link.pathname === '/unsubscribe')
    .map((link) => link.searchParams.get('token'))
    .find((candidate): candidate is string => candidate !== null);
  if (token === undefined) {
    throw new Error('the reminder carried no unsubscribe link');
  }
  return token;
}
