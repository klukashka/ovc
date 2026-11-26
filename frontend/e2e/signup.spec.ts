import { AccountSwitchBoundary } from './navigation-readiness.ts';
import type { TrafficAfterReady } from './realtime.ts';
import {
  trafficAfterReady,
  todayQueryPaths,
  unverifiedTutorTodayQueryPaths,
  expectReadyForNavigation,
} from './realtime.ts';
import type { BrowserContext, Page, Request, Response, Route, Frame } from '@playwright/test';

import type { ObservedBrowser } from './browser-diagnostics.ts';

import {
  type SentMailLinks,
  type TestAccount,
  newTestAccount,
  pageSettled,
  registerTutor,
  signInThroughTheApi,
} from './accounts.ts';
import type { Deployment } from './deployment.ts';
import type { PageDiagnostics } from './fixtures.ts';
import { diagnosedTest as test, expect, seedVerifiedUser, signUpAndConfirmTutor } from './fixtures.ts';
import { recordLazyChunkAddresses } from './observers.ts';

test.use({ locale: 'ru-RU' });

function emailedLink(messages: readonly SentMailLinks[], path: string): string {
  const link = messages
    .flatMap((message) => message.links)
    .filter((candidate) => new URL(candidate).pathname === path)
    .at(-1);
  if (link === undefined) {
    throw new Error(`no emailed link to ${path}`);
  }
  return link;
}

async function freshContext(browser: ObservedBrowser, deployment: Deployment): Promise<BrowserContext> {
  return browser.newContext({ baseURL: deployment.origin, ignoreHTTPSErrors: true, locale: 'ru-RU' });
}

async function signUpThroughTheForm(page: Page, account: TestAccount): Promise<void> {
  await page.goto('/signup');
  await page.getByRole('link', { name: /Я преподаю/ }).click();
  await page.getByLabel('Имя').fill(account.name);
  await page.getByLabel('Email').fill(account.email);
  await page.getByLabel('Пароль', { exact: true }).fill(account.password);
  await page.getByLabel(/^Я даю согласие на обработку моих персональных данных/).check();
  await page.getByRole('button', { name: 'Зарегистрироваться' }).click();
}

async function expectSignedInToToday(page: Page): Promise<void> {
  await expect(page).toHaveURL('/today');
  await expect(page.getByRole('heading', { level: 1, name: 'Сегодня' })).toBeVisible();
}

function switchBarrier(): {
  readonly promise: Promise<void>;
  readonly resolve: () => void;
  readonly reject: (error: unknown) => void;
} {
  let resolve = (): void => {
    throw new Error('barrier_not_initialized');
  };
  let reject: (error: unknown) => void = (): void => {
    throw new Error('barrier_not_initialized');
  };
  const promise = new Promise<void>((accept, refuse) => {
    resolve = accept;
    reject = refuse;
  });
  return { promise, resolve, reject };
}

