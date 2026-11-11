import { randomUUID } from 'node:crypto';

import type { Locator, Page, Response } from '@playwright/test';

import { type TestAccount, newTestAccount, registerTutor, signInThroughTheApi, testNetAddress } from './accounts.ts';
import type { Deployment } from './deployment.ts';
import {
  chooseFromPageMenu,
  diagnosedTest as test,
  expect,
  seedVerifiedUser,
  signUpAndConfirmTutor,
} from './fixtures.ts';
import { accessToken, callApi } from './tutoring.ts';

const documentBytes = Buffer.from(
  '%PDF-1.7\n1 0 obj\n<< /Type /Catalog >>\nendobj\ntrailer\n<< /Root 1 0 R >>\n%%EOF\n',
);
const pictureBytes = Buffer.from(
  'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNkYAAAAAYAAjCB0C8AAAAASUVORK5CYII=',
  'base64',
);
const sentinels = {
  answer: 'zebra-sentinel-answer',
  explanation: 'quokka-sentinel-explanation',
  value: '31.415926',
  tolerance: '0.027182',
  points: 97,
};

type BlockSeed = Readonly<Record<string, unknown>>;

interface SavedTemplate {
  readonly id: string;
  readonly version: number;
}

interface SavedBlock {
  readonly id: string;
  readonly type: string;
  readonly optionIds: readonly string[];
  readonly optionTexts: readonly string[];
}

function objectField(value: unknown, name: string): unknown {
  if (typeof value !== 'object' || value === null || !(name in value)) {
    throw new Error(`the answer has no ${name}`);
  }
  return Object.entries(value).find(([key]) => key === name)?.[1];
}

function stringField(value: unknown, name: string): string {
  const found = objectField(value, name);
  if (typeof found !== 'string') {
    throw new Error(`${name} is not a string`);
  }
  return found;
}

function numberField(value: unknown, name: string): number {
  const found = objectField(value, name);
  if (typeof found !== 'number') {
    throw new Error(`${name} is not a number`);
  }
  return found;
}

function listField(value: unknown, name: string): readonly unknown[] {
  const found = objectField(value, name);
  if (!Array.isArray(found)) {
    throw new Error(`${name} is not a list`);
  }
  return found;
}

async function api(
  deployment: Deployment,
  tutor: TestAccount,
  method: 'GET' | 'POST' | 'PUT' | 'PATCH',
  path: string,
  expected: number,
  body?: unknown,
): Promise<unknown> {
  const answer = await callApi(deployment, await accessToken(deployment, tutor), method, path, body);
  if (answer.status !== expected) {
    throw new Error(`${method} ${path} answered ${String(answer.status)}`);
  }
  return answer.body;
}

async function upload(
  deployment: Deployment,
  tutor: TestAccount,
  context: 'library' | 'prompt_image',
  file: { readonly name: string; readonly type: string; readonly bytes: Buffer },
): Promise<string> {
  const body = new FormData();
  body.append('file', new Blob([new Uint8Array(file.bytes)], { type: file.type }), file.name);
  const response = await fetch(`${deployment.apiUrl}/api/v1/uploads?context=${context}`, {
    signal: AbortSignal.timeout(10_000),
    method: 'POST',
    headers: {
      Origin: deployment.origin,
      'X-Forwarded-For': testNetAddress(),
      Authorization: `Bearer ${await accessToken(deployment, tutor)}`,
    },
    body,
  });
  if (response.status !== 201) {
    throw new Error(`uploading answered ${String(response.status)}`);
  }
  return stringField(await response.json(), 'id');
}

async function createFolder(
  deployment: Deployment,
  tutor: TestAccount,
  name: string,
  parentId: string | null = null,
): Promise<string> {
  return stringField(
    await api(deployment, tutor, 'POST', '/library/folders', 201, { name, parent_id: parentId }),
    'id',
  );
}

async function createMaterial(
  deployment: Deployment,
  tutor: TestAccount,
  title: string,
  folderId: string | null,
): Promise<string> {
  const attachmentId = await upload(deployment, tutor, 'library', {
    name: `${title}.pdf`,
    type: 'application/pdf',
    bytes: documentBytes,
  });
  const created = await api(deployment, tutor, 'POST', '/library/materials', 201, {
    attachment_id: attachmentId,
    title,
    folder_id: folderId,
  });
  return stringField(created, 'id');
}

async function createTemplate(
  deployment: Deployment,
  tutor: TestAccount,
  title: string,
  blocks: readonly BlockSeed[] = [],
): Promise<SavedTemplate> {
  const token = await accessToken(deployment, tutor);
  const response = await fetch(`${deployment.apiUrl}/api/v1/library/templates`, {
    signal: AbortSignal.timeout(10_000),
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      Origin: deployment.origin,
      'X-Forwarded-For': testNetAddress(),
      Authorization: `Bearer ${token}`,
      'Idempotency-Key': randomUUID(),
    },
    body: JSON.stringify({ title, folder_id: null, blocks }),
  });
  if (response.status !== 201) {
    throw new Error(`creating a template answered ${String(response.status)}`);
  }
  const created: unknown = await response.json();
  return { id: stringField(created, 'id'), version: numberField(created, 'version') };
}

async function savedBlocks(deployment: Deployment, tutor: TestAccount, id: string): Promise<SavedBlock[]> {
  const template = await api(deployment, tutor, 'GET', `/library/templates/${id}`, 200);
  return listField(template, 'blocks').map((block) => {
    const options =
      typeof block === 'object' && block !== null && 'options' in block ? listField(block, 'options') : [];
    return {
      id: stringField(block, 'id'),
      type: stringField(block, 'type'),
      optionIds: options.map((option) => stringField(option, 'id')),
      optionTexts: options.map((option) => stringField(option, 'text')),
    };
  });
}

function instruction(text: string): BlockSeed {
  return { id: randomUUID(), type: 'instruction', text };
}

function singleChoice(prompt: string, options: readonly string[]): BlockSeed {
  return {
    id: randomUUID(),
    type: 'single_choice',
    prompt,
    points: 1,
    options: options.map((text, index) => ({ id: randomUUID(), text, correct: index === 0 })),
  };
}

