import eslintComments from '@eslint-community/eslint-plugin-eslint-comments';
import js from '@eslint/js';
import jsxA11y from 'eslint-plugin-jsx-a11y';
import reactHooks from 'eslint-plugin-react-hooks';
import { defineConfig } from 'eslint/config';
import globals from 'globals';
import tseslint from 'typescript-eslint';

import { languageChunks } from './scripts/bundle-budget.ts';

const testDoubleMessage = 'Use a fake behind an injected interface instead of replacing behavior.';
const fetchMessage = 'Do not replace fetch; inject a fake client instead.';

const forbiddenTestDoubles = [
  'mock',
  'doMock',
  'importMock',
  'mockObject',
  'spyOn',
  'fn',
  'stubGlobal',
  'stubEnv',
  'useFakeTimers',
  'setSystemTime',
].flatMap((property) => [
  { object: 'vi', property, message: testDoubleMessage },
  { object: 'vitest', property, message: testDoubleMessage },
]);

const forbiddenDynamicImports = [
  { selector: 'ImportExpression[source.value=/^vitest(\\/|$)/]', message: testDoubleMessage },
];

const forbiddenFetchReplacement = [
  {
    selector:
      "CallExpression[callee.object.name='Object'][callee.property.name=/^(defineProperties|assign)$/] Property[key.value='fetch']",
    message: fetchMessage,
  },
  { selector: "AssignmentExpression > MemberExpression.left[property.name='fetch']", message: fetchMessage },
  { selector: "AssignmentExpression > MemberExpression.left[property.value='fetch']", message: fetchMessage },
  { selector: "AssignmentExpression > Identifier.left[name='fetch']", message: fetchMessage },
  {
    selector:
      "CallExpression[callee.object.name='Object'][callee.property.name='defineProperty'] > Literal[value='fetch']",
    message: fetchMessage,
  },
  {
    selector:
      "CallExpression[callee.object.name='Reflect'][callee.property.name=/^(set|defineProperty)$/] > Literal[value='fetch']",
    message: fetchMessage,
  },
  {
    selector:
      "CallExpression[callee.object.name='Object'][callee.property.name=/^(defineProperties|assign)$/] Property[key.name='fetch']",
    message: fetchMessage,
  },
];

const reflectMessage =
  'AGENTS.md, types are precise and safe: no Reflect API in hand-written TypeScript and JavaScript; use typed access instead.';

const forbiddenReflectGlobals = [{ name: 'Reflect', message: reflectMessage }];

const forbiddenReflectSyntax = [
  { selector: "MemberExpression[property.name='Reflect']", message: reflectMessage },
  { selector: "MemberExpression[property.value='Reflect']", message: reflectMessage },
  { selector: "MemberExpression > TemplateLiteral.property[quasis.0.value.raw='Reflect']", message: reflectMessage },
  { selector: "ObjectPattern > Property[key.name='Reflect']", message: reflectMessage },
  { selector: "ObjectPattern > Property[key.value='Reflect']", message: reflectMessage },
];

const domSinkMessage = 'Render text through React; never write HTML, run strings as code or parse markup.';
const cookieMessage = 'OVC keeps tokens out of script-readable cookies; the refresh cookie is HttpOnly.';
const storageMessage = 'Web Storage is used only through src/lib/preferences.';
const indexedDbMessage = 'OVC stores nothing in IndexedDB.';

const globalObjects = ['window', 'globalThis', 'self'];

const forbiddenDomProperties = [
  { property: 'insertAdjacentHTML', message: domSinkMessage },
  { property: 'createContextualFragment', message: domSinkMessage },
  { object: 'document', property: 'write', message: domSinkMessage },
  { object: 'document', property: 'writeln', message: domSinkMessage },
  { property: 'setHTMLUnsafe', message: domSinkMessage },
  { property: 'parseHTMLUnsafe', message: domSinkMessage },
  ...globalObjects.flatMap((object) => [
    { object, property: 'indexedDB', message: indexedDbMessage },
    { object, property: 'eval', message: domSinkMessage },
    { object, property: 'Function', message: domSinkMessage },
    { object, property: 'DOMParser', message: domSinkMessage },
  ]),
];

const storageNames = '/^(localStorage|sessionStorage)$/';
const htmlPropertyNames = '/^(innerHTML|outerHTML|srcdoc|srcDoc|dangerouslySetInnerHTML)$/';
const lookupSinkNames = '/^(innerHTML|outerHTML|srcdoc|cookie|cookieStore)$/';

