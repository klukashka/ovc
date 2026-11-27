import { randomBytes } from 'node:crypto';

import { expect, test, testNetAddress } from './fixtures.ts';

const password = 'probe horse battery staple';

async function registerTutor(apiUrl: string, origin: string, email: string): Promise<void> {
  const response = await fetch(`${apiUrl}/api/v1/auth/register`, {
    signal: AbortSignal.timeout(10_000),
    method: 'POST',
    headers: { 'Content-Type': 'application/json', Origin: origin, 'X-Forwarded-For': testNetAddress() },
    body: JSON.stringify({
      role: 'tutor',
      name: 'A language tutor',
      email,
      password,
      language: 'en',
      timezone: 'Europe/Moscow',
      consent: true,
    }),
  });
  expect(response.status).toBe(202);
}

test('the browser keeps the host-only secure refresh cookie on the loopback preview', async ({
  page,
  deployment,
  control,
}) => {
  const email = `probe-${randomBytes(4).toString('hex')}@example.org`;
  await registerTutor(deployment.apiUrl, deployment.origin, email);
  await control.tick();
  const [confirmation] = await control.mail(email);
  const link = confirmation?.links.find((candidate) => candidate.includes('token='));
  if (link === undefined) {
    throw new Error('the confirmation email carries no token link');
  }
  const token = new URL(link).searchParams.get('token');
  if (token === null) {
    throw new Error('the confirmation link carries no token');
  }
  await page.goto('/favicon.svg');

  const statuses = await page.evaluate(
    async ({ confirmationToken, newPassword }) => {
      const confirmed = await fetch('/api/v1/auth/verify-email', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ token: confirmationToken, new_password: newPassword }),
      });
      const refreshed = await fetch('/api/v1/auth/refresh', { method: 'POST' });
      return [confirmed.status, refreshed.status];
    },
    { confirmationToken: token, newPassword: password },
  );

  const refresh = (await page.context().cookies()).find((cookie) => cookie.name === '__Host-ovc_refresh');
  expect(statuses).toEqual([200, 200]);
  expect(refresh).toMatchObject({ domain: '127.0.0.1', path: '/', secure: true, httpOnly: true, sameSite: 'Strict' });
});
