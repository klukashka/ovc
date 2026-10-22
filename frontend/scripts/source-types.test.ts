import { describe, expect, test } from 'vitest';

import { publicExtensions, sourceExtensions, unlintedFiles } from './source-types.ts';

describe('source file types', () => {
  test('refuses source files the lint rules do not cover', () => {
    const paths = ['src/main.tsx', 'src/lib/a.ts', 'src/styles.css', 'src/x.js', 'src/y.mts', 'src/z.jsx', 'src/w.cjs'];

    const unlinted = unlintedFiles(paths, sourceExtensions);

    expect(unlinted).toEqual(['src/x.js', 'src/y.mts', 'src/z.jsx', 'src/w.cjs']);
  });

  test('allows only linted scripts and static assets in public', () => {
    const paths = ['public/theme-init.js', 'public/favicon.svg', 'public/a.mjs', 'public/b.html', 'public/c.cjs'];

    const unlinted = unlintedFiles(paths, publicExtensions);

    expect(unlinted).toEqual(['public/a.mjs', 'public/b.html', 'public/c.cjs']);
  });
});