const forbiddenStorageSyntax = [
  { selector: `MemberExpression[property.name=${storageNames}]`, message: storageMessage },
  { selector: `MemberExpression[property.value=${storageNames}]`, message: storageMessage },
  {
    selector: `MemberExpression > TemplateLiteral.property[quasis.0.value.raw=${storageNames}]`,
    message: storageMessage,
  },
  { selector: `ObjectPattern > Property[key.name=${storageNames}]`, message: storageMessage },
  { selector: `ObjectPattern > Property[key.value=${storageNames}]`, message: storageMessage },
  {
    selector: `CallExpression[callee.object.name=/^(Reflect|Object)$/] > Literal[value=${storageNames}]`,
    message: storageMessage,
  },
  {
    selector: `CallExpression[callee.object.name=/^(Reflect|Object)$/] > TemplateLiteral[quasis.0.value.raw=${storageNames}]`,
    message: storageMessage,
  },
  { selector: "MemberExpression[property.name='storageArea']", message: storageMessage },
];

const forbiddenDomSyntax = [
  { selector: "JSXAttribute[name.name='dangerouslySetInnerHTML']", message: domSinkMessage },
  { selector: 'JSXAttribute[name.name=/^src[Dd]oc$/]', message: domSinkMessage },
  {
    selector: 'AssignmentExpression > MemberExpression.left[property.name=/^(innerHTML|outerHTML|srcdoc)$/]',
    message: domSinkMessage,
  },
  {
    selector: 'AssignmentExpression > MemberExpression.left[property.value=/^(innerHTML|outerHTML|srcdoc)$/]',
    message: domSinkMessage,
  },
  {
    selector:
      "CallExpression[callee.property.name='setAttribute'] > Literal.arguments:nth-child(1)[value=/^(srcdoc|on.*)$/i]",
    message: domSinkMessage,
  },
  {
    selector:
      "CallExpression[callee.property.name='setAttributeNS'] > Literal.arguments:nth-child(2)[value=/^(srcdoc|on.*)$/i]",
    message: domSinkMessage,
  },
  {
    selector: "CallExpression[callee.property.name='setAttribute'][arguments.0.type!='Literal']",
    message: 'Name the attribute with a literal so the lint rules can check it.',
  },
  { selector: `Property[key.name=${htmlPropertyNames}]`, message: domSinkMessage },
  { selector: `Property[key.value=${htmlPropertyNames}]`, message: domSinkMessage },
  { selector: "NewExpression[callee.name='DOMParser']", message: domSinkMessage },
  {
    selector:
      'MemberExpression[property.name=/^(write|writeln)$/] > MemberExpression.object[property.name=/^(document|ownerDocument)$/]',
    message: domSinkMessage,
  },
  { selector: 'MemberExpression[property.name=/^(cookie|cookieStore)$/]', message: cookieMessage },
  { selector: 'MemberExpression[property.value=/^(cookie|cookieStore)$/]', message: cookieMessage },
  {
    selector: `MemberExpression > TemplateLiteral.property[quasis.0.value.raw=${lookupSinkNames}]`,
    message: domSinkMessage,
  },
  {
    selector: `CallExpression[callee.object.name='Reflect'] > Literal[value=${lookupSinkNames}]`,
    message: domSinkMessage,
  },
];

const forbiddenDomGlobals = [
  { name: 'indexedDB', message: indexedDbMessage },
  { name: 'DOMParser', message: domSinkMessage },
];

const forbiddenStorageGlobals = [
  { name: 'localStorage', message: storageMessage },
  { name: 'sessionStorage', message: storageMessage },
  { name: 'cookieStore', message: cookieMessage },
];

const forbiddenTestDoubleImports = {
  paths: [
    { name: 'vitest', importNames: ['vi', 'vitest'], message: testDoubleMessage },
    { name: 'msw', message: testDoubleMessage },
    { name: 'nock', message: testDoubleMessage },
    { name: 'sinon', message: testDoubleMessage },
    { name: 'testdouble', message: testDoubleMessage },
    { name: 'tinyspy', message: testDoubleMessage },
  ],
  patterns: [{ group: ['*mock*', 'msw/*', 'sinon/*', 'testdouble/*'], message: testDoubleMessage }],
};

const testOnlyModuleImports = {
  ...forbiddenTestDoubleImports,
  patterns: [
    ...forbiddenTestDoubleImports.patterns,
    {
      group: ['**/fakeTransport*', '**/transportContract*', '**/fakeGateway*', '**/realtimeContract*'],
      message: 'Fakes and contracts are for tests; production code uses the real adapter',
    },
  ],
};

const languageTextsMessage =
  'A language file holds only its texts: it loads before the first render, so it imports only types and runs no code.';

