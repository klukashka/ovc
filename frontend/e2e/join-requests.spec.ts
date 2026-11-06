import type { BrowserContext, Locator, Page } from '@playwright/test';

import type { ObservedBrowser } from './browser-diagnostics.ts';

import { type TestAccount, emailedLinkTo, newTestAccount, signInThroughTheApi } from './accounts.ts';
import type { Deployment } from './deployment.ts';
import { bellIn, panelRowsIn } from './notifications.ts';
import { diagnosedTest as test, expect, seedVerifiedUser, signUpAndConfirmTutor } from './fixtures.ts';
import { busyControls, offsetsBelowHeading, recordBusyControls, recordFirstOffsets } from './observers.ts';
import {
  acceptSignedIn,
  createCard,
  decideJoinRequest,
  deleteAccount,
  endRelationship,
  enterTutorCode,
  registerStudentWithCode,
  seedActiveStudent,
  seedDeclinedRequests,
  seedJoinRequests,
  seedStudentThroughCode,
  tutorCode,
  visibleJoinRequests,
} from './tutoring.ts';

const codePattern = /^[2-9A-HJKMNP-Z]{4}-[2-9A-HJKMNP-Z]{4}$/;
const pageSize = 30;

async function tutorContext(browser: ObservedBrowser, deployment: Deployment): Promise<BrowserContext> {
  return browser.newContext({
    baseURL: deployment.origin,
    ignoreHTTPSErrors: true,
    locale: 'ru-RU',
    timezoneId: 'Europe/Moscow',
    viewport: { width: 1280, height: 800 },
    colorScheme: 'light',
  });
}

function joinRequests(page: Page): Locator {
  return page.getByRole('region', { name: 'Заявки на занятия' });
}

function sentRequests(page: Page, name: string): Locator {
  return page.getByRole('region', { name }).getByRole('listitem');
}

