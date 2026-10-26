import { expect, seedVerifiedUser, test } from './fixtures.ts';
import { accessToken, callApi, createCard, invitationToken, seedActiveStudent, tutorCode } from './tutoring.ts';

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
