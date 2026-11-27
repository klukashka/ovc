import { readFile } from 'node:fs/promises';

import type { Locator, Page, Response } from '@playwright/test';

import { type TestAccount, signInThroughTheApi } from './accounts.ts';
import type { Deployment } from './deployment.ts';
import { type TestControl, diagnosedTest as test, expect, seedVerifiedUser } from './fixtures.ts';
import { accessToken, callApi, seedActiveStudent } from './tutoring.ts';
import { expectDangerConfirmation, expectLastDangerItem, openedMenu } from './dangerActions.ts';

const zipSignature = 'PK\u0003\u0004';
const exportReadyTimeout = 30_000;
const shownAfterRequest = /^Your export is (waiting to start|being prepared|ready)\.$/;

async function openMyData(page: Page, control: TestControl, account: TestAccount): Promise<Response | null> {
  await page.clock.install({ time: await control.now() });
  await signInThroughTheApi(page, account);
  const response = await page.goto('/settings/data');
  await expect(page.getByRole('heading', { level: 1, name: 'My data' })).toBeVisible();
  return response;
}

function exportSection(page: Page): Locator {
  return page.getByRole('region', { name: 'Export of your data' });
}

function requestDialog(page: Page): Locator {
  return page.getByRole('dialog', { name: 'Request an export' });
}

async function requestThroughThePage(page: Page, password: string): Promise<void> {
  await exportSection(page).getByRole('button', { name: 'Request an export' }).click();
  await requestDialog(page).getByLabel('Current password').fill(password);
  await requestDialog(page).getByRole('button', { name: 'Request export' }).click();
  await expect(requestDialog(page)).toBeHidden();
}

async function exportState(deployment: Deployment, token: string): Promise<unknown> {
  const answer = await callApi(deployment, token, 'GET', '/exports/current');
  const body = answer.body;
  const current = typeof body === 'object' && body !== null && 'export' in body ? body.export : null;
  return typeof current === 'object' && current !== null && 'state' in current ? current.state : null;
}

function deletionDialog(page: Page): Locator {
  return page.getByRole('dialog', { name: 'Delete the export?' });
}

async function openDeletion(page: Page): Promise<Locator> {
  await exportSection(page).getByRole('button', { name: 'Export actions', exact: true }).click();
  await openedMenu(page).getByRole('button', { name: 'Delete export', exact: true }).click();
  return deletionDialog(page);
}

async function seedReadyExport(deployment: Deployment, control: TestControl, account: TestAccount): Promise<void> {
  const token = await accessToken(deployment, account);
  const requested = await callApi(deployment, token, 'POST', '/exports', { password: account.password });
  expect(requested.status).toBe(202);
  await control.tick();
  await expect.poll(() => exportState(deployment, token), { timeout: exportReadyTimeout }).toBe('ready');
}

interface DownloadEvidence {
  readonly suggestedName: string;
  readonly firstBytes: string;
  readonly addressHolders: number;
  readonly addressInDocument: boolean;
  readonly urlBefore: string;
  readonly urlAfter: string;
}

function mintedAddress(response: Response): Promise<string> {
  return response
    .json()
    .then((body: unknown) =>
      typeof body === 'object' && body !== null && 'url' in body && typeof body.url === 'string' ? body.url : '',
    );
}

async function downloadThroughThePage(page: Page): Promise<DownloadEvidence> {
  const urlBefore = page.url();
  const minted = page.waitForResponse(
    (response) => response.request().method() === 'POST' && response.url().endsWith('/api/v1/exports/current/download'),
  );
  const downloading = page.waitForEvent('download');
  await exportSection(page).getByRole('button', { name: 'Download' }).click();
  const address = await mintedAddress(await minted);
  const download = await downloading;
  const saved = await download.path();
  const bytes = await readFile(saved);
  const holders = await page.evaluate(
    (signed) =>
      [...document.querySelectorAll('a, iframe, img')].filter(
        (element) => element.getAttribute('href') === signed || element.getAttribute('src') === signed,
      ).length,
    address,
  );
  const inDocument = await page.evaluate((signed) => document.documentElement.outerHTML.includes(signed), address);
  return {
    suggestedName: download.suggestedFilename(),
    firstBytes: bytes.subarray(0, 4).toString('latin1'),
    addressHolders: address === '' ? -1 : holders,
    addressInDocument: inDocument,
    urlBefore,
    urlAfter: page.url(),
  };
}

