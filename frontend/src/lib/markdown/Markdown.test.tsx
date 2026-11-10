import { cleanup, render } from '@testing-library/react';
import { afterEach, describe, expect, test } from 'vitest';

import { Markdown } from './Markdown.tsx';
import { maxInlineNesting } from './inline.ts';

afterEach(cleanup);

const allowedElements = new Set(['DIV', 'P', 'STRONG', 'EM', 'CODE', 'PRE', 'UL', 'OL', 'LI', 'A', 'BR']);
const allowedAttributes = new Set(['class', 'dir', 'href', 'target', 'rel', 'start']);

const injectionCorpus: readonly string[] = [
  '<script>alert(1)</script>',
  '<img src=x onerror=alert(1)>',
  '<a href="javascript:alert(1)">click</a>',
  '<iframe src="https://frame.example"></iframe>',
  '<style>body{display:none}</style>',
  '<svg onload=alert(1)><circle /></svg>',
  '<details open ontoggle=alert(1)>x</details>',
  '[click](javascript:alert(1))',
  '[click](JaVaScRiPt:alert(1))',
  '[click](\u0001javascript:alert(1))',
  '[click](&#106;avascript:alert(1))',
  '[click](javascript&colon;alert(1))',
  '[click](data:text/html;base64,PHNjcmlwdD5hbGVydCgxKTwvc2NyaXB0Pg==)',
  '[click](data:text/html,<script>alert(1)</script>)',
  '[click](vbscript:msgbox(1))',
  '[click](file:///etc/passwd)',
  '[click](http://plain.example)',
  '[click](//protocol-relative.example)',
  '[click](https://user:secret@host.example)',
  '[click](https://user@host.example)',
  '[click](https://:secret@host.example)',
  '[click](https://host.example" onmouseover="alert(1))',
  '[click](<javascript:alert(1)>)',
  '![picture](https://images.example/a.png)',
  '<https://autolink.example>',
  '[nested [link](javascript:alert(1))](https://outer.example)',
  '***bold italic*** **bold *italic inside* bold** *a **b** c*',
  '*unbalanced **markers _everywhere* __and** `here',
  '**[link with **emphasis**](https://ok.example)**',
  '```\n<script>alert(1)</script>\n```',
  '- <b>item</b>\n- [x](javascript:alert(1))\n1. <i>first</i>',
  '\\<script\\>alert(1)\\</script\\>',
];

function renderedElements(source: string): Element[] {
  const { container } = render(<Markdown source={source} />);
  return Array.from(container.querySelectorAll('*'));
}

function elementDepth(element: Element, root: Element): number {
  let depth = 0;
  let current: Element | null = element;
  while (current !== null && current !== root) {
    depth += 1;
    current = current.parentElement;
  }
  return depth;
}

describe('restricted Markdown renderer', () => {
  test.each(injectionCorpus)('renders only the subset for %j', (source) => {
    const input = source;

    const elements = renderedElements(input);

    const tagNames = elements.map((element) => element.tagName);
    const attributeNames = elements.flatMap((element) => element.getAttributeNames());
    const links = elements.filter((element) => element.tagName === 'A');
    expect(tagNames.filter((tagName) => !allowedElements.has(tagName))).toEqual([]);
    expect(attributeNames.filter((name) => !allowedAttributes.has(name))).toEqual([]);
    for (const link of links) {
      const url = new URL(link.getAttribute('href') ?? '');
      expect(url.protocol).toBe('https:');
      expect(url.hostname).not.toBe('');
      expect(url.username + url.password).toBe('');
      expect(link.getAttribute('target')).toBe('_blank');
      expect(link.getAttribute('rel')).toBe('noopener noreferrer');
    }
  });

  test('raw HTML and rejected links are shown as text', () => {
    const source = '<b>bold</b> [click](javascript:alert(1)) [data](data:text/html,x)';

    const { container } = render(<Markdown source={source} />);

    expect(container.querySelector('b')).toBeNull();
    expect(container.querySelector('a')).toBeNull();
    expect(container.textContent).toBe(source);
  });

  test('an https link opens in a new tab without an opener or a referrer', () => {
    const source = 'See [the guide](https://docs.example/guide).';

    const { getByRole } = render(<Markdown source={source} />);

    const link = getByRole('link', { name: 'the guide' });
    expect(link.getAttribute('href')).toBe('https://docs.example/guide');
    expect(link.getAttribute('target')).toBe('_blank');
    expect(link.getAttribute('rel')).toBe('noopener noreferrer');
  });

  test('paragraphs and list items isolate their direction', () => {
    const source = 'שלום world\n\n- مرحبا\n- hello';

    const { container } = render(<Markdown source={source} />);

    const isolated = Array.from(container.querySelectorAll('p, li')).map((element) => element.getAttribute('dir'));
    expect(isolated).toEqual(['auto', 'auto', 'auto']);
  });

  test('an ordered list keeps its first number', () => {
    const source = '4. four\n5. five';

    const { container } = render(<Markdown source={source} />);

    expect(container.querySelector('ol')?.getAttribute('start')).toBe('4');
  });

  test('deeply nested emphasis renders within the nesting limit', () => {
    const pairs = 1_666;
    const source = `${'*a '.repeat(pairs)}${' a*'.repeat(pairs)}`;

    const { container } = render(<Markdown source={source} />);

    const root = container.firstElementChild;
    const deepest = Math.max(
      ...Array.from(container.querySelectorAll('em')).map((element) => elementDepth(element, root ?? container)),
    );
    expect(deepest).toBeLessThanOrEqual(maxInlineNesting + 1);
    expect(container.textContent.replaceAll(/[\sa]/gu, '')).toBe('');
  });
});
