import type { Page } from '@playwright/test';

declare global {
  interface Window {
    ovcBusyControls?: string[][];
    ovcFirstOffsets?: Map<string, number>;
    ovcFirstBodyTheme?: string;
    ovcSchemeChanges?: number;
  }
}

export async function recordBusyControls(page: Page): Promise<void> {
  await page.addInitScript(() => {
    const snapshots: string[][] = [];
    window.ovcBusyControls = snapshots;
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
  });
}

export async function busyControls(page: Page): Promise<string[][]> {
  const recorded: unknown = await page.evaluate((): unknown => window.ovcBusyControls);
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
    ({ tracked }) => {
      const first = new Map<string, number>();
      window.ovcFirstOffsets = first;
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
    { tracked: selectors },
  );
}

export interface Offsets {
  readonly first: Readonly<Record<string, number>>;
  readonly now: Readonly<Record<string, number>>;
}

export async function offsetsBelowHeading(page: Page, selectors: readonly string[]): Promise<Offsets> {
  return page.evaluate(
    ({ tracked }) => {
      const first = window.ovcFirstOffsets ?? new Map<string, number>();
      const heading = document.querySelector('main h1');
      const offsets: { first: Record<string, number>; now: Record<string, number> } = { first: {}, now: {} };
      for (const selector of tracked) {
        const earliest = first.get(selector);
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
    { tracked: selectors },
  );
}
