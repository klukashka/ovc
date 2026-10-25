import { signInThroughTheApi } from './accounts.ts';
import type { TestAccount } from './accounts.ts';
import type { Deployment } from './deployment.ts';
import { diagnosedTest as test, expect, seedVerifiedUser } from './fixtures.ts';
import { busyControls, recordBusyControls } from './observers.ts';
import { accessToken, callApi, deleteAccount, seedBlockedStudent, seedBlockedStudents, tutorCode } from './tutoring.ts';

const codePattern = /^[2-9A-HJKMNP-Z]{4}-[2-9A-HJKMNP-Z]{4}$/;
const dailyRegenerations = 10;
const pageSize = 30;

async function unblockThroughTheApi(deployment: Deployment, tutor: TestAccount, name: string): Promise<string> {
  const token = await accessToken(deployment, tutor);
  const listed = await callApi(deployment, token, 'GET', '/tutor/blocked-requesters');
  const items: unknown =
    typeof listed.body === 'object' && listed.body !== null && 'items' in listed.body ? listed.body.items : [];
  const found: unknown = (Array.isArray(items) ? items : []).find(
    (item: unknown) => typeof item === 'object' && item !== null && 'name' in item && item.name === name,
  );
  const id =
    typeof found === 'object' && found !== null && 'student_id' in found && typeof found.student_id === 'string'
      ? found.student_id
      : '';
  expect((await callApi(deployment, token, 'DELETE', `/tutor/blocked-requesters/${id}`)).status).toBe(204);
  return id;
}

test.use({ locale: 'ru-RU' });

test('teaching settings › code and regenerate', async ({ page, browserName, deployment }) => {
  const maria = await seedVerifiedUser(deployment, 'Мария');
  const before = await tutorCode(deployment, maria);
  if (browserName === 'chromium') {
    await page.context().grantPermissions(['clipboard-read', 'clipboard-write']);
  }
  await signInThroughTheApi(page, maria);
  await page.goto('/settings/teaching');
  const section = page.getByRole('region', { name: 'Код репетитора' });
  await expect(section.getByText(codePattern)).toHaveText(before);

  await section.getByRole('button', { name: 'Скопировать код' }).click();
  await expect(section.getByRole('status')).toHaveText('Код скопирован.');
  await expect(section.getByRole('status')).toBeFocused();
  if (browserName === 'chromium') {
    expect(await page.evaluate(() => navigator.clipboard.readText())).toBe(before);
  }
  await section.getByRole('button', { name: 'Обновить код' }).click();

  await expect(section.getByRole('status')).toHaveText('Создан новый код. Старый больше не работает.');
  await expect(section.getByRole('status')).toBeFocused();
  const after = await section.getByText(codePattern).textContent();
  expect(after).not.toBe(before);
  expect(await tutorCode(deployment, maria)).toBe(after);
});

test('teaching settings › the daily regeneration limit is explained', async ({ page, deployment, diagnostics }) => {
  diagnostics.expectFailure({ method: 'POST', path: '/api/v1/tutor/code/regenerate', status: 429 });
  const maria = await seedVerifiedUser(deployment, 'Мария');
  const token = await accessToken(deployment, maria);
  for (let attempt = 0; attempt < dailyRegenerations; attempt += 1) {
    expect((await callApi(deployment, token, 'POST', '/tutor/code/regenerate')).status).toBe(200);
  }
  await signInThroughTheApi(page, maria);
  await page.goto('/settings/teaching');
  const section = page.getByRole('region', { name: 'Код репетитора' });

  await section.getByRole('button', { name: 'Обновить код' }).click();

  await expect(section.getByRole('alert')).toHaveText(
    'На сегодня достигнут предел обновлений кода. Попробуйте завтра.',
  );
  await expect(section.getByRole('alert')).toBeFocused();
});

test('teaching settings › blocked students and unblock', async ({ page, deployment, control }) => {
  const maria = await seedVerifiedUser(deployment, 'Мария');
  await recordBusyControls(page);
  await signInThroughTheApi(page, maria);
  await page.goto('/settings/teaching');
  const section = page.getByRole('region', { name: 'Заблокированные ученики' });
  await expect(section.getByText('Вы никого не заблокировали.')).toBeVisible();
  const blocked = await seedBlockedStudent(deployment, control, maria, 'Назойливый ученик');

  await page.reload();
  await expect(section.getByText(blocked.name)).toBeVisible();
  await section.getByRole('button', { name: `Разблокировать: ${blocked.name}` }).click();

  await expect(section.getByRole('status')).toHaveText('Ученик разблокирован.');
  await expect(section.getByRole('status')).toBeFocused();
  await expect(section.getByText('Вы никого не заблокировали.')).toBeVisible();
  expect(await busyControls(page)).toEqual([[`Разблокировать: ${blocked.name}`]]);
});

