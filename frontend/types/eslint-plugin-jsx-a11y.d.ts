declare module 'eslint-plugin-jsx-a11y' {
  import type { ESLint, Linter } from 'eslint';

  const plugin: ESLint.Plugin & {
    readonly flatConfigs: { readonly recommended: Linter.Config; readonly strict: Linter.Config };
  };
  export default plugin;
}
