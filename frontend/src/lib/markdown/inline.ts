import { safeLinkTarget } from './links.ts';
import type { InlineNode, StepCounter } from './syntax.ts';

export const maxInlineNesting = 8;

const backslashCode = 0x5c;
const backtickCode = 0x60;
const plainRole = 0;
const removedBackslashRole = 1;
const escapedCharacterRole = 2;

const whitespacePattern = /^\s$/u;
const punctuationPattern = /^[\p{P}\p{S}]$/u;

type EmphasisCharacter = '*' | '_';
type MarkerKind = 'open_bracket' | 'close_bracket' | 'open_paren' | 'close_paren';

const markerText: Readonly<Record<MarkerKind, string>> = {
  open_bracket: '[',
  close_bracket: ']',
  open_paren: '(',
  close_paren: ')',
};

interface BacktickRun {
  readonly start: number;
  readonly length: number;
}

interface CodeSpan {
  readonly start: number;
  readonly end: number;
  readonly text: string;
}

interface MarkerToken {
  readonly kind: MarkerKind;
  readonly position: number;
}

interface DelimiterToken {
  readonly kind: 'delimiter';
  readonly character: EmphasisCharacter;
  readonly length: number;
  readonly canOpen: boolean;
  readonly canClose: boolean;
}

type Token =
  | { readonly kind: 'text'; readonly text: string }
  | { readonly kind: 'code'; readonly text: string }
  | { readonly kind: 'break' }
  | MarkerToken
  | DelimiterToken;

interface LinkRange {
  readonly open: number;
  readonly close: number;
  readonly end: number;
  readonly href: string;
}

interface DelimiterRun {
  readonly character: EmphasisCharacter;
  readonly original: number;
  readonly canOpen: boolean;
  readonly canClose: boolean;
  readonly ordinal: number;
  remaining: number;
  previous: DelimiterRun | null;
  next: DelimiterRun | null;
  cell: Cell;
}

type Piece =
  { readonly kind: 'node'; readonly node: InlineNode } | { readonly kind: 'delimiter'; readonly run: DelimiterRun };

interface Cell {
  piece: Piece;
  previous: Cell | null;
  next: Cell | null;
}

function isAsciiPunctuation(code: number): boolean {
  return (
    (code >= 0x21 && code <= 0x2f) ||
    (code >= 0x3a && code <= 0x40) ||
    (code >= 0x5b && code <= 0x60) ||
    (code >= 0x7b && code <= 0x7e)
  );
}

function isWhitespace(character: string | undefined): boolean {
  return character === undefined || whitespacePattern.test(character);
}

function isPunctuation(character: string | undefined): boolean {
  return character !== undefined && punctuationPattern.test(character);
}

function isMarkerToken(token: Token | undefined): token is MarkerToken {
  return (
    token !== undefined &&
    (token.kind === 'open_bracket' ||
      token.kind === 'close_bracket' ||
      token.kind === 'open_paren' ||
      token.kind === 'close_paren')
  );
}

function scanEscapesAndBackticks(
  source: string,
  counter: StepCounter,
): { readonly roles: Uint8Array; readonly runs: BacktickRun[] } {
  const roles = new Uint8Array(source.length);
  const runs: BacktickRun[] = [];
  let index = 0;
  while (index < source.length) {
    counter.steps += 1;
    const code = source.charCodeAt(index);
    if (code === backslashCode && index + 1 < source.length && isAsciiPunctuation(source.charCodeAt(index + 1))) {
      roles[index] = removedBackslashRole;
      roles[index + 1] = escapedCharacterRole;
      index += 2;
    } else if (code === backtickCode) {
      let end = index;
      while (end < source.length && source.charCodeAt(end) === backtickCode) {
        counter.steps += 1;
        end += 1;
      }
      runs.push({ start: index, length: end - index });
      index = end;
    } else {
      index += 1;
    }
  }
  return { roles, runs };
}

