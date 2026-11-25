import type { BrowserContext, Locator, Page } from '@playwright/test';

import type { ObservedBrowser } from './browser-diagnostics.ts';

import { type TestAccount, newTestAccount, pageLoaded, signInThroughTheApi } from './accounts.ts';
import type { Deployment } from './deployment.ts';
import { storedAnswers, studentWork } from './assignment.ts';
import { createReadyTemplate, issueThroughTheApi } from './homework.ts';
import { channelOf } from './messages.ts';
import { notificationRows, bellIn, panelRowsIn } from './notifications.ts';
import { addDays, cardLessonStatuses, isoWeekday, localDate, scheduleLessonsWithIds } from './schedule.ts';
import {
  chooseFromPageMenu,
  diagnosedTest as test,
  expect,
  openPageMenu,
  expectRowsNamed,
  seedVerifiedUser,
  signUpAndConfirmTutor,
  tableRows,
} from './fixtures.ts';
import {
  accessToken,
  callApi,
  cardState,
  cardStatesNamed,
  confirmInTheOwnBrowser,
  createCard,
  createGroup,
  endRelationship,
  groupMemberNames,
  invitationToken,
  myTutorNames,
  removeStudent,
  seedActiveStudent,
  setCardNotes,
  signUpOnInvitation,
  signUpActiveStudentOnInvitation,
} from './tutoring.ts';

const endedForStudent = 'Your lessons with a tutor on OVC have ended';
const endedForTutor = 'Ученик прекратил занятия в OVC';

async function englishContext(browser: ObservedBrowser, deployment: Deployment): Promise<BrowserContext> {
  return browser.newContext({
    baseURL: deployment.origin,
    ignoreHTTPSErrors: true,
    locale: 'en-GB',
    timezoneId: 'Europe/Moscow',
    viewport: { width: 1280, height: 800 },
    colorScheme: 'dark',
  });
}

async function expectTokenNowhere(page: Page, token: string): Promise<void> {
  const stored = await page.evaluate(() => ({
    address: globalThis.location.href,
    historyState: JSON.stringify(globalThis.history.state),
    local: Object.values(globalThis.localStorage).join(' '),
    session: Object.values(globalThis.sessionStorage).join(' '),
  }));
  expect(token).not.toBe('');
  expect(stored.address).not.toContain(token);
  expect(stored.historyState).not.toContain(token);
  expect(stored.local).not.toContain(token);
  expect(stored.session).not.toContain(token);
}

function rowsOf(page: Page, name: string): Locator {
  return page.getByRole('list', { name, exact: true }).getByRole('listitem');
}

interface PilotFourSeed {
  readonly maria: TestAccount;
  readonly boris: TestAccount;
  readonly borisCard: string;
  readonly seedLink: string;
  readonly groupId: string;
  readonly seriesLessonId: string;
  readonly groupLessonId: string;
  readonly seriesLessonCount: number;
}

const emailListLimit = 20;

function openedMenu(page: Page): Locator {
  return page.locator('main [aria-expanded="true"] + div');
}

const transparent = 'rgba(0, 0, 0, 0)';

async function backgroundOf(locator: Locator): Promise<string> {
  return locator.evaluate((element) => getComputedStyle(element).backgroundColor);
}

function allCancelled(count: number): string[] {
  return Array.from({ length: count }, () => 'cancelled');
}

async function seriesEndDate(deployment: Deployment, tutor: TestAccount, lessonId: string): Promise<unknown> {
  const answer = await callApi(deployment, await accessToken(deployment, tutor), 'GET', `/lessons/${lessonId}`);
  const body = answer.body;
  if (answer.status !== 200 || typeof body !== 'object' || body === null || !('series' in body)) {
    throw new Error(`reading the lesson answered ${String(answer.status)}`);
  }
  const series: unknown = body.series;
  return typeof series === 'object' && series !== null && 'end_date' in series ? series.end_date : null;
}

let seeded: PilotFourSeed | null = null;

function pilotFour(): PilotFourSeed {
  if (seeded === null) {
    throw new Error('the pilot 4 seed has not run');
  }
  return seeded;
}