async function confirmAccountWithReadiness(
  page: Page,
  origin: string,
  closingProbe?: () => Promise<void>,
  duplicateProbe?: () => Promise<void>,
): Promise<void> {
  const owner = await page.evaluateHandle(() => ({ document: window.document }));
  const boundary = new AccountSwitchBoundary();
  const ids = new Map<Request, number>();
  let sequence = 0;
  const observer: { traffic: TrafficAfterReady | null } = { traffic: null };
  let logout: Request | null = null;
  let confirmationCount = 0;
  let logoutCount = 0;
  let held = false;
  let holdCount = 0;
  let confirmationAdmissions = 0;
  let lateEntries = 0;
  let lateForwarded = 0;
  const late: { request: Request | null } = { request: null };
  let closing = false;
  const isClosing = (): boolean => closing;
  const routeOperations = new Set<Promise<void>>();
  const routeFailures: Error[] = [];
  let released = false;
  let documentChanged = false;
  const release = switchBarrier();
  const routeDone = switchBarrier();
  const checkOwner = (): Promise<boolean> => page.evaluate((observed) => observed.document === window.document, owner);
  const owns = (request: Request): boolean => {
    const url = new URL(request.url());
    return (
      request.serviceWorker() === null &&
      request.frame() === page.mainFrame() &&
      url.origin === origin &&
      url.search === '' &&
      url.hash === '' &&
      request.method() === 'POST'
    );
  };
  const onRequest = (request: Request): void => {
    if (closing) return;
    const path = new URL(request.url()).pathname;
    if (path !== '/api/v1/auth/logout' && path !== '/api/v1/auth/verify-email') return;
    sequence += 1;
    ids.set(request, sequence);
    if (path === '/api/v1/auth/logout') {
      logoutCount += 1;
      logout = request;
      boundary.request(sequence, 'logout', owns(request) && !documentChanged);
    } else {
      confirmationCount += 1;
      if (boundary.request(sequence, 'confirmation', owns(request) && !documentChanged))
        observer.traffic = trafficAfterReady(page, todayQueryPaths);
    }
  };
  const onResponse = (response: Response): void => {
    if (response.request() !== logout) return;
    const id = ids.get(response.request());
    if (id === undefined) throw new Error('account_switch_logout_identity_missing');
    boundary.response(id, response.status());
  };
  const onFrame = (frame: Frame): void => {
    if (frame === page.mainFrame() && confirmationCount === 0) documentChanged = true;
  };
  const hold = (route: Route): Promise<void> => {
    const enteredWhileClosing = closing;
    if (enteredWhileClosing) {
      lateEntries += 1;
      late.request ??= route.request();
    }
    const operation = (async (): Promise<void> => {
      try {
        if (!isClosing()) {
          expect(owns(route.request())).toBe(true);
          confirmationAdmissions += 1;
          if (confirmationAdmissions !== 1) throw new Error('account_switch_duplicate_route');
          expect(await checkOwner(), 'account_switch_route_document_owner').toBe(true);
          if (!isClosing()) {
            held = true;
            holdCount += 1;
            await release.promise;
          }
        }
      } finally {
        await route.fallback();
        if (enteredWhileClosing) {
          lateForwarded += 1;
        }
      }
    })();
    const tracked = operation
      .then(
        () => {
          routeDone.resolve();
        },
        (error: unknown) => {
          routeFailures.push(new Error('account_switch_route_failed', { cause: error }));
          routeDone.reject(error);
        },
      )
      .finally(() => {
        routeOperations.delete(tracked);
      });
    routeOperations.add(tracked);
    return tracked;
  };
  const address = `${origin}/api/v1/auth/verify-email`;
  page.on('request', onRequest);
  page.on('response', onResponse);
  page.on('framenavigated', onFrame);
  const joined = routeDone.promise;
  const routeOutcome = joined.then(
    () => true,
    () => false,
  );
  const finish = async (): Promise<void> => {
    closing = true;
    if (!released) release.resolve();
    const failures: Error[] = [];
    if (closingProbe !== undefined) {
      const originalConfirmationCount = confirmationCount;
      const originalObserver = observer.traffic;
      const originalHoldCount = holdCount;
      try {
        await closingProbe();
        expect(late.request, 'account_switch_late_request_observed').not.toBeNull();
        const response = await late.request?.response();
        expect(response, 'account_switch_late_response_present').not.toBeNull();
        expect(await response?.finished(), 'account_switch_late_request_terminal').toBeNull();
        expect(lateEntries, 'account_switch_late_callback_entered').toBe(1);
        expect(lateForwarded, 'account_switch_late_callback_forwarded').toBe(1);
        expect(confirmationCount, 'account_switch_closing_confirmation_not_enrolled').toBe(originalConfirmationCount);
        expect(observer.traffic === originalObserver, 'account_switch_closing_observer_unchanged').toBe(true);
        expect(holdCount, 'account_switch_closing_hold_not_enrolled').toBe(originalHoldCount);
      } catch (error: unknown) {
        failures.push(new Error('account_switch_closing_probe_failed', { cause: error }));
      }
    }
    try {
      await page.unroute(address, hold);
    } catch (error) {
      failures.push(new Error('account_switch_route_removal_failed', { cause: error }));
    }
    while (routeOperations.size > 0) await Promise.all([...routeOperations]);
    page.off('request', onRequest);
    page.off('response', onResponse);
    page.off('framenavigated', onFrame);
    const disposed = await Promise.allSettled([observer.traffic?.dispose(), owner.dispose()]);
    for (const result of disposed)
      if (result.status === 'rejected')
        failures.push(new Error('account_switch_disposal_failed', { cause: result.reason }));
    failures.push(...routeFailures);
    if (failures.length > 0) throw new AggregateError(failures, 'account_switch_cleanup_failed');
  };
  const failures: unknown[] = [];
  try {
    await page.route(address, hold);
    await page.getByRole('button', { name: 'Подтвердить и войти' }).click();
    await expect.poll(() => held).toBe(true);
    expect(boundary.valid()).toBe(true);
    expect(logoutCount).toBe(1);
    expect(confirmationCount).toBe(1);
    const observed = observer.traffic;
    if (observed === null) throw new Error('account_switch_observer_missing');
    expect(observed.readyReceived()).toBe(false);
    release.resolve();
    released = true;
    expect(await routeOutcome).toBe(true);
    await expectSignedInToToday(page);
    await expectReadyForNavigation(page, observed);
    await duplicateProbe?.();
    if (!boundary.valid()) throw new Error('account_switch_boundary_invalid');
  } catch (error: unknown) {
    failures.push(error);
  } finally {
    try {
      await finish();
    } catch (error: unknown) {
      failures.push(error);
    }
  }
  if (failures.length > 0) throw new AggregateError(failures, 'account_switch_failed');
}