test('teaching settings › an unblock of a student unblocked meanwhile is refused with an alert that takes focus', async ({
  page,
  deployment,
  control,
  diagnostics,
}) => {
  const maria = await seedVerifiedUser(deployment, 'Мария');
  const first = await seedBlockedStudent(deployment, control, maria, 'Первый разблокированный ученик');
  await seedBlockedStudent(deployment, control, maria, 'Второй разблокированный ученик');
  await signInThroughTheApi(page, maria);
  await page.goto('/settings/teaching');
  const section = page.getByRole('region', { name: 'Заблокированные ученики' });
  await expect(section.getByRole('button', { name: /^Разблокировать: / })).toHaveCount(2);
  const firstId = await unblockThroughTheApi(deployment, maria, first.name);
  diagnostics.expectFailure({ method: 'DELETE', path: `/api/v1/tutor/blocked-requesters/${firstId}`, status: 404 });

  await section.getByRole('button', { name: `Разблокировать: ${first.name}` }).click();

  await expect(section.getByRole('alert')).toHaveText('Этот ученик уже не заблокирован. Список обновлён.');
  await expect(section.getByRole('alert')).toBeFocused();
  await expect(section.getByRole('button', { name: /^Разблокировать: / })).toHaveCount(1);
});

test('teaching settings › a second refused unblock while the alert shows takes focus again', async ({
  page,
  deployment,
  control,
  diagnostics,
}) => {
  const maria = await seedVerifiedUser(deployment, 'Мария');
  const first = await seedBlockedStudent(deployment, control, maria, 'Первый разблокированный ученик');
  const second = await seedBlockedStudent(deployment, control, maria, 'Второй разблокированный ученик');
  await signInThroughTheApi(page, maria);
  await page.goto('/settings/teaching');
  const section = page.getByRole('region', { name: 'Заблокированные ученики' });
  const alert = section.getByRole('alert');
  await expect(section.getByRole('button', { name: /^Разблокировать: / })).toHaveCount(2);
  const firstId = await unblockThroughTheApi(deployment, maria, first.name);
  diagnostics.expectFailure({ method: 'DELETE', path: `/api/v1/tutor/blocked-requesters/${firstId}`, status: 404 });
  await section.getByRole('button', { name: `Разблокировать: ${first.name}` }).click();
  await expect(section.getByRole('button', { name: /^Разблокировать: / })).toHaveCount(1);
  const secondId = await unblockThroughTheApi(deployment, maria, second.name);
  diagnostics.expectFailure({ method: 'DELETE', path: `/api/v1/tutor/blocked-requesters/${secondId}`, status: 404 });

  await section.getByRole('button', { name: `Разблокировать: ${second.name}` }).click();

  await expect(alert).toHaveText('Этот ученик уже не заблокирован. Список обновлён.');
  await expect(alert).toBeFocused();
  await expect(section.getByText('Вы никого не заблокировали.')).toBeVisible();
  await expect(section.getByRole('status')).toHaveCount(0);
});

test('teaching settings › a blocked student who deleted their account is listed without the account', async ({
  page,
  deployment,
  control,
}) => {
  const maria = await seedVerifiedUser(deployment, 'Мария');
  const blocked = await seedBlockedStudent(deployment, control, maria, 'Удаливший аккаунт ученик');
  await deleteAccount(deployment, blocked);
  await signInThroughTheApi(page, maria);

  await page.goto('/settings/teaching');

  const section = page.getByRole('region', { name: 'Заблокированные ученики' });
  await expect(section.getByText('Удалённый аккаунт')).toBeVisible();
  await expect(section.getByText(blocked.name)).toHaveCount(0);
  await expect(
    section.getByRole('button', { name: /^Разблокировать удалённый аккаунт, заблокированный \d{1,2} \S+ \d{4}/ }),
  ).toBeVisible();
});

test('teaching settings › show more moves focus to the first new blocked student', async ({
  page,
  deployment,
  control,
}) => {
  const maria = await seedVerifiedUser(deployment, 'Мария');
  await seedBlockedStudents(deployment, control, maria, pageSize + 1);
  await signInThroughTheApi(page, maria);
  await page.goto('/settings/teaching');
  const section = page.getByRole('region', { name: 'Заблокированные ученики' });
  const unblockButtons = section.getByRole('button', { name: /^Разблокировать: / });
  await expect(unblockButtons).toHaveCount(pageSize);

  await section.getByRole('button', { name: 'Показать ещё' }).click();

  await expect(unblockButtons).toHaveCount(pageSize + 1);
  await expect(unblockButtons.nth(pageSize)).toBeFocused();
  await expect(section.getByRole('button', { name: 'Показать ещё' })).toHaveCount(0);
});