test.describe('Boris in English', () => {
  test.use({
    locale: 'en-GB',
    timezoneId: 'Europe/Moscow',
    viewport: { width: 1280, height: 800 },
    colorScheme: 'dark',
  });

  test('pilot-2 › steps 1–3 Boris signs up with Maria’s code, confirms, and Maria accepts him from Today', async ({
    page,
    observedBrowser: browser,
    deployment,
    control,
    diagnostics,
  }) => {
    const maria = await signUpAndConfirmTutor(deployment, 'Мария');
    const boris = newTestAccount('Boris', 'student');
    const mariaContext = await tutorContext(browser, deployment);
    const mariaPage = await mariaContext.newPage();
    await diagnostics.watch(mariaPage);
    await signInThroughTheApi(mariaPage, maria);

    await mariaPage.goto('/settings/teaching');
    const shownCode = mariaPage.getByRole('region', { name: 'Код репетитора' }).getByText(codePattern);
    await expect(shownCode).toHaveText(await tutorCode(deployment, maria));
    const code = (await shownCode.textContent()) ?? '';

    await page.goto('/signup');
    await page.getByRole('link', { name: /I study/ }).click();
    await expect(page).toHaveURL('/signup/student');
    await expect(page.getByText('Got a link from your tutor? Open it instead.')).toBeVisible();
    await page.getByLabel('Tutor code', { exact: true }).fill(code.toLowerCase());
    await page.getByLabel('Name', { exact: true }).fill(boris.name);
    await page.getByLabel('Email', { exact: true }).fill(boris.email);
    await page.getByLabel('Password', { exact: true }).fill(boris.password);
    await page.getByLabel(/I agree to the processing of my personal data/).check();
    await page.getByRole('button', { name: 'Sign up' }).click();
    await expect(page.getByRole('heading', { level: 1, name: 'Check your email' })).toBeFocused();
    await page.getByRole('main').getByRole('link', { name: 'Sign in', exact: true }).click();
    await page.getByLabel('Email', { exact: true }).fill(boris.email);
    await page.getByLabel('Password', { exact: true }).fill(boris.password);
    await page.getByRole('button', { name: 'Sign in', exact: true }).click();
    await expect(page).toHaveURL('/home');
    await expect(page.getByRole('region', { name: 'Email confirmation' })).toBeVisible();
    await expect(page.getByText('Request sent. Waiting for the tutor.')).toBeVisible();
    await mariaPage.goto('/today');
    await expect(joinRequests(mariaPage).getByText('Новых заявок нет.')).toBeVisible();
    expect(await visibleJoinRequests(deployment, maria)).toBe(0);
    const mariaBell = bellIn(mariaPage, /^Уведомления/);
    await expect(mariaBell).toHaveAccessibleName('Уведомления');

    await control.tick();
    await page.goto(emailedLinkTo(await control.mail(boris.email), '/verify-email'));
    await expect(page.getByRole('status')).toHaveText('Request sent. Waiting for the tutor.');
    await expect(page.getByRole('status')).toBeFocused();
    await page.getByRole('main').getByRole('link', { name: 'Continue' }).click();
    await expect(page).toHaveURL('/home');
    await expect(page.getByText('Request sent. Waiting for the tutor.')).toBeVisible();
    await expect(page.getByRole('region', { name: 'Email confirmation' })).toHaveCount(0);

    const row = joinRequests(mariaPage).getByRole('listitem').filter({ hasText: boris.email });
    await expect(row).toContainText('Boris');
    await expect(row).toContainText('Хочет заниматься');
    await expect(mariaBell).toHaveAccessibleName('Уведомления, 1 непрочитанное');
    await mariaBell.click();
    const mariaRows = panelRowsIn(mariaPage, 'Уведомления');
    await expect(mariaRows).toHaveCount(1);
    await expect(mariaRows.first()).toContainText('Boris');
    await expect(mariaRows.first()).toContainText('Новый запрос на занятия');
    await mariaPage.keyboard.press('Escape');
    await expect(joinRequests(mariaPage).getByText(codePattern)).toHaveText(code);
    await row.getByRole('button', { name: 'Принять: Boris' }).click();

    const accepted = joinRequests(mariaPage).getByRole('status');
    await expect(accepted).toHaveText('Заявка принята. Ученик теперь в разделе «Ученики».');
    await expect(accepted).toBeFocused();
    await expect(joinRequests(mariaPage).getByRole('listitem')).toHaveCount(0);
    await expect(joinRequests(mariaPage).getByText('Новых заявок нет.')).toBeVisible();
    await mariaPage.goto('/students');
    await expect(
      mariaPage.getByRole('list', { name: 'Действующие' }).getByRole('link', { name: /Boris/ }),
    ).toBeVisible();
    await expect(page.getByRole('list', { name: 'Tutors' }).getByRole('listitem')).toHaveText([/Мария/]);
    const borisBell = bellIn(page, /^Notifications/);
    await expect(borisBell).toHaveAccessibleName('Notifications, 1 unread');
    await borisBell.click();
    const borisRows = panelRowsIn(page, 'Notifications');
    await expect(borisRows).toHaveCount(1);
    await expect(borisRows.first()).toContainText('Мария');
    await expect(borisRows.first()).toContainText('Your request was accepted');
    await diagnostics.closeContext(mariaContext);
  });

  test('confirmation › a held request whose tutor is no longer active answers that the code is not valid and continues to code entry', async ({
    page,
    deployment,
    control,
  }) => {
    const maria = await seedVerifiedUser(deployment, 'Мария');
    const boris = newTestAccount('Boris', 'student');
    await registerStudentWithCode(deployment, boris, await tutorCode(deployment, maria), 'en');
    deployment.administer(['users', 'block', maria.email]);
    await signInThroughTheApi(page, boris);
    await control.tick();
    await page.goto(emailedLinkTo(await control.mail(boris.email), '/verify-email'));
    await expect(page.getByRole('status')).toHaveText('This code is no longer valid — enter a new one.');
    await expect(page.getByRole('status')).toBeFocused();

    await page.getByRole('main').getByRole('link', { name: 'Continue' }).click();

    await expect(page).toHaveURL('/home');
    await expect(page.getByText('No tutor yet.')).toBeVisible();
    await expect(page.getByRole('textbox', { name: 'Enter your tutor’s code' })).toBeVisible();
  });
});