async function confirmAccountWithCleanupProof(
  page: Page,
  origin: string,
  diagnostics: PageDiagnostics,
  duplicate = false,
): Promise<void> {
  const address = `${origin}/api/v1/auth/verify-email`;
  let borrowed = 0;
  const sentinel = async (route: Route): Promise<void> => {
    borrowed += 1;
    await route.fallback();
  };
  const probe = async (): Promise<void> => {
    diagnostics.expectFailure({ method: 'POST', path: '/api/v1/auth/verify-email', status: 422 });
    const response = page.waitForResponse(
      (candidate) => candidate.url() === address && candidate.request().method() === 'POST',
    );
    const request = page.evaluate(async () => {
      const result = await fetch('/api/v1/auth/verify-email', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ token: 'account-switch-cleanup-invalid-token' }),
      });
      await result.arrayBuffer();
      return result.status;
    });
    const [status, terminal] = await Promise.all([request, response]);
    expect(status, 'account_switch_probe_refused').toBe(422);
    expect(terminal.status(), 'account_switch_probe_response_refused').toBe(422);
    expect(await terminal.finished(), 'account_switch_probe_exact_request_terminal').toBeNull();
    await diagnostics.finishHttpPhase();
  };
  const failures: unknown[] = [];
  try {
    await page.route(address, sentinel);
    const outcome = await confirmAccountWithReadiness(page, origin, probe, duplicate ? probe : undefined).then(
      () => null,
      (error: unknown) => error,
    );
    if (duplicate) {
      expect(outcome, 'account_switch_duplicate_failed').toBeInstanceOf(AggregateError);
      const combined: readonly unknown[] = outcome instanceof AggregateError ? outcome.errors : [];
      const [body, cleanup] = combined;
      expect(
        body instanceof Error && body.message === 'account_switch_boundary_invalid',
        'account_switch_body_failure_retained',
      ).toBe(true);
      const routeFailures: readonly unknown[] = cleanup instanceof AggregateError ? cleanup.errors : [];
      const routeFailure = routeFailures[0];
      expect(
        routeFailure instanceof Error &&
          routeFailure.cause instanceof Error &&
          routeFailure.cause.message === 'account_switch_duplicate_route',
        'account_switch_duplicate_failure_retained',
      ).toBe(true);
      expect(
        cleanup instanceof AggregateError && cleanup.message === 'account_switch_cleanup_failed',
        'account_switch_cleanup_failure_retained',
      ).toBe(true);
      expect(combined, 'account_switch_exact_body_and_cleanup_failures').toHaveLength(2);
      expect(routeFailures, 'account_switch_no_other_cleanup_failures').toHaveLength(1);
      expect(
        routeFailure instanceof Error && routeFailure.message === 'account_switch_route_failed',
        'account_switch_route_failure_retained',
      ).toBe(true);
    } else if (outcome !== null) {
      throw new Error('account_switch_unexpected_failure', { cause: outcome });
    }
    expect(borrowed, 'account_switch_borrowed_route_during_closing').toBe(duplicate ? 3 : 2);
    await probe();
    expect(borrowed, 'account_switch_borrowed_route_after_cleanup').toBe(duplicate ? 4 : 3);
  } catch (error: unknown) {
    failures.push(error);
  } finally {
    try {
      await page.unroute(address, sentinel);
    } catch (error: unknown) {
      failures.push(error);
    }
  }
  if (failures.length > 0) throw new AggregateError(failures, 'account_switch_cleanup_proof_failed');
}

test('pilot-1 › steps 1–3 sign up, confirm in a fresh browser with a new password, see the first-run checklist, then save the bio and see its step done', async ({
  diagnostics,
  page,
  observedBrowser: browser,
  deployment,
  control,
}) => {
  const maria = newTestAccount('Мария');
  await signUpThroughTheForm(page, maria);
  await expect(page.getByRole('heading', { level: 1, name: 'Проверьте почту' })).toBeVisible();
  await expect(page.getByText(maria.email)).toBeVisible();
  await control.tick();
  const link = emailedLink(await control.mail(maria.email), '/verify-email');
  const fresh = await freshContext(browser, deployment);
  const confirming = await fresh.newPage();
  const confirmingDiagnostics = await diagnostics.watch(confirming);
  confirmingDiagnostics.expectFailure({ method: 'POST', path: '/api/v1/auth/verify-email', status: 422 });
  const confirmingReady = trafficAfterReady(confirming, todayQueryPaths);
  await confirmingReady.initialize();

  await confirming.goto(link);
  await expect(confirming).toHaveURL('/verify-email');
  await confirming.getByLabel('Новый пароль').fill(maria.password);
  await confirmingDiagnostics.finishHttpPhase();
  await confirming.getByRole('button', { name: 'Подтвердить и войти' }).click();

  await expectSignedInToToday(confirming);
  await expectReadyForNavigation(confirming, confirmingReady);
  await expect(confirming.getByRole('region', { name: 'Подтверждение email' })).toHaveCount(0);
  const checklist = confirming.getByRole('region', { name: 'С чего начать' });
  await expect(checklist.getByRole('listitem').first()).toHaveText(/Заполните профиль.*ещё не выполнено/);
  await confirming.goto('/settings/profile');
  await confirming.getByLabel('О себе').fill('Английский для взрослых, разговорная практика.');
  await confirming.getByRole('button', { name: 'Сохранить' }).click();
  await expect(confirming.getByRole('status')).toHaveText('Сохранено.');
  await confirming.reload();
  await expect(confirming.getByLabel('О себе')).toHaveValue('Английский для взрослых, разговорная практика.');
  await confirming.goto('/today');
  await expect(checklist.getByRole('listitem').first()).toHaveText(/Заполните профиль.*выполнено/);
  await expect(checklist.getByRole('listitem').first()).not.toHaveText(/ещё не выполнено/);
  await diagnostics.closeContext(fresh);
});

