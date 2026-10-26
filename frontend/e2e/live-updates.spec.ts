import type { Locator, Page } from '@playwright/test';

import { type TestAccount, newTestAccount, signInThroughTheApi } from './accounts.ts';
import type { Deployment } from './deployment.ts';
import { diagnosedTest as test, expect, seedVerifiedUser } from './fixtures.ts';
import { accountId, waitForSockets } from './realtime.ts';
import {
  confirmInTheOwnBrowser,
  createCard,
  createGroup,
  endRelationship,
  seedActiveStudent,
  decideJoinRequest,
  removeStudent,
  seedJoinRequests,
  signUpOnInvitation,
} from './tutoring.ts';

function joinRequests(page: Page): Locator {
  return page.getByRole('region', { name: 'Заявки на занятия' });
}

function acceptButton(page: Page, name: string): Locator {
  return joinRequests(page).getByRole('button', { name: `Принять: ${name}` });
}

interface RequestsOnToday {
  readonly maria: TestAccount;
  readonly shown: readonly TestAccount[];
}

async function requestsOnToday(page: Page, deployment: Deployment, count: number): Promise<RequestsOnToday> {
  const maria = await seedVerifiedUser(deployment, 'Мария');
  const requesters = await seedJoinRequests(deployment, maria, count);
  await signInThroughTheApi(page, maria);
  await page.goto('/today');
  const rows = joinRequests(page).getByRole('listitem');
  await expect(rows).toHaveCount(count);
  await waitForSockets(deployment, await accountId(deployment, maria), 1);
  const texts = await rows.allTextContents();
  const shown = texts.map((text) => {
    const found = requesters.find((candidate) => text.includes(candidate.email));
    if (found === undefined) {
      throw new Error('a shown request belongs to no seeded requester');
    }
    return found;
  });
  return { maria, shown };
}

function shownAt(shown: readonly TestAccount[], index: number): TestAccount {
  const found = shown[index];
  if (found === undefined) {
    throw new Error(`no request is shown at ${String(index)}`);
  }
  return found;
}

test('live updates › removing the focused request row moves focus to the next row', async ({ page, deployment }) => {
  const { maria, shown } = await requestsOnToday(page, deployment, 3);
  await acceptButton(page, shownAt(shown, 1).name).focus();

  await decideJoinRequest(deployment, maria, shownAt(shown, 1).email, 'accept');

  await expect(acceptButton(page, shownAt(shown, 2).name)).toBeFocused();
  await expect(joinRequests(page).getByRole('listitem')).toHaveCount(2);
});

test('live updates › removing the focused request row moves focus to the previous row when it was the last', async ({
  page,
  deployment,
}) => {
  const { maria, shown } = await requestsOnToday(page, deployment, 2);
  await acceptButton(page, shownAt(shown, 1).name).focus();

  await decideJoinRequest(deployment, maria, shownAt(shown, 1).email, 'decline');

  await expect(acceptButton(page, shownAt(shown, 0).name)).toBeFocused();
});

test('live updates › removing the focused request row moves focus to the list heading when it was the only row', async ({
  page,
  deployment,
}) => {
  const { maria, shown } = await requestsOnToday(page, deployment, 1);
  await acceptButton(page, shownAt(shown, 0).name).focus();

  await decideJoinRequest(deployment, maria, shownAt(shown, 0).email, 'block');

  await expect(joinRequests(page).getByRole('heading', { name: 'Заявки на занятия' })).toBeFocused();
  await expect(joinRequests(page).getByText('Новых заявок нет.')).toBeVisible();
});

test('live updates › a dialog whose student was removed closes onto the notice', async ({
  page,
  deployment,
  diagnostics,
}) => {
  const pavel = await seedVerifiedUser(deployment, 'Pavel', 'en');
  const boris = newTestAccount('Boris Account', 'student');
  const card = await createCard(deployment, pavel, 'Boris', boris.email);
  await signUpOnInvitation(deployment, card.link, boris, 'en');
  await signInThroughTheApi(page, pavel);
  await page.goto(`/students/${card.id}`);
  await page.getByRole('button', { name: 'End working together' }).click();
  await expect(page.getByRole('dialog', { name: 'End working together?' })).toBeVisible();
  await waitForSockets(deployment, await accountId(deployment, pavel), 1);
  diagnostics.expectFailure({ method: 'GET', path: `/api/v1/students/${card.id}`, status: 404 });

  await removeStudent(deployment, pavel, card.id);

  await expect(page.getByRole('dialog')).toHaveCount(0);
  await expect(page.getByRole('status').filter({ hasText: 'This student is no longer on your list.' })).toBeFocused();
  await expect(page.getByRole('main').getByRole('link', { name: 'All students' })).toBeVisible();
});

