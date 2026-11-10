import { parseInline } from './inline.ts';
import type { BlockNode, ListItem, StepCounter } from './syntax.ts';

const maxMarkerIndent = 3;
const maxOrderedDigits = 9;
const minFenceLength = 3;

interface Fence {
  readonly character: '`' | '~';
  readonly length: number;
}

type ListMarker =
  | { readonly kind: 'unordered'; readonly character: string; readonly content: string }
  | { readonly kind: 'ordered'; readonly delimiter: string; readonly start: number; readonly content: string };

function splitLines(source: string, counter: StepCounter): string[] {
  counter.steps += source.length;
  return source.replaceAll('\r\n', '\n').replaceAll('\r', '\n').split('\n');
}

function isBlank(line: string, counter: StepCounter): boolean {
  counter.steps += line.length;
  return line.trim() === '';
}

function markerIndent(line: string, counter: StepCounter): number | null {
  let indent = 0;
  while (indent < line.length && line[indent] === ' ') {
    counter.steps += 1;
    indent += 1;
    if (indent > maxMarkerIndent) {
      return null;
    }
  }
  return indent;
}

function openingFence(line: string, counter: StepCounter): Fence | null {
  const indent = markerIndent(line, counter);
  if (indent === null) {
    return null;
  }
  const character = line[indent];
  if (character !== '`' && character !== '~') {
    return null;
  }
  let end = indent;
  while (line[end] === character) {
    counter.steps += 1;
    end += 1;
  }
  const length = end - indent;
  return length >= minFenceLength ? { character, length } : null;
}

function closesFence(line: string, fence: Fence, counter: StepCounter): boolean {
  const opening = openingFence(line, counter);
  if (opening?.character !== fence.character || opening.length < fence.length) {
    return false;
  }
  return isBlank(line.replaceAll(fence.character, ''), counter);
}

function itemContent(line: string, afterMarker: number, counter: StepCounter): string | null {
  const next = line[afterMarker];
  if (next !== undefined && next !== ' ' && next !== '\t') {
    return null;
  }
  counter.steps += line.length - afterMarker;
  return line.slice(afterMarker).trim();
}

function listMarker(line: string, counter: StepCounter): ListMarker | null {
  const indent = markerIndent(line, counter);
  if (indent === null) {
    return null;
  }
  const first = line[indent];
  if (first === '-' || first === '*' || first === '+') {
    const content = itemContent(line, indent + 1, counter);
    return content === null ? null : { kind: 'unordered', character: first, content };
  }
  let end = indent;
  while (end < line.length && end - indent < maxOrderedDigits && /\d/u.test(line[end] ?? '')) {
    counter.steps += 1;
    end += 1;
  }
  const delimiter = line[end];
  if (end === indent || (delimiter !== '.' && delimiter !== ')')) {
    return null;
  }
  const content = itemContent(line, end + 1, counter);
  return content === null
    ? null
    : { kind: 'ordered', delimiter, start: Number.parseInt(line.slice(indent, end), 10), content };
}

function sameList(marker: ListMarker | null, list: ListMarker): boolean {
  if (marker === null) {
    return false;
  }
  if (marker.kind === 'unordered' && list.kind === 'unordered') {
    return marker.character === list.character;
  }
  if (marker.kind === 'ordered' && list.kind === 'ordered') {
    return marker.delimiter === list.delimiter;
  }
  return false;
}

function interruptsParagraph(marker: ListMarker | null): boolean {
  return marker !== null && marker.content !== '' && (marker.kind === 'unordered' || marker.start === 1);
}

class BlockParser {
  private index = 0;
  private readonly blocks: BlockNode[] = [];

  private readonly lines: readonly string[];
  private readonly counter: StepCounter;

  constructor(lines: readonly string[], counter: StepCounter) {
    this.lines = lines;
    this.counter = counter;
  }

  parse(): BlockNode[] {
    while (this.index < this.lines.length) {
      this.counter.steps += 1;
      const line = this.lines[this.index] ?? '';
      if (isBlank(line, this.counter)) {
        this.index += 1;
        continue;
      }
      const fence = openingFence(line, this.counter);
      if (fence !== null) {
        this.codeBlock(fence);
        continue;
      }
      const marker = listMarker(line, this.counter);
      if (marker === null) {
        this.paragraph();
      } else {
        this.list(marker);
      }
    }
    return this.blocks;
  }

  private codeBlock(fence: Fence): void {
    const content: string[] = [];
    this.index += 1;
    while (this.index < this.lines.length) {
      this.counter.steps += 1;
      const line = this.lines[this.index] ?? '';
      this.index += 1;
      if (closesFence(line, fence, this.counter)) {
        break;
      }
      content.push(line);
    }
    this.counter.steps += content.length;
    this.blocks.push({ kind: 'code_block', text: content.join('\n') });
  }

  private paragraph(): void {
    const content: string[] = [];
    while (this.index < this.lines.length) {
      this.counter.steps += 1;
      const line = this.lines[this.index] ?? '';
      if (content.length > 0 && this.endsParagraph(line)) {
        break;
      }
      this.counter.steps += line.length;
      content.push(line.trim());
      this.index += 1;
    }
    this.blocks.push({ kind: 'paragraph', content: parseInline(content.join('\n'), this.counter) });
  }

  private endsParagraph(line: string): boolean {
    return (
      isBlank(line, this.counter) ||
      openingFence(line, this.counter) !== null ||
      interruptsParagraph(listMarker(line, this.counter))
    );
  }

  private nextNonBlank(): number {
    let next = this.index;
    while (next < this.lines.length && isBlank(this.lines[next] ?? '', this.counter)) {
      this.counter.steps += 1;
      next += 1;
    }
    return next;
  }

  private list(first: ListMarker): void {
    const items: string[][] = [];
    let current = [first.content];
    this.index += 1;
    while (this.index < this.lines.length) {
      this.counter.steps += 1;
      const line = this.lines[this.index] ?? '';
      if (isBlank(line, this.counter)) {
        const next = this.nextNonBlank();
        if (!sameList(listMarker(this.lines[next] ?? '', this.counter), first)) {
          break;
        }
        this.index = next;
        continue;
      }
      if (openingFence(line, this.counter) !== null) {
        break;
      }
      const marker = listMarker(line, this.counter);
      if (marker !== null) {
        if (!sameList(marker, first)) {
          break;
        }
        items.push(current);
        current = [marker.content];
      } else {
        this.counter.steps += line.length;
        current.push(line.trim());
      }
      this.index += 1;
    }
    items.push(current);
    const parsed: ListItem[] = items.map((item) => parseInline(item.join('\n'), this.counter));
    this.blocks.push(
      first.kind === 'ordered'
        ? { kind: 'ordered_list', start: first.start, items: parsed }
        : { kind: 'unordered_list', items: parsed },
    );
  }
}

export function parseMarkdownCounted(source: string): { readonly blocks: BlockNode[]; readonly steps: number } {
  const counter: StepCounter = { steps: 0 };
  const blocks = new BlockParser(splitLines(source, counter), counter).parse();
  return { blocks, steps: counter.steps };
}

export function parseMarkdown(source: string): BlockNode[] {
  return parseMarkdownCounted(source).blocks;
}
