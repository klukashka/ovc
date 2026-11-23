import { newTestAccount, signInThroughTheApi } from './accounts.ts';
import { diagnosedTest as test, expect, seedVerifiedUser } from './fixtures.ts';
import { createCard } from './tutoring.ts';

test.use({ locale: 'en-GB', colorScheme: 'dark', clockMovedFarAhead: true });

test('student card › a card whose email was erased refuses Create a new link and Send until an email is entered', async ({
  page,
  deployment,
  control,
}) => {
  const pavel = await seedVerifiedUser(deployment, 'Pavel', 'en');
  const card = await createCard(deployment, pavel, 'Boris', newTestAccount('Boris', 'student').email);
  const newEmail = newTestAccount('Boris', 'student').email;
  await control.advance(90 * 24 * 60 * 60 + 60);
  await control.tick();
  await signInThroughTheApi(page, pavel);
  await page.goto(`/students/${card.id}`);
  const main = page.getByRole('main');
  await main.getByRole('button', { name: 'Enter email' }).waitFor();
  const explanations = await main
    .getByText(
      'The email was removed because the invitation was not accepted within 90 days, and the link no longer works.',
    )
    .count();
  const erasedControls = await main
    .getByRole('button', { name: /^(Copy link|Send by email|Create a new link)$/ })
    .count();
  const erasedLinks = await main.getByText(/\/invite\//).count();

  await main.getByRole('button', { name: 'Enter email' }).click();
  const dialog = page.getByRole('dialog', { name: 'Name and email' });
  await dialog.getByLabel('Email (optional)').fill(newEmail);
  await dialog.getByRole('button', { name: 'Save' }).click();

  expect(explanations).toBe(1);
  expect(erasedControls).toBe(0);
  expect(erasedLinks).toBe(0);
  await expect(dialog).toBeHidden();
  await expect(page.getByRole('status').filter({ hasText: 'Saved.' })).toBeFocused();
  await expect(main.getByRole('button', { name: 'Send by email' })).toBeVisible();
  await expect(main.getByRole('button', { name: 'Create a new link' })).toBeVisible();
  await expect(main.getByText(/\/invite\//)).not.toHaveText(card.link);
});
