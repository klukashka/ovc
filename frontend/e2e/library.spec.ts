import { open } from 'node:fs/promises';

import type { Locator, Page } from '@playwright/test';

import { type TestAccount, newTestAccount, registerTutor, signInThroughTheApi, testNetAddress } from './accounts.ts';
import type { Deployment } from './deployment.ts';
import { chooseFromPageMenu, diagnosedTest as test, expect, seedVerifiedUser } from './fixtures.ts';
import { expectRefetchedAfterTheSocketIsReady, trafficAfterReady } from './realtime.ts';
import { accessToken, callApi, inSeedBatches } from './tutoring.ts';

const unknownFolderId = '00000000-0000-4000-8000-00000000f01d';
const documentBytes = Buffer.from(
  '%PDF-1.7\n1 0 obj\n<< /Type /Catalog >>\nendobj\ntrailer\n<< /Root 1 0 R >>\n%%EOF\n',
);
const markup = '<img src=x onerror="document.title=1"><b>Bold</b>';
const pageMenu = 'More library actions';
const libraryQueryPaths: readonly string[] = ['/api/v1/library', '/api/v1/library/folders'];

interface UploadFile {
  readonly name: string;
  readonly mimeType: string;
  readonly buffer: Buffer;
}

const chapterFile: UploadFile = { name: 'Chapter one.pdf', mimeType: 'application/pdf', buffer: documentBytes };

function identifierOf(body: unknown): string {
  if (typeof body !== 'object' || body === null || !('id' in body) || typeof body.id !== 'string') {
    throw new Error('the answer has no identifier');
  }
  return body.id;
}

async function createFolder(
  deployment: Deployment,
  tutor: TestAccount,
  name: string,
  parentId: string | null = null,
): Promise<string> {
  const answer = await callApi(deployment, await accessToken(deployment, tutor), 'POST', '/library/folders', {
    name,
    parent_id: parentId,
  });
  if (answer.status !== 201) {
    throw new Error(`creating a folder answered ${String(answer.status)}`);
  }
  return identifierOf(answer.body);
}

async function createTemplate(
  deployment: Deployment,
  tutor: TestAccount,
  title: string,
  folderId: string | null = null,
): Promise<string> {
  const answer = await callApi(deployment, await accessToken(deployment, tutor), 'POST', '/library/templates', {
    title,
    folder_id: folderId,
    blocks: [],
  });
  if (answer.status !== 201) {
    throw new Error(`creating a template answered ${String(answer.status)}`);
  }
  return identifierOf(answer.body);
}

async function deleteTemplate(deployment: Deployment, tutor: TestAccount, id: string): Promise<void> {
  const answer = await callApi(deployment, await accessToken(deployment, tutor), 'DELETE', `/library/templates/${id}`);
  if (answer.status !== 204) {
    throw new Error(`deleting a template answered ${String(answer.status)}`);
  }
}

async function createMaterial(
  deployment: Deployment,
  tutor: TestAccount,
  title: string,
  folderId: string | null,
): Promise<string> {
  const token = await accessToken(deployment, tutor);
  const body = new FormData();
  body.append('file', new Blob([new Uint8Array(chapterFile.buffer)], { type: chapterFile.mimeType }), chapterFile.name);
  const uploaded = await fetch(`${deployment.apiUrl}/api/v1/uploads?context=library`, {
    signal: AbortSignal.timeout(10_000),
    method: 'POST',
    headers: { Origin: deployment.origin, 'X-Forwarded-For': testNetAddress(), Authorization: `Bearer ${token}` },
    body,
  });
  if (uploaded.status !== 201) {
    throw new Error(`uploading a material answered ${String(uploaded.status)}`);
  }
  const attachmentId = identifierOf(await uploaded.json());
  const answer = await callApi(deployment, token, 'POST', '/library/materials', {
    attachment_id: attachmentId,
    title,
    folder_id: folderId,
  });
  if (answer.status !== 201) {
    throw new Error(`creating a material answered ${String(answer.status)}`);
  }
  return identifierOf(answer.body);
}

async function openLibrary(page: Page, tutor: TestAccount, address = '/homework/library'): Promise<void> {
  await signInThroughTheApi(page, tutor);
  const traffic = trafficAfterReady(page, libraryQueryPaths);
  await traffic.initialize();
  await page.goto(address);
  await page.getByRole('heading', { level: 1, name: 'Library' }).waitFor();
  await expectRefetchedAfterTheSocketIsReady(traffic);
}