test.describe('Anna in Russian', () => {
  test.use({ locale: 'ru-RU', timezoneId: 'Europe/Berlin', colorScheme: 'light' });

  test('my tutors › past tutors list an ended relationship and a closed account', async ({ page, deployment }) => {
    const maria = await seedVerifiedUser(deployment, 'Мария');
    const pavel = await seedVerifiedUser(deployment, 'Pavel', 'en');
    const { student: anna, cardId } = await seedActiveStudent(deployment, maria, 'Анна', 'ru');
    await acceptSignedIn(deployment, anna, (await createCard(deployment, pavel, 'Anna', anna.email)).link);
    await endRelationship(deployment, maria, cardId);
    await deleteAccount(deployment, pavel);
    await signInThroughTheApi(page, anna);

    await page.goto('/tutors');

    await expect(page.getByRole('main').getByText('Репетитора пока нет.')).toBeVisible();
    const past = page.getByRole('list', { name: 'Прошлые репетиторы' }).getByRole('listitem');
    await expect(past).toHaveCount(2);
    await expect(past.filter({ hasText: 'Мария' })).toContainText(/Занятия завершены \d/);
    await expect(past.filter({ hasText: 'Pavel' })).toContainText(/Аккаунт закрыт — результаты доступны до \d/);
  });

  test('my tutors › a code that is no longer valid is refused with an alert that takes focus', async ({
    page,
    deployment,
    diagnostics,
  }) => {
    diagnostics.expectFailure({ method: 'POST', path: '/api/v1/join-requests', status: 422 });
    const maria = await seedVerifiedUser(deployment, 'Мария');
    const { student: anna, cardId } = await seedActiveStudent(deployment, maria, 'Анна', 'ru');
    await endRelationship(deployment, maria, cardId);
    await signInThroughTheApi(page, anna);
    await page.goto('/tutors');
    const entry = page.getByRole('form', { name: 'Код репетитора' });
    await entry.getByLabel('Код репетитора').fill('2222-2222');

    await entry.getByRole('button', { name: 'Отправить запрос' }).click();

    await expect(entry.getByRole('alert')).toHaveText('Этот код больше не действует — попросите у репетитора новый.');
    await expect(entry.getByRole('alert')).toBeFocused();
    await expect(page.getByRole('region', { name: 'Отправленные запросы' })).toHaveCount(0);
  });

  test('my tutors › a valid code sends a request, empties the field and lists the request as sent', async ({
    page,
    deployment,
  }) => {
    const maria = await seedVerifiedUser(deployment, 'Мария');
    const olga = await seedVerifiedUser(deployment, 'Ольга');
    const { student: anna, cardId } = await seedActiveStudent(deployment, maria, 'Анна', 'ru');
    await endRelationship(deployment, maria, cardId);
    await signInThroughTheApi(page, anna);
    await page.goto('/tutors');
    const entry = page.getByRole('form', { name: 'Код репетитора' });
    await entry.getByLabel('Код репетитора').fill(await tutorCode(deployment, olga));

    await entry.getByRole('button', { name: 'Отправить запрос' }).click();

    await expect(entry.getByRole('status')).toHaveText('Запрос отправлен. Ждём ответа репетитора.');
    await expect(entry.getByRole('status')).toBeFocused();
    await expect(entry.getByLabel('Код репетитора')).toHaveValue('');
    const requests = sentRequests(page, 'Отправленные запросы');
    await expect(requests).toHaveCount(1);
    await expect(requests).toContainText(['Запрос отправлен']);
  });

  test('my tutors › after an invalid code alert the form still sends a valid code', async ({
    page,
    deployment,
    diagnostics,
  }) => {
    diagnostics.expectFailure({ method: 'POST', path: '/api/v1/join-requests', status: 422 });
    const maria = await seedVerifiedUser(deployment, 'Мария');
    const olga = await seedVerifiedUser(deployment, 'Ольга');
    const { student: anna, cardId } = await seedActiveStudent(deployment, maria, 'Анна', 'ru');
    await endRelationship(deployment, maria, cardId);
    await signInThroughTheApi(page, anna);
    await page.goto('/tutors');
    const entry = page.getByRole('form', { name: 'Код репетитора' });
    await entry.getByLabel('Код репетитора').fill('2222-2222');
    await entry.getByRole('button', { name: 'Отправить запрос' }).click();
    await entry.getByRole('alert').waitFor();
    await entry.getByLabel('Код репетитора').fill(await tutorCode(deployment, olga));

    await entry.getByRole('button', { name: 'Отправить запрос' }).click();

    await expect(entry.getByRole('status')).toHaveText('Запрос отправлен. Ждём ответа репетитора.');
    await expect(entry.getByRole('status')).toBeFocused();
    await expect(entry.getByLabel('Код репетитора')).toHaveValue('');
    await expect(sentRequests(page, 'Отправленные запросы')).toHaveCount(1);
    expect(await visibleJoinRequests(deployment, olga)).toBe(1);
  });

  test('my tutors › a request the tutor declined with a block shows as not accepted', async ({ page, deployment }) => {
    const { anna } = await seedRequestBlockedByOlga(deployment);
    await signInThroughTheApi(page, anna);

    await page.goto('/tutors');

    const requests = sentRequests(page, 'Отправленные запросы');
    await expect(requests).toHaveCount(1);
    await expect(requests).toContainText(['Не принят']);
    await expect(requests).toContainText(['Ваш запрос не приняли.']);
  });

  test('my tutors › the code of a tutor who blocked the student lists a new sent request that the tutor never sees', async ({
    page,
    deployment,
    control,
  }) => {
    const { anna, olga } = await seedRequestBlockedByOlga(deployment);
    await control.advance(60);
    await signInThroughTheApi(page, anna);
    await page.goto('/tutors');
    const entry = page.getByRole('form', { name: 'Код репетитора' });
    const requests = sentRequests(page, 'Отправленные запросы');
    await expect(requests).toHaveCount(1);
    await entry.getByLabel('Код репетитора').fill(await tutorCode(deployment, olga));

    await entry.getByRole('button', { name: 'Отправить запрос' }).click();

    await expect(entry.getByRole('status')).toHaveText('Запрос отправлен. Ждём ответа репетитора.');
    await expect(requests).toHaveCount(2);
    await expect(requests.first()).toContainText('Запрос отправлен. Ждём ответа репетитора.');
    await expect(requests.last()).toContainText('Не принят');
    expect(await visibleJoinRequests(deployment, olga)).toBe(0);
  });
});

