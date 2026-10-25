import { describe, expect, test } from 'vitest';

import { checkBundle, kilobyte, parseManifest, type ServedEntry, unexpectedServedEntries } from './bundle-budget.ts';

const homePage = 'src/features/home/HomePage.tsx';
const todayPage = 'src/features/home/TodayPage.tsx';
const pageModules = [homePage, todayPage];

const firstLoadChunks = ['_gate', '_rule', '_address', '_invitation'];

function firstLoadEntries(homeImports: readonly string[]): Record<string, unknown> {
  return {
    _gate: { file: 'assets/access-gate.js', name: 'access-gate' },
    _rule: { file: 'assets/access-rule.js', name: 'access-rule' },
    _address: { file: 'assets/address-token.js', name: 'address-token' },
    _invitation: { file: 'assets/invitation-token.js', name: 'invitation-token' },
    [homePage]: { file: 'assets/HomePage.js', isDynamicEntry: true, imports: ['_vendor', '_shared', ...homeImports] },
  };
}

function manifestWith(
  entryImports: readonly string[],
  homeImports: readonly string[] = [],
  without: readonly string[] = [],
  extra: Readonly<Record<string, unknown>> = {},
): ReturnType<typeof parseManifest> {
  const entries: Record<string, unknown> = {
    'index.html': {
      file: 'assets/index.js',
      isEntry: true,
      imports: entryImports,
      css: ['assets/index.css'],
      dynamicImports: [homePage, todayPage],
    },
    _vendor: { file: 'assets/vendor.js' },
    [todayPage]: { file: 'assets/TodayPage.js', isDynamicEntry: true, imports: ['_vendor'] },
    _shared: { file: 'assets/shared.js', imports: ['_vendor'] },
    ...firstLoadEntries(homeImports),
    ...extra,
  };
  return parseManifest(Object.fromEntries(Object.entries(entries).filter(([key]) => !without.includes(key))));
}

function sizes(table: Readonly<Record<string, number>>): (file: string) => number {
  return (file) => (table[file] ?? 0) * kilobyte;
}

const withinBudget = { 'assets/index.js': 30, 'assets/index.css': 10, 'assets/vendor.js': 140, 'assets/shared.js': 5 };

describe('bundle check', () => {
  test('counts the entry, its static imports and the page chunk with its imports in a first load', () => {
    const manifest = manifestWith(['_vendor', ...firstLoadChunks]);

    const report = checkBundle(manifest, pageModules, sizes({ ...withinBudget, 'assets/HomePage.js': 4 }));

    expect(report.failures).toEqual([]);
    expect(report.lines).toContain("first load of a student's Home: 189.0 KB gzip (limit 250.0 KB)");
  });

  test('fails when the entry with its static imports or the student first load is above its budget', () => {
    const manifest = manifestWith(['_vendor', ...firstLoadChunks]);

    const report = checkBundle(
      manifest,
      pageModules,
      sizes({ ...withinBudget, 'assets/index.js': 41, 'assets/HomePage.js': 60 }),
    );

    expect(report.failures).toEqual([
      'the entry and its static imports are 181.0 KB gzip of JavaScript, above 180.0 KB',
      "the first load of a student's Home is 256.0 KB gzip, above 250.0 KB",
    ]);
  });

  test('fails when a page module is part of the first load of every page or has no chunk of its own', () => {
    const manifest = manifestWith(['_vendor', ...firstLoadChunks, homePage]);

    const report = checkBundle(manifest, [...pageModules, 'src/features/missing/MissingPage.tsx'], sizes(withinBudget));

    expect(report.failures).toEqual([
      `${homePage} is not loaded on demand`,
      'src/features/missing/MissingPage.tsx is not loaded on demand',
    ]);
  });

  test("fails when a tutor's first load is above its budget", () => {
    const manifest = manifestWith(['_vendor', ...firstLoadChunks]);

    const report = checkBundle(manifest, pageModules, sizes({ ...withinBudget, 'assets/TodayPage.js': 300 }));

    expect(report.failures).toEqual(["the first load of a tutor's Today is 480.0 KB gzip, above 400.0 KB"]);
  });

  test('fails when the access rule is loaded only with a page or a token holder has no chunk of its own', () => {
    const manifest = manifestWith(['_vendor', '_rule', '_address'], ['_gate'], ['_invitation']);

    const report = checkBundle(manifest, pageModules, sizes(withinBudget));

    expect(report.failures).toEqual([
      'src/routes/Gate.tsx is not part of the first load in its own chunk access-gate',
      'src/features/auth/InvitationToken.tsx is not part of the first load in its own chunk invitation-token',
    ]);
  });

  test('fails when a first-load chunk name is used twice or names a chunk loaded on demand', () => {
    const extra = {
      _copy: { file: 'assets/address-token-copy.js', name: 'address-token' },
      _invitation: { file: 'assets/invitation-token.js', name: 'invitation-token', isDynamicEntry: true },
    };
    const manifest = manifestWith(['_vendor', ...firstLoadChunks, '_copy'], [], [], extra);

    const report = checkBundle(manifest, pageModules, sizes(withinBudget));

    expect(report.failures).toEqual([
      'src/features/auth/AddressToken.tsx is not part of the first load in its own chunk address-token',
      'src/features/auth/InvitationToken.tsx is not part of the first load in its own chunk invitation-token',
    ]);
  });

  test('refuses a manifest entry without a file', () => {
    const malformed = { 'index.html': { isEntry: true } };

    const parse = (): unknown => parseManifest(malformed);

    expect(parse).toThrow('manifest entry index.html has no file');
  });
});