test('live updates › a card whose state changed while its dialog was open closes the dialog onto the changed notice', async ({
  page,
  deployment,
  control,
}) => {
  const pavel = await seedVerifiedUser(deployment, 'Pavel', 'en');
  const boris = newTestAccount('Boris Account', 'student');
  const card = await createCard(deployment, pavel, 'Boris', boris.email);
  await signUpOnInvitation(deployment, card.link, boris, 'en');
  await signInThroughTheApi(page, pavel);
  await page.goto(`/students/${card.id}`);
  await page.getByRole('button', { name: 'End working together' }).click();
  await expect(page.getByRole('dialog', { name: 'End working together?' })).toBeVisible();
  await waitForSockets(deployment, await accountId(deployment, pavel), 1);

  await confirmInTheOwnBrowser(deployment, control, boris);

  await expect(page.getByRole('dialog')).toHaveCount(0);
  await expect(page.getByRole('status').filter({ hasText: 'This card changed in the meantime.' })).toBeFocused();
  await expect(page.getByRole('main').getByText('Email not confirmed')).toHaveCount(0);
});

test('live updates › a stop dialog whose tutor removed the link closes onto the notice', async ({
  page,
  deployment,
}) => {
  const pavel = await seedVerifiedUser(deployment, 'Pavel', 'en');
  const boris = newTestAccount('Boris', 'student');
  const card = await createCard(deployment, pavel, 'Boris', boris.email);
  await signUpOnInvitation(deployment, card.link, boris, 'en');
  await signInThroughTheApi(page, boris);
  await page.goto('/tutors');
  await page.getByRole('button', { name: 'More actions for Pavel' }).click();
  await page.getByRole('button', { name: 'Stop studying' }).click();
  await expect(page.getByRole('dialog', { name: 'Stop studying with this tutor?' })).toBeVisible();
  await waitForSockets(deployment, await accountId(deployment, boris), 1);

  await removeStudent(deployment, pavel, card.id);

  await expect(page.getByRole('dialog')).toHaveCount(0);
  await expect(page.getByRole('status').filter({ hasText: 'The link to the tutor was removed.' })).toBeFocused();
  await expect(page.getByRole('main').getByText('No tutor yet.')).toBeVisible();
});

test('live updates › a stop dialog whose tutor ended the relationship closes onto the changed notice', async ({
  page,
  deployment,
}) => {
  const pavel = await seedVerifiedUser(deployment, 'Pavel', 'en');
  const { student: anna, cardId } = await seedActiveStudent(deployment, pavel, 'Anna');
  await signInThroughTheApi(page, anna);
  await page.goto('/tutors');
  await page.getByRole('button', { name: 'More actions for Pavel' }).click();
  await page.getByRole('button', { name: 'Stop studying' }).click();
  await expect(page.getByRole('dialog', { name: 'Stop studying with this tutor?' })).toBeVisible();
  await waitForSockets(deployment, await accountId(deployment, anna), 1);

  await endRelationship(deployment, pavel, cardId);

  await expect(page.getByRole('dialog')).toHaveCount(0);
  await expect(
    page.getByRole('status').filter({ hasText: 'Your tutors changed in the meantime. The list below is up to date.' }),
  ).toBeFocused();
});

test('live updates › removing the focused group member moves focus to the next member', async ({
  page,
  deployment,
}) => {
  const pavel = await seedVerifiedUser(deployment, 'Pavel', 'en');
  const anna = await seedActiveStudent(deployment, pavel, 'Anna');
  const boris = await seedActiveStudent(deployment, pavel, 'Boris');
  const groupId = await createGroup(deployment, pavel, 'B1 evening', [anna.cardId, boris.cardId]);
  await signInThroughTheApi(page, pavel);
  await page.goto(`/groups/${groupId}`);
  const members = page.getByRole('list', { name: 'Students' });
  await expect(members.getByRole('listitem')).toHaveCount(2);
  await waitForSockets(deployment, await accountId(deployment, pavel), 1);
  const shown = await members.getByRole('link').allTextContents();
  const [focusedName = '', nextName = ''] = shown;
  const focusedCard = focusedName === 'Anna' ? anna.cardId : boris.cardId;
  await members.getByRole('link', { name: focusedName }).focus();

  await endRelationship(deployment, pavel, focusedCard);

  await expect(members.getByRole('link', { name: nextName })).toBeFocused();
  await expect(members.getByRole('listitem')).toHaveCount(1);
});