function codeSpanText(raw: string, counter: StepCounter): string {
  counter.steps += raw.length;
  const flattened = raw.replaceAll('\n', ' ');
  if (flattened.length >= 2 && flattened.startsWith(' ') && flattened.endsWith(' ') && flattened.trim() !== '') {
    return flattened.slice(1, -1);
  }
  return flattened;
}

function matchCodeSpans(source: string, runs: readonly BacktickRun[], counter: StepCounter): CodeSpan[] {
  const runsByLength = new Map<number, number[]>();
  runs.forEach((run, index) => {
    counter.steps += 1;
    const sameLength = runsByLength.get(run.length);
    if (sameLength === undefined) {
      runsByLength.set(run.length, [index]);
    } else {
      sameLength.push(index);
    }
  });
  const cursors = new Map<number, number>();
  const spans: CodeSpan[] = [];
  let coveredUntil = 0;
  runs.forEach((run, index) => {
    counter.steps += 1;
    if (run.start < coveredUntil) {
      return;
    }
    const sameLength = runsByLength.get(run.length) ?? [];
    let cursor = cursors.get(run.length) ?? 0;
    while (cursor < sameLength.length && (sameLength[cursor] ?? index) <= index) {
      counter.steps += 1;
      cursor += 1;
    }
    cursors.set(run.length, cursor);
    const closerIndex = sameLength[cursor];
    const closer = closerIndex === undefined ? undefined : runs[closerIndex];
    if (closer === undefined) {
      return;
    }
    const end = closer.start + closer.length;
    spans.push({
      start: run.start,
      end,
      text: codeSpanText(source.slice(run.start + run.length, closer.start), counter),
    });
    coveredUntil = end;
  });
  return spans;
}

function delimiterToken(source: string, start: number, end: number, character: EmphasisCharacter): DelimiterToken {
  const before = source[start - 1];
  const after = source[end];
  const spaceBefore = isWhitespace(before);
  const spaceAfter = isWhitespace(after);
  const punctuationBefore = isPunctuation(before);
  const punctuationAfter = isPunctuation(after);
  const leftFlanking = !spaceAfter && (!punctuationAfter || spaceBefore || punctuationBefore);
  const rightFlanking = !spaceBefore && (!punctuationBefore || spaceAfter || punctuationAfter);
  if (character === '*') {
    return { kind: 'delimiter', character, length: end - start, canOpen: leftFlanking, canClose: rightFlanking };
  }
  return {
    kind: 'delimiter',
    character,
    length: end - start,
    canOpen: leftFlanking && (!rightFlanking || punctuationBefore),
    canClose: rightFlanking && (!leftFlanking || punctuationAfter),
  };
}

function markerKind(character: string | undefined): MarkerKind | null {
  switch (character) {
    case '[':
      return 'open_bracket';
    case ']':
      return 'close_bracket';
    case '(':
      return 'open_paren';
    case ')':
      return 'close_paren';
    default:
      return null;
  }
}

function tokenize(source: string, roles: Uint8Array, spans: readonly CodeSpan[], counter: StepCounter): Token[] {
  const tokens: Token[] = [];
  let textStart = 0;
  let index = 0;
  let spanIndex = 0;
  const flushText = (end: number): void => {
    if (end > textStart) {
      counter.steps += end - textStart;
      tokens.push({ kind: 'text', text: source.slice(textStart, end) });
    }
  };
  while (index < source.length) {
    counter.steps += 1;
    const role = roles[index];
    if (role === removedBackslashRole) {
      flushText(index);
      index += 1;
      textStart = index;
      continue;
    }
    if (role === escapedCharacterRole) {
      index += 1;
      continue;
    }
    const span = spans[spanIndex];
    if (span?.start === index) {
      flushText(index);
      tokens.push({ kind: 'code', text: span.text });
      index = span.end;
      textStart = index;
      spanIndex += 1;
      continue;
    }
    const character = source[index];
    const marker = markerKind(character);
    if (character === '\n') {
      flushText(index);
      tokens.push({ kind: 'break' });
      index += 1;
      textStart = index;
    } else if (marker !== null) {
      flushText(index);
      tokens.push({ kind: marker, position: index });
      index += 1;
      textStart = index;
    } else if (character === '*' || character === '_') {
      let end = index;
      while (end < source.length && source[end] === character && roles[end] === plainRole) {
        counter.steps += 1;
        end += 1;
      }
      flushText(index);
      tokens.push(delimiterToken(source, index, end, character));
      index = end;
      textStart = index;
    } else {
      index += 1;
    }
  }
  flushText(source.length);
  return tokens;
}

