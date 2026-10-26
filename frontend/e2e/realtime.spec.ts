import type { Page, WebSocket } from '@playwright/test';

import { newTestAccount, signInThroughTheApi } from './accounts.ts';
import { diagnosedTest as test, expect, seedVerifiedUser } from './fixtures.ts';
import { accountId, closeSockets, waitForSockets } from './realtime.ts';
import { accessToken, callApi, createCard, removeStudent, seedJoinRequests, signUpOnInvitation } from './tutoring.ts';

function socketAddresses(page: Page): string[] {
  const addresses: string[] = [];
  page.on('websocket', (socket: WebSocket) => {
    addresses.push(socket.url());
  });
  return addresses;
}

interface ReconnectRecord {
  readonly openSockets: () => number;
  readonly socketCounts: readonly number[];
  readonly joinRequestReads: () => number;
  readonly readsBeforeSocket: readonly number[];
}

function recordSocketsAndJoinRequestReads(page: Page): ReconnectRecord {
  const socketCounts: number[] = [];
  const readsBeforeSocket: number[] = [];
  let open = 0;
  let reads = 0;
  page.on('request', (request) => {
    if (new URL(request.url()).pathname === '/api/v1/join-requests' && request.method() === 'GET') {
      reads += 1;
    }
  });
  page.on('websocket', (socket: WebSocket) => {
    readsBeforeSocket.push(reads);
    open += 1;
    socketCounts.push(open);
    socket.on('close', () => {
      open -= 1;
      socketCounts.push(open);
    });
  });
  return { openSockets: () => open, socketCounts, joinRequestReads: () => reads, readsBeforeSocket };
}

for (const code of [4429, 1012] as const) {
  test(`realtime › reconnects and refetches after the server closes with ${String(code)}`, async ({
    page,
    deployment,
  }) => {
    const maria = await seedVerifiedUser(deployment, 'Мария');
    const user = await accountId(deployment, maria);
    await signInThroughTheApi(page, maria);
    const record = recordSocketsAndJoinRequestReads(page);
    await page.goto('/today');
    await expect(page.getByRole('region', { name: 'Заявки на занятия' })).toBeVisible();
    await waitForSockets(deployment, user, 1);

    const closed = await closeSockets(deployment, user, code);

    await expect.poll(() => record.socketCounts.length).toBe(3);
    await waitForSockets(deployment, user, 1);
    const readsBeforeReconnect = record.readsBeforeSocket[1] ?? Number.POSITIVE_INFINITY;
    await expect.poll(() => record.joinRequestReads()).toBeGreaterThan(readsBeforeReconnect);
    expect(closed).toBe(1);
    expect(record.socketCounts).toEqual([1, 0, 1]);
    expect(record.openSockets()).toBe(1);
  });
}

test('realtime › the socket address carries no token and no CSP violation occurs', async ({ page, deployment }) => {
  const maria = await seedVerifiedUser(deployment, 'Мария');
  const addresses = socketAddresses(page);
  await signInThroughTheApi(page, maria);
  const opened = page.waitForEvent('websocket');

  await page.goto('/students');

  await opened;
  await waitForSockets(deployment, await accountId(deployment, maria), 1);
  expect(addresses).toEqual([new URL('/api/v1/ws', deployment.origin.replace(/^https:/, 'wss:')).toString()]);
});

test('realtime › signing out everywhere in another browser signs this tab out with a message', async ({
  page,
  deployment,
  diagnostics,
}) => {
  const maria = await seedVerifiedUser(deployment, 'Мария');
  const user = await accountId(deployment, maria);
  await signInThroughTheApi(page, maria);
  await page.goto('/students');
  await waitForSockets(deployment, user, 1);
  diagnostics.expectFailure({ method: 'POST', path: '/api/v1/auth/refresh', status: 401 });

  const signedOut = await callApi(deployment, await accessToken(deployment, maria), 'POST', '/auth/logout-all');

  expect(signedOut.status).toBe(204);
  await expect(page.getByRole('status')).toHaveText('Вы вышли из аккаунта на этом устройстве');
  await expect(page).toHaveURL('/');
  await expect(page.getByRole('heading', { level: 1, name: 'Вход' })).toBeVisible();
  await waitForSockets(deployment, user, 0);
});

test("realtime › a tutor's Students page and Today show a join request without reloading", async ({
  page,
  deployment,
  control,
}) => {
  const maria = await seedVerifiedUser(deployment, 'Мария');
  const user = await accountId(deployment, maria);
  await signInThroughTheApi(page, maria);
  await page.goto('/students');
  const today = await page.context().newPage();
  await today.goto('/today');
  await expect(today.getByRole('region', { name: 'Заявки на занятия' })).toBeVisible();
  await waitForSockets(deployment, user, 2);

  await seedJoinRequests(deployment, control, maria, 1);

  for (const shown of [page, today]) {
    await expect(shown.getByRole('region', { name: 'Заявки на занятия' }).getByRole('listitem')).toHaveCount(1);
  }
});

test("realtime › a student's My tutors drops a tutor who removed the link without reloading", async ({
  page,
  deployment,
}) => {
  const pavel = await seedVerifiedUser(deployment, 'Pavel', 'en');
  const boris = newTestAccount('Boris', 'student');
  const card = await createCard(deployment, pavel, 'Boris', boris.email);
  await signUpOnInvitation(deployment, card.link, boris, 'en');
  await signInThroughTheApi(page, boris);
  await page.goto('/tutors');
  await expect(page.getByRole('list', { name: 'Tutors', exact: true }).getByRole('listitem')).toHaveCount(1);
  await waitForSockets(deployment, await accountId(deployment, boris), 1);
  await page.getByRole('button', { name: 'More actions for Pavel' }).focus();

  await removeStudent(deployment, pavel, card.id);

  await expect(page.getByRole('heading', { level: 1, name: 'My tutors' })).toBeFocused();
  await expect(page.getByRole('main').getByText('No tutor yet.')).toBeVisible();
  await expect(page.getByRole('list', { name: 'Tutors', exact: true })).toHaveCount(0);
});