function items(page: Page): Locator {
  return page.getByRole('table', { name: 'Library items', exact: true });
}

function dialog(page: Page): Locator {
  return page.getByRole('dialog');
}

function heading(page: Page): Locator {
  return page.getByRole('heading', { level: 1, name: 'Library' });
}

function menuButton(page: Page): Locator {
  return page.locator('main header').getByRole('button', { name: pageMenu, exact: true });
}

async function chooseRowAction(page: Page, title: string, action: string): Promise<void> {
  await page.getByRole('button', { name: `Actions for ${title}`, exact: true }).click();
  await page.locator('main [aria-expanded="true"] + div').getByRole('button', { name: action, exact: true }).click();
}

async function submitNewFolder(page: Page, name: string): Promise<void> {
  await chooseFromPageMenu(page, pageMenu, 'New folder');
  await dialog(page).getByRole('textbox', { name: 'Folder name' }).fill(name);
  await dialog(page).getByRole('button', { name: 'Create', exact: true }).click();
}

async function uploadThroughTheMenu(page: Page, file: UploadFile): Promise<void> {
  const chooser = page.waitForEvent('filechooser');
  await chooseFromPageMenu(page, pageMenu, 'Upload material');
  await (await chooser).setFiles(file);
}

test.use({ locale: 'en-GB', viewport: { width: 1280, height: 800 }, reducedMotion: 'reduce' });

