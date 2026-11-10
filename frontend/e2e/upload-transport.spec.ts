import { readdir } from 'node:fs/promises';
import { resolve } from 'node:path';

import type { Page, Response as BrowserResponse } from '@playwright/test';

import { type UploadContractSubject, uploadContract, contractUploadFile } from '../src/lib/api/uploadContract.ts';
import type { UploadTransfer } from '../src/lib/api/transport.ts';
import { signInThroughTheApi } from './accounts.ts';
import { diagnosedTest as test, expect, seedVerifiedUser } from './fixtures.ts';
import { channelOf } from './messages.ts';
import {
  accessTokenExpiredWithinSocketGrace,
  expectRefetchedAfterTheSocketIsReady,
  trafficAfterReady,
} from './realtime.ts';
import { accessToken, createGroup } from './tutoring.ts';

async function uploadModulePath(): Promise<string> {
  const files = await readdir(resolve(import.meta.dirname, '../dist/assets'));
  const modules = files.filter((file) => /^uploadTransport-[A-Za-z0-9_-]+\.js$/.test(file));
  const module = modules[0];
  if (modules.length !== 1 || module === undefined) throw new Error('the production upload module is not unique');
  return `/assets/${module}`;
}

function browserSubject(
  page: Page,
  modulePath: string,
  token: string,
  channel: string,
  outside: string,
): UploadContractSubject {
  return {
    upload: async (file, scope, signal, progress): Promise<Response> => {
      const result = await page.evaluate(
        async ({ modulePath, token, channel, file, aborted }) => {
          const module: unknown = await import(modulePath);
          function isUploadModule(
            value: unknown,
          ): value is { readonly sendUpload: (request: Request, upload: UploadTransfer) => Promise<Response> } {
            return (
              typeof value === 'object' &&
              value !== null &&
              'sendUpload' in value &&
              typeof value.sendUpload === 'function'
            );
          }
          if (!isUploadModule(module)) throw new Error('the production upload module has no transport');
          const signal = aborted ? AbortSignal.timeout(0) : new AbortController().signal;
          if (aborted && !signal.aborted)
            await new Promise<void>((resolve) => {
              signal.addEventListener(
                'abort',
                () => {
                  resolve();
                },
                { once: true },
              );
            });
          const body = new FormData();
          body.append('file', new File([new Uint8Array(file.bytes)], file.name, { type: file.mediaType }));
          const samples: { loaded: number; total: number | null }[] = [];
          try {
            const response = await module.sendUpload(
              new Request(`${location.origin}/api/v1/uploads?context=chat&scope=channel:${channel}`, {
                method: 'POST',
                signal,
                headers: { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' },
              }),
              {
                body,
                progress: (loaded, total) => {
                  samples.push({ loaded, total });
                },
              },
            );
            return { kind: 'response' as const, status: response.status, text: await response.text(), samples };
          } catch (error) {
            if (error instanceof DOMException && error.name === 'TimeoutError')
              return { kind: 'aborted' as const, samples };
            throw error;
          }
        },
        { modulePath, token, channel: scope === 'member' ? channel : outside, file, aborted: signal.aborted },
      );
      for (const sample of result.samples) progress(sample.loaded, sample.total);
      if (result.kind === 'aborted') throw new DOMException('upload expired', 'TimeoutError');
      return new Response(result.text, { status: result.status, headers: { 'Content-Type': 'application/json' } });
    },
  };
}

function isUploadResponse(response: BrowserResponse): boolean {
  return response.request().method() === 'POST' && new URL(response.url()).pathname === '/api/v1/uploads';
}

function deferred(): {
  readonly promise: Promise<void>;
  readonly resolve: () => void;
  readonly reject: (error: unknown) => void;
} {
  let resolve = (): void => undefined;
  let reject: (error: unknown) => void = (): void => undefined;
  const promise = new Promise<void>((accept, refuse) => {
    resolve = accept;
    reject = refuse;
  });
  return { promise, resolve, reject };
}

async function holdUpload(
  page: Page,
): Promise<{ readonly reached: Promise<void>; readonly release: () => Promise<void> }> {
  const reached = deferred();
  const release = deferred();
  const completed = deferred();
  const pattern = '**/api/v1/uploads?**';
  await page.route(pattern, async (route) => {
    reached.resolve();
    await release.promise;
    try {
      await route.continue();
      completed.resolve();
    } catch (error) {
      completed.reject(error);
      throw error;
    }
  });
  return {
    reached: reached.promise,
    release: async (): Promise<void> => {
      release.resolve();
      await completed.promise;
      await page.unroute(pattern);
    },
  };
}

const filePayload = {
  name: contractUploadFile.name,
  mimeType: contractUploadFile.mediaType,
  buffer: Buffer.from(contractUploadFile.bytes),
};

test.use({ locale: 'en-GB' });

test('upload transport › the production browser XHR and fake share multipart, pre-abort, progress and scope contracts', async ({
  page,
  deployment,
  diagnostics,
}) => {
  const tutor = await seedVerifiedUser(deployment, 'Upload contract tutor', 'en');
  const other = await seedVerifiedUser(deployment, 'Another upload tutor', 'en');
  const group = await createGroup(deployment, tutor, 'Upload group', []);
  const otherGroup = await createGroup(deployment, other, 'Other group', []);
  const channel = await channelOf(deployment, tutor, `/groups/${group}`);
  const outside = await channelOf(deployment, other, `/groups/${otherGroup}`);
  const token = await accessToken(deployment, tutor);
  await page.goto('/');
  const subject = browserSubject(page, await uploadModulePath(), token, channel, outside);
  diagnostics.expectFailure({
    method: 'POST',
    path: `/api/v1/uploads?context=chat&scope=channel:${outside}`,
    status: 404,
  });

  const run = async (): Promise<void> => {
    for (const scenario of Object.values(uploadContract)) await scenario(subject);
  };

  await expect(run()).resolves.toBeUndefined();
});

test('upload transport › an expired multipart upload refreshes once, removal aborts, and changing accounts clears a held upload', async ({
  page,
  deployment,
  control,
  diagnostics,
}) => {
  const tutor = await seedVerifiedUser(deployment, 'Uploading tutor', 'en');
  const other = await seedVerifiedUser(deployment, 'Next tutor', 'en');
  const group = await createGroup(deployment, tutor, 'Upload lifecycle', []);
  const nextGroup = await createGroup(deployment, other, 'Next account chat', []);
  const channel = await channelOf(deployment, tutor, `/groups/${group}`);
  const nextChannel = await channelOf(deployment, other, `/groups/${nextGroup}`);
  await signInThroughTheApi(page, tutor);
  const traffic = trafficAfterReady(page, [
    `/api/v1/channels/${channel}`,
    `/api/v1/channels/${channel}/messages`,
    '/api/v1/channels',
    '/api/v1/channels/unread',
  ]);
  await traffic.initialize();
  await page.goto(`/messages/${channel}`);
  await expectRefetchedAfterTheSocketIsReady(traffic);
  const uploads: number[] = [];
  let refreshes = 0;
  page.on('response', (response) => {
    if (isUploadResponse(response)) uploads.push(response.status());
    if (response.request().method() === 'POST' && new URL(response.url()).pathname === '/api/v1/auth/refresh')
      refreshes += 1;
  });
  diagnostics.expectFailure({
    method: 'POST',
    path: `/api/v1/uploads?context=chat&scope=channel%3A${channel}`,
    status: 401,
  });
  diagnostics.expectAbortedRequest(/^\/api\/v1\/uploads$/);

  const expired = await holdUpload(page);
  const saved = page.waitForResponse((response) => isUploadResponse(response) && response.status() === 201);
  await page.locator('input[type=file]').setInputFiles(filePayload);
  await expired.reached;
  await control.advance(accessTokenExpiredWithinSocketGrace);
  await expired.release();
  const uploaded: unknown = await (await saved).json();
  await page.getByRole('list', { name: 'Attachments', exact: true }).getByText('Uploaded', { exact: true }).waitFor();
  const replayStatuses = [...uploads];
  const replayRefreshes = refreshes;
  await page.getByRole('button', { name: 'Remove file', exact: true }).click();
  const removed = await holdUpload(page);
  await page.locator('input[type=file]').setInputFiles({ ...filePayload, name: 'cancelled.txt' });
  await removed.reached;
  const removedFailure = page.waitForEvent('requestfailed', {
    predicate: (request) => new URL(request.url()).pathname === '/api/v1/uploads',
  });
  await page.getByRole('button', { name: 'Remove file', exact: true }).click();
  await removed.release();
  const removalError = (await removedFailure).failure()?.errorText;
  const afterRemoval = await page.getByRole('list', { name: 'Attachments', exact: true }).getByRole('listitem').count();
  const switched = await holdUpload(page);
  await page.locator('input[type=file]').setInputFiles({ ...filePayload, name: 'previous-account.txt' });
  await switched.reached;
  const switchedFailure = page.waitForEvent('requestfailed', {
    predicate: (request) => new URL(request.url()).pathname === '/api/v1/uploads',
  });
  await page.getByRole('button', { name: /Account menu/ }).click();
  await page.getByRole('button', { name: 'Sign out', exact: true }).click();
  await page.getByRole('button', { name: 'Sign in', exact: true }).waitFor();
  await switched.release();
  const switchError = (await switchedFailure).failure()?.errorText;
  await signInThroughTheApi(page, other);
  await page.goto(`/messages/${nextChannel}`);

  expect(replayStatuses).toEqual([401, 201]);
  expect(replayRefreshes).toBe(1);
  expect(uploaded).toMatchObject({
    filename: filePayload.name,
    media_type: filePayload.mimeType,
    size_bytes: filePayload.buffer.length,
  });
  expect(afterRemoval).toBe(0);
  expect(removalError).toMatch(/abort|cancel/i);
  expect(switchError).toMatch(/abort|cancel/i);
  await expect(page.getByRole('heading', { name: 'Next account chat', exact: true })).toBeVisible();
  await expect(page.getByRole('list', { name: 'Attachments', exact: true }).getByRole('listitem')).toHaveCount(0);
  await expect(page.getByRole('main')).not.toContainText('previous-account.txt');
});