function expectDownloadedArchive(evidence: DownloadEvidence): void {
  expect(evidence.suggestedName).toMatch(/^ovc-export-\d{4}-\d{2}-\d{2}\.zip$/);
  expect(evidence.firstBytes).toBe(zipSignature);
  expect(evidence.urlAfter).toBe(evidence.urlBefore);
  expect(evidence.addressHolders).toBe(0);
  expect(evidence.addressInDocument).toBe(false);
}

test.describe('My data in English on a computer', () => {
  test.use({ locale: 'en-GB', timezoneId: 'Europe/Moscow', viewport: { width: 1280, height: 800 } });

  test('the request dialog opens on the password field and refuses a wrong password inline with focus kept', async ({
    page,
    deployment,
    control,
    diagnostics,
  }) => {
    const tutor = await seedVerifiedUser(deployment, 'Maria', 'en');
    await openMyData(page, control, tutor);
    await exportSection(page).getByRole('button', { name: 'Request an export' }).click();
    const passwordField = requestDialog(page).getByLabel('Current password');
    await expect(passwordField).toBeFocused();
    await passwordField.fill(`${tutor.password} but wrong`);
    diagnostics.expectFailure({ method: 'POST', path: '/api/v1/exports', status: 422 });

    await requestDialog(page).getByRole('button', { name: 'Request export' }).click();

    await expect(requestDialog(page).getByText('This value is incorrect.')).toBeVisible();
    await expect(passwordField).toHaveAttribute('aria-invalid', 'true');
    await expect(passwordField).toBeFocused();
  });

  test('a request closes the dialog and the Export requested notice takes focus', async ({
    page,
    deployment,
    control,
  }) => {
    const tutor = await seedVerifiedUser(deployment, 'Maria', 'en');
    await openMyData(page, control, tutor);
    await exportSection(page).getByRole('button', { name: 'Request an export' }).click();
    await requestDialog(page).getByLabel('Current password').fill(tutor.password);

    await requestDialog(page).getByRole('button', { name: 'Request export' }).click();

    await expect(requestDialog(page)).toBeHidden();
    await expect(exportSection(page).getByText('Export requested. We will notify you when it is ready.')).toBeFocused();
    await expect(exportSection(page).getByText(shownAfterRequest)).toBeVisible();
  });

  test('a requested export becomes ready after a worker tick and the notification opens My data', async ({
    page,
    deployment,
    control,
  }) => {
    const tutor = await seedVerifiedUser(deployment, 'Maria', 'en');
    await openMyData(page, control, tutor);
    await requestThroughThePage(page, tutor.password);
    await expect(exportSection(page).getByText(shownAfterRequest)).toBeVisible();
    await control.tick();
    await expect(exportSection(page).getByText('Your export is ready.')).toBeVisible({ timeout: exportReadyTimeout });
    await page.getByRole('navigation', { name: 'Settings sections' }).getByRole('link', { name: 'Profile' }).click();
    await expect(page).toHaveURL('/settings/profile');
    await page
      .getByRole('navigation')
      .getByRole('button', { name: /^Notifications/ })
      .click();
    const readyRow = page
      .getByRole('region', { name: 'Notifications' })
      .getByRole('link', { name: /Your data export is ready to download/ });
    await expect(readyRow).toBeVisible();

    await readyRow.click();

    await expect(page).toHaveURL('/settings/data');
    await expect(exportSection(page).getByText('Your export is ready.')).toBeVisible();
  });

  test('Download yields the ZIP without keeping the address', async ({ page, deployment, control }) => {
    const tutor = await seedVerifiedUser(deployment, 'Maria', 'en');
    await seedReadyExport(deployment, control, tutor);
    const pageResponse = await openMyData(page, control, tutor);
    await expect(exportSection(page).getByText('Your export is ready.')).toBeVisible();

    const evidence = await downloadThroughThePage(page);

    expectDownloadedArchive(evidence);
    expect(pageResponse?.headers()['referrer-policy']).toBe('no-referrer');
  });

  test('Delete export removes a ready export, and a new request inside 24 hours is refused with the time', async ({
    page,
    deployment,
    control,
  }) => {
    const tutor = await seedVerifiedUser(deployment, 'Maria', 'en');
    await seedReadyExport(deployment, control, tutor);
    await openMyData(page, control, tutor);
    await expect(exportSection(page).getByText('Your export is ready.')).toBeVisible();

    const dialog = await openDeletion(page);
    await dialog.getByRole('button', { name: 'Delete export', exact: true }).click();

    await expect(exportSection(page).getByText('The export was deleted.')).toBeFocused();
    await expect(exportSection(page).getByText('You have no export yet.')).toBeVisible();
    const request = exportSection(page).getByRole('button', { name: 'Request an export' });
    await expect(request).toHaveAttribute('aria-disabled', 'true');
    await expect(request).toHaveAccessibleDescription(/^You can request a new export after .+\.$/);
    await expect(exportSection(page).getByText(/^You can request a new export after .+\.$/)).toBeVisible();
  });

  test('Delete export is the dangerous item of Export actions and asks first, opening on Cancel', async ({
    page,
    deployment,
    control,
  }) => {
    const tutor = await seedVerifiedUser(deployment, 'Maria', 'en');
    await seedReadyExport(deployment, control, tutor);
    await openMyData(page, control, tutor);
    await expect(exportSection(page).getByText('Your export is ready.')).toBeVisible();
    await expect(exportSection(page).getByRole('button', { name: 'Delete export', exact: true })).toHaveCount(0);
    await exportSection(page).getByRole('button', { name: 'Export actions', exact: true }).click();
    await expectLastDangerItem(page, 'Delete export', false);

    await openedMenu(page).getByRole('button', { name: 'Delete export', exact: true }).click();

    const dialog = deletionDialog(page);
    await expectDangerConfirmation(dialog, 'Delete export', 'Cancel');
    await expect(dialog).toContainText('a download link created in the last 5 minutes keeps working until it expires');
    expect(await exportState(deployment, await accessToken(deployment, tutor))).toBe('ready');
  });

  test('Cancel and Escape close the deletion dialog onto Export actions and keep the export', async ({
    page,
    deployment,
    control,
  }) => {
    const tutor = await seedVerifiedUser(deployment, 'Maria', 'en');
    await seedReadyExport(deployment, control, tutor);
    await openMyData(page, control, tutor);
    const menu = exportSection(page).getByRole('button', { name: 'Export actions', exact: true });

    const cancelled = await openDeletion(page);
    await cancelled.getByRole('button', { name: 'Cancel', exact: true }).click();
    await expect(cancelled).toBeHidden();
    await expect(menu).toBeFocused();
    const escaped = await openDeletion(page);
    await page.keyboard.press('Escape');

    await expect(escaped).toBeHidden();
    await expect(menu).toBeFocused();
    expect(await exportState(deployment, await accessToken(deployment, tutor))).toBe('ready');
  });

  test('an export replaced elsewhere while the deletion dialog is open is not deleted', async ({
    page,
    deployment,
    control,
  }) => {
    const tutor = await seedVerifiedUser(deployment, 'Maria', 'en');
    await seedReadyExport(deployment, control, tutor);
    await control.advance(24 * 60 * 60 + 60);
    await openMyData(page, control, tutor);
    const dialog = await openDeletion(page);
    const token = await accessToken(deployment, tutor);
    expect((await callApi(deployment, token, 'POST', '/exports', { password: tutor.password })).status).toBe(202);

    await dialog.getByRole('button', { name: 'Delete export', exact: true }).click();

    await expect(dialog).toBeHidden();
    await expect(
      exportSection(page).getByText('Your export changed in the meantime. The page shows its current state.'),
    ).toBeFocused();
    expect(await exportState(deployment, token)).not.toBeNull();
  });

  test('every session signed out while the deletion dialog is open leaves through sign-in and deletes nothing', async ({
    page,
    deployment,
    control,
    diagnostics,
  }) => {
    const tutor = await seedVerifiedUser(deployment, 'Maria', 'en');
    await seedReadyExport(deployment, control, tutor);
    await openMyData(page, control, tutor);
    let deletions = 0;
    page.on('request', (request) => {
      if (request.method() === 'DELETE' && new URL(request.url()).pathname === '/api/v1/exports/current') {
        deletions += 1;
      }
    });
    const dialog = await openDeletion(page);
    await expect(dialog).toBeVisible();
    diagnostics.expectFailure({ method: 'POST', path: '/api/v1/auth/refresh', status: 401 });

    const signedOut = await callApi(deployment, await accessToken(deployment, tutor), 'POST', '/auth/logout-all');

    expect(signedOut.status).toBe(204);
    await expect(page.getByRole('heading', { level: 1, name: 'Sign in' })).toBeVisible();
    await expect(page.getByRole('dialog')).toHaveCount(0);
    expect(deletions).toBe(0);
    expect(await exportState(deployment, await accessToken(deployment, tutor))).toBe('ready');
  });

  test('Cancel in the request dialog returns focus to Request an export', async ({ page, deployment, control }) => {
    const tutor = await seedVerifiedUser(deployment, 'Maria', 'en');
    await openMyData(page, control, tutor);
    await exportSection(page).getByRole('button', { name: 'Request an export' }).click();
    await requestDialog(page).getByLabel('Current password').fill(tutor.password);

    await requestDialog(page).getByRole('button', { name: 'Cancel' }).click();

    await expect(requestDialog(page)).toBeHidden();
    await expect(exportSection(page).getByRole('button', { name: 'Request an export' })).toBeFocused();
  });

  test('reopening the request dialog after typing a password shows an empty password field', async ({
    page,
    deployment,
    control,
  }) => {
    const tutor = await seedVerifiedUser(deployment, 'Maria', 'en');
    await openMyData(page, control, tutor);
    const request = exportSection(page).getByRole('button', { name: 'Request an export' });
    await request.click();
    await requestDialog(page).getByLabel('Current password').fill(tutor.password);
    await requestDialog(page).getByRole('button', { name: 'Cancel' }).click();
    await expect(requestDialog(page)).toBeHidden();

    await request.click();

    await expect(requestDialog(page).getByLabel('Current password')).toBeFocused();
    await expect(requestDialog(page).getByLabel('Current password')).toHaveValue('');
  });

  test('a student requests an export and downloads it', async ({ page, deployment, control }) => {
    const tutor = await seedVerifiedUser(deployment, 'Maria', 'en');
    const { student } = await seedActiveStudent(deployment, tutor, 'Anna', 'en');
    await openMyData(page, control, student);
    await requestThroughThePage(page, student.password);
    await expect(exportSection(page).getByText(shownAfterRequest)).toBeVisible();
    await control.tick();
    await expect(exportSection(page).getByText('Your export is ready.')).toBeVisible({ timeout: exportReadyTimeout });

    const evidence = await downloadThroughThePage(page);

    expectDownloadedArchive(evidence);
  });
});