test.describe('Olga in Russian', () => {
  test.use({
    locale: 'ru-RU',
    timezoneId: 'Europe/Moscow',
    viewport: { width: 1280, height: 800 },
    colorScheme: 'light',
  });

  test('join requests › decline and block on Students moves focus to the notice and empties the list', async ({
    page,
    deployment,
  }) => {
    const { anna, olga } = await seedRequestToOlga(deployment);
    await recordBusyControls(page);
    await signInThroughTheApi(page, olga);
    await page.goto('/students');
    const request = joinRequests(page).getByRole('listitem').filter({ hasText: anna.email });
    await request.getByRole('button', { name: 'Другие действия: Анна' }).click();

    await joinRequests(page).getByRole('button', { name: 'Отклонить и заблокировать' }).click();

    const declined = joinRequests(page).getByRole('status');
    await expect(declined).toHaveText('Заявка отклонена. Заявки этого ученика к вам больше не приходят.');
    await expect(declined).toBeFocused();
    await expect(joinRequests(page).getByRole('listitem')).toHaveCount(0);
    expect(await busyControls(page)).toEqual([['Другие действия: Анна']]);
  });

  test('join requests › a new request of a blocked student does not appear on Students', async ({
    page,
    deployment,
    control,
  }) => {
    const { anna, olga } = await seedRequestBlockedByOlga(deployment);
    await control.advance(60);
    await enterTutorCode(deployment, anna, await tutorCode(deployment, olga));
    await signInThroughTheApi(page, olga);

    await page.goto('/students');

    await expect(page.getByRole('heading', { level: 1, name: 'Ученики' })).toBeVisible();
    await expect(joinRequests(page)).toHaveCount(0);
    expect(await visibleJoinRequests(deployment, olga)).toBe(0);
  });
});

