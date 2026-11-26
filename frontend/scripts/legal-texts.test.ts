import { describe, expect, test } from 'vitest';

import { legalDocumentNames, legalLanguages } from '../src/legal/document.ts';
import { type LegalTextContext, type LegalTextSource, legalTextProblems } from './legal-texts.ts';

const draft = [
  '---',
  'version: 2026-11-27.1',
  'date: 2026-11-27',
  'status: draft',
  '---',
  '',
  '**Общие положения.**',
  '',
].join('\n');

function records(name: string, count: number): string {
  return Array.from(
    { length: count },
    (_, index) => `knowledge/reviews/legal-text-${name}-2026-11-30-${String(index + 1)}.md`,
  ).join(', ');
}

function reviewed(reviewedOn: string, reviews: string): string {
  return [
    '---',
    'version: 2026-11-27.1',
    'date: 2026-11-27',
    'status: reviewed',
    `reviewed: ${reviewedOn}`,
    `reviews: ${reviews}`,
    '---',
    '',
    'Текст.',
  ].join('\n');
}

function allTexts(source: string | ((name: string) => string)): LegalTextSource[] {
  return legalLanguages.flatMap((language) =>
    legalDocumentNames.map((name) => ({ language, name, source: typeof source === 'string' ? source : source(name) })),
  );
}

function withText(texts: readonly LegalTextSource[], replaced: LegalTextSource): LegalTextSource[] {
  return texts.map((text) => (text.language === replaced.language && text.name === replaced.name ? replaced : text));
}

function context(deploymentExists: boolean): LegalTextContext {
  return {
    deploymentExists,
    today: '2026-12-01',
    reviewRecordIdentity: (path) =>
      path.startsWith('knowledge/reviews/legal-text-') && !path.includes('missing') ? path.toLowerCase() : null,
  };
}