test.describe('library folders', () => {
  test('New folder opens on its name field', async ({ page, deployment }) => {
    const tutor = await seedVerifiedUser(deployment, 'Maria', 'en');
    await openLibrary(page, tutor);
    await page.getByRole('heading', { name: 'Build your first assignment' }).waitFor();

    await chooseFromPageMenu(page, pageMenu, 'New folder');

    await expect(dialog(page).getByRole('textbox', { name: 'Folder name' })).toBeFocused();
  });

  test('saving a new folder focuses the new row, showing markup as text', async ({ page, deployment }) => {
    const tutor = await seedVerifiedUser(deployment, 'Maria', 'en');
    await openLibrary(page, tutor);
    await page.getByRole('heading', { name: 'Build your first assignment' }).waitFor();

    await submitNewFolder(page, markup);

    await expect(items(page).getByRole('link', { name: markup, exact: true })).toBeFocused();
    await expect(page.locator('main img')).toHaveCount(0);
  });

  test('a taken name shows under the name field and keeps focus there', async ({ page, deployment, diagnostics }) => {
    const tutor = await seedVerifiedUser(deployment, 'Maria', 'en');
    await createFolder(deployment, tutor, 'Grammar');
    await openLibrary(page, tutor);
    diagnostics.expectFailure({ method: 'POST', path: '/api/v1/library/folders', status: 422 });

    await submitNewFolder(page, 'GRAMMAR');

    await expect(dialog(page).getByText('A folder with this name already exists here.')).toBeVisible();
    await expect(dialog(page).getByRole('textbox', { name: 'Folder name' })).toBeFocused();
  });

  test('the depth limit shows under the name field inside a third-level folder with its path', async ({
    page,
    deployment,
    diagnostics,
  }) => {
    const tutor = await seedVerifiedUser(deployment, 'Maria', 'en');
    const first = await createFolder(deployment, tutor, 'Grammar');
    const second = await createFolder(deployment, tutor, 'Verbs', first);
    const third = await createFolder(deployment, tutor, 'Irregular', second);
    await openLibrary(page, tutor, `/homework/library?folder=${third}`);
    diagnostics.expectFailure({ method: 'POST', path: '/api/v1/library/folders', status: 422 });

    await submitNewFolder(page, 'Too deep');

    await expect(dialog(page).getByText('Folders can be nested at most three levels deep.')).toBeVisible();
    await expect(dialog(page).getByRole('textbox', { name: 'Folder name' })).toBeFocused();
    await expect(page.getByRole('navigation', { name: 'Folder path' })).toHaveText('Library/Grammar/Verbs/Irregular');
  });

  test('Rename opens on the current name', async ({ page, deployment }) => {
    const tutor = await seedVerifiedUser(deployment, 'Maria', 'en');
    await createFolder(deployment, tutor, 'Grammer');
    await openLibrary(page, tutor);

    await chooseRowAction(page, 'Grammer', 'Rename');

    await expect(dialog(page).getByRole('textbox', { name: 'Folder name' })).toBeFocused();
    await expect(dialog(page).getByRole('textbox', { name: 'Folder name' })).toHaveValue('Grammer');
  });

  test('saving a renamed folder focuses its row', async ({ page, deployment }) => {
    const tutor = await seedVerifiedUser(deployment, 'Maria', 'en');
    await createFolder(deployment, tutor, 'Grammer');
    await openLibrary(page, tutor);
    await chooseRowAction(page, 'Grammer', 'Rename');

    await dialog(page).getByRole('textbox', { name: 'Folder name' }).fill('Grammar');
    await dialog(page).getByRole('button', { name: 'Save', exact: true }).click();

    await expect(items(page).getByRole('link', { name: 'Grammar', exact: true })).toBeFocused();
  });

  test('Move opens on the destination', async ({ page, deployment }) => {
    const tutor = await seedVerifiedUser(deployment, 'Maria', 'en');
    await createFolder(deployment, tutor, 'Grammar');
    await createFolder(deployment, tutor, 'Vocabulary');
    await openLibrary(page, tutor);

    await chooseRowAction(page, 'Vocabulary', 'Move');

    await expect(dialog(page).getByRole('combobox', { name: 'Destination' })).toBeFocused();
  });

  test('moving announces Moved. with focus', async ({ page, deployment }) => {
    const tutor = await seedVerifiedUser(deployment, 'Maria', 'en');
    await createFolder(deployment, tutor, 'Grammar');
    await createFolder(deployment, tutor, 'Vocabulary');
    await openLibrary(page, tutor);

    await chooseRowAction(page, 'Vocabulary', 'Move');
    await dialog(page).getByRole('combobox', { name: 'Destination' }).selectOption({ label: 'Grammar' });
    await dialog(page).getByRole('button', { name: 'Move', exact: true }).click();

    await expect(page.getByText('Moved.', { exact: true })).toBeFocused();
    await expect(items(page).getByRole('link')).toHaveText(['Grammar']);
  });

  test('deleting a folder asks first and opens on Cancel', async ({ page, deployment }) => {
    const tutor = await seedVerifiedUser(deployment, 'Maria', 'en');
    await createFolder(deployment, tutor, 'Grammar');
    await openLibrary(page, tutor);

    await chooseRowAction(page, 'Grammar', 'Delete');

    await expect(dialog(page).getByRole('button', { name: 'Cancel', exact: true })).toBeFocused();
    await expect(dialog(page)).toContainText('Delete this folder?');
  });

  test('deleting a folder that is not empty shows the refusal in the dialog', async ({
    page,
    deployment,
    diagnostics,
  }) => {
    const tutor = await seedVerifiedUser(deployment, 'Maria', 'en');
    const grammar = await createFolder(deployment, tutor, 'Grammar');
    await createFolder(deployment, tutor, 'Verbs', grammar);
    await openLibrary(page, tutor);
    diagnostics.expectFailure({ method: 'DELETE', path: `/api/v1/library/folders/${grammar}`, status: 409 });

    await chooseRowAction(page, 'Grammar', 'Delete');
    await dialog(page).getByRole('button', { name: 'Delete', exact: true }).click();

    await expect(dialog(page).getByText('Only an empty folder can be deleted.')).toBeFocused();
  });

  test('deleting a folder moves focus to the row that took its place', async ({ page, deployment }) => {
    const tutor = await seedVerifiedUser(deployment, 'Maria', 'en');
    await createFolder(deployment, tutor, 'Alpha');
    await createFolder(deployment, tutor, 'Beta');
    await openLibrary(page, tutor);

    await chooseRowAction(page, 'Alpha', 'Delete');
    await dialog(page).getByRole('button', { name: 'Delete', exact: true }).click();

    await expect(items(page).getByRole('link', { name: 'Beta', exact: true })).toBeFocused();
    await expect(items(page).getByRole('link')).toHaveText(['Beta']);
  });

  test('opening a folder from its row moves focus to the page heading', async ({ page, deployment }) => {
    const tutor = await seedVerifiedUser(deployment, 'Maria', 'en');
    const grammar = await createFolder(deployment, tutor, 'Grammar');
    await createFolder(deployment, tutor, 'Verbs', grammar);
    await openLibrary(page, tutor);
    await items(page).getByRole('link', { name: 'Grammar', exact: true }).focus();

    await page.keyboard.press('Enter');

    await expect(items(page).getByRole('link', { name: 'Verbs', exact: true })).toBeVisible();
    await expect(heading(page)).toBeFocused();
  });

  test('choosing a folder from the breadcrumbs moves focus to the page heading', async ({ page, deployment }) => {
    const tutor = await seedVerifiedUser(deployment, 'Maria', 'en');
    const grammar = await createFolder(deployment, tutor, 'Grammar');
    const verbs = await createFolder(deployment, tutor, 'Verbs', grammar);
    await openLibrary(page, tutor, `/homework/library?folder=${verbs}`);
    await page
      .getByRole('navigation', { name: 'Folder path' })
      .getByRole('link', { name: 'Grammar', exact: true })
      .focus();

    await page.keyboard.press('Enter');

    await expect(items(page).getByRole('link', { name: 'Verbs', exact: true })).toBeVisible();
    await expect(heading(page)).toBeFocused();
  });

  test('choosing a folder in the tree keeps focus on its tree item', async ({ page, deployment }) => {
    const tutor = await seedVerifiedUser(deployment, 'Maria', 'en');
    await createFolder(deployment, tutor, 'Grammar');
    await createFolder(deployment, tutor, 'Reading');
    await openLibrary(page, tutor);
    const treeItem = page
      .getByRole('navigation', { name: 'Folders' })
      .getByRole('link', { name: 'Reading', exact: true });
    await treeItem.focus();

    await page.keyboard.press('Enter');

    await expect(page.getByText('This folder is empty')).toBeVisible();
    await expect(treeItem).toHaveAttribute('aria-current', 'page');
    await expect(treeItem).toBeFocused();
  });

  test('an unknown folder shows Not found', async ({ page, deployment, diagnostics }) => {
    const tutor = await seedVerifiedUser(deployment, 'Maria', 'en');
    await signInThroughTheApi(page, tutor);
    diagnostics.expectFailure({ method: 'GET', path: `/api/v1/library?folder=${unknownFolderId}`, status: 404 });

    await page.goto(`/homework/library?folder=${unknownFolderId}`);

    await expect(page.getByRole('heading', { level: 1, name: 'Page not found' })).toBeFocused();
  });
});

