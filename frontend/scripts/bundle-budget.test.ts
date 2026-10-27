import { describe, expect, test } from 'vitest';

import {
  type BuiltChunk,
  checkBundle,
  committedPublicFiles,
  kilobyte,
  languageChunkContentProblems,
  manifestNamedFiles,
  parseManifest,
  type ServedEntry,
  type ServedFiles,
  unexpectedServedEntries,
} from './bundle-budget.ts';

const homePage = 'src/features/home/HomePage.tsx';
const todayPage = 'src/features/home/TodayPage.tsx';
const pageModules = [homePage, todayPage];

const firstLoadChunks = ['_gate', '_rule', '_address', '_invitation'];

const russianTexts = 'src/lib/i18n/ru.ts';
const englishTexts = 'src/lib/i18n/en.ts';

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
      dynamicImports: [homePage, todayPage, russianTexts, englishTexts],
    },
    [russianTexts]: { file: 'assets/language-ru-a1.js', isDynamicEntry: true },
    [englishTexts]: { file: 'assets/language-en-b2.js', isDynamicEntry: true },
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
      'the entry and its static imports with the largest language chunk are 181.0 KB gzip of JavaScript, above 180.0 KB',
      "the first load of a student's Home is 256.0 KB gzip, above 250.0 KB",
    ]);
  });

  test('counts the largest language chunk in the entry and page budgets', () => {
    const manifest = manifestWith(['_vendor', ...firstLoadChunks]);
    const languageSizes = { 'assets/language-ru-a1.js': 6, 'assets/language-en-b2.js': 5 };

    const report = checkBundle(
      manifest,
      pageModules,
      sizes({ ...withinBudget, ...languageSizes, 'assets/index.js': 35 }),
    );

    expect(report.lines).toEqual([
      'entry and its static imports: 175.0 KB gzip of JavaScript, with the largest language chunk 6.0 KB: 181.0 KB (limit 180.0 KB)',
      "first load of a student's Home: 196.0 KB gzip (limit 250.0 KB)",
      "first load of a tutor's Today: 191.0 KB gzip (limit 400.0 KB)",
      '2 page modules load on demand',
    ]);
    expect(report.failures).toEqual([
      'the entry and its static imports with the largest language chunk are 181.0 KB gzip of JavaScript, above 180.0 KB',
    ]);
  });

  test('fails when a language is part of the first load of every page or has no chunk of its own', () => {
    const manifest = manifestWith(['_vendor', ...firstLoadChunks, russianTexts], [], [englishTexts]);

    const report = checkBundle(manifest, pageModules, sizes(withinBudget));

    expect(report.failures).toEqual([
      `${russianTexts} is not a language chunk loaded on demand`,
      `${englishTexts} is not a language chunk loaded on demand`,
    ]);
  });

  test('fails when a language chunk is not named as one, so the token pages could not tell it from route code', () => {
    const extra = { [englishTexts]: { file: 'assets/en-b2.js', isDynamicEntry: true } };
    const manifest = manifestWith(['_vendor', ...firstLoadChunks], [], [], extra);

    const report = checkBundle(manifest, pageModules, sizes(withinBudget));

    expect(report.failures).toEqual([`${englishTexts} is not a language chunk loaded on demand`]);
  });

  test('fails when a language chunk imports other chunks or another chunk takes a language chunk name', () => {
    const extra = {
      [russianTexts]: { file: 'assets/language-ru-a1.js', isDynamicEntry: true, imports: ['_shared'] },
      [englishTexts]: { file: 'assets/language-en-b2.js', isDynamicEntry: true, dynamicImports: [todayPage] },
      [todayPage]: { file: 'assets/language-ru-c3.js', isDynamicEntry: true, imports: ['_vendor'] },
    };
    const manifest = manifestWith(['_vendor', ...firstLoadChunks], [], [], extra);

    const report = checkBundle(manifest, pageModules, sizes(withinBudget));

    expect(report.failures).toEqual([
      `${russianTexts} imports other chunks, so its chunk could carry code`,
      `${englishTexts} imports other chunks, so its chunk could carry code`,
      `${todayPage} is built as assets/language-ru-c3.js, a language chunk name, but is not a language's texts`,
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

describe('language chunk contents', () => {
  const root = '/work/frontend';

  function builtChunk(fileName: string, facade: string | null, modules: readonly string[]): BuiltChunk {
    return {
      fileName,
      facadeModuleId: facade === null ? null : `${root}/${facade}`,
      moduleIds: modules.map((module) => `${root}/${module}`),
    };
  }

  test('accepts each language built alone into its own chunk', () => {
    const chunks = [
      builtChunk('assets/language-ru-a1.js', russianTexts, [russianTexts]),
      builtChunk('assets/language-en-b2.js', englishTexts, [englishTexts]),
      builtChunk('assets/index.js', 'index.html', ['src/main.tsx', 'src/lib/i18n/i18n.ts']),
    ];

    const problems = languageChunkContentProblems(chunks);

    expect(problems).toEqual([]);
  });

  test('refuses a language chunk that also holds code and a language merged into another chunk', () => {
    const chunks = [
      builtChunk('assets/language-ru-a1.js', russianTexts, [russianTexts, 'src/lib/format/dates.ts']),
      builtChunk('assets/index.js', 'index.html', ['src/main.tsx', englishTexts]),
    ];

    const problems = languageChunkContentProblems(chunks);

    expect(problems).toEqual([
      `assets/language-ru-a1.js holds ${russianTexts} together with other modules`,
      `assets/index.js holds ${englishTexts} together with other modules`,
    ]);
  });
});

describe('served build contents', () => {
  const served: ServedFiles = {
    trackedPublicFiles: new Set(['favicon.svg', 'theme-init.js']),
    namedAssets: new Set([
      'assets/index-a1b2.js',
      'assets/index-a1b2.css',
      'assets/golos-text-cyrillic.woff2',
      'assets/golos-text-cyrillic.woff',
      'assets/report.js',
      'assets/nested/chunk.js',
    ]),
  };

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

    const unexpected = unexpectedServedEntries(entries, served);

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

    const unexpected = unexpectedServedEntries(entries, served);

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

    const unexpected = unexpectedServedEntries(entries, served);

    expect(unexpected).toEqual(['assets/report.js (link)']);
  });

  test('refuses a nested folder inside the assets folder', () => {
    const entries = builtOutput([
      { path: 'assets/nested', kind: 'directory' },
      { path: 'assets/nested/chunk.js', kind: 'file' },
    ]);

    const unexpected = unexpectedServedEntries(entries, served);

    expect(unexpected).toEqual(['assets/nested (directory)', 'assets/nested/chunk.js (file)']);
  });

  test('refuses a file in the public folder that Git does not track', () => {
    const entries = builtOutput([{ path: 'notes.txt', kind: 'file' }]);

    const unexpected = unexpectedServedEntries(entries, served);

    expect(unexpected).toEqual(['notes.txt (file)']);
  });

  test('refuses dotfiles, also when Git tracks them in the public folder', () => {
    const entries = builtOutput([
      { path: '.env.production', kind: 'file' },
      { path: '.well-known', kind: 'directory' },
      { path: 'assets/.index-a1b2.js', kind: 'file' },
    ]);
    const withTrackedDotfile: ServedFiles = {
      trackedPublicFiles: new Set([...served.trackedPublicFiles, '.env.production']),
      namedAssets: new Set([...served.namedAssets, 'assets/.index-a1b2.js']),
    };

    const unexpected = unexpectedServedEntries(entries, withTrackedDotfile);

    expect(unexpected).toEqual(['.env.production (file)', '.well-known (directory)', 'assets/.index-a1b2.js (file)']);
  });

  test('refuses an asset file that the build manifest does not name', () => {
    const entries = builtOutput([{ path: 'assets/leftover-c3d4.js', kind: 'file' }]);

    const unexpected = unexpectedServedEntries(entries, served);

    expect(unexpected).toEqual(['assets/leftover-c3d4.js (file)']);
  });

  test('names the chunk files, their stylesheets and their assets from the manifest', () => {
    const manifest = parseManifest({
      'index.html': {
        file: 'assets/index-a1b2.js',
        css: ['assets/index-a1b2.css'],
        assets: ['assets/golos-text-cyrillic.woff2'],
      },
      'src/features/home/HomePage.tsx': { file: 'assets/HomePage-e5f6.js', isDynamicEntry: true },
    });

    const named = manifestNamedFiles(manifest);

    expect([...named].sort()).toEqual([
      'assets/HomePage-e5f6.js',
      'assets/golos-text-cyrillic.woff2',
      'assets/index-a1b2.css',
      'assets/index-a1b2.js',
    ]);
  });

  test('serves only committed regular files of the public folder, never a link, an assumed-unchanged or a skipped entry', () => {
    const listing = {
      indexEntries: [
        'H 100644 aaaf7945d168a15611e96918078c2880c276eebe 0\tpublic/favicon.svg',
        'H 100755 2a3921f54ce5d1a653558e9fd9948290fe2e34b9 0\tpublic/theme-init.js',
        'H 120000 3b18e512dba79e4c8300dd08aeb37f8e728b8dad 0\tpublic/notes.txt',
        'H 100644 4b825dc642cb6eb9a060e54bf8d69288fbee4904 0\tpublic/replaced.txt',
        'h 100644 5c1b14949828006ed75a3e8858957f86a2f7e2eb 0\tpublic/assumed-unchanged.txt',
        'S 100644 6d0b8c1f2f3e4a5b6c7d8e9f0a1b2c3d4e5f6a7b 0\tpublic/skipped.txt',
      ].join('\0'),
      uncommitted: '',
    };
    const kinds = new Map([['public/replaced.txt', 'link' as const]]);

    const committed = committedPublicFiles('public', listing, (path) => kinds.get(path) ?? 'file');

    expect([...committed.files].sort()).toEqual(['favicon.svg', 'theme-init.js']);
    expect(committed.problems).toEqual([]);
  });

  test('refuses a public folder with uncommitted or untracked changes', () => {
    const listing = {
      indexEntries: 'H 100644 aaaf7945d168a15611e96918078c2880c276eebe 0\tpublic/favicon.svg',
      uncommitted: [' M public/favicon.svg', 'A  public/staged.txt', '?? public/new.txt'].join('\0'),
    };

    const committed = committedPublicFiles('public', listing, () => 'file');

    expect(committed.problems).toEqual([
      'public/ has changes that are not committed: M public/favicon.svg, A  public/staged.txt, ?? public/new.txt',
    ]);
  });
});