function literalToken(token: Token): Token {
  if (token.kind === 'delimiter') {
    return { kind: 'text', text: token.character.repeat(token.length) };
  }
  if (isMarkerToken(token)) {
    return { kind: 'text', text: markerText[token.kind] };
  }
  return token;
}

function whitespacePrefix(source: string, counter: StepCounter): Int32Array {
  const prefix = new Int32Array(source.length + 1);
  for (let index = 0; index < source.length; index += 1) {
    counter.steps += 1;
    prefix[index + 1] = (prefix[index] ?? 0) + (isWhitespace(source[index]) ? 1 : 0);
  }
  return prefix;
}

function unescapedSlice(source: string, roles: Uint8Array, from: number, to: number, counter: StepCounter): string {
  const parts: string[] = [];
  let partStart = from;
  for (let index = from; index < to; index += 1) {
    counter.steps += 1;
    if (roles[index] === removedBackslashRole) {
      parts.push(source.slice(partStart, index));
      partStart = index + 1;
    }
  }
  parts.push(source.slice(partStart, to));
  return parts.join('');
}

function matchParens(tokens: readonly Token[], counter: StepCounter): Int32Array {
  const matches = new Int32Array(tokens.length).fill(-1);
  const open: number[] = [];
  tokens.forEach((token, index) => {
    counter.steps += 1;
    if (token.kind === 'open_paren') {
      open.push(index);
    } else if (token.kind === 'close_paren') {
      const opener = open.pop();
      if (opener !== undefined) {
        matches[opener] = index;
      }
    }
  });
  return matches;
}

function resolveLinks(source: string, roles: Uint8Array, tokens: Token[], counter: StepCounter): LinkRange[] {
  const spaces = whitespacePrefix(source, counter);
  const parenMatches = matchParens(tokens, counter);
  const links: LinkRange[] = [];
  const openers: number[] = [];
  let index = 0;
  while (index < tokens.length) {
    counter.steps += 1;
    const token = tokens[index];
    if (token?.kind === 'open_bracket') {
      openers.push(index);
      index += 1;
      continue;
    }
    if (token?.kind !== 'close_bracket') {
      index += 1;
      continue;
    }
    const opener = openers.pop();
    const destinationOpen = tokens[index + 1];
    const destinationEnd = parenMatches[index + 1] ?? -1;
    const destinationClose = tokens[destinationEnd];
    if (
      opener === undefined ||
      destinationOpen?.kind !== 'open_paren' ||
      destinationOpen.position !== token.position + 1 ||
      destinationClose?.kind !== 'close_paren'
    ) {
      index += 1;
      continue;
    }
    const from = destinationOpen.position + 1;
    const to = destinationClose.position;
    if (to === from || (spaces[to] ?? 0) !== (spaces[from] ?? 0)) {
      index += 1;
      continue;
    }
    const href = safeLinkTarget(unescapedSlice(source, roles, from, to, counter));
    if (href === null) {
      for (let literal = index + 1; literal <= destinationEnd; literal += 1) {
        counter.steps += 1;
        const original = tokens[literal];
        if (original !== undefined) {
          tokens[literal] = literalToken(original);
        }
      }
    } else {
      links.push({ open: opener, close: index, end: destinationEnd, href });
      openers.length = 0;
    }
    index = destinationEnd + 1;
  }
  return links;
}

function appendNode(nodes: InlineNode[], node: InlineNode, counter: StepCounter): void {
  const last = nodes.at(-1);
  if (node.kind === 'text' && last?.kind === 'text') {
    counter.steps += node.text.length;
    nodes[nodes.length - 1] = { kind: 'text', text: last.text + node.text };
    return;
  }
  nodes.push(node);
}

