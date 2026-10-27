import { ESLint } from 'eslint';
import { resolve } from 'node:path';
import { beforeAll, describe, expect, test } from 'vitest';

const frontendRoot = resolve(import.meta.dirname, '..');
const fixturePath = resolve(frontendRoot, 'src/lib/preferences/store.ts');
const componentPath = resolve(frontendRoot, 'src/routes/NotFoundPage.tsx');

const eslint = new ESLint({ cwd: frontendRoot, flags: ['unstable_native_nodejs_ts_config'] });

async function ruleIds(source: string, filePath: string = fixturePath): Promise<string[]> {
  const [result] = await eslint.lintText(source, { filePath });
  return (result?.messages ?? []).map((message) => message.ruleId ?? 'fatal');
}

describe('frontend lint configuration', () => {
  beforeAll(async () => {
    await eslint.lintText('export const warm = 1;\n', { filePath: fixturePath });
  });

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
    ['a Reflect.get call', "export const value: unknown = Reflect.get(globalThis, 'x');\n", 'no-restricted-globals'],
    [
      'Reflect reached through globalThis',
      "export const value: unknown = globalThis.Reflect.get(globalThis, 'x');\n",
      'no-restricted-syntax',
    ],
    [
      'Reflect reached by computed key',
      "export const value: unknown = globalThis['Reflect'].get(globalThis, 'x');\n",
      'no-restricted-syntax',
    ],
    [
      'destructured Reflect',
      'const { Reflect: reflection } = globalThis;\nexport const value = reflection;\n',
      'no-restricted-syntax',
    ],
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
    ['a dynamic import of vitest', "export const testing = await import('vitest');\n", 'no-restricted-syntax'],
    [
      'Object.defineProperty on fetch',
      "Object.defineProperty(globalThis, 'fetch', { value: null });\n",
      'no-restricted-syntax',
    ],
    [
      'a dynamic import of a vitest subpath',
      "export const testing = await import('vitest/node');\n",
      'no-restricted-syntax',
    ],
    [
      'Object.assign with a quoted fetch key',
      "Object.assign(globalThis, { 'fetch': null });\n",
      'no-restricted-syntax',
    ],
    [
      'Object.defineProperties on fetch',
      'Object.defineProperties(globalThis, { fetch: { value: null } });\n',
      'no-restricted-syntax',
    ],
    ['sinon', "import sinon from 'sinon';\nexport const stub = sinon;\n", 'no-restricted-imports'],
    ['testdouble', "import td from 'testdouble';\nexport const double = td;\n", 'no-restricted-imports'],
    [
      'a module named like a mock',
      "import { thing } from './api-mock';\nexport const value = thing;\n",
      'no-restricted-imports',
    ],
    ['a spy library', "import { spy } from 'tinyspy';\nexport const watch = spy;\n", 'no-restricted-imports'],
  ])('rejects %s', async (_name, source, expectedRule) => {
    const found = await ruleIds(source);

    expect(found).toContain(expectedRule);
  });

  test.each([
    [
      'dangerouslySetInnerHTML',
      'export function Fixture(): React.JSX.Element {\n  return <div dangerouslySetInnerHTML={{ __html: "x" }} />;\n}\n',
      'no-restricted-syntax',
    ],
    [
      'an iframe srcDoc',
      'export function Fixture(): React.JSX.Element {\n  return <iframe title="x" srcDoc="<p>x</p>" />;\n}\n',
      'no-restricted-syntax',
    ],
    ['assigning innerHTML', "document.body.innerHTML = '<p>x</p>';\n", 'no-restricted-syntax'],
    ['assigning outerHTML', "document.body.outerHTML = '<p>x</p>';\n", 'no-restricted-syntax'],
    ['assigning innerHTML by computed key', "document.body['innerHTML'] = '<p>x</p>';\n", 'no-restricted-syntax'],
    [
      'assigning srcdoc',
      "export function fill(frame: HTMLIFrameElement): void {\n  frame.srcdoc = '<p>x</p>';\n}\n",
      'no-restricted-syntax',
    ],
    ['setting srcdoc as an attribute', "document.body.setAttribute('srcdoc', '<p>x</p>');\n", 'no-restricted-syntax'],
    ['insertAdjacentHTML', "document.body.insertAdjacentHTML('beforeend', '<p>x</p>');\n", 'no-restricted-properties'],
    ['document.write', "document.write('<p>x</p>');\n", 'no-restricted-properties'],
    ['document.writeln', "document.writeln('<p>x</p>');\n", 'no-restricted-properties'],
    ['document.cookie', 'export const cookies = document.cookie;\n', 'no-restricted-syntax'],
    ['document.cookie by computed key', "export const cookies = document['cookie'];\n", 'no-restricted-syntax'],
    [
      'createContextualFragment',
      "export const fragment = document.createRange().createContextualFragment('<p>x</p>');\n",
      'no-restricted-properties',
    ],
    [
      'DOMParser',
      "export const parsed = new DOMParser().parseFromString('<p>x</p>', 'text/html');\n",
      'no-restricted-globals',
    ],
    ['indexedDB', "export const opened = indexedDB.open('ovc');\n", 'no-restricted-globals'],
    ['window.indexedDB', "export const opened = window.indexedDB.open('ovc');\n", 'no-restricted-properties'],
    ['eval', "export const value: unknown = eval('1');\n", 'no-eval'],
    ['new Function', "export const run = new Function('return 1');\n", 'no-new-func'],
    ['localStorage', "export const stored = localStorage.getItem('ovc.theme');\n", 'no-restricted-globals'],
    ['sessionStorage', "export const stored = sessionStorage.getItem('ovc.theme');\n", 'no-restricted-globals'],
    [
      'window.localStorage',
      "export const stored = window.localStorage.getItem('ovc.theme');\n",
      'no-restricted-syntax',
    ],
    [
      'globalThis.sessionStorage',
      "export const stored = globalThis.sessionStorage.getItem('ovc.theme');\n",
      'no-restricted-syntax',
    ],
    ['self.localStorage', "export const stored = self.localStorage.getItem('ovc.theme');\n", 'no-restricted-syntax'],
    [
      "window['localStorage']",
      "export const stored = window['localStorage'].getItem('ovc.theme');\n",
      'no-restricted-syntax',
    ],
    [
      'an aliased window',
      "const w = window;\nexport const stored = w.localStorage.getItem('ovc.theme');\n",
      'no-restricted-syntax',
    ],
    ['defaultView', "export const stored = document.defaultView?.localStorage.getItem('x');\n", 'no-restricted-syntax'],
    [
      'destructured storage',
      'const { sessionStorage: kept } = window;\nexport const stored = kept;\n',
      'no-restricted-syntax',
    ],
    ['optional chaining', "export const stored = window?.localStorage.getItem('x');\n", 'no-restricted-syntax'],
    ['Reflect.get', "export const stored: unknown = Reflect.get(window, 'localStorage');\n", 'no-restricted-syntax'],
    ['an aliased document cookie', 'const d = document;\nexport const cookies = d.cookie;\n', 'no-restricted-syntax'],
    ['cookieStore', "export const cookies = window.cookieStore.get('x');\n", 'no-restricted-syntax'],
    [
      'Object.assign of innerHTML',
      "Object.assign(document.body, { innerHTML: '<p>x</p>' });\n",
      'no-restricted-syntax',
    ],
    [
      'createElement with dangerouslySetInnerHTML',
      "import { createElement } from 'react';\nexport const node = createElement('div', { dangerouslySetInnerHTML: { __html: 'x' } });\n",
      'no-restricted-syntax',
    ],
    ['setHTMLUnsafe', "document.body.setHTMLUnsafe('<p>x</p>');\n", 'no-restricted-properties'],
    ['an upper-case srcdoc attribute', "document.body.setAttribute('SRCDOC', 'x');\n", 'no-restricted-syntax'],
    ['an event-handler attribute', "document.body.setAttribute('onclick', 'x');\n", 'no-restricted-syntax'],
    [
      'a computed attribute name',
      "const name = 'id';\ndocument.body.setAttribute(name, 'x');\n",
      'no-restricted-syntax',
    ],
    ['a string timer', "setTimeout('alert(1)', 1);\n", '@typescript-eslint/no-implied-eval'],
    [
      'a quoted destructuring key',
      "const { 'localStorage': kept } = window;\nexport const stored = kept;\n",
      'no-restricted-syntax',
    ],
    ['a template-literal key', 'export const stored = window[`localStorage`];\n', 'no-restricted-syntax'],
    [
      'a property descriptor',
      "export const found = Object.getOwnPropertyDescriptor(window, 'localStorage');\n",
      'no-restricted-syntax',
    ],
    [
      'a storage event area',
      "addEventListener('storage', (event) => event.storageArea?.clear());\n",
      'no-restricted-syntax',
    ],
    ['a nested document.write', "window.document.write('<p>x</p>');\n", 'no-restricted-syntax'],
    ['setAttributeNS with a handler', "document.body.setAttributeNS(null, 'onclick', 'x');\n", 'no-restricted-syntax'],
    ['Reflect.set of innerHTML', "Reflect.set(document.body, 'innerHTML', '<p>x</p>');\n", 'no-restricted-syntax'],
    ['a template-literal cookie key', 'export const cookies = document[`cookie`];\n', 'no-restricted-syntax'],
  ])('rejects %s in application code', async (_name, source, expectedRule) => {
    const found = await ruleIds(source, componentPath);

    expect(found).toContain(expectedRule);
  });

  test('allows Web Storage only inside the preferences module', async () => {
    const source = "export const stored = window.localStorage.getItem('ovc.theme');\n";

    const inPreferences = await ruleIds(source);
    const elsewhere = await ruleIds(source, componentPath);

    expect(inPreferences).toEqual([]);
    expect(elsewhere).toContain('no-restricted-syntax');
  });

  test('applies the same rules to the scripts in public/ except the theme script', async () => {
    const source = "window.localStorage.getItem('ovc.theme');\nsetTimeout('alert(1)', 1);\n";

    const otherScript = await ruleIds(source, resolve(frontendRoot, 'public/other.js'));
    const themeScript = await ruleIds(
      "window.localStorage.getItem('ovc.theme');\n",
      resolve(frontendRoot, 'public/theme-init.js'),
    );

    expect(otherScript).toEqual(expect.arrayContaining(['no-restricted-syntax', 'no-implied-eval']));
    expect(themeScript).toEqual([]);
  });

  test.each([
    ['an end-to-end page script', 'e2e/observers.ts'],
    ['a frontend script', 'scripts/check-comments.ts'],
    ['a configuration file', 'playwright.config.ts'],
    ['a public script', 'public/theme-init.js'],
  ])('refuses a Reflect.get call in %s', async (_name, path) => {
    const source = "Reflect.get(globalThis, 'x');\n";

    const found = await ruleIds(source, resolve(frontendRoot, path));

    expect(found).toContain('no-restricted-globals');
  });

  test('accepts an ordinary attribute whose value looks like a handler name', async () => {
    const found = await ruleIds("document.body.setAttribute('title', 'one');\n", componentPath);

    expect(found).toEqual([]);
  });

  test('refuses the authentication fake and contract outside tests and allows them in tests', async () => {
    const source =
      "import { FakeAuthServer } from '../lib/api/fakeTransport.ts';\nexport const server = new FakeAuthServer();\n";

    const inProduction = await ruleIds(source, componentPath);
    const inTest = await ruleIds(source, resolve(frontendRoot, 'src/routes/access.test.ts'));

    expect(inProduction).toContain('no-restricted-imports');
    expect(inTest).not.toContain('no-restricted-imports');
  });

  test('refuses the authentication contract outside tests', async () => {
    const source =
      "import { authTransportContract } from '../lib/api/transportContract.ts';\nexport const contract = authTransportContract;\n";

    const messages = await ruleIds(source, componentPath);

    expect(messages).toContain('no-restricted-imports');
  });

  test('accepts plain typed code', async () => {
    const found = await ruleIds('export function double(value: number): number {\n  return value * 2;\n}\n');

    expect(found).toEqual([]);
  });

  test.each([
    ['a value import', "import { languages } from './languages.ts';\nexport const ru = { list: languages };\n"],
    ['a dynamic import', "export const ru = { later: import('./languages.ts') };\n"],
    ['a re-export', "export { languages } from './languages.ts';\n"],
    ['a call', 'export const ru = { stamp: String(1) };\n'],
    ['a function', 'export const ru = { stamp: (): string => "x" };\n'],
  ])('refuses %s in a language file', async (_name, source) => {
    const found = await ruleIds(source, resolve(frontendRoot, 'src/lib/i18n/ru.ts'));

    expect(found).toContain('no-restricted-syntax');
  });

  test('accepts texts with a type-only import in a language file', async () => {
    const source =
      "import type { Resources } from './resources.ts';\n\nexport const en: Resources = {} as Resources;\n";

    const found = await ruleIds(source, resolve(frontendRoot, 'src/lib/i18n/en.ts'));

    expect(found).not.toContain('no-restricted-syntax');
  });
});
