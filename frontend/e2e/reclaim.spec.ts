import { type TestAccount, emailedLinkTo, newTestAccount, signInThroughTheApi } from './accounts.ts';
import type { Deployment } from './deployment.ts';
import type { TestControl } from './fixtures.ts';
import { diagnosedTest as test, expect, seedVerifiedUser } from './fixtures.ts';
import {
  createCard,
  enterTutorCode,
  myTutorNames,
  registerStudentWithCode,
  removeStudent,
  signUpOnInvitation,
  tutorCode,
  visibleJoinRequests,
} from './tutoring.ts';

test.use({ locale: 'ru-RU', colorScheme: 'light' });

test('reclaim › a confirmation from another browser asks to keep or drop the link', async ({
  page,
  browser,
  deployment,
  control,
  diagnostics,
}) => {
  diagnostics.expectFailure({ method: 'POST', path: '/api/v1/auth/verify-email', status: 422 });
  const maria = await seedVerifiedUser(deployment, 'Мария');
  const pavel = await seedVerifiedUser(deployment, 'Pavel', 'en');
  const owner = newTestAccount('Самозванец', 'student');
  const card = await createCard(deployment, maria, 'Анна', owner.email);
  await signUpOnInvitation(deployment, card.link, owner, 'ru');
  await enterTutorCode(deployment, owner, await tutorCode(deployment, pavel));
  await control.tick();
  const confirmation = emailedLinkTo(await control.mail(owner.email), '/verify-email');

  await page.goto(confirmation);
  await page.getByLabel('Новый пароль').fill(`${owner.password} reclaimed`);
  await page.getByRole('button', { name: 'Подтвердить и войти' }).click();

  await expect(page).toHaveURL('/reclaim');
  await expect(page.getByRole('heading', { level: 1, name: 'Оставить или удалить прежние связи' })).toBeVisible();
  const account = page.getByRole('region', { name: 'Ваш аккаунт' });
  await expect(account).toContainText('Самозванец');
  await expect(account.getByRole('link', { name: 'Изменить имя и фото' })).toHaveAttribute('href', '/settings/profile');
  await expect(page.getByText(/^Выберите до \d{1,2} [а-я]+\.?, \d{2}:\d{2}: всё, что вы не оставите/)).toBeVisible();
  const links = page.getByRole('region', { name: 'Репетиторы, добавленные до подтверждения' });
  const requests = page.getByRole('region', { name: 'Запросы, отправленные до подтверждения' });
  await expect(links.getByRole('group')).toHaveCount(1);
  await expect(requests.getByRole('group')).toHaveCount(1);
  const mariaContext = await browser.newContext({
    baseURL: deployment.origin,
    ignoreHTTPSErrors: true,
    locale: 'ru-RU',
  });
  const mariaPage = await mariaContext.newPage();
  await diagnostics.watch(mariaPage);
  await signInThroughTheApi(mariaPage, maria);
  await mariaPage.goto(`/students/${card.id}`);
  await expect(mariaPage.getByRole('main').getByText('Ждём ответа ученика')).toBeVisible();
  await expect(mariaPage.getByText('Самозванец')).toHaveCount(0);
  expect(await visibleJoinRequests(deployment, pavel)).toBe(0);

  await links.getByRole('group', { name: 'Мария' }).getByLabel('Оставить').check();
  await requests.getByRole('group', { name: 'Pavel' }).getByLabel('Удалить').check();
  await page.getByRole('button', { name: 'Сохранить выбор' }).click();

  await expect(page.getByRole('status')).toHaveText('Теперь вы занимаетесь с репетитором.');
  await expect(page.getByRole('status')).toBeFocused();
  await page.getByRole('main').getByRole('link', { name: 'Продолжить' }).click();
  await expect(page).toHaveURL('/home');
  await expect(page.getByRole('list', { name: 'Репетиторы' }).getByRole('listitem')).toHaveText([/Мария/]);
  await mariaPage.reload();
  await expect(mariaPage.getByRole('heading', { level: 1, name: 'Анна' })).toBeVisible();
  await expect(mariaPage.getByRole('main').getByText('Ждём ответа ученика')).toHaveCount(0);
  expect(await visibleJoinRequests(deployment, pavel)).toBe(0);
  expect(await myTutorNames(deployment, { ...owner, password: `${owner.password} reclaimed` })).toEqual(['Мария']);
  await page.goto('/reclaim');
  await expect(page.getByRole('status')).toHaveText('Выбирать больше нечего.');
  await expect(page.getByRole('main').getByRole('link', { name: 'Продолжить' })).toHaveAttribute('href', '/home');
  await mariaContext.close();
});

async function squatOnTheInvitation(
  deployment: Deployment,
  control: TestControl,
  maria: TestAccount,
  pavel: TestAccount | null,
): Promise<{ readonly owner: TestAccount; readonly cardId: string }> {
  const owner = newTestAccount('Самозванец', 'student');
  const card = await createCard(deployment, maria, 'Анна', owner.email);
  await signUpOnInvitation(deployment, card.link, owner, 'ru');
  if (pavel !== null) {
    await enterTutorCode(deployment, owner, await tutorCode(deployment, pavel));
  }
  await control.tick();
  return { owner, cardId: card.id };
}

