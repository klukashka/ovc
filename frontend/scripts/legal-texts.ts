import { posix } from 'node:path';

import {
  type LegalDocument,
  type LegalDocumentName,
  type LegalLanguage,
  legalDocumentNames,
  legalLanguages,
  parseLegalDocument,
} from '../src/legal/document.ts';

export const minimumReviewRecords = 2;

export interface LegalTextSource {
  readonly language: LegalLanguage;
  readonly name: LegalDocumentName;
  readonly source: string | undefined;
}

export interface LegalTextContext {
  readonly deploymentExists: boolean;
  readonly today: string;
  readonly reviewRecordIdentity: (path: string) => string | null;
}

const htmlPattern = /<[a-zA-Z/!?]/;
const headingPattern = /^#{1,6}\s/m;
const codeBlockPattern = /^\s*(```|~~~)/m;
const linkPattern = /\]\(([^)\s]*)[^)]*\)/g;
const draftMarkPattern = /\*\*(Черновик|Draft)\.\*\*|\[(проверить|verify)|\[[^\]]+\](?!\()/u;
const reviewFolder = 'knowledge/reviews/';

function pathOf(text: LegalTextSource): string {
  return `src/legal/${text.language}/${text.name}.md`;
}

function documentName(path: string): string {
  return posix.basename(path, '.md');
}

function authoringProblems(path: string, body: string): string[] {
  const problems: string[] = [];
  if (htmlPattern.test(body)) {
    problems.push(`${path}: raw HTML is not allowed`);
  }
  if (headingPattern.test(body)) {
    problems.push(`${path}: headings are not allowed; write a bold paragraph`);
  }
  if (codeBlockPattern.test(body)) {
    problems.push(`${path}: code blocks are not allowed`);
  }
  for (const match of body.matchAll(linkPattern)) {
    const target = match[1] ?? '';
    if (!target.startsWith('https://')) {
      problems.push(`${path}: the link "${target}" is not an https address`);
    }
  }
  return problems;
}

function reviewProblems(path: string, document: LegalDocument, context: LegalTextContext): string[] {
  const { metadata } = document;
  if (metadata.status !== 'reviewed' || metadata.reviewed === null) {
    return context.deploymentExists
      ? [`${path}: a deployment configuration exists, so the text must be reviewed, not ${metadata.status}`]
      : [];
  }
  const problems: string[] = [];
  if (draftMarkPattern.test(document.body)) {
    problems.push(`${path}: a reviewed text keeps no draft banner, check marks or bracketed placeholders`);
  }
  if (metadata.reviewed > context.today || metadata.reviewed < metadata.date) {
    problems.push(`${path}: the review date must lie between the text's date and today`);
  }
  const records = metadata.reviews.map((record) => posix.normalize(record));
  const identities = records.map((record) => context.reviewRecordIdentity(record));
  const distinct = new Set(identities.filter((identity) => identity !== null));
  if (distinct.size !== identities.filter((identity) => identity !== null).length) {
    problems.push(`${path}: a review record is listed twice`);
  }
  if (distinct.size < minimumReviewRecords) {
    problems.push(`${path}: a reviewed text lists at least ${String(minimumReviewRecords)} review records`);
  }
  for (const [index, record] of records.entries()) {
    const prefix = `${reviewFolder}legal-text-${documentName(path)}-`;
    const named = record.startsWith(prefix) && record.endsWith('.md') && !record.slice(prefix.length).includes('/');
    if (!named || record.includes('..') || identities[index] === null) {
      problems.push(
        `${path}: the review record ${record} is not an existing ${reviewFolder}legal-text-${documentName(path)}-…md file`,
      );
    }
  }
  return problems;
}

function documentProblems(
  text: LegalTextSource,
  context: LegalTextContext,
): {
  readonly document: LegalDocument | null;
  readonly problems: string[];
} {
  const path = pathOf(text);
  if (text.source === undefined) {
    return { document: null, problems: [`${path}: the text is missing`] };
  }
  const parsed = parseLegalDocument(text.source);
  if (parsed.kind === 'refused') {
    return { document: null, problems: parsed.problems.map((problem) => `${path}: ${problem}`) };
  }
  return {
    document: parsed.document,
    problems: [...authoringProblems(path, parsed.document.body), ...reviewProblems(path, parsed.document, context)],
  };
}

export function legalTextProblems(texts: readonly LegalTextSource[], context: LegalTextContext): string[] {
  const problems: string[] = [];
  for (const name of legalDocumentNames) {
    const checked = legalLanguages.map((language) => {
      const text = texts.find((candidate) => candidate.language === language && candidate.name === name);
      return documentProblems(text ?? { language, name, source: undefined }, context);
    });
    problems.push(...checked.flatMap((result) => result.problems));
    const [first, second] = checked.map((result) => result.document);
    if (
      first !== null &&
      first !== undefined &&
      second !== null &&
      second !== undefined &&
      (first.metadata.version !== second.metadata.version || first.metadata.date !== second.metadata.date)
    ) {
      problems.push(`src/legal/*/${name}.md: the Russian and English texts carry different versions or dates`);
    }
  }
  return problems;
}