test.describe('library materials', () => {
  test('an upload opens on its title from the file name', async ({ page, deployment }) => {
    const tutor = await seedVerifiedUser(deployment, 'Maria', 'en');
    await openLibrary(page, tutor);

    await uploadThroughTheMenu(page, chapterFile);

    await expect(dialog(page).getByRole('textbox', { name: 'Title' })).toBeFocused();
    await expect(dialog(page).getByRole('textbox', { name: 'Title' })).toHaveValue('Chapter one');
    await expect(dialog(page).getByText('Uploaded: Chapter one.pdf')).toBeVisible();
  });

  test('saving an upload focuses the material row', async ({ page, deployment }) => {
    const tutor = await seedVerifiedUser(deployment, 'Maria', 'en');
    await openLibrary(page, tutor);
    await uploadThroughTheMenu(page, chapterFile);
    await dialog(page).getByText('Uploaded: Chapter one.pdf').waitFor();

    await dialog(page).getByRole('button', { name: 'Save', exact: true }).click();

    await expect(items(page).getByRole('button', { name: 'Chapter one', exact: true })).toBeFocused();
    await expect(items(page).locator('tbody tr')).toContainText(['Chapter one.pdf']);
  });

  test('opening a material downloads its file and renaming focuses its row', async ({ page, deployment }) => {
    const tutor = await seedVerifiedUser(deployment, 'Maria', 'en');
    await createMaterial(deployment, tutor, 'Chapter one', null);
    await openLibrary(page, tutor);

    const downloading = page.waitForEvent('download');
    await items(page).getByRole('button', { name: 'Chapter one', exact: true }).click();
    const download = await downloading;
    await chooseRowAction(page, 'Chapter one', 'Rename');
    await dialog(page).getByRole('textbox', { name: 'Title' }).fill('Chapter 1');
    await dialog(page).getByRole('button', { name: 'Save', exact: true }).click();

    expect(download.suggestedFilename()).toBe('Chapter one.pdf');
    await expect(items(page).getByRole('button', { name: 'Chapter 1', exact: true })).toBeFocused();
  });

  test('search finds materials across folders with their path, skips a blank query and clears back', async ({
    page,
    deployment,
  }) => {
    const tutor = await seedVerifiedUser(deployment, 'Maria', 'en');
    const reading = await createFolder(deployment, tutor, 'Reading');
    await createMaterial(deployment, tutor, 'Chapter one', reading);
    await openLibrary(page, tutor);
    const queries: string[] = [];
    page.on('request', (request) => {
      const address = new URL(request.url());
      if (address.pathname === '/api/v1/library' && address.searchParams.has('q')) {
        queries.push(address.searchParams.get('q') ?? '');
      }
    });
    const search = page.getByRole('searchbox', { name: 'Search the library' });
    const results = page.getByRole('table', { name: 'Search results', exact: true });

    await search.fill('   ');
    await search.fill('CHAPTER');
    await results.waitFor();
    const found = await results.locator('tbody tr').allTextContents();
    await search.fill('nothing like this');
    await page.getByText('Nothing found').waitFor();
    await search.fill('');

    expect(found).toHaveLength(1);
    expect(found[0]).toContain('Library / Reading');
    expect(queries).toContain('CHAPTER');
    expect(queries).not.toContain('   ');
    await expect(items(page).getByRole('link')).toHaveText(['Reading']);
  });

  test('moving a material announces Moved. with focus and the material appears in its folder', async ({
    page,
    deployment,
  }) => {
    const tutor = await seedVerifiedUser(deployment, 'Maria', 'en');
    const reading = await createFolder(deployment, tutor, 'Reading');
    await createMaterial(deployment, tutor, 'Chapter one', null);
    await openLibrary(page, tutor);

    await chooseRowAction(page, 'Chapter one', 'Move');
    await dialog(page).getByRole('combobox', { name: 'Destination' }).selectOption({ label: 'Reading' });
    await dialog(page).getByRole('button', { name: 'Move', exact: true }).click();

    await expect(page.getByText('Moved.', { exact: true })).toBeFocused();
    await expect(items(page).locator('tbody tr')).toHaveCount(1);
    await page.goto(`/homework/library?folder=${reading}`);
    await expect(items(page).getByRole('button', { name: 'Chapter one', exact: true })).toBeVisible();
  });

  test('a folder created while searching gets focus on its tree item', async ({ page, deployment }) => {
    const tutor = await seedVerifiedUser(deployment, 'Maria', 'en');
    await createMaterial(deployment, tutor, 'Chapter one', null);
    await openLibrary(page, tutor);
    await page.getByRole('searchbox', { name: 'Search the library' }).fill('chapter');
    await page.getByRole('table', { name: 'Search results', exact: true }).waitFor();

    await submitNewFolder(page, 'Archive');

    await expect(
      page.getByRole('navigation', { name: 'Folders' }).getByRole('link', { name: 'Archive', exact: true }),
    ).toBeFocused();
  });

  test('deleting the last material of a folder focuses the page heading and shows the empty folder', async ({
    page,
    deployment,
  }) => {
    const tutor = await seedVerifiedUser(deployment, 'Maria', 'en');
    const reading = await createFolder(deployment, tutor, 'Reading');
    await createMaterial(deployment, tutor, 'Chapter one', reading);
    await openLibrary(page, tutor, `/homework/library?folder=${reading}`);

    await chooseRowAction(page, 'Chapter one', 'Delete');
    const question = await dialog(page).textContent();
    await dialog(page).getByRole('button', { name: 'Delete', exact: true }).click();

    expect(question).toContain('Delete this material? Lessons and assignments that use it keep the file.');
    await expect(page.getByText('This folder is empty')).toBeVisible();
    await expect(heading(page)).toBeFocused();
  });
});

