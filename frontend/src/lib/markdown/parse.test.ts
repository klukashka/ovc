import { describe, expect, test } from 'vitest';

import { maxInlineNesting } from './inline.ts';
import { parseMarkdown, parseMarkdownCounted } from './parse.ts';
import type { InlineNode } from './syntax.ts';

const adversarialLength = 10_000;
const stepsPerCharacter = 32;

function filled(unit: string): string {
  return unit.repeat(Math.ceil(adversarialLength / unit.length)).slice(0, adversarialLength);
}

function nested(open: string, close: string): string {
  const pairs = Math.floor(adversarialLength / (open.length + close.length));
  return (open.repeat(pairs) + close.repeat(pairs)).padEnd(adversarialLength, 'x');
}

function growingBacktickRuns(): string {
  let text = '';
  for (let length = 1; text.length < adversarialLength; length += 1) {
    text += `${'`'.repeat(length)}a`;
  }
  return text.slice(0, adversarialLength);
}

const adversarialInputs: Readonly<Record<string, string>> = {
  'unclosed brackets': filled('['),
  'unopened brackets': filled(']'),
  'unclosed parentheses': filled('('),
  'unclosed link destinations': filled('[a]('),
  'nested invalid https destinations': nested('[a](https:', ')'),
  'nested javascript destinations': nested('[a](javascript:', ')'),
  'bracket pile before a link': filled('[[[[a](https://x.example)'),
  'valid links': filled('[a](https://x.example/a) '),
  'a single asterisk run': filled('*'),
  'unmatched asterisk openers': filled('*a '),
  'unmatched underscore openers': filled('_a '),
  'alternating delimiters': filled('*_'),
  'deeply nested emphasis': nested('*a ', ' a*'),
  'deeply nested strong emphasis': nested('**a ', ' a**'),
  'runs whose lengths defeat matching': filled('***a* '),
  'unbalanced markers before brackets': filled('**a [b'),
  'a single backtick run': filled('`'),
  'backtick runs of growing length': growingBacktickRuns(),
  backslashes: filled('\\'),
  'escaped asterisks': filled('\\*'),
  'raw HTML': filled('<script>alert(1)</script>'),
  'short lines': filled('a\n'),
  'blank lines': filled('\n'),
  'list items': filled('- a\n'),
  'separated ordered items': filled('1. a\n\n'),
  'indented markers': filled('   - '),
  'long ordered numbers': filled('123456789. a\n'),
  'unclosed fences': filled('```\n'),
  'tilde fences with info': filled('~~~ a\n'),
};

function inlineOf(source: string): readonly InlineNode[] {
  const [block] = parseMarkdown(source);
  if (block?.kind !== 'paragraph') {
    throw new Error(`expected one paragraph for ${source}`);
  }
  return block.content;
}

function nestingDepth(nodes: readonly InlineNode[]): number {
  return Math.max(
    0,
    ...nodes.map((node) =>
      node.kind === 'emphasis' || node.kind === 'strong' || node.kind === 'link' ? 1 + nestingDepth(node.children) : 0,
    ),
  );
}

describe('restricted Markdown parser steps', () => {
  test.each(Object.entries(adversarialInputs))(
    '%s stay within a fixed multiple of the input length',
    (_name, source) => {
      const input = source;

      const { steps } = parseMarkdownCounted(input);

      expect(input).toHaveLength(adversarialLength);
      expect(steps).toBeGreaterThanOrEqual(input.length);
      expect(steps).toBeLessThanOrEqual(stepsPerCharacter * input.length);
    },
  );
});