function pieceNode(piece: Piece): InlineNode {
  if (piece.kind === 'node') {
    return piece.node;
  }
  return { kind: 'text', text: piece.run.character.repeat(piece.run.remaining) };
}

function collectNodes(first: Cell | null, stop: Cell | null, counter: StepCounter): InlineNode[] {
  const nodes: InlineNode[] = [];
  let cell = first;
  while (cell !== null && cell !== stop) {
    counter.steps += 1;
    appendNode(nodes, pieceNode(cell.piece), counter);
    cell = cell.next;
  }
  return nodes;
}

function unlinkDelimiter(run: DelimiterRun): void {
  if (run.previous !== null) {
    run.previous.next = run.next;
  }
  if (run.next !== null) {
    run.next.previous = run.previous;
  }
  run.previous = null;
  run.next = null;
}

function removeCell(cell: Cell): void {
  if (cell.previous !== null) {
    cell.previous.next = cell.next;
  }
  if (cell.next !== null) {
    cell.next.previous = cell.previous;
  }
}

function delimiterKey(run: DelimiterRun): string {
  return `${run.character}${String(run.original % 3)}${run.canOpen ? 'o' : 'c'}`;
}

function oddMatch(opener: DelimiterRun, closer: DelimiterRun): boolean {
  return (
    (closer.canOpen || opener.canClose) &&
    (opener.original + closer.original) % 3 === 0 &&
    !(opener.original % 3 === 0 && closer.original % 3 === 0)
  );
}

function findOpener(closer: DelimiterRun, bottom: number, counter: StepCounter): DelimiterRun | null {
  let opener = closer.previous;
  while (opener !== null && opener.ordinal > bottom) {
    counter.steps += 1;
    if (opener.character === closer.character && opener.canOpen && !oddMatch(opener, closer)) {
      return opener;
    }
    opener = opener.previous;
  }
  return null;
}

function wrapBetween(opener: DelimiterRun, closer: DelimiterRun, used: number, counter: StepCounter): void {
  let between = opener.next;
  while (between !== null && between !== closer) {
    counter.steps += 1;
    const next = between.next;
    unlinkDelimiter(between);
    between = next;
  }
  opener.next = closer;
  closer.previous = opener;
  const children = collectNodes(opener.cell.next, closer.cell, counter);
  const wrapper: Cell = {
    piece: { kind: 'node', node: used === 2 ? { kind: 'strong', children } : { kind: 'emphasis', children } },
    previous: opener.cell,
    next: closer.cell,
  };
  opener.cell.next = wrapper;
  closer.cell.previous = wrapper;
}

function buildCells(pieces: readonly Piece[], counter: StepCounter): { head: Cell; firstRun: DelimiterRun | null } {
  const head: Cell = { piece: { kind: 'node', node: { kind: 'text', text: '' } }, previous: null, next: null };
  let tail = head;
  let firstRun: DelimiterRun | null = null;
  let lastRun: DelimiterRun | null = null;
  for (const piece of pieces) {
    counter.steps += 1;
    const cell: Cell = { piece, previous: tail, next: null };
    tail.next = cell;
    tail = cell;
    if (piece.kind === 'delimiter') {
      piece.run.cell = cell;
      piece.run.previous = lastRun;
      if (lastRun === null) {
        firstRun = piece.run;
      } else {
        lastRun.next = piece.run;
      }
      lastRun = piece.run;
    }
  }
  return { head, firstRun };
}

function emphasize(pieces: readonly Piece[], counter: StepCounter): InlineNode[] {
  const { head, firstRun } = buildCells(pieces, counter);
  const bottoms = new Map<string, number>();
  let closer = firstRun;
  while (closer !== null) {
    counter.steps += 1;
    if (!closer.canClose) {
      closer = closer.next;
      continue;
    }
    const key = delimiterKey(closer);
    const opener = findOpener(closer, bottoms.get(key) ?? -1, counter);
    if (opener === null) {
      bottoms.set(key, closer.ordinal - 1);
      const next = closer.next;
      if (!closer.canOpen) {
        unlinkDelimiter(closer);
      }
      closer = next;
      continue;
    }
    const used = closer.remaining >= 2 && opener.remaining >= 2 ? 2 : 1;
    opener.remaining -= used;
    closer.remaining -= used;
    wrapBetween(opener, closer, used, counter);
    if (opener.remaining === 0) {
      removeCell(opener.cell);
      unlinkDelimiter(opener);
    }
    if (closer.remaining === 0) {
      const next = closer.next;
      removeCell(closer.cell);
      unlinkDelimiter(closer);
      closer = next;
    }
  }
  return collectNodes(head.next, null, counter);
}