function shortText(prompt: string, answers: readonly string[]): BlockSeed {
  return { id: randomUUID(), type: 'short_text', prompt, points: 1, accepted_answers: answers };
}

async function openBuilder(page: Page, tutor: TestAccount, id: string): Promise<void> {
  await signInThroughTheApi(page, tutor);
  await page.goto(`/homework/templates/${id}`);
  await titleField(page).waitFor();
}

function titleField(page: Page): Locator {
  return page.getByRole('textbox', { name: 'Title', exact: true });
}

function block(page: Page, name: string): Locator {
  return page.getByRole('region', { name, exact: true });
}

function addBlockButton(page: Page, label = 'Add block'): Locator {
  return page.getByRole('button', { name: label, exact: true });
}

async function addBlock(page: Page, type: string, label = 'Add block'): Promise<void> {
  await addBlockButton(page, label).click();
  await page.locator('main [aria-expanded="true"] + div').getByRole('button', { name: type, exact: true }).click();
}

function dialog(page: Page): Locator {
  return page.getByRole('dialog');
}

function isTemplateSave(response: Response, id: string): boolean {
  return (
    response.request().method() === 'PUT' && new URL(response.url()).pathname === `/api/v1/library/templates/${id}`
  );
}

async function holdSaves(page: Page, templateId: string): Promise<() => Promise<void>> {
  let release: () => void = () => undefined;
  const resumed = new Promise<void>((resolve) => {
    release = resolve;
  });
  await page.route('**/api/v1/library/templates/*', async (route) => {
    if (route.request().method() === 'PUT') {
      await resumed;
    }
    await route.continue();
  });
  return async () => {
    const answered = page.waitForResponse((response) => isTemplateSave(response, templateId));
    release();
    await answered;
    await page.unroute('**/api/v1/library/templates/*');
  };
}

async function moveTemplate(
  deployment: Deployment,
  tutor: TestAccount,
  template: SavedTemplate,
  folderId: string,
): Promise<void> {
  await api(deployment, tutor, 'PATCH', `/library/templates/${template.id}`, 200, {
    folder_id: folderId,
    version: template.version,
  });
}

async function pictureCount(deployment: Deployment, tutor: TestAccount, id: string): Promise<number> {
  const template = await api(deployment, tutor, 'GET', `/library/templates/${id}`, 200);
  return listField(template, 'blocks')
    .map((entry) =>
      typeof entry === 'object' && entry !== null && 'images' in entry ? listField(entry, 'images') : [],
    )
    .reduce((sum, images) => sum + images.length, 0);
}

async function sentinelTemplate(deployment: Deployment, tutor: TestAccount): Promise<SavedTemplate> {
  const materialId = await createMaterial(deployment, tutor, 'Verb table', null);
  const pictureId = await upload(deployment, tutor, 'prompt_image', {
    name: 'dot.png',
    type: 'image/png',
    bytes: pictureBytes,
  });
  const question = { points: sentinels.points, explanation: sentinels.explanation };
  return createTemplate(deployment, tutor, 'Sentinels', [
    instruction('Read **carefully**.'),
    { id: randomUUID(), type: 'material', material_id: materialId },
    { ...singleChoice('Pick one', ['alpha', 'beta']), ...question, image_ids: [pictureId] },
    {
      id: randomUUID(),
      type: 'multiple_choice',
      prompt: 'Pick some',
      scoring: 'partial',
      options: [
        { id: randomUUID(), text: 'gamma', correct: true },
        { id: randomUUID(), text: 'delta', correct: false },
      ],
      ...question,
    },
    { ...shortText('Say it', [sentinels.answer]), ...question },
    {
      id: randomUUID(),
      type: 'short_number',
      prompt: 'Count it',
      value: sentinels.value,
      tolerance: sentinels.tolerance,
      ...question,
    },
    { id: randomUUID(), type: 'long_text', prompt: 'Write it', max_length: 400, ...question },
    { id: randomUUID(), type: 'file', prompt: 'Send it', allowed_kinds: ['pdf'], max_files: 1, ...question },
  ]);
}

async function dragOnto(page: Page, handle: Locator, target: Locator): Promise<void> {
  const dataTransfer = await page.evaluateHandle(() => new DataTransfer());
  await handle.dispatchEvent('dragstart', { dataTransfer });
  await target.getByRole('heading').dispatchEvent('dragenter', { dataTransfer });
  await target.getByRole('heading').dispatchEvent('dragover', { dataTransfer });
  await target.getByRole('heading').dispatchEvent('drop', { dataTransfer });
  await handle.dispatchEvent('dragend', { dataTransfer });
}

async function textareaValues(page: Page, label: string): Promise<string[]> {
  return page
    .getByRole('textbox', { name: label, exact: true })
    .evaluateAll((elements) =>
      elements.map((element) => (element instanceof HTMLTextAreaElement ? element.value : '')),
    );
}

test.use({ locale: 'en-GB', viewport: { width: 1280, height: 900 }, reducedMotion: 'reduce' });

