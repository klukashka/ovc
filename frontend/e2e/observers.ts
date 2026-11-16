import type { Page, Request } from '@playwright/test';

import { languageChunkPattern } from '../scripts/bundle-budget.ts';

declare global {
  interface Window {
    ovcBusyControls?: string[][];
    ovcFirstOffsets?: Map<string, number>;
    ovcFirstBodyTheme?: string;
    ovcHeadingsAtPopstate?: string[];
    ovcSchemeChanges?: number;
    ovcShownRegion?: ShownRegion;
  }
}

export interface ShownRegion {
  busy: boolean;
  replaced: boolean;
}

export async function recordShownRegion(page: Page, selector: string): Promise<void> {
  await page.evaluate(
    ({ watched }) => {
      const region = document.querySelector(watched);
      const holder = region?.closest('[aria-busy]');
      if (region === null || holder === null || holder === undefined) {
        throw new Error(`no ${watched} inside an element with aria-busy is shown`);
      }
      const shown: ShownRegion = { busy: false, replaced: false };
      window.ovcShownRegion = shown;
      new MutationObserver(() => {
        shown.replaced ||= !region.isConnected;
        shown.busy ||= holder.getAttribute('aria-busy') === 'true';
      }).observe(document, { subtree: true, childList: true, attributes: true, attributeFilter: ['aria-busy'] });
    },
    { watched: selector },
  );
}

export async function shownRegion(page: Page): Promise<ShownRegion> {
  const recorded: unknown = await page.evaluate((): unknown => window.ovcShownRegion);
  if (
    typeof recorded !== 'object' ||
    recorded === null ||
    !('busy' in recorded) ||
    typeof recorded.busy !== 'boolean' ||
    !('replaced' in recorded) ||
    typeof recorded.replaced !== 'boolean'
  ) {
    throw new Error('no shown region was recorded on this page');
  }
  return { busy: recorded.busy, replaced: recorded.replaced };
}

export async function recordHeadingsAtPopstate(page: Page): Promise<void> {
  await page.evaluate(() => {
    const headings: string[] = [];
    window.ovcHeadingsAtPopstate = headings;
    window.addEventListener('popstate', () => {
      headings.push(document.querySelector('h1')?.textContent ?? '');
    });
  });
}

export async function headingsAtPopstate(page: Page): Promise<string[]> {
  const recorded: unknown = await page.evaluate((): unknown => window.ovcHeadingsAtPopstate);
  if (!Array.isArray(recorded) || !recorded.every((heading): heading is string => typeof heading === 'string')) {
    throw new Error('no headings at popstate were recorded on this page');
  }
  return recorded;
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

const scriptAsset = /\/assets\/[^/]+\.js$/;
const shellScript = /(?:src|href)="(\/assets\/[^"]+\.js)"/g;

export async function recordLazyChunkAddresses(page: Page, origin: string): Promise<string[]> {
  const shell = await (await page.request.get(`${origin}/`)).text();
  const initial = new Set([...shell.matchAll(shellScript)].map((match) => match[1]));
  const addresses: string[] = [];
  await page.route(scriptAsset, async (route) => {
    const path = new URL(route.request().url()).pathname;
    if (!initial.has(path) && !languageChunkPattern.test(path.slice(1))) {
      addresses.push(await page.evaluate(() => globalThis.location.href));
    }
    await route.continue();
  });
  return addresses;
}

export function apiRequestsInFlight(page: Page): () => number {
  const pending = new Set<Request>();
  page.on('request', (request) => {
    if (new URL(request.url()).pathname.startsWith('/api/')) {
      pending.add(request);
    }
  });
  const settled = (request: Request): void => {
    pending.delete(request);
  };
  page.on('requestfinished', settled);
  page.on('requestfailed', settled);
  return () => pending.size;
}