function delimiterPiece(token: DelimiterToken, ordinal: number, placeholder: Cell): Piece {
  return {
    kind: 'delimiter',
    run: {
      character: token.character,
      original: token.length,
      canOpen: token.canOpen,
      canClose: token.canClose,
      ordinal,
      remaining: token.length,
      previous: null,
      next: null,
      cell: placeholder,
    },
  };
}

function tokenPiece(token: Token, ordinal: number, placeholder: Cell): Piece {
  if (token.kind === 'delimiter') {
    return delimiterPiece(token, ordinal, placeholder);
  }
  if (isMarkerToken(token)) {
    return { kind: 'node', node: { kind: 'text', text: markerText[token.kind] } };
  }
  return { kind: 'node', node: token };
}

function buildInline(tokens: readonly Token[], links: readonly LinkRange[], counter: StepCounter): InlineNode[] {
  const placeholder: Cell = { piece: { kind: 'node', node: { kind: 'text', text: '' } }, previous: null, next: null };
  const pieces: Piece[] = [];
  let linkIndex = 0;
  let index = 0;
  while (index < tokens.length) {
    counter.steps += 1;
    const link = links[linkIndex];
    if (link?.open === index) {
      const linkPieces = tokens
        .slice(link.open + 1, link.close)
        .map((token, offset) => tokenPiece(token, link.open + 1 + offset, placeholder));
      counter.steps += linkPieces.length;
      pieces.push({ kind: 'node', node: { kind: 'link', href: link.href, children: emphasize(linkPieces, counter) } });
      index = link.end + 1;
      linkIndex += 1;
      continue;
    }
    const token = tokens[index];
    if (token !== undefined) {
      pieces.push(tokenPiece(token, index, placeholder));
    }
    index += 1;
  }
  return emphasize(pieces, counter);
}

function flattenInto(nodes: readonly InlineNode[], output: InlineNode[], counter: StepCounter): void {
  const pending: InlineNode[] = [...nodes].reverse();
  let node = pending.pop();
  while (node !== undefined) {
    counter.steps += 1;
    if (node.kind === 'emphasis' || node.kind === 'strong' || node.kind === 'link') {
      for (let child = node.children.length - 1; child >= 0; child -= 1) {
        const childNode = node.children[child];
        if (childNode !== undefined) {
          pending.push(childNode);
        }
      }
    } else {
      appendNode(output, node, counter);
    }
    node = pending.pop();
  }
}

function limitNesting(nodes: readonly InlineNode[], depth: number, counter: StepCounter): InlineNode[] {
  if (depth >= maxInlineNesting) {
    const flat: InlineNode[] = [];
    flattenInto(nodes, flat, counter);
    return flat;
  }
  const limited: InlineNode[] = [];
  for (const node of nodes) {
    counter.steps += 1;
    if (node.kind === 'emphasis' || node.kind === 'strong' || node.kind === 'link') {
      appendNode(limited, { ...node, children: limitNesting(node.children, depth + 1, counter) }, counter);
    } else {
      appendNode(limited, node, counter);
    }
  }
  return limited;
}

export function parseInline(source: string, counter: StepCounter): InlineNode[] {
  const { roles, runs } = scanEscapesAndBackticks(source, counter);
  const spans = matchCodeSpans(source, runs, counter);
  const tokens = tokenize(source, roles, spans, counter);
  const links = resolveLinks(source, roles, tokens, counter);
  return limitNesting(buildInline(tokens, links, counter), 0, counter);
}