test.describe('builder blocks', () => {
  test('every block type is built through the page, saved, and shown as ready to issue', async ({
    page,
    deployment,
  }) => {
    const tutor = await seedVerifiedUser(deployment, 'Maria', 'en');
    const folderId = await createFolder(deployment, tutor, 'Grammar');
    await createMaterial(deployment, tutor, 'Irregular verbs', folderId);
    const template = await createTemplate(deployment, tutor, 'New assignment');
    await openBuilder(page, tutor, template.id);

    await titleField(page).fill('Past simple');
    await addBlock(page, 'Instruction');
    await block(page, '1. Instruction').getByRole('textbox', { name: 'Text', exact: true }).fill('Read the **rules**.');
    await addBlock(page, 'Single choice');
    const single = block(page, '2. Single choice');
    await single.getByRole('textbox', { name: 'Question', exact: true }).fill('Past of go?');
    await single.getByRole('textbox', { name: 'Option 1', exact: true }).fill('goed');
    await single.getByRole('textbox', { name: 'Option 2', exact: true }).fill('went');
    await single.getByRole('radio', { name: 'Correct: option 2', exact: true }).check();
    await addBlock(page, 'Multiple choice');
    const multiple = block(page, '3. Multiple choice');
    await multiple.getByRole('textbox', { name: 'Question', exact: true }).fill('Irregular verbs?');
    await multiple.getByRole('textbox', { name: 'Option 1', exact: true }).fill('go');
    await multiple.getByRole('textbox', { name: 'Option 2', exact: true }).fill('walk');
    await multiple.getByRole('checkbox', { name: 'Correct: option 1', exact: true }).check();
    await multiple.getByRole('radio', { name: 'Partial credit', exact: true }).check();
    await addBlock(page, 'Short text');
    const short = block(page, '4. Short text');
    await short.getByRole('textbox', { name: 'Question', exact: true }).fill('Past of see?');
    await short.getByRole('textbox', { name: 'New accepted answer', exact: true }).fill('saw');
    await short.getByRole('textbox', { name: 'New accepted answer', exact: true }).press('Enter');
    await addBlock(page, 'Number');
    const number = block(page, '5. Number');
    await number.getByRole('textbox', { name: 'Question', exact: true }).fill('Half of seven?');
    await number.getByRole('textbox', { name: 'Correct value', exact: true }).fill('3.5');
    await number.getByRole('textbox', { name: 'Tolerance', exact: true }).fill('0.1');
    await addBlock(page, 'Long text');
    const long = block(page, '6. Long text');
    await long.getByRole('textbox', { name: 'Question', exact: true }).fill('Describe your weekend.');
    await long.getByRole('textbox', { name: 'Maximum length in characters', exact: true }).fill('300');
    await addBlock(page, 'File');
    const file = block(page, '7. File');
    await file.getByRole('textbox', { name: 'Question', exact: true }).fill('Record yourself.');
    await file.getByRole('checkbox', { name: 'Audio', exact: true }).check();
    await file.getByRole('textbox', { name: 'Maximum number of files', exact: true }).fill('2');
    await addBlock(page, 'Material');
    await dialog(page).getByRole('button', { name: 'Grammar', exact: true }).click();
    await dialog(page)
      .getByRole('button', { name: /^Irregular verbs/ })
      .click();
    await page.getByText('Ready to issue', { exact: true }).waitFor();
    await page.getByRole('status').filter({ hasText: 'Saved' }).waitFor();

    const blocks = await savedBlocks(deployment, tutor, template.id);
    expect(blocks.map((saved) => saved.type)).toEqual([
      'instruction',
      'single_choice',
      'multiple_choice',
      'short_text',
      'short_number',
      'long_text',
      'file',
      'material',
    ]);
    expect(blocks[1]?.optionTexts).toEqual(['goed', 'went']);
    await expect(block(page, '8. Material').getByText('Irregular verbs.pdf')).toBeVisible();
    await expect(titleField(page)).toHaveValue('Past simple');
  });

  test('a save shows Saving… while it is in flight and Saved once it answers', async ({ page, deployment }) => {
    const tutor = await seedVerifiedUser(deployment, 'Maria', 'en');
    const template = await createTemplate(deployment, tutor, 'Verbs');
    await openBuilder(page, tutor, template.id);
    const release = await holdSaves(page, template.id);

    await titleField(page).fill('Verbs, week 2');
    await page.getByRole('status').filter({ hasText: 'Saving…' }).waitFor();
    await release();

    await expect(page.getByRole('status').filter({ hasText: 'Saved' })).toBeVisible();
    await expect(page.getByRole('status').filter({ hasText: 'Saving…' })).toHaveCount(0);
  });

  test('a stale save shows the changed-elsewhere notice with focus and keeps the typing', async ({
    page,
    deployment,
    diagnostics,
  }) => {
    const tutor = await seedVerifiedUser(deployment, 'Maria', 'en');
    const template = await createTemplate(deployment, tutor, 'Verbs');
    const folderId = await createFolder(deployment, tutor, 'Archive');
    await openBuilder(page, tutor, template.id);
    await moveTemplate(deployment, tutor, template, folderId);
    diagnostics.expectFailure({ method: 'PUT', path: `/api/v1/library/templates/${template.id}`, status: 409 });

    await titleField(page).fill('Verbs typed here');

    await expect(page.getByRole('status').filter({ hasText: 'This changed elsewhere' })).toBeFocused();
    await expect(titleField(page)).toHaveValue('Verbs typed here');
  });

  test('Reload after a stale save shows the stored template and focuses the title', async ({
    page,
    deployment,
    diagnostics,
  }) => {
    const tutor = await seedVerifiedUser(deployment, 'Maria', 'en');
    const template = await createTemplate(deployment, tutor, 'Verbs');
    const folderId = await createFolder(deployment, tutor, 'Archive');
    await openBuilder(page, tutor, template.id);
    await moveTemplate(deployment, tutor, template, folderId);
    diagnostics.expectFailure({ method: 'PUT', path: `/api/v1/library/templates/${template.id}`, status: 409 });
    await titleField(page).fill('Verbs typed here');
    await page.getByRole('status').filter({ hasText: 'This changed elsewhere' }).waitFor();

    await page.getByRole('button', { name: 'Reload', exact: true }).click();

    await expect(titleField(page)).toBeFocused();
    await expect(titleField(page)).toHaveValue('Verbs');
    await expect(
      page.getByRole('navigation', { name: 'Folder path' }).getByRole('link', { name: 'Archive' }),
    ).toBeVisible();
  });

  test('a blank title is not saved and asks for a title', async ({ page, deployment }) => {
    const tutor = await seedVerifiedUser(deployment, 'Maria', 'en');
    const template = await createTemplate(deployment, tutor, 'Verbs');
    await openBuilder(page, tutor, template.id);

    await titleField(page).fill('   ');

    await expect(page.getByText('Enter a title', { exact: true })).toBeVisible();
    await expect(titleField(page)).toHaveAttribute('aria-invalid', 'true');
  });
});