async function openMyDataInRussian(page: Page, control: TestControl, account: TestAccount): Promise<void> {
  await page.clock.install({ time: await control.now() });
  await signInThroughTheApi(page, account);
  await page.goto('/settings/data');
  await page.getByRole('heading', { level: 1, name: 'Мои данные' }).waitFor();
}

function russianExportSection(page: Page): Locator {
  return page.getByRole('region', { name: 'Выгрузка ваших данных' });
}

async function insideThePageWidth(page: Page, control: Locator): Promise<boolean> {
  const box = await control.boundingBox();
  const width = await page.evaluate(() => document.documentElement.clientWidth);
  return box !== null && box.x >= 0 && box.x + box.width <= width;
}

async function scrollsSideways(page: Page): Promise<boolean> {
  return page.evaluate(() => document.documentElement.scrollWidth > document.documentElement.clientWidth);
}

test.describe('My data in Russian on a phone', () => {
  test.use({ locale: 'ru-RU', timezoneId: 'Europe/Moscow', viewport: { width: 360, height: 780 } });

  test('a ready export shows its state in Russian with Download and Export actions inside the 360 px width', async ({
    page,
    deployment,
    control,
  }) => {
    const tutor = await seedVerifiedUser(deployment, 'Мария');
    await seedReadyExport(deployment, control, tutor);
    await page.clock.install({ time: await control.now() });
    await signInThroughTheApi(page, tutor);

    await page.goto('/settings/data');

    const section = russianExportSection(page);
    await expect(page.getByRole('heading', { level: 1, name: 'Мои данные' })).toBeVisible();
    await expect(section.getByText('Выгрузка готова.')).toBeVisible();
    await expect(section.getByText(/^Доступна до .+\.$/)).toBeVisible();
    expect(await insideThePageWidth(page, section.getByRole('button', { name: 'Скачать' }))).toBe(true);
    expect(
      await insideThePageWidth(page, section.getByRole('button', { name: 'Действия с выгрузкой', exact: true })),
    ).toBe(true);
    expect(await scrollsSideways(page)).toBe(false);
  });

  test('a request in Russian closes the dialog and the notice takes focus without sideways scrolling', async ({
    page,
    deployment,
    control,
  }) => {
    const tutor = await seedVerifiedUser(deployment, 'Мария');
    await openMyDataInRussian(page, control, tutor);
    await russianExportSection(page).getByRole('button', { name: 'Запросить выгрузку' }).click();
    const dialog = page.getByRole('dialog', { name: 'Запросить выгрузку' });
    await dialog.getByLabel('Текущий пароль').fill(tutor.password);

    await dialog.getByRole('button', { name: 'Запросить выгрузку' }).click();

    await expect(dialog).toBeHidden();
    await expect(
      russianExportSection(page).getByText('Выгрузка запрошена. Мы уведомим вас, когда она будет готова.'),
    ).toBeFocused();
    expect(await scrollsSideways(page)).toBe(false);
  });
});