test.describe('in Russian', () => {
  test.use({ locale: 'ru-RU', colorScheme: 'light' });

  test('join requests › show more on Today moves focus to the first new request', async ({
    page,
    mariaWithJoinRequests,
  }) => {
    const { maria } = mariaWithJoinRequests;
    await recordBusyControls(page);
    await signInThroughTheApi(page, maria);
    await page.goto('/today');
    const accepts = joinRequests(page).getByRole('button', { name: /^Принять: / });
    await expect(accepts).toHaveCount(pageSize);

    await joinRequests(page).getByRole('button', { name: 'Показать ещё' }).click();

    await expect(accepts).toHaveCount(pageSize + 1);
    await expect(accepts.nth(pageSize)).toBeFocused();
    expect(await busyControls(page)).toEqual([['Показать ещё']]);
  });

  test('join requests › a plain decline on Today removes the request and moves focus to the notice', async ({
    page,
    deployment,
  }) => {
    const maria = await seedVerifiedUser(deployment, 'Мария');
    const requesters = await seedJoinRequests(deployment, maria, 2);
    await recordBusyControls(page);
    await signInThroughTheApi(page, maria);
    await page.goto('/today');
    const rows = joinRequests(page).getByRole('listitem');
    const firstEmail = emailOf(requesters, 'Requester 01');

    await rows
      .filter({ hasText: firstEmail })
      .getByRole('button', { name: /^Отклонить: / })
      .click();

    const declined = joinRequests(page).getByRole('status');
    await expect(declined).toHaveText('Заявка отклонена.');
    await expect(declined).toBeFocused();
    await expect(rows.filter({ hasText: firstEmail })).toHaveCount(0);
    await expect(rows.filter({ hasText: emailOf(requesters, 'Requester 02') })).toHaveCount(1);
    expect(await busyControls(page)).toEqual([['Отклонить: Requester 01']]);
  });

  test('join requests › a decline of a request accepted meanwhile is refused with an alert that takes focus', async ({
    socketSlots,
    page,
    deployment,
    diagnostics,
  }) => {
    const maria = await seedVerifiedUser(deployment, 'Мария');
    const requesters = await seedJoinRequests(deployment, maria, 2);
    await recordBusyControls(page);
    await socketSlots.keepPageSocketsOff(maria);
    await signInThroughTheApi(page, maria);
    await page.goto('/today');
    const rows = joinRequests(page).getByRole('listitem');
    const secondEmail = emailOf(requesters, 'Requester 02');
    await expect(rows.filter({ hasText: secondEmail })).toHaveCount(1);
    const acceptedElsewhere = await decideJoinRequest(deployment, maria, secondEmail, 'accept');
    diagnostics.expectFailure({
      method: 'POST',
      path: `/api/v1/join-requests/${acceptedElsewhere}/decline`,
      status: 409,
    });

    await rows
      .filter({ hasText: secondEmail })
      .getByRole('button', { name: /^Отклонить: / })
      .click();

    const changed = joinRequests(page).getByRole('alert');
    await expect(changed).toHaveText('Эта заявка уже не действует. Список обновлён.');
    await expect(changed).toBeFocused();
    await expect(rows.filter({ hasText: secondEmail })).toHaveCount(0);
    expect(await busyControls(page)).toEqual([['Отклонить: Requester 02']]);
  });

  test('join requests › arriving requests do not move the Students controls', async ({ page, deployment }) => {
    const studentsControls = ['main nav', 'main form[role="search"]'];
    const maria = await seedVerifiedUser(deployment, 'Мария');
    await seedJoinRequests(deployment, maria, 1);
    await recordFirstOffsets(page, studentsControls);
    await signInThroughTheApi(page, maria);

    await page.goto('/students');

    await expect(joinRequests(page).getByRole('listitem')).toHaveCount(1);
    await expect(page.getByRole('heading', { level: 2, name: 'Пригласите первого ученика' })).toBeVisible();
    const onStudents = await offsetsBelowHeading(page, studentsControls);
    expect(Object.keys(onStudents.first).sort()).toEqual([...studentsControls].sort());
    expect(onStudents.now).toEqual(onStudents.first);
  });

  test('join requests › arriving requests do not move the Today code and status', async ({ page, deployment }) => {
    const todayParts = ['main h3', 'main [data-readiness]'];
    const maria = await seedVerifiedUser(deployment, 'Мария');
    await seedJoinRequests(deployment, maria, 1);
    await recordFirstOffsets(page, todayParts);
    await signInThroughTheApi(page, maria);

    await page.goto('/today');

    await expect(joinRequests(page).getByRole('listitem')).toHaveCount(1);
    await expect(page.locator('main [data-readiness="ready"]')).toBeVisible();
    const onToday = await offsetsBelowHeading(page, todayParts);
    expect(Object.keys(onToday.first).sort()).toEqual([...todayParts].sort());
    expect(onToday.now).toEqual(onToday.first);
  });

  test('my tutors › show more moves focus to the first new sent request', async ({ page, deployment, control }) => {
    const maria = await seedVerifiedUser(deployment, 'Мария');
    const olga = await seedVerifiedUser(deployment, 'Ольга');
    const { student: anna } = await seedActiveStudent(deployment, maria, 'Анна', 'ru');
    await seedDeclinedRequests(deployment, control, anna, olga, pageSize + 1);
    await signInThroughTheApi(page, anna);
    await page.goto('/tutors');
    const requests = sentRequests(page, 'Отправленные запросы');
    await expect(requests).toHaveCount(pageSize);

    await page
      .getByRole('region', { name: 'Отправленные запросы' })
      .getByRole('button', { name: 'Показать ещё' })
      .click();

    await expect(requests).toHaveCount(pageSize + 1);
    await expect(requests.nth(pageSize)).toBeFocused();
  });

  test('my tutors › show more that adds no visible request moves focus to the last request shown', async ({
    page,
    deployment,
    control,
  }) => {
    const maria = await seedVerifiedUser(deployment, 'Мария');
    const olga = await seedVerifiedUser(deployment, 'Ольга');
    const { student: anna } = await seedStudentThroughCode(deployment, maria, 'Анна', 'ru');
    await control.advance(3601);
    await seedDeclinedRequests(deployment, control, anna, olga, pageSize);
    await signInThroughTheApi(page, anna);
    await page.goto('/tutors');
    const region = page.getByRole('region', { name: 'Отправленные запросы' });
    const requests = sentRequests(page, 'Отправленные запросы');
    await expect(requests).toHaveCount(pageSize);

    await region.getByRole('button', { name: 'Показать ещё' }).press('Enter');

    await expect(region.getByRole('button', { name: 'Показать ещё' })).toHaveCount(0);
    await expect(requests).toHaveCount(pageSize);
    await expect(requests.nth(pageSize - 1)).toBeFocused();
  });

  test('home › a code sent from Home replaces the field with a notice that takes focus', async ({
    page,
    deployment,
  }) => {
    const maria = await seedVerifiedUser(deployment, 'Мария');
    const olga = await seedVerifiedUser(deployment, 'Ольга');
    const { student: anna, cardId } = await seedActiveStudent(deployment, maria, 'Анна', 'ru');
    await endRelationship(deployment, maria, cardId);
    await signInThroughTheApi(page, anna);
    await page.goto('/home');
    const field = page.getByRole('textbox', { name: 'Введите код репетитора' });
    await field.fill(await tutorCode(deployment, olga));

    await page.getByRole('button', { name: 'Отправить запрос' }).click();

    const notice = page.getByRole('status').filter({ hasText: 'Запрос отправлен. Ждём ответа репетитора.' });
    await expect(notice).toBeFocused();
    await expect(field).toHaveCount(0);
    expect(await visibleJoinRequests(deployment, olga)).toBe(1);
  });
});

function emailOf(students: readonly { readonly name: string; readonly email: string }[], name: string): string {
  const student = students.find((candidate) => candidate.name === name);
  if (student === undefined) {
    throw new Error(`no seeded requester named ${name}`);
  }
  return student.email;
}

async function seedRequestToOlga(
  deployment: Deployment,
): Promise<{ readonly anna: TestAccount; readonly olga: TestAccount }> {
  const maria = await seedVerifiedUser(deployment, 'Мария');
  const olga = await seedVerifiedUser(deployment, 'Ольга');
  const { student: anna, cardId } = await seedActiveStudent(deployment, maria, 'Анна', 'ru');
  await endRelationship(deployment, maria, cardId);
  await enterTutorCode(deployment, anna, await tutorCode(deployment, olga));
  return { anna, olga };
}

async function seedRequestBlockedByOlga(
  deployment: Deployment,
): Promise<{ readonly anna: TestAccount; readonly olga: TestAccount }> {
  const seeded = await seedRequestToOlga(deployment);
  await decideJoinRequest(deployment, seeded.olga, seeded.anna.email, 'block');
  return seeded;
}