test('pilot-3 › step 1 seeded tutors are confirmed and sign in to Today', async ({ page, deployment }) => {
  const maria = await signUpAndConfirmTutor(deployment, 'Мария');
  const anotherTutor = await signUpAndConfirmTutor(deployment, 'Ольга');

  await signInThroughTheApi(page, maria);
  const firstReady = trafficAfterReady(page, todayQueryPaths);
  await firstReady.initialize();
  await page.goto('/today');
  await expectSignedInToToday(page);
  await expectReadyForNavigation(page, firstReady);
  await signInThroughTheApi(page, anotherTutor);
  const secondReady = trafficAfterReady(page, todayQueryPaths);
  await secondReady.initialize();
  await page.goto('/today');

  await expectSignedInToToday(page);
  await expect(page.getByRole('region', { name: 'Подтверждение email' })).toHaveCount(0);
  await expect(page.getByRole('button', { name: 'Ольга, Меню аккаунта' })).toBeVisible();
  await expectReadyForNavigation(page, secondReady);
});

test('the confirmation link opened in a signed-in browser asks for no password and ends other sessions', async ({
  diagnostics,
  page,
  observedBrowser: browser,
  deployment,
  control,
}) => {
  const account = newTestAccount('Unverified tutor');
  await registerTutor(deployment, account, 'ru');
  await control.tick();
  const link = emailedLink(await control.mail(account.email), '/verify-email');
  await signInThroughTheApi(page, account);
  const elsewhere = await freshContext(browser, deployment);
  const otherPage = await elsewhere.newPage();
  const otherDiagnostics = await diagnostics.watch(otherPage);
  await signInThroughTheApi(otherPage, account);
  const otherReady = trafficAfterReady(otherPage, unverifiedTutorTodayQueryPaths);
  await otherReady.initialize();
  await otherPage.goto('/today');
  await expect(otherPage.getByRole('region', { name: 'Подтверждение email' })).toBeVisible();
  await expectReadyForNavigation(otherPage, otherReady);
  const tab = await page.context().newPage();
  await diagnostics.watch(tab);
  const tabReady = trafficAfterReady(tab, todayQueryPaths);
  await tabReady.initialize();
  otherDiagnostics.expectFailure({ method: 'POST', path: '/api/v1/auth/refresh', status: 401 });
  await tab.goto(link);
  await expectSignedInToToday(tab);
  await expect(tab.getByLabel('Новый пароль')).toHaveCount(0);
  await expect(tab.getByRole('region', { name: 'Подтверждение email' })).toHaveCount(0);

  await expectReadyForNavigation(tab, tabReady);
  await expect(otherPage.getByRole('heading', { level: 1, name: 'Вход' })).toBeVisible();
  await otherDiagnostics.finishHttpPhase();
  await pageSettled(otherPage);
  await otherPage.reload();

  await expect(otherPage.getByRole('heading', { level: 1, name: 'Вход' })).toBeVisible();
  await diagnostics.closeContext(elsewhere);
});

test.describe('on a phone', () => {
  test.use({ viewport: { width: 360, height: 780 } });

  test('the resend button waits 60 seconds after sign-up', async ({ page }) => {
    await page.clock.install();
    await signUpThroughTheForm(page, newTestAccount('Patient tutor'));
    await expect(page.getByRole('button', { name: 'Отправить ещё раз' })).toBeDisabled();
    await expect(page.getByText(/^Можно отправить через \d+ с\.$/)).toBeVisible();
    const resendWidth = (await page.getByRole('button', { name: 'Отправить ещё раз' }).boundingBox())?.width;
    const columnWidth = (await page.locator('main article').first().boundingBox())?.width;

    await page.clock.runFor('01:01');
    await page.getByRole('button', { name: 'Отправить ещё раз' }).click();

    await expect(page.getByRole('status')).toHaveText('Письмо отправлено ещё раз.');
    expect(resendWidth).toBeDefined();
    expect(Math.abs((resendWidth ?? 0) - (columnWidth ?? 0))).toBeLessThanOrEqual(1);
  });
});