test.describe('builder order and focus', () => {
  test('a new block takes focus on its first field', async ({ page, deployment }) => {
    const tutor = await seedVerifiedUser(deployment, 'Maria', 'en');
    const template = await createTemplate(deployment, tutor, 'Order', [instruction('First')]);
    await openBuilder(page, tutor, template.id);

    await addBlock(page, 'Long text');

    await expect(block(page, '2. Long text').getByRole('textbox', { name: 'Question', exact: true })).toBeFocused();
  });

  test('Move down keeps focus on the pressed button, also when it becomes unavailable at the bottom', async ({
    page,
    deployment,
  }) => {
    const tutor = await seedVerifiedUser(deployment, 'Maria', 'en');
    const template = await createTemplate(deployment, tutor, 'Order', [
      instruction('First'),
      instruction('Second'),
      instruction('Third'),
    ]);
    await openBuilder(page, tutor, template.id);
    await block(page, '1. Instruction').getByRole('button', { name: 'Move down', exact: true }).focus();

    await page.keyboard.press('Enter');
    await page.keyboard.press('Enter');

    const atBottom = block(page, '3. Instruction').getByRole('button', { name: 'Move down', exact: true });
    await expect(atBottom).toBeFocused();
    await expect(atBottom).toHaveAttribute('aria-disabled', 'true');
    expect(await textareaValues(page, 'Text')).toEqual(['Second', 'Third', 'First']);
  });

  test('Move up keeps focus on the pressed button, also when it becomes unavailable at the top', async ({
    page,
    deployment,
  }) => {
    const tutor = await seedVerifiedUser(deployment, 'Maria', 'en');
    const template = await createTemplate(deployment, tutor, 'Order', [instruction('First'), instruction('Second')]);
    await openBuilder(page, tutor, template.id);
    await block(page, '2. Instruction').getByRole('button', { name: 'Move up', exact: true }).focus();

    await page.keyboard.press('Enter');

    const atTop = block(page, '1. Instruction').getByRole('button', { name: 'Move up', exact: true });
    await expect(atTop).toBeFocused();
    await expect(atTop).toHaveAttribute('aria-disabled', 'true');
    expect(await textareaValues(page, 'Text')).toEqual(['Second', 'First']);
  });

  test('dragging a block by its handle onto another block moves it there', async ({ page, deployment }) => {
    const tutor = await seedVerifiedUser(deployment, 'Maria', 'en');
    const template = await createTemplate(deployment, tutor, 'Order', [
      instruction('First'),
      instruction('Second'),
      instruction('Third'),
    ]);
    await openBuilder(page, tutor, template.id);
    const saved = page.waitForResponse((response) => isTemplateSave(response, template.id));

    await dragOnto(page, block(page, '3. Instruction').locator('[data-drag-handle]'), block(page, '1. Instruction'));
    await saved;

    expect(await textareaValues(page, 'Text')).toEqual(['Third', 'First', 'Second']);
  });

  test('a dragged block handle carries no text that a field would take on a drop', async ({ page, deployment }) => {
    const tutor = await seedVerifiedUser(deployment, 'Maria', 'en');
    const template = await createTemplate(deployment, tutor, 'Order', [instruction('First'), instruction('Second')]);
    await openBuilder(page, tutor, template.id);
    const dataTransfer = await page.evaluateHandle(() => new DataTransfer());

    await block(page, '2. Instruction').locator('[data-drag-handle]').dispatchEvent('dragstart', { dataTransfer });

    expect(await dataTransfer.evaluate((transfer) => [...transfer.types])).toEqual(['application/x-ovc-block']);
    expect(await dataTransfer.evaluate((transfer) => transfer.getData('text/plain'))).toBe('');
  });

  test('Duplicate copies a block with fresh identifiers and focuses the copy', async ({ page, deployment }) => {
    const tutor = await seedVerifiedUser(deployment, 'Maria', 'en');
    const template = await createTemplate(deployment, tutor, 'Copies', [singleChoice('Past of go?', ['went', 'goed'])]);
    await openBuilder(page, tutor, template.id);
    const saved = page.waitForResponse((response) => isTemplateSave(response, template.id));

    await block(page, '1. Single choice').getByRole('button', { name: 'Duplicate', exact: true }).click();
    await saved;

    await expect(block(page, '2. Single choice').getByRole('textbox', { name: 'Question', exact: true })).toBeFocused();
    const [original, copy] = await savedBlocks(deployment, tutor, template.id);
    expect(copy?.id).not.toBe(original?.id);
    expect(copy?.optionTexts).toEqual(original?.optionTexts);
    expect(copy?.optionIds.filter((id) => original?.optionIds.includes(id) === true)).toEqual([]);
  });

  test('Delete focuses the heading of the block that took its place', async ({ page, deployment }) => {
    const tutor = await seedVerifiedUser(deployment, 'Maria', 'en');
    const template = await createTemplate(deployment, tutor, 'Removals', [
      instruction('First'),
      instruction('Second'),
      instruction('Third'),
    ]);
    await openBuilder(page, tutor, template.id);

    await block(page, '2. Instruction').getByRole('button', { name: 'Delete', exact: true }).click();

    await expect(block(page, '2. Instruction').getByRole('heading')).toBeFocused();
    expect(await textareaValues(page, 'Text')).toEqual(['First', 'Third']);
  });

  test('deleting the last block focuses the heading of the block before it', async ({ page, deployment }) => {
    const tutor = await seedVerifiedUser(deployment, 'Maria', 'en');
    const template = await createTemplate(deployment, tutor, 'Removals', [instruction('First'), instruction('Second')]);
    await openBuilder(page, tutor, template.id);

    await block(page, '2. Instruction').getByRole('button', { name: 'Delete', exact: true }).click();

    await expect(block(page, '1. Instruction').getByRole('heading')).toBeFocused();
  });

  test('deleting the only block focuses Add block', async ({ page, deployment }) => {
    const tutor = await seedVerifiedUser(deployment, 'Maria', 'en');
    const template = await createTemplate(deployment, tutor, 'Removals', [instruction('First')]);
    await openBuilder(page, tutor, template.id);

    await block(page, '1. Instruction').getByRole('button', { name: 'Delete', exact: true }).click();

    await expect(addBlockButton(page)).toBeFocused();
    await expect(page.getByText('Add at least one block', { exact: true })).toBeVisible();
  });

  test('Add option focuses the new option', async ({ page, deployment }) => {
    const tutor = await seedVerifiedUser(deployment, 'Maria', 'en');
    const template = await createTemplate(deployment, tutor, 'Options', [
      singleChoice('Past of go?', ['went', 'goed']),
    ]);
    await openBuilder(page, tutor, template.id);
    const question = block(page, '1. Single choice');

    await question.getByRole('button', { name: 'Add option', exact: true }).click();

    await expect(question.getByRole('textbox', { name: 'Option 3', exact: true })).toBeFocused();
  });

  test('a removed option passes focus to the option after it', async ({ page, deployment }) => {
    const tutor = await seedVerifiedUser(deployment, 'Maria', 'en');
    const template = await createTemplate(deployment, tutor, 'Options', [
      singleChoice('Past of go?', ['went', 'goed']),
    ]);
    await openBuilder(page, tutor, template.id);
    const question = block(page, '1. Single choice');

    await question.getByRole('button', { name: 'Remove option 1', exact: true }).click();

    await expect(question.getByRole('textbox', { name: 'Option 1', exact: true })).toBeFocused();
    await expect(question.getByRole('textbox', { name: 'Option 1', exact: true })).toHaveValue('goed');
  });

  test('removing the last option passes focus to the option before it', async ({ page, deployment }) => {
    const tutor = await seedVerifiedUser(deployment, 'Maria', 'en');
    const template = await createTemplate(deployment, tutor, 'Options', [
      singleChoice('Past of go?', ['went', 'goed']),
    ]);
    await openBuilder(page, tutor, template.id);
    const question = block(page, '1. Single choice');

    await question.getByRole('button', { name: 'Remove option 2', exact: true }).click();

    await expect(question.getByRole('textbox', { name: 'Option 1', exact: true })).toBeFocused();
  });

  test('removing the only option passes focus to Add option', async ({ page, deployment }) => {
    const tutor = await seedVerifiedUser(deployment, 'Maria', 'en');
    const template = await createTemplate(deployment, tutor, 'Options', [singleChoice('Past of go?', ['went'])]);
    await openBuilder(page, tutor, template.id);
    const question = block(page, '1. Single choice');

    await question.getByRole('button', { name: 'Remove option 1', exact: true }).click();

    await expect(question.getByRole('button', { name: 'Add option', exact: true })).toBeFocused();
    await expect(question.getByText('Add at least two options', { exact: true })).toBeVisible();
  });

  test('a removed accepted answer passes focus to the answer field', async ({ page, deployment }) => {
    const tutor = await seedVerifiedUser(deployment, 'Maria', 'en');
    const template = await createTemplate(deployment, tutor, 'Answers', [shortText('Past of go?', ['went', 'gone'])]);
    await openBuilder(page, tutor, template.id);
    const question = block(page, '1. Short text');

    await question.getByRole('button', { name: 'Remove the answer went', exact: true }).click();

    await expect(question.getByRole('textbox', { name: 'New accepted answer', exact: true })).toBeFocused();
    await expect(question.getByRole('button', { name: /^Remove the answer/ })).toHaveText(['Remove the answer gone']);
  });
});