describe('served build contents', () => {
  const publicFiles = new Set(['favicon.svg', 'theme-init.js']);

  function builtOutput(extra: readonly ServedEntry[] = []): ServedEntry[] {
    return [
      { path: 'index.html', kind: 'file' },
      { path: 'favicon.svg', kind: 'file' },
      { path: 'theme-init.js', kind: 'file' },
      { path: 'assets', kind: 'directory' },
      { path: 'assets/index-a1b2.js', kind: 'file' },
      { path: 'assets/index-a1b2.css', kind: 'file' },
      { path: 'assets/golos-text-cyrillic.woff2', kind: 'file' },
      { path: 'assets/golos-text-cyrillic.woff', kind: 'file' },
      ...extra,
    ];
  }

  test('accepts the page, the public files and hashed assets', () => {
    const entries = builtOutput();

    const unexpected = unexpectedServedEntries(entries, publicFiles);

    expect(unexpected).toEqual([]);
  });

  test('refuses source maps, manifests, reports and keys wherever they appear', () => {
    const entries = builtOutput([
      { path: 'assets/index-a1b2.js.map', kind: 'file' },
      { path: '.vite', kind: 'directory' },
      { path: '.vite/manifest.json', kind: 'file' },
      { path: 'stats.html', kind: 'file' },
      { path: 'assets/preview-private-key.pem', kind: 'file' },
    ]);

    const unexpected = unexpectedServedEntries(entries, publicFiles);

    expect(unexpected).toEqual([
      '.vite (directory)',
      '.vite/manifest.json (file)',
      'assets/index-a1b2.js.map (file)',
      'assets/preview-private-key.pem (file)',
      'stats.html (file)',
    ]);
  });

  test('refuses a symbolic link even when its name looks like an asset', () => {
    const entries = builtOutput([{ path: 'assets/report.js', kind: 'link' }]);

    const unexpected = unexpectedServedEntries(entries, publicFiles);

    expect(unexpected).toEqual(['assets/report.js (link)']);
  });

  test('refuses a nested folder inside the assets folder', () => {
    const entries = builtOutput([
      { path: 'assets/nested', kind: 'directory' },
      { path: 'assets/nested/chunk.js', kind: 'file' },
    ]);

    const unexpected = unexpectedServedEntries(entries, publicFiles);

    expect(unexpected).toEqual(['assets/nested (directory)', 'assets/nested/chunk.js (file)']);
  });
});
