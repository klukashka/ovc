import { signInThroughTheApi } from './accounts.ts';
import { diagnosedTest as test, expect, seedVerifiedUser } from './fixtures.ts';
import { accessToken, callApi, seedBlockedStudent, seedBlockedStudents, tutorCode } from './tutoring.ts';

const codePattern = /^[2-9A-HJKMNP-Z]{4}-[2-9A-HJKMNP-Z]{4}$/;
const dailyRegenerations = 10;
const pageSize = 30;

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