test.describe('builder material picker', () => {
  test('adding a material block opens the picker on its search field', async ({ page, deployment }) => {
    const tutor = await seedVerifiedUser(deployment, 'Maria', 'en');
    const template = await createTemplate(deployment, tutor, 'Materials');
    await openBuilder(page, tutor, template.id);

    await addBlock(page, 'Material');

    await expect(dialog(page).getByRole('searchbox', { name: 'Search the library', exact: true })).toBeFocused();
  });

  test('opening a folder in the picker moves focus to the picker heading', async ({ page, deployment }) => {
    const tutor = await seedVerifiedUser(deployment, 'Maria', 'en');
    const folderId = await createFolder(deployment, tutor, 'Grammar');
    await createMaterial(deployment, tutor, 'Irregular verbs', folderId);
    const template = await createTemplate(deployment, tutor, 'Materials');
    await openBuilder(page, tutor, template.id);
    await addBlock(page, 'Material');

    await dialog(page).getByRole('button', { name: 'Grammar', exact: true }).click();

    await expect(dialog(page).getByRole('heading', { name: 'Choose a material', exact: true })).toBeFocused();
    await expect(dialog(page).getByRole('button', { name: /^Irregular verbs/ })).toBeVisible();
  });

  test('choosing a material inserts the block and focuses Change material', async ({ page, deployment }) => {
    const tutor = await seedVerifiedUser(deployment, 'Maria', 'en');
    await createMaterial(deployment, tutor, 'Verb table', null);
    const template = await createTemplate(deployment, tutor, 'Materials', [instruction('Read it.')]);
    await openBuilder(page, tutor, template.id);
    await addBlock(page, 'Material');

    await dialog(page)
      .getByRole('button', { name: /^Verb table/ })
      .click();

    const material = block(page, '2. Material');
    await expect(material.getByRole('button', { name: 'Change material', exact: true })).toBeFocused();
    await expect(material.getByText('Verb table.pdf')).toBeVisible();
  });

  test('opening a path entry in the picker moves focus to the picker heading', async ({ page, deployment }) => {
    const tutor = await seedVerifiedUser(deployment, 'Maria', 'en');
    const grammar = await createFolder(deployment, tutor, 'Grammar');
    const verbs = await createFolder(deployment, tutor, 'Verbs', grammar);
    await createFolder(deployment, tutor, 'Irregular', verbs);
    const template = await createTemplate(deployment, tutor, 'Materials');
    await openBuilder(page, tutor, template.id);
    await addBlock(page, 'Material');
    await dialog(page).getByRole('button', { name: 'Grammar', exact: true }).click();
    await dialog(page).getByRole('button', { name: 'Verbs', exact: true }).click();
    await dialog(page).getByRole('button', { name: 'Irregular', exact: true }).waitFor();

    await dialog(page)
      .getByRole('navigation', { name: 'Folder path' })
      .getByRole('button', { name: 'Grammar', exact: true })
      .click();

    await expect(dialog(page).getByRole('heading', { name: 'Choose a material', exact: true })).toBeFocused();
    await expect(dialog(page).getByRole('button', { name: 'Verbs', exact: true })).toBeVisible();
  });

  test('Show more in the picker focuses the first new material', async ({ page, deployment }) => {
    const tutor = await seedVerifiedUser(deployment, 'Maria', 'en');
    for (let index = 1; index <= 30; index += 1) {
      await createTemplate(deployment, tutor, `A quiz ${String(index).padStart(2, '0')}`);
    }
    await createMaterial(deployment, tutor, 'Zebra notes', null);
    const template = await createTemplate(deployment, tutor, 'B builder');
    await openBuilder(page, tutor, template.id);
    await addBlock(page, 'Material');

    await dialog(page).getByRole('button', { name: 'Show more', exact: true }).click();

    await expect(dialog(page).getByRole('button', { name: /^Zebra notes/ })).toBeFocused();
  });

  test('Change material opens the picker and returns focus to itself after a choice', async ({ page, deployment }) => {
    const tutor = await seedVerifiedUser(deployment, 'Maria', 'en');
    const first = await createMaterial(deployment, tutor, 'Verb table', null);
    await createMaterial(deployment, tutor, 'Noun table', null);
    const template = await createTemplate(deployment, tutor, 'Materials', [
      { id: randomUUID(), type: 'material', material_id: first },
    ]);
    await openBuilder(page, tutor, template.id);
    const material = block(page, '1. Material');
    await material.getByRole('button', { name: 'Change material', exact: true }).click();

    await dialog(page)
      .getByRole('button', { name: /^Noun table/ })
      .click();

    await expect(material.getByRole('button', { name: 'Change material', exact: true })).toBeFocused();
    await expect(material.getByText('Noun table.pdf')).toBeVisible();
  });

  const pickerDismissals: readonly [string, (page: Page) => Promise<void>][] = [
    ['Cancel', (page): Promise<void> => dialog(page).getByRole('button', { name: 'Cancel', exact: true }).click()],
    ['Escape', (page): Promise<void> => page.keyboard.press('Escape')],
  ];

  for (const [dismissal, dismiss] of pickerDismissals) {
    test(`Change material dismissed with ${dismissal} keeps the material and returns focus to itself`, async ({
      page,
      deployment,
    }) => {
      const tutor = await seedVerifiedUser(deployment, 'Maria', 'en');
      const first = await createMaterial(deployment, tutor, 'Verb table', null);
      await createMaterial(deployment, tutor, 'Noun table', null);
      const template = await createTemplate(deployment, tutor, 'Materials', [
        { id: randomUUID(), type: 'material', material_id: first },
      ]);
      await openBuilder(page, tutor, template.id);
      const material = block(page, '1. Material');
      await material.getByRole('button', { name: 'Change material', exact: true }).click();
      await dialog(page)
        .getByRole('button', { name: /^Noun table/ })
        .waitFor();

      await dismiss(page);

      await expect(page.getByRole('dialog')).toHaveCount(0);
      await expect(material.getByRole('button', { name: 'Change material', exact: true })).toBeFocused();
      await expect(material.getByText('Verb table.pdf')).toBeVisible();
    });
  }

  test('cancelling the picker of a new block inserts nothing and returns focus to Add block', async ({
    page,
    deployment,
  }) => {
    const tutor = await seedVerifiedUser(deployment, 'Maria', 'en');
    const template = await createTemplate(deployment, tutor, 'Materials');
    await openBuilder(page, tutor, template.id);
    await addBlock(page, 'Material');

    await dialog(page).getByRole('button', { name: 'Cancel', exact: true }).click();

    await expect(addBlockButton(page)).toBeFocused();
    await expect(page.getByRole('region', { name: /Material$/ })).toHaveCount(0);
  });
});

