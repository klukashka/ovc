import { expect, seedVerifiedUser, test } from './fixtures.ts';
import { accessToken, callApi, createCard, invitationToken, seedActiveStudent, tutorCode } from './tutoring.ts';

import { contentSecurityPolicyWithTestRoom } from '../security-headers.ts';

test('the preview server answers no cross-origin requests', async ({ request }) => {
  const preflight = await request.fetch('/api/v1/health/live', {
    method: 'OPTIONS',
    headers: { Origin: 'http://localhost:9', 'Access-Control-Request-Method': 'GET' },
  });
  const simple = await request.get('/api/v1/health/live', { headers: { Origin: 'http://localhost:9' } });

  expect(preflight.headers()['access-control-allow-origin']).toBeUndefined();
  expect(simple.headers()['access-control-allow-origin']).toBeUndefined();
});

test('FR-1.3 › student routes refuse a tutor and tutor routes refuse a student', async ({ deployment }) => {
  const maria = await seedVerifiedUser(deployment, 'Мария');
  const pavel = await seedVerifiedUser(deployment, 'Pavel', 'en');
  const { student: anna } = await seedActiveStudent(deployment, maria, 'Анна', 'ru');
  const pavelToken = await accessToken(deployment, pavel);
  const card = await createCard(deployment, maria, 'Pavel', pavel.email);

  const codeEntry = await callApi(deployment, pavelToken, 'POST', '/join-requests', {
    code: await tutorCode(deployment, maria),
  });
  const acceptance = await callApi(
    deployment,
    pavelToken,
    'POST',
    `/invitations/${encodeURIComponent(invitationToken(card.link))}/accept`,
    {},
  );
  const tutorList = await callApi(deployment, pavelToken, 'GET', '/tutors');
  const requestList = await callApi(deployment, await accessToken(deployment, anna), 'GET', '/join-requests');

  expect(codeEntry.status).toBe(409);
  expect(codeEntry.body).toMatchObject({ error: { code: 'role_mismatch' } });
  expect(acceptance.status).toBe(409);
  expect(acceptance.body).toMatchObject({ error: { code: 'role_mismatch' } });
  expect(tutorList.status).toBe(404);
  expect(requestList.status).toBe(404);
});

test('the room origin forwards only the room, served with the no-store, no-referrer and framing headers', async ({
  request,
  deployment,
}) => {
  const health = await request.get(`${deployment.roomOrigin}/api/v1/health/ready`);
  const mail = await request.get(`${deployment.roomOrigin}/__test__/mail`);
  const room = await request.get(`${deployment.roomOrigin}/__test__/fake-room`);

  const headers = room.headers();
  expect(health.status()).toBe(404);
  expect(mail.status()).toBe(404);
  expect(room.status()).toBe(200);
  expect(headers['cache-control']).toBe('no-store');
  expect(headers['referrer-policy']).toBe('no-referrer');
  expect(headers['x-content-type-options']).toBe('nosniff');
  expect(headers['content-security-policy']).toContain(`frame-ancestors ${deployment.origin}`);
  expect(headers['content-security-policy']).toContain("default-src 'none'");
  expect(headers['x-frame-options']).toBeUndefined();
  expect(headers['access-control-allow-origin']).toBeUndefined();
});

test('no recording control exists', async ({ request, deployment }) => {
  const lesson = '00000000-0000-4000-8000-000000000001';

  const recording = await request.post(`${deployment.apiUrl}/__test__/fake-provider/lessons/${lesson}/recording`, {
    data: {},
  });
  const ready = await request.post(`${deployment.apiUrl}/__test__/fake-provider/lessons/${lesson}/ready`, { data: {} });

  expect(recording.status()).toBe(404);
  expect(ready.status()).toBe(404);
});

test('every page, asset and proxied API response carries the security headers', async ({ request, deployment }) => {
  const today = await request.get('/today');
  const live = await request.get('/api/v1/health/live');
  const entry = await request.get('/');
  const script = /<script type="module"[^>]* src="([^"]+)"/.exec(await entry.text())?.[1];
  expect(script).toMatch(/^\/assets\/.+\.js$/);
  const paths = [
    '/students/1',
    '/no/such/page',
    '/theme-init.js',
    '/favicon.svg',
    String(script),
    '/api/v1/no-such-route',
  ];

  const responses = await Promise.all(paths.map((path) => request.get(path)));

  expect(today.headers()['content-security-policy']).toBe(contentSecurityPolicyWithTestRoom(deployment.roomOrigin));
  const checked = [
    { path: '/today', response: today },
    { path: '/api/v1/health/live', response: live },
    ...paths.map((path, index) => ({ path, response: responses[index] })),
  ];
  for (const { path, response } of checked) {
    const headers = response?.headers() ?? {};
    expect(headers['referrer-policy'], path).toBe('no-referrer');
    expect(headers['x-content-type-options'], path).toBe('nosniff');
  }
  expect(live.status()).toBe(200);
  expect(responses.at(-1)?.status()).toBe(404);
});

test('the preview server has no build manifest to serve', async ({ request, deployment }) => {
  const addresses = ['/.vite/manifest.json', '//.vite/manifest.json', '//%2Evite/manifest.json'];

  const bodies = await Promise.all(
    addresses.map(async (address) => (await request.get(`${deployment.origin}${address}`)).text()),
  );

  expect(bodies.filter((body) => body.includes('isEntry'))).toEqual([]);
});
