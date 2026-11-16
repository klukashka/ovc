import type { Locator, Page } from '@playwright/test';

export const saveDelayMilliseconds = 1000;

export interface PageClockOption {
  readonly pageClock?: boolean;
}

export async function installPageClock(page: Page, option: PageClockOption = {}): Promise<void> {
  if (option.pageClock === true) {
    await page.clock.install();
  }
}

export async function skipSaveDelay(page: Page): Promise<void> {
  await page.clock.fastForward(saveDelayMilliseconds);
}

export async function skipSaveDelayOnceArmed(page: Page, armed: Locator): Promise<void> {
  await armed.waitFor();
  await skipSaveDelay(page);
}

export async function skipVisibleSaveDelay(page: Page): Promise<void> {
  await skipSaveDelayOnceArmed(
    page,
    page.getByRole('status').filter({ hasText: /^(Saving…|Saved|Сохраняется…|Сохранено)$/ }),
  );
}