test.describe('builder pictures', () => {
  test('a prompt picture uploads and shows once the assignment is saved', async ({ page, deployment }) => {
    const tutor = await seedVerifiedUser(deployment, 'Maria', 'en');
    const template = await createTemplate(deployment, tutor, 'Pictures', [singleChoice('Which?', ['a', 'b'])]);
    await openBuilder(page, tutor, template.id);
    const question = block(page, '1. Single choice');

    const chooser = page.waitForEvent('filechooser');
    await question.getByRole('button', { name: 'Add picture', exact: true }).click();
    await (await chooser).setFiles({ name: 'dot.png', mimeType: 'image/png', buffer: pictureBytes });
    const picture = question.getByRole('img', { name: 'dot.png', exact: true });
    await picture.waitFor();

    await expect
      .poll(() => picture.evaluate((image) => image instanceof HTMLImageElement && image.naturalWidth))
      .toBe(1);
    expect(await pictureCount(deployment, tutor, template.id)).toBe(1);
  });

  test('removing a prompt picture focuses Add picture', async ({ page, deployment }) => {
    const tutor = await seedVerifiedUser(deployment, 'Maria', 'en');
    const pictureId = await upload(deployment, tutor, 'prompt_image', {
      name: 'dot.png',
      type: 'image/png',
      bytes: pictureBytes,
    });
    const template = await createTemplate(deployment, tutor, 'Pictures', [
      { ...singleChoice('Which?', ['a', 'b']), image_ids: [pictureId] },
    ]);
    await openBuilder(page, tutor, template.id);
    const question = block(page, '1. Single choice');

    await question.getByRole('button', { name: 'Remove the picture dot.png', exact: true }).click();

    await expect(question.getByRole('button', { name: 'Add picture', exact: true })).toBeFocused();
    await expect(question.getByRole('img')).toHaveCount(0);
  });

  test('removing a prompt picture focuses the Remove of the picture after it', async ({ page, deployment }) => {
    const tutor = await seedVerifiedUser(deployment, 'Maria', 'en');
    const pictures = [
      await upload(deployment, tutor, 'prompt_image', { name: 'one.png', type: 'image/png', bytes: pictureBytes }),
      await upload(deployment, tutor, 'prompt_image', { name: 'two.png', type: 'image/png', bytes: pictureBytes }),
    ];
    const template = await createTemplate(deployment, tutor, 'Pictures', [
      { ...singleChoice('Which?', ['a', 'b']), image_ids: pictures },
    ]);
    await openBuilder(page, tutor, template.id);
    const question = block(page, '1. Single choice');

    await question.getByRole('button', { name: 'Remove the picture one.png', exact: true }).click();

    await expect(question.getByRole('button', { name: 'Remove the picture two.png', exact: true })).toBeFocused();
  });

  test('closing a picture refusal returns focus to Add picture', async ({ page, deployment }) => {
    const tutor = await seedVerifiedUser(deployment, 'Maria', 'en');
    const template = await createTemplate(deployment, tutor, 'Pictures', [singleChoice('Which?', ['a', 'b'])]);
    await openBuilder(page, tutor, template.id);
    const question = block(page, '1. Single choice');
    const chooser = page.waitForEvent('filechooser');
    await question.getByRole('button', { name: 'Add picture', exact: true }).click();
    await (await chooser).setFiles({ name: 'notes.txt', mimeType: 'text/plain', buffer: Buffer.from('notes') });
    await question.getByRole('alert').waitFor();

    await question.getByRole('button', { name: 'Close', exact: true }).click();

    await expect(question.getByRole('button', { name: 'Add picture', exact: true })).toBeFocused();
    await expect(question.getByRole('alert')).toHaveCount(0);
  });

  test('a file that is not a picture is refused before it is sent, with focus on the refusal', async ({
    page,
    deployment,
  }) => {
    const tutor = await seedVerifiedUser(deployment, 'Maria', 'en');
    const template = await createTemplate(deployment, tutor, 'Pictures', [singleChoice('Which?', ['a', 'b'])]);
    await openBuilder(page, tutor, template.id);
    const question = block(page, '1. Single choice');
    const chooser = page.waitForEvent('filechooser');
    await question.getByRole('button', { name: 'Add picture', exact: true }).click();

    await (await chooser).setFiles({ name: 'notes.txt', mimeType: 'text/plain', buffer: Buffer.from('notes') });

    await expect(question.getByRole('alert')).toHaveText('This file type is not allowed here');
    await expect(question.getByRole('alert')).toBeFocused();
  });

  test('an unconfirmed tutor builds questions without prompt pictures', async ({ page, deployment }) => {
    const tutor = newTestAccount('Unconfirmed tutor');
    await registerTutor(deployment, tutor, 'en');
    const template = await createTemplate(deployment, tutor, 'Drafts');
    await openBuilder(page, tutor, template.id);

    await addBlock(page, 'Single choice');

    await expect(block(page, '1. Single choice').getByRole('textbox', { name: 'Question', exact: true })).toBeFocused();
    await expect(page.getByRole('button', { name: 'Add picture', exact: true })).toHaveCount(0);
  });
});

