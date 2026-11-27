import type { BrowserContext, Locator, Page } from '@playwright/test';

import type { ObservedBrowser } from './browser-diagnostics.ts';

import { type TestAccount, emailedLinkTo, newTestAccount, pageLoaded, signInThroughTheApi } from './accounts.ts';
import type { Deployment } from './deployment.ts';
import { bellIn, panelRowsIn } from './notifications.ts';
import {
  type Diagnostics,
  chooseFromPageMenu,
  diagnosedTest as test,
  expect,
  openPageMenu,
  seedVerifiedUser,
  signUpAndConfirmTutor,
  tableRows,
} from './fixtures.ts';
import { recordLazyChunkAddresses } from './observers.ts';
import {
  createCard,
  decideJoinRequest,
  endRelationship,
  enterTutorCode,
  invitationIsUsable,
  invitationToken,
  myTutorNames,
  confirmInTheOwnBrowser,
  seedActiveStudent,
  sendInvitation,
  setCardNotes,
  signUpOnInvitation,
  tutorCode,
  signUpActiveStudentOnInvitation,
} from './tutoring.ts';

const knownPreferenceKeys = ['ovc.language', 'ovc.sidebar', 'ovc.theme'];

function recordAddresses(page: Page): string[] {
  const addresses: string[] = [];
  page.on('framenavigated', (frame) => {
    if (frame === page.mainFrame()) {
      addresses.push(frame.url());
    }
  });
  return addresses;
}

async function tutorContext(browser: ObservedBrowser, deployment: Deployment, locale: string): Promise<BrowserContext> {
  return browser.newContext({
    baseURL: deployment.origin,
    ignoreHTTPSErrors: true,
    locale,
    timezoneId: 'Europe/Moscow',
    viewport: { width: 1280, height: 800 },
    colorScheme: 'light',
  });
}

async function expectTokenNowhere(page: Page, token: string): Promise<void> {
  const stored = await page.evaluate(() => ({
    address: globalThis.location.href,
    local: Object.keys(globalThis.localStorage),
    localValues: Object.values(globalThis.localStorage).join(' '),
    session: Object.values(globalThis.sessionStorage).join(' '),
    historyState: JSON.stringify(globalThis.history.state),
    cookies: document.cookie,
  }));
  expect(stored.address).not.toContain(token);
  expect(stored.local.every((key) => knownPreferenceKeys.includes(key))).toBe(true);
  expect(stored.localValues).not.toContain(token);
  expect(stored.session).not.toContain(token);
  expect(stored.historyState).not.toContain(token);
  expect(stored.cookies).toBe('');
}

interface PolicyLinkObservation {
  readonly referrerPolicy: string | undefined;
  readonly address: string;
  readonly href: string | null;
  readonly target: string | null;
  readonly rel: string | null;
  readonly policyAddress: string;
  readonly referrer: string;
  readonly openerCleared: boolean;
  readonly policyHeadings: readonly string[];
}

async function openThePolicyFrom(page: Page, link: string, diagnostics: Diagnostics): Promise<PolicyLinkObservation> {
  const opened = await page.goto(link);
  await page.waitForURL('**/invite');
  const policyLink = page.getByRole('link', { name: 'Privacy policy (opens in a new tab)' });
  await policyLink.waitFor();
  const openedTab = page.context().waitForEvent('page');
  await policyLink.click();
  const policy = await openedTab;
  await diagnostics.watch(policy);
  await policy.getByRole('heading', { level: 1 }).waitFor();
  const observation: PolicyLinkObservation = {
    referrerPolicy: opened?.headers()['referrer-policy'],
    address: page.url(),
    href: await policyLink.getAttribute('href'),
    target: await policyLink.getAttribute('target'),
    rel: await policyLink.getAttribute('rel'),
    policyAddress: policy.url(),
    referrer: await policy.evaluate(() => document.referrer),
    openerCleared: await policy.evaluate(() => globalThis.opener === null),
    policyHeadings: await policy.getByRole('heading', { level: 1 }).allTextContents(),
  };
  await policy.close();
  return observation;
}

function homeTutorRows(page: Page): Locator {
  return page.getByRole('list', { name: /^(Репетиторы|Tutors)$/ }).getByRole('listitem');
}

function myTutorRows(page: Page): Locator {
  return tableRows(page, /^(Репетиторы|Tutors)$/);
}

async function expectTutors(rows: Locator, names: readonly string[]): Promise<void> {
  await expect(rows).toHaveCount(names.length);
  for (const name of names) {
    await expect(rows.filter({ hasText: name })).toHaveCount(1);
  }
}