const languageTextsSyntax = [
  "ImportDeclaration[importKind!='type']",
  'ImportExpression',
  'ExportAllDeclaration',
  'ExportNamedDeclaration[source]',
  'CallExpression',
  'NewExpression',
  'TaggedTemplateExpression',
  'FunctionDeclaration',
  'FunctionExpression',
  'ArrowFunctionExpression',
  'ClassDeclaration',
].map((selector) => ({ selector, message: languageTextsMessage }));

export default defineConfig(
  { ignores: ['dist', 'node_modules', 'playwright-report', 'test-results', 'src/lib/api/schema.ts'] },
  js.configs.recommended,
  {
    files: ['**/*.{ts,tsx}'],
    extends: [tseslint.configs.strictTypeChecked, tseslint.configs.stylisticTypeChecked],
    languageOptions: {
      parserOptions: { projectService: true, tsconfigRootDir: import.meta.dirname },
    },
    rules: {
      '@typescript-eslint/explicit-function-return-type': 'error',
      '@typescript-eslint/ban-ts-comment': [
        'error',
        { 'ts-expect-error': true, 'ts-ignore': true, 'ts-nocheck': true, 'ts-check': true },
      ],
      '@typescript-eslint/no-explicit-any': 'error',
    },
  },
  {
    files: ['src/**/*.{ts,tsx}'],
    extends: [reactHooks.configs.flat['recommended-latest'], jsxA11y.flatConfigs.strict],
    languageOptions: { globals: globals.browser },
  },
  {
    files: ['public/**/*.js'],
    languageOptions: { globals: globals.browser, sourceType: 'script' },
  },
  {
    files: ['*.ts', 'scripts/**/*.ts', 'e2e/**/*.ts'],
    languageOptions: { globals: globals.node },
  },
  {
    plugins: { '@eslint-community/eslint-comments': eslintComments },
    linterOptions: { noInlineConfig: true, reportUnusedDisableDirectives: 'error' },
    rules: {
      '@eslint-community/eslint-comments/no-use': ['error', { allow: [] }],
      'no-restricted-properties': ['error', ...forbiddenTestDoubles],
      'no-restricted-syntax': [
        'error',
        ...forbiddenFetchReplacement,
        ...forbiddenDynamicImports,
        ...forbiddenReflectSyntax,
      ],
      'no-restricted-globals': ['error', ...forbiddenReflectGlobals],
      'no-restricted-imports': ['error', forbiddenTestDoubleImports],
    },
  },
  {
    files: ['src/**/*.{ts,tsx}'],
    ignores: ['src/**/*.test.{ts,tsx}', 'src/lib/api/fakeTransport.ts', 'src/lib/realtime/fakeGateway.ts'],
    rules: { 'no-restricted-imports': ['error', testOnlyModuleImports] },
  },
  {
    files: ['src/**/*.{ts,tsx}', 'public/**/*.js'],
    rules: {
      'no-eval': 'error',
      'no-implied-eval': 'off',
      '@typescript-eslint/no-implied-eval': 'error',
      'no-new-func': 'error',
      'no-restricted-properties': ['error', ...forbiddenTestDoubles, ...forbiddenDomProperties],
      'no-restricted-syntax': [
        'error',
        ...forbiddenFetchReplacement,
        ...forbiddenDynamicImports,
        ...forbiddenReflectSyntax,
        ...forbiddenDomSyntax,
        ...forbiddenStorageSyntax,
      ],
      'no-restricted-globals': [
        'error',
        ...forbiddenReflectGlobals,
        ...forbiddenDomGlobals,
        ...forbiddenStorageGlobals,
      ],
    },
  },
  {
    files: ['public/**/*.js'],
    rules: { '@typescript-eslint/no-implied-eval': 'off', 'no-implied-eval': 'error' },
  },
  {
    files: languageChunks.map(({ module }) => module),
    rules: {
      'no-restricted-syntax': [
        'error',
        ...forbiddenFetchReplacement,
        ...forbiddenDynamicImports,
        ...forbiddenReflectSyntax,
        ...forbiddenDomSyntax,
        ...forbiddenStorageSyntax,
        ...languageTextsSyntax,
      ],
    },
  },
  {
    files: ['src/lib/preferences/store.ts', 'public/theme-init.js'],
    rules: {
      'no-restricted-syntax': [
        'error',
        ...forbiddenFetchReplacement,
        ...forbiddenDynamicImports,
        ...forbiddenReflectSyntax,
        ...forbiddenDomSyntax,
      ],
      'no-restricted-globals': [
        'error',
        ...forbiddenReflectGlobals,
        ...forbiddenDomGlobals,
        { name: 'cookieStore', message: cookieMessage },
      ],
    },
  },
);