describe('the legal text check', () => {
  test('passes six draft texts while no deployment configuration exists', () => {
    const texts = allTexts(draft);

    const problems = legalTextProblems(texts, context(false));

    expect(problems).toEqual([]);
  });

  test('refuses every draft text once a deployment configuration exists', () => {
    const texts = allTexts(draft);

    const problems = legalTextProblems(texts, context(true));

    expect(problems).toHaveLength(6);
    expect(problems[0]).toContain('a deployment configuration exists');
  });

  test('passes reviewed texts with a valid review date and two existing records once deployed', () => {
    const texts = allTexts((name) => reviewed('2026-11-30', records(name, 2)));

    const problems = legalTextProblems(texts, context(true));

    expect(problems).toEqual([]);
  });

  test('refuses a reviewed text with a bad review date or records that are too few, repeated, foreign or missing', () => {
    const variants: ((name: string) => string)[] = [
      (name: string): string => reviewed('2026-12-02', records(name, 2)),
      (name: string): string => reviewed('2026-11-26', records(name, 2)),
      (name: string): string => reviewed('2026-11-30', records(name, 1)),
      (name: string): string => reviewed('2026-11-30', `${records(name, 1)}, ${records(name, 1)}`),
      (name: string): string =>
        reviewed('2026-11-30', `${records(name, 1)}, knowledge/reviews/../../frontend/package.json`),
      (name: string): string =>
        reviewed('2026-11-30', `${records(name, 1)}, knowledge/reviews/legal-text-other-2026-11-30-1.md`),
      (name: string): string =>
        reviewed('2026-11-30', `${records(name, 1)}, knowledge/reviews/legal-text-${name}-missing.md`),
    ];

    const results = variants.map((variant) => legalTextProblems(allTexts(variant), context(true)));

    expect(results.map((problems) => problems.length > 0)).toEqual([true, true, true, true, true, true, true]);
  });

  test('refuses a reviewed text that keeps its draft banner, a check mark or a placeholder', () => {
    const bodies = ['**Черновик.** Текст.', 'Текст. [проверить: статья]', 'Оператор: [ФИО].'];

    const results = bodies.map((body) =>
      legalTextProblems(
        allTexts((name) => `${reviewed('2026-11-30', records(name, 2))}\n${body}`),
        context(true),
      ),
    );

    expect(results.map((problems) => problems.length > 0)).toEqual([true, true, true]);
  });

  test('passes a reviewed text whose https link has bracketed link text', () => {
    const texts = allTexts(
      (name) => `${reviewed('2026-11-30', records(name, 2))}\nСм. [официальный текст закона](https://example.org/law).`,
    );

    const problems = legalTextProblems(texts, context(true));

    expect(problems).toEqual([]);
  });

  test('refuses an unclosed check mark and two records that differ only in letter case', () => {
    const unclosed = allTexts((name) => `${reviewed('2026-11-30', records(name, 2))}\n[проверить статью`);
    const sameFile = allTexts((name) =>
      reviewed('2026-11-30', `knowledge/reviews/legal-text-${name}-a.md, knowledge/reviews/legal-text-${name}-A.md`),
    );

    const results = [unclosed, sameFile].map((texts) => legalTextProblems(texts, context(true)));

    expect(results[0]?.some((problem) => problem.includes('keeps no draft banner'))).toBe(true);
    expect(results[1]?.some((problem) => problem.includes('listed twice'))).toBe(true);
  });

  test('refuses review fields on a draft and a reviewed status without them', () => {
    const draftWithDate = draft.replace('status: draft', 'status: draft\nreviewed: 2026-11-30');
    const reviewedWithoutFields = draft.replace('status: draft', 'status: reviewed');
    const draftWithRecords = draft.replace('status: draft', 'status: draft\nreviews: knowledge/reviews/a.md');

    const results = [draftWithDate, reviewedWithoutFields, draftWithRecords].map((source) =>
      legalTextProblems(withText(allTexts(draft), { language: 'ru', name: 'privacy', source }), context(false)),
    );

    expect(results[0]).toContain('src/legal/ru/privacy.md: a draft carries no review fields');
    expect(results[1]?.some((problem) => problem.includes('needs the ISO date of its review'))).toBe(true);
    expect(results[2]).toContain('src/legal/ru/privacy.md: a draft carries no review fields');
  });

  test('refuses a missing text, an unknown or malformed field and a version mismatch between languages', () => {
    const base = allTexts(draft);
    const missing = withText(base, { language: 'en', name: 'terms', source: undefined });
    const unknown = withText(base, {
      language: 'ru',
      name: 'terms',
      source: draft.replace('status: draft', 'status: draft\nauthor: x'),
    });
    const malformed = withText(base, {
      language: 'ru',
      name: 'consent',
      source: draft.replace('2026-11-27.1', '2026-11'),
    });
    const mismatch = withText(base, {
      language: 'en',
      name: 'privacy',
      source: draft.replace('2026-11-27.1', '2026-11-27.2'),
    });

    const dateMismatch = withText(base, {
      language: 'en',
      name: 'consent',
      source: draft
        .replace('version: 2026-11-27.1', 'version: 2026-11-28.1')
        .replace('date: 2026-11-27', 'date: 2026-11-28'),
    });
    const foreignVersion = withText(base, {
      language: 'ru',
      name: 'privacy',
      source: draft.replace('2026-11-27.1', '2026-11-20.1'),
    });

    const results = [missing, unknown, malformed, mismatch, dateMismatch, foreignVersion].map((texts) =>
      legalTextProblems(texts, context(false)),
    );

    expect(results[0]).toContain('src/legal/en/terms.md: the text is missing');
    expect(results[1]).toContain('src/legal/ru/terms.md: the field "author" is unknown');
    expect(results[2]).toContain(
      "src/legal/ru/consent.md: the version must read YYYY-MM-DD.n, starting with the text's date",
    );
    expect(results[3]).toContain(
      'src/legal/*/privacy.md: the Russian and English texts carry different versions or dates',
    );
    expect(results[4]).toContain(
      'src/legal/*/consent.md: the Russian and English texts carry different versions or dates',
    );
    expect(results[5]).toContain(
      "src/legal/ru/privacy.md: the version must read YYYY-MM-DD.n, starting with the text's date",
    );
  });

  test('refuses raw HTML, a heading, a code block and a link that is not https', () => {
    const bodies = ['<b>Жирный</b>', '# Заголовок', '```\nкод\n```', '[согласие](/consent)'];

    const results = bodies.map((body) =>
      legalTextProblems(
        withText(allTexts(draft), { language: 'ru', name: 'privacy', source: `${draft}${body}\n` }),
        context(false),
      ),
    );

    expect(results.map((problems) => problems.length)).toEqual([1, 1, 1, 1]);
  });
});
