import eslintComments from '@eslint-community/eslint-plugin-eslint-comments';
import js from '@eslint/js';
import jsxA11y from 'eslint-plugin-jsx-a11y';
import reactHooks from 'eslint-plugin-react-hooks';
import { defineConfig } from 'eslint/config';
import globals from 'globals';
import tseslint from 'typescript-eslint';

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
      'no-restricted-syntax': ['error', ...forbiddenFetchReplacement, ...forbiddenDynamicImports],
      'no-restricted-imports': ['error', forbiddenTestDoubleImports],
    },
  },
);