test.describe('library templates', () => {
  test('a template row shows its readiness and Duplicate focuses the copy', async ({ page, deployment }) => {
    const tutor = await seedVerifiedUser(deployment, 'Maria', 'en');
    await createTemplate(deployment, tutor, 'Essay');
    await openLibrary(page, tutor);
    const row = await items(page).locator('tbody tr').first().textContent();

    await chooseRowAction(page, 'Essay', 'Duplicate');

    expect(row).toContain('Incomplete');
    expect(row).toContain('0 questions');
    await expect(items(page).getByRole('link', { name: 'Essay (copy)', exact: true })).toBeFocused();
  });

  test('a template deleted elsewhere shows the gone notice with focus and leaves the list', async ({
    page,
    deployment,
    diagnostics,
  }) => {
    const tutor = await seedVerifiedUser(deployment, 'Maria', 'en');
    await createTemplate(deployment, tutor, 'Essay');
    const gone = await createTemplate(deployment, tutor, 'Quiz');
    await openLibrary(page, tutor);
    await items(page).getByRole('link', { name: 'Quiz', exact: true }).waitFor();
    await deleteTemplate(deployment, tutor, gone);
    diagnostics.expectFailure({ method: 'POST', path: `/api/v1/library/templates/${gone}/duplicate`, status: 404 });

    await chooseRowAction(page, 'Quiz', 'Duplicate');

    await expect(page.getByText('This item is no longer in your library.')).toBeFocused();
    await expect(items(page).getByRole('link')).toHaveText(['Essay']);
  });

  test('moving a template announces Moved. with focus', async ({ page, deployment }) => {
    const tutor = await seedVerifiedUser(deployment, 'Maria', 'en');
    await createFolder(deployment, tutor, 'Unit 1');
    await createTemplate(deployment, tutor, 'Essay');
    await openLibrary(page, tutor);

    await chooseRowAction(page, 'Essay', 'Move');
    await dialog(page).getByRole('combobox', { name: 'Destination' }).selectOption({ label: 'Unit 1' });
    await dialog(page).getByRole('button', { name: 'Move', exact: true }).click();

    await expect(page.getByText('Moved.', { exact: true })).toBeFocused();
    await expect(items(page).getByRole('link')).toHaveText(['Unit 1']);
  });

  test('Show more focuses the first new row', async ({ page, deployment }) => {
    const tutor = await seedVerifiedUser(deployment, 'Maria', 'en');
    const unit = await createFolder(deployment, tutor, 'Unit 1');
    await inSeedBatches(31, (index) =>
      createTemplate(deployment, tutor, `Quiz ${String(index + 1).padStart(2, '0')}`, unit),
    );
    await openLibrary(page, tutor, `/homework/library?folder=${unit}`);
    await items(page).locator('tbody tr').nth(29).waitFor();

    await page.getByRole('button', { name: 'Show more', exact: true }).click();

    await expect(items(page).locator('tbody tr')).toHaveCount(31);
    await expect(items(page).getByRole('link', { name: 'Quiz 31', exact: true })).toBeFocused();
  });

  test('deleting the last row focuses the row before it', async ({ page, deployment }) => {
    const tutor = await seedVerifiedUser(deployment, 'Maria', 'en');
    await createTemplate(deployment, tutor, 'Essay');
    await createTemplate(deployment, tutor, 'Quiz');
    await openLibrary(page, tutor);

    await chooseRowAction(page, 'Quiz', 'Delete');
    const question = await dialog(page).textContent();
    await dialog(page).getByRole('button', { name: 'Delete', exact: true }).click();

    expect(question).toContain('Delete this assignment? Files used only in it are deleted too.');
    await expect(items(page).getByRole('link', { name: 'Essay', exact: true })).toBeFocused();
    await expect(items(page).getByRole('link')).toHaveText(['Essay']);
  });

  test('New assignment opens the builder with its heading focused', async ({ page, deployment }) => {
    const tutor = await seedVerifiedUser(deployment, 'Maria', 'en');
    await createTemplate(deployment, tutor, 'Essay');
    await openLibrary(page, tutor);

    await page.locator('main header').getByRole('button', { name: 'New assignment', exact: true }).click();

    await expect(page).toHaveURL(/\/homework\/templates\/[0-9a-f-]{36}$/);
    await expect(page.getByRole('heading', { level: 1, name: 'Assignment builder' })).toBeFocused();
  });
});

