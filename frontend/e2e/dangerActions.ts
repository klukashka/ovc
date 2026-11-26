import type { Locator, Page } from '@playwright/test';

import { expect } from './fixtures.ts';

export const transparent = 'rgba(0, 0, 0, 0)';

export function openedMenu(page: Page): Locator {
  return page.locator('main [aria-expanded="true"] + div');
}

export async function backgroundOf(locator: Locator): Promise<string> {
  return locator.evaluate((element) => getComputedStyle(element).backgroundColor);
}

export async function expectLastDangerItem(page: Page, item: string, separated: boolean): Promise<void> {
  const lastItem = openedMenu(page).locator(':scope > button:last-child');
  await expect(lastItem).toHaveText(item);
  await expect(lastItem).toHaveClass(/text-danger/);
  await expect(lastItem.locator('svg')).toHaveCount(1);
  await expect(openedMenu(page).locator(':scope > hr')).toHaveCount(separated ? 1 : 0);
}

export async function expectDangerConfirmation(dialog: Locator, confirm: string, cancel: string): Promise<void> {
  await expect(dialog.getByRole('button', { name: cancel, exact: true })).toBeFocused();
  await expect(dialog.getByRole('button')).toHaveText([confirm, cancel]);
  expect(await backgroundOf(dialog.getByRole('button', { name: confirm, exact: true }))).not.toBe(transparent);
  expect(await backgroundOf(dialog.getByRole('button', { name: cancel, exact: true }))).toBe(transparent);
}