function addressesAfterRemoval(addresses: readonly string[], token: string): string[] {
  const removed = addresses.findIndex((address) => !address.includes(token));
  return removed < 0 ? [...addresses] : addresses.slice(removed);
}

async function dotColours(rows: Locator): Promise<string[]> {
  return rows.evaluateAll((items) =>
    items.map((item) => {
      const dot = item.querySelector('[aria-hidden="true"].rounded-full.size-2');
      return dot === null ? '' : getComputedStyle(dot).backgroundColor;
    }),
  );
}

async function signUpOnThePage(page: Page, account: TestAccount, labels: SignUpLabels): Promise<void> {
  await page.getByRole('button', { name: labels.choice }).click();
  const form = page.getByRole('form', { name: labels.choice });
  await expect(form.getByLabel(labels.email, { exact: true })).toHaveValue('');
  await form.getByLabel(labels.name, { exact: true }).fill(account.name);
  await form.getByLabel(labels.email, { exact: true }).fill(account.email);
  await form.getByLabel(labels.password, { exact: true }).fill(account.password);
  await form.getByLabel(labels.consent).check();
  await form.getByRole('button', { name: labels.submit }).click();
}

interface SignUpLabels {
  readonly choice: string;
  readonly name: string;
  readonly email: string;
  readonly password: string;
  readonly consent: RegExp;
  readonly submit: string;
}

const russianSignUp: SignUpLabels = {
  choice: 'Создать аккаунт',
  name: 'Имя',
  email: 'Email',
  password: 'Пароль',
  consent: /^Я даю согласие на обработку моих персональных данных/,
  submit: 'Создать аккаунт и присоединиться',
};

const englishSignUp: SignUpLabels = {
  choice: 'Create account',
  name: 'Name',
  email: 'Email',
  password: 'Password',
  consent: /^I consent to the processing of my personal data/,
  submit: 'Create account and join',
};