test('a resend starts the 60-second countdown again and it counts down each second', async ({ page }) => {
  await page.clock.install();
  await signUpThroughTheForm(page, newTestAccount('Repeating tutor'));
  await expect(page.getByText(/^Можно отправить через \d+ с\.$/)).toBeVisible();
  await page.clock.pauseAt((await page.evaluate(() => Date.now())) + 61_000);
  await page.getByRole('button', { name: 'Отправить ещё раз' }).click();
  await expect(page.getByText('Можно отправить через 60 с.')).toBeVisible();

  await page.clock.runFor(1000);

  await expect(page.getByRole('status')).toHaveText('Письмо отправлено ещё раз.');
  await expect(page.getByText('Можно отправить через 59 с.')).toBeVisible();
  await expect(page.getByRole('button', { name: 'Отправить ещё раз' })).toBeDisabled();
});

test('a signed-in unverified tutor resends from the confirmation page and is told when to try again', async ({
  diagnostics,
  page,
  deployment,
}) => {
  const account = newTestAccount('Hasty tutor');
  await registerTutor(deployment, account, 'ru');
  await signInThroughTheApi(page, account);
  const ready = trafficAfterReady(page, unverifiedTutorTodayQueryPaths);
  await ready.initialize();
  await page.goto('/today');
  await expectReadyForNavigation(page, ready);
  await page
    .getByRole('region', { name: 'Подтверждение email' })
    .getByRole('link', { name: 'Подтвердить email' })
    .click();

  diagnostics.expectFailure({ method: 'POST', path: '/api/v1/auth/verify-email/resend', status: 429 });
  await page.getByRole('button', { name: 'Отправить ещё раз' }).click();

  await expect(page.getByRole('heading', { level: 1, name: 'Проверьте почту' })).toBeVisible();
  await expect(page.getByText(account.email)).toBeVisible();
  await expect(page.getByRole('alert')).toHaveText(/^Слишком много попыток, попробуйте через 1 минуту\.$/);
  await diagnostics.finishHttpPhase();
});

test('an invalid confirmation link is refused without asking for a password', async ({ diagnostics, page }) => {
  diagnostics.expectFailure({ method: 'POST', path: '/api/v1/auth/verify-email', status: 422 });
  await page.goto('/verify-email?token=not-a-real-token');

  await expect(page.getByRole('alert')).toHaveText('Ссылка недействительна или устарела.');
  await expect(page).toHaveURL('/verify-email');
  await expect(page.getByLabel('Новый пароль')).toHaveCount(0);
  await diagnostics.finishHttpPhase();
});

test('reloading a token page after the token was removed asks to open the link again', async ({
  page,
  diagnostics,
}) => {
  diagnostics.expectFailure({ method: 'POST', path: '/api/v1/auth/verify-email', status: 422 });
  await page.goto('/verify-email?token=not-a-real-token');
  await expect(page).toHaveURL('/verify-email');
  await expect(page.getByRole('alert')).toHaveText('Ссылка недействительна или устарела.');
  await diagnostics.finishHttpPhase();

  await page.reload();

  await expect(page.getByText('Откройте ссылку из письма ещё раз.')).toBeVisible();
});

test('a forgotten password is reset through the emailed link and the new password signs in', async ({
  page,
  deployment,
  control,
}) => {
  const account = await seedVerifiedUser(deployment, 'Forgetful tutor');
  await control.advance(61);
  await page.goto('/forgot-password');
  await page.getByLabel('Email').fill(account.email);
  await page.getByRole('button', { name: 'Отправить ссылку' }).click();
  await expect(page.getByRole('status')).toContainText(account.email);
  await control.tick();
  const link = emailedLink(await control.mail(account.email), '/reset-password');
  const newPassword = `${account.password} renewed`;
  await page.goto(link);
  await expect(page).toHaveURL('/reset-password');
  await page.getByLabel('Новый пароль').fill(newPassword);
  await page.getByRole('button', { name: 'Сохранить пароль' }).click();
  await expect(page.getByRole('status')).toHaveText('Пароль изменён. Войдите с новым паролем.');
  await expect(page.getByRole('status')).toBeFocused();

  await page.getByRole('main').getByRole('link', { name: 'Войти', exact: true }).click();
  await page.getByLabel('Email').fill(account.email);
  await page.getByLabel('Пароль', { exact: true }).fill(newPassword);
  await page.getByRole('button', { name: 'Войти', exact: true }).click();

  await expectSignedInToToday(page);
});