test('reclaim › a refusal reloads the list', async ({ page, deployment, control, diagnostics }) => {
  diagnostics.expectFailure({ method: 'POST', path: '/api/v1/auth/verify-email', status: 422 });
  diagnostics.expectFailure({ method: 'POST', path: '/api/v1/users/me/reclaim', status: 404 });
  const maria = await seedVerifiedUser(deployment, 'Мария');
  const pavel = await seedVerifiedUser(deployment, 'Pavel', 'en');
  const { owner, cardId } = await squatOnTheInvitation(deployment, control, maria, pavel);
  await page.goto(emailedLinkTo(await control.mail(owner.email), '/verify-email'));
  await page.getByLabel('Новый пароль').fill(`${owner.password} reclaimed`);
  await page.getByRole('button', { name: 'Подтвердить и войти' }).click();
  await expect(page).toHaveURL('/reclaim');
  const links = page.getByRole('region', { name: 'Репетиторы, добавленные до подтверждения' });
  const requests = page.getByRole('region', { name: 'Запросы, отправленные до подтверждения' });
  await links.getByRole('group', { name: 'Мария' }).getByLabel('Оставить').check();
  await requests.getByRole('group', { name: 'Pavel' }).getByLabel('Оставить').check();
  await removeStudent(deployment, maria, cardId);

  await page.getByRole('button', { name: 'Сохранить выбор' }).click();

  await expect(page.getByRole('alert')).toHaveText('Список изменился. Проверьте выбор и сохраните ещё раз.');
  await expect(page.getByRole('alert')).toBeFocused();
  await expect(links).toHaveCount(0);
  await expect(requests.getByRole('group', { name: 'Pavel' }).getByLabel('Оставить')).toBeChecked();
  await page.getByRole('button', { name: 'Сохранить выбор' }).click();
  await expect(page.getByRole('status')).toHaveText('Запрос отправлен. Ждём ответа репетитора.');
  expect(await visibleJoinRequests(deployment, pavel)).toBe(1);
});

test('reclaim › a reset of an unconfirmed account asks for the choice after sign-in, and an invitation can be accepted meanwhile', async ({
  page,
  deployment,
  control,
}) => {
  const maria = await seedVerifiedUser(deployment, 'Мария');
  const pavel = await seedVerifiedUser(deployment, 'Pavel', 'en');
  const { owner } = await squatOnTheInvitation(deployment, control, maria, null);
  const newPassword = `${owner.password} reset`;
  await control.advance(61);
  await page.goto('/forgot-password');
  await page.getByLabel('Email').fill(owner.email);
  await page.getByRole('button', { name: 'Отправить ссылку' }).click();
  await expect(page.getByRole('status')).toContainText(owner.email);
  await control.tick();
  await page.goto(emailedLinkTo(await control.mail(owner.email), '/reset-password'));
  await page.getByLabel('Новый пароль').fill(newPassword);
  await page.getByRole('button', { name: 'Сохранить пароль' }).click();

  await expect(page.getByRole('status')).toHaveText('Пароль изменён. Войдите с новым паролем.');
  await expect(page.getByText('После входа выберите, какие прежние связи и запросы оставить.')).toBeVisible();
  await page.getByRole('main').getByRole('link', { name: 'Войти', exact: true }).click();
  await page.getByLabel('Email').fill(owner.email);
  await page.getByLabel('Пароль', { exact: true }).fill(newPassword);
  await page.getByRole('button', { name: 'Войти', exact: true }).click();
  await expect(page).toHaveURL('/reclaim');
  await expect(page.getByRole('group', { name: 'Мария' })).toBeVisible();

  const card = await createCard(deployment, pavel, 'Anna', owner.email);
  await page.goto(card.link);
  await expect(page).toHaveURL('/invite');
  const waiting = page.getByRole('link', { name: 'Есть связи, которые ждут вашего выбора' });
  await expect(waiting).toHaveAttribute('href', '/reclaim');
  await page.getByRole('button', { name: 'Принять' }).click();
  await expect(page).toHaveURL('/tutors');
  await expect(page.getByRole('list', { name: 'Репетиторы' }).getByRole('listitem')).toHaveText([/Pavel/]);
  await page.goto('/reclaim');
  await expect(page.getByRole('group', { name: 'Мария' })).toBeVisible();
});

test('reclaim › keeping a request whose tutor is no longer active says that the code is not valid', async ({
  page,
  deployment,
  control,
  diagnostics,
}) => {
  diagnostics.expectFailure({ method: 'POST', path: '/api/v1/auth/verify-email', status: 422 });
  const maria = await seedVerifiedUser(deployment, 'Мария');
  const owner = newTestAccount('Самозванец', 'student');
  await registerStudentWithCode(deployment, owner, await tutorCode(deployment, maria));
  await control.tick();
  await page.goto(emailedLinkTo(await control.mail(owner.email), '/verify-email'));
  await page.getByLabel('Новый пароль').fill(`${owner.password} reclaimed`);
  await page.getByRole('button', { name: 'Подтвердить и войти' }).click();
  await expect(page).toHaveURL('/reclaim');
  const requests = page.getByRole('region', { name: 'Запросы, отправленные до подтверждения' });
  await requests.getByRole('group', { name: 'Мария' }).getByLabel('Оставить').check();
  deployment.administer(['users', 'block', maria.email]);

  await page.getByRole('button', { name: 'Сохранить выбор' }).click();

  await expect(page.getByRole('status')).toHaveText('Этот код больше не действует — введите новый.');
  await expect(page.getByRole('status')).toBeFocused();
});