test.describe('pilot 4, Maria in Russian and Boris in English', () => {
  test.describe.configure({ mode: 'serial' });
  test.use({ locale: 'ru-RU', timezoneId: 'Europe/Moscow', viewport: { width: 1280, height: 800 } });

  test('pilot-4 › seed Boris joins through the copied link, confirms, Maria adds him to B1 evening with Anna and schedules his series and a group lesson', async ({
    page,
    deployment,
    control,
  }) => {
    const maria = await signUpAndConfirmTutor(deployment, 'Мария');
    const anna = await signUpActiveStudentOnInvitation(deployment, control, maria, 'Анна', 'ru');
    const groupId = await createGroup(deployment, maria, 'B1 evening', [anna.cardId]);
    const boris = newTestAccount('Boris', 'student');
    const card = await createCard(deployment, maria, 'Boris', boris.email);
    await signUpOnInvitation(deployment, card.link, boris, 'en');
    await confirmInTheOwnBrowser(deployment, control, boris);
    await setCardNotes(deployment, maria, card.id, 'Прошли вводный урок.');
    await signInThroughTheApi(page, maria);
    await page.goto(`/groups/${groupId}`);

    await page.getByRole('button', { name: 'Добавить учеников' }).click();
    await page.getByRole('dialog', { name: 'Добавить учеников' }).getByRole('checkbox', { name: 'Boris' }).check();
    const added = page.waitForResponse(
      (response) =>
        response.request().method() === 'PATCH' && new URL(response.url()).pathname === `/api/v1/groups/${groupId}`,
    );

    await page.getByRole('dialog', { name: 'Добавить учеников' }).getByRole('button', { name: 'Добавить' }).click();
    await added;
    const tomorrow = addDays(localDate(await control.now(), 'Europe/Moscow'), 1);
    const series = await scheduleLessonsWithIds(deployment, maria, {
      relationship_id: card.id,
      title: 'Урок',
      date: tomorrow,
      start: '17:00',
      duration_minutes: 60,
      weekdays: [isoWeekday(tomorrow), isoWeekday(addDays(tomorrow, 1))],
      end_date: addDays(tomorrow, 365),
    });
    const groupLesson = await scheduleLessonsWithIds(deployment, maria, {
      group_id: groupId,
      title: 'Урок B1 evening',
      date: addDays(tomorrow, 7),
      start: '19:00',
      duration_minutes: 90,
    });
    const exercises = await createReadyTemplate(deployment, maria, 'Упражнения');
    await issueThroughTheApi(deployment, maria, exercises.id, { group_id: groupId });
    await control.tick();
    await control.advance(1);

    await expect(page.getByRole('status').filter({ hasText: 'Ученики добавлены в группу.' })).toBeFocused();
    await expectRowsNamed(rowsOf(page, 'Ученики'), ['Анна', 'Boris']);
    expect(await cardState(deployment, maria, card.id)).toEqual({ state: 'active', groups: ['B1 evening'] });
    expect(series.length).toBeGreaterThan(emailListLimit);
    expect(groupLesson).toHaveLength(1);
    expect((await studentWork(deployment, boris, 'todo')).map((work) => work.title)).toEqual(['Упражнения']);
    const [firstOfSeries] = series;
    const [theGroupLesson] = groupLesson;
    if (firstOfSeries === undefined || theGroupLesson === undefined) {
      throw new Error('the seed scheduled no lessons');
    }
    seeded = {
      maria,
      boris,
      borisCard: card.id,
      seedLink: card.link,
      groupId,
      seriesLessonId: firstOfSeries.id,
      seriesLessonCount: series.length,
      groupLessonId: theGroupLesson.id,
    };
  });

  test('pilot-4 › step 1 Maria ends working with Boris, which cancels his lessons and ends his series, and he gets one email and leaves B1 evening', async ({
    page,
    deployment,
    control,
  }) => {
    const { maria, boris, borisCard, groupId, seriesLessonId, seriesLessonCount } = pilotFour();
    await signInThroughTheApi(page, maria);
    await page.goto(`/students/${borisCard}`);
    await chooseFromPageMenu(page, 'Действия с карточкой', 'Завершить занятия');
    const dialog = page.getByRole('dialog', { name: 'Завершить занятия?' });
    await expect(dialog.getByRole('button', { name: 'Отмена' })).toBeFocused();
    await expect(dialog).toContainText('Ученик выйдет из ваших групп, будущие уроки отменятся');

    await dialog.getByRole('button', { name: 'Завершить', exact: true }).click();

    await expect(
      page.getByRole('status').filter({ hasText: 'Вы больше не занимаетесь с этим учеником.' }),
    ).toBeFocused();
    await expect(page.getByRole('main').getByText('Занятия завершены', { exact: true })).toBeVisible();
    await expect(page.getByRole('button', { name: 'Пригласить снова' })).toBeVisible();
    const endedOn = localDate(await control.now(), 'Europe/Moscow');
    await control.tick();
    const borisMail = await control.mail(boris.email);
    const ended = borisMail.filter((mail) => mail.subject === endedForStudent);
    expect(ended).toHaveLength(1);
    expect(ended[0]?.text).toContain('These lessons were cancelled:');
    expect(ended[0]?.text.match(/Урок, /g)).toHaveLength(emailListLimit);
    expect(ended[0]?.text).toContain(`and ${String(seriesLessonCount - emailListLimit)} more`);
    expect(borisMail.filter((mail) => mail.subject.startsWith('Lesson'))).toEqual([]);
    expect(await cardLessonStatuses(deployment, maria, borisCard)).toEqual(allCancelled(seriesLessonCount));
    expect(await seriesEndDate(deployment, maria, seriesLessonId)).toBe(endedOn);
    expect(await groupMemberNames(deployment, maria, groupId)).toEqual(['Анна']);
    expect(await cardState(deployment, maria, borisCard)).toEqual({ state: 'ended', groups: [] });
    expect((await notificationRows(deployment, boris)).at(0)).toEqual({
      event: 'relationship.ended',
      label: 'Мария',
      unread: true,
    });
  });

  test('pilot-4 › step 2 two days later horizon extension creates no lessons for the ended series', async ({
    deployment,
    control,
  }) => {
    const { maria, borisCard, seriesLessonCount } = pilotFour();

    await control.advance(2 * 86_400);
    await control.tick();

    expect(await cardLessonStatuses(deployment, maria, borisCard)).toEqual(allCancelled(seriesLessonCount));
  });

  test('pilot-4 › step 3 Boris keeps the read-only private chat and no longer sees the group chat', async ({
    observedBrowser: browser,
    deployment,
    diagnostics,
  }) => {
    const { maria, boris, borisCard } = pilotFour();
    const channel = await channelOf(deployment, maria, `/students/${borisCard}`);
    const context = await englishContext(browser, deployment);
    const page = await context.newPage();
    await diagnostics.watch(page);
    await signInThroughTheApi(page, boris);

    await page.goto(`/messages/${channel}`);

    await expect(page.getByRole('heading', { level: 2, name: 'Мария', exact: true })).toBeFocused();
    await expect(page.getByRole('textbox', { name: 'Message', exact: true })).toHaveAttribute('readonly', '');
    await expect(page.getByRole('button', { name: 'Send', exact: true })).toHaveAttribute('aria-disabled', 'true');
    await expect(page.getByRole('main').getByText('This chat is read-only', { exact: true })).toHaveCount(2);
    await expect(page.getByRole('region', { name: 'Chats', exact: true }).getByRole('link')).toHaveCount(1);
    await expect(page.getByRole('region', { name: 'Chats', exact: true })).not.toContainText('B1 evening');
    await diagnostics.closeContext(context);
  });

  test('pilot-4 › step 4 Boris opens the group lesson’s address and it is not found', async ({
    observedBrowser: browser,
    deployment,
    diagnostics,
  }) => {
    const { boris, groupLessonId } = pilotFour();
    const borisContext = await englishContext(browser, deployment);
    const borisPage = await borisContext.newPage();
    const watched = await diagnostics.watch(borisPage);
    watched.expectFailure({ method: 'GET', path: `/api/v1/lessons/${groupLessonId}`, status: 404 });
    await signInThroughTheApi(borisPage, boris);

    await borisPage.goto(`/lessons/${groupLessonId}`);

    await expect(borisPage.getByRole('heading', { level: 1, name: 'Page not found' })).toBeVisible();
    await expect(borisPage.getByRole('main').getByText('Урок B1 evening')).toHaveCount(0);
    await diagnostics.closeContext(borisContext);
  });

  test('pilot-4 › step 5 the Closed tab shows the group assignment, which is read-only and cannot be submitted', async ({
    observedBrowser: browser,
    deployment,
    diagnostics,
  }) => {
    const { boris } = pilotFour();
    const [closed] = await studentWork(deployment, boris, 'closed');
    if (closed === undefined) {
      throw new Error('Boris has no closed work');
    }
    const borisContext = await englishContext(browser, deployment);
    const borisPage = await borisContext.newPage();
    await diagnostics.watch(borisPage);
    await signInThroughTheApi(borisPage, boris);

    await borisPage.goto('/homework?status=closed');

    const table = borisPage.getByRole('table', { name: 'Assignments' });
    await expect(table.locator('[data-row-link]')).toHaveText(['Упражнения']);
    await expect(table).toContainText('Closed');
    await table.getByRole('link', { name: 'Упражнения' }).click();
    await borisPage.getByRole('heading', { level: 1, name: 'Упражнения' }).waitFor();
    await expect(
      borisPage.getByText('This assignment is closed and can no longer be submitted.', { exact: true }),
    ).toBeVisible();
    await expect(borisPage.getByRole('button', { name: /^Submit/ })).toHaveCount(0);
    await expect(borisPage.getByRole('textbox').first()).not.toBeEditable();
    const refused = await callApi(
      deployment,
      await accessToken(deployment, boris),
      'POST',
      `/submissions/${closed.submissionId}/submit`,
      { version: (await storedAnswers(deployment, boris, closed.submissionId)).version },
    );
    expect(refused.status).toBe(409);
    expect(refused.body).toMatchObject({ error: { code: 'not_editable' } });
    await diagnostics.closeContext(borisContext);
  });

  test('pilot-4 › step 6 the copied link from the seed is no longer valid for Boris', async ({
    observedBrowser: browser,
    deployment,
    diagnostics,
  }) => {
    const { boris, seedLink } = pilotFour();
    const borisContext = await englishContext(browser, deployment);
    const borisPage = await borisContext.newPage();
    const watched = await diagnostics.watch(borisPage);
    watched.expectFailure({
      method: 'GET',
      path: `/api/v1/invitations/${invitationToken(seedLink)}`,
      status: 404,
    });
    await signInThroughTheApi(borisPage, boris);

    await borisPage.goto(seedLink);

    await expect(
      borisPage.getByText('This invitation is no longer valid — ask your tutor for a new link.'),
    ).toBeVisible();
    await expect(borisPage.getByRole('button', { name: 'Accept' })).toHaveCount(0);
    await diagnostics.closeContext(borisContext);
  });

  test('pilot-4 › step 7 Invite again makes Boris active with his history, but not back in B1 evening', async ({
    page,
    observedBrowser: browser,
    deployment,
    diagnostics,
  }) => {
    const { maria, boris, borisCard, groupId } = pilotFour();
    const borisContext = await englishContext(browser, deployment);
    const borisPage = await borisContext.newPage();
    await diagnostics.watch(borisPage);
    await signInThroughTheApi(borisPage, boris);
    await signInThroughTheApi(page, maria);
    await page.goto(`/students/${borisCard}`);

    await page.getByRole('button', { name: 'Пригласить снова' }).click();
    await expect(page.getByRole('status').filter({ hasText: 'Создана новая ссылка-приглашение.' })).toBeFocused();
    const link = (
      (await page
        .getByRole('main')
        .getByText(/\/invite\//)
        .textContent()) ?? ''
    ).trim();
    await expectTokenNowhere(page, invitationToken(link));
    await borisPage.goto(link);
    await borisPage.getByRole('button', { name: 'Accept' }).click();

    await expect(borisPage).toHaveURL('/tutors');
    await expect(tableRows(borisPage, 'Tutors')).toHaveText([/Мария/]);
    await bellIn(page, /^Уведомления/).click();
    const borisAccepted = panelRowsIn(page, 'Уведомления')
      .filter({ hasText: 'Boris' })
      .filter({ hasText: 'Приглашение принято' })
      .filter({ hasText: 'Не прочитано' });
    await expect(borisAccepted).toHaveCount(2);
    await page.keyboard.press('Escape');
    await pageLoaded(page);
    await page.reload();
    await expect(page.getByRole('heading', { level: 1, name: 'Boris' })).toBeVisible();
    await expect(page.getByRole('main').getByText('Занятия завершены', { exact: true })).toHaveCount(0);
    await expect(page.getByRole('form', { name: 'Заметки' }).getByLabel('Заметки')).toHaveValue('Прошли вводный урок.');
    const cardActions = await openPageMenu(page, 'Действия с карточкой');
    await expect(cardActions.getByRole('button', { name: 'Завершить занятия' })).toBeVisible();
    await page.keyboard.press('Escape');
    expect(await cardState(deployment, maria, borisCard)).toEqual({ state: 'active', groups: [] });
    expect(await groupMemberNames(deployment, maria, groupId)).toEqual(['Анна']);
    const channel = await channelOf(deployment, maria, `/students/${borisCard}`);
    await borisPage.goto(`/messages/${channel}`);
    await expect(borisPage.getByRole('textbox', { name: 'Message', exact: true })).not.toHaveAttribute('readonly', '');
    await expect(borisPage.getByRole('button', { name: 'Send', exact: true })).toHaveAttribute(
      'aria-disabled',
      'false',
    );
    await expect(borisPage.getByRole('main').getByText('This chat is read-only', { exact: true })).toHaveCount(0);
    await diagnostics.closeContext(borisContext);
  });
});

test.describe('Anna in Russian', () => {
  test.use({ locale: 'ru-RU', timezoneId: 'Europe/Berlin', colorScheme: 'light' });

  test('my tutors › the stop studying dialog opens on Cancel and says that the student leaves the tutor’s groups', async ({
    page,
    deployment,
  }) => {
    const maria = await seedVerifiedUser(deployment, 'Мария');
    const { student: anna, cardId } = await seedActiveStudent(deployment, maria, 'Анна', 'ru');
    await createGroup(deployment, maria, 'B1 evening', [cardId]);
    await signInThroughTheApi(page, anna);
    await page.goto('/tutors');
    await page.getByRole('button', { name: 'Другие действия: Мария' }).click();

    await page.getByRole('button', { name: 'Прекратить занятия' }).click();

    const dialog = page.getByRole('dialog', { name: 'Прекратить занятия с этим репетитором?' });
    await expect(dialog.getByRole('button', { name: 'Отмена' })).toBeFocused();
    await expect(dialog).toContainText('Вы выйдете из групп этого репетитора');
  });

  test.describe('Maria and her student Anna, shared by the tests below', () => {
    test.describe.configure({ mode: 'default' });

    test('my tutors › cancelling stop studying returns focus to the menu button', async ({
      page,
      deployment,
      mariaAndAnna: { maria, anna, cardId },
    }) => {
      await signInThroughTheApi(page, anna);
      await page.goto('/tutors');
      const menu = page.getByRole('button', { name: 'Другие действия: Мария' });
      await menu.click();
      await page.getByRole('button', { name: 'Прекратить занятия' }).click();
      const dialog = page.getByRole('dialog', { name: 'Прекратить занятия с этим репетитором?' });

      await dialog.getByRole('button', { name: 'Отмена' }).click();

      await expect(menu).toBeFocused();
      await expect(page.getByRole('dialog')).toHaveCount(0);
      expect(await cardState(deployment, maria, cardId)).toEqual({ state: 'active', groups: [] });
    });

    test('my tutors › Stop studying is the last item of the tutor’s menu, with a warning icon', async ({
      page,
      mariaAndAnna: { anna },
    }) => {
      await signInThroughTheApi(page, anna);
      await page.goto('/tutors');

      await page.getByRole('button', { name: 'Другие действия: Мария' }).click();

      const lastItem = openedMenu(page).locator(':scope > button:last-child');
      await expect(lastItem).toHaveText('Прекратить занятия');
      await expect(lastItem.locator('svg')).toHaveCount(1);
    });

    test('my tutors › the stop studying dialog shows the filled Stop studying before a quiet Cancel and opens on Cancel', async ({
      page,
      mariaAndAnna: { anna },
    }) => {
      await signInThroughTheApi(page, anna);
      await page.goto('/tutors');
      await page.getByRole('button', { name: 'Другие действия: Мария' }).click();

      await openedMenu(page).getByRole('button', { name: 'Прекратить занятия' }).click();

      const dialog = page.getByRole('dialog', { name: 'Прекратить занятия с этим репетитором?' });
      await expect(dialog.getByRole('button', { name: 'Отмена' })).toBeFocused();
      await expect(dialog.getByRole('button')).toHaveText(['Прекратить занятия', 'Отмена']);
      expect(await backgroundOf(dialog.getByRole('button', { name: 'Прекратить занятия' }))).not.toBe(transparent);
      expect(await backgroundOf(dialog.getByRole('button', { name: 'Отмена' }))).toBe(transparent);
    });

    test('my tutors › the stop studying dialog opens again after Cancel', async ({ page, mariaAndAnna: { anna } }) => {
      await signInThroughTheApi(page, anna);
      await page.goto('/tutors');
      const menu = page.getByRole('button', { name: 'Другие действия: Мария' });
      await menu.click();
      await page.getByRole('button', { name: 'Прекратить занятия' }).click();
      const dialog = page.getByRole('dialog', { name: 'Прекратить занятия с этим репетитором?' });
      await dialog.getByRole('button', { name: 'Отмена' }).click();
      await expect(menu).toBeFocused();

      await menu.click();
      await page.getByRole('button', { name: 'Прекратить занятия' }).click();

      await expect(dialog.getByRole('button', { name: 'Отмена' })).toBeFocused();
      await expect(dialog.getByRole('button', { name: 'Прекратить занятия' })).toHaveAttribute(
        'aria-disabled',
        'false',
      );
    });
  });

  test('my tutors › stop studying ends the relationship, leaves the groups, emails the tutor once and moves focus to the notice', async ({
    page,
    deployment,
    control,
  }) => {
    const maria = await seedVerifiedUser(deployment, 'Мария');
    const { student: anna, cardId } = await seedActiveStudent(deployment, maria, 'Анна', 'ru');
    const groupId = await createGroup(deployment, maria, 'B1 evening', [cardId]);
    await signInThroughTheApi(page, anna);
    await page.goto('/tutors');
    await page.getByRole('button', { name: 'Другие действия: Мария' }).click();
    await page.getByRole('button', { name: 'Прекратить занятия' }).click();
    const dialog = page.getByRole('dialog', { name: 'Прекратить занятия с этим репетитором?' });
    await expect(dialog.getByRole('button', { name: 'Прекратить занятия' })).toHaveAttribute('aria-disabled', 'false');

    await dialog.getByRole('button', { name: 'Прекратить занятия' }).click();

    const notice = page
      .getByRole('status')
      .filter({ hasText: 'Вы больше не занимаетесь с этим репетитором. Он теперь в списке прошлых репетиторов.' });
    await expect(notice).toBeFocused();
    await expect(page.getByRole('dialog')).toHaveCount(0);
    await expect(page.getByRole('main').getByText('Репетитора пока нет.')).toBeVisible();
    await expect(tableRows(page, 'Прошлые репетиторы')).toHaveText([/Мария.*Занятия завершены \d/]);
    await control.tick();
    const ended = (await control.mail(maria.email)).filter((mail) => mail.subject === endedForTutor);
    expect(ended).toHaveLength(1);
    expect(await cardState(deployment, maria, cardId)).toEqual({ state: 'ended', groups: [] });
    expect(await groupMemberNames(deployment, maria, groupId)).toEqual([]);
  });
});

test.describe('Boris in English', () => {
  test.use({ locale: 'en-GB', timezoneId: 'Europe/Moscow', colorScheme: 'dark' });

  test.describe('on a phone', () => {
    test.use({ viewport: { width: 360, height: 780 } });

    test('my tutors › stop studying before the email is confirmed fills the screen with its dialog and removes the link silently', async ({
      page,
      deployment,
      control,
    }) => {
      const pavel = await seedVerifiedUser(deployment, 'Pavel', 'en');
      const boris = newTestAccount('Boris', 'student');
      const card = await createCard(deployment, pavel, 'Boris', boris.email);
      await signUpOnInvitation(deployment, card.link, boris, 'en');
      await control.tick();
      const mailBefore = (await control.mail(pavel.email)).length;
      await signInThroughTheApi(page, boris);
      await page.goto('/tutors');
      await page.getByRole('button', { name: 'More actions for Pavel' }).click();
      await page.getByRole('button', { name: 'Stop studying' }).click();
      const dialog = page.getByRole('dialog', { name: 'Stop studying with this tutor?' });
      await expect(dialog).toContainText(
        'Your email is not confirmed yet, so the link to this tutor is removed: your future lessons with this tutor are cancelled, and your chat with them and your work for them are deleted. The tutor gets no message.',
      );
      await expect(dialog.getByRole('button', { name: 'Stop studying' })).toHaveAttribute('aria-disabled', 'false');
      await expect(dialog.getByRole('button', { name: 'Cancel' })).toBeFocused();
      expect(await dialog.boundingBox()).toEqual({ x: 0, y: 0, width: 360, height: 780 });

      await dialog.getByRole('button', { name: 'Stop studying' }).click();

      await expect(page.getByRole('status').filter({ hasText: 'The link to the tutor was removed.' })).toBeFocused();
      await expect(page.getByRole('main').getByText('No tutor yet.')).toBeVisible();
      await expect(page.getByRole('list', { name: 'Past tutors', exact: true })).toHaveCount(0);
      expect(await myTutorNames(deployment, boris)).toEqual([]);
      expect(await cardStatesNamed(deployment, pavel, 'Boris')).toEqual(['invited']);
      await control.tick();
      expect(await control.mail(pavel.email)).toHaveLength(mailBefore);
    });
  });

  test('my tutors › stopping a link the tutor removed meanwhile closes the dialog and shows the current list', async ({
    socketSlots,
    page,
    deployment,
    diagnostics,
  }) => {
    const pavel = await seedVerifiedUser(deployment, 'Pavel', 'en');
    const boris = newTestAccount('Boris', 'student');
    const card = await createCard(deployment, pavel, 'Boris', boris.email);
    await signUpOnInvitation(deployment, card.link, boris, 'en');
    diagnostics.expectFailure({ method: 'POST', path: `/api/v1/tutors/${card.id}/end`, status: 404 });
    await socketSlots.keepPageSocketsOff(boris);
    await signInThroughTheApi(page, boris);
    await page.goto('/tutors');
    await page.getByRole('button', { name: 'More actions for Pavel' }).click();
    await page.getByRole('button', { name: 'Stop studying' }).click();
    const stop = page.getByRole('dialog', { name: 'Stop studying with this tutor?' }).getByRole('button', {
      name: 'Stop studying',
    });
    await expect(stop).toHaveAttribute('aria-disabled', 'false');
    await removeStudent(deployment, pavel, card.id);

    await stop.click();

    await expect(page.getByRole('dialog')).toHaveCount(0);
    await expect(
      page
        .getByRole('status')
        .filter({ hasText: 'Your tutors changed in the meantime. The list below is up to date.' }),
    ).toBeFocused();
    await expect(page.getByRole('main').getByText('No tutor yet.')).toBeVisible();
  });

  test('my tutors › a tutor who ended the relationship while the menu was open closes the dialog with the changed-meanwhile notice', async ({
    socketSlots,
    page,
    deployment,
  }) => {
    const pavel = await seedVerifiedUser(deployment, 'Pavel', 'en');
    const { student: anna, cardId } = await seedActiveStudent(deployment, pavel, 'Anna');
    await socketSlots.keepPageSocketsOff(anna);
    await signInThroughTheApi(page, anna);
    await page.goto('/tutors');
    await page.getByRole('button', { name: 'More actions for Pavel' }).click();
    await endRelationship(deployment, pavel, cardId);

    await page.getByRole('button', { name: 'Stop studying' }).click();

    await expect(page.getByRole('dialog')).toHaveCount(0);
    await expect(
      page
        .getByRole('status')
        .filter({ hasText: 'Your tutors changed in the meantime. The list below is up to date.' }),
    ).toBeFocused();
    await expect(tableRows(page, 'Past tutors')).toHaveText([/Pavel/]);
  });
});