test('a student sign-up with a code that is not valid creates nothing and says so under the code', async ({
  diagnostics,
  page,
  control,
}) => {
  const student = newTestAccount('Ученик без кода', 'student');
  await page.goto('/signup');
  await page.getByRole('link', { name: /Я учусь/ }).click();
  await expect(page).toHaveURL('/signup/student');
  await expect(page.getByText('Получили ссылку от репетитора? Откройте её вместо этого.')).toBeVisible();
  await page.getByLabel('Код репетитора').fill('2222-2222');
  await page.getByLabel('Имя').fill(student.name);
  await page.getByLabel('Email').fill(student.email);
  await page.getByLabel('Пароль', { exact: true }).fill(student.password);
  await page.getByLabel(/^Я даю согласие на обработку моих персональных данных/).check();

  diagnostics.expectFailure({ method: 'POST', path: '/api/v1/auth/register', status: 422 });
  await page.getByRole('button', { name: 'Зарегистрироваться' }).click();

  await expect(page.getByRole('alert')).toHaveText('Проверьте выделенные поля.');
  await expect(page.getByRole('alert')).toBeFocused();
  await expect(page.getByText('Этот код больше не действует — попросите у репетитора новый.')).toBeVisible();
  await expect(page.getByLabel('Код репетитора')).toHaveAttribute('aria-invalid', 'true');
  await control.tick();
  expect(await control.mail(student.email)).toEqual([]);
  await diagnostics.finishHttpPhase();
});

test('a signed-in tutor opening sign-up is sent to Today', async ({ signedInPage: page }) => {
  await page.goto('/signup/tutor');

  await expectSignedInToToday(page);
});

test('confirming another account in a signed-in browser warns, signs out first and leaves no tab acting as the old account', async ({
  diagnostics,
  page,
  deployment,
  control,
}) => {
  const first = await seedVerifiedUser(deployment, 'Первый репетитор');
  const second = newTestAccount('Второй репетитор');
  await registerTutor(deployment, second, 'ru');
  await control.tick();
  const link = emailedLink(await control.mail(second.email), '/verify-email');
  await page.context().clock.install();
  await signInThroughTheApi(page, first);
  const firstReady = trafficAfterReady(page, todayQueryPaths);
  await firstReady.initialize();
  await page.goto('/today');
  await expectReadyForNavigation(page, firstReady);
  const otherTab = await page.context().newPage();
  await diagnostics.watch(otherTab);
  const otherReady = trafficAfterReady(otherTab, todayQueryPaths);
  await otherReady.initialize();
  await otherTab.goto('/today');
  await expect(otherTab.getByRole('button', { name: 'Первый репетитор, Меню аккаунта' })).toBeVisible();
  await expectReadyForNavigation(otherTab, otherReady);
  diagnostics.expectFailure({ method: 'POST', path: '/api/v1/auth/verify-email', status: 422 });
  await page.goto(link);
  await expect(page.getByRole('status')).toContainText(first.email);

  await page.getByLabel('Новый пароль').fill(second.password);
  await diagnostics.finishHttpPhase();
  await confirmAccountWithCleanupProof(page, deployment.origin, diagnostics);
  const refreshed = page.waitForResponse((response) => new URL(response.url()).pathname === '/api/v1/auth/refresh');
  await page.clock.fastForward('12:00');
  const refreshResponse = await refreshed;
  expect(refreshResponse.status()).toBe(200);
  expect(await refreshResponse.finished()).toBeNull();

  await expect(page.getByRole('button', { name: 'Второй репетитор, Меню аккаунта' })).toBeVisible();
  await expect(otherTab.getByRole('heading', { level: 1, name: 'Вход' })).toBeVisible();
});

test('account-switch cleanup preserves body and route failures after a real duplicate confirmation', async ({
  diagnostics,
  page,
  deployment,
  control,
}) => {
  const first = await seedVerifiedUser(deployment, 'Original tutor');
  const second = newTestAccount('Confirming tutor');
  await registerTutor(deployment, second, 'ru');
  await control.tick();
  const link = emailedLink(await control.mail(second.email), '/verify-email');
  await signInThroughTheApi(page, first);
  diagnostics.expectFailure({ method: 'POST', path: '/api/v1/auth/verify-email', status: 422 });
  await page.goto(link);
  await expect(page.getByRole('status')).toContainText(first.email);
  await page.getByLabel('Новый пароль').fill(second.password);
  await diagnostics.finishHttpPhase();

  await confirmAccountWithCleanupProof(page, deployment.origin, diagnostics, true);

  await expectSignedInToToday(page);
  await expect(page.getByRole('button', { name: 'Confirming tutor, Меню аккаунта' })).toBeVisible();
});

test('a confirmation link with an over-long token leaves the address before any page chunk loads and is refused', async ({
  diagnostics,
  page,
  deployment,
}) => {
  const chunkAddresses = await recordLazyChunkAddresses(page, deployment.origin);

  diagnostics.expectFailure({ method: 'POST', path: '/api/v1/auth/verify-email', status: 422 });
  await page.goto(`/verify-email?token=${'a'.repeat(65)}`);

  await expect(page.getByRole('alert')).toHaveText('Ссылка недействительна или устарела.');
  await expect(page.getByLabel('Новый пароль')).toHaveCount(0);
  expect(chunkAddresses).not.toEqual([]);
  expect(chunkAddresses.filter((address) => address.includes('token='))).toEqual([]);
  await diagnostics.finishHttpPhase();
});

