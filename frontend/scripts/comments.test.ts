import { describe, expect, test } from 'vitest';

import { cssComments, htmlComments, scriptComments } from './comments.ts';

describe('scriptComments', () => {
  test.each([
    ['line comment', 'const value = 1;\n// explain\n', 2],
    ['block comment', '/* explain */\nconst value = 1;\n', 1],
    ['trailing comment', 'const value = 1; // explain\n', 1],
    ['comment at end of file', 'const value = 1;\n/* end */', 2],
    ['tool directive', 'const value = 1;\n// eslint-disable-next-line\nconst other = 2;\n', 2],
  ])('finds a %s', (_name, source, line) => {
    const found = scriptComments('example.ts', source);

    expect(found.map((finding) => finding.line)).toEqual([line]);
  });

  test('finds a comment inside a JSX expression', () => {
    const source = 'export const view = <div>{/* explain */}</div>;\n';

    const found = scriptComments('example.tsx', source);

    expect(found).toHaveLength(1);
  });

  test.each([
    ['URL in a string', "const address = 'https://example.org/path';\n"],
    ['URL in JSX text', 'export const view = <p>https://example.org</p>;\n'],
    ['slashes in a regular expression', 'const pattern = /\\/\\//;\n'],
    ['comment markers in a template literal', 'const text = `/* not a comment */ // either`;\n'],
  ])('ignores a %s', (_name, source) => {
    const found = scriptComments(source.includes('<p>') ? 'example.tsx' : 'example.ts', source);

    expect(found).toEqual([]);
  });

  test.each([
    ['example.cts', 'const value = 1;\n// explain\n'],
    ['tsconfig.json', '{\n  // explain\n  "files": []\n}\n'],
  ])('finds a comment in %s', (path, source) => {
    const found = scriptComments(path, source);

    expect(found.map((finding) => finding.line)).toEqual([2]);
  });

  test('ignores comment markers inside JSON strings', () => {
    const found = scriptComments('data.json', '{ "url": "https://example.org/*x*/" }\n');

    expect(found).toEqual([]);
  });

  test('allows a shebang on the first line', () => {
    const found = scriptComments('example.ts', '#!/usr/bin/env node\nconst value = 1;\n');

    expect(found).toEqual([]);
  });
});

describe('cssComments', () => {
  test('finds a block comment and ignores one inside a string', () => {
    const source = ".a { content: '/* text */'; }\n/* explain */\n";

    const found = cssComments(source);

    expect(found.map((finding) => finding.line)).toEqual([2]);
  });
});

describe('htmlComments', () => {
  test('finds a markup comment', () => {
    const found = htmlComments('<div></div>\n<!-- explain -->\n');

    expect(found.map((finding) => finding.line)).toEqual([2]);
  });
});