describe('restricted Markdown parser', () => {
  test('inline formatting becomes strong, emphasis, code and https links', () => {
    const source = 'Read **this** and *that*, run `go test` and see [the guide](https://docs.example/a_(b)).';

    const content = inlineOf(source);

    expect(content).toEqual([
      { kind: 'text', text: 'Read ' },
      { kind: 'strong', children: [{ kind: 'text', text: 'this' }] },
      { kind: 'text', text: ' and ' },
      { kind: 'emphasis', children: [{ kind: 'text', text: 'that' }] },
      { kind: 'text', text: ', run ' },
      { kind: 'code', text: 'go test' },
      { kind: 'text', text: ' and see ' },
      { kind: 'link', href: 'https://docs.example/a_(b)', children: [{ kind: 'text', text: 'the guide' }] },
      { kind: 'text', text: '.' },
    ]);
  });

  test('blocks split into paragraphs, lists and code blocks', () => {
    const source = 'First line\nsecond line\n\n- one\n- two\n\n3. three\n4. four\n\n```js\n<b>*raw*</b>\n```\nAfter';

    const blocks = parseMarkdown(source);

    expect(blocks).toEqual([
      {
        kind: 'paragraph',
        content: [{ kind: 'text', text: 'First line' }, { kind: 'break' }, { kind: 'text', text: 'second line' }],
      },
      { kind: 'unordered_list', items: [[{ kind: 'text', text: 'one' }], [{ kind: 'text', text: 'two' }]] },
      { kind: 'ordered_list', start: 3, items: [[{ kind: 'text', text: 'three' }], [{ kind: 'text', text: 'four' }]] },
      { kind: 'code_block', text: '<b>*raw*</b>' },
      { kind: 'paragraph', content: [{ kind: 'text', text: 'After' }] },
    ]);
  });

  test('links outside https, with user information or without a host stay text', () => {
    const sources = [
      '[a](javascript:alert(1))',
      '[a](JAVASCRIPT:alert(1))',
      '[a](data:text/html,<script>alert(1)</script>)',
      '[a](http://plain.example)',
      '[a](https://user:secret@host.example)',
      '[a](https://user@host.example)',
      '[a](https://:secret@host.example)',
      '[a](https://)',
      '[a](//host.example)',
      '[a](vbscript:msgbox)',
      '[a](https://host.example "title")',
    ];

    const contents = sources.map(inlineOf);

    expect(contents).toEqual(sources.map((source) => [{ kind: 'text', text: source }]));
  });

  test('escapes, unbalanced markers and intraword underscores stay literal', () => {
    const source = '\\*not\\* **open _a snake_case_name `tick';

    const content = inlineOf(source);

    expect(content).toEqual([{ kind: 'text', text: '*not* **open _a snake_case_name `tick' }]);
  });

  test('a link inside a link keeps only the inner link', () => {
    const source = '[outer [inner](https://inner.example) text](https://outer.example)';

    const content = inlineOf(source);

    expect(content).toEqual([
      { kind: 'text', text: '[outer ' },
      { kind: 'link', href: 'https://inner.example/', children: [{ kind: 'text', text: 'inner' }] },
      { kind: 'text', text: ' text](https://outer.example)' },
    ]);
  });

  test('nesting deeper than the limit is flattened into text', () => {
    const source = nested('*a ', ' a*');

    const content = inlineOf(source);

    expect(nestingDepth(content)).toBe(maxInlineNesting);
  });

  test('a link beyond the nesting limit renders as its text', () => {
    const source = `${'*a '.repeat(maxInlineNesting)}[link](https://x.example)${' a*'.repeat(maxInlineNesting)}`;

    const content = inlineOf(source);

    expect(nestingDepth(content)).toBe(maxInlineNesting);
    expect(JSON.stringify(content)).not.toContain('"link"');
  });

  test('indented markers continue a list only with the same bullet and up to three spaces', () => {
    const source = '- a\n   - b\n    - c\n  * d';

    const blocks = parseMarkdown(source);

    expect(blocks).toEqual([
      {
        kind: 'unordered_list',
        items: [
          [{ kind: 'text', text: 'a' }],
          [{ kind: 'text', text: 'b' }, { kind: 'break' }, { kind: 'text', text: '- c' }],
        ],
      },
      { kind: 'unordered_list', items: [[{ kind: 'text', text: 'd' }]] },
    ]);
  });

  test('tilde fences close only on a fence of the same character and at least the same length', () => {
    const source = '~~~~ info\n```\n~~~\n~~~~~\nafter';

    const blocks = parseMarkdown(source);

    expect(blocks).toEqual([
      { kind: 'code_block', text: '```\n~~~' },
      { kind: 'paragraph', content: [{ kind: 'text', text: 'after' }] },
    ]);
  });
});
