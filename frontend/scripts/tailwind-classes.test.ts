import ts from 'typescript';
import { describe, expect, test } from 'vitest';

import { classUses, hasSelector, missingClasses } from './tailwind-classes.ts';

const fileName = '/fixture/Component.tsx';
const sharedName = '/fixture/shared.ts';

function compile(source: string, shared = ''): ts.Program {
  const files = new Map([
    [fileName, source],
    [sharedName, shared],
  ]);
  const options: ts.CompilerOptions = {
    jsx: ts.JsxEmit.Preserve,
    noLib: true,
    target: ts.ScriptTarget.ES2022,
    module: ts.ModuleKind.ESNext,
    moduleResolution: ts.ModuleResolutionKind.Bundler,
  };
  const host = ts.createCompilerHost(options);
  const original = host.getSourceFile.bind(host);
  host.getSourceFile = (
    name: string,
    version: ts.ScriptTarget | ts.CreateSourceFileOptions,
  ): ts.SourceFile | undefined => {
    const text = files.get(name);
    return text === undefined
      ? original(name, version)
      : ts.createSourceFile(name, text, version, true, ts.ScriptKind.TSX);
  };
  host.fileExists = (name: string): boolean => files.has(name);
  host.readFile = (name: string): string | undefined => files.get(name);
  host.directoryExists = (name: string): boolean => name === '/fixture' || name === '/';
  return ts.createProgram([fileName, sharedName], options, host);
}

function tokens(source: string, shared = ''): string[] {
  return classUses(compile(source, shared), [fileName]).map((use) => use.token);
}

describe('Tailwind class check', () => {
  test('collects literal, template, conditional and constant class names', () => {
    const source = [
      "const button = 'rounded-sm px-2';",
      "const tone = { ok: 'text-ok', bad: 'text-bad' };",
      "const picked = 'not-a-class';",
      "const strong = 'font-bold';",
      'const weights = { strong };',
      'export function Component(props: { on: boolean; state: "ok" | "bad" }) {',
      '  return (',
      '    <div className="flex gap-2">',
      '      <span className={`items-center ${tone[picked]} pl-${props.on ? 1 : 2} end`} />',
      "      <b className={props.on ? 'font-medium' : 'font-normal'} />",
      '      <i className={button} />',
      '      <em className={weights[picked]} />',
      '    </div>',
      '  );',
      '}',
    ].join('\n');

    const found = tokens(source);

    expect(found).toEqual(
      expect.arrayContaining([
        'flex',
        'gap-2',
        'items-center',
        'end',
        'text-ok',
        'text-bad',
        'font-medium',
        'font-normal',
        'rounded-sm',
        'px-2',
        'font-bold',
      ]),
    );
    expect(found).not.toContain('pl-');
    expect(found).not.toContain('not-a-class');
  });

  test('follows constants imported from another module and skips call arguments', () => {
    const shared = "export const card = 'rounded-sm shadow-card';";
    const source = [
      "import { card } from './shared';",
      'declare function label(key: string): string;',
      'export function Component() {',
      "  return <div className={card}><span className={label('page.title')} /></div>;",
      '}',
    ].join('\n');

    const found = tokens(source, shared);

    expect(found).toEqual(['rounded-sm', 'shadow-card']);
  });

  test('matches Tailwind escaping and refuses prefixes of longer classes', () => {
    const css =
      '.flex-col{display:flex}.desktop\\:pointer-fine\\:min-h-10{min-height:2.5rem}.p-0\\.5{padding:2px}.pl-\\(--x\\){padding:1px}.\\32 xl\\:p-1{padding:3px}';

    const found = ['desktop:pointer-fine:min-h-10', 'p-0.5', 'pl-(--x)', '2xl:p-1', 'flex'].map((token) =>
      hasSelector(token, css),
    );

    expect(found).toEqual([true, true, true, true, false]);
  });

  test('reports each class with no CSS once per file', () => {
    const uses = [
      { token: 'flex', file: 'a.tsx' },
      { token: 'no-such-class', file: 'a.tsx' },
      { token: 'no-such-class', file: 'a.tsx' },
    ];

    const missing = missingClasses(uses, '.flex{display:flex}');

    expect(missing).toEqual([{ token: 'no-such-class', file: 'a.tsx' }]);
  });
});