test.describe('Anna in Russian on a phone', () => {
  test.use({
    locale: 'ru-RU',
    timezoneId: 'Europe/Berlin',
    viewport: { width: 390, height: 844 },
    colorScheme: 'light',
  });

  test('pilot-1 › step 5 Anna signs up on the invitation page, sees Maria on Home and confirms in the same browser', async ({
    page,
    observedBrowser: browser,
    deployment,
    control,
    diagnostics,
  }) => {
    const maria = await signUpAndConfirmTutor(deployment, 'Мария');
    const anna = newTestAccount('Анна', 'student');
    const card = await createCard(deployment, maria, 'Анна', anna.email);
    await sendInvitation(deployment, maria, card.id);
    await control.tick();
    const link = emailedLinkTo(await control.mail(anna.email), '/invite');
    const token = invitationToken(link);
    const addresses = recordAddresses(page);

    await page.goto(link);
    await expect(page).toHaveURL('/invite');
    await expect(page.getByRole('heading', { level: 2, name: 'Мария' })).toBeVisible();
    await expect(page.getByText('Мария приглашает вас заниматься в OVC.')).toBeVisible();
    await expect(page.getByText(anna.email)).toHaveCount(0);
    await signUpOnThePage(page, anna, russianSignUp);

    await expect(page).toHaveURL('/home');
    await expect(page.getByRole('heading', { level: 1, name: 'Главная' })).toBeFocused();
    const banner = page.getByRole('region', { name: 'Подтверждение email' });
    await expect(banner).toContainText('Подтвердите email, чтобы продолжить занятия с репетитором.');
    await expectTutors(homeTutorRows(page), ['Мария']);
    const mariaContext = await tutorContext(browser, deployment, 'ru-RU');
    const mariaPage = await mariaContext.newPage();
    await diagnostics.watch(mariaPage);
    await signInThroughTheApi(mariaPage, maria);
    await mariaPage.goto(`/students/${card.id}`);
    await expect(mariaPage.getByRole('main').getByText('Email не подтверждён')).toBeVisible();
    const mariaCardActions = await openPageMenu(mariaPage, 'Действия с карточкой');
    await expect(mariaCardActions.getByRole('button', { name: 'Завершить занятия' })).toBeVisible();
    await mariaPage.keyboard.press('Escape');
    const mariaBell = bellIn(mariaPage, /^Уведомления/);
    await expect(mariaBell).toHaveAccessibleName('Уведомления, 1 непрочитанное');
    await mariaBell.click();
    const mariaRows = panelRowsIn(mariaPage, 'Уведомления');
    await expect(mariaRows).toHaveCount(1);
    await expect(mariaRows.first()).toContainText('Анна');
    await expect(mariaRows.first()).toContainText('Приглашение принято');
    await mariaPage.keyboard.press('Escape');

    await control.tick();
    await page.goto(emailedLinkTo(await control.mail(anna.email), '/verify-email'));
    await expect(page.getByRole('status')).toHaveText('Теперь вы занимаетесь с репетитором.');
    await expect(page.getByRole('status')).toBeFocused();
    await expect(page.getByLabel('Новый пароль')).toHaveCount(0);
    await page.getByRole('main').getByRole('link', { name: 'Продолжить' }).click();

    await expect(page).toHaveURL('/home');
    await expectTutors(homeTutorRows(page), ['Мария']);
    await expect(page.getByRole('region', { name: 'Подтверждение email' })).toHaveCount(0);
    await pageLoaded(mariaPage);
    await mariaPage.reload();
    await expect(mariaPage.getByRole('heading', { level: 1, name: 'Анна' })).toBeVisible();
    await expect(mariaBell).toHaveAccessibleName('Уведомления, 1 непрочитанное');
    await expect(mariaPage.getByRole('heading', { level: 2, name: 'Аккаунт ученика' })).toBeVisible();
    await expect(mariaPage.getByRole('main').getByText(anna.name, { exact: true }).last()).toBeVisible();
    for (const word of ['Email не подтверждён', 'Ждём ответа ученика', 'Приглашение отправлено', 'Занятия завершены']) {
      await expect(mariaPage.getByRole('main').getByText(word)).toHaveCount(0);
    }
    await pageLoaded(mariaPage);
    await mariaPage.goto('/students?state=active');
    await expect(tableRows(mariaPage, 'Действующие').getByRole('link', { name: /Анна/ })).toBeVisible();
    expect(addresses.at(-1)).not.toContain(token);
    expect(addressesAfterRemoval(addresses, token).filter((address) => address.includes(token))).toEqual([]);
    await expectTokenNowhere(page, token);
    await diagnostics.closeContext(mariaContext);
  });

  test('pilot-3 › steps 1–2 Pavel invites Anna, who accepts signed in and sees both tutors with their colours', async ({
    page,
    deployment,
    control,
  }) => {
    const maria = await signUpAndConfirmTutor(deployment, 'Мария');
    const { student: anna } = await signUpActiveStudentOnInvitation(deployment, control, maria, 'Анна', 'ru');
    const pavel = await signUpAndConfirmTutor(deployment, 'Pavel', 'en');
    const card = await createCard(deployment, pavel, 'Anna', anna.email);
    await sendInvitation(deployment, pavel, card.id);
    await control.tick();
    const link = emailedLinkTo(await control.mail(anna.email), '/invite');
    await signInThroughTheApi(page, anna);

    await page.goto(link);
    await expect(page.getByRole('heading', { level: 2, name: 'Pavel' })).toBeVisible();
    await expect(page.getByRole('status')).toHaveText(`Вы вошли как ${anna.email}.`);
    await page.getByRole('button', { name: 'Принять' }).click();

    await expect(page).toHaveURL('/tutors');
    await expect(page.getByRole('heading', { level: 1, name: 'Мои репетиторы' })).toBeFocused();
    await expectTutors(myTutorRows(page), ['Мария', 'Pavel']);
    const colours = await dotColours(myTutorRows(page));
    expect(colours.every((colour) => colour !== '' && colour !== 'rgba(0, 0, 0, 0)')).toBe(true);
    expect(new Set(colours).size).toBe(2);
    expect(await invitationIsUsable(deployment, link)).toBe(false);
  });
});