test.describe('builder preview', () => {
  test('the preview opens on Close and holds no key, explanation or points in its DOM or its response', async ({
    page,
    deployment,
  }) => {
    const tutor = await seedVerifiedUser(deployment, 'Maria', 'en');
    const template = await sentinelTemplate(deployment, tutor);
    await openBuilder(page, tutor, template.id);
    const studentView = page.waitForResponse(
      (response) => new URL(response.url()).pathname === `/api/v1/library/templates/${template.id}/student-view`,
    );

    await page.getByRole('button', { name: 'Preview', exact: true }).click();
    const responseText = await (await studentView).text();

    const preview = dialog(page);
    await expect(preview.getByRole('button', { name: 'Close', exact: true })).toBeFocused();
    await expect(preview.locator('ol > li')).toHaveCount(8);
    await expect(preview.getByRole('img', { name: 'dot.png', exact: true })).toBeVisible();
    await expect(preview.getByRole('link', { name: 'Verb table', exact: true })).toBeVisible();
    const previewText = await preview.innerText();
    const previewMarkup = await preview.innerHTML();
    for (const sentinel of [sentinels.answer, sentinels.explanation, sentinels.value, sentinels.tolerance]) {
      expect(responseText).not.toContain(sentinel);
      expect(previewText).not.toContain(sentinel);
      expect(previewMarkup).not.toContain(sentinel);
    }
    expect(previewText).not.toContain(String(sentinels.points));
    for (const key of ['"correct"', '"accepted_answers"', '"explanation"', '"points"', '"scoring"', '"max_points"']) {
      expect(responseText).not.toContain(key);
    }
  });

  test('the preview frame switches to the phone width', async ({ page, deployment }) => {
    const tutor = await seedVerifiedUser(deployment, 'Maria', 'en');
    const template = await createTemplate(deployment, tutor, 'Frames', [instruction('Read it.')]);
    await openBuilder(page, tutor, template.id);
    await page.getByRole('button', { name: 'Preview', exact: true }).click();

    await dialog(page).getByRole('button', { name: 'Phone', exact: true }).click();

    await expect(dialog(page).getByRole('button', { name: 'Phone', exact: true })).toHaveAttribute(
      'aria-pressed',
      'true',
    );
    await expect(dialog(page).locator('[data-preview-frame="phone"]')).toBeVisible();
  });

  test('closing the preview returns focus to Preview', async ({ page, deployment }) => {
    const tutor = await seedVerifiedUser(deployment, 'Maria', 'en');
    const template = await createTemplate(deployment, tutor, 'Frames', [instruction('Read it.')]);
    await openBuilder(page, tutor, template.id);
    await page.getByRole('button', { name: 'Preview', exact: true }).click();

    await dialog(page).getByRole('button', { name: 'Close', exact: true }).click();

    await expect(page.getByRole('button', { name: 'Preview', exact: true })).toBeFocused();
  });
});

