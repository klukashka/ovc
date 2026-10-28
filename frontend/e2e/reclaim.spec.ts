import type { Browser, Locator, Page } from '@playwright/test';

import { type TestAccount, emailedLinkTo, newTestAccount, signInThroughTheApi } from './accounts.ts';
import type { Deployment } from './deployment.ts';
import type { Diagnostics, PageDiagnostics, TestControl } from './fixtures.ts';
import { diagnosedTest as test, expect, seedVerifiedUser } from './fixtures.ts';
import {
  callApi,
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

async function confirmWithANewPassword(page: Page, control: TestControl, owner: TestAccount): Promise<void> {
  await page.goto(emailedLinkTo(await control.mail(owner.email), '/verify-email'));
  await page.getByLabel('Новый пароль').fill(`${owner.password} reclaimed`);
  await page.getByRole('button', { name: 'Подтвердить и войти' }).click();
}

function linksBeforeConfirmation(page: Page): Locator {
  return page.getByRole('region', { name: 'Репетиторы, добавленные до подтверждения' });
}

function requestsBeforeConfirmation(page: Page): Locator {
  return page.getByRole('region', { name: 'Запросы, отправленные до подтверждения' });
}

async function keepMariaAndDropPavel(page: Page): Promise<void> {
  await linksBeforeConfirmation(page).getByRole('group', { name: 'Мария' }).getByLabel('Оставить').check();
  await requestsBeforeConfirmation(page).getByRole('group', { name: 'Pavel' }).getByLabel('Удалить').check();
  await page.getByRole('button', { name: 'Сохранить выбор' }).click();
}

async function openTheCardAsMaria(
  browser: Browser,
  deployment: Deployment,
  diagnostics: Diagnostics,
  maria: TestAccount,
  cardId: string,
): Promise<{ readonly mariaPage: Page; readonly close: () => Promise<void> }> {
  const mariaContext = await browser.newContext({
    baseURL: deployment.origin,
    ignoreHTTPSErrors: true,
    locale: 'ru-RU',
  });
  const mariaPage = await mariaContext.newPage();
  await diagnostics.watch(mariaPage);
  await signInThroughTheApi(mariaPage, maria);
  await mariaPage.goto(`/students/${cardId}`);
  return { mariaPage, close: (): Promise<void> => diagnostics.closeContext(mariaContext) };
}

async function resetThroughTheApi(
  deployment: Deployment,
  control: TestControl,
  account: TestAccount,
  newPassword: string,
): Promise<void> {
  expect((await callApi(deployment, null, 'POST', '/auth/password/forgot', { email: account.email })).status).toBe(202);
  await control.tick();
  const token = new URL(emailedLinkTo(await control.mail(account.email), '/reset-password')).searchParams.get('token');
  const reset = await callApi(deployment, null, 'POST', '/auth/password/reset', { token, new_password: newPassword });
  expect(reset.status).toBe(200);
}

test('reclaim › a confirmation from another browser asks to keep or drop the link', async ({
  page,
  deployment,
  control,
  diagnostics,
}) => {
  diagnostics.expectFailure({ method: 'POST', path: '/api/v1/auth/verify-email', status: 422 });
  const maria = await seedVerifiedUser(deployment, 'Мария');
  const pavel = await seedVerifiedUser(deployment, 'Pavel', 'en');
  const { owner } = await squatOnTheInvitation(deployment, control, maria, pavel);

  await confirmWithANewPassword(page, control, owner);

  await expect(page).toHaveURL('/reclaim');
  await expect(page.getByRole('heading', { level: 1, name: 'Оставить или удалить прежние связи' })).toBeVisible();
  const account = page.getByRole('region', { name: 'Ваш аккаунт' });
  await expect(account).toContainText('Самозванец');
  await expect(account.getByRole('link', { name: 'Изменить имя и фото' })).toHaveAttribute('href', '/settings/profile');
  await expect(page.getByText(/^Выберите до \d{1,2} [а-я]+\.?, \d{2}:\d{2}: всё, что вы не оставите/)).toBeVisible();
  await expect(linksBeforeConfirmation(page).getByRole('group')).toHaveCount(1);
  await expect(requestsBeforeConfirmation(page).getByRole('group')).toHaveCount(1);
});

test('reclaim › the tutors see the squatter neither before nor after the choice, and the kept link becomes the student’s', async ({
  page,
  browser,
  deployment,
  control,
  diagnostics,
}) => {
  diagnostics.expectFailure({ method: 'POST', path: '/api/v1/auth/verify-email', status: 422 });
  const maria = await seedVerifiedUser(deployment, 'Мария');
  const pavel = await seedVerifiedUser(deployment, 'Pavel', 'en');
  const { owner, cardId } = await squatOnTheInvitation(deployment, control, maria, pavel);
  await confirmWithANewPassword(page, control, owner);
  await expect(page).toHaveURL('/reclaim');
  const { mariaPage, close } = await openTheCardAsMaria(browser, deployment, diagnostics, maria, cardId);
  await expect(mariaPage.getByRole('main').getByText('Ждём ответа ученика')).toBeVisible();
  await expect(mariaPage.getByText('Самозванец')).toHaveCount(0);
  expect(await visibleJoinRequests(deployment, pavel)).toBe(0);
  await keepMariaAndDropPavel(page);
  await expect(page.getByRole('status')).toHaveText('Теперь вы занимаетесь с репетитором.');
  await expect(page.getByRole('status')).toBeFocused();

  await mariaPage.reload();

  await expect(mariaPage.getByRole('heading', { level: 1, name: 'Анна' })).toBeVisible();
  await expect(mariaPage.getByRole('main').getByText('Ждём ответа ученика')).toHaveCount(0);
  expect(await visibleJoinRequests(deployment, pavel)).toBe(0);
  expect(await myTutorNames(deployment, { ...owner, password: `${owner.password} reclaimed` })).toEqual(['Мария']);
  await close();
});

test('reclaim › after the choice Continue leads Home to the kept tutor and the choice page has nothing left', async ({
  page,
  deployment,
  control,
  diagnostics,
}) => {
  diagnostics.expectFailure({ method: 'POST', path: '/api/v1/auth/verify-email', status: 422 });
  const maria = await seedVerifiedUser(deployment, 'Мария');
  const pavel = await seedVerifiedUser(deployment, 'Pavel', 'en');
  const { owner } = await squatOnTheInvitation(deployment, control, maria, pavel);
  await confirmWithANewPassword(page, control, owner);
  await expect(page).toHaveURL('/reclaim');
  await keepMariaAndDropPavel(page);
  await expect(page.getByRole('status')).toHaveText('Теперь вы занимаетесь с репетитором.');
  await page.getByRole('main').getByRole('link', { name: 'Продолжить' }).click();
  await expect(page).toHaveURL('/home');
  await expect(page.getByRole('list', { name: 'Репетиторы' }).getByRole('listitem')).toHaveText([/Мария/]);

  await page.goto('/reclaim');

  await expect(page.getByRole('status')).toHaveText('Выбирать больше нечего.');
  await expect(page.getByRole('main').getByRole('link', { name: 'Продолжить' })).toHaveAttribute('href', '/home');
});

async function reachTheRefusal(
  page: Page,
  deployment: Deployment,
  control: TestControl,
  diagnostics: PageDiagnostics,
): Promise<TestAccount> {
  diagnostics.expectFailure({ method: 'POST', path: '/api/v1/auth/verify-email', status: 422 });
  diagnostics.expectFailure({ method: 'POST', path: '/api/v1/users/me/reclaim', status: 404 });
  const maria = await seedVerifiedUser(deployment, 'Мария');
  const pavel = await seedVerifiedUser(deployment, 'Pavel', 'en');
  const { owner, cardId } = await squatOnTheInvitation(deployment, control, maria, pavel);
  await confirmWithANewPassword(page, control, owner);
  await expect(page).toHaveURL('/reclaim');
  await linksBeforeConfirmation(page).getByRole('group', { name: 'Мария' }).getByLabel('Оставить').check();
  await requestsBeforeConfirmation(page).getByRole('group', { name: 'Pavel' }).getByLabel('Оставить').check();
  await removeStudent(deployment, maria, cardId);
  return pavel;
}

test('reclaim › a refusal reloads the list', async ({ page, deployment, control, diagnostics }) => {
  await reachTheRefusal(page, deployment, control, diagnostics);

  await page.getByRole('button', { name: 'Сохранить выбор' }).click();

  await expect(page.getByRole('alert')).toHaveText('Список изменился. Проверьте выбор и сохраните ещё раз.');
  await expect(page.getByRole('alert')).toBeFocused();
  await expect(linksBeforeConfirmation(page)).toHaveCount(0);
  await expect(
    requestsBeforeConfirmation(page).getByRole('group', { name: 'Pavel' }).getByLabel('Оставить'),
  ).toBeChecked();
});

test('reclaim › after a refusal the reloaded choice saves', async ({ page, deployment, control, diagnostics }) => {
  const pavel = await reachTheRefusal(page, deployment, control, diagnostics);
  await page.getByRole('button', { name: 'Сохранить выбор' }).click();
  await expect(page.getByRole('alert')).toHaveText('Список изменился. Проверьте выбор и сохраните ещё раз.');

  await page.getByRole('button', { name: 'Сохранить выбор' }).click();

  await expect(page.getByRole('status')).toHaveText('Запрос отправлен. Ждём ответа репетитора.');
  expect(await visibleJoinRequests(deployment, pavel)).toBe(1);
});

test('reclaim › a reset of an unconfirmed account asks for the choice after sign-in', async ({
  page,
  deployment,
  control,
}) => {
  const maria = await seedVerifiedUser(deployment, 'Мария');
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
});

test('reclaim › an invitation accepted while the choice waits leaves the choice waiting', async ({
  page,
  deployment,
  control,
}) => {
  const maria = await seedVerifiedUser(deployment, 'Мария');
  const pavel = await seedVerifiedUser(deployment, 'Pavel', 'en');
  const { owner } = await squatOnTheInvitation(deployment, control, maria, null);
  const resetOwner = { ...owner, password: `${owner.password} reset` };
  await control.advance(61);
  await resetThroughTheApi(deployment, control, owner, resetOwner.password);
  await signInThroughTheApi(page, resetOwner);
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