test.describe('in English', () => {
  test.use({ locale: 'en-GB', colorScheme: 'dark' });

  test('invitations › a tutor sees the role explanation and signs out to use a student account', async ({
    page,
    deployment,
  }) => {
    const maria = await seedVerifiedUser(deployment, 'Maria', 'en');
    const pavel = await seedVerifiedUser(deployment, 'Pavel', 'en');
    const card = await createCard(deployment, maria, 'A new student', null);
    await signInThroughTheApi(page, pavel);
    await page.goto(card.link);
    await expect(page).toHaveURL('/invite');
    await expect(page.getByRole('status')).toHaveText(
      'Tutor accounts cannot study. To accept this invitation, sign out and use a student account.',
    );
    await expect(page.getByRole('button', { name: 'Accept' })).toHaveCount(0);

    await page.getByRole('button', { name: 'Sign out to use a student account' }).click();

    await expect(page.getByRole('button', { name: 'Create account' })).toBeFocused();
    await expect(page.getByRole('button', { name: 'Sign in to accept' })).toBeVisible();
    await expect(page).toHaveURL('/invite');
    expect(await invitationIsUsable(deployment, card.link)).toBe(true);
  });

  test('invitations › email mismatch message', async ({ page, deployment, diagnostics }) => {
    const maria = await seedVerifiedUser(deployment, 'Maria', 'en');
    const invited = newTestAccount('Invited student', 'student');
    const somebodyElse = newTestAccount('Somebody else', 'student');
    const card = await createCard(deployment, maria, 'Invited student', invited.email);
    diagnostics.expectFailure({
      method: 'POST',
      path: `/api/v1/invitations/${invitationToken(card.link)}/accept`,
      status: 409,
    });

    await page.goto(card.link);
    await signUpOnThePage(page, somebodyElse, englishSignUp);

    const form = page.getByRole('form', { name: 'Create account' });
    await expect(form.getByRole('alert')).toHaveText('This invitation is for another email address.');
    await expect(form.getByRole('alert')).toBeFocused();
    await expect(page).toHaveURL('/invite');
    expect(await invitationIsUsable(deployment, card.link)).toBe(true);
  });

  test('invitations › the token leaves the address and never appears in next', async ({ page, deployment }) => {
    const maria = await seedVerifiedUser(deployment, 'Maria', 'en');
    const pavel = await seedVerifiedUser(deployment, 'Pavel', 'en');
    const { student } = await seedActiveStudent(deployment, pavel, 'Boris');
    const card = await createCard(deployment, maria, 'Boris', null);
    const token = invitationToken(card.link);
    const addresses = recordAddresses(page);
    const chunkAddresses = await recordLazyChunkAddresses(page, deployment.origin);
    const foreignRequests: string[] = [];
    page.on('request', (request) => {
      if (new URL(request.url()).origin !== deployment.origin) {
        foreignRequests.push(request.url());
      }
    });
    const opened = await page.goto(card.link);
    expect(opened?.headers()['referrer-policy']).toBe('no-referrer');
    await expect(page).toHaveURL('/invite');
    await expect(page.getByRole('heading', { level: 2, name: 'Maria' })).toBeVisible();
    await expectTokenNowhere(page, token);
    await expect(page.getByRole('navigation').getByRole('link', { name: 'Sign in' })).toHaveAttribute('href', '/');
    await page.getByRole('button', { name: 'Sign in to accept' }).click();
    const signIn = page.getByRole('form', { name: 'Sign in to accept' });
    await signIn.getByLabel('Email').fill(student.email);
    await signIn.getByLabel('Password', { exact: true }).fill(student.password);
    await signIn.getByRole('button', { name: 'Sign in' }).click();
    await expect(page.getByRole('status')).toHaveText(`You are signed in as ${student.email}.`);
    await expect(page.getByRole('status')).toBeFocused();
    await expect(page).toHaveURL('/invite');
    await expectTokenNowhere(page, token);

    await page.getByRole('button', { name: 'Accept' }).click();

    await expect(page).toHaveURL('/tutors');
    await expectTutors(myTutorRows(page), ['Pavel', 'Maria']);
    await expectTokenNowhere(page, token);
    expect(addresses.at(-1)).not.toContain(token);
    expect(addressesAfterRemoval(addresses, token).filter((address) => address.includes(token))).toEqual([]);
    expect(addresses.filter((address) => new URL(address).searchParams.has('next'))).toEqual([]);
    expect(chunkAddresses).not.toEqual([]);
    expect(chunkAddresses.filter((address) => address.includes(token))).toEqual([]);
    expect(foreignRequests).toEqual([]);
    expect((await myTutorNames(deployment, student)).sort()).toEqual(['Maria', 'Pavel']);
  });

  test('invitations › an invitation that is no longer valid is refused and a reload asks to open the link again', async ({
    page,
    deployment,
    diagnostics,
  }) => {
    diagnostics.expectFailure({ method: 'GET', path: '/api/v1/invitations/not-a-real-token', status: 404 });
    const pavel = await seedVerifiedUser(deployment, 'Pavel', 'en');
    const { student } = await seedActiveStudent(deployment, pavel, 'Boris');
    await signInThroughTheApi(page, student);
    await page.goto('/invite/not-a-real-token');
    await expect(page.getByRole('alert')).toHaveText(
      'This invitation is no longer valid — ask your tutor for a new link.',
    );
    await expect(page).toHaveURL('/invite');

    await page.reload();

    await expect(page.getByText('Open the link from your tutor again.')).toBeVisible();
  });

  test('invitations › the page links the privacy policy in a new tab for a card with and without an email', async ({
    page,
    deployment,
    control,
    diagnostics,
  }) => {
    diagnostics.expectFailure({ method: 'GET', path: '/api/v1/invitations/not-a-real-token', status: 404 });
    const maria = await seedVerifiedUser(deployment, 'Maria', 'en');
    const invited = newTestAccount('Invited student', 'student');
    const emailed = await createCard(deployment, maria, 'Invited student', invited.email);
    await sendInvitation(deployment, maria, emailed.id);
    await control.tick();
    const links = {
      emailed: emailedLinkTo(await control.mail(invited.email), '/invite'),
      copied: (await createCard(deployment, maria, 'Copied link', null)).link,
      invalid: `${deployment.origin}/invite/not-a-real-token`,
    };

    const observed: Record<string, PolicyLinkObservation> = {};
    for (const [name, link] of Object.entries(links)) {
      observed[name] = await openThePolicyFrom(page, link, diagnostics);
    }

    for (const [name, link] of Object.entries(links)) {
      const observation = observed[name];
      expect(observation, name).toEqual({
        referrerPolicy: 'no-referrer',
        address: `${deployment.origin}/invite`,
        href: '/privacy',
        target: '_blank',
        rel: 'noopener noreferrer',
        policyAddress: `${deployment.origin}/privacy`,
        referrer: '',
        openerCleared: true,
        policyHeadings: ['Privacy policy'],
      });
      expect(observation?.address, name).not.toContain(invitationToken(link));
    }
  });

  test('invitations › a sign-up with a taken email and its own password stays on the page and accepts', async ({
    page,
    deployment,
  }) => {
    const maria = await seedVerifiedUser(deployment, 'Maria', 'en');
    const pavel = await seedVerifiedUser(deployment, 'Pavel', 'en');
    const { student } = await seedActiveStudent(deployment, pavel, 'Boris');
    const card = await createCard(deployment, maria, 'Boris', null);
    await page.goto(card.link);
    await signUpOnThePage(page, student, englishSignUp);
    await expect(page.getByRole('status')).toHaveText(`You are signed in as ${student.email}.`);
    await expect(page.getByRole('status')).toBeFocused();
    await expect(page).toHaveURL('/invite');

    await page.getByRole('button', { name: 'Accept' }).click();

    await expect(page).toHaveURL('/tutors');
    await expectTutors(myTutorRows(page), ['Pavel', 'Maria']);
  });

  test('invitations › a sign-up with a taken email and another password asks to check the email or sign in', async ({
    page,
    deployment,
    diagnostics,
  }) => {
    diagnostics.expectFailure({ method: 'POST', path: '/api/v1/auth/login', status: 401 });
    const maria = await seedVerifiedUser(deployment, 'Maria', 'en');
    const pavel = await seedVerifiedUser(deployment, 'Pavel', 'en');
    const { student } = await seedActiveStudent(deployment, pavel, 'Boris');
    const card = await createCard(deployment, maria, 'Boris', null);
    await page.goto(card.link);

    await signUpOnThePage(page, { ...student, password: `${student.password} other` }, englishSignUp);

    await expect(page.getByRole('status')).toHaveText('Check your email, or sign in to accept.');
    await expect(page.getByRole('status')).toBeFocused();
    await expect(page.getByRole('form', { name: 'Sign in to accept' })).toBeVisible();
    await expect(page.getByRole('form', { name: 'Create account' })).toHaveCount(0);
    await expect(page).toHaveURL('/invite');
    expect(await invitationIsUsable(deployment, card.link)).toBe(true);
  });

  test('students › ending a student who confirmed meanwhile closes the dialog and shows the current card', async ({
    socketSlots,
    page,
    deployment,
    control,
    diagnostics,
  }) => {
    const pavel = await seedVerifiedUser(deployment, 'Pavel', 'en');
    const boris = newTestAccount('Boris Account', 'student');
    const card = await createCard(deployment, pavel, 'Boris', boris.email);
    diagnostics.expectFailure({ method: 'POST', path: `/api/v1/students/${card.id}/remove`, status: 409 });
    await signUpOnInvitation(deployment, card.link, boris, 'en');
    await socketSlots.keepPageSocketsOff(pavel);
    await signInThroughTheApi(page, pavel);
    await page.goto(`/students/${card.id}`);
    await chooseFromPageMenu(page, 'Card actions', 'End working together');
    await confirmInTheOwnBrowser(deployment, control, boris);

    await page
      .getByRole('dialog', { name: 'End working together?' })
      .getByRole('button', { name: 'End', exact: true })
      .click();

    await expect(page.getByRole('dialog')).toHaveCount(0);
    const notice = page.getByRole('status').filter({ hasText: 'This card changed in the meantime.' });
    await expect(notice).toBeFocused();
    await expect(page.getByRole('main').getByText('Email not confirmed')).toHaveCount(0);
    await expect(page.getByRole('main').getByText('Boris Account')).toBeVisible();
    await expect(page).toHaveURL(`/students/${card.id}`);
  });

  test('students › withdrawing an invitation accepted meanwhile moves focus to the changed-card notice', async ({
    socketSlots,
    page,
    deployment,
    diagnostics,
  }) => {
    const pavel = await seedVerifiedUser(deployment, 'Pavel', 'en');
    const boris = newTestAccount('Boris Account', 'student');
    const card = await createCard(deployment, pavel, 'Boris', boris.email);
    diagnostics.expectFailure({ method: 'DELETE', path: `/api/v1/students/${card.id}`, status: 409 });
    await socketSlots.keepPageSocketsOff(pavel);
    await signInThroughTheApi(page, pavel);
    await page.goto(`/students/${card.id}`);
    await chooseFromPageMenu(page, 'Card actions', 'Withdraw invitation');
    await signUpOnInvitation(deployment, card.link, boris, 'en');

    await page
      .getByRole('dialog', { name: 'Withdraw the invitation?' })
      .getByRole('button', { name: 'Withdraw invitation', exact: true })
      .click();

    await expect(page.getByRole('status').filter({ hasText: 'This card changed in the meantime.' })).toBeFocused();
    await expect(page.getByRole('main').getByText('Email not confirmed')).toBeVisible();
  });

  test('students › a second stale refusal while the changed-card notice shows moves focus to it again', async ({
    socketSlots,
    page,
    deployment,
    control,
    diagnostics,
  }) => {
    const pavel = await seedVerifiedUser(deployment, 'Pavel', 'en');
    const boris = newTestAccount('Boris Account', 'student');
    const card = await createCard(deployment, pavel, 'Boris', boris.email);
    diagnostics.expectFailure({ method: 'DELETE', path: `/api/v1/students/${card.id}`, status: 409 });
    diagnostics.expectFailure({ method: 'POST', path: `/api/v1/students/${card.id}/remove`, status: 409 });
    await socketSlots.keepPageSocketsOff(pavel);
    await signInThroughTheApi(page, pavel);
    await page.goto(`/students/${card.id}`);
    const notice = page.getByRole('status').filter({ hasText: 'This card changed in the meantime.' });
    await chooseFromPageMenu(page, 'Card actions', 'Withdraw invitation');
    await signUpOnInvitation(deployment, card.link, boris, 'en');
    await page
      .getByRole('dialog', { name: 'Withdraw the invitation?' })
      .getByRole('button', { name: 'Withdraw invitation', exact: true })
      .click();
    await notice.waitFor();
    await chooseFromPageMenu(page, 'Card actions', 'End working together');
    await expect(
      page.getByRole('dialog', { name: 'End working together?' }).getByRole('button', { name: 'Cancel' }),
    ).toBeFocused();
    await confirmInTheOwnBrowser(deployment, control, boris);

    await page
      .getByRole('dialog', { name: 'End working together?' })
      .getByRole('button', { name: 'End', exact: true })
      .click();

    await expect(page.getByRole('dialog')).toHaveCount(0);
    await expect(notice).toBeFocused();
    await expect(notice).toHaveCount(1);
    await expect(page.getByRole('main').getByText('Email not confirmed')).toHaveCount(0);
    const cardActions = await openPageMenu(page, 'Card actions');
    await expect(cardActions.getByRole('button', { name: 'End working together' })).toBeVisible();
    await expect(page).toHaveURL(`/students/${card.id}`);
  });

  test('students › invite again on a card reactivated meanwhile moves focus to the changed-card notice', async ({
    socketSlots,
    page,
    deployment,
    diagnostics,
  }) => {
    const pavel = await seedVerifiedUser(deployment, 'Pavel', 'en');
    const { student: anna, cardId } = await seedActiveStudent(deployment, pavel, 'Anna');
    await endRelationship(deployment, pavel, cardId);
    diagnostics.expectFailure({ method: 'POST', path: `/api/v1/students/${cardId}/invitation`, status: 409 });
    await socketSlots.keepPageSocketsOff(pavel);
    await signInThroughTheApi(page, pavel);
    await page.goto(`/students/${cardId}`);
    await expect(page.getByRole('button', { name: 'Invite again' })).toBeVisible();
    await enterTutorCode(deployment, anna, await tutorCode(deployment, pavel));
    await decideJoinRequest(deployment, pavel, anna.email, 'accept');

    await page.getByRole('button', { name: 'Invite again' }).click();

    await expect(page.getByRole('status').filter({ hasText: 'This card changed in the meantime.' })).toBeFocused();
    await expect(page.getByRole('button', { name: 'Invite again' })).toHaveCount(0);
    const cardActions = await openPageMenu(page, 'Card actions');
    await expect(cardActions.getByRole('button', { name: 'End working together' })).toBeVisible();
    await expect(page.getByRole('main').getByText(/\/invite\//)).toHaveCount(0);
  });

  test('students › a provisional student is removed and a new card with a fresh link replaces it', async ({
    page,
    deployment,
  }) => {
    const pavel = await seedVerifiedUser(deployment, 'Pavel', 'en');
    const boris = newTestAccount('Boris Account', 'student');
    const card = await createCard(deployment, pavel, 'Boris', boris.email);
    await signUpOnInvitation(deployment, card.link, boris, 'en');
    await setCardNotes(deployment, pavel, card.id, 'Met at the open day.');
    await signInThroughTheApi(page, pavel);
    await page.goto(`/students/${card.id}`);
    await expect(page.getByRole('main').getByText('Email not confirmed')).toBeVisible();
    await expect(page.getByText('Boris Account')).toBeVisible();
    await chooseFromPageMenu(page, 'Card actions', 'End working together');
    const dialog = page.getByRole('dialog', { name: 'End working together?' });
    await expect(dialog).toContainText('A new card with a fresh link replaces this one');
    await expect(dialog.getByRole('button', { name: 'Cancel' })).toBeFocused();

    await dialog.getByRole('button', { name: 'End', exact: true }).click();

    await expect(page).not.toHaveURL(`/students/${card.id}`);
    await expect(page).toHaveURL(/\/students\/[0-9a-f-]{36}$/);
    await expect(page.getByRole('status').filter({ hasText: 'The link was removed.' })).toBeVisible();
    await expect(page.getByRole('heading', { level: 1, name: 'Boris' })).toBeFocused();
    await expect(page.getByRole('main').getByText('Invitation sent')).toBeVisible();
    await expect(page.getByText('Boris Account')).toHaveCount(0);
    await expect(page.getByRole('form', { name: 'Notes' }).getByLabel('Notes')).toHaveValue('Met at the open day.');
    const freshLink = await page
      .getByRole('main')
      .getByText(/\/invite\//)
      .textContent();
    expect(freshLink?.trim()).not.toBe(card.link);
    expect(await invitationIsUsable(deployment, freshLink?.trim() ?? '')).toBe(true);
    expect(await myTutorNames(deployment, boris)).toEqual([]);
  });
});

async function boxOf(
  locator: Locator,
): Promise<{ readonly top: number; readonly bottom: number; readonly width: number }> {
  const box = await locator.boundingBox();
  if (box === null) {
    throw new Error('the element has no layout box');
  }
  return { top: box.y, bottom: box.y + box.height, width: box.width };
}

async function stackedAtFullWidth(page: Page, first: Locator, second: Locator): Promise<boolean> {
  const column = await boxOf(page.locator('main article').first());
  const upper = await boxOf(first);
  const lower = await boxOf(second);
  return (
    Math.abs(upper.width - column.width) <= 1 &&
    Math.abs(lower.width - column.width) <= 1 &&
    lower.top >= upper.bottom - 0.5
  );
}

async function scrollsSideways(page: Page): Promise<boolean> {
  return page.evaluate(() => document.documentElement.scrollWidth > document.documentElement.clientWidth);
}

test.describe('an invitation on a phone in Russian', () => {
  test.use({ locale: 'ru-RU', viewport: { width: 360, height: 780 }, colorScheme: 'dark' });

  test('invitations › on a phone in Russian the privacy policy link fits the width without sideways scrolling', async ({
    page,
    deployment,
  }) => {
    const maria = await seedVerifiedUser(deployment, 'Мария', 'ru');
    const card = await createCard(deployment, maria, 'Борис', null);
    const policyLink = page.getByRole('link', { name: 'Политика конфиденциальности (откроется в новой вкладке)' });

    await page.goto(card.link);

    await page.waitForURL('**/invite');
    await policyLink.waitFor();
    const linkBox = await policyLink.boundingBox();
    const pageWidth = await page.evaluate(() => document.documentElement.clientWidth);
    expect(linkBox !== null && linkBox.x >= 0 && linkBox.x + linkBox.width <= pageWidth).toBe(true);
    expect(await scrollsSideways(page)).toBe(false);
  });

  test('invitations › on a phone in Russian the privacy policy link opens the Russian policy in a new tab', async ({
    page,
    deployment,
    diagnostics,
  }) => {
    const maria = await seedVerifiedUser(deployment, 'Мария', 'ru');
    const card = await createCard(deployment, maria, 'Борис', null);
    await page.goto(card.link);
    await page.waitForURL('**/invite');
    const policyLink = page.getByRole('link', { name: 'Политика конфиденциальности (откроется в новой вкладке)' });
    await policyLink.waitFor();
    const openedTab = page.context().waitForEvent('page');

    await policyLink.click();

    const policy = await openedTab;
    await diagnostics.watch(policy);
    await expect(policy.getByRole('heading', { level: 1 })).toHaveText('Политика конфиденциальности');
    expect(policy.url()).toBe(`${deployment.origin}/privacy`);
    expect(await policy.evaluate(() => document.documentElement.lang)).toBe('ru');
    await expect(policyLink).toHaveAttribute('target', '_blank');
    await policy.close();
  });

  test('invitations › on a phone the tutor stacks above the invitation, the choices and Accept are full width and the token stays out', async ({
    page,
    deployment,
  }) => {
    const markup = '<img src=x onerror="document.title=1"><b>Bold</b>';
    const tutorName = `${markup} Длинноеимяпреподавателябезпробеловдляширины`;
    const maria = await seedVerifiedUser(deployment, tutorName, 'ru');
    const pavel = await seedVerifiedUser(deployment, 'Павел', 'ru');
    const { student } = await seedActiveStudent(deployment, pavel, 'Борис', 'ru');
    const card = await createCard(deployment, maria, 'Борис', null);
    const token = invitationToken(card.link);
    const tutorHeading = page.getByRole('heading', { level: 2, name: tutorName });
    const opened = await page.goto(card.link);
    await tutorHeading.waitFor();
    const signedOutLayout = {
      avatarAboveName: await boxOf(tutorHeading.locator('xpath=../preceding-sibling::*[1]')).then(
        async (avatar) => avatar.bottom <= (await boxOf(tutorHeading)).top + 0.5,
      ),
      choicesStacked: await stackedAtFullWidth(
        page,
        page.getByRole('button', { name: 'Создать аккаунт' }),
        page.getByRole('button', { name: 'Войти и принять' }),
      ),
      sideways: await scrollsSideways(page),
    };
    await page.getByRole('button', { name: 'Войти и принять' }).click();
    const signIn = page.getByRole('form', { name: 'Войти и принять' });
    await signIn.getByLabel('Email').fill(student.email);
    await signIn.getByLabel('Пароль', { exact: true }).fill(student.password);
    await signIn.getByRole('button', { name: 'Войти' }).click();
    await expect(page.getByRole('status')).toBeFocused();
    const acceptLayout = {
      acceptStacked: await stackedAtFullWidth(
        page,
        page.getByRole('button', { name: 'Принять' }),
        page.getByRole('button', { name: 'Войти под другим аккаунтом' }),
      ),
      sideways: await scrollsSideways(page),
    };

    await page.getByRole('button', { name: 'Принять' }).click();

    await expect(page).toHaveURL('/tutors');
    await expect(page.getByRole('heading', { level: 1, name: 'Мои репетиторы' })).toBeFocused();
    expect(opened?.headers()['referrer-policy']).toBe('no-referrer');
    expect(signedOutLayout).toEqual({ avatarAboveName: true, choicesStacked: true, sideways: false });
    expect(acceptLayout).toEqual({ acceptStacked: true, sideways: false });
    await expectTokenNowhere(page, token);
    await expectTutors(myTutorRows(page), ['Павел', tutorName]);
    await expect(page.getByRole('main').locator('img, b')).toHaveCount(0);
    await expect(page).not.toHaveTitle('1');
  });
});