test('a signed-in account can sign out from the public frame', async ({ page, deployment }) => {
  const account = newTestAccount('Leaving tutor');
  await registerTutor(deployment, account, 'ru');
  await signInThroughTheApi(page, account);
  await page.goto('/verify-email');

  await page.getByRole('banner').getByRole('button', { name: 'Выйти' }).click();

  await expect(page.getByRole('navigation').getByRole('link', { name: 'Войти' })).toBeVisible();
  await expect(page.getByRole('banner').getByRole('button', { name: 'Выйти' })).toHaveCount(0);
});

interface ResetRequestHold {
  readonly method: 'GET' | 'POST';
  readonly address: string;
  readonly release: () => void;
  readonly wait: Promise<void>;
  request: Request | null;
  count: number;
}

async function holdResetRefusals(
  page: Page,
  origin: string,
): Promise<{
  readonly refresh: ResetRequestHold;
  readonly account: ResetRequestHold;
  readonly dispose: () => Promise<void>;
}> {
  const make = (method: 'GET' | 'POST', path: string): ResetRequestHold => {
    const barrier = switchBarrier();
    return {
      method,
      address: `${origin}${path}`,
      release: barrier.resolve,
      wait: barrier.promise,
      request: null,
      count: 0,
    };
  };
  const refresh = make('POST', '/api/v1/auth/refresh');
  const account = make('GET', '/api/v1/users/me');
  const operations = new Set<Promise<void>>();
  const failures: unknown[] = [];
  const routes: { address: string; handler: (route: Route) => Promise<void> }[] = [];
  let closing = false;
  const dispose = async (): Promise<void> => {
    closing = true;
    refresh.release();
    account.release();
    for (const { address, handler } of routes) {
      try {
        await page.unroute(address, handler);
      } catch (error: unknown) {
        failures.push(error);
      }
    }
    while (operations.size > 0) await Promise.all([...operations]);
    if (failures.length > 0) throw new AggregateError(failures, 'reset_refusal_route_cleanup_failed');
  };
  try {
    for (const hold of [refresh, account]) {
      const handler = (route: Route): Promise<void> => {
        const operation = (async (): Promise<void> => {
          try {
            if (!closing) {
              const request = route.request();
              hold.count += 1;
              expect(request.url()).toBe(hold.address);
              if (request.method() !== hold.method) throw new Error('reset_refusal_wrong_method');
              expect(request.serviceWorker()).toBeNull();
              expect(request.frame()).toBe(page.mainFrame());
              expect(hold.count).toBe(1);
              hold.request = request;
              await hold.wait;
            }
          } finally {
            await route.fallback();
          }
        })();
        const tracked = operation
          .catch((error: unknown) => {
            failures.push(error);
          })
          .finally(() => {
            operations.delete(tracked);
          });
        operations.add(tracked);
        return tracked;
      };
      routes.push({ address: hold.address, handler });
      await page.route(hold.address, handler);
    }
  } catch (error: unknown) {
    failures.push(error);
    await dispose();
    throw new Error('reset_refusal_setup_failed');
  }
  return { refresh, account, dispose };
}

test('reset refusal holds reject a real wrong-method request and preserve a borrowed route', async ({
  diagnostics,
  page,
  deployment,
}) => {
  const startup = page.waitForResponse(`${deployment.origin}/api/v1/auth/refresh`);
  await page.goto('/');
  expect(await (await startup).finished()).toBeNull();
  const address = `${deployment.origin}/api/v1/auth/refresh`;
  let borrowed = 0;
  const sentinel = async (route: Route): Promise<void> => {
    borrowed += 1;
    await route.fallback();
  };
  await page.route(address, sentinel);
  const holds = await holdResetRefusals(page, deployment.origin);
  holds.refresh.release();
  holds.account.release();
  diagnostics.expectFailure({ method: 'GET', path: '/api/v1/auth/refresh', status: 405 });
  const failures: unknown[] = [];
  let disposed = false;
  const probe = (): Promise<number> =>
    page.evaluate(async () => {
      const response = await fetch('/api/v1/auth/refresh');
      await response.arrayBuffer();
      return response.status;
    });

  try {
    const first = await probe();
    await diagnostics.finishHttpPhase();
    const outcome = await holds.dispose().then(
      () => null,
      (error: unknown) => error,
    );
    disposed = true;
    diagnostics.expectFailure({ method: 'GET', path: '/api/v1/auth/refresh', status: 405 });
    const second = await probe();

    expect([first, second]).toEqual([405, 405]);
    expect(outcome).toBeInstanceOf(AggregateError);
    if (!(outcome instanceof AggregateError)) throw new Error('reset_refusal_negative_control_insensitive');
    const errors: readonly unknown[] = outcome.errors;
    expect(errors).toHaveLength(1);
    expect(errors[0]).toBeInstanceOf(Error);
    expect(errors[0] instanceof Error && errors[0].message).toBe('reset_refusal_wrong_method');
    expect(holds.refresh.count).toBe(1);
    expect(borrowed).toBe(2);
    await diagnostics.finishHttpPhase();
  } catch (error: unknown) {
    failures.push(error);
  } finally {
    if (!disposed) {
      try {
        await holds.dispose();
      } catch (error: unknown) {
        failures.push(error);
      }
    }
    try {
      await page.unroute(address, sentinel);
    } catch (error: unknown) {
      failures.push(error);
    }
  }
  if (failures.length > 0) throw new AggregateError(failures, 'reset_refusal_negative_control_failed');
});