test.describe('library uploads', () => {
  test('an unsupported file is refused inside the upload dialog with focus', async ({
    page,
    deployment,
    diagnostics,
  }) => {
    const tutor = await seedVerifiedUser(deployment, 'Maria', 'en');
    await openLibrary(page, tutor);
    diagnostics.expectFailure({ method: 'POST', path: '/api/v1/uploads?context=library', status: 415 });

    await uploadThroughTheMenu(page, { name: 'fake.png', mimeType: 'image/png', buffer: Buffer.from('not a picture') });

    await expect(dialog(page).getByRole('alert')).toHaveText('This file type is not supported.');
    await expect(dialog(page).getByRole('alert')).toBeFocused();
  });

  test('a full storage is refused inside the upload dialog with focus, and Cancel returns focus to the menu', async ({
    page,
    deployment,
    diagnostics,
  }) => {
    const tutor = await seedVerifiedUser(deployment, 'Maria', 'en');
    deployment.administer(['tutors', 'set-quota', tutor.email, '--files', '1', '--recordings', '1']);
    await openLibrary(page, tutor);
    diagnostics.expectFailure({ method: 'POST', path: '/api/v1/uploads?context=library', status: 507 });

    await uploadThroughTheMenu(page, chapterFile);
    await dialog(page).getByRole('alert').waitFor();
    const refusal = await dialog(page).getByRole('alert').textContent();
    await dialog(page).getByRole('button', { name: 'Cancel', exact: true }).click();

    expect(refusal).toBe('The file storage is full.');
    await expect(dialog(page)).toHaveCount(0);
    await expect(menuButton(page)).toBeFocused();
  });

  test('a file over 100 MB is refused in the dialog with focus before it is sent', async ({
    page,
    deployment,
  }, testInfo) => {
    const tutor = await seedVerifiedUser(deployment, 'Maria', 'en');
    const path = testInfo.outputPath('lecture recording.mp4');
    const handle = await open(path, 'w');
    await handle.truncate(100_000_001);
    await handle.close();
    await openLibrary(page, tutor);
    const uploads: string[] = [];
    page.on('request', (request) => {
      if (new URL(request.url()).pathname === '/api/v1/uploads') {
        uploads.push(request.url());
      }
    });
    const chooser = page.waitForEvent('filechooser');
    await chooseFromPageMenu(page, pageMenu, 'Upload material');

    await (await chooser).setFiles(path);

    await expect(dialog(page).getByRole('alert')).toHaveText('File is larger than 100 MB');
    await expect(dialog(page).getByRole('alert')).toBeFocused();
    expect(uploads).toEqual([]);
  });

  test('a WebP picture over 8 MB is refused in the dialog with focus before it is sent', async ({
    page,
    deployment,
  }) => {
    const tutor = await seedVerifiedUser(deployment, 'Maria', 'en');
    await openLibrary(page, tutor);
    const uploads: string[] = [];
    page.on('request', (request) => {
      if (new URL(request.url()).pathname === '/api/v1/uploads') {
        uploads.push(request.url());
      }
    });

    await uploadThroughTheMenu(page, { name: 'photo.webp', mimeType: 'image/webp', buffer: Buffer.alloc(8_388_609) });

    await expect(dialog(page).getByRole('alert')).toHaveText('WebP pictures can be at most 8 MB');
    await expect(dialog(page).getByRole('alert')).toBeFocused();
    expect(uploads).toEqual([]);
  });

  test('an unconfirmed tutor creates folders and is asked to confirm the email before uploading', async ({
    page,
    deployment,
  }) => {
    const tutor = newTestAccount('Unconfirmed tutor');
    await registerTutor(deployment, tutor, 'en');
    await openLibrary(page, tutor);
    await submitNewFolder(page, 'Drafts');
    await items(page).getByRole('link', { name: 'Drafts', exact: true }).waitFor();

    await chooseFromPageMenu(page, pageMenu, 'Upload material');

    await expect(dialog(page).getByText('Confirm your email to upload files.')).toBeVisible();
    await expect(dialog(page).getByRole('link', { name: 'Confirm email' })).toBeVisible();
  });
});

test.describe('library checklist step', () => {
  test('a tutor without assignments sees the undone step linking to the library', async ({ page, deployment }) => {
    const tutor = await seedVerifiedUser(deployment, 'Maria', 'en');
    await signInThroughTheApi(page, tutor);

    await page.goto('/today');

    const step = page.getByRole('region', { name: 'Getting started' }).locator('[data-step="assignment"]');
    await expect(step).toHaveText(/Create an assignment.*not done/);
    await expect(step.getByRole('link', { name: 'Open the library' })).toHaveAttribute('href', '/homework/library');
  });

  test('a tutor with an assignment sees the step done', async ({ page, deployment }) => {
    const tutor = await seedVerifiedUser(deployment, 'Maria', 'en');
    await createTemplate(deployment, tutor, 'First quiz');
    await signInThroughTheApi(page, tutor);

    await page.goto('/today');

    const step = page.getByRole('region', { name: 'Getting started' }).locator('[data-step="assignment"]');
    await expect(step).toHaveAttribute('data-done', 'true');
  });
});
