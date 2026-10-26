import type { Browser, BrowserContext, Locator, Page } from '@playwright/test';

import { type TestAccount, newTestAccount, signInThroughTheApi } from './accounts.ts';
import type { Deployment } from './deployment.ts';
import { notificationRows, bellIn, panelRowsIn } from './notifications.ts';
import { diagnosedTest as test, expect, expectRowsNamed, seedVerifiedUser } from './fixtures.ts';
import {
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
} from './tutoring.ts';

const endedForStudent = 'Your lessons with a tutor on OVC have ended';
const endedForTutor = 'Ученик прекратил занятия в OVC';

async function englishContext(browser: Browser, deployment: Deployment): Promise<BrowserContext> {
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

  test('pilot-4 › seed Boris joins through the copied link, confirms, and Maria adds him to B1 evening with Anna', async ({
    page,
    deployment,
    control,
  }) => {
    const maria = await seedVerifiedUser(deployment, 'Мария');
    const anna = await seedActiveStudent(deployment, control, maria, 'Анна', 'ru');
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
    await page.getByRole('dialog', { name: 'Добавить учеников' }).getByRole('button', { name: 'Добавить' }).click();

    await expect(page.getByRole('status').filter({ hasText: 'Ученики добавлены в группу.' })).toBeFocused();
    await expectRowsNamed(rowsOf(page, 'Ученики'), ['Анна', 'Boris']);
    expect(await cardState(deployment, maria, card.id)).toEqual({ state: 'active', groups: ['B1 evening'] });
    seeded = { maria, boris, borisCard: card.id, seedLink: card.link, groupId };
  });

  test('pilot-4 › step 1 Maria ends working with Boris, who gets one email and leaves B1 evening', async ({
    page,
    deployment,
    control,
  }) => {
    const { maria, boris, borisCard, groupId } = pilotFour();
    await signInThroughTheApi(page, maria);
    await page.goto(`/students/${borisCard}`);
    await page.getByRole('button', { name: 'Завершить занятия' }).click();
    const dialog = page.getByRole('dialog', { name: 'Завершить занятия?' });
    await expect(dialog.getByRole('button', { name: 'Отмена' })).toBeFocused();
    await expect(dialog).toContainText('Ученик выйдет из ваших групп, будущие уроки отменятся');

    await dialog.getByRole('button', { name: 'Завершить', exact: true }).click();

    await expect(
      page.getByRole('status').filter({ hasText: 'Вы больше не занимаетесь с этим учеником.' }),
    ).toBeFocused();
    await expect(page.getByRole('main').getByText('Занятия завершены', { exact: true })).toBeVisible();
    await expect(page.getByRole('button', { name: 'Пригласить снова' })).toBeVisible();
    await control.tick();
    const ended = (await control.mail(boris.email)).filter((mail) => mail.subject === endedForStudent);
    expect(ended).toHaveLength(1);
    expect(await groupMemberNames(deployment, maria, groupId)).toEqual(['Анна']);
    expect(await cardState(deployment, maria, borisCard)).toEqual({ state: 'ended', groups: [] });
    expect((await notificationRows(deployment, boris)).at(0)).toEqual({
      event: 'relationship.ended',
      label: 'Мария',
      unread: true,
    });
  });

  test('pilot-4 › step 6 the copied link from the seed is no longer valid for Boris', async ({
    browser,
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
    await borisContext.close();
  });

  test('pilot-4 › step 7 Invite again makes Boris active with his history, but not back in B1 evening', async ({
    page,
    browser,
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
    await expect(rowsOf(borisPage, 'Tutors')).toHaveText([/Мария/]);
    await bellIn(page, /^Уведомления/).click();
    const borisAccepted = panelRowsIn(page, 'Уведомления')
      .filter({ hasText: 'Boris' })
      .filter({ hasText: 'Приглашение принято' })
      .filter({ hasText: 'Не прочитано' });
    await expect(borisAccepted).toHaveCount(2);
    await page.keyboard.press('Escape');
    await page.reload();
    await expect(page.getByRole('heading', { level: 1, name: 'Boris' })).toBeVisible();
    await expect(page.getByRole('main').getByText('Занятия завершены', { exact: true })).toHaveCount(0);
    await expect(page.getByRole('form', { name: 'Заметки' }).getByLabel('Заметки')).toHaveValue('Прошли вводный урок.');
    await expect(page.getByRole('button', { name: 'Завершить занятия' })).toBeVisible();
    expect(await cardState(deployment, maria, borisCard)).toEqual({ state: 'active', groups: [] });
    expect(await groupMemberNames(deployment, maria, groupId)).toEqual(['Анна']);
    await borisContext.close();
  });
});

test.describe('Anna in Russian', () => {
  test.use({ locale: 'ru-RU', timezoneId: 'Europe/Berlin', colorScheme: 'light' });

  test('my tutors › the stop studying dialog opens on Cancel and says that the student leaves the tutor’s groups', async ({
    page,
    deployment,
    control,
  }) => {
    const maria = await seedVerifiedUser(deployment, 'Мария');
    const { student: anna, cardId } = await seedActiveStudent(deployment, control, maria, 'Анна', 'ru');
    await createGroup(deployment, maria, 'B1 evening', [cardId]);
    await signInThroughTheApi(page, anna);
    await page.goto('/tutors');
    await page.getByRole('button', { name: 'Другие действия: Мария' }).click();

    await page.getByRole('button', { name: 'Прекратить занятия' }).click();

    const dialog = page.getByRole('dialog', { name: 'Прекратить занятия с этим репетитором?' });
    await expect(dialog.getByRole('button', { name: 'Отмена' })).toBeFocused();
    await expect(dialog).toContainText('Вы выйдете из групп этого репетитора');
  });

  test('my tutors › cancelling stop studying returns focus to the menu button', async ({
    page,
    deployment,
    control,
  }) => {
    const maria = await seedVerifiedUser(deployment, 'Мария');
    const { student: anna, cardId } = await seedActiveStudent(deployment, control, maria, 'Анна', 'ru');
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

  test('my tutors › the stop studying dialog opens again after Cancel', async ({ page, deployment, control }) => {
    const maria = await seedVerifiedUser(deployment, 'Мария');
    const { student: anna } = await seedActiveStudent(deployment, control, maria, 'Анна', 'ru');
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
    await expect(dialog.getByRole('button', { name: 'Прекратить занятия' })).toHaveAttribute('aria-disabled', 'false');
  });

  test('my tutors › stop studying ends the relationship, leaves the groups, emails the tutor once and moves focus to the notice', async ({
    page,
    deployment,
    control,
  }) => {
    const maria = await seedVerifiedUser(deployment, 'Мария');
    const { student: anna, cardId } = await seedActiveStudent(deployment, control, maria, 'Анна', 'ru');
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
    await expect(rowsOf(page, 'Прошлые репетиторы')).toHaveText([/Мария.*Занятия завершены \d/]);
    await control.tick();
    const ended = (await control.mail(maria.email)).filter((mail) => mail.subject === endedForTutor);
    expect(ended).toHaveLength(1);
    expect(await cardState(deployment, maria, cardId)).toEqual({ state: 'ended', groups: [] });
    expect(await groupMemberNames(deployment, maria, groupId)).toEqual([]);
  });
});

test.describe('Boris in English', () => {
  test.use({ locale: 'en-GB', timezoneId: 'Europe/Moscow', colorScheme: 'dark' });

  test('my tutors › stop studying before the email is confirmed removes the link silently', async ({
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
    await expect(dialog).toContainText('Your email is not confirmed yet, so the link to this tutor is simply removed.');
    await expect(dialog.getByRole('button', { name: 'Stop studying' })).toHaveAttribute('aria-disabled', 'false');

    await dialog.getByRole('button', { name: 'Stop studying' }).click();

    await expect(page.getByRole('status').filter({ hasText: 'The link to the tutor was removed.' })).toBeFocused();
    await expect(page.getByRole('main').getByText('No tutor yet.')).toBeVisible();
    await expect(page.getByRole('list', { name: 'Past tutors', exact: true })).toHaveCount(0);
    expect(await myTutorNames(deployment, boris)).toEqual([]);
    expect(await cardStatesNamed(deployment, pavel, 'Boris')).toEqual(['invited']);
    await control.tick();
    expect(await control.mail(pavel.email)).toHaveLength(mailBefore);
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
    control,
  }) => {
    const pavel = await seedVerifiedUser(deployment, 'Pavel', 'en');
    const { student: anna, cardId } = await seedActiveStudent(deployment, control, pavel, 'Anna');
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
    await expect(rowsOf(page, 'Past tutors')).toHaveText([/Pavel/]);
  });
});