test('resetting the password in a browser signed in to that account ends its session there', async ({
  diagnostics,
  page,
  deployment,
  control,
}) => {
  const account = await seedVerifiedUser(deployment, 'Resetting tutor');
  await control.advance(61);
  await signInThroughTheApi(page, account);
  await page.goto('/forgot-password');
  await page.getByLabel('Email').fill(account.email);
  await page.getByRole('button', { name: 'Отправить ссылку' }).click();
  await expect(page.getByRole('status')).toContainText(account.email);
  await control.tick();
  const initialAccount = page.waitForResponse(
    (response) =>
      response.request().method() === 'GET' &&
      response.url() === `${deployment.origin}/api/v1/users/me` &&
      response.status() === 200,
  );
  const traffic = trafficAfterReady(page, []);
  try {
    await traffic.initialize();
    await traffic.prepareNavigation();
    await page.goto(emailedLink(await control.mail(account.email), '/reset-password'));
    expect(await (await initialAccount).finished()).toBeNull();
    await expectReadyForNavigation(page, traffic);
  } finally {
    await traffic.dispose();
  }
  await expect(page.getByRole('banner').getByRole('button', { name: 'Выйти' })).toBeVisible();
  const holds = await holdResetRefusals(page, deployment.origin);

  try {
    await page.getByLabel('Новый пароль').fill(`${account.password} renewed`);
    diagnostics.expectFailure({ method: 'GET', path: '/api/v1/users/me', status: 401 });
    diagnostics.expectFailure({ method: 'POST', path: '/api/v1/auth/refresh', status: 401 });
    await page.getByRole('button', { name: 'Сохранить пароль' }).click();
    await expect.poll(() => holds.refresh.request !== null && holds.account.request !== null).toBe(true);
    holds.refresh.release();
    const refreshed = await holds.refresh.request?.response();
    expect(refreshed?.status()).toBe(401);
    await expect(page.getByRole('navigation').getByRole('link', { name: 'Войти' })).toBeVisible();
    holds.account.release();
    const refused = await holds.account.request?.response();

    expect(refused?.status()).toBe(401);
    expect(await refused?.json()).toMatchObject({ error: { code: 'unauthenticated' } });
    expect(await refused?.finished()).toBeNull();
    expect(await refreshed?.finished()).toBeNull();
    await expect(page.getByRole('status')).toHaveText('Пароль изменён. Войдите с новым паролем.');
    await expect(page.getByRole('navigation').getByRole('link', { name: 'Войти' })).toBeVisible();
    await expect(page.getByRole('banner').getByRole('button', { name: 'Выйти' })).toHaveCount(0);
    await diagnostics.finishHttpPhase();
    expect([holds.refresh.count, holds.account.count]).toEqual([1, 1]);
  } finally {
    await holds.dispose();
  }
});

test('a problem the server finds is shown under the field it names', async ({ diagnostics, page }) => {
  const account = newTestAccount('   ');

  diagnostics.expectFailure({ method: 'POST', path: '/api/v1/auth/register', status: 422 });
  await signUpThroughTheForm(page, account);

  await expect(page.getByRole('alert')).toHaveText('Проверьте выделенные поля.');
  await expect(page.getByLabel('Имя')).toHaveAttribute('aria-invalid', 'true');
  await expect(page.getByText('Проверьте значение.')).toBeVisible();
  await diagnostics.finishHttpPhase();
});

test('a reset link with an over-long token leaves the address before any page chunk loads and is refused on sending', async ({
  diagnostics,
  page,
  deployment,
}) => {
  const chunkAddresses = await recordLazyChunkAddresses(page, deployment.origin);
  await page.goto(`/reset-password?token=${'a'.repeat(65)}`);
  await page.getByLabel('Новый пароль').fill('a sufficiently long password');

  diagnostics.expectFailure({ method: 'POST', path: '/api/v1/auth/password/reset', status: 422 });
  await page.getByRole('button', { name: 'Сохранить пароль' }).click();

  await expect(page.getByRole('alert')).toHaveText('Ссылка недействительна или устарела.');
  await expect(page.getByRole('link', { name: 'Запросить новую ссылку' })).toBeVisible();
  expect(chunkAddresses).not.toEqual([]);
  expect(chunkAddresses.filter((address) => address.includes('token='))).toEqual([]);
  await diagnostics.finishHttpPhase();
});