test.describe('Maria in Russian', () => {
  test.use({ locale: 'ru-RU', timezoneId: 'Europe/Moscow', colorScheme: 'light' });

  test('pilot-2 › step 5 Maria builds Past simple, week 1 from the library and previews it without keys', async ({
    page,
    deployment,
  }) => {
    const maria = await signUpAndConfirmTutor(deployment, 'Мария', 'ru');
    await signInThroughTheApi(page, maria);
    await page.goto('/homework/library');
    await page.getByRole('heading', { level: 1, name: 'Библиотека' }).waitFor();

    await chooseFromPageMenu(page, 'Другие действия с библиотекой', 'Новая папка');
    await dialog(page).getByRole('textbox', { name: 'Название папки' }).fill('Grammar');
    await dialog(page).getByRole('button', { name: 'Создать', exact: true }).click();
    await page.getByRole('table', { name: 'Содержимое библиотеки' }).getByRole('link', { name: 'Grammar' }).click();
    const chooser = page.waitForEvent('filechooser');
    await chooseFromPageMenu(page, 'Другие действия с библиотекой', 'Загрузить материал');
    await (await chooser).setFiles({ name: 'Irregular verbs.pdf', mimeType: 'application/pdf', buffer: documentBytes });
    await dialog(page).getByText('Файл загружен: Irregular verbs.pdf').waitFor();
    await dialog(page).getByRole('button', { name: 'Сохранить', exact: true }).click();
    await page
      .getByRole('table', { name: 'Содержимое библиотеки' })
      .getByRole('button', { name: 'Irregular verbs', exact: true })
      .waitFor();
    await page.getByRole('navigation', { name: 'Путь к папке' }).getByRole('link', { name: 'Библиотека' }).click();
    await page.locator('main header').getByRole('button', { name: 'Новое задание', exact: true }).click();
    await expect(page.getByRole('heading', { level: 1, name: 'Конструктор задания' })).toBeFocused();

    const title = page.getByRole('textbox', { name: 'Название', exact: true });
    await title.fill('Past simple, week 1');
    await addBlock(page, 'Инструкция', 'Добавить блок');
    await block(page, '1. Инструкция')
      .getByRole('textbox', { name: 'Текст', exact: true })
      .fill('Повторите таблицу и ответьте на вопросы.');
    await addBlock(page, 'Материал', 'Добавить блок');
    await dialog(page).getByRole('searchbox', { name: 'Поиск по библиотеке' }).fill('Irregular');
    await dialog(page)
      .getByRole('button', { name: /^Irregular verbs/ })
      .click();
    await expect(block(page, '2. Материал').getByRole('button', { name: 'Заменить материал' })).toBeFocused();

    await addBlock(page, 'Один вариант', 'Добавить блок');
    const single = block(page, '3. Один вариант');
    await single.getByRole('textbox', { name: 'Вопрос', exact: true }).fill('Прошедшее время от go?');
    await single.getByRole('textbox', { name: 'Вариант 1', exact: true }).fill('goed');
    await single.getByRole('textbox', { name: 'Вариант 2', exact: true }).fill('went');
    await single.getByRole('radio', { name: 'Верный: вариант 2', exact: true }).check();
    await single.getByRole('textbox', { name: 'Баллы', exact: true }).fill('2');
    await single.getByRole('button', { name: 'Пояснение', exact: true }).click();
    await single.getByRole('textbox', { name: 'Пояснение', exact: true }).fill('Неправильный глагол go: went.');

    await addBlock(page, 'Короткий ответ', 'Добавить блок');
    const short = block(page, '4. Короткий ответ');
    await short.getByRole('textbox', { name: 'Вопрос', exact: true }).fill('Напишите прошедшее время от go.');
    await short.getByRole('textbox', { name: 'Новый правильный ответ', exact: true }).fill('went');
    await short.getByRole('button', { name: 'Добавить ответ', exact: true }).click();
    await short.getByRole('textbox', { name: 'Баллы', exact: true }).fill('2');
    await short.getByRole('button', { name: 'Пояснение', exact: true }).click();
    await short.getByRole('textbox', { name: 'Пояснение', exact: true }).fill('Форма went не образуется по правилу.');

    await addBlock(page, 'Файл', 'Добавить блок');
    const file = block(page, '5. Файл');
    await file.getByRole('textbox', { name: 'Вопрос', exact: true }).fill('Пришлите фото упражнения 3.');
    await file.getByRole('textbox', { name: 'Баллы', exact: true }).fill('6');
    await file.getByRole('button', { name: 'Пояснение', exact: true }).click();
    await file.getByRole('textbox', { name: 'Пояснение', exact: true }).fill('Проверьте порядок слов.');

    await expect(page.getByText('Готово к выдаче', { exact: true })).toBeVisible();
    await expect(page.getByRole('status').filter({ hasText: 'Сохранено' })).toBeVisible();
    const templateId = new URL(page.url()).pathname.split('/').at(-1) ?? '';
    const studentView = page.waitForResponse(
      (response) => new URL(response.url()).pathname === `/api/v1/library/templates/${templateId}/student-view`,
    );
    await page.getByRole('button', { name: 'Предпросмотр', exact: true }).click();
    const responseText = await (await studentView).text();
    const preview = dialog(page);
    await expect(preview.getByRole('button', { name: 'Закрыть', exact: true })).toBeFocused();
    await expect(preview.locator('ol > li')).toHaveCount(5);
    await expect(preview.locator('ol > li').nth(1).getByRole('link', { name: 'Irregular verbs' })).toBeVisible();
    await expect(preview.locator('ol > li').nth(2).getByRole('radio')).toHaveCount(2);
    const previewText = await preview.innerText();
    for (const explanation of ['Неправильный глагол', 'не образуется по правилу', 'Проверьте порядок слов']) {
      expect(previewText).not.toContain(explanation);
      expect(responseText).not.toContain(explanation);
    }
    for (const key of ['"correct"', '"accepted_answers"', '"explanation"', '"points"', '"max_points"']) {
      expect(responseText).not.toContain(key);
    }
    expect(previewText).not.toMatch(/балл/i);
    await preview.getByRole('button', { name: 'Закрыть', exact: true }).click();

    await page.getByRole('navigation', { name: 'Путь к папке' }).getByRole('link', { name: 'Библиотека' }).click();
    const row = page
      .getByRole('table', { name: 'Содержимое библиотеки' })
      .locator('tr', { hasText: 'Past simple, week 1' });
    await expect(row).toContainText('Готово');
    await expect(row).toContainText('3 вопроса');
    await expect(row).toContainText('10 баллов');
  });
});
