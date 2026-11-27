import { signInThroughTheApi } from './accounts.ts';
import { diagnosedTest as test, expect, seedVerifiedUser } from './fixtures.ts';
import { channelOf, messages, openSettledConversation, postMessage, send } from './messages.ts';
import { accessToken, callApi, createGroup, deleteAccount, seedActiveStudent } from './tutoring.ts';

test.use({
  locale: 'en-GB',
  timezoneId: 'Europe/Moscow',
  viewport: { width: 1280, height: 800 },
  clockMovedFarAhead: true,
});

test('copy text › a message with only attachments, a deleted placeholder and an erased placeholder offer no Copy text', async ({
  page,
  deployment,
  control,
}) => {
  const tutor = await seedVerifiedUser(deployment, 'Copy tutor', 'en');
  const { student, cardId } = await seedActiveStudent(deployment, tutor, 'Copy student', 'en');
  const group = await createGroup(deployment, tutor, 'Copy exclusions', [cardId]);
  const channel = await channelOf(deployment, tutor, `/groups/${group}`);
  await postMessage(deployment, student, channel, 'Will be erased', null);
  const deleted = await postMessage(deployment, tutor, channel, 'Will be deleted', null);
  const deletion = await callApi(deployment, await accessToken(deployment, tutor), 'DELETE', `/messages/${deleted}`);
  if (deletion.status !== 204) throw new Error('message deletion failed');
  await deleteAccount(deployment, student);
  await control.advance(31 * 24 * 60 * 60);
  await control.tick();
  await signInThroughTheApi(page, tutor);
  await openSettledConversation(page, channel, 'with the chat list');
  await page.getByText('Message removed', { exact: true }).waitFor();
  await page.locator('input[type=file]').setInputFiles({
    name: 'copy-exclusion.png',
    mimeType: 'image/png',
    buffer: Buffer.from(
      'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==',
      'base64',
    ),
  });
  await page.getByRole('list', { name: 'Attachments', exact: true }).getByText('Uploaded', { exact: true }).waitFor();
  await send(page, '');
  const attachment = messages(page).filter({ has: page.getByRole('img', { name: 'copy-exclusion.png' }) });

  await attachment.getByRole('button', { name: 'Message actions' }).click();

  await expect(attachment.getByRole('button', { name: 'Message actions' })).toHaveAttribute('aria-expanded', 'true');
  await expect(page.getByRole('button', { name: 'Copy text', exact: true })).toHaveCount(0);
  await expect(
    messages(page).filter({ hasText: 'Message deleted' }).getByRole('button', { name: 'Message actions' }),
  ).toHaveCount(0);
  await expect(
    messages(page).filter({ hasText: 'Message removed' }).getByRole('button', { name: 'Message actions' }),
  ).toHaveCount(0);
});
