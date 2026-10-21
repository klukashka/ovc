import { describe, expect, test } from 'vitest';

import { unlintedSourceFiles } from './source-types.ts';

describe('source file types', () => {
  test('refuses files the lint rules do not cover', () => {
    const paths = ['src/main.tsx', 'src/lib/a.ts', 'src/styles.css', 'src/x.js', 'src/y.mts', 'src/z.jsx', 'src/w.cjs'];

    const unlinted = unlintedSourceFiles(paths);

    expect(unlinted).toEqual(['src/x.js', 'src/y.mts', 'src/z.jsx', 'src/w.cjs']);
  });
});
