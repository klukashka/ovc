import { ESLint } from 'eslint';
import { resolve } from 'node:path';
import { describe, expect, test } from 'vitest';

const frontendRoot = resolve(import.meta.dirname, '..');
const fixturePath = resolve(frontendRoot, 'src/lib/preferences/store.ts');

async function ruleIds(source: string): Promise<string[]> {
  const eslint = new ESLint({ cwd: frontendRoot, flags: ['unstable_native_nodejs_ts_config'] });
  const [result] = await eslint.lintText(source, { filePath: fixturePath });
  return (result?.messages ?? []).map((message) => message.ruleId ?? 'fatal');
}

describe('frontend lint configuration', () => {
  test.each([
    ['importing vi', "import { vi } from 'vitest';\nexport const tools = vi;\n", 'no-restricted-imports'],
    [
      'importing vitest under its other name',
      "import { vitest } from 'vitest';\nexport const tools = vitest;\n",
      'no-restricted-imports',
    ],
    [
      'a namespace import of vitest',
      "import * as testing from 'vitest';\nexport const tools = testing;\n",
      'no-restricted-imports',
    ],
    ['vi.mock', "import { vi } from 'vitest';\nvi.mock('./module');\n", 'no-restricted-properties'],
    ['vitest.mock', "import { vitest } from 'vitest';\nvitest.mock('./module');\n", 'no-restricted-properties'],
    ['vi.importMock', "import { vi } from 'vitest';\nawait vi.importMock('./module');\n", 'no-restricted-properties'],
    ['vi.mockObject', "import { vi } from 'vitest';\nvi.mockObject({});\n", 'no-restricted-properties'],
    ['vi.useFakeTimers', "import { vi } from 'vitest';\nvi.useFakeTimers();\n", 'no-restricted-properties'],
    ['vi.setSystemTime', "import { vi } from 'vitest';\nvi.setSystemTime(0);\n", 'no-restricted-properties'],
    ['vi.doMock', "import { vi } from 'vitest';\nvi.doMock('./module');\n", 'no-restricted-properties'],
    ['vi.spyOn', "import { vi } from 'vitest';\nvi.spyOn(console, 'log');\n", 'no-restricted-properties'],
    ['vi.fn', "import { vi } from 'vitest';\nexport const handler = vi.fn();\n", 'no-restricted-properties'],
    ['vi.stubGlobal', "import { vi } from 'vitest';\nvi.stubGlobal('fetch', null);\n", 'no-restricted-properties'],
    ['vi.stubEnv', "import { vi } from 'vitest';\nvi.stubEnv('MODE', 'test');\n", 'no-restricted-properties'],
    ['assigning globalThis.fetch', 'globalThis.fetch = async () => Response.error();\n', 'no-restricted-syntax'],
    ['assigning window.fetch', 'window.fetch = async () => Response.error();\n', 'no-restricted-syntax'],
    ['assigning fetch by computed key', "window['fetch'] = async () => Response.error();\n", 'no-restricted-syntax'],
    ['Reflect.set on fetch', "Reflect.set(globalThis, 'fetch', null);\n", 'no-restricted-syntax'],
    ['Object.assign of fetch', 'Object.assign(globalThis, { fetch: null });\n', 'no-restricted-syntax'],
    [
      'an ESLint directive',
      '// eslint-disable-next-line\nexport const value = 1;\n',
      '@eslint-community/eslint-comments/no-use',
    ],
    [
      'a TypeScript directive',
      '// @ts-expect-error\nexport const value: number = 1;\n',
      '@typescript-eslint/ban-ts-comment',
    ],
    ['explicit any', 'export const value: any = 1;\n', '@typescript-eslint/no-explicit-any'],
    ['a mocking library', "import nock from 'nock';\nexport const scope = nock;\n", 'no-restricted-imports'],
    [
      'a mocking library subpath',
      "import { setupServer } from 'msw/node';\nexport const server = setupServer;\n",
      'no-restricted-imports',
    ],
    ['a spy library', "import { spy } from 'tinyspy';\nexport const watch = spy;\n", 'no-restricted-imports'],
  ])('rejects %s', async (_name, source, expectedRule) => {
    const found = await ruleIds(source);

    expect(found).toContain(expectedRule);
  });

  test('accepts plain typed code', async () => {
    const found = await ruleIds('export function double(value: number): number {\n  return value * 2;\n}\n');

    expect(found).toEqual([]);
  });
});
