import { type Language, languages } from '../lib/i18n/languages.ts';

export const legalDocumentNames = ['privacy', 'consent', 'terms'] as const;

export type LegalDocumentName = (typeof legalDocumentNames)[number];

export const legalLanguages = languages;

export type LegalLanguage = Language;

export type LegalStatus = 'draft' | 'reviewed';

export interface LegalMetadata {
  readonly version: string;
  readonly date: string;
  readonly status: LegalStatus;
  readonly reviewed: string | null;
  readonly reviews: readonly string[];
}

export interface LegalDocument {
  readonly metadata: LegalMetadata;
  readonly body: string;
}

export type ParsedLegalDocument =
  | { readonly kind: 'parsed'; readonly document: LegalDocument }
  | { readonly kind: 'refused'; readonly problems: readonly string[] };

const fence = '---';
const versionPattern = /^\d{4}-\d{2}-\d{2}\.[1-9]\d*$/;
const maximumVersionLength = 32;
const datePattern = /^\d{4}-\d{2}-\d{2}$/;
const knownFields: ReadonlySet<string> = new Set(['version', 'date', 'status', 'reviewed', 'reviews']);

export function isIsoDate(value: string): boolean {
  if (!datePattern.test(value)) {
    return false;
  }
  const parsed = new Date(`${value}T00:00:00Z`);
  return !Number.isNaN(parsed.getTime()) && parsed.toISOString().startsWith(value);
}

function fieldsOf(lines: readonly string[]): { readonly fields: Map<string, string>; readonly problems: string[] } {
  const fields = new Map<string, string>();
  const problems: string[] = [];
  for (const line of lines) {
    const separator = line.indexOf(':');
    if (separator <= 0) {
      problems.push(`the front matter line "${line}" is not a field`);
      continue;
    }
    const name = line.slice(0, separator).trim();
    const value = line.slice(separator + 1).trim();
    if (!knownFields.has(name)) {
      problems.push(`the field "${name}" is unknown`);
    } else if (fields.has(name)) {
      problems.push(`the field "${name}" is repeated`);
    } else {
      fields.set(name, value);
    }
  }
  return { fields, problems };
}

function statusOf(value: string | undefined): LegalStatus | null {
  return value === 'draft' || value === 'reviewed' ? value : null;
}

function reviewsOf(value: string): string[] {
  return value
    .split(',')
    .map((path) => path.trim())
    .filter((path) => path !== '');
}

function metadataOf(fields: ReadonlyMap<string, string>): {
  readonly metadata: LegalMetadata | null;
  readonly problems: string[];
} {
  const problems: string[] = [];
  const version = fields.get('version') ?? '';
  const date = fields.get('date') ?? '';
  const status = statusOf(fields.get('status'));
  const reviewed = fields.get('reviewed') ?? null;
  const reviews = fields.get('reviews');
  if (!versionPattern.test(version) || version.length > maximumVersionLength || !version.startsWith(`${date}.`)) {
    problems.push("the version must read YYYY-MM-DD.n, starting with the text's date");
  }
  if (!isIsoDate(date)) {
    problems.push('the date must be an ISO date');
  }
  if (status === null) {
    problems.push('the status must be draft or reviewed');
  }
  if (status === 'draft' && (reviewed !== null || reviews !== undefined)) {
    problems.push('a draft carries no review fields');
  }
  if (status === 'reviewed' && (reviewed === null || !isIsoDate(reviewed))) {
    problems.push('a reviewed text needs the ISO date of its review');
  }
  if (status === 'reviewed' && (reviews === undefined || reviewsOf(reviews).length === 0)) {
    problems.push('a reviewed text lists its review records');
  }
  if (problems.length > 0 || status === null) {
    return { metadata: null, problems };
  }
  return {
    metadata: { version, date, status, reviewed, reviews: reviews === undefined ? [] : reviewsOf(reviews) },
    problems,
  };
}

export function parseLegalDocument(source: string): ParsedLegalDocument {
  const lines = source.split('\n');
  const closing = lines.indexOf(fence, 1);
  if (lines[0] !== fence || closing < 0) {
    return { kind: 'refused', problems: ['the text does not start with a front matter block'] };
  }
  const { fields, problems: fieldProblems } = fieldsOf(lines.slice(1, closing));
  const { metadata, problems } = metadataOf(fields);
  const all = [...fieldProblems, ...problems];
  if (metadata === null || all.length > 0) {
    return { kind: 'refused', problems: all };
  }
  return {
    kind: 'parsed',
    document: {
      metadata,
      body: lines
        .slice(closing + 1)
        .join('\n')
        .trim(),
    },
  };
}
