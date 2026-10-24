import type { Page } from '@playwright/test';

const busyKey = 'ovcBusyControls';
const offsetsKey = 'ovcFirstOffsets';

export async function recordBusyControls(page: Page): Promise<void> {
  await page.addInitScript((key) => {
    const snapshots: string[][] = [];
    Reflect.set(window, key, snapshots);
    const busy = 'button[aria-busy="true"]';
    new MutationObserver((records) => {
      if (records.some((record) => record.target instanceof Element && record.target.matches(busy))) {
        snapshots.push(
          [...document.querySelectorAll(busy)].map(
            (button) => button.getAttribute('aria-label') ?? button.textContent.trim(),
          ),
        );
      }
    }).observe(document, { subtree: true, attributes: true, attributeFilter: ['aria-busy'] });
  }, busyKey);
}

export async function busyControls(page: Page): Promise<string[][]> {
  const recorded: unknown = await page.evaluate((key): unknown => Reflect.get(window, key), busyKey);
  if (
    !Array.isArray(recorded) ||
    !recorded.every(
      (snapshot): snapshot is string[] =>
        Array.isArray(snapshot) && snapshot.every((name): name is string => typeof name === 'string'),
    )
  ) {
    throw new Error('no busy controls were recorded on this page');
  }
  return recorded;
}

export async function recordFirstOffsets(page: Page, selectors: readonly string[]): Promise<void> {
  await page.addInitScript(
    ({ key, tracked }) => {
      const first = new Map<string, number>();
      Reflect.set(window, key, first);
      function measure(): void {
        const heading = document.querySelector('main h1');
        if (heading === null) {
          return;
        }
        for (const selector of tracked) {
          const element = document.querySelector(selector);
          if (first.has(selector) || element === null || element.getClientRects().length === 0) {
            continue;
          }
          first.set(selector, element.getBoundingClientRect().top - heading.getBoundingClientRect().bottom);
        }
      }
      new MutationObserver(measure).observe(document, { subtree: true, childList: true, attributes: true });
    },
    { key: offsetsKey, tracked: selectors },
  );
}

export interface Offsets {
  readonly first: Readonly<Record<string, number>>;
  readonly now: Readonly<Record<string, number>>;
}

export async function offsetsBelowHeading(page: Page, selectors: readonly string[]): Promise<Offsets> {
  return page.evaluate(
    ({ key, tracked }) => {
      const recorded: unknown = Reflect.get(window, key);
      const first = recorded instanceof Map ? recorded : new Map<unknown, unknown>();
      const heading = document.querySelector('main h1');
      const offsets: { first: Record<string, number>; now: Record<string, number> } = { first: {}, now: {} };
      for (const selector of tracked) {
        const earliest: unknown = first.get(selector);
        const element = document.querySelector(selector);
        if (typeof earliest === 'number') {
          offsets.first[selector] = earliest;
        }
        if (element !== null && heading !== null) {
          offsets.now[selector] = element.getBoundingClientRect().top - heading.getBoundingClientRect().bottom;
        }
      }
      return offsets;
    },
    { key: offsetsKey, tracked: selectors },
  );
}
