import ts from 'typescript';

export interface CommentFinding {
  readonly line: number;
  readonly text: string;
}

function scriptKind(path: string): ts.ScriptKind {
  if (path.endsWith('.json')) {
    return ts.ScriptKind.JSON;
  }
  if (path.endsWith('.tsx')) {
    return ts.ScriptKind.TSX;
  }
  if (path.endsWith('.ts') || path.endsWith('.mts') || path.endsWith('.cts')) {
    return ts.ScriptKind.TS;
  }
  return ts.ScriptKind.JSX;
}

function lineOf(source: string, position: number): number {
  return source.slice(0, position).split('\n').length;
}

export function scriptComments(path: string, source: string): CommentFinding[] {
  const file = ts.createSourceFile(path, source, ts.ScriptTarget.Latest, true, scriptKind(path));
  const positions = new Map<number, CommentFinding>();

  function record(ranges: readonly ts.CommentRange[] | undefined): void {
    for (const range of ranges ?? []) {
      positions.set(range.pos, { line: lineOf(source, range.pos), text: source.slice(range.pos, range.end) });
    }
  }

  function visit(node: ts.Node): void {
    record(ts.getLeadingCommentRanges(source, node.getFullStart()));
    record(ts.getTrailingCommentRanges(source, node.getEnd()));
    for (const child of node.getChildren()) {
      visit(child);
    }
  }

  visit(file);
  const findings = [...positions.values()].filter((finding) => !(finding.line === 1 && finding.text.startsWith('#!')));
  return findings.sort((left, right) => left.line - right.line);
}

function withoutQuotedStrings(source: string): string {
  return source.replace(/"(?:[^"\\\n]|\\.)*"|'(?:[^'\\\n]|\\.)*'/g, (match) => ' '.repeat(match.length));
}

export function cssComments(source: string): CommentFinding[] {
  const cleaned = withoutQuotedStrings(source);
  return [...cleaned.matchAll(/\/\*/g)].map((match) => ({
    line: lineOf(source, match.index),
    text: source.slice(match.index, source.indexOf('*/', match.index) + 2),
  }));
}

export function htmlComments(source: string): CommentFinding[] {
  return [...source.matchAll(/<!--/g)].map((match) => ({ line: lineOf(source, match.index), text: '<!--' }));
}
